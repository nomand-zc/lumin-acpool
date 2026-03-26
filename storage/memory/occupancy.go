package memory

import (
	"context"
	"sync/atomic"
)

// loadOrCreateCounter 返回 accountID 对应的 *atomic.Int64，不存在时创建。
// 使用 LoadOrStore 保证并发安全，无需额外锁。
func (s *Store) loadOrCreateCounter(accountID string) *atomic.Int64 {
	if v, ok := s.occupancyStore.Load(accountID); ok {
		return v.(*atomic.Int64)
	}
	newCounter := new(atomic.Int64)
	actual, _ := s.occupancyStore.LoadOrStore(accountID, newCounter)
	return actual.(*atomic.Int64)
}

func (s *Store) IncrOccupancy(_ context.Context, accountID string) (int64, error) {
	return s.loadOrCreateCounter(accountID).Add(1), nil
}

func (s *Store) DecrOccupancy(_ context.Context, accountID string) error {
	counter := s.loadOrCreateCounter(accountID)
	for {
		cur := counter.Load()
		if cur <= 0 {
			// 已经是 0，不减到负数
			return nil
		}
		if counter.CompareAndSwap(cur, cur-1) {
			// 计数降到 0 时清理 key，避免内存泄漏
			if cur-1 == 0 {
				s.occupancyStore.Delete(accountID)
			}
			return nil
		}
		// CAS 失败，重试
	}
}

func (s *Store) GetOccupancy(_ context.Context, accountID string) (int64, error) {
	if v, ok := s.occupancyStore.Load(accountID); ok {
		return v.(*atomic.Int64).Load(), nil
	}
	return 0, nil
}

func (s *Store) GetOccupancies(_ context.Context, accountIDs []string) (map[string]int64, error) {
	result := make(map[string]int64, len(accountIDs))
	for _, id := range accountIDs {
		if v, ok := s.occupancyStore.Load(id); ok {
			result[id] = v.(*atomic.Int64).Load()
		}
	}
	return result, nil
}
