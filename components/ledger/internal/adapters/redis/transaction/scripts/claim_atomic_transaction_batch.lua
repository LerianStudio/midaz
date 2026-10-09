local current = redis.call("GET", KEYS[1])

if not current then
    redis.call("SET", KEYS[1], ARGV[1])
    return {"claimed", ARGV[1]}
end

local decoded, record = pcall(cjson.decode, current)
if not decoded or type(record) ~= "table" then
    return redis.error_reply("ATOMIC_BATCH_IDEMPOTENCY_INVALID")
end

if (record.formatVersion ~= 1 and record.formatVersion ~= 2) or
   type(record.requestFingerprint) ~= "string" or
   type(record.state) ~= "string" then
    return redis.error_reply("ATOMIC_BATCH_IDEMPOTENCY_INVALID")
end

-- ARGV[3], when non-empty, is a second fingerprint the stored record may
-- carry for the same request. It only widens the match; ARGV[1] is what a
-- first claim stores.
local legacy = ARGV[3] or ""
if record.requestFingerprint ~= ARGV[2] and (legacy == "" or record.requestFingerprint ~= legacy) then
    return {"fingerprint_conflict", current}
end

if record.state == "complete" then
    return {"replayed", current}
end

if record.state == "claimed" or record.state == "prepared" or record.state == "applied" then
    return {"in_progress", current}
end

return redis.error_reply("ATOMIC_BATCH_IDEMPOTENCY_INVALID")
