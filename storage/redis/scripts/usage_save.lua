-- usage_save.lua
-- 原子性地删除旧用量数据并写入新用量数据，保证不存在部分写入的中间状态。
--
-- KEYS[1] = countKey (usage:<accountID>:count)
-- ARGV[1] = oldCount (当前已有条目数，用于删除旧 key)
-- ARGV[2] = newCount (新条目数)
-- ARGV[3] = keyPrefix (usage:<accountID>: 前缀，脚本内拼接 ruleIndex 生成完整 key)
-- ARGV[4] = fieldCount (每条规则的 field+value 对数，即 field 数量)
-- ARGV[5..] = rule0_field0, rule0_val0, ..., rule0_fieldN, rule0_valN,
--             rule1_field0, rule1_val0, ..., (依次排列，每条规则 fieldCount*2 个元素)

local count_key = KEYS[1]
local old_count = tonumber(ARGV[1])
local new_count = tonumber(ARGV[2])
local key_prefix = ARGV[3]
local field_count = tonumber(ARGV[4])

-- 删除旧条目
for i = 0, old_count - 1 do
	redis.call("DEL", key_prefix .. tostring(i))
end

-- 写入新条目
local base = 5 -- ARGV 从第 5 个元素开始是规则数据
for i = 0, new_count - 1 do
	local rule_key = key_prefix .. tostring(i)
	local offset = base + i * field_count * 2
	for j = 0, field_count - 1 do
		local field = ARGV[offset + j * 2]
		local val   = ARGV[offset + j * 2 + 1]
		redis.call("HSET", rule_key, field, val)
	end
end

-- 更新 count
redis.call("SET", count_key, tostring(new_count))

return 1
