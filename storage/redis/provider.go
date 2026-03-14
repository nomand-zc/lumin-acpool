package redis

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-acpool/storage"
	"github.com/nomand-zc/lumin-acpool/storage/filtercond"
)

var (
	//go:embed scripts/provider_add.lua
	scriptProviderAdd string

	//go:embed scripts/provider_remove.lua
	scriptProviderRemove string
)

func (s *Store) GetProvider(ctx context.Context, key account.ProviderKey) (*account.ProviderInfo, error) {
	redisKey := provRedisKey(s.keyPrefix, key.Type, key.Name)
	data, err := s.client.HGetAll(ctx, redisKey)
	if err != nil {
		return nil, fmt.Errorf("redis store: failed to get provider: %w", err)
	}
	if len(data) == 0 {
		return nil, storage.ErrNotFound
	}

	info, err := unmarshalProviderFromHash(data)
	if err != nil {
		return nil, fmt.Errorf("redis store: failed to unmarshal provider: %w", err)
	}
	return info, nil
}

func (s *Store) SearchProviders(ctx context.Context, filter *storage.SearchFilter) ([]*account.ProviderInfo, error) {
	members, err := s.client.SMembers(ctx, provIndexRedisKey(s.keyPrefix))
	if err != nil {
		return nil, fmt.Errorf("redis store: failed to get provider ids: %w", err)
	}

	if len(members) == 0 {
		return nil, nil
	}

	pipe := s.client.Pipeline(ctx)
	cmds := make(map[string]*pipelineCmd, len(members))
	for _, member := range members {
		key := s.keyPrefix + provKeyDataPrefix + member
		cmd := pipe.HGetAll(ctx, key)
		cmds[member] = &pipelineCmd{cmd: cmd}
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return nil, fmt.Errorf("redis store: failed to pipeline get providers: %w", err)
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

		if !matchProviderSearchFilter(info, filter) {
			continue
		}

		if s.providerEvaluator.Match(info, extraCond) {
			result = append(result, info)
		}
	}

	return result, nil
}

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
	if filter.SupportedModel != "" && !info.SupportsModel(filter.SupportedModel) {
		return false
	}
	return true
}

func (s *Store) AddProvider(ctx context.Context, info *account.ProviderInfo) error {
	redisKey := provRedisKey(s.keyPrefix, info.ProviderType, info.ProviderName)
	indexKey := provIndexRedisKey(s.keyPrefix)
	member := provIndexMember(info.ProviderType, info.ProviderName)

	exists, err := s.client.Exists(ctx, redisKey)
	if err != nil {
		return fmt.Errorf("redis store: failed to check provider existence: %w", err)
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
		return fmt.Errorf("redis store: %w", err)
	}

	args := []any{member}
	for k, v := range fields {
		args = append(args, k, fmt.Sprintf("%v", v))
	}

	result, err := s.client.Eval(ctx, scriptProviderAdd, []string{redisKey, indexKey}, args...)
	if err != nil {
		return fmt.Errorf("redis store: failed to add provider: %w", err)
	}
	if result.(int64) == 0 {
		return storage.ErrAlreadyExists
	}
	return nil
}

func (s *Store) UpdateProvider(ctx context.Context, info *account.ProviderInfo) error {
	redisKey := provRedisKey(s.keyPrefix, info.ProviderType, info.ProviderName)

	exists, err := s.client.Exists(ctx, redisKey)
	if err != nil {
		return fmt.Errorf("redis store: failed to check provider existence: %w", err)
	}
	if exists == 0 {
		return storage.ErrNotFound
	}

	info.UpdatedAt = time.Now()

	fields, err := marshalProviderToHash(info)
	if err != nil {
		return fmt.Errorf("redis store: %w", err)
	}

	args := make([]any, 0, len(fields)*2)
	for k, v := range fields {
		args = append(args, k, v)
	}

	err = s.client.HSet(ctx, redisKey, args...)
	if err != nil {
		return fmt.Errorf("redis store: failed to update provider: %w", err)
	}
	return nil
}

// RemoveProvider 删除 Provider
// 删除操作本事是一个低频操作， 暂不做优化
func (s *Store) RemoveProvider(ctx context.Context, key account.ProviderKey) error {
	redisKey := provRedisKey(s.keyPrefix, key.Type, key.Name)
	indexKey := provIndexRedisKey(s.keyPrefix)
	member := provIndexMember(key.Type, key.Name)

	exists, err := s.client.Exists(ctx, redisKey)
	if err != nil {
		return fmt.Errorf("redis store: failed to check provider existence: %w", err)
	}
	if exists == 0 {
		return storage.ErrNotFound
	}

	// 级联删除该 Provider 下的所有 Account 及关联数据
	filter := &storage.SearchFilter{
		ProviderType: key.Type,
		ProviderName: key.Name,
	}
	accounts, err := s.SearchAccounts(ctx, filter)
	if err != nil {
		return fmt.Errorf("redis store: failed to search accounts for cascade removal: %w", err)
	}
	for _, acct := range accounts {
		if removeErr := s.RemoveAccount(ctx, acct.ID); removeErr != nil && removeErr != storage.ErrNotFound {
			return fmt.Errorf("redis store: failed to cascade remove account %s: %w", acct.ID, removeErr)
		}
	}

	// 删除 Provider 自身
	_, err = s.client.Eval(ctx, scriptProviderRemove, []string{redisKey, indexKey}, member)
	if err != nil {
		return fmt.Errorf("redis store: failed to remove provider: %w", err)
	}
	return nil
}
