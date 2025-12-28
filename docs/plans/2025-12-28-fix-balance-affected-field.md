# Fix BalanceAffected Field Not Set Implementation Plan

> **For Agents:** REQUIRED SUB-SKILL: Use executing-plans to implement this plan task-by-task.

**Goal:** Fix critical bug where `BalanceAffected` field defaults to `false` in operation creation, causing reconciliation discrepancies.

**Architecture:** Single code fix in `create-operation.go` plus data repair migration. The bug is that Go's zero value for `bool` is `false`, and the field is not explicitly set to `true` for normal operations.

**Tech Stack:** Go 1.23+, PostgreSQL, Midaz transaction service

**Global Prerequisites:**
- Environment: macOS/Linux, Go 1.23+
- Tools: `go`, `psql`, `make`
- Access: Database credentials for transaction database
- State: Clean working tree, on feature branch

**Verification before starting:**
```bash
# Run ALL these commands and verify output:
go version         # Expected: go version go1.23+
psql --version     # Expected: psql (PostgreSQL) 14+
git status         # Expected: clean working tree (or known changes)
```

## Historical Precedent

**Query:** "balance affected operation reconciliation"
**Index Status:** Empty (new project)

No historical data available. This is normal for new projects.
Proceeding with standard planning approach.

---

## Root Cause Analysis

### The Bug

**File:** `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/create-operation.go`
**Lines:** 138-155

```go
save := &operation.Operation{
    ID:              libCommons.GenerateUUIDv7().String(),
    TransactionID:   transactionID,
    // ... other fields
    // BalanceAffected NOT SET - defaults to false in Go!
}
```

### Correct Implementation (for reference)

**File:** `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/http/in/transaction.go`
**Line:** 1171

```go
BalanceAffected: !isAnnotation,  // true for normal ops, false for annotations
```

### Semantics

| Scenario | `BalanceAffected` | Reason |
|----------|-------------------|--------|
| Normal transaction | `true` | Affects balance, included in reconciliation |
| Annotation transaction | `false` | Does NOT affect balance, excluded from reconciliation |

### Data Impact

Operations created via `create-operation.go` (async queue path) incorrectly have `balance_affected = false`, causing:
- Reconciliation queries (`WHERE balance_affected = true`) exclude these operations
- `current_balance != sum(operations)` discrepancy

---

## Task 1: Create the failing test for BalanceAffected field

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/create-operation_test.go`

**Prerequisites:**
- Go 1.23+
- Current working directory: repository root

**Step 1: Write the failing test**

Add this test at the end of the file (after line 99):

```go
// TestCreateOperationForBalance_SetsBalanceAffectedTrue verifies that operations
// created via createOperationForBalance have BalanceAffected set to true.
func TestCreateOperationForBalance_SetsBalanceAffectedTrue(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOpRepo := operation.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := UseCase{
		OperationRepo: mockOpRepo,
		MetadataRepo:  mockMetadataRepo,
	}

	// Capture the operation passed to Create
	var capturedOp *operation.Operation
	mockOpRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, op *operation.Operation) (*operation.Operation, error) {
			capturedOp = op
			return op, nil
		}).
		Times(1)

	// Setup test data
	available := decimal.NewFromInt(100)
	onHold := decimal.NewFromInt(0)
	testBalance := &mmodel.Balance{
		ID:             libCommons.GenerateUUIDv7().String(),
		AccountID:      libCommons.GenerateUUIDv7().String(),
		Alias:          "@test",
		OrganizationID: libCommons.GenerateUUIDv7().String(),
		LedgerID:       libCommons.GenerateUUIDv7().String(),
		Available:      available,
		OnHold:         onHold,
	}

	testFromTo := pkgTransaction.FromTo{
		AccountAlias: "@test",
		IsFrom:       true,
		Amount: &pkgTransaction.Amount{
			Asset: "USD",
			Value: decimal.NewFromInt(10),
		},
	}

	testDSL := &pkgTransaction.Transaction{
		Description: "Test transaction",
		Send: pkgTransaction.Send{
			Asset: "USD",
		},
	}

	testValidate := pkgTransaction.Responses{
		From: []pkgTransaction.Amount{
			{
				Asset: "USD",
				Value: decimal.NewFromInt(10),
			},
		},
	}

	ctx := context.Background()
	logger := libLog.NewLogrus(libLog.Config{Level: "error"})
	var span trace.Span

	// Call the function under test
	op, err := uc.createOperationForBalance(
		ctx,
		logger,
		&span,
		testBalance,
		testFromTo,
		"test-txn-id",
		testDSL,
		testValidate,
	)

	// Verify
	require.NoError(t, err)
	require.NotNil(t, op)
	require.NotNil(t, capturedOp)

	// THE KEY ASSERTION: BalanceAffected must be true for normal operations
	assert.True(t, capturedOp.BalanceAffected,
		"BalanceAffected must be true for normal (non-annotation) operations")
}
```

**Step 2: Add required imports**

At the top of the test file, ensure these imports are present:

```go
import (
	"context"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/mock/gomock"
)
```

**Step 3: Run test to verify it fails**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v -run "TestCreateOperationForBalance_SetsBalanceAffectedTrue" ./components/transaction/internal/services/command/`

