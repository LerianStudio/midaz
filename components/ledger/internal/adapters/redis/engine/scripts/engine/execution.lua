-- validateIndexedDependency proves that a caller-supplied causal reference is
-- still the current indexed state and that both its immutable evidence and the
-- receipt written last by that execution remain present and correctly scoped.
local function validateIndexedDependency(request, dependency)
    local rawIndex = redis.call("HGET", KEYS[6], dependency.transactionId)
    if not rawIndex then technical("dependency_evidence_missing", "transaction dependency index is absent") end
    local index = decodeJSON(rawIndex)
    requireObject(index)
    if smallInteger(index.formatVersion, 1) ~= 1 or index.tenantId ~= request.tenantId or index.organizationId ~= request.organizationId or index.ledgerId ~= request.ledgerId or index.transactionId ~= dependency.transactionId or index.executionId ~= dependency.executionId or index.receiptField ~= dependency.executionId or index.recoveryField ~= dependency.transactionId .. ":" .. dependency.executionId then
        technical("dependency_evidence_conflict", "transaction dependency index has changed")
    end

    local rawEvidence = redis.call("HGET", KEYS[request.evidenceKeyIndex], index.recoveryField)
    if not rawEvidence then rawEvidence = redis.call("HGET", KEYS[2], index.recoveryField) end
    if not rawEvidence then technical("dependency_evidence_missing", "transaction dependency evidence is absent") end
    local evidence = decodeJSON(rawEvidence)
    requireObject(evidence)
    requireObject(evidence.record)
    if smallInteger(evidence.formatVersion, 1) ~= 1 or evidence.applicationState ~= "confirmed" or (evidence.replayState ~= "reconstructible" and evidence.replayState ~= "materialized") or (evidence.durabilityState ~= "pending" and evidence.durabilityState ~= "complete") or smallInteger(evidence.record.formatVersion, 2) ~= 2 or evidence.record.tenantId ~= request.tenantId or evidence.record.organizationId ~= request.organizationId or evidence.record.ledgerId ~= request.ledgerId or evidence.record.transactionId ~= dependency.transactionId or evidence.record.executionId ~= dependency.executionId then
        technical("dependency_evidence_invalid", "transaction dependency evidence is invalid")
    end

    local rawReceipt = redis.call("HGET", KEYS[3], index.receiptField)
    if not rawReceipt then technical("dependency_evidence_missing", "transaction dependency receipt is absent") end
    local receipt = decodeJSON(rawReceipt)
    requireObject(receipt)
    if smallInteger(receipt.formatVersion, 1) ~= 1 or receipt.tenantId ~= request.tenantId or receipt.organizationId ~= request.organizationId or receipt.ledgerId ~= request.ledgerId or receipt.executionId ~= dependency.executionId then
        technical("dependency_evidence_invalid", "transaction dependency receipt is invalid")
    end
end

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
    expectRedisType(KEYS[6], "hash")

    -- Guard comparison prevents competing lifecycle transitions. Existing
    -- recovery without a receipt means a prior outcome cannot be safely replayed.
    local preparedProtection = {}
    for _, transaction in ipairs(request.transactions) do
        local currentIndex = redis.call("HGET", KEYS[6], transaction.id)
        local predecessor = nil
        for _, dependency in ipairs(transaction.dependencies) do
            validateIndexedDependency(request, dependency)
            if dependency.kind == "predecessor" then predecessor = dependency end
        end
        if currentIndex and (not predecessor) then
            technical("transaction_state_conflict", "transaction state already has a newer execution")
        end
        if (not currentIndex) and predecessor then
            technical("dependency_evidence_missing", "transaction predecessor index is absent")
        end
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
        local keyIndex, markerIndex, legacyMarkerIndex = 5 + 3 * i, 6 + 3 * i, 7 + 3 * i
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
        current.organizationId = balance.organizationId
        current.ledgerId = balance.ledgerId
        current.balanceRef = balance.balanceRef
        local item = {
            current = current, blob = blob, keyIndex = keyIndex, seeded = not raw,
            deleted = redis.call("EXISTS", KEYS[markerIndex], KEYS[legacyMarkerIndex]) > 0
        }
        pool[scopedBalanceRef(balance.organizationId, balance.ledgerId, balance.balanceRef)] = item
        -- Index the single internal overdraft companion for later draw or repay
        -- movements generated from a primary account posting.
        if current.key == "overdraft" then
            local companionRef = scopedBalanceRef(balance.organizationId, balance.ledgerId, current.accountId)
            if companions[companionRef] then technical("invalid_balance", "multiple overdraft companions for one account") end
            companions[companionRef] = item
        end
    end
    -- Limit repair is a separate precommit operation. Mixing repair with a money
    -- mutation would make the execution result and retry boundary ambiguous.
    if #normalization > 0 then error({ kind = "normalization", keys = normalization }, 0) end

    return pool, companions
