local raw = redis.call("GET", KEYS[1])
if not raw then
    redis.call("ZREM", KEYS[2], ARGV[4])
    return 0
end
local ok, record = pcall(cjson.decode, raw)
if not ok or type(record) ~= "table" or record.outcome_id ~= ARGV[1] then
    return redis.error_reply("TRACER_OUTCOME_CONFLICT")
end
if record.state == "DELIVERED" then
    redis.call("ZREM", KEYS[2], ARGV[4])
    return 1
end
if record.state ~= ARGV[2] then
    return redis.error_reply("TRACER_OUTCOME_CONFLICT")
end
record.state = "DELIVERED"
record.last_error = nil
record.updated_at_unix_ms = tonumber(ARGV[3])
redis.call("SET", KEYS[1], cjson.encode(record), "PX", tonumber(ARGV[5]))
redis.call("ZREM", KEYS[2], ARGV[4])
return 1
