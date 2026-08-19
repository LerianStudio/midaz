if redis.call("EXISTS", KEYS[1]) == 1 then
    return 0
end
redis.call("ZREM", KEYS[2], ARGV[1])
redis.call("ZREM", KEYS[3], ARGV[1])
redis.call("SREM", KEYS[4], ARGV[1])
return 1
