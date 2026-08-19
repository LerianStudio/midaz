local raw = redis.call("HGET", KEYS[1], KEYS[2])
if not raw or raw ~= ARGV[1] then
    return 0
end

return redis.call("HDEL", KEYS[1], KEYS[2])
