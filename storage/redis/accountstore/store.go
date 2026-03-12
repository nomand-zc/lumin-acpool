package accountstore

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
var _ storage.AccountStorage = (*Store)(nil)

// Store 是基于 Redis 的 AccountStorage 实现。
//
// 数据结构设计：
//   - account:{id}                                                → Hash，存储账号全部字段
//   - accounts:index                                              → Set，全局索引
//   - accounts:type:{provider_type}                               → Set，层级 1 索引
//   - accounts:provider:{provider_type}/{provider_name}           → Set，层级 2 索引
//   - accounts:group:{provider_type}/{provider_name}/{status}     → Set，层级 3 组合索引
//
// 查询优化：
//   Search/Count 时先从 Filter 中提取 provider_type、provider_name、status 等值条件，
//   按三层优先级自动选取最精确的索引缩小候选集，再对残余条件做内存过滤。
//
// 乐观锁：使用 Lua 脚本实现 CAS（compare-and-swap），version 字段用于并发控制。
type Store struct {
	client    storeRedis.Client
	keyPrefix string
	evaluator *storeRedis.FilterEvaluator
}

// NewStore 创建一个新的 Redis 账号存储实例。
func NewStore(opts ...Option) (*Store, error) {
	o := DefaultOptions()
	for _, opt := range opts {
		opt(o)
	}

	client, err := buildClient(o)
	if err != nil {
		return nil, fmt.Errorf("accountstore: %w", err)
	}

	store := &Store{
		client:    client,
		keyPrefix: o.KeyPrefix,
		evaluator: storeRedis.NewFilterEvaluator(accountFieldExtractor),
	}

	return store, nil
}

// ============================
// 读取
// ============================

func (s *Store) Get(ctx context.Context, id string) (*account.Account, error) {
	key := accountKey(s.keyPrefix, id)
	data, err := s.client.HGetAll(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("accountstore: failed to get account: %w", err)
	}
	if len(data) == 0 {
		return nil, storage.ErrNotFound
	}

	acct, err := unmarshalAccountFromHash(data)
	if err != nil {
		return nil, fmt.Errorf("accountstore: failed to unmarshal account: %w", err)
	}
	return acct, nil
}

// ============================
// 查询（利用三层索引下推）
// ============================

// resolveIDs 根据 SearchFilter 利用分级索引获取候选 ID 列表，
// 同时返回索引无法覆盖的残余 Filter（需要内存过滤）。
// 如果无法利用任何索引，回退到全局索引。
func (s *Store) resolveIDs(ctx context.Context, filter *storage.SearchFilter) (ids []string, residual *filtercond.Filter, err error) {
	// 从 SearchFilter 一级字段直接构建索引条件
	ic := extractIndexFromSearchFilter(filter)
	if ic != nil {
		keys, allPushed := ic.resolveIndexKeys(s.keyPrefix)
		if len(keys) > 0 {
			// 收集所有匹配 key 的成员（多个 key 取并集）
			if len(keys) == 1 {
				ids, err = s.client.SMembers(ctx, keys[0])
				if err != nil {
					return nil, nil, fmt.Errorf("accountstore: failed to get index members: %w", err)
				}
			} else {
				idSet := make(map[string]struct{})
				for _, k := range keys {
					members, err2 := s.client.SMembers(ctx, k)
					if err2 != nil {
						return nil, nil, fmt.Errorf("accountstore: failed to get index members: %w", err2)
					}
					for _, m := range members {
						idSet[m] = struct{}{}
					}
				}
				ids = make([]string, 0, len(idSet))
				for id := range idSet {
					ids = append(ids, id)
				}
			}

			// 确定残余 Filter：仅 ExtraCond 需内存过滤
			if allPushed {
				var extra *filtercond.Filter
				if filter != nil {
					extra = filter.ExtraCond
				}
				return ids, extra, nil
			}
			// 部分索引条件未被覆盖，需回退到 ExtraCond 做内存过滤
			var extra *filtercond.Filter
			if filter != nil {
				extra = filter.ExtraCond
			}
			return ids, extra, nil
		}
	}

	// 回退：全局索引
	ids, err = s.client.SMembers(ctx, accountIndexKey(s.keyPrefix))
	if err != nil {
		return nil, nil, fmt.Errorf("accountstore: failed to get account ids: %w", err)
	}
	var extra *filtercond.Filter
	if filter != nil {
		extra = filter.ExtraCond
	}
	return ids, extra, nil
}

// fetchAndFilter 通过 Pipeline 批量获取 IDs 对应的账号，并使用残余 Filter 做内存过滤。
func (s *Store) fetchAndFilter(ctx context.Context, ids []string, residual *filtercond.Filter) ([]*account.Account, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	// 使用 Pipeline 批量获取
	pipe := s.client.Pipeline(ctx)
	cmds := make(map[string]*pipelineCmd, len(ids))
	for _, id := range ids {
		key := accountKey(s.keyPrefix, id)
		cmd := pipe.HGetAll(ctx, key)
		cmds[id] = &pipelineCmd{cmd: cmd}
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, fmt.Errorf("accountstore: failed to pipeline get accounts: %w", err)
	}

	// 反序列化并过滤
	var result []*account.Account
	for _, id := range ids {
		pc := cmds[id]
		data, err := pc.cmd.Result()
		if err != nil || len(data) == 0 {
			continue
		}

		acct, err := unmarshalAccountFromHash(data)
		if err != nil {
			continue
		}

		if s.evaluator.Match(acct, residual) {
			result = append(result, acct)
		}
	}

	return result, nil
}

