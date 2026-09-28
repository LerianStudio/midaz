-- Copyright (c) 2026 Lerian Studio. All rights reserved.
-- Use of this source code is governed by the Elastic License 2.0
-- that can be found in the LICENSE file.

-- KEYS: due schedule, legacy backup hash, engine recover hash, receipt hash,
--       guard hash, protection hash, protected evidence hash, transaction
--       index hash, followed by one materialized transaction key per member.
--       Scoped receipts then add guard, protection, evidence, and index hashes
--       for each member in the same order.
-- ARGV: schedule member, exact score, current unix millis, tenant ID,
--       organization UUID, ledger UUID, execution UUID.
if #KEYS < 8 or #ARGV ~= 7 then
    return redis.error_reply("ERR invalid engine recovery cleanup arguments")
end

local function redisType(key)
    local result = redis.call("TYPE", key)
    if type(result) == "table" then return result.ok end
    return result
end

local scheduleKind = redisType(KEYS[1])
if scheduleKind ~= "none" and scheduleKind ~= "zset" then
    return redis.error_reply("WRONGTYPE engine recovery cleanup schedule must be a sorted set")
end
for index = 2, 8 do
    local kind = redisType(KEYS[index])
    if kind ~= "none" and kind ~= "hash" then
        return redis.error_reply("WRONGTYPE engine recovery cleanup artifacts must be hashes")
    end
end
local member, expectedScore = ARGV[1], tonumber(ARGV[2])
local nowMS = tonumber(ARGV[3])
local tenantID, organizationID, ledgerID, executionID = ARGV[4], ARGV[5], ARGV[6], ARGV[7]
if not expectedScore or expectedScore < 1 or expectedScore % 1 ~= 0 or
    not nowMS or nowMS < 1 or nowMS % 1 ~= 0 then
    return redis.error_reply("ERR invalid engine recovery cleanup time")
end

local currentScore = redis.call("ZSCORE", KEYS[1], member)
if not currentScore or tonumber(currentScore) ~= expectedScore or expectedScore > nowMS then return 0 end

local rawReceipt = redis.call("HGET", KEYS[4], executionID)
if not rawReceipt then
    redis.call("ZREM", KEYS[1], member)
    return 2
end

local ok, receipt = pcall(cjson.decode, rawReceipt)
if not ok or type(receipt) ~= "table" then
    return redis.error_reply("ERR invalid cleanup receipt JSON")
end
if receipt.formatVersion ~= 1 or receipt.tenantId ~= tenantID or
    receipt.organizationId ~= organizationID or receipt.ledgerId ~= ledgerID or
    receipt.executionId ~= executionID then
    return redis.error_reply("ERR cleanup receipt scope differs")
end

local protection = receipt.protection
if protection == nil then
    redis.call("ZREM", KEYS[1], member)
    return 2
