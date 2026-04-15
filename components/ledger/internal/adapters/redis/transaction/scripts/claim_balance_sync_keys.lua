-- claim_balance_sync_keys.lua
-- Fetches due balance sync keys from the ZSET schedule with distributed locking.
--
-- The ZSET score is a fractional-second timestamp set by balance_atomic_operation.lua
-- at mutation time (dueAt = seconds + microseconds/1e6). The score serves as an
-- optimistic concurrency token: if a newer mutation overwrites the score via ZADD
-- between claim and removal, remove_balance_sync_keys_batch.lua detects the
-- mismatch and skips ZREM, preserving the entry for the next sync cycle.
--
-- KEYS[1]: schedule sorted set (e.g., "schedule:{transactions}:balance-sync-v2")
-- ARGV[1]: max number of keys to return
-- ARGV[2]: claim lock TTL in seconds (default 600)
-- ARGV[3]: lock key prefix (default "lock:{transactions}:balance-sync:")

local scheduleKey = KEYS[1]
if not scheduleKey then
  return {}
end

local t = redis.call('TIME')
local now = tonumber(t[1]) + tonumber(t[2]) / 1000000
local limit = tonumber(ARGV[1]) or 25
local claimTTL = tonumber(ARGV[2]) or 600
local claimPrefix = ARGV[3] or "lock:{transactions}:balance-sync:"

-- Return due members up to 'now' that we can claim (SET NX EX).
-- Returns alternating [member, score, member, score, ...] so the caller
-- can later do a conditional ZREM (only remove if score hasn't changed).
local due = redis.call('ZRANGEBYSCORE', scheduleKey, '-inf', now, 'WITHSCORES', 'LIMIT', 0, limit)
local claimed = {}

-- 'due' is now [member1, score1, member2, score2, ...]
for i = 1, #due, 2 do
  local member = due[i]
  local score  = due[i + 1]
  local lockKey = claimPrefix .. member
  local ok = redis.call('SET', lockKey, '1', 'NX', 'EX', claimTTL)
  if ok then
    table.insert(claimed, member)
    table.insert(claimed, score)
  end
end

return claimed