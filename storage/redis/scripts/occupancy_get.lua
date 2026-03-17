local key = KEYS[1]
local val = redis.call("GET", key)
if val == false then
	return 0
end
return tonumber(val)
