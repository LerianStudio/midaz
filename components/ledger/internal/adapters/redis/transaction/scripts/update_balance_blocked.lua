-- update_balance_blocked.lua
-- Rewrites the account-level Blocked flag in place on every cached balance
-- blob of an account in one atomic step. Redis serializes EVAL execution, so
-- this can never interleave with a concurrent balance_atomic_operation.lua
-- debit/credit on the same keys — there is no GET-then-SET window to race.
--
-- All keys share the {transactions} hash tag, so a multi-key EVAL is legal in
-- cluster mode. An absent key is skipped: the on-demand hydration fills the
-- next cache miss with the freshly persisted PostgreSQL value. The blob is
-- mutated via the same cjson.decode -> mutate -> cjson.encode round trip the
-- atomic script performs, preserving live transactional state
-- (Available, OnHold, Version, OverdraftUsed) that may be ahead of PostgreSQL.
-- Never DEL: deleting would discard those pending write-behind deltas.
--
-- KEYS[1..N] = balance keys of the account (already tenant-prefixed by Go)
-- ARGV[1]    = Blocked (0/1)
-- ARGV[2]    = TTL in seconds
--
-- Returns { written, corrupt }:
--   written = keys whose blob was rewritten
--   corrupt = keys holding a non-JSON value, left untouched (a corrupt blob
--             cannot approve a transaction anyway — the atomic script fails
--             decoding it — so skipping is fail-safe; Go logs the count)

local blocked = tonumber(ARGV[1])
local ttl = tonumber(ARGV[2])

local written = 0
local corrupt = 0

for i = 1, #KEYS do
    local cur = redis.call('GET', KEYS[i])

    if cur then
        local ok, balance = pcall(cjson.decode, cur)

        if ok and type(balance) == 'table' then
            -- Drop any legacy camelCase alias a non-contract writer may have
            -- left behind, so the cache carries a single authoritative key.
            balance.blocked = nil

            balance.Blocked = blocked

            redis.call('SET', KEYS[i], cjson.encode(balance), 'EX', ttl)

            written = written + 1
        else
            corrupt = corrupt + 1
        end
    end
end

return { written, corrupt }
