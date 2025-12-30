# Handoff: Reconciliation Query Fix for ON_HOLD Operations

## Metadata
- **Created**: 2025-12-30T01:39:52Z
- **Branch**: fix/fred-several-ones-dec-13-2025
- **Commit**: 3b612671f726587a8512aee169e8ce441ad9113a
- **Previous Handoff**: 2025-12-29_22-20-24_key-format-mismatch-fix.md
- **Status**: PARTIAL SUCCESS - Reduced discrepancies from 5 to 2

---

## Task Summary

### Original Problem
The `make reconcile` command was showing **5 balance discrepancies** after running integration tests.

### What Was Done This Session
1. Resumed from previous handoff which implemented key-format mismatch fix
2. Ran integration tests on clean database (214 passed, 6 skipped, 2 unrelated failures)
3. Ran reconciliation - still showed 5 discrepancies
4. **Discovered the real issue**: Reconciliation query didn't account for ON_HOLD operations
5. Fixed reconciliation query - reduced to 2 discrepancies
6. Identified remaining 2 as a **separate issue** (version mismatch)

### Current State
- **Discrepancies reduced**: 5 → 2
- **Integration tests**: 214 pass, 2 fail (pre-existing AccountTypes 500 error)
- **Remaining issues**: 2 @external/USD accounts with balance version mismatches

---

## Root Cause Analysis

### Issue 1: Reconciliation Query Incomplete (FIXED)

The reconciliation query calculated:
```sql
expected = CREDITS - DEBITS
```

But ON_HOLD operations **also reduce available balance**:
```go
// From testutil_test.go - ON_HOLD behavior
initialAvailable: 100, initialOnHold: 0
-> after ON_HOLD 30 ->
expectedAvailable: 70, expectedOnHold: 30
```

**Fix Applied** to `scripts/reconciliation/run_reconciliation.sh`:
```sql
expected = CREDITS - DEBITS - ON_HOLD (where status IN ('APPROVED', 'PENDING'))
```

- APPROVED ON_HOLD: Committed PENDINGs where hold is offset by CREDIT
- PENDING ON_HOLD: Active holds that reduce available
- CANCELED ON_HOLD: Ignored (fully reversed, net zero effect)

### Issue 2: Balance Version Mismatch (NOT FIXED)

Two @external/USD accounts show:
- Balance version 4 or 6
- Only 2 operations recorded
- Version jumped without corresponding operations

| Account | Balance | Version | Operations | Expected |
|---------|---------|---------|------------|----------|
| @external/USD | -10 | 6 | 2 (DEBIT 10, DEBIT 3) | -13 |
| @external/USD | -10 | 4 | 2 (DEBIT 10, DEBIT 3) | -13 |

**Hypothesis**: Version is incremented during PENDING transaction lifecycle for optimistic locking, but no operation is recorded for source accounts when:
1. PENDING is created (destination gets ON_HOLD, source version bumps?)
2. PENDING is canceled (no operations recorded)

---

## Critical References

### Files MUST READ to continue:

1. **`scripts/reconciliation/run_reconciliation.sh`** (lines 187-209)
   - Fixed reconciliation query with ON_HOLD handling
   - This is the change made this session

2. **`pkg/transaction/validations.go`** (lines 248-268)
   - `OperateBalances()` and `applyBalanceOperation()` - how balance updates work
   - Key to understanding version increment logic

3. **`components/transaction/internal/adapters/http/in/transaction.go`**
   - Lines 1579-1620: COMMIT/CANCEL transaction handling
   - Where operations should be created

4. **`pkg/transaction/testutil_test.go`** (lines 254-268)
   - Shows expected ON_HOLD behavior
   - Confirms ON_HOLD reduces available

### Test Files:

5. **`tests/integration/transaction_lifecycle_flow_test.go`**
   - Creates the PENDING/COMMIT/CANCEL scenarios
   - Source of the test data causing discrepancies

---

## Changes Made This Session

```
Modified files:
- scripts/reconciliation/run_reconciliation.sh
  - Lines 191-197: Added ON_HOLD to balance calculation
  - Added LEFT JOIN to transaction table for status check
  - Added comment explaining the formula
```

