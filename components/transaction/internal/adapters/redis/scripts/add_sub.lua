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
    return s:sub(1, 1) == "-"
end

local function cloneBalance(tbl)
    local copy = {}
    for k, v in pairs(tbl) do
        copy[k] = v
    end
    return copy
end

local function updateTransactionHash(transactionBackupQueue, transactionKey, balances)
    local transaction

    local raw = redis.call("HGET", transactionBackupQueue, transactionKey)
    if not raw then
        transaction = { balances = balances }
    else
        local ok, decoded = pcall(cjson.decode, raw)
        if ok and type(decoded) == "table" then
            transaction = decoded
            transaction.balances = balances
        else
            transaction = { balances = balances }
        end
    end

    local updated = cjson.encode(transaction)
    redis.call("HSET", transactionBackupQueue, transactionKey, updated)

    return updated
end

local function main()
    local ttl = 3600
    local groupSize = 15
    local returnBalances = {}
    local operations = {}
    local balances = {}

    local transactionBackupQueue = KEYS[1]
    local transactionKey = KEYS[2]

    for i = 1, #ARGV, groupSize do
        local redisBalanceKey = ARGV[i]
        local isPending = tonumber(ARGV[i + 1])
        local transactionStatus = ARGV[i + 2]
        local operation = ARGV[i + 3]
        local amount = ARGV[i + 4]
        local alias = ARGV[i + 5]

        local balance = {
            ID = ARGV[i + 6],
            Available = ARGV[i + 7],
            OnHold = ARGV[i + 8],
            Version = tonumber(ARGV[i + 9]),
            AccountType = ARGV[i + 10],
            AllowSending = tonumber(ARGV[i + 11]),
            AllowReceiving = tonumber(ARGV[i + 12]),
            AssetCode = ARGV[i + 13],
            AccountID = ARGV[i + 14],
        }

        table.insert(operations, {
            redisBalanceKey = redisBalanceKey,
            isPending = isPending,
            transactionStatus = transactionStatus,
            operation = operation,
            amount = amount,
            alias = alias,
            balance = balance
        })

        if not balances[redisBalanceKey] then
            balances[redisBalanceKey] = balance
        end
    end

    for _, op in ipairs(operations) do
        local balance = balances[op.redisBalanceKey]

        local result = balance.Available
        local resultOnHold = balance.OnHold
        local isFrom = false

        if op.isPending == 1 then
            if op.operation == "ON_HOLD" and op.transactionStatus == "PENDING" then
                result = sub_decimal(balance.Available, op.amount)
                resultOnHold = add_decimal(balance.OnHold, op.amount)
                isFrom = true
            elseif op.operation == "RELEASE" and op.transactionStatus == "CANCELED" then
                resultOnHold = sub_decimal(balance.OnHold, op.amount)
                result = add_decimal(balance.Available, op.amount)
                isFrom = true
            elseif op.transactionStatus == "APPROVED" then
                if op.operation == "DEBIT" then
                    resultOnHold = sub_decimal(balance.OnHold, op.amount)
                    isFrom = true
                else
                    result = add_decimal(balance.Available, op.amount)
                end
            end
        else
            if op.operation == "DEBIT" then
                result = sub_decimal(balance.Available, op.amount)
            else
                result = add_decimal(balance.Available, op.amount)
            end
        end

        if op.isPending == 1 and isFrom and balance.AccountType == "external" then
            return redis.error_reply("0098")
        end

        if startsWithMinus(result) and balance.AccountType ~= "external" then
            return redis.error_reply("0018")
        end

        local returnBalance = cloneBalance(balance)
        returnBalance.Alias = op.alias
        table.insert(returnBalances, returnBalance)

        balance.Available = result
        balance.OnHold = resultOnHold
        balance.Version = balance.Version + 1
    end

    local pendingUpdates = {}
    for balanceKey, balance in pairs(balances) do
        local redisBalance = cjson.encode(balance)
        table.insert(pendingUpdates, balanceKey)
        table.insert(pendingUpdates, redisBalance)
    end

    if #pendingUpdates > 0 then
        redis.call("MSET", unpack(pendingUpdates))

        for j = 1, #pendingUpdates, 2 do
            local balanceKey = pendingUpdates[j]
            redis.call("EXPIRE", balanceKey, ttl)
        end
    end

    updateTransactionHash(transactionBackupQueue, transactionKey, returnBalances)

    local returnBalancesEncoded = cjson.encode(returnBalances)
    return returnBalancesEncoded
end

return main()