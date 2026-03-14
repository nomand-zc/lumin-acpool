package memory

import (
	"context"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
)

// statsGetOrCreate 获取或创建统计记录（调用方必须持有锁）。
func (s *Store) statsGetOrCreate(accountID string) *account.AccountStats {
	st, ok := s.statsStore[accountID]
	if !ok {
		st = &account.AccountStats{AccountID: accountID}
		s.statsStore[accountID] = st
	}
	return st
}

func (s *Store) GetStats(_ context.Context, accountID string) (*account.AccountStats, error) {
	s.statsMu.Lock()
	defer s.statsMu.Unlock()

	st := s.statsGetOrCreate(accountID)
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

func (s *Store) IncrSuccess(_ context.Context, accountID string) error {
	s.statsMu.Lock()
	defer s.statsMu.Unlock()

	st := s.statsGetOrCreate(accountID)
	st.TotalCalls++
	st.SuccessCalls++
	st.ConsecutiveFailures = 0
	now := time.Now()
	st.LastUsedAt = &now
	return nil
}

func (s *Store) IncrFailure(_ context.Context, accountID string, errMsg string) (int, error) {
	s.statsMu.Lock()
	defer s.statsMu.Unlock()

	st := s.statsGetOrCreate(accountID)
	st.TotalCalls++
	st.FailedCalls++
	st.ConsecutiveFailures++
	now := time.Now()
	st.LastErrorAt = &now
	st.LastErrorMsg = errMsg
	return st.ConsecutiveFailures, nil
}

func (s *Store) UpdateLastUsed(_ context.Context, accountID string, t time.Time) error {
	s.statsMu.Lock()
	defer s.statsMu.Unlock()

	st := s.statsGetOrCreate(accountID)
	st.LastUsedAt = &t
	return nil
}

func (s *Store) GetConsecutiveFailures(_ context.Context, accountID string) (int, error) {
	s.statsMu.Lock()
	defer s.statsMu.Unlock()

	st := s.statsGetOrCreate(accountID)
	return st.ConsecutiveFailures, nil
}

func (s *Store) ResetConsecutiveFailures(_ context.Context, accountID string) error {
	s.statsMu.Lock()
	defer s.statsMu.Unlock()

	st := s.statsGetOrCreate(accountID)
	st.ConsecutiveFailures = 0
	return nil
}

func (s *Store) RemoveStats(_ context.Context, accountID string) error {
	s.statsMu.Lock()
	defer s.statsMu.Unlock()

	delete(s.statsStore, accountID)
	return nil
}
