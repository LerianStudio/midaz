-- A batch create carries no lifecycleAction key; absent and null both mean one.
local function lifecycleAction(record)
    local value = record.lifecycleAction
    if value == nil or value == cjson.null then return "" end
    return value
end

local current = redis.call("GET", KEYS[1])
if not current then
    return {"missing", ""}
end

local currentDecoded, currentRecord = pcall(cjson.decode, current)
local nextDecoded, nextRecord = pcall(cjson.decode, ARGV[2])
if not currentDecoded or type(currentRecord) ~= "table" or
   not nextDecoded or type(nextRecord) ~= "table" then
    return redis.error_reply("ATOMIC_BATCH_IDEMPOTENCY_INVALID")
end

if (currentRecord.formatVersion ~= 1 and currentRecord.formatVersion ~= 2) or
   (nextRecord.formatVersion ~= 1 and nextRecord.formatVersion ~= 2) or
   type(currentRecord.state) ~= "string" or type(nextRecord.state) ~= "string" or
   type(currentRecord.ownerToken) ~= "string" or type(nextRecord.ownerToken) ~= "string" or
   type(currentRecord.requestFingerprint) ~= "string" or type(nextRecord.requestFingerprint) ~= "string" or
   type(currentRecord.batchId) ~= "string" or type(nextRecord.batchId) ~= "string" or
   type(currentRecord.transactionIds) ~= "table" or type(nextRecord.transactionIds) ~= "table" or
   type(nextRecord.executionId) ~= "string" then
    return redis.error_reply("ATOMIC_BATCH_IDEMPOTENCY_INVALID")
end

if currentRecord.ownerToken ~= ARGV[1] then
    return {"stale_owner", current}
end

local indexTarget = redis.call("GET", KEYS[2])
if currentRecord.state ~= "prepared" then
    if currentRecord.state == "applied" and current == ARGV[2] and indexTarget == KEYS[1] then
        return {"already_transitioned", current}
    end

    return {"state_conflict", current}
end

if indexTarget then
    return {"index_conflict", current}
end

if nextRecord.state ~= "applied" or nextRecord.executionId ~= ARGV[3] then
    return redis.error_reply("ATOMIC_BATCH_IDEMPOTENCY_TRANSITION_INVALID")
end

if currentRecord.ownerToken ~= nextRecord.ownerToken or
   currentRecord.requestFingerprint ~= nextRecord.requestFingerprint or
   currentRecord.batchId ~= nextRecord.batchId or
   lifecycleAction(currentRecord) ~= lifecycleAction(nextRecord) then
    return redis.error_reply("ATOMIC_BATCH_IDEMPOTENCY_IDENTITY_CHANGED")
end

if currentRecord.formatVersion ~= nextRecord.formatVersion then
    return redis.error_reply("ATOMIC_BATCH_IDEMPOTENCY_FORMAT_CHANGED")
end

if cjson.encode(currentRecord.transactionIds) ~= cjson.encode(nextRecord.transactionIds) then
    return redis.error_reply("ATOMIC_BATCH_IDEMPOTENCY_TRANSACTIONS_CHANGED")
end

if currentRecord.executionId ~= nil and currentRecord.executionId ~= cjson.null then
    return redis.error_reply("ATOMIC_BATCH_IDEMPOTENCY_EXECUTION_CHANGED")
end

redis.call("SET", KEYS[1], ARGV[2])
redis.call("SET", KEYS[2], KEYS[1])
return {"updated", ARGV[2]}