end

-- loadAccountProtection reads the exceptional closing controls of every account
-- of the declared pool inside this same atomic execution. PostgreSQL owns the
-- closing state; these keys are the live controls a movement must honor.
--
-- A marker that exists but carries no value is a failure of the protection surface
-- and refuses technically. That is deliberately distinct from the absence of both
-- markers, which is the normal state of an open account and refuses nothing.
--
-- The two cases are told apart by the GET reply and nothing else: an absent key
-- answers Lua `false`, an existing key with an empty value answers `""`. The
-- comparisons below MUST stay value comparisons against `""`; a truthiness test
-- collapses both replies into one and reads a protection failure as absence.
local function loadAccountProtection(request)
    local protection = {}
    for _, account in ipairs(request.accounts) do
        local closingKey, closedKey = KEYS[account.closingKeyIndex], KEYS[account.closedKeyIndex]
        local ownershipKey = KEYS[account.ownershipKeyIndex]
        expectRedisType(closingKey, "string")
        expectRedisType(closedKey, "string")
        expectRedisType(ownershipKey, "string")
        local closing, closed = redis.call("GET", closingKey), redis.call("GET", closedKey)
        local owner = redis.call("GET", ownershipKey)
        if closing == "" or closed == "" or owner == "" then
            technical("account_protection_unreadable", "account protection marker carries no value")
        end
        protection[account.accountId] = {
            closing = closing and true or false, closed = closed and true or false,
            owner = owner, token = account.admissionToken
        }
    end

    return protection
end

-- validateAccountClosingMarkers refuses an execution over a closing or closed
-- account before the balance pool is even read, so not even the precommit limit
-- repair that a malformed cached balance would request can reach Redis.
--
-- Only the accounts this execution uses decide: a balance that merely sits in the
-- declared pool is not a movement, and its account's closing refuses nothing. The
-- declared account order makes the refusal deterministic.
local function validateAccountClosingMarkers(request, protection)
    local owners, used = {}, {}
    for _, balance in ipairs(request.balances) do owners[balance.balanceRef] = balance.snapshot.accountId end
    for _, transaction in ipairs(request.transactions) do
        for _, requirement in ipairs(transaction.balanceRequirements) do used[owners[requirement.balanceRef]] = true end
        for _, posting in ipairs(transaction.postings) do used[owners[posting.balanceRef]] = true end
    end
    for _, account in ipairs(request.accounts) do
        local state = protection[account.accountId]
        if used[account.accountId] then
            if state.closed then technical("account_closed", "account is closed") end
            if state.closing then technical("account_closing_in_progress", "account closing is in progress") end
        end
    end
end

-- validateAccountAvailability refuses one balance whose account may not take part
-- in a new execution. It is unconditional by design: a closing is not a live block
-- control, so cancellation, permissions, honored skips and a presented
-- account-block exception never exempt it.
--
-- A balance the pool read from Redis is already admitted. One this execution would
-- seed from the request fills a cache miss, and may do so only while the
-- administrative ownership of its account still carries this caller's admission
-- token: without it nothing proved the account was open when the seed was read.
local function validateAccountAvailability(protection, item)
    local state = protection[item.current.accountId]
    if not state then technical("invalid_protocol", "balance account is missing from the account protection block") end
    if state.closed then technical("account_closed", "account is closed") end
    if state.closing then technical("account_closing_in_progress", "account closing is in progress") end
    if item.seeded and (state.token == "" or state.owner ~= state.token) then
        technical("admission_not_confirmed", "balance seed admission is not confirmed")
    end
