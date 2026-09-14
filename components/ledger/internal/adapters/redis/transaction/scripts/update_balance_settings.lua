-- update_balance_settings.lua
-- Conditionally replaces a cached balance with a settings-patched dual-format
-- blob prepared losslessly by Go. Comparing the exact observed bytes prevents
-- the PATCH from overwriting a concurrent transaction mutation.
--
-- KEYS[1] = balance key (already tenant-prefixed by Go)
-- ARGV[1] = exact bytes observed by Go
-- ARGV[2] = replacement bytes prepared by Go
-- ARGV[3] = TTL in seconds (the shared balance snapshot lifetime). The
-- command-layer delete marker is intentionally longer than this lifetime so a
-- failed post-commit eviction cannot leave a deleted balance mutable after the
-- marker expires.
--
-- Returns:
--    1  = written
--    0  = key absent (no-op; the next transaction reloads from PostgreSQL)
--    2  = value changed since Go observed it (caller may retry)

-- Delete markers use a dedicated top-level namespace in new releases. Honor the legacy
-- :deleted key during one rolling-deploy release as well; new writers dual-write both markers
-- so old Lua remains safe while the fleet is mixed. The legacy suffix has a known collision
-- tradeoff and must be removed after all old binaries are retired.
local deleteMarkerNamespacePrefix = "balance_delete_marker:{transactions}:"
local balanceNamespacePrefix = "balance:{transactions}:"
local markerKey, replacements = string.gsub(KEYS[1], balanceNamespacePrefix, deleteMarkerNamespacePrefix, 1)
if replacements == 0 then
    markerKey = deleteMarkerNamespacePrefix .. KEYS[1]
end

if redis.call('EXISTS', markerKey) == 1 or redis.call('EXISTS', KEYS[1] .. ':deleted') == 1 then
    return redis.error_reply("0019")
end

local cur = redis.call('GET', KEYS[1])

if not cur then
    return 0
end

if cur ~= ARGV[1] then
    return 2
end

redis.call('SET', KEYS[1], ARGV[2], 'EX', tonumber(ARGV[3]))

return 1
