local key = KEYS[1]
local expectedVersion = ARGV[1]
local hasStatusUpdate = tonumber(ARGV[2])

if redis.call("EXISTS", key) == 0 then
	return -1
end

local currentVersion = redis.call("HGET", key, "version")
if currentVersion ~= expectedVersion then
	return -2
end

local id = redis.call("HGET", key, "id")
local oldStatus = nil
if hasStatusUpdate == 1 then
	oldStatus = tonumber(redis.call("HGET", key, "status"))
end

for i = 3, #ARGV, 2 do
	redis.call("HSET", key, ARGV[i], ARGV[i+1])
end

redis.call("HINCRBY", key, "version", 1)

-- 更新状态索引（仅当包含状态更新时）
if hasStatusUpdate == 1 then
	if KEYS[2] ~= KEYS[5] then
		redis.call("SREM", KEYS[2], id)
		redis.call("SADD", KEYS[5], id)
	end
	if KEYS[3] ~= KEYS[6] then
		redis.call("SREM", KEYS[3], id)
		redis.call("SADD", KEYS[6], id)
	end
	if KEYS[4] ~= KEYS[7] then
		redis.call("SREM", KEYS[4], id)
		redis.call("SADD", KEYS[7], id)
	end

	-- 更新 Provider 的可用账号数量
	local newStatus = tonumber(redis.call("HGET", key, "status"))
	local AVAILABLE = 1
	if oldStatus ~= newStatus and KEYS[8] ~= "" then
		if redis.call("EXISTS", KEYS[8]) == 1 then
			if oldStatus == AVAILABLE and newStatus ~= AVAILABLE then
				local cur = tonumber(redis.call("HGET", KEYS[8], "available_account_count")) or 0
				redis.call("HSET", KEYS[8], "available_account_count", math.max(cur - 1, 0))
			elseif oldStatus ~= AVAILABLE and newStatus == AVAILABLE then
				redis.call("HINCRBY", KEYS[8], "available_account_count", 1)
			end
		end
	end
end

return 1
