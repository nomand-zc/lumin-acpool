package statsstore

const (
	// Redis key 格式。
	// stats:{account_id} -> Hash，存储单个账号的统计数据。

	keyStatsPrefix = "stats:"
)

// Hash 字段名常量。
const (
	fieldAccountID           = "account_id"
	fieldTotalCalls          = "total_calls"
	fieldSuccessCalls        = "success_calls"
	fieldFailedCalls         = "failed_calls"
	fieldConsecutiveFailures = "consecutive_failures"
	fieldLastUsedAt          = "last_used_at"
	fieldLastErrorAt         = "last_error_at"
	fieldLastErrorMsg        = "last_error_msg"
)

// statsKey 返回统计数据的 Redis Hash key。
func statsKey(prefix, accountID string) string {
	return prefix + keyStatsPrefix + accountID
}

// luaIncrSuccess 使用 Lua 脚本原子递增成功计数：
// 递增 total_calls 和 success_calls，重置 consecutive_failures，更新 last_used_at。
var luaIncrSuccess = `
	local key = KEYS[1]
	local now = ARGV[1]
	local accountID = ARGV[2]
	
	redis.call("HSET", key, "account_id", accountID)
	redis.call("HINCRBY", key, "total_calls", 1)
	redis.call("HINCRBY", key, "success_calls", 1)
	redis.call("HSET", key, "consecutive_failures", "0")
	redis.call("HSET", key, "last_used_at", now)
	
	return 1
`

// luaIncrFailure 使用 Lua 脚本原子递增失败计数：
// 递增 total_calls、failed_calls 和 consecutive_failures，更新 last_error_at 和 last_error_msg。
// 返回递增后的 consecutive_failures 值。
var luaIncrFailure = `
	local key = KEYS[1]
	local now = ARGV[1]
	local errMsg = ARGV[2]
	local accountID = ARGV[3]
	
	redis.call("HSET", key, "account_id", accountID)
	redis.call("HINCRBY", key, "total_calls", 1)
	redis.call("HINCRBY", key, "failed_calls", 1)
	local failures = redis.call("HINCRBY", key, "consecutive_failures", 1)
	redis.call("HSET", key, "last_error_at", now)
	redis.call("HSET", key, "last_error_msg", errMsg)
	
	return failures
`
