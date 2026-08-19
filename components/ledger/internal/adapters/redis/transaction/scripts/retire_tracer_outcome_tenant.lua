local current = redis.call("ZSCORE", KEYS[1], ARGV[1])
if not current or tonumber(current) ~= tonumber(ARGV[2]) then
    return 0
end
return redis.call("ZREM", KEYS[1], ARGV[1])
