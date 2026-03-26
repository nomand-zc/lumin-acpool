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

	// 事务性更新 Provider 计数
	s.provMu.Lock()
	key := stored.ProviderKey()
	if prov, ok := s.providers[key]; ok {
		prov.AccountCount++
		if stored.Status == account.StatusAvailable {
			prov.AvailableAccountCount++
		}
		prov.UpdatedAt = now
	}
	s.provMu.Unlock()

	return nil
}

func (s *Store) UpdateAccount(_ context.Context, acct *account.Account, fields storage.UpdateField) error {
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

	oldStatus := old.Status

	// 按 fields 选择性更新字段
	if fields.Has(storage.UpdateFieldCredential) {
		old.Credential = acct.Credential
	}
	if fields.Has(storage.UpdateFieldStatus) {
		old.Status = acct.Status
		old.CooldownUntil = acct.CooldownUntil
		old.CircuitOpenUntil = acct.CircuitOpenUntil
	}
	if fields.Has(storage.UpdateFieldPriority) {
		old.Priority = acct.Priority
	}
	if fields.Has(storage.UpdateFieldTags) {
		old.Tags = acct.Tags
	}
	if fields.Has(storage.UpdateFieldMetadata) {
		old.Metadata = acct.Metadata
	}
	if fields.Has(storage.UpdateFieldUsageRules) {
		old.UsageRules = acct.UsageRules
	}

	old.UpdatedAt = time.Now()
	old.Version++ // 递增版本号

	// 如果状态发生变更，更新 Provider 可用计数
	if fields.Has(storage.UpdateFieldStatus) && oldStatus != old.Status {
		s.provMu.Lock()
		key := old.ProviderKey()
		if prov, ok := s.providers[key]; ok {
			if oldStatus == account.StatusAvailable && old.Status != account.StatusAvailable {
				if prov.AvailableAccountCount > 0 {
					prov.AvailableAccountCount--
				}
			} else if oldStatus != account.StatusAvailable && old.Status == account.StatusAvailable {
				prov.AvailableAccountCount++
			}
			prov.UpdatedAt = time.Now()
		}
		s.provMu.Unlock()
	}

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

	// 事务性更新 Provider 计数
	s.provMu.Lock()
	key := acct.ProviderKey()
	if prov, ok := s.providers[key]; ok {
		if prov.AccountCount > 0 {
			prov.AccountCount--
		}
		if acct.Status == account.StatusAvailable && prov.AvailableAccountCount > 0 {
			prov.AvailableAccountCount--
		}
		prov.UpdatedAt = time.Now()
	}
	s.provMu.Unlock()

	// 删除关联的统计数据
	s.statsMu.Lock()
	delete(s.statsStore, id)
	s.statsMu.Unlock()

	// 删除关联的用量追踪数据
	s.usageMu.Lock()
	delete(s.usageStore, id)
	s.usageMu.Unlock()

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

	type removedEntry struct {
		id   string
		acct *account.Account
	}
	var toRemove []removedEntry

	s.acctMu.Lock()
	for id, acct := range s.accounts {
		if !matchAccountSearchFilter(acct, filter) {
			continue
		}
		if filterFn(acct) {
			s.acctRemoveFromIndex(acct)
			delete(s.accounts, id)
			toRemove = append(toRemove, removedEntry{id: id, acct: acct})
		}
	}
	s.acctMu.Unlock()

	if len(toRemove) == 0 {
		return nil
	}

	// 按照与 RemoveAccount 相同的锁顺序：provMu → statsMu → usageMu

	// 更新 Provider 计数
	s.provMu.Lock()
	for _, entry := range toRemove {
		key := entry.acct.ProviderKey()
		if prov, ok := s.providers[key]; ok {
			if prov.AccountCount > 0 {
				prov.AccountCount--
			}
			if entry.acct.Status == account.StatusAvailable && prov.AvailableAccountCount > 0 {
				prov.AvailableAccountCount--
			}
			prov.UpdatedAt = time.Now()
		}
	}
	s.provMu.Unlock()

	// 清理关联的统计数据
	s.statsMu.Lock()
	for _, entry := range toRemove {
		delete(s.statsStore, entry.id)
	}
	s.statsMu.Unlock()

	// 清理关联的用量追踪数据
	s.usageMu.Lock()
	for _, entry := range toRemove {
		delete(s.usageStore, entry.id)
	}
	s.usageMu.Unlock()

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
