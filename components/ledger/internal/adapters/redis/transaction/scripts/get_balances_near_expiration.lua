local scheduleKey = KEYS[1]
if not scheduleKey then
  return {}
end

local t = redis.call('TIME')
local now = tonumber(t[1]) * 1000000 + tonumber(t[2])
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


