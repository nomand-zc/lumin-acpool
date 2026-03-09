package memory

import (
	"context"
	"sync"
	"time"

	"github.com/nomand-zc/lumin-acpool/filtercond"
	"github.com/nomand-zc/lumin-acpool/provider"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-client/usagerule"
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
	return s.copyProviderInfo(info), nil
}

func (s *ProviderStore) List(_ context.Context, filter *filtercond.Filter) ([]*provider.ProviderInfo, error) {
	filterFn, err := s.converter.Convert(filter)
	if err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*provider.ProviderInfo, 0)
	for _, info := range s.providers {
		if filterFn(info) {
			result = append(result, s.copyProviderInfo(info))
		}
	}
	return result, nil
}

func (s *ProviderStore) ListByType(_ context.Context, providerType string, filter *filtercond.Filter) ([]*provider.ProviderInfo, error) {
	filterFn, err := s.converter.Convert(filter)
	if err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	keys, ok := s.typeIndex[providerType]
	if !ok {
		return []*provider.ProviderInfo{}, nil
	}

	result := make([]*provider.ProviderInfo, 0, len(keys))
	for key := range keys {
		info, exists := s.providers[key]
		if !exists {
			continue
		}
		if filterFn(info) {
			result = append(result, s.copyProviderInfo(info))
		}
	}
	return result, nil
}

func (s *ProviderStore) ListByModel(_ context.Context, model string, filter *filtercond.Filter) ([]*provider.ProviderInfo, error) {
	filterFn, err := s.converter.Convert(filter)
	if err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	keys, ok := s.modelIndex[model]
	if !ok {
		return []*provider.ProviderInfo{}, nil
	}

	result := make([]*provider.ProviderInfo, 0, len(keys))
	for key := range keys {
		info, exists := s.providers[key]
		if !exists {
			continue
		}
		if filterFn(info) {
			result = append(result, s.copyProviderInfo(info))
		}
	}
	return result, nil
}

func (s *ProviderStore) Add(_ context.Context, info *provider.ProviderInfo) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.providers[info.Key]; exists {
		return storage.ErrAlreadyExists
	}

	now := time.Now()
	stored := s.copyProviderInfo(info)
	if stored.CreatedAt.IsZero() {
		stored.CreatedAt = now
	}
	stored.UpdatedAt = now

	s.providers[info.Key] = stored
	s.addToIndex(stored)
	return nil
}

func (s *ProviderStore) Update(_ context.Context, info *provider.ProviderInfo) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	old, exists := s.providers[info.Key]
	if !exists {
		return storage.ErrNotFound
	}

	// 先从旧索引中移除
	s.removeFromIndex(old)

	stored := s.copyProviderInfo(info)
	stored.UpdatedAt = time.Now()

	s.providers[info.Key] = stored
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
	// 类型索引
	if s.typeIndex[info.Key.Type] == nil {
		s.typeIndex[info.Key.Type] = make(map[provider.ProviderKey]struct{})
	}
	s.typeIndex[info.Key.Type][info.Key] = struct{}{}

	// 模型索引
	for _, model := range info.SupportedModels {
		if s.modelIndex[model] == nil {
			s.modelIndex[model] = make(map[provider.ProviderKey]struct{})
		}
		s.modelIndex[model][info.Key] = struct{}{}
	}
}

// removeFromIndex 从 Type 和 Model 索引中移除供应商
func (s *ProviderStore) removeFromIndex(info *provider.ProviderInfo) {
	// 类型索引
	if keys, ok := s.typeIndex[info.Key.Type]; ok {
		delete(keys, info.Key)
		if len(keys) == 0 {
			delete(s.typeIndex, info.Key.Type)
		}
	}

	// 模型索引
	for _, model := range info.SupportedModels {
		if keys, ok := s.modelIndex[model]; ok {
			delete(keys, info.Key)
			if len(keys) == 0 {
				delete(s.modelIndex, model)
			}
		}
	}
}

// copyProviderInfo 深拷贝 ProviderInfo，防止外部修改内部存储数据
func (s *ProviderStore) copyProviderInfo(src *provider.ProviderInfo) *provider.ProviderInfo {
	dst := *src

	// 深拷贝 Tags
	if src.Tags != nil {
		dst.Tags = make(map[string]string, len(src.Tags))
		for k, v := range src.Tags {
			dst.Tags[k] = v
		}
	}

	// 深拷贝 SupportedModels
	if src.SupportedModels != nil {
		dst.SupportedModels = make([]string, len(src.SupportedModels))
		copy(dst.SupportedModels, src.SupportedModels)
	}

	// 深拷贝 UsageRules
	if src.UsageRules != nil {
		dst.UsageRules = make([]*usagerule.UsageRule, len(src.UsageRules))
		copy(dst.UsageRules, src.UsageRules)
	}

	// 深拷贝 Metadata
	if src.Metadata != nil {
		dst.Metadata = make(map[string]any, len(src.Metadata))
		for k, v := range src.Metadata {
			dst.Metadata[k] = v
		}
	}

	return &dst
}
