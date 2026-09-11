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
