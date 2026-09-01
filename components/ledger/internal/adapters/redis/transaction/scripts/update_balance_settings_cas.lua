-- update_balance_settings_cas.lua
-- Compare-and-swap rewrite of a cached balance JSON blob, used by
-- UpdateBalanceCacheSettings to apply a settings-only PATCH without clobbering
-- a concurrent write from balance_atomic_operation.lua.
--
-- The comparison is against the RAW value Go read via GET (ARGV[1]), not just
-- a version field: any concurrent mutation between that GET and this EVAL —
-- whichever field it touched — invalidates the CAS, and Go retries by
-- re-reading and re-applying the settings mutation on top of the fresh value.
-- Comparing full values also means this script never has to cjson.decode the
-- blob, so it carries zero risk of numeric-encoding drift versus the Go/Lua
-- boundary.
--
-- KEYS[1] = balance key (already tenant-prefixed by Go)
-- ARGV[1] = raw value Go read via GET (expected current value)
-- ARGV[2] = new JSON blob (marshaled in Go, Lua-native CamelCase keys)
-- ARGV[3] = TTL in seconds
--
-- Returns:
--    1  = written
--    0  = key absent (no-op; the next transaction reloads from PostgreSQL)
--   -1  = conflict: the value changed since Go's GET

local cur = redis.call('GET', KEYS[1])

if not cur then
    return 0
end

if cur ~= ARGV[1] then
    return -1
end

redis.call('SET', KEYS[1], ARGV[2], 'EX', ARGV[3])

return 1
