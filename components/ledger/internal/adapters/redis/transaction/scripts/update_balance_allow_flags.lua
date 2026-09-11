-- update_balance_allow_flags.lua
-- Applies an allow-flags PATCH (AllowSending / AllowReceiving) to a cached
-- balance JSON blob in one atomic step. Redis serializes EVAL execution, so
-- this can never interleave with a concurrent balance_atomic_operation.lua
-- debit/credit on the same key — there is no GET-then-SET window to race.
--
-- The blob is mutated via the same cjson.decode -> mutate -> cjson.encode
-- round trip balance_atomic_operation.lua performs on every transaction,
-- preserving live transactional state (Available, OnHold, Version,
-- OverdraftUsed) that may be ahead of PostgreSQL. Never DEL: deleting would
-- discard those pending write-behind deltas.
--
-- Each flag is tri-state so a PATCH carrying only one of them leaves the
-- other exactly as it is on the blob:
--   -1 = keep (field untouched, aliases included)
--    0 = false
--    1 = true
--
-- KEYS[1] = balance key (already tenant-prefixed by Go)
-- ARGV[1] = AllowSending tri-state (-1/0/1)
-- ARGV[2] = AllowReceiving tri-state (-1/0/1)
-- ARGV[3] = TTL in seconds
--
-- Returns:
--    1  = written
--    0  = key absent (no-op; the next transaction reloads from PostgreSQL)
--   -2  = cached value was not valid JSON (corrupt blob)

local allowSending = tonumber(ARGV[1])
local allowReceiving = tonumber(ARGV[2])
local ttl = tonumber(ARGV[3])

local cur = redis.call('GET', KEYS[1])

if not cur then
    return 0
end

local ok, balance = pcall(cjson.decode, cur)

if not ok then
    return -2
end

-- A flag being written drops its legacy camelCase aliases first, so the blob
-- carries a single authoritative key. A flag being kept is left completely
-- alone: dropping an alias without writing a replacement would erase the only
-- copy a legacy Go-marshalled blob has. Leaving it is harmless — Go unmarshals
-- case-insensitively and the atomic script never reads the allow flags.
if allowSending ~= -1 then
    balance.allowSending = nil
    balance.allowsending = nil

    balance.AllowSending = allowSending
end

if allowReceiving ~= -1 then
    balance.allowReceiving = nil
    balance.allowreceiving = nil

    balance.AllowReceiving = allowReceiving
end

redis.call('SET', KEYS[1], cjson.encode(balance), 'EX', ttl)

return 1
