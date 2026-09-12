-- Copyright (c) 2026 Lerian Studio. All rights reserved.
-- Use of this source code is governed by the Elastic License 2.0
-- that can be found in the LICENSE file.

if #KEYS ~= 1 or #ARGV ~= 2 then
    return redis.error_reply("BALANCE_LIMIT_INVALID")
end

local raw = redis.call("GET", KEYS[1])
if not raw then
    return 0
end

if raw ~= ARGV[1] then
    return 2
end

-- Go prepares and validates the complete replacement before any repair write.
if raw ~= ARGV[2] then
    redis.call("SET", KEYS[1], ARGV[2], "KEEPTTL")
end

return 1
