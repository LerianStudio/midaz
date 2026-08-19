local existingRaw = redis.call("GET", KEYS[5])
if existingRaw then
    local ok, existing = pcall(cjson.decode, existingRaw)
    if not ok or type(existing) ~= "table" or existing.transaction_id ~= ARGV[2] or
       existing.outcome_id ~= ARGV[3] or existing.owner ~= ARGV[1] or
       existing.economic_plan_version ~= ARGV[6] or existing.economic_plan_digest ~= ARGV[7] or
       (existing.economic_phase or "") ~= ARGV[11] then
        return redis.error_reply("TRACER_OUTCOME_CONFLICT")
    end
    if existing.state == "PREPARED" and
       (redis.call("EXISTS", KEYS[3]) ~= 1 or redis.call("GET", KEYS[4]) ~= ARGV[1]) then
        return redis.error_reply("TRACER_OUTCOME_STALE_EXECUTOR")
    end
    return existingRaw
end

local backupRaw = redis.call("HGET", KEYS[1], KEYS[2])
if not backupRaw then
    return redis.error_reply("EXPECTED_ECONOMIC_PLAN_MISSING")
end
local backupOK, backup = pcall(cjson.decode, backupRaw)
if not backupOK or type(backup) ~= "table" or backup.transaction_id ~= ARGV[2] or
   type(backup.expected_economic_plan) ~= "table" or
   tostring(backup.expected_economic_plan.version) ~= ARGV[6] or
   backup.expected_economic_plan.digest ~= ARGV[7] then
    return redis.error_reply("EXPECTED_ECONOMIC_PLAN_MISMATCH")
end

local executionExists = redis.call("EXISTS", KEYS[3])
if executionExists == 0 then
    redis.call("SET", KEYS[3], "")
    redis.call("SET", KEYS[4], ARGV[1])
elseif redis.call("GET", KEYS[4]) ~= ARGV[1] then
    return redis.error_reply("TRANSACTION_EXECUTION_ATTEMPT_MISMATCH")
end

local record = {
    version = 1,
    transaction_id = ARGV[2],
    outcome_id = ARGV[3],
    organization_id = ARGV[4],
    ledger_id = ARGV[5],
    state = "PREPARED",
    owner = ARGV[1],
    economic_plan_version = ARGV[6],
    economic_plan_digest = ARGV[7],
    economic_phase = ARGV[11],
    prepared_at_unix_ms = tonumber(ARGV[8]),
    updated_at_unix_ms = tonumber(ARGV[8]),
    delivery_attempts = 0
}
local encoded = cjson.encode(record)
redis.call("SET", KEYS[5], encoded)
redis.call("ZADD", KEYS[6], tonumber(ARGV[9]), ARGV[10])
return encoded
