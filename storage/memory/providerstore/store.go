package providerstore

import (
	"context"
	"sync"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-acpool/storage/filtercond"
)

// Compile-time interface compliance check.
var _ storage.ProviderStorage = (*Store)(nil)

// Store is the in-memory storage implementation for providers.
// Uses a read-write lock for concurrency safety and maintains Type and Model secondary indexes for hot-path query acceleration.
type Store struct {
	mu sync.RWMutex
	// providers is the primary storage: ProviderKey -> ProviderInfo
	providers map[account.ProviderKey]*account.ProviderInfo
	// typeIndex is the type index: providerType -> ProviderKey set
	typeIndex map[string]map[account.ProviderKey]struct{}
	// modelIndex is the model index: model -> ProviderKey set
	modelIndex map[string]map[account.ProviderKey]struct{}
	// converter is the condition converter.
	converter *Converter
}

// NewStore creates a new in-memory provider storage instance.
func NewStore() *Store {
	return &Store{
		providers:  make(map[account.ProviderKey]*account.ProviderInfo),
		typeIndex:  make(map[string]map[account.ProviderKey]struct{}),
		modelIndex: make(map[string]map[account.ProviderKey]struct{}),
		converter:  &Converter{},
	}
}

func (s *Store) Get(_ context.Context, key account.ProviderKey) (*account.ProviderInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	info, ok := s.providers[key]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return info.Clone(), nil
}

func (s *Store) Search(_ context.Context, filter *storage.SearchFilter) ([]*account.ProviderInfo, error) {
	var cond *filtercond.Filter
	if filter != nil {
		cond = filter.ExtraCond
	}
	filterFn, err := s.converter.Convert(cond)
	if err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	// 如果指定了 SupportedModel，先利用 modelIndex 缩小候选集
	candidates := s.providers
	if filter != nil && filter.SupportedModel != "" {
		keys, ok := s.modelIndex[filter.SupportedModel]
		if !ok || len(keys) == 0 {
			return nil, nil
		}
		candidates = make(map[account.ProviderKey]*account.ProviderInfo, len(keys))
		for key := range keys {
			if info, exists := s.providers[key]; exists {
				candidates[key] = info
			}
		}
	}

	result := make([]*account.ProviderInfo, 0)
	for _, info := range candidates {
		if !matchSearchFilter(info, filter) {
			continue
		}
		if filterFn(info) {
			result = append(result, info.Clone())
		}
	}
	return result, nil
}

// matchSearchFilter 检查供应商是否匹配 SearchFilter 的一级字段条件。
func matchSearchFilter(info *account.ProviderInfo, filter *storage.SearchFilter) bool {
	if filter == nil {
		return true
	}
	if filter.ProviderType != "" && info.ProviderType != filter.ProviderType {
		return false
	}
	if filter.ProviderName != "" && info.ProviderName != filter.ProviderName {
		return false
	}
	if filter.Status != 0 && int(info.Status) != filter.Status {
		return false
	}
	// SupportedModel 已通过 modelIndex 预过滤，此处无需重复检查
	return true
}

func (s *Store) Add(_ context.Context, info *account.ProviderInfo) error {
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

func (s *Store) Update(_ context.Context, info *account.ProviderInfo) error {
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

func (s *Store) Remove(_ context.Context, key account.ProviderKey) error {
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
func (s *Store) addToIndex(info *account.ProviderInfo) {
	key := info.ProviderKey()
	// Type index
	if s.typeIndex[info.ProviderType] == nil {
		s.typeIndex[info.ProviderType] = make(map[account.ProviderKey]struct{})
	}
	s.typeIndex[info.ProviderType][key] = struct{}{}

	// Model index
	for _, model := range info.SupportedModels {
		if s.modelIndex[model] == nil {
			s.modelIndex[model] = make(map[account.ProviderKey]struct{})
		}
		s.modelIndex[model][key] = struct{}{}
	}
}

// removeFromIndex removes the provider from the Type and Model indexes.
func (s *Store) removeFromIndex(info *account.ProviderInfo) {
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
