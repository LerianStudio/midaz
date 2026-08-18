if ARGV[6] ~= "" then
    if #KEYS ~= 5 or redis.call("GET", KEYS[5]) ~= ARGV[6] then
        return redis.error_reply("FINANCIAL_DATASET_GENERATION_MISMATCH")
    end
end

local backupRaw = redis.call("HGET", KEYS[1], KEYS[2])
local outcomeRaw = redis.call("GET", KEYS[3])
local tombstoneRaw = redis.call("GET", KEYS[4])

if tombstoneRaw then
    if backupRaw or outcomeRaw then
        return redis.error_reply("TRANSACTION_PERSISTENCE_EVIDENCE_DIVERGED")
    end
    local tombstoneOK, tombstone = pcall(cjson.decode, tombstoneRaw)
    if not tombstoneOK or type(tombstone) ~= "table" or
       tombstone.identity ~= ARGV[1] or tombstone.owner ~= ARGV[3] or
       tombstone.outcome ~= ARGV[4] or tombstone.redis_generation ~= ARGV[6] or
       type(tombstone.operations) ~= "table" or type(tombstone.balancesAfter) ~= "table" then
        return redis.error_reply("TRANSACTION_PERSISTENCE_TOMBSTONE_MISMATCH")
    end
    if ARGV[5] ~= "" and tombstone.action ~= ARGV[5] then
        return redis.error_reply("TRANSACTION_PERSISTENCE_TOMBSTONE_ACTION_MISMATCH")
    end
    return cjson.encode({
        terminal = true,
        raw = tombstoneRaw,
        outcomeRaw = ""
    })
end

if not backupRaw then
    return redis.error_reply("TRANSACTION_BACKUP_MISSING")
end

local backupOK, envelope = pcall(cjson.decode, backupRaw)
if not backupOK or type(envelope) ~= "table" or envelope.transaction_id ~= ARGV[1] then
    return redis.error_reply("TRANSACTION_BACKUP_INVALID")
end
if envelope.action ~= nil and ARGV[5] ~= "" and envelope.action ~= ARGV[5] then
    return redis.error_reply("TRANSACTION_BACKUP_ACTION_MISMATCH")
end

if ARGV[2] == "1" then
    if envelope.attempt_owner ~= ARGV[3] or envelope.expected_outcome ~= ARGV[4] or
       envelope.balances == nil or envelope.balancesAfter == nil then
        return redis.error_reply("TRANSACTION_BACKUP_ATTEMPT_MISMATCH")
    end
    if not outcomeRaw then
        return redis.error_reply("TRANSACTION_OUTCOME_MISSING")
    end
    local outcomeOK, outcome = pcall(cjson.decode, outcomeRaw)
    if not outcomeOK or type(outcome) ~= "table" or outcome.identity ~= ARGV[1] or
       outcome.owner ~= ARGV[3] or outcome.outcome ~= ARGV[4] or
       type(outcome.before) ~= "table" or type(outcome.after) ~= "table" then
        return redis.error_reply("TRANSACTION_OUTCOME_MISMATCH")
    end
    if ARGV[6] ~= "" and envelope.redis_generation ~= ARGV[6] then
        return redis.error_reply("FINANCIAL_DATASET_GENERATION_MISMATCH")
    end
end

-- This command is intentionally read-only. Go validates the candidate against
-- these exact Lua-authored bytes before a separate expected-raw CAS can choose
-- operation IDs and bind the economic digest once.
return cjson.encode({
    terminal = false,
    raw = backupRaw,
    outcomeRaw = outcomeRaw or ""
})
