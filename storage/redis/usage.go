package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-client/usagerule"
)

const (
	usageKeyPrefix = "usage:"
	usageKeyCount  = ":count"
)

// Usage Hash 字段名常量。
const (
	usageFieldSourceType      = "source_type"
	usageFieldTimeGranularity = "time_granularity"
	usageFieldWindowSize      = "window_size"
	usageFieldRuleTotal       = "rule_total"
	usageFieldLocalUsed       = "local_used"
	usageFieldRemoteUsed      = "remote_used"
	usageFieldRemoteRemain    = "remote_remain"
	usageFieldWindowStart     = "window_start"
	usageFieldWindowEnd       = "window_end"
	usageFieldLastSyncAt      = "last_sync_at"
)

func usageCountRedisKey(prefix, accountID string) string {
	return prefix + usageKeyPrefix + accountID + usageKeyCount
}

func usageRuleRedisKey(prefix, accountID string, ruleIndex int) string {
	return prefix + usageKeyPrefix + accountID + ":" + strconv.Itoa(ruleIndex)
}

func marshalUsageToHash(u *account.TrackedUsage) map[string]any {
	fields := map[string]any{
		usageFieldLocalUsed:    fmt.Sprintf("%.6f", u.LocalUsed),
		usageFieldRemoteUsed:   fmt.Sprintf("%.6f", u.RemoteUsed),
		usageFieldRemoteRemain: fmt.Sprintf("%.6f", u.RemoteRemain),
		usageFieldWindowStart:  FormatTimePtr(u.WindowStart),
		usageFieldWindowEnd:    FormatTimePtr(u.WindowEnd),
		usageFieldLastSyncAt:   FormatTime(u.LastSyncAt),
	}

	if u.Rule != nil {
		fields[usageFieldSourceType] = int(u.Rule.SourceType)
		fields[usageFieldTimeGranularity] = string(u.Rule.TimeGranularity)
		fields[usageFieldWindowSize] = u.Rule.WindowSize
		fields[usageFieldRuleTotal] = fmt.Sprintf("%.6f", u.Rule.Total)
	}

	return fields
}

func unmarshalUsageFromHash(data map[string]string) (*account.TrackedUsage, error) {
	if len(data) == 0 {
		return nil, nil
	}

	usage := &account.TrackedUsage{
		Rule: &usagerule.UsageRule{
			SourceType:      usagerule.SourceType(ParseInt(data[usageFieldSourceType])),
			TimeGranularity: usagerule.TimeGranularity(data[usageFieldTimeGranularity]),
			WindowSize:      ParseInt(data[usageFieldWindowSize]),
			Total:           ParseFloat64(data[usageFieldRuleTotal]),
		},
		LocalUsed:    ParseFloat64(data[usageFieldLocalUsed]),
		RemoteUsed:   ParseFloat64(data[usageFieldRemoteUsed]),
		RemoteRemain: ParseFloat64(data[usageFieldRemoteRemain]),
		WindowStart:  ParseTimePtr(data[usageFieldWindowStart]),
		WindowEnd:    ParseTimePtr(data[usageFieldWindowEnd]),
	}

	if s := data[usageFieldLastSyncAt]; s != "" {
		if t, err := ParseTime(s); err == nil {
			usage.LastSyncAt = t
		}
	}

	return usage, nil
}

func (s *Store) GetAllUsages(ctx context.Context, accountID string) ([]*account.TrackedUsage, error) {
	countKey := usageCountRedisKey(s.keyPrefix, accountID)
	countStr, err := s.client.Get(ctx, countKey)
	if err != nil {
		if IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("redis store: failed to get usage count: %w", err)
	}

	count, err := strconv.Atoi(countStr)
	if err != nil || count <= 0 {
		return nil, nil
	}

	var result []*account.TrackedUsage
	for i := range count {
		key := usageRuleRedisKey(s.keyPrefix, accountID, i)
		data, err := s.client.HGetAll(ctx, key)
		if err != nil {
			return nil, fmt.Errorf("redis store: failed to get usage at index %d: %w", i, err)
		}
		if len(data) == 0 {
			continue
		}

		usage, err := unmarshalUsageFromHash(data)
		if err != nil {
			return nil, fmt.Errorf("redis store: failed to unmarshal usage at index %d: %w", i, err)
		}
		result = append(result, usage)
	}

	return result, nil
}

func (s *Store) SaveUsages(ctx context.Context, accountID string, usages []*account.TrackedUsage) error {
	countKey := usageCountRedisKey(s.keyPrefix, accountID)

	oldCountStr, _ := s.client.Get(ctx, countKey)
	oldCount, _ := strconv.Atoi(oldCountStr)

	for i := range oldCount {
		key := usageRuleRedisKey(s.keyPrefix, accountID, i)
		if err := s.client.Del(ctx, key); err != nil {
			return fmt.Errorf("redis store: failed to delete old usage at index %d: %w", i, err)
		}
	}

	for i, u := range usages {
		key := usageRuleRedisKey(s.keyPrefix, accountID, i)
		fields := marshalUsageToHash(u)

		args := make([]any, 0, len(fields)*2)
		for k, v := range fields {
			args = append(args, k, v)
		}

		if err := s.client.HSet(ctx, key, args...); err != nil {
			return fmt.Errorf("redis store: failed to save usage at index %d: %w", i, err)
		}
	}

	if err := s.client.Set(ctx, countKey, strconv.Itoa(len(usages)), 0); err != nil {
		return fmt.Errorf("redis store: failed to update usage count: %w", err)
	}

	return nil
}

func (s *Store) IncrLocalUsed(ctx context.Context, accountID string, ruleIndex int, amount float64) error {
	key := usageRuleRedisKey(s.keyPrefix, accountID, ruleIndex)

	exists, err := s.client.Exists(ctx, key)
	if err != nil {
		return fmt.Errorf("redis store: failed to check usage existence: %w", err)
	}
	if exists == 0 {
		return nil
	}

	_, err = s.client.HIncrByFloat(ctx, key, usageFieldLocalUsed, amount)
	if err != nil {
		return fmt.Errorf("redis store: failed to incr local used: %w", err)
	}
	return nil
}

func (s *Store) RemoveUsages(ctx context.Context, accountID string) error {
	countKey := usageCountRedisKey(s.keyPrefix, accountID)

	countStr, _ := s.client.Get(ctx, countKey)
	count, _ := strconv.Atoi(countStr)

	keys := []string{countKey}
	for i := range count {
		keys = append(keys, usageRuleRedisKey(s.keyPrefix, accountID, i))
	}

	if err := s.client.Del(ctx, keys...); err != nil {
		return fmt.Errorf("redis store: failed to remove usages: %w", err)
	}
	return nil
}

func (s *Store) CalibrateRule(ctx context.Context, accountID string, ruleIndex int, usage *account.TrackedUsage) error {
	key := usageRuleRedisKey(s.keyPrefix, accountID, ruleIndex)

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
		FormatTimePtr(usage.WindowStart),
		FormatTimePtr(usage.WindowEnd),
		FormatTime(time.Now()),
	)
	if err != nil {
		return fmt.Errorf("redis store: failed to calibrate rule: %w", err)
	}
	return nil
}

// _ 消除未使用的导入警告。
var _ = json.Marshal
