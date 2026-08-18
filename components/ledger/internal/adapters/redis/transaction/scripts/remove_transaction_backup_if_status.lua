local raw = redis.call("HGET", KEYS[1], KEYS[2])
if not raw then
    return 0
end

local ok, envelope = pcall(cjson.decode, raw)
if not ok or type(envelope) ~= "table" or type(envelope.transaction_status) ~= "string" then
    return redis.error_reply("TRANSACTION_BACKUP_INVALID")
end

if string.upper(envelope.transaction_status) ~= string.upper(ARGV[1]) then
    return 0
end

local owner = envelope.attempt_owner or ""
local outcome = envelope.expected_outcome or ""
if owner ~= ARGV[2] or outcome ~= ARGV[3] then
    return 0
end
if ARGV[4] == "1" and envelope.balancesAfter ~= nil then
    return 0
end

return redis.call("HDEL", KEYS[1], KEYS[2])
