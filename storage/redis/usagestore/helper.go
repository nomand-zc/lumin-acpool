package usagestore

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/nomand-zc/lumin-acpool/account"
	storeRedis "github.com/nomand-zc/lumin-acpool/storage/redis"
	"github.com/nomand-zc/lumin-client/usagerule"
)

const (
	// Redis key 格式。
	// usage:{account_id}          -> 元数据 key (String，存储规则数量)
	// usage:{account_id}:{index}  -> Hash，存储单条规则的追踪数据

	keyUsagePrefix = "usage:"
	keyUsageCount  = ":count"
)

// Hash 字段名常量。
const (
	fieldSourceType      = "source_type"
	fieldTimeGranularity = "time_granularity"
	fieldWindowSize      = "window_size"
	fieldRuleTotal       = "rule_total"
	fieldLocalUsed       = "local_used"
	fieldRemoteUsed      = "remote_used"
	fieldRemoteRemain    = "remote_remain"
	fieldWindowStart     = "window_start"
	fieldWindowEnd       = "window_end"
	fieldLastSyncAt      = "last_sync_at"
)

// usageCountKey 返回指定账号用量规则数量的 Redis key。
func usageCountKey(prefix, accountID string) string {
	return prefix + keyUsagePrefix + accountID + keyUsageCount
}

// usageRuleKey 返回指定账号某条规则的 Redis Hash key。
func usageRuleKey(prefix, accountID string, ruleIndex int) string {
	return prefix + keyUsagePrefix + accountID + ":" + strconv.Itoa(ruleIndex)
}

// usageKeyPattern 返回指定账号所有用量规则 key 的匹配模式。
func usageKeyPattern(prefix, accountID string) string {
	return prefix + keyUsagePrefix + accountID + ":*"
}

// marshalUsageToHash 将 TrackedUsage 序列化为 Redis Hash 字段值映射。
func marshalUsageToHash(u *account.TrackedUsage) map[string]any {
	fields := map[string]any{
		fieldLocalUsed:    fmt.Sprintf("%.6f", u.LocalUsed),
		fieldRemoteUsed:   fmt.Sprintf("%.6f", u.RemoteUsed),
		fieldRemoteRemain: fmt.Sprintf("%.6f", u.RemoteRemain),
		fieldWindowStart:  storeRedis.FormatTimePtr(u.WindowStart),
		fieldWindowEnd:    storeRedis.FormatTimePtr(u.WindowEnd),
		fieldLastSyncAt:   storeRedis.FormatTime(u.LastSyncAt),
	}

	if u.Rule != nil {
		fields[fieldSourceType] = int(u.Rule.SourceType)
		fields[fieldTimeGranularity] = string(u.Rule.TimeGranularity)
		fields[fieldWindowSize] = u.Rule.WindowSize
		fields[fieldRuleTotal] = fmt.Sprintf("%.6f", u.Rule.Total)
	}

	return fields
}

// unmarshalUsageFromHash 从 Redis Hash 字段值映射中反序列化 TrackedUsage。
func unmarshalUsageFromHash(data map[string]string) (*account.TrackedUsage, error) {
	if len(data) == 0 {
		return nil, nil
	}

	usage := &account.TrackedUsage{
		Rule: &usagerule.UsageRule{
			SourceType:      usagerule.SourceType(storeRedis.ParseInt(data[fieldSourceType])),
			TimeGranularity: usagerule.TimeGranularity(data[fieldTimeGranularity]),
			WindowSize:      storeRedis.ParseInt(data[fieldWindowSize]),
			Total:           storeRedis.ParseFloat64(data[fieldRuleTotal]),
		},
		LocalUsed:    storeRedis.ParseFloat64(data[fieldLocalUsed]),
		RemoteUsed:   storeRedis.ParseFloat64(data[fieldRemoteUsed]),
		RemoteRemain: storeRedis.ParseFloat64(data[fieldRemoteRemain]),
		WindowStart:  storeRedis.ParseTimePtr(data[fieldWindowStart]),
		WindowEnd:    storeRedis.ParseTimePtr(data[fieldWindowEnd]),
	}

	if s := data[fieldLastSyncAt]; s != "" {
		if t, err := storeRedis.ParseTime(s); err == nil {
			usage.LastSyncAt = t
		}
	}

	return usage, nil
}

// _ 消除未使用的导入警告。
var _ = json.Marshal
