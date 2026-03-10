package usagestore

import (
	"context"
	"fmt"
	"sync"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
)

// Compile-time interface compliance check.
var _ storage.UsageStore = (*MemoryUsageStore)(nil)

// MemoryUsageStore 是 UsageStore 的内存实现。
// 使用互斥锁保证并发安全，适用于单机部署场景。
type MemoryUsageStore struct {
	mu    sync.Mutex
	store map[string][]*account.TrackedUsage
}

// NewMemoryUsageStore 创建一个内存用量存储实例。
func NewMemoryUsageStore() *MemoryUsageStore {
	return &MemoryUsageStore{
		store: make(map[string][]*account.TrackedUsage),
	}
}

func (s *MemoryUsageStore) GetAll(_ context.Context, accountID string) ([]*account.TrackedUsage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	usages, ok := s.store[accountID]
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

func (s *MemoryUsageStore) Save(_ context.Context, accountID string, usages []*account.TrackedUsage) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 存储副本，避免外部后续修改影响内部状态
	stored := make([]*account.TrackedUsage, len(usages))
	for i, u := range usages {
		cp := *u
		stored[i] = &cp
	}
	s.store[accountID] = stored
	return nil
}

func (s *MemoryUsageStore) IncrLocalUsed(_ context.Context, accountID string, ruleIndex int, amount float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	usages, ok := s.store[accountID]
	if !ok {
		return nil // 未初始化，静默忽略
	}

	if ruleIndex < 0 || ruleIndex >= len(usages) {
		return fmt.Errorf("usagestore: rule index %d out of range [0, %d)", ruleIndex, len(usages))
	}

	usages[ruleIndex].LocalUsed += amount
	return nil
}

func (s *MemoryUsageStore) Remove(_ context.Context, accountID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.store, accountID)
	return nil
}
