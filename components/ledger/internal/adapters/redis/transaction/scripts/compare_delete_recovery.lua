-- Copyright (c) 2026 Lerian Studio. All rights reserved.
-- Use of this source code is governed by the Elastic License 2.0
-- that can be found in the LICENSE file.

-- KEYS: tenant-scoped backup and attempt hashes.
-- ARGV: raw transaction:execution field, exact envelope, legacy attempt field.
if #KEYS ~= 2 or #ARGV ~= 3 then
    return redis.error_reply("ERR invalid recovery acknowledgement arguments")
end

for _, key in ipairs(KEYS) do
    local kind = redis.call("TYPE", key).ok
    if kind ~= "none" and kind ~= "hash" then
        return redis.error_reply("WRONGTYPE recovery acknowledgement requires hashes")
    end
end

local current = redis.call("HGET", KEYS[1], ARGV[1])
if not current then return 0 end
if current ~= ARGV[2] then return 2 end

redis.call("HDEL", KEYS[1], ARGV[1])
redis.call("HDEL", KEYS[2], ARGV[3])
return 1
