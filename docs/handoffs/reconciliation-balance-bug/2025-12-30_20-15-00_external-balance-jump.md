# Handoff: External Balance Jump Causing Reconciliation Discrepancy

## Metadata
- **Created**: 2025-12-30T20:15:00Z
- **Branch**: fix/fred-several-ones-dec-13-2025
- **Status**: ROOT CAUSE FOUND - fix pending

---

## Summary
Reconciliation now reports a **real** discrepancy on an external account. The balance value in PostgreSQL and Redis is **~+1e23**, but the sum of operations for the same account is **-199**. The discrepancy is **+100000000000000000000000**. This indicates a **balance jump** that did not come from recorded operations.

This should not exist. We need to identify **where** the balance was written and **why** it bypassed operation persistence.

---

## Impact
- Reconciliation is reporting a warning due to 1 balance discrepancy.
- The account is external (`account_type = external`), but reconciliation expects balance = sum(ops).
- If not fixed, this indicates **data integrity drift** between balance state and operation ledger.

---

## Primary Evidence (Concrete Values)

### Account / Balance
- **Account ID**: `019b70c4-b969-7821-a8e3-cfa76e1367c2`
- **Balance ID**: `019b70c4-b96a-7181-a6f1-0fc6f35c8ded`
- **Alias**: `@external/USD`
- **Key**: `default`
- **Account Type**: `external`
- **Balance.available**: `99999999999999999999801`
- **Balance.version**: `9`
- **Timestamp**: `2025-12-30 19:38:19.439594+00`

### Operation Summary
- **total_credits**: `10000000000000000000000.234567890123456789`
- **total_debits**: `10000000000000000000199.234567890123456789`
- **expected_balance**: `-199.000000000000000000`
- **discrepancy**: `+100000000000000000000000.000000000000000000`

### Operation Snapshots Show a Jump
The per-operation snapshot sequence shows a **huge reset** mid-stream:
- Op4 `available_balance_after = -48.765432109876543211`
- Op5 `available_balance` starts at `99999999999999999999951.234567890123456789`

This implies a **non-operation write** to the balance (or cache reseed) between ops.

---

## Supporting Evidence (Redis)

Redis key shows same large value:
```
balance:{transactions}:019b70c4-b963-7af2-9c19-2b560403ac92:019b70c4-b965-7efc-a744-4d3f7b5e4f5d:@external/USD#default
```

Value (abridged):
```
{"Available":"99999999999999999999801","Version":9,"AccountType":"external","Alias":"0#@external/USD#default",...}
```

This confirms the drift is **not only DB**; cache holds the same inflated value.

---

## Queries Used (Copy/Paste)

### 1) Balance row
```sql
SELECT id, account_id, alias, asset_code, available, on_hold, version, created_at, updated_at
FROM balance
WHERE account_id = '019b70c4-b969-7821-a8e3-cfa76e1367c2'
  AND asset_code = 'USD';
```

### 2) Reconciliation calc for this account
```sql
WITH balance_calc AS (
  SELECT
    b.id,
    b.account_id,
    b.asset_code,
    b.key,
    b.available::DECIMAL AS current_balance,
    COALESCE(SUM(CASE WHEN o.type = 'CREDIT' AND o.balance_affected THEN o.amount ELSE 0 END), 0)::DECIMAL AS total_credits,
    COALESCE(SUM(CASE WHEN o.type = 'DEBIT' AND o.balance_affected THEN o.amount ELSE 0 END), 0)::DECIMAL AS total_debits,
    COALESCE(SUM(CASE WHEN o.type = 'ON_HOLD' AND o.balance_affected AND t.status = 'PENDING' THEN o.amount ELSE 0 END), 0)::DECIMAL AS total_on_hold,
    COUNT(o.id) AS operation_count
  FROM balance b
  LEFT JOIN operation o ON b.account_id = o.account_id
    AND b.asset_code = o.asset_code
    AND b.key = o.balance_key
    AND o.deleted_at IS NULL
  LEFT JOIN transaction t ON o.transaction_id = t.id
  WHERE b.deleted_at IS NULL
    AND b.account_id = '019b70c4-b969-7821-a8e3-cfa76e1367c2'
    AND b.asset_code = 'USD'
  GROUP BY b.id, b.account_id, b.asset_code, b.key, b.available
)
SELECT *,
  (total_credits - total_debits - total_on_hold) AS expected_balance,
  (current_balance - (total_credits - total_debits - total_on_hold)) AS discrepancy
FROM balance_calc;
```

