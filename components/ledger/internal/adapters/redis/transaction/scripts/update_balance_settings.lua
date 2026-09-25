-- update_balance_settings.lua
-- Applies a settings-only PATCH to a cached balance JSON blob in one atomic
-- step. Redis serializes EVAL execution, so this can never interleave with a
-- concurrent balance_atomic_operation.lua debit/credit on the same key —
-- there is no GET-then-SET window to race.
--
-- Mirrors the same cjson.decode -> mutate -> cjson.encode round trip
-- balance_atomic_operation.lua already performs on every transaction (see
-- that script's NX-seed and post-mutation SET), so encoding parity with the
-- Lua-native cache format is inherent rather than something Go has to
-- replicate field by field.
--
-- KEYS[1] = balance key (already tenant-prefixed by Go)
-- ARGV[1] = AllowOverdraft (0/1)
-- ARGV[2] = OverdraftLimitEnabled (0/1)
-- ARGV[3] = OverdraftLimit (string; "0" when disabled/unset)
-- ARGV[4] = BalanceScope (string)
-- ARGV[5] = TTL in seconds
--
-- Returns:
--    1  = written
--    0  = key absent (no-op; the next transaction reloads from PostgreSQL)
--   -2  = cached value was not valid JSON (corrupt blob)

local cur = redis.call('GET', KEYS[1])

if not cur then
    return 0
end

local ok, balance = pcall(cjson.decode, cur)

if not ok then
    return -2
end

-- Drop every legacy camelCase alias a pre-fix writer may have left behind,
-- so the cache carries a single authoritative key per field.
balance.allowOverdraft = nil
balance.allowoverdraft = nil
balance.overdraftLimitEnabled = nil
balance.overdraftlimitenabled = nil
balance.overdraftLimit = nil
balance.overdraftlimit = nil
balance.balanceScope = nil
balance.balancescope = nil

balance.AllowOverdraft = tonumber(ARGV[1])
balance.OverdraftLimitEnabled = tonumber(ARGV[2])
balance.OverdraftLimit = ARGV[3]
balance.BalanceScope = ARGV[4]

redis.call('SET', KEYS[1], cjson.encode(balance), 'EX', tonumber(ARGV[5]))

return 1
