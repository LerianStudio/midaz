--[[
================================================================================
                    REDIS BALANCE UPDATE ATOMIC SCRIPT
================================================================================

PURPOSE:
    Atomically updates account balances in Redis during transaction processing.
    Handles all transaction lifecycle states (PENDING, APPROVED, CANCELED, NOTED)
    with precise decimal arithmetic to avoid floating-point errors.

TRANSACTION STATE MACHINE:
    - PENDING  + ON_HOLD : Locks funds (Available -= amount, OnHold += amount)
    - CANCELED + RELEASE : Unlocks funds (OnHold -= amount, Available += amount)
    - APPROVED + DEBIT   : Finalizes send (OnHold -= amount)
    - APPROVED + CREDIT  : Finalizes receive (Available += amount)
    - NOTED    + any     : No balance change (audit/informational record only)

KEYS (passed via Redis KEYS array):
    KEYS[1] = backup_queue:{transactions}    -- Hash storing transaction snapshots for crash recovery
    KEYS[2] = transaction key                -- Unique key (org:ledger:transaction format)
    KEYS[3] = schedule:{transactions}:balance-sync -- Sorted Set for scheduling DB sync operations
    KEYS[4] = idemp:{transactions}:{org}:{ledger}:{tx_id} -- Idempotency key (48h TTL)

ARGV (passed via Redis ARGV array):
    ARGV[1]       = scheduleSync flag (1=enabled, 0=disabled)
    ARGV[2..17]   = First balance operation (16 fields per balance)
    ARGV[18..33]  = Second balance operation (if present)
    ...and so on for additional balances...

BALANCE OPERATION FIELDS (16 fields per balance, groupSize=16):
    [i+0]  = redisBalanceKey   -- Redis key for this balance
    [i+1]  = isPending         -- 1 if two-phase transaction, 0 if immediate
    [i+2]  = transactionStatus -- PENDING, APPROVED, CANCELED, or NOTED
    [i+3]  = operation         -- ON_HOLD, RELEASE, DEBIT, or CREDIT
    [i+4]  = amount            -- Transaction amount as decimal string
    [i+5]  = alias             -- Account alias for display purposes
    [i+6]  = ID                -- Balance record UUID from PostgreSQL
    [i+7]  = Available         -- Available balance from PostgreSQL (initial value)
    [i+8]  = OnHold            -- OnHold balance from PostgreSQL (initial value)
    [i+9]  = Version           -- Optimistic locking version number
    [i+10] = AccountType       -- "internal" or "external"
    [i+11] = AllowSending      -- 1 if account can send, 0 if not
    [i+12] = AllowReceiving    -- 1 if account can receive, 0 if not
    [i+13] = AssetCode         -- Currency/asset code (e.g., "USD", "BRL")
    [i+14] = AccountID         -- Parent account UUID from PostgreSQL
    [i+15] = Key               -- Composite key for balance lookup

ERROR CODES RETURNED:
    0018 = Internal accounts cannot have negative Available balance (insufficient funds)
    0061 = Balance cache corruption detected (data integrity failure)
    0098 = External accounts cannot use ON_HOLD operations (business rule)
    0130 = OnHold balance cannot go negative (data integrity - more released than held)

DEPENDENCIES:
    This script uses the built-in 'cjson' module provided by Redis for all Lua scripts.
    No external dependencies are required.

TIMING PARAMETERS:
    TTL        = 3600 seconds (1 hour cache lifetime)
    warnBefore = 600 seconds (schedule DB sync 10 min before cache expiry)
    dueAt      = now + 3000 seconds (sync scheduled ~50 min after creation)
    TTL_IDEMP  = 172800 seconds (48 hours idempotency window)

WHY CUSTOM DECIMAL ARITHMETIC:
    IEEE 754 floating-point cannot exactly represent all decimals (e.g., 0.1 + 0.2 = 0.30000000000000004).
    Financial systems require exact arithmetic, so we implement digit-by-digit string math.

ATOMICITY GUARANTEE:
    This entire script executes as a single Redis transaction (EVAL is atomic).
    Either all balance updates succeed, or none do (via rollback on error).

================================================================================
--]]


--[[
================================================================================
                        DECIMAL ARITHMETIC FUNCTIONS
================================================================================
    These functions implement precise decimal arithmetic using string manipulation.

    WHY: Lua's native number type uses IEEE 754 double-precision floats, which
    cannot exactly represent all decimal values. In financial systems, errors like
    0.1 + 0.2 = 0.30000000000000004 are unacceptable.

    APPROACH: Parse decimal strings into integer and fractional parts, perform
    digit-by-digit arithmetic with carry/borrow propagation, then reassemble.
================================================================================
--]]


--[[
    split_decimal(s) - Parses a decimal string into its components

    INPUT:  A decimal string like "-123.456" or "789"
    OUTPUT: Three values:
            1. Integer part with sign (e.g., "-123" or "789")
            2. Fractional part without decimal point (e.g., "456" or "")
            3. Boolean indicating if the number was negative

    WHY: Separating components allows digit-by-digit arithmetic.
         The sign is tracked separately to handle negative number edge cases.
--]]
local function split_decimal(s)
    -- Initialize sign as empty string (positive number assumed)
    -- WHY: We track sign separately to simplify the arithmetic logic;
    --      the core add/sub algorithms work on absolute values
    local sign = ""

    -- Check if the first character is a minus sign
    -- WHY: Negative numbers need special handling in arithmetic operations;
    --      we extract the sign here to process the magnitude separately
    if s:sub(1, 1) == "-" then
        -- Store the minus sign for later reconstruction
        -- WHY: We need to know the original sign to correctly handle
        --      operations like (-a) + (-b) = -(a + b)
        sign = "-"
        -- Remove the minus sign from the string, keeping only the magnitude
        -- WHY: The arithmetic functions work on positive values; sign is
        --      handled at a higher level through operation transformation
        s = s:sub(2)
    end

    -- Use Lua pattern matching to extract integer and fractional parts
    -- Pattern "^(%d+)%.(%d+)$" matches: start, digits, dot, digits, end
    -- WHY: We need to separate these parts to align decimal points
    --      during addition/subtraction (e.g., 1.5 + 2.25 requires alignment)
    local intp, fracp = s:match("^(%d+)%.(%d+)$")
    -- If pattern matched (number has decimal point)
    -- WHY: Numbers with fractional parts are handled differently than integers
    if intp then
        -- Return: signed integer part, fractional part, and negativity flag
        -- WHY: The caller needs all three pieces to perform arithmetic:
        --      - integer part for whole number arithmetic
        --      - fractional part for decimal arithmetic
        --      - sign flag to determine operation transformation
        return sign .. intp, fracp, sign ~= ""
    else
        -- No decimal point found - treat entire string as integer part
        -- WHY: Whole numbers like "100" have no fractional component,
        --      so we return empty string for the fractional part
        return sign .. s, "", sign ~= ""
    end
end -- End of split_decimal function


