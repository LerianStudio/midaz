if redis.call("HGET", KEYS[1], ARGV[1]) then
    return 1
end

if redis.call("EXISTS", KEYS[2], KEYS[3], KEYS[4]) ~= 0 then
    return 1
end

return 0
