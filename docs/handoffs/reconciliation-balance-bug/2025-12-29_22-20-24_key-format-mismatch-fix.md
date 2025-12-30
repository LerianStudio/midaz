# Handoff: Reconciliation Balance Discrepancy Bug Fix

## Metadata
- **Created**: 2025-12-30T01:20:22Z
- **Branch**: fix/fred-several-ones-dec-13-2025
- **Commit**: a275bc7ebe4cfea20d8728d56534aad71efe32ad
- **Status**: IN PROGRESS - Fix implemented but not fully verified

---

## Task Summary

### What We Were Investigating
The `make reconcile` command was showing **5 balance discrepancies** after running integration tests. The reconciliation script calculates expected balance as `CREDITS - DEBITS` but the actual balances didn't match.

### Root Cause Identified
**Key Format Mismatch in `ValidateFromToOperation`**

When committing PENDING transactions:
1. `validate.From` map is populated with **concatenated keys**: `"0#@external/USD#default"`
2. `ValidateFromToOperation` looks up using **split aliases**: `"@external/USD"`
3. Lookup fails, returns zero-value `Amount{Operation: ""}`
4. No DEBIT operations are created for commits
5. Balance is updated but operation record is missing

### Fixes Implemented

#### Fix 1: HandleAccountFields returns copies (DONE)
**File**: `components/transaction/internal/adapters/http/in/transaction.go:1034-1053`
- Changed to make copies instead of mutating original slice entries
- Prevents data corruption when called multiple times

#### Fix 2: ValidateSendSourceAndDistribute key handling (DONE)
**File**: `components/transaction/internal/adapters/http/in/transaction.go:1579-1596`
- Added `validationDSL` copy with concatenated aliases for validation
- Kept original `parserDSL` for storage

#### Fix 3: findAmountByAlias helper (DONE)
**File**: `pkg/transaction/validations.go:106-125`
- Added helper function to handle key format mismatch
- Tries direct lookup first, then searches for concatenated key containing alias
- Used in `ValidateFromToOperation` instead of direct map access

---

## Critical References

### Files MUST READ to continue:

1. **`pkg/transaction/validations.go`** (lines 106-154)
   - `findAmountByAlias()` - new helper function
   - `ValidateFromToOperation()` - modified to use helper

2. **`components/transaction/internal/adapters/http/in/transaction.go`**
   - Lines 1034-1053: `HandleAccountFields()` - copy fix
   - Lines 1579-1596: `commitOrCancelTransaction()` - validationDSL pattern
   - Lines 1604-1620: `executeCommitOrCancel()` - dual alias handling
   - Lines 1082-1182: `BuildOperations()` - where matching happens

3. **`scripts/reconciliation/run_reconciliation.sh`**
   - Lines 187-203: Balance consistency check SQL query

### Test Files:

4. **`pkg/transaction/validations_test.go`** (lines 942-1023)
   - `TestFindAmountByAlias` - comprehensive tests for new helper

5. **`components/transaction/internal/services/command/msgpack_operations_test.go`**
   - Verifies operations survive msgpack round-trip (confirmed NOT the issue)

---

## Recent Changes (Git Diff Summary)

```
Modified files:
- components/transaction/internal/adapters/http/in/transaction.go
  - HandleAccountFields: copy instead of mutate (lines 1034-1053)
  - commitOrCancelTransaction: validationDSL pattern (lines 1579-1596)
  - executeCommitOrCancel: comment update (lines 1604-1620)

- pkg/transaction/validations.go
  - Added findAmountByAlias() (lines 106-125)
  - Modified ValidateFromToOperation() to use helper (lines 127-154)

- pkg/transaction/validations_test.go
  - Added TestFindAmountByAlias (lines 942-1023)

- pkg/mmodel/transaction.go (by subagent, needs verification)
  - TransactionRevert() BalanceKey handling

- components/transaction/internal/services/command/msgpack_operations_test.go
  - New test file to verify msgpack serialization
```

---

## Key Learnings

### What Worked
1. **Explorer agents** were highly effective at tracing code flow
2. **Mental execution** ("walk through like a debugger") identified the key mismatch
3. **msgpack round-trip test** definitively ruled out serialization as the issue

