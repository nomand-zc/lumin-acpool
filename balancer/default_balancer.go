package balancer

import (
	"context"
	"errors"
	"fmt"
	rand "math/rand/v2"
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
		result, err := b.pickExact(ctx, selReq, maxRetries)
		if err == nil {
			return result, nil
		}

		// 精确供应商选号失败：若开启 failover 则降级到自动选号
		if enableFailover {
			fallbackReq := &selector.SelectRequest{
				UserID: selReq.UserID,
				Model:  selReq.Model,
				Tags:   selReq.Tags,
			}
			// 保留 ProviderKey.Type 作为类型约束（如果有），清除精确 Name
			if selReq.ProviderKey != nil && selReq.ProviderKey.Type != "" {
				fallbackReq.ProviderKey = &account.ProviderKey{Type: selReq.ProviderKey.Type}
			}

			fallbackResult, fallbackErr := b.pickAuto(ctx, fallbackReq, maxRetries, enableFailover)
			if fallbackErr != nil {
				// fallback 也失败，返回原始精确选号错误（更有诊断价值）
				return nil, err
			}
			fallbackResult.Fallback = true
			return fallbackResult, nil
		}

		return nil, err
	default:
		return b.pickAuto(ctx, selReq, maxRetries, enableFailover)
	}
}

