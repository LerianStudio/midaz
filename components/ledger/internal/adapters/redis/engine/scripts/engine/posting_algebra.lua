local function applyDebitPosting(current, nextState, posting)
    if current.direction == "debit" then
        nextState.available = add_decimal(current.available, posting.amount)
    else
        nextState.available = sub_decimal(current.available, posting.amount)
    end
    return posting.amount, "0"
end

local function applyCreditPosting(current, nextState, posting)
    if current.direction == "debit" then
        nextState.available = sub_decimal(current.available, posting.amount)
    else
        nextState.available = add_decimal(current.available, posting.amount)
    end
    local primaryAmount, delta = posting.amount, "0"
    if current.direction ~= "debit" and current.accountType ~= "external" and cmp_decimal(current.overdraftUsed, "0") > 0 then
        local repay = min_decimal(posting.amount, current.overdraftUsed)
        if cmp_decimal(posting.overdraftAmount, "0") > 0 then repay = min_decimal(repay, posting.overdraftAmount) end
        nextState.overdraftUsed = sub_decimal(current.overdraftUsed, repay)
        nextState.available = sub_decimal(nextState.available, repay)
        primaryAmount, delta = sub_decimal(posting.amount, repay), sub_decimal("0", repay)
    end
    return primaryAmount, delta
end

local function applyReservePosting(current, nextState, posting)
    nextState.onHold = add_decimal(current.onHold, posting.amount)
    return posting.amount, "0"
end

local function applyUnreservePosting(current, nextState, posting, transactionIndex, postingIndex)
    if cmp_decimal(current.onHold, posting.amount) < 0 then
        refuse("onhold_underflow", transactionIndex, postingIndex, posting.balanceRef)
    end
    nextState.onHold = sub_decimal(current.onHold, posting.amount)
    return posting.amount, "0"
end

local function applyHoldPosting(current, nextState, posting)
    if current.direction == "debit" then
        nextState.available = add_decimal(current.available, posting.amount)
    else
        nextState.available = sub_decimal(current.available, posting.amount)
    end
    nextState.onHold = add_decimal(current.onHold, posting.amount)
    return posting.amount, "0"
end

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
    if current.direction ~= "debit" and current.accountType ~= "external" and cmp_decimal(posting.overdraftAmount, "0") > 0 then
        local repay = min_decimal(min_decimal(posting.amount, posting.overdraftAmount), current.overdraftUsed)
        nextState.overdraftUsed = sub_decimal(current.overdraftUsed, repay)
        nextState.available = sub_decimal(nextState.available, repay)
        primaryAmount, delta = sub_decimal(posting.amount, repay), sub_decimal("0", repay)
    end
    return primaryAmount, delta
end

local postingAlgebra = {
    debit = applyDebitPosting,
    credit = applyCreditPosting,
    reserve = applyReservePosting,
    unreserve = applyUnreservePosting,
    hold = applyHoldPosting,
    release = applyReleasePosting
}
