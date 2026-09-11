-- isObject identifies ordinary JSON-object tables while excluding arrays,
-- preserved number tokens, and the private null sentinel.
local function isObject(value)
    return type(value) == "table" and not arrayKinds[value] and not numberTokens[value] and value ~= nullValue
end

-- requireObject enforces an object-shaped protocol value.
local function requireObject(value)
    if not isObject(value) then technical("invalid_protocol", "expected JSON object") end
end

-- requireArray enforces an explicitly tagged JSON array, including empty arrays.
local function requireArray(value)
    if type(value) ~= "table" or not arrayKinds[value] then technical("invalid_protocol", "expected JSON array") end
end

-- text validates a UTF-8 string and optionally permits the empty value.
local function text(value, allowEmpty)
    if type(value) ~= "string" or (not allowEmpty and value == "") or not validUTF8(value) then
        technical("invalid_protocol", "invalid text field")
    end
    return value
end

-- uuid validates the canonical lowercase, non-nil UUID representation used by
-- every persisted and correlated engine identity.
local function uuid(value)
    text(value, false)
    if #value ~= 36 or not value:match("^%x%x%x%x%x%x%x%x%-%x%x%x%x%-%x%x%x%x%-%x%x%x%x%-%x%x%x%x%x%x%x%x%x%x%x%x$") or value == "00000000-0000-0000-0000-000000000000" or value ~= value:lower() then
        technical("invalid_protocol", "invalid UUID")
    end
    return value
end

-- integerText validates a canonical, nonnegative decimal integer string without
-- converting it to a Lua number, and bounds it by a same-format maximum.
local function integerText(value, maxValue)
    if type(value) ~= "string" or not value:match("^%d+$") or (#value > 1 and value:sub(1, 1) == "0") or #value > #maxValue or (#value == #maxValue and value > maxValue) then
        technical("invalid_protocol", "invalid integer text")
    end
    return value
end

-- smallInteger converts a preserved JSON integer token only after proving that
-- it fits within the small trusted range needed for indices and configuration.
local function smallInteger(value, maximum)
    local token = type(value) == "table" and numberTokens[value]
    if not token then technical("invalid_protocol", "expected integer JSON token") end
    integerText(token, tostring(maximum))
    return tonumber(token)
end

-- bool enforces a native JSON boolean rather than accepting truthy values.
local function bool(value)
    if type(value) ~= "boolean" then technical("invalid_protocol", "invalid boolean") end
    return value
end

-- clone makes the shallow working copies used for immutable snapshots and
-- prepared mutations. Nested structures are cloned explicitly by their callers.
local function clone(value)
    local result = {}
    for key, item in pairs(value) do result[key] = item end
    return result
end

-- logicalRef validates the alias#key identity used inside the wire protocol and
-- rejects physical Redis namespaces, hash-tag characters, and control bytes.
local function logicalRef(value)
    text(value, false)
    if not value:match("^[^#]+#[^#]+$") or value:find("[{}%c]") or value:match("^balance:") or value:match("^tenant:") then
        technical("invalid_protocol", "invalid logical balance reference")
    end
    return value
end

-- money validates and canonicalizes a decimal string without changing its exact
-- value. It removes redundant zeros and normalizes every representation of zero.
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
    -- Remove insignificant zeros while retaining one integer digit and all
    -- significant fractional digits.
    integer = integer:gsub("^0+", "")
    if integer == "" then integer = "0" end
    fraction = fraction:gsub("0+$", "")
    local result = integer
    if fraction ~= "" then result = result .. "." .. fraction end
    if negative and result ~= "0" then result = "-" .. result end
    return result
end

-- canonicalMoney requires wire values to already match the engine's unique
-- decimal representation; normalization at this boundary would hide ambiguity.
local function canonicalMoney(value)
    local normalized = money(value)
    if normalized ~= value then technical("invalid_protocol", "noncanonical decimal in wire") end
    return normalized
end

-- nonnegative canonicalizes a balance value and rejects negative accounting
-- state while still accepting signed input only when it normalizes to zero.
local function nonnegative(value)
    value = money(value)
    if value:sub(1, 1) == "-" then technical("invalid_balance", "negative accounting state") end
    return value
end
