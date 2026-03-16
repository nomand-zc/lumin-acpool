package redis

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-acpool/storage/filtercond"
)

var (
	//go:embed scripts/account_add.lua
	scriptAccountAdd string

	//go:embed scripts/account_update.lua
	scriptAccountUpdate string

	//go:embed scripts/account_remove.lua
	scriptAccountRemove string
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

func (s *Store) AddAccount(ctx context.Context, acct *account.Account) error {
	key := acctDataKey(s.keyPrefix, acct.ID)
	ik := acctAllIndexKeys(s.keyPrefix, acct.ProviderType, acct.ProviderName, int(acct.Status))
	provKey := provRedisKey(s.keyPrefix, acct.ProviderType, acct.ProviderName)

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

	availableIncr := 0
	if acct.Status == account.StatusAvailable {
		availableIncr = 1
	}

	args := []any{acct.ID}
	for k, v := range fields {
		args = append(args, k, fmt.Sprintf("%v", v))
	}
	args = append(args, availableIncr)

	result, err := s.client.Eval(ctx, scriptAccountAdd,
		[]string{key, ik.global, ik.typeIdx, ik.provider, ik.group, provKey}, args...)
	if err != nil {
		return fmt.Errorf("redis store: failed to add account: %w", err)
	}

	if result.(int64) == 0 {
		return storage.ErrAlreadyExists
	}

	return nil
}

func (s *Store) UpdateAccount(ctx context.Context, acct *account.Account, fields storage.UpdateField) error {
	key := acctDataKey(s.keyPrefix, acct.ID)

	oldData, err := s.client.HGetAll(ctx, key)
	if err != nil {
		return fmt.Errorf("redis store: failed to get old account data: %w", err)
	}
	if len(oldData) == 0 {
		return storage.ErrNotFound
	}

	acct.UpdatedAt = time.Now()

	// 按 fields 选择性序列化需要更新的字段
	hashFields := make(map[string]string)
	hashFields[acctFieldUpdatedAt] = FormatTime(acct.UpdatedAt)

	if fields.Has(storage.UpdateFieldCredential) {
		credentialJSON, err := json.Marshal(acct.Credential.ToMap())
		if err != nil {
			return fmt.Errorf("redis store: failed to marshal credential: %w", err)
		}
		hashFields[acctFieldCredential] = string(credentialJSON)
	}
	if fields.Has(storage.UpdateFieldStatus) {
		hashFields[acctFieldStatus] = acctFormatInt(int(acct.Status))
		hashFields[acctFieldCooldownUntil] = FormatTimePtr(acct.CooldownUntil)
		hashFields[acctFieldCircuitOpenUntil] = FormatTimePtr(acct.CircuitOpenUntil)
	}
	if fields.Has(storage.UpdateFieldPriority) {
		hashFields[acctFieldPriority] = acctFormatInt(acct.Priority)
	}
	if fields.Has(storage.UpdateFieldTags) {
		tagsJSON, err := MarshalJSON(acct.Tags)
		if err != nil {
			return fmt.Errorf("redis store: failed to marshal tags: %w", err)
		}
		hashFields[acctFieldTags] = tagsJSON
	}
	if fields.Has(storage.UpdateFieldMetadata) {
		metadataJSON, err := MarshalJSON(acct.Metadata)
		if err != nil {
			return fmt.Errorf("redis store: failed to marshal metadata: %w", err)
		}
		hashFields[acctFieldMetadata] = metadataJSON
	}
	if fields.Has(storage.UpdateFieldUsageRules) {
		usageRulesJSON, err := MarshalJSON(acct.UsageRules)
		if err != nil {
			return fmt.Errorf("redis store: failed to marshal usage_rules: %w", err)
		}
		hashFields[acctFieldUsageRules] = usageRulesJSON
	}

	// 构建 KEYS 和 ARGV
	// 当包含状态更新时，需要传入索引 Key 和 Provider Key 来更新索引和计数
	hasStatusUpdate := 0
	var keys []string
	if fields.Has(storage.UpdateFieldStatus) {
		hasStatusUpdate = 1
		oldType := oldData[acctFieldProviderType]
		oldName := oldData[acctFieldProviderName]
		oldStatus := ParseInt(oldData[acctFieldStatus])

		oldIK := acctAllIndexKeys(s.keyPrefix, oldType, oldName, oldStatus)
		newIK := acctAllIndexKeys(s.keyPrefix, acct.ProviderType, acct.ProviderName, int(acct.Status))
		provKey := provRedisKey(s.keyPrefix, acct.ProviderType, acct.ProviderName)

		keys = []string{
			key,
			oldIK.typeIdx, oldIK.provider, oldIK.group,
			newIK.typeIdx, newIK.provider, newIK.group,
			provKey,
		}
	} else {
		// 不更新状态时，索引 Key 使用占位符（Lua 脚本中 hasStatusUpdate=0 不会使用）
		keys = []string{key, "", "", "", "", "", "", ""}
	}

	args := []any{acctFormatInt(acct.Version), hasStatusUpdate}
	for k, v := range hashFields {
		args = append(args, k, v)
	}

	result, err := s.client.Eval(ctx, scriptAccountUpdate, keys, args...)
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

func (s *Store) RemoveAccount(ctx context.Context, id string) error {
	key := acctDataKey(s.keyPrefix, id)

	data, err := s.client.HGetAll(ctx, key)
	if err != nil {
		return fmt.Errorf("redis store: failed to get account for removal: %w", err)
	}
	if len(data) == 0 {
		return storage.ErrNotFound
	}

	providerType := data[acctFieldProviderType]
	providerName := data[acctFieldProviderName]
	status := ParseInt(data[acctFieldStatus])

	ik := acctAllIndexKeys(s.keyPrefix, providerType, providerName, status)
	provKey := provRedisKey(s.keyPrefix, providerType, providerName)

	availableDecr := 0
	if account.Status(status) == account.StatusAvailable {
		availableDecr = 1
	}

	_, err = s.client.Eval(ctx, scriptAccountRemove,
		[]string{key, ik.global, ik.typeIdx, ik.provider, ik.group, provKey},
		id, availableDecr)
	if err != nil {
		return fmt.Errorf("redis store: failed to remove account: %w", err)
	}

	// 删除关联的统计数据
	if err := s.RemoveStats(ctx, id); err != nil {
		return fmt.Errorf("redis store: failed to remove account stats: %w", err)
	}

	// 删除关联的用量追踪数据
	if err := s.RemoveUsages(ctx, id); err != nil {
		return fmt.Errorf("redis store: failed to remove account usages: %w", err)
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
