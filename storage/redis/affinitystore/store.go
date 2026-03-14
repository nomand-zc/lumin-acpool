package affinitystore

import (
	"context"
	"fmt"

	"github.com/nomand-zc/lumin-acpool/storage"
	storeRedis "github.com/nomand-zc/lumin-acpool/storage/redis"
)

const (
	// Redis key 格式。
	// affinity:{key} -> String，存储亲和键到目标 ID 的映射。

	keyAffinityPrefix = "affinity:"
)

// Compile-time interface compliance check.
var _ storage.AffinityStore = (*Store)(nil)

// Store 是基于 Redis 的 AffinityStore 实现。
//
// 数据结构设计：
//   - affinity:{key} → String，值为 targetID
//
// 适用于集群部署场景，多个实例共享绑定关系。
// Redis 天然支持高并发读写，非常适合亲和存储。
type Store struct {
	client    storeRedis.Client
	keyPrefix string
}

// NewStore 创建一个新的 Redis 亲和存储实例。
func NewStore(opts ...Option) (*Store, error) {
	o := DefaultOptions()
	for _, opt := range opts {
		opt(o)
	}

	client, err := buildClient(o)
	if err != nil {
		return nil, fmt.Errorf("affinitystore: %w", err)
	}

	return &Store{
		client:    client,
		keyPrefix: o.KeyPrefix,
	}, nil
}

// affinityKey 返回亲和绑定关系的 Redis key。
func (s *Store) affinityKey(key string) string {
	return s.keyPrefix + keyAffinityPrefix + key
}

// Get 获取亲和键对应的绑定目标 ID。
func (s *Store) GetAffinity(affinityKey string) (string, bool) {
	key := s.affinityKey(affinityKey)
	val, err := s.client.Get(context.Background(), key)
	if err != nil {
		return "", false
	}
	return val, true
}

// Set 设置亲和键到目标 ID 的绑定关系。
// 使用 SET 命令直接覆盖写入。
func (s *Store) SetAffinity(affinityKey string, targetID string) {
	key := s.affinityKey(affinityKey)
	err := s.client.Set(context.Background(), key, targetID, 0)
	if err != nil {
		// AffinityStore 接口不返回 error，记录错误后静默处理
		fmt.Printf("affinitystore: failed to set affinity: %v\n", err)
	}
}
