package balancer

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/cooldown"
	"github.com/nomand-zc/lumin-acpool/resolver"
	"github.com/nomand-zc/lumin-acpool/selector"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-acpool/usagetracker"
	"github.com/nomand-zc/lumin-client/usagerule"
)

// defaultBalancer is the default implementation of the Balancer interface.
type defaultBalancer struct {
	opts Options
}

// New creates a load balancer instance.
func New(opts ...Option) (Balancer, error) {
	o := defaultOptions
	for _, opt := range opts {
		opt(&o)
	}

	// Validate required dependencies: AccountStorage is always needed (for ReportSuccess/ReportFailure)
	if o.AccountStorage == nil {
		return nil, fmt.Errorf("balancer: AccountStorage is required")
	}

	// 当配置了 CooldownManager 但未提供 UsageTracker 时，自动创建一个内置冷却回调的实例。
	// 配额达到安全阈值时主动将账号标记为 CoolingDown，Resolver 查 Status=Available 时自然排除。
	// 注意：若上层已提供 UsageTracker，需在创建时自行通过 WithCallback 配置冷却回调。
	if o.CooldownManager != nil && o.UsageTracker == nil {
		o.UsageTracker = newUsageTrackerWithCooldown(o.AccountStorage, o.CooldownManager)
	}

	// If Resolver is not set, use the default Storage-based implementation (requires ProviderStorage)
	if o.Resolver == nil {
		if o.ProviderStorage == nil {
			return nil, fmt.Errorf("balancer: either Resolver or ProviderStorage is required")
		}
		o.Resolver = resolver.NewStorageResolver(o.ProviderStorage, o.AccountStorage)
	}

	return &defaultBalancer{opts: o}, nil
}

// Pick performs a single selection.
func (b *defaultBalancer) Pick(ctx context.Context, req *PickRequest) (*PickResult, error) {
	if req.Model == "" {
		return nil, ErrModelRequired
	}

	// Build SelectRequest (to be passed to Selector/GroupSelector)
	selReq := &selector.SelectRequest{
		UserID:      req.UserID,
		Model:       req.Model,
		ProviderKey: req.ProviderKey,
		Tags:        req.Tags,
	}

	// Determine the maximum retry count
	maxRetries := req.MaxRetries
	if maxRetries <= 0 {
		maxRetries = b.opts.DefaultMaxRetries
	}

	enableFailover := req.EnableFailover || b.opts.DefaultEnableFailover

	// Determine dispatch mode based on the three states of ProviderKey
	switch {
	case selReq.IsExactProvider():
		return b.pickExact(ctx, selReq, maxRetries)
	default:
		return b.pickAuto(ctx, selReq, maxRetries, enableFailover)
	}
}

// pickExact handles Mode 1: exact provider specification.
func (b *defaultBalancer) pickExact(ctx context.Context, selReq *selector.SelectRequest, maxRetries int) (*PickResult, error) {
	// Resolve provider via Resolver (unified validation of existence, active status, and model support)
	provInfo, err := b.opts.Resolver.ResolveProvider(ctx, *selReq.ProviderKey, selReq.Model)
	if err != nil {
		switch {
		case errors.Is(err, resolver.ErrProviderNotFound):
			return nil, ErrProviderNotFound
		case errors.Is(err, resolver.ErrProviderInactive):
			return nil, ErrNoAvailableProvider
		case errors.Is(err, resolver.ErrModelNotSupported):
			return nil, ErrModelNotSupported
		default:
			return nil, fmt.Errorf("balancer: resolve provider: %w", err)
		}
	}

	// Select an account from this provider (with retry)
	return b.selectAccountFromProvider(ctx, provInfo, selReq, maxRetries)
}

