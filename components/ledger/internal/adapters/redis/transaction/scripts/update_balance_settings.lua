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
-- ARGV[5] = TTL in seconds (86400, the balance snapshot lifetime). The
-- command-layer delete marker is intentionally longer than this lifetime so a
-- failed post-commit eviction cannot leave a deleted balance mutable after the
-- marker expires.
--
-- Returns:
--    1  = written
--    0  = key absent (no-op; the next transaction reloads from PostgreSQL)
--   -2  = cached value was not valid JSON (corrupt blob)

-- Delete markers use a dedicated top-level namespace in new releases. Honor the legacy
-- :deleted key during one rolling-deploy release as well; new writers dual-write both markers
-- so old Lua remains safe while the fleet is mixed. The legacy suffix has a known collision
-- tradeoff and must be removed after all old binaries are retired.
local deleteMarkerNamespacePrefix = "balance_delete_marker:{transactions}:"
local balanceNamespacePrefix = "balance:{transactions}:"
local markerKey, replacements = string.gsub(KEYS[1], balanceNamespacePrefix, deleteMarkerNamespacePrefix, 1)
if replacements == 0 then
    markerKey = deleteMarkerNamespacePrefix .. KEYS[1]
end

if redis.call('EXISTS', markerKey) == 1 or redis.call('EXISTS', KEYS[1] .. ':deleted') == 1 then
    return redis.error_reply("0019")
end

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
