package usagestore

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
	storeRedis "github.com/nomand-zc/lumin-acpool/storage/redis"
)

// Compile-time interface compliance check.
var _ storage.UsageStore = (*Store)(nil)

// Store 是基于 Redis 的 UsageStore 实现。
//
// 数据结构设计：
//   - usage:{account_id}:count   → String，存储规则数量
//   - usage:{account_id}:{index} → Hash，存储单条规则的追踪数据
//
// 每条规则使用独立的 Hash key，这样 IncrLocalUsed 可以直接使用 HINCRBYFLOAT
// 对 local_used 字段进行原子递增，无需锁定整个账号的用量数据。
type Store struct {
	client    storeRedis.Client
	keyPrefix string
}

// NewStore 创建一个新的 Redis 用量存储实例。
func NewStore(opts ...Option) (*Store, error) {
	o := DefaultOptions()
	for _, opt := range opts {
		opt(o)
	}

	client, err := buildClient(o)
	if err != nil {
		return nil, fmt.Errorf("usagestore: %w", err)
	}

	return &Store{
		client:    client,
		keyPrefix: o.KeyPrefix,
	}, nil
}

func (s *Store) GetAllUsages(ctx context.Context, accountID string) ([]*account.TrackedUsage, error) {
	// 获取规则数量
	countKey := usageCountKey(s.keyPrefix, accountID)
	countStr, err := s.client.Get(ctx, countKey)
	if err != nil {
		if storeRedis.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("usagestore: failed to get usage count: %w", err)
	}

	count, err := strconv.Atoi(countStr)
	if err != nil || count <= 0 {
		return nil, nil
	}

	// 逐条获取
	var result []*account.TrackedUsage
	for i := range count {
		key := usageRuleKey(s.keyPrefix, accountID, i)
		data, err := s.client.HGetAll(ctx, key)
		if err != nil {
			return nil, fmt.Errorf("usagestore: failed to get usage at index %d: %w", i, err)
		}
		if len(data) == 0 {
			continue
		}

		usage, err := unmarshalUsageFromHash(data)
		if err != nil {
			return nil, fmt.Errorf("usagestore: failed to unmarshal usage at index %d: %w", i, err)
		}
		result = append(result, usage)
	}

	return result, nil
}

func (s *Store) SaveUsages(ctx context.Context, accountID string, usages []*account.TrackedUsage) error {
	// 使用 Lua 脚本原子执行：删除旧数据 + 写入新数据
	// 构建 Lua 脚本参数
	countKey := usageCountKey(s.keyPrefix, accountID)

	// 先获取旧的规则数量用于清理
	oldCountStr, _ := s.client.Get(ctx, countKey)
	oldCount, _ := strconv.Atoi(oldCountStr)

	// 删除旧的规则 key
	for i := range oldCount {
		key := usageRuleKey(s.keyPrefix, accountID, i)
		if err := s.client.Del(ctx, key); err != nil {
			return fmt.Errorf("usagestore: failed to delete old usage at index %d: %w", i, err)
		}
	}

	// 写入新数据
	for i, u := range usages {
		key := usageRuleKey(s.keyPrefix, accountID, i)
		fields := marshalUsageToHash(u)

		args := make([]any, 0, len(fields)*2)
		for k, v := range fields {
			args = append(args, k, v)
		}

		if err := s.client.HSet(ctx, key, args...); err != nil {
			return fmt.Errorf("usagestore: failed to save usage at index %d: %w", i, err)
		}
	}

	// 更新规则数量
	if err := s.client.Set(ctx, countKey, strconv.Itoa(len(usages)), 0); err != nil {
		return fmt.Errorf("usagestore: failed to update usage count: %w", err)
	}

	return nil
}

func (s *Store) IncrLocalUsed(ctx context.Context, accountID string, ruleIndex int, amount float64) error {
	key := usageRuleKey(s.keyPrefix, accountID, ruleIndex)

	// 检查 key 是否存在
	exists, err := s.client.Exists(ctx, key)
	if err != nil {
		return fmt.Errorf("usagestore: failed to check usage existence: %w", err)
	}
	if exists == 0 {
		// 未初始化，静默忽略（与内存实现保持一致）
		return nil
	}

	_, err = s.client.HIncrByFloat(ctx, key, fieldLocalUsed, amount)
	if err != nil {
		return fmt.Errorf("usagestore: failed to incr local used: %w", err)
	}
	return nil
}

func (s *Store) RemoveUsages(ctx context.Context, accountID string) error {
	countKey := usageCountKey(s.keyPrefix, accountID)

	// 获取规则数量
	countStr, _ := s.client.Get(ctx, countKey)
	count, _ := strconv.Atoi(countStr)

	// 删除所有规则 key
	keys := []string{countKey}
	for i := range count {
		keys = append(keys, usageRuleKey(s.keyPrefix, accountID, i))
	}

	if err := s.client.Del(ctx, keys...); err != nil {
		return fmt.Errorf("usagestore: failed to remove usages: %w", err)
	}
	return nil
}

func (s *Store) CalibrateRule(ctx context.Context, accountID string, ruleIndex int, usage *account.TrackedUsage) error {
	key := usageRuleKey(s.keyPrefix, accountID, ruleIndex)

	// 使用 Lua 脚本原子校准：设置远端数据，重置本地计数
	script := `
		local key = KEYS[1]
		if redis.call("EXISTS", key) == 0 then
			return 0
		end
		
		redis.call("HSET", key, "remote_used", ARGV[1])
		redis.call("HSET", key, "remote_remain", ARGV[2])
		redis.call("HSET", key, "local_used", "0")
		redis.call("HSET", key, "window_start", ARGV[3])
		redis.call("HSET", key, "window_end", ARGV[4])
		redis.call("HSET", key, "last_sync_at", ARGV[5])
		
		return 1
	`

	_, err := s.client.Eval(ctx, script, []string{key},
		fmt.Sprintf("%.6f", usage.RemoteUsed),
		fmt.Sprintf("%.6f", usage.RemoteRemain),
		storeRedis.FormatTimePtr(usage.WindowStart),
		storeRedis.FormatTimePtr(usage.WindowEnd),
		storeRedis.FormatTime(time.Now()),
	)
	if err != nil {
		return fmt.Errorf("usagestore: failed to calibrate rule: %w", err)
	}
	return nil
}
