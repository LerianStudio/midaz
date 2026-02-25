-- Copyright (c) 2026 Lerian Studio. All rights reserved.
-- Use of this source code is governed by the Elastic License 2.0
-- that can be found in the LICENSE file.

-- balance_atomic_operation_v2.lua
--
-- Integer arithmetic version of the atomic balance operation script.
-- All monetary amounts are pre-scaled integers (e.g., USD cents, BTC satoshis).
-- This eliminates ~150 lines of string-based decimal arithmetic from v1,
-- reducing Lua execution time from ~200μs to ~30-50μs per transaction.
--
-- KEYS:
--   KEYS[1] = transactionBackupQueue (Redis hash for crash recovery)
--   KEYS[2] = transactionKey (hash field for this transaction)
--   KEYS[3] = scheduleKey (sorted set for balance sync scheduling)
--
-- ARGV layout:
--   ARGV[1] = scheduleSync (0 = disabled, 1 = enabled)
--   ARGV[2] = scale (integer: number of decimal places, e.g., 2 for USD, 8 for BTC)
--   ARGV[3..] = balance operations in groups of 16:
--     [i+0]  redisBalanceKey (string)
--     [i+1]  isPending (0/1)
--     [i+2]  transactionStatus (CREATED/PENDING/APPROVED/CANCELED)
--     [i+3]  operation (DEBIT/CREDIT/ON_HOLD/RELEASE)
--     [i+4]  amount (integer: pre-scaled)
--     [i+5]  alias (string)
--     [i+6]  ID (string: balance UUID)
--     [i+7]  Available (integer: pre-scaled)
--     [i+8]  OnHold (integer: pre-scaled)
--     [i+9]  Version (integer)
--     [i+10] AccountType (string)
--     [i+11] AccountID (string: UUID)
--     [i+12] AssetCode (string)
--     [i+13] AllowSending (0/1)
--     [i+14] AllowReceiving (0/1)
--     [i+15] Key (string: balance key)

-- Maximum safe integer for Lua 5.1 float64 arithmetic (2^53)
local MAX_SAFE_INT = 9007199254740992

local function roundToInt(value)
    if value >= 0 then
        return math.floor(value + 0.5)
    end

    return math.ceil(value - 0.5)
end

local function rescaleValue(value, fromScale, toScale)
    local n = tonumber(value) or 0
    local targetScale = tonumber(toScale) or 0
    local sourceScale = tonumber(fromScale)

    local result

    -- Legacy payloads (scale missing) stored unscaled decimal units.
    -- Convert to the target integer scale expected by v2 arithmetic.
    if sourceScale == nil then
        result = roundToInt(n * (10 ^ targetScale))
    elseif sourceScale == targetScale then
        result = roundToInt(n)
    elseif sourceScale < targetScale then
        result = roundToInt(n * (10 ^ (targetScale - sourceScale)))
    else
        result = roundToInt(n / (10 ^ (sourceScale - targetScale)))
    end

    -- Overflow guard: reject if rescaled value exceeds float64 safe integer boundary (2^53).
    -- Mirrors the Go-side check in ScaleToInt (pkg/transaction/scale.go).
    -- Returns (value, overflowed) so the caller can trigger rollback + error_reply.
    if result > MAX_SAFE_INT or result < -MAX_SAFE_INT then
        return nil, true
    end

    return result, false
end

local function toLegacyAmount(value, scale)
    if scale == nil or scale <= 0 then
        return value
    end

    return value / (10 ^ scale)
end

local function toLegacyBalance(balance, scale)
    return {
        id = balance.ID,
        alias = balance.Alias,
        accountId = balance.AccountID,
        assetCode = balance.AssetCode,
        available = toLegacyAmount(balance.Available, scale),
        onHold = toLegacyAmount(balance.OnHold, scale),
        version = balance.Version,
        accountType = balance.AccountType,
        allowSending = balance.AllowSending,
        allowReceiving = balance.AllowReceiving,
        key = balance.Key,
    }
end

local function updateTransactionHash(transactionBackupQueue, transactionKey, balances)
    local transaction

    local raw = redis.call("HGET", transactionBackupQueue, transactionKey)
    if not raw then
        transaction = { balances = balances }
    else
        local ok, decoded = pcall(cjson.decode, raw)
        if ok and type(decoded) == "table" then
            transaction = decoded
            transaction.balances = balances
        else
            transaction = { balances = balances }
        end
    end

    local updated = cjson.encode(transaction)
    redis.call("HSET", transactionBackupQueue, transactionKey, updated)

    return updated
end

local function rollback(rollbackBalances, ttl)
    if next(rollbackBalances) then
        local msetArgs = {}
        for key, value in pairs(rollbackBalances) do
            table.insert(msetArgs, key)
            table.insert(msetArgs, value)
        end
        redis.call("MSET", unpack(msetArgs))

        for key, _ in pairs(rollbackBalances) do
            redis.call("EXPIRE", key, ttl)
        end
    end
end

