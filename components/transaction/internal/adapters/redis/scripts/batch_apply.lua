-- Multi-key atomic balance apply script for financial transactions
-- Applies a batch of operations across multiple balance keys atomically.
-- Validates all operations first; if any fails, aborts without changes.
--
-- IMPORTANT: This script uses descriptive variable names for maintainability.
-- All decimal arithmetic is string-based to avoid IEEE-754 floating-point errors.
-- The script ensures financial data integrity through two-phase processing.
--
-- Contract
--   KEYS:   list of balance keys (one per operation)
--   ARGV:
--     [1] isPending           : 1 or 0
--     [2] transactionStatus   : "PENDING" | "APPROVED" | "CANCELED" | "CREATED"
--     [3] enforceOCC          : 1 to enable version check, else 0
--     For each i in [1..#KEYS], stride=11 values:
--       [i,1]  operation      : "DEBIT" | "CREDIT" | "ON_HOLD" | "RELEASE"
--       [i,2]  amount         : non-negative decimal string
--       [i,3]  id             : balance id
--       [i,4]  availableSeed  : available used if key does not yet exist
--       [i,5]  onHoldSeed     : onHold used if key does not yet exist
--       [i,6]  versionSeed    : version used if key does not yet exist
--       [i,7]  accountType    : "external" or other
--       [i,8]  allowSending   : 1 or 0
--       [i,9]  allowReceiving : 1 or 0
--       [i,10] assetCode      : asset code string
--       [i,11] accountId      : account id string
--
-- Phases
--   1) Validation: for each key, read current (if present), normalize, compute result, and check:
--        - policy: allowSending/allowReceiving, external on-hold-from
--        - invariants: available>=0 (non-external), onHold>=0
--        - inputs: amount shape, op/status combos
--        - OCC: optional version match
--      No writes occur in this phase.
--   2) Commit: apply all writes (SET with TTL preservation) if and only if validation passes for all.
--
-- Error codes  (consistent with single-key script):
--   0017: invalid script format/arguments
--   0018: insufficient funds (available<0 or onHold<0)
--   0019: account ineligibility (sending/receiving not allowed)
--   0086: version mismatch (OCC) when enforceOCC=1
--   0098: external account pending 'from' flow not allowed

-- Decimal utilities (string-based math, no floats)
-- Implement exact decimal math to avoid float rounding and support large numbers.
local function trim_leading_zeros_from_integer(integer_string)
    integer_string = integer_string:gsub("^0+", "")
    if integer_string == "" then return "0" end
    return integer_string
end

local function trim_trailing_zeros_from_fraction(fraction_string)
    fraction_string = fraction_string:gsub("0+$", "")
    return (fraction_string == "" and "0") or fraction_string
end

-- Parse decimal string, returning (isNegative, integerPart, fractionalPart)
-- Accepts optional sign and fractional part; rejects malformed forms
local function normalize_decimal(decimal_string)
    if decimal_string == nil then decimal_string = "0" end
    if type(decimal_string) ~= "string" then decimal_string = tostring(decimal_string) end
    decimal_string = decimal_string:gsub("^%s+", ""):gsub("%s+$", "")
    if decimal_string == "" or decimal_string == "." or decimal_string == "+." or decimal_string == "-." then
        return false, "0", "0"
    end
    local is_negative = false
    local first_character = decimal_string:sub(1,1)
    if first_character == "-" then
        is_negative = true
        decimal_string = decimal_string:sub(2)
    elseif first_character == "+" then
        decimal_string = decimal_string:sub(2)
    end

    -- Split optional fractional portion
    local integer_part, fractional_part = decimal_string:match("^(%d*)%.(%d*)$")
    if integer_part then
        if integer_part == "" then integer_part = "0" end
        if fractional_part == "" then fractional_part = "0" end
    else
        if decimal_string:match("^%d+$") then
            integer_part = decimal_string
            fractional_part = "0"
        else
            return nil -- invalid input
        end
    end
    integer_part = trim_leading_zeros_from_integer(integer_part)
    return is_negative, integer_part, fractional_part
end

