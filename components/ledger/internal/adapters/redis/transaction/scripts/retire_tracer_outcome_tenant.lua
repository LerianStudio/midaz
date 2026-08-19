if redis.call("ZCARD", KEYS[1]) > 0 then
    return 0
end
local current = redis.call("ZSCORE", KEYS[2], ARGV[1])
if not current or tonumber(current) ~= tonumber(ARGV[2]) then
    return 0
end
return redis.call("ZREM", KEYS[2], ARGV[1])
