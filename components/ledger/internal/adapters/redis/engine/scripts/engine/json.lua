-- ARGV: wire JSON, maximum request bytes, maximum total prepared bytes.
-- Physical Redis keys are supplied only through KEYS. Financial decimals and
-- versions remain text; numeric JSON tokens are preserved without a double round-trip.

local arrayKinds = {}
local numberTokens = {}
local nullValue = {}
local commitStarted = false
local MAX_JSON_DEPTH = 128

-- object creates a table that is intentionally encoded as a JSON object.
local function object() return {} end

-- array creates a table and records its JSON array identity. Lua tables do not
-- otherwise distinguish an empty object from an empty array.
local function array()
    local value = {}
    arrayKinds[value] = true
    return value
end

-- numberToken wraps raw JSON numeric text so large integers and exact decimals
-- survive decoding and encoding without an IEEE-754 conversion.
local function numberToken(value)
    local token = {}
    numberTokens[token] = value
    return token
end

-- technical aborts before commit with a closed engine error that represents an
-- invalid protocol, corrupt state, or another non-business failure.
local function technical(code, message)
    error({ kind = "technical", code = code, message = message }, 0)
end

-- refuse aborts before commit with a deterministic business refusal correlated
-- to the transaction, posting, and balance that caused it.
local function refuse(code, transactionIndex, postingIndex, balanceRef)
    error({
        kind = "failure", code = code, transactionIndex = transactionIndex,
        postingIndex = postingIndex, balanceRef = balanceRef or ""
    }, 0)
end

-- validUTF8 rejects malformed UTF-8, overlong sequences, surrogate code points,
-- and values outside the Unicode range before text enters the engine protocol.
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

-- decodeJSON parses the wire payload while preserving every JSON number as raw
-- text. The dedicated parser also rejects duplicate keys and excessive nesting,
-- properties that the generic Redis cjson decoder cannot safely guarantee here.
local function decodeJSON(raw)
    if type(raw) ~= "string" then technical("invalid_json", "JSON must be text") end
    local position = 1
    local parseValue
    -- skipWhitespace advances the shared cursor across JSON whitespace only.
    local function skipWhitespace()
        while position <= #raw and raw:sub(position, position):match("[ \t\r\n]") do position = position + 1 end
    end
    -- parseString finds one complete JSON string, delegates escape decoding to
    -- cjson, and validates the decoded UTF-8 before returning it.
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
    -- parseNumber validates JSON number grammar and returns the original token
    -- instead of coercing it to a Lua number.
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
    -- parseValue recursively decodes one JSON value from the current cursor.
    -- The depth argument provides a hard bound against pathological nesting.
    parseValue = function(depth)
        if depth > MAX_JSON_DEPTH then technical("invalid_json", "JSON nesting limit exceeded") end
        skipWhitespace()
        local char = raw:sub(position, position)
        if char == '"' then return parseString() end
        -- Objects reject duplicate keys so later fields cannot silently replace
        -- identities, amounts, or execution controls validated earlier.
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
        -- Arrays retain explicit type metadata even when they contain no items.
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
        -- Match the three nonnumeric JSON literals before attempting a number.
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

-- encodeJSON serializes engine values deterministically. Object keys are sorted,
-- number tokens retain their original text, and unsafe native numbers are
-- rejected so receipts and comparisons remain stable across executions.
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
    -- Stable object ordering makes receipt bytes and structural comparisons
    -- deterministic even though Lua table iteration order is unspecified.
    local keys = {}
    for key, _ in pairs(value) do
        if type(key) ~= "string" then technical("serialization_failed", "invalid JSON object key") end
        keys[#keys + 1] = key
    end
    table.sort(keys)
    for _, key in ipairs(keys) do parts[#parts + 1] = cjson.encode(key) .. ":" .. encodeJSON(value[key], depth + 1) end
    return "{" .. table.concat(parts, ",") .. "}"
end
