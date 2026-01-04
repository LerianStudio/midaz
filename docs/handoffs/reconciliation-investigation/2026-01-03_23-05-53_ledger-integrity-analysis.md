---
date: 2026-01-04T02:05:53Z
session_name: reconciliation-investigation
git_commit: 8b2743137380ab4b40b57e9d83b4e2cafef4299c
branch: fix/fred-several-ones-dec-13-2025
repository: LerianStudio/midaz
topic: "Reconciliation Report Analysis - Ledger Integrity Investigation"
tags: [reconciliation, ledger, balance-discrepancy, pending-transactions, redis, investigation]
status: complete
outcome: UNKNOWN
root_span_id:
turn_span_id:
---

# Handoff: Reconciliation Report Deep Dive - Ledger Integrity Analysis

## Task Summary

Investigated reconciliation report inconsistencies showing CRITICAL status. The user wanted to determine if the ledger is flawed or if the reconciliation engine has bugs.

**Conclusion: The ledger HAS logic bugs causing real data inconsistencies.**

Key findings:
1. **5 balance discrepancies** - available amounts don't match sum of operations
2. **4 on_hold discrepancies** - ON_HOLD operations never released
3. **250/250 Redis mismatches** - Redis cache completely empty
4. **7 unsettled transactions** - pending transactions with incomplete operation sets

## Critical References

- `components/reconciliation/internal/adapters/postgres/balance_check.go` - Balance verification SQL logic
- `components/transaction/internal/services/command/create-balance-transaction-operations-async.go:282-290` - Bug location: missing operation creation on status update
- `components/transaction/internal/adapters/redis/scripts/add_sub.lua:1-71` - Expected state machine documentation

## Recent Changes

**No code changes were made** - this was a read-only investigation session.

## Learnings

### What Worked
- **Direct database queries** via MCP dbhub tools - allowed verification of reconciliation findings against raw data
- **Parallel agent exploration** - Two Explore agents simultaneously mapped reconciliation engine architecture and balance calculation logic
- **Timeline analysis with versions** - Comparing `balance.version` vs `operation.balance_version_after` revealed version gaps indicating missed operations
- **Cross-referencing transaction status with operations** - Exposed the missing RELEASE/DEBIT pattern

### What Failed
- **WebFetch to localhost** - Both WebFetch and Firecrawl can't access localhost URLs; used curl via Bash instead
- **Initial assumption about stale writes** - First hypothesis was that balance writes were being rejected due to version conflicts, but data showed versions match; the bug is in operation creation, not balance persistence

### Key Decisions
- Decision: **Investigate via database queries rather than just code review**
  - Alternatives: Could have only reviewed code paths
  - Reason: Database state provides ground truth; code review alone wouldn't reveal which bugs are actively causing issues vs theoretical

- Decision: **Compare working vs non-working transactions**
  - Alternatives: Focus only on failing cases
  - Reason: Transaction `019b84b2-172a-79ae-9578-b1ee834d8fb3` has proper ON_HOLD + RELEASE, proving the system CAN work correctly; others are missing RELEASE, proving inconsistent behavior

## Files Modified

None - investigation only.

## Key Database Findings

### Balance Discrepancies Confirmed
```sql
-- Account 019b84b4-242a-75e7-9c37-0793b5be31ba (@external/USD)
-- Has 2 DEBIT operations: 10 + 3 = 13
-- Balance shows available = -10 (should be -13)
-- Discrepancy: +3
```

### Missing RELEASE Operations
```
TX 019b84b4-2442-7073-8b73-6b565cacdf5e (APPROVED, pending=true)
  └─ Only has ON_HOLD 3, NO DEBIT to release

TX 019b84b4-244f-7984-a08a-60db19447a11 (CANCELED, pending=true)
  └─ Only has ON_HOLD 2, NO RELEASE operation

TX 019b84b2-172a-79ae-9578-b1ee834d8fb3 (CANCELED, pending=true)
  ├─ ON_HOLD 2 ✓
  └─ RELEASE 2 ✓ (CORRECT - proves system can work)
```

### Redis Cache Empty
```bash
redis-cli KEYS "*" | wc -l
# Output: 0
```
All 250 sampled balances show redis_version=0.

## Root Cause Analysis

### Bug #1: Missing Operations on Status Update
**Location:** `create-balance-transaction-operations-async.go:282-290`

```go
if pending && (tran.Status.Code == constant.APPROVED || tran.Status.Code == constant.CANCELED) {
    _, err = uc.UpdateTransactionStatus(ctx, tran)  // Only updates status!
    // NO operations created for RELEASE/DEBIT!
}
```

When a pending transaction is committed/canceled and the transaction already exists (unique violation), the code only updates the status but doesn't create the required RELEASE/DEBIT operations.

### Bug #2: Balance Amount Not Applied
Operations are recorded correctly (version increments), but the balance.available doesn't reflect all operations. Second DEBIT of 3 was recorded but balance stayed at -10 instead of becoming -13.

### Bug #3: Redis Infrastructure Failure
The BalanceSyncWorker is either not running or Redis was cleared. Doesn't cause data corruption (PostgreSQL is source of truth) but affects availability.

## Action Items & Next Steps

1. **Fix missing operation creation** - Modify `handleCreateTransactionError` to ensure RELEASE/DEBIT operations are created when updating pending transaction status
2. **Investigate balance update atomicity** - Determine why operations are recorded but balance amounts aren't updated in some cases
3. **Restart/check BalanceSyncWorker** - Redis cache needs repopulation
4. **Add regression tests** - Cover pending transaction commit/cancel flows with operation verification
5. **Consider adding alerts** - Reconciliation CRITICAL status should trigger alerts

## Useful Commands for Continuation

```bash
# Fetch reconciliation report
curl -s http://127.0.0.1:3005/reconciliation/report | jq '.'

# Check balance discrepancies via SQL
# (Use MCP dbhub tools with transaction-replica)

# Check Redis keys
redis-cli KEYS "balance:*"
redis-cli KEYS "schedule:*"
```

## Other Notes

### Reconciliation Engine Architecture
- 11 different checkers run in parallel
- Located in `components/reconciliation/`
- Runs on 5-minute interval via worker
- Uses PostgreSQL replicas for read-only queries

### Expected Transaction State Machine (from Lua script)
```
PENDING  + ON_HOLD  → Available -= amount, OnHold += amount
CANCELED + RELEASE  → OnHold -= amount, Available += amount
APPROVED + DEBIT    → OnHold -= amount
APPROVED + CREDIT   → Available += amount
```

### Database Connection Strings
- `onboarding-replica` - Read-only onboarding data
- `transaction-replica` - Read-only transaction data (where balance/operation tables live)
