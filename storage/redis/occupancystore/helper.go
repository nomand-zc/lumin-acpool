package occupancystore

const (
	// Redis key 格式。
	// occupancy:{account_id} -> String，存储单个账号的当前并发占用计数。
	keyOccupancyPrefix = "occupancy:"
)

// occupancyKey 返回占用计数的 Redis key。
func occupancyKey(prefix, accountID string) string {
	return prefix + keyOccupancyPrefix + accountID
}

// luaIncr 使用 Lua 脚本原子递增计数，返回递增后的值。
// 对不存在的 key 从 0 开始递增（Redis INCR 的默认行为）。
var luaIncr = `
	local key = KEYS[1]
	local newVal = redis.call("INCR", key)
	return newVal
`

// luaDecr 使用 Lua 脚本原子递减计数，保证不低于 0。
// 当计数归零时自动删除 key，避免 key 泄漏。
var luaDecr = `
	local key = KEYS[1]
	local val = redis.call("GET", key)
	if val == false or tonumber(val) <= 0 then
		redis.call("DEL", key)
		return 0
	end
	local newVal = redis.call("DECR", key)
	if newVal <= 0 then
		redis.call("DEL", key)
		return 0
	end
	return newVal
`

// luaGet 使用 Lua 脚本获取当前计数，不存在返回 0。
var luaGet = `
	local key = KEYS[1]
	local val = redis.call("GET", key)
	if val == false then
		return 0
	end
	return tonumber(val)
`
