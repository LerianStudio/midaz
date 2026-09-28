-- Copyright (c) 2026 Lerian Studio. All rights reserved.
-- Use of this source code is governed by the Elastic License 2.0
-- that can be found in the LICENSE file.

-- KEYS: tenant-scoped selected recovery hash and optional legacy attempt hash.
-- ARGV: raw transaction:execution field, exact envelope, legacy attempt field,
--       clear legacy attempt flag (0|1).
if #KEYS ~= 2 or #ARGV ~= 4 then
    return redis.error_reply("ERR invalid recovery acknowledgement arguments")
end

if ARGV[4] ~= "0" and ARGV[4] ~= "1" then
    return redis.error_reply("ERR invalid recovery acknowledgement source")
end

local recoveryKind = redis.call("TYPE", KEYS[1]).ok
if recoveryKind ~= "none" and recoveryKind ~= "hash" then
    return redis.error_reply("WRONGTYPE recovery acknowledgement requires a hash")
end
if ARGV[4] == "1" then
    local attemptsKind = redis.call("TYPE", KEYS[2]).ok
    if attemptsKind ~= "none" and attemptsKind ~= "hash" then
        return redis.error_reply("WRONGTYPE recovery acknowledgement requires an attempt hash")
    end
end

local current = redis.call("HGET", KEYS[1], ARGV[1])
if not current then return 0 end
if current ~= ARGV[2] then return 2 end

redis.call("HDEL", KEYS[1], ARGV[1])
if ARGV[4] == "1" then redis.call("HDEL", KEYS[2], ARGV[3]) end
return 1
