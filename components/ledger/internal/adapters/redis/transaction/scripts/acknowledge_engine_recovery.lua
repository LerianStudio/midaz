-- Copyright (c) 2026 Lerian Studio. All rights reserved.
-- Use of this source code is governed by the Elastic License 2.0
-- that can be found in the LICENSE file.

-- KEYS: selected recovery hash, source-specific attempt hash, receipt hash,
--       guard hash, protection hash, cleanup schedule, protected evidence
--       hash, transaction index hash, and optionally the atomic-batch
--       idempotency record plus execution index.
-- ARGV: recovery field, exact envelope, attempt field, transaction UUID,
--       execution UUID, terminal flag (0|1), durable completion unix millis,
--       legacy recovery source flag (0|1), and optionally the exact receipt
--       token, batch owner, complete record, organization UUID, ledger UUID.
local batchMode = #KEYS == 10 and #ARGV == 13
if (#KEYS ~= 8 or #ARGV ~= 8) and not batchMode then
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
    local kind = redisType(key)
    if kind ~= "none" and kind ~= "hash" then
        return redis.error_reply("WRONGTYPE protected recovery acknowledgement requires hashes")
    end
end

for index = 7, 8 do
    local kind = redisType(KEYS[index])
    if kind ~= "none" and kind ~= "hash" then
        return redis.error_reply("WRONGTYPE protected write-behind artifacts must be hashes")
    end
end

local scheduleKind = redisType(KEYS[6])
if scheduleKind ~= "none" and scheduleKind ~= "zset" then
    return redis.error_reply("WRONGTYPE protected recovery cleanup schedule must be a sorted set")
end

if batchMode then
    for index = 9, 10 do
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
    if type(p) ~= "table" or (p.formatVersion ~= 1 and p.formatVersion ~= 2) or type(p.retentionSeconds) ~= "number" or
        p.retentionSeconds < 1 or p.retentionSeconds > 604800 or p.retentionSeconds % 1 ~= 0 or
        type(p.transactions) ~= "table" or type(p.recoveryFields) ~= "table" or
        type(p.acknowledged) ~= "table" or type(p.terminalCompletedAtMs) ~= "table" or
        #p.transactions == 0 or #p.transactions ~= #p.recoveryFields or
        (p.formatVersion == 2 and (type(p.indexFields) ~= "table" or #p.indexFields ~= #p.transactions)) then
        return nil, "invalid receipt protection"
    end
    return receipt, nil
end

local function markDurabilityComplete(raw)
    local pending = '"durabilityState":"pending"'
    local complete = '"durabilityState":"complete"'
    local first, last = string.find(raw, pending, 1, true)
    if not first or string.find(raw, pending, last + 1, true) then
        return nil
    end

    return string.sub(raw, 1, first - 1) .. complete .. string.sub(raw, last + 1)
end

local currentRaw = redis.call("HGET", KEYS[3], executionID)
if not currentRaw then
    local decoded, possibleEnvelope = pcall(cjson.decode, expected)
    if decoded and type(possibleEnvelope) == "table" and possibleEnvelope.applicationState ~= nil then
        return redis.error_reply("ERR write-behind receipt is missing")
    end
    redis.call("HDEL", KEYS[1], field)
    redis.call("HDEL", KEYS[2], ARGV[3])
    return 1
end

local receipt, receiptError = decodeReceipt(currentRaw, executionID)
if receiptError then return redis.error_reply("ERR " .. receiptError) end
if receipt.protection == nil then
    -- Legacy receipts carry no equivalent completion/retention proof. The
    -- recovery is durably acknowledged, but their receipt and guard stay
    -- persistent indefinitely.
    redis.call("HDEL", KEYS[1], field)
    redis.call("HDEL", KEYS[2], ARGV[3])
    return 1
end

local member = false
local memberIndex = false
for index, id in ipairs(receipt.protection.transactions) do
    if id == transactionID and receipt.protection.recoveryFields[index] == field then
        member = true
        memberIndex = index
    end
end
if not member then return redis.error_reply("ERR recovery is not a member of its receipt") end

local completedEvidence, completedIndex
if receipt.protection.formatVersion == 2 then
    if ARGV[8] ~= "0" or receipt.protection.indexFields[memberIndex] ~= transactionID then
        return redis.error_reply("ERR invalid indexed recovery source")
    end
    local envelopeDecoded, envelope = pcall(cjson.decode, expected)
    if not envelopeDecoded or type(envelope) ~= "table" or envelope.formatVersion ~= 1 or
        envelope.applicationState ~= "confirmed" or envelope.replayState ~= "reconstructible" or
        envelope.durabilityState ~= "pending" or type(envelope.record) ~= "table" or
        envelope.record.formatVersion ~= 2 or envelope.record.tenantId ~= receipt.tenantId or
        envelope.record.organizationId ~= receipt.organizationId or envelope.record.ledgerId ~= receipt.ledgerId or
        envelope.record.transactionId ~= transactionID or envelope.record.executionId ~= executionID then
        return redis.error_reply("ERR invalid write-behind recovery envelope")
    end
    local rawIndex = redis.call("HGET", KEYS[8], transactionID)
    if not rawIndex then return redis.error_reply("ERR transaction evidence index is missing") end
    local indexDecoded, index = pcall(cjson.decode, rawIndex)
    if not indexDecoded or type(index) ~= "table" or index.formatVersion ~= 1 or
        index.tenantId ~= receipt.tenantId or index.organizationId ~= receipt.organizationId or
        index.ledgerId ~= receipt.ledgerId or index.transactionId ~= transactionID or
        index.applicationState ~= "confirmed" or
        (index.replayState ~= "reconstructible" and index.replayState ~= "materialized") or
        (index.durabilityState ~= "pending" and index.durabilityState ~= "complete") or
        type(index.dependencies) ~= "table" then
        return redis.error_reply("ERR transaction evidence index differs")
    end
    completedEvidence = markDurabilityComplete(expected)
    if not completedEvidence then
        return redis.error_reply("ERR write-behind recovery envelope durability marker differs")
    end
    if index.executionId == executionID then
        if index.recoveryField ~= field or index.receiptField ~= executionID or
            index.replayState ~= "reconstructible" or index.durabilityState ~= "pending" then
            return redis.error_reply("ERR current transaction evidence index differs")
        end
        completedIndex = markDurabilityComplete(rawIndex)
        if not completedIndex then
            return redis.error_reply("ERR transaction evidence index durability marker differs")
        end
    else
        local protectsPredecessor = false
        for _, dependency in ipairs(index.dependencies) do
            if type(dependency) == "table" and dependency.kind == "predecessor" and
                dependency.tenantId == receipt.tenantId and dependency.organizationId == receipt.organizationId and
                dependency.ledgerId == receipt.ledgerId and dependency.transactionId == transactionID and
                dependency.executionId == executionID then
                protectsPredecessor = true
            end
        end
        if not protectsPredecessor then
            return redis.error_reply("ERR delayed recovery is not protected by current index")
        end
    end
end

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
local currentExecutionBatchReady = false
for linkedExecution, linked in pairs(receipts) do
	local ready, batchReady, terminalAt = true, true, 0
	for _, id in ipairs(linked.protection.transactions) do
		local completed = linked.protection.terminalCompletedAtMs[id]
		if linked.protection.acknowledged[id] ~= true then
			batchReady = false
		end
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
	if linkedExecution == executionID then
		currentExecutionReady = ready
		currentExecutionBatchReady = batchReady
	end
end

local batchPayload
local batchRetentionSeconds
if batchMode then
    if receipt.organizationId ~= ARGV[12] or receipt.ledgerId ~= ARGV[13] then
        return redis.error_reply("ERR atomic batch receipt scope differs")
    end

    local indexTarget = redis.call("GET", KEYS[10])
    if not indexTarget or indexTarget ~= KEYS[9] then
        return redis.error_reply("ERR atomic batch execution index differs")
    end
    local currentBatchRaw = redis.call("GET", KEYS[9])
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
		if currentExecutionBatchReady then
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
    redis.call("SET", KEYS[9], batchPayload, "EX", batchRetentionSeconds)
    redis.call("SET", KEYS[10], KEYS[9], "EX", batchRetentionSeconds)
end
if completedEvidence then
    redis.call("HSET", KEYS[7], field, completedEvidence)
    if completedIndex then redis.call("HSET", KEYS[8], transactionID, completedIndex) end
end
redis.call("HDEL", KEYS[1], field)
redis.call("HDEL", KEYS[2], ARGV[3])
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
