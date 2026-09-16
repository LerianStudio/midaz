local expectedOwner = ARGV[1]
local expectedExecution = ARGV[2]
local decodedIDs, expectedTransactionIDs = pcall(cjson.decode, ARGV[3])

if not decodedIDs or type(expectedTransactionIDs) ~= "table" or #expectedTransactionIDs == 0 then
    return redis.error_reply("ATOMIC_BATCH_REFUSAL_ABORT_INVALID_TRANSACTIONS")
end

for _, transactionID in ipairs(expectedTransactionIDs) do
    if type(transactionID) ~= "string" or transactionID == "" then
        return redis.error_reply("ATOMIC_BATCH_REFUSAL_ABORT_INVALID_TRANSACTIONS")
    end
end

local function hasEngineEvidence()
    if redis.call("HEXISTS", KEYS[3], expectedExecution) == 1 then
        return true
    end

    for _, transactionID in ipairs(expectedTransactionIDs) do
        if redis.call("HEXISTS", KEYS[4], transactionID .. ":" .. expectedExecution) == 1 then
            return true
        end
    end

    return false
end

local current = redis.call("GET", KEYS[1])
if not current then
    if redis.call("EXISTS", KEYS[2]) == 1 then
        return {"index_conflict", ""}
    end
    if hasEngineEvidence() then
        return {"engine_evidence", ""}
    end

    return {"already_deleted", ""}
end

local decoded, record = pcall(cjson.decode, current)
if not decoded or type(record) ~= "table" or
   (record.formatVersion ~= 1 and record.formatVersion ~= 2) or
   type(record.state) ~= "string" or
   type(record.ownerToken) ~= "string" or
   type(record.executionId) ~= "string" or
   type(record.transactionIds) ~= "table" then
    return redis.error_reply("ATOMIC_BATCH_IDEMPOTENCY_INVALID")
end

if record.ownerToken ~= expectedOwner then
    return {"stale_owner", current}
end
if record.state ~= "applied" then
    return {"state_conflict", current}
end
if record.executionId ~= expectedExecution then
    return {"execution_conflict", current}
end
if cjson.encode(record.transactionIds) ~= cjson.encode(expectedTransactionIDs) then
    return {"transaction_conflict", current}
end

local indexTarget = redis.call("GET", KEYS[2])
if indexTarget ~= KEYS[1] then
    return {"index_conflict", current}
end
if hasEngineEvidence() then
    return {"engine_evidence", current}
end

redis.call("DEL", KEYS[1], KEYS[2])
return {"deleted", current}
