--[[
================================================================================
Midaz Redis Transaction Authorization Script (add_sub.lua)
================================================================================

PURPOSE:
This script is the CORE of Midaz's transaction processing engine. It atomically
applies balance mutations for transaction authorization and settlement across
multiple balances, ensuring consistency and preventing race conditions.

TRANSACTION FLOW DIAGRAM:
┌───────────────────────────────────────────────────────────────────────┐
│ Transaction States & Operations                                       │
├───────────────────────────────────────────────────────────────────────┤
│ PENDING Flow (isPending=1):                                          │
│   ├─ PENDING + ON_HOLD:    Available → OnHold (lock funds)          │
│   ├─ CANCELED + RELEASE:   OnHold → Available (unlock funds)        │
│   └─ APPROVED:                                                       │
│       ├─ DEBIT:  OnHold ↓ (consume locked funds)                    │
│       └─ CREDIT: Available ↑ (receive funds)                        │
│                                                                       │
│ DIRECT Flow (isPending=0):                                           │
│   ├─ DEBIT:  Available ↓ (immediate deduction)                      │
│   └─ CREDIT: Available ↑ (immediate addition)                       │
└───────────────────────────────────────────────────────────────────────┘

REDIS KEYS:
┌─────┬──────────────────────────────────────────────────────────────┐
│ Key │ Description                                                  │
├─────┼──────────────────────────────────────────────────────────────┤
│ [1] │ Transaction backup hash (e.g., "backup_queue:{transactions}")│
│ [2] │ Unique transaction key within the backup hash               │
└─────┴──────────────────────────────────────────────────────────────┘

ARGUMENTS (15 fields per balance):
┌────┬──────────────────┬─────────┬──────────────────────────────────────┐
│ #  │ Field            │ Type    │ Description                          │
├────┼──────────────────┼─────────┼──────────────────────────────────────┤
│ 1  │ redisBalanceKey  │ string  │ Internal Redis key for this balance │
│ 2  │ isPending        │ number  │ 1 if pending flow, 0 for direct     │
│ 3  │ transactionStatus│ string  │ "PENDING"|"APPROVED"|"CANCELED"     │
│ 4  │ operation        │ string  │ "DEBIT"|"CREDIT"|"ON_HOLD"|"RELEASE"│
│ 5  │ amount           │ string  │ Decimal amount (e.g., "100.50")     │
│ 6  │ alias            │ string  │ Balance alias (e.g., "@account1")   │
│ 7  │ ID               │ string  │ Balance UUID                         │
│ 8  │ Available        │ string  │ Current available amount (decimal)  │
│ 9  │ OnHold           │ string  │ Current on-hold amount (decimal)    │
│ 10 │ Version          │ number  │ Optimistic locking version          │
│ 11 │ AccountType      │ string  │ "internal" or "external"            │
│ 12 │ AllowSending     │ number  │ 1 or 0 (not enforced here)          │
│ 13 │ AllowReceiving   │ number  │ 1 or 0 (not enforced here)          │
│ 14 │ AssetCode        │ string  │ Currency/asset code (e.g., "USD")   │
│ 15 │ AccountID        │ string  │ Parent account UUID                  │
└────┴──────────────────┴─────────┴──────────────────────────────────────┘

KEY BEHAVIORS:
1. ATOMICITY: All balance updates succeed together or all fail with rollback
2. DECIMAL PRECISION: String-based math prevents floating-point errors
3. OPTIMISTIC LOCKING: Version field prevents concurrent modifications
4. TTL MANAGEMENT: All keys expire after 3600 seconds (1 hour)
5. ROLLBACK: On validation failure, all changes are atomically reverted

VALIDATION RULES & ERROR CODES:
┌──────┬────────────────────────────────────────────────────────────┐
│ Code │ Description                                                │
├──────┼────────────────────────────────────────────────────────────┤
│ 0061 │ Balance key unexpectedly missing (race condition)         │
│ 0098 │ External accounts cannot be "from" side in pending flows  │
│ 0018 │ Non-external accounts cannot have negative Available      │
└──────┴────────────────────────────────────────────────────────────┘

RETURN VALUE:
JSON array of balance snapshots captured BEFORE mutations are applied.
These pre-update snapshots are used for audit trails and diagnostics.
If post-update values are needed, callers must read from Redis separately.
]]

