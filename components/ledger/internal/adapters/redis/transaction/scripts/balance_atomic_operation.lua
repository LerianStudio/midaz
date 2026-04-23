local function split_decimal(s)
    local sign = ""

    if s:sub(1, 1) == "-" then
        sign = "-"
        s = s:sub(2)
    end

    local intp, fracp = s:match("^(%d+)%.(%d+)$")
    if intp then
        return sign .. intp, fracp, sign ~= ""
    else
        return sign .. s, "", sign ~= ""
    end
end

local function rtrim_zeros(frac)
    frac = frac:gsub("0+$", "")
    return (frac == "" and "0") or frac
end

local sub_decimal

local function add_decimal(a, b)
    a = tostring(a)
    b = tostring(b)
    local ai, af, a_negative = split_decimal(a)
    local bi, bf, b_negative = split_decimal(b)

    if a_negative and b_negative then
        local result = add_decimal(a:sub(2), b:sub(2))
        return "-" .. result
    end

    if a_negative then
        return sub_decimal(b, a:sub(2))
    end

    if b_negative then
        return sub_decimal(a, b:sub(2))
    end

    if ai:sub(1, 1) == "-" then ai = ai:sub(2) end
    if bi:sub(1, 1) == "-" then bi = bi:sub(2) end

    if #af < #bf then
        af = af .. string.rep("0", #bf - #af)
    elseif #bf < #af then
        bf = bf .. string.rep("0", #af - #bf)
    end

    local carry = 0
    local frac_sum = {}
    for i = #af, 1, -1 do
        local da = tonumber(af:sub(i, i))
        local db = tonumber(bf:sub(i, i))
        local s = da + db + carry
        carry = math.floor(s / 10)
        frac_sum[#af - i + 1] = tostring(s % 10)
    end

    local rii = ai:reverse()
    local rbi = bi:reverse()
    local max_i = math.max(#rii, #rbi)
    local int_sum = {}
    for i = 1, max_i do
        local da = tonumber(rii:sub(i, i)) or 0
        local db = tonumber(rbi:sub(i, i)) or 0
        local s = da + db + carry
        carry = math.floor(s / 10)
        int_sum[i] = tostring(s % 10)
    end
    if carry > 0 then
        int_sum[#int_sum + 1] = tostring(carry)
    end

    local int_res = table.concat(int_sum):reverse()
    local frac_res = table.concat(frac_sum):reverse()
    frac_res = rtrim_zeros(frac_res)

    if frac_res == "0" then
        return int_res
    end
    return int_res .. "." .. frac_res
end

sub_decimal = function(a, b)
    a = tostring(a)
    b = tostring(b)
    local ai, af, a_negative = split_decimal(a)
    local bi, bf, b_negative = split_decimal(b)

    if a_negative and b_negative then
        return sub_decimal(b:sub(2), a:sub(2))
    end

    if a_negative then
        local result = add_decimal(a:sub(2), b)
        return "-" .. result
    end

    if b_negative then
        return add_decimal(a, b:sub(2))
    end

    local a_num = tonumber(a)
    local b_num = tonumber(b)
    if a_num < b_num then
        local result = sub_decimal(b, a)
        return "-" .. result
    end

    if ai:sub(1, 1) == "-" then ai = ai:sub(2) end
    if bi:sub(1, 1) == "-" then bi = bi:sub(2) end

    if #af < #bf then
        af = af .. string.rep("0", #bf - #af)
    elseif #bf < #af then
        bf = bf .. string.rep("0", #af - #bf)
    end

    local borrow = 0
    local frac_res_tbl = {}
    for i = #af, 1, -1 do
        local da = tonumber(af:sub(i, i))
        local db = tonumber(bf:sub(i, i))
        local diff = da - db - borrow
        if diff < 0 then
            diff = diff + 10
            borrow = 1
        else
            borrow = 0
        end
        frac_res_tbl[#af - i + 1] = tostring(diff)
    end

    local rii = ai:reverse()
    local rbi = bi:reverse()
    local max_i = math.max(#rii, #rbi)
    local int_res_tbl = {}
    for i = 1, max_i do
        local da = tonumber(rii:sub(i, i)) or 0
        local db = tonumber(rbi:sub(i, i)) or 0
        local diff = da - db - borrow
        if diff < 0 then
            diff = diff + 10
            borrow = 1
        else
            borrow = 0
        end
        int_res_tbl[i] = tostring(diff)
    end

    local res_int_rev = table.concat(int_res_tbl)
    local res_int = res_int_rev:reverse():gsub("^0+", "")
    if res_int == "" then
        res_int = "0"
    end

    local frac_normal = table.concat(frac_res_tbl):reverse()
    frac_normal = rtrim_zeros(frac_normal)

    if frac_normal == "0" then
        return res_int
    end
    return res_int .. "." .. frac_normal
end

local function startsWithMinus(s)
    return s:sub(1, 1) == "-"
end

-- isPositive checks if a decimal string represents a value greater than zero
-- Returns true if the value is positive (not negative and not zero)
local function isPositive(s)
    if startsWithMinus(s) then
        return false
    end
    -- Check if it's zero (could be "0", "0.0", "0.00", etc.)
    local normalized = s:gsub("%.?0+$", ""):gsub("^0+", "")
    return normalized ~= "" and normalized ~= "."
end

local function cloneBalance(tbl)
    local copy = {}
    for k, v in pairs(tbl) do
        copy[k] = v
    end
    return copy
end

-- min_decimal returns the smaller of two decimal strings. Implemented via
-- sub_decimal so the caller does not need a numeric coercion step — this
-- keeps the precision behavior identical to add_decimal/sub_decimal for
-- values that overflow Lua's double representation.
local function min_decimal(a, b)
    if startsWithMinus(sub_decimal(a, b)) then
        return a
    end
    return b
end

local function updateTransactionHash(transactionBackupQueue, transactionKey, balances, balancesAfter)
    local transaction

    local raw = redis.call("HGET", transactionBackupQueue, transactionKey)
    if not raw then
        transaction = { balances = balances, balancesAfter = balancesAfter }
    else
        local ok, decoded = pcall(cjson.decode, raw)
        if ok and type(decoded) == "table" then
            transaction = decoded
            transaction.balances = balances
            transaction.balancesAfter = balancesAfter
        else
            transaction = { balances = balances, balancesAfter = balancesAfter }
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

    local groupSize = 23
    local returnBalances = {}
    local returnBalancesAfter = {}
    local rollbackBalances = {}

    local transactionBackupQueue = KEYS[1]
    local transactionKey = KEYS[2]
    local scheduleKey = KEYS[3]

    -- Schedule balance sync immediately (eligible for worker pickup right away).
    -- The worker uses a dual-trigger (size OR timeout) to batch multiple keys
    -- before flushing to PostgreSQL, so immediate eligibility does not mean
    -- immediate DB write — the worker accumulates keys efficiently.
    --
    -- Uses fractional-second precision (seconds + microseconds / 1e6) to prevent
    -- the conditional ZREM from removing entries re-scheduled in the same second.
    -- Fractional seconds keep scores in the ~1e9 range (valid Unix timestamps),
    -- ensuring rollback compatibility with versions that interpret scores as seconds.
    local timeNow = redis.call("TIME")
    local dueAt = tonumber(timeNow[1]) + tonumber(timeNow[2]) / 1000000

    for i = 1, #ARGV, groupSize do
        local redisBalanceKey = ARGV[i]
        local isPending = tonumber(ARGV[i + 1])
        local transactionStatus = ARGV[i + 2]
        local operation = ARGV[i + 3]

        local amount = ARGV[i + 4]

        local alias = ARGV[i + 5]

        local routeValidationEnabled = tonumber(ARGV[i + 6])

        -- Balance object stored in Redis cache.
        --
        -- FIELD USAGE:
        -- Fields used by THIS Lua script for atomic operations:
        --   - ID:          Returned to Go for operation tracking
        --   - Available:   Balance calculations (DEBIT/CREDIT)
        --   - OnHold:      Balance calculations (ON_HOLD/RELEASE)
        --   - Version:     Optimistic concurrency control
        --   - AccountType: Validation 0018 (external account cannot have positive balance)
        --   - AccountID:   Returned to Go for operation tracking
        --   - Direction:   Direction-aware overdraft gating ("credit" vs "debit")
        --   - OverdraftUsed:        Tracked when a debit exceeds Available
        --   - AllowOverdraft:       Enables negative-result pass-through
        --   - OverdraftLimitEnabled:Gates the OverdraftLimit check
        --   - OverdraftLimit:       Hard cap on OverdraftUsed (when enabled)
        --   - BalanceScope:         "transactional" / "internal" (cache-only)
        --
        -- Fields NOT used by Lua, but required in cache for Go pre-validation:
        --   - AssetCode:      Used by ValidateIfBalanceExistsOnRedis for validation 0034
        --   - AllowSending:   Used by ValidateIfBalanceExistsOnRedis for validation 0024
        --   - AllowReceiving: Used by ValidateIfBalanceExistsOnRedis for validation 0024
        --   - Key:            Used by ValidateIfBalanceExistsOnRedis for balance identification
        --
        -- WARNING: Do NOT remove the "cache-only" fields. They are essential for the
        -- transaction validation flow that reads balances from cache before calling Lua.
        -- See: get-balances.go ValidateIfBalanceExistsOnRedis()
        local balance = {
            -- Fields used by Lua
            ID = ARGV[i + 7],
            Available = ARGV[i + 8],
            OnHold = ARGV[i + 9],
            Version = tonumber(ARGV[i + 10]),
            AccountType = ARGV[i + 11],
            AccountID = ARGV[i + 12],
            -- Fields for cache only (used by Go pre-validation, not by Lua)
            AssetCode = ARGV[i + 13],
            AllowSending = tonumber(ARGV[i + 14]),
            AllowReceiving = tonumber(ARGV[i + 15]),
            Key = ARGV[i + 16],
            -- Overdraft fields
            Direction = ARGV[i + 17],
            OverdraftUsed = ARGV[i + 18],
            AllowOverdraft = tonumber(ARGV[i + 19]),
            OverdraftLimitEnabled = tonumber(ARGV[i + 20]),
            OverdraftLimit = ARGV[i + 21],
            BalanceScope = ARGV[i + 22],
        }

        -- Preserve the Go-provided version before the cache may overwrite it.
        -- Used for stale-version detection on overdraft-relevant operations.
        local incomingVersion = balance.Version

        local redisBalance = cjson.encode(balance)
        local ok = redis.call("SET", redisBalanceKey, redisBalance, "EX", ttl, "NX")
        if not ok then
            local currentBalance = redis.call("GET", redisBalanceKey)
            if not currentBalance then
                return redis.error_reply("0061")
            end
            balance = cjson.decode(currentBalance)

            -- Backwards compatibility: legacy cache entries may lack the new
            -- overdraft fields. Fill in safe defaults so subsequent logic does
            -- not reference nil values.
            if balance.Direction == nil then
                balance.Direction = ""
            end
            if balance.OverdraftUsed == nil then
                balance.OverdraftUsed = "0"
            end
            if balance.AllowOverdraft == nil then
                balance.AllowOverdraft = 0
            end
            if balance.OverdraftLimitEnabled == nil then
                balance.OverdraftLimitEnabled = 0
            end
            if balance.OverdraftLimit == nil then
                balance.OverdraftLimit = "0"
            end
            if balance.BalanceScope == nil then
                balance.BalanceScope = "transactional"
            end
        end

        -- Capture pre-operation OverdraftUsed so hasChange can detect repayment
        -- or accrual even when Available/OnHold remain unchanged.
        local originalOverdraftUsed = balance.OverdraftUsed

        if not rollbackBalances[redisBalanceKey] then
            rollbackBalances[redisBalanceKey] = cjson.encode(balance)
        end

        local result = balance.Available
        local resultOnHold = balance.OnHold

        -- Direction-aware arithmetic on Available:
        -- For direction=debit balances (e.g., overdraft tracking), DEBIT
        -- increases Available and CREDIT decreases it. For direction=credit
        -- balances (and empty/legacy), DEBIT decreases and CREDIT increases.
        -- OnHold semantics are direction-agnostic: holds always add, releases
        -- always subtract, regardless of balance direction.
        local isDebitDirection = (balance.Direction == "debit")

        if isPending == 1 then
            if operation == "DEBIT" and transactionStatus == "PENDING" and routeValidationEnabled == 1 then
                -- Double-entry: DEBIT only updates Available.
                -- The OnHold++ will be a separate ON_HOLD operation.
                if isDebitDirection then
                    result = add_decimal(balance.Available, amount)
                else
                    result = sub_decimal(balance.Available, amount)
                end
            elseif operation == "ON_HOLD" and transactionStatus == "PENDING" and routeValidationEnabled == 1 then
                -- Double-entry: ON_HOLD only increments OnHold.
                -- The Available-- was already done by the separate DEBIT operation.
                resultOnHold = add_decimal(balance.OnHold, amount)
            elseif operation == "ON_HOLD" and transactionStatus == "PENDING" then
                if isDebitDirection then
                    result = add_decimal(balance.Available, amount)
                else
                    result = sub_decimal(balance.Available, amount)
                end
                resultOnHold = add_decimal(balance.OnHold, amount)
            elseif operation == "RELEASE" and transactionStatus == "CANCELED" and routeValidationEnabled == 1 then
                -- Double-entry: RELEASE only decrements OnHold.
                -- The Available++ will be a separate CREDIT operation.
                resultOnHold = sub_decimal(balance.OnHold, amount)
            elseif operation == "RELEASE" and transactionStatus == "CANCELED" then
                resultOnHold = sub_decimal(balance.OnHold, amount)
                if isDebitDirection then
                    result = sub_decimal(balance.Available, amount)
                else
                    result = add_decimal(balance.Available, amount)
                end
            elseif operation == "CREDIT" and transactionStatus == "CANCELED" and routeValidationEnabled == 1 then
                -- Double-entry: CREDIT updates Available only.
                if isDebitDirection then
                    result = sub_decimal(balance.Available, amount)
                else
                    result = add_decimal(balance.Available, amount)
                end
            elseif operation == "ON_HOLD" and transactionStatus == "APPROVED" and routeValidationEnabled == 1 then
                -- Double-entry: ON_HOLD in APPROVED only decrements OnHold.
                -- The Available++ will be a separate CREDIT operation.
                resultOnHold = sub_decimal(balance.OnHold, amount)
            elseif transactionStatus == "APPROVED" then
                if operation == "DEBIT" then
                    resultOnHold = sub_decimal(balance.OnHold, amount)
                else
                    if isDebitDirection then
                        result = sub_decimal(balance.Available, amount)
                    else
                        result = add_decimal(balance.Available, amount)
                    end
                end
            end
        else
            if isDebitDirection then
                if operation == "DEBIT" then
                    result = add_decimal(balance.Available, amount)
                else
                    result = sub_decimal(balance.Available, amount)
                end
            else
                if operation == "DEBIT" then
                    result = sub_decimal(balance.Available, amount)
                else
                    result = add_decimal(balance.Available, amount)
                end
            end
        end


        -- newOverdraftUsed holds the post-operation OverdraftUsed candidate.
        -- It is written back to `balance.OverdraftUsed` only AFTER the
        -- pre-mutation snapshot is cloned into `returnBalances`, so the
        -- "before" snapshot faithfully reflects the state the caller read.
        local newOverdraftUsed = balance.OverdraftUsed

        -- Overdraft repayment: a credit that arrives on a direction=credit
        -- balance with outstanding OverdraftUsed repays the overdraft first
        -- before growing Available. This mirrors the debit path in reverse:
        -- the deficit is paid down, and only the remainder increases the
        -- account holder's spendable balance.
        --
        -- The companion balance's Available is decremented in lock-step by a
        -- sibling CREDIT op queued by the Go enrichment layer
        -- (transaction_overdraft_enrichment.go:buildCompanionCreditOp). The
        -- stale-version check below keeps Go's repayAmount in sync with
        -- Lua's authoritative decrement — if they disagree (e.g. a
        -- concurrent transaction already reduced OverdraftUsed), the whole
        -- batch rolls back with 0175 so the caller re-reads state and
        -- retries with a consistent split.
        if operation == "CREDIT" and not isDebitDirection and
            balance.AccountType ~= "external" and
            isPositive(balance.OverdraftUsed) then
            if balance.Version ~= incomingVersion then
                rollback(rollbackBalances, ttl)
                return redis.error_reply("0175")
            end

            local repay = min_decimal(amount, balance.OverdraftUsed)

            newOverdraftUsed = sub_decimal(balance.OverdraftUsed, repay)
            -- `result` was already computed as balance.Available + amount
            -- by the direction-aware block above. Subtract the repayment
            -- portion to expose just the remainder that flows into
            -- Available; the repayment itself leaves Available untouched
            -- because it goes toward paying down OverdraftUsed.
            result = sub_decimal(result, repay)
        end

        if startsWithMinus(result) and balance.AccountType ~= "external" then
            -- Direction-aware overdraft: credit-direction balances with
            -- AllowOverdraft=1 may go temporarily negative. The shortfall
            -- is floored at zero in Available and accrued in OverdraftUsed,
            -- subject to OverdraftLimit when enabled.
            --
            -- Debit-direction balances and credit-direction balances without
            -- AllowOverdraft fall through to the legacy 0018 rejection.
            if balance.Direction == "credit" and (balance.AllowOverdraft or 0) == 1 then
                -- deficit = abs(result). Because result is negative,
                -- sub_decimal("0", result) produces the absolute value.
                local deficit = sub_decimal("0", result)

                -- Stale-version check: if Go's pre-computed split assumed an
                -- older OverdraftUsed/Available, the floor math will be wrong.
                -- Reject so the caller can re-read and retry (Phase 1 behavior;
                -- retry is added in Phase 2).
                if balance.Version ~= incomingVersion then
                    rollback(rollbackBalances, ttl)
                    return redis.error_reply("0175")
                end

                -- Compute the candidate OverdraftUsed locally; the limit
                -- check below operates on this candidate so the "before"
                -- snapshot remains untouched.
                newOverdraftUsed = add_decimal(balance.OverdraftUsed, deficit)

                if (balance.OverdraftLimitEnabled or 0) == 1 then
                    -- sub_decimal(limit, newOverdraftUsed) is negative iff
                    -- the candidate strictly exceeds limit. Equal is allowed
                    -- (at-limit).
                    if startsWithMinus(sub_decimal(balance.OverdraftLimit, newOverdraftUsed)) then
                        rollback(rollbackBalances, ttl)
                        return redis.error_reply("0167")
                    end
                end

                result = "0"
            else
                rollback(rollbackBalances, ttl)
                return redis.error_reply("0018")
            end
        end

        -- External accounts cannot have positive balance (they represent debt to external entities)
        -- This validation MUST be atomic to prevent race conditions where two concurrent
        -- transactions both credit an external account
        if operation == "CREDIT" and balance.AccountType == "external" and isPositive(result) then
            rollback(rollbackBalances, ttl)
            return redis.error_reply("0018")
        end

        -- Only update balance and increment version if there was an actual change.
        -- This prevents version gaps when destinations are processed during PENDING
        -- transactions (CREDIT + PENDING has no effect, so no version increment).
        --
        -- OverdraftUsed is included so pure overdraft accrual paths (Available
        -- floored at 0, OverdraftUsed incremented) still mark the balance as
        -- changed and trigger a cache + sync update. We compare against
        -- `newOverdraftUsed` (the candidate computed above) rather than the
        -- still-original `balance.OverdraftUsed` to detect overdraft deltas.
        local hasChange = (result ~= balance.Available)
            or (resultOnHold ~= balance.OnHold)
            or (newOverdraftUsed ~= originalOverdraftUsed)

        if hasChange then
            balance.Alias = alias
            -- Snapshot the pre-mutation state first so the "before" payload
            -- reflects what the caller read (especially OverdraftUsed).
            table.insert(returnBalances, cloneBalance(balance))

            balance.Available = result
            balance.OnHold = resultOnHold
            balance.OverdraftUsed = newOverdraftUsed
            balance.Version = balance.Version + 1

            table.insert(returnBalancesAfter, cloneBalance(balance))

            redisBalance = cjson.encode(balance)
            redis.call("SET", redisBalanceKey, redisBalance, "EX", ttl)

            redis.call("ZADD", scheduleKey, dueAt, redisBalanceKey)
        end
    end

    -- Handle empty array case: cjson encodes {} as object, but Go expects array
    -- When no changes occurred, use cjson.decode("[]") to get proper array type
    -- for both the transaction hash and the return value
    if #returnBalances == 0 then
        local emptyArray = cjson.decode("[]")
        updateTransactionHash(transactionBackupQueue, transactionKey, emptyArray, emptyArray)
        return cjson.encode({ before = cjson.decode("[]"), after = cjson.decode("[]") })
    end

    updateTransactionHash(transactionBackupQueue, transactionKey, returnBalances, returnBalancesAfter)

    return cjson.encode({ before = returnBalances, after = returnBalancesAfter })
end

return main()