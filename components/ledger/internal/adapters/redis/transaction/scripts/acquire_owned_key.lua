local ttl = tonumber(ARGV[2])
local acquired

-- A surviving owner companion is authoritative even if eviction removed the
-- main value first. Never overwrite an owner-only state.
if redis.call('EXISTS', KEYS[2]) == 1 then
    return 0
end

if ttl and ttl > 0 then
    acquired = redis.call('SET', KEYS[1], '', 'EX', ttl, 'NX')
else
    acquired = redis.call('SET', KEYS[1], '', 'NX')
end

if acquired then
    if ttl and ttl > 0 then
        redis.call('SET', KEYS[2], ARGV[1], 'EX', ttl)
    else
        redis.call('SET', KEYS[2], ARGV[1])
    end
    return 1
end

return 0
