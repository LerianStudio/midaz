local function main()
    if #ARGV ~= 3 or #KEYS < 5 then technical("invalid_protocol", "invalid script argument count") end
    local maximumRequest, maximumPrepared = positiveBudget(ARGV[2]), positiveBudget(ARGV[3])
    if #ARGV[1] > maximumRequest then technical("request_bytes_exceeded", "request exceeds byte budget") end
    local request = decodeRequest(ARGV[1])
    return execute(request, maximumPrepared)
end

local ok, result = pcall(main)
if ok then return result end
if commitStarted then
    return redis.error_reply("MIDAZ_ENGINE_TECH_V1 " .. encodeJSON({ code = "indeterminate", message = "accounting commit failed after writes may have started" }))
end
if type(result) == "table" and result.kind == "failure" then
    return redis.error_reply("MIDAZ_ENGINE_V1 " .. encodeJSON({
        code = result.code, transactionIndex = result.transactionIndex,
        postingIndex = result.postingIndex, balanceRef = result.balanceRef
    }))
end
if type(result) == "table" and result.kind == "normalization" then
    return redis.error_reply("BALANCE_LIMIT_NORMALIZATION_REQUIRED:" .. encodeJSON(result.keys))
end
if type(result) == "table" and result.kind == "technical" then
    return redis.error_reply("MIDAZ_ENGINE_TECH_V1 " .. encodeJSON({ code = result.code, message = result.message }))
end
return redis.error_reply("MIDAZ_ENGINE_TECH_V1 " .. encodeJSON({ code = "script_runtime_failed", message = "accounting execution failed before commit" }))
