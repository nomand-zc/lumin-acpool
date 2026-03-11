package storage

import (
	"context"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage/filtercond"
)

// UsageStore 用量追踪数据存储接口。
// 支持内存（单机）和 Redis（集群）两种后端。
type UsageStore interface {
	// GetAll 获取指定账号所有规则的追踪数据。
	GetAll(ctx context.Context, accountID string) ([]*account.TrackedUsage, error)

	// Save 保存指定账号所有规则的追踪数据。
	Save(ctx context.Context, accountID string, usages []*account.TrackedUsage) error

	// IncrLocalUsed 原子递增指定规则的本地已用量。
	// ruleIndex: 规则索引，amount: 增量。
	IncrLocalUsed(ctx context.Context, accountID string, ruleIndex int, amount float64) error

	// CalibrateRule 原子校准指定规则的远端数据并重置本地计数。
	// 使用原子操作 SET remote_used=?, remote_remain=?, local_used=0, ...
	// 避免 Save 全量替换导致并发 IncrLocalUsed 的增量丢失。
	CalibrateRule(ctx context.Context, accountID string, ruleIndex int, usage *account.TrackedUsage) error

	// Remove 删除指定账号的追踪数据。
	Remove(ctx context.Context, accountID string) error
}

// ProviderStorage is the provider storage interface.
// Responsible for CRUD operations on ProviderInfo metadata.
type ProviderStorage interface {
	// Get retrieves provider info by ProviderKey.
	// Returns ErrNotFound if not found.
	Get(ctx context.Context, key account.ProviderKey) (*account.ProviderInfo, error)

	// Search queries provider list.
	// Returns all providers when filter is nil.
	Search(ctx context.Context, filter *filtercond.Filter) ([]*account.ProviderInfo, error)

	// Add adds a provider.
	// Returns ErrAlreadyExists if the ProviderKey already exists.
	Add(ctx context.Context, info *account.ProviderInfo) error

	// Update updates provider info (full replacement).
	// Returns ErrNotFound if the ProviderKey does not exist.
	Update(ctx context.Context, info *account.ProviderInfo) error

	// Remove deletes a provider.
	// Returns ErrNotFound if the ProviderKey does not exist.
	Remove(ctx context.Context, key account.ProviderKey) error
}

// AccountStorage is the account storage interface.
// Responsible for CRUD operations on Account aggregate roots, all queries support filtercond universal filter conditions.
type AccountStorage interface {
	// Get retrieves a single account by ID.
	// Returns ErrNotFound if not found.
	Get(ctx context.Context, id string) (*account.Account, error)

	// Search queries account list.
	// Returns all accounts when filter is nil.
	Search(ctx context.Context, filter *filtercond.Filter) ([]*account.Account, error)

	// Add adds an account.
	// Returns ErrAlreadyExists if the ID already exists.
	Add(ctx context.Context, acct *account.Account) error

	// Update updates account info (full replacement).
	// Returns ErrNotFound if the ID does not exist.
	// Returns ErrVersionConflict if the version does not match (optimistic lock conflict).
	Update(ctx context.Context, acct *account.Account) error

	// Remove deletes an account.
	// Returns ErrNotFound if the ID does not exist.
	Remove(ctx context.Context, id string) error

	// RemoveFilter batch deletes accounts by condition.
	// Deletes all accounts when filter is nil.
	RemoveFilter(ctx context.Context, filter *filtercond.Filter) error

	// Count returns the account count.
	// Returns the total count when filter is nil.
	Count(ctx context.Context, filter *filtercond.Filter) (int, error)

	// CountByProvider returns the account count under the specified provider.
	// Returns the total count under that provider when filter is nil.
	CountByProvider(ctx context.Context, key account.ProviderKey, filter *filtercond.Filter) (int, error)
}

// AffinityStore 是亲和绑定关系的存储接口。
// 负责维护 affinityKey → targetID 的映射关系。
//
// 在单机部署时，可使用内置的 MemoryAffinityStore（内存实现）；
// 在集群部署时，应注入基于 Redis/数据库等共享存储的实现，
// 使多个实例能够共享绑定关系，充分发挥亲和策略的效果。
type AffinityStore interface {
	// Get 获取亲和键对应的绑定目标 ID。
	// 返回目标 ID 和是否存在。
	Get(affinityKey string) (targetID string, exists bool)

	// Set 设置亲和键到目标 ID 的绑定关系。
	Set(affinityKey string, targetID string)
}

// StatsStore 运行时统计存储接口。
// 支持高频原子更新，避免与 AccountStorage 的全量覆盖竞争。
type StatsStore interface {
	// Get 获取指定账号的运行统计。
	// 如果账号不存在统计记录，返回零值的 AccountStats（不返回错误）。
	Get(ctx context.Context, accountID string) (*account.AccountStats, error)

	// IncrSuccess 原子递增成功计数，重置连续失败计数，更新 LastUsedAt。
	IncrSuccess(ctx context.Context, accountID string) error

	// IncrFailure 原子递增失败计数和连续失败计数，更新错误信息。
	// 返回递增后的连续失败次数（用于熔断判断，避免 TOCTOU 竞态）。
	IncrFailure(ctx context.Context, accountID string, errMsg string) (int, error)

	// UpdateLastUsed 更新最后使用时间。
	UpdateLastUsed(ctx context.Context, accountID string, t time.Time) error

	// GetConsecutiveFailures 获取连续失败次数（供 CircuitBreaker 使用）。
	GetConsecutiveFailures(ctx context.Context, accountID string) (int, error)

	// ResetConsecutiveFailures 重置连续失败次数（成功时调用）。
	ResetConsecutiveFailures(ctx context.Context, accountID string) error

	// Remove 删除统计数据（账号注销时调用）。
	Remove(ctx context.Context, accountID string) error
}

// OccupancyStore 占用计数存储接口。
// 存储每个账号当前的并发占用数量，支持原子操作确保竞态安全。
// 在单机部署时，可使用内置的 MemoryOccupancyStore（内存实现）；
// 在集群部署时，应注入基于 Redis 等共享存储的实现，
// 使多个实例能够共享占用状态，实现跨实例的并发控制。
type OccupancyStore interface {
	// Incr 原子递增指定账号的占用计数，并返回递增后的值。
	// 用于 Acquire 操作：先递增，再判断是否超过上限。
	Incr(ctx context.Context, accountID string) (int64, error)

	// Decr 原子递减指定账号的占用计数（不会低于 0）。
	// 用于 Release 操作。
	Decr(ctx context.Context, accountID string) error

	// Get 获取指定账号当前的占用计数。
	// 用于 FilterAvailable 操作：判断是否还有余量。
	Get(ctx context.Context, accountID string) (int64, error)
}
