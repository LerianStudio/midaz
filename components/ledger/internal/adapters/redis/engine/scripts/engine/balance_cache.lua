local cacheCompatibilityFieldNames = {
    id = "ID", accountId = "AccountID", accountType = "AccountType",
    assetCode = "AssetCode", alias = "Alias", key = "Key", direction = "Direction",
    balanceScope = "BalanceScope", available = "Available", onHold = "OnHold",
    overdraftUsed = "OverdraftUsed", overdraftLimit = "OverdraftLimit",
    version = "Version", allowSending = "AllowSending", allowReceiving = "AllowReceiving",
    allowOverdraft = "AllowOverdraft", overdraftLimitEnabled = "OverdraftLimitEnabled"
}

local function cachedField(blob, field, fallback)
    local compatible = blob[cacheCompatibilityFieldNames[field]]
    if compatible ~= nil then return compatible end
    if blob[field] ~= nil then return blob[field] end
    return fallback
end

local function cachedBool(blob, field, fallback)
    local value = cachedField(blob, field, fallback)
    if type(value) == "boolean" then return value end
    local token = type(value) == "table" and numberTokens[value]
    if token == "0" then return false end
    if token == "1" then return true end
    technical("invalid_balance", "invalid cached boolean")
end

local function state(snapshot, numericVersion)
    return {
        available = snapshot.available, onHold = snapshot.onHold,
        overdraftUsed = snapshot.overdraftUsed,
        version = numericVersion and numberToken(snapshot.version) or snapshot.version
    }
end

local function snapshotCopy(snapshot, numericVersion)
    local result = clone(snapshot)
    if numericVersion then result.version = numberToken(snapshot.version) end
    return result
end

local function validateSnapshot(snapshot)
    requireObject(snapshot)
    uuid(snapshot.id)
    uuid(snapshot.accountId)
    text(snapshot.alias, false)
    text(snapshot.key, false)
    text(snapshot.accountType, false)
    text(snapshot.assetCode, false)
    if snapshot.direction ~= "" and snapshot.direction ~= "credit" and snapshot.direction ~= "debit" then
        technical("invalid_balance", "unsupported account direction")
    end
    if snapshot.balanceScope ~= "transactional" and snapshot.balanceScope ~= "internal" then
        technical("invalid_balance", "unsupported balance scope")
    end
    snapshot.available = money(snapshot.available)
    snapshot.onHold = nonnegative(snapshot.onHold)
    snapshot.overdraftUsed = nonnegative(snapshot.overdraftUsed)
    snapshot.overdraftLimit = nonnegative(snapshot.overdraftLimit)
    if snapshot.accountType ~= "external" and cmp_decimal(snapshot.available, "0") < 0 then
        technical("invalid_balance", "negative nonexternal available balance")
    end
    integerText(snapshot.version, "9223372036854775807")
    bool(snapshot.allowSending)
    bool(snapshot.allowReceiving)
    bool(snapshot.allowOverdraft)
    bool(snapshot.overdraftLimitEnabled)
end

local function decodeBalance(blob, seed, ref)
    requireObject(blob)
    if blob.SchemaVersion ~= nil and numberTokens[blob.SchemaVersion] ~= "2" then
        technical("invalid_balance", "unsupported cache schema version")
    end
    local version = cachedField(blob, "version")
    if type(version) == "table" then version = numberTokens[version] end
    local snapshot = {
        id = cachedField(blob, "id"), accountId = cachedField(blob, "accountId"),
        accountType = cachedField(blob, "accountType"), assetCode = cachedField(blob, "assetCode"),
        alias = cachedField(blob, "alias", seed.alias), key = cachedField(blob, "key"),
        direction = cachedField(blob, "direction", ""), balanceScope = cachedField(blob, "balanceScope", "transactional"),
        available = cachedField(blob, "available"), onHold = cachedField(blob, "onHold"),
        overdraftUsed = cachedField(blob, "overdraftUsed", "0"),
        overdraftLimit = cachedField(blob, "overdraftLimit", "0"), version = version,
        allowSending = cachedBool(blob, "allowSending", false), allowReceiving = cachedBool(blob, "allowReceiving", false),
        allowOverdraft = cachedBool(blob, "allowOverdraft", false), overdraftLimitEnabled = cachedBool(blob, "overdraftLimitEnabled", false),
        balanceRef = ref
    }
    if snapshot.key == ref then snapshot.key = seed.key end
    if snapshot.id ~= seed.id or snapshot.accountId ~= seed.accountId or snapshot.assetCode ~= seed.assetCode or snapshot.accountType ~= seed.accountType or snapshot.alias ~= seed.alias or snapshot.key ~= seed.key then
        technical("balance_identity_mismatch", "cached balance identity differs from execution scope")
    end
    for _, field in ipairs({ "available", "onHold", "overdraftUsed" }) do
        if money(snapshot[field]) ~= snapshot[field] then
            technical("invalid_balance", "noncanonical cached accounting money")
        end
    end
    local rawLimit = snapshot.overdraftLimit
    local ok, normalized = pcall(money, rawLimit)
    if type(rawLimit) ~= "string" then technical("invalid_balance", "invalid cached limit type") end
    if not ok or normalized ~= rawLimit then
        snapshot.overdraftLimit = "0"
        validateSnapshot(snapshot)
        return snapshot, true
    end
    validateSnapshot(snapshot)
    return snapshot, false
end

local function encodeBalance(item)
    local blob = item.blob and clone(item.blob) or object()
    for field, compatible in pairs(cacheCompatibilityFieldNames) do
        local value = item.current[field]
        blob[field] = value
        if field == "version" then blob[compatible] = numberToken(value)
        elseif type(value) == "boolean" then blob[compatible] = value and 1 or 0
        else blob[compatible] = value end
    end
    blob.SchemaVersion = 2
    return encodeJSON(blob)
end
