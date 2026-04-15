-- claim_balance_sync_keys.lua
-- Fetches due balance sync keys from the ZSET schedule with distributed locking.
--
-- The ZSET score is a fractional-second timestamp set by balance_atomic_operation.lua
-- at mutation time (dueAt = seconds + microseconds/1e6). The score serves as an
-- optimistic concurrency token: if a newer mutation overwrites the score via ZADD
-- between claim and removal, remove_balance_sync_keys_batch.lua detects the
-- mismatch and skips ZREM, preserving the entry for the next sync cycle.
--
-- Legacy migration: scores > 1e12 are from v3.6.0 (pure microsecond format,
-- seconds * 1e6 + microseconds). These are normalized to fractional seconds
-- by dividing by 1e6, making them eligible on the next poll cycle.
--
-- KEYS[1]: schedule sorted set (e.g., "schedule:{transactions}:balance-sync")
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

-- Normalize legacy microsecond scores from v3.6.0.
-- Scores > 1e12 are unambiguously microseconds (seconds won't reach 1e12 until
-- year ~33700). Convert them to fractional seconds so they become eligible.
-- This runs atomically within this Lua script — no race with ZADD in
-- balance_atomic_operation.lua (which now writes fractional seconds).
local legacy = redis.call('ZRANGEBYSCORE', scheduleKey, '1000000000000', '+inf', 'WITHSCORES', 'LIMIT', 0, limit)
for i = 1, #legacy, 2 do
  redis.call('ZADD', scheduleKey, tonumber(legacy[i + 1]) / 1000000, legacy[i])
end

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