**Diff summary**:
```sql
-- Before:
COALESCE(SUM(CASE WHEN o.type = 'CREDIT'...)) -
COALESCE(SUM(CASE WHEN o.type = 'DEBIT'...)) as expected

-- After:
COALESCE(SUM(CASE WHEN o.type = 'CREDIT'...)) -
COALESCE(SUM(CASE WHEN o.type = 'DEBIT'...)) -
COALESCE(SUM(CASE WHEN o.type = 'ON_HOLD' AND t.status IN ('APPROVED', 'PENDING')...)) as expected
```

---

## Key Learnings

### What Worked
1. **Database tracing** - Walking through operation versions identified the pattern
2. **Test file analysis** - `testutil_test.go` confirmed ON_HOLD behavior
3. **Incremental fixes** - Fixing query in stages showed progress (5→3→2)

### What We Discovered
1. **ON_HOLD reduces available** - Not just increases on_hold column
2. **CANCELED ON_HOLD is reversed** - Should not count in expected calculation
3. **PENDING ON_HOLD is active** - Must count in expected calculation
4. **Version bumps without operations** - This is a separate data integrity issue

### Architecture Insight

| Operation Type | Effect on Available | Count in Expected? |
|----------------|--------------------|--------------------|
| CREDIT | + amount | Yes |
| DEBIT | - amount | Yes |
| ON_HOLD (APPROVED) | - amount | Yes (offset by CREDIT from COMMIT) |
| ON_HOLD (PENDING) | - amount | Yes (active hold) |
| ON_HOLD (CANCELED) | 0 (reversed) | No |

---

## Action Items (Next Session)

### Immediate - Investigate Version Mismatch
1. [ ] Trace where balance version is incremented without operation
2. [ ] Check if this happens in `commitOrCancelTransaction()` flow
3. [ ] Verify if version bump is intentional (optimistic locking) or a bug
4. [ ] If bug: fix to either record operation OR not bump version

### Investigation Queries
```sql
-- Find accounts where version doesn't match operation count
SELECT
    b.alias,
    b.version as balance_version,
    COUNT(o.id) as operation_count,
    b.version - COUNT(o.id) as version_gap
FROM balance b
LEFT JOIN operation o ON b.account_id = o.account_id
    AND b.asset_code = o.asset_code
    AND b.key = o.balance_key
WHERE b.deleted_at IS NULL
GROUP BY b.id, b.alias, b.version
HAVING b.version != COUNT(o.id)
ORDER BY version_gap DESC;
```

### If Version Bump is Intentional
- Document that reconciliation version check is not meaningful
- OR modify reconciliation to ignore version-based checks
- OR record "no-op" operations for version tracking

### Potential Code Locations to Check
- `components/transaction/internal/services/command/` - Balance update logic
- Look for `Version` or `version` increment without `operation` creation

---

## Verification Commands

```bash
# Run reconciliation
make reconcile

# Check remaining discrepancies
docker exec midaz-postgres-primary psql -U midaz -d transaction -c "
WITH balance_calc AS (
    SELECT b.alias, b.available, b.version,
        COALESCE(SUM(CASE WHEN o.type = 'CREDIT' THEN o.amount ELSE 0 END), 0) -
        COALESCE(SUM(CASE WHEN o.type = 'DEBIT' THEN o.amount ELSE 0 END), 0) -
        COALESCE(SUM(CASE WHEN o.type = 'ON_HOLD' AND t.status IN ('APPROVED','PENDING') THEN o.amount ELSE 0 END), 0) as expected,
        COUNT(o.id) as ops
    FROM balance b
    LEFT JOIN operation o ON b.account_id = o.account_id AND b.asset_code = o.asset_code AND b.key = o.balance_key
    LEFT JOIN transaction t ON o.transaction_id = t.id
    WHERE b.deleted_at IS NULL
    GROUP BY b.id, b.alias, b.available, b.version
)
SELECT * FROM balance_calc WHERE ABS(available - expected) > 0;
"
```

---

## Resume Command

```
/resume-handoff docs/handoffs/reconciliation-balance-bug/2025-12-29_22-39-52_reconciliation-query-fix.md
```
