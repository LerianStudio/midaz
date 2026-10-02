-- Fee debts: a deferrable debit's unfunded rest opens on its payer's live list, a
-- collect settles that list oldest first, and a revert cancels, reopens and refunds.
-- Nothing here writes Redis; the commit phase publishes the changed lists.

local feeDebtCap = 256

-- take is the part of owed that available funds pay: min(max(available, 0), owed).
local function take(available, owed)
    if cmp_decimal(available, "0") <= 0 then return "0" end
    return min_decimal(available, owed)
end

-- feeDebtConflict aborts before commit when the live list disagrees with what Go
-- declared, so a lost or stale list parks the execution instead of guessing.
local function feeDebtConflict(message)
    technical("fee_debt_conflict", message)
end

-- decodeFeeDebtList validates one stored list: version 1, a positive nextSeq above
-- every item's seq, strictly ascending seqs, unique ids and 0 < remaining <= opened.
-- Any failure is corrupt live state, so it is a conflict, not a protocol error.
local function decodeFeeDebtList(raw)
    local decoded, list = pcall(function()
        local list = decodeJSON(raw)
        requireObject(list)
        requireArray(list.items)
        if numberTokens[list.v] ~= "1" or integerText(list.nextSeq, maximumFeeDebtSeq) == "0" then error("invalid fee-debt list", 0) end
        local ids, previous = {}, "0"
        for _, item in ipairs(list.items) do
            requireObject(item)
            text(item.id, false)
            logicalRef(item.creditRef)
            uuid(item.originTransactionId)
            text(item.assetCode, false)
            optionalDebtRoute(item.debitRoute)
            optionalDebtRoute(item.creditRoute)
            local remaining, opened = canonicalMoney(item.remaining), canonicalMoney(item.opened)
            if ids[item.id] or cmp_decimal(remaining, "0") <= 0 or cmp_decimal(remaining, opened) > 0 or
                cmp_decimal(positiveSeq(item.seq), previous) <= 0 or cmp_decimal(item.seq, list.nextSeq) >= 0 then
                error("invalid fee-debt item", 0)
            end
            ids[item.id], previous = true, item.seq
        end
        return list
    end)
    if not decoded then feeDebtConflict("corrupt fee-debt list") end
    return list
end

