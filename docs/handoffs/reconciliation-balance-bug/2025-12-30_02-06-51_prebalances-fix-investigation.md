# Handoff: PreBalances Fix Investigation - Incomplete

## Metadata
- **Created**: 2025-12-30T02:06:51Z
- **Branch**: fix/fred-several-ones-dec-13-2025
- **Commit**: da99b4c954dc4c7b13e7ec61561fba8dd4865b23
- **Previous Handoff**: 2025-12-29_22-39-52_reconciliation-query-fix.md
- **Status**: IN PROGRESS - Root cause found, fix implemented locally (needs validation)

---

## Task Summary

### Update (2025-12-30)
**Root cause confirmed**: async commit/cancel operations are not persisted because metadata outbox insertion fails with duplicate key, aborting the DB transaction.

**Why it happens**
- Initial PENDING transaction inserts a `metadata_outbox` row (status PENDING).
- In local dev, `METADATA_OUTBOX_WORKER_ENABLED` defaults to `false`, so the row stays PENDING.
- Commit/cancel reuses the same transaction ID and tries to insert another outbox entry inside `CreateBalanceTransactionOperationsAsync`.
- Unique index `idx_metadata_outbox_entity_pending` (`entity_id`, `entity_type` WHERE status IN ('PENDING','PROCESSING')) triggers a duplicate key error.
- That error aborts the atomic transaction (balances + operations), so commit/cancel operations never persist.
- Handler already updated transaction status synchronously in async mode, so DB shows APPROVED/CANCELED with missing ops → reconciliation mismatch.

**Evidence**
- Transaction service logs: `duplicate key value violates unique constraint "idx_metadata_outbox_entity_pending"` during `CreateBalanceTransactionOperationsAsync`.
- `metadata_outbox` table contains PENDING entries for the transaction IDs.
- Missing commit/cancel operations in Postgres for the affected transactions; only the initial ON_HOLD ops exist.

**Fix applied (code change)**
- Make outbox insert idempotent to avoid aborting the transaction:
  - `INSERT ... ON CONFLICT (entity_id, entity_type) WHERE status IN ('PENDING','PROCESSING') DO NOTHING`
  - File: `components/transaction/internal/adapters/postgres/outbox/outbox.postgresql.go`

**Next validation**
- Re-run `TestIntegration_Transactions_Lifecycle_PendingCommitCancelRevert`
- Run `make reconcile` and verify discrepancies do not grow.
- (Optional) enable `METADATA_OUTBOX_WORKER_ENABLED=true` locally to process outbox entries.

### What We Were Investigating
Why `make reconcile` shows **2 balance discrepancies** after running integration tests on a fresh database.

### What Was Done This Session

1. **Launched 4 explorer agents in parallel** to investigate:
   - Agent 1: Where version is incremented
   - Agent 2: commitOrCancelTransaction flow
   - Agent 3: PENDING lifecycle operations
   - Agent 4: Verify ON_HOLD reconciliation fix logic

2. **Identified potential bug** in `transaction.go:1255-1260`:
   - `BuildOperations()` returns `preBalances` (filtered to accounts with operations)
   - But code was discarding it (`_`) and passing ALL `balances` to `executeAndRespondTransaction()`
   - This caused destination account versions to be bumped without operations during PENDING

3. **Applied fix** to use `preBalances` instead of `balances`

4. **Attempted to fix reconciliation formula** (changing `APPROVED or PENDING` to `PENDING` only)
   - **THIS WAS WRONG** - it made discrepancies worse (2 → 4)
   - Reverted to original formula

5. **Current state**: Fix is live (hot reload), but **still 2 discrepancies**

### Critical Finding (Superseded)
The earlier hypothesis about missing source ops during PENDING creation was incorrect. The actual issue is missing commit/cancel operations due to outbox duplicate insert failures (see Update above).

---

## Root Cause Analysis (Ongoing)

### The 2 Discrepant Accounts

| Account | Balance | Version | Operations | Expected |
|---------|---------|---------|------------|----------|
| @external/USD | -10 | 4 | 2 DEBITs (10+3=13) | -13 |
| @external/USD | -10 | 6 | 2 DEBITs (10+3=13) | -13 |

### Transaction Flow Being Traced (Incomplete)

For version=4 @external/USD account, the ledger contains:
```
APPROVED | DEBIT:10, CREDIT:10       | @external→rev-...
APPROVED | ON_HOLD:3                 | rev-... (destination)
APPROVED | DEBIT:3, CREDIT:3         | @external→rev-...
```

