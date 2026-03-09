package balancer

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/provider"
	"github.com/nomand-zc/lumin-acpool/resolver"
	"github.com/nomand-zc/lumin-acpool/selector"
	"github.com/nomand-zc/lumin-acpool/storage"
)

// defaultBalancer Balancer 接口的默认实现
type defaultBalancer struct {
	opts Options
}

// New 创建负载均衡器实例
func New(opts ...Option) (Balancer, error) {
	o := defaultOptions
	for _, opt := range opts {
		opt(&o)
	}

	// 校验必填依赖
	if o.AccountStorage == nil {
		return nil, fmt.Errorf("balancer: AccountStorage is required")
	}
	if o.ProviderStorage == nil {
		return nil, fmt.Errorf("balancer: ProviderStorage is required")
	}

	// 如果未设置 Resolver，使用基于 Storage 的默认实现
	if o.Resolver == nil {
		o.Resolver = resolver.NewStorageResolver(o.ProviderStorage, o.AccountStorage)
	}

	return &defaultBalancer{opts: o}, nil
}

// Pick 执行一次选取
func (b *defaultBalancer) Pick(ctx context.Context, req *PickRequest) (*PickResult, error) {
	if req.Model == "" {
		return nil, ErrModelRequired
	}

	// 构建 SelectRequest（后续传递给 Selector/GroupSelector）
	selReq := &selector.SelectRequest{
		Model:       req.Model,
		ProviderKey: req.ProviderKey,
		Tags:        req.Tags,
	}

	// 确定最大重试次数
	maxRetries := req.MaxRetries
	if maxRetries <= 0 {
		maxRetries = b.opts.DefaultMaxRetries
	}

	enableFailover := req.EnableFailover || b.opts.DefaultEnableFailover

	// 根据 ProviderKey 的三态决定调度模式
	switch {
	case selReq.IsExactProvider():
		return b.pickExact(ctx, selReq, maxRetries)
	default:
		return b.pickAuto(ctx, selReq, maxRetries, enableFailover)
	}
}

// pickExact 模式 1: 精确指定供应商
func (b *defaultBalancer) pickExact(ctx context.Context, selReq *selector.SelectRequest, maxRetries int) (*PickResult, error) {
	// 验证供应商存在
	provInfo, err := b.opts.ProviderStorage.Get(ctx, *selReq.ProviderKey)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, ErrProviderNotFound
		}
		return nil, fmt.Errorf("balancer: get provider: %w", err)
	}

	// 验证供应商是否支持该模型
	if !provInfo.SupportsModel(selReq.Model) {
		return nil, ErrModelNotSupported
	}

	// 验证供应商是否活跃
	if !provInfo.IsActive() {
		return nil, ErrNoAvailableProvider
	}

	// 从该供应商下选号（带重试）
	return b.selectAccountFromProvider(ctx, provInfo, selReq, maxRetries)
}

// pickAuto 模式 2/3: 按类型或全自动选择
func (b *defaultBalancer) pickAuto(ctx context.Context, selReq *selector.SelectRequest, maxRetries int, enableFailover bool) (*PickResult, error) {
	// 通过 Resolver 解析候选供应商列表
	providerType := ""
	if selReq.IsProviderTypeOnly() {
		providerType = selReq.ProviderKey.Type
	}
	candidates, err := b.opts.Resolver.ResolveProviders(ctx, selReq.Model, providerType)
	if err != nil {
		return nil, fmt.Errorf("balancer: resolve providers: %w", err)
	}

	if len(candidates) == 0 {
		return nil, ErrModelNotSupported
	}

	// 排除已尝试的供应商（故障转移场景）
	var excludeProviderKeys []provider.ProviderKey

	for {
		// 过滤掉已排除的供应商
		filtered := filterProviders(candidates, excludeProviderKeys)
		if len(filtered) == 0 {
			return nil, ErrNoAvailableProvider
		}

		// 使用 GroupSelector 选供应商
		chosen, err := b.opts.GroupSelector.Select(filtered, selReq)
		if err != nil {
			return nil, fmt.Errorf("balancer: group select: %w", err)
		}

		// 从该供应商下选号
		result, err := b.selectAccountFromProvider(ctx, chosen, selReq, maxRetries)
		if err == nil {
			return result, nil
		}

		// 如果选号失败且启用了故障转移，排除该供应商后重试
		if enableFailover && (errors.Is(err, ErrNoAvailableAccount) || errors.Is(err, ErrMaxRetriesExceeded)) {
			excludeProviderKeys = append(excludeProviderKeys, chosen.ProviderKey())
			// 重置 ExcludeAccountIDs（换供应商后不需要排除之前供应商的账号）
			selReq.ExcludeAccountIDs = nil
			continue
		}

		return nil, err
	}
}

