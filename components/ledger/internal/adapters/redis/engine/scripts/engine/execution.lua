-- prepareExecutionProtection establishes replay and concurrency safety before
-- reading balances. It returns a saved response when this exact execution is
-- complete; otherwise it validates guards and prepares coordinator updates in memory.
local function prepareExecutionProtection(request)
    expectRedisType(KEYS[3], "hash")
    local replay = storedReceipt(request)
    if replay then return replay end

    -- A replay needs only the receipt hash. A new execution validates every
    -- shared key type before it can calculate or publish accounting state.
    expectRedisType(KEYS[1], "zset")
    expectRedisType(KEYS[2], "hash")
    expectRedisType(KEYS[4], "hash")
    local protectionKey = KEYS[5]
    expectRedisType(protectionKey, "hash")

    -- Guard comparison prevents competing lifecycle transitions. Existing
    -- recovery without a receipt means a prior outcome cannot be safely replayed.
    local preparedProtection = {}
    for _, transaction in ipairs(request.transactions) do
        local current = redis.call("HGET", KEYS[4], transaction.guardField)
        if (current or "") ~= transaction.expectedGuard then
            technical("execution_guard_conflict", "transaction execution guard has changed")
        end
        if redis.call("HEXISTS", KEYS[2], transaction.recoveryField) == 1 then
            technical("execution_outcome_unknown", "recovery exists without a complete execution receipt")
        end
        -- Extend the transaction coordinator in memory. It is written only after
        -- all request, balance, calculation, and serialization work succeeds.
        local rawCoordinator = redis.call("HGET", protectionKey, transaction.id)
        local coordinator
        if rawCoordinator then
            coordinator = decodeJSON(rawCoordinator)
            requireObject(coordinator)
            requireObject(coordinator.executions)
            if smallInteger(coordinator.formatVersion, 1) ~= 1 then
                technical("invalid_protocol", "invalid transaction protection coordinator")
            end
        else
            coordinator = { formatVersion = 1, executions = object() }
        end
        coordinator.executions[request.executionId] = 0
        preparedProtection[#preparedProtection + 1] = {
            field = transaction.id,
            value = encodeJSON(coordinator)
        }
    end

    return nil, preparedProtection, protectionKey
end

