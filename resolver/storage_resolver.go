package resolver

import (
	"context"
	"errors"
	"fmt"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
)

// storageResolver is the default implementation of Resolver based on Storage.
type storageResolver struct {
	providerStorage storage.ProviderStorage
	accountStorage  storage.AccountStorage
}

// NewStorageResolver creates a Storage-based resolver instance.
func NewStorageResolver(providerStorage storage.ProviderStorage, accountStorage storage.AccountStorage) Resolver {
	return &storageResolver{
		providerStorage: providerStorage,
		accountStorage:  accountStorage,
	}
}

// ResolveProvider resolves the specified provider exactly.
func (r *storageResolver) ResolveProvider(ctx context.Context, key account.ProviderKey, model string) (*account.ProviderInfo, error) {
	provInfo, err := r.providerStorage.GetProvider(ctx, key)
	if err != nil {
		if errors.Is(err, storage.ErrNotFound) {
			return nil, ErrProviderNotFound
		}
		return nil, fmt.Errorf("resolver: get provider: %w", err)
	}

	// Verify if the provider is active
	if !provInfo.IsActive() {
		return nil, ErrProviderInactive
	}

	// Verify if the provider supports the model
	if !provInfo.SupportsModel(model) {
		return nil, ErrModelNotSupported
	}

	if provInfo.AccountCount <= 0 || provInfo.AvailableAccountCount <= 0 {
		return nil, ErrNoAccount
	}

	return provInfo, nil
}

// ResolveProviders resolves active providers that support the specified model from storage.
func (r *storageResolver) ResolveProviders(ctx context.Context, model string, providerType string) ([]*account.ProviderInfo, error) {
	filter := &storage.SearchFilter{
		ProviderType:   providerType,
		SupportedModel: model,
		// 不在 filter 层限制 Status，因为 Active 和 Degraded 都视为可用，
		// 由下方 IsActive() 统一判断。
	}

	candidate, err := r.providerStorage.SearchProviders(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("resolver: search providers: %w", err)
	}

	var active []*account.ProviderInfo
	for _, provInfo := range candidate {
		if !provInfo.IsActive() {
			continue
		}
		if provInfo.AccountCount > 0 && provInfo.AvailableAccountCount > 0 {
			active = append(active, provInfo)
		}
	}

	if len(active) == 0 {
		return nil, nil // 返回空切片而非错误，由调用方通过 len(candidates)==0 判断
	}

	return active, nil
}

// ResolveAccounts resolves available accounts under the specified provider from storage.
func (r *storageResolver) ResolveAccounts(ctx context.Context, req ResolveAccountsRequest) ([]*account.Account, error) {
	// Only query available accounts under the specified provider
	filter := &storage.SearchFilter{
		ProviderType: req.Key.Type,
		ProviderName: req.Key.Name,
		Status:       int(account.StatusAvailable),
	}

	accounts, err := r.accountStorage.SearchAccounts(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("resolver: search accounts: %w", err)
	}

	// Filter out excluded account IDs
	if len(req.ExcludeIDs) > 0 {
		accounts = filterExcluded(accounts, req.ExcludeIDs)
	}

	// Filter by tags
	if len(req.Tags) > 0 {
		accounts = filterByTags(accounts, req.Tags)
	}

	return accounts, nil
}

// filterExcluded filters out accounts with specified IDs.
func filterExcluded(accounts []*account.Account, excludeIDs []string) []*account.Account {
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

// filterByTags filters accounts by tags (must contain all specified tag key-value pairs).
func filterByTags(accounts []*account.Account, tags map[string]string) []*account.Account {
	var result []*account.Account
	for _, acct := range accounts {
		if matchTags(acct.Tags, tags) {
			result = append(result, acct)
		}
	}
	return result
}

// matchTags checks if accountTags contains all requiredTags.
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
