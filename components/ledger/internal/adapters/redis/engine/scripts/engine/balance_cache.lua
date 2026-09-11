-- Cache records are dual-written during compatibility rollout. The uppercase
-- legacy field remains authoritative when both representations are present.
local cacheCompatibilityFieldNames = {
    id = "ID", accountId = "AccountID", accountType = "AccountType",
    assetCode = "AssetCode", alias = "Alias", key = "Key", direction = "Direction",
    balanceScope = "BalanceScope", available = "Available", onHold = "OnHold",
    overdraftUsed = "OverdraftUsed", overdraftLimit = "OverdraftLimit",
    version = "Version", allowSending = "AllowSending", allowReceiving = "AllowReceiving",
    allowOverdraft = "AllowOverdraft", overdraftLimitEnabled = "OverdraftLimitEnabled"
}

-- cachedField reads one logical balance field across the legacy and current
-- cache shapes, then falls back only when neither representation exists.
local function cachedField(blob, field, fallback)
    local compatible = blob[cacheCompatibilityFieldNames[field]]
    if compatible ~= nil then return compatible end
    if blob[field] ~= nil then return blob[field] end
    return fallback
end

-- cachedBool decodes both current JSON booleans and legacy 0/1 numeric tokens
-- into the single boolean representation used by the engine.
local function cachedBool(blob, field, fallback)
    local value = cachedField(blob, field, fallback)
    if type(value) == "boolean" then return value end
    local token = type(value) == "table" and numberTokens[value]
    if token == "0" then return false end
    if token == "1" then return true end
    technical("invalid_balance", "invalid cached boolean")
end

-- state extracts the mutable accounting boundary recorded before and after a
-- movement. numericVersion selects JSON-number encoding for persisted evidence.
local function state(snapshot, numericVersion)
    return {
        available = snapshot.available, onHold = snapshot.onHold,
        overdraftUsed = snapshot.overdraftUsed,
        version = numericVersion and numberToken(snapshot.version) or snapshot.version
    }
end

-- snapshotCopy creates an independent snapshot and optionally marks its version
-- for JSON-number encoding without changing the in-memory string representation.
local function snapshotCopy(snapshot, numericVersion)
    local result = clone(snapshot)
    if numericVersion then result.version = numberToken(snapshot.version) end
    return result
end

-- validateSnapshot establishes the complete balance invariant required by the
-- posting algebra: identity, supported account semantics, canonical money,
-- bounded version, and explicit permissions.
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
    -- Normalize values read from cache, then reject accounting states that the
    -- engine is not allowed to carry forward.
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

-- decodeBalance converts a live Redis cache record into the engine snapshot.
-- The immutable seed verifies identity only; cached accounting values always
-- win. The second return value requests limit normalization before execution.
local function decodeBalance(blob, seed, ref)
    requireObject(blob)
    if blob.SchemaVersion ~= nil and numberTokens[blob.SchemaVersion] ~= "2" then
        technical("invalid_balance", "unsupported cache schema version")
    end
    local version = cachedField(blob, "version")
    if type(version) == "table" then version = numberTokens[version] end
    -- Build one normalized view regardless of which compatibility fields were
    -- present in the stored document.
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
    -- A cache entry may provide live amounts, but it may never redirect an
    -- execution to a different balance or account identity.
    if snapshot.id ~= seed.id or snapshot.accountId ~= seed.accountId or snapshot.assetCode ~= seed.assetCode or snapshot.accountType ~= seed.accountType or snapshot.alias ~= seed.alias or snapshot.key ~= seed.key then
        technical("balance_identity_mismatch", "cached balance identity differs from execution scope")
    end
    for _, field in ipairs({ "available", "onHold", "overdraftUsed" }) do
        if money(snapshot[field]) ~= snapshot[field] then
            technical("invalid_balance", "noncanonical cached accounting money")
        end
    end
    -- Historical cache values may contain a noncanonical overdraft limit. Signal
    -- the Go repair path before any accounting mutation instead of repairing it
    -- as a side effect of this execution.
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

-- encodeBalance serializes the final live state in both current and legacy field
-- shapes so readers on either side of the rollout observe the same balance.
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