**Expected output:**
```
=== RUN   TestCreateOperationForBalance_SetsBalanceAffectedTrue
    create-operation_test.go:XXX:
        	Error Trace:	...
        	Error:      	Should be true
        	Test:       	TestCreateOperationForBalance_SetsBalanceAffectedTrue
        	Messages:   	BalanceAffected must be true for normal (non-annotation) operations
--- FAIL: TestCreateOperationForBalance_SetsBalanceAffectedTrue
FAIL
```

**If you see different error:** Check imports and mock setup. The test should fail because `BalanceAffected` is `false` (not set).

**If Task Fails:**

1. **Test won't compile:**
   - Check: Import paths are correct
   - Fix: Verify all imports exist in go.mod
   - Rollback: `git checkout -- components/transaction/internal/services/command/create-operation_test.go`

2. **Mock not found:**
   - Run: `make generate` to regenerate mocks
   - Check: `mongodb.MockRepository` exists

---

## Task 2: Fix the bug - Set BalanceAffected to true

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/create-operation.go:138-155`

**Prerequisites:**
- Task 1 completed (failing test exists)

**Step 1: Locate the operation struct initialization**

The struct is at lines 138-155. Find this code:

```go
save := &operation.Operation{
	ID:              libCommons.GenerateUUIDv7().String(),
	TransactionID:   transactionID,
	Description:     description,
	Type:            typeOperation,
	AssetCode:       dsl.Send.Asset,
	ChartOfAccounts: ft.ChartOfAccounts,
	Amount:          amount,
	Balance:         balance,
	BalanceAfter:    balanceAfter,
	BalanceID:       blc.ID,
	AccountID:       blc.AccountID,
	AccountAlias:    blc.Alias,
	OrganizationID:  blc.OrganizationID,
	LedgerID:        blc.LedgerID,
	CreatedAt:       time.Now(),
	UpdatedAt:       time.Now(),
}
```

**Step 2: Add BalanceAffected field**

Replace the struct initialization with:

```go
save := &operation.Operation{
	ID:              libCommons.GenerateUUIDv7().String(),
	TransactionID:   transactionID,
	Description:     description,
	Type:            typeOperation,
	AssetCode:       dsl.Send.Asset,
	ChartOfAccounts: ft.ChartOfAccounts,
	Amount:          amount,
	Balance:         balance,
	BalanceAfter:    balanceAfter,
	BalanceID:       blc.ID,
	AccountID:       blc.AccountID,
	AccountAlias:    blc.Alias,
	OrganizationID:  blc.OrganizationID,
	LedgerID:        blc.LedgerID,
	BalanceAffected: true, // Operations created here always affect balance
	CreatedAt:       time.Now(),
	UpdatedAt:       time.Now(),
}
```

**Step 3: Run test to verify it passes**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v -run "TestCreateOperationForBalance_SetsBalanceAffectedTrue" ./components/transaction/internal/services/command/`

