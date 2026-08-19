if redis.call("EXISTS", KEYS[3]) ~= 1 or redis.call("GET", KEYS[4]) ~= ARGV[1] then
    return redis.error_reply("TRANSACTION_EXECUTION_ATTEMPT_MISMATCH")
end
if redis.call("EXISTS", KEYS[5]) == 1 then
    return redis.error_reply("TRANSACTION_OUTCOME_ALREADY_EXISTS")
end
if #KEYS == 6 and redis.call("GET", KEYS[6]) ~= ARGV[5] then
    return redis.error_reply("FINANCIAL_DATASET_GENERATION_MISMATCH")
end

local existing = redis.call("HGET", KEYS[1], KEYS[2])
if existing then
    local ok, envelope = pcall(cjson.decode, existing)
    local incomingOK, incoming = pcall(cjson.decode, ARGV[4])
    if not ok or type(envelope) ~= "table" or envelope.transaction_id ~= ARGV[2] or
       envelope.attempt_owner ~= ARGV[1] or envelope.expected_outcome ~= ARGV[3] or
       envelope.balancesAfter ~= nil or not incomingOK or type(incoming) ~= "table" then
        return redis.error_reply("TRANSACTION_BACKUP_OWNER_MISMATCH")
    end
    local existingPlan = envelope.expected_economic_plan
    local incomingPlan = incoming.expected_economic_plan
    if (existingPlan == nil) ~= (incomingPlan == nil) or
       (existingPlan ~= nil and (type(existingPlan) ~= "table" or type(incomingPlan) ~= "table" or
        existingPlan.version ~= incomingPlan.version or existingPlan.digest ~= incomingPlan.digest)) then
        return redis.error_reply("TRANSACTION_BACKUP_OWNER_MISMATCH")
    end

    -- A lost first seed response is an exact no-op. Preserve the original
    -- envelope rather than replacing timestamps or request context.
    return 0
end

redis.call("HSET", KEYS[1], KEYS[2], ARGV[4])
return 1
