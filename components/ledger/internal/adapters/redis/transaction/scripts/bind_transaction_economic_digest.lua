if ARGV[8] ~= "" then
    if #KEYS ~= 4 or redis.call("GET", KEYS[4]) ~= ARGV[8] then
        return redis.error_reply("FINANCIAL_DATASET_GENERATION_MISMATCH")
    end
end

local current = redis.call("HGET", KEYS[1], KEYS[2])
if not current or current ~= ARGV[1] then
    return redis.error_reply("TRANSACTION_BACKUP_CHANGED")
end

local ok, envelope = pcall(cjson.decode, current)
if not ok or type(envelope) ~= "table" or envelope.transaction_id ~= ARGV[3] or
   type(envelope.operations) ~= "table" or type(envelope.balancesAfter) ~= "table" then
    return redis.error_reply("TRANSACTION_BACKUP_INVALID")
end
if ARGV[7] ~= "" and envelope.action ~= nil and envelope.action ~= ARGV[7] then
    return redis.error_reply("TRANSACTION_BACKUP_ACTION_MISMATCH")
end

if ARGV[4] == "1" then
    if envelope.attempt_owner ~= ARGV[5] or envelope.expected_outcome ~= ARGV[6] then
        return redis.error_reply("TRANSACTION_BACKUP_ATTEMPT_MISMATCH")
    end
    local outcomeRaw = redis.call("GET", KEYS[3])
    if not outcomeRaw then
        return redis.error_reply("TRANSACTION_OUTCOME_MISSING")
    end
    local outcomeOK, outcome = pcall(cjson.decode, outcomeRaw)
    if not outcomeOK or type(outcome) ~= "table" or outcome.identity ~= ARGV[3] or
       outcome.owner ~= ARGV[5] or outcome.outcome ~= ARGV[6] then
        return redis.error_reply("TRANSACTION_OUTCOME_MISMATCH")
    end
end

if envelope.economic_effect_digest ~= nil and envelope.economic_effect_digest ~= "" and
   envelope.economic_effect_digest ~= ARGV[2] then
    return redis.error_reply("TRANSACTION_ECONOMIC_DIGEST_MISMATCH")
end
envelope.economic_effect_digest = ARGV[2]
redis.call("HSET", KEYS[1], KEYS[2], cjson.encode(envelope))

return 1