// pickAuto handles Mode 2/3: selection by type or fully automatic.
func (b *defaultBalancer) pickAuto(ctx context.Context, selReq *selector.SelectRequest, maxRetries int, enableFailover bool) (*PickResult, error) {
	// Resolve candidate provider list via Resolver
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

	// Exclude already-tried providers (failover scenario)
	var excludeProviderKeys []account.ProviderKey

	for {
		// 检查 context 是否已取消，避免 failover 循环长时间阻塞
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Filter out excluded providers
		filtered := filterProviders(candidates, excludeProviderKeys)
		if len(filtered) == 0 {
			return nil, ErrNoAvailableProvider
		}

		// Use GroupSelector to select a provider
		chosen, err := b.opts.GroupSelector.Select(filtered, selReq)
		if err != nil {
			return nil, fmt.Errorf("balancer: group select: %w", err)
		}

		// Select an account from this provider
		result, err := b.selectAccountFromProvider(ctx, chosen, selReq, maxRetries)
		if err == nil {
			return result, nil
		}

		// If selection failed and failover is enabled, exclude this provider and retry
		if enableFailover && (errors.Is(err, ErrNoAvailableAccount) || errors.Is(err, ErrMaxRetriesExceeded) || errors.Is(err, ErrOccupancyFull)) {
			excludeProviderKeys = append(excludeProviderKeys, chosen.ProviderKey())
			// Reset ExcludeAccountIDs (no need to exclude accounts from previous provider after switching)
			selReq.ExcludeAccountIDs = nil
			continue
		}

		return nil, err
	}
}

// selectAccountFromProvider selects an account from the specified provider (with retry).
func (b *defaultBalancer) selectAccountFromProvider(
	ctx context.Context,
	provInfo *account.ProviderInfo,
	selReq *selector.SelectRequest,
	maxRetries int,
) (*PickResult, error) {
	// Save the original ExcludeAccountIDs
	originalExclude := selReq.ExcludeAccountIDs

	for i := 0; i <= maxRetries; i++ {
		// Resolve available accounts under this provider via Resolver
		accounts, err := b.opts.Resolver.ResolveAccounts(ctx, resolver.ResolveAccountsRequest{
			Key:        provInfo.ProviderKey(),
			Tags:       selReq.Tags,
			ExcludeIDs: selReq.ExcludeAccountIDs,
		})
		if err != nil {
			selReq.ExcludeAccountIDs = originalExclude
			return nil, fmt.Errorf("balancer: resolve accounts: %w", err)
		}

		if len(accounts) == 0 {
			// Restore the original exclude list
			selReq.ExcludeAccountIDs = originalExclude
			return nil, ErrNoAvailableAccount
		}

		// 占用过滤：排除已达并发上限的账号
		accounts = b.opts.OccupancyController.FilterAvailable(ctx, accounts)
		if len(accounts) == 0 {
			selReq.ExcludeAccountIDs = originalExclude
			return nil, ErrOccupancyFull
		}

		// Use Selector to select an account
		chosen, err := b.opts.Selector.Select(accounts, selReq)
		if err != nil {
			if errors.Is(err, selector.ErrEmptyCandidates) || errors.Is(err, selector.ErrNoAvailableAccount) {
				selReq.ExcludeAccountIDs = originalExclude
				return nil, ErrNoAvailableAccount
			}
			selReq.ExcludeAccountIDs = originalExclude
			return nil, fmt.Errorf("balancer: select account: %w", err)
		}

		// 占用获取：原子操作确保竞态安全
		if !b.opts.OccupancyController.Acquire(ctx, chosen) {
			// 竞态失败（FilterAvailable 通过但 Acquire 时已被其他请求占满），排除后重试
			selReq.ExcludeAccountIDs = append(selReq.ExcludeAccountIDs, chosen.ID)
			continue
		}

		// Update LastUsedAt via StatsStore
		now := time.Now()
		if b.opts.StatsStore != nil {
			if err := b.opts.StatsStore.UpdateLastUsed(ctx, chosen.ID, now); err != nil {
				// UpdateLastUsed 失败，需释放已获取的占用槽位
				b.opts.OccupancyController.Release(ctx, chosen.ID)
				selReq.ExcludeAccountIDs = append(selReq.ExcludeAccountIDs, chosen.ID)
				continue
			}
		}

		// Restore the exclude list
		selReq.ExcludeAccountIDs = originalExclude

		return &PickResult{
			Account:     deepCopyAccount(chosen),
			ProviderKey: provInfo.ProviderKey(),
			Attempts:    i,
		}, nil
	}

	// 重试次数耗尽
	selReq.ExcludeAccountIDs = originalExclude
	return nil, ErrMaxRetriesExceeded
}

