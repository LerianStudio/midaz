if #KEYS ~= 2 or #ARGV ~= 6 then
    return redis.error_reply("ATOMIC_BATCH_IDEMPOTENCY_ARGUMENTS_INVALID")
end

local indexTarget = redis.call("GET", KEYS[2])
if not indexTarget then return {"missing", ""} end
if indexTarget ~= KEYS[1] then return {"index_conflict", ""} end

local current = redis.call("GET", KEYS[1])
if not current then return {"missing", ""} end

local decoded, record = pcall(cjson.decode, current)
if not decoded or type(record) ~= "table" or
   (record.formatVersion ~= 1 and record.formatVersion ~= 2) or
   type(record.ownerToken) ~= "string" or type(record.executionId) ~= "string" or
   type(record.transactionIds) ~= "table" then
    return redis.error_reply("ATOMIC_BATCH_IDEMPOTENCY_INVALID")
end

if record.ownerToken ~= ARGV[1] then return {"stale_owner", current} end
if record.executionId ~= ARGV[2] then return {"state_conflict", current} end

local member = false
for _, id in ipairs(record.transactionIds) do
    if id == ARGV[3] then member = true break end
end
if not member then return {"state_conflict", current} end

if record.state ~= "applied" and record.state ~= "complete" then
    return {"state_conflict", current}
end

if type(ARGV[4]) ~= "string" or ARGV[4] == "" then
    return redis.error_reply("ATOMIC_BATCH_IDEMPOTENCY_INITIAL_RESPONSE_INVALID")
end
local responseBytes, maxBytes = tonumber(ARGV[5]), tonumber(ARGV[6])
if not responseBytes or responseBytes < 1 or not maxBytes or maxBytes < 1 then
    return redis.error_reply("ATOMIC_BATCH_IDEMPOTENCY_INITIAL_RESPONSE_INVALID")
end

local responses = record.initialResponses
if responses == nil or responses == cjson.null then responses = {} end
if type(responses) ~= "table" then
    return redis.error_reply("ATOMIC_BATCH_IDEMPOTENCY_INITIAL_RESPONSE_INVALID")
end

local existing = responses[ARGV[3]]
if existing ~= nil and existing ~= cjson.null then
    if existing == ARGV[4] then return {"already_captured", current} end
    return {"response_conflict", current}
end

if record.state == "complete" then return {"state_conflict", current} end

local total = responseBytes
for _, encoded in pairs(responses) do
    if type(encoded) ~= "string" then
        return redis.error_reply("ATOMIC_BATCH_IDEMPOTENCY_INITIAL_RESPONSE_INVALID")
    end
    -- Base64 always has four characters per three original bytes. The exact
    -- byte total is checked by Go when reading; this conservative Lua guard
    -- prevents an unbounded record during concurrent captures.
    total = total + math.floor(#encoded * 3 / 4)
end
if total > maxBytes then return {"state_conflict", current} end

responses[ARGV[3]] = ARGV[4]
record.initialResponses = responses
local payload = cjson.encode(record)
redis.call("SET", KEYS[1], payload)
return {"captured", payload}
