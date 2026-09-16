-- Copyright (c) 2026 Lerian Studio. All rights reserved.
-- Use of this source code is governed by the Elastic License 2.0
-- that can be found in the LICENSE file.

-- KEYS: selected recovery hash, optional legacy attempt hash, receipt hash,
--       guard hash, protection hash, cleanup schedule, and optionally the
--       atomic-batch idempotency record plus execution index.
-- ARGV: recovery field, exact envelope, attempt field, transaction UUID,
--       execution UUID, terminal flag (0|1), durable completion unix millis,
--       clear legacy attempt flag (0|1), and optionally the exact receipt
--       token, batch owner, complete record, organization UUID, ledger UUID.
local batchMode = #KEYS == 8 and #ARGV == 13
if (#KEYS ~= 6 or #ARGV ~= 8) and not batchMode then
    return redis.error_reply("ERR invalid protected recovery acknowledgement arguments")
end

if ARGV[8] ~= "0" and ARGV[8] ~= "1" then
    return redis.error_reply("ERR invalid protected recovery acknowledgement source")
end

local function redisType(key)
    local result = redis.call("TYPE", key)
    if type(result) == "table" then return result.ok end
    return result
end

for index = 1, 5 do
    local key = KEYS[index]
    if index ~= 2 or ARGV[8] == "1" then
        local kind = redisType(key)
        if kind ~= "none" and kind ~= "hash" then
            return redis.error_reply("WRONGTYPE protected recovery acknowledgement requires hashes")
        end
    end
end

local scheduleKind = redisType(KEYS[6])
if scheduleKind ~= "none" and scheduleKind ~= "zset" then
    return redis.error_reply("WRONGTYPE protected recovery cleanup schedule must be a sorted set")
end

if batchMode then
    for index = 7, 8 do
        local kind = redisType(KEYS[index])
        if kind ~= "none" and kind ~= "string" then
            return redis.error_reply("WRONGTYPE atomic batch finalization requires string keys")
        end
    end
end

local protectionKey = KEYS[5]
local protectionKind = redisType(protectionKey)
if protectionKind ~= "none" and protectionKind ~= "hash" then
    return redis.error_reply("WRONGTYPE transaction protection coordinator must be a hash")
end

local field, expected = ARGV[1], ARGV[2]
local transactionID, executionID = ARGV[4], ARGV[5]
local terminal = ARGV[6] == "1"
local completedAtMS = tonumber(ARGV[7])
if (ARGV[6] ~= "0" and ARGV[6] ~= "1") or not completedAtMS or completedAtMS < 1 then
    return redis.error_reply("ERR invalid durable completion proof")
end

local current = redis.call("HGET", KEYS[1], field)
if not current then return 0 end
if current ~= expected then return 2 end

local function decodeReceipt(raw, expectedExecution)
    local ok, receipt = pcall(cjson.decode, raw)
    if not ok or type(receipt) ~= "table" then return nil, "invalid receipt JSON" end
    if receipt.executionId ~= expectedExecution then return nil, "receipt execution differs" end
    if receipt.protection == nil then return receipt, nil end
    if type(receipt.organizationId) ~= "string" or receipt.organizationId == "" or
        type(receipt.ledgerId) ~= "string" or receipt.ledgerId == "" then
        return nil, "receipt scope differs"
    end
    local p = receipt.protection
    if type(p) ~= "table" or p.formatVersion ~= 1 or type(p.retentionSeconds) ~= "number" or
        p.retentionSeconds < 1 or p.retentionSeconds > 604800 or p.retentionSeconds % 1 ~= 0 or
        type(p.transactions) ~= "table" or type(p.recoveryFields) ~= "table" or
        type(p.acknowledged) ~= "table" or type(p.terminalCompletedAtMs) ~= "table" or
        #p.transactions == 0 or #p.transactions ~= #p.recoveryFields then
        return nil, "invalid receipt protection"
    end
    return receipt, nil
end

local currentRaw = redis.call("HGET", KEYS[3], executionID)
if not currentRaw then
    redis.call("HDEL", KEYS[1], field)
    if ARGV[8] == "1" then redis.call("HDEL", KEYS[2], ARGV[3]) end
    return 1
end

local receipt, receiptError = decodeReceipt(currentRaw, executionID)
if receiptError then return redis.error_reply("ERR " .. receiptError) end
if receipt.protection == nil then
    -- Legacy receipts carry no equivalent completion/retention proof. The
    -- recovery is durably acknowledged, but their receipt and guard stay
    -- persistent indefinitely.
    redis.call("HDEL", KEYS[1], field)
    if ARGV[8] == "1" then redis.call("HDEL", KEYS[2], ARGV[3]) end
    return 1
end

local member = false
for index, id in ipairs(receipt.protection.transactions) do
    if id == transactionID and receipt.protection.recoveryFields[index] == field then member = true end
end
if not member then return redis.error_reply("ERR recovery is not a member of its receipt") end

receipt.protection.acknowledged[transactionID] = true
if terminal then
    local previous = receipt.protection.terminalCompletedAtMs[transactionID]
    if type(previous) ~= "number" or completedAtMS > previous then
        receipt.protection.terminalCompletedAtMs[transactionID] = completedAtMS
    end
end

-- A terminal acknowledgement also supplies terminal proof to earlier PENDING
-- executions of the same transaction. The coordinator was written atomically
-- with each new-format receipt.
local receipts = { [executionID] = receipt }
local coordinatorRaw = terminal and redis.call("HGET", protectionKey, transactionID) or nil
local coordinator
if coordinatorRaw then
    local ok
    ok, coordinator = pcall(cjson.decode, coordinatorRaw)
    if not ok or type(coordinator) ~= "table" or coordinator.formatVersion ~= 1 or type(coordinator.executions) ~= "table" then
        return redis.error_reply("ERR invalid transaction protection coordinator")
    end
    for linkedExecution, _ in pairs(coordinator.executions) do
        if receipts[linkedExecution] == nil then
            local linkedRaw = redis.call("HGET", KEYS[3], linkedExecution)
            if linkedRaw then
                local linked = decodeReceipt(linkedRaw, linkedExecution)
                if linked and linked.protection then
                    for _, id in ipairs(linked.protection.transactions) do
                        if id == transactionID then
                            local previous = linked.protection.terminalCompletedAtMs[transactionID]
                            if type(previous) ~= "number" or completedAtMS > previous then
                                linked.protection.terminalCompletedAtMs[transactionID] = completedAtMS
                            end
                            receipts[linkedExecution] = linked
                        end
                    end
                end
            end
        end
    end
end

local deadlines, scheduleMembers = {}, {}
local currentExecutionReady = false
for linkedExecution, linked in pairs(receipts) do
    local ready, terminalAt = true, 0
    for _, id in ipairs(linked.protection.transactions) do
        local completed = linked.protection.terminalCompletedAtMs[id]
        if linked.protection.acknowledged[id] ~= true or type(completed) ~= "number" or completed < 1 then
            ready = false
        elseif completed > terminalAt then
            terminalAt = completed
        end
    end
    if ready then
        local deadline = terminalAt + linked.protection.retentionSeconds * 1000
        linked.protection.cleanupAfterMs = deadline
        deadlines[linkedExecution] = deadline
        scheduleMembers[linkedExecution] = linked.organizationId .. ":" .. linked.ledgerId .. ":" .. linkedExecution
    end
    if linkedExecution == executionID then currentExecutionReady = ready end
end

local batchPayload
local batchRetentionSeconds
if batchMode then
    if receipt.organizationId ~= ARGV[12] or receipt.ledgerId ~= ARGV[13] then
        return redis.error_reply("ERR atomic batch receipt scope differs")
    end

    local indexTarget = redis.call("GET", KEYS[8])
    if not indexTarget or indexTarget ~= KEYS[7] then
        return redis.error_reply("ERR atomic batch execution index differs")
    end
    local currentBatchRaw = redis.call("GET", KEYS[7])
    local currentBatchDecoded, currentBatch = pcall(cjson.decode, currentBatchRaw or "")
    if not currentBatchDecoded or type(currentBatch) ~= "table" or
       (currentBatch.formatVersion ~= 1 and currentBatch.formatVersion ~= 2) or currentBatch.ownerToken ~= ARGV[10] or
       currentBatch.executionId ~= executionID or type(currentBatch.transactionIds) ~= "table" or
       #currentBatch.transactionIds ~= #receipt.protection.transactions then
        return redis.error_reply("ERR invalid atomic batch idempotency record")
    end
    for index, id in ipairs(receipt.protection.transactions) do
        if currentBatch.transactionIds[index] ~= id then
            return redis.error_reply("ERR atomic batch transaction membership differs")
        end
    end
    if currentBatch.state == "complete" then
        if type(currentBatch.response) ~= "table" then
            return redis.error_reply("ERR completed atomic batch response is invalid")
        end
    elseif currentBatch.state == "applied" then
        if currentExecutionReady then
            if currentBatch.formatVersion == 2 then
                if type(currentBatch.initialResponses) ~= "table" then
                    return redis.error_reply("ERR atomic batch initial responses are invalid")
                end
                for _, id in ipairs(currentBatch.transactionIds) do
                    if type(currentBatch.initialResponses[id]) ~= "string" then
                        return redis.error_reply("ERR atomic batch initial response is missing")
                    end
                end
            end
            if ARGV[9] == "" then
                return 3
            end
            if currentRaw ~= ARGV[9] then
                return 4
            end
            if ARGV[11] == "" then
                return 3
            end

            local nextDecoded, nextBatch = pcall(cjson.decode, ARGV[11])
            if not nextDecoded or type(nextBatch) ~= "table" or
               nextBatch.formatVersion ~= currentBatch.formatVersion or nextBatch.state ~= "complete" or
               nextBatch.ownerToken ~= currentBatch.ownerToken or
               nextBatch.requestFingerprint ~= currentBatch.requestFingerprint or
               nextBatch.batchId ~= currentBatch.batchId or
               nextBatch.executionId ~= currentBatch.executionId or
               type(nextBatch.transactionIds) ~= "table" or type(nextBatch.response) ~= "table" or
               cjson.encode(nextBatch.transactionIds) ~= cjson.encode(currentBatch.transactionIds) then
                return redis.error_reply("ERR invalid atomic batch terminal record")
            end
            if currentBatch.formatVersion == 2 then
                if type(nextBatch.initialResponses) ~= "table" then
                    return redis.error_reply("ERR atomic batch initial responses changed")
                end
                for _, id in ipairs(currentBatch.transactionIds) do
                    if nextBatch.initialResponses[id] ~= currentBatch.initialResponses[id] then
                        return redis.error_reply("ERR atomic batch initial responses changed")
                    end
                end
            end

            batchPayload = ARGV[11]
            batchRetentionSeconds = receipt.protection.retentionSeconds
        end
    else
        return redis.error_reply("ERR atomic batch idempotency state differs")
    end
end

-- Every validation and read happens before the acknowledgement deletion. Only
-- deterministic hash writes remain afterward.
if batchPayload then
    redis.call("SET", KEYS[7], batchPayload, "EX", batchRetentionSeconds)
    redis.call("SET", KEYS[8], KEYS[7], "EX", batchRetentionSeconds)
end
redis.call("HDEL", KEYS[1], field)
if ARGV[8] == "1" then redis.call("HDEL", KEYS[2], ARGV[3]) end
for linkedExecution, linked in pairs(receipts) do
    redis.call("HSET", KEYS[3], linkedExecution, cjson.encode(linked))
end

-- Once an execution is ready, record its deadline in every participating
-- transaction coordinator. A guard expires only after every linked execution
-- has independently reached terminal durability and full acknowledgement.
for linkedExecution, deadline in pairs(deadlines) do
    local linked = receipts[linkedExecution]
    for _, id in ipairs(linked.protection.transactions) do
        local raw = redis.call("HGET", protectionKey, id)
        if raw then
            local ok, state = pcall(cjson.decode, raw)
            if ok and type(state) == "table" and state.formatVersion == 1 and type(state.executions) == "table" and state.executions[linkedExecution] ~= nil then
                state.executions[linkedExecution] = deadline
                local allReady, guardDeadline = true, 0
                for _, executionDeadline in pairs(state.executions) do
                    if type(executionDeadline) ~= "number" or executionDeadline < 1 then
                        allReady = false
                    elseif executionDeadline > guardDeadline then
                        guardDeadline = executionDeadline
                    end
                end
                -- Valkey 8.1 lacks per-hash-field expiry. Keep the fully proven
                -- deadline in the coordinator for the bounded cleanup owner.
                if allReady then state.cleanupAfterMs = guardDeadline end
                redis.call("HSET", protectionKey, id, cjson.encode(state))
            end
        end
    end
    redis.call("ZADD", KEYS[6], deadline, scheduleMembers[linkedExecution])
end

return 1