--[[
    rtrim_zeros(frac) - Removes trailing zeros from fractional part

    INPUT:  A fractional string like "500" (representing .500)
    OUTPUT: Normalized string like "5" (representing .5)

    WHY: Prevents accumulating unnecessary trailing zeros in results.
         "100.500" should be normalized to "100.5" for clean output.
         Also ensures consistent representation for comparison operations.
--]]
local function rtrim_zeros(frac)
    -- Use gsub to remove all trailing zeros using pattern "0+$"
    -- Pattern: "0+" = one or more zeros, "$" = end of string
    -- WHY: Trailing zeros in fractional parts don't change the value
    --      (0.50 == 0.5) but clutter output and waste memory
    frac = frac:gsub("0+$", "")
    -- If all zeros were removed (empty string), return "0"
    -- WHY: An empty fractional part should be represented as "0"
    --      rather than empty string for consistent behavior downstream
    --      (also allows clean concatenation in parent functions)
    return (frac == "" and "0") or frac
end -- End of rtrim_zeros function


--[[
    normalize_decimal_str(s) - Normalizes common decimal string variants

    INPUT:  Decimal-like string (e.g., ".5", "1.", "-.5")
    OUTPUT: Normalized string (e.g., "0.5", "1.0", "-0.5")

    WHY: Ensures split_decimal and digit loops receive canonical forms.
--]]
local function normalize_decimal_str(s)
    s = tostring(s)

    -- Handle trailing decimal point (e.g., "1." -> "1.0")
    if s:match("^%-?%d+%.$") then
        return s .. "0"
    end

    -- Handle leading decimal point (e.g., ".5" -> "0.5", "-.5" -> "-0.5")
    if s:match("^%-?%.%d+$") then
        if s:sub(1, 1) == "-" then
            return "-0" .. s:sub(2)
        end
        return "0" .. s
    end

    return s
end -- End of normalize_decimal_str function


--[[
    is_decimal_string(s) - Validates strict decimal string format

    INPUT:  String value
    OUTPUT: true if format is integer or decimal, false otherwise

    WHY: Rejects scientific notation or corrupted values that break arithmetic.
--]]
local function is_decimal_string(s)
    s = tostring(s)
    if s:match("^%-?%d+$") then return true end
    if s:match("^%-?%d+%.%d+$") then return true end
    return false
end -- End of is_decimal_string function