-- ================================================================================
-- STEP 1: DECIMAL ARITHMETIC HELPERS
-- ================================================================================
-- These functions implement precise decimal arithmetic using string manipulation
-- to avoid floating-point errors that could corrupt financial calculations.
-- All amounts in Midaz are processed as decimal strings to maintain accuracy.
-- ================================================================================

-- Phase 1.1: Parse decimal string into components
-- ────────────────────────────────────────────────────────────────────────────────
-- Splits a decimal string into its integer and fractional parts, tracking sign.
-- Examples:
--   "123.45"  → integer="123", fraction="45", is_negative=false
--   "-50.7"   → integer="-50", fraction="7", is_negative=true
--   "100"     → integer="100", fraction="", is_negative=false
-- ────────────────────────────────────────────────────────────────────────────────
local function split_decimal(s)
    local sign = ""

    -- Step 1.1.1: Extract sign character if present
    if s:sub(1, 1) == "-" then
        sign = "-"
        s = s:sub(2)  -- Remove sign for further processing
    end

    -- Step 1.1.2: Attempt to match decimal pattern (digits.digits)
    local intp, fracp = s:match("^(%d+)%.(%d+)$")
    if intp then
        -- Has decimal point: return components separately
        return sign .. intp, fracp, sign ~= ""
    else
        -- No decimal point: treat as integer with empty fraction
        return sign .. s, "", sign ~= ""
    end
end

-- Step 1.1: fractional normalization helper
-- Trim trailing zeros from fractional part; return "0" when empty
local function rtrim_zeros(frac)
    frac = frac:gsub("0+$", "")
    return (frac == "" and "0") or frac
end

-- Forward declaration for subtract helper to allow mutual recursion
local sub_decimal

-- Add two decimal strings with sign support; returns a canonical decimal string
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

-- Subtract decimal string b from a with sign support
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

-- Step 2: General-purpose helpers ------------------------------------------------------------
-- Utility: test negative sign without numeric conversion
local function startsWithMinus(s)
    return s:sub(1, 1) == "-"
end

-- Shallow copy balance table for safe return/backup snapshot
local function cloneBalance(tbl)
    local copy = {}
    for k, v in pairs(tbl) do
        copy[k] = v
    end
    return copy
end

-- Step 3: Backup & rollback helpers ----------------------------------------------------------
-- Update per-transaction backup payload under KEYS[1] hash
local function updateTransactionHash(transactionBackupQueue, transactionKey, balances)
    local transaction

    local raw = redis.call("HGET", transactionBackupQueue, transactionKey)
    -- 3.1 Try to read the existing transaction payload; if not present, start fresh
    if not raw then
        transaction = { balances = balances }
    else
        local ok, decoded = pcall(cjson.decode, raw)
        -- 3.2 If the payload is valid JSON object, replace balances field; otherwise, reinitialize
        if ok and type(decoded) == "table" then
            transaction = decoded
            transaction.balances = balances
        else
            transaction = { balances = balances }
        end
    end

    -- 3.3 Persist the upserted payload back to the backup hash
    local updated = cjson.encode(transaction)
    redis.call("HSET", transactionBackupQueue, transactionKey, updated)

    return updated
end

-- Roll back all touched keys to their original JSON values and restore TTL
local function rollback(rollbackBalances, ttl)
  if next(rollbackBalances) then
      -- 3.4 Build MSET arguments to atomically restore all keys
      local msetArgs = {}
      for key, value in pairs(rollbackBalances) do
          table.insert(msetArgs, key)
          table.insert(msetArgs, value)
      end
      redis.call("MSET", unpack(msetArgs))

      -- 3.5 Re-apply TTL to each restored key
      for key, _ in pairs(rollbackBalances) do
          redis.call("EXPIRE", key, ttl)
      end
  end
end