-- loadFeeDebts reads every declared list once. An absent key is a debtor that
-- never had a debt; its list starts empty at nextSeq 1.
local function loadFeeDebts(request)
    local feeDebts = { lists = {}, order = {}, changes = array() }
    for _, entry in ipairs(request.feeDebts) do
        local key = KEYS[entry.keyIndex]
        expectRedisType(key, "string")
        local raw = redis.call("GET", key)
        local list = { entry = entry, exists = raw ~= false, changed = false, reserved = 0 }
        list.value = raw and decodeFeeDebtList(raw) or { v = numberToken("1"), nextSeq = "1", items = array() }
        feeDebts.lists[scopedBalanceRef(entry.organizationId, entry.ledgerId, entry.balanceRef)] = list
        feeDebts.order[#feeDebts.order + 1] = list
    end
    return feeDebts
end

local function feeDebtList(step, balanceRef)
    return step.feeDebts.lists[scopedBalanceRef(step.transaction.organizationId, step.transaction.ledgerId, balanceRef)]
end

local function feeDebtBalance(step, balanceRef)
    return step.pool[scopedBalanceRef(step.transaction.organizationId, step.transaction.ledgerId, balanceRef)]
end

-- emitFeeDebtChange records one applied change for the response and for the
-- recovery record of its transaction, in execution order.
local function emitFeeDebtChange(step, kind, postingRef, debtorRef, debt, amount)
    local change = {
        transactionId = step.transaction.id, postingRef = postingRef, kind = kind, debtId = debt.id,
        debtorRef = debtorRef, creditRef = debt.creditRef, originTransactionId = debt.originTransactionId,
        seq = debt.seq, assetCode = debt.assetCode, amount = amount, opened = debt.opened,
        debitRoute = debt.debitRoute, creditRoute = debt.creditRoute
    }
    step.changes[#step.changes + 1] = change
    step.feeDebts.changes[#step.feeDebts.changes + 1] = change
end

-- cancelFeeDebts removes, from every declared list of a revert's scope, each debt
-- its parent opened, and remembers what was still open for the refund.
local function cancelFeeDebts(step)
    local transaction, parent = step.transaction, step.transaction.parentTransactionId
    if transaction.action ~= "revert" or parent == nil or parent == nullValue then return end
    for _, list in ipairs(step.feeDebts.order) do
        local entry, items, i = list.entry, list.value.items, 1
        local sameScope = entry.organizationId == transaction.organizationId and entry.ledgerId == transaction.ledgerId
        while sameScope and items[i] do
            local debt = items[i]
            if debt.originTransactionId == parent then
                table.remove(items, i)
                step.canceled[debt.id], list.changed = debt.remaining, true
                emitFeeDebtChange(step, "canceled", "", entry.balanceRef, debt, debt.remaining)
            else
                i = i + 1
            end
        end
    end
end

-- reopenFeeDebts restores, in order, what the reverted parent settled: a live debt
-- grows back, a gone one is reinserted at its seq. The debtor is touched like any
-- moved balance, so no debt is restored onto a deleted balance or closing account.
local function reopenFeeDebts(step)
    local transaction = step.transaction
    for _, entry in ipairs(transaction.reopenFeeDebts or {}) do
        step.touch(feeDebtBalance(step, entry.debtorRef), step.txIndex, -1, transaction.rejectBlockedBalances, step.exempt)
        local list = feeDebtList(step, entry.debtorRef)
        local items, debt, position = list.value.items, nil, #list.value.items + 1
        for i, item in ipairs(items) do
            if item.id == entry.debtId or item.seq == entry.seq then debt = item break end
            if position > #items and cmp_decimal(item.seq, entry.seq) > 0 then position = i end
        end
        if debt then
            debt.remaining = add_decimal(debt.remaining, entry.amount)
            if debt.id ~= entry.debtId or debt.seq ~= entry.seq or debt.creditRef ~= entry.creditRef or debt.opened ~= entry.opened or cmp_decimal(debt.remaining, debt.opened) > 0 then
                feeDebtConflict("reopened debt does not match its live item")
            end
        else
            if cmp_decimal(entry.seq, list.value.nextSeq) >= 0 then feeDebtConflict("reopened debt is newer than its list") end
            debt = {
                id = entry.debtId, creditRef = entry.creditRef, remaining = entry.amount, opened = entry.opened,
                originTransactionId = entry.debtId:sub(1, 36), seq = entry.seq,
                assetCode = feeDebtBalance(step, entry.creditRef).current.assetCode,
                debitRoute = entry.debitRoute, creditRoute = entry.creditRoute
            }
            table.insert(items, position, debt)
        end
        list.changed = true
        emitFeeDebtChange(step, "reopened", "", entry.debtorRef, debt, entry.amount)
    end
end

-- deferShortfall returns what a deferrable debit moves. On an internal balance
-- whose direction is not debit that is take(available, amount), and the rest is
-- reserved as a debt its funded credit opens; anywhere else it moves everything.
local function deferShortfall(step, postingIndex, posting, item)
    local current, paid = item.current, posting.amount
    if current.accountType ~= "external" and current.direction ~= "debit" then paid = take(current.available, posting.amount) end
    local shortfall = sub_decimal(posting.amount, paid)
    if cmp_decimal(shortfall, "0") > 0 then
        local list = feeDebtList(step, posting.balanceRef)
        if #list.value.items + list.reserved >= feeDebtCap then
            refuse("insufficient_funds", step.txIndex, postingIndex, posting.balanceRef)
        end
        list.reserved = list.reserved + 1
    end
    step.deferrals[posting.ref] = {
        paid = paid, shortfall = shortfall, payerRef = posting.balanceRef, assetCode = current.assetCode, debitRoute = posting.debtRoute
    }
    return paid
end

-- openFeeDebt appends the unfunded rest of the debit a credit is funded by.
local function openFeeDebt(step, posting)
    local deferral = step.deferrals[posting.fundedByRef]
    if cmp_decimal(deferral.shortfall, "0") <= 0 then return end
    local list = feeDebtList(step, deferral.payerRef)
    local debt = {
        id = step.transaction.id .. ":" .. posting.fundedByRef, creditRef = posting.balanceRef,
        remaining = deferral.shortfall, opened = deferral.shortfall, originTransactionId = step.transaction.id,
        seq = list.value.nextSeq, assetCode = deferral.assetCode, debitRoute = deferral.debitRoute, creditRoute = posting.debtRoute
    }
    list.value.items[#list.value.items + 1] = debt
    list.value.nextSeq = add_decimal(list.value.nextSeq, "1")
    list.reserved, list.changed = list.reserved - 1, true
    emitFeeDebtChange(step, "opened", posting.fundedByRef, deferral.payerRef, debt, deferral.shortfall)
end

-- collectFeeDebts settles the debtor's list from its head with at most take(available,
-- amount), one debtor debit and one creditor credit per debt. It never refuses: it stops
-- at the first debt it may not settle, and settles nothing when the debtor may not pay.
local function collectFeeDebts(step, posting, debtor)
    local current, list = debtor.current, feeDebtList(step, posting.balanceRef)
    if current.direction == "debit" or not current.allowSending or not step.settleable(debtor) then return end
    local budget, settled, ordinals, last = take(current.available, posting.amount), {}, {}, -1
    for i, id in ipairs(posting.items) do ordinals[id] = i - 1 end
    local items = list.value.items
    while items[1] and cmp_decimal(budget, "0") > 0 do
        local debt = items[1]
        local ordinal, creditor = ordinals[debt.id], feeDebtBalance(step, debt.creditRef)
        if not ordinal or ordinal <= last or not creditor or creditor == debtor or not step.settleable(creditor) or
            creditor.current.direction ~= "credit" or not creditor.current.allowReceiving or
            cmp_decimal(creditor.current.overdraftUsed, "0") > 0 or creditor.current.assetCode ~= current.assetCode then
            break
        end
        local amount = min_decimal(budget, debt.remaining)
        budget, last = sub_decimal(budget, amount), ordinal
        debt.remaining = sub_decimal(debt.remaining, amount)
        if debt.remaining == "0" then table.remove(items, 1) end
        settled[#settled + 1] = { debt = debt, creditor = creditor, amount = amount, ordinal = ordinal }
    end
    if #settled == 0 then return end
    list.changed = true
    for _, settlement in ipairs(settled) do
        local debtorNext = clone(debtor.current)
        debtorNext.available = sub_decimal(debtorNext.available, settlement.amount)
        step.record(debtor, debtorNext, posting, "fee_debt_debit", "debit", settlement.amount, "0", settlement.ordinal)
        local creditorNext = clone(settlement.creditor.current)
        creditorNext.available = add_decimal(creditorNext.available, settlement.amount)
        step.record(settlement.creditor, creditorNext, posting, "fee_debt_credit", "credit", settlement.amount, "0", settlement.ordinal)
        emitFeeDebtChange(step, "settled", posting.ref, posting.balanceRef, settlement.debt, settlement.amount)
    end
end

-- refundFeeDebts pays the debtor, as an ordinary credit per debt, opened minus
-- canceled of each debt the parent opened; it must equal the entry's expectedRefund
-- and each creditor pays its part as a debit without overdraft, never partially.
local function refundFeeDebts(step, postingIndex, posting, debtor)
    local list, transaction = feeDebtList(step, posting.balanceRef), step.transaction
    for _, entry in ipairs(posting.refunds) do
        if not list.exists or cmp_decimal(entry.seq, list.value.nextSeq) >= 0 then
            feeDebtConflict("refund does not match its live list")
        end
    end
    for _, entry in ipairs(posting.refunds) do
        if cmp_decimal(sub_decimal(entry.opened, step.canceled[entry.debtId] or "0"), entry.expectedRefund) ~= 0 then
            technical("fee_debt_record_pending", "refund does not match the debt's record")
        end
    end
    local refunds, pending = {}, {}
    for index, entry in ipairs(posting.refunds) do
        local amount = entry.expectedRefund
        if cmp_decimal(amount, "0") > 0 then
            local creditor = feeDebtBalance(step, entry.creditRef)
            step.touch(creditor, step.txIndex, postingIndex, transaction.rejectBlockedBalances, step.exempt)
            pending[creditor] = add_decimal(pending[creditor] or "0", amount)
            if creditor.current.accountType == "external" or creditor.current.direction ~= "credit" or cmp_decimal(creditor.current.available, pending[creditor]) < 0 then
                refuse("insufficient_funds", step.txIndex, postingIndex, entry.creditRef)
            end
            refunds[#refunds + 1] = { entry = entry, creditor = creditor, amount = amount, ordinal = index - 1 }
        end
    end
    for _, refund in ipairs(refunds) do
        local debtorNext = clone(debtor.current)
        local credited, delta = applyCreditPosting(debtor.current, debtorNext, { amount = refund.amount, overdraftAmount = posting.overdraftAmount })
        step.record(debtor, debtorNext, posting, "fee_debt_refund_credit", "credit", credited, delta, refund.ordinal)
        mirrorOverdraft(step, postingIndex, posting, debtor, delta, refund.ordinal)
        local creditorNext = clone(refund.creditor.current)
        creditorNext.available = sub_decimal(creditorNext.available, refund.amount)
        step.record(refund.creditor, creditorNext, posting, "fee_debt_refund_debit", "debit", refund.amount, "0", refund.ordinal)
        local entry = refund.entry
        emitFeeDebtChange(step, "refunded", posting.ref, posting.balanceRef, {
            id = entry.debtId, creditRef = entry.creditRef, originTransactionId = entry.debtId:sub(1, 36),
            seq = entry.seq, assetCode = debtor.current.assetCode, opened = entry.opened
        }, refund.amount)
    end
end

-- prepareFeeDebtWrites serializes every changed list for the commit phase.
local function prepareFeeDebtWrites(feeDebts, charge)
    local prepared = {}
    for _, list in ipairs(feeDebts.order) do
        if list.changed then prepared[#prepared + 1] = { key = KEYS[list.entry.keyIndex], value = charge(encodeJSON(list.value)) } end
    end
    return prepared
end