--[[
    compare_positive_decimals(a, b) - Compares two non-negative decimal strings

    INPUT:  Two decimal strings without sign (e.g., "123.45", "67.89", "0")
    OUTPUT: -1 if a < b, 0 if a == b, 1 if a > b

    WHY: Avoids tonumber() precision loss for large values. We compare
         by integer length, lexicographic integer digits, then fractional digits.
--]]
local function compare_positive_decimals(a, b)
    -- Split into integer and fractional parts (signs ignored if present)
    local ai, af = split_decimal(a)
    local bi, bf = split_decimal(b)

    -- Defensive: strip any minus sign (shouldn't be present for positive inputs)
    if ai:sub(1, 1) == "-" then ai = ai:sub(2) end
    if bi:sub(1, 1) == "-" then bi = bi:sub(2) end

    -- Normalize integer parts by removing leading zeros
    ai = ai:gsub("^0+", "")
    bi = bi:gsub("^0+", "")
    if ai == "" then ai = "0" end
    if bi == "" then bi = "0" end

    -- Compare integer length first
    if #ai < #bi then return -1 end
    if #ai > #bi then return 1 end

    -- Same length: lexicographic compare integer digits
    if ai < bi then return -1 end
    if ai > bi then return 1 end

    -- Integers equal: compare fractional parts
    local maxf = math.max(#af, #bf)
    if maxf == 0 then return 0 end

    if #af < maxf then af = af .. string.rep("0", maxf - #af) end
    if #bf < maxf then bf = bf .. string.rep("0", maxf - #bf) end

    if af < bf then return -1 end
    if af > bf then return 1 end
    return 0
end -- End of compare_positive_decimals function


-- Forward declaration of sub_decimal function
-- WHY: Lua requires forward declaration because add_decimal and sub_decimal
--      are mutually recursive (add_decimal may call sub_decimal and vice versa).
--      Without this, sub_decimal would be nil when add_decimal tries to call it.
local sub_decimal


--[[
    add_decimal(a, b) - Adds two decimal numbers with arbitrary precision

    INPUT:  Two decimal strings (e.g., "123.45", "-67.89")
    OUTPUT: Sum as decimal string with trailing zeros trimmed

    ALGORITHM:
        1. Handle sign combinations (transforms to simpler cases)
        2. Pad fractional parts to equal length for alignment
        3. Add fractional parts right-to-left with carry propagation
        4. Add integer parts right-to-left with carry propagation
        5. Combine and normalize result

    WHY: Standard floating-point addition has precision issues.
         This function guarantees exact decimal arithmetic required for
         financial transactions where every cent must be accounted for.
--]]
local function add_decimal(a, b)
    -- Convert inputs to strings to handle numeric inputs
    -- WHY: Callers might pass Lua numbers; tostring ensures
    --      consistent string processing regardless of input type
    a = tostring(a)
    -- Convert second operand to string as well
    -- WHY: Same reason - ensures uniform handling of both operands
    b = tostring(b)
    -- Split first operand into integer part, fractional part, and sign flag
    -- WHY: We need these components for digit-by-digit arithmetic;
    --      the sign flag tells us if we need to transform the operation
    local ai, af, a_negative = split_decimal(a)
    -- Split second operand similarly
    -- WHY: Both operands must be decomposed the same way for the
    --      arithmetic algorithm to work correctly
    local bi, bf, b_negative = split_decimal(b)

    -- CASE 1: Both numbers are negative: (-a) + (-b) = -(a + b)
    -- WHY: Transform double-negative addition into positive addition
    --      followed by sign flip; simplifies the core algorithm
    if a_negative and b_negative then
        -- Recursively add the absolute values (strip minus signs)
        -- WHY: We've reduced the problem to positive number addition,
        --      which is the core case our algorithm handles
        local result = add_decimal(a:sub(2), b:sub(2))
        -- Prepend minus sign to the result
        -- WHY: The sum of two negatives is always negative
        return "-" .. result
    end

    -- CASE 2: First number is negative: (-a) + b = b - a
    -- WHY: Transform into subtraction; this reduces the number of
    --      cases the core algorithm must handle directly
    if a_negative then
        -- Convert to subtraction: b minus the absolute value of a
        -- WHY: Subtraction is easier to reason about than mixed-sign addition;
        --      sub_decimal handles all the complexity of comparing magnitudes
        return sub_decimal(b, a:sub(2))
    end

    -- CASE 3: Second number is negative: a + (-b) = a - b
    -- WHY: Transform into subtraction (commutative property doesn't apply
    --      when signs differ, so we handle this case explicitly)
    if b_negative then
        -- Convert to subtraction: a minus the absolute value of b
        -- WHY: Same reason as CASE 2 - delegate to sub_decimal
        return sub_decimal(a, b:sub(2))
    end

    -- At this point, both numbers are positive (main case)

    -- Strip any remaining minus signs from integer parts (defensive)
    -- WHY: After the sign transformations above, there shouldn't be
    --      minus signs, but this guards against edge cases in split_decimal
    if ai:sub(1, 1) == "-" then ai = ai:sub(2) end
    -- Same defensive stripping for second operand
    -- WHY: Ensures the digit-by-digit loops below only see digits 0-9
    if bi:sub(1, 1) == "-" then bi = bi:sub(2) end

    -- Pad fractional parts to equal length for proper alignment
    -- WHY: When adding 1.5 + 2.25, we need to align as:
    --        1.50
    --      + 2.25
    --      ------
    --      Without padding, digit positions would be misaligned
    if #af < #bf then
        -- Pad first fractional part with trailing zeros
        -- WHY: "5" becomes "50" to align with "25" in example above
        af = af .. string.rep("0", #bf - #af)
    elseif #bf < #af then
        -- Pad second fractional part with trailing zeros
        -- WHY: Symmetric case - ensure both have same number of digits
        bf = bf .. string.rep("0", #af - #bf)
    end

    -- Initialize carry for fractional part addition
    -- WHY: When digit sum >= 10, we need to carry 1 to the next position
    --      (e.g., 7 + 5 = 12, write 2 and carry 1)
    local carry = 0
    -- Table to accumulate fractional result digits
    -- WHY: Lua tables are more efficient than string concatenation in loops
    local frac_sum = {}
    -- Iterate through fractional digits from RIGHT to LEFT
    -- WHY: Addition must start from least significant digit to propagate
    --      carries correctly (just like manual addition on paper)
    for i = #af, 1, -1 do
        -- Extract i-th digit from first fractional part
        -- WHY: We process one digit at a time for precise arithmetic
        local da = tonumber(af:sub(i, i))
        -- Extract corresponding digit from second fractional part
        -- WHY: These aligned digits will be added together
        local db = tonumber(bf:sub(i, i))
        -- Add the two digits plus any carry from previous position
        -- WHY: This is the core addition step; carry propagates the
        --      "tens" digit when sum >= 10
        local s = da + db + carry
        -- Calculate new carry (1 if sum >= 10, else 0)
        -- WHY: math.floor(s / 10) extracts the tens digit
        --      e.g., math.floor(15 / 10) = 1
        carry = math.floor(s / 10)
        -- Store the ones digit of the sum (s % 10 extracts units digit)
        -- Index calculation: #af - i + 1 builds result in reverse order
        -- WHY: We're iterating right-to-left but storing left-to-right
        --      for easier final concatenation
        frac_sum[#af - i + 1] = tostring(s % 10)
    end

    -- Reverse integer parts for right-to-left processing
    -- WHY: String reversal lets us iterate from index 1 upward
    --      while still processing digits right-to-left
    local rii = ai:reverse()
    -- Reverse second integer part similarly
    -- WHY: Both must be reversed for the loop to align digits correctly
    local rbi = bi:reverse()
    -- Determine the maximum length to iterate
    -- WHY: The longer number determines how many iterations we need;
    --      shorter number's missing digits are treated as 0
    local max_i = math.max(#rii, #rbi)
    -- Table to accumulate integer result digits
    -- WHY: Same efficiency reason as frac_sum above
    local int_sum = {}
    -- Iterate through integer digits from RIGHT to LEFT (position 1 = rightmost)
    -- WHY: We continue with the carry from fractional part addition;
    --      this is how decimal point crossing works in manual addition
    for i = 1, max_i do
        -- Extract i-th digit from reversed first integer (or 0 if past end)
        -- WHY: "or 0" handles case where numbers have different lengths
        --      e.g., adding 5 + 123, the 5 only has one digit
        local da = tonumber(rii:sub(i, i)) or 0
        -- Extract corresponding digit from second integer (or 0)
        -- WHY: Same handling for potentially shorter second operand
        local db = tonumber(rbi:sub(i, i)) or 0
        -- Add digits plus carry from previous position (or from frac addition)
        -- WHY: The carry from the last fractional digit flows into
        --      the first integer digit (crossing the decimal point)
        local s = da + db + carry
        -- Calculate new carry for next iteration
        -- WHY: Same carry propagation logic as in fractional part
        carry = math.floor(s / 10)
        -- Store the ones digit at position i
        -- WHY: Building result in same order as iteration since
        --      we already reversed the inputs
        int_sum[i] = tostring(s % 10)
    end
    -- If there's a final carry, append it as the most significant digit
    -- WHY: Adding 999 + 1 = 1000 creates an extra digit;
    --      without this, we'd lose the leading 1
    if carry > 0 then
        -- Append carry as new most significant digit
        -- WHY: This extends the result by one digit when needed
        int_sum[#int_sum + 1] = tostring(carry)
    end

    -- Concatenate integer digits and reverse to get correct order
    -- WHY: int_sum was built in reverse order (LSB first),
    --      so we reverse to get MSB first (normal reading order)
    local int_res = table.concat(int_sum):reverse()
    -- Concatenate fractional digits and reverse to get correct order
    -- WHY: Same reversal needed for fractional part
    local frac_res = table.concat(frac_sum):reverse()
    -- Remove trailing zeros from fractional part
    -- WHY: Normalize result (0.50 -> 0.5) for clean output
    frac_res = rtrim_zeros(frac_res)

    -- If fractional part is just "0", return integer only
    -- WHY: No need to write "5.0" when "5" is cleaner and equivalent
    if frac_res == "0" then
        -- Return integer result without decimal point
        -- WHY: Clean output format for whole number results
        return int_res
    end
    -- Return complete decimal number with dot separator
    -- WHY: Standard decimal notation for non-whole numbers
    return int_res .. "." .. frac_res
end -- End of add_decimal function


--[[
    sub_decimal(a, b) - Subtracts two decimal numbers with arbitrary precision

    INPUT:  Two decimal strings (e.g., "123.45", "67.89")
    OUTPUT: Difference as decimal string (a - b) with trailing zeros trimmed

    ALGORITHM:
        1. Handle sign combinations (transforms to simpler cases)
        2. If a < b, compute b - a and negate result
        3. Pad fractional parts to equal length for alignment
        4. Subtract fractional parts right-to-left with borrow propagation
        5. Subtract integer parts right-to-left with borrow propagation
        6. Combine and normalize result

    WHY: Subtraction is needed for balance decrements (debits).
         Like add_decimal, this ensures exact arithmetic for financial values.
--]]
sub_decimal = function(a, b)
    -- Convert inputs to strings for consistent processing
    -- WHY: Handles case where caller passes Lua number instead of string
    a = tostring(a)
    -- Convert second operand to string as well
    -- WHY: Both operands must be strings for the character-by-character processing
    b = tostring(b)
    -- Split first operand into components
    -- WHY: Same decomposition as add_decimal for digit-by-digit processing
    local ai, af, a_negative = split_decimal(a)
    -- Split second operand into components
    -- WHY: Need both operands decomposed for the subtraction algorithm
    local bi, bf, b_negative = split_decimal(b)

    -- CASE 1: Both numbers negative: (-a) - (-b) = (-a) + b = b - a
    -- WHY: Transform double-negative case; subtracting a negative
    --      is the same as adding its absolute value
    if a_negative and b_negative then
        -- Recursively compute with swapped operands and signs removed
        -- WHY: (-5) - (-3) = 3 - 5 = -2, which is handled by the
        --      magnitude comparison in the recursive call
        return sub_decimal(b:sub(2), a:sub(2))
    end

    -- CASE 2: First number negative: (-a) - b = -(a + b)
    -- WHY: Subtracting from a negative makes it more negative
    if a_negative then
        -- Add magnitudes and negate the result
        -- WHY: (-5) - 3 = -(5 + 3) = -8
        local result = add_decimal(a:sub(2), b)
        -- Prepend minus sign to indicate negative result
        -- WHY: The sum of magnitudes with a leading negative sign
        return "-" .. result
    end

    -- CASE 3: Second number negative: a - (-b) = a + b
    -- WHY: Subtracting a negative is equivalent to addition
    if b_negative then
        -- Convert to addition of absolute values
        -- WHY: 5 - (-3) = 5 + 3 = 8
        return add_decimal(a, b:sub(2))
    end

    -- At this point, both numbers are positive
    -- Check if a < b (result would be negative)
    -- WHY: We need to know which number is larger to ensure
    --      the subtraction algorithm works (subtracting smaller from larger)
    -- NOTE: Use string-based comparison to avoid tonumber() precision loss
    local cmp = compare_positive_decimals(a, b)
    -- If equal, result is zero
    if cmp == 0 then
        return "0"
    end
    -- If a < b, swap and negate: a - b = -(b - a)
    -- WHY: Our subtraction algorithm requires the larger number first;
    --      if a < b, we compute b - a and flip the sign
    if cmp < 0 then
        -- Recursively compute b - a (which will be positive)
        -- WHY: b - a is positive when b > a, then we negate
        local result = sub_decimal(b, a)
        -- Negate the result since we swapped operands
        -- WHY: 3 - 5 = -(5 - 3) = -2
        return "-" .. result
    end

    -- Strip any remaining minus signs from integer parts (defensive)
    -- WHY: After all sign transformations, integers should be positive,
    --      but this guards against any edge cases
    if ai:sub(1, 1) == "-" then ai = ai:sub(2) end
    -- Same defensive stripping for second operand
    -- WHY: Ensures clean digit-only strings for the loops below
    if bi:sub(1, 1) == "-" then bi = bi:sub(2) end

    -- Pad fractional parts to equal length for proper alignment
    -- WHY: Same reason as in add_decimal - align decimal points
    --      for correct digit-by-digit processing
    if #af < #bf then
        -- Pad first fractional part with trailing zeros
        -- WHY: "5" becomes "50" to align with "25"
        af = af .. string.rep("0", #bf - #af)
    elseif #bf < #af then
        -- Pad second fractional part with trailing zeros
        -- WHY: Symmetric case for alignment
        bf = bf .. string.rep("0", #af - #bf)
    end

    -- Initialize borrow for fractional part subtraction
    -- WHY: When subtracting a larger digit from a smaller one,
    --      we need to borrow 1 from the next position (like manual subtraction)
    local borrow = 0
    -- Table to accumulate fractional result digits
    -- WHY: Efficient accumulation for later concatenation
    local frac_res_tbl = {}
    -- Iterate through fractional digits from RIGHT to LEFT
    -- WHY: Subtraction must start from least significant digit
    --      to properly propagate borrows
    for i = #af, 1, -1 do
        -- Extract i-th digit from first fractional part
        -- WHY: This is the digit we're subtracting FROM
        local da = tonumber(af:sub(i, i))
        -- Extract corresponding digit from second fractional part
        -- WHY: This is the digit we're subtracting
        local db = tonumber(bf:sub(i, i))
        -- Compute difference accounting for any borrow from previous position
        -- WHY: If previous subtraction needed to borrow, this digit
        --      is effectively reduced by 1
        local diff = da - db - borrow
        -- If difference is negative, we need to borrow from next position
        -- WHY: Can't have negative digits; borrow adds 10 to current position
        --      and subtracts 1 from next position (like manual subtraction)
        if diff < 0 then
            -- Add 10 to make difference positive (borrowed from next position)
            -- WHY: e.g., 3 - 7 = -4 -> -4 + 10 = 6 (with borrow)
            diff = diff + 10
            -- Set borrow flag for next iteration
            -- WHY: The next position must give up 1 to cover our borrow
            borrow = 1
        else
            -- No borrow needed - clear the flag
            -- WHY: Reset for next iteration when difference is non-negative
            borrow = 0
        end
        -- Store the result digit (same indexing as add_decimal)
        -- WHY: Building result in reverse order for later reversal
        frac_res_tbl[#af - i + 1] = tostring(diff)
    end

    -- Reverse integer parts for right-to-left processing
    -- WHY: Same technique as add_decimal for iteration convenience
    local rii = ai:reverse()
    -- Reverse second integer part similarly
    -- WHY: Both must be reversed for aligned digit access
    local rbi = bi:reverse()
    -- Determine maximum iteration length
    -- WHY: Process all digits of the longer number
    local max_i = math.max(#rii, #rbi)
    -- Table to accumulate integer result digits
    -- WHY: Efficient accumulation for later concatenation
    local int_res_tbl = {}
    -- Iterate through integer digits from RIGHT to LEFT
    -- WHY: Continue with borrow from fractional subtraction
    for i = 1, max_i do
        -- Extract i-th digit from reversed first integer (or 0)
        -- WHY: "or 0" handles shorter number case
        local da = tonumber(rii:sub(i, i)) or 0
        -- Extract corresponding digit from second integer (or 0)
        -- WHY: Same handling for potentially shorter second operand
        local db = tonumber(rbi:sub(i, i)) or 0
        -- Compute difference with borrow from previous position
        -- WHY: Borrow from last fractional digit flows into first integer digit
        local diff = da - db - borrow
        -- Handle negative difference with borrow
        -- WHY: Same borrow logic as fractional part
        if diff < 0 then
            -- Add 10 and set borrow for next position
            -- WHY: Standard borrow propagation in subtraction
            diff = diff + 10
            -- Mark that we borrowed from next position
            -- WHY: Next iteration must account for this borrow
            borrow = 1
        else
            -- Clear borrow flag
            -- WHY: No borrow needed when difference >= 0
            borrow = 0
        end
        -- Store result digit
        -- WHY: Accumulate for final concatenation
        int_res_tbl[i] = tostring(diff)
    end

    -- Concatenate and reverse integer result
    -- WHY: int_res_tbl was built LSB-first, need MSB-first for output
    local res_int_rev = table.concat(int_res_tbl)
    -- Reverse and strip leading zeros (e.g., "007" -> "7")
    -- WHY: Leading zeros in integers are meaningless and should be removed
    --      for clean output (gsub pattern "^0+" matches leading zeros)
    local res_int = res_int_rev:reverse():gsub("^0+", "")
    -- If all digits were zeros, use "0" instead of empty string
    -- WHY: The result "0" should be represented as "0", not ""
    if res_int == "" then
        -- Set to single zero for proper representation
        -- WHY: Empty string would be invalid output format
        res_int = "0"
    end

    -- Concatenate and reverse fractional result
    -- WHY: Same reversal needed as for integer part
    local frac_normal = table.concat(frac_res_tbl):reverse()
    -- Remove trailing zeros from fractional part
    -- WHY: Normalize output (0.50 -> 0.5)
    frac_normal = rtrim_zeros(frac_normal)

    -- If fractional part is just "0", return integer only
    -- WHY: Clean output without unnecessary ".0" suffix
    if frac_normal == "0" then
        -- Return integer result only
        -- WHY: "5" is cleaner than "5.0"
        return res_int
    end
    -- Return complete decimal with fractional part
    -- WHY: Standard decimal notation for non-whole results
    return res_int .. "." .. frac_normal
end -- End of sub_decimal function


--[[
================================================================================
                            UTILITY FUNCTIONS
================================================================================
--]]


--[[
    startsWithMinus(s) - Checks if a string starts with a minus sign

    INPUT:  A string value
    OUTPUT: Boolean true if first character is "-", false otherwise

    WHY: Used to detect negative balance results, which indicate
         insufficient funds for internal accounts (error condition).
--]]
local function startsWithMinus(s)
    -- Check if first character equals "-"
    -- WHY: Simple and efficient check for negative numbers;
    --      used to enforce business rule that internal accounts
    --      cannot have negative balances
    return s:sub(1, 1) == "-"
end -- End of startsWithMinus function


--[[
    cloneBalance(tbl) - Creates a shallow copy of a balance table

    INPUT:  A Lua table representing a balance
    OUTPUT: A new table with the same key-value pairs

    WHY: Used to capture balance state BEFORE modifications for the
         transaction backup. Without cloning, the backup would reference
         the modified balance (Lua tables are passed by reference).
--]]
local function cloneBalance(tbl)
    -- Create new empty table for the copy
    -- WHY: We need a separate memory allocation to avoid
    --      reference aliasing issues
    local copy = {}
    -- Iterate through all key-value pairs in source table
    -- WHY: Shallow copy is sufficient here since all values are
    --      primitives (strings, numbers), not nested tables
    for k, v in pairs(tbl) do
        -- Copy each key-value pair to the new table
        -- WHY: This creates independent copies of each field
        copy[k] = v
    end
    -- Return the cloned table
    -- WHY: Caller gets a new table they can modify without
    --      affecting the original
    return copy
end -- End of cloneBalance function


--[[
    updateTransactionHash(transactionBackupQueue, transactionKey, balances)

    PURPOSE: Stores or updates a transaction's balance snapshot in the backup queue.
             This enables crash recovery - if Redis fails mid-transaction,
             the backup data can be used to restore consistency.

    INPUT:
        transactionBackupQueue - Redis Hash key for storing backups
        transactionKey        - Unique identifier for this transaction
        balances              - Array of balance states BEFORE modification

    OUTPUT: The JSON-encoded transaction data that was stored

    WHY: Financial systems require durability guarantees. This backup
         allows reconstruction of transaction state if a failure occurs
         between Redis update and PostgreSQL sync.
--]]
local function updateTransactionHash(transactionBackupQueue, transactionKey, balances)
    -- Declare transaction variable in local scope
    -- WHY: Will hold the transaction object to be stored,
    --      either newly created or retrieved and updated
    local transaction

    -- Attempt to retrieve existing transaction backup from Redis Hash
    -- WHY: Transactions may have multiple balance operations (multi-party);
    --      we need to update existing backup rather than overwrite
    local raw = redis.call("HGET", transactionBackupQueue, transactionKey)
    -- Check if no existing backup was found
    -- WHY: First operation for this transaction needs to create new backup;
    --      subsequent operations update the existing one
    if not raw then
        -- Create new transaction object with balances array
        -- WHY: Initialize the backup structure for first-time storage
        transaction = { balances = balances }
    else
        -- Existing backup found - decode and update it
        -- WHY: pcall provides safe JSON parsing that won't crash on invalid data
        local ok, decoded = pcall(cjson.decode, raw)
        -- Check if decode succeeded and result is a table
        -- WHY: Defensive check against corrupted data in Redis;
        --      if decode fails, we create fresh backup
        if ok and type(decoded) == "table" then
            -- Use existing transaction structure
            -- WHY: Preserve any other fields that might exist in the backup
            transaction = decoded
            -- Update balances array with new values
            -- WHY: This is the actual update - new balance states are recorded
            transaction.balances = balances
        else
            -- Decode failed - create fresh backup (data was corrupted)
            -- WHY: Recovery from corruption - better to lose old backup
            --      than to fail entirely
            transaction = { balances = balances }
        end
    end

    -- Encode transaction object to JSON for storage
    -- WHY: Redis stores strings; cjson.encode converts Lua table to JSON
    local updated = cjson.encode(transaction)
    -- Store the updated backup in Redis Hash
    -- WHY: HSET atomically updates the hash field;
    --      Hash structure allows multiple transactions to be stored efficiently
    redis.call("HSET", transactionBackupQueue, transactionKey, updated)

    -- Return the encoded data (useful for debugging/logging)
    -- WHY: Caller might need to verify what was stored
    return updated
end -- End of updateTransactionHash function


--[[
    rollback(rollbackBalances, ttl)

    PURPOSE: Restores balances to their pre-modification state when an
             error occurs (e.g., insufficient funds, business rule violation).

    INPUT:
        rollbackBalances - Table mapping Redis keys to their original JSON values
        ttl              - Time-to-live to set on restored keys

    WHY: Ensures atomicity - if ANY balance operation fails, ALL balances
         in the transaction are restored to their original state.
         This is the "all or nothing" guarantee of the transaction.
--]]
local function rollback(rollbackBalances, ttl)
    -- Check if there are any balances to rollback
    -- WHY: next() returns nil for empty tables; skip rollback if nothing to restore
    --      (this can happen if error occurs on first balance operation)
  if next(rollbackBalances) then
      -- Build argument array for MSET command
      -- WHY: MSET is more efficient than multiple SET calls;
      --      it restores all balances in a single Redis command
      local msetArgs = {}
      -- Iterate through all balances that need restoration
      -- WHY: Each key-value pair represents a balance that was modified
      --      and needs to be reverted
      for key, value in pairs(rollbackBalances) do
          -- Add key to arguments array
          -- WHY: MSET expects alternating key, value, key, value...
          table.insert(msetArgs, key)
          -- Add original value to arguments array
          -- WHY: This is the pre-modification state we're restoring to
          table.insert(msetArgs, value)
      end
      -- Execute MSET to restore all balances atomically
      -- WHY: Single command restores all balances at once;
      --      unpack() expands the table into individual arguments
      redis.call("MSET", unpack(msetArgs))

      -- Restore TTL on each key (MSET doesn't preserve TTL)
      -- WHY: MSET overwrites keys without setting expiration;
      --      we must explicitly restore the TTL to prevent indefinite caching
      for key, _ in pairs(rollbackBalances) do
          -- Set expiration on restored balance key
          -- WHY: Balance cache should expire after TTL seconds
          --      to ensure eventual consistency with PostgreSQL
          redis.call("EXPIRE", key, ttl)
      end
  end
end -- End of rollback function


-- Forward declaration for idempKey (set in main)
local idempKey = nil


--[[
    fail(code, rollbackBalances, ttl)

    PURPOSE: Atomically rolls back balance modifications AND removes the
             idempotency key when an error occurs, then returns an error.

    INPUT:
        code            - Error code string to return (e.g., "0018", "0061")
        rollbackBalances - Table mapping Redis keys to their original JSON values
        ttl              - Time-to-live for restored balance keys

    WHY: When a failure occurs AFTER acquiring the idempotency key, we must:
         1. Rollback all balance modifications (atomicity)
         2. Delete the idempotency key so RabbitMQ retry can re-attempt
         Without deleting the idempotency key, the retry would skip processing
         (thinking it already succeeded) and leave the system in an inconsistent state.
--]]
local function fail(code, rollbackBalances, ttl)
    -- Rollback all balance modifications
    rollback(rollbackBalances, ttl)
    -- Delete idempotency key so retry can re-attempt
    -- WHY: The idempotency key was acquired but the operation failed.
    --      Without deleting it, retries would see the key and skip processing,
    --      leaving the transaction in a broken state.
    if idempKey then
        redis.call("DEL", idempKey)
    end
    -- Return error to caller
    return redis.error_reply(code)
end -- End of fail function


--[[
================================================================================
                              MAIN FUNCTION
================================================================================

    This is the entry point that orchestrates the entire balance update process.
    It processes each balance operation sequentially, applying the appropriate
    arithmetic based on transaction state and operation type.

    FLOW:
        1. Initialize configuration and extract KEYS/ARGV
        2. For each balance operation:
           a. Get or create balance in Redis cache
           b. Store original state for potential rollback
           c. Calculate new balance based on operation type
           d. Validate business rules (no negative internal balances, etc.)
           e. If validation fails, rollback all changes and return error
           f. Store updated balance with new TTL
           g. Schedule balance sync if enabled
        3. Update transaction backup hash
        4. Return all balance states (pre-modification) for response
================================================================================
--]]
local function main()
    -- Cache TTL: 1 hour (3600 seconds)
    -- WHY: Balance cache provides fast reads but must eventually sync to PostgreSQL.
    --      1 hour is long enough for transaction batching but short enough to
    --      ensure reasonable consistency if sync fails.
    local ttl = 3600 -- 1 hour
    -- Number of ARGV fields per balance operation
    -- WHY: Each balance operation requires exactly 16 fields (see header comment).
    --      This constant enables correct iteration through ARGV array.
    local groupSize = 16
    -- Array to collect balance states for response
    -- WHY: The caller needs to know the pre-modification balance states
    --      for transaction logging and audit purposes
    local returnBalances = {}
    -- Map of Redis keys to their original JSON values for rollback
    -- WHY: If any operation fails, we need to restore ALL modified balances
    --      to maintain transaction atomicity
    local rollbackBalances = {}

    -- Extract Redis Hash key for transaction backups from KEYS[1]
    -- WHY: This hash stores transaction snapshots for crash recovery;
    --      format is "backup_queue:{transactions}"
    local transactionBackupQueue = KEYS[1]
    -- Extract unique transaction identifier from KEYS[2]
    -- WHY: Used as hash field name in backup queue;
    --      format is typically "org:ledger:transaction"
    local transactionKey = KEYS[2]
    -- Extract Sorted Set key for balance sync scheduling from KEYS[3]
    -- WHY: This sorted set tracks when each balance needs to be synced
    --      back to PostgreSQL; format is "schedule:{transactions}:balance-sync"
    local scheduleKey = KEYS[3]

    -- Extract idempotency key from KEYS[4]
    -- WHY: Prevents duplicate balance effects on RabbitMQ redelivery.
    --      Format: "idemp:{transactions}:{org}:{ledger}:{tx_id}"
    --      TTL: 48 hours (24h retry window + buffer)
    idempKey = KEYS[4]
    local ttlIdemp = 172800 -- 48 hours in seconds

    -- =========================================================================
    -- IDEMPOTENCY CHECK (MUST BE FIRST)
    -- =========================================================================
    -- Atomically acquire idempotency lock before ANY balance modifications.
    -- If key already exists, this transaction was already processed - return
    -- cached balances without modifying anything.
    -- WHY: RabbitMQ can redeliver messages (heartbeat timeout, network partition).
    --      Without idempotency, two workers could both apply balance updates,
    --      resulting in double-credit/debit (data corruption).
    -- =========================================================================
    local idempAcquired = redis.call("SET", idempKey, "1", "NX", "EX", ttlIdemp)
    if not idempAcquired then
        -- Transaction was already processed - return current balances from cache
        -- without applying any modifications.
        -- WHY: The balance updates already happened in a previous delivery.
        --      We return cached balances so the Go code can proceed normally
        --      (e.g., mark status as CONFIRMED) without special-casing.
        local cachedBalances = {}
        for i = 2, #ARGV, groupSize do
            local redisBalanceKey = ARGV[i]
            local alias = ARGV[i + 5]
            local currentBalance = redis.call("GET", redisBalanceKey)
            if currentBalance then
                local ok, b = pcall(cjson.decode, currentBalance)
                if ok and type(b) == "table" then
                    b.Alias = alias
                    table.insert(cachedBalances, b)
                end
            end
        end
        -- Return cached balances (may be empty if cache expired, which is fine)
        return cjson.encode(cachedBalances)
    end

    -- First argument: whether to schedule balance sync (1 = enabled, 0 = disabled)
    -- WHY: Some operations (like read-only queries) don't need sync scheduling;
    --      this flag allows caller to opt out
    local scheduleSync = tonumber(ARGV[1]) or 1

    -- Calculate pre-expire warning time: 10 minutes before TTL expires
    -- WHY: We want to sync balances to PostgreSQL BEFORE the cache expires,
    --      giving a 10-minute buffer for the sync operation to complete
    -- schedule a pre-expire warning 10 minutes before the TTL
    local warnBefore = 600 -- 10 minutes
    -- Get current Redis server time (returns [seconds, microseconds])
    -- WHY: Using Redis server time ensures consistent timing across
    --      distributed clients; avoids clock skew issues
    local timeNow = redis.call("TIME")
    -- Extract seconds component from TIME result
    -- WHY: We only need second-level precision for sync scheduling
    local nowSec = tonumber(timeNow[1])
    -- Calculate when sync should occur: TTL - warnBefore = 3600 - 600 = 3000 seconds from now
    -- WHY: This schedules sync to happen 50 minutes after creation,
    --      leaving 10 minutes before cache expiry for the sync to complete
    local dueAt = nowSec + (ttl - warnBefore)

    -- Start from index 2 since ARGV[1] is the scheduleSync flag
    -- WHY: ARGV[1] is the scheduleSync flag, actual balance data starts at ARGV[2]
    --      Loop increments by groupSize (16) to move to next balance operation
    for i = 2, #ARGV, groupSize do
        -- Extract Redis key for this balance (e.g., "balance:{org}:{ledger}:{account}:{asset}")
        -- WHY: This is where the balance JSON is stored in Redis
        local redisBalanceKey = ARGV[i]
        -- Extract pending flag: 1 if two-phase transaction, 0 if immediate
        -- WHY: Two-phase transactions use ON_HOLD/RELEASE mechanics;
        --      immediate transactions directly modify Available balance
        local isPending = tonumber(ARGV[i + 1])
        -- Extract transaction status: PENDING, APPROVED, CANCELED, or NOTED
        -- WHY: Status determines which balance fields to modify and how
        local transactionStatus = ARGV[i + 2]
        -- Extract operation type: ON_HOLD, RELEASE, DEBIT, or CREDIT
        -- WHY: Combined with status, this determines the exact arithmetic to apply
        local operation = ARGV[i + 3]

        -- Extract transaction amount as decimal string
        -- WHY: This is the value to add/subtract from balance fields;
        --      stored as string to preserve precision for decimal arithmetic
        local amount = ARGV[i + 4]

        -- Extract account alias for display purposes
        -- WHY: Human-readable identifier stored with balance for UI display
        local alias = ARGV[i + 5]

        -- Construct balance object from ARGV fields
        -- WHY: These fields come from PostgreSQL and represent the authoritative
        --      account configuration; cache may have stale config data
        local balance = {
            -- Balance record UUID from PostgreSQL
            -- WHY: Primary key for syncing back to database
            ID = ARGV[i + 6],
            -- Available balance from PostgreSQL (initial value for new cache entries)
            -- WHY: Starting point for balance calculations; may be overwritten
            --      by cached value if balance already exists in Redis
            Available = ARGV[i + 7],
            -- OnHold balance from PostgreSQL (initial value for new cache entries)
            -- WHY: Funds locked by pending transactions; updated during two-phase flow
            OnHold = ARGV[i + 8],
            -- Optimistic locking version number
            -- WHY: Incremented on each modification; used to detect concurrent updates
            --      during PostgreSQL sync
            Version = tonumber(ARGV[i + 9]) or 0,
            -- Account type: "internal" or "external"
            -- WHY: Internal accounts cannot go negative; external accounts (off-ledger)
            --      can have negative balances and cannot use ON_HOLD operations
            AccountType = ARGV[i + 10],
            -- Permission flag: 1 if account can send funds
            -- WHY: Business rule enforcement - some accounts are receive-only
            AllowSending = tonumber(ARGV[i + 11]),
            -- Permission flag: 1 if account can receive funds
            -- WHY: Business rule enforcement - some accounts are send-only
            AllowReceiving = tonumber(ARGV[i + 12]),
            -- Asset/currency code (e.g., "USD", "BRL", "BTC")
            -- WHY: Multi-currency support; each balance is for a specific asset
            AssetCode = ARGV[i + 13],
            -- Parent account UUID from PostgreSQL
            -- WHY: Links balance to its account for relationship queries
            AccountID = ARGV[i + 14],
            -- Composite key for balance lookup
            -- WHY: Alternative lookup key format used in some queries
            Key = ARGV[i + 15],
        }

        -- Encode balance object to JSON for Redis storage
        -- WHY: Redis stores strings; we need JSON format for the SET command
        local redisBalance = cjson.encode(balance)
        -- Attempt to create balance in cache with NX (only if not exists)
        -- WHY: NX flag ensures we don't overwrite existing cached balance;
        --      if balance exists, we need to use the cached values instead
        local ok = redis.call("SET", redisBalanceKey, redisBalance, "EX", ttl, "NX")
        -- Check if SET NX failed (balance already exists in cache)
        -- WHY: When ok is nil, the key already exists and we must read the cached version
        if not ok then
            -- Balance exists in cache - get current volatile fields only
            -- WHY: Cached balance has the live Available/OnHold values from
            --      previous transactions; ARGV values might be stale
            local currentBalance = redis.call("GET", redisBalanceKey)
            -- Verify we got data back (should always succeed after failed SET NX)
            -- WHY: If GET returns nil after SET NX failed, cache is corrupted;
            --      this should never happen but we check defensively
            if not currentBalance then
                -- Return error code 0061: Balance cache corruption detected
                -- WHY: This indicates a serious data integrity issue;
                --      the balance existed (SET NX failed) but GET returned nil
                return fail("0061", rollbackBalances, ttl)
            end
            -- Decode the cached balance JSON to Lua table
            -- WHY: We need to extract the volatile fields from cached data
            local cachedBalance = cjson.decode(currentBalance)

            -- Preserve volatile fields from cache (these change with transactions)
            -- WHY: Available and OnHold are modified by transactions and must
            --      come from cache to reflect the current state
            balance.Available = cachedBalance.Available
            -- Copy OnHold from cache
            -- WHY: OnHold is volatile - tracks funds locked by pending transactions
            balance.OnHold = cachedBalance.OnHold
            -- Copy version from cache (with fallback to 0)
            -- WHY: Version number tracks modifications; must be continuous
            balance.Version = cachedBalance.Version or 0

            -- Keep identity fields from ARGV (source of truth from PostgreSQL):
            -- ID, AccountID, AccountType, AllowSending, AllowReceiving, AssetCode, Key
            -- These were already set from ARGV at lines 248-259
            -- WHY: Identity/config fields may have changed in PostgreSQL;
            --      ARGV always has the latest values from the source of truth
        end

        -- Normalize and validate decimal inputs to avoid corruption
        -- WHY: Prevents scientific notation or malformed strings from
        --      breaking arithmetic or silently producing wrong values
        balance.Available = normalize_decimal_str(balance.Available)
        balance.OnHold = normalize_decimal_str(balance.OnHold)
        amount = normalize_decimal_str(amount)

        if not is_decimal_string(balance.Available) or not is_decimal_string(balance.OnHold) or not is_decimal_string(amount) then
            return fail("0061", rollbackBalances, ttl)
        end

        -- Store original balance state for rollback (only if not already stored)
        -- WHY: If this balance key was already processed in this transaction,
        --      we keep the FIRST original state (not intermediate states)
        if not rollbackBalances[redisBalanceKey] then
            -- Encode current balance state before any modifications
            -- WHY: This is our "checkpoint" for rollback if errors occur later
            rollbackBalances[redisBalanceKey] = cjson.encode(balance)
        end

        -- Initialize result variables with current balance values
        -- WHY: These will be modified based on operation type;
        --      starting with current values handles the "no change" case
        local result = balance.Available
        -- Initialize OnHold result with current value
        -- WHY: OnHold may or may not be modified depending on operation type
        local resultOnHold = balance.OnHold
        -- Flag to track if this is a "from" account (sending funds)
        -- WHY: Used later to enforce ON_HOLD restriction on external accounts
        local isFrom = false

        -- Process based on whether this is a two-phase (pending) transaction
        -- WHY: Two-phase transactions use ON_HOLD/RELEASE mechanics;
        --      immediate transactions directly modify Available
        if isPending == 1 then
            -- Two-phase transaction: check status and operation combination

            -- CASE: PENDING + ON_HOLD - Lock funds for pending transaction
            -- WHY: When a transaction is created in PENDING state, funds are
            --      moved from Available to OnHold to "reserve" them
            if operation == "ON_HOLD" and transactionStatus == "PENDING" then
                -- Decrease Available by amount (funds being locked)
                -- WHY: These funds are no longer available for other transactions
                result = sub_decimal(balance.Available, amount)
                -- Increase OnHold by amount (funds now reserved)
                -- WHY: Track how much is locked for pending transactions
                resultOnHold = add_decimal(balance.OnHold, amount)
                -- Mark as "from" account for external account check
                -- WHY: ON_HOLD operation only happens on source accounts
                isFrom = true
            -- CASE: CANCELED + RELEASE - Unlock funds from canceled transaction
            -- WHY: When a pending transaction is canceled, release the held funds
            --      back to Available balance
            elseif operation == "RELEASE" and transactionStatus == "CANCELED" then
                -- Decrease OnHold by amount (funds being released)
                -- WHY: These funds are no longer reserved
                resultOnHold = sub_decimal(balance.OnHold, amount)
                -- Increase Available by amount (funds now available again)
                -- WHY: Released funds return to available balance
                result = add_decimal(balance.Available, amount)
                -- Mark as "from" account for external account check
                -- WHY: RELEASE operation only happens on source accounts
                isFrom = true
            -- CASE: APPROVED - Finalize the transaction
            -- WHY: When pending transaction is approved, finalize the fund movement
            elseif transactionStatus == "APPROVED" then
                -- Check if this is a DEBIT (source account)
                -- WHY: Source account needs OnHold decreased (funds leave the hold)
                if operation == "DEBIT" then
                    -- Decrease OnHold by amount (completing the transfer out)
                    -- WHY: Funds were in OnHold, now they're actually sent
                    resultOnHold = sub_decimal(balance.OnHold, amount)
                    -- Mark as "from" account for external account check
                    -- WHY: DEBIT operation marks this as source account
                    isFrom = true
                else
                    -- CREDIT (destination account): increase Available
                    -- WHY: Destination receives funds directly to Available
                    --      (destination doesn't use OnHold in two-phase)
                    result = add_decimal(balance.Available, amount)
                end
            end
        else
            -- Immediate transaction (not two-phase): direct balance modification
            -- WHY: Simple transactions don't use OnHold; they directly
            --      increase or decrease Available balance
            if operation == "DEBIT" then
                -- Decrease Available by amount (sending funds)
                -- WHY: Direct debit reduces available balance immediately
                result = sub_decimal(balance.Available, amount)
            else
                -- CREDIT: Increase Available by amount (receiving funds)
                -- WHY: Direct credit increases available balance immediately
                result = add_decimal(balance.Available, amount)
            end
        end

        -- BUSINESS RULE: External accounts cannot use ON_HOLD operations
        -- WHY: External accounts represent off-ledger entities (banks, exchanges);
        --      they don't have a concept of "held" funds - transactions are
        --      either done or not done from their perspective
        if isPending == 1 and isFrom and balance.AccountType == "external" then
            -- Return error code 0098: External accounts cannot use ON_HOLD
            -- WHY: Clear error code for API consumers to handle appropriately.
            --      Atomicity requirement - if this rule is violated,
            --      we must undo all balance modifications AND delete idempotency key.
            return fail("0098", rollbackBalances, ttl)
        end

        -- BUSINESS RULE: Internal accounts cannot have negative Available balance
        -- WHY: Internal accounts represent real funds on the ledger;
        --      negative balance would mean "money from nowhere" which
        --      violates double-entry accounting principles
        if startsWithMinus(result) and balance.AccountType ~= "external" then
            -- Return error code 0018: Insufficient funds (negative Available balance)
            -- WHY: Standard error code for insufficient funds condition.
            --      Atomicity requirement - insufficient funds means
            --      the entire transaction must be rejected AND idempotency key deleted.
            return fail("0018", rollbackBalances, ttl)
        end

        -- BUSINESS RULE: OnHold balance cannot go negative
        -- WHY: OnHold represents funds locked by pending transactions. If OnHold goes
        --      negative, it means more funds are being released than were ever held,
        --      indicating a data integrity issue (e.g., duplicate RELEASE, race condition,
        --      or mismatch between ON_HOLD and RELEASE amounts).
        if startsWithMinus(resultOnHold) then
            -- Return error code 0130: OnHold insufficient (negative OnHold)
            -- WHY: Distinct from 0018 to differentiate between Available vs OnHold issues.
            --      Atomicity requirement - OnHold inconsistency indicates
            --      a serious data integrity problem that must be rejected AND idempotency key deleted.
            return fail("0130", rollbackBalances, ttl)
        end

        -- Clear any lowercase alias field that might exist from cache
        -- WHY: Older cached balances might have lowercase "alias" field;
        --      we standardize on "Alias" (capitalized) for consistency
        balance.alias = nil  -- Clear lowercase version if it exists from cache
        -- Set the alias from the current operation's ARGV
        -- WHY: Alias might have changed in PostgreSQL; ARGV has latest value
        balance.Alias = alias
        -- Add a CLONE of current balance state to return array
        -- WHY: We return pre-modification states; cloneBalance ensures
        --      subsequent modifications don't affect the returned data
        table.insert(returnBalances, cloneBalance(balance))

        -- Apply calculated result to balance object
        -- WHY: Update the balance with new Available value after arithmetic
        balance.Available = result
        -- Apply calculated OnHold result
        -- WHY: Update OnHold (may or may not have changed based on operation)
        balance.OnHold = resultOnHold
        -- Increment version number for optimistic locking
        -- WHY: Each modification increments version; PostgreSQL sync uses this
        --      to detect concurrent modifications and prevent data loss
        balance.Version = (balance.Version or 0) + 1

        -- Encode updated balance to JSON for storage
        -- WHY: Prepare for Redis SET command
        redisBalance = cjson.encode(balance)
        -- Store updated balance in Redis with TTL
        -- WHY: This is the actual balance update; EX sets expiration time
        --      to ensure cache doesn't persist indefinitely
        redis.call("SET", redisBalanceKey, redisBalance, "EX", ttl)

        -- Only schedule balance sync if enabled (scheduleSync == 1)
        -- WHY: Allows caller to skip sync scheduling for certain operations
        --      (e.g., read-only queries or when sync is handled elsewhere)
        if scheduleSync == 1 then
            -- Add balance key to sorted set with due time as score
            -- WHY: ZADD puts the key in a sorted set ordered by dueAt timestamp;
            --      a background worker polls this set to find balances needing sync
            redis.call("ZADD", scheduleKey, dueAt, redisBalanceKey)
        end
    end -- End of balance operations loop

    -- Store transaction backup with all pre-modification balance states
    -- WHY: This creates a recovery point; if system crashes between Redis
    --      update and PostgreSQL sync, we can reconstruct the transaction
    updateTransactionHash(transactionBackupQueue, transactionKey, returnBalances)

    -- Encode return balances array to JSON
    -- WHY: Redis Lua scripts return strings; caller expects JSON array
    local returnBalancesEncoded = cjson.encode(returnBalances)
    -- Return the pre-modification balance states
    -- WHY: Caller needs these for transaction logging, audit trails,
    --      and to confirm what balances looked like before the transaction
    return returnBalancesEncoded
end -- End of main function


-- Execute main function and return result
-- WHY: This is the script entry point; Redis EVAL runs this line
--      and returns whatever main() returns to the caller
return main()
