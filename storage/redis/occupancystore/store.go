package occupancystore

import (
	"context"
	"fmt"

	"github.com/nomand-zc/lumin-acpool/storage"
	storeRedis "github.com/nomand-zc/lumin-acpool/storage/redis"
)

// 编译期接口合规性检查。
var _ storage.OccupancyStore = (*Store)(nil)

// Store 是基于 Redis 的 OccupancyStore 实现。
//
// 数据结构设计：
//   - occupancy:{account_id} → String，存储当前并发占用计数
//
// 使用 Redis INCR 命令实现原子递增，使用 Lua 脚本实现原子递减（不低于 0）。
// 适用于集群部署场景，多个实例共享占用状态。
type Store struct {
	client    storeRedis.Client
	keyPrefix string
}

// NewStore 创建一个新的 Redis 占用计数存储实例。
func NewStore(opts ...Option) (*Store, error) {
	o := DefaultOptions()
	for _, opt := range opts {
		opt(o)
	}

	client, err := buildClient(o)
	if err != nil {
		return nil, fmt.Errorf("occupancystore: %w", err)
	}

	return &Store{
		client:    client,
		keyPrefix: o.KeyPrefix,
	}, nil
}

func (s *Store) IncrOccupancy(ctx context.Context, accountID string) (int64, error) {
	key := occupancyKey(s.keyPrefix, accountID)
	// 使用 Lua 脚本原子递增，Redis INCR 对不存在的 key 从 0 开始递增。
	result, err := s.client.Eval(ctx, luaIncr, []string{key})
	if err != nil {
		return 0, fmt.Errorf("occupancystore: failed to incr: %w", err)
	}
	newVal, ok := result.(int64)
	if !ok {
		return 0, fmt.Errorf("occupancystore: unexpected incr result type: %T", result)
	}
	return newVal, nil
}

func (s *Store) DecrOccupancy(ctx context.Context, accountID string) error {
	key := occupancyKey(s.keyPrefix, accountID)
	// 使用 Lua 脚本保证原子递减且不低于 0，归零时自动清理 key。
	_, err := s.client.Eval(ctx, luaDecr, []string{key})
	if err != nil {
		return fmt.Errorf("occupancystore: failed to decr: %w", err)
	}
	return nil
}

func (s *Store) GetOccupancy(ctx context.Context, accountID string) (int64, error) {
	key := occupancyKey(s.keyPrefix, accountID)
	// 使用 Lua 脚本获取当前计数，不存在返回 0。
	result, err := s.client.Eval(ctx, luaGet, []string{key})
	if err != nil {
		return 0, fmt.Errorf("occupancystore: failed to get: %w", err)
	}
	val, ok := result.(int64)
	if !ok {
		return 0, fmt.Errorf("occupancystore: unexpected get result type: %T", result)
	}
	return val, nil
}
