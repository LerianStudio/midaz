local raw = redis.call("GET", KEYS[1])
if not raw then
    redis.call("ZREM", KEYS[2], ARGV[5])
    return 0
end
local ok, record = pcall(cjson.decode, raw)
if not ok or type(record) ~= "table" or record.outcome_id ~= ARGV[1] or record.state ~= ARGV[2] then
    return redis.error_reply("TRACER_OUTCOME_CONFLICT")
end
record.delivery_attempts = (tonumber(record.delivery_attempts) or 0) + 1
record.last_error = ARGV[3]
record.updated_at_unix_ms = tonumber(ARGV[4])
redis.call("SET", KEYS[1], cjson.encode(record))
redis.call("ZADD", KEYS[2], tonumber(ARGV[6]), ARGV[5])
return 1
