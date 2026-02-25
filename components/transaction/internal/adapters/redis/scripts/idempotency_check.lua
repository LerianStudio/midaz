-- KEYS[1] = idempotency internal key
-- ARGV[1] = TTL in seconds
-- Return nil when lock is acquired (new request), or cached value for duplicates.

local existing = redis.call("GET", KEYS[1])
if existing then
    return existing
end

local locked = redis.call("SET", KEYS[1], "", "EX", ARGV[1], "NX")
if locked then
    return nil
end

return redis.call("GET", KEYS[1])
