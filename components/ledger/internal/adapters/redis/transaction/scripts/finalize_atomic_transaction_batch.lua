local indexTarget = redis.call("GET", KEYS[2])
if not indexTarget then
    return {"missing", ""}
end
if indexTarget ~= KEYS[1] then
    return {"index_conflict", ""}
end

local current = redis.call("GET", KEYS[1])
if not current then
    return {"missing", ""}
end

local currentDecoded, currentRecord = pcall(cjson.decode, current)
local nextDecoded, nextRecord = pcall(cjson.decode, ARGV[3])
if not currentDecoded or type(currentRecord) ~= "table" or
   not nextDecoded or type(nextRecord) ~= "table" or
   currentRecord.formatVersion ~= 1 or nextRecord.formatVersion ~= 1 or
   type(currentRecord.state) ~= "string" or type(nextRecord.state) ~= "string" or
   type(currentRecord.ownerToken) ~= "string" or type(nextRecord.ownerToken) ~= "string" or
   type(currentRecord.requestFingerprint) ~= "string" or type(nextRecord.requestFingerprint) ~= "string" or
   type(currentRecord.batchId) ~= "string" or type(nextRecord.batchId) ~= "string" or
   type(currentRecord.executionId) ~= "string" or type(nextRecord.executionId) ~= "string" or
   type(currentRecord.transactionIds) ~= "table" or type(nextRecord.transactionIds) ~= "table" then
    return redis.error_reply("ATOMIC_BATCH_IDEMPOTENCY_INVALID")
end

if currentRecord.ownerToken ~= ARGV[1] then
    return {"stale_owner", current}
end
if currentRecord.executionId ~= ARGV[2] then
    return {"execution_conflict", current}
end

if currentRecord.state == "complete" then
    return {"already_complete", current}
end
if currentRecord.state ~= "applied" or nextRecord.state ~= "complete" then
    return {"state_conflict", current}
end

if currentRecord.ownerToken ~= nextRecord.ownerToken or
   currentRecord.requestFingerprint ~= nextRecord.requestFingerprint or
   currentRecord.batchId ~= nextRecord.batchId or
   currentRecord.executionId ~= nextRecord.executionId then
    return redis.error_reply("ATOMIC_BATCH_IDEMPOTENCY_IDENTITY_CHANGED")
end
if cjson.encode(currentRecord.transactionIds) ~= cjson.encode(nextRecord.transactionIds) then
    return redis.error_reply("ATOMIC_BATCH_IDEMPOTENCY_TRANSACTIONS_CHANGED")
end

local replayTTL = tonumber(ARGV[4])
if not replayTTL or replayTTL <= 0 then
    return redis.error_reply("ATOMIC_BATCH_IDEMPOTENCY_TTL_INVALID")
end

redis.call("SET", KEYS[1], ARGV[3], "EX", replayTTL)
redis.call("SET", KEYS[2], KEYS[1], "EX", replayTTL)
return {"completed", ARGV[3]}
