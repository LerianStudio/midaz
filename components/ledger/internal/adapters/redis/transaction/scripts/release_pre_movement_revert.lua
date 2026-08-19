if redis.call("GET", KEYS[6]) ~= ARGV[5] then
    return -1
end
if redis.call("EXISTS", KEYS[3], KEYS[4], KEYS[5]) ~= 0 then
    return 0
end

local raw = redis.call("HGET", KEYS[1], KEYS[2])
if not raw then
    return 1
end

local ok, envelope = pcall(cjson.decode, raw)
if not ok or type(envelope) ~= "table" or
   envelope.transaction_id ~= ARGV[4] or
   envelope.parent_transaction_id ~= ARGV[6] or
   string.upper(envelope.transaction_status or "") ~= string.upper(ARGV[1]) or
   (envelope.attempt_owner or "") ~= ARGV[2] or
   (envelope.expected_outcome or "") ~= ARGV[3] or
   envelope.balancesAfter ~= nil then
    return 0
end

redis.call("HDEL", KEYS[1], KEYS[2])
return 1