-- Compare magnitudes of two non-negative decimals: returns -1/0/1
local function compare_magnitude(first_integer, first_fraction, second_integer, second_fraction)
    if #first_integer ~= #second_integer then
        return (#first_integer > #second_integer) and 1 or -1
    end
    if first_integer ~= second_integer then
        return (first_integer > second_integer) and 1 or -1
    end
    local max_fraction_length = math.max(#first_fraction, #second_fraction)
    if #first_fraction < max_fraction_length then 
        first_fraction = first_fraction .. string.rep("0", max_fraction_length - #first_fraction) 
    end
    if #second_fraction < max_fraction_length then 
        second_fraction = second_fraction .. string.rep("0", max_fraction_length - #second_fraction) 
    end
    if first_fraction ~= second_fraction then
        return (first_fraction > second_fraction) and 1 or -1
    end
    return 0
end

-- Compare signed decimals: returns -1 if first<second, 0 if equal, 1 if first>second
local function compare_decimal(first_decimal, second_decimal)
    local first_is_negative, first_integer, first_fraction = normalize_decimal(first_decimal)
    local second_is_negative, second_integer, second_fraction = normalize_decimal(second_decimal)
    if first_is_negative == nil or second_is_negative == nil then return nil end
    if first_is_negative ~= second_is_negative then
        return first_is_negative and -1 or 1
    end
    local magnitude_comparison = compare_magnitude(first_integer, first_fraction, second_integer, second_fraction)
    return first_is_negative and -magnitude_comparison or magnitude_comparison
end

local subtract_decimal

-- Add two decimal strings; returns canonical decimal string or redis.error_reply
local function add_decimal(first_decimal, second_decimal)
    local first_is_negative, first_integer, first_fraction = normalize_decimal(first_decimal)
    local second_is_negative, second_integer, second_fraction = normalize_decimal(second_decimal)
    if first_is_negative == nil or second_is_negative == nil then return redis.error_reply("0017") end

    if first_is_negative and second_is_negative then
        local sum_result = add_decimal(first_decimal:sub(2), second_decimal:sub(2))
        if type(sum_result) ~= "string" then return sum_result end
        return "-" .. sum_result
    end
    if first_is_negative then return subtract_decimal(second_decimal, first_decimal:sub(2)) end
    if second_is_negative then return subtract_decimal(first_decimal, second_decimal:sub(2)) end

    -- Align fraction lengths by padding with zeros
    if #first_fraction < #second_fraction then
        first_fraction = first_fraction .. string.rep("0", #second_fraction - #first_fraction)
    elseif #second_fraction < #first_fraction then
        second_fraction = second_fraction .. string.rep("0", #first_fraction - #second_fraction)
    end

    local carry_digit = 0 -- carry across digits
    local fraction_sum_digits = {}
    for digit_position = #first_fraction, 1, -1 do
        local first_digit = tonumber(first_fraction:sub(digit_position, digit_position)) or 0
        local second_digit = tonumber(second_fraction:sub(digit_position, digit_position)) or 0
        local digit_sum = first_digit + second_digit + carry_digit
        carry_digit = math.floor(digit_sum / 10)
        fraction_sum_digits[#first_fraction - digit_position + 1] = tostring(digit_sum % 10)
    end

    local reversed_first_integer = first_integer:reverse()
    local reversed_second_integer = second_integer:reverse()
    local max_integer_length = math.max(#reversed_first_integer, #reversed_second_integer)
    local integer_sum_digits = {}
    for digit_position = 1, max_integer_length do
        local first_digit = tonumber(reversed_first_integer:sub(digit_position, digit_position)) or 0
        local second_digit = tonumber(reversed_second_integer:sub(digit_position, digit_position)) or 0
        local digit_sum = first_digit + second_digit + carry_digit
        carry_digit = math.floor(digit_sum / 10)
        integer_sum_digits[digit_position] = tostring(digit_sum % 10)
    end
    if carry_digit > 0 then
        integer_sum_digits[#integer_sum_digits + 1] = tostring(carry_digit)
    end

    local integer_result = table.concat(integer_sum_digits):reverse()
    local fraction_result = table.concat(fraction_sum_digits):reverse()
    fraction_result = trim_trailing_zeros_from_fraction(fraction_result)

    if fraction_result == "0" then
        return integer_result
    end
    return integer_result .. "." .. fraction_result
end

-- Subtract two decimal strings: first - second; may return negative string
subtract_decimal = function(first_decimal, second_decimal)
    local first_is_negative, first_integer, first_fraction = normalize_decimal(first_decimal)
    local second_is_negative, second_integer, second_fraction = normalize_decimal(second_decimal)
    if first_is_negative == nil or second_is_negative == nil then return redis.error_reply("0017") end

    if first_is_negative and second_is_negative then
        return subtract_decimal(second_decimal:sub(2), first_decimal:sub(2))
    end
    if first_is_negative then
        local sum_result = add_decimal(first_decimal:sub(2), second_decimal)
        if type(sum_result) ~= "string" then return sum_result end
        return "-" .. sum_result
    end
    if second_is_negative then
        return add_decimal(first_decimal, second_decimal:sub(2))
    end

    local comparison_result = compare_decimal(first_decimal, second_decimal)
    if comparison_result == nil then return redis.error_reply("0017") end
    if comparison_result < 0 then
        local difference_result = subtract_decimal(second_decimal, first_decimal)
        if type(difference_result) ~= "string" then return difference_result end
        return "-" .. difference_result
    end

    -- Align fraction lengths by padding with zeros
    if #first_fraction < #second_fraction then
        first_fraction = first_fraction .. string.rep("0", #second_fraction - #first_fraction)
    elseif #second_fraction < #first_fraction then
        second_fraction = second_fraction .. string.rep("0", #first_fraction - #second_fraction)
    end

    local borrow_digit = 0 -- borrow across digits
    local fraction_difference_digits = {}
    for digit_position = #first_fraction, 1, -1 do
        local first_digit = tonumber(first_fraction:sub(digit_position, digit_position)) or 0
        local second_digit = tonumber(second_fraction:sub(digit_position, digit_position)) or 0
        local digit_difference = first_digit - second_digit - borrow_digit
        if digit_difference < 0 then
            digit_difference = digit_difference + 10
            borrow_digit = 1
        else
            borrow_digit = 0
        end
        fraction_difference_digits[#first_fraction - digit_position + 1] = tostring(digit_difference)
    end

    local reversed_first_integer = first_integer:reverse()
    local reversed_second_integer = second_integer:reverse()
    local max_integer_length = math.max(#reversed_first_integer, #reversed_second_integer)
    local integer_difference_digits = {}
    for digit_position = 1, max_integer_length do
        local first_digit = tonumber(reversed_first_integer:sub(digit_position, digit_position)) or 0
        local second_digit = tonumber(reversed_second_integer:sub(digit_position, digit_position)) or 0
        local digit_difference = first_digit - second_digit - borrow_digit
        if digit_difference < 0 then
            digit_difference = digit_difference + 10
            borrow_digit = 1
        else
            borrow_digit = 0
        end
        integer_difference_digits[digit_position] = tostring(digit_difference)
    end

    local reversed_integer_result = table.concat(integer_difference_digits)
    local integer_result = reversed_integer_result:reverse():gsub("^0+", "")
    if integer_result == "" then
        integer_result = "0"
    end

    local fraction_result = table.concat(fraction_difference_digits):reverse()
    fraction_result = trim_trailing_zeros_from_fraction(fraction_result)

    if fraction_result == "0" then
        return integer_result
    end
    return integer_result .. "." .. fraction_result
end

-- Check if a string starts with a minus sign (indicates negative number)
local function starts_with_minus_sign(value_string)
    return type(value_string) == "string" and value_string:sub(1,1) == "-"
end

-- Validate decimal format: accepts signed integers or decimals with dot separator
local function is_valid_decimal_format(decimal_string)
    if type(decimal_string) ~= "string" then decimal_string = tostring(decimal_string) end
    return decimal_string:match("^[+-]?%d+%.%d+$") or decimal_string:match("^[+-]?%d+$")
end

-- Normalize balance JSON into canonical lower/camelCase keys used by Go
-- Accepts legacy uppercase keys for backward compatibility
local function normalize_balance_to_canonical_format(raw_balance)
    local normalized_balance = {}
    normalized_balance.id = (raw_balance.id or raw_balance.ID) and tostring(raw_balance.id or raw_balance.ID) or nil
    normalized_balance.accountId = (raw_balance.accountId or raw_balance.AccountID) and tostring(raw_balance.accountId or raw_balance.AccountID) or nil
    normalized_balance.assetCode = (raw_balance.assetCode or raw_balance.AssetCode) and tostring(raw_balance.assetCode or raw_balance.AssetCode) or nil
    normalized_balance.available = tostring(raw_balance.available or raw_balance.Available or "0")
    normalized_balance.onHold = tostring(raw_balance.onHold or raw_balance.OnHold or "0")
    normalized_balance.version = tonumber(raw_balance.version or raw_balance.Version or 0)
    normalized_balance.accountType = (raw_balance.accountType or raw_balance.AccountType) and tostring(raw_balance.accountType or raw_balance.AccountType) or nil
    normalized_balance.allowSending = tonumber(raw_balance.allowSending or raw_balance.AllowSending or 0)
    normalized_balance.allowReceiving = tonumber(raw_balance.allowReceiving or raw_balance.AllowReceiving or 0)
    return normalized_balance
end

-- Entry point: batch validate and commit updates
local function main()
    local default_ttl_seconds = 3600 -- default TTL for new balances (seconds)
    local is_pending_transaction = tonumber(ARGV[1])
    local transaction_status = ARGV[2]
    local enforce_optimistic_concurrency_control = tonumber(ARGV[3] or "0")

    -- Validate core arguments
    if is_pending_transaction ~= 0 and is_pending_transaction ~= 1 then
        return redis.error_reply("0017") -- invalid script format
    end
    if transaction_status ~= "PENDING" and transaction_status ~= "APPROVED" and transaction_status ~= "CANCELED" and transaction_status ~= "CREATED" then
        return redis.error_reply("0017") -- invalid script format
    end

    -- ARGV stride per key: 11 arguments per balance operation
    local arguments_per_balance = 11
    local number_of_operations = #KEYS
    if (#ARGV - 3) ~= number_of_operations * arguments_per_balance then
        return redis.error_reply("0017") -- invalid script format
    end

    -- In-memory buffers for two-phase apply
    local balance_states = {}   -- per key: { key, balance, ttl }
    local balance_updates = {}  -- per key: { available, onHold }

    -- Reject duplicate keys in KEYS to avoid ambiguous semantics in a single batch
    local seen_keys = {}
    for i = 1, number_of_operations do
        local k = KEYS[i]
        if seen_keys[k] then
            return redis.error_reply("0017") -- invalid: duplicate keys in batch
        end
        seen_keys[k] = true
    end

    -- Validation phase: read, compute, and validate all operations
    for operation_index = 1, number_of_operations do
        local argument_offset = 3 + (operation_index - 1) * arguments_per_balance -- compute ARGV offset for entry
        local operation_type = ARGV[argument_offset + 1]
        local transaction_amount = ARGV[argument_offset + 2]
        local balance_id = ARGV[argument_offset + 3]
        local initial_available_balance = ARGV[argument_offset + 4]
        local initial_on_hold_balance = ARGV[argument_offset + 5]
        local initial_version_number = tonumber(ARGV[argument_offset + 6])
        local account_type = ARGV[argument_offset + 7]
        local allow_sending_flag = tonumber(ARGV[argument_offset + 8])
        local allow_receiving_flag = tonumber(ARGV[argument_offset + 9])
        local asset_code = ARGV[argument_offset + 10]
        local account_id = ARGV[argument_offset + 11]

        -- Validate operation type and amount format
        if operation_type ~= "DEBIT" and operation_type ~= "CREDIT" and operation_type ~= "ON_HOLD" and operation_type ~= "RELEASE" then
            return redis.error_reply("0017") -- invalid script format
        end
        if not is_valid_decimal_format(transaction_amount) or starts_with_minus_sign(transaction_amount) then
            return redis.error_reply("0017") -- invalid script format (negative amounts not allowed)
        end

        local redis_key = KEYS[operation_index]
        local existing_balance_json = redis.call("GET", redis_key)   -- load current value, if any
        local existing_ttl_seconds = redis.call("TTL", redis_key) -- TTL: -2 no key; -1 persistent; >0 seconds

        local current_balance = {
            id = balance_id,
            available = initial_available_balance,
            onHold = initial_on_hold_balance,
            version = initial_version_number,
            accountType = account_type,
            allowSending = allow_sending_flag,
            allowReceiving = allow_receiving_flag,
            assetCode = asset_code,
            accountId = account_id,
        }

        if existing_balance_json then
            local decoded_balance = cjson.decode(existing_balance_json)
            current_balance = normalize_balance_to_canonical_format(decoded_balance)
        else
            -- Do not write during validation; use provided seed and mark as new key
            existing_ttl_seconds = -2
        end

        -- Optional OCC check: reject if provided version != stored version
        if enforce_optimistic_concurrency_control == 1 and existing_balance_json then
            local provided_version_number = initial_version_number or 0
            local stored_version_number = tonumber(current_balance.version) or 0
            if provided_version_number ~= stored_version_number then
                return redis.error_reply("0086") -- version mismatch (race condition)
            end
        end

        -- Compute result using same logic matrix as single-key script
        local new_available_balance = current_balance.available
        local new_on_hold_balance = current_balance.onHold
        local is_from_account = false       -- pending 'from' flow (blocks external accounts)
        local is_debit_operation = false    -- requires allowSending permission
        local is_credit_operation = false   -- requires allowReceiving permission

        if is_pending_transaction == 1 then
            if operation_type == "ON_HOLD" and transaction_status == "PENDING" then
                new_available_balance = subtract_decimal(current_balance.available, transaction_amount)
                new_on_hold_balance = add_decimal(current_balance.onHold, transaction_amount)
                is_from_account = true
                is_debit_operation = true
            elseif operation_type == "RELEASE" and transaction_status == "CANCELED" then
                new_on_hold_balance = subtract_decimal(current_balance.onHold, transaction_amount)
                new_available_balance = add_decimal(current_balance.available, transaction_amount)
                is_from_account = false
                is_credit_operation = true
            elseif transaction_status == "APPROVED" then
                if operation_type == "DEBIT" then
                    new_on_hold_balance = subtract_decimal(current_balance.onHold, transaction_amount)
                    is_from_account = true
                    is_debit_operation = true
                elseif operation_type == "CREDIT" then
                    new_available_balance = add_decimal(current_balance.available, transaction_amount)
                    is_credit_operation = true
                end
            end
        else
            if operation_type == "DEBIT" then
                new_available_balance = subtract_decimal(current_balance.available, transaction_amount)
                is_debit_operation = true
            elseif operation_type == "CREDIT" then
                new_available_balance = add_decimal(current_balance.available, transaction_amount)
                is_credit_operation = true
            end
        end

        -- Policy and invariant checks (abort entire batch on any failure)
        if is_pending_transaction == 1 and is_from_account and current_balance.accountType == "external" then
            return redis.error_reply("0098") -- external account cannot operate on-hold
        end
        if is_debit_operation and current_balance.allowSending ~= 1 then
            return redis.error_reply("0019") -- account not eligible for sending
        end
        if is_credit_operation and current_balance.allowReceiving ~= 1 then
            return redis.error_reply("0019") -- account not eligible for receiving
        end
        if starts_with_minus_sign(new_available_balance) and current_balance.accountType ~= "external" then
            return redis.error_reply("0018") -- insufficient funds (negative balance not allowed)
        end
        if starts_with_minus_sign(new_on_hold_balance) then
            return redis.error_reply("0018") -- insufficient funds (negative on-hold not allowed)
        end

        -- Store state and computed update for commit phase
        balance_states[operation_index] = { 
            key = redis_key, 
            balance = current_balance, 
            ttl = existing_ttl_seconds 
        }
        balance_updates[operation_index] = { 
            available = new_available_balance, 
            onHold = new_on_hold_balance 
        }
    end

    -- Commit phase: apply all updates atomically
    local result_json_array = {}
    for operation_index = 1, number_of_operations do
        local balance_state = balance_states[operation_index]
        local balance_update = balance_updates[operation_index]
        
        -- Apply the computed changes to the balance
        balance_state.balance.available = balance_update.available
        balance_state.balance.onHold = balance_update.onHold
        balance_state.balance.version = tonumber(balance_state.balance.version or 0) + 1 -- increment version for OCC
        
        local updated_balance_json = cjson.encode(balance_state.balance)
        
        -- Set with appropriate TTL handling
        if balance_state.ttl == -2 then
            -- New key: set with default TTL
            redis.call("SET", balance_state.key, updated_balance_json, "EX", default_ttl_seconds)
        elseif balance_state.ttl == -1 then
            -- Existing persistent key: keep persistent (no TTL)
            redis.call("SET", balance_state.key, updated_balance_json)
        else
            -- Existing key with TTL: preserve remaining TTL
            local remaining_ttl_seconds = balance_state.ttl
            if remaining_ttl_seconds < 1 then remaining_ttl_seconds = 1 end
            redis.call("SET", balance_state.key, updated_balance_json, "EX", remaining_ttl_seconds)
        end
        
        result_json_array[operation_index] = updated_balance_json -- return updated balance JSON for each key
    end

    return result_json_array
end

return main()