-- loadBalancePool resolves the authoritative live accounting state. Redis values
-- supersede request seeds after identity validation; seeds are used only for cache
-- misses. Deletion markers and overdraft companion balances are captured together.
local function loadBalancePool(request)
    local pool, companions = {}, {}
    local normalization = array()
    for i, balance in ipairs(request.balances) do
        local keyIndex, markerIndex, legacyMarkerIndex = 3 + 3 * i, 4 + 3 * i, 5 + 3 * i
        expectRedisType(KEYS[keyIndex], "string")
        expectRedisType(KEYS[markerIndex], "string")
        expectRedisType(KEYS[legacyMarkerIndex], "string")
        local raw = redis.call("GET", KEYS[keyIndex])
        local blob, current
        if raw then
            blob = decodeJSON(raw)
            local repair
            current, repair = decodeBalance(blob, balance.snapshot, balance.balanceRef)
            if repair then normalization[#normalization + 1] = KEYS[keyIndex] end
        else
            -- A cache miss is seeded only in working memory. The balance is not
            -- published until the complete execution reaches the commit phase.
            current = clone(balance.snapshot)
            current.balanceRef = balance.balanceRef
        end
        local item = {
            current = current, blob = blob, keyIndex = keyIndex,
            deleted = redis.call("EXISTS", KEYS[markerIndex], KEYS[legacyMarkerIndex]) > 0
        }
        pool[balance.balanceRef] = item
        -- Index the single internal overdraft companion for later draw or repay
        -- movements generated from a primary account posting.
        if current.key == "overdraft" then
            if companions[current.accountId] then technical("invalid_balance", "multiple overdraft companions for one account") end
            companions[current.accountId] = item
        end
    end
    -- Limit repair is a separate precommit operation. Mixing repair with a money
    -- mutation would make the execution result and retry boundary ambiguous.
    if #normalization > 0 then error({ kind = "normalization", keys = normalization }, 0) end

    return pool, companions
end

-- validateLiveBalanceAvailability rejects every requirement or posting that
-- targets a balance protected by either deletion marker, or by the live
-- account-block control when the lifecycle action requires it. These checks use
-- live Redis state inside the same atomic execution as the eventual mutation.
local function validateLiveBalanceAvailability(request, pool)
    for txIndex, transaction in ipairs(request.transactions) do
        for _, requirement in ipairs(transaction.balanceRequirements) do
            if pool[requirement.balanceRef].deleted then
                refuse("balance_deleted", txIndex - 1, -1, requirement.balanceRef)
            end
            if transaction.rejectBlockedBalances and pool[requirement.balanceRef].current.blocked then
                refuse("account_blocked", txIndex - 1, -1, requirement.balanceRef)
            end
        end
        for postingIndex, posting in ipairs(transaction.postings) do
            if pool[posting.balanceRef].deleted then
                refuse("balance_deleted", txIndex - 1, postingIndex - 1, posting.balanceRef)
            end
            if transaction.rejectBlockedBalances and pool[posting.balanceRef].current.blocked then
                refuse("account_blocked", txIndex - 1, postingIndex - 1, posting.balanceRef)
            end
        end
    end
end

-- applyTransactionsInMemory evaluates ordered transactions against a shared
-- working pool without issuing Redis writes. Later transactions observe state
-- produced by earlier transactions in the same execution.
local function applyTransactionsInMemory(request, pool, companions)
    local movements, touched, touchedSet, transactionResults = array(), {}, {}, {}
    -- touch repeats deletion and account-block protection at the exact mutation
    -- site, including companion movements generated internally rather than
    -- declared as postings.
    local function touch(item, txIndex, postingIndex, rejectBlockedBalances)
        if item.deleted then refuse("balance_deleted", txIndex, postingIndex, item.current.balanceRef) end
        if rejectBlockedBalances and item.current.blocked then
            refuse("account_blocked", txIndex, postingIndex, item.current.balanceRef)
        end
    end

    for txIndex, transaction in ipairs(request.transactions) do
        local txMovements, txTouched, txTouchedSet = array(), {}, {}
        -- record materializes one real state transition. No-op calculations do
        -- not create movements or versions; changed balances advance exactly once
        -- and are tracked globally and for the current transaction.
        local function record(item, nextState, posting, role, postingType, amount, delta)
            local previous = item.current
            if cmp_decimal(previous.available, nextState.available) == 0 and cmp_decimal(previous.onHold, nextState.onHold) == 0 and cmp_decimal(previous.overdraftUsed, nextState.overdraftUsed) == 0 then return end
            if previous.version == "9223372036854775807" then technical("version_overflow", "balance version cannot advance") end
            nextState.version = add_decimal(previous.version, "1")
            local movement = {
                ref = transaction.id .. ":" .. #posting.ref .. ":" .. posting.ref .. ":" .. role .. ":0",
                transactionId = transaction.id, postingRef = posting.ref, role = role,
                balanceRef = previous.balanceRef, type = postingType, amount = amount,
                overdraftDelta = delta, before = state(previous, false), after = state(nextState, false)
            }
            movements[#movements + 1], txMovements[#txMovements + 1] = movement, movement
            item.current = nextState
            if not touchedSet[item] then touchedSet[item], touched[#touched + 1] = true, item end
            if not txTouchedSet[item] then txTouchedSet[item], txTouched[#txTouched + 1] = true, item end
        end
        -- Validate nonmonetary requirements against the live working state before
        -- applying any posting belonging to this transaction.
        for _, requirement in ipairs(transaction.balanceRequirements) do
            local current = pool[requirement.balanceRef].current
            if current.assetCode ~= requirement.assetCode then
                refuse("asset_mismatch", txIndex - 1, -1, requirement.balanceRef)
            end
            if requirement.permission == "send" and not current.allowSending then
                refuse("sending_not_allowed", txIndex - 1, -1, requirement.balanceRef)
            end
            if requirement.permission == "receive" and not current.allowReceiving then
                refuse("receiving_not_allowed", txIndex - 1, -1, requirement.balanceRef)
            end
            if requirement.forbidExternal and current.accountType == "external" then
                refuse("external_hold_not_allowed", txIndex - 1, -1, requirement.balanceRef)
            end
        end
        -- Apply the closed posting algebra first, then resolve any debt created or
        -- repaid by that primary transition.
        for postingIndex, posting in ipairs(transaction.postings) do
            local item = pool[posting.balanceRef]
            touch(item, txIndex - 1, postingIndex - 1, transaction.rejectBlockedBalances)
            local current, nextState = item.current, clone(item.current)
            local external = current.accountType == "external"
            local amount = posting.amount
            local postingType = posting.type
            local primaryAmount, delta = postingAlgebra[postingType](current, nextState, posting, txIndex - 1, postingIndex - 1)
            -- A negative internal available value is either a deterministic
            -- refusal or an authorized overdraft draw. The primary balance never
            -- persists negative: authorized debt moves into overdraftUsed.
            if cmp_decimal(nextState.available, "0") < 0 and not external then
                if postingType == "hold" or posting.drawPolicy == "forbidden" or current.direction ~= "credit" or not current.allowOverdraft then
                    refuse("insufficient_funds", txIndex - 1, postingIndex - 1, posting.balanceRef)
                end
                if posting.drawPolicy == "route_denied" then
                    refuse("overdraft_not_eligible", txIndex - 1, postingIndex - 1, posting.balanceRef)
                end
                if postingType ~= "debit" then technical("invalid_balance", "unexpected debt-producing posting") end
                local draw = sub_decimal("0", nextState.available)
                nextState.overdraftUsed = add_decimal(current.overdraftUsed, draw)
                if current.overdraftLimitEnabled and cmp_decimal(nextState.overdraftUsed, current.overdraftLimit) > 0 then
                    refuse("overdraft_limit_exceeded", txIndex - 1, postingIndex - 1, posting.balanceRef)
                end
                nextState.available, primaryAmount, delta = "0", sub_decimal(amount, draw), draw
            end

            -- Mirror every overdraft draw or repayment on the account's dedicated
            -- companion balance so both sides of the debt remain explicit.
            local companion, companionNext, companionAmount, companionType
            if cmp_decimal(delta, "0") ~= 0 then
                companion = companions[current.accountId]
                if not companion then refuse("overdraft_companion_missing", txIndex - 1, postingIndex - 1, posting.balanceRef) end
                if companion == item or companion.current.direction ~= "debit" or companion.current.balanceScope ~= "internal" or companion.current.accountType == "external" or companion.current.assetCode ~= current.assetCode then
                    technical("invalid_companion", "invalid overdraft companion")
                end
                touch(companion, txIndex - 1, postingIndex - 1, transaction.rejectBlockedBalances)
                companionNext = clone(companion.current)
                if cmp_decimal(delta, "0") > 0 then
                    companionAmount, companionType = delta, "debit"
                    companionNext.available = add_decimal(companion.current.available, companionAmount)
                else
                    companionAmount, companionType = sub_decimal("0", delta), "credit"
                    if cmp_decimal(companion.current.available, companionAmount) < 0 then
                        refuse("insufficient_funds", txIndex - 1, postingIndex - 1, companion.current.balanceRef)
                    end
                    companionNext.available = sub_decimal(companion.current.available, companionAmount)
                end
            end
            record(item, nextState, posting, "primary", postingType, primaryAmount, delta)
            if companion then record(companion, companionNext, posting, "overdraft_companion", companionType, companionAmount, "0") end
        end
        -- Freeze the state reached by this transaction for its correlated recovery
        -- record before a later transaction can mutate the shared working pool.
        local txFinal = array()
        for _, item in ipairs(txTouched) do txFinal[#txFinal + 1] = snapshotCopy(item.current, false) end
        transactionResults[#transactionResults + 1] = { movements = txMovements, final = txFinal }
    end

    return movements, touched, transactionResults
end

-- prepareExecutionWrites serializes every value and accounts for its byte cost
-- before the first Redis write. This keeps all predictable allocation, encoding,
-- and size failures on the safe precommit side of the execution boundary.
local function prepareExecutionWrites(request, maximumPrepared, preparedProtection, movements, touched, transactionResults)
    local final = array()
    for _, item in ipairs(touched) do final[#final + 1] = snapshotCopy(item.current, false) end
    local response = encodeJSON({ protocolVersion = 1, movements = movements, final = final })
    local preparedBytes = #response
    if preparedBytes > maximumPrepared then technical("prepared_bytes_exceeded", "response exceeds prepared byte budget") end
    -- A true no-op has no state to protect or recover and therefore publishes no
    -- balance, guard, recovery, coordinator, or receipt writes.
    if #movements == 0 then return response, nil end

    -- charge accumulates every prepared string against one global response and
    -- persistence budget before that string may reach a Redis command.
    local function charge(value)
        if #value > maximumPrepared - preparedBytes then technical("prepared_bytes_exceeded", "execution exceeds prepared byte budget") end
        preparedBytes = preparedBytes + #value
        return value
    end
    for _, coordinator in ipairs(preparedProtection) do
        charge(coordinator.field)
        charge(coordinator.value)
    end
    -- Encode the final cache documents from the fully evaluated working state.
    local preparedBalances, preparedRecoverRecords = {}, {}
    for _, item in ipairs(touched) do
        preparedBalances[#preparedBalances + 1] = { key = KEYS[item.keyIndex], value = charge(encodeBalance(item)) }
    end
    -- Build one immutable recovery envelope per transaction, using numeric JSON
    -- version tokens only in the persisted evidence format.
    for i, transaction in ipairs(request.transactions) do
        local txResult = transactionResults[i]
        local recoveryMovements, recoveryFinal = array(), array()
        for _, movement in ipairs(txResult.movements) do
            local saved = clone(movement)
            saved.before, saved.after = clone(movement.before), clone(movement.after)
            saved.before.version, saved.after.version = numberToken(movement.before.version), numberToken(movement.after.version)
            recoveryMovements[#recoveryMovements + 1] = saved
        end
        for _, snapshot in ipairs(txResult.final) do recoveryFinal[#recoveryFinal + 1] = snapshotCopy(snapshot, true) end
        preparedRecoverRecords[#preparedRecoverRecords + 1] = {
            field = transaction.recoveryField,
            value = charge(encodeJSON({
                formatVersion = 2, tenantId = request.tenantId, organizationId = request.organizationId,
                ledgerId = request.ledgerId, executionId = request.executionId,
                intentFingerprint = request.intentFingerprint, transactionId = transaction.id,
                payload = transaction.completionPlan, result = { movements = recoveryMovements, final = recoveryFinal }
            }))
        }
    end
    -- The receipt records the exact replay response and the ordered artifacts that
    -- must remain protected until durable completion and retention are satisfied.
    local protectedTransactions, protectedRecoveryFields = array(), array()
    for _, transaction in ipairs(request.transactions) do
        protectedTransactions[#protectedTransactions + 1] = transaction.id
        protectedRecoveryFields[#protectedRecoveryFields + 1] = transaction.recoveryField
    end
    local receipt = charge(encodeJSON({
        formatVersion = 1, tenantId = request.tenantId, organizationId = request.organizationId,
        ledgerId = request.ledgerId, executionId = request.executionId,
        intentFingerprint = request.intentFingerprint, response = response,
        protection = {
            formatVersion = 1, retentionSeconds = request.retentionSeconds,
            transactions = protectedTransactions, recoveryFields = protectedRecoveryFields,
            acknowledged = object(), terminalCompletedAtMs = object()
        }
    }))
    for _, transaction in ipairs(request.transactions) do
        charge(transaction.guardField)
        charge(transaction.nextGuard)
        charge(transaction.recoveryField)
    end

    return response, preparedBalances, preparedRecoverRecords, receipt
end

-- commitPreparedExecution is the only money-changing publication phase. Every
-- argument has already been validated and serialized. Once commitStarted is set,
-- any unexpected failure is indeterminate because Redis does not roll writes back.
local function commitPreparedExecution(request, protectionKey, preparedBalances, preparedRecoverRecords, preparedProtection, receipt)
    local now = redis.call("TIME")
    local score = tonumber(now[1]) + tonumber(now[2]) / 1000000

    -- Only prepared commands remain. Runtime failures here are indeterminate;
    -- Redis script execution does not roll back earlier successful writes.
    commitStarted = true
    -- Publish live balances first, then the synchronization and recovery evidence,
    -- lifecycle guards, and cleanup coordinators. The receipt is written last so
    -- its presence proves that the complete prepared command sequence ran.
    for _, balance in ipairs(preparedBalances) do redis.call("SET", balance.key, balance.value, "EX", balance_cache_ttl_seconds) end
    for _, balance in ipairs(preparedBalances) do redis.call("ZADD", KEYS[1], score, balance.key) end
    for _, recoverRecord in ipairs(preparedRecoverRecords) do redis.call("HSET", KEYS[2], recoverRecord.field, recoverRecord.value) end
    for _, transaction in ipairs(request.transactions) do redis.call("HSET", KEYS[4], transaction.guardField, transaction.nextGuard) end
    for _, coordinator in ipairs(preparedProtection) do redis.call("HSET", protectionKey, coordinator.field, coordinator.value) end
    redis.call("HSET", KEYS[3], request.receiptField, receipt)
end

-- execute tells the complete engine story: replay or protect, load live balances,
-- evaluate in memory, prepare every output, and finally publish the prepared state.
local function execute(request, maximumPrepared)
    local replay, preparedProtection, protectionKey = prepareExecutionProtection(request)
    if replay then return replay end

    local pool, companions = loadBalancePool(request)
    validateLiveBalanceAvailability(request, pool)

    local movements, touched, transactionResults = applyTransactionsInMemory(request, pool, companions)
    local response, preparedBalances, preparedRecoverRecords, receipt = prepareExecutionWrites(
        request, maximumPrepared, preparedProtection, movements, touched, transactionResults
    )
    if not preparedBalances then return response end

    commitPreparedExecution(
        request, protectionKey, preparedBalances, preparedRecoverRecords, preparedProtection, receipt
    )
    return response
end
