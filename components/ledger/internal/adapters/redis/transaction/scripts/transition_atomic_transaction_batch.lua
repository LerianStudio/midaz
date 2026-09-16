local current = redis.call("GET", KEYS[1])
if not current then
    return {"missing", ""}
end

local currentDecoded, currentRecord = pcall(cjson.decode, current)
local nextDecoded, nextRecord = pcall(cjson.decode, ARGV[3])
if not currentDecoded or type(currentRecord) ~= "table" or
   not nextDecoded or type(nextRecord) ~= "table" then
    return redis.error_reply("ATOMIC_BATCH_IDEMPOTENCY_INVALID")
end

if currentRecord.formatVersion ~= 1 or nextRecord.formatVersion ~= 1 or
   type(currentRecord.state) ~= "string" or type(nextRecord.state) ~= "string" or
   type(currentRecord.ownerToken) ~= "string" or type(nextRecord.ownerToken) ~= "string" or
   type(currentRecord.requestFingerprint) ~= "string" or type(nextRecord.requestFingerprint) ~= "string" or
   type(currentRecord.batchId) ~= "string" or type(nextRecord.batchId) ~= "string" then
    return redis.error_reply("ATOMIC_BATCH_IDEMPOTENCY_INVALID")
end

if currentRecord.ownerToken ~= ARGV[1] then
    return {"stale_owner", current}
end

if currentRecord.state ~= ARGV[2] then
    if current == ARGV[3] then
        return {"already_transitioned", current}
    end

    return {"state_conflict", current}
end

if currentRecord.ownerToken ~= nextRecord.ownerToken or
   currentRecord.requestFingerprint ~= nextRecord.requestFingerprint or
   currentRecord.batchId ~= nextRecord.batchId then
    return redis.error_reply("ATOMIC_BATCH_IDEMPOTENCY_IDENTITY_CHANGED")
end

if currentRecord.state == "prepared" or currentRecord.state == "applied" then
    if type(currentRecord.transactionIds) ~= "table" or
       type(nextRecord.transactionIds) ~= "table" or
       cjson.encode(currentRecord.transactionIds) ~= cjson.encode(nextRecord.transactionIds) then
        return redis.error_reply("ATOMIC_BATCH_IDEMPOTENCY_TRANSACTIONS_CHANGED")
    end
end

if currentRecord.executionId ~= nil and currentRecord.executionId ~= cjson.null and
   currentRecord.executionId ~= nextRecord.executionId then
    return redis.error_reply("ATOMIC_BATCH_IDEMPOTENCY_EXECUTION_CHANGED")
end

local allowed =
    (currentRecord.state == "claimed" and nextRecord.state == "prepared") or
    (currentRecord.state == "prepared" and nextRecord.state == "applied") or
    (currentRecord.state == "applied" and nextRecord.state == "complete")
if not allowed then
    return redis.error_reply("ATOMIC_BATCH_IDEMPOTENCY_TRANSITION_INVALID")
end

local replayTTL = tonumber(ARGV[4])
if nextRecord.state == "complete" then
    if not replayTTL or replayTTL <= 0 then
        return redis.error_reply("ATOMIC_BATCH_IDEMPOTENCY_TTL_INVALID")
    end
    redis.call("SET", KEYS[1], ARGV[3], "EX", replayTTL)
else
    if not replayTTL or replayTTL ~= 0 then
        return redis.error_reply("ATOMIC_BATCH_IDEMPOTENCY_TTL_INVALID")
    end
    redis.call("SET", KEYS[1], ARGV[3])
end

return {"updated", ARGV[3]}
