-- Atomic rate limit check and increment script
-- Returns: {allowed: 0|1, current_count: number, ttl: number}
-- allowed: 1 if request is allowed, 0 if rate limit exceeded
-- current_count: current count after increment (if allowed) or before increment (if denied)
-- ttl: remaining TTL in seconds

local key = KEYS[1]
local increment = tonumber(ARGV[1])
local max_items = tonumber(ARGV[2])
local expiration = tonumber(ARGV[3])

-- Get current count
local current = redis.call('GET', key)
if current == false then
    current = 0
else
    current = tonumber(current)
end

-- Check if increment would exceed limit
if current + increment > max_items then
    -- Rate limit exceeded, return current state without incrementing
    local ttl = redis.call('TTL', key)
    if ttl < 0 then
        ttl = 0
    end
    return {0, current, ttl}
end

-- Increment atomically
local new_count = redis.call('INCRBY', key, increment)

-- Set expiration if this is a new key
if new_count == increment then
    redis.call('EXPIRE', key, expiration)
end

-- Get TTL
local ttl = redis.call('TTL', key)
if ttl < 0 then
    ttl = expiration
end

return {1, new_count, ttl}
