package memory

import (
	"context"
	"fmt"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
)

func (s *Store) GetAllUsages(_ context.Context, accountID string) ([]*account.TrackedUsage, error) {
	s.usageMu.Lock()
	defer s.usageMu.Unlock()

	usages, ok := s.usageStore[accountID]
	if !ok {
		return nil, nil
	}

	// 返回副本，避免外部修改内部状态
	result := make([]*account.TrackedUsage, len(usages))
	for i, u := range usages {
		cp := *u
		result[i] = &cp
	}
	return result, nil
}

func (s *Store) SaveUsages(_ context.Context, accountID string, usages []*account.TrackedUsage) error {
	s.usageMu.Lock()
	defer s.usageMu.Unlock()

	// 存储副本，避免外部后续修改影响内部状态
	stored := make([]*account.TrackedUsage, len(usages))
	for i, u := range usages {
		cp := *u
		stored[i] = &cp
	}
	s.usageStore[accountID] = stored
	return nil
}

func (s *Store) IncrLocalUsed(_ context.Context, accountID string, ruleIndex int, amount float64) error {
	s.usageMu.Lock()
	defer s.usageMu.Unlock()

	usages, ok := s.usageStore[accountID]
	if !ok {
		return nil // 未初始化，静默忽略
	}

	if ruleIndex < 0 || ruleIndex >= len(usages) {
		return fmt.Errorf("memory store: rule index %d out of range [0, %d)", ruleIndex, len(usages))
	}

	usages[ruleIndex].LocalUsed += amount
	return nil
}

func (s *Store) RemoveUsages(_ context.Context, accountID string) error {
	s.usageMu.Lock()
	defer s.usageMu.Unlock()

	delete(s.usageStore, accountID)
	return nil
}

func (s *Store) CalibrateRule(_ context.Context, accountID string, ruleIndex int, usage *account.TrackedUsage) error {
	s.usageMu.Lock()
	defer s.usageMu.Unlock()

	usages, ok := s.usageStore[accountID]
	if !ok {
		return nil // 未初始化，静默忽略
	}

	if ruleIndex < 0 || ruleIndex >= len(usages) {
		return fmt.Errorf("memory store: rule index %d out of range [0, %d)", ruleIndex, len(usages))
	}

	u := usages[ruleIndex]
	u.RemoteUsed = usage.RemoteUsed
	u.RemoteRemain = usage.RemoteRemain
	u.LocalUsed = 0
	u.WindowStart = usage.WindowStart
	u.WindowEnd = usage.WindowEnd
	u.LastSyncAt = time.Now()
	return nil
}