**Expected output:**
```
=== RUN   TestCreateOperationForBalance_SetsBalanceAffectedTrue
--- PASS: TestCreateOperationForBalance_SetsBalanceAffectedTrue
PASS
ok  	github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command	0.XXXs
```

**Step 4: Run all create-operation tests**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v -run "TestCreateOperation" ./components/transaction/internal/services/command/`

**Expected output:**
```
=== RUN   TestCreateOperationSuccess
--- PASS: TestCreateOperationSuccess
=== RUN   TestCreateOperationError
--- PASS: TestCreateOperationError
=== RUN   TestCreateOperation_NilResultChannel_Panics
--- PASS: TestCreateOperation_NilResultChannel_Panics
=== RUN   TestCreateOperation_NilErrorChannel_Panics
--- PASS: TestCreateOperation_NilErrorChannel_Panics
=== RUN   TestCreateOperationForBalance_SetsBalanceAffectedTrue
--- PASS: TestCreateOperationForBalance_SetsBalanceAffectedTrue
PASS
```

**If Task Fails:**

1. **Syntax error:**
   - Check: Comma after `BalanceAffected: true,`
   - Fix: Ensure proper Go struct syntax
   - Rollback: `git checkout -- components/transaction/internal/services/command/create-operation.go`

2. **Other tests break:**
   - Run: `go test -v ./components/transaction/internal/services/command/`
   - Analyze: Which test failed and why
   - Rollback: `git checkout -- .`

---

## Task 3: Create data repair migration

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/migrations/000014_fix_balance_affected.up.sql`
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/migrations/000014_fix_balance_affected.down.sql`

**Prerequisites:**
- Task 2 completed

**Step 1: Verify next migration number**

Run: `ls -la /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/migrations/*.sql | tail -10`

**Expected output:** Shows existing migrations. Use the next number (e.g., if 000013 exists, use 000014).

**Step 2: Create the UP migration**

Create file `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/migrations/000014_fix_balance_affected.up.sql`:

```sql
-- Migration: Fix balance_affected field for operations from non-annotation transactions
--
-- Root Cause: create-operation.go didn't set BalanceAffected, defaulting to false in Go
--
-- This migration repairs existing data by:
-- 1. Finding operations where balance_affected = false (incorrectly set)
-- 2. Joining with transactions to identify non-annotation transactions (status != 'NOTED')
-- 3. Setting balance_affected = true for those operations
--
-- Operations from annotation transactions (status = 'NOTED') correctly have
-- balance_affected = false and will NOT be modified.

-- First, let's see what we're about to fix (for audit purposes in logs)
DO $$
DECLARE
    affected_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO affected_count
    FROM operation o
    INNER JOIN transaction t ON o.transaction_id = t.id
    WHERE o.balance_affected = false
      AND o.deleted_at IS NULL
      AND t.deleted_at IS NULL
      AND t.status != 'NOTED';

    RAISE NOTICE 'Operations to be repaired: %', affected_count;
END $$;

-- Repair the data
UPDATE operation o
SET
    balance_affected = true,
    updated_at = NOW()
FROM transaction t
WHERE o.transaction_id = t.id
  AND o.balance_affected = false
  AND o.deleted_at IS NULL
  AND t.deleted_at IS NULL
  AND t.status != 'NOTED';

-- Verify the fix (log count of remaining issues)
DO $$
DECLARE
    remaining_count INTEGER;
BEGIN
    SELECT COUNT(*) INTO remaining_count
    FROM operation o
    INNER JOIN transaction t ON o.transaction_id = t.id
    WHERE o.balance_affected = false
      AND o.deleted_at IS NULL
      AND t.deleted_at IS NULL
      AND t.status != 'NOTED';

    IF remaining_count > 0 THEN
        RAISE WARNING 'Migration incomplete: % operations still have balance_affected = false for non-NOTED transactions', remaining_count;
    ELSE
        RAISE NOTICE 'Migration complete: All non-annotation operations now have balance_affected = true';
    END IF;
END $$;
```

**Step 3: Create the DOWN migration**

Create file `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/migrations/000014_fix_balance_affected.down.sql`:

```sql
-- Down migration: Revert balance_affected fix
--
-- WARNING: This migration cannot accurately restore the original state because
-- we don't track which operations were originally set to false incorrectly.
--
-- This is a DATA REPAIR migration - rolling back would reintroduce the bug.
-- The down migration is intentionally a no-op for safety.

-- No-op: Data repair migrations should not be reversed
SELECT 1;
```

**Step 4: Verify migration files exist**

Run: `ls -la /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/migrations/000014*.sql`

**Expected output:**
```
-rw-r--r--  1 user  group  XXX Dec 28 XX:XX 000014_fix_balance_affected.down.sql
-rw-r--r--  1 user  group  XXX Dec 28 XX:XX 000014_fix_balance_affected.up.sql
```

**If Task Fails:**

1. **Migration number conflict:**
   - Check: `ls components/transaction/migrations/*.sql | tail -5`
   - Fix: Use next available number
   - Rollback: Delete the created files

2. **Permission denied:**
   - Check: Directory permissions
   - Fix: `chmod 755 components/transaction/migrations/`

---

## Task 4: Run unit tests for transaction command package

**Files:**
- Test: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/`

**Prerequisites:**
- Tasks 1-3 completed

**Step 1: Run all command package tests**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v -race ./components/transaction/internal/services/command/...`

**Expected output:**
```
=== RUN   TestCreateOperationSuccess
--- PASS: TestCreateOperationSuccess
=== RUN   TestCreateOperationError
--- PASS: TestCreateOperationError
=== RUN   TestCreateOperation_NilResultChannel_Panics
--- PASS: TestCreateOperation_NilResultChannel_Panics
=== RUN   TestCreateOperation_NilErrorChannel_Panics
--- PASS: TestCreateOperation_NilErrorChannel_Panics
=== RUN   TestCreateOperationForBalance_SetsBalanceAffectedTrue
--- PASS: TestCreateOperationForBalance_SetsBalanceAffectedTrue
[... other tests ...]
PASS
```

**Step 2: Run tests with coverage**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -cover ./components/transaction/internal/services/command/...`

**Expected output:**
```
ok  	github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command	X.XXXs	coverage: XX.X% of statements
```

**If Task Fails:**

1. **Tests fail:**
   - Run: `go test -v ./components/transaction/internal/services/command/... 2>&1 | grep -A5 "FAIL"`
   - Analyze: Which test and why
   - Fix: Address the specific failure

2. **Race condition detected:**
   - Analyze: The race detector output
   - Fix: Add proper synchronization if needed
   - Rollback: `git stash` and investigate

---

## Task 5: Run Code Review

**Prerequisites:**
- Tasks 1-4 completed

1. **Dispatch all 3 reviewers in parallel:**
   - REQUIRED SUB-SKILL: Use requesting-code-review
   - All reviewers run simultaneously (code-reviewer, business-logic-reviewer, security-reviewer)
   - Wait for all to complete

2. **Handle findings by severity (MANDATORY):**

**Critical/High/Medium Issues:**
- Fix immediately (do NOT add TODO comments for these severities)
- Re-run all 3 reviewers in parallel after fixes
- Repeat until zero Critical/High/Medium issues remain

**Low Issues:**
- Add `TODO(review):` comments in code at the relevant location
- Format: `TODO(review): [Issue description] (reported by [reviewer] on [date], severity: Low)`
- This tracks tech debt for future resolution

**Cosmetic/Nitpick Issues:**
- Add `FIXME(nitpick):` comments in code at the relevant location
- Format: `FIXME(nitpick): [Issue description] (reported by [reviewer] on [date], severity: Cosmetic)`
- Low-priority improvements tracked inline

3. **Proceed only when:**
   - Zero Critical/High/Medium issues remain
   - All Low issues have TODO(review): comments added
   - All Cosmetic issues have FIXME(nitpick): comments added

---

## Task 6: Commit the code fix

**Files:**
- `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/create-operation.go`
- `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/create-operation_test.go`

**Prerequisites:**
- Tasks 1-5 completed

**Step 1: Stage the changes**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && git add components/transaction/internal/services/command/create-operation.go components/transaction/internal/services/command/create-operation_test.go`

**Step 2: Create commit**

Run:
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && git commit -m "$(cat <<'EOF'
fix(transaction): set BalanceAffected to true in createOperationForBalance

Operations created via the async queue path were not explicitly setting
the BalanceAffected field, causing it to default to false (Go's zero
value for bool). This caused reconciliation discrepancies because:

- Reconciliation queries filter on WHERE balance_affected = true
- Operations with false were excluded from balance sum calculations
- Result: current_balance != sum(operations)

Root cause: create-operation.go line 138-155 struct initialization
missing BalanceAffected field.

Fix: Explicitly set BalanceAffected: true for operations created here.
These are always balance-affecting operations (annotation transactions
use a different code path in transaction.go).

Includes test to verify BalanceAffected is set correctly.
EOF
)"
```

**Expected output:**
```
[branch-name abc1234] fix(transaction): set BalanceAffected to true in createOperationForBalance
 2 files changed, XX insertions(+), X deletions(-)
```

**If Task Fails:**

1. **Commit rejected by hook:**
   - Run: `go fmt ./...` and `golangci-lint run`
   - Fix: Address linting issues
   - Retry: Commit again

---

## Task 7: Commit the data repair migration

**Files:**
- `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/migrations/000014_fix_balance_affected.up.sql`
- `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/migrations/000014_fix_balance_affected.down.sql`

**Prerequisites:**
- Task 6 completed

**Step 1: Stage the migration files**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && git add components/transaction/migrations/000014_fix_balance_affected.up.sql components/transaction/migrations/000014_fix_balance_affected.down.sql`

**Step 2: Create commit**

Run:
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && git commit -m "$(cat <<'EOF'
fix(migration): repair balance_affected for existing operations

Data repair migration to fix operations that were incorrectly created
with balance_affected = false due to the bug in createOperationForBalance.

Migration logic:
- Updates operations where balance_affected = false
- Only for non-annotation transactions (status != 'NOTED')
- Annotation transactions correctly have balance_affected = false

This resolves 40+ reconciliation discrepancies where:
- current_balance != sum(operations where balance_affected = true)

The down migration is a no-op because this is a data repair - reverting
would reintroduce the data integrity issue.
EOF
)"
```

**Expected output:**
```
[branch-name def5678] fix(migration): repair balance_affected for existing operations
 2 files changed, XX insertions(+)
 create mode 100644 components/transaction/migrations/000014_fix_balance_affected.down.sql
 create mode 100644 components/transaction/migrations/000014_fix_balance_affected.up.sql
```

---

## Task 8: Verify property tests still pass

**Files:**
- Test: `/Users/fredamaral/repos/lerianstudio/midaz/tests/property/balance_consistency_test.go`
- Test: `/Users/fredamaral/repos/lerianstudio/midaz/tests/property/operations_sum_test.go`

**Prerequisites:**
- Tasks 6-7 completed
- Docker environment running (for integration tests)

**Step 1: Check if services are running**

Run: `docker ps | grep -E "(midaz|postgres|redis)" | head -5`

**Expected output:** Shows running containers for midaz services.

**If services not running:**
- Run: `make up` or `docker-compose up -d`
- Wait: 30 seconds for services to start

**Step 2: Run balance consistency property test**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v -timeout 5m -run "TestProperty_BalanceConsistency_API" ./tests/property/...`

**Expected output:**
```
=== RUN   TestProperty_BalanceConsistency_API
--- PASS: TestProperty_BalanceConsistency_API (XX.XXs)
PASS
```

**Step 3: Run operations sum property test**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v -timeout 5m -run "TestProperty_OperationsSum_API" ./tests/property/...`

**Expected output:**
```
=== RUN   TestProperty_OperationsSum_API
--- PASS: TestProperty_OperationsSum_API (XX.XXs)
PASS
```

**If Task Fails:**

1. **Services not running:**
   - Run: `make up`
   - Wait: 60 seconds
   - Retry: Tests

2. **Test timeout:**
   - Increase: `-timeout 10m`
   - Check: Service logs for errors

3. **Test failure:**
   - Analyze: Error message for root cause
   - Note: The property tests check `BalanceAffected` - they should pass with the fix

---

## Task 9: Run reconciliation script to verify fix

**Files:**
- Script: `/Users/fredamaral/repos/lerianstudio/midaz/scripts/reconciliation/run_reconciliation.sh`

**Prerequisites:**
- Tasks 6-8 completed
- Migration applied to test database

**Step 1: Apply migration to local database**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && make migrate-up`

**Expected output:**
```
Applying migrations...
000014_fix_balance_affected.up.sql: Operations to be repaired: XX
000014_fix_balance_affected.up.sql: Migration complete: All non-annotation operations now have balance_affected = true
Done.
```

**Step 2: Run reconciliation check**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && ./scripts/reconciliation/run_reconciliation.sh`

**Expected output:**
```
--- BALANCE CONSISTENCY SUMMARY ---
 total_balances | balances_with_discrepancy | discrepancy_percentage | total_absolute_discrepancy
----------------+---------------------------+------------------------+----------------------------
           XXX |                         0 |                   0.00 |                          0
```

**Key verification:** `balances_with_discrepancy` should be `0` after the migration.

**If Task Fails:**

1. **Script not found:**
   - Check: `ls -la scripts/reconciliation/run_reconciliation.sh`
   - Fix: Ensure script exists and is executable

2. **Database connection error:**
   - Check: Database credentials in environment
   - Fix: Export required env vars

3. **Discrepancies remain:**
   - Analyze: Which balances still have issues
   - Check: Were there annotation transactions mixed in?
   - Verify: Migration completed successfully

---

## Task 10: Final verification and summary

**Prerequisites:**
- All previous tasks completed

**Step 1: Run full test suite for transaction component**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -race ./components/transaction/...`

**Expected output:**
```
ok  	github.com/LerianStudio/midaz/v3/components/transaction/...
[all packages pass]
```

**Step 2: Verify commits**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && git log --oneline -3`

**Expected output:**
```
[hash] fix(migration): repair balance_affected for existing operations
[hash] fix(transaction): set BalanceAffected to true in createOperationForBalance
[previous commit]
```

**Step 3: Create summary**

The fix is complete. Summary:

| Item | Status |
|------|--------|
| Code fix in create-operation.go | Complete |
| Unit test for BalanceAffected | Added |
| Data repair migration | Created |
| Property tests | Passing |
| Reconciliation | Zero discrepancies |

---

## Post-Implementation Notes

### What was fixed

1. **Code bug:** `create-operation.go` now explicitly sets `BalanceAffected: true`
2. **Existing data:** Migration repairs operations from non-annotation transactions
3. **Tests:** New test verifies `BalanceAffected` is set correctly

### Why annotation transactions are excluded from repair

Annotation transactions (`status = 'NOTED'`) intentionally have `balance_affected = false` because:
- They're created via `/transactions/annotation` endpoint
- They're accounting entries that don't affect actual balances
- The HTTP handler correctly sets `BalanceAffected: !isAnnotation`

### Monitoring after deployment

After deploying to production:
1. Run reconciliation script daily for 1 week
2. Monitor for any new discrepancies
3. Alert if `balances_with_discrepancy > 0`

---

## Plan Checklist

- [x] Historical precedent queried (artifact-query --mode planning)
- [x] Historical Precedent section included in plan
- [x] Header with goal, architecture, tech stack, prerequisites
- [x] Verification commands with expected output
- [x] Tasks broken into bite-sized steps (2-5 min each)
- [x] Exact file paths for all files
- [x] Complete code (no placeholders)
- [x] Exact commands with expected output
- [x] Failure recovery steps for each task
- [x] Code review checkpoints after batches
- [x] Severity-based issue handling documented
- [x] Passes Zero-Context Test
- [x] Plan avoids known failure patterns
