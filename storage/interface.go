package storage

import (
	"context"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/provider"
	"github.com/nomand-zc/lumin-acpool/storage/filtercond"
)

// ProviderStorage 供应商存储接口
// 负责 ProviderInfo 元数据的增删改查
type ProviderStorage interface {
	// Get 根据 ProviderKey 获取供应商信息
	// 如果不存在，返回 ErrNotFound
	Get(ctx context.Context, key provider.ProviderKey) (*provider.ProviderInfo, error)

	// Search 查询供应商列表
	// filter 为 nil 时返回全部供应商
	Search(ctx context.Context, filter *filtercond.Filter) ([]*provider.ProviderInfo, error)

	// Add 添加供应商
	// 如果 ProviderKey 已存在，返回 ErrAlreadyExists
	Add(ctx context.Context, info *provider.ProviderInfo) error

	// Update 更新供应商信息（整体覆盖）
	// 如果 ProviderKey 不存在，返回 ErrNotFound
	Update(ctx context.Context, info *provider.ProviderInfo) error

	// Remove 删除供应商
	// 如果 ProviderKey 不存在，返回 ErrNotFound
	Remove(ctx context.Context, key provider.ProviderKey) error
}

// AccountStorage 账号存储接口
// 负责 Account 聚合根的增删改查，所有查询均支持 filtercond 通用过滤条件
type AccountStorage interface {
	// Get 根据 ID 获取单个账号
	// 如果不存在，返回 ErrNotFound
	Get(ctx context.Context, id string) (*account.Account, error)

	// Search 查询账号列表
	// filter 为 nil 时返回全部账号
	Search(ctx context.Context, filter *filtercond.Filter) ([]*account.Account, error)

	// Add 添加账号
	// 如果 ID 已存在，返回 ErrAlreadyExists
	Add(ctx context.Context, acct *account.Account) error

	// Update 更新账号信息（整体覆盖）
	// 如果 ID 不存在，返回 ErrNotFound
	Update(ctx context.Context, acct *account.Account) error

	// Remove 删除账号
	// 如果 ID 不存在，返回 ErrNotFound
	Remove(ctx context.Context, id string) error

	// RemoveFilter 按条件批量删除账号
	// filter 为 nil 时删除全部账号
	RemoveFilter(ctx context.Context, filter *filtercond.Filter) error

	// Count 统计账号数量
	// filter 为 nil 时返回全部账号数量
	Count(ctx context.Context, filter *filtercond.Filter) (int, error)

	// CountByProvider 统计指定供应商下的账号数量
	// filter 为 nil 时返回该供应商下的全部账号数量
	CountByProvider(ctx context.Context, key provider.ProviderKey, filter *filtercond.Filter) (int, error)
}
