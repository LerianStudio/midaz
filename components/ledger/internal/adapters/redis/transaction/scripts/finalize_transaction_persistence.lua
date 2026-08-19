if #KEYS == 5 and redis.call("GET", KEYS[5]) ~= ARGV[8] then
    return redis.error_reply("FINANCIAL_DATASET_GENERATION_MISMATCH")
end

-- cjson.decode turns "[]" into an empty Lua table that cjson.encode would
-- write back as "{}", which the Go decoder cannot unmarshal into a slice.
-- Restore the array type before any re-encode.
local function force_array(value)
    if type(value) == "table" and next(value) == nil then
        return cjson.decode("[]")
    end
    return value
end

if type(ARGV[4]) ~= "string" or ARGV[4] == "" or
   type(ARGV[9]) ~= "string" or ARGV[9] == "" or
   type(ARGV[10]) ~= "string" or ARGV[10] == "" then
    return redis.error_reply("TRANSACTION_ECONOMIC_DIGEST_INVALID")
end

local backup = redis.call("HGET", KEYS[1], KEYS[2])
local outcomeRaw = redis.call("GET", KEYS[3])
local tombstoneRaw = redis.call("GET", KEYS[4])

if tombstoneRaw then
    if backup or outcomeRaw then
        return redis.error_reply("TRANSACTION_PERSISTENCE_EVIDENCE_DIVERGED")
    end
    local tombstoneOK, tombstone = pcall(cjson.decode, tombstoneRaw)
    if not tombstoneOK or type(tombstone) ~= "table" or
       tombstone.identity ~= ARGV[1] or tombstone.owner ~= ARGV[2] or
       tombstone.outcome ~= ARGV[3] or tombstone.redis_generation ~= (ARGV[8] or "") or
       tombstone.economic_effect_digest ~= ARGV[4] or
       tombstone.transaction_amount ~= ARGV[9] or
       tombstone.transaction_asset_code ~= ARGV[10] or
       tombstone.parent_transaction_id ~= ARGV[5] or
       type(tombstone.transaction_status) ~= "string" or
       string.upper(tombstone.transaction_status) ~= string.upper(ARGV[6]) or
       tombstone.action ~= ARGV[7] or
       type(tombstone.operations) ~= "table" or type(tombstone.balancesAfter) ~= "table" then
        return redis.error_reply("TRANSACTION_PERSISTENCE_TOMBSTONE_MISMATCH")
    end
    return 1
end

if not backup and not outcomeRaw then
    return redis.error_reply("TRANSACTION_PERSISTENCE_TOMBSTONE_MISSING")
end

if outcomeRaw then
    local outcomeOK, outcome = pcall(cjson.decode, outcomeRaw)
    if not outcomeOK or type(outcome) ~= "table" or outcome.identity ~= ARGV[1] or
       outcome.owner ~= ARGV[2] or outcome.outcome ~= ARGV[3] then
        return redis.error_reply("TRANSACTION_OUTCOME_MISMATCH")
    end
end

local envelope = nil
if backup then
    local backupOK
    backupOK, envelope = pcall(cjson.decode, backup)
    if not backupOK or type(envelope) ~= "table" or envelope.transaction_id ~= ARGV[1] or
       envelope.attempt_owner ~= ARGV[2] or envelope.expected_outcome ~= ARGV[3] or
       envelope.economic_effect_digest ~= ARGV[4] or type(envelope.operations) ~= "table" or
       type(envelope.balancesAfter) ~= "table" or
       (envelope.parent_transaction_id or "") ~= ARGV[5] or
       type(envelope.transaction_status) ~= "string" or
       string.upper(envelope.transaction_status) ~= string.upper(ARGV[6]) or
       envelope.action ~= ARGV[7] then
        return redis.error_reply("TRANSACTION_BACKUP_MISMATCH")
    end
end

if backup and not outcomeRaw then
    return redis.error_reply("TRANSACTION_OUTCOME_MISSING")
end
if not backup then
    return redis.error_reply("TRANSACTION_BACKUP_MISSING")
end

local tombstone = {
    identity = ARGV[1],
    parent_transaction_id = envelope.parent_transaction_id or "",
    owner = ARGV[2],
    outcome = ARGV[3],
    redis_generation = ARGV[8] or "",
    transaction_status = ARGV[6],
    action = ARGV[7],
    transaction_amount = ARGV[9],
    transaction_asset_code = ARGV[10],
    operations = force_array(envelope.operations),
    balancesAfter = force_array(envelope.balancesAfter),
    economic_effect_digest = envelope.economic_effect_digest,
    expected_economic_plan = envelope.expected_economic_plan,
    operation_type_override = envelope.operation_type_override
}
-- ARGV[11] is a TTL in seconds, kept as a string so this script stays free
-- of Lua number conversions (the money-path guard forbids them entirely).
local tombstoneTTL = ARGV[11]
if type(tombstoneTTL) == "string" and tombstoneTTL ~= "" and tombstoneTTL ~= "0" then
    redis.call("SET", KEYS[4], cjson.encode(tombstone), "EX", tombstoneTTL)
else
    redis.call("SET", KEYS[4], cjson.encode(tombstone))
end
redis.call("HDEL", KEYS[1], KEYS[2])
redis.call("DEL", KEYS[3])

return 1
