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

local function startsWithMinus(s)
    return s:sub(1, 1) == "-"
end

-- isPositive checks if a decimal string represents a value greater than zero
-- Returns true if the value is positive (not negative and not zero)
local function isPositive(s)
    if startsWithMinus(s) then
        return false
    end
    -- Check if it's zero (could be "0", "0.0", "0.00", etc.)
    local normalized = s:gsub("%.?0+$", ""):gsub("^0+", "")
    return normalized ~= "" and normalized ~= "."
end

local function cloneBalance(tbl)
    local copy = {}
    for k, v in pairs(tbl) do
        copy[k] = v
    end
    return copy
end

-- min_decimal returns the smaller of two decimal strings, comparing
-- directly via cmp_decimal — no subtraction step — so precision matches
-- add_decimal/sub_decimal for values that overflow Lua's double
-- representation.
local function min_decimal(a, b)
    if cmp_decimal(a, b) < 0 then
        return a
    end
    return b
end

-- canonical_decimal_text converts a trusted decimal representation to the
-- canonical string shape used by the lower-camel cache contract. Numeric
-- inputs are first encoded by cjson so the projection reflects the value that
-- the legacy uppercase field will actually serialize, including any rounding
-- that already occurred in Lua. This is deliberately not a precision repair.
local function canonical_decimal_text(value)
    local text
    if type(value) == "string" then
        text = value
    elseif type(value) == "number" then
        text = cjson.encode(value)
    else
        return nil
    end

    local mantissa, exponentText = text:match("^([^eE]+)[eE]([+-]?%d+)$")
    local exponent = 0
    if mantissa then
        exponent = tonumber(exponentText)
        -- cjson-produced IEEE-754 exponents are tiny; cap externally supplied
        -- legacy text so projection validation cannot allocate unbounded padding.
        if not exponent or math.abs(exponent) > 10000 then
            return nil
        end
    else
        mantissa = text
    end

    local negative = false
    if mantissa:sub(1, 1) == "-" then
        negative = true
        mantissa = mantissa:sub(2)
    end

    local integer, fraction = mantissa:match("^(%d+)%.(%d+)$")
    if not integer then
        integer = mantissa:match("^(%d+)$")
        fraction = ""
    end
    if not integer then
        return nil
    end

    local digits = integer .. fraction
    local decimalPosition = #integer + exponent
    if decimalPosition <= 0 then
        fraction = string.rep("0", -decimalPosition) .. digits
        integer = "0"
    elseif decimalPosition >= #digits then
        integer = digits .. string.rep("0", decimalPosition - #digits)
        fraction = ""
    else
        integer = digits:sub(1, decimalPosition)
        fraction = digits:sub(decimalPosition + 1)
    end

    integer = integer:gsub("^0+", "")
    if integer == "" then
        integer = "0"
    end
    fraction = fraction:gsub("0+$", "")

    local canonical = integer
    if fraction ~= "" then
        canonical = canonical .. "." .. fraction
    end
    if negative and canonical ~= "0" then
        canonical = "-" .. canonical
    end

    return canonical
end

local function canonical_version_text(version)
    if type(version) ~= "string" or not version:match("^%d+$") then
        return nil
    end
    if #version > 1 and version:sub(1, 1) == "0" then
        return nil
    end
    local maxInt64 = "9223372036854775807"
    if #version > #maxInt64 or (#version == #maxInt64 and version > maxInt64) then
        return nil
    end
    return version
end

local function increment_version(version)
    if not canonical_version_text(version) then
        return nil
    end
    return canonical_version_text(add_decimal(version, "1"))
end

-- Redis Lua numbers cannot represent every int64. Keep Version as validated
-- decimal text in memory, then splice it into the JSON object as a number.
local function encode_balance(balance)
    local version = canonical_version_text(balance.Version)
    if not version then
        return nil
    end

    local encodable = {}
    for key, value in pairs(balance) do
        if key ~= "Version" then
            encodable[key] = value
        end
    end
    local ok, encoded = pcall(cjson.encode, encodable)
    if not ok or encoded:sub(-1) ~= "}" then
        return nil
    end
    return encoded:sub(1, -2) .. ',"Version":' .. version .. "}"
end

local function encode_balance_array(balances)
    local encoded = {}
    for i, balance in ipairs(balances) do
        encoded[i] = encode_balance(balance)
        if not encoded[i] then
            return nil
        end
    end
    return "[" .. table.concat(encoded, ",") .. "]"
end

local function modern_flag(value)
    if value == 0 then
        return false
    end
    if value == 1 then
        return true
    end
    return nil
end

local function legacy_flag(value)
    if value == false then
        return 0
    end
    if value == true then
        return 1
    end
    return nil
end

-- normalize_modern_identity keeps the uppercase Alias/Key pair untouched for
-- legacy response correlation while making the lower-camel identity suitable
-- for strict new-schema readers.
local function normalize_modern_identity(alias, key)
    if type(alias) ~= "string" or type(key) ~= "string" then
        return nil, nil
    end

    if key == "" then
        key = "default"
    end

    local keyAlias, logicalKey = key:match("^([^#]+)#([^#]+)$")
    if keyAlias then
        if alias ~= keyAlias and alias ~= key then
            return nil, nil
        end
        return keyAlias, logicalKey
    end
    if key:find("#") then
        return nil, nil
    end

    local aliasBase, aliasKey = alias:match("^([^#]+)#([^#]+)$")
    if aliasBase then
        if aliasKey ~= key then
            return nil, nil
        end
        return aliasBase, key
    end
    if alias == "" or alias:find("#") then
        return nil, nil
    end

    return alias, key
end

-- project_dual_fields derives every lower-camel field from the authoritative
-- uppercase state. It never changes the uppercase financial representation.
local function project_dual_fields(balance, alias)
    local textFields = {
        "ID", "AccountID", "AccountType", "AssetCode", "Key", "Direction", "BalanceScope"
    }
    for _, name in ipairs(textFields) do
        if type(balance[name]) ~= "string" then
            return false
        end
    end

    local modernAlias, modernKey = normalize_modern_identity(alias, balance.Key)
    if not modernAlias then
        return false
    end

    local available = canonical_decimal_text(balance.Available)
    local onHold = canonical_decimal_text(balance.OnHold)
    local overdraftUsed = canonical_decimal_text(balance.OverdraftUsed)
    local overdraftLimit = canonical_decimal_text(balance.OverdraftLimit)
    local version = canonical_version_text(balance.Version)
    local allowSending = modern_flag(balance.AllowSending)
    local allowReceiving = modern_flag(balance.AllowReceiving)
    local blocked = modern_flag(balance.Blocked)
    local allowOverdraft = modern_flag(balance.AllowOverdraft)
    local overdraftLimitEnabled = modern_flag(balance.OverdraftLimitEnabled)
    if not available or not onHold or not overdraftUsed or not overdraftLimit or not version
        or allowSending == nil or allowReceiving == nil or blocked == nil or allowOverdraft == nil
        or overdraftLimitEnabled == nil then
        return false
    end

    balance.SchemaVersion = 2
    balance.id = balance.ID
    balance.accountId = balance.AccountID
    balance.accountType = balance.AccountType
    balance.assetCode = balance.AssetCode
    balance.alias = modernAlias
    balance.key = modernKey
    balance.direction = balance.Direction
    balance.balanceScope = balance.BalanceScope
    balance.available = available
    balance.onHold = onHold
    balance.overdraftUsed = overdraftUsed
    balance.overdraftLimit = overdraftLimit
    balance.version = version
    balance.allowSending = allowSending
    balance.allowReceiving = allowReceiving
    balance.blocked = blocked
    balance.allowOverdraft = allowOverdraft
    balance.overdraftLimitEnabled = overdraftLimitEnabled

    return true
end

local function apply_legacy_defaults(balance)
    if balance.Direction == nil then balance.Direction = "" end
    if balance.OverdraftUsed == nil then balance.OverdraftUsed = "0" end
    if balance.AllowOverdraft == nil then balance.AllowOverdraft = 0 end
    if balance.OverdraftLimitEnabled == nil then balance.OverdraftLimitEnabled = 0 end
    if balance.OverdraftLimit == nil then balance.OverdraftLimit = "0" end
    if balance.BalanceScope == nil then balance.BalanceScope = "transactional" end
    if balance.Blocked == nil then balance.Blocked = 0 end
end

local isCanonicalLimit

local function is_uuid_text(value)
    if type(value) ~= "string" then return false end
    local a, b, c, d, e = value:match("^(%x+)%-(%x+)%-(%x+)%-(%x+)%-(%x+)$")
    return a ~= nil and #a == 8 and #b == 4 and #c == 4 and #d == 4 and #e == 12
end

-- decode_cached_balance promotes strict lower-camel fields into the legacy
-- in-memory shape field by field. Presence of an uppercase field is
-- authoritative, even when its value is null or malformed; only absence may
-- select the lower field. This lets the financial path remain unchanged.
local function decode_cached_balance(cached, exactUpperVersion, exactSchemaVersion,
    duplicateLowerVersion, incoming, expectedAlias)
    local hadUpperID = cached.ID ~= nil
    local selectedLower = false
    local selectedLowerKey = false
    local aliasMissing = false

    if cached.SchemaVersion ~= nil and exactSchemaVersion ~= "2" then
        return nil
    end

    local function select_text(upper, lower, required, default)
        if cached[upper] ~= nil then return true end
        if cached[lower] == nil then
            if required then return false end
            cached[upper] = default
            return true
        end
        selectedLower = true
        if type(cached[lower]) ~= "string" or (required and cached[lower] == "") then return false end
        cached[upper] = cached[lower]
        return true
    end

    local function select_money(upper, lower, required, default, nonnegative)
        if cached[upper] ~= nil then return true end
        if cached[lower] == nil then
            if required then return false end
            cached[upper] = default
            return true
        end
        selectedLower = true
        if not isCanonicalLimit(cached[lower]) or (nonnegative and startsWithMinus(cached[lower])) then
            return false
        end
        cached[upper] = cached[lower]
        return true
    end

    local function select_flag(upper, lower, required, default)
        if cached[upper] ~= nil then return true end
        if cached[lower] == nil then
            if required then return false end
            cached[upper] = default
            return true
        end
        selectedLower = true
        local promoted = legacy_flag(cached[lower])
        if promoted == nil then return false end
        cached[upper] = promoted
        return true
    end

    if not select_text("ID", "id", true)
        or not select_text("AccountID", "accountId", true)
        or not select_money("Available", "available", true, nil, false)
        or not select_money("OnHold", "onHold", true, nil, true)
        or not select_money("OverdraftUsed", "overdraftUsed", false, "0", true)
        or not select_text("AccountType", "accountType", true)
        or not select_text("AssetCode", "assetCode", true)
        or not select_flag("AllowSending", "allowSending", true)
        or not select_flag("AllowReceiving", "allowReceiving", true)
        or not select_flag("Blocked", "blocked", false, 0)
        or not select_text("Direction", "direction", false, "")
        or not select_flag("AllowOverdraft", "allowOverdraft", false, 0)
        or not select_flag("OverdraftLimitEnabled", "overdraftLimitEnabled", false, 0)
        or not select_money("OverdraftLimit", "overdraftLimit", false, "0", true)
        or not select_text("BalanceScope", "balanceScope", false, "transactional") then
        return nil
    end

    if cached.Alias == nil then
        if cached.alias == nil then
            aliasMissing = true
        else
            selectedLower = true
            if type(cached.alias) ~= "string" or cached.alias == "" then return nil end
            cached.Alias = cached.alias
        end
    elseif type(cached.Alias) ~= "string" then
        return nil
    end
    if cached.Key == nil then
        if cached.key == nil then
            cached.Key = "default"
        else
            selectedLower = true
            selectedLowerKey = true
            if type(cached.key) ~= "string" or cached.key:find("#") then return nil end
            cached.Key = cached.key
        end
    end

    if cached.Version == nil then
        selectedLower = true
        if duplicateLowerVersion then return nil end
        cached.Version = canonical_version_text(cached.version)
        if not cached.Version then return nil end
    else
        cached.Version = exactUpperVersion
        if not cached.Version then return nil end
    end

    if selectedLower then
        if (not hadUpperID and exactSchemaVersion ~= "2")
            or (aliasMissing and not hadUpperID)
            or selectedLowerKey and cached.Key:find("#") then return nil end
        if type(cached.ID) ~= "string" or type(cached.AccountID) ~= "string"
            or not is_uuid_text(cached.ID) or not is_uuid_text(cached.AccountID) then
            return nil
        end
        cached.ID = string.lower(cached.ID)
        cached.AccountID = string.lower(cached.AccountID)
        if cached.ID ~= incoming.ID or cached.AccountID ~= incoming.AccountID
            or cached.AccountType ~= incoming.AccountType or cached.AssetCode ~= incoming.AssetCode then
            return nil
        end
        if cached.Direction ~= "" and cached.Direction ~= "credit" and cached.Direction ~= "debit" then
            return nil
        end
        if cached.BalanceScope ~= "transactional" and cached.BalanceScope ~= "internal" then
            return nil
        end

        local identityAlias = cached.Alias
        if aliasMissing and hadUpperID then
            identityAlias = expectedAlias
        end
        local modernAlias, modernKey = normalize_modern_identity(identityAlias, cached.Key)
        local expectedModernAlias, expectedModernKey = normalize_modern_identity(expectedAlias, incoming.Key)
        if not modernAlias or not expectedModernAlias or modernAlias ~= expectedModernAlias
            or modernKey ~= expectedModernKey then
            return nil
        end
        cached.Alias = expectedAlias
        cached.Key = incoming.Key
    else
        apply_legacy_defaults(cached)
    end

    return cached
end

local function balance_from_args(i)
    return {
        ID = ARGV[i + 7],
        Available = ARGV[i + 8],
        OnHold = ARGV[i + 9],
        Version = ARGV[i + 10],
        AccountType = ARGV[i + 11],
        AccountID = ARGV[i + 12],
        AssetCode = ARGV[i + 13],
        AllowSending = tonumber(ARGV[i + 14]),
        AllowReceiving = tonumber(ARGV[i + 15]),
        Key = ARGV[i + 16],
        Direction = ARGV[i + 17],
        OverdraftUsed = ARGV[i + 18],
        AllowOverdraft = tonumber(ARGV[i + 19]),
        OverdraftLimitEnabled = tonumber(ARGV[i + 20]),
        OverdraftLimit = ARGV[i + 21],
        BalanceScope = ARGV[i + 22],
        Blocked = tonumber(ARGV[i + 24]),
    }
end

local function updateTransactionHash(transactionBackupQueue, transactionKey, balances, balancesAfter)
    local transaction

    local raw = redis.call("HGET", transactionBackupQueue, transactionKey)
    if not raw then
        transaction = { balances = balances, balancesAfter = balancesAfter }
    else
        local ok, decoded = pcall(cjson.decode, raw)
        if ok and type(decoded) == "table" then
            transaction = decoded
            transaction.balances = balances
            transaction.balancesAfter = balancesAfter
        else
            transaction = { balances = balances, balancesAfter = balancesAfter }
        end
    end

    local encodedBalances = encode_balance_array(balances)
    local encodedBalancesAfter = encode_balance_array(balancesAfter)
    if not encodedBalances or not encodedBalancesAfter then
        return nil
    end

    local encodable = {}
    for key, value in pairs(transaction) do
        if key ~= "balances" and key ~= "balancesAfter" then
            encodable[key] = value
        end
    end
    local ok, encodedTransaction = pcall(cjson.encode, encodable)
    if not ok or encodedTransaction:sub(-1) ~= "}" then
        return nil
    end
    local updated = encodedTransaction:sub(1, -2)
    if updated ~= "{" then
        updated = updated .. ","
    end
    updated = updated .. '"balances":' .. encodedBalances
        .. ',"balancesAfter":' .. encodedBalancesAfter .. "}"
    redis.call("HSET", transactionBackupQueue, transactionKey, updated)

    return updated
end

-- finalizeSuccess stamps the idempotency marker of this execution and returns the
-- response verbatim. It is the ONLY exit that writes the marker, so an aborted
-- batch never leaves one behind: an aborted batch is rolled back, and a retry of
-- it must be free to reproduce the same rejection.
--
-- The marker holds the exact response string, which is what lets the replay gate
-- at the top of main() re-report a consumed execution without re-encoding it.
local function finalizeSuccess(applyMarkerKey, response, markerTTL)
    if applyMarkerKey ~= "" then
        redis.call("SET", applyMarkerKey, response, "EX", markerTTL)
    end

    return response
end

local function rollback(rollbackBalances, ttl)
  if next(rollbackBalances) then
      local msetArgs = {}
      for key, value in pairs(rollbackBalances) do
          table.insert(msetArgs, key)
          table.insert(msetArgs, value)
      end
      redis.call("MSET", unpack(msetArgs))

      for key, _ in pairs(rollbackBalances) do
          redis.call("EXPIRE", key, ttl)
      end
  end
end

isCanonicalLimit = function(value)
    if type(value) ~= "string" or value == "" then
        return false
    end

    local unsigned = value
    if string.sub(unsigned, 1, 1) == "-" then
        unsigned = string.sub(unsigned, 2)
        if unsigned == "0" then
            return false
        end
    end

    local integer, fraction = string.match(unsigned, "^(%d+)%.(%d+)$")
    if integer then
        if string.sub(fraction, -1) == "0" then
            return false
        end
    else
        integer = string.match(unsigned, "^(%d+)$")
    end

    return integer ~= nil and (#integer == 1 or string.sub(integer, 1, 1) ~= "0")
end

local function isJSONNumber(value)
    local mantissa = value
    if value:find("[eE]") then
        mantissa = value:match("^(.+)[eE][+-]?%d+$")
        if not mantissa then
            return false
        end
    end
    if mantissa:sub(1, 1) == "-" then
        mantissa = mantissa:sub(2)
    end
    local whole = mantissa:match("^(%d+)%.%d+$") or mantissa:match("^%d+$")
    return whole ~= nil and (#whole == 1 or whole:sub(1, 1) ~= "0")
end

-- cjson accepts non-JSON scalar syntax. Check tokens before any batch write,
-- including blobs whose limit already has a canonical representation.
local function hasValidJSONTokens(raw)
    local depth = 0
    local cursor = 1
    local sawLimit = false
    local sawVersion = false
    local sawSchemaVersion = false
    local sawLowerVersion = false
    local duplicateLowerVersion = false
    local exactVersion
    local exactSchemaVersion
    while cursor <= #raw do
        local char = raw:sub(cursor, cursor)
        if char == '"' then
            local tokenStart = cursor
            cursor = cursor + 1
            while cursor <= #raw do
                local tokenChar = raw:sub(cursor, cursor)
                if tokenChar:byte() < 32 then
                    return false
                elseif tokenChar == "\\" then
                    cursor = cursor + 2
                elseif tokenChar == '"' then
                    break
                else
                    cursor = cursor + 1
                end
            end
            if depth == 1 then
                local following = cursor + 1
                while raw:sub(following, following):match("%s") do
                    following = following + 1
                end
                if raw:sub(following, following) == ":" then
                    local field = cjson.decode(raw:sub(tokenStart, cursor))
                    if field == "OverdraftLimit" then
                        if sawLimit then
                            return false
                        end
                        sawLimit = true
                    elseif field == "Version" then
                        if sawVersion then
                            return false, nil, true
                        end
                        sawVersion = true
                        following = following + 1
                        while raw:sub(following, following):match("%s") do
                            following = following + 1
                        end
                        local valueEnd = following
                        while valueEnd <= #raw and not raw:sub(valueEnd, valueEnd):match("[%s,%]}]") do
                            valueEnd = valueEnd + 1
                        end
                        exactVersion = raw:sub(following, valueEnd - 1)
                        if not canonical_version_text(exactVersion) then
                            return false, nil, true
                        end
                    elseif field == "version" then
                        if sawLowerVersion then duplicateLowerVersion = true end
                        sawLowerVersion = true
                    elseif field == "SchemaVersion" then
                        if sawSchemaVersion then
                            exactSchemaVersion = "duplicate"
                        end
                        sawSchemaVersion = true
                        following = following + 1
                        while raw:sub(following, following):match("%s") do
                            following = following + 1
                        end
                        local valueEnd = following
                        while valueEnd <= #raw and not raw:sub(valueEnd, valueEnd):match("[%s,%]}]") do
                            valueEnd = valueEnd + 1
                        end
                        local exactValue = raw:sub(following, valueEnd - 1)
                        if exactSchemaVersion ~= "duplicate" then
                            exactSchemaVersion = exactValue
                        end
                    end
                end
            end
        elseif char == "{" or char == "[" then
            depth = depth + 1
        elseif char == "}" or char == "]" then
            depth = depth - 1
        elseif char:match("%s") then
            if char ~= " " and char ~= "\t" and char ~= "\r" and char ~= "\n" then
                return false
            end
        elseif char ~= ":" and char ~= "," then
            local tokenStart = cursor
            while cursor <= #raw and not raw:sub(cursor, cursor):match("[%s,%]}]") do
                cursor = cursor + 1
            end
            local token = raw:sub(tokenStart, cursor - 1)
            if token ~= "true" and token ~= "false" and token ~= "null" and not isJSONNumber(token) then
                return false
            end
            cursor = cursor - 1
        end
        cursor = cursor + 1
    end
    return true, exactVersion, false, exactSchemaVersion, duplicateLowerVersion
end

local function main()
    -- The command-layer delete marker deliberately lives longer than the shared
    -- balance snapshot lifetime, so a failed
    -- post-commit cache eviction cannot expose a deleted balance to a later
    -- atomic operation after the marker expires.
    local ttl = balance_cache_ttl_seconds

    -- Lifetime of the idempotency marker written by finalizeSuccess. It is the
    -- snapshot lifetime, not a copy of it: the marker has to outlive the
    -- asynchronous flush to PostgreSQL, because until that flush lands the
    -- database still reports the pre-execution state and a client retry would
    -- look legitimate. That window is exactly what `ttl` bounds, so the two move
    -- together by construction.
    local markerTTL = ttl

    local groupSize = 25
    local returnBalances = {}
    local returnBalancesAfter = {}
    local rollbackBalances = {}

    local transactionBackupQueue = KEYS[1]
    local transactionKey = KEYS[2]
    local scheduleKey = KEYS[3]

    -- KEYS[4] is the account-block exception key, present ONLY when the request
    -- body carried an accountBlockExceptionId. It shares the balance keys'
    -- {transactions} hash tag, which is what makes this multi-key EVAL legal in
    -- cluster mode and is why the grant can be validated and deleted in the same
    -- atomic step that mutates the balances.
    local exceptionKey = KEYS[4]

    -- ARGV carries a header ahead of the per-operation groups. Its first FOUR
    -- slots are always present, whether or not a grant was presented:
    --   ARGV[1] -> the source alias the caller's transaction debits, as Go
    --              resolved it from the batch for the presented grant ("" when none)
    --   ARGV[2] -> the amount that alias is debited by, canonical decimal string
    --   ARGV[3] -> how many bypassed balance keys follow ("0" when none)
    --   ARGV[4] -> the idempotency marker key of this execution, already
    --              tenant-namespaced by Go ("" only if a caller omits it)
    --   ARGV[5 .. 4+N] -> those N balance keys
    --
    -- The count makes the header self-describing, so the stride of every loop
    -- below is derived once here and no loop has to know whether a grant exists.
    -- This layout is a lock-step contract with luaArgsHeaderFixedSize in
    -- consumer.redis.go; the script ships embedded in the binary, so the two
    -- always travel together.
    local argvHeaderFixed = 4
    local expectedGrantAlias = ARGV[1] or ""
    local expectedGrantAmount = ARGV[2] or ""
    local grantedKeyCount = tonumber(ARGV[3]) or 0
    local applyMarkerKey = ARGV[4] or ""
    local argvHeader = argvHeaderFixed + grantedKeyCount

    -- Replay gate. This is the FIRST thing the script does, ahead of the grant
    -- validation and of the delete-marker and account-block pre-passes, because
    -- every one of those gates was already answered by the execution that wrote
    -- the marker: this one only re-reports a consumed result. Requiring a grant
    -- here would be actively wrong — the grant was consumed by that first
    -- execution, and the client resending the command (a go-redis retry after a
    -- read timeout, invisible to the calling Go code) presents no new one.
    --
    -- The stored payload is returned verbatim, with the flag spliced onto its
    -- front as raw string bytes. It is deliberately NOT decoded and re-encoded:
    -- a cjson round trip can reformat the decimal strings that carry financial
    -- values, so the replayed response would no longer be the response the first
    -- caller got.
    --
    -- An empty slot 4 is defensive only: it degrades to the pre-marker behavior
    -- (execute normally, write nothing) instead of aborting.
    if applyMarkerKey ~= "" then
        local storedResponse = redis.call("GET", applyMarkerKey)
        if storedResponse then
            return '{"replayed":true,' .. string.sub(storedResponse, 2)
        end
    end

    -- One logical debit can touch MORE THAN ONE balance of the granted account:
    -- when the debit overdraws, the system derives an overdraft companion leg on
    -- the same account. The companion's blob carries the same account-level
    -- Blocked flag, so a bypass naming only the primary would be rejected on the
    -- companion and a valid grant would be unusable on every overdrawing
    -- transaction. Go decides the exact set; this is a lookup over it.
    local grantedBalanceKeys = {}

    for i = 1, grantedKeyCount do
        grantedBalanceKeys[ARGV[argvHeaderFixed + i]] = true
    end

    -- Account-block exception: validate the presented grant against the
    -- transaction BEFORE the block guard, so the guard can honor it, and delete
    -- it only at the very END of a fully successful batch. Validating early and
    -- deleting late is what makes the grant survive an abort: a script that
    -- returns an error leaves its writes applied (Redis has no rollback), so a
    -- DEL placed up here would burn a single-use grant on a batch that moved no
    -- money. Single use is still guaranteed, because EVAL is atomic: two
    -- concurrent transactions presenting the same identifier cannot both see the
    -- key, and the one that reaches the DEL is the one that mutated balances.
    --
    -- A grant is consumed whenever it is presented and valid, even by a
    -- transaction that needed no bypass -- an identifier must not survive a
    -- request that presented it.
    --
    -- Expiry needs no comparison here: the key carries a native Redis TTL, so an
    -- expired grant is simply absent.
    local grantValidated = false

    if exceptionKey then
        if expectedGrantAlias == "" or expectedGrantAmount == "" or grantedKeyCount < 1 then
            return redis.error_reply("0508")
        end

        local raw = redis.call("GET", exceptionKey)
        if not raw then
            return redis.error_reply("0508")
        end

        local ok, decoded = pcall(cjson.decode, raw)
        if not ok or type(decoded) ~= "table" then
            return redis.error_reply("0508")
        end

        if type(decoded.Alias) ~= "string" or type(decoded.Amount) ~= "string" then
            return redis.error_reply("0508")
        end

        if decoded.Alias ~= expectedGrantAlias or
            cmp_decimal(decoded.Amount, expectedGrantAmount) ~= 0 then
            return redis.error_reply("0508")
        end

        grantValidated = true
    end

    -- Schedule balance sync immediately (eligible for worker pickup right away).
    -- The worker uses a dual-trigger (size OR timeout) to batch multiple keys
    -- before flushing to PostgreSQL, so immediate eligibility does not mean
    -- immediate DB write — the worker accumulates keys efficiently.
    --
    -- Uses fractional-second precision (seconds + microseconds / 1e6) to prevent
    -- the conditional ZREM from removing entries re-scheduled in the same second.
    -- Fractional seconds keep scores in the ~1e9 range (valid Unix timestamps),
    -- ensuring rollback compatibility with versions that interpret scores as seconds.
    local timeNow = redis.call("TIME")
    local dueAt = tonumber(timeNow[1]) + tonumber(timeNow[2]) / 1000000

    -- Delete marker guard: reject the whole batch before any mutation if any balance
    -- in it carries a live deletion marker. New writers use a dedicated top-level namespace,
    -- while the legacy :deleted marker remains honored during one rolling-deploy release.
    -- Both namespaces share the balance key's transaction hash slot. Running this pre-pass
    -- ahead of the first SET below means a rejection here leaves zero side
    -- effects across the batch, so no rollback is required. The stride mirrors
    -- the main loop below (groupSize=25; ARGV[i] is the balance key). A bounded
    -- per-key EXISTS check early-returns on the first delete marker found, so the
    -- whole batch is rejected without unpacking a client-influenced number of keys.
    local deleteMarkerNamespacePrefix = "balance_delete_marker:" .. transaction_hash_tag .. ":"
    local balanceNamespacePrefix = "balance:" .. transaction_hash_tag .. ":"
    for i = argvHeader + 1, #ARGV, groupSize do
        local markerKey, replacements = string.gsub(ARGV[i], balanceNamespacePrefix, deleteMarkerNamespacePrefix, 1)
        if replacements == 0 then
            markerKey = deleteMarkerNamespacePrefix .. ARGV[i]
        end

        -- Keep this legacy check until all old binaries are retired. New writers dual-write both
        -- markers so old Lua, which only checks this suffix, remains safe in the mixed fleet.
        if redis.call("EXISTS", markerKey) == 1 or redis.call("EXISTS", ARGV[i] .. balance_deletion_marker_suffix) == 1 then
            return redis.error_reply("0019")
        end
    end

    -- Account-block guard: reject the whole batch with 0502 before any mutation
    -- when any involved balance (source AND destination -- the block is
    -- bidirectional) belongs to a blocked account. The effective state mirrors
    -- the SET NX semantics of the main loop: an existing cached blob wins over
    -- the caller-supplied ARGV value (a legacy blob without the field counts as
    -- not blocked); an absent key falls back to ARGV[i+24], the value the Go
    -- hydration read from PostgreSQL. CANCELED batches skip the guard entirely
    -- (RF-4C: a cancel only returns on-hold funds or aborts a future credit,
    -- so blocking it would deadlock an innocent counterparty). A pending
    -- created before the block is rejected on commit because this guard runs
    -- on every execution. Like the delete-marker guard above, a rejection here
    -- leaves zero side effects, so no rollback is required.
    --
    -- A validated single-use grant bypasses the rejection for exactly the
    -- balances it authorizes (grantedBalanceKeys: the debited source balance plus
    -- the overdraft companions the system derived from that same debit). Every
    -- other balance in the batch still answers to the guard, so neither a blocked
    -- destination nor a sibling balance of the source is let through.
    for i = argvHeader + 1, #ARGV, groupSize do
        if ARGV[i + 2] ~= "CANCELED" then
            local blocked = tonumber(ARGV[i + 24]) or 0

            local cur = redis.call("GET", ARGV[i])
            if cur then
                local ok, decoded = pcall(cjson.decode, cur)
                if ok and type(decoded) == "table" then
                    if decoded.Blocked ~= nil then
                        blocked = tonumber(decoded.Blocked) or 0
                    elseif decoded.blocked == true then
                        blocked = 1
                    elseif decoded.blocked == false then
                        blocked = 0
                    end
                end
            end

            if blocked == 1 and not (grantValidated and grantedBalanceKeys[ARGV[i]]) then
                return redis.error_reply("0502")
            end
        end
    end

    -- Check the entire warm-cache batch before any SET NX or monetary write.
    -- Go canonicalizes decimals; Lua never converts monetary strings to numbers.
    local limitsToNormalize = {}
    local checkedKeys = {}
    local cachedVersions = {}
    local cachedSchemaVersions = {}
    local cachedDuplicateLowerVersions = {}
    local cachedRawBalances = {}
    for i = argvHeader + 1, #ARGV, groupSize do
        local key = ARGV[i]
        if not checkedKeys[key] then
            checkedKeys[key] = true
            local raw = redis.call("GET", key)
            if raw then
                cachedRawBalances[key] = raw
                local ok, cached = pcall(cjson.decode, raw)
                if not ok or type(cached) ~= "table" or not string.match(raw, "^%s*{") then
                    return redis.error_reply("BALANCE_LIMIT_INVALID")
                end
                local validTokens, exactVersion, invalidVersion, exactSchemaVersion, duplicateLowerVersion =
                    hasValidJSONTokens(raw)
                if invalidVersion then
                    return redis.error_reply("BALANCE_DUAL_PROJECTION_INVALID")
                end
                if not validTokens then
                    return redis.error_reply("BALANCE_LIMIT_INVALID")
                end
                cachedVersions[key] = exactVersion
                cachedSchemaVersions[key] = exactSchemaVersion
                cachedDuplicateLowerVersions[key] = duplicateLowerVersion
                if cached.OverdraftLimit ~= nil then
                    if type(cached.OverdraftLimit) ~= "string" then
                        return redis.error_reply("BALANCE_LIMIT_INVALID")
                    end
                    if not isCanonicalLimit(cached.OverdraftLimit) then
                        table.insert(limitsToNormalize, key)
                    end
                end
            end
        end
    end
    if #limitsToNormalize > 0 then
        return redis.error_reply("BALANCE_LIMIT_NORMALIZATION_REQUIRED:" .. cjson.encode(limitsToNormalize))
    end

    -- Validate every prospective dual projection before the first SET NX. A
    -- malformed later operation must not turn a serializer failure into a
    -- partially-seeded batch. Pure lower-camel entries are validated and
    -- promoted in memory; mixed entries retain legacy field authority.
    for i = argvHeader + 1, #ARGV, groupSize do
        if not canonical_version_text(ARGV[i + 10]) then
            return redis.error_reply("BALANCE_DUAL_PROJECTION_INVALID")
        end
        local candidate
        local raw = cachedRawBalances[ARGV[i]]
        if raw then
            local ok, decoded = pcall(cjson.decode, raw)
            if not ok or type(decoded) ~= "table" then
                return redis.error_reply("BALANCE_CACHE_SHAPE_UNSUPPORTED")
            end
            candidate = decode_cached_balance(decoded, cachedVersions[ARGV[i]],
                cachedSchemaVersions[ARGV[i]], cachedDuplicateLowerVersions[ARGV[i]],
                balance_from_args(i), ARGV[i + 5])
            if not candidate then
                return redis.error_reply("BALANCE_CACHE_SHAPE_UNSUPPORTED")
            end
        else
            candidate = balance_from_args(i)
        end

        candidate.Alias = ARGV[i + 5]
        if not project_dual_fields(candidate, candidate.Alias) then
            return redis.error_reply("BALANCE_DUAL_PROJECTION_INVALID")
        end
        if not encode_balance(candidate) then
            return redis.error_reply("BALANCE_DUAL_PROJECTION_INVALID")
        end
    end

    for i = argvHeader + 1, #ARGV, groupSize do
        local redisBalanceKey = ARGV[i]
        local isPending = tonumber(ARGV[i + 1])
        local transactionStatus = ARGV[i + 2]
        local operation = ARGV[i + 3]

        local amount = ARGV[i + 4]

        local alias = ARGV[i + 5]

        local routeValidationEnabled = tonumber(ARGV[i + 6])

        -- Balance object stored in Redis cache.
        --
        -- FIELD USAGE:
        -- Fields used by THIS Lua script for atomic operations:
        --   - ID:          Returned to Go for operation tracking
        --   - Available:   Balance calculations (DEBIT/CREDIT)
        --   - OnHold:      Balance calculations (ON_HOLD/RELEASE)
        --   - Version:     Optimistic concurrency control
        --   - AccountType: External-account carve-outs (skip overdraft/floor logic)
        --   - AccountID:   Returned to Go for operation tracking
        --   - Direction:   Direction-aware overdraft gating ("credit" vs "debit")
        --   - OverdraftUsed:        Tracked when a debit exceeds Available
        --   - AllowOverdraft:       Enables negative-result pass-through
        --   - OverdraftLimitEnabled:Gates the OverdraftLimit check
        --   - OverdraftLimit:       Hard cap on OverdraftUsed (when enabled)
        --   - BalanceScope:         "transactional" / "internal" (cache-only)
        --   - Blocked:              Account-block flag read by the pre-mutation guard above
        --
        -- Fields NOT used by Lua, but required in cache for Go pre-validation:
        --   - AssetCode:      Asset compatibility validation (0034)
        --   - AllowSending:   Sending permission validation (0024)
        --   - AllowReceiving: Receiving permission validation (0024)
        --   - Key:            Balance identification
        --
        -- WARNING: Do NOT remove the "cache-only" fields. They are essential for the
        -- transaction validation flow that reads balances from cache before calling Lua.
        -- The query cache-aside validation contract requires these fields.
        local balance = balance_from_args(i)

        -- Exact overdraft delta supplied by Go for pending-cancel reversals.
        -- Normal transaction paths pass zero and keep Lua's live-state split
        -- calculation authoritative.
        local overdraftAmount = ARGV[i + 23]

        -- Preserve the Go-provided version before the cache may overwrite it.
        -- Used for stale-version detection on overdraft-relevant operations.
        local incomingVersion = balance.Version

        -- Keep the failure image in the legacy shape. If a later operation
        -- rejects the batch, rollback must not leave a schema-only upgrade
        -- behind as a side effect of a failed financial operation.
        local rollbackBalance = encode_balance(balance)
        if not rollbackBalance then
            return redis.error_reply("BALANCE_DUAL_PROJECTION_INVALID")
        end

        balance.Alias = alias
        project_dual_fields(balance, alias)
        local redisBalance = encode_balance(balance)
        if not redisBalance then
            return redis.error_reply("BALANCE_DUAL_PROJECTION_INVALID")
        end
        local ok = redis.call("SET", redisBalanceKey, redisBalance, "EX", ttl, "NX")
        if not ok then
            local currentBalance = redis.call("GET", redisBalanceKey)
            if not currentBalance then
                return redis.error_reply("0139")
            end
            local validCurrent, currentVersion, invalidCurrentVersion, currentSchemaVersion,
                duplicateLowerVersion =
                hasValidJSONTokens(currentBalance)
            if not validCurrent or invalidCurrentVersion then
                rollback(rollbackBalances, ttl)
                return redis.error_reply("BALANCE_DUAL_PROJECTION_INVALID")
            end
            local decodeOK, decodedCurrent = pcall(cjson.decode, currentBalance)
            if not decodeOK or type(decodedCurrent) ~= "table" or
                not string.match(currentBalance, "^%s*{") then
                rollback(rollbackBalances, ttl)
                return redis.error_reply("BALANCE_DUAL_PROJECTION_INVALID")
            end
            balance = decodedCurrent
            rollbackBalance = currentBalance
            balance = decode_cached_balance(balance, currentVersion, currentSchemaVersion,
                duplicateLowerVersion, balance_from_args(i), alias)
            if not balance then
                rollback(rollbackBalances, ttl)
                return redis.error_reply("BALANCE_CACHE_SHAPE_UNSUPPORTED")
            end
        end

        -- Capture pre-operation OverdraftUsed so hasChange can detect repayment
        -- or accrual even when Available/OnHold remain unchanged.
        local originalOverdraftUsed = balance.OverdraftUsed

        if not rollbackBalances[redisBalanceKey] then
            rollbackBalances[redisBalanceKey] = rollbackBalance
        end

        local result = balance.Available
        local resultOnHold = balance.OnHold

        -- Direction-aware arithmetic on Available:
        -- For direction=debit balances (e.g., overdraft tracking), DEBIT
        -- increases Available and CREDIT decreases it. For direction=credit
        -- balances (and empty/legacy), DEBIT decreases and CREDIT increases.
        -- OnHold semantics are direction-agnostic: holds always add, releases
        -- always subtract, regardless of balance direction.
        local isDebitDirection = (balance.Direction == "debit")

        if isPending == 1 then
            if operation == "DEBIT" and transactionStatus == "PENDING" and routeValidationEnabled == 1 then
                -- Double-entry: DEBIT only updates Available.
                -- The OnHold++ will be a separate ON_HOLD operation.
                if isDebitDirection then
                    result = add_decimal(balance.Available, amount)
                else
                    result = sub_decimal(balance.Available, amount)
                end
            elseif operation == "DEBIT" and transactionStatus == "PENDING" and isDebitDirection then
                -- Pending overdraft companion. Holds no longer draw overdraft,
                -- so no new pending emits this leg; the branch is retained
                -- because a pending created under an earlier build may still
                -- replay through the backup consumer. It grows a
                -- direction=debit balance, so it never reaches the accrual
                -- block below.
                result = add_decimal(balance.Available, amount)
            elseif operation == "ON_HOLD" and transactionStatus == "PENDING" and routeValidationEnabled == 1 then
                -- Double-entry: ON_HOLD only increments OnHold.
                -- The Available-- was already done by the separate DEBIT operation.
                resultOnHold = add_decimal(balance.OnHold, amount)
            elseif operation == "ON_HOLD" and transactionStatus == "PENDING" then
                if isDebitDirection then
                    result = add_decimal(balance.Available, amount)
                else
                    result = sub_decimal(balance.Available, amount)
                end
                resultOnHold = add_decimal(balance.OnHold, amount)
            elseif operation == "RELEASE" and transactionStatus == "CANCELED" and routeValidationEnabled == 1 then
                -- Double-entry: RELEASE only decrements OnHold.
                -- The Available++ will be a separate CREDIT operation.
                resultOnHold = sub_decimal(balance.OnHold, amount)
            elseif operation == "RELEASE" and transactionStatus == "CANCELED" then
                resultOnHold = sub_decimal(balance.OnHold, amount)
                if isDebitDirection then
                    result = sub_decimal(balance.Available, amount)
                else
                    result = add_decimal(balance.Available, amount)
                end
            elseif operation == "CREDIT" and transactionStatus == "CANCELED" and routeValidationEnabled == 1 then
                -- Double-entry: CREDIT updates Available only.
                if isDebitDirection then
                    result = sub_decimal(balance.Available, amount)
                else
                    result = add_decimal(balance.Available, amount)
                end
            elseif operation == "CREDIT" and transactionStatus == "CANCELED" and isDebitDirection then
                -- Legacy pending overdraft companion cancel: shrink the
                -- direction=debit liability that was created by the pending
                -- companion DEBIT.
                result = sub_decimal(balance.Available, amount)
            elseif operation == "ON_HOLD" and transactionStatus == "APPROVED" and routeValidationEnabled == 1 then
                -- Double-entry: ON_HOLD in APPROVED only decrements OnHold.
                -- The Available++ will be a separate CREDIT operation.
                resultOnHold = sub_decimal(balance.OnHold, amount)
            elseif transactionStatus == "APPROVED" then
                if operation == "DEBIT" then
                    resultOnHold = sub_decimal(balance.OnHold, amount)
                else
                    if isDebitDirection then
                        result = sub_decimal(balance.Available, amount)
                    else
                        result = add_decimal(balance.Available, amount)
                    end
                end
            end
        else
            if isDebitDirection then
                if operation == "DEBIT" then
                    result = add_decimal(balance.Available, amount)
                else
                    result = sub_decimal(balance.Available, amount)
                end
            else
                if operation == "DEBIT" then
                    result = sub_decimal(balance.Available, amount)
                else
                    result = add_decimal(balance.Available, amount)
                end
            end
        end


        -- newOverdraftUsed holds the post-operation OverdraftUsed candidate.
        -- It is written back to `balance.OverdraftUsed` only AFTER the
        -- pre-mutation snapshot is cloned into `returnBalances`, so the
        -- "before" snapshot faithfully reflects the state the caller read.
        local newOverdraftUsed = balance.OverdraftUsed

        -- Overdraft repayment: a credit that arrives on a direction=credit
        -- balance with outstanding OverdraftUsed repays the overdraft first
        -- before growing Available. This mirrors the debit path in reverse:
        -- the deficit is paid down, and only the remainder increases the
        -- account holder's spendable balance.
        --
        -- The companion balance's Available is decremented in lock-step by a
        -- sibling CREDIT op queued by the Go enrichment layer
        -- (transaction_overdraft_enrichment.go:buildCompanionCreditOp). The
        -- stale-version check below keeps Go's repayAmount in sync with
        -- Lua's authoritative decrement — if they disagree (e.g. a
        -- concurrent transaction already reduced OverdraftUsed), the whole
        -- batch rolls back with 0174 so the caller re-reads state and
        -- retries with a consistent split.
        --
        -- DEFERRED destination legs are excluded. A two-phase transaction
        -- carries its destination CREDIT into every batch of the lifecycle, but
        -- the leg only POSTS on the commit. On the create and on the cancel the
        -- ladder above matches no branch, so `result` stays at the untouched
        -- Available; repaying from it would subtract from a balance that never
        -- received the credit, pushing `result` negative so the floor block
        -- below re-accrues the deficit on top of the outstanding OverdraftUsed
        -- and DOUBLES it on a leg that moved no money.
        --
        -- Enumerating the ladder, a CREDIT on a direction=credit balance posts
        -- to Available in exactly two cases: CANCELED with route validation ON
        -- (the source restore) and APPROVED (the commit). There is no
        -- CREDIT+PENDING branch at all, and the CANCELED+isDebitDirection branch
        -- cannot reach here because the repayment requires not isDebitDirection.
        -- So the deferred legs are PENDING, and CANCELED with route validation
        -- OFF — a shape only the destination leg can have, because in a cancel
        -- batch the source restore is the RELEASE branch in the legacy shape and
        -- the route-validated CREDIT in the other. The Go enrichment layer gates
        -- refund collection on the SAME rule so the two sides cannot drift.
        local isDeferredCreditLeg = isPending == 1 and
            (transactionStatus == "PENDING" or
                (transactionStatus == "CANCELED" and routeValidationEnabled == 0))

        if operation == "CREDIT" and not isDebitDirection and
            balance.AccountType ~= "external" and
            not isDeferredCreditLeg and
            isPositive(balance.OverdraftUsed) then
            local sameBatchCancelCredit = operation == "CREDIT" and transactionStatus == "CANCELED" and
                routeValidationEnabled == 1 and balance.Version == increment_version(incomingVersion)
            if balance.Version ~= incomingVersion and not sameBatchCancelCredit then
                rollback(rollbackBalances, ttl)
                return redis.error_reply("0174")
            end

            local repay = min_decimal(amount, balance.OverdraftUsed)
            if isPositive(overdraftAmount) then
                repay = min_decimal(min_decimal(overdraftAmount, amount), balance.OverdraftUsed)
            end

            newOverdraftUsed = sub_decimal(balance.OverdraftUsed, repay)
            -- `result` was already computed as balance.Available + amount
            -- by the direction-aware block above. Subtract the repayment
            -- portion to expose just the remainder that flows into
            -- Available; the repayment itself leaves Available untouched
            -- because it goes toward paying down OverdraftUsed.
            result = sub_decimal(result, repay)
        end

        -- Legacy pending cancel reversal: RELEASE both clears OnHold and
        -- restores Available. If the original hold consumed overdraft, only
        -- the non-overdraft portion should return to Available and the exact
        -- overdraft delta must be removed from OverdraftUsed.
        if operation == "RELEASE" and transactionStatus == "CANCELED" and routeValidationEnabled == 0 and
            not isDebitDirection and balance.AccountType ~= "external" and isPositive(overdraftAmount) then
            if balance.Version ~= incomingVersion then
                rollback(rollbackBalances, ttl)
                return redis.error_reply("0174")
            end

            local repay = min_decimal(min_decimal(overdraftAmount, amount), balance.OverdraftUsed)
            newOverdraftUsed = sub_decimal(balance.OverdraftUsed, repay)
            result = sub_decimal(result, repay)
        end

        -- A HOLD never draws overdraft. Debt is created only by CONCLUSIVE
        -- operations — the direct create, and the revert that is shaped like
        -- one — so a pending create that would overdraw falls through to the
        -- 0018 rejection below and moves nothing, on every route version.
        --
        -- Only the source hold can go negative in a PENDING batch: the legacy
        -- shape's single ON_HOLD and the route-validated shape's DEBIT leg both
        -- subtract from Available. Nothing else there reaches this block — the
        -- pending companion DEBIT lands on a direction=debit balance and grows
        -- it, and the deferred destination credit leaves Available untouched.
        --
        -- This gates the CREATE side only. A pending that already drew overdraft
        -- under an earlier build still has to be committed or canceled, so the
        -- unwind branches above (the pending companion, the RELEASE overdraft
        -- reversal, the same-batch cancel credit) are deliberately retained.
        local isHold = transactionStatus == "PENDING"

        if startsWithMinus(result) and balance.AccountType ~= "external" then
            -- Direction-aware overdraft: credit-direction balances with
            -- AllowOverdraft=1 may go temporarily negative. The shortfall
            -- is floored at zero in Available and accrued in OverdraftUsed,
            -- subject to OverdraftLimit when enabled.
            --
            -- Debit-direction balances, credit-direction balances without
            -- AllowOverdraft, and every hold fall through to the legacy 0018
            -- rejection.
            if balance.Direction == "credit" and (balance.AllowOverdraft or 0) == 1 and not isHold then
                -- deficit = abs(result). Because result is negative,
                -- sub_decimal("0", result) produces the absolute value.
                local deficit = sub_decimal("0", result)

                -- Stale-version check: if Go's pre-computed split assumed an
                -- older OverdraftUsed/Available, the floor math will be wrong.
                -- Reject so the caller can re-read and retry (Phase 1 behavior;
                -- retry is added in Phase 2).
                if balance.Version ~= incomingVersion then
                    rollback(rollbackBalances, ttl)
                    return redis.error_reply("0174")
                end

                -- Compute the candidate OverdraftUsed locally; the limit
                -- check below operates on this candidate so the "before"
                -- snapshot remains untouched.
                newOverdraftUsed = add_decimal(balance.OverdraftUsed, deficit)

                if (balance.OverdraftLimitEnabled or 0) == 1 then
                    -- sub_decimal(limit, newOverdraftUsed) is negative iff
                    -- the candidate strictly exceeds limit. Equal is allowed
                    -- (at-limit).
                    if startsWithMinus(sub_decimal(balance.OverdraftLimit, newOverdraftUsed)) then
                        rollback(rollbackBalances, ttl)
                        return redis.error_reply("0167")
                    end
                end

                result = "0"
            else
                rollback(rollbackBalances, ttl)
                return redis.error_reply("0018")
            end
        end

        -- Only update balance and increment version if there was an actual change.
        -- This prevents version gaps when destinations are processed during PENDING
        -- transactions (CREDIT + PENDING has no effect, so no version increment).
        --
        -- OverdraftUsed is included so pure overdraft accrual paths (Available
        -- floored at 0, OverdraftUsed incremented) still mark the balance as
        -- changed and trigger a cache + sync update. We compare against
        -- `newOverdraftUsed` (the candidate computed above) rather than the
        -- still-original `balance.OverdraftUsed` to detect overdraft deltas.
        local hasChange = (result ~= balance.Available)
            or (resultOnHold ~= balance.OnHold)
            or (newOverdraftUsed ~= originalOverdraftUsed)

        if hasChange then
            balance.Alias = alias
            if not project_dual_fields(balance, alias) then
                rollback(rollbackBalances, ttl)
                return redis.error_reply("BALANCE_DUAL_PROJECTION_INVALID")
            end
            -- Snapshot the pre-mutation state first so the "before" payload
            -- reflects what the caller read (especially OverdraftUsed).
            table.insert(returnBalances, cloneBalance(balance))

            balance.Available = result
            balance.OnHold = resultOnHold
            balance.OverdraftUsed = newOverdraftUsed
            local nextVersion = increment_version(balance.Version)
            if not nextVersion then
                rollback(rollbackBalances, ttl)
                return redis.error_reply("BALANCE_VERSION_OVERFLOW")
            end
            balance.Version = nextVersion

            if not project_dual_fields(balance, alias) then
                rollback(rollbackBalances, ttl)
                return redis.error_reply("BALANCE_DUAL_PROJECTION_INVALID")
            end

            table.insert(returnBalancesAfter, cloneBalance(balance))

            redisBalance = encode_balance(balance)
            if not redisBalance then
                rollback(rollbackBalances, ttl)
                return redis.error_reply("BALANCE_DUAL_PROJECTION_INVALID")
            end
            redis.call("SET", redisBalanceKey, redisBalance, "EX", ttl)

            redis.call("ZADD", scheduleKey, dueAt, redisBalanceKey)
        end
    end

    -- Consume the grant. Every abort above returns before this point, so an
    -- identifier is burned only by a batch that ran to completion -- and it is
    -- burned by every such batch, including one that needed no bypass.
    if grantValidated then
        redis.call("DEL", exceptionKey)
    end

    -- Handle empty array case: cjson encodes {} as object, but Go expects array
    -- When no changes occurred, use cjson.decode("[]") to get proper array type
    -- for both the transaction hash and the return value
    if #returnBalances == 0 then
        local emptyArray = cjson.decode("[]")
        if not updateTransactionHash(transactionBackupQueue, transactionKey, emptyArray, emptyArray) then
            rollback(rollbackBalances, ttl)
            return redis.error_reply("BALANCE_DUAL_PROJECTION_INVALID")
        end

        -- A batch that changed nothing is still a completed execution: marking it
        -- is what makes a resend of it a replay instead of a re-evaluation.
        return finalizeSuccess(applyMarkerKey,
            cjson.encode({ before = cjson.decode("[]"), after = cjson.decode("[]") }), markerTTL)
    end

    if not updateTransactionHash(transactionBackupQueue, transactionKey, returnBalances, returnBalancesAfter) then
        rollback(rollbackBalances, ttl)
        return redis.error_reply("BALANCE_DUAL_PROJECTION_INVALID")
    end

    local encodedBefore = encode_balance_array(returnBalances)
    local encodedAfter = encode_balance_array(returnBalancesAfter)
    if not encodedBefore or not encodedAfter then
        rollback(rollbackBalances, ttl)
        return redis.error_reply("BALANCE_DUAL_PROJECTION_INVALID")
    end

    return finalizeSuccess(applyMarkerKey,
        '{"before":' .. encodedBefore .. ',"after":' .. encodedAfter .. "}", markerTTL)
end

return main()
