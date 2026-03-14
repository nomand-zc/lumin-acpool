package memory

import (
	"context"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-acpool/storage/filtercond"
)

func (s *Store) GetAccount(_ context.Context, id string) (*account.Account, error) {
	s.acctMu.RLock()
	defer s.acctMu.RUnlock()

	acct, ok := s.accounts[id]
	if !ok {
		return nil, storage.ErrNotFound
	}
	return acct.Clone(), nil
}

func (s *Store) SearchAccounts(_ context.Context, filter *storage.SearchFilter) ([]*account.Account, error) {
	var cond *filtercond.Filter
	if filter != nil {
		cond = filter.ExtraCond
	}
	filterFn, err := s.acctConverter.Convert(cond)
	if err != nil {
		return nil, err
	}

	s.acctMu.RLock()
	defer s.acctMu.RUnlock()

	result := make([]*account.Account, 0)
	for _, acct := range s.accounts {
		if !matchAccountSearchFilter(acct, filter) {
			continue
		}
		if filterFn(acct) {
			result = append(result, acct.Clone())
		}
	}
	return result, nil
}

// matchAccountSearchFilter 检查账号是否匹配 SearchFilter 的一级字段条件。
func matchAccountSearchFilter(acct *account.Account, filter *storage.SearchFilter) bool {
	if filter == nil {
		return true
	}
	if filter.ProviderType != "" && acct.ProviderType != filter.ProviderType {
		return false
	}
	if filter.ProviderName != "" && acct.ProviderName != filter.ProviderName {
		return false
	}
	if filter.Status != 0 && int(acct.Status) != filter.Status {
		return false
	}
	return true
}

func (s *Store) AddAccount(_ context.Context, acct *account.Account) error {
	s.acctMu.Lock()
	defer s.acctMu.Unlock()

	if _, exists := s.accounts[acct.ID]; exists {
		return storage.ErrAlreadyExists
	}

	now := time.Now()
	stored := acct.Clone()
	if stored.CreatedAt.IsZero() {
		stored.CreatedAt = now
	}
	stored.UpdatedAt = now
	stored.Version = 1

	s.accounts[acct.ID] = stored
	s.acctAddToIndex(stored)
	return nil
}

func (s *Store) UpdateAccount(_ context.Context, acct *account.Account) error {
	s.acctMu.Lock()
	defer s.acctMu.Unlock()

	old, exists := s.accounts[acct.ID]
	if !exists {
		return storage.ErrNotFound
	}

	// 乐观锁检查：版本号必须一致
	if old.Version != acct.Version {
		return storage.ErrVersionConflict
	}

	// 先从旧索引移除
	s.acctRemoveFromIndex(old)

	stored := acct.Clone()
	stored.UpdatedAt = time.Now()
	stored.Version++ // 递增版本号

	s.accounts[acct.ID] = stored
	// 添加到新索引
	s.acctAddToIndex(stored)
	return nil
}

func (s *Store) RemoveAccount(_ context.Context, id string) error {
	s.acctMu.Lock()
	defer s.acctMu.Unlock()

	acct, exists := s.accounts[id]
	if !exists {
		return storage.ErrNotFound
	}

	s.acctRemoveFromIndex(acct)
	delete(s.accounts, id)
	return nil
}

func (s *Store) RemoveAccounts(_ context.Context, filter *storage.SearchFilter) error {
	var cond *filtercond.Filter
	if filter != nil {
		cond = filter.ExtraCond
	}
	filterFn, err := s.acctConverter.Convert(cond)
	if err != nil {
		return err
	}

	s.acctMu.Lock()
	defer s.acctMu.Unlock()

	for id, acct := range s.accounts {
		if !matchAccountSearchFilter(acct, filter) {
			continue
		}
		if filterFn(acct) {
			s.acctRemoveFromIndex(acct)
			delete(s.accounts, id)
		}
	}
	return nil
}

func (s *Store) CountAccounts(_ context.Context, filter *storage.SearchFilter) (int, error) {
	var cond *filtercond.Filter
	if filter != nil {
		cond = filter.ExtraCond
	}
	filterFn, err := s.acctConverter.Convert(cond)
	if err != nil {
		return 0, err
	}

	s.acctMu.RLock()
	defer s.acctMu.RUnlock()

	count := 0
	for _, acct := range s.accounts {
		if !matchAccountSearchFilter(acct, filter) {
			continue
		}
		if filterFn(acct) {
			count++
		}
	}
	return count, nil
}

// --- Account 索引辅助方法 ---

// acctAddToIndex 将账号添加到 ProviderKey 二级索引。
func (s *Store) acctAddToIndex(acct *account.Account) {
	key := acct.ProviderKey()
	if s.acctProviderIndex[key] == nil {
		s.acctProviderIndex[key] = make(map[string]struct{})
	}
	s.acctProviderIndex[key][acct.ID] = struct{}{}
}

// acctRemoveFromIndex 将账号从 ProviderKey 二级索引移除。
func (s *Store) acctRemoveFromIndex(acct *account.Account) {
	key := acct.ProviderKey()
	if ids, ok := s.acctProviderIndex[key]; ok {
		delete(ids, acct.ID)
		if len(ids) == 0 {
			delete(s.acctProviderIndex, key)
		}
	}
}
