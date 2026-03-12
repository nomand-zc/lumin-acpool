package providerstore

import (
	"context"
	"fmt"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-acpool/storage/filtercond"
	storeRedis "github.com/nomand-zc/lumin-acpool/storage/redis"
)

// Compile-time interface compliance check.
var _ storage.ProviderStorage = (*Store)(nil)

// Store 是基于 Redis 的 ProviderStorage 实现。
//
// 数据结构设计：
//   - provider:{type}/{name}  → Hash，存储供应商全部字段
//   - providers:index         → Set，存储所有供应商 key（"type/name" 格式）
type Store struct {
	client    storeRedis.Client
	keyPrefix string
	evaluator *storeRedis.FilterEvaluator
}

// NewStore 创建一个新的 Redis 供应商存储实例。
func NewStore(opts ...Option) (*Store, error) {
	o := DefaultOptions()
	for _, opt := range opts {
		opt(o)
	}

	client, err := buildClient(o)
	if err != nil {
		return nil, fmt.Errorf("providerstore: %w", err)
	}

	store := &Store{
		client:    client,
		keyPrefix: o.KeyPrefix,
		evaluator: storeRedis.NewFilterEvaluator(providerFieldExtractor),
	}

	return store, nil
}

func (s *Store) Get(ctx context.Context, key account.ProviderKey) (*account.ProviderInfo, error) {
	redisKey := providerRedisKey(s.keyPrefix, key.Type, key.Name)
	data, err := s.client.HGetAll(ctx, redisKey)
	if err != nil {
		return nil, fmt.Errorf("providerstore: failed to get provider: %w", err)
	}
	if len(data) == 0 {
		return nil, storage.ErrNotFound
	}

	info, err := unmarshalProviderFromHash(data)
	if err != nil {
		return nil, fmt.Errorf("providerstore: failed to unmarshal provider: %w", err)
	}
	return info, nil
}

func (s *Store) Search(ctx context.Context, filter *storage.SearchFilter) ([]*account.ProviderInfo, error) {
	// 获取所有供应商索引
	members, err := s.client.SMembers(ctx, providerIndexRedisKey(s.keyPrefix))
	if err != nil {
		return nil, fmt.Errorf("providerstore: failed to get provider ids: %w", err)
	}

	if len(members) == 0 {
		return nil, nil
	}

	// 使用 Pipeline 批量获取
	pipe := s.client.Pipeline(ctx)
	cmds := make(map[string]*pipelineCmd, len(members))
	for _, member := range members {
		key := s.keyPrefix + keyProviderPrefix + member
		cmd := pipe.HGetAll(ctx, key)
		cmds[member] = &pipelineCmd{cmd: cmd}
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, fmt.Errorf("providerstore: failed to pipeline get providers: %w", err)
	}

	var extraCond *filtercond.Filter
	if filter != nil {
		extraCond = filter.ExtraCond
	}

	var result []*account.ProviderInfo
	for _, member := range members {
		pc := cmds[member]
		data, err := pc.cmd.Result()
		if err != nil || len(data) == 0 {
			continue
		}

		info, err := unmarshalProviderFromHash(data)
		if err != nil {
			continue
		}

		// 先按一级字段过滤
		if !matchProviderSearchFilter(info, filter) {
			continue
		}

		// 再按 ExtraCond 过滤
		if s.evaluator.Match(info, extraCond) {
			result = append(result, info)
		}
	}

	return result, nil
}

// matchProviderSearchFilter 检查供应商是否匹配 SearchFilter 的一级字段条件。
func matchProviderSearchFilter(info *account.ProviderInfo, filter *storage.SearchFilter) bool {
	if filter == nil {
		return true
	}
	if filter.ProviderType != "" && info.ProviderType != filter.ProviderType {
		return false
	}
	if filter.ProviderName != "" && info.ProviderName != filter.ProviderName {
		return false
	}
	if filter.Status != 0 && int(info.Status) != filter.Status {
		return false
	}
	return true
}

func (s *Store) Add(ctx context.Context, info *account.ProviderInfo) error {
	redisKey := providerRedisKey(s.keyPrefix, info.ProviderType, info.ProviderName)
	indexKey := providerIndexRedisKey(s.keyPrefix)
	member := providerIndexMember(info.ProviderType, info.ProviderName)

	// 检查是否已存在
	exists, err := s.client.Exists(ctx, redisKey)
	if err != nil {
		return fmt.Errorf("providerstore: failed to check provider existence: %w", err)
	}
	if exists > 0 {
		return storage.ErrAlreadyExists
	}

	now := time.Now()
	if info.CreatedAt.IsZero() {
		info.CreatedAt = now
	}
	info.UpdatedAt = now

	fields, err := marshalProviderToHash(info)
	if err != nil {
		return fmt.Errorf("providerstore: %w", err)
	}

	// 使用 Lua 脚本原子执行
	script := `
		local key = KEYS[1]
		local indexKey = KEYS[2]
		local member = ARGV[1]
		
		if redis.call("EXISTS", key) == 1 then
			return 0
		end
		
		for i = 2, #ARGV, 2 do
			redis.call("HSET", key, ARGV[i], ARGV[i+1])
		end
		
		redis.call("SADD", indexKey, member)
		return 1
	`

	args := []any{member}
	for k, v := range fields {
		args = append(args, k, fmt.Sprintf("%v", v))
	}

	result, err := s.client.Eval(ctx, script, []string{redisKey, indexKey}, args...)
	if err != nil {
		return fmt.Errorf("providerstore: failed to add provider: %w", err)
	}
	if result.(int64) == 0 {
		return storage.ErrAlreadyExists
	}
	return nil
}

func (s *Store) Update(ctx context.Context, info *account.ProviderInfo) error {
	redisKey := providerRedisKey(s.keyPrefix, info.ProviderType, info.ProviderName)

	// 检查是否存在
	exists, err := s.client.Exists(ctx, redisKey)
	if err != nil {
		return fmt.Errorf("providerstore: failed to check provider existence: %w", err)
	}
	if exists == 0 {
		return storage.ErrNotFound
	}

	info.UpdatedAt = time.Now()

	fields, err := marshalProviderToHash(info)
	if err != nil {
		return fmt.Errorf("providerstore: %w", err)
	}

	// 写入 Hash 字段
	args := make([]any, 0, len(fields)*2)
	for k, v := range fields {
		args = append(args, k, v)
	}

	err = s.client.HSet(ctx, redisKey, args...)
	if err != nil {
		return fmt.Errorf("providerstore: failed to update provider: %w", err)
	}
	return nil
}

func (s *Store) Remove(ctx context.Context, key account.ProviderKey) error {
	redisKey := providerRedisKey(s.keyPrefix, key.Type, key.Name)
	indexKey := providerIndexRedisKey(s.keyPrefix)
	member := providerIndexMember(key.Type, key.Name)

	// 检查是否存在
	exists, err := s.client.Exists(ctx, redisKey)
	if err != nil {
		return fmt.Errorf("providerstore: failed to check provider existence: %w", err)
	}
	if exists == 0 {
		return storage.ErrNotFound
	}

	// 使用 Lua 脚本原子执行
	script := `
		redis.call("DEL", KEYS[1])
		redis.call("SREM", KEYS[2], ARGV[1])
		return 1
	`
	_, err = s.client.Eval(ctx, script, []string{redisKey, indexKey}, member)
	if err != nil {
		return fmt.Errorf("providerstore: failed to remove provider: %w", err)
	}
	return nil
}

// pipelineCmd 包装 Pipeline 命令结果。
type pipelineCmd struct {
	cmd interface {
		Result() (map[string]string, error)
	}
}
