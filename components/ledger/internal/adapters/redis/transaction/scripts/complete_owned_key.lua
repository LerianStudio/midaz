if redis.call('GET', KEYS[2]) ~= ARGV[1] then
    return 0
end
local current = redis.call('GET', KEYS[1])
-- The exact owner remains authoritative if Redis evicted only the main key.
-- A durable terminal caller may rematerialize its replay, but it can never
-- overwrite a non-empty foreign main value.
if current and current ~= '' then
    return 0
end

redis.call('SET', KEYS[1], ARGV[2], 'EX', ARGV[3])
redis.call('DEL', KEYS[2])
return 1