end

-- validateAccountClosingAvailability applies the account protection to every
-- balance this execution actually uses. A companion that only sits in the pool is
-- left alone; one that moves repeats the check at its own mutation site.
local function validateAccountClosingAvailability(request, pool, protection)
    for _, transaction in ipairs(request.transactions) do
        for _, requirement in ipairs(transaction.balanceRequirements) do
            validateAccountAvailability(protection, pool[requirement.balanceRef])
        end
        for _, posting in ipairs(transaction.postings) do
            validateAccountAvailability(protection, pool[posting.balanceRef])
        end
    end
end

-- validateAccountBlockExceptions authorizes a transaction-scoped primary
-- outflow from the live single-use grant. It runs after receipt replay and live
-- balance loading, but before any monetary calculation or Redis write. The
-- primary balance and the engine-derived overdraft companion are the only live
-- controls the grant exempts.
local function validateAccountBlockExceptions(request, pool, companions)
    local exemptions, grantKeys = {}, array()
    for txIndex, transaction in ipairs(request.transactions) do
        local grant = transaction.accountBlockException
        if grant then
            local posting = transaction.postings[grant.primaryPostingIndex]
            local primary = pool[scopedBalanceRef(transaction.organizationId, transaction.ledgerId, posting.balanceRef)]
            local key = KEYS[grant.keyIndex]
            expectRedisType(key, "string")
            local raw = redis.call("GET", key)
            local valid, decoded = false, nil
            if raw then
                local decodedOK
                decodedOK, decoded = pcall(cjson.decode, raw)
                if decodedOK and type(decoded) == "table" and type(decoded.Alias) == "string" then
                    local amountOK, amount = pcall(money, decoded.Amount)
                    valid = amountOK and decoded.Alias == grant.alias and cmp_decimal(amount, grant.amount) == 0
                end
            end
            if not valid then
                refuse("account_block_exception_invalid", txIndex - 1, grant.primaryPostingIndex - 1, posting.balanceRef)
            end

            local primaryRef = scopedBalanceRef(transaction.organizationId, transaction.ledgerId, primary.current.balanceRef)
            local exempt = { [primaryRef] = true }
            local companion = companions[scopedBalanceRef(transaction.organizationId, transaction.ledgerId, primary.current.accountId)]
            if companion then exempt[scopedBalanceRef(transaction.organizationId, transaction.ledgerId, companion.current.balanceRef)] = true end
            exemptions[txIndex] = exempt
            grantKeys[#grantKeys + 1] = key
        end
    end

    return exemptions, grantKeys
end

local function blockedByLiveControl(rejectBlockedBalances, item, exemptions)
    local ref = scopedBalanceRef(item.current.organizationId, item.current.ledgerId, item.current.balanceRef)
    return rejectBlockedBalances and item.current.blocked and not (exemptions and exemptions[ref])
end

-- validateLiveBalanceAvailability rejects every requirement or posting that
-- targets a balance protected by either deletion marker, or by the live
-- account-block control when the lifecycle action requires it. These checks use
-- live Redis state inside the same atomic execution as the eventual mutation.
local function validateLiveBalanceAvailability(request, pool, exemptions)
    for txIndex, transaction in ipairs(request.transactions) do
        local exempt = exemptions[txIndex]
        for _, requirement in ipairs(transaction.balanceRequirements) do
            local item = pool[scopedBalanceRef(transaction.organizationId, transaction.ledgerId, requirement.balanceRef)]
            if item.deleted then
                refuse("balance_deleted", txIndex - 1, -1, requirement.balanceRef)
            end
            if blockedByLiveControl(transaction.rejectBlockedBalances, item, exempt) then
                refuse("account_blocked", txIndex - 1, -1, requirement.balanceRef)
            end
        end
        for postingIndex, posting in ipairs(transaction.postings) do
            local item = pool[scopedBalanceRef(transaction.organizationId, transaction.ledgerId, posting.balanceRef)]
            if item.deleted then
                refuse("balance_deleted", txIndex - 1, postingIndex - 1, posting.balanceRef)
            end
            if blockedByLiveControl(transaction.rejectBlockedBalances, item, exempt) then
                refuse("account_blocked", txIndex - 1, postingIndex - 1, posting.balanceRef)
            end
        end
    end
end

-- applyTransactionsInMemory evaluates ordered transactions against a shared
-- working pool without issuing Redis writes. Later transactions observe state
-- produced by earlier transactions in the same execution.
local function applyTransactionsInMemory(request, pool, companions, exemptions, protection)
    local movements, touched, touchedSet, transactionResults = array(), {}, {}, {}
    -- touch repeats deletion, account-closing and account-block protection at the
    -- exact mutation site, including companion movements generated internally
    -- rather than declared as postings.
    local function touch(item, txIndex, postingIndex, rejectBlockedBalances, exempt)
        validateAccountAvailability(protection, item)
        if item.deleted then refuse("balance_deleted", txIndex, postingIndex, item.current.balanceRef) end
        if blockedByLiveControl(rejectBlockedBalances, item, exempt) then
            refuse("account_blocked", txIndex, postingIndex, item.current.balanceRef)
        end
    end

    for txIndex, transaction in ipairs(request.transactions) do
        local exempt = exemptions[txIndex]
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
            local current = pool[scopedBalanceRef(transaction.organizationId, transaction.ledgerId, requirement.balanceRef)].current
            if current.assetCode ~= requirement.assetCode then
                refuse("asset_mismatch", txIndex - 1, -1, requirement.balanceRef)
            end
            local requirementRef = scopedBalanceRef(transaction.organizationId, transaction.ledgerId, requirement.balanceRef)
            if requirement.permission == "send" and not current.allowSending and not (exempt and exempt[requirementRef]) then
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
            local item = pool[scopedBalanceRef(transaction.organizationId, transaction.ledgerId, posting.balanceRef)]
            touch(item, txIndex - 1, postingIndex - 1, transaction.rejectBlockedBalances, exempt)
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
                companion = companions[scopedBalanceRef(transaction.organizationId, transaction.ledgerId, current.accountId)]
                if not companion then refuse("overdraft_companion_missing", txIndex - 1, postingIndex - 1, posting.balanceRef) end
                if companion == item or companion.current.direction ~= "debit" or companion.current.balanceScope ~= "internal" or companion.current.accountType == "external" or companion.current.assetCode ~= current.assetCode then
                    technical("invalid_companion", "invalid overdraft companion")
                end
                touch(companion, txIndex - 1, postingIndex - 1, transaction.rejectBlockedBalances, exempt)
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
local function prepareExecutionWrites(request, maximumPrepared, preparedProtection, movements, touched, transactionResults, appliedAtUnixMicro)
    local final = array()
    for _, item in ipairs(touched) do final[#final + 1] = snapshotCopy(item.current, false) end
    local response = encodeJSON({ protocolVersion = 1, movements = movements, final = final, appliedAtUnixMicro = numberToken(appliedAtUnixMicro) })
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
    local preparedBalances, preparedRecoverRecords, preparedIndexes = {}, {}, {}
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
        local record = {
            formatVersion = 2, tenantId = request.tenantId, organizationId = request.organizationId,
            ledgerId = request.ledgerId, executionId = request.executionId,
            intentFingerprint = request.intentFingerprint, transactionId = transaction.id,
            payload = transaction.completionPlan,
            result = { movements = recoveryMovements, final = recoveryFinal, appliedAtUnixMicro = numberToken(appliedAtUnixMicro) }
        }
        preparedRecoverRecords[#preparedRecoverRecords + 1] = {
            field = transaction.recoveryField,
            value = charge(encodeJSON({
                formatVersion = 1, applicationState = "confirmed", replayState = "reconstructible",
                durabilityState = "pending", record = record, dependencies = transaction.dependencies
            }))
        }
        preparedIndexes[#preparedIndexes + 1] = {
            field = transaction.id,
            value = charge(encodeJSON({
                formatVersion = 1, tenantId = request.tenantId, organizationId = request.organizationId,
                ledgerId = request.ledgerId, transactionId = transaction.id, executionId = request.executionId,
                action = transaction.action, applicationState = "confirmed",
                replayState = "reconstructible", durabilityState = "pending",
                recoveryField = transaction.recoveryField, receiptField = request.receiptField,
                dependencies = transaction.dependencies
            }))
        }
    end
    -- The receipt records the exact replay response and the ordered artifacts that
    -- must remain protected until durable completion and retention are satisfied.
    local protectedTransactions, protectedRecoveryFields, protectedIndexFields = array(), array(), array()
    for _, transaction in ipairs(request.transactions) do
        protectedTransactions[#protectedTransactions + 1] = transaction.id
        protectedRecoveryFields[#protectedRecoveryFields + 1] = transaction.recoveryField
        protectedIndexFields[#protectedIndexFields + 1] = transaction.id
    end
    local receipt = charge(encodeJSON({
        formatVersion = 1, tenantId = request.tenantId, organizationId = request.organizationId,
        ledgerId = request.ledgerId, executionId = request.executionId,
        intentFingerprint = request.intentFingerprint, response = response,
        protection = {
            formatVersion = 2, retentionSeconds = request.retentionSeconds,
            transactions = protectedTransactions, recoveryFields = protectedRecoveryFields, indexFields = protectedIndexFields,
            acknowledged = object(), terminalCompletedAtMs = object()
        }
    }))
    for _, transaction in ipairs(request.transactions) do
        charge(transaction.guardField)
        charge(transaction.nextGuard)
        charge(transaction.recoveryField)
        charge(transaction.id)
    end

    return response, preparedBalances, preparedRecoverRecords, preparedIndexes, receipt
end

-- commitPreparedExecution is the only money-changing publication phase. Every
-- argument has already been validated and serialized. Once commitStarted is set,
-- any unexpected failure is indeterminate because Redis does not roll writes back.
local function commitPreparedExecution(request, protectionKey, preparedBalances, preparedRecoverRecords, preparedIndexes, preparedProtection, grantKeys, receipt, score)
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
    for _, index in ipairs(preparedIndexes) do redis.call("HSET", KEYS[6], index.field, index.value) end
    for _, grantKey in ipairs(grantKeys) do redis.call("DEL", grantKey) end
    redis.call("HSET", KEYS[3], request.receiptField, receipt)
end

-- execute tells the complete engine story: replay or protect, load live balances,
-- evaluate in memory, prepare every output, and finally publish the prepared state.
local function execute(request, maximumPrepared)
    local replay, preparedProtection, protectionKey = prepareExecutionProtection(request)
    if replay then return replay end

    local now = redis.call("TIME")
    local appliedAtUnixMicro = now[1] .. string.format("%06d", tonumber(now[2]))
    local score = tonumber(now[1]) + tonumber(now[2]) / 1000000

    -- The closing controls answer before the pool is read and before the grant is
    -- even looked at: a movement over a closing or closed account is refused, never
    -- exempted, and the single-use grant it presented stays unconsumed for the
    -- account's own regularization.
    local protection = loadAccountProtection(request)
    validateAccountClosingMarkers(request, protection)

    local pool, companions = loadBalancePool(request)
    validateAccountClosingAvailability(request, pool, protection)
    local exemptions, grantKeys = validateAccountBlockExceptions(request, pool, companions)
    validateLiveBalanceAvailability(request, pool, exemptions)

    local movements, touched, transactionResults = applyTransactionsInMemory(request, pool, companions, exemptions, protection)
    local response, preparedBalances, preparedRecoverRecords, preparedIndexes, receipt = prepareExecutionWrites(
        request, maximumPrepared, preparedProtection, movements, touched, transactionResults, appliedAtUnixMicro
    )
    if not preparedBalances then
        if #grantKeys > 0 then technical("invalid_protocol", "account-block exception execution has no movements") end
        return response
    end

    commitPreparedExecution(
        request, protectionKey, preparedBalances, preparedRecoverRecords, preparedIndexes, preparedProtection, grantKeys, receipt, score
    )
    return response
end
