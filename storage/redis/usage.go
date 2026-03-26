package redis

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/nomand-zc/lumin-acpool/account"
	"github.com/nomand-zc/lumin-client/usagerule"
)

//go:embed scripts/usage_save.lua
var scriptUsageSave string

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

func (s *Store) GetCurrentUsages(ctx context.Context, accountID string) ([]*account.TrackedUsage, error) {
	countKey := usageCountRedisKey(s.keyPrefix, accountID)
	keyPattern := s.keyPrefix + usageKeyPrefix + accountID + ":"

	// Lua 脚本：在 Redis 服务端过滤当前窗口内的数据，避免将过期数据传回客户端。
	// RFC3339Nano 格式天然支持字典序比较，无需额外解析。
	script := `
		local count_str = redis.call("GET", KEYS[1])
		if not count_str then
			return {}
		end
		local count = tonumber(count_str)
		if not count or count <= 0 then
			return {}
		end

		local now = ARGV[1]
		local prefix = ARGV[2]
		local results = {}

		for i = 0, count - 1 do
			local key = prefix .. tostring(i)
			local window_end = redis.call("HGET", key, "window_end")
			-- 仅返回窗口未过期的数据（window_end 为空或 >= 当前时间）
			if not window_end or window_end == "" or window_end >= now then
				local data = redis.call("HGETALL", key)
				if #data > 0 then
					table.insert(results, cjson.encode(data))
				end
			end
		end
		return results
	`

	now := time.Now().Format(time.RFC3339Nano)
	rawResults, err := s.client.Eval(ctx, script, []string{countKey}, now, keyPattern)
	if err != nil {
		if IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("redis store: failed to get current usages: %w", err)
	}

	results, ok := rawResults.([]interface{})
	if !ok || len(results) == 0 {
		return nil, nil
	}

	var usages []*account.TrackedUsage
	for _, r := range results {
		jsonStr, ok := r.(string)
		if !ok {
			continue
		}
		// Lua cjson.encode(HGETALL result) 返回 ["field1","val1","field2","val2",...] 数组
		var pairs []string
		if err := json.Unmarshal([]byte(jsonStr), &pairs); err != nil {
			continue
		}
		data := make(map[string]string, len(pairs)/2)
		for j := 0; j+1 < len(pairs); j += 2 {
			data[pairs[j]] = pairs[j+1]
		}
		usage, err := unmarshalUsageFromHash(data)
		if err != nil {
			continue
		}
		usages = append(usages, usage)
	}

	return usages, nil
}

func (s *Store) SaveUsages(ctx context.Context, accountID string, usages []*account.TrackedUsage) error {
	countKey := usageCountRedisKey(s.keyPrefix, accountID)
	keyPrefix := usageRuleRedisKey(s.keyPrefix, accountID, 0)
	// 去掉末尾的 "0"，保留 "usage:<accountID>:" 前缀供 Lua 脚本拼接索引
	keyPrefix = keyPrefix[:len(keyPrefix)-1]

	// 查询当前已有条目数，用于 Lua 脚本删除旧 key
	oldCountStr, _ := s.client.Get(ctx, countKey)
	oldCount, _ := strconv.Atoi(oldCountStr)

	// 将所有规则的字段序列化为 ARGV 列表
	// 格式：ARGV[5..] = rule0_f0, rule0_v0, rule0_f1, rule0_v1, ..., rule1_f0, rule1_v0, ...
	// 要求所有规则的字段数必须一致（marshalUsageToHash 对相同 Rule 存在性产生相同字段集）
	var ruleArgs []any
	actualFieldCount := -1
	for _, u := range usages {
		fields := marshalUsageToHash(u)
		if actualFieldCount == -1 {
			actualFieldCount = len(fields)
		} else if len(fields) != actualFieldCount {
			return fmt.Errorf("redis store: inconsistent usage field count: got %d, expected %d", len(fields), actualFieldCount)
		}
		for k, v := range fields {
			ruleArgs = append(ruleArgs, k, fmt.Sprintf("%v", v))
		}
	}

	if actualFieldCount < 0 {
		actualFieldCount = 0
	}

	// ARGV: [oldCount, newCount, keyPrefix, fieldCount, rule0_fields..., rule1_fields..., ...]
	args := []any{
		strconv.Itoa(oldCount),
		strconv.Itoa(len(usages)),
		keyPrefix,
		strconv.Itoa(actualFieldCount),
	}
	args = append(args, ruleArgs...)

	_, err := s.client.Eval(ctx, scriptUsageSave, []string{countKey}, args...)
	if err != nil {
		return fmt.Errorf("redis store: failed to save usages atomically: %w", err)
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
