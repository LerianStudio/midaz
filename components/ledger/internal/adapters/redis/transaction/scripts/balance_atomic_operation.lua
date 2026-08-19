local function normalize_integer(intp)
    intp = intp:gsub("^0+", "")
    return (intp == "" and "0") or intp
end

local function trim_fraction(fracp)
    return fracp:gsub("0+$", "")
end

local function split_decimal(value)
    local s = tostring(value)
    local negative = false

    if s:sub(1, 1) == "-" then
        negative = true
        s = s:sub(2)
    end

    local intp, fracp = s:match("^(%d+)%.(%d+)$")
    if not intp then
        intp = s:match("^(%d+)$")
        fracp = ""
    end
    if not intp then
        error("invalid decimal string")
    end

    intp = normalize_integer(intp)
    fracp = trim_fraction(fracp)
    if intp == "0" and fracp == "" then
        negative = false
    end

    return intp, fracp, negative
end

local function format_decimal(intp, fracp, negative)
    intp = normalize_integer(intp)
    fracp = trim_fraction(fracp)

    if intp == "0" and fracp == "" then
        return "0"
    end

    local result = intp
    if fracp ~= "" then
        result = result .. "." .. fracp
    end
    if negative then
        result = "-" .. result
    end

    return result
end

local function decimal_digit(s, index)
    if index < 1 or index > #s then
        return 0
    end
    return string.byte(s, index) - string.byte("0")
end

local function compare_digit_strings(a, b)
    if #a < #b then
        return -1
    end
    if #a > #b then
        return 1
    end
    if a < b then
        return -1
    end
    if a > b then
        return 1
    end
    return 0
end

