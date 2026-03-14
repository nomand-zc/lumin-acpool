-- provider_remove.lua
-- 原子删除供应商：删除 Hash key 并从索引 Set 中移除。
-- KEYS[1] = provider hash key
-- KEYS[2] = provider index set key
-- ARGV[1] = index member (type/name)

redis.call("DEL", KEYS[1])
redis.call("SREM", KEYS[2], ARGV[1])
return 1
