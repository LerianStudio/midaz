-- release_account_admin_ownership.lua
-- Remove the exclusive administrative ownership of one account only when it still
-- carries the caller's token.
--
-- KEYS[1]: account administrative ownership key
-- ARGV[1]: owner token
--
-- The compare-and-delete keeps a release that arrives late from removing an
-- ownership another operation took afterwards. A key holding seed admissions is
-- the other mode and is never touched.
--
-- Returns 1 when the ownership was removed, 0 when it was missing or held by
-- someone else, -1 when the key is of a type no writer produces.

local key, token = KEYS[1], ARGV[1]

local kind = redis.call('TYPE', key)
if type(kind) == 'table' then kind = kind.ok end

if kind == 'string' then
    if redis.call('GET', key) == token then
        return redis.call('DEL', key)
    end

    return 0
end

if kind == 'none' or kind == 'zset' then
    return 0
end

return -1
