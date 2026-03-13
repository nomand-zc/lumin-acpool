-- provider_add.lua
-- 原子添加供应商：检查 key 不存在后写入 Hash 并加入索引 Set。
-- KEYS[1] = provider hash key
-- KEYS[2] = provider index set key
-- ARGV[1] = index member (type/name)
-- ARGV[2..N] = field/value pairs

local key = KEYS[1]
local indexKey = KEYS[2]
local member = ARGV[1]

if redis.call("EXISTS", key) == 1 then
    return 0
end

for i = 2, #ARGV, 2 do
    redis.call("HSET", key, ARGV[i], ARGV[i+1])
end

redis.call("SADD", indexKey, member)
return 1
