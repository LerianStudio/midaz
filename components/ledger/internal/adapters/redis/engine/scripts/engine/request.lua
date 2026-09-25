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
    if key == nil then technical("invalid_protocol", "missing declared Redis key") end
    local actual = redisType(key)
    if actual ~= "none" and actual ~= expected then
        technical("wrong_key_type", "unexpected Redis key type")
    end
end

-- positiveLimit validates one trusted ceiling as a bounded positive integer
-- before converting the small value for arithmetic inside the script.
local function positiveLimit(raw, description)
    integerText(raw, "2147483647")
    local value = tonumber(raw)
    if value < 1 then technical("invalid_protocol", description .. " must be positive") end
    return value
end

local function positiveBudget(raw) return positiveLimit(raw, "byte budget") end

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
local function decodeRequest(raw, maximumTransactions, maximumPostings, maximumBalances)
    local request = decodeJSON(raw)
    requireObject(request)
    -- Validate the execution envelope and the fixed positions of shared keys.
    if smallInteger(request.protocolVersion, 3) ~= 3 then technical("invalid_protocol", "unsupported protocol version") end
    text(request.tenantId, true)
    uuid(request.organizationId)
    uuid(request.ledgerId)
    uuid(request.executionId)
    text(request.intentFingerprint, false)
    if smallInteger(request.retentionSeconds, 604800) < 1 then technical("invalid_protocol", "invalid retention window") end
    if request.receiptField ~= request.executionId then technical("invalid_protocol", "invalid receipt field") end
    local evidenceKeyIndex = smallInteger(request.evidenceKeyIndex, #KEYS)
    if smallInteger(request.scheduleKeyIndex, #KEYS) ~= 1 or smallInteger(request.recoveryKeyIndex, #KEYS) ~= 2 or smallInteger(request.receiptKeyIndex, #KEYS) ~= 3 or smallInteger(request.guardKeyIndex, #KEYS) ~= 4 or smallInteger(request.protectionKeyIndex, #KEYS) ~= 5 or smallInteger(request.transactionIndexKeyIndex, #KEYS) ~= 6 or evidenceKeyIndex ~= 7 then
        technical("invalid_protocol", "invalid shared key indices")
    end
	request.evidenceKeyIndex = evidenceKeyIndex
    requireArray(request.balances)
    requireArray(request.transactions)
    requireArray(request.accounts)
    if #request.transactions == 0 or #request.transactions > maximumTransactions or #request.balances > maximumBalances then
        technical("invalid_protocol", "execution exceeds transaction or balance limit")
    end
    local grantCount = 0
    for _, transaction in ipairs(request.transactions) do
        requireObject(transaction)
        if transaction.accountBlockException ~= nil then grantCount = grantCount + 1 end
    end
    local declaredScopes = request.scopeKeys
    local extraScopes = 0
    if declaredScopes ~= nil then
        requireArray(declaredScopes)
        if #declaredScopes < 2 then technical("invalid_protocol", "invalid scope key inventory") end
        extraScopes = #declaredScopes - 1
    end
    if #KEYS ~= 7 + 3 * #request.balances + grantCount + 3 * #request.accounts + 5 * extraScopes then technical("invalid_protocol", "invalid execution cardinality") end
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
        uuid(balance.organizationId)
        uuid(balance.ledgerId)
        logicalRef(balance.balanceRef)
        local scopeRef = scopedBalanceRef(balance.organizationId, balance.ledgerId, balance.balanceRef)
        if refs[scopeRef] then technical("invalid_protocol", "duplicate balance reference") end
        if smallInteger(balance.keyIndex, #KEYS) ~= 5 + 3 * i or smallInteger(balance.deleteKeyIndex, #KEYS) ~= 6 + 3 * i or smallInteger(balance.legacyDeleteKeyIndex, #KEYS) ~= 7 + 3 * i then
            technical("invalid_protocol", "invalid balance key indices")
        end
        local expectedMarker, replacements = KEYS[5 + 3 * i]:gsub(balance_cache_namespace_prefix, balance_deletion_marker_namespace_prefix, 1)
        if replacements ~= 1 or KEYS[6 + 3 * i] ~= expectedMarker or KEYS[7 + 3 * i] ~= KEYS[5 + 3 * i] .. balance_deletion_marker_suffix then
            technical("invalid_protocol", "invalid deletion marker key")
        end
        local seed = balance.snapshot
        requireObject(seed)
        if seed.organizationId ~= balance.organizationId or seed.ledgerId ~= balance.ledgerId then
            technical("invalid_protocol", "balance snapshot scope mismatch")
        end
        canonicalMoney(seed.available)
        canonicalMoney(seed.onHold)
        canonicalMoney(seed.overdraftUsed)
        canonicalMoney(seed.overdraftLimit)
        validateSnapshot(seed)
        if balance.balanceRef ~= seed.alias .. "#" .. seed.key or ids[seed.id] then technical("invalid_protocol", "invalid balance identity") end
        local scopeAlias = scopedBalanceRef(balance.organizationId, balance.ledgerId, seed.alias)
        if aliases[scopeAlias] and aliases[scopeAlias] ~= seed.accountId then technical("invalid_protocol", "alias identifies different accounts") end
        local account = accounts[seed.accountId]
        if account and (account.alias ~= seed.alias or account.assetCode ~= seed.assetCode or account.accountType ~= seed.accountType) then
            technical("invalid_protocol", "inconsistent account identity")
        end
        refs[scopeRef], ids[seed.id], accounts[seed.accountId], aliases[scopeAlias] = seed, true, seed, seed.accountId
    end
    -- Validate the account protection block that closes the key inventory: one
    -- closing, closed and administrative ownership key per account of the pool, in
    -- ascending account order. Every key must end in its own account identifier, so
    -- the controls of one account can never be served from another's key.
    local protection = {}
    local protectionBase, previousAccountId = 7 + 3 * #request.balances + grantCount, nil
    for i, account in ipairs(request.accounts) do
        requireObject(account)
        uuid(account.accountId)
        if previousAccountId and account.accountId <= previousAccountId then
            technical("invalid_protocol", "unordered account protection block")
        end
        previousAccountId = account.accountId
        local base = protectionBase + 3 * (i - 1)
        if smallInteger(account.closingKeyIndex, #KEYS) ~= base + 1 or smallInteger(account.closedKeyIndex, #KEYS) ~= base + 2 or smallInteger(account.ownershipKeyIndex, #KEYS) ~= base + 3 then
            technical("invalid_protocol", "invalid account protection key indices")
        end
        local suffix = ":" .. account.accountId
        for offset = 1, 3 do
            if KEYS[base + offset]:sub(-#suffix) ~= suffix then technical("invalid_protocol", "invalid account protection key") end
        end
        -- An empty token is the normal declaration of a caller that owns no
        -- admission; it may then use only balances the cache already holds.
        text(account.admissionToken, true)
        account.closingKeyIndex, account.closedKeyIndex, account.ownershipKeyIndex = base + 1, base + 2, base + 3
        protection[account.accountId] = account
    end
    for accountId in pairs(accounts) do
        if not protection[accountId] then
            technical("invalid_protocol", "balance account is missing from the account protection block")
        end
    end
    request.accountProtection = protection
    local primaryScope = request.organizationId .. ":" .. request.ledgerId
    local scopeKeyMap = {}
    scopeKeyMap[primaryScope] = { organizationId = request.organizationId, ledgerId = request.ledgerId,
        receiptKeyIndex = 3, guardKeyIndex = 4, protectionKeyIndex = 5,
        transactionIndexKeyIndex = 6, evidenceKeyIndex = 7 }
    if declaredScopes ~= nil then
        local first = declaredScopes[1]
        requireObject(first)
        if first.organizationId ~= request.organizationId or first.ledgerId ~= request.ledgerId or
            smallInteger(first.receiptKeyIndex, #KEYS) ~= 3 or smallInteger(first.guardKeyIndex, #KEYS) ~= 4 or
            smallInteger(first.protectionKeyIndex, #KEYS) ~= 5 or smallInteger(first.transactionIndexKeyIndex, #KEYS) ~= 6 or
            smallInteger(first.evidenceKeyIndex, #KEYS) ~= 7 then
            technical("invalid_protocol", "invalid primary scope key indices")
        end
        local base = protectionBase + 3 * #request.accounts
        for i = 2, #declaredScopes do
            local item = declaredScopes[i]
            requireObject(item)
            uuid(item.organizationId)
            uuid(item.ledgerId)
            local scope = item.organizationId .. ":" .. item.ledgerId
            if scopeKeyMap[scope] then technical("invalid_protocol", "duplicate scope key inventory") end
            local index = base + 5 * (i - 2)
            if smallInteger(item.receiptKeyIndex, #KEYS) ~= index + 1 or
                smallInteger(item.guardKeyIndex, #KEYS) ~= index + 2 or
                smallInteger(item.protectionKeyIndex, #KEYS) ~= index + 3 or
                smallInteger(item.transactionIndexKeyIndex, #KEYS) ~= index + 4 or
                smallInteger(item.evidenceKeyIndex, #KEYS) ~= index + 5 then
                technical("invalid_protocol", "invalid scope key indices")
            end
            local names = { "receipts", "guards", "protection", "transaction-index", "evidence" }
            for offset, name in ipairs(names) do
                local suffix = ":" .. name .. ":" .. scope
                if KEYS[index + offset]:sub(-#suffix) ~= suffix then technical("invalid_protocol", "invalid scoped coordination key") end
            end
            item.receiptKeyIndex, item.guardKeyIndex, item.protectionKeyIndex = index + 1, index + 2, index + 3
            item.transactionIndexKeyIndex, item.evidenceKeyIndex = index + 4, index + 5
            scopeKeyMap[scope] = item
        end
    end
    request.scopeKeyMap = scopeKeyMap
    -- Validate transaction correlation, guard advancement, recovery payloads,
    -- and the closed set of balance references used by requirements and postings.
    local transactions, grantOrdinal, postingCount = {}, 0, 0
    for _, transaction in ipairs(request.transactions) do
        requireObject(transaction)
        uuid(transaction.organizationId)
        uuid(transaction.ledgerId)
        uuid(transaction.id)
        if not scopeKeyMap[transaction.organizationId .. ":" .. transaction.ledgerId] then
            technical("invalid_protocol", "transaction scope has no coordination keys")
        end
        if transactions[transaction.id] or transaction.guardField ~= transaction.id or transaction.recoveryField ~= transaction.id .. ":" .. request.executionId then
            technical("invalid_protocol", "invalid transaction correlation")
        end
        transactions[transaction.id] = true
        bool(transaction.rejectBlockedBalances)
        text(transaction.expectedGuard, true)
        text(transaction.nextGuard, false)
        if transaction.expectedGuard == transaction.nextGuard then technical("invalid_protocol", "execution guard must advance") end
        text(transaction.completionPlan, false)
        text(transaction.action, false)
        requireArray(transaction.dependencies)
        if #transaction.dependencies > 2 then technical("invalid_protocol", "too many transaction dependencies") end
        local dependencyKinds, dependencyIdentities = {}, {}
        local parentTransactionID = transaction.parentTransactionId
        local hasParent = parentTransactionID ~= nil and parentTransactionID ~= nullValue
        if hasParent then uuid(parentTransactionID) end
        -- A parent may travel without an origin reference: once the parent
        -- execution is durable its evidence is reaped, leaving nothing to
        -- reference. A reference that is present must still name the parent.
        for _, dependency in ipairs(transaction.dependencies) do
            requireObject(dependency)
            if dependency.kind ~= "predecessor" and dependency.kind ~= "origin" then
                technical("invalid_protocol", "invalid transaction dependency kind")
            end
            if dependency.tenantId ~= request.tenantId or dependency.organizationId ~= transaction.organizationId or dependency.ledgerId ~= transaction.ledgerId then
                technical("invalid_protocol", "transaction dependency scope mismatch")
            end
            uuid(dependency.transactionId)
            uuid(dependency.executionId)
            if dependency.transactionId == transaction.id and dependency.executionId == request.executionId then
                technical("invalid_protocol", "cyclic transaction dependency")
            end
            local identity = dependency.kind .. ":" .. dependency.transactionId .. ":" .. dependency.executionId
            if dependencyKinds[dependency.kind] or dependencyIdentities[identity] then
                technical("invalid_protocol", "ambiguous transaction dependency")
            end
            dependencyKinds[dependency.kind], dependencyIdentities[identity] = true, true
            if dependency.kind == "predecessor" and dependency.transactionId ~= transaction.id then
                technical("invalid_protocol", "invalid predecessor transaction")
            end
            if dependency.kind == "origin" then
                if not hasParent or dependency.transactionId ~= parentTransactionID or dependency.transactionId == transaction.id then
                    technical("invalid_protocol", "invalid origin transaction")
                end
            end
        end
        requireArray(transaction.balanceRequirements)
        requireArray(transaction.postings)
        if #transaction.postings == 0 then technical("invalid_protocol", "empty transaction postings") end
        if #transaction.postings > maximumPostings - postingCount then
            technical("invalid_protocol", "execution exceeds posting limit")
        end
        postingCount = postingCount + #transaction.postings
        for _, requirement in ipairs(transaction.balanceRequirements) do
            validBalanceRequirement(requirement)
            if not refs[scopedBalanceRef(transaction.organizationId, transaction.ledgerId, requirement.balanceRef)] then technical("invalid_protocol", "invalid balance requirement reference") end
        end
        local postingRefs = {}
        for postingIndex, posting in ipairs(transaction.postings) do
            validPosting(posting)
            if postingRefs[posting.ref] or not refs[scopedBalanceRef(transaction.organizationId, transaction.ledgerId, posting.balanceRef)] then technical("invalid_protocol", "invalid posting reference") end
            postingRefs[posting.ref] = { posting = posting, index = postingIndex }
        end
        if transaction.accountBlockException ~= nil then
            local grant = transaction.accountBlockException
            requireObject(grant)
            grantOrdinal = grantOrdinal + 1
            uuid(grant.exceptionId)
            text(grant.alias, false)
            canonicalMoney(grant.amount)
            text(grant.primaryPostingRef, false)
            if cmp_decimal(grant.amount, "0") <= 0 then technical("invalid_protocol", "invalid account-block exception amount") end
            local primary = postingRefs[grant.primaryPostingRef]
            if not primary then technical("invalid_protocol", "unknown account-block exception posting") end
            local seed = refs[scopedBalanceRef(transaction.organizationId, transaction.ledgerId, primary.posting.balanceRef)]
            if seed.key == "overdraft" or seed.alias ~= grant.alias or cmp_decimal(primary.posting.amount, grant.amount) ~= 0 then
                technical("invalid_protocol", "account-block exception does not match primary posting")
            end
            local expectedKeyIndex = 7 + 3 * #request.balances + grantOrdinal
            if smallInteger(grant.keyIndex, #KEYS) ~= expectedKeyIndex then technical("invalid_protocol", "invalid account-block exception key index") end
            local suffix = ":" .. grant.exceptionId
            if KEYS[expectedKeyIndex]:sub(-#suffix) ~= suffix then technical("invalid_protocol", "invalid account-block exception key") end
            grant.keyIndex = expectedKeyIndex
            grant.primaryPostingIndex = primary.index
        end
    end
    return request
end
