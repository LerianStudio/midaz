-- remove_balance_sync_keys_batch.lua
-- Conditionally removes members from the balance sync schedule and their locks.
-- Only removes a member if its current ZSET score matches the claimed score,
-- preventing removal of entries re-scheduled by newer balance mutations.
--
-- KEYS[1]: schedule key (e.g., "schedule:{transactions}:balance-sync")
-- ARGV[1]: lock prefix (e.g., "lock:{transactions}:balance-sync:")
-- ARGV[2..N]: alternating pairs of [member, claimedScore, member, claimedScore, ...]

local scheduleKey = KEYS[1]
local lockPrefix = ARGV[1]
local removed = 0

for i = 2, #ARGV, 2 do
    local member = ARGV[i]
    local claimedScore = tonumber(ARGV[i + 1])

    -- Only remove if the score has NOT been updated by a newer mutation.
    -- If a new ZADD happened after our claim, currentScore > claimedScore
    -- and we must NOT remove (the newer mutation needs to be synced).
    local currentScore = redis.call('ZSCORE', scheduleKey, member)

    if currentScore then
        currentScore = tonumber(currentScore)
        if currentScore <= claimedScore then
            redis.call('ZREM', scheduleKey, member)
            removed = removed + 1
        end
    end

    -- Always remove the lock (it's ours regardless of ZREM outcome)
    local lockKey = lockPrefix .. member
    redis.call('DEL', lockKey)
end

return removed
