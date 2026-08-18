local backup = redis.call("HGET", KEYS[1], KEYS[2])
if not backup then
    return 1
end

local ok, envelope = pcall(cjson.decode, backup)
if not ok or type(envelope) ~= "table" then
    return redis.error_reply("TRANSACTION_BACKUP_INVALID")
end

if envelope.transaction_id ~= ARGV[1] or envelope.parent_transaction_id ~= ARGV[2] then
    return redis.error_reply("TRANSACTION_BACKUP_IDENTITY_MISMATCH")
end
if type(envelope.transaction_status) ~= "string" or string.upper(envelope.transaction_status) ~= string.upper(ARGV[3]) then
    return redis.error_reply("TRANSACTION_BACKUP_STATUS_MISMATCH")
end
if (envelope.attempt_owner ~= nil and envelope.attempt_owner ~= "") or
   (envelope.expected_outcome ~= nil and envelope.expected_outcome ~= "") then
    return redis.error_reply("TRANSACTION_BACKUP_ATTEMPT_MISMATCH")
end
if envelope.balancesAfter == nil then
    return redis.error_reply("TRANSACTION_BACKUP_NOT_TERMINAL")
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

return redis.call("HDEL", KEYS[1], KEYS[2])
