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

local function string_end(raw, start_at)
    local cursor = start_at + 1
    while cursor <= #raw do
        local char = raw:sub(cursor, cursor)
        if char:byte() < 32 then
            return nil
        elseif char == "\\" then
            cursor = cursor + 2
        elseif char == '"' then
            return cursor
        else
            cursor = cursor + 1
        end
    end

    return nil
end

local function value_end(raw, start_at)
    local first = raw:sub(start_at, start_at)
    if first == '"' then
        return string_end(raw, start_at)
    end

    if first == "{" or first == "[" then
        local depth = 0
        local cursor = start_at
        while cursor <= #raw do
            local char = raw:sub(cursor, cursor)
            if char == '"' then
                cursor = string_end(raw, cursor)
                if not cursor then
                    return nil
                end
            elseif char == "{" or char == "[" then
                depth = depth + 1
            elseif char == "}" or char == "]" then
                depth = depth - 1
                if depth == 0 then
                    return cursor
                end
            end
            cursor = cursor + 1
        end

        return nil
    end

    local cursor = start_at
    while cursor <= #raw do
        local char = raw:sub(cursor, cursor)
        if char == "," or char == "}" or char == "]" or char:match("%s") then
            return cursor - 1
        end
        cursor = cursor + 1
    end

    return #raw
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
local upper_start, upper_end
local lower_start, lower_end
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
            local name = cjson.decode(raw:sub(token_start, cursor))
            if raw:sub(following, following) == ":"
                and (name == "OverdraftLimit" or name == "overdraftLimit") then
                if name == "OverdraftLimit" and upper_start
                    or name == "overdraftLimit" and lower_start then
                    return invalid()
                end
                following = following + 1
                while raw:sub(following, following):match("%s") do
                    following = following + 1
                end

                local ending = value_end(raw, following)
                if not ending or name == "OverdraftLimit" and raw:sub(following, following) ~= '"' then
                    return invalid()
                end

                if name == "OverdraftLimit" then
                    upper_start, upper_end = following, ending
                else
                    lower_start, lower_end = following, ending
                end
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

if not upper_start then
    return invalid()
end

if balance.OverdraftLimit ~= ARGV[1] then
    return 2
end

local replacement = cjson.encode(ARGV[2])
local repaired = raw
local function replace_value(value, start_at, end_at)
    return value:sub(1, start_at - 1) .. replacement .. value:sub(end_at + 1)
end

-- Replace the later token first so the earlier token's offsets remain valid.
if lower_start and lower_start > upper_start then
    repaired = replace_value(repaired, lower_start, lower_end)
    repaired = replace_value(repaired, upper_start, upper_end)
elseif lower_start then
    repaired = replace_value(repaired, upper_start, upper_end)
    repaired = replace_value(repaired, lower_start, lower_end)
else
    repaired = replace_value(repaired, upper_start, upper_end)
end

redis.call("SET", KEYS[1], repaired, "KEEPTTL")
return 1
