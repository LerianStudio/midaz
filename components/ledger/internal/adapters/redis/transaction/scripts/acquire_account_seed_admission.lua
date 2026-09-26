-- acquire_account_seed_admission.lua
-- Admit one cache-miss balance seed on an account, alongside every other live
-- seed admission, unless an exclusive administrative owner holds the account.
--
-- KEYS[1]: account administrative ownership key
-- ARGV[1]: admission token
--
-- The ownership key has two modes, told apart by its Redis type:
--   string -> one exclusive owner (a closing, a balance creation or deletion);
--   zset   -> the live seed admissions: member = token, score = acquisition
--             instant in milliseconds on the server clock.
-- Seed admissions coexist with one another and never with an exclusive owner.
-- Checking the type and writing the member in one script is what keeps an
-- exclusive acquisition from landing between the two.
--
-- ZADD NX keeps the first instant of a token admitted twice, so the score always
-- says how long the oldest unresolved admission has been waiting.
--
-- Returns 1 when the token is a live admission, 0 when an exclusive owner holds
-- the account, -1 when the key is in a state no writer produces (another type, or
-- an exclusive owner carrying no token). Nothing is written unless it returns 1.

local key, token = KEYS[1], ARGV[1]

local kind = redis.call('TYPE', key)
if type(kind) == 'table' then kind = kind.ok end

if kind == 'string' then
    local owner = redis.call('GET', key)
    if string.match(owner, '^%s*$') then
        return -1
    end

    return 0
end

if kind ~= 'none' and kind ~= 'zset' then
    return -1
end

local now = redis.call('TIME')
local acquiredAtMillis = now[1] .. string.format('%03d', math.floor(tonumber(now[2]) / 1000))

redis.call('ZADD', key, 'NX', acquiredAtMillis, token)

return 1
