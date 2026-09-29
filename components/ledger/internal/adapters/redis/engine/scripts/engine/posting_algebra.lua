-- applyDebitPosting applies a debit according to the account's natural direction.
-- It returns the amount represented by the primary movement and no overdraft delta.
local function applyDebitPosting(current, nextState, posting)
    if current.direction == "debit" then
        nextState.available = add_decimal(current.available, posting.amount)
    else
        nextState.available = sub_decimal(current.available, posting.amount)
    end
    return posting.amount, "0"
end

-- applyCreditPosting applies a credit and repays existing overdraft usage before
-- exposing the remaining amount as available on an internal credit-direction account.
local function applyCreditPosting(current, nextState, posting)
    if current.direction == "debit" then
        nextState.available = sub_decimal(current.available, posting.amount)
    else
        nextState.available = add_decimal(current.available, posting.amount)
    end
    local primaryAmount, delta = posting.amount, "0"
    -- Repayment is bounded by the posting, outstanding debt, and the optional
    -- route-provided overdraft allocation carried by the posting.
    if current.direction ~= "debit" and current.accountType ~= "external" and cmp_decimal(current.overdraftUsed, "0") > 0 then
        local repay = min_decimal(posting.amount, current.overdraftUsed)
        if cmp_decimal(posting.overdraftAmount, "0") > 0 then repay = min_decimal(repay, posting.overdraftAmount) end
        nextState.overdraftUsed = sub_decimal(current.overdraftUsed, repay)
        nextState.available = sub_decimal(nextState.available, repay)
        primaryAmount, delta = sub_decimal(posting.amount, repay), sub_decimal("0", repay)
    end
    return primaryAmount, delta
end

-- applyReservePosting increases the reserved amount while deliberately leaving
-- the available amount unchanged.
local function applyReservePosting(current, nextState, posting)
    nextState.onHold = add_decimal(current.onHold, posting.amount)
    return posting.amount, "0"
end

-- applyUnreservePosting releases a reserved amount and refuses the transaction
-- when it would make the on-hold state negative.
local function applyUnreservePosting(current, nextState, posting, transactionIndex, postingIndex)
    if cmp_decimal(current.onHold, posting.amount) < 0 then
        refuse("onhold_underflow", transactionIndex, postingIndex, posting.balanceRef)
    end
    nextState.onHold = sub_decimal(current.onHold, posting.amount)
    return posting.amount, "0"
end

-- applyHoldPosting applies the direction-dependent availability leg and records
-- the same amount in the balance's on-hold state.
local function applyHoldPosting(current, nextState, posting)
    if current.direction == "debit" then
        nextState.available = add_decimal(current.available, posting.amount)
    else
        nextState.available = sub_decimal(current.available, posting.amount)
    end
    nextState.onHold = add_decimal(current.onHold, posting.amount)
    return posting.amount, "0"
end

-- applyReleasePosting removes an existing hold and reverses its availability leg.
-- On internal credit-direction accounts it also records any overdraft repayment
-- assigned to this release by the posting plan.
local function applyReleasePosting(current, nextState, posting, transactionIndex, postingIndex)
    if cmp_decimal(current.onHold, posting.amount) < 0 then
        refuse("onhold_underflow", transactionIndex, postingIndex, posting.balanceRef)
    end
    nextState.onHold = sub_decimal(current.onHold, posting.amount)
    if current.direction == "debit" then
        nextState.available = sub_decimal(current.available, posting.amount)
    else
        nextState.available = add_decimal(current.available, posting.amount)
    end
    local primaryAmount, delta = posting.amount, "0"
    -- Never repay more than the released amount, the routed overdraft amount, or
    -- the account's current outstanding overdraft usage.
    if current.direction ~= "debit" and current.accountType ~= "external" and cmp_decimal(posting.overdraftAmount, "0") > 0 then
        local repay = min_decimal(min_decimal(posting.amount, posting.overdraftAmount), current.overdraftUsed)
        nextState.overdraftUsed = sub_decimal(current.overdraftUsed, repay)
        nextState.available = sub_decimal(nextState.available, repay)
        primaryAmount, delta = sub_decimal(posting.amount, repay), sub_decimal("0", repay)
    end
    return primaryAmount, delta
end

-- postingAlgebra is the closed dispatch table from validated posting type to its
-- accounting state transition. Unknown types are rejected before this lookup.
local postingAlgebra = {
    debit = applyDebitPosting,
    credit = applyCreditPosting,
    reserve = applyReservePosting,
    unreserve = applyUnreservePosting,
    hold = applyHoldPosting,
    release = applyReleasePosting
}

-- mirrorOverdraft records an overdraft draw (delta > 0) or repayment (delta < 0)
-- of item's posting on its account's companion, so both sides of the debt stay
-- explicit. It is called after item's own movement is recorded.
local function mirrorOverdraft(step, postingIndex, posting, item, delta, ordinal)
    if cmp_decimal(delta, "0") == 0 then return end
    local transaction, txIndex, current = step.transaction, step.txIndex, item.current
    local companion = step.companions[scopedBalanceRef(transaction.organizationId, transaction.ledgerId, current.accountId)]
    if not companion then refuse("overdraft_companion_missing", txIndex, postingIndex, posting.balanceRef) end
    if companion == item or companion.current.direction ~= "debit" or companion.current.balanceScope ~= "internal" or companion.current.accountType == "external" or companion.current.assetCode ~= current.assetCode then
        technical("invalid_companion", "invalid overdraft companion")
    end
    step.touch(companion, txIndex, postingIndex, transaction.rejectBlockedBalances, step.exempt)
    local companionNext, amount, companionType = clone(companion.current), delta, "debit"
    if cmp_decimal(delta, "0") > 0 then
        companionNext.available = add_decimal(companion.current.available, amount)
    else
        amount, companionType = sub_decimal("0", delta), "credit"
        if cmp_decimal(companion.current.available, amount) < 0 then
            refuse("insufficient_funds", txIndex, postingIndex, companion.current.balanceRef)
        end
        companionNext.available = sub_decimal(companion.current.available, amount)
    end
    step.record(companion, companionNext, posting, "overdraft_companion", companionType, amount, "0", ordinal)
end