**Key observation**: Transaction 2 only has `ON_HOLD:3` for the DESTINATION (`rev-...`), but **no operation for the SOURCE (`@external/USD`)**. This is where the missing operation is!

### The Real Bug (Hypothesis) — Superseded
The earlier hypothesis about missing source ops during PENDING creation was incorrect. The discrepancy is caused by commit/cancel operations never persisting due to outbox duplicate insertion failures (see Update above).

---

## Changes Made This Session

### 1. transaction.go (APPLIED - but doesn't fix the issue)
```go
// Line 1255-1263: Changed to use preBalances
operations, preBalances, err := handler.BuildOperations(...)
// ...
return handler.executeAndRespondTransaction(..., preBalances, parserDSL)
```

### 2. run_reconciliation.sh (REVERTED)
- Attempted to change ON_HOLD filter from `('APPROVED', 'PENDING')` to `('PENDING')`
- **Reverted** because it made things worse - original formula is correct

### 3. outbox.postgresql.go (APPLIED)
- Added idempotent insert to avoid duplicate key aborts:
  - `ON CONFLICT (entity_id, entity_type) WHERE status IN ('PENDING','PROCESSING') DO NOTHING`

---

## Key Learnings

### What Worked
1. **Parallel explorer agents** - efficiently gathered information from 4 angles
2. **Database tracing** - SQL queries to trace operations were invaluable
3. **Testing formula changes** - empirically verified formula correctness

### What Failed
1. **Agent 4's analysis was partially wrong** - claimed double-counting, but testing proved formula was correct
2. **preBalances fix** - doesn't address the actual issue (source ops not being created)

### Critical Insight
The reconciliation formula `CREDITS - DEBITS - ON_HOLD(APPROVED|PENDING)` **IS CORRECT**.

The real issue is **missing operations**, not formula calculation. Specifically, when PENDING is created, the SOURCE account is not getting an operation recorded.

---

## Critical References

### Files MUST READ to continue:

1. **`components/transaction/internal/adapters/http/in/transaction.go`**
   - Lines 1185-1261: `createTransaction()` - where PENDING is created
   - Lines 1285-1295: `buildFromToList()` - only includes source for PENDING
   - Lines 1082-1112: `BuildOperations()` - creates operations for accounts in fromTo

2. **`pkg/transaction/validations.go`**
   - Lines 248-268: `OperateBalances()` and `applyBalanceOperation()`

3. **`tests/integration/transaction_lifecycle_flow_test.go`**
   - Creates the PENDING/COMMIT/CANCEL scenarios causing discrepancies
4. **`components/transaction/internal/adapters/postgres/outbox/outbox.postgresql.go`**
   - `Create()` now uses `ON CONFLICT ... DO NOTHING` to avoid duplicate-key aborts

### Investigation Queries

```sql
-- Find missing operations: accounts where version > operation count
SELECT b.alias, b.version, COUNT(o.id) as ops, b.version - COUNT(o.id) as missing
FROM balance b
LEFT JOIN operation o ON b.account_id = o.account_id
    AND b.asset_code = o.asset_code AND b.key = o.balance_key AND o.deleted_at IS NULL
WHERE b.deleted_at IS NULL
GROUP BY b.id, b.alias, b.version
HAVING b.version > COUNT(o.id)
ORDER BY missing DESC;
```

---

## Action Items (Next Session)

### Immediate - Validate Outbox Idempotency Fix

1. [ ] Run `go test ./tests/integration -run TestIntegration_Transactions_Lifecycle_PendingCommitCancelRevert -count=1 -v`
2. [ ] Run `make reconcile` and verify discrepancies do not increase.
3. [ ] Confirm commit/cancel operations now exist for the pending transactions in Postgres.
4. [ ] (Optional) enable `METADATA_OUTBOX_WORKER_ENABLED=true` locally and verify outbox drains.

### Key Questions to Answer

1. **Does the outbox insert now skip duplicates without aborting the atomic transaction?**
2. **Do commit/cancel operations persist for the same transaction ID?**
3. **Does reconciliation stabilize after multiple runs of the lifecycle test?**

---

## Resume Command

```
/resume-handoff docs/handoffs/reconciliation-balance-bug/2025-12-30_02-06-51_prebalances-fix-investigation.md
```
