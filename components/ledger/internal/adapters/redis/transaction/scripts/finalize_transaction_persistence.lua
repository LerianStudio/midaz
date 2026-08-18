if #KEYS == 4 and redis.call("GET", KEYS[4]) ~= ARGV[5] then
    return redis.error_reply("FINANCIAL_DATASET_GENERATION_MISMATCH")
end

local backup = redis.call("HGET", KEYS[1], KEYS[2])
local outcomeRaw = redis.call("GET", KEYS[3])

-- The previous invocation can have committed and lost its response.
if not backup and not outcomeRaw then
    return 1
end

if outcomeRaw then
    local outcomeOK, outcome = pcall(cjson.decode, outcomeRaw)
    if not outcomeOK or type(outcome) ~= "table" or outcome.identity ~= ARGV[1] or
       outcome.owner ~= ARGV[2] or outcome.outcome ~= ARGV[3] then
        return redis.error_reply("TRANSACTION_OUTCOME_MISMATCH")
    end
end

if backup then
    local backupOK, envelope = pcall(cjson.decode, backup)
    if not backupOK or type(envelope) ~= "table" or envelope.transaction_id ~= ARGV[1] or
       envelope.attempt_owner ~= ARGV[2] or envelope.expected_outcome ~= ARGV[3] or
       envelope.balancesAfter == nil or envelope.operations == nil then
        return redis.error_reply("TRANSACTION_BACKUP_MISMATCH")
    end

    local expectedOK, expected = pcall(cjson.decode, ARGV[4])
    if not expectedOK or type(expected) ~= "table" or type(envelope.operations) ~= "table" then
        return redis.error_reply("TRANSACTION_OPERATIONS_INVALID")
    end
    local counts = {}
    local expectedCount = 0
    for _, operationID in ipairs(expected) do
        if type(operationID) ~= "string" or operationID == "" then
            return redis.error_reply("TRANSACTION_OPERATIONS_INVALID")
        end
        counts[operationID] = (counts[operationID] or 0) + 1
        expectedCount = expectedCount + 1
    end
    local actualCount = 0
    for _, operation in ipairs(envelope.operations) do
        if type(operation) ~= "table" or type(operation.id) ~= "string" or
           counts[operation.id] == nil or counts[operation.id] == 0 then
            return redis.error_reply("TRANSACTION_OPERATIONS_MISMATCH")
        end
        counts[operation.id] = counts[operation.id] - 1
        actualCount = actualCount + 1
    end
    if actualCount ~= expectedCount then
        return redis.error_reply("TRANSACTION_OPERATIONS_MISMATCH")
    end
    for _, remaining in pairs(counts) do
        if remaining ~= 0 then
            return redis.error_reply("TRANSACTION_OPERATIONS_MISMATCH")
        end
    end
end

-- A backup with no immutable outcome is not proven to be the same economic
-- attempt. Preserve it for reconciliation instead of deleting evidence.
if backup and not outcomeRaw then
    return redis.error_reply("TRANSACTION_OUTCOME_MISSING")
end

if backup then
    redis.call("HDEL", KEYS[1], KEYS[2])
end
if outcomeRaw then
    redis.call("DEL", KEYS[3])
end

return 1
