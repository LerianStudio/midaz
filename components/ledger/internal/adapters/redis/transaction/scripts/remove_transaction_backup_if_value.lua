-- KEYS[1] = backup queue hash key
-- ARGV[1] = hash field (tenant-prefixed transaction key)
-- ARGV[2] = expected exact raw value
--
-- The hash field is data, not a Redis key: passing it in KEYS would make
-- Redis Cluster hash both names into slots and reject the call with
-- CROSSSLOT whenever they disagree.
local raw = redis.call("HGET", KEYS[1], ARGV[1])
if not raw or raw ~= ARGV[2] then
    return 0
end

return redis.call("HDEL", KEYS[1], ARGV[1])
