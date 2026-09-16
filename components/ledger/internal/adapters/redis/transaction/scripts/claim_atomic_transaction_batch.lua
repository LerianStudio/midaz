local current = redis.call("GET", KEYS[1])

if not current then
    redis.call("SET", KEYS[1], ARGV[1])
    return {"claimed", ARGV[1]}
end

local decoded, record = pcall(cjson.decode, current)
if not decoded or type(record) ~= "table" then
    return redis.error_reply("ATOMIC_BATCH_IDEMPOTENCY_INVALID")
end

if record.formatVersion ~= 1 or
   type(record.requestFingerprint) ~= "string" or
   type(record.state) ~= "string" then
    return redis.error_reply("ATOMIC_BATCH_IDEMPOTENCY_INVALID")
end

if record.requestFingerprint ~= ARGV[2] then
    return {"fingerprint_conflict", current}
end

if record.state == "complete" then
    return {"replayed", current}
end

if record.state == "claimed" or record.state == "prepared" or record.state == "applied" then
    return {"in_progress", current}
end

return redis.error_reply("ATOMIC_BATCH_IDEMPOTENCY_INVALID")
