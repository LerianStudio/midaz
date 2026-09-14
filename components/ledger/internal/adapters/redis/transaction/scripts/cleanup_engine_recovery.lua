-- Copyright (c) 2026 Lerian Studio. All rights reserved.
-- Use of this source code is governed by the Elastic License 2.0
-- that can be found in the LICENSE file.

-- KEYS: due schedule, legacy backup hash, engine recover hash, receipt hash,
--       guard hash, protection hash.
-- ARGV: schedule member, exact score, current unix millis, tenant ID,
--       organization UUID, ledger UUID, execution UUID.
if #KEYS ~= 6 or #ARGV ~= 7 then
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
for index = 2, #KEYS do
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
if type(protection) ~= "table" or protection.formatVersion ~= 1 or
    type(protection.retentionSeconds) ~= "number" or protection.retentionSeconds < 1 or
    protection.retentionSeconds > 604800 or protection.retentionSeconds % 1 ~= 0 or
    type(protection.transactions) ~= "table" or type(protection.recoveryFields) ~= "table" or
    type(protection.acknowledged) ~= "table" or type(protection.terminalCompletedAtMs) ~= "table" or
    type(protection.cleanupAfterMs) ~= "number" or protection.cleanupAfterMs < 1 or
    protection.cleanupAfterMs % 1 ~= 0 or #protection.transactions == 0 or
    #protection.transactions ~= #protection.recoveryFields then
    return redis.error_reply("ERR invalid cleanup receipt protection")
end

local seen, coordinators, latestTerminalAt = {}, {}, 0
for index, transactionID in ipairs(protection.transactions) do
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

    local rawCoordinator = redis.call("HGET", KEYS[6], transactionID)
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
        coordinators[#coordinators + 1] = { field = transactionID, value = cjson.encode(coordinator) }
    else
        coordinators[#coordinators + 1] = { field = transactionID, value = false }
    end
end

local recomputedDeadline = latestTerminalAt + protection.retentionSeconds * 1000
if protection.cleanupAfterMs ~= recomputedDeadline then
    return redis.error_reply("ERR cleanup receipt deadline differs from terminal proof")
end

if protection.cleanupAfterMs ~= expectedScore then
    redis.call("ZADD", KEYS[1], protection.cleanupAfterMs, member)
    return 3
end

-- Every receipt, recovery, and coordinator check plus every replacement JSON
-- encoding succeeded before the first artifact mutation.
redis.call("HDEL", KEYS[4], executionID)
for _, coordinator in ipairs(coordinators) do
    if coordinator.value then
        redis.call("HSET", KEYS[6], coordinator.field, coordinator.value)
    else
        redis.call("HDEL", KEYS[6], coordinator.field)
        redis.call("HDEL", KEYS[5], coordinator.field)
    end
end
redis.call("ZREM", KEYS[1], member)
return 1
