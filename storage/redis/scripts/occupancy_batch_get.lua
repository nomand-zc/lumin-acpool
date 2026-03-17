-- 批量获取多个账号的占用计数。
-- KEYS: 所有待查询的 occupancy key
-- 返回: 与 KEYS 顺序一致的占用数数组。
local results = {}
for i, key in ipairs(KEYS) do
	local val = redis.call("GET", key)
	if val == false then
		results[i] = 0
	else
		results[i] = tonumber(val)
	end
end
return results
