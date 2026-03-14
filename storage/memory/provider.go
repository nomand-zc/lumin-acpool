package memory

import (
	"context"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-acpool/storage/filtercond"
)

func (s *Store) GetProvider(_ context.Context, key account.ProviderKey) (*account.ProviderInfo, error) {
	s.provMu.RLock()
	defer s.provMu.RUnlock()

	info, ok := s.providers[key]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return info.Clone(), nil
}

func (s *Store) SearchProviders(_ context.Context, filter *storage.SearchFilter) ([]*account.ProviderInfo, error) {
	var cond *filtercond.Filter
	if filter != nil {
		cond = filter.ExtraCond
	}
	filterFn, err := s.provConverter.Convert(cond)
	if err != nil {
		return nil, err
	}

	s.provMu.RLock()
	defer s.provMu.RUnlock()

	// 如果指定了 SupportedModel，先利用 modelIndex 缩小候选集
	candidates := s.providers
	if filter != nil && filter.SupportedModel != "" {
		keys, ok := s.provModelIndex[filter.SupportedModel]
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
		if !matchProviderSearchFilter(info, filter) {
			continue
		}
		if filterFn(info) {
			result = append(result, info.Clone())
		}
	}
	return result, nil
}

// matchProviderSearchFilter 检查供应商是否匹配 SearchFilter 的一级字段条件。
func matchProviderSearchFilter(info *account.ProviderInfo, filter *storage.SearchFilter) bool {
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

func (s *Store) AddProvider(_ context.Context, info *account.ProviderInfo) error {
	s.provMu.Lock()
	defer s.provMu.Unlock()

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
	s.provAddToIndex(stored)
	return nil
}

func (s *Store) UpdateProvider(_ context.Context, info *account.ProviderInfo) error {
	s.provMu.Lock()
	defer s.provMu.Unlock()

	key := info.ProviderKey()
	old, exists := s.providers[key]
	if !exists {
		return storage.ErrNotFound
	}

	// 先从旧索引移除
	s.provRemoveFromIndex(old)

	stored := info.Clone()
	stored.UpdatedAt = time.Now()

	s.providers[key] = stored
	// 添加到新索引
	s.provAddToIndex(stored)
	return nil
}

func (s *Store) RemoveProvider(_ context.Context, key account.ProviderKey) error {
	s.provMu.Lock()
	defer s.provMu.Unlock()

	info, exists := s.providers[key]
	if !exists {
		return storage.ErrNotFound
	}

	// 级联删除该 Provider 下的所有 Account 及关联数据
	s.acctMu.Lock()
	if ids, ok := s.acctProviderIndex[key]; ok {
		for id := range ids {
			if acct, acctExists := s.accounts[id]; acctExists {
				s.acctRemoveFromIndex(acct)
				delete(s.accounts, id)
			}
			// 删除关联的统计数据
			s.statsMu.Lock()
			delete(s.statsStore, id)
			s.statsMu.Unlock()
			// 删除关联的用量追踪数据
			s.usageMu.Lock()
			delete(s.usageStore, id)
			s.usageMu.Unlock()
		}
	}
	s.acctMu.Unlock()

	s.provRemoveFromIndex(info)
	delete(s.providers, key)
	return nil
}

// --- Provider 索引维护方法 ---

// provAddToIndex 将供应商添加到 Type 和 Model 索引。
func (s *Store) provAddToIndex(info *account.ProviderInfo) {
	key := info.ProviderKey()
	// Type 索引
	if s.provTypeIndex[info.ProviderType] == nil {
		s.provTypeIndex[info.ProviderType] = make(map[account.ProviderKey]struct{})
	}
	s.provTypeIndex[info.ProviderType][key] = struct{}{}

	// Model 索引
	for _, model := range info.SupportedModels {
		if s.provModelIndex[model] == nil {
			s.provModelIndex[model] = make(map[account.ProviderKey]struct{})
		}
		s.provModelIndex[model][key] = struct{}{}
	}
}

// provRemoveFromIndex 将供应商从 Type 和 Model 索引移除。
func (s *Store) provRemoveFromIndex(info *account.ProviderInfo) {
	key := info.ProviderKey()
	// Type 索引
	if keys, ok := s.provTypeIndex[info.ProviderType]; ok {
		delete(keys, key)
		if len(keys) == 0 {
			delete(s.provTypeIndex, info.ProviderType)
		}
	}

	// Model 索引
	for _, model := range info.SupportedModels {
		if keys, ok := s.provModelIndex[model]; ok {
			delete(keys, key)
			if len(keys) == 0 {
				delete(s.provModelIndex, model)
			}
		}
	}
}