// ReportSuccess reports a successful call.
func (b *defaultBalancer) ReportSuccess(ctx context.Context, accountID string) error {
	// 0. 释放占用槽位（Pick 时 Acquire 的对称操作）
	b.opts.OccupancyController.Release(ctx, accountID)

	// 1. 更新运行时统计（通过 StatsStore 原子操作）
	if b.opts.StatsStore != nil {
		if err := b.opts.StatsStore.IncrSuccess(ctx, accountID); err != nil {
			return fmt.Errorf("balancer: incr success stats: %w", err)
		}
	}

	// 2. 记录用量到 UsageTracker（成功请求计为 1 次）
	if b.opts.UsageTracker != nil {
		if err := b.opts.UsageTracker.RecordUsage(ctx, accountID, usagerule.SourceTypeRequest, 1.0); err != nil {
			return fmt.Errorf("balancer: record usage: %w", err)
		}
	}

	// 3. 通知熔断器
	if b.opts.CircuitBreaker != nil {
		acct, err := b.opts.AccountStorage.Get(ctx, accountID)
		if err != nil {
			if errors.Is(err, storage.ErrNotFound) {
				return ErrAccountNotFound
			}
			return fmt.Errorf("balancer: get account: %w", err)
		}

		if err := b.opts.CircuitBreaker.RecordSuccess(ctx, acct); err != nil {
			return fmt.Errorf("balancer: circuit breaker record success: %w", err)
		}

		// 仅当状态需要变更时才持久化 Account（使用乐观锁避免竞态覆盖）
		if acct.Status == account.StatusCircuitOpen {
			acct.Status = account.StatusAvailable
			acct.CircuitOpenUntil = nil
			acct.UpdatedAt = time.Now()
			if err := b.opts.AccountStorage.Update(ctx, acct); err != nil {
				if errors.Is(err, storage.ErrVersionConflict) {
					// 版本冲突，已被其他实例更新，忽略（幂等）
					return nil
				}
				return fmt.Errorf("balancer: update account: %w", err)
			}
		}
	}

	return nil
}

// ReportFailure reports a failed call.
func (b *defaultBalancer) ReportFailure(ctx context.Context, accountID string, callErr error) error {
	// 0. 释放占用槽位（Pick 时 Acquire 的对称操作）
	b.opts.OccupancyController.Release(ctx, accountID)

	// 1. 更新运行时统计（通过 StatsStore 原子操作）
	errMsg := ""
	if callErr != nil {
		errMsg = callErr.Error()
	}
	var consecutiveFailures int
	if b.opts.StatsStore != nil {
		var err error
		consecutiveFailures, err = b.opts.StatsStore.IncrFailure(ctx, accountID, errMsg)
		if err != nil {
			return fmt.Errorf("balancer: incr failure stats: %w", err)
		}
	}

	// 2. 记录用量到 UsageTracker（无论成功失败，请求已发出即消耗配额）
	if b.opts.UsageTracker != nil {
		_ = b.opts.UsageTracker.RecordUsage(ctx, accountID, usagerule.SourceTypeRequest, 1.0)
	}

	// 3. 判断是否需要变更 Account 状态
	needUpdate := false

	acct, err := b.opts.AccountStorage.Get(ctx, accountID)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return ErrAccountNotFound
		}
		return fmt.Errorf("balancer: get account: %w", err)
	}

	// 4. 检查是否是限流错误，优先冷却处理
	if isRateLimitError(callErr) {
		// Double check: 通过 UsageTracker 标记对应规则已耗尽
		if b.opts.UsageTracker != nil {
			_ = b.opts.UsageTracker.CalibrateFromResponse(ctx, accountID, usagerule.SourceTypeRequest)
		}

		if b.opts.CooldownManager != nil {
			retryAfter := extractRetryAfter(callErr)
			b.opts.CooldownManager.StartCooldown(acct, retryAfter)
			acct.Status = account.StatusCoolingDown
			needUpdate = true
		}
	} else if b.opts.CircuitBreaker != nil {
		// 非限流错误，通知熔断器
		tripped, cbErr := b.opts.CircuitBreaker.RecordFailure(ctx, acct, consecutiveFailures)
		if cbErr != nil {
			return fmt.Errorf("balancer: circuit breaker record failure: %w", cbErr)
		}
		if tripped {
			acct.Status = account.StatusCircuitOpen
			needUpdate = true
		}
	}

	// 5. 仅当状态变更时才持久化（使用乐观锁避免竞态覆盖）
	if needUpdate {
		acct.UpdatedAt = time.Now()
		if err := b.opts.AccountStorage.Update(ctx, acct); err != nil {
			if errors.Is(err, storage.ErrVersionConflict) {
				// 版本冲突，已被其他实例更新（如已被标记为 CoolingDown/CircuitOpen），忽略
				return nil
			}
			return fmt.Errorf("balancer: update account: %w", err)
		}
	}

	return nil
}

