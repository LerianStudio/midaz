-- release_account_seed_admission.lua
-- Remove the caller's own seed admission from one account.
--
-- KEYS[1]: account administrative ownership key
-- ARGV[1]: admission token
--
-- Only the caller's member is removed, so a late release never drops another
-- admission. Redis deletes a sorted set when its last member goes, so an account
-- with no live admission owns no key. An exclusive owner is never touched: it is
-- the other mode, released by its own owner.
--
-- Returns 1 when the member was removed, 0 when it was not there, -1 when the key
-- is of a type no writer produces.

local key, token = KEYS[1], ARGV[1]

local kind = redis.call('TYPE', key)
if type(kind) == 'table' then kind = kind.ok end

if kind == 'zset' then
    return redis.call('ZREM', key, token)
end

if kind == 'none' or kind == 'string' then
    return 0
end

return -1
