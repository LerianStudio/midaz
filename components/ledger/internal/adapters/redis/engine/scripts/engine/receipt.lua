-- validateStoredResponse proves that a saved accounting result could have been
-- produced by the current immutable request. A receipt is replay evidence, so
-- malformed or uncorrelated contents must never be returned as trusted success.
local function validateStoredResponse(raw, request)
    local response = decodeJSON(raw)
    requireObject(response)
    if smallInteger(response.protocolVersion, 1) ~= 1 then technical("invalid_receipt", "unsupported saved response") end
    requireArray(response.movements)
    requireArray(response.final)
    if #response.movements == 0 or #response.final == 0 then
        technical("invalid_receipt", "stored execution receipt has no accounting movements")
    end
    -- Index the requested postings and balance seeds before validating the saved
    -- movement graph against their immutable identities.
    local postings, seeds = {}, {}
    for _, transaction in ipairs(request.transactions) do
        local refs = {}
        for _, posting in ipairs(transaction.postings) do refs[posting.ref] = posting end
        postings[transaction.id] = refs
    end
    for _, balance in ipairs(request.balances) do seeds[balance.balanceRef] = balance.snapshot end
    -- Rebuild each balance's ordered version chain from the recorded movements.
    -- Every transition must advance exactly once and start where the prior one ended.
    local movementRefs, finalRefs, last = {}, {}, {}
    for index, movement in ipairs(response.movements) do
        requireObject(movement)
        text(movement.ref, false)
        uuid(movement.transactionId)
        text(movement.postingRef, false)
        logicalRef(movement.balanceRef)
        local transaction = postings[movement.transactionId]
        local posting = transaction and transaction[movement.postingRef]
        if not posting or (movement.role == "primary" and (movement.balanceRef ~= posting.balanceRef or movement.type ~= posting.type)) then
            technical("invalid_receipt", "saved movement does not match execution")
        end
        if movementRefs[movement.ref] or (movement.role ~= "primary" and movement.role ~= "overdraft_companion") then
            technical("invalid_receipt", "invalid saved movement identity")
        end
        local expectedRef = movement.transactionId .. ":" .. #movement.postingRef .. ":" .. movement.postingRef .. ":" .. movement.role .. ":0"
        if movement.ref ~= expectedRef then technical("invalid_receipt", "invalid saved movement reference") end
        -- A companion movement is valid only immediately after its primary
        -- overdraft movement and on that account's reserved overdraft balance.
        if movement.role == "overdraft_companion" then
            local primary = response.movements[index - 1]
            local seed = seeds[posting.balanceRef]
            if not primary or primary.role ~= "primary" or primary.transactionId ~= movement.transactionId or primary.postingRef ~= movement.postingRef or primary.overdraftDelta == "0" or movement.balanceRef ~= seed.alias .. "#overdraft" then
                technical("invalid_receipt", "invalid saved companion correlation")
            end
        end
        movementRefs[movement.ref] = true
        if movement.type ~= "debit" and movement.type ~= "credit" and movement.type ~= "reserve" and movement.type ~= "unreserve" and movement.type ~= "hold" and movement.type ~= "release" then
            technical("invalid_receipt", "invalid saved movement type")
        end
        nonnegative(movement.amount)
        canonicalMoney(movement.overdraftDelta)
        for _, boundary in ipairs({ movement.before, movement.after }) do
            requireObject(boundary)
            canonicalMoney(boundary.available)
            nonnegative(boundary.onHold)
            nonnegative(boundary.overdraftUsed)
            integerText(boundary.version, "9223372036854775807")
        end
        if add_decimal(movement.before.version, "1") ~= movement.after.version then
            technical("invalid_receipt", "invalid saved movement version")
        end
        local previous = last[movement.balanceRef]
        if previous and encodeJSON(previous) ~= encodeJSON(movement.before) then
            technical("invalid_receipt", "broken saved movement chain")
        end
        last[movement.balanceRef] = movement.after
    end
    -- Final snapshots must preserve seed identity and exactly match the last
    -- reconstructed state for every balance changed by the execution.
    for _, final in ipairs(response.final) do
        validateSnapshot(final)
        logicalRef(final.balanceRef)
        local seed = seeds[final.balanceRef]
        if seed then
            for _, field in ipairs({ "id", "accountId", "accountType", "assetCode", "alias", "key" }) do
                if final[field] ~= seed[field] then technical("invalid_receipt", "saved balance identity mismatch") end
            end
        end
        if finalRefs[final.balanceRef] or not last[final.balanceRef] or encodeJSON(state(final, false)) ~= encodeJSON(last[final.balanceRef]) then
            technical("invalid_receipt", "invalid saved final balance")
        end
        finalRefs[final.balanceRef] = true
    end
    for ref, _ in pairs(last) do
        if not finalRefs[ref] then technical("invalid_receipt", "missing saved final balance") end
    end
end

-- decodeStoredReceipt validates receipt scope, intent, cleanup protection, and
-- the embedded accounting response before returning the exact saved response bytes.
local function decodeStoredReceipt(raw, request)
    local receipt = decodeJSON(raw)
    requireObject(receipt)
    if smallInteger(receipt.formatVersion, 1) ~= 1 or receipt.tenantId ~= request.tenantId or receipt.organizationId ~= request.organizationId or receipt.ledgerId ~= request.ledgerId or receipt.executionId ~= request.executionId then
        technical("invalid_receipt", "saved receipt identity mismatch")
    end
    if receipt.intentFingerprint ~= request.intentFingerprint then
        technical("execution_fingerprint_conflict", "execution identity was reused for a different intent")
    end
    -- Protection metadata ties cleanup eligibility to the same ordered set of
    -- transactions and recovery records that the execution originally committed.
    if receipt.protection ~= nil then
        local protection = receipt.protection
        requireObject(protection)
        if smallInteger(protection.formatVersion, 1) ~= 1 or smallInteger(protection.retentionSeconds, 604800) < 1 then
            technical("invalid_receipt", "invalid saved receipt protection")
        end
        requireArray(protection.transactions)
        requireArray(protection.recoveryFields)
        requireObject(protection.acknowledged)
        requireObject(protection.terminalCompletedAtMs)
        if #protection.transactions ~= #request.transactions or #protection.recoveryFields ~= #request.transactions then
            technical("invalid_receipt", "saved receipt protection cardinality differs")
        end
        for index, transaction in ipairs(request.transactions) do
            if protection.transactions[index] ~= transaction.id or protection.recoveryFields[index] ~= transaction.recoveryField then
                technical("invalid_receipt", "saved receipt protection identity differs")
            end
        end
    end
    text(receipt.response, false)
    validateStoredResponse(receipt.response, request)
    return receipt.response
end

-- storedReceipt looks up this execution's atomic replay record. A valid receipt
-- short-circuits all balance work; a fingerprint conflict remains explicit,
-- while any other corruption becomes an indeterminate invalid-receipt failure.
local function storedReceipt(request)
    local raw = redis.call("HGET", KEYS[3], request.receiptField)
    if not raw then return nil end
    local ok, response = pcall(decodeStoredReceipt, raw, request)
    if ok then return response end
    if type(response) == "table" and response.kind == "technical" and response.code == "execution_fingerprint_conflict" then
        error(response, 0)
    end
    technical("invalid_receipt", "stored execution receipt cannot be validated")
end
