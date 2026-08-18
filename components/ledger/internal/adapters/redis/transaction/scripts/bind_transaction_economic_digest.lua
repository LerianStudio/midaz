if ARGV[9] ~= "" then
    if #KEYS ~= 4 or redis.call("GET", KEYS[4]) ~= ARGV[9] then
        return redis.error_reply("FINANCIAL_DATASET_GENERATION_MISMATCH")
    end
end

local current = redis.call("HGET", KEYS[1], KEYS[2])
if not current or current ~= ARGV[1] then
    return redis.error_reply("TRANSACTION_BACKUP_CHANGED")
end

local operationsOK, operations = pcall(cjson.decode, ARGV[2])
local envelopeOK, envelope = pcall(cjson.decode, current)
if not operationsOK or type(operations) ~= "table" or #operations == 0 or
   not envelopeOK or type(envelope) ~= "table" or envelope.transaction_id ~= ARGV[4] then
    return redis.error_reply("TRANSACTION_BACKUP_INVALID")
end

local legacyAnnotation = envelope.effect_mode_version == nil and envelope.effect_mode == nil and
    envelope.transaction_status == "NOTED"
local versionedAnnotation = envelope.effect_mode_version == 1 and
    envelope.effect_mode == "ANNOTATION_ONLY" and envelope.transaction_status == "NOTED"
local annotationOnly = legacyAnnotation or versionedAnnotation
if annotationOnly then
    local beforePresent = envelope.balances ~= nil and envelope.balances ~= cjson.null and
        (type(envelope.balances) ~= "table" or #envelope.balances ~= 0)
    local afterPresent = envelope.balancesAfter ~= nil and envelope.balancesAfter ~= cjson.null and
        (type(envelope.balancesAfter) ~= "table" or #envelope.balancesAfter ~= 0)
    if ARGV[5] ~= "0" or envelope.attempt_owner ~= nil or envelope.expected_outcome ~= nil or
       beforePresent or afterPresent then
        return redis.error_reply("TRANSACTION_ANNOTATION_EFFECT_MISMATCH")
    end
elseif type(envelope.balances) ~= "table" or type(envelope.balancesAfter) ~= "table" then
    return redis.error_reply("TRANSACTION_BACKUP_INVALID")
end
if ARGV[8] ~= "" and envelope.action ~= nil and envelope.action ~= ARGV[8] then
    return redis.error_reply("TRANSACTION_BACKUP_ACTION_MISMATCH")
end

if ARGV[5] == "1" then
    if envelope.attempt_owner ~= ARGV[6] or envelope.expected_outcome ~= ARGV[7] then
        return redis.error_reply("TRANSACTION_BACKUP_ATTEMPT_MISMATCH")
    end
    local outcomeRaw = redis.call("GET", KEYS[3])
    if not outcomeRaw then
        return redis.error_reply("TRANSACTION_OUTCOME_MISSING")
    end
    local outcomeOK, outcome = pcall(cjson.decode, outcomeRaw)
    if not outcomeOK or type(outcome) ~= "table" or outcome.identity ~= ARGV[4] or
       outcome.owner ~= ARGV[6] or outcome.outcome ~= ARGV[7] then
        return redis.error_reply("TRANSACTION_OUTCOME_MISMATCH")
    end
end

if envelope.economic_effect_digest ~= nil and envelope.economic_effect_digest ~= "" then
    return redis.error_reply("TRANSACTION_BACKUP_CHANGED")
end

if envelope.operations == nil or envelope.operations == cjson.null or
   (type(envelope.operations) == "table" and #envelope.operations == 0) then
    envelope.operations = operations
elseif type(envelope.operations) ~= "table" then
    return redis.error_reply("TRANSACTION_BACKUP_INVALID")
end
if envelope.action == nil and ARGV[8] ~= "" then
    envelope.action = ARGV[8]
end
envelope.economic_effect_digest = ARGV[3]
local updated = cjson.encode(envelope)
redis.call("HSET", KEYS[1], KEYS[2], updated)

return updated
