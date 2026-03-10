package resolver

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-acpool/storage/filtercond"
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
	provInfo, err := r.providerStorage.Get(ctx, key)
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

	return provInfo, nil
}

// ResolveProviders resolves active providers that support the specified model from storage.
func (r *storageResolver) ResolveProviders(ctx context.Context, model string, providerType string) ([]*account.ProviderInfo, error) {
	// Active status filter
statusFilter := filtercond.In(storage.ProviderFieldStatus, int(account.ProviderStatusActive), int(account.ProviderStatusDegraded))

	var filter *filtercond.Filter
	if providerType != "" {
		// Filter by type + active status + supports specified model
		filter = filtercond.And(
			filtercond.Equal(storage.ProviderFieldType, providerType),
			statusFilter,
			filtercond.Equal(storage.ProviderFieldSupportedModel, model),
		)
	} else {
		// Fully automatic, by model + active status
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

// ResolveAccounts resolves available accounts under the specified provider from storage.
func (r *storageResolver) ResolveAccounts(ctx context.Context, key account.ProviderKey, tags map[string]string, excludeIDs []string) ([]*account.Account, error) {
	// Only query available accounts under the specified provider
	filter := filtercond.And(
		filtercond.Equal(storage.AccountFieldProviderType, key.Type),
		filtercond.Equal(storage.AccountFieldProviderName, key.Name),
		filtercond.Equal(storage.AccountFieldStatus, int(account.StatusAvailable)),
	)

	accounts, err := r.accountStorage.Search(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("resolver: search accounts: %w", err)
	}

	// Filter out excluded account IDs
	if len(excludeIDs) > 0 {
		accounts = filterExcluded(accounts, excludeIDs)
	}

	// Filter by tags
	if len(tags) > 0 {
		accounts = filterByTags(accounts, tags)
	}

	return accounts, nil
}

// filterExcluded filters out accounts with specified IDs.
func filterExcluded(accounts []*account.Account, excludeIDs []string) []*account.Account {
	var result []*account.Account
	for _, acct := range accounts {
		if !slices.Contains(excludeIDs, acct.ID) {
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