// selectAccountFromProvider 从指定供应商下选号（带重试）
func (b *defaultBalancer) selectAccountFromProvider(
	ctx context.Context,
	provInfo *provider.ProviderInfo,
	selReq *selector.SelectRequest,
	maxRetries int,
) (*PickResult, error) {
	// 保存原始的 ExcludeAccountIDs
	originalExclude := selReq.ExcludeAccountIDs

	for i := 0; i <= maxRetries; i++ {
		// 通过 Resolver 解析该供应商下可用的账号
		accounts, err := b.opts.Resolver.ResolveAccounts(ctx, provInfo.ProviderKey(), selReq.Tags, selReq.ExcludeAccountIDs)
		if err != nil {
			return nil, fmt.Errorf("balancer: resolve accounts: %w", err)
		}

		if len(accounts) == 0 {
			// 恢复原始排除列表
			selReq.ExcludeAccountIDs = originalExclude
			return nil, ErrNoAvailableAccount
		}

		// 使用 Selector 选账号
		chosen, err := b.opts.Selector.Select(accounts, selReq)
		if err != nil {
			if errors.Is(err, selector.ErrEmptyCandidates) || errors.Is(err, selector.ErrNoAvailableAccount) {
				selReq.ExcludeAccountIDs = originalExclude
				return nil, ErrNoAvailableAccount
			}
			return nil, fmt.Errorf("balancer: select account: %w", err)
		}

		// 更新 LastUsedAt
		now := time.Now()
		chosen.LastUsedAt = &now
		chosen.UpdatedAt = now
		if err := b.opts.AccountStorage.Update(ctx, chosen); err != nil {
			return nil, fmt.Errorf("balancer: update last used: %w", err)
		}

		// 恢复排除列表
		selReq.ExcludeAccountIDs = originalExclude

		return &PickResult{
			Account:     deepCopyAccount(chosen),
			ProviderKey: provInfo.ProviderKey(),
			Attempts:    i,
		}, nil
	}

	return nil, nil
}

// ReportSuccess 上报调用成功
func (b *defaultBalancer) ReportSuccess(ctx context.Context, accountID string) error {
	acct, err := b.opts.AccountStorage.Get(ctx, accountID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return ErrAccountNotFound
		}
		return fmt.Errorf("balancer: get account: %w", err)
	}

	// 更新统计
	acct.TotalCalls++
	acct.SuccessCalls++
	acct.ConsecutiveFailures = 0
	now := time.Now()
	acct.UpdatedAt = now

	// 通知熔断器
	if b.opts.CircuitBreaker != nil {
		b.opts.CircuitBreaker.RecordSuccess(acct)
	}

	// 持久化
	if err := b.opts.AccountStorage.Update(ctx, acct); err != nil {
		return fmt.Errorf("balancer: update account: %w", err)
	}

	return nil
}

