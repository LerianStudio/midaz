-- acquire_account_admin_ownership.lua
-- Take the exclusive administrative ownership of one account (a closing, a
-- balance creation or deletion) only when nothing holds it.
--
-- KEYS[1]: account administrative ownership key
-- ARGV[1]: owner token
--
-- A string key is another exclusive owner and a sorted set holds live seed
-- admissions; either refuses. The owner is written without an expiry: ownership
-- of work whose outcome is unknown is resolved by reconciliation, never by age.
--
-- Returns 1 when the caller now owns the account, 0 when another holder refused
-- it, -1 when the key is in a state no writer produces (another type, or an
-- exclusive owner carrying no token). Nothing is written unless it returns 1.

local key, token = KEYS[1], ARGV[1]

local kind = redis.call('TYPE', key)
if type(kind) == 'table' then kind = kind.ok end

if kind == 'none' then
    redis.call('SET', key, token)

    return 1
end

if kind == 'zset' then
    return 0
end

if kind == 'string' then
    local owner = redis.call('GET', key)
    if string.match(owner, '^%s*$') then
        return -1
    end

    return 0
end

return -1
