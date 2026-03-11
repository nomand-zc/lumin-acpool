package occupancystore

import (
	"context"
	"sync"

	"github.com/nomand-zc/lumin-acpool/storage"
)

// 编译期接口合规性检查。
var _ storage.OccupancyStore = (*MemoryOccupancyStore)(nil)

// MemoryOccupancyStore 是 OccupancyStore 的内存实现。
// 使用互斥锁保证并发安全，适用于单机部署场景。
type MemoryOccupancyStore struct {
	mu    sync.Mutex
	store map[string]int64
}

// NewMemoryOccupancyStore 创建一个内存占用计数存储实例。
func NewMemoryOccupancyStore() *MemoryOccupancyStore {
	return &MemoryOccupancyStore{
		store: make(map[string]int64),
	}
}

func (s *MemoryOccupancyStore) Incr(_ context.Context, accountID string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.store[accountID]++
	return s.store[accountID], nil
}

func (s *MemoryOccupancyStore) Decr(_ context.Context, accountID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.store[accountID] > 0 {
		s.store[accountID]--
	}
	// 清理零值键，避免内存泄漏
	if s.store[accountID] == 0 {
		delete(s.store, accountID)
	}
	return nil
}

func (s *MemoryOccupancyStore) Get(_ context.Context, accountID string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.store[accountID], nil
}
