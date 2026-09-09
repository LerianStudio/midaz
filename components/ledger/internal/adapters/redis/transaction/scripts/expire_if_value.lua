-- expire_if_value.lua
-- Atomically shorten a key's TTL only when its value matches the expected owner token.
--
-- KEYS[1]: key whose expiry is being shortened
-- ARGV[1]: expected value
-- ARGV[2]: expiry in seconds
-- Returns 1 when the expiry was updated, otherwise 0.

if redis.call('GET', KEYS[1]) == ARGV[1] then
    return redis.call('EXPIRE', KEYS[1], ARGV[2])
end

return 0
