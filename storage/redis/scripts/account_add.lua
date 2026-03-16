local key = KEYS[1]
local id = ARGV[1]

if redis.call("EXISTS", key) == 1 then
	return 0
end

for i = 2, #ARGV - 1, 2 do
	redis.call("HSET", key, ARGV[i], ARGV[i+1])
end

redis.call("SADD", KEYS[2], id)
redis.call("SADD", KEYS[3], id)
redis.call("SADD", KEYS[4], id)
redis.call("SADD", KEYS[5], id)

-- 更新 Provider 计数
local availIncr = tonumber(ARGV[#ARGV])
if redis.call("EXISTS", KEYS[6]) == 1 then
	redis.call("HINCRBY", KEYS[6], "account_count", 1)
	if availIncr > 0 then
		redis.call("HINCRBY", KEYS[6], "available_account_count", availIncr)
	end
end

return 1
