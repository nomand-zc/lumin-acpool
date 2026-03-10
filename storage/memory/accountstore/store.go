package accountstore

import (
	"context"
	"sync"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/provider"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-acpool/storage/filtercond"
)

// Compile-time interface compliance check.
var _ storage.AccountStorage = (*Store)(nil)

// Store is the in-memory storage implementation for accounts.
// Uses a read-write lock for concurrency safety and maintains a ProviderKey secondary index for hot-path query acceleration.
type Store struct {
	mu sync.RWMutex
	// accounts is the primary storage: id -> Account
	accounts map[string]*account.Account
	// providerIndex is the secondary index: ProviderKey -> id set
	providerIndex map[provider.ProviderKey]map[string]struct{}
	// converter is the condition converter.
	converter *Converter
}

// NewStore creates a new in-memory account storage instance.
func NewStore() *Store {
	return &Store{
		accounts:      make(map[string]*account.Account),
		providerIndex: make(map[provider.ProviderKey]map[string]struct{}),
		converter:     &Converter{},
	}
}

func (s *Store) Get(_ context.Context, id string) (*account.Account, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	acct, ok := s.accounts[id]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return acct.Clone(), nil
}

func (s *Store) Search(_ context.Context, filter *filtercond.Filter) ([]*account.Account, error) {
	filterFn, err := s.converter.Convert(filter)
	if err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*account.Account, 0)
	for _, acct := range s.accounts {
		if filterFn(acct) {
			result = append(result, acct.Clone())
		}
	}
	return result, nil
}

func (s *Store) Add(_ context.Context, acct *account.Account) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.accounts[acct.ID]; exists {
		return storage.ErrAlreadyExists
	}

	now := time.Now()
	stored := acct.Clone()
	if stored.CreatedAt.IsZero() {
		stored.CreatedAt = now
	}
	stored.UpdatedAt = now

	s.accounts[acct.ID] = stored
	s.addToIndex(stored)
	return nil
}

func (s *Store) Update(_ context.Context, acct *account.Account) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	old, exists := s.accounts[acct.ID]
	if !exists {
		return storage.ErrNotFound
	}

	// Remove from old index first
	s.removeFromIndex(old)

	stored := acct.Clone()
	stored.UpdatedAt = time.Now()

	s.accounts[acct.ID] = stored
	// Add to new index
	s.addToIndex(stored)
	return nil
}

func (s *Store) Remove(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	acct, exists := s.accounts[id]
	if !exists {
		return storage.ErrNotFound
	}

	s.removeFromIndex(acct)
	delete(s.accounts, id)
	return nil
}

func (s *Store) RemoveFilter(_ context.Context, filter *filtercond.Filter) error {
	filterFn, err := s.converter.Convert(filter)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for id, acct := range s.accounts {
		if filterFn(acct) {
			s.removeFromIndex(acct)
			delete(s.accounts, id)
		}
	}
	return nil
}

func (s *Store) Count(_ context.Context, filter *filtercond.Filter) (int, error) {
	filterFn, err := s.converter.Convert(filter)
	if err != nil {
		return 0, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	for _, acct := range s.accounts {
		if filterFn(acct) {
			count++
		}
	}
	return count, nil
}

func (s *Store) CountByProvider(_ context.Context, key provider.ProviderKey, filter *filtercond.Filter) (int, error) {
	filterFn, err := s.converter.Convert(filter)
	if err != nil {
		return 0, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	ids, ok := s.providerIndex[key]
	if !ok {
		return 0, nil
	}

	count := 0
	for id := range ids {
		acct, exists := s.accounts[id]
		if !exists {
			continue
		}
		if filterFn(acct) {
			count++
		}
	}
	return count, nil
}

// --- Internal helper methods ---

// providerKeyOf returns the ProviderKey for the given Account.
func providerKeyOf(acct *account.Account) provider.ProviderKey {
	return acct.ProviderKey()
}

// addToIndex adds the account to the ProviderKey secondary index.
func (s *Store) addToIndex(acct *account.Account) {
	key := providerKeyOf(acct)
	if s.providerIndex[key] == nil {
		s.providerIndex[key] = make(map[string]struct{})
	}
	s.providerIndex[key][acct.ID] = struct{}{}
}

// removeFromIndex removes the account from the ProviderKey secondary index.
func (s *Store) removeFromIndex(acct *account.Account) {
	key := providerKeyOf(acct)
	if ids, ok := s.providerIndex[key]; ok {
		delete(ids, acct.ID)
		if len(ids) == 0 {
			delete(s.providerIndex, key)
		}
	}
}
