local scheduleKey = KEYS[1]
local now = tonumber(ARGV[1])
local limit = tonumber(ARGV[2]) or 25
local claimTTL = tonumber(ARGV[3]) or 600
local claimPrefix = ARGV[4] or "lock:{transactions}:balance-sync:"

-- Return due members up to 'now' that we can claim (SET NX EX)
local due = redis.call('ZRANGEBYSCORE', scheduleKey, '-inf', now, 'LIMIT', 0, limit)
local claimed = {}

for i = 1, #due do
  local member = due[i]
  local lockKey = claimPrefix .. member
  local ok = redis.call('SET', lockKey, '1', 'NX', 'EX', claimTTL)
  if ok then
    table.insert(claimed, member)
  end
end

return claimed


