if redis.call('EXISTS', KEYS[2]) == 1 then
    return 0
end
if redis.call('GET', KEYS[1]) ~= '' then
    return 0
end

redis.call('SET', KEYS[1], ARGV[1], 'EX', ARGV[2])
return 1