local function compare_absolute_parts(ai, af, bi, bf)
    local integer_comparison = compare_digit_strings(ai, bi)
    if integer_comparison ~= 0 then
        return integer_comparison
    end

    local scale = math.max(#af, #bf)
    for i = 1, scale do
        local a_digit = decimal_digit(af, i)
        local b_digit = decimal_digit(bf, i)
        if a_digit < b_digit then
            return -1
        end
        if a_digit > b_digit then
            return 1
        end
    end

    return 0
end

local function add_absolute(ai, af, bi, bf, negative)
    local scale = math.max(#af, #bf)
    af = af .. string.rep("0", scale - #af)
    bf = bf .. string.rep("0", scale - #bf)

    local carry = 0
    local frac_sum = {}
    for i = scale, 1, -1 do
        local sum = decimal_digit(af, i) + decimal_digit(bf, i) + carry
        carry = math.floor(sum / 10)
        frac_sum[scale - i + 1] = tostring(sum % 10)
    end

    local a_reverse = ai:reverse()
    local b_reverse = bi:reverse()
    local width = math.max(#a_reverse, #b_reverse)
    local int_sum = {}
    for i = 1, width do
        local sum = decimal_digit(a_reverse, i) + decimal_digit(b_reverse, i) + carry
        carry = math.floor(sum / 10)
        int_sum[i] = tostring(sum % 10)
    end
    if carry > 0 then
        int_sum[#int_sum + 1] = tostring(carry)
    end

    return format_decimal(table.concat(int_sum):reverse(), table.concat(frac_sum):reverse(), negative)
end

-- subtract_absolute assumes a >= b and performs one exact digit-wise borrow.
local function subtract_absolute(ai, af, bi, bf, negative)
    local scale = math.max(#af, #bf)
    af = af .. string.rep("0", scale - #af)
    bf = bf .. string.rep("0", scale - #bf)

    local borrow = 0
    local frac_result = {}
    for i = scale, 1, -1 do
        local difference = decimal_digit(af, i) - decimal_digit(bf, i) - borrow
        if difference < 0 then
            difference = difference + 10
            borrow = 1
        else
            borrow = 0
        end
        frac_result[scale - i + 1] = tostring(difference)
    end

    local a_reverse = ai:reverse()
    local b_reverse = bi:reverse()
    local int_result = {}
    for i = 1, math.max(#a_reverse, #b_reverse) do
        local difference = decimal_digit(a_reverse, i) - decimal_digit(b_reverse, i) - borrow
        if difference < 0 then
            difference = difference + 10
            borrow = 1
        else
            borrow = 0
        end
        int_result[i] = tostring(difference)
    end

    if borrow ~= 0 then
        error("invalid absolute decimal subtraction")
    end

    return format_decimal(table.concat(int_result):reverse(), table.concat(frac_result):reverse(), negative)
end

local function compare_decimal(a, b)
    local ai, af, a_negative = split_decimal(a)
    local bi, bf, b_negative = split_decimal(b)

    if a_negative ~= b_negative then
        return a_negative and -1 or 1
    end

    local absolute_comparison = compare_absolute_parts(ai, af, bi, bf)
    return a_negative and -absolute_comparison or absolute_comparison
end

local function add_decimal(a, b)
    local ai, af, a_negative = split_decimal(a)
    local bi, bf, b_negative = split_decimal(b)

    if a_negative == b_negative then
        return add_absolute(ai, af, bi, bf, a_negative)
    end

    local absolute_comparison = compare_absolute_parts(ai, af, bi, bf)
    if absolute_comparison == 0 then
        return "0"
    end
    if absolute_comparison > 0 then
        return subtract_absolute(ai, af, bi, bf, a_negative)
    end
    return subtract_absolute(bi, bf, ai, af, b_negative)
end

local function sub_decimal(a, b)
    local ai, af, a_negative = split_decimal(a)
    local bi, bf, b_negative = split_decimal(b)

    if a_negative ~= b_negative then
        return add_absolute(ai, af, bi, bf, a_negative)
    end

    local absolute_comparison = compare_absolute_parts(ai, af, bi, bf)
    if absolute_comparison == 0 then
        return "0"
    end
    if absolute_comparison > 0 then
        return subtract_absolute(ai, af, bi, bf, a_negative)
    end
    return subtract_absolute(bi, bf, ai, af, not a_negative)
end

-- isPositive checks if a decimal string represents a value greater than zero
-- Returns true if the value is positive (not negative and not zero)
local function isPositive(s)
    return compare_decimal(s, "0") > 0
end

local function cloneBalance(tbl)
    local copy = {}
    for k, v in pairs(tbl) do
        copy[k] = v
    end
    return copy
end

-- min_decimal returns the smaller of two exact decimal strings without
-- coercing either full value to Lua's binary floating-point number type.
local function min_decimal(a, b)
    if compare_decimal(a, b) < 0 then
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

    local groupSize = 24
    local returnBalances = {}
    local returnBalancesAfter = {}
    local rollbackBalances = {}

    local transactionBackupQueue = KEYS[1]
    local transactionKey = KEYS[2]
    local scheduleKey = KEYS[3]
    local argStart = 1
    local attemptOwner = nil
    local desiredOutcome = nil
    local transactionIdentity = nil
    local economicPlanVersion = nil
    local economicPlanDigest = nil
	local tracerOutcomeV2 = false
	local tracerOutcomeID = nil
	local tracerOutcomeState = nil
	local tracerOutcomeMember = nil
	local tracerOutcomeRecord = nil

    -- The reusable outcome protocol passes an owned attempt pair plus an
    -- immutable outcome key in the same {transactions} slot. A replay of the
    -- same outcome returns the exact original snapshots without moving funds;
    -- an opposite terminal outcome conflicts before the first balance write.
    if #KEYS >= 6 then
        attemptOwner = ARGV[1]
        desiredOutcome = ARGV[2]
        transactionIdentity = ARGV[3]
        argStart = 4

        if #KEYS == 8 or #KEYS == 9 then
			if ARGV[argStart] ~= "TRACER_OUTCOME_V2" then
				return redis.error_reply("TRACER_OUTCOME_INVALID")
			end
			tracerOutcomeV2 = true
			tracerOutcomeID = ARGV[argStart + 1]
			tracerOutcomeState = ARGV[argStart + 2]
			tracerOutcomeMember = ARGV[argStart + 3]
			argStart = argStart + 4
		end

        if #KEYS == 7 or #KEYS == 9 then
            if redis.call("GET", KEYS[#KEYS]) ~= ARGV[argStart] then
                return redis.error_reply("FINANCIAL_DATASET_GENERATION_MISMATCH")
            end
			argStart = argStart + 1
        end
    end

    if ARGV[argStart] == "EXPECTED_ECONOMIC_PLAN" then
        economicPlanVersion = ARGV[argStart + 1]
        economicPlanDigest = ARGV[argStart + 2]
        argStart = argStart + 3

        local rawEnvelope = redis.call("HGET", transactionBackupQueue, transactionKey)
        if not rawEnvelope then
            return redis.error_reply("EXPECTED_ECONOMIC_PLAN_MISSING")
        end
        local ok, envelope = pcall(cjson.decode, rawEnvelope)
        if not ok or type(envelope) ~= "table" or type(envelope.expected_economic_plan) ~= "table" or
           tostring(envelope.expected_economic_plan.version) ~= economicPlanVersion or
           envelope.expected_economic_plan.digest ~= economicPlanDigest then
            return redis.error_reply("EXPECTED_ECONOMIC_PLAN_MISMATCH")
        end
    end

	if tracerOutcomeV2 then
		local tracerRaw = redis.call("GET", KEYS[7])
		local tracerOK
		tracerOK, tracerOutcomeRecord = pcall(cjson.decode, tracerRaw or "")
		if not tracerOK or type(tracerOutcomeRecord) ~= "table" or
		   tracerOutcomeRecord.transaction_id ~= transactionIdentity or
		   tracerOutcomeRecord.outcome_id ~= tracerOutcomeID or
		   (tracerOutcomeRecord.state ~= "PENDING_HELD" and
		    (tostring(tracerOutcomeRecord.economic_plan_version) ~= economicPlanVersion or
		     tracerOutcomeRecord.economic_plan_digest ~= economicPlanDigest)) then
			return redis.error_reply("TRACER_OUTCOME_CONFLICT")
		end
		local validTransition =
			(tracerOutcomeRecord.state == "PREPARED" and tracerOutcomeRecord.owner == attemptOwner and
			 (tracerOutcomeState == "PENDING_HELD" or tracerOutcomeState == "COMMITTED" or tracerOutcomeState == "ABORTED")) or
			(tracerOutcomeRecord.state == "PENDING_HELD" and
			 (tracerOutcomeState == "COMMITTED" or tracerOutcomeState == "ABORTED")) or
			(tracerOutcomeRecord.state == tracerOutcomeState and tracerOutcomeRecord.owner == attemptOwner) or
			(tracerOutcomeRecord.state == "DELIVERED" and tracerOutcomeRecord.owner == attemptOwner and
			 type(tracerOutcomeRecord.economic_outcome) == "table" and
			 ((tracerOutcomeState == "COMMITTED" and tracerOutcomeRecord.economic_outcome.outcome == "COMMITTED") or
			  (tracerOutcomeState == "ABORTED" and tracerOutcomeRecord.economic_outcome.outcome == "ABORTED")))
		if not validTransition then
			return redis.error_reply("TRACER_OUTCOME_STALE_EXECUTOR")
		end
	end

    if #KEYS >= 6 then
        local existingRaw = redis.call("GET", KEYS[6])
		if existingRaw then
			local existing = cjson.decode(existingRaw)
			if existing.identity ~= transactionIdentity or existing.outcome ~= desiredOutcome then
				return redis.error_reply("0099")
			end
			if tracerOutcomeV2 and existing.owner ~= attemptOwner then
				return redis.error_reply("TRACER_OUTCOME_STALE_EXECUTOR")
			end
            if economicPlanDigest ~= nil and
               (tostring(existing.economic_plan_version) ~= economicPlanVersion or existing.economic_plan_digest ~= economicPlanDigest) then
                return redis.error_reply("EXPECTED_ECONOMIC_PLAN_MISMATCH")
            end

            return cjson.encode({ before = existing.before, after = existing.after })
        end

        if redis.call("EXISTS", KEYS[4]) ~= 1 or redis.call("GET", KEYS[5]) ~= attemptOwner then
            return redis.error_reply("0084")
        end
    end

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
	local dueAtMS = tonumber(timeNow[1]) * 1000 + math.floor(tonumber(timeNow[2]) / 1000)

	local function writeOutcome(before, after)
		local economicOutcome
		if #before == 0 and #after == 0 then
			economicOutcome = cjson.decode('{"before":[],"after":[]}')
		else
			economicOutcome = { before = before, after = after }
		end
		economicOutcome.identity = transactionIdentity
		economicOutcome.outcome = desiredOutcome
		economicOutcome.owner = attemptOwner
		economicOutcome.economic_plan_version = economicPlanVersion
		economicOutcome.economic_plan_digest = economicPlanDigest
		redis.call("SET", KEYS[6], cjson.encode(economicOutcome))
		if tracerOutcomeV2 then
			tracerOutcomeRecord.state = tracerOutcomeState
			tracerOutcomeRecord.owner = attemptOwner
			tracerOutcomeRecord.economic_plan_version = economicPlanVersion
			tracerOutcomeRecord.economic_plan_digest = economicPlanDigest
			tracerOutcomeRecord.updated_at_unix_ms = dueAtMS
			tracerOutcomeRecord.economic_outcome = economicOutcome
			redis.call("SET", KEYS[7], cjson.encode(tracerOutcomeRecord))
			if tracerOutcomeState == "PENDING_HELD" then
				redis.call("ZREM", KEYS[8], tracerOutcomeMember)
			else
				redis.call("ZADD", KEYS[8], dueAtMS, tracerOutcomeMember)
			end
		end
		redis.call("DEL", KEYS[4], KEYS[5])
	end

    -- Delete marker guard: reject the whole batch before any mutation if any balance
    -- in it carries a live deletion marker. The delete marker is a SEPARATE key
    -- "<balanceKey>:deleted" that never overwrites the balance itself, and it
    -- shares the balance key's {transactions} hash slot. Running this pre-pass
    -- ahead of the first SET below means a rejection here leaves zero side
    -- effects across the batch, so no rollback is required. The stride mirrors
    -- the main loop below (groupSize=24; ARGV[i] is the balance key). A bounded
    -- per-key EXISTS check early-returns on the first delete marker found, so the
    -- whole batch is rejected without unpacking a client-influenced number of keys.
    for i = argStart, #ARGV, groupSize do
        if redis.call("EXISTS", ARGV[i] .. ":deleted") == 1 then
            return redis.error_reply("0019")
        end
    end

    for i = argStart, #ARGV, groupSize do
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
        --   - AccountType: External-account carve-outs (skip overdraft/floor logic)
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

        -- Exact overdraft delta supplied by Go for pending-cancel reversals.
        -- Normal transaction paths pass zero and keep Lua's live-state split
        -- calculation authoritative.
        local overdraftAmount = ARGV[i + 23]

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
            elseif operation == "DEBIT" and transactionStatus == "PENDING" and isDebitDirection then
                -- Legacy pending overdraft companion: the user-facing source
                -- is an ON_HOLD, but the internal direction=debit companion
                -- receives a synthetic DEBIT so liability state matches the
                -- one-phase overdraft path.
                result = add_decimal(balance.Available, amount)
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
            elseif operation == "CREDIT" and transactionStatus == "CANCELED" and isDebitDirection then
                -- Legacy pending overdraft companion cancel: shrink the
                -- direction=debit liability that was created by the pending
                -- companion DEBIT.
                result = sub_decimal(balance.Available, amount)
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
        -- batch rolls back with 0174 so the caller re-reads state and
        -- retries with a consistent split.
        if operation == "CREDIT" and not isDebitDirection and
            balance.AccountType ~= "external" and
            isPositive(balance.OverdraftUsed) then
            local sameBatchCancelCredit = operation == "CREDIT" and transactionStatus == "CANCELED" and
                routeValidationEnabled == 1 and tonumber(balance.Version) == (tonumber(incomingVersion) + 1)
            if balance.Version ~= incomingVersion and not sameBatchCancelCredit then
                rollback(rollbackBalances, ttl)
                return redis.error_reply("0174")
            end

            local repay = min_decimal(amount, balance.OverdraftUsed)
            if isPositive(overdraftAmount) then
                repay = min_decimal(min_decimal(overdraftAmount, amount), balance.OverdraftUsed)
            end

            newOverdraftUsed = sub_decimal(balance.OverdraftUsed, repay)
            -- `result` was already computed as balance.Available + amount
            -- by the direction-aware block above. Subtract the repayment
            -- portion to expose just the remainder that flows into
            -- Available; the repayment itself leaves Available untouched
            -- because it goes toward paying down OverdraftUsed.
            result = sub_decimal(result, repay)
        end

        -- Legacy pending cancel reversal: RELEASE both clears OnHold and
        -- restores Available. If the original hold consumed overdraft, only
        -- the non-overdraft portion should return to Available and the exact
        -- overdraft delta must be removed from OverdraftUsed.
        if operation == "RELEASE" and transactionStatus == "CANCELED" and routeValidationEnabled == 0 and
            not isDebitDirection and balance.AccountType ~= "external" and isPositive(overdraftAmount) then
            if balance.Version ~= incomingVersion then
                rollback(rollbackBalances, ttl)
                return redis.error_reply("0174")
            end

            local repay = min_decimal(min_decimal(overdraftAmount, amount), balance.OverdraftUsed)
            newOverdraftUsed = sub_decimal(balance.OverdraftUsed, repay)
            result = sub_decimal(result, repay)
        end

        if compare_decimal(result, "0") < 0 and balance.AccountType ~= "external" then
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
                    return redis.error_reply("0174")
                end

                -- Compute the candidate OverdraftUsed locally; the limit
                -- check below operates on this candidate so the "before"
                -- snapshot remains untouched.
                newOverdraftUsed = add_decimal(balance.OverdraftUsed, deficit)

                if (balance.OverdraftLimitEnabled or 0) == 1 then
                    -- Equal is allowed; one decimal quantum above the limit
                    -- is rejected regardless of integer width or scale.
                    if compare_decimal(newOverdraftUsed, balance.OverdraftLimit) > 0 then
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
        if #KEYS >= 6 then
			writeOutcome(emptyArray, emptyArray)
        end
        return cjson.encode({ before = cjson.decode("[]"), after = cjson.decode("[]") })
    end

    updateTransactionHash(transactionBackupQueue, transactionKey, returnBalances, returnBalancesAfter)

    if #KEYS >= 6 then
		writeOutcome(returnBalances, returnBalancesAfter)
    end

    return cjson.encode({ before = returnBalances, after = returnBalancesAfter })
end

return main()
