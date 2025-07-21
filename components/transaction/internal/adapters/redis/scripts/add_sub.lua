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

local sub_decimal

local function add_decimal(a, b)
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

    local a_num = tonumber(a)
    local b_num = tonumber(b)
    if a_num < b_num then
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
    return s:sub(1,1) == "-"
end

local function main()
    local ttl = 3600
    local key = KEYS[1]

    local isPending = tonumber(ARGV[1])
    local transactionStatus = ARGV[2]
    local operation = ARGV[3]

    local amount = ARGV[4]

    local balance = {
        ID = ARGV[5],
        Available = ARGV[6],
        OnHold = ARGV[7],
        Version = tonumber(ARGV[8]),
        AccountType = ARGV[9],
        AllowSending = tonumber(ARGV[10]),
        AllowReceiving = tonumber(ARGV[11]),
        AssetCode = ARGV[12],
        AccountID = ARGV[13],
    }

    local newBalanceEncoded = cjson.encode(balance)
    local ok = redis.call("SET", key, newBalanceEncoded, "EX", ttl, "NX")
    if not ok then
        local current = redis.call("GET", key)
        if not current then
            return redis.error_reply("0061")
        end

        balance = cjson.decode(current)
    end

    local result = balance.Available
    local resultOnHold = balance.OnHold
    local isFrom = false

    if isPending == 1 then
        if operation == "ON_HOLD" and transactionStatus == "PENDING" then
            result = sub_decimal(balance.Available, amount)
            resultOnHold = add_decimal(balance.OnHold, amount)
            isFrom = true
        elseif operation == "RELEASE" and transactionStatus == "CANCELED" then
            resultOnHold = sub_decimal(balance.OnHold, amount)
            result = add_decimal(balance.Available, amount)
            isFrom = true
        elseif transactionStatus == "APPROVED" then
            if operation == "DEBIT" then
                resultOnHold = sub_decimal(balance.OnHold, amount)
                isFrom = true
            else
                result = add_decimal(balance.Available, amount)
            end
        end
    else
        if operation == "DEBIT" then
            result = sub_decimal(balance.Available, amount)
        else
            result = add_decimal(balance.Available, amount)
        end
    end

    if isPending == 1 and isFrom and balance.AccountType == "external" then
         return redis.error_reply("0098")
    end

    if startsWithMinus(result) and balance.AccountType ~= "external" then
        return redis.error_reply("0018")
    end

    local balanceEncoded = cjson.encode(balance)

    balance.Available = result
    balance.OnHold = resultOnHold
    balance.Version = balance.Version + 1

    local finalBalanceEncoded = cjson.encode(balance)
    redis.call("SET", key, finalBalanceEncoded, "EX", ttl)

    return balanceEncoded
end

return main()