package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-acpool/storage/filtercond"
)

// ============================
// Account 读取
// ============================

func (s *Store) GetAccount(ctx context.Context, id string) (*account.Account, error) {
	key := acctDataKey(s.keyPrefix, id)
	data, err := s.client.HGetAll(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("redis store: failed to get account: %w", err)
	}
	if len(data) == 0 {
		return nil, storage.ErrNotFound
	}

	acct, err := unmarshalAccountFromHash(data)
	if err != nil {
		return nil, fmt.Errorf("redis store: failed to unmarshal account: %w", err)
	}
	return acct, nil
}

// ============================
// Account 查询（利用三层索引下推）
// ============================

func (s *Store) resolveAccountIDs(ctx context.Context, filter *storage.SearchFilter) (ids []string, residual *filtercond.Filter, err error) {
	ic := extractIndexFromSearchFilter(filter)
	if ic != nil {
		keys, allPushed := ic.resolveIndexKeys(s.keyPrefix)
		if len(keys) > 0 {
			if len(keys) == 1 {
				ids, err = s.client.SMembers(ctx, keys[0])
				if err != nil {
					return nil, nil, fmt.Errorf("redis store: failed to get index members: %w", err)
				}
			} else {
				idSet := make(map[string]struct{})
				for _, k := range keys {
					members, err2 := s.client.SMembers(ctx, k)
					if err2 != nil {
						return nil, nil, fmt.Errorf("redis store: failed to get index members: %w", err2)
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

			if allPushed {
				var extra *filtercond.Filter
				if filter != nil {
					extra = filter.ExtraCond
				}
				return ids, extra, nil
			}
			var extra *filtercond.Filter
			if filter != nil {
				extra = filter.ExtraCond
			}
			return ids, extra, nil
		}
	}

	ids, err = s.client.SMembers(ctx, acctGlobalIndexKey(s.keyPrefix))
	if err != nil {
		return nil, nil, fmt.Errorf("redis store: failed to get account ids: %w", err)
	}
	var extra *filtercond.Filter
	if filter != nil {
		extra = filter.ExtraCond
	}
	return ids, extra, nil
}

func (s *Store) fetchAndFilterAccounts(ctx context.Context, ids []string, residual *filtercond.Filter) ([]*account.Account, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	pipe := s.client.Pipeline(ctx)
	cmds := make(map[string]*pipelineCmd, len(ids))
	for _, id := range ids {
		key := acctDataKey(s.keyPrefix, id)
		cmd := pipe.HGetAll(ctx, key)
		cmds[id] = &pipelineCmd{cmd: cmd}
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, fmt.Errorf("redis store: failed to pipeline get accounts: %w", err)
	}

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

		if s.accountEvaluator.Match(acct, residual) {
			result = append(result, acct)
		}
	}

	return result, nil
}

func (s *Store) SearchAccounts(ctx context.Context, filter *storage.SearchFilter) ([]*account.Account, error) {
	ids, residual, err := s.resolveAccountIDs(ctx, filter)
	if err != nil {
		return nil, err
	}
	return s.fetchAndFilterAccounts(ctx, ids, residual)
}

// ============================
// Account 写入（同步维护三层索引）
// ============================

var luaAddAccount = `
	local key = KEYS[1]
	local id = ARGV[1]
	
	if redis.call("EXISTS", key) == 1 then
		return 0
	end
	
	for i = 2, #ARGV, 2 do
		redis.call("HSET", key, ARGV[i], ARGV[i+1])
	end
	
	redis.call("SADD", KEYS[2], id)
	redis.call("SADD", KEYS[3], id)
	redis.call("SADD", KEYS[4], id)
	redis.call("SADD", KEYS[5], id)
	
	return 1
`

func (s *Store) AddAccount(ctx context.Context, acct *account.Account) error {
	key := acctDataKey(s.keyPrefix, acct.ID)
	ik := acctAllIndexKeys(s.keyPrefix, acct.ProviderType, acct.ProviderName, int(acct.Status))

	now := time.Now()
	if acct.CreatedAt.IsZero() {
		acct.CreatedAt = now
	}
	acct.UpdatedAt = now
	acct.Version = 1

	fields, err := marshalAccountToHash(acct)
	if err != nil {
		return fmt.Errorf("redis store: %w", err)
	}

	args := []any{acct.ID}
	for k, v := range fields {
		args = append(args, k, fmt.Sprintf("%v", v))
	}

	result, err := s.client.Eval(ctx, luaAddAccount,
		[]string{key, ik.global, ik.typeIdx, ik.provider, ik.group}, args...)
	if err != nil {
		return fmt.Errorf("redis store: failed to add account: %w", err)
	}

	if result.(int64) == 0 {
		return storage.ErrAlreadyExists
	}

	return nil
}

var luaUpdateAccount = `
	local key = KEYS[1]
	local expectedVersion = ARGV[1]
	
	if redis.call("EXISTS", key) == 0 then
		return -1
	end
	
	local currentVersion = redis.call("HGET", key, "version")
	if currentVersion ~= expectedVersion then
		return -2
	end
	
	local id = redis.call("HGET", key, "id")
	
	for i = 2, #ARGV, 2 do
		redis.call("HSET", key, ARGV[i], ARGV[i+1])
	end
	
	redis.call("HINCRBY", key, "version", 1)
	
	if KEYS[2] ~= KEYS[5] then
		redis.call("SREM", KEYS[2], id)
		redis.call("SADD", KEYS[5], id)
	end
	if KEYS[3] ~= KEYS[6] then
		redis.call("SREM", KEYS[3], id)
		redis.call("SADD", KEYS[6], id)
	end
	if KEYS[4] ~= KEYS[7] then
		redis.call("SREM", KEYS[4], id)
		redis.call("SADD", KEYS[7], id)
	end
	
	return 1
`

func (s *Store) UpdateAccount(ctx context.Context, acct *account.Account) error {
	key := acctDataKey(s.keyPrefix, acct.ID)

	oldData, err := s.client.HGetAll(ctx, key)
	if err != nil {
		return fmt.Errorf("redis store: failed to get old account data: %w", err)
	}
	if len(oldData) == 0 {
		return storage.ErrNotFound
	}

	oldType := oldData[acctFieldProviderType]
	oldName := oldData[acctFieldProviderName]
	oldStatus := ParseInt(oldData[acctFieldStatus])

	oldIK := acctAllIndexKeys(s.keyPrefix, oldType, oldName, oldStatus)
	newIK := acctAllIndexKeys(s.keyPrefix, acct.ProviderType, acct.ProviderName, int(acct.Status))

	acct.UpdatedAt = time.Now()
	fields, err := marshalAccountToHash(acct)
	if err != nil {
		return fmt.Errorf("redis store: %w", err)
	}

	delete(fields, acctFieldVersion)

	args := []any{acctFormatInt(acct.Version)}
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
		return fmt.Errorf("redis store: failed to update account: %w", err)
	}

	switch result.(int64) {
	case -1:
		return storage.ErrNotFound
	case -2:
		return storage.ErrVersionConflict
	case 1:
		return nil
	default:
		return fmt.Errorf("redis store: unexpected update result: %v", result)
	}
}

var luaRemoveAccount = `
	redis.call("DEL", KEYS[1])
	redis.call("SREM", KEYS[2], ARGV[1])
	redis.call("SREM", KEYS[3], ARGV[1])
	redis.call("SREM", KEYS[4], ARGV[1])
	redis.call("SREM", KEYS[5], ARGV[1])
	return 1
`

func (s *Store) RemoveAccount(ctx context.Context, id string) error {
	key := acctDataKey(s.keyPrefix, id)

	data, err := s.client.HGetAll(ctx, key)
	if err != nil {
		return fmt.Errorf("redis store: failed to get account for removal: %w", err)
	}
	if len(data) == 0 {
		return storage.ErrNotFound
	}

	ik := acctAllIndexKeys(s.keyPrefix,
		data[acctFieldProviderType], data[acctFieldProviderName], ParseInt(data[acctFieldStatus]))

	_, err = s.client.Eval(ctx, luaRemoveAccount,
		[]string{key, ik.global, ik.typeIdx, ik.provider, ik.group}, id)
	if err != nil {
		return fmt.Errorf("redis store: failed to remove account: %w", err)
	}
	return nil
}

func (s *Store) RemoveAccounts(ctx context.Context, filter *storage.SearchFilter) error {
	accounts, err := s.SearchAccounts(ctx, filter)
	if err != nil {
		return fmt.Errorf("redis store: failed to search accounts for removal: %w", err)
	}

	for _, acct := range accounts {
		if err := s.RemoveAccount(ctx, acct.ID); err != nil && err != storage.ErrNotFound {
			return fmt.Errorf("redis store: failed to remove account %s: %w", acct.ID, err)
		}
	}
	return nil
}

// ============================
// Account 计数（优先使用 SCARD O(1)）
// ============================

func (s *Store) CountAccounts(ctx context.Context, filter *storage.SearchFilter) (int, error) {
	if filter == nil {
		n, err := s.client.SCard(ctx, acctGlobalIndexKey(s.keyPrefix))
		if err != nil {
			return 0, fmt.Errorf("redis store: failed to count accounts: %w", err)
		}
		return int(n), nil
	}

	ic := extractIndexFromSearchFilter(filter)
	if ic != nil && filter.ExtraCond == nil {
		keys, allPushed := ic.resolveIndexKeys(s.keyPrefix)
		if len(keys) > 0 && allPushed {
			total := int64(0)
			for _, k := range keys {
				n, err := s.client.SCard(ctx, k)
				if err != nil {
					return 0, fmt.Errorf("redis store: failed to scard: %w", err)
				}
				total += n
			}
			return int(total), nil
		}
	}

	accounts, err := s.SearchAccounts(ctx, filter)
	if err != nil {
		return 0, err
	}
	return len(accounts), nil
}
