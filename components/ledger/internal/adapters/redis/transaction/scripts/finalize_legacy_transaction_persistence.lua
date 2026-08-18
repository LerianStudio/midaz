local backup = redis.call("HGET", KEYS[1], KEYS[2])
local tombstoneRaw = redis.call("GET", KEYS[3])

local expectedOK, expected = pcall(cjson.decode, ARGV[4])
if not expectedOK or type(expected) ~= "table" then
    return redis.error_reply("TRANSACTION_OPERATIONS_INVALID")
end

local function operations_match(operations)
    if type(operations) ~= "table" then
        return false
    end
    local counts = {}
    local expectedCount = 0
    for _, operationID in ipairs(expected) do
        if type(operationID) ~= "string" or operationID == "" then
            return false
        end
        counts[operationID] = (counts[operationID] or 0) + 1
        expectedCount = expectedCount + 1
    end

    local actualCount = 0
    for _, operation in ipairs(operations) do
        if type(operation) ~= "table" or type(operation.id) ~= "string" or
           counts[operation.id] == nil or counts[operation.id] == 0 then
            return false
        end
        counts[operation.id] = counts[operation.id] - 1
        actualCount = actualCount + 1
    end
    if actualCount ~= expectedCount then
        return false
    end
    for _, remaining in pairs(counts) do
        if remaining ~= 0 then
            return false
        end
    end
    return true
end

if tombstoneRaw then
    if backup then
        return redis.error_reply("TRANSACTION_PERSISTENCE_EVIDENCE_DIVERGED")
    end
    local tombstoneOK, tombstone = pcall(cjson.decode, tombstoneRaw)
    if not tombstoneOK or type(tombstone) ~= "table" or
       tombstone.identity ~= ARGV[1] or tombstone.parent_transaction_id ~= ARGV[2] or
       tombstone.owner ~= "" or tombstone.outcome ~= "" or tombstone.redis_generation ~= "" or
       tombstone.action ~= "revert" or
       type(tombstone.transaction_status) ~= "string" or
       string.upper(tombstone.transaction_status) ~= string.upper(ARGV[3]) or
       type(tombstone.balancesAfter) ~= "table" or not operations_match(tombstone.operations) then
        return redis.error_reply("TRANSACTION_PERSISTENCE_TOMBSTONE_MISMATCH")
    end
    return 1
end

if not backup then
    return redis.error_reply("TRANSACTION_PERSISTENCE_TOMBSTONE_MISSING")
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

if type(envelope.operations) ~= "table" then
    return redis.error_reply("TRANSACTION_OPERATIONS_INVALID")
end
if not operations_match(envelope.operations) then
    return redis.error_reply("TRANSACTION_OPERATIONS_MISMATCH")
end

local tombstone = {
    identity = ARGV[1],
    parent_transaction_id = ARGV[2],
    owner = "",
    outcome = "",
    redis_generation = "",
    transaction_status = envelope.transaction_status,
    action = "revert",
    operations = envelope.operations,
    balancesAfter = envelope.balancesAfter
}
redis.call("SET", KEYS[3], cjson.encode(tombstone))
redis.call("HDEL", KEYS[1], KEYS[2])

return 1
