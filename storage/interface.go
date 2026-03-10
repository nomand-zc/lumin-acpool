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
