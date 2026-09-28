-- mark_account_closing_write.lua
-- Record on the closing marker that its attempt issued the closing write.
--
-- KEYS[1]: closing marker key
-- ARGV[1]: marker value before the write was issued
-- ARGV[2]: marker value once the write was issued
--
-- The transition is conditional on the attempt's own value, so a marker that
-- belongs to another attempt is never rewritten. A marker already carrying the
-- second value reports success: the phase is what matters, not how many times it
-- was recorded.
-- Returns 1 when the marker carries the attempt's write phase, otherwise 0.

local current = redis.call('GET', KEYS[1])

if current == ARGV[2] then
    return 1
end

if current == ARGV[1] then
    redis.call('SET', KEYS[1], ARGV[2])

    return 1
end

return 0
