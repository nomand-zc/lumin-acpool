package statsstore

import (
	"context"
	"sync"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
)

// Compile-time interface compliance check.
var _ storage.StatsStore = (*MemoryStatsStore)(nil)

// MemoryStatsStore 是 StatsStore 的内存实现。
// 使用互斥锁保证并发安全，适用于单机部署场景。
type MemoryStatsStore struct {
	mu    sync.Mutex
	store map[string]*account.AccountStats
}

// NewMemoryStatsStore 创建一个内存统计存储实例。
func NewMemoryStatsStore() *MemoryStatsStore {
	return &MemoryStatsStore{
		store: make(map[string]*account.AccountStats),
	}
}

// getOrCreate 获取或创建统计记录（调用方必须持有锁）。
func (s *MemoryStatsStore) getOrCreate(accountID string) *account.AccountStats {
	st, ok := s.store[accountID]
	if !ok {
		st = &account.AccountStats{AccountID: accountID}
		s.store[accountID] = st
	}
	return st
}

func (s *MemoryStatsStore) Get(_ context.Context, accountID string) (*account.AccountStats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	st := s.getOrCreate(accountID)
	// 返回副本，避免外部修改内部状态
	cp := *st
	if st.LastUsedAt != nil {
		t := *st.LastUsedAt
		cp.LastUsedAt = &t
	}
	if st.LastErrorAt != nil {
		t := *st.LastErrorAt
		cp.LastErrorAt = &t
	}
	return &cp, nil
}

func (s *MemoryStatsStore) IncrSuccess(_ context.Context, accountID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	st := s.getOrCreate(accountID)
	st.TotalCalls++
	st.SuccessCalls++
	st.ConsecutiveFailures = 0
	now := time.Now()
	st.LastUsedAt = &now
	return nil
}

func (s *MemoryStatsStore) IncrFailure(_ context.Context, accountID string, errMsg string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	st := s.getOrCreate(accountID)
	st.TotalCalls++
	st.FailedCalls++
	st.ConsecutiveFailures++
	now := time.Now()
	st.LastErrorAt = &now
	st.LastErrorMsg = errMsg
	return st.ConsecutiveFailures, nil
}

func (s *MemoryStatsStore) UpdateLastUsed(_ context.Context, accountID string, t time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	st := s.getOrCreate(accountID)
	st.LastUsedAt = &t
	return nil
}

func (s *MemoryStatsStore) GetConsecutiveFailures(_ context.Context, accountID string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	st := s.getOrCreate(accountID)
	return st.ConsecutiveFailures, nil
}

func (s *MemoryStatsStore) ResetConsecutiveFailures(_ context.Context, accountID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	st := s.getOrCreate(accountID)
	st.ConsecutiveFailures = 0
	return nil
}

func (s *MemoryStatsStore) Remove(_ context.Context, accountID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.store, accountID)
	return nil
}
