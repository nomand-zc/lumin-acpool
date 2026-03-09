package memory

import (
	"context"
	"sync"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/provider"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-acpool/storage/filtercond"
	"github.com/nomand-zc/lumin-client/usagerule"
)

// AccountStore 账号的内存存储实现
// 使用读写锁保证并发安全，维护 ProviderKey 二级索引加速热路径查询
type AccountStore struct {
	mu sync.RWMutex
	// accounts 主存储：id -> Account
	accounts map[string]*account.Account
	// providerIndex 二级索引：ProviderKey -> id 集合
	providerIndex map[provider.ProviderKey]map[string]struct{}
	// converter 条件转换器
	converter *AccountConverter
}

// NewAccountStore 创建一个新的内存账号存储实例
func NewAccountStore() *AccountStore {
	return &AccountStore{
		accounts:      make(map[string]*account.Account),
		providerIndex: make(map[provider.ProviderKey]map[string]struct{}),
		converter:     &AccountConverter{},
	}
}

func (s *AccountStore) Get(_ context.Context, id string) (*account.Account, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	acct, ok := s.accounts[id]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return s.copyAccount(acct), nil
}

func (s *AccountStore) Search(_ context.Context, filter *filtercond.Filter) ([]*account.Account, error) {
	filterFn, err := s.converter.Convert(filter)
	if err != nil {
		return nil, err
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*account.Account, 0)
	for _, acct := range s.accounts {
		if filterFn(acct) {
			result = append(result, s.copyAccount(acct))
		}
	}
	return result, nil
}

func (s *AccountStore) Add(_ context.Context, acct *account.Account) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.accounts[acct.ID]; exists {
		return storage.ErrAlreadyExists
	}

	now := time.Now()
	stored := s.copyAccount(acct)
	if stored.CreatedAt.IsZero() {
		stored.CreatedAt = now
	}
	stored.UpdatedAt = now

	s.accounts[acct.ID] = stored
	s.addToIndex(stored)
	return nil
}

func (s *AccountStore) Update(_ context.Context, acct *account.Account) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	old, exists := s.accounts[acct.ID]
	if !exists {
		return storage.ErrNotFound
	}

	// 先从旧的索引中移除
	s.removeFromIndex(old)

	stored := s.copyAccount(acct)
	stored.UpdatedAt = time.Now()

	s.accounts[acct.ID] = stored
	// 添加到新的索引
	s.addToIndex(stored)
	return nil
}

func (s *AccountStore) Remove(_ context.Context, id string) error {
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

func (s *AccountStore) RemoveFilter(_ context.Context, filter *filtercond.Filter) error {
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

func (s *AccountStore) Count(_ context.Context, filter *filtercond.Filter) (int, error) {
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

func (s *AccountStore) CountByProvider(_ context.Context, key provider.ProviderKey, filter *filtercond.Filter) (int, error) {
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

// --- 内部辅助方法 ---

// providerKeyOf 获取 Account 对应的 ProviderKey
func providerKeyOf(acct *account.Account) provider.ProviderKey {
	return acct.ProviderKey()
}

// addToIndex 将账号添加到 ProviderKey 二级索引
func (s *AccountStore) addToIndex(acct *account.Account) {
	key := providerKeyOf(acct)
	if s.providerIndex[key] == nil {
		s.providerIndex[key] = make(map[string]struct{})
	}
	s.providerIndex[key][acct.ID] = struct{}{}
}

// removeFromIndex 将账号从 ProviderKey 二级索引中移除
func (s *AccountStore) removeFromIndex(acct *account.Account) {
	key := providerKeyOf(acct)
	if ids, ok := s.providerIndex[key]; ok {
		delete(ids, acct.ID)
		if len(ids) == 0 {
			delete(s.providerIndex, key)
		}
	}
}

// copyAccount 深拷贝 Account，防止外部修改内部存储数据
func (s *AccountStore) copyAccount(src *account.Account) *account.Account {
	dst := *src

	// 深拷贝 Tags
	if src.Tags != nil {
		dst.Tags = make(map[string]string, len(src.Tags))
		for k, v := range src.Tags {
			dst.Tags[k] = v
		}
	}

	// 深拷贝 Metadata
	if src.Metadata != nil {
		dst.Metadata = make(map[string]any, len(src.Metadata))
		for k, v := range src.Metadata {
			dst.Metadata[k] = v
		}
	}

	// 深拷贝 UsageStats
	if src.UsageStats != nil {
		dst.UsageStats = make([]*usagerule.UsageStats, len(src.UsageStats))
		copy(dst.UsageStats, src.UsageStats)
	}

	// 深拷贝时间指针
	if src.LastUsedAt != nil {
		t := *src.LastUsedAt
		dst.LastUsedAt = &t
	}
	if src.LastErrorAt != nil {
		t := *src.LastErrorAt
		dst.LastErrorAt = &t
	}
	if src.CooldownUntil != nil {
		t := *src.CooldownUntil
		dst.CooldownUntil = &t
	}
	if src.CircuitOpenUntil != nil {
		t := *src.CircuitOpenUntil
		dst.CircuitOpenUntil = &t
	}

	return &dst
}
