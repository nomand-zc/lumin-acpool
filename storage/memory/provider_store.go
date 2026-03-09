package memory

import (
	"context"
	"sync"
	"time"

	"github.com/nomand-zc/lumin-acpool/provider"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-acpool/storage/filtercond"
)

// ProviderStore 供应商的内存存储实现
// 使用读写锁保证并发安全，维护 Type 和 Model 二级索引加速热路径查询
type ProviderStore struct {
	mu sync.RWMutex
	// providers 主存储：ProviderKey -> ProviderInfo
	providers map[provider.ProviderKey]*provider.ProviderInfo
	// typeIndex 类型索引：providerType -> ProviderKey 集合
	typeIndex map[string]map[provider.ProviderKey]struct{}
	// modelIndex 模型索引：model -> ProviderKey 集合
	modelIndex map[string]map[provider.ProviderKey]struct{}
	// converter 条件转换器
	converter *ProviderConverter
}

// NewProviderStore 创建一个新的内存供应商存储实例
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

	// 先从旧索引中移除
	s.removeFromIndex(old)

	stored := info.Clone()
	stored.UpdatedAt = time.Now()

	s.providers[key] = stored
	// 添加到新索引
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

// --- 索引维护方法 ---

// addToIndex 将供应商添加到 Type 和 Model 索引
func (s *ProviderStore) addToIndex(info *provider.ProviderInfo) {
	key := info.ProviderKey()
	// 类型索引
	if s.typeIndex[info.ProviderType] == nil {
		s.typeIndex[info.ProviderType] = make(map[provider.ProviderKey]struct{})
	}
	s.typeIndex[info.ProviderType][key] = struct{}{}

	// 模型索引
	for _, model := range info.SupportedModels {
		if s.modelIndex[model] == nil {
			s.modelIndex[model] = make(map[provider.ProviderKey]struct{})
		}
		s.modelIndex[model][key] = struct{}{}
	}
}

// removeFromIndex 从 Type 和 Model 索引中移除供应商
func (s *ProviderStore) removeFromIndex(info *provider.ProviderInfo) {
	key := info.ProviderKey()
	// 类型索引
	if keys, ok := s.typeIndex[info.ProviderType]; ok {
		delete(keys, key)
		if len(keys) == 0 {
			delete(s.typeIndex, info.ProviderType)
		}
	}

	// 模型索引
	for _, model := range info.SupportedModels {
		if keys, ok := s.modelIndex[model]; ok {
			delete(keys, key)
			if len(keys) == 0 {
				delete(s.modelIndex, model)
			}
		}
	}
}
