--------------------------------------------------------------------------------
-- GET BALANCES NEAR EXPIRATION (Pre-Expiry Sync Scheduler Polling Script)
--------------------------------------------------------------------------------
-- PURPOSE:
--   This script retrieves balance keys that are due for synchronization with
--   the database and atomically claims them using distributed locks. It serves
--   as the polling mechanism of a pre-expiry sync scheduler.
--
-- DATA FLOW:
--   1. add_sub.lua schedules a balance with ZADD (score = dueAt timestamp)
--   2. This script polls for due members and claims them with distributed locks
--   3. Go worker processes claimed members (syncs balance state to database)
--   4. unschedule_synced_balance.lua removes the member and releases the lock
--
-- ATOMICITY:
--   This entire script executes atomically in Redis (single-threaded execution).
--   No other commands can interleave, ensuring the query-then-claim operation
--   is race-condition free within this script's scope.
--
-- INPUTS:
--   KEYS[1] - The Redis sorted set key for balance sync schedule
--             (e.g., "schedule:{transactions}:balance-sync")
--   ARGV[1] - Maximum number of balances to claim per invocation (default: 25)
--   ARGV[2] - TTL for claim locks in seconds (default: 600)
--   ARGV[3] - Prefix for lock keys (default: "lock:{transactions}:balance-sync:")
--
-- OUTPUTS:
--   Returns an array of balance keys that were successfully claimed by this worker
--------------------------------------------------------------------------------

-- WHAT: Extracts the Redis sorted set key name for balance sync schedule from KEYS array
-- WHY:  Parameter from Go code. Value is "schedule:{transactions}:balance-sync".
--       Using KEYS[] (not hardcoded) ensures Redis Cluster compatibility, as Redis
--       Cluster needs to know all keys accessed by a script to route it correctly.
local scheduleKey = KEYS[1]

-- WHAT: Defensive nil check - returns empty table if no key was provided
-- WHY:  Safety guard to prevent nil dereference errors. Allows graceful degradation
--       if the caller forgets to pass the required key. An empty array signals
--       "nothing to process" to the Go code without causing a Redis error.
if not scheduleKey then
  return {}
end

-- WHAT: Calls Redis TIME command which returns [seconds, microseconds] since Unix epoch
-- WHY:  Using Redis server time (not client time) ensures ALL workers across different
--       machines see the exact same "now" value. This prevents clock skew race conditions
--       where one worker's clock is ahead/behind others, which could cause duplicate
--       processing or missed balances.
local t = redis.call('TIME')

-- WHAT: Converts the seconds portion (t[1]) from string to number
-- WHY:  Redis returns strings; we need a number for the ZRANGEBYSCORE comparison.
--       We only use seconds precision (not microseconds) because our scheduling
--       granularity is in seconds, and sub-second precision isn't needed.
local now = tonumber(t[1])

-- WHAT: Extracts the batch size limit from ARGV[1], defaulting to 25 if not provided
-- WHY:  Limits how many balances a single worker can claim at once. Default of 25
--       prevents a single worker from hogging too much work, ensures fair distribution
--       across workers, and keeps individual processing batches manageable. This also
--       bounds memory usage and processing time per poll cycle.
local limit = tonumber(ARGV[1]) or 25

-- WHAT: Extracts the claim lock TTL from ARGV[2], defaulting to 600 seconds (10 min)
-- WHY:  The TTL should match or exceed the sync warning window. 600s provides ample
--       time for the worker to complete the database sync operation. If a worker dies
--       mid-processing, the lock auto-expires after this time, allowing another worker
--       to retry. Too short = premature unlock and duplicate work; too long = stuck
--       balances if worker crashes.
local claimTTL = tonumber(ARGV[2]) or 600

-- WHAT: Extracts the lock key prefix from ARGV[3] with a sensible default
-- WHY:  Lock keys are formed as prefix + member. The default follows the project's
--       key naming convention: "lock:{transactions}:balance-sync:". Using a prefix
--       allows easy identification/cleanup of lock keys and keeps them namespaced
--       away from other Redis data.
local claimPrefix = ARGV[3] or "lock:{transactions}:balance-sync:"

-- WHAT: Queries the sorted set for all members with scores between -infinity and 'now',
--       returning at most 'limit' results starting from offset 0
-- WHY:  The sorted set stores balances with their due-at timestamp as the score.
--       ZRANGEBYSCORE finds all balances whose scheduled sync time has arrived or passed.
--       '-inf' ensures we catch any balance scheduled in the past (including those
--       that might have been missed). The LIMIT clause prevents returning an unbounded
--       result set which could overwhelm memory or processing capacity.
local due = redis.call('ZRANGEBYSCORE', scheduleKey, '-inf', now, 'LIMIT', 0, limit)

-- WHAT: Initializes an empty table to collect successfully claimed balance keys
-- WHY:  We only return balances where we successfully acquired the distributed lock.
--       This table accumulates the "winners" as we iterate through the due list.
local claimed = {}

-- WHAT: Iterates through each due balance member using a numeric for loop
-- WHY:  Lua tables are 1-indexed. We process each due balance to attempt claiming it.
--       Using a for loop with #due handles empty arrays gracefully (loop body never runs).
for i = 1, #due do
  -- WHAT: Extracts the current balance member (key) from the due array
  -- WHY:  Each member is a balance identifier that needs to be processed.
  --       We need the raw member value to both create its lock key and to return it
  --       if successfully claimed.
  local member = due[i]

  -- WHAT: Constructs the distributed lock key by concatenating prefix with member
  -- WHY:  Each balance gets its own unique lock key. This allows fine-grained locking
  --       where multiple workers can process different balances concurrently, but only
  --       one worker can process any specific balance at a time.
  local lockKey = claimPrefix .. member

  -- WHAT: Attempts to acquire a distributed lock using SET with NX and EX options
  -- WHY:  SET NX EX is an atomic mutex operation:
  --         - NX (Not eXists): Only sets if the key doesn't already exist
  --         - EX claimTTL: Auto-expires after claimTTL seconds
  --       This is the standard Redis distributed lock pattern. If another worker already
  --       claimed this balance, NX causes SET to return nil (failure). The '1' value
  --       is arbitrary - we only care about key existence, not its value.
  --       EX ensures automatic cleanup if the worker crashes or takes too long.
  local ok = redis.call('SET', lockKey, '1', 'NX', 'EX', claimTTL)

  -- WHAT: Checks if the SET command succeeded (returned "OK", which is truthy)
  -- WHY:  SET NX returns "OK" on success (we got the lock) or nil on failure
  --       (someone else has the lock). We only want to return balances where WE
  --       won the lock race, not balances that are already being processed.
  if ok then
    -- WHAT: Appends the successfully-claimed member to the claimed array
    -- WHY:  table.insert is the idiomatic Lua way to append to an array.
    --       We build up the list of balances this worker is now responsible for.
    table.insert(claimed, member)
  end
  -- NOTE: If ok is nil/false, we silently skip this member. It's not an error -
  -- it just means another worker claimed it first, which is expected behavior
  -- in a distributed system with multiple competing workers.
end

-- WHAT: Returns the array of successfully claimed balance keys to the caller
-- WHY:  The Go code will receive this list and process each balance (sync to database).
--       Returning only claimed balances ensures the caller only works on balances
--       it exclusively owns, preventing duplicate processing across workers.
--       An empty array is valid and means "nothing new to process right now".
return claimed
