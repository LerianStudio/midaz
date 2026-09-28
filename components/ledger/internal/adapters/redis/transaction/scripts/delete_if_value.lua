-- delete_if_value.lua
-- Atomically delete a key only when its value matches the expected owner token.
--
-- KEYS[1]: key to delete
-- ARGV[1]: expected value
-- Returns 1 when the key was deleted, otherwise 0.

if redis.call('GET', KEYS[1]) == ARGV[1] then
    return redis.call('DEL', KEYS[1])
end

return 0
