local raw = redis.call("GET", KEYS[3])
if not raw then
    return redis.error_reply("TRACER_OUTCOME_MISSING")
end
local ok, record = pcall(cjson.decode, raw)
if not ok or type(record) ~= "table" or record.transaction_id ~= ARGV[1] or
   record.outcome_id ~= ARGV[2] then
    return redis.error_reply("TRACER_OUTCOME_CONFLICT")
end
if record.state == "ABORTED" then
    return raw
end
if record.state ~= "PREPARED" or record.owner ~= ARGV[3] or
   redis.call("EXISTS", KEYS[1]) ~= 1 or redis.call("GET", KEYS[2]) ~= ARGV[3] then
    return redis.error_reply("TRACER_OUTCOME_STALE_EXECUTOR")
end

local empty = cjson.decode("[]")
record.state = "ABORTED"
record.updated_at_unix_ms = tonumber(ARGV[4])
record.economic_outcome = {
    identity = ARGV[1],
    outcome = "ABORTED",
    owner = ARGV[3],
    economic_plan_version = record.economic_plan_version,
    economic_plan_digest = record.economic_plan_digest,
    before = empty,
    after = empty
}
local encoded = cjson.encode(record)
redis.call("SET", KEYS[3], encoded)
redis.call("ZADD", KEYS[4], tonumber(ARGV[4]), ARGV[5])
redis.call("DEL", KEYS[1], KEYS[2])
return encoded
