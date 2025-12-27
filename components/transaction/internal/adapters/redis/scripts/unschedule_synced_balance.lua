--------------------------------------------------------------------------------
-- UNSCHEDULE SYNCED BALANCE
--------------------------------------------------------------------------------
-- Purpose: Remove a balance sync entry from Redis after the sync operation
--          completes. This script acts as the cleanup phase in the balance
--          synchronization workflow.
--
-- Workflow Context:
--   1. Worker calls GetBalanceSyncKeys -> get_balances_near_expiration.lua
--      claims balances by adding them to the schedule ZSET with lock keys
--   2. Worker processes each claimed balance (syncs to database)
--   3. Worker calls RemoveBalanceSyncKey -> THIS SCRIPT cleans up
--   4. Called regardless of sync success/failure - cleanup must always happen
--
-- Philosophy: Fail open, not closed. Schedule removal is guaranteed.
--             Lock cleanup is best-effort because locks auto-expire (600s TTL).
--
-- INPUTS:
--   KEYS[1]: Schedule sorted set key
--            Go caller: utils.BalanceSyncScheduleKey ("schedule:{transactions}:balance-sync")
--   ARGV[1]: Balance key to remove
--            Format: "balance:{transactions}:orgID:ledgerID:key"
--   ARGV[2]: Optional lock prefix
--            Go caller: utils.BalanceSyncLockPrefix, Lua fallback: "lock:{transactions}:balance-sync:"
--
-- OUTPUTS:
--   Returns 1 if member was removed from schedule, 0 if it didn't exist
--
-- DEPENDENCIES:
--   This script uses standard Redis commands only (ZREM, DEL).
--   No external modules required.
--
-- GO CROSS-REFERENCES:
--   - Called from: consumer.redis.go:RemoveBalanceSyncKey()
--   - Constants: pkg/utils/cache.go (BalanceSyncScheduleKey, BalanceSyncLockPrefix)
--   - Worker: bootstrap/balance.worker.go (syncAndRecordBalance)
--------------------------------------------------------------------------------

-- WHAT: Assigns the first key argument to scheduleKey variable
-- WHY:  Called from Go with utils.BalanceSyncScheduleKey = "schedule:{transactions}:balance-sync"
--       This ZSET tracks all balances that need synchronization to the database
local scheduleKey = KEYS[1]

-- WHAT: Assigns the first argument (balance key) to member variable
-- WHY:  This is the specific balance to unschedule, format is
--       "balance:{transactions}:organizationID:ledgerID:key"
--       The member was previously added to the schedule ZSET when it needed syncing
local member = ARGV[1]

-- WHAT: Assigns lock key prefix, using default if not provided
-- WHY:  Defensive programming - makes script flexible if caller wants a different
--       lock pattern. The default matches the pattern used by get_balances_near_expiration.lua
--       which creates locks when claiming balances for processing
local claimPrefix = ARGV[2] or "lock:{transactions}:balance-sync:"

-- WHAT: Validates that required inputs are present and non-empty
-- WHY:  Safety guard - prevents nil/empty key operations that could:
--       1. Corrupt data by operating on wrong keys
--       2. Cause silent failures that are hard to debug
--       3. Leave orphaned schedule entries or locks
--       Early failure with clear error message aids troubleshooting
--       Note: In Lua, only nil and false are falsy; empty string "" is truthy,
--       so we must explicitly check for empty strings.
if not scheduleKey or scheduleKey == "" or not member or member == "" then
  return redis.error_reply("invalid scheduleKey or member")
end

-- WHAT: Removes the member (balance key) from the schedule sorted set (ZSET)
-- WHY:  This is the PRIMARY operation - signals "this balance no longer needs syncing"
--       ZREM returns 1 if member was removed, 0 if it didn't exist
--       The return value enables observability - Go caller can log whether
--       the member actually existed (useful for detecting race conditions or bugs)
local removed = redis.call('ZREM', scheduleKey, member)

-- WHAT: Constructs the lock key by concatenating prefix with member
-- WHY:  The lock was created by get_balances_near_expiration.lua when this balance
--       was claimed for processing. The lock prevents other workers from
--       processing the same balance concurrently
local lockKey = claimPrefix .. member

-- WHAT: Deletes the lock key (best-effort, return value intentionally ignored)
-- WHY:  Lock cleanup is a "nice-to-have" secondary operation because:
--       1. Lock may not exist - another worker could have claimed it, or it expired
--       2. Lock has 600s TTL auto-expiry anyway, so it will self-clean
--       3. Ignoring the return value prevents cascade failures - we guarantee
--          the schedule removal (the critical operation) completes even if
--          lock cleanup has issues
--       This follows the "fail open" philosophy - schedule state is authoritative,
--       locks are just coordination hints
redis.call('DEL', lockKey)

-- WHAT: Returns the result of ZREM (0 or 1)
-- WHY:  Enables observability in the Go layer:
--       - 1 means member was successfully removed (normal case)
--       - 0 means member didn't exist (may indicate race condition, duplicate
--         call, or timing issue - worth logging for debugging)
--       This transparency helps operators understand system behavior
return removed
