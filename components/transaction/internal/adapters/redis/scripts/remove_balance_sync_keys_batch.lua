-- remove_balance_sync_keys_batch.lua
-- Removes multiple members from the balance sync schedule and their associated locks.
-- KEYS[1]: schedule key (e.g., "schedule:{transactions}:balance-sync")
-- ARGV[1]: lock prefix (e.g., "lock:{transactions}:balance-sync:")
-- ARGV[2..N]: members to remove

local scheduleKey = KEYS[1]
local lockPrefix = ARGV[1]
local removed = 0

for i = 2, #ARGV do
    local member = ARGV[i]

    -- Remove from sorted set
    local zremResult = redis.call('ZREM', scheduleKey, member)
    removed = removed + zremResult

    -- Remove associated lock if exists
    local lockKey = lockPrefix .. member
    redis.call('DEL', lockKey)
end

return removed
