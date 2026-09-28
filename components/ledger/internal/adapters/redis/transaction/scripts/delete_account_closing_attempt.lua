-- delete_account_closing_attempt.lua
-- Remove a closing marker that belongs to the calling attempt, in either phase.
--
-- KEYS[1]: closing marker key
-- ARGV[1]: marker value before the closing write was issued
-- ARGV[2]: marker value once the closing write was issued
--
-- Both values are accepted because the owner may remove its marker before or
-- after issuing its write, and only the owner calls this. A marker carrying any
-- other value belongs to another attempt and is left untouched.
-- Returns 1 when the marker was removed, otherwise 0.

local current = redis.call('GET', KEYS[1])

if current == ARGV[1] or current == ARGV[2] then
    return redis.call('DEL', KEYS[1])
end

return 0
