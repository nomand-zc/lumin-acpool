package statsstore

import (
	"context"
	"fmt"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
	storeRedis "github.com/nomand-zc/lumin-acpool/storage/redis"
)

// Compile-time interface compliance check.
var _ storage.StatsStore = (*Store)(nil)

// Store 是基于 Redis 的 StatsStore 实现。
//
// 数据结构设计：
//   - stats:{account_id} → Hash，存储统计数据的各个字段
//
// 使用 Redis Hash 的 HINCRBY 命令实现高频原子递增，
// 使用 Lua 脚本保证复合操作（如 IncrSuccess 需要同时修改多个字段）的原子性。
type Store struct {
	client    storeRedis.Client
	keyPrefix string
}

// NewStore 创建一个新的 Redis 统计存储实例。
func NewStore(opts ...Option) (*Store, error) {
	o := DefaultOptions()
	for _, opt := range opts {
		opt(o)
	}

	client, err := buildClient(o)
	if err != nil {
		return nil, fmt.Errorf("statsstore: %w", err)
	}

	return &Store{
		client:    client,
		keyPrefix: o.KeyPrefix,
	}, nil
}

func (s *Store) GetStats(ctx context.Context, accountID string) (*account.AccountStats, error) {
	key := statsKey(s.keyPrefix, accountID)
	data, err := s.client.HGetAll(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("statsstore: failed to get stats: %w", err)
	}

	if len(data) == 0 {
		// 不存在统计记录，返回零值
		return &account.AccountStats{AccountID: accountID}, nil
	}

	stats := &account.AccountStats{
		AccountID:           data[fieldAccountID],
		TotalCalls:          storeRedis.ParseInt64(data[fieldTotalCalls]),
		SuccessCalls:        storeRedis.ParseInt64(data[fieldSuccessCalls]),
		FailedCalls:         storeRedis.ParseInt64(data[fieldFailedCalls]),
		ConsecutiveFailures: storeRedis.ParseInt(data[fieldConsecutiveFailures]),
		LastUsedAt:          storeRedis.ParseTimePtr(data[fieldLastUsedAt]),
		LastErrorAt:         storeRedis.ParseTimePtr(data[fieldLastErrorAt]),
		LastErrorMsg:        data[fieldLastErrorMsg],
	}

	if stats.AccountID == "" {
		stats.AccountID = accountID
	}

	return stats, nil
}

func (s *Store) IncrSuccess(ctx context.Context, accountID string) error {
	key := statsKey(s.keyPrefix, accountID)
	now := storeRedis.FormatTime(time.Now())

	_, err := s.client.Eval(ctx, luaIncrSuccess, []string{key}, now, accountID)
	if err != nil {
		return fmt.Errorf("statsstore: failed to incr success: %w", err)
	}
	return nil
}

func (s *Store) IncrFailure(ctx context.Context, accountID string, errMsg string) (int, error) {
	key := statsKey(s.keyPrefix, accountID)
	now := storeRedis.FormatTime(time.Now())

	result, err := s.client.Eval(ctx, luaIncrFailure, []string{key}, now, errMsg, accountID)
	if err != nil {
		return 0, fmt.Errorf("statsstore: failed to incr failure: %w", err)
	}

	failures, ok := result.(int64)
	if !ok {
		return 0, fmt.Errorf("statsstore: unexpected result type: %T", result)
	}
	return int(failures), nil
}

func (s *Store) UpdateLastUsed(ctx context.Context, accountID string, t time.Time) error {
	key := statsKey(s.keyPrefix, accountID)
	timeStr := storeRedis.FormatTime(t)

	err := s.client.HSet(ctx, key, fieldAccountID, accountID, fieldLastUsedAt, timeStr)
	if err != nil {
		return fmt.Errorf("statsstore: failed to update last used: %w", err)
	}
	return nil
}

func (s *Store) GetConsecutiveFailures(ctx context.Context, accountID string) (int, error) {
	key := statsKey(s.keyPrefix, accountID)
	val, err := s.client.HGet(ctx, key, fieldConsecutiveFailures)
	if err != nil {
		if storeRedis.IsNotFound(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("statsstore: failed to get consecutive failures: %w", err)
	}
	return storeRedis.ParseInt(val), nil
}

func (s *Store) ResetConsecutiveFailures(ctx context.Context, accountID string) error {
	key := statsKey(s.keyPrefix, accountID)
	err := s.client.HSet(ctx, key, fieldConsecutiveFailures, "0")
	if err != nil {
		return fmt.Errorf("statsstore: failed to reset consecutive failures: %w", err)
	}
	return nil
}

func (s *Store) RemoveStats(ctx context.Context, accountID string) error {
	key := statsKey(s.keyPrefix, accountID)
	err := s.client.Del(ctx, key)
	if err != nil {
		return fmt.Errorf("statsstore: failed to remove stats: %w", err)
	}
	return nil
}
