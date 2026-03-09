package storage

import (
	"context"

	"github.com/nomand-zc/lumin-acpool/filtercond"
	"github.com/nomand-zc/lumin-acpool/provider"
)

// ProviderStorage 供应商存储接口
// 负责 ProviderInfo 元数据的增删改查
type ProviderStorage interface {
	// Get 根据 ProviderKey 获取供应商信息
	// 如果不存在，返回 ErrNotFound
	Get(ctx context.Context, key provider.ProviderKey) (*provider.ProviderInfo, error)

	// List 查询供应商列表
	// filter 为 nil 时返回全部供应商
	List(ctx context.Context, filter *filtercond.Filter) ([]*provider.ProviderInfo, error)

	// ListByType 查询指定类型下的所有供应商
	// filter 为 nil 时返回该类型下的全部供应商
	ListByType(ctx context.Context, providerType string, filter *filtercond.Filter) ([]*provider.ProviderInfo, error)

	// ListByModel 查询支持指定模型的所有供应商
	// filter 为 nil 时返回所有支持该模型的供应商
	ListByModel(ctx context.Context, model string, filter *filtercond.Filter) ([]*provider.ProviderInfo, error)

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
