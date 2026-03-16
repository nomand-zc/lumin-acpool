-- 删除账号数据和索引
redis.call("DEL", KEYS[1])
redis.call("SREM", KEYS[2], ARGV[1])
redis.call("SREM", KEYS[3], ARGV[1])
redis.call("SREM", KEYS[4], ARGV[1])
redis.call("SREM", KEYS[5], ARGV[1])

-- 更新 Provider 计数
local availDecr = tonumber(ARGV[2])
if redis.call("EXISTS", KEYS[6]) == 1 then
	local curCount = tonumber(redis.call("HGET", KEYS[6], "account_count")) or 0
	local curAvail = tonumber(redis.call("HGET", KEYS[6], "available_account_count")) or 0
	local newCount = math.max(curCount - 1, 0)
	local newAvail = math.max(curAvail - availDecr, 0)
	redis.call("HSET", KEYS[6], "account_count", newCount)
	redis.call("HSET", KEYS[6], "available_account_count", newAvail)
end

return 1
