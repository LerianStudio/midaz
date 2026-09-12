-- main validates the fixed Redis script envelope and delegates the already
-- decoded request to the engine orchestration. Business decisions remain inside
-- execute, where live state and writes share the same atomic Redis invocation.
local function main()
    if #ARGV ~= 3 or #KEYS < 5 then technical("invalid_protocol", "invalid script argument count") end
    local maximumRequest, maximumPrepared = positiveBudget(ARGV[2]), positiveBudget(ARGV[3])
    if #ARGV[1] > maximumRequest then technical("request_bytes_exceeded", "request exceeds byte budget") end
    local request = decodeRequest(ARGV[1])
    return execute(request, maximumPrepared)
end

-- Convert all controlled Lua errors into the closed wire protocol understood by
-- the Go adapter. No raw runtime detail or mutable accounting data is exposed.
local ok, result = pcall(main)
if ok then return result end

-- Any error after publication began has an unknown outcome. The caller must use
-- receipt and recovery evidence instead of retrying the accounting mutation.
if commitStarted then
    return redis.error_reply("MIDAZ_ENGINE_TECH_V1 " .. encodeJSON({ code = "indeterminate", message = "accounting commit failed after writes may have started" }))
end
-- Business refusals identify the exact transaction, posting, and balance without
-- classifying the execution as a technical failure.
if type(result) == "table" and result.kind == "failure" then
    return redis.error_reply("MIDAZ_ENGINE_V1 " .. encodeJSON({
        code = result.code, transactionIndex = result.transactionIndex,
        postingIndex = result.postingIndex, balanceRef = result.balanceRef
    }))
end
-- Cache normalization is a precommit repair request and can be handled before the
-- same immutable accounting execution is submitted again.
if type(result) == "table" and result.kind == "normalization" then
    return redis.error_reply("BALANCE_LIMIT_NORMALIZATION_REQUIRED:" .. encodeJSON(result.keys))
end
-- Known technical failures preserve their closed code for safe classification by Go.
if type(result) == "table" and result.kind == "technical" then
    return redis.error_reply("MIDAZ_ENGINE_TECH_V1 " .. encodeJSON({ code = result.code, message = result.message }))
end
-- Unexpected precommit failures are intentionally collapsed into one non-sensitive
-- code because no accounting writes have started and raw Lua errors are not stable API.
return redis.error_reply("MIDAZ_ENGINE_TECH_V1 " .. encodeJSON({ code = "script_runtime_failed", message = "accounting execution failed before commit" }))
