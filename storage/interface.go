package storage

import (
	"context"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/provider"
	"github.com/nomand-zc/lumin-acpool/storage/filtercond"
)

// ProviderStorage is the provider storage interface.
// Responsible for CRUD operations on ProviderInfo metadata.
type ProviderStorage interface {
	// Get retrieves provider info by ProviderKey.
	// Returns ErrNotFound if not found.
	Get(ctx context.Context, key provider.ProviderKey) (*provider.ProviderInfo, error)

	// Search queries provider list.
	// Returns all providers when filter is nil.
	Search(ctx context.Context, filter *filtercond.Filter) ([]*provider.ProviderInfo, error)

	// Add adds a provider.
	// Returns ErrAlreadyExists if the ProviderKey already exists.
	Add(ctx context.Context, info *provider.ProviderInfo) error

	// Update updates provider info (full replacement).
	// Returns ErrNotFound if the ProviderKey does not exist.
	Update(ctx context.Context, info *provider.ProviderInfo) error

	// Remove deletes a provider.
	// Returns ErrNotFound if the ProviderKey does not exist.
	Remove(ctx context.Context, key provider.ProviderKey) error
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
	CountByProvider(ctx context.Context, key provider.ProviderKey, filter *filtercond.Filter) (int, error)
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
