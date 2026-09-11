-- redisType normalizes the different TYPE reply shapes returned by supported
-- Redis and Valkey clients into one type name.
local function redisType(key)
    local result = redis.call("TYPE", key)
    if type(result) == "table" then return result.ok end
    return result
end

-- expectRedisType permits an absent key but rejects an existing key whose Redis
-- data type would make the planned command unsafe or ambiguous.
local function expectRedisType(key, expected)
    local actual = redisType(key)
    if actual ~= "none" and actual ~= expected then
        technical("wrong_key_type", "unexpected Redis key type")
    end
end

-- positiveBudget validates a trusted byte ceiling as a bounded positive integer
-- before converting the small value for arithmetic inside the script.
local function positiveBudget(raw)
    integerText(raw, "2147483647")
    local value = tonumber(raw)
    if value < 1 then technical("invalid_protocol", "byte budget must be positive") end
    return value
end

-- validPosting validates one declarative accounting mutation. It accepts only
-- the closed posting and draw-policy vocabularies and canonical positive money.
local function validPosting(posting)
    requireObject(posting)
    text(posting.ref, false)
    logicalRef(posting.balanceRef)
    if posting.type ~= "debit" and posting.type ~= "credit" and posting.type ~= "reserve" and posting.type ~= "unreserve" and posting.type ~= "hold" and posting.type ~= "release" then
        technical("invalid_protocol", "unknown posting type")
    end
    if posting.drawPolicy ~= "forbidden" and posting.drawPolicy ~= "allowed" and posting.drawPolicy ~= "route_denied" then
        technical("invalid_protocol", "unknown draw policy")
    end
    canonicalMoney(posting.amount)
    canonicalMoney(posting.overdraftAmount)
    if cmp_decimal(posting.amount, "0") <= 0 or cmp_decimal(posting.overdraftAmount, "0") < 0 then
        technical("invalid_protocol", "invalid posting amount")
    end
end

-- validBalanceRequirement validates a nonmonetary precondition attached to a
-- transaction, such as asset identity, permission direction, or external ban.
local function validBalanceRequirement(requirement)
    requireObject(requirement)
    logicalRef(requirement.balanceRef)
    text(requirement.assetCode, false)
    if requirement.permission ~= "send" and requirement.permission ~= "receive" then
        technical("invalid_protocol", "unknown balance permission")
    end
    bool(requirement.forbidExternal)
end