func (s *Store) Search(ctx context.Context, filter *storage.SearchFilter) ([]*account.Account, error) {
	ids, residual, err := s.resolveIDs(ctx, filter)
	if err != nil {
		return nil, err
	}
	return s.fetchAndFilter(ctx, ids, residual)
}

// ============================
// 写入（同步维护三层索引）
// ============================

// luaAddAccount 使用 Lua 脚本原子执行：写入 Hash + 添加全部索引。
// KEYS: [1]=accountKey, [2]=globalIdx, [3]=typeIdx, [4]=providerIdx, [5]=groupIdx
// ARGV: [1]=id, [2..]=field/value pairs
var luaAddAccount = `
	local key = KEYS[1]
	local id = ARGV[1]
	
	-- 检查是否已存在
	if redis.call("EXISTS", key) == 1 then
		return 0
	end
	
	-- 写入 Hash 字段
	for i = 2, #ARGV, 2 do
		redis.call("HSET", key, ARGV[i], ARGV[i+1])
	end
	
	-- 添加全部索引
	redis.call("SADD", KEYS[2], id)
	redis.call("SADD", KEYS[3], id)
	redis.call("SADD", KEYS[4], id)
	redis.call("SADD", KEYS[5], id)
	
	return 1
`

func (s *Store) Add(ctx context.Context, acct *account.Account) error {
	key := accountKey(s.keyPrefix, acct.ID)
	ik := allIndexKeys(s.keyPrefix, acct.ProviderType, acct.ProviderName, int(acct.Status))

	now := time.Now()
	if acct.CreatedAt.IsZero() {
		acct.CreatedAt = now
	}
	acct.UpdatedAt = now
	acct.Version = 1

	fields, err := marshalAccountToHash(acct)
	if err != nil {
		return fmt.Errorf("accountstore: %w", err)
	}

	args := []any{acct.ID}
	for k, v := range fields {
		args = append(args, k, fmt.Sprintf("%v", v))
	}

	result, err := s.client.Eval(ctx, luaAddAccount,
		[]string{key, ik.global, ik.typeIdx, ik.provider, ik.group}, args...)
	if err != nil {
		return fmt.Errorf("accountstore: failed to add account: %w", err)
	}

	if result.(int64) == 0 {
		return storage.ErrAlreadyExists
	}

	return nil
}

// luaUpdateAccount 使用 Lua 脚本实现乐观锁更新 + 索引迁移。
//
// 当 provider_type、provider_name 或 status 任一发生变化时，需要：
//  1. 从旧的 type/provider/group 索引中移除
//  2. 添加到新的 type/provider/group 索引中
//
// KEYS: [1]=accountKey
//
//	[2]=oldTypeIdx, [3]=oldProviderIdx, [4]=oldGroupIdx
//	[5]=newTypeIdx, [6]=newProviderIdx, [7]=newGroupIdx
//
// ARGV: [1]=expectedVersion, [2..]=field/value pairs
var luaUpdateAccount = `
	local key = KEYS[1]
	local expectedVersion = ARGV[1]
	
	-- 检查 key 是否存在
	if redis.call("EXISTS", key) == 0 then
		return -1
	end
	
	-- 检查版本号
	local currentVersion = redis.call("HGET", key, "version")
	if currentVersion ~= expectedVersion then
		return -2
	end
	
	local id = redis.call("HGET", key, "id")
	
	-- 更新字段
	for i = 2, #ARGV, 2 do
		redis.call("HSET", key, ARGV[i], ARGV[i+1])
	end
	
	-- 递增版本号
	redis.call("HINCRBY", key, "version", 1)
	
	-- 迁移索引（旧 key 和新 key 相同时跳过，避免不必要的写入）
	-- 层级 1: type 索引
	if KEYS[2] ~= KEYS[5] then
		redis.call("SREM", KEYS[2], id)
		redis.call("SADD", KEYS[5], id)
	end
	-- 层级 2: provider 索引
	if KEYS[3] ~= KEYS[6] then
		redis.call("SREM", KEYS[3], id)
		redis.call("SADD", KEYS[6], id)
	end
	-- 层级 3: group 索引
	if KEYS[4] ~= KEYS[7] then
		redis.call("SREM", KEYS[4], id)
		redis.call("SADD", KEYS[7], id)
	end
	
	return 1
`

