package affinitystore

import (
	"sync"

	"github.com/nomand-zc/lumin-acpool/storage"
)

// Compile-time interface compliance check.
var _ storage.AffinityStore = (*Store)(nil)

// Store 是基于内存的 storage.AffinityStore 实现。
// 适用于单机部署场景，进程重启后绑定关系会丢失。
type Store struct {
	mu         sync.RWMutex
	bindings   map[string]string
	maxEntries int
}

// StoreOption 是 Store 的配置选项。
type StoreOption func(*Store)

// WithMaxEntries 设置映射表的最大条目数（默认：10000）。
// 超过上限时清空映射表重建，以防内存无限增长。
func WithMaxEntries(n int) StoreOption {
	return func(s *Store) {
		if n > 0 {
			s.maxEntries = n
		}
	}
}

// NewStore 创建基于内存的亲和存储实例。
func NewStore(opts ...StoreOption) *Store {
	s := &Store{
		bindings:   make(map[string]string),
		maxEntries: 10000,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Get 获取亲和键对应的绑定目标 ID。
func (s *Store) Get(affinityKey string) (string, bool) {
	s.mu.RLock()
	targetID, exists := s.bindings[affinityKey]
	s.mu.RUnlock()
	return targetID, exists
}

// Set 设置亲和键到目标 ID 的绑定关系。
func (s *Store) Set(affinityKey string, targetID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// 检查容量，超过上限时清空重建
	if len(s.bindings) >= s.maxEntries {
		s.bindings = make(map[string]string, s.maxEntries/2)
	}
	s.bindings[affinityKey] = targetID
}
