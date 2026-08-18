local backup = redis.call("HGET", KEYS[1], KEYS[2])
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

    local outcomeRaw = redis.call("GET", KEYS[3])
    if not outcomeRaw then
        return redis.error_reply("TRANSACTION_OUTCOME_MISSING")
    end
    local outcomeOK, outcome = pcall(cjson.decode, outcomeRaw)
    if not outcomeOK or type(outcome) ~= "table" or outcome.identity ~= ARGV[1] or
       outcome.owner ~= ARGV[3] or outcome.outcome ~= ARGV[4] then
        return redis.error_reply("TRANSACTION_OUTCOME_MISMATCH")
    end

	if ARGV[7] ~= "" then
		if #KEYS ~= 4 or redis.call("GET", KEYS[4]) ~= ARGV[7] or envelope.redis_generation ~= ARGV[7] then
			return redis.error_reply("FINANCIAL_DATASET_GENERATION_MISMATCH")
		end
	end
end

local operationsOK, operations = pcall(cjson.decode, ARGV[5])
if not operationsOK or type(operations) ~= "table" then
    return redis.error_reply("TRANSACTION_OPERATIONS_INVALID")
end

if envelope.action ~= nil and ARGV[6] ~= "" and envelope.action ~= ARGV[6] then
    return redis.error_reply("TRANSACTION_BACKUP_ACTION_MISMATCH")
end

if envelope.operations ~= nil then
    return cjson.encode(envelope.operations)
end

envelope.operations = operations
if ARGV[6] ~= "" and envelope.action == nil then
    envelope.action = ARGV[6]
end

redis.call("HSET", KEYS[1], KEYS[2], cjson.encode(envelope))
return cjson.encode(operations)
