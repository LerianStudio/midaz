if redis.call('GET', KEYS[2]) ~= ARGV[1] then
    return 0
end
if redis.call('GET', KEYS[1]) ~= '' then
    return 0
end

redis.call('DEL', KEYS[2])
return redis.call('DEL', KEYS[1])