### 3) Operation snapshots
```sql
SELECT id, type, amount, available_balance, available_balance_after, on_hold_balance, on_hold_balance_after, created_at
FROM operation
WHERE account_id = '019b70c4-b969-7821-a8e3-cfa76e1367c2'
  AND asset_code = 'USD'
  AND deleted_at IS NULL
ORDER BY created_at;
```

### 4) Redis key
```bash
docker exec -i midaz-valkey redis-cli -a lerian -p 5704 --raw \
  GET "balance:{transactions}:019b70c4-b963-7af2-9c19-2b560403ac92:019b70c4-b965-7efc-a744-4d3f7b5e4f5d:@external/USD#default"
```

---

## Hypotheses (Need Verification)

1) **Balance cache re-seed with wrong base**  
   A service re-populated Redis balance state from DB (or vice versa) with a large value
   that did not originate from operations. This could happen in the balance sync worker
   or in a direct DB update path.

2) **Direct balance update path bypassed ops**  
   Some write path updates `balance.available` without emitting operations
   (e.g., manual update endpoint, migration, or a bug in transaction flow).

3) **Outbox/transaction rollback path left partial state**  
   If a transaction wrote balance state in cache (or DB) but operations insert failed/rolled
   back, the balance could diverge. This would match the sudden jump.

4) **Data seeding/test harness**  
   Integration tests inject very large amounts (see `tests/integration/*` and fuzz tests).
   A test might have written the huge value directly (or via a special code path)
   without corresponding ops for this account.

---

## Next Steps (Suggested)

### A) Trace writes to this balance
- Search logs around **2025-12-30 19:38:19 UTC** for:
  - balance updates
  - redis balance set events
  - sync worker actions
  - transaction commits touching this account

### B) Audit balance sync worker
- Check whether the sync worker can **overwrite** DB balances using cache values.
- Verify whether it can write balances without validating operation parity.

### C) Identify any direct balance writes
- Check any code paths that update `balance.available` without inserting ops.
- Verify UpdateBalance or BalanceRepo update calls in transaction service.

### D) Reproduce on clean DB
- Run integration tests that include very large values.
- Run reconciliation and inspect whether a mismatch appears again.

---

## Files / Components Likely Relevant

- `components/transaction/internal/adapters/redis/scripts/add_sub.lua`
  - Cache mutation logic and TTL/seed behavior.
- `components/transaction/internal/adapters/redis/consumer.redis.go`
  - Balance sync and cache write paths.
- `components/transaction/internal/services/command/update-balance.go`
  - Balance updates that might bypass operations.
- `components/transaction/internal/adapters/postgres/balance/balance.postgresql.go`
  - Direct DB writes to balance table.
- `components/transaction/internal/bootstrap/balance.worker.go`
  - Balance sync worker (Redis -> PostgreSQL).

---

## Notes
- Reconciliation was updated to use decimal, so the discrepancy is now visible instead of failing with bigint overflow.
- This is **not** a reconciliation math issue; expected balance is correct.
- The core issue is a **state divergence** between balance and operations.

---

## Follow-up Code Path Findings (2025-12-30)

### Where balance.available can be written
Only three code paths update `balance.available` in PostgreSQL:
- **BalancesUpdate**: `components/transaction/internal/services/command/update-balance.go` -> `BalanceRepo.BalancesUpdate` (writes computed balances from operations).
- **SyncBalance**: `components/transaction/internal/bootstrap/balance.worker.go` -> `BalanceRepo.Sync` (writes Redis cache state to DB if version is higher).
- **UpdateBalance** (HTTP PATCH) does **not** touch amounts; it only updates allow_sending/allow_receiving in `balance.postgresql.go`.

This means any "jump" must come from either:
1) the computed balances passed into `BalancesUpdate`, or
2) Redis cache values later synced by the balance sync worker.

