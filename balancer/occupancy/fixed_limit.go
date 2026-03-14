package occupancy

import (
	"context"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-acpool/storage/memory/occupancystore"
)

// 编译期接口合规性检查。
var _ Controller = (*FixedLimit)(nil)

// FixedLimit 固定并发上限的占用控制器。
// 通过 OccupancyStore 原子计数器跟踪每个账号的当前并发数，并与预设的固定上限进行比较。
// 支持五层粒度的限额配置（优先级由高到低）：
//   - 账号 Metadata 级别（Account.Metadata["occupancy_limit"]，可通过管理 API 动态调整）
//   - 账号 ID 级别（accountLimits，代码级配置）
//   - provider_type + provider_name 级别（providerKeyLimits）
//   - provider_type 级别（providerTypeLimits）
//   - 全局默认值（defaultLimit）
type FixedLimit struct {
	store storage.OccupancyStore

	defaultLimit       int64
	providerTypeLimits map[string]int64              // provider_type → limit
	providerKeyLimits  map[account.ProviderKey]int64 // ProviderKey{Type, Name} → limit
	accountLimits      map[string]int64              // accountID → limit
}

// FixedLimitOption 是 FixedLimit 的配置选项函数。
type FixedLimitOption func(*FixedLimit)

// NewFixedLimit 创建一个固定并发上限的占用控制器。
// defaultLimit 为全局默认并发上限，所有未单独配置的账号都使用此值。
// 若未通过 WithStore 注入存储后端，默认使用 MemoryOccupancyStore。
func NewFixedLimit(defaultLimit int64, opts ...FixedLimitOption) *FixedLimit {
	f := &FixedLimit{
		defaultLimit:       defaultLimit,
		providerTypeLimits: make(map[string]int64),
		providerKeyLimits:  make(map[account.ProviderKey]int64),
		accountLimits:      make(map[string]int64),
	}
	for _, opt := range opts {
		opt(f)
	}
	if f.store == nil {
		f.store = occupancystore.NewMemoryOccupancyStore()
	}
	return f
}

// WithStore 设置占用计数存储后端。
// 默认使用 MemoryOccupancyStore（单机），集群部署时应注入 Redis 实现。
func WithStore(store storage.OccupancyStore) FixedLimitOption {
	return func(f *FixedLimit) { f.store = store }
}

// WithProviderTypeLimit 设置指定 provider_type 的并发上限。
// 优先级低于 ProviderKey 级别和账号 ID 级别。
func WithProviderTypeLimit(providerType string, limit int64) FixedLimitOption {
	return func(f *FixedLimit) {
		f.providerTypeLimits[providerType] = limit
	}
}

// WithProviderKeyLimit 设置指定 provider_type + provider_name 的并发上限。
// 优先级低于账号 ID 级别，高于 provider_type 级别。
func WithProviderKeyLimit(key account.ProviderKey, limit int64) FixedLimitOption {
	return func(f *FixedLimit) {
		f.providerKeyLimits[key] = limit
	}
}

// WithAccountLimit 设置指定账号 ID 的并发上限。
// 优先级最高。
func WithAccountLimit(accountID string, limit int64) FixedLimitOption {
	return func(f *FixedLimit) {
		f.accountLimits[accountID] = limit
	}
}

func (f *FixedLimit) FilterAvailable(ctx context.Context, accounts []*account.Account) []*account.Account {
	result := make([]*account.Account, 0, len(accounts))
	for _, acct := range accounts {
		limit := f.getLimit(acct)
		current, err := f.store.GetOccupancy(ctx, acct.ID)
		if err != nil {
			// 存储查询失败，保守策略：保留该账号（不误排除）
			result = append(result, acct)
			continue
		}
		if current < limit {
			result = append(result, acct)
		}
	}
	return result
}

func (f *FixedLimit) Acquire(ctx context.Context, acct *account.Account) bool {
	limit := f.getLimit(acct)

	// 原子递增并判断是否超过上限
	newVal, err := f.store.IncrOccupancy(ctx, acct.ID)
	if err != nil {
		// 存储操作失败，保守策略：拒绝获取
		return false
	}

	if newVal > limit {
		// 超过上限，回退计数
		_ = f.store.DecrOccupancy(ctx, acct.ID)
		return false
	}

	return true
}

func (f *FixedLimit) Release(ctx context.Context, accountID string) {
	_ = f.store.DecrOccupancy(ctx, accountID)
}

// getLimit 按照 Metadata > 账号ID > ProviderKey > ProviderType > 默认值 的优先级获取限额。
func (f *FixedLimit) getLimit(acct *account.Account) int64 {
	// 优先级 1：账号 Metadata 级别（可通过管理 API 动态调整，无需重启）
	if limit, ok := metadataInt64(acct, MetaKeyOccupancyLimit); ok && limit > 0 {
		return limit
	}

	// 优先级 2：账号 ID 级别（代码级配置）
	if limit, ok := f.accountLimits[acct.ID]; ok {
		return limit
	}

	// 优先级 3：ProviderKey 级别（provider_type + provider_name）
	key := acct.ProviderKey()
	if limit, ok := f.providerKeyLimits[key]; ok {
		return limit
	}

	// 优先级 4：provider_type 级别
	if limit, ok := f.providerTypeLimits[acct.ProviderType]; ok {
		return limit
	}

	// 优先级 5：全局默认值
	return f.defaultLimit
}
