local backup = redis.call("HGET", KEYS[1], KEYS[2])
local tombstoneRaw = redis.call("GET", KEYS[3])

-- cjson.decode turns "[]" into an empty Lua table that cjson.encode would
-- write back as "{}", which the Go decoder cannot unmarshal into a slice.
-- Restore the array type before any re-encode.
local function force_array(value)
    if type(value) == "table" and next(value) == nil then
        return cjson.decode("[]")
    end
    return value
end

if type(ARGV[5]) ~= "string" or ARGV[5] == "" or
   type(ARGV[6]) ~= "string" or ARGV[6] == "" or
   type(ARGV[7]) ~= "string" or ARGV[7] == "" then
    return redis.error_reply("TRANSACTION_ECONOMIC_DIGEST_INVALID")
end
local expectedOK, expected = pcall(cjson.decode, ARGV[4])
if not expectedOK or type(expected) ~= "table" then
    return redis.error_reply("TRANSACTION_OPERATIONS_INVALID")
end

local function operations_match(operations)
    if type(operations) ~= "table" then
        return false
    end
    local counts = {}
    for _, operationID in ipairs(expected) do
        if type(operationID) ~= "string" or operationID == "" then
            return false
        end
        counts[operationID] = (counts[operationID] or 0) + 1
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
    if actualCount ~= #expected then
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
       tombstone.economic_effect_digest ~= ARGV[5] or
       tombstone.transaction_amount ~= ARGV[6] or
       tombstone.transaction_asset_code ~= ARGV[7] or
       type(tombstone.balancesAfter) ~= "table" or not operations_match(tombstone.operations) then
        return redis.error_reply("TRANSACTION_PERSISTENCE_TOMBSTONE_MISMATCH")
    end
    return 1
end

if not backup then
    return redis.error_reply("TRANSACTION_PERSISTENCE_TOMBSTONE_MISSING")
end

local backupOK, envelope = pcall(cjson.decode, backup)
if not backupOK or type(envelope) ~= "table" or envelope.transaction_id ~= ARGV[1] or
   envelope.parent_transaction_id ~= ARGV[2] or type(envelope.transaction_status) ~= "string" or
   string.upper(envelope.transaction_status) ~= string.upper(ARGV[3]) then
    return redis.error_reply("TRANSACTION_BACKUP_MISMATCH")
end
if (envelope.attempt_owner ~= nil and envelope.attempt_owner ~= "") or
   (envelope.expected_outcome ~= nil and envelope.expected_outcome ~= "") then
    return redis.error_reply("TRANSACTION_BACKUP_ATTEMPT_MISMATCH")
end
if envelope.economic_effect_digest ~= ARGV[5] or type(envelope.balancesAfter) ~= "table" or
   not operations_match(envelope.operations) then
    return redis.error_reply("TRANSACTION_ECONOMIC_DIGEST_MISMATCH")
end

local tombstone = {
    identity = ARGV[1],
    parent_transaction_id = ARGV[2],
    owner = "",
    outcome = "",
    redis_generation = "",
    transaction_status = envelope.transaction_status,
    action = "revert",
    transaction_amount = ARGV[6],
    transaction_asset_code = ARGV[7],
    operations = force_array(envelope.operations),
    balancesAfter = force_array(envelope.balancesAfter),
    economic_effect_digest = envelope.economic_effect_digest,
    expected_economic_plan = envelope.expected_economic_plan,
    operation_type_override = envelope.operation_type_override
}
-- ARGV[8] is a TTL in seconds, kept as a string so this script stays free
-- of Lua number conversions (the money-path guard forbids them entirely).
local tombstoneTTL = ARGV[8]
if type(tombstoneTTL) == "string" and tombstoneTTL ~= "" and tombstoneTTL ~= "0" then
    redis.call("SET", KEYS[3], cjson.encode(tombstone), "EX", tombstoneTTL)
else
    redis.call("SET", KEYS[3], cjson.encode(tombstone))
end
redis.call("HDEL", KEYS[1], KEYS[2])

return 1
