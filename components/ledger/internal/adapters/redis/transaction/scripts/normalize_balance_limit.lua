-- Copyright (c) 2026 Lerian Studio. All rights reserved.
-- Use of this source code is governed by the Elastic License 2.0
-- that can be found in the LICENSE file.

local function invalid()
    return redis.error_reply("BALANCE_LIMIT_INVALID")
end

local function canonical(value)
    if type(value) ~= "string" or value == "" then
        return false
    end

    local unsigned = value
    if unsigned:sub(1, 1) == "-" then
        unsigned = unsigned:sub(2)
    end

    local whole, fraction = unsigned:match("^(%d+)%.(%d+)$")
    if not whole then
        whole = unsigned:match("^%d+$")
        if not whole then
            return false
        end
    end

    if #whole > 1 and whole:sub(1, 1) == "0" then
        return false
    end
    if fraction and fraction:sub(-1) == "0" then
        return false
    end

    return value ~= "-0"
end

local function json_number(value)
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

local raw = redis.call("GET", KEYS[1])
if not raw then
    return 0
end

local ok, balance = pcall(cjson.decode, raw)
if not ok or raw:match("^%s*(.)") ~= "{" or type(balance) ~= "table"
    or type(balance.OverdraftLimit) ~= "string" then
    return invalid()
end

if not canonical(ARGV[2]) then
    return invalid()
end

-- Locate the value token without re-encoding unrelated numeric or extension fields.
local depth = 0
local cursor = 1
local value_start, value_end
while cursor <= #raw do
    local char = raw:sub(cursor, cursor)
    if char == '"' then
        local token_start = cursor
        cursor = cursor + 1
        while cursor <= #raw do
            local token_char = raw:sub(cursor, cursor)
            if token_char:byte() < 32 then
                return invalid()
            elseif token_char == "\\" then
                cursor = cursor + 2
            elseif token_char == '"' then
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
            if raw:sub(following, following) == ":"
                and cjson.decode(raw:sub(token_start, cursor)) == "OverdraftLimit" then
                if value_start then
                    return invalid()
                end
                following = following + 1
                while raw:sub(following, following):match("%s") do
                    following = following + 1
                end
                if raw:sub(following, following) ~= '"' then
                    return invalid()
                end
                value_start = following
                following = following + 1
                while following <= #raw do
                    local value_char = raw:sub(following, following)
                    if value_char == "\\" then
                        following = following + 2
                    elseif value_char == '"' then
                        break
                    else
                        following = following + 1
                    end
                end
                value_end = following
            end
        end
    elseif char == "{" or char == "[" then
        depth = depth + 1
    elseif char == "}" or char == "]" then
        depth = depth - 1
    elseif char:match("%s") then
        if char ~= " " and char ~= "\t" and char ~= "\r" and char ~= "\n" then
            return invalid()
        end
    elseif char ~= ":" and char ~= "," then
        -- cjson accepts non-JSON numbers such as NaN and hexadecimal, so
        -- validate scalar tokens textually before preserving their bytes.
        local token_start = cursor
        while cursor <= #raw and not raw:sub(cursor, cursor):match("[%s,%]}]") do
            cursor = cursor + 1
        end
        local token = raw:sub(token_start, cursor - 1)
        if token ~= "true" and token ~= "false" and token ~= "null" and not json_number(token) then
            return invalid()
        end
        cursor = cursor - 1
    end
    cursor = cursor + 1
end

if not value_start then
    return invalid()
end

if balance.OverdraftLimit ~= ARGV[1] then
    return 2
end

local repaired = raw:sub(1, value_start - 1) .. cjson.encode(ARGV[2]) .. raw:sub(value_end + 1)
redis.call("SET", KEYS[1], repaired, "KEEPTTL")
return 1
