if redis.call("GET", KEYS[3]) then
    return redis.error_reply("TRANSACTION_PERSISTENCE_EVIDENCE_DIVERGED")
end

local current = redis.call("HGET", KEYS[1], KEYS[2])
if not current or current ~= ARGV[1] then
    return redis.error_reply("TRANSACTION_BACKUP_CHANGED")
end

local expectedOK, expected = pcall(cjson.decode, ARGV[6])
local envelopeOK, envelope = pcall(cjson.decode, current)
if not expectedOK or type(expected) ~= "table" or not envelopeOK or type(envelope) ~= "table" or
   envelope.transaction_id ~= ARGV[3] or envelope.parent_transaction_id ~= ARGV[4] or
   type(envelope.transaction_status) ~= "string" or
   string.upper(envelope.transaction_status) ~= string.upper(ARGV[5]) or
   type(envelope.operations) ~= "table" or type(envelope.balancesAfter) ~= "table" then
    return redis.error_reply("TRANSACTION_BACKUP_MISMATCH")
end
if (envelope.attempt_owner ~= nil and envelope.attempt_owner ~= "") or
   (envelope.expected_outcome ~= nil and envelope.expected_outcome ~= "") then
    return redis.error_reply("TRANSACTION_BACKUP_ATTEMPT_MISMATCH")
end

local counts = {}
for _, operationID in ipairs(expected) do
    if type(operationID) ~= "string" or operationID == "" then
        return redis.error_reply("TRANSACTION_OPERATIONS_INVALID")
    end
    counts[operationID] = (counts[operationID] or 0) + 1
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
if actualCount ~= #expected then
    return redis.error_reply("TRANSACTION_OPERATIONS_MISMATCH")
end
for _, remaining in pairs(counts) do
    if remaining ~= 0 then
        return redis.error_reply("TRANSACTION_OPERATIONS_MISMATCH")
    end
end

if envelope.economic_effect_digest ~= nil and envelope.economic_effect_digest ~= "" and
   envelope.economic_effect_digest ~= ARGV[2] then
    return redis.error_reply("TRANSACTION_ECONOMIC_DIGEST_MISMATCH")
end
-- cjson.decode turns "[]" into an empty Lua table that cjson.encode would
-- write back as "{}", which the Go decoder cannot unmarshal into a slice.
-- Restore the array type on every envelope slice before re-encoding.
local function force_array(value)
    if type(value) == "table" and next(value) == nil then
        return cjson.decode("[]")
    end
    return value
end

envelope.economic_effect_digest = ARGV[2]
envelope.operations = force_array(envelope.operations)
envelope.balancesAfter = force_array(envelope.balancesAfter)
envelope.balances = force_array(envelope.balances)
redis.call("HSET", KEYS[1], KEYS[2], cjson.encode(envelope))

return 1
