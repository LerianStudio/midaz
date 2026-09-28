local rawIndex = redis.call("HGET", KEYS[1], ARGV[1])
if not rawIndex then return 0 end

local ok, index = pcall(cjson.decode, rawIndex)
if not ok or type(index) ~= "table" or index.formatVersion ~= 1 or index.transactionId ~= ARGV[1] or index.executionId ~= ARGV[2] then
    return 0
end

redis.call("SET", KEYS[2], ARGV[3], "PX", ARGV[4])
return 1