### Redis cache write behavior (add_sub.lua)
- `components/transaction/internal/adapters/redis/scripts/add_sub.lua` writes to Redis via `SET`:
  - If key **does not exist**: seeds from **DB-provided** balance values (ARGV).
  - If key **exists**: keeps cached Available/OnHold/Version and overwrites identity fields from DB.
- After mutation it **always** writes the updated balance JSON to Redis and schedules a sync (ZADD) if enabled.

### Balance sync worker behavior
- `components/transaction/internal/bootstrap/balance.worker.go` periodically pulls scheduled balance keys and calls `SyncBalance`.
- `BalanceRepo.Sync` updates DB **whenever Redis version is higher**, without checking operations parity.

### Implication
A balance can be persisted to DB **without matching operations** if:
1) Redis was updated by a transaction, but downstream DB transaction/operation persistence failed or was never processed.
2) Later, the balance sync worker wrote that Redis state back to DB (version check passes).

This matches the observed mid-stream jump: op4 ends at ~-48; op5 begins at ~1e23, which suggests a **non-operation balance write** occurred between those ops and became the new base for subsequent operations.

---

## Concrete log anchors to search (around 2025-12-30 19:38:19 UTC)
Look for these markers to confirm Redis->DB sync or cache refresh paths:
- `BalanceSyncWorker: Synced key` (balance worker writes Redis to DB)
- `BALANCE_UPDATE_SKIPPED` / `STALE_BALANCE_SKIP` (UpdateBalances used cache refresh)
- `DB_UPDATE_START` / `DB_UPDATE_SUCCESS` (balances updated in PostgreSQL)
- Redis Lua script execution logs: `redis.add_sum_balance_script` and `result value:` (AddSumBalancesRedis path)

---

## Next concrete checks
1) Identify the **transaction ID(s)** around the op4/op5 boundary and correlate with logs.
2) Check for **balance sync** events for this balance key in the same window.
3) Inspect Redis backup queue `backup_queue:{transactions}` for any failed/stranded transactions affecting this balance.

---

## Root Cause Found (2025-12-30)

### ⚠️ Lua decimal comparison bug in `add_sub.lua` (precision loss)
**Cause:** `sub_decimal()` uses `tonumber()` to compare large decimal strings:
```lua
local a_num = tonumber(a)
local b_num = tonumber(b)
if a_num < b_num then
  return "-" .. sub_decimal(b, a)
end
```
For values above ~9e15, `tonumber()` loses precision (IEEE-754). Two distinct
values can compare equal, so `a < b` fails incorrectly. When `a < b` is missed,
the subtraction algorithm runs as if `a >= b`, which yields a **10^n complement**
result (i.e., `10^digits + a - b`). This exactly matches the observed **+1e23**
jump.

### Concrete Evidence (Logs)
**Transaction b981 (credit 9,999,999,999,999,999,999,999):**
- Correct math should be: `-10000000000000000000047.765... + 9999999999999999999999 = -48.765...`
- Transaction JSON logged balanceAfter = `-48.765432109876543211` (expected).

**Immediately after, Lua script pre-state for next tx (b986) shows corrupted cached value:**
```
Available":"99999999999999999999951.234567890123456789","Version":4
```
That is exactly `100000000000000000000000 - 48.765432109876543211`.

### Minimal Repro (Lua)
```
a = tonumber("9999999999999999999999")                  -> 1e+22
b = tonumber("10000000000000000000047.765432109876543211") -> 1e+22
a < b == false (precision loss)
```
This makes `sub_decimal()` treat `a >= b` and return the **10^23 complement**.

---

## Why This Produces the Observed Jump
In `add_decimal()`, when `balance.Available` is **negative** and `amount` is **positive**,
it calls `sub_decimal(b, |a|)` to compute `b - |a|`.
Because `sub_decimal()` compares via `tonumber()`, the sign check fails for large values.
That causes the algorithm to compute `10^n + b - |a|`, yielding the **+1e23** offset.

---

## Fix Direction (Required)
Replace the `tonumber()` comparison in `sub_decimal()` with **string-based magnitude comparison**:
1) Normalize integer parts (strip leading zeros).
2) Compare integer length.
3) If equal, lexicographic compare integer digits.
4) If integer equal, compare fractional parts (pad to equal length).

This avoids float precision loss and keeps subtraction sign correct for large values.
