-- write_config_key.lua
-- Writes one balance or account configuration key when the new value is newer than,
-- or describes another row than, the stored one. Go prepares the canonical value; this
-- script only compares identity and version, so the comparison and the write are one
-- atomic step and two writers can never both believe they won with different values.
--
-- KEYS[1] = configuration key (already tenant-prefixed by Go)
-- ARGV[1] = canonical value prepared by Go
-- ARGV[2] = its configVersion, canonical decimal text
-- ARGV[3] = name of the identity field in the value ("balanceId" or "accountId")
-- ARGV[4] = identity the value describes
-- ARGV[5] = TTL in seconds, applied only when the value is written
--
-- Returns {status, stored value after the call}:
--    1 = written: the key was absent, unreadable, of another identity or older
--    0 = same version already stored: nothing written, the expiry untouched; Go
--        compares the content
--    2 = a newer version is stored: nothing written

local function canonicalVersion(value)
    return type(value) == "string" and (value == "0" or string.match(value, "^[1-9]%d*$") ~= nil)
end

-- Canonical non-negative integers compare by length first, then digit by digit, which
-- is exact for any int64 where a Lua number would round.
local function compareVersions(left, right)
    if #left ~= #right then
        return #left < #right and -1 or 1
    end

    if left == right then
        return 0
    end

    return left < right and -1 or 1
end

local stored = redis.call('GET', KEYS[1])

if stored then
    local ok, decoded = pcall(cjson.decode, stored)

    if ok and type(decoded) == "table" and decoded[ARGV[3]] == ARGV[4] and canonicalVersion(decoded.configVersion) then
        local order = compareVersions(ARGV[2], decoded.configVersion)

        if order < 0 then
            return {2, stored}
        end

        if order == 0 then
            return {0, stored}
        end
    end
end

redis.call('SET', KEYS[1], ARGV[1], 'EX', tonumber(ARGV[5]))

return {1, ARGV[1]}
