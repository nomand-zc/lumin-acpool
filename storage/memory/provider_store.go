package memory

import (
	"context"
	"sync"
	"time"

	"github.com/nomand-zc/lumin-acpool/provider"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-acpool/storage/filtercond"
)

// ProviderStore is the in-memory storage implementation for providers.
// Uses a read-write lock for concurrency safety and maintains Type and Model secondary indexes for hot-path query acceleration.
type ProviderStore struct {
	mu sync.RWMutex
	// providers is the primary storage: ProviderKey -> ProviderInfo
	providers map[provider.ProviderKey]*provider.ProviderInfo
	// typeIndex is the type index: providerType -> ProviderKey set
	typeIndex map[string]map[provider.ProviderKey]struct{}
	// modelIndex is the model index: model -> ProviderKey set
	modelIndex map[string]map[provider.ProviderKey]struct{}
	// converter is the condition converter.
	converter *ProviderConverter
}

// NewProviderStore creates a new in-memory provider storage instance.
func NewProviderStore() *ProviderStore {
	return &ProviderStore{
		providers:  make(map[provider.ProviderKey]*provider.ProviderInfo),
		typeIndex:  make(map[string]map[provider.ProviderKey]struct{}),
		modelIndex: make(map[string]map[provider.ProviderKey]struct{}),
		converter:  &ProviderConverter{},
	}
}

func (s *ProviderStore) Get(_ context.Context, key provider.ProviderKey) (*provider.ProviderInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	info, ok := s.providers[key]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return info.Clone(), nil
}

func (s *ProviderStore) Search(_ context.Context, filter *filtercond.Filter) ([]*provider.ProviderInfo, error) {
	filterFn, err := s.converter.Convert(filter)
	if err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*provider.ProviderInfo, 0)
	for _, info := range s.providers {
		if filterFn(info) {
			result = append(result, info.Clone())
		}
	}
	return result, nil
}

func (s *ProviderStore) Add(_ context.Context, info *provider.ProviderInfo) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := info.ProviderKey()
	if _, exists := s.providers[key]; exists {
		return storage.ErrAlreadyExists
	}

	now := time.Now()
	stored := info.Clone()
	if stored.CreatedAt.IsZero() {
		stored.CreatedAt = now
	}
	stored.UpdatedAt = now

	s.providers[key] = stored
	s.addToIndex(stored)
	return nil
}

func (s *ProviderStore) Update(_ context.Context, info *provider.ProviderInfo) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := info.ProviderKey()
	old, exists := s.providers[key]
	if !exists {
		return storage.ErrNotFound
	}

	// Remove from old indexes first
	s.removeFromIndex(old)

	stored := info.Clone()
	stored.UpdatedAt = time.Now()

	s.providers[key] = stored
	// Add to new indexes
	s.addToIndex(stored)
	return nil
}

func (s *ProviderStore) Remove(_ context.Context, key provider.ProviderKey) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	info, exists := s.providers[key]
	if !exists {
		return storage.ErrNotFound
	}

	s.removeFromIndex(info)
	delete(s.providers, key)
	return nil
}

// --- Index maintenance methods ---

// addToIndex adds the provider to the Type and Model indexes.
func (s *ProviderStore) addToIndex(info *provider.ProviderInfo) {
	key := info.ProviderKey()
	// Type index
	if s.typeIndex[info.ProviderType] == nil {
		s.typeIndex[info.ProviderType] = make(map[provider.ProviderKey]struct{})
	}
	s.typeIndex[info.ProviderType][key] = struct{}{}

	// Model index
	for _, model := range info.SupportedModels {
		if s.modelIndex[model] == nil {
			s.modelIndex[model] = make(map[provider.ProviderKey]struct{})
		}
		s.modelIndex[model][key] = struct{}{}
	}
}

// removeFromIndex removes the provider from the Type and Model indexes.
func (s *ProviderStore) removeFromIndex(info *provider.ProviderInfo) {
	key := info.ProviderKey()
	// Type index
	if keys, ok := s.typeIndex[info.ProviderType]; ok {
		delete(keys, key)
		if len(keys) == 0 {
			delete(s.typeIndex, info.ProviderType)
		}
	}

	// Model index
	for _, model := range info.SupportedModels {
		if keys, ok := s.modelIndex[model]; ok {
			delete(keys, key)
			if len(keys) == 0 {
				delete(s.modelIndex, model)
			}
		}
	}
}
