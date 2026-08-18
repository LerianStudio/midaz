if ARGV[7] ~= "" then
    if #KEYS ~= 5 or redis.call("GET", KEYS[5]) ~= ARGV[7] then
        return redis.error_reply("FINANCIAL_DATASET_GENERATION_MISMATCH")
    end
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
       tombstone.identity ~= ARGV[1] or tombstone.owner ~= ARGV[3] or
       tombstone.outcome ~= ARGV[4] or tombstone.redis_generation ~= ARGV[7] or
       type(tombstone.operations) ~= "table" or type(tombstone.balancesAfter) ~= "table" then
        return redis.error_reply("TRANSACTION_PERSISTENCE_TOMBSTONE_MISMATCH")
    end
    if ARGV[6] ~= "" and tombstone.action ~= ARGV[6] then
        return redis.error_reply("TRANSACTION_PERSISTENCE_TOMBSTONE_ACTION_MISMATCH")
    end
    return cjson.encode({
        terminal = true,
        operations = tombstone.operations,
        balancesAfter = tombstone.balancesAfter,
        economicEffectDigest = tombstone.economic_effect_digest or "",
        raw = tombstoneRaw
    })
end

if not backup then
    return redis.error_reply("TRANSACTION_BACKUP_MISSING")
end

local ok, envelope = pcall(cjson.decode, backup)
if not ok or type(envelope) ~= "table" then
    return redis.error_reply("TRANSACTION_BACKUP_INVALID")
end

if envelope.transaction_id ~= ARGV[1] then
    return redis.error_reply("TRANSACTION_BACKUP_IDENTITY_MISMATCH")
end

if ARGV[2] == "1" then
    if envelope.attempt_owner ~= ARGV[3] or envelope.expected_outcome ~= ARGV[4] or envelope.balancesAfter == nil then
        return redis.error_reply("TRANSACTION_BACKUP_ATTEMPT_MISMATCH")
    end

    if not outcomeRaw then
        return redis.error_reply("TRANSACTION_OUTCOME_MISSING")
    end
    local outcomeOK, outcome = pcall(cjson.decode, outcomeRaw)
    if not outcomeOK or type(outcome) ~= "table" or outcome.identity ~= ARGV[1] or
       outcome.owner ~= ARGV[3] or outcome.outcome ~= ARGV[4] then
        return redis.error_reply("TRANSACTION_OUTCOME_MISMATCH")
    end

	if ARGV[7] ~= "" and envelope.redis_generation ~= ARGV[7] then
		return redis.error_reply("FINANCIAL_DATASET_GENERATION_MISMATCH")
	end
end

local operationsOK, operations = pcall(cjson.decode, ARGV[5])
if not operationsOK or type(operations) ~= "table" then
    return redis.error_reply("TRANSACTION_OPERATIONS_INVALID")
end

if envelope.action ~= nil and ARGV[6] ~= "" and envelope.action ~= ARGV[6] then
    return redis.error_reply("TRANSACTION_BACKUP_ACTION_MISMATCH")
end

if envelope.operations == nil then
    envelope.operations = operations
    if ARGV[6] ~= "" and envelope.action == nil then
        envelope.action = ARGV[6]
    end
    backup = cjson.encode(envelope)
    redis.call("HSET", KEYS[1], KEYS[2], backup)
end

return cjson.encode({
    terminal = false,
    operations = envelope.operations,
    balancesAfter = envelope.balancesAfter or {},
    economicEffectDigest = envelope.economic_effect_digest or "",
    raw = backup
})
