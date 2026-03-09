package resolver

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/provider"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-acpool/storage/filtercond"
)

// storageResolver 基于 Storage 的 Resolver 默认实现
type storageResolver struct {
	providerStorage storage.ProviderStorage
	accountStorage  storage.AccountStorage
}

// NewStorageResolver 创建基于 Storage 的解析器实例
func NewStorageResolver(providerStorage storage.ProviderStorage, accountStorage storage.AccountStorage) Resolver {
	return &storageResolver{
		providerStorage: providerStorage,
		accountStorage:  accountStorage,
	}
}

// ResolveProvider 精确解析指定供应商
func (r *storageResolver) ResolveProvider(ctx context.Context, key provider.ProviderKey, model string) (*provider.ProviderInfo, error) {
	provInfo, err := r.providerStorage.Get(ctx, key)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, ErrProviderNotFound
		}
		return nil, fmt.Errorf("resolver: get provider: %w", err)
	}

	// 校验供应商是否活跃
	if !provInfo.IsActive() {
		return nil, ErrProviderInactive
	}

	// 校验供应商是否支持该模型
	if !provInfo.SupportsModel(model) {
		return nil, ErrModelNotSupported
	}

	return provInfo, nil
}

// ResolveProviders 从存储中解析出支持指定模型的活跃供应商
func (r *storageResolver) ResolveProviders(ctx context.Context, model string, providerType string) ([]*provider.ProviderInfo, error) {
	// 活跃状态过滤
	statusFilter := filtercond.In(storage.ProviderFieldStatus, int(provider.ProviderStatusActive), int(provider.ProviderStatusDegraded))

	var filter *filtercond.Filter
	if providerType != "" {
		// 按类型 + 活跃状态 + 支持指定模型
		filter = filtercond.And(
			filtercond.Equal(storage.ProviderFieldType, providerType),
			statusFilter,
			filtercond.Equal(storage.ProviderFieldSupportedModel, model),
		)
	} else {
		// 全自动，按模型 + 活跃状态
		filter = filtercond.And(
			filtercond.Equal(storage.ProviderFieldSupportedModel, model),
			statusFilter,
		)
	}

	candidates, err := r.providerStorage.Search(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("resolver: search providers: %w", err)
	}
	return candidates, nil
}

// ResolveAccounts 从存储中解析出指定供应商下的可用账号
func (r *storageResolver) ResolveAccounts(ctx context.Context, key provider.ProviderKey, tags map[string]string, excludeIDs []string) ([]*account.Account, error) {
	// 只查指定供应商下可用状态的账号
	filter := filtercond.And(
		filtercond.Equal(storage.AccountFieldProviderType, key.Type),
		filtercond.Equal(storage.AccountFieldProviderName, key.Name),
		filtercond.Equal(storage.AccountFieldStatus, int(account.StatusAvailable)),
	)

	accounts, err := r.accountStorage.Search(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("resolver: search accounts: %w", err)
	}

	// 过滤掉排除的账号 ID
	if len(excludeIDs) > 0 {
		accounts = filterExcluded(accounts, excludeIDs)
	}

	// 按标签过滤
	if len(tags) > 0 {
		accounts = filterByTags(accounts, tags)
	}

	return accounts, nil
}

// filterExcluded 过滤掉指定 ID 的账号
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
