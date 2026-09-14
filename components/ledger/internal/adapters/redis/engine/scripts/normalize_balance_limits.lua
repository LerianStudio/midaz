-- Copyright (c) 2026 Lerian Studio. All rights reserved.
-- Use of this source code is governed by the Elastic License 2.0
-- that can be found in the LICENSE file.

if #KEYS == 0 or #ARGV ~= 2 * #KEYS then
    return redis.error_reply("invalid balance limit normalization arguments")
end

for i, key in ipairs(KEYS) do
    local keyType = redis.call("TYPE", key)
    if type(keyType) == "table" then keyType = keyType.ok end
    if keyType == "none" then return -1 end
    if keyType ~= "string" then return -2 end

    local current = redis.call("GET", key)
    if not current then return -1 end
    if current ~= ARGV[2 * i - 1] then return 0 end
end

for i, key in ipairs(KEYS) do
    local expected = ARGV[2 * i - 1]
    local replacement = ARGV[2 * i]
    if replacement ~= expected then
        redis.call("SET", key, replacement, "KEEPTTL")
    end
end

return 1