// ReportFailure 上报调用失败
func (b *defaultBalancer) ReportFailure(ctx context.Context, accountID string, callErr error) error {
	acct, err := b.opts.AccountStorage.Get(ctx, accountID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return ErrAccountNotFound
		}
		return fmt.Errorf("balancer: get account: %w", err)
	}

	// 更新统计
	acct.TotalCalls++
	acct.FailedCalls++
	acct.ConsecutiveFailures++
	now := time.Now()
	acct.LastErrorAt = &now
	acct.UpdatedAt = now
	if callErr != nil {
		acct.LastErrorMsg = callErr.Error()
	}

	// 判断是否为限流错误，优先处理冷却
	if isRateLimitError(callErr) && b.opts.CooldownManager != nil {
		retryAfter := extractRetryAfter(callErr)
		b.opts.CooldownManager.StartCooldown(acct, retryAfter)
		acct.Status = account.StatusCoolingDown
	} else if b.opts.CircuitBreaker != nil {
		// 通知熔断器
		tripped := b.opts.CircuitBreaker.RecordFailure(acct)
		if tripped {
			acct.Status = account.StatusCircuitOpen
		}
	}

	// 持久化
	if err := b.opts.AccountStorage.Update(ctx, acct); err != nil {
		return fmt.Errorf("balancer: update account: %w", err)
	}

	return nil
}

// --- 辅助函数 ---

// filterProviders 过滤掉已排除的供应商
func filterProviders(candidates []*provider.ProviderInfo, excludeKeys []provider.ProviderKey) []*provider.ProviderInfo {
	if len(excludeKeys) == 0 {
		return candidates
	}
	var result []*provider.ProviderInfo
	for _, p := range candidates {
		excluded := false
		for _, ek := range excludeKeys {
			if p.ProviderType == ek.Type && p.ProviderName == ek.Name {
				excluded = true
				break
			}
		}
		if !excluded {
			result = append(result, p)
		}
	}
	return result
}

// deepCopyAccount 深拷贝账号对象
func deepCopyAccount(src *account.Account) *account.Account {
	if src == nil {
		return nil
	}

	dst := *src

	// 拷贝 Tags
	if src.Tags != nil {
		dst.Tags = make(map[string]string, len(src.Tags))
		for k, v := range src.Tags {
			dst.Tags[k] = v
		}
	}

	// 拷贝 Metadata
	if src.Metadata != nil {
		dst.Metadata = make(map[string]any, len(src.Metadata))
		for k, v := range src.Metadata {
			dst.Metadata[k] = v
		}
	}

	// 拷贝时间指针
	if src.LastUsedAt != nil {
		t := *src.LastUsedAt
		dst.LastUsedAt = &t
	}
	if src.LastErrorAt != nil {
		t := *src.LastErrorAt
		dst.LastErrorAt = &t
	}
	if src.CooldownUntil != nil {
		t := *src.CooldownUntil
		dst.CooldownUntil = &t
	}
	if src.CircuitOpenUntil != nil {
		t := *src.CircuitOpenUntil
		dst.CircuitOpenUntil = &t
	}

	return &dst
}

// rateLimitError 限流错误接口
// 实现此接口的错误类型会被 balancer 识别为限流错误
type rateLimitError interface {
	IsRateLimit() bool
}

// httpStatusError HTTP 状态码错误接口
// 用于从 HTTP 错误中提取状态码判断是否为限流（429）
type httpStatusError interface {
	StatusCode() int
}

// retryAfterError 携带重试时间的错误接口
type retryAfterError interface {
	RetryAfter() *time.Time
}

// isRateLimitError 判断是否为限流错误
func isRateLimitError(err error) bool {
	if err == nil {
		return false
	}

	// 优先检查 rateLimitError 接口
	var rle rateLimitError
	if errors.As(err, &rle) {
		return rle.IsRateLimit()
	}

	// 其次检查 HTTP 状态码 429
	var hse httpStatusError
	if errors.As(err, &hse) {
		return hse.StatusCode() == http.StatusTooManyRequests
	}

	return false
}

// extractRetryAfter 从错误中提取重试时间
func extractRetryAfter(err error) *time.Time {
	if err == nil {
		return nil
	}

	var rae retryAfterError
	if errors.As(err, &rae) {
		return rae.RetryAfter()
	}

	return nil
}