-- Step 4: Main pipeline ----------------------------------------------------------------------
-- KEYS:
--   KEYS[1] = transaction backup hash key (e.g., "backup_queue:{transactions}")
--   KEYS[2] = transaction key within the backup hash
-- ARGV groups (15 fields per balance) — see header for mapping.
local function main()
    -- TTL (seconds) applied to per-balance keys written by this script
    local ttl = 3600
    -- Number of ARGV fields per balance; MUST stay in sync with producer (Go)
    local groupSize = 15
    -- 4.0 Initialize accumulators
    local returnBalances = {}
    local rollbackBalances = {}

    local transactionBackupQueue = KEYS[1]
    local transactionKey = KEYS[2]
    
    -- ────────────────────────────────────────────────────────────────────────────
    -- Phase 4.2: MAIN PROCESSING LOOP - Process each balance in the transaction
    -- ────────────────────────────────────────────────────────────────────────────
    -- Each iteration processes one balance mutation (either debit or credit side).
    -- For a typical transfer, there are two iterations: one for source, one for dest.
    -- Complex transactions may involve many balances (e.g., multi-party splits).
    -- ────────────────────────────────────────────────────────────────────────────
    
    for i = 1, #ARGV, groupSize do
        -- ┌─────────────────────────────────────────────────────────────────────┐
        -- │ Step 4.2.1: EXTRACT CONTROL PARAMETERS                             │
        -- │ These determine how this specific balance will be modified         │
        -- └─────────────────────────────────────────────────────────────────────┘
        
        local redisBalanceKey = ARGV[i]          -- Redis key where balance is stored
        local isPending = tonumber(ARGV[i + 1])  -- 1=two-phase commit, 0=immediate
        local transactionStatus = ARGV[i + 2]    -- Current transaction state
        local operation = ARGV[i + 3]            -- What to do with this balance

        -- ┌─────────────────────────────────────────────────────────────────────┐
        -- │ Step 4.2.2: EXTRACT TRANSACTION AMOUNT                             │
        -- │ The decimal string representing how much to move                   │
        -- └─────────────────────────────────────────────────────────────────────┘
        
        local amount = ARGV[i + 4]               -- e.g., "100.50" for $100.50

        -- ┌─────────────────────────────────────────────────────────────────────┐
        -- │ Step 4.2.3: EXTRACT BALANCE IDENTIFIER                             │
        -- │ Used for correlation and debugging                                 │
        -- └─────────────────────────────────────────────────────────────────────┘
        
        local alias = ARGV[i + 5]                -- Human-readable reference

        -- ┌─────────────────────────────────────────────────────────────────────┐
        -- │ Step 4.2.4: CONSTRUCT INITIAL BALANCE OBJECT                       │
        -- │ This represents either the expected state (new balance) or will    │
        -- │ be replaced with the actual state from Redis (existing balance)    │
        -- └─────────────────────────────────────────────────────────────────────┘
        
        local balance = {
            ID = ARGV[i + 6],                    -- Unique balance identifier
            Available = ARGV[i + 7],             -- Funds available for use
            OnHold = ARGV[i + 8],                -- Funds locked pending approval
            Version = tonumber(ARGV[i + 9]),     -- Concurrency control counter
            AccountType = ARGV[i + 10],          -- "internal" or "external"
            AllowSending = tonumber(ARGV[i + 11]),    -- Permission flag (future use)
            AllowReceiving = tonumber(ARGV[i + 12]),  -- Permission flag (future use)
            AssetCode = ARGV[i + 13],            -- Currency or asset type
            AccountID = ARGV[i + 14],            -- Parent account reference
        }

        local redisBalance = cjson.encode(balance)
        -- Seed balance key if it does not exist; ensure we can decode a baseline state
        local ok = redis.call("SET", redisBalanceKey, redisBalance, "EX", ttl, "NX")
        if not ok then
            -- Contention path: fetch existing state; if still missing, surface storage error "0061"
            local currentBalance = redis.call("GET", redisBalanceKey)
            if not currentBalance then
                return redis.error_reply("0061") -- ErrBalanceUpdateFailed
            end
            balance = cjson.decode(currentBalance)
        end

        -- Record original (pre-mutation) value once for potential rollback
        if not rollbackBalances[redisBalanceKey] then
            rollbackBalances[redisBalanceKey] = cjson.encode(balance)
        end

        -- Initialize result cursors and From/To discriminator
        local result = balance.Available
        local resultOnHold = balance.OnHold
        local isFrom = false -- true when this leg represents the debit/source side

        -- ┌─────────────────────────────────────────────────────────────────────┐
        -- │ Step 4.5.2: APPLY TRANSACTION LOGIC                                │
        -- │ This is the CORE business logic that determines how balances change│
        -- └─────────────────────────────────────────────────────────────────────┘
        
        if isPending == 1 then
            -- ╔═══════════════════════════════════════════════════════════════════╗
            -- ║ PENDING FLOW: Two-Phase Commit Pattern                           ║
            -- ║ Used for transactions that require authorization/approval        ║
            -- ║ Phase 1: Lock funds (PENDING)                                   ║
            -- ║ Phase 2: Either commit (APPROVED) or rollback (CANCELED)        ║
            -- ╚═══════════════════════════════════════════════════════════════════╝
            
            if operation == "ON_HOLD" and transactionStatus == "PENDING" then
                -- ┌─────────────────────────────────────────────────────────────┐
                -- │ SCENARIO: Initial Authorization Request                      │
                -- │ Action: Lock funds by moving from Available to OnHold       │
                -- │ Example: Credit card authorization at gas pump              │
                -- └─────────────────────────────────────────────────────────────┘
                result = sub_decimal(balance.Available, amount)      -- Available ↓
                resultOnHold = add_decimal(balance.OnHold, amount)  -- OnHold ↑
                isFrom = true  -- Mark as source account
                
            elseif operation == "RELEASE" and transactionStatus == "CANCELED" then
                -- ┌─────────────────────────────────────────────────────────────┐
                -- │ SCENARIO: Transaction Cancellation                          │
                -- │ Action: Release locked funds back to Available              │
                -- │ Example: Order cancelled before fulfillment                 │
                -- └─────────────────────────────────────────────────────────────┘
                resultOnHold = sub_decimal(balance.OnHold, amount)  -- OnHold ↓
                result = add_decimal(balance.Available, amount)     -- Available ↑
                isFrom = true  -- Mark as source account
                
            elseif transactionStatus == "APPROVED" then
                -- ┌─────────────────────────────────────────────────────────────┐
                -- │ SCENARIO: Transaction Settlement                            │
                -- │ Action: Finalize the transfer of funds                      │
                -- └─────────────────────────────────────────────────────────────┘
                
                if operation == "DEBIT" then
                    -- SOURCE SIDE: Consume the previously locked funds
                    -- The funds were already removed from Available during ON_HOLD
                    -- Now we just remove them from OnHold (completing the debit)
                    resultOnHold = sub_decimal(balance.OnHold, amount)  -- OnHold ↓
                    isFrom = true
                    -- Note: Available stays the same (was reduced in PENDING phase)
                else
                    -- DESTINATION SIDE: Receive the transferred funds
                    -- Add funds to Available (they were never on hold here)
                    result = add_decimal(balance.Available, amount)  -- Available ↑
                    -- Note: OnHold stays the same (destination never held these funds)
                end
            end
            -- Other combinations (e.g., PENDING+CREDIT) are no-ops by design
            
        else
            -- ╔═══════════════════════════════════════════════════════════════════╗
            -- ║ DIRECT FLOW: Immediate Balance Changes                           ║
            -- ║ Used for instant transfers without authorization phase           ║
            -- ║ Example: Internal transfers between user's own accounts          ║
            -- ╚═══════════════════════════════════════════════════════════════════╝
            
            if operation == "DEBIT" then
                -- Direct deduction from available balance
                result = sub_decimal(balance.Available, amount)  -- Available ↓
            else
                -- Direct addition to available balance
                result = add_decimal(balance.Available, amount)  -- Available ↑
            end
        end

        -- ┌─────────────────────────────────────────────────────────────────────┐
        -- │ Step 4.6: BUSINESS RULE VALIDATION                                 │
        -- │ Enforce critical constraints to maintain system integrity          │
        -- └─────────────────────────────────────────────────────────────────────┘
        
        -- Validation Rule 1: External Account Hold Restriction
        -- ─────────────────────────────────────────────────────────────────────
        -- External accounts represent bank accounts or external payment systems
        -- that we don't control. We cannot place holds on their funds.
        -- ─────────────────────────────────────────────────────────────────────
        if isPending == 1 and isFrom and balance.AccountType == "external" then
            -- VIOLATION: Attempted to place hold on external account
            rollback(rollbackBalances, ttl)  -- Revert all changes made so far
            return redis.error_reply("0098")  -- ErrOnHoldExternalAccount
        end

        -- Validation Rule 2: Overdraft Protection
        -- ─────────────────────────────────────────────────────────────────────
        -- Internal accounts cannot have negative balances. This prevents
        -- overdrafts and ensures all transactions are fully funded.
        -- External accounts CAN go negative (representing money owed to us).
        -- ─────────────────────────────────────────────────────────────────────
        if startsWithMinus(result) and balance.AccountType ~= "external" then
            -- VIOLATION: Transaction would cause negative balance
            rollback(rollbackBalances, ttl)  -- Revert all changes made so far
            return redis.error_reply("0018")  -- ErrInsufficientFunds
        end

        -- ┌─────────────────────────────────────────────────────────────────────┐
        -- │ Step 4.7: CAPTURE PRE-MUTATION SNAPSHOT                            │
        -- │ Save the balance state BEFORE applying changes for audit trail     │
        -- └─────────────────────────────────────────────────────────────────────┘
        
        balance.Alias = alias  -- Attach correlation identifier
        table.insert(returnBalances, cloneBalance(balance))  -- Deep copy for immutability

        -- ┌─────────────────────────────────────────────────────────────────────┐
        -- │ Step 4.8: COMMIT BALANCE CHANGES                                   │
        -- │ Apply the calculated mutations to the balance object               │
        -- └─────────────────────────────────────────────────────────────────────┘
        
        balance.Available = result           -- Apply new available amount
        balance.OnHold = resultOnHold       -- Apply new on-hold amount
        balance.Version = balance.Version + 1  -- Increment for optimistic locking

        -- ┌─────────────────────────────────────────────────────────────────────┐
        -- │ Step 4.9: PERSIST TO REDIS                                         │
        -- │ Write the updated balance back to Redis with TTL refresh           │
        -- └─────────────────────────────────────────────────────────────────────┘
        
        redisBalance = cjson.encode(balance)
        redis.call("SET", redisBalanceKey, redisBalance, "EX", ttl)
        
    end  -- END OF BALANCE PROCESSING LOOP

    -- ╔═════════════════════════════════════════════════════════════════════════╗
    -- ║ PHASE 5: TRANSACTION FINALIZATION                                      ║
    -- ╚═════════════════════════════════════════════════════════════════════════╝
    
    -- Step 5.1: Archive Transaction Metadata
    -- ─────────────────────────────────────────────────────────────────────────────
    -- Store a permanent record of this transaction including all affected balances.
    -- This enables audit trails, debugging, and potential recovery operations.
    -- ─────────────────────────────────────────────────────────────────────────────
    updateTransactionHash(transactionBackupQueue, transactionKey, returnBalances)

    -- Step 5.2: Prepare and Return Results
    -- ─────────────────────────────────────────────────────────────────────────────
    -- IMPORTANT: We return PRE-mutation snapshots, not the final states.
    -- This design choice provides:
    -- 1. Audit trail of what values were before the transaction
    -- 2. Ability to verify the changes by comparing with current Redis values
    -- 3. Idempotency support (can detect if transaction was already applied)
    -- ─────────────────────────────────────────────────────────────────────────────
    local returnBalancesEncoded = cjson.encode(returnBalances)
    return returnBalancesEncoded
end

return main()