end
if type(protection) ~= "table" or (protection.formatVersion ~= 1 and protection.formatVersion ~= 2) or
    type(protection.retentionSeconds) ~= "number" or protection.retentionSeconds < 1 or
    protection.retentionSeconds > 604800 or protection.retentionSeconds % 1 ~= 0 or
    type(protection.transactions) ~= "table" or type(protection.recoveryFields) ~= "table" or
    type(protection.acknowledged) ~= "table" or type(protection.terminalCompletedAtMs) ~= "table" or
    type(protection.cleanupAfterMs) ~= "number" or protection.cleanupAfterMs < 1 or
    protection.cleanupAfterMs % 1 ~= 0 or #protection.transactions == 0 or
    #protection.transactions ~= #protection.recoveryFields or
    (protection.formatVersion == 2 and (type(protection.indexFields) ~= "table" or
     #protection.indexFields ~= #protection.transactions)) then
    return redis.error_reply("ERR invalid cleanup receipt protection")
end

local scoped = protection.scopes ~= nil
if scoped and (protection.formatVersion ~= 2 or type(protection.scopes) ~= "table" or
    #protection.scopes ~= #protection.transactions) then
    return redis.error_reply("ERR invalid cleanup receipt scopes")
end
if protection.formatVersion == 2 and #KEYS ~= 8 + #protection.transactions * (scoped and 5 or 1) then
    return redis.error_reply("ERR invalid cleanup receipt keys")
end
for index = 9, 8 + (protection.formatVersion == 2 and #protection.transactions or 0) do
    local kind = redisType(KEYS[index])
    if kind ~= "none" and kind ~= "string" then
        return redis.error_reply("WRONGTYPE materialized transaction artifact must be a string")
    end
end
if scoped then
    for index, part in ipairs(protection.scopes) do
        if type(part) ~= "table" or type(part.organizationId) ~= "string" or part.organizationId == "" or
            type(part.ledgerId) ~= "string" or part.ledgerId == "" then
            return redis.error_reply("ERR invalid cleanup transaction scope")
        end
        for offset = 1, 4 do
            local kind = redisType(KEYS[8 + #protection.transactions + 4 * (index - 1) + offset])
            if kind ~= "none" and kind ~= "hash" then
                return redis.error_reply("WRONGTYPE scoped cleanup artifact must be a hash")
            end
        end
    end
end

local function partKeys(index)
    if not scoped then return 5, 6, 7, 8, organizationID, ledgerID end
    local start = 8 + #protection.transactions + 4 * (index - 1)
    local part = protection.scopes[index]
    return start + 1, start + 2, start + 3, start + 4, part.organizationId, part.ledgerId
end

local seen, coordinators, latestTerminalAt = {}, {}, 0
local indexDeletes, dependencyBlocked = {}, false
for index, transactionID in ipairs(protection.transactions) do
    local guardKey, protectionKey, evidenceKey, indexKey, partOrganizationID, partLedgerID = partKeys(index)
    local recoveryField = protection.recoveryFields[index]
    local terminalAt = protection.terminalCompletedAtMs[transactionID]
    if type(transactionID) ~= "string" or transactionID == "" or seen[transactionID] or
        recoveryField ~= transactionID .. ":" .. executionID or
        protection.acknowledged[transactionID] ~= true or type(terminalAt) ~= "number" or
        terminalAt < 1 or terminalAt % 1 ~= 0 then
        return redis.error_reply("ERR incomplete cleanup receipt proof")
    end
    if redis.call("HEXISTS", KEYS[2], recoveryField) == 1 or
        redis.call("HEXISTS", KEYS[3], recoveryField) == 1 then
        return redis.error_reply("ERR cleanup recovery member still exists")
    end
    if terminalAt > latestTerminalAt then latestTerminalAt = terminalAt end
    seen[transactionID] = true

    if protection.formatVersion == 2 then
        if protection.indexFields[index] ~= transactionID then
            return redis.error_reply("ERR cleanup index protection differs")
        end
        local rawEvidence = redis.call("HGET", KEYS[evidenceKey], recoveryField)
        if not rawEvidence then return redis.error_reply("ERR cleanup evidence is missing") end
        local evidenceDecoded, evidence = pcall(cjson.decode, rawEvidence)
        if not evidenceDecoded or type(evidence) ~= "table" or evidence.formatVersion ~= 1 or
            evidence.applicationState ~= "confirmed" or evidence.durabilityState ~= "complete" or
            type(evidence.record) ~= "table" or evidence.record.formatVersion ~= 2 or
            evidence.record.tenantId ~= tenantID or evidence.record.organizationId ~= partOrganizationID or
            evidence.record.ledgerId ~= partLedgerID or evidence.record.transactionId ~= transactionID or
            evidence.record.executionId ~= executionID then
            return redis.error_reply("ERR cleanup evidence differs")
        end

        local rawIndex = redis.call("HGET", KEYS[indexKey], transactionID)
        if not rawIndex then return redis.error_reply("ERR cleanup transaction index is missing") end
        local indexDecoded, currentIndex = pcall(cjson.decode, rawIndex)
        if not indexDecoded or type(currentIndex) ~= "table" or currentIndex.formatVersion ~= 1 or
            currentIndex.tenantId ~= tenantID or currentIndex.organizationId ~= partOrganizationID or
            currentIndex.ledgerId ~= partLedgerID or currentIndex.transactionId ~= transactionID or
            type(currentIndex.executionId) ~= "string" or type(currentIndex.dependencies) ~= "table" then
            return redis.error_reply("ERR cleanup transaction index differs")
        end
        if currentIndex.executionId == executionID then
            if currentIndex.durabilityState ~= "complete" or currentIndex.recoveryField ~= recoveryField or
                currentIndex.receiptField ~= executionID then
                return redis.error_reply("ERR cleanup current transaction index is not durable")
            end
            indexDeletes[#indexDeletes + 1] = { key = indexKey, field = transactionID }
        else
            for _, dependency in ipairs(currentIndex.dependencies) do
                if type(dependency) == "table" and dependency.executionId == executionID and
                    dependency.transactionId == transactionID and dependency.kind == "predecessor" and
                    currentIndex.durabilityState == "pending" then
                    dependencyBlocked = true
                end
            end
        end
    end

    local rawCoordinator = redis.call("HGET", KEYS[protectionKey], transactionID)
    if not rawCoordinator then return redis.error_reply("ERR cleanup coordinator missing") end
    local decoded, coordinator = pcall(cjson.decode, rawCoordinator)
    if not decoded or type(coordinator) ~= "table" or coordinator.formatVersion ~= 1 or
        type(coordinator.executions) ~= "table" or
        coordinator.executions[executionID] ~= protection.cleanupAfterMs then
        return redis.error_reply("ERR cleanup coordinator differs")
    end

    coordinator.executions[executionID] = nil
    local remaining, allRemainingReady, remainingDeadline = false, true, 0
    for linkedExecution, deadline in pairs(coordinator.executions) do
        if type(linkedExecution) ~= "string" or linkedExecution == "" or
            type(deadline) ~= "number" or deadline < 0 or deadline % 1 ~= 0 then
            return redis.error_reply("ERR invalid linked cleanup coordinator")
        end
        remaining = true
        if deadline < 1 then
            allRemainingReady = false
        elseif deadline > remainingDeadline then
            remainingDeadline = deadline
        end
    end
    if remaining then
        if allRemainingReady then
            coordinator.cleanupAfterMs = remainingDeadline
        else
            coordinator.cleanupAfterMs = nil
        end
        coordinators[#coordinators + 1] = { guardKey = guardKey, protectionKey = protectionKey, field = transactionID, value = cjson.encode(coordinator) }
    else
        coordinators[#coordinators + 1] = { guardKey = guardKey, protectionKey = protectionKey, field = transactionID, value = false }
    end
end

local recomputedDeadline = latestTerminalAt + protection.retentionSeconds * 1000
if protection.cleanupAfterMs ~= recomputedDeadline then
    return redis.error_reply("ERR cleanup receipt deadline differs from terminal proof")
end

if dependencyBlocked then
    redis.call("ZADD", KEYS[1], nowMS + 60000, member)
    return 3
end

if protection.cleanupAfterMs > nowMS then
    redis.call("ZADD", KEYS[1], protection.cleanupAfterMs, member)
    return 3
end

-- Every receipt, recovery, and coordinator check plus every replacement JSON
-- encoding succeeded before the first artifact mutation.
redis.call("HDEL", KEYS[4], executionID)
if protection.formatVersion == 2 then
    for index, recoveryField in ipairs(protection.recoveryFields) do
        local _, _, evidenceKey = partKeys(index)
        redis.call("HDEL", KEYS[evidenceKey], recoveryField)
        local materializedRaw = redis.call("GET", KEYS[8 + index])
        if materializedRaw then
            local decoded, materialized = pcall(cjson.decode, materializedRaw)
            if decoded and type(materialized) == "table" and materialized.formatVersion == 1 and
                materialized.executionId == executionID then
                redis.call("DEL", KEYS[8 + index])
            end
        end
    end
    for _, item in ipairs(indexDeletes) do redis.call("HDEL", KEYS[item.key], item.field) end
end
for _, coordinator in ipairs(coordinators) do
    if coordinator.value then
        redis.call("HSET", KEYS[coordinator.protectionKey], coordinator.field, coordinator.value)
    else
        redis.call("HDEL", KEYS[coordinator.protectionKey], coordinator.field)
        redis.call("HDEL", KEYS[coordinator.guardKey], coordinator.field)
    end
end
redis.call("ZREM", KEYS[1], member)
return 1
