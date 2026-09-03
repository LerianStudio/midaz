-- set_account_blocked_batch.lua
-- Flips ONLY the AccountBlocked field on every cached balance JSON blob named in
-- KEYS, in one atomic EVAL. Redis serializes EVAL execution, so this can never
-- interleave with a concurrent balance_atomic_operation.lua debit/credit on the
-- same key -- there is no GET-then-SET window to race.
--
-- Mirrors the cjson.decode -> mutate one field -> cjson.encode round trip that
-- scripts/update_balance_settings.lua performs for the settings-only PATCH, so
-- the live transactional state the atomic script owns (Available, OnHold,
-- Version, OverdraftUsed) is preserved verbatim: this script never reads or
-- writes those fields. The backup queue and the sync schedule are separate keys
-- and are never touched here.
--
-- KEYS[1..N] = balance keys (already tenant-prefixed by Go); all share the
--              {transactions} hash tag, so they resolve to one Redis Cluster slot.
-- ARGV[1]    = AccountBlocked (0/1)
-- ARGV[2]    = TTL in seconds
--
-- Two-pass, all-or-nothing: every present key is decoded first; a single corrupt
-- blob aborts the whole batch with -2 BEFORE any SET, so a partial flip can never
-- be observed. A key that is absent is skipped in both passes (no-op; the next
-- transaction reloads it from PostgreSQL, carrying the freshly-propagated flag).
--
-- Returns:
--   >= 0 = number of keys written
--   -2   = a cached value was not valid JSON (corrupt blob); nothing was written

local blocked = tonumber(ARGV[1])
local ttl = tonumber(ARGV[2])

-- Pass 1: GET + decode every present key. Abort on the first corrupt blob so no
-- SET runs (all-or-nothing).
local decoded = {}
local present = {}

for i = 1, #KEYS do
    local cur = redis.call('GET', KEYS[i])

    if cur then
        local ok, balance = pcall(cjson.decode, cur)

        if not ok then
            return -2
        end

        decoded[i] = balance
        present[#present + 1] = i
    end
end

-- Pass 2: flip ONLY AccountBlocked and re-encode each present key.
for _, i in ipairs(present) do
    local balance = decoded[i]

    -- Drop every legacy camelCase alias a pre-fix writer may have left behind,
    -- so the cache carries a single authoritative key the atomic script reads
    -- back as balance.AccountBlocked.
    balance.accountBlocked = nil
    balance.accountblocked = nil

    balance.AccountBlocked = blocked

    redis.call('SET', KEYS[i], cjson.encode(balance), 'EX', ttl)
end

return #present
