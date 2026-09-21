if #KEYS ~= 3 or #ARGV ~= 6 then
    return redis.error_reply("ATOMIC_BATCH_IDEMPOTENCY_ARGUMENTS_INVALID")
end

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
   (currentRecord.formatVersion ~= 1 and currentRecord.formatVersion ~= 2) or
   (nextRecord.formatVersion ~= 1 and nextRecord.formatVersion ~= 2) or
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
if currentRecord.formatVersion ~= nextRecord.formatVersion then
    return redis.error_reply("ATOMIC_BATCH_IDEMPOTENCY_FORMAT_CHANGED")
end
if currentRecord.formatVersion == 2 then
    if type(currentRecord.initialResponses) ~= "table" or type(nextRecord.initialResponses) ~= "table" then
        return redis.error_reply("ATOMIC_BATCH_IDEMPOTENCY_INITIAL_RESPONSES_INVALID")
    end
    for _, id in ipairs(currentRecord.transactionIds) do
        if type(currentRecord.initialResponses[id]) ~= "string" or
           currentRecord.initialResponses[id] ~= nextRecord.initialResponses[id] then
            return redis.error_reply("ATOMIC_BATCH_IDEMPOTENCY_INITIAL_RESPONSES_CHANGED")
        end
    end
end
if cjson.encode(currentRecord.transactionIds) ~= cjson.encode(nextRecord.transactionIds) then
    return redis.error_reply("ATOMIC_BATCH_IDEMPOTENCY_TRANSACTIONS_CHANGED")
end

local replayTTL = tonumber(ARGV[4])
if replayTTL == 0 then
    local receiptRaw = redis.call("HGET", KEYS[3], ARGV[2])
    if not receiptRaw then
        return redis.error_reply("ATOMIC_BATCH_IDEMPOTENCY_RECEIPT_MISSING")
    end

    local receiptDecoded, receipt = pcall(cjson.decode, receiptRaw)
    if not receiptDecoded or type(receipt) ~= "table" or
       receipt.formatVersion ~= 1 or receipt.organizationId ~= ARGV[5] or
       receipt.ledgerId ~= ARGV[6] or receipt.executionId ~= ARGV[2] or
       type(receipt.protection) ~= "table" or
       (receipt.protection.formatVersion ~= 1 and receipt.protection.formatVersion ~= 2) or
       type(receipt.protection.retentionSeconds) ~= "number" or
       receipt.protection.retentionSeconds < 1 or receipt.protection.retentionSeconds > 604800 or
       receipt.protection.retentionSeconds % 1 ~= 0 or
       type(receipt.protection.transactions) ~= "table" or
       (receipt.protection.formatVersion == 2 and
        (type(receipt.protection.indexFields) ~= "table" or
         #receipt.protection.indexFields ~= #receipt.protection.transactions)) or
       cjson.encode(receipt.protection.transactions) ~= cjson.encode(currentRecord.transactionIds) then
        return redis.error_reply("ATOMIC_BATCH_IDEMPOTENCY_RECEIPT_INVALID")
    end

    replayTTL = receipt.protection.retentionSeconds
end
if not replayTTL or replayTTL <= 0 then
    return redis.error_reply("ATOMIC_BATCH_IDEMPOTENCY_TTL_INVALID")
end

redis.call("SET", KEYS[1], ARGV[3], "EX", replayTTL)
redis.call("SET", KEYS[2], KEYS[1], "EX", replayTTL)
return {"completed", ARGV[3]}
