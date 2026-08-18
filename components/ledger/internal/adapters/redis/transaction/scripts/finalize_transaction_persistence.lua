if #KEYS == 5 and redis.call("GET", KEYS[5]) ~= ARGV[5] then
    return redis.error_reply("FINANCIAL_DATASET_GENERATION_MISMATCH")
end

if type(ARGV[4]) ~= "string" or ARGV[4] == "" then
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
       tombstone.outcome ~= ARGV[3] or tombstone.redis_generation ~= (ARGV[5] or "") or
       tombstone.economic_effect_digest ~= ARGV[4] or
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
       type(envelope.balancesAfter) ~= "table" then
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
    redis_generation = ARGV[5] or "",
    transaction_status = envelope.transaction_status or "",
    action = envelope.action or "",
    operations = envelope.operations,
    balancesAfter = envelope.balancesAfter,
    economic_effect_digest = envelope.economic_effect_digest
}
redis.call("SET", KEYS[4], cjson.encode(tombstone))
redis.call("HDEL", KEYS[1], KEYS[2])
redis.call("DEL", KEYS[3])

return 1
