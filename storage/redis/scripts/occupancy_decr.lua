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