-- decodeRequest validates the entire Go-to-Lua contract before live state is
-- read or mutated. It proves scope, key inventory, balance identity, transaction
-- correlation, and that every requirement and posting references the declared pool.
local function decodeRequest(raw)
    local request = decodeJSON(raw)
    requireObject(request)
    -- Validate the execution envelope and the fixed positions of shared keys.
    if smallInteger(request.protocolVersion, 1) ~= 1 then technical("invalid_protocol", "unsupported protocol version") end
    text(request.tenantId, true)
    uuid(request.organizationId)
    uuid(request.ledgerId)
    uuid(request.executionId)
    text(request.intentFingerprint, false)
    if smallInteger(request.retentionSeconds, 604800) < 1 then technical("invalid_protocol", "invalid retention window") end
    if request.receiptField ~= request.executionId then technical("invalid_protocol", "invalid receipt field") end
    if smallInteger(request.scheduleKeyIndex, #KEYS) ~= 1 or smallInteger(request.recoveryKeyIndex, #KEYS) ~= 2 or smallInteger(request.receiptKeyIndex, #KEYS) ~= 3 or smallInteger(request.guardKeyIndex, #KEYS) ~= 4 or smallInteger(request.protectionKeyIndex, #KEYS) ~= 5 then
        technical("invalid_protocol", "invalid shared key indices")
    end
    requireArray(request.balances)
    requireArray(request.transactions)
    if #request.transactions == 0 or #KEYS ~= 5 + 3 * #request.balances then technical("invalid_protocol", "invalid execution cardinality") end
    -- All physical keys must be unique and use the same transaction hash tag so
    -- the complete execution belongs to one Redis Cluster slot.
    local seenKeys = {}
    for _, key in ipairs(KEYS) do
        local _, opens = key:gsub("{", "")
        local _, closes = key:gsub("}", "")
        if not key:find(transaction_hash_tag, 1, true) or opens ~= 1 or closes ~= 1 or seenKeys[key] then
            technical("invalid_protocol", "invalid physical key inventory")
        end
        seenKeys[key] = true
    end
    -- Validate immutable balance seeds and build lookup sets used to constrain
    -- every later transaction reference to this request's declared pool.
    local refs, ids, accounts, aliases = {}, {}, {}, {}
    for i, balance in ipairs(request.balances) do
        requireObject(balance)
        logicalRef(balance.balanceRef)
        if refs[balance.balanceRef] then technical("invalid_protocol", "duplicate balance reference") end
        if smallInteger(balance.keyIndex, #KEYS) ~= 3 + 3 * i or smallInteger(balance.deleteKeyIndex, #KEYS) ~= 4 + 3 * i or smallInteger(balance.legacyDeleteKeyIndex, #KEYS) ~= 5 + 3 * i then
            technical("invalid_protocol", "invalid balance key indices")
        end
        local expectedMarker, replacements = KEYS[3 + 3 * i]:gsub(balance_cache_namespace_prefix, balance_deletion_marker_namespace_prefix, 1)
        if replacements ~= 1 or KEYS[4 + 3 * i] ~= expectedMarker or KEYS[5 + 3 * i] ~= KEYS[3 + 3 * i] .. balance_deletion_marker_suffix then
            technical("invalid_protocol", "invalid deletion marker key")
        end
        local seed = balance.snapshot
        requireObject(seed)
        canonicalMoney(seed.available)
        canonicalMoney(seed.onHold)
        canonicalMoney(seed.overdraftUsed)
        canonicalMoney(seed.overdraftLimit)
        validateSnapshot(seed)
        if balance.balanceRef ~= seed.alias .. "#" .. seed.key or ids[seed.id] then technical("invalid_protocol", "invalid balance identity") end
        if aliases[seed.alias] and aliases[seed.alias] ~= seed.accountId then technical("invalid_protocol", "alias identifies different accounts") end
        local account = accounts[seed.accountId]
        if account and (account.alias ~= seed.alias or account.assetCode ~= seed.assetCode or account.accountType ~= seed.accountType) then
            technical("invalid_protocol", "inconsistent account identity")
        end
        refs[balance.balanceRef], ids[seed.id], accounts[seed.accountId], aliases[seed.alias] = true, true, seed, seed.accountId
    end
    -- Validate transaction correlation, guard advancement, recovery payloads,
    -- and the closed set of balance references used by requirements and postings.
    local transactions = {}
    for _, transaction in ipairs(request.transactions) do
        requireObject(transaction)
        uuid(transaction.id)
        if transactions[transaction.id] or transaction.guardField ~= transaction.id or transaction.recoveryField ~= transaction.id .. ":" .. request.executionId then
            technical("invalid_protocol", "invalid transaction correlation")
        end
        transactions[transaction.id] = true
        bool(transaction.rejectBlockedBalances)
        text(transaction.expectedGuard, true)
        text(transaction.nextGuard, false)
        if transaction.expectedGuard == transaction.nextGuard then technical("invalid_protocol", "execution guard must advance") end
        text(transaction.completionPlan, false)
        requireObject(decodeJSON(transaction.completionPlan))
        requireArray(transaction.balanceRequirements)
        requireArray(transaction.postings)
        if #transaction.postings == 0 then technical("invalid_protocol", "empty transaction postings") end
        for _, requirement in ipairs(transaction.balanceRequirements) do
            validBalanceRequirement(requirement)
            if not refs[requirement.balanceRef] then technical("invalid_protocol", "invalid balance requirement reference") end
        end
        local postingRefs = {}
        for _, posting in ipairs(transaction.postings) do
            validPosting(posting)
            if postingRefs[posting.ref] or not refs[posting.balanceRef] then technical("invalid_protocol", "invalid posting reference") end
            postingRefs[posting.ref] = true
        end
    end
    return request
end