// --- Helper functions ---

// filterProviders filters out excluded providers.
func filterProviders(candidates []*account.ProviderInfo, excludeKeys []account.ProviderKey) []*account.ProviderInfo {
	if len(excludeKeys) == 0 {
		return candidates
	}
	var result []*account.ProviderInfo
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

// deepCopyAccount creates a deep copy of an account object.
func deepCopyAccount(src *account.Account) *account.Account {
	return src.Clone()
}

// rateLimitError is the rate limit error interface.
// Error types implementing this interface will be recognized as rate limit errors by the balancer.
type rateLimitError interface {
	IsRateLimit() bool
}

// httpStatusError is the HTTP status code error interface.
// Used to extract status codes from HTTP errors to determine rate limiting (429).
type httpStatusError interface {
	StatusCode() int
}

// retryAfterError is the error interface carrying retry-after time.
type retryAfterError interface {
	RetryAfter() *time.Time
}

// isRateLimitError checks whether the error is a rate limit error.
func isRateLimitError(err error) bool {
	if err == nil {
		return false
	}

	// Check rateLimitError interface first
	var rle rateLimitError
	if errors.As(err, &rle) {
		return rle.IsRateLimit()
	}

	// Then check for HTTP status code 429
	var hse httpStatusError
	if errors.As(err, &hse) {
		return hse.StatusCode() == http.StatusTooManyRequests
	}

	return false
}

// extractRetryAfter extracts the retry-after time from the error.
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

// newUsageTrackerWithCooldown 创建一个内置冷却回调的 UsageTracker。
// 当 RecordUsage 检测到配额达到安全阈值时，自动触发冷却机制：
//   - 将账号状态设置为 CoolingDown
//   - 根据规则窗口剩余时间计算冷却时长
//   - 持久化到 AccountStorage
func newUsageTrackerWithCooldown(
	accountStorage storage.AccountStorage,
	cooldownMgr cooldown.CooldownManager,
) usagetracker.UsageTracker {
	return usagetracker.NewUsageTracker(
		usagetracker.WithCallback(func(ctx context.Context, accountID string, rule *usagerule.UsageRule) {
			acct, err := accountStorage.Get(ctx, accountID)
			if err != nil {
				return // 获取失败静默忽略，不影响主流程
			}

			// 仅对 Available 状态的账号触发冷却
			if acct.Status != account.StatusAvailable {
				return
			}

			// 触发冷却
			cooldownMgr.StartCooldown(acct, nil)
			acct.Status = account.StatusCoolingDown
			acct.UpdatedAt = time.Now()
			_ = accountStorage.Update(ctx, acct) // 乐观锁持久化，冲突静默忽略
		}),
	)
}