func (s *Store) Update(ctx context.Context, acct *account.Account) error {
	key := accountKey(s.keyPrefix, acct.ID)

	// 获取旧的 provider_type, provider_name, status 用于索引迁移
	oldData, err := s.client.HGetAll(ctx, key)
	if err != nil {
		return fmt.Errorf("accountstore: failed to get old account data: %w", err)
	}
	if len(oldData) == 0 {
		return storage.ErrNotFound
	}

	oldType := oldData[fieldProviderType]
	oldName := oldData[fieldProviderName]
	oldStatus := storeRedis.ParseInt(oldData[fieldStatus])

	oldIK := allIndexKeys(s.keyPrefix, oldType, oldName, oldStatus)
	newIK := allIndexKeys(s.keyPrefix, acct.ProviderType, acct.ProviderName, int(acct.Status))

	acct.UpdatedAt = time.Now()
	fields, err := marshalAccountToHash(acct)
	if err != nil {
		return fmt.Errorf("accountstore: %w", err)
	}

	// 去掉 version 字段，由 Lua 脚本递增
	delete(fields, fieldVersion)

	args := []any{formatInt(acct.Version)}
	for k, v := range fields {
		args = append(args, k, fmt.Sprintf("%v", v))
	}

	result, err := s.client.Eval(ctx, luaUpdateAccount,
		[]string{
			key,
			oldIK.typeIdx, oldIK.provider, oldIK.group,
			newIK.typeIdx, newIK.provider, newIK.group,
		}, args...)
	if err != nil {
		return fmt.Errorf("accountstore: failed to update account: %w", err)
	}

	switch result.(int64) {
	case -1:
		return storage.ErrNotFound
	case -2:
		return storage.ErrVersionConflict
	case 1:
		return nil
	default:
		return fmt.Errorf("accountstore: unexpected update result: %v", result)
	}
}

// luaRemoveAccount 使用 Lua 脚本原子删除账号及其所有索引。
// KEYS: [1]=accountKey, [2]=globalIdx, [3]=typeIdx, [4]=providerIdx, [5]=groupIdx
// ARGV: [1]=id
var luaRemoveAccount = `
	redis.call("DEL", KEYS[1])
	redis.call("SREM", KEYS[2], ARGV[1])
	redis.call("SREM", KEYS[3], ARGV[1])
	redis.call("SREM", KEYS[4], ARGV[1])
	redis.call("SREM", KEYS[5], ARGV[1])
	return 1
`

func (s *Store) Remove(ctx context.Context, id string) error {
	key := accountKey(s.keyPrefix, id)

	// 先获取账号信息，以便删除所有层级的索引
	data, err := s.client.HGetAll(ctx, key)
	if err != nil {
		return fmt.Errorf("accountstore: failed to get account for removal: %w", err)
	}
	if len(data) == 0 {
		return storage.ErrNotFound
	}

	ik := allIndexKeys(s.keyPrefix,
		data[fieldProviderType], data[fieldProviderName], storeRedis.ParseInt(data[fieldStatus]))

	_, err = s.client.Eval(ctx, luaRemoveAccount,
		[]string{key, ik.global, ik.typeIdx, ik.provider, ik.group}, id)
	if err != nil {
		return fmt.Errorf("accountstore: failed to remove account: %w", err)
	}
	return nil
}

func (s *Store) RemoveFilter(ctx context.Context, filter *storage.SearchFilter) error {
	accounts, err := s.Search(ctx, filter)
	if err != nil {
		return fmt.Errorf("accountstore: failed to search accounts for removal: %w", err)
	}

	for _, acct := range accounts {
		if err := s.Remove(ctx, acct.ID); err != nil && err != storage.ErrNotFound {
			return fmt.Errorf("accountstore: failed to remove account %s: %w", acct.ID, err)
		}
	}
	return nil
}

// ============================
// 计数（优先使用 SCARD O(1)）
// ============================

func (s *Store) Count(ctx context.Context, filter *storage.SearchFilter) (int, error) {
	if filter == nil {
		// 无过滤条件，SCARD 全局索引 O(1)
		n, err := s.client.SCard(ctx, accountIndexKey(s.keyPrefix))
		if err != nil {
			return 0, fmt.Errorf("accountstore: failed to count accounts: %w", err)
		}
		return int(n), nil
	}

	// 尝试索引下推：如果所有一级字段条件都被索引覆盖且无 ExtraCond，直接 SCARD
	ic := extractIndexFromSearchFilter(filter)
	if ic != nil && filter.ExtraCond == nil {
		keys, allPushed := ic.resolveIndexKeys(s.keyPrefix)
		if len(keys) > 0 && allPushed {
			total := int64(0)
			for _, k := range keys {
				n, err := s.client.SCard(ctx, k)
				if err != nil {
					return 0, fmt.Errorf("accountstore: failed to scard: %w", err)
				}
				total += n
			}
			return int(total), nil
		}
	}

	// 回退：索引缩小范围 + 内存过滤
	accounts, err := s.Search(ctx, filter)
	if err != nil {
		return 0, err
	}
	return len(accounts), nil
}

// pipelineCmd 包装 Pipeline 命令结果。
type pipelineCmd struct {
	cmd interface {
		Result() (map[string]string, error)
	}
}
