package scheduler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/filtercond"
	"github.com/nomand-zc/lumin-acpool/provider"
	"github.com/nomand-zc/lumin-acpool/selector"

	"github.com/nomand-zc/lumin-acpool/storage"
)

// defaultScheduler Scheduler 接口的默认实现
type defaultScheduler struct {
	opts Options
}

// New 创建调度器实例
func New(opts ...Option) (Scheduler, error) {
	o := defaultOptions
	for _, opt := range opts {
		opt(&o)
	}

	// 校验必填依赖
	if o.AccountStorage == nil {
		return nil, fmt.Errorf("scheduler: AccountStorage is required")
	}
	if o.ProviderStorage == nil {
		return nil, fmt.Errorf("scheduler: ProviderStorage is required")
	}

	return &defaultScheduler{opts: o}, nil
}

// Schedule 执行一次调度
func (s *defaultScheduler) Schedule(ctx context.Context, req *ScheduleRequest) (*ScheduleResult, error) {
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
		maxRetries = s.opts.DefaultMaxRetries
	}

	enableFailover := req.EnableFailover || s.opts.DefaultEnableFailover

	// 根据 ProviderKey 的三态决定调度模式
	switch {
	case selReq.IsExactProvider():
		return s.scheduleExact(ctx, selReq, maxRetries)
	default:
		return s.scheduleAuto(ctx, selReq, maxRetries, enableFailover)
	}
}

// scheduleExact 模式 1: 精确指定供应商
func (s *defaultScheduler) scheduleExact(ctx context.Context, selReq *selector.SelectRequest, maxRetries int) (*ScheduleResult, error) {
	// 验证供应商存在
	provInfo, err := s.opts.ProviderStorage.Get(ctx, *selReq.ProviderKey)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, ErrProviderNotFound
		}
		return nil, fmt.Errorf("scheduler: get provider: %w", err)
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
	return s.selectAccountFromProvider(ctx, provInfo, selReq, maxRetries)
}

// scheduleAuto 模式 2/3: 按类型或全自动选择
func (s *defaultScheduler) scheduleAuto(ctx context.Context, selReq *selector.SelectRequest, maxRetries int, enableFailover bool) (*ScheduleResult, error) {
	// 获取候选供应商列表
	candidates, err := s.getCandidateProviders(ctx, selReq)
	if err != nil {
		return nil, err
	}

	if len(candidates) == 0 {
		return nil, ErrModelNotSupported
	}

	// 排除已尝试的供应商（故障转移场景）
	var excludeProviderKeys []provider.ProviderKey

	for {
		// 过滤掉已排除的供应商
		filtered := s.filterProviders(candidates, excludeProviderKeys)
		if len(filtered) == 0 {
			return nil, ErrNoAvailableProvider
		}

		// 使用 GroupSelector 选供应商
		chosen, err := s.opts.GroupSelector.Select(filtered, selReq)
		if err != nil {
			return nil, fmt.Errorf("scheduler: group select: %w", err)
		}

		// 从该供应商下选号
		result, err := s.selectAccountFromProvider(ctx, chosen, selReq, maxRetries)
		if err == nil {
			return result, nil
		}

		// 如果选号失败且启用了故障转移，排除该供应商后重试
		if enableFailover && (errors.Is(err, ErrNoAvailableAccount) || errors.Is(err, ErrMaxRetriesExceeded)) {
			excludeProviderKeys = append(excludeProviderKeys, chosen.Key)
			// 重置 ExcludeAccountIDs（换供应商后不需要排除之前供应商的账号）
			selReq.ExcludeAccountIDs = nil
			continue
		}

		return nil, err
	}
}

// getCandidateProviders 获取候选供应商列表
func (s *defaultScheduler) getCandidateProviders(ctx context.Context, selReq *selector.SelectRequest) ([]*provider.ProviderInfo, error) {
	// 构建过滤条件：只要活跃的供应商
	statusFilter := filtercond.In(storage.ProviderFieldStatus, int(provider.ProviderStatusActive), int(provider.ProviderStatusDegraded))

	if selReq.IsProviderTypeOnly() {
		// 模式 2: 按类型筛选 + 活跃状态 + 支持指定模型
		filter := filtercond.And(
			filtercond.Equal(storage.ProviderFieldType, selReq.ProviderKey.Type),
			statusFilter,
			filtercond.Equal(storage.ProviderFieldSupportedModel, selReq.Model),
		)
		candidates, err := s.opts.ProviderStorage.Search(ctx, filter)
		if err != nil {
			return nil, fmt.Errorf("scheduler: search providers by type: %w", err)
		}
		return candidates, nil
	}

	// 模式 3: 全自动，按模型 + 活跃状态查找
	filter := filtercond.And(
		filtercond.Equal(storage.ProviderFieldSupportedModel, selReq.Model),
		statusFilter,
	)
	candidates, err := s.opts.ProviderStorage.Search(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("scheduler: search providers by model: %w", err)
	}
	return candidates, nil
}

