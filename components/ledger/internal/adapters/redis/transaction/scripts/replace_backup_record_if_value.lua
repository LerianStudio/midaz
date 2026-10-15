-- Copyright (c) 2026 Lerian Studio. All rights reserved.
-- Use of this source code is governed by the Elastic License 2.0
-- that can be found in the LICENSE file.

-- Replaces one legacy backup record only while it still holds the exact value
-- the caller read, so a record rewritten or removed since then is left as is.
--
-- KEYS: tenant-scoped legacy backup hash.
-- ARGV: physical field exactly as read, expected record, replacement record.
-- Returns 0 when the field is absent, 1 when replaced, 2 when it holds another value.
if #KEYS ~= 1 or #ARGV ~= 3 then
    return redis.error_reply("ERR invalid backup record replacement arguments")
end

local kind = redis.call("TYPE", KEYS[1]).ok
if kind ~= "none" and kind ~= "hash" then
    return redis.error_reply("WRONGTYPE backup record replacement requires a hash")
end

local current = redis.call("HGET", KEYS[1], ARGV[1])
if not current then return 0 end
if current ~= ARGV[2] then return 2 end

redis.call("HSET", KEYS[1], ARGV[1], ARGV[3])
return 1