local function main()
    local ttl = 3600 -- 1 hour

    local groupSize = 16 -- MUST match luaOperationArgGroupSize in consumer.redis.go
    local returnBalances = {}
    local rollbackBalances = {}

    local transactionBackupQueue = KEYS[1]
    local transactionKey = KEYS[2]
    local scheduleKey = KEYS[3]

    -- ARGV[1]: whether to schedule balance sync (1 = enabled, 0 = disabled)
    local scheduleSync = tonumber(ARGV[1]) or 1

    -- ARGV[2]: scale factor (number of decimal places)
    local scale = tonumber(ARGV[2]) or 0

    -- Schedule a pre-expire warning 10 minutes before the TTL
    local warnBefore = 600 -- 10 minutes; must match balanceSyncWarnBeforeSeconds in consumer.redis.go
    local timeNow = redis.call("TIME")
    local nowSec = tonumber(timeNow[1])
    local dueAt = nowSec + (ttl - warnBefore)

    -- Start from index 3 since ARGV[1]=scheduleSync, ARGV[2]=scale
    for i = 3, #ARGV, groupSize do
        local redisBalanceKey = ARGV[i]
        local isPending = tonumber(ARGV[i + 1])
        local transactionStatus = ARGV[i + 2]
        local operation = ARGV[i + 3]

        -- Pre-scaled integer amount from Go
        local amount = tonumber(ARGV[i + 4])

        local alias = ARGV[i + 5]

        -- Balance object for Redis cache.
        -- Available and OnHold are pre-scaled integers from Go.
        local balance = {
            ID = ARGV[i + 6],
            Available = tonumber(ARGV[i + 7]),
            OnHold = tonumber(ARGV[i + 8]),
            Version = tonumber(ARGV[i + 9]),
            AccountType = ARGV[i + 10],
            AccountID = ARGV[i + 11],
            AssetCode = ARGV[i + 12],
            AllowSending = tonumber(ARGV[i + 13]),
            AllowReceiving = tonumber(ARGV[i + 14]),
            Key = ARGV[i + 15],
        }

        balance.Alias = alias

        local redisBalance = cjson.encode(toLegacyBalance(balance, scale))
        local rollbackValue = redisBalance
        local ok = redis.call("SET", redisBalanceKey, redisBalance, "EX", ttl, "NX")
        if not ok then
            local currentBalance = redis.call("GET", redisBalanceKey)
            if not currentBalance then
                return redis.error_reply("0061")
            end
            rollbackValue = currentBalance
            balance = cjson.decode(currentBalance)
            local currentScale = tonumber(balance.Scale)
            if currentScale == nil then
                currentScale = tonumber(balance.scale)
            end

            local availableValue = balance.Available
            if availableValue == nil then
                availableValue = balance.available
            end

            local onHoldValue = balance.OnHold
            if onHoldValue == nil then
                onHoldValue = balance.onHold
            end

            local availRescaled, availOverflow = rescaleValue(availableValue, currentScale, scale)
            if availOverflow then
                rollback(rollbackBalances, ttl)
                return redis.error_reply("0142")
            end

            local holdRescaled, holdOverflow = rescaleValue(onHoldValue, currentScale, scale)
            if holdOverflow then
                rollback(rollbackBalances, ttl)
                return redis.error_reply("0142")
            end

            balance.Available = availRescaled
            balance.OnHold = holdRescaled
            balance.Version = tonumber(balance.Version) or tonumber(balance.version) or 0
            if balance.ID == nil then
                balance.ID = balance.id
            end
            if balance.Alias == nil then
                balance.Alias = balance.alias
            end
            if balance.AccountType == nil then
                balance.AccountType = balance.accountType
            end
            if balance.AccountID == nil then
                balance.AccountID = balance.accountId
            end
            if balance.AssetCode == nil then
                balance.AssetCode = balance.assetCode
            end
            if balance.AllowSending == nil then
                balance.AllowSending = tonumber(balance.allowSending) or 0
            end
            if balance.AllowReceiving == nil then
                balance.AllowReceiving = tonumber(balance.allowReceiving) or 0
            end
            if balance.Key == nil then
                balance.Key = balance.key
            end

            -- Remove legacy lowercase fields to avoid ambiguous JSON keys after re-encode.
            -- Keeping both (e.g., available + Available) can cause downstream decoding
            -- to read stale values depending on key matching order.
            balance.id = nil
            balance.alias = nil
            balance.accountId = nil
            balance.assetCode = nil
            balance.available = nil
            balance.onHold = nil
            balance.version = nil
            balance.accountType = nil
            balance.allowSending = nil
            balance.allowReceiving = nil
            balance.key = nil
            balance.scale = nil
        end

        if not rollbackBalances[redisBalanceKey] then
            rollbackBalances[redisBalanceKey] = rollbackValue
        end

        -- Native integer arithmetic — the entire point of v2.
        -- These are simple additions/subtractions on pre-scaled integers.
        -- No string parsing, no digit-by-digit loops, no decimal point handling.
        local avail = balance.Available
        local hold = balance.OnHold

        if isPending == 1 then
            if operation == "ON_HOLD" and transactionStatus == "PENDING" then
                avail = avail - amount
                hold = hold + amount
            elseif operation == "RELEASE" and transactionStatus == "CANCELED" then
                hold = hold - amount
                avail = avail + amount
            elseif transactionStatus == "APPROVED_COMPENSATE" then
                -- Compensation for pending transactions: reverses the arithmetic of all
                -- pending operation types. This handles compensation for any pending
                -- transaction status (PENDING, CANCELED, APPROVED) by undoing the
                -- original operation's effect without business rule checks.
                --
                -- DEBIT compensation: APPROVED flow did hold -= amount, so undo: hold += amount
                -- CREDIT compensation: APPROVED flow did avail += amount, so undo: avail -= amount
                -- ON_HOLD compensation: PENDING flow did avail -= amount, hold += amount, so undo both
                -- RELEASE compensation: CANCELED flow did hold -= amount, avail += amount, so undo both
                if operation == "DEBIT" then
                    hold = hold + amount
                elseif operation == "CREDIT" then
                    avail = avail - amount
                elseif operation == "ON_HOLD" then
                    avail = avail + amount
                    hold = hold - amount
                elseif operation == "RELEASE" then
                    hold = hold + amount
                    avail = avail - amount
                end
            elseif transactionStatus == "APPROVED" then
                if operation == "DEBIT" then
                    hold = hold - amount
                elseif operation == "RELEASE" then
                    hold = hold - amount
                    avail = avail + amount
                else
                    avail = avail + amount
                end
            end
        else
            if transactionStatus == "APPROVED_COMPENSATE" then
                -- Compensation for non-pending transactions: invert the arithmetic
                -- of the original operation without business rule checks.
                -- DEBIT's (avail - amount) is reversed to (avail + amount).
                -- CREDIT's (avail + amount) is reversed to (avail - amount).
                if operation == "DEBIT" then
                    avail = avail + amount
                else
                    avail = avail - amount
                end
            else
                if operation == "DEBIT" then
                    avail = avail - amount
                else
                    avail = avail + amount
                end
            end
        end

		-- Precision overflow guard: reject if result exceeds 2^53
		-- Error code 0142 = ErrPrecisionOverflow (NOT 0062 which is ErrNoAccountIDsProvided)
		if avail > MAX_SAFE_INT or avail < -MAX_SAFE_INT then
			rollback(rollbackBalances, ttl)
			return redis.error_reply("0142")
		end

		if hold > MAX_SAFE_INT or hold < -MAX_SAFE_INT then
			rollback(rollbackBalances, ttl)
			return redis.error_reply("0142")
		end

        -- Compensation bypasses business rule checks: when APPROVED_COMPENSATE is used,
        -- the operation is a system-initiated reversal that must always succeed regardless
        -- of current balance state. Without this skip, an intervening transaction could
        -- cause the compensation to hit insufficient-funds (0018), leaving the system
        -- in an inconsistent state.
        if transactionStatus ~= "APPROVED_COMPENSATE" then
            -- Validation: non-external accounts cannot have negative available balance
            if avail < 0 and balance.AccountType ~= "external" then
                rollback(rollbackBalances, ttl)
                return redis.error_reply("0018")
            end

            -- Validation: external accounts cannot have positive balance (they represent
            -- debt to external entities). This check MUST be atomic to prevent race
            -- conditions where two concurrent transactions both credit an external account.
            if operation == "CREDIT" and balance.AccountType == "external" and avail > 0 then
                rollback(rollbackBalances, ttl)
                return redis.error_reply("0018")
            end
        end

        -- Only update balance and increment version if there was an actual change.
        -- This prevents version gaps when destinations are processed during PENDING
        -- transactions (CREDIT + PENDING has no effect, so no version increment).
        local hasChange = (avail ~= balance.Available) or (hold ~= balance.OnHold)

        if hasChange then
            balance.Alias = alias
            table.insert(returnBalances, toLegacyBalance(balance, scale))

            balance.Available = avail
            balance.OnHold = hold
            balance.Version = balance.Version + 1

            redisBalance = cjson.encode(toLegacyBalance(balance, scale))
            redis.call("SET", redisBalanceKey, redisBalance, "EX", ttl)

            -- Only schedule balance sync if enabled (scheduleSync == 1)
            if scheduleSync == 1 then
                redis.call("ZADD", scheduleKey, dueAt, redisBalanceKey)
            end
        end
    end

    -- Handle empty array case: cjson encodes {} as object, but Go expects array
    if #returnBalances == 0 then
        local emptyArray = cjson.decode("[]")
        updateTransactionHash(transactionBackupQueue, transactionKey, emptyArray)
        return "[]"
    end

    updateTransactionHash(transactionBackupQueue, transactionKey, returnBalances)

    local returnBalancesEncoded = cjson.encode(returnBalances)
    return returnBalancesEncoded
end

return main()
