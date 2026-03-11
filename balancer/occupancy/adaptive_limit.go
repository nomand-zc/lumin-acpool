package occupancy

import (
	"context"
	"math"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-acpool/storage/memory/occupancystore"
	"github.com/nomand-zc/lumin-acpool/usagetracker"
)

// 编译期接口合规性检查。
var _ Controller = (*AdaptiveLimit)(nil)

// AdaptiveLimit 基于账号额度和限流规则动态调整并发上限的占用控制器。
// 根据 UsageTracker 提供的剩余配额和时间窗口信息，实时计算每个账号允许的最大并发数。
//
// 动态计算公式：
//
//	允许并发数 = floor(剩余配额 / 窗口剩余秒数 × 因子)
//
// 其中：
//   - 剩余配额: 通过 UsageTracker.MinRemainRatio 获取最小剩余比例，乘以总量得到
//   - 窗口剩余秒数: 通过 UsageTracker.GetTrackedUsages 获取窗口结束时间计算
//   - 因子 (Factor): 调控参数，默认 1.0
//
// 适用于账号有明确配额限制（如每分钟 N 次请求）的场景。
type AdaptiveLimit struct {
	store   storage.OccupancyStore
	tracker usagetracker.UsageTracker

	// factor 调控因子，控制实际并发上限与理论值的比例。
	// 默认 1.0，设置为 0.8 表示保守策略（留 20% 余量），设置为 1.2 表示激进策略。
	factor float64

	// minLimit 最小并发上限，防止在配额极低时完全阻塞。
	// 默认 1，确保至少允许一个并发请求。
	minLimit int64

	// maxLimit 最大并发上限，防止在配额充裕时过度并发。
	// 默认 0（不限制）。
	maxLimit int64

	// fallbackLimit 当无法获取配额信息时的回退并发上限。
	// 默认 1（保守策略）。
	fallbackLimit int64
}

// AdaptiveLimitOption 是 AdaptiveLimit 的配置选项函数。
type AdaptiveLimitOption func(*AdaptiveLimit)

// NewAdaptiveLimit 创建一个基于配额动态调整的占用控制器。
// tracker 用于获取账号的实时配额信息（必需）。
// 若未通过 WithAdaptiveStore 注入存储后端，默认使用 MemoryOccupancyStore。
func NewAdaptiveLimit(tracker usagetracker.UsageTracker, opts ...AdaptiveLimitOption) *AdaptiveLimit {
	a := &AdaptiveLimit{
		tracker:       tracker,
		factor:        1.0,
		minLimit:      1,
		maxLimit:      0,
		fallbackLimit: 1,
	}
	for _, opt := range opts {
		opt(a)
	}
	if a.store == nil {
		a.store = occupancystore.NewMemoryOccupancyStore()
	}
	return a
}

// WithAdaptiveStore 设置占用计数存储后端。
func WithAdaptiveStore(store storage.OccupancyStore) AdaptiveLimitOption {
	return func(a *AdaptiveLimit) { a.store = store }
}

// WithFactor 设置调控因子（默认 1.0）。
// 值 < 1.0 为保守策略（留余量），值 > 1.0 为激进策略。
func WithFactor(factor float64) AdaptiveLimitOption {
	return func(a *AdaptiveLimit) { a.factor = factor }
}

// WithMinLimit 设置最小并发上限（默认 1）。
// 防止在配额极低时完全阻塞请求。
func WithMinLimit(min int64) AdaptiveLimitOption {
	return func(a *AdaptiveLimit) { a.minLimit = min }
}

// WithMaxLimit 设置最大并发上限（默认 0，不限制）。
// 防止在配额充裕时过度并发。
func WithMaxLimit(max int64) AdaptiveLimitOption {
	return func(a *AdaptiveLimit) { a.maxLimit = max }
}

// WithFallbackLimit 设置回退并发上限（默认 1）。
// 当无法获取配额信息时使用此值。
func WithFallbackLimit(limit int64) AdaptiveLimitOption {
	return func(a *AdaptiveLimit) { a.fallbackLimit = limit }
}

func (a *AdaptiveLimit) FilterAvailable(ctx context.Context, accounts []*account.Account) []*account.Account {
	result := make([]*account.Account, 0, len(accounts))
	for _, acct := range accounts {
		limit := a.calculateLimit(ctx, acct.ID)
		current, err := a.store.Get(ctx, acct.ID)
		if err != nil {
			// 存储查询失败，保守策略：保留该账号
			result = append(result, acct)
			continue
		}
		if current < limit {
			result = append(result, acct)
		}
	}
	return result
}

func (a *AdaptiveLimit) Acquire(ctx context.Context, acct *account.Account) bool {
	limit := a.calculateLimit(ctx, acct.ID)

	// 原子递增并判断是否超过上限
	newVal, err := a.store.Incr(ctx, acct.ID)
	if err != nil {
		return false
	}

	if newVal > limit {
		// 超过上限，回退计数
		_ = a.store.Decr(ctx, acct.ID)
		return false
	}

	return true
}

func (a *AdaptiveLimit) Release(ctx context.Context, accountID string) {
	_ = a.store.Decr(ctx, accountID)
}

// calculateLimit 基于当前配额和窗口信息动态计算并发上限。
// 公式：limit = floor(remainAmount / remainSeconds * factor)
// 其中 remainAmount 为最小剩余配额，remainSeconds 为距窗口结束的秒数。
func (a *AdaptiveLimit) calculateLimit(ctx context.Context, accountID string) int64 {
	// 获取追踪数据以计算窗口剩余时间
	usages, err := a.tracker.GetTrackedUsages(ctx, accountID)
	if err != nil || len(usages) == 0 {
		return a.fallbackLimit
	}

	// 获取最小剩余比例
	minRatio, err := a.tracker.MinRemainRatio(ctx, accountID)
	if err != nil {
		return a.fallbackLimit
	}

	// 找到约束最紧的规则来计算
	var (
		bestLimit int64 = math.MaxInt64
		found     bool
	)

	now := time.Now()
	for _, u := range usages {
		if u.Rule == nil || u.Rule.Total <= 0 {
			continue
		}

		// 计算该规则的剩余量
		remainAmount := u.EstimatedRemain()
		if remainAmount <= 0 {
			return a.minLimit // 已耗尽，返回最小值
		}

		// 计算窗口剩余秒数
		var remainSeconds float64
		if u.WindowEnd != nil && now.Before(*u.WindowEnd) {
			remainSeconds = u.WindowEnd.Sub(now).Seconds()
		} else {
			// 窗口已过期或未设置，使用 minRatio * Total 作为保守估计
			remainSeconds = 60 // 默认假设 60 秒窗口
		}

		if remainSeconds <= 0 {
			remainSeconds = 1 // 防止除零
		}

		// 计算该规则允许的并发数 = 剩余量 / 剩余秒数 * 因子
		limit := int64(math.Floor(remainAmount / remainSeconds * a.factor))

		if limit < bestLimit {
			bestLimit = limit
			found = true
		}
	}

	if !found {
		return a.fallbackLimit
	}

	// 如果 minRatio 非常低（接近耗尽），进一步压缩
	if minRatio < 0.1 {
		bestLimit = int64(math.Ceil(float64(bestLimit) * minRatio * 10))
	}

	// 应用上下限约束
	if bestLimit < a.minLimit {
		bestLimit = a.minLimit
	}
	if a.maxLimit > 0 && bestLimit > a.maxLimit {
		bestLimit = a.maxLimit
	}

	return bestLimit
}