### What Failed
1. **First fix attempt** removed duplicate `HandleAccountFields` calls - broke tests because BOTH concatenated AND split aliases are needed
2. **Copy fix alone** wasn't sufficient - the key format mismatch was the real issue

### Architecture Insights

| Component | Alias Format | Example |
|-----------|-------------|---------|
| `validate.From` keys | Concatenated | `"0#@external/USD#default"` |
| `blc.Alias` (from DB) | Raw/Split | `"@external/USD"` |
| `fromTo` (concat entries) | Concatenated | `"0#@external/USD#default"` |
| `fromTo` (split entries) | Raw/Split | `"@external/USD"` |

The system needs BOTH:
- **Concatenated aliases** for validation map keys (unique identifiers)
- **Split aliases** for matching `balance.Alias` in `BuildOperations`

### Dual-Alias Design Pattern
```
buildCommitFromToList(parserDSL) → concatenated aliases for validation
executeCommitOrCancel adds → split aliases for balance matching
```

---

## Current State

### Tests
- **Unit tests**: PASS (36 tests in transaction package)
- **Integration tests**: 17 failures (some due to CRM service issues, pre-existing AccountTypes 500 error)
- **Reconciliation**: Still showing 15 discrepancies (needs clean DB test)

### Services
- Transaction and Onboarding: Running
- CRM: Restarting (unrelated issue)
- Console: Unhealthy (unrelated)

---

## Action Items (Next Session)

### Immediate
1. [ ] Clean database volumes completely
2. [ ] Rebuild all Docker images: `make build && make down && make up`
3. [ ] Run integration tests on clean env
4. [ ] Run reconciliation to verify 0 discrepancies

### Verification Steps
```bash
# 1. Clean everything
make down
docker volume rm midaz_primary_volume midaz_mongodb_volume midaz_replica_volume midaz_redis_volume

# 2. Rebuild images (prod, not dev)
docker compose -f components/transaction/docker-compose.yml build --no-cache
docker compose -f components/onboarding/docker-compose.yml build --no-cache

# 3. Start services
make up
sleep 60

# 4. Run tests
make test-integration

# 5. Run reconciliation
make reconcile
```

### If Discrepancies Persist
1. Check if `findAmountByAlias` is being called correctly
2. Add debug logging in `ValidateFromToOperation` to see what keys are being looked up
3. Verify the `parts[1] == alias` comparison in `findAmountByAlias` handles edge cases

### Potential Issues to Watch
- The `findAmountByAlias` assumes concatenated format is always `"index#alias#balanceKey"`
- If alias itself contains `#` characters, the split logic may fail
- The fix only handles From/To maps, check if other maps have similar issues

---

## Database Queries for Debugging

```sql
-- Check operations for discrepant accounts
SELECT o.type, o.amount, o.account_alias, t.status, o.balance_version_before, o.balance_version_after
FROM operation o
JOIN transaction t ON o.transaction_id = t.id
WHERE o.account_alias LIKE '%lc-%' OR o.account_alias LIKE '%rev-%'
ORDER BY o.account_alias, o.balance_version_before;

-- Check balance discrepancies
WITH balance_calc AS (
    SELECT b.alias, b.available as current_balance,
        COALESCE(SUM(CASE WHEN o.type = 'CREDIT' AND o.balance_affected = true THEN o.amount ELSE 0 END), 0) -
        COALESCE(SUM(CASE WHEN o.type = 'DEBIT' AND o.balance_affected = true THEN o.amount ELSE 0 END), 0) as expected
    FROM balance b
    LEFT JOIN operation o ON b.account_id = o.account_id AND b.asset_code = o.asset_code AND b.key = o.balance_key
    WHERE b.deleted_at IS NULL
    GROUP BY b.id, b.alias, b.available
)
SELECT alias, current_balance, expected, (current_balance - expected) as discrepancy
FROM balance_calc
WHERE ABS(current_balance - expected) > 0;
```

---

## Resume Command

```
/resume-handoff docs/handoffs/reconciliation-balance-bug/2025-12-29_22-20-24_key-format-mismatch-fix.md
```