// pickExact handles Mode 1: exact provider specification.
func (b *defaultBalancer) pickExact(ctx context.Context, selReq *selector.SelectRequest,
	maxRetries int) (*PickResult, error) {
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
// TODO: 改为并行批量探测来提升效率
func (b *defaultBalancer) pickAuto(ctx context.Context, selReq *selector.SelectRequest, maxRetries int,
	enableFailover bool) (*PickResult, error) {
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

	// 随机打散候选供应商列表，分散高并发下的热点竞争。
	// 对于 GroupPriority 等使用 sort.SliceStable 的策略，Shuffle 后同 Priority 的供应商顺序随机化，
	// 不影响 Priority 排序的语义（高优先级仍然优先），只是打破同优先级的确定性顺序。
	rand.Shuffle(len(candidates), func(i, j int) {
		candidates[i], candidates[j] = candidates[j], candidates[i]
	})

	// Exclude already-tried providers (failover scenario)
	var excludeProviderKeys []account.ProviderKey

	// 懒加载缓存：每个 Provider 的账号列表只查询一次，failover 切换 Provider 时直接复用。
	// 缓存生命周期仅限于本次 Pick 请求，不存在跨请求的状态过期问题。
	// FilterAvailable 每次仍实时执行（占用状态是动态的），只跳过重复的存储查询。
	providerAccountsCache := make(map[account.ProviderKey][]*account.Account)

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

		// 使用带缓存的版本替代 selectAccountFromProvider，避免 failover 时重复查询同一 Provider 的账号
		result, err := b.selectAccountFromProviderCached(ctx, chosen, selReq, maxRetries, providerAccountsCache)
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

// selectAccountFromProviderCached 带懒加载缓存的账号查询。
// 同一 Provider 在本次 Pick 请求生命周期内只执行一次 ResolveAccounts 存储查询，
// failover 时切换回同一 Provider 直接复用缓存，避免重复的 O(n) 存储查询开销。
// FilterAvailable 每次仍实时执行，确保占用状态的实时性。
func (b *defaultBalancer) selectAccountFromProviderCached(
	ctx context.Context,
	provInfo *account.ProviderInfo,
	selReq *selector.SelectRequest,
	maxRetries int,
	cache map[account.ProviderKey][]*account.Account,
) (*PickResult, error) {
	provKey := provInfo.ProviderKey()

	accounts, ok := cache[provKey]
	if !ok {
		// 首次访问该 Provider，查询账号列表并缓存
		var err error
		accounts, err = b.opts.Resolver.ResolveAccounts(ctx, resolver.ResolveAccountsRequest{
			Key:  provKey,
			Tags: selReq.Tags,
		})
		if err != nil {
			return nil, fmt.Errorf("balancer: resolve accounts: %w", err)
		}
		cache[provKey] = accounts
	}

	if len(accounts) == 0 {
		return nil, ErrNoAvailableAccount
	}

	// 占用过滤：每次实时执行，排除已达并发上限的账号（占用状态是动态的，不能缓存）
	available := b.opts.OccupancyController.FilterAvailable(ctx, accounts)
	if len(available) == 0 {
		return nil, ErrOccupancyFull
	}

	// 随机打散账号列表，分散竞争热点
	rand.Shuffle(len(available), func(i, j int) {
		available[i], available[j] = available[j], available[i]
	})

	// 委托 acquireFromAccounts 进行 Select + Acquire 重试循环
	return b.acquireFromAccounts(ctx, provInfo, available, selReq, maxRetries)
}

// acquireFromAccounts 从已过滤的账号列表中选取并获取占用槽位。
// 接收已完成 ResolveAccounts + FilterAvailable + Shuffle 的账号列表，
// 直接进行 Select + Acquire 重试循环，避免重复的存储查询开销。
// 调用链：pickAuto/pickExact → selectAccountFromProvider → acquireFromAccounts
func (b *defaultBalancer) acquireFromAccounts(
	ctx context.Context,
	provInfo *account.ProviderInfo,
	accounts []*account.Account,
	selReq *selector.SelectRequest,
	maxRetries int,
) (*PickResult, error) {
	// 在已过滤的账号列表上进行 Select + Acquire 重试循环
	for i := 0; i <= maxRetries; i++ {
		// 排除已尝试过的账号
		filtered := excludeAccounts(accounts, selReq.ExcludeAccountIDs)
		if len(filtered) == 0 {
			return nil, ErrNoAvailableAccount
		}

		// Use Selector to select an account
		chosen, err := b.opts.Selector.Select(filtered, selReq)
		if err != nil {
			if errors.Is(err, selector.ErrEmptyCandidates) || errors.Is(err, selector.ErrNoAvailableAccount) {
				return nil, ErrNoAvailableAccount
			}
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

		return &PickResult{
			Account:     chosen.Clone(),
			ProviderKey: provInfo.ProviderKey(),
			Attempts:    i,
		}, nil
	}

	// 重试次数耗尽
	return nil, ErrMaxRetriesExceeded
}

// selectAccountFromProvider 从指定供应商中选取账号。
// 执行一次 ResolveAccounts + FilterAvailable + Shuffle，然后委托 acquireFromAccounts 进行 Select + Acquire 重试循环。
// 被 pickExact 和 pickAuto 共同调用。
// 调用链：pickAuto/pickExact → selectAccountFromProvider → acquireFromAccounts
func (b *defaultBalancer) selectAccountFromProvider(
	ctx context.Context,
	provInfo *account.ProviderInfo,
	selReq *selector.SelectRequest,
	maxRetries int,
) (*PickResult, error) {
	// Resolve available accounts under this provider via Resolver
	accounts, err := b.opts.Resolver.ResolveAccounts(ctx, resolver.ResolveAccountsRequest{
		Key:  provInfo.ProviderKey(),
		Tags: selReq.Tags,
	})
	if err != nil {
		return nil, fmt.Errorf("balancer: resolve accounts: %w", err)
	}

	if len(accounts) == 0 {
		return nil, ErrNoAvailableAccount
	}

	// 占用过滤：排除已达并发上限的账号
	accounts = b.opts.OccupancyController.FilterAvailable(ctx, accounts)
	if len(accounts) == 0 {
		return nil, ErrOccupancyFull
	}

	// 随机打散账号列表，分散竞争热点
	rand.Shuffle(len(accounts), func(i, j int) {
		accounts[i], accounts[j] = accounts[j], accounts[i]
	})

	// 委托 acquireFromAccounts 进行 Select + Acquire 重试循环
	return b.acquireFromAccounts(ctx, provInfo, accounts, selReq, maxRetries)
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
		// 性能优化：仅当存在连续失败记录时，才需要检查并恢复熔断状态。
		// 正常路径（连续失败数为 0）完全无存储查询。
		consecutiveFailures := 0
		if b.opts.StatsStore != nil {
			if stats, statsErr := b.opts.StatsStore.GetStats(ctx, accountID); statsErr == nil {
				consecutiveFailures = stats.ConsecutiveFailures
			}
		}

		if consecutiveFailures > 0 {
			// 有连续失败记录，检查并尝试恢复熔断状态
			acct, err := b.opts.AccountStorage.GetAccount(ctx, accountID)
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
				if err := b.opts.AccountStorage.UpdateAccount(ctx, acct, storage.UpdateFieldStatus); err != nil {
					if errors.Is(err, storage.ErrVersionConflict) {
						// 版本冲突，已被其他实例更新，忽略（幂等）
						return nil
					}
					return fmt.Errorf("balancer: update account: %w", err)
				}
			}
		}

		// 无论是否需要 GetAccount，都重置连续失败计数
		if b.opts.StatsStore != nil {
			_ = b.opts.StatsStore.ResetConsecutiveFailures(ctx, accountID)
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
	// TODO: 请求次数需要更新、token用量待评估
	if b.opts.UsageTracker != nil {
		_ = b.opts.UsageTracker.RecordUsage(ctx, accountID, usagerule.SourceTypeRequest, 1.0)
	}

	// 3. 判断是否需要变更 Account 状态
	needUpdate := false

	acct, err := b.opts.AccountStorage.GetAccount(ctx, accountID)
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
		// TODO: 此处需要根据httpErr来做精确的状态处理，比如账号是否被封禁等
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
		if err := b.opts.AccountStorage.UpdateAccount(ctx, acct, storage.UpdateFieldStatus); err != nil {
			if errors.Is(err, storage.ErrVersionConflict) {
				// 版本冲突：重新获取最新版本并重试一次（避免熔断状态丢失）
				latestAcct, getErr := b.opts.AccountStorage.GetAccount(ctx, accountID)
				if getErr != nil {
					return nil // 无法获取最新数据，静默忽略
				}
				// 如果账号已被标记为更严重的终态（或更高优先级的熔断状态），不覆盖
				if latestAcct.Status == account.StatusBanned ||
					latestAcct.Status == account.StatusInvalidated ||
					latestAcct.Status == account.StatusDisabled ||
					latestAcct.Status == account.StatusCircuitOpen {
					return nil
				}
				latestAcct.Status = acct.Status
				latestAcct.CircuitOpenUntil = acct.CircuitOpenUntil
				latestAcct.CooldownUntil = acct.CooldownUntil
				latestAcct.UpdatedAt = time.Now()
				// 重试一次，若再次冲突则静默忽略
				_ = b.opts.AccountStorage.UpdateAccount(ctx, latestAcct, storage.UpdateFieldStatus)
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

// excludeAccounts 从账号列表中排除指定 ID 的账号。
func excludeAccounts(accounts []*account.Account, excludeIDs []string) []*account.Account {
	if len(excludeIDs) == 0 {
		return accounts
	}
	excludeSet := make(map[string]struct{}, len(excludeIDs))
	for _, id := range excludeIDs {
		excludeSet[id] = struct{}{}
	}
	result := make([]*account.Account, 0, len(accounts))
	for _, acct := range accounts {
		if _, excluded := excludeSet[acct.ID]; !excluded {
			result = append(result, acct)
		}
	}
	return result
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
			acct, err := accountStorage.GetAccount(ctx, accountID)
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
			_ = accountStorage.UpdateAccount(ctx, acct, storage.UpdateFieldStatus) // 乐观锁持久化，冲突静默忽略
		}),
	)
}
