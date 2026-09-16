local current = redis.call("GET", KEYS[1])
if not current then
    return {"missing", ""}
end

local decoded, record = pcall(cjson.decode, current)
if not decoded or type(record) ~= "table" or
   record.formatVersion ~= 1 or
   type(record.state) ~= "string" or
   type(record.ownerToken) ~= "string" then
    return redis.error_reply("ATOMIC_BATCH_IDEMPOTENCY_INVALID")
end

if record.ownerToken ~= ARGV[1] then
    return {"stale_owner", current}
end

if record.state ~= "claimed" and record.state ~= "prepared" then
    return {"protected", current}
end

if record.executionId ~= nil and record.executionId ~= cjson.null then
    return {"protected", current}
end

if ARGV[2] == "1" then
    return {"protected", current}
end

redis.call("DEL", KEYS[1])
return {"deleted", current}
