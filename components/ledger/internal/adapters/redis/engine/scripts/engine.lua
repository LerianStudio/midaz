-- Copyright (c) 2026 Lerian Studio. All rights reserved.
-- Use of this source code is governed by the Elastic License 2.0
-- that can be found in the LICENSE file.

local function split_decimal(s)
    local sign = ""

    if s:sub(1, 1) == "-" then
        sign = "-"
        s = s:sub(2)
    end

    local intp, fracp = s:match("^(%d+)%.(%d+)$")
    if intp then
        return sign .. intp, fracp, sign ~= ""
    else
        return sign .. s, "", sign ~= ""
    end
end

local function rtrim_zeros(frac)
    frac = frac:gsub("0+$", "")
    return (frac == "" and "0") or frac
end

-- cmp_decimal compares two decimal strings and returns -1, 0, or 1 for
-- a<b, a==b, a>b. String-based (no tonumber) so magnitudes and scales
-- beyond IEEE-754 double precision still compare correctly.
local function cmp_decimal(a, b)
    a = tostring(a)
    b = tostring(b)
    local ai, af, a_negative = split_decimal(a)
    local bi, bf, b_negative = split_decimal(b)

    if a_negative then ai = ai:sub(2) end
    if b_negative then bi = bi:sub(2) end

    ai = ai:gsub("^0+", "")
    if ai == "" then ai = "0" end
    bi = bi:gsub("^0+", "")
    if bi == "" then bi = "0" end

    -- Zero has no sign: -0, 0.00, and 0 all compare equal to zero.
    if a_negative and ai == "0" and rtrim_zeros(af) == "0" then a_negative = false end
    if b_negative and bi == "0" and rtrim_zeros(bf) == "0" then b_negative = false end

    if a_negative ~= b_negative then
        if a_negative then return -1 end
        return 1
    end

    local maxFrac = math.max(#af, #bf)
    local afPadded = af .. string.rep("0", maxFrac - #af)
    local bfPadded = bf .. string.rep("0", maxFrac - #bf)

    local unsigned
    if #ai ~= #bi then
        unsigned = (#ai < #bi) and -1 or 1
    elseif ai ~= bi then
        unsigned = (ai < bi) and -1 or 1
    elseif afPadded ~= bfPadded then
        unsigned = (afPadded < bfPadded) and -1 or 1
    else
        unsigned = 0
    end

    if a_negative then
        return -unsigned
    end
    return unsigned
end

local sub_decimal

local function add_decimal(a, b)
    a = tostring(a)
    b = tostring(b)
    local ai, af, a_negative = split_decimal(a)
    local bi, bf, b_negative = split_decimal(b)

    if a_negative and b_negative then
        local result = add_decimal(a:sub(2), b:sub(2))
        return "-" .. result
    end

    if a_negative then
        return sub_decimal(b, a:sub(2))
    end

    if b_negative then
        return sub_decimal(a, b:sub(2))
    end

    if ai:sub(1, 1) == "-" then ai = ai:sub(2) end
    if bi:sub(1, 1) == "-" then bi = bi:sub(2) end

    if #af < #bf then
        af = af .. string.rep("0", #bf - #af)
    elseif #bf < #af then
        bf = bf .. string.rep("0", #af - #bf)
    end

    local carry = 0
    local frac_sum = {}
    for i = #af, 1, -1 do
        local da = tonumber(af:sub(i, i))
        local db = tonumber(bf:sub(i, i))
        local s = da + db + carry
        carry = math.floor(s / 10)
        frac_sum[#af - i + 1] = tostring(s % 10)
    end

    local rii = ai:reverse()
    local rbi = bi:reverse()
    local max_i = math.max(#rii, #rbi)
    local int_sum = {}
    for i = 1, max_i do
        local da = tonumber(rii:sub(i, i)) or 0
        local db = tonumber(rbi:sub(i, i)) or 0
        local s = da + db + carry
        carry = math.floor(s / 10)
        int_sum[i] = tostring(s % 10)
    end
    if carry > 0 then
        int_sum[#int_sum + 1] = tostring(carry)
    end

    local int_res = table.concat(int_sum):reverse()
    local frac_res = table.concat(frac_sum):reverse()
    frac_res = rtrim_zeros(frac_res)

    if frac_res == "0" then
        return int_res
    end
    return int_res .. "." .. frac_res
end

sub_decimal = function(a, b)
    a = tostring(a)
    b = tostring(b)
    local ai, af, a_negative = split_decimal(a)
    local bi, bf, b_negative = split_decimal(b)

    if a_negative and b_negative then
        return sub_decimal(b:sub(2), a:sub(2))
    end

    if a_negative then
        local result = add_decimal(a:sub(2), b)
        return "-" .. result
    end

    if b_negative then
        return add_decimal(a, b:sub(2))
    end

    if cmp_decimal(a, b) < 0 then
        local result = sub_decimal(b, a)
        return "-" .. result
    end

    if ai:sub(1, 1) == "-" then ai = ai:sub(2) end
    if bi:sub(1, 1) == "-" then bi = bi:sub(2) end

    if #af < #bf then
        af = af .. string.rep("0", #bf - #af)
    elseif #bf < #af then
        bf = bf .. string.rep("0", #af - #bf)
    end

    local borrow = 0
    local frac_res_tbl = {}
    for i = #af, 1, -1 do
        local da = tonumber(af:sub(i, i))
        local db = tonumber(bf:sub(i, i))
        local diff = da - db - borrow
        if diff < 0 then
            diff = diff + 10
            borrow = 1
        else
            borrow = 0
        end
        frac_res_tbl[#af - i + 1] = tostring(diff)
    end

    local rii = ai:reverse()
    local rbi = bi:reverse()
    local max_i = math.max(#rii, #rbi)
    local int_res_tbl = {}
    for i = 1, max_i do
        local da = tonumber(rii:sub(i, i)) or 0
        local db = tonumber(rbi:sub(i, i)) or 0
        local diff = da - db - borrow
        if diff < 0 then
            diff = diff + 10
            borrow = 1
        else
            borrow = 0
        end
        int_res_tbl[i] = tostring(diff)
    end

    -- cmp_decimal above guarantees a >= b at this point, so the integer loop
    -- can never underflow past the most significant digit.
    if borrow ~= 0 then
        error("sub_decimal: borrow invariant violated")
    end

    local res_int_rev = table.concat(int_res_tbl)
    local res_int = res_int_rev:reverse():gsub("^0+", "")
    if res_int == "" then
        res_int = "0"
    end

    local frac_normal = table.concat(frac_res_tbl):reverse()
    frac_normal = rtrim_zeros(frac_normal)

    if frac_normal == "0" then
        return res_int
    end
    return res_int .. "." .. frac_normal
end

local function min_decimal(a, b)
    if cmp_decimal(a, b) < 0 then return a end
    return b
end

-- ARGV: wire JSON, maximum request bytes, maximum total prepared bytes.
-- Physical Redis keys are supplied only through KEYS. Financial decimals and
-- versions remain text; numeric JSON tokens are preserved without a double round-trip.

local arrayKinds = {}
local numberTokens = {}
local nullValue = {}
local commitStarted = false
local MAX_JSON_DEPTH = 128

local function object() return {} end
local function array()
    local value = {}
    arrayKinds[value] = true
    return value
end

local function numberToken(value)
    local token = {}
    numberTokens[token] = value
    return token
end

local function technical(code, message)
    error({ kind = "technical", code = code, message = message }, 0)
end

local function refuse(code, transactionIndex, postingIndex, balanceRef)
    error({
        kind = "failure", code = code, transactionIndex = transactionIndex,
        postingIndex = postingIndex, balanceRef = balanceRef or ""
    }, 0)
end

local function validUTF8(value)
    local i = 1
    while i <= #value do
        local a = value:byte(i)
        local count = 0
        if a < 128 then count = 0
        elseif a >= 194 and a <= 223 then count = 1
        elseif a >= 224 and a <= 239 then count = 2
        elseif a >= 240 and a <= 244 then count = 3
        else return false end
        if i + count > #value then return false end
        for j = 1, count do
            local b = value:byte(i + j)
            if b < 128 or b > 191 then return false end
        end
        local b = value:byte(i + 1)
        if count == 2 and ((a == 224 and b < 160) or (a == 237 and b > 159)) then return false end
        if count == 3 and ((a == 240 and b < 144) or (a == 244 and b > 143)) then return false end
        i = i + count + 1
    end
    return true
end

local function decodeJSON(raw)
    if type(raw) ~= "string" then technical("invalid_json", "JSON must be text") end
    local position = 1
    local parseValue
    local function skipWhitespace()
        while position <= #raw and raw:sub(position, position):match("[ \t\r\n]") do position = position + 1 end
    end
    local function parseString()
        local start = position
        position = position + 1
        while position <= #raw do
            local char = raw:sub(position, position)
            if char == '"' then
                position = position + 1
                local ok, value = pcall(cjson.decode, raw:sub(start, position - 1))
                if not ok or type(value) ~= "string" or not validUTF8(value) then
                    technical("invalid_json", "invalid JSON string")
                end
                return value
            end
            if char:byte() < 32 then technical("invalid_json", "unescaped control character") end
            if char == "\\" then
                position = position + 1
                local escape = raw:sub(position, position)
                if escape == "u" then
                    local hex = raw:sub(position + 1, position + 4)
                    if #hex ~= 4 or not hex:match("^%x%x%x%x$") then technical("invalid_json", "invalid Unicode escape") end
                    position = position + 4
                elseif not escape:match('^["\\/bfnrt]$') then
                    technical("invalid_json", "invalid JSON escape")
                end
            end
            position = position + 1
        end
        technical("invalid_json", "unterminated JSON string")
    end
    local function parseNumber()
        local start = position
        if raw:sub(position, position) == "-" then position = position + 1 end
        local first = raw:sub(position, position)
        if first == "0" then
            position = position + 1
        elseif first:match("^[1-9]$") then
            repeat position = position + 1 until not raw:sub(position, position):match("^%d$")
        else technical("invalid_json", "invalid JSON number") end
        if raw:sub(position, position) == "." then
            position = position + 1
            if not raw:sub(position, position):match("^%d$") then technical("invalid_json", "invalid JSON fraction") end
            repeat position = position + 1 until not raw:sub(position, position):match("^%d$")
        end
        if raw:sub(position, position):match("^[eE]$") then
            position = position + 1
            if raw:sub(position, position):match("^[+-]$") then position = position + 1 end
            if not raw:sub(position, position):match("^%d$") then technical("invalid_json", "invalid JSON exponent") end
            repeat position = position + 1 until not raw:sub(position, position):match("^%d$")
        end
        return numberToken(raw:sub(start, position - 1))
    end
    parseValue = function(depth)
        if depth > MAX_JSON_DEPTH then technical("invalid_json", "JSON nesting limit exceeded") end
        skipWhitespace()
        local char = raw:sub(position, position)
        if char == '"' then return parseString() end
        if char == "{" then
            local value = object()
            position = position + 1
            skipWhitespace()
            if raw:sub(position, position) == "}" then position = position + 1 return value end
            while true do
                if raw:sub(position, position) ~= '"' then technical("invalid_json", "object key must be a string") end
                local key = parseString()
                if value[key] ~= nil then technical("invalid_json", "duplicate JSON object key") end
                skipWhitespace()
                if raw:sub(position, position) ~= ":" then technical("invalid_json", "missing JSON colon") end
                position = position + 1
                value[key] = parseValue(depth + 1)
                skipWhitespace()
                local delimiter = raw:sub(position, position)
                position = position + 1
                if delimiter == "}" then return value end
                if delimiter ~= "," then technical("invalid_json", "invalid JSON object delimiter") end
                skipWhitespace()
            end
        end
        if char == "[" then
            local value = array()
            position = position + 1
            skipWhitespace()
            if raw:sub(position, position) == "]" then position = position + 1 return value end
            while true do
                value[#value + 1] = parseValue(depth + 1)
                skipWhitespace()
                local delimiter = raw:sub(position, position)
                position = position + 1
                if delimiter == "]" then return value end
                if delimiter ~= "," then technical("invalid_json", "invalid JSON array delimiter") end
            end
        end
        for literal, value in pairs({ ["true"] = true, ["false"] = false, ["null"] = nullValue }) do
            if raw:sub(position, position + #literal - 1) == literal then
                position = position + #literal
                return value
            end
        end
        if char == "-" or char:match("^%d$") then return parseNumber() end
        technical("invalid_json", "invalid JSON value")
    end
    local value = parseValue(0)
    skipWhitespace()
    if position ~= #raw + 1 then technical("invalid_json", "trailing JSON content") end
    return value
end

local function encodeJSON(value, depth)
    depth = depth or 0
    if depth > MAX_JSON_DEPTH then technical("serialization_failed", "JSON nesting limit exceeded") end
    if value == nullValue then return "null" end
    local kind = type(value)
    if kind == "string" then return cjson.encode(value) end
    if kind == "boolean" then return value and "true" or "false" end
    if kind == "number" then
        if value ~= math.floor(value) or value > 9007199254740991 or value < -9007199254740991 then
            technical("serialization_failed", "unsafe JSON numeric value")
        end
        return string.format("%.0f", value)
    end
    if kind ~= "table" then technical("serialization_failed", "unsupported JSON value") end
    if numberTokens[value] then return numberTokens[value] end
    local parts = {}
    if arrayKinds[value] then
        for i = 1, #value do parts[i] = encodeJSON(value[i], depth + 1) end
        return "[" .. table.concat(parts, ",") .. "]"
    end
    local keys = {}
    for key, _ in pairs(value) do
        if type(key) ~= "string" then technical("serialization_failed", "invalid JSON object key") end
        keys[#keys + 1] = key
    end
    table.sort(keys)
    for _, key in ipairs(keys) do parts[#parts + 1] = cjson.encode(key) .. ":" .. encodeJSON(value[key], depth + 1) end
    return "{" .. table.concat(parts, ",") .. "}"
end

local function isObject(value)
    return type(value) == "table" and not arrayKinds[value] and not numberTokens[value] and value ~= nullValue
end

local function requireObject(value)
    if not isObject(value) then technical("invalid_protocol", "expected JSON object") end
end

local function requireArray(value)
    if type(value) ~= "table" or not arrayKinds[value] then technical("invalid_protocol", "expected JSON array") end
end

local function text(value, allowEmpty)
    if type(value) ~= "string" or (not allowEmpty and value == "") or not validUTF8(value) then
        technical("invalid_protocol", "invalid text field")
    end
    return value
end

local function uuid(value)
    text(value, false)
    if #value ~= 36 or not value:match("^%x%x%x%x%x%x%x%x%-%x%x%x%x%-%x%x%x%x%-%x%x%x%x%-%x%x%x%x%x%x%x%x%x%x%x%x$") or value == "00000000-0000-0000-0000-000000000000" or value ~= value:lower() then
        technical("invalid_protocol", "invalid UUID")
    end
    return value
end

local function integerText(value, maxValue)
    if type(value) ~= "string" or not value:match("^%d+$") or (#value > 1 and value:sub(1, 1) == "0") or #value > #maxValue or (#value == #maxValue and value > maxValue) then
        technical("invalid_protocol", "invalid integer text")
    end
    return value
end

local function smallInteger(value, maximum)
    local token = type(value) == "table" and numberTokens[value]
    if not token then technical("invalid_protocol", "expected integer JSON token") end
    integerText(token, tostring(maximum))
    return tonumber(token)
end

local function bool(value)
    if type(value) ~= "boolean" then technical("invalid_protocol", "invalid boolean") end
    return value
end

local function clone(value)
    local result = {}
    for key, item in pairs(value) do result[key] = item end
    return result
end

local function logicalRef(value)
    text(value, false)
    if not value:match("^[^#]+#[^#]+$") or value:find("[{}%c]") or value:match("^balance:") or value:match("^tenant:") then
        technical("invalid_protocol", "invalid logical balance reference")
    end
    return value
end

local function money(value)
    if type(value) ~= "string" then technical("invalid_balance", "money must be a decimal string") end
    local negative = value:sub(1, 1) == "-"
    local unsigned = negative and value:sub(2) or value
    local integer, fraction = unsigned:match("^(%d+)%.(%d+)$")
    if not integer then
        integer = unsigned:match("^(%d+)$")
        fraction = ""
    end
    if not integer then technical("invalid_balance", "invalid decimal text") end
    integer = integer:gsub("^0+", "")
    if integer == "" then integer = "0" end
    fraction = fraction:gsub("0+$", "")
    local result = integer
    if fraction ~= "" then result = result .. "." .. fraction end
    if negative and result ~= "0" then result = "-" .. result end
    return result
end

local function canonicalMoney(value)
    local normalized = money(value)
    if normalized ~= value then technical("invalid_protocol", "noncanonical decimal in wire") end
    return normalized
end

local function nonnegative(value)
    value = money(value)
    if value:sub(1, 1) == "-" then technical("invalid_balance", "negative accounting state") end
    return value
end

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

local function redisType(key)
    local result = redis.call("TYPE", key)
    if type(result) == "table" then return result.ok end
    return result
end

local function expectRedisType(key, expected)
    local actual = redisType(key)
    if actual ~= "none" and actual ~= expected then
        technical("wrong_key_type", "unexpected Redis key type")
    end
end

local function positiveBudget(raw)
    integerText(raw, "2147483647")
    local value = tonumber(raw)
    if value < 1 then technical("invalid_protocol", "byte budget must be positive") end
    return value
end

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

local function validBalanceRequirement(requirement)
    requireObject(requirement)
    logicalRef(requirement.balanceRef)
    text(requirement.assetCode, false)
    if requirement.permission ~= "send" and requirement.permission ~= "receive" then
        technical("invalid_protocol", "unknown balance permission")
    end
    bool(requirement.forbidExternal)
end

local function decodeRequest(raw)
    local request = decodeJSON(raw)
    requireObject(request)
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
    if #request.transactions == 0 or #KEYS ~= 5 + 2 * #request.balances then technical("invalid_protocol", "invalid execution cardinality") end
    local seenKeys = {}
    for _, key in ipairs(KEYS) do
        local _, opens = key:gsub("{", "")
        local _, closes = key:gsub("}", "")
        if not key:find(transaction_hash_tag, 1, true) or opens ~= 1 or closes ~= 1 or seenKeys[key] then
            technical("invalid_protocol", "invalid physical key inventory")
        end
        seenKeys[key] = true
    end
    local refs, ids, accounts, aliases = {}, {}, {}, {}
    for i, balance in ipairs(request.balances) do
        requireObject(balance)
        logicalRef(balance.balanceRef)
        if refs[balance.balanceRef] then technical("invalid_protocol", "duplicate balance reference") end
        if smallInteger(balance.keyIndex, #KEYS) ~= 4 + 2 * i or smallInteger(balance.deleteKeyIndex, #KEYS) ~= 5 + 2 * i then
            technical("invalid_protocol", "invalid balance key indices")
        end
        if KEYS[5 + 2 * i] ~= KEYS[4 + 2 * i] .. balance_deletion_marker_suffix then technical("invalid_protocol", "invalid deletion marker key") end
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
    local transactions = {}
    for _, transaction in ipairs(request.transactions) do
        requireObject(transaction)
        uuid(transaction.id)
        if transactions[transaction.id] or transaction.guardField ~= transaction.id or transaction.recoveryField ~= transaction.id .. ":" .. request.executionId then
            technical("invalid_protocol", "invalid transaction correlation")
        end
        transactions[transaction.id] = true
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

local function validateStoredResponse(raw, request)
    local response = decodeJSON(raw)
    requireObject(response)
    if smallInteger(response.protocolVersion, 1) ~= 1 then technical("invalid_receipt", "unsupported saved response") end
    requireArray(response.movements)
    requireArray(response.final)
    if #response.movements == 0 or #response.final == 0 then
        technical("invalid_receipt", "stored execution receipt has no accounting movements")
    end
    local postings, seeds = {}, {}
    for _, transaction in ipairs(request.transactions) do
        local refs = {}
        for _, posting in ipairs(transaction.postings) do refs[posting.ref] = posting end
        postings[transaction.id] = refs
    end
    for _, balance in ipairs(request.balances) do seeds[balance.balanceRef] = balance.snapshot end
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

local function decodeStoredReceipt(raw, request)
    local receipt = decodeJSON(raw)
    requireObject(receipt)
    if smallInteger(receipt.formatVersion, 1) ~= 1 or receipt.tenantId ~= request.tenantId or receipt.organizationId ~= request.organizationId or receipt.ledgerId ~= request.ledgerId or receipt.executionId ~= request.executionId then
        technical("invalid_receipt", "saved receipt identity mismatch")
    end
    if receipt.intentFingerprint ~= request.intentFingerprint then
        technical("execution_fingerprint_conflict", "execution identity was reused for a different intent")
    end
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

local function prepareExecutionProtection(request)
    expectRedisType(KEYS[3], "hash")
    local replay = storedReceipt(request)
    if replay then return replay end

    expectRedisType(KEYS[1], "zset")
    expectRedisType(KEYS[2], "hash")
    expectRedisType(KEYS[4], "hash")
    local protectionKey = KEYS[5]
    expectRedisType(protectionKey, "hash")

    local preparedProtection = {}
    for _, transaction in ipairs(request.transactions) do
        local current = redis.call("HGET", KEYS[4], transaction.guardField)
        if (current or "") ~= transaction.expectedGuard then
            technical("execution_guard_conflict", "transaction execution guard has changed")
        end
        if redis.call("HEXISTS", KEYS[2], transaction.recoveryField) == 1 then
            technical("execution_outcome_unknown", "recovery exists without a complete execution receipt")
        end
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

local function loadBalancePool(request)
    local pool, companions = {}, {}
    local normalization = array()
    for i, balance in ipairs(request.balances) do
        local keyIndex, markerIndex = 4 + 2 * i, 5 + 2 * i
        expectRedisType(KEYS[keyIndex], "string")
        expectRedisType(KEYS[markerIndex], "string")
        local raw = redis.call("GET", KEYS[keyIndex])
        local blob, current
        if raw then
            blob = decodeJSON(raw)
            local repair
            current, repair = decodeBalance(blob, balance.snapshot, balance.balanceRef)
            if repair then normalization[#normalization + 1] = KEYS[keyIndex] end
        else
            current = clone(balance.snapshot)
            current.balanceRef = balance.balanceRef
        end
        local item = {
            current = current, blob = blob, keyIndex = keyIndex,
            deleted = redis.call("EXISTS", KEYS[markerIndex]) == 1
        }
        pool[balance.balanceRef] = item
        if current.key == "overdraft" then
            if companions[current.accountId] then technical("invalid_balance", "multiple overdraft companions for one account") end
            companions[current.accountId] = item
        end
    end
    if #normalization > 0 then error({ kind = "normalization", keys = normalization }, 0) end

    return pool, companions
end

local function validateLiveBalanceAvailability(request, pool)
    for txIndex, transaction in ipairs(request.transactions) do
        for _, requirement in ipairs(transaction.balanceRequirements) do
            if pool[requirement.balanceRef].deleted then
                refuse("balance_deleted", txIndex - 1, -1, requirement.balanceRef)
            end
        end
        for postingIndex, posting in ipairs(transaction.postings) do
            if pool[posting.balanceRef].deleted then
                refuse("balance_deleted", txIndex - 1, postingIndex - 1, posting.balanceRef)
            end
        end
    end
end

local function applyTransactionsInMemory(request, pool, companions)
    local movements, touched, touchedSet, transactionResults = array(), {}, {}, {}
    local function touch(item, txIndex, postingIndex)
        if item.deleted then refuse("balance_deleted", txIndex, postingIndex, item.current.balanceRef) end
    end

    for txIndex, transaction in ipairs(request.transactions) do
        local txMovements, txTouched, txTouchedSet = array(), {}, {}
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
        for postingIndex, posting in ipairs(transaction.postings) do
            local item = pool[posting.balanceRef]
            touch(item, txIndex - 1, postingIndex - 1)
            local current, nextState = item.current, clone(item.current)
            local external = current.accountType == "external"
            local amount = posting.amount
            local postingType = posting.type
            local primaryAmount, delta = postingAlgebra[postingType](current, nextState, posting, txIndex - 1, postingIndex - 1)
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

            local companion, companionNext, companionAmount, companionType
            if cmp_decimal(delta, "0") ~= 0 then
                companion = companions[current.accountId]
                if not companion then refuse("overdraft_companion_missing", txIndex - 1, postingIndex - 1, posting.balanceRef) end
                if companion == item or companion.current.direction ~= "debit" or companion.current.balanceScope ~= "internal" or companion.current.accountType == "external" or companion.current.assetCode ~= current.assetCode then
                    technical("invalid_companion", "invalid overdraft companion")
                end
                touch(companion, txIndex - 1, postingIndex - 1)
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
        local txFinal = array()
        for _, item in ipairs(txTouched) do txFinal[#txFinal + 1] = snapshotCopy(item.current, false) end
        transactionResults[#transactionResults + 1] = { movements = txMovements, final = txFinal }
    end

    return movements, touched, transactionResults
end

local function prepareExecutionWrites(request, maximumPrepared, preparedProtection, movements, touched, transactionResults)
    local final = array()
    for _, item in ipairs(touched) do final[#final + 1] = snapshotCopy(item.current, false) end
    local response = encodeJSON({ protocolVersion = 1, movements = movements, final = final })
    local preparedBytes = #response
    if preparedBytes > maximumPrepared then technical("prepared_bytes_exceeded", "response exceeds prepared byte budget") end
    if #movements == 0 then return response, nil end

    local function charge(value)
        if #value > maximumPrepared - preparedBytes then technical("prepared_bytes_exceeded", "execution exceeds prepared byte budget") end
        preparedBytes = preparedBytes + #value
        return value
    end
    for _, coordinator in ipairs(preparedProtection) do
        charge(coordinator.field)
        charge(coordinator.value)
    end
    local preparedBalances, preparedRecoverRecords = {}, {}
    for _, item in ipairs(touched) do
        preparedBalances[#preparedBalances + 1] = { key = KEYS[item.keyIndex], value = charge(encodeBalance(item)) }
    end
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

local function commitPreparedExecution(request, protectionKey, preparedBalances, preparedRecoverRecords, preparedProtection, receipt)
    local now = redis.call("TIME")
    local score = tonumber(now[1]) + tonumber(now[2]) / 1000000

    -- Only prepared commands remain. Runtime failures here are indeterminate;
    -- Redis script execution does not roll back earlier successful writes.
    commitStarted = true
    for _, balance in ipairs(preparedBalances) do redis.call("SET", balance.key, balance.value, "EX", balance_cache_ttl_seconds) end
    for _, balance in ipairs(preparedBalances) do redis.call("ZADD", KEYS[1], score, balance.key) end
    for _, recoverRecord in ipairs(preparedRecoverRecords) do redis.call("HSET", KEYS[2], recoverRecord.field, recoverRecord.value) end
    for _, transaction in ipairs(request.transactions) do redis.call("HSET", KEYS[4], transaction.guardField, transaction.nextGuard) end
    for _, coordinator in ipairs(preparedProtection) do redis.call("HSET", protectionKey, coordinator.field, coordinator.value) end
    redis.call("HSET", KEYS[3], request.receiptField, receipt)
end

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

local function main()
    if #ARGV ~= 3 or #KEYS < 5 then technical("invalid_protocol", "invalid script argument count") end
    local maximumRequest, maximumPrepared = positiveBudget(ARGV[2]), positiveBudget(ARGV[3])
    if #ARGV[1] > maximumRequest then technical("request_bytes_exceeded", "request exceeds byte budget") end
    local request = decodeRequest(ARGV[1])
    return execute(request, maximumPrepared)
end

local ok, result = pcall(main)
if ok then return result end
if commitStarted then
    return redis.error_reply("MIDAZ_ENGINE_TECH_V1 " .. encodeJSON({ code = "indeterminate", message = "accounting commit failed after writes may have started" }))
end
if type(result) == "table" and result.kind == "failure" then
    return redis.error_reply("MIDAZ_ENGINE_V1 " .. encodeJSON({
        code = result.code, transactionIndex = result.transactionIndex,
        postingIndex = result.postingIndex, balanceRef = result.balanceRef
    }))
end
if type(result) == "table" and result.kind == "normalization" then
    return redis.error_reply("BALANCE_LIMIT_NORMALIZATION_REQUIRED:" .. encodeJSON(result.keys))
end
if type(result) == "table" and result.kind == "technical" then
    return redis.error_reply("MIDAZ_ENGINE_TECH_V1 " .. encodeJSON({ code = result.code, message = result.message }))
end
return redis.error_reply("MIDAZ_ENGINE_TECH_V1 " .. encodeJSON({ code = "script_runtime_failed", message = "accounting execution failed before commit" }))
