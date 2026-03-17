local key = KEYS[1]
local newVal = redis.call("INCR", key)
return newVal
