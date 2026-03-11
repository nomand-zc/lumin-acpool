package occupancy

import "github.com/nomand-zc/lumin-acpool/account"

// Metadata Key 常量，用于从 Account.Metadata 中读取占用控制参数。
// 这些参数优先级高于本地代码配置，可通过管理 API 动态调整，无需重启服务。
const (
	// MetaKeyOccupancyLimit 固定并发上限（int64 / float64）。
	// 用于 FixedLimit 策略，优先级最高（高于账号 ID 级别的本地配置）。
	MetaKeyOccupancyLimit = "occupancy_limit"

	// MetaKeyOccupancyFactor 调控因子（float64）。
	// 用于 AdaptiveLimit 策略，覆盖全局 factor 配置。
	MetaKeyOccupancyFactor = "occupancy_factor"

	// MetaKeyOccupancyMinLimit 最小并发上限（int64 / float64）。
	// 用于 AdaptiveLimit 策略，覆盖全局 minLimit 配置。
	MetaKeyOccupancyMinLimit = "occupancy_min_limit"

	// MetaKeyOccupancyMaxLimit 最大并发上限（int64 / float64）。
	// 用于 AdaptiveLimit 策略，覆盖全局 maxLimit 配置。
	MetaKeyOccupancyMaxLimit = "occupancy_max_limit"

	// MetaKeyOccupancyFallbackLimit 回退并发上限（int64 / float64）。
	// 用于 AdaptiveLimit 策略，覆盖全局 fallbackLimit 配置。
	MetaKeyOccupancyFallbackLimit = "occupancy_fallback_limit"
)

// metadataInt64 从 Account.Metadata 中读取指定 key 的 int64 值。
// 支持 JSON 反序列化后的 float64 → int64 自动转换（JSON 数字默认解析为 float64）。
// 如果 key 不存在或类型不匹配，返回 (0, false)。
func metadataInt64(acct *account.Account, key string) (int64, bool) {
	if acct.Metadata == nil {
		return 0, false
	}
	v, ok := acct.Metadata[key]
	if !ok {
		return 0, false
	}
	switch val := v.(type) {
	case int64:
		return val, true
	case int:
		return int64(val), true
	case float64:
		// JSON 反序列化的数字默认为 float64
		return int64(val), true
	default:
		return 0, false
	}
}

// metadataFloat64 从 Account.Metadata 中读取指定 key 的 float64 值。
// 如果 key 不存在或类型不匹配，返回 (0, false)。
func metadataFloat64(acct *account.Account, key string) (float64, bool) {
	if acct.Metadata == nil {
		return 0, false
	}
	v, ok := acct.Metadata[key]
	if !ok {
		return 0, false
	}
	switch val := v.(type) {
	case float64:
		return val, true
	case int64:
		return float64(val), true
	case int:
		return float64(val), true
	default:
		return 0, false
	}
}
