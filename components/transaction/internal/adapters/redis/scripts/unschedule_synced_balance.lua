local scheduleKey = KEYS[1]
local member = ARGV[1]
local claimPrefix = ARGV[2] or "lock:{transactions}:balance-sync:"

-- Remove the provided member from the schedule sorted set
local removed = redis.call('ZREM', scheduleKey, member)

-- Best-effort clean up: release lock if it exists
local lockKey = claimPrefix .. member
redis.call('DEL', lockKey)

return removed