// selectAccountFromProvider 从指定供应商下选号（带重试）
func (s *defaultScheduler) selectAccountFromProvider(
	ctx context.Context,
	provInfo *provider.ProviderInfo,
	selReq *selector.SelectRequest,
	maxRetries int,
) (*ScheduleResult, error) {
	// 保存原始的 ExcludeAccountIDs
	originalExclude := selReq.ExcludeAccountIDs

	for i := 0; i <= maxRetries; i++ {
		// 从存储中获取该供应商下可用的账号
		accounts, err := s.getAvailableAccounts(ctx, provInfo.Key, selReq)
		if err != nil {
			return nil, err
		}

		if len(accounts) == 0 {
			// 恢复原始排除列表
			selReq.ExcludeAccountIDs = originalExclude
			return nil, ErrNoAvailableAccount
		}

		// 使用 Selector 选账号
		chosen, err := s.opts.Selector.Select(accounts, selReq)
		if err != nil {
			if errors.Is(err, selector.ErrEmptyCandidates) || errors.Is(err, selector.ErrNoAvailableAccount) {
				selReq.ExcludeAccountIDs = originalExclude
				return nil, ErrNoAvailableAccount
			}
			return nil, fmt.Errorf("scheduler: select account: %w", err)
		}

		// 更新 LastUsedAt
		now := time.Now()
		chosen.LastUsedAt = &now
		chosen.UpdatedAt = now
		if err := s.opts.AccountStorage.Update(ctx, chosen); err != nil {
			return nil, fmt.Errorf("scheduler: update last used: %w", err)
		}

		// 恢复排除列表
		selReq.ExcludeAccountIDs = originalExclude

		return &ScheduleResult{
			Account:     deepCopyAccount(chosen),
			ProviderKey: provInfo.Key,
			Attempts:    i,
		}, nil
	}

	return nil, nil
}

// getAvailableAccounts 获取指定供应商下的可用账号
func (s *defaultScheduler) getAvailableAccounts(ctx context.Context, key provider.ProviderKey, selReq *selector.SelectRequest) ([]*account.Account, error) {
	// 只查指定供应商下可用状态的账号
	filter := filtercond.And(
		filtercond.Equal(storage.AccountFieldProviderType, key.Type),
		filtercond.Equal(storage.AccountFieldProviderName, key.Name),
		filtercond.Equal(storage.AccountFieldStatus, int(account.StatusAvailable)),
	)

	accounts, err := s.opts.AccountStorage.Search(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("scheduler: search accounts: %w", err)
	}

	// 过滤掉 ExcludeAccountIDs 中的账号
	if len(selReq.ExcludeAccountIDs) > 0 {
		accounts = filterExcluded(accounts, selReq.ExcludeAccountIDs)
	}

	// 按标签过滤
	if len(selReq.Tags) > 0 {
		accounts = filterByTags(accounts, selReq.Tags)
	}

	return accounts, nil
}

// ReportSuccess 上报调用成功
func (s *defaultScheduler) ReportSuccess(ctx context.Context, accountID string) error {
	acct, err := s.opts.AccountStorage.Get(ctx, accountID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return ErrAccountNotFound
		}
		return fmt.Errorf("scheduler: get account: %w", err)
	}

	// 更新统计
	acct.TotalCalls++
	acct.SuccessCalls++
	acct.ConsecutiveFailures = 0
	now := time.Now()
	acct.UpdatedAt = now

	// 通知熔断器
	if s.opts.CircuitBreaker != nil {
		s.opts.CircuitBreaker.RecordSuccess(acct)
	}

	// 持久化
	if err := s.opts.AccountStorage.Update(ctx, acct); err != nil {
		return fmt.Errorf("scheduler: update account: %w", err)
	}

	return nil
}

// ReportFailure 上报调用失败
func (s *defaultScheduler) ReportFailure(ctx context.Context, accountID string, callErr error) error {
	acct, err := s.opts.AccountStorage.Get(ctx, accountID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return ErrAccountNotFound
		}
		return fmt.Errorf("scheduler: get account: %w", err)
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
	if isRateLimitError(callErr) && s.opts.CooldownManager != nil {
		retryAfter := extractRetryAfter(callErr)
		s.opts.CooldownManager.StartCooldown(acct, retryAfter)
		acct.Status = account.StatusCoolingDown
	} else if s.opts.CircuitBreaker != nil {
		// 通知熔断器
		tripped := s.opts.CircuitBreaker.RecordFailure(acct)
		if tripped {
			acct.Status = account.StatusCircuitOpen
		}
	}

	// 持久化
	if err := s.opts.AccountStorage.Update(ctx, acct); err != nil {
		return fmt.Errorf("scheduler: update account: %w", err)
	}

	return nil
}

// --- 辅助函数 ---

// filterProviders 过滤掉已排除的供应商
func (s *defaultScheduler) filterProviders(candidates []*provider.ProviderInfo, excludeKeys []provider.ProviderKey) []*provider.ProviderInfo {
	if len(excludeKeys) == 0 {
		return candidates
	}
	var result []*provider.ProviderInfo
	for _, p := range candidates {
		excluded := false
		for _, ek := range excludeKeys {
			if p.Key.Type == ek.Type && p.Key.Name == ek.Name {
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

func filterExcluded(accounts []*account.Account, excludeIDs []string) []*account.Account {
	var result []*account.Account
	for _, acct := range accounts {
		if !slices.Contains(excludeIDs, acct.ID) {
			result = append(result, acct)
		}
	}
	return result
}

// filterByTags 按标签过滤账号（必须包含所有指定的标签键值对）
func filterByTags(accounts []*account.Account, tags map[string]string) []*account.Account {
	var result []*account.Account
	for _, acct := range accounts {
		if matchTags(acct.Tags, tags) {
			result = append(result, acct)
		}
	}
	return result
}

// matchTags 判断 accountTags 是否包含所有 requiredTags
func matchTags(accountTags, requiredTags map[string]string) bool {
	if len(requiredTags) == 0 {
		return true
	}
	if len(accountTags) == 0 {
		return false
	}
	for k, v := range requiredTags {
		if av, ok := accountTags[k]; !ok || av != v {
			return false
		}
	}
	return true
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
// 实现此接口的错误类型会被 scheduler 识别为限流错误
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
