# Hybrid Transaction Consistency Implementation Plan

> **For Agents:** REQUIRED SUB-SKILL: Use executing-plans to implement this plan task-by-task.

**Goal:** Implement saga-like balance status tracking with higher retry tolerance to support the "201 promise" - once a client receives 201, the system will retry balance persistence for up to 24 hours before marking as FAILED.

**SLA:** The "201 promise" has a **24-hour SLA**. After 288 retries (~24h), transactions are marked `FAILED` and routed to DLQ for manual intervention. This is a pragmatic balance between guaranteed persistence and operational reality.

**Architecture:** Add a `balance_status` field to transactions that tracks whether balance updates completed. Increase retry tolerance from 5 attempts (~50s) to time-based (24h) with exponential backoff capped at 5 minutes. Update balance_status on success/failure. Expose status via existing transaction GET endpoint.

**Tech Stack:** Go 1.22+, PostgreSQL 14+, RabbitMQ 3.x, Redis/Valkey

**Global Prerequisites:**
- Environment: macOS/Linux, Go 1.22+, Docker (for PostgreSQL and RabbitMQ)
- Tools: `go`, `psql`, `make`
- Access: Local development environment with transaction service running
- State: Branch `fix/fred-several-ones-dec-13-2025`, clean working tree

**Verification before starting:**
```bash
# Run ALL these commands and verify output:
go version                                    # Expected: go version go1.22+
docker ps | grep -E "postgres|rabbit|valkey"  # Expected: containers running
git status                                    # Expected: clean or expected changes
cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./pkg/constant/... -v -count=1  # Expected: PASS
```

## Historical Precedent

**Query:** "transaction balance retry saga consistency"
**Index Status:** Empty (new project)

No historical data available in artifact index. However, the handoff document at `docs/handoffs/e2e-test-fix/2026-01-04_04-14-32_async-transaction-consistency.md` provides critical context:

### Key Findings from Handoff
- **Root Cause:** `handleBalanceUpdateError` was returning `nil` when `usedCache=TRUE`, causing silent data loss (82-97% in chaos tests)
- **Current Fix Applied:** The code now returns error when `usedCache=TRUE` (lines 178-186 of `update-balance.go`)
- **The 201 Promise:** Once client gets 201, data MUST eventually be persisted
- **Recommended Approach:** Hybrid (Saga + higher retry + status tracking)

### Failure Patterns to AVOID
- Returning success when balance updates fail (was causing orphan transactions)
- Fixed retry counts that can exhaust before infrastructure recovers (5 retries = ~50s)
- No visibility into balance update status (clients can't verify)

---

## Phase 1: Database Schema Changes

### Task 1: Create balance_status migration files

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/migrations/000020_add_balance_status_to_transaction.up.sql`
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/migrations/000020_add_balance_status_to_transaction.down.sql`

**Prerequisites:**
- Directory exists: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/migrations/`
- PostgreSQL is running

**Step 1: Write the UP migration**

Create `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/migrations/000020_add_balance_status_to_transaction.up.sql`:

```sql
BEGIN;

-- Add balance_status column to track async balance update state
-- Values:
--   PENDING   - Balance update queued but not yet confirmed
--   CONFIRMED - Balance update completed successfully
--   FAILED    - Balance update failed after max retries (in DLQ)
-- Default PENDING for new transactions created in async mode
-- NULL for sync transactions (balance updated synchronously)
ALTER TABLE "transaction"
    ADD COLUMN balance_status TEXT
    CHECK (balance_status IS NULL OR balance_status IN ('PENDING', 'CONFIRMED', 'FAILED'));

-- Index for efficient status queries (partial index excludes NULLs and CONFIRMED)
CREATE INDEX idx_transaction_balance_status_pending
    ON "transaction" (balance_status, created_at)
    WHERE balance_status = 'PENDING';

-- Index for failed transactions requiring attention
CREATE INDEX idx_transaction_balance_status_failed
    ON "transaction" (balance_status, updated_at)
    WHERE balance_status = 'FAILED';

COMMENT ON COLUMN "transaction".balance_status IS 'Tracks async balance update state: PENDING=queued, CONFIRMED=completed, FAILED=DLQ. NULL for sync transactions.';

COMMIT;
```

**Step 2: Write the DOWN migration**

Create `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/migrations/000020_add_balance_status_to_transaction.down.sql`:

```sql
BEGIN;

DROP INDEX IF EXISTS idx_transaction_balance_status_failed;
DROP INDEX IF EXISTS idx_transaction_balance_status_pending;
ALTER TABLE "transaction" DROP COLUMN IF EXISTS balance_status;

COMMIT;
```

**Step 3: Verify migration files exist**

Run:
```bash
ls -la /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/migrations/000020*
```

**Expected output:**
```
-rw-r--r--  1 user  group  XXX  Jan  4 XX:XX 000020_add_balance_status_to_transaction.down.sql
-rw-r--r--  1 user  group  XXX  Jan  4 XX:XX 000020_add_balance_status_to_transaction.up.sql
```

**If Task Fails:**

1. **Migration files not found:**
   - Check: `ls /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/migrations/`
   - Fix: Ensure correct path, check for typos
   - Rollback: Remove any partial files created

2. **Migration number conflict:**
   - Check: `ls /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/migrations/000020*`
   - Fix: Use next available number (000021, etc.)

---

### Task 2: Add BalanceStatus constants

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/constant/transaction.go`

**Prerequisites:**
- File exists and is readable

**Step 1: Write the failing test**

This is a constant addition, no failing test needed. We'll verify by import.

**Step 2: Add constants to transaction.go**

Modify `/Users/fredamaral/repos/lerianstudio/midaz/pkg/constant/transaction.go` to add balance status constants after line 11:

```go
package constant

// CREATED represents a transaction status indicating it has been created.
const (
	CREATED             = "CREATED"
	APPROVED            = "APPROVED"
	PENDING             = "PENDING"
	CANCELED            = "CANCELED"
	NOTED               = "NOTED"
	UniqueViolationCode = "23505"
)

// BalanceStatus represents the state of async balance updates for a transaction.
// Used for saga-like consistency tracking.
const (
	// BalanceStatusPending indicates balance update is queued but not yet confirmed.
	BalanceStatusPending = "PENDING"
	// BalanceStatusConfirmed indicates balance update completed successfully.
	BalanceStatusConfirmed = "CONFIRMED"
	// BalanceStatusFailed indicates balance update failed after max retries (in DLQ).
	BalanceStatusFailed = "FAILED"
)
```

**Step 3: Verify constants compile**

Run:
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./pkg/constant/...
```

**Expected output:**
```
(no output - success)
```

**If Task Fails:**

1. **Syntax error:**
   - Check: Go syntax with `gofmt -d pkg/constant/transaction.go`
   - Fix: Correct any syntax issues
   - Rollback: `git checkout -- pkg/constant/transaction.go`

---

### Task 3: Update Transaction model to include BalanceStatus

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/transaction.go`

**Prerequisites:**
- Task 2 completed (constants exist)

**Step 1: Add BalanceStatus field to Transaction struct**

In `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/transaction.go`, find the Transaction struct (around line 123) and add the BalanceStatus field after the Operations field:

Find this block:
```go
	// List of operations associated with this transaction
	Operations []*Operation `json:"operations"`
} // @name Transaction
```

Replace with:
```go
	// List of operations associated with this transaction
	Operations []*Operation `json:"operations"`

	// BalanceStatus tracks the state of async balance updates
	// Values: PENDING, CONFIRMED, FAILED. Nil for sync transactions.
	// example: CONFIRMED
	BalanceStatus *string `json:"balanceStatus,omitempty" example:"CONFIRMED"`
} // @name Transaction
```

**Step 2: Verify model compiles**

Run:
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./pkg/mmodel/...
```

**Expected output:**
```
(no output - success)
```

**If Task Fails:**

1. **Compile error:**
   - Check: `go build ./pkg/mmodel/...`
   - Fix: Check JSON tags, field types
   - Rollback: `git checkout -- pkg/mmodel/transaction.go`

---

### Task 4: Update PostgreSQL model for BalanceStatus

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/postgres/transaction/transaction.go`

**Prerequisites:**
- Task 3 completed (mmodel updated)

**Step 1: Add BalanceStatus to TransactionPostgreSQLModel struct**

In `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/postgres/transaction/transaction.go`, find the TransactionPostgreSQLModel struct (around line 44) and add the BalanceStatus field after Metadata:

Find:
```go
	Metadata                 map[string]any              // Additional custom attributes
}
```

Replace with:
```go
	Metadata                 map[string]any              // Additional custom attributes
	BalanceStatus            *string                     // Async balance update status: PENDING, CONFIRMED, FAILED
}
```

**Step 2: Update ToEntity method to map BalanceStatus**

Find the ToEntity method (around line 64). After line 80 (before the closing brace of the Transaction struct literal), add:

Find this section:
```go
	if t.Route != nil {
		transaction.Route = *t.Route
	}
```

After it, add:
```go
	if t.BalanceStatus != nil {
		transaction.BalanceStatus = t.BalanceStatus
	}
```

**Step 3: Update FromEntity method to map BalanceStatus**

Find the FromEntity method (around line 101). After line 120 (the Route mapping), add:

Find this section:
```go
	if !libCommons.IsNilOrEmpty(&transaction.Route) {
		t.Route = &transaction.Route
	}
```

After it, add:
```go
	if transaction.BalanceStatus != nil {
		t.BalanceStatus = transaction.BalanceStatus
	}
```

**Step 4: Verify model compiles**

Run:
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./components/transaction/internal/adapters/postgres/transaction/...
```

**Expected output:**
```
(no output - success)
```

**If Task Fails:**

1. **Compile error:**
   - Check: Error message for line numbers
   - Fix: Match field types with mmodel
   - Rollback: `git checkout -- components/transaction/internal/adapters/postgres/transaction/transaction.go`

---

### Task 5: Update Repository column lists for BalanceStatus

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/postgres/transaction/transaction.postgresql.go`

**Prerequisites:**
- Task 4 completed

**Step 1: Add balance_status to transactionColumnList**

Find `transactionColumnList` (around line 36) and add `"balance_status"` at the end:

Find:
```go
var transactionColumnList = []string{
	"id",
	"parent_transaction_id",
	"description",
	"status",
	"status_description",
	"amount",
	"asset_code",
	"chart_of_accounts_group_name",
	"ledger_id",
	"organization_id",
	"body",
	"created_at",
	"updated_at",
	"deleted_at",
	"route",
}
```

Replace with:
```go
var transactionColumnList = []string{
	"id",
	"parent_transaction_id",
	"description",
	"status",
	"status_description",
	"amount",
	"asset_code",
	"chart_of_accounts_group_name",
	"ledger_id",
	"organization_id",
	"body",
	"created_at",
	"updated_at",
	"deleted_at",
	"route",
	"balance_status",
}
```

**Step 2: Add balance_status to transactionColumnListPrefixed**

Find `transactionColumnListPrefixed` (around line 54) and add `"t.balance_status"` at the end:

Find:
```go
var transactionColumnListPrefixed = []string{
	"t.id",
	"t.parent_transaction_id",
	"t.description",
	"t.status",
	"t.status_description",
	"t.amount",
	"t.asset_code",
	"t.chart_of_accounts_group_name",
	"t.ledger_id",
	"t.organization_id",
	"t.body",
	"t.created_at",
	"t.updated_at",
	"t.deleted_at",
	"t.route",
}
```

Replace with:
```go
var transactionColumnListPrefixed = []string{
	"t.id",
	"t.parent_transaction_id",
	"t.description",
	"t.status",
	"t.status_description",
	"t.amount",
	"t.asset_code",
	"t.chart_of_accounts_group_name",
	"t.ledger_id",
	"t.organization_id",
	"t.body",
	"t.created_at",
	"t.updated_at",
	"t.deleted_at",
	"t.route",
	"t.balance_status",
}
```

**Step 3: Verify repository compiles**

Run:
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./components/transaction/internal/adapters/postgres/transaction/...
```

**Expected output:**
```
(no output - success)
```

---

### Task 6: Update Create method to include BalanceStatus

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/postgres/transaction/transaction.postgresql.go`

**Prerequisites:**
- Task 5 completed

**Step 1: Update Create INSERT statement**

Find the Create method INSERT statement (around line 155). Update it to include balance_status:

Find:
```go
	result, err := executor.ExecContext(ctx, `INSERT INTO transaction VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15) ON CONFLICT (id) DO NOTHING`,
		record.ID,
		record.ParentTransactionID,
		record.Description,
		record.Status,
		record.StatusDescription,
		record.Amount,
		record.AssetCode,
		record.ChartOfAccountsGroupName,
		record.LedgerID,
		record.OrganizationID,
		record.Body,
		record.CreatedAt,
		record.UpdatedAt,
		record.DeletedAt,
		record.Route,
	)
```

Replace with:
```go
	result, err := executor.ExecContext(ctx, `INSERT INTO transaction VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16) ON CONFLICT (id) DO NOTHING`,
		record.ID,
		record.ParentTransactionID,
		record.Description,
		record.Status,
		record.StatusDescription,
		record.Amount,
		record.AssetCode,
		record.ChartOfAccountsGroupName,
		record.LedgerID,
		record.OrganizationID,
		record.Body,
		record.CreatedAt,
		record.UpdatedAt,
		record.DeletedAt,
		record.Route,
		record.BalanceStatus,
	)
```

**Step 2: Verify repository compiles**

Run:
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./components/transaction/internal/adapters/postgres/transaction/...
```

**Expected output:**
```
(no output - success)
```

---

### Task 7: Update Scan methods to include BalanceStatus

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/postgres/transaction/transaction.postgresql.go`

**Prerequisites:**
- Task 6 completed

**Step 1: Update scanTransactionRows**

Find the scanTransactionRows function (around line 296). Add BalanceStatus to the Scan:

Find the rows.Scan call and add `&transaction.BalanceStatus` at the end:

```go
		if err := rows.Scan(
			&transaction.ID,
			&transaction.ParentTransactionID,
			&transaction.Description,
			&transaction.Status,
			&transaction.StatusDescription,
			&transaction.Amount,
			&transaction.AssetCode,
			&transaction.ChartOfAccountsGroupName,
			&transaction.LedgerID,
			&transaction.OrganizationID,
			&body,
			&transaction.CreatedAt,
			&transaction.UpdatedAt,
			&transaction.DeletedAt,
			&transaction.Route,
			&transaction.BalanceStatus,
		); err != nil {
```

**Step 2: Update fetchExistingTransaction Scan**

Find the fetchExistingTransaction method (around line 210). Add BalanceStatus to the Scan:

```go
	if scanErr := row.Scan(
		&existing.ID,
		&existing.ParentTransactionID,
		&existing.Description,
		&existing.Status,
		&existing.StatusDescription,
		&existing.Amount,
		&existing.AssetCode,
		&existing.ChartOfAccountsGroupName,
		&existing.LedgerID,
		&existing.OrganizationID,
		&body,
		&existing.CreatedAt,
		&existing.UpdatedAt,
		&existing.DeletedAt,
		&existing.Route,
		&existing.BalanceStatus,
	); scanErr != nil {
```

**Step 3: Update ListByIDs Scan**

Find the ListByIDs method Scan (around line 524). Add BalanceStatus:

```go
		if err := rows.Scan(
			&transaction.ID,
			&transaction.ParentTransactionID,
			&transaction.Description,
			&transaction.Status,
			&transaction.StatusDescription,
			&transaction.Amount,
			&transaction.AssetCode,
			&transaction.ChartOfAccountsGroupName,
			&transaction.LedgerID,
			&transaction.OrganizationID,
			&body,
			&transaction.CreatedAt,
			&transaction.UpdatedAt,
			&transaction.DeletedAt,
			&transaction.Route,
			&transaction.BalanceStatus,
		); err != nil {
```

**Step 4: Update Find Scan**

Find the Find method Scan (around line 615). Add BalanceStatus:

```go
	if err := row.Scan(
		&transaction.ID,
		&transaction.ParentTransactionID,
		&transaction.Description,
		&transaction.Status,
		&transaction.StatusDescription,
		&transaction.Amount,
		&transaction.AssetCode,
		&transaction.ChartOfAccountsGroupName,
		&transaction.LedgerID,
		&transaction.OrganizationID,
		&body,
		&transaction.CreatedAt,
		&transaction.UpdatedAt,
		&transaction.DeletedAt,
		&transaction.Route,
		&transaction.BalanceStatus,
	); err != nil {
```

**Step 5: Update FindByParentID Scan**

Find the FindByParentID method Scan (around line 705). Add BalanceStatus:

```go
	if err := row.Scan(
		&transaction.ID,
		&transaction.ParentTransactionID,
		&transaction.Description,
		&transaction.Status,
		&transaction.StatusDescription,
		&transaction.Amount,
		&transaction.AssetCode,
		&transaction.ChartOfAccountsGroupName,
		&transaction.LedgerID,
		&transaction.OrganizationID,
		&body,
		&transaction.CreatedAt,
		&transaction.UpdatedAt,
		&transaction.DeletedAt,
		&transaction.Route,
		&transaction.BalanceStatus,
	); err != nil {
```

**Step 6: Verify repository compiles**

Run:
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./components/transaction/internal/adapters/postgres/transaction/...
```

**Expected output:**
```
(no output - success)
```

**If Task Fails:**

1. **Column count mismatch:**
   - Check: Ensure column lists match Scan parameters
   - Fix: Count columns in both lists and Scan calls

---

### Task 8: Add UpdateBalanceStatus method to Repository

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/postgres/transaction/transaction.postgresql.go`

**Prerequisites:**
- Task 7 completed

**Step 1: Add interface method to Repository**

Find the Repository interface (around line 78). Add UpdateBalanceStatus method:

After line 85 (`Delete` method), add:
```go
	UpdateBalanceStatus(ctx context.Context, organizationID, ledgerID, id uuid.UUID, status string) error
```

**Step 2: Add ValidateBalanceStatus helper function**

Before the UpdateBalanceStatus method, add a validation helper:

```go
// ValidateBalanceStatus validates that the status is a valid BalanceStatus value.
// Returns error if status is not PENDING, CONFIRMED, or FAILED.
func ValidateBalanceStatus(status string) error {
	switch status {
	case constant.BalanceStatusPending, constant.BalanceStatusConfirmed, constant.BalanceStatusFailed:
		return nil
	default:
		return fmt.Errorf("invalid balance status: %q (must be PENDING, CONFIRMED, or FAILED)", status)
	}
}
```

**Step 3: Add UpdateBalanceStatus implementation with validation and state machine**

After the Delete method (around line 887), add the new method:

```go
// UpdateBalanceStatus updates the balance_status column for a transaction.
// Used by async processing to mark transactions as CONFIRMED or FAILED.
//
// State machine transitions enforced:
//   - NULL/PENDING -> CONFIRMED (success)
//   - NULL/PENDING -> FAILED (DLQ after max retries)
//   - FAILED -> PENDING (manual retry - future feature)
// Invalid transitions (CONFIRMED -> anything) are silently ignored (idempotent).
func (r *TransactionPostgreSQLRepository) UpdateBalanceStatus(ctx context.Context, organizationID, ledgerID, id uuid.UUID, status string) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.update_transaction_balance_status")
	defer span.End()

	// Input validation - defense in depth (DB has CHECK constraint too)
	if err := ValidateBalanceStatus(status); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Invalid balance status", err)
		logger.Errorf("Invalid balance status: %v", err)
		return pkg.ValidateBusinessError(err, "BalanceStatus")
	}

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)
		logger.Errorf("Failed to get database connection: %v", err)
		return pkg.ValidateInternalError(err, "Transaction")
	}

	ctx, spanExec := tracer.Start(ctx, "postgres.update_balance_status.exec")
	defer spanExec.End()

	// State machine: Only update if transition is valid
	// Valid: NULL->CONFIRMED, NULL->FAILED, PENDING->CONFIRMED, PENDING->FAILED, FAILED->PENDING
	// Invalid (idempotent): CONFIRMED->anything (already terminal)
	result, err := db.ExecContext(ctx,
		`UPDATE transaction SET balance_status = $1, updated_at = now()
		 WHERE organization_id = $2 AND ledger_id = $3 AND id = $4
		 AND deleted_at IS NULL
		 AND (
		   balance_status IS NULL
		   OR balance_status = 'PENDING'
		   OR (balance_status = 'FAILED' AND $1 = 'PENDING')
		 )`,
		status, organizationID, ledgerID, id)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanExec, "Failed to update balance_status", err)
		logger.Errorf("Failed to update balance_status: %v", err)
		return pkg.ValidateInternalError(err, "Transaction")
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)
		logger.Errorf("Failed to get rows affected: %v", err)
		return pkg.ValidateInternalError(err, "Transaction")
	}

	// Audit log for compliance (PCI-DSS, SOX)
	logger.WithFields(map[string]interface{}{
		"transaction_id":  id,
		"organization_id": organizationID,
		"ledger_id":       ledgerID,
		"new_status":      status,
		"rows_affected":   rowsAffected,
	}).Info("balance_status_transition")

	if rowsAffected == 0 {
		// Could be: not found, already in target state (idempotent), or invalid transition
		// Check if transaction exists to distinguish
		var exists bool
		_ = db.QueryRowContext(ctx,
			`SELECT EXISTS(SELECT 1 FROM transaction WHERE organization_id = $1 AND ledger_id = $2 AND id = $3 AND deleted_at IS NULL)`,
			organizationID, ledgerID, id).Scan(&exists)

		if !exists {
			err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, reflect.TypeOf(Transaction{}).Name())
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Transaction not found for balance_status update", err)
			logger.Warnf("Transaction not found for balance_status update: %v", err)
			return err
		}

		// Transaction exists but no update - likely already CONFIRMED (idempotent case)
		logger.Infof("No balance_status update needed for transaction %s (already in terminal state or same state)", id)
		return nil
	}

	logger.Infof("Updated balance_status to %s for transaction %s", status, id)
	return nil
}
```

**Step 4: Verify repository compiles**

Run:
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./components/transaction/internal/adapters/postgres/transaction/...
```

**Expected output:**
```
(no output - success)
```

---

### Task 9: Run Code Review for Phase 1

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

**Cosmetic/Nitpick Issues:**
- Add `FIXME(nitpick):` comments in code at the relevant location

3. **Commit Phase 1 changes:**

Run:
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && git add -A && git commit -m "feat(transaction): add balance_status column for saga-like consistency

- Add balance_status column (PENDING/CONFIRMED/FAILED) to transaction table
- Add BalanceStatus constants to pkg/constant
- Update Transaction model in mmodel and PostgreSQL adapter
- Add UpdateBalanceStatus repository method
- Add indexes for efficient status queries

Part of hybrid transaction consistency implementation."
```

---

## Phase 2: Retry Configuration Changes

### Task 10: Increase maxRetries constant

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/rabbitmq/consumer.rabbitmq.go`

**Prerequisites:**
- Phase 1 completed

**Step 1: Update maxRetries constant**

Find the maxRetries constant (line 33):

Find:
```go
// maxRetries is the maximum number of delivery attempts (including first delivery)
// before rejecting as a poison message to prevent infinite retry loops.
// Set to 5 to allow 4 retries with backoff delays: 0s, 5s, 15s, 30s (50s total).
const maxRetries = 5
```

Replace with:
```go
// maxRetries is the maximum number of delivery attempts (including first delivery)
// before routing to DLQ. Increased from 5 to 288 to span ~24 hours with
// capped exponential backoff (max 5 minutes between attempts).
// This supports the "201 promise" - once client gets 201, we keep trying
// until infrastructure recovers (typically 1-30 minutes for DB/Redis restarts).
const maxRetries = 288
```

**Step 2: Update retryBackoffDelays**

Find the retryBackoffDelays slice (around line 80):

Find:
```go
// Retry backoff delays - designed to span ~50 seconds total
// to cover typical PostgreSQL restart times (10-30s)
var retryBackoffDelays = []time.Duration{
	0,                // Retry 1 (attempt 2): immediate
	5 * time.Second,  // Retry 2 (attempt 3): 5s delay
	15 * time.Second, // Retry 3 (attempt 4): 15s delay
	30 * time.Second, // Retry 4 (attempt 5): 30s delay
}
```

Replace with:
```go
// Retry backoff delays - exponential backoff capped at 5 minutes
// Spans ~24 hours to handle extended infrastructure outages while
// supporting the "201 promise" for transaction consistency.
// Pattern: 0s, 5s, 15s, 30s, 1m, 2m, 5m, 5m, 5m... (capped at 5m)
var retryBackoffDelays = []time.Duration{
	0,                 // Retry 1 (attempt 2): immediate
	5 * time.Second,   // Retry 2 (attempt 3): 5s delay
	15 * time.Second,  // Retry 3 (attempt 4): 15s delay
	30 * time.Second,  // Retry 4 (attempt 5): 30s delay
	1 * time.Minute,   // Retry 5 (attempt 6): 1m delay
	2 * time.Minute,   // Retry 6 (attempt 7): 2m delay
	5 * time.Minute,   // Retry 7+ (attempt 8+): capped at 5m
}
```

**Step 3: Verify consumer compiles**

Run:
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./components/transaction/internal/adapters/rabbitmq/...
```

**Expected output:**
```
(no output - success)
```

---

### Task 11: Add DLQ alerting metric for balance_status

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/rabbitmq/metrics.go`

**Prerequisites:**
- Task 10 completed

**Step 1: Add new metric for balance status failures**

After the messageRetryMetric (around line 42), add:

```go
	balanceStatusFailedMetric = metrics.Metric{
		Name:        "transaction_balance_status_failed_total",
		Unit:        "1",
		Description: "Total number of transactions with balance_status=FAILED (DLQ)",
	}
```

**Step 2: Add RecordBalanceStatusFailed method**

After RecordMessageRetry method (around line 140), add:

```go
// RecordBalanceStatusFailed increments the transaction_balance_status_failed_total counter.
// This is called when a transaction's balance update fails after max retries and is marked FAILED.
func (dm *DLQMetrics) RecordBalanceStatusFailed(ctx context.Context, queue, transactionID string) {
	if dm == nil || dm.factory == nil {
		return
	}

	dm.factory.Counter(balanceStatusFailedMetric).
		WithLabels(map[string]string{
			"queue":          sanitizeLabelValue(queue),
			"transaction_id": sanitizeLabelValue(transactionID),
		}).
		AddOne(ctx)
}

// recordBalanceStatusFailed is a package-level helper that records a balance status failure.
func recordBalanceStatusFailed(ctx context.Context, queue, transactionID string) {
	dm := GetDLQMetrics()
	if dm != nil {
		dm.RecordBalanceStatusFailed(ctx, queue, transactionID)
	}
}
```

**Step 3: Verify metrics compile**

Run:
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./components/transaction/internal/adapters/rabbitmq/...
```

**Expected output:**
```
(no output - success)
```

---

### Task 12: Commit Phase 2 changes

Run:
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && git add -A && git commit -m "feat(rabbitmq): increase retry tolerance to 24h with capped backoff

- Increase maxRetries from 5 to 288 (~24 hours)
- Update backoff delays: 0s, 5s, 15s, 30s, 1m, 2m, 5m (capped)
- Add balance_status_failed metric for observability
- Supports 201 promise: keep retrying until infrastructure recovers

Part of hybrid transaction consistency implementation."
```

---

## Phase 3: Async Flow Integration

### Task 13: Add idempotency check and set PENDING status

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/create-balance-transaction-operations-async.go`

**Prerequisites:**
- Phase 2 completed
- Read the file to understand current structure

**Why idempotency matters:** RabbitMQ can redeliver messages (heartbeat timeout, network partition). Without idempotency, two workers could both apply balance updates → **double balance effect** (double-credit/debit).

**Step 1: Import required packages**

Verify these imports are present:
```go
import (
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	libPointers "github.com/LerianStudio/lib-commons/v2/commons/pointers"
)
```

**Step 2: Add idempotency check at the start of processing**

At the beginning of `CreateBalanceTransactionOperationsAsync`, after extracting the transaction ID, add:

**CRITICAL:** Balance updates and transaction creation happen in the **same database transaction**. If a transaction exists (regardless of status), the balance was already applied. We must skip ALL processing if transaction exists.

```go
func (uc *UseCase) CreateBalanceTransactionOperationsAsync(ctx context.Context, data mmodel.Queue) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)
	// ... existing setup code ...

	// Extract transaction ID from queue data
	transactionID := data.QueueData[0].ID // Adjust based on actual structure

	// IDEMPOTENCY CHECK: If transaction EXISTS, balance was already applied (atomic DB txn)
	// We only need to ensure status is CONFIRMED if it was left as PENDING
	existing, err := uc.TransactionRepo.Find(ctx, data.OrganizationID, data.LedgerID, transactionID)
	if err == nil {
		// Transaction exists = balance already applied in same DB transaction
		// Just ensure status is updated to CONFIRMED (may have failed on prior attempt)
		currentStatus := ""
		if existing.BalanceStatus != nil {
			currentStatus = *existing.BalanceStatus
		}

		if currentStatus == "" || currentStatus == constant.BalanceStatusPending {
			// Status needs to be updated to CONFIRMED
			if statusErr := uc.TransactionRepo.UpdateBalanceStatus(
				ctx, data.OrganizationID, data.LedgerID, transactionID,
				constant.BalanceStatusConfirmed,
			); statusErr != nil {
				logger.Warnf("Failed to update balance_status to CONFIRMED on retry for %s: %v", transactionID, statusErr)
			} else {
				logger.Infof("Updated balance_status to CONFIRMED on retry for %s", transactionID)
			}
		}

		logger.Infof("Transaction %s already exists (status=%s), skipping duplicate message (idempotent)",
			transactionID, currentStatus)
		return nil // SKIP ALL PROCESSING - balance was already applied
	}
	// Transaction not found - this is first-time processing, continue

	// ... rest of existing logic ...
}
```

**Step 3: Set balance_status=PENDING in transaction creation**

Find where the transaction is created (search for `TransactionRepo.Create`). In the transaction struct being created, add:

```go
BalanceStatus: libPointers.String(constant.BalanceStatusPending),
```

**Step 4: Verify service compiles**

Run:
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./components/transaction/internal/services/command/...
```

**Expected output:**
```
(no output - success)
```

---

### Task 14: Set CONFIRMED status on successful balance update (with retry)

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/create-balance-transaction-operations-async.go`

**Prerequisites:**
- Task 13 completed

**Why retry matters:** If balance is updated but CONFIRMED status fails, clients see PENDING. They might retry, causing **duplicate balance effects** (double-credit/debit). This is NOT acceptable in a financial ledger.

**Step 1: Update balance_status after successful balance update with retry**

After the balance update succeeds (after `UpdateBalances` returns nil), add a call to update the status with retry logic:

```go
// Mark transaction as CONFIRMED after successful balance update
// CRITICAL: Must succeed to prevent client confusion and duplicate transactions
var statusErr error
for attempts := 0; attempts < 3; attempts++ {
	statusErr = uc.TransactionRepo.UpdateBalanceStatus(ctx, message.OrganizationID, message.LedgerID, transactionID, constant.BalanceStatusConfirmed)
	if statusErr == nil {
		break
	}
	logger.Warnf("Attempt %d/3: Failed to update balance_status to CONFIRMED for transaction %s: %v", attempts+1, transactionID, statusErr)
	if attempts < 2 {
		time.Sleep(time.Duration(attempts+1) * 100 * time.Millisecond) // 100ms, 200ms backoff
	}
}

if statusErr != nil {
	// After 3 attempts, log critical error but don't fail the transaction
	// The balance IS updated, status is cosmetic but important for client visibility
	logger.Errorf("CRITICAL: Failed to update balance_status to CONFIRMED for transaction %s after 3 attempts: %v", transactionID, statusErr)
	// Emit metric for alerting
	metrics.RecordBalanceStatusUpdateFailure(ctx, "CONFIRMED", transactionID.String())
}
```

**Step 2: Add metrics function to metrics.go**

**File:** `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/rabbitmq/metrics.go`

Add this metric definition after `balanceStatusFailedMetric` (around line 45):

```go
	balanceStatusUpdateFailedMetric = metrics.Metric{
		Name:        "transaction_balance_status_update_failed_total",
		Unit:        "1",
		Description: "Total number of balance_status update failures after retries",
	}
```

Add this function after `RecordBalanceStatusFailed` (around line 120):

```go
// RecordBalanceStatusUpdateFailure records when a status update fails after retries.
// Uses structured logging for alerting systems (Datadog, PagerDuty, etc).
// Note: transaction_id is logged, not in Prometheus label (avoids high cardinality).
func RecordBalanceStatusUpdateFailure(ctx context.Context, targetStatus, transactionID string) {
	dm := GetDLQMetrics()
	if dm != nil && dm.factory != nil {
		dm.factory.Counter(balanceStatusUpdateFailedMetric).
			WithLabels(map[string]string{
				"target_status": targetStatus,
			}).
			AddOne(ctx)
	}

	// Structured log for alerting - transaction_id here (not in metric)
	logger := mlog.NewLoggerFromContext(ctx)
	logger.WithFields(map[string]interface{}{
		"event":          "balance_status_update_failure",
		"target_status":  targetStatus,
		"transaction_id": transactionID,
		"severity":       "critical",
	}).Error("Balance status update failed after retries")
}
```

**Why these specific values for retry backoff (100ms, 200ms)?**
- First retry (100ms): Allows DB connection pool to recover from transient issues
- Second retry (200ms): Slightly longer to handle brief network hiccups
- Total max wait: 300ms - fast enough to not significantly delay message processing
- These values are intentionally short because the balance IS already updated; status is visibility-only

**Step 3: Verify service compiles**

Run:
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./components/transaction/internal/services/command/...
```

**Expected output:**
```
(no output - success)
```

---

### Task 15: Set FAILED status when routing to DLQ

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/rabbitmq/consumer.rabbitmq.go`
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/bootstrap/rabbitmq.server.go`

**Prerequisites:**
- Task 14 completed

**Why this matters:** Without updating status to FAILED, transactions stay PENDING forever after DLQ routing. Clients cannot distinguish "still processing" from "permanently failed". This breaks the visibility promise.

**Step 1: Add TransactionRepo to Consumer struct**

In `consumer.rabbitmq.go`, add the repository to the Consumer struct:

Find the Consumer struct definition and add:
```go
type Consumer struct {
	// ... existing fields ...
	TransactionRepo transaction.Repository // Add this field
}
```

**Step 2: Update NewConsumer constructor**

Add the repository parameter to the NewConsumer function:

```go
func NewConsumer(
	conn *amqp.Connection,
	logger mlog.Logger,
	txRepo transaction.Repository, // Add this parameter
	// ... other params ...
) *Consumer {
	return &Consumer{
		// ... existing fields ...
		TransactionRepo: txRepo,
	}
}
```

**Step 3: Update bootstrap to pass repository**

In `rabbitmq.server.go`, where Consumer is created, pass the TransactionRepo:

```go
consumer := rabbitmq.NewConsumer(
	conn,
	logger,
	transactionRepo, // Add this
	// ... other params ...
)
```

**Step 4: Implement FAILED status update in DLQ routing**

Find the `routeToDLQIfMaxRetries` method (around line 449). After the decision to route to DLQ, add:

```go
func (c *Consumer) routeToDLQIfMaxRetries(ctx context.Context, delivery amqp.Delivery, queueName string, retryCount int) bool {
	if retryCount < maxRetries {
		return false // Not yet at max retries
	}

	logger := c.logger.WithFields(map[string]interface{}{
		"queue":       queueName,
		"retry_count": retryCount,
	})

	// Extract transaction info from message body and update status to FAILED
	c.markTransactionsAsFailed(ctx, delivery.Body, queueName)

	// ... existing DLQ routing logic ...
	return true
}

// markTransactionsAsFailed extracts transaction IDs from message and marks them as FAILED.
// Best-effort: failures are logged but don't block DLQ routing.
func (c *Consumer) markTransactionsAsFailed(ctx context.Context, body []byte, queueName string) {
	if c.TransactionRepo == nil {
		c.logger.Warn("TransactionRepo not configured, cannot mark transactions as FAILED")
		return
	}

	// Deserialize the message to extract transaction info
	var msg mmodel.Queue
	if err := msgpack.Unmarshal(body, &msg); err != nil {
		c.logger.Warnf("Failed to unmarshal message for FAILED status update: %v", err)
		return
	}

	// Update each transaction in the queue data
	for _, item := range msg.QueueData {
		if item.ID == uuid.Nil {
			continue
		}

		if err := c.TransactionRepo.UpdateBalanceStatus(ctx, msg.OrganizationID, msg.LedgerID, item.ID, constant.BalanceStatusFailed); err != nil {
			c.logger.Warnf("Failed to mark transaction %s as FAILED: %v (best-effort)", item.ID, err)
		} else {
			c.logger.Infof("Marked transaction %s as FAILED (routed to DLQ)", item.ID)
		}

		// Record metric for alerting
		recordBalanceStatusFailed(ctx, queueName, item.ID.String())
	}
}
```

**Step 5: Add required imports**

Ensure these imports are present in `consumer.rabbitmq.go`:
```go
import (
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/vmihailenco/msgpack/v5"
)
```

**Step 6: Verify all bootstrap paths wire TransactionRepo correctly**

Before compiling, verify all Consumer instantiation points:

```bash
# Find all places where Consumer is created
rg "NewConsumer\(" components/transaction/ --type go -C 3
```

**Expected:** All usages pass `transactionRepo` as parameter. If any are missing, update them.

**Common bootstrap locations:**
- `components/transaction/internal/bootstrap/rabbitmq.server.go`
- Any test files that mock the consumer

**Step 7: Verify consumer compiles**

Run:
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./components/transaction/internal/adapters/rabbitmq/...
cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./components/transaction/internal/bootstrap/...
```

**Expected output:**
```
(no output - success)
```

**If compilation fails due to constructor signature change:**
Update all usages found in Step 6 to pass the TransactionRepo.

---

### Task 16: Run Code Review for Phase 3

1. **Dispatch all 3 reviewers in parallel:**
   - REQUIRED SUB-SKILL: Use requesting-code-review
   - All reviewers run simultaneously

2. **Handle findings by severity as documented above**

3. **Commit Phase 3 changes:**

Run:
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && git add -A && git commit -m "feat(transaction): integrate balance_status into async flow

- Add idempotency check to prevent duplicate balance effects
- Set balance_status=PENDING when creating async transaction
- Update to CONFIRMED on successful balance update (with retry)
- Mark as FAILED when routing to DLQ after max retries
- Add TransactionRepo dependency to RabbitMQ consumer

Part of hybrid transaction consistency implementation."
```

---

## Phase 4: Testing and Verification

### Task 17: Add unit tests for ValidateBalanceStatus and UpdateBalanceStatus

**Files:**
- Modify or Create: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/postgres/transaction/transaction.postgresql_test.go`

**Prerequisites:**
- Phase 3 completed

**Step 1: Add unit test for ValidateBalanceStatus (no DB required)**

```go
func TestValidateBalanceStatus(t *testing.T) {
	tests := []struct {
		name    string
		status  string
		wantErr bool
	}{
		// Valid statuses
		{"valid PENDING", "PENDING", false},
		{"valid CONFIRMED", "CONFIRMED", false},
		{"valid FAILED", "FAILED", false},
		// Invalid statuses
		{"lowercase pending", "pending", true},     // Case sensitive
		{"lowercase confirmed", "confirmed", true},
		{"empty string", "", true},
		{"random string", "INVALID", true},
		{"partial match", "PEND", true},
		{"with spaces", " PENDING", true},
		{"sql injection attempt", "PENDING'; DROP TABLE--", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateBalanceStatus(tt.status)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateBalanceStatus(%q) error = %v, wantErr %v", tt.status, err, tt.wantErr)
			}
		})
	}
}
```

**Step 2: Add integration test stubs for UpdateBalanceStatus**

```go
func TestUpdateBalanceStatus_Success(t *testing.T) {
	// Setup: Create a transaction with PENDING status
	// Act: Update to CONFIRMED
	// Assert: Status is updated, updated_at is changed
	t.Skip("Test requires database integration setup - implement in integration test suite")
}

func TestUpdateBalanceStatus_NotFound(t *testing.T) {
	// Setup: Use non-existent transaction ID
	// Act: Attempt to update status
	// Assert: Returns ErrEntityNotFound
	t.Skip("Test requires database integration setup - implement in integration test suite")
}

func TestUpdateBalanceStatus_InvalidTransition(t *testing.T) {
	// Setup: Create a transaction with CONFIRMED status
	// Act: Attempt to update to FAILED
	// Assert: No rows affected (idempotent, no error)
	t.Skip("Test requires database integration setup - implement in integration test suite")
}
```

**Step 3: Run tests**

Run:
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./components/transaction/internal/adapters/postgres/transaction/... -v -run "ValidateBalanceStatus|UpdateBalanceStatus" -count=1
```

**Expected output:**
```
=== RUN   TestValidateBalanceStatus
=== RUN   TestValidateBalanceStatus/valid_PENDING
=== RUN   TestValidateBalanceStatus/valid_CONFIRMED
...
--- PASS: TestValidateBalanceStatus (0.00s)
--- SKIP: TestUpdateBalanceStatus_Success
--- SKIP: TestUpdateBalanceStatus_NotFound
--- SKIP: TestUpdateBalanceStatus_InvalidTransition
PASS
```

---

### Task 18: Add unit test for retry backoff

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/rabbitmq/consumer.rabbitmq_test.go` (if exists) or create new file

**Prerequisites:**
- Task 17 completed

**Step 1: Add test for calculateRetryBackoff**

```go
func TestCalculateRetryBackoff_Extended(t *testing.T) {
	tests := []struct {
		name        string
		retryCount  int
		wantAtLeast time.Duration
		wantAtMost  time.Duration
	}{
		{"first retry immediate", 1, 0, 0},
		{"second retry 5s", 2, 5 * time.Second, 5 * time.Second},
		{"third retry 15s", 3, 15 * time.Second, 15 * time.Second},
		{"fourth retry 30s", 4, 30 * time.Second, 30 * time.Second},
		{"fifth retry 1m", 5, 1 * time.Minute, 1 * time.Minute},
		{"sixth retry 2m", 6, 2 * time.Minute, 2 * time.Minute},
		{"seventh retry capped at 5m", 7, 5 * time.Minute, 5 * time.Minute},
		{"hundredth retry still 5m", 100, 5 * time.Minute, 5 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateRetryBackoff(tt.retryCount)
			if got < tt.wantAtLeast || got > tt.wantAtMost {
				t.Errorf("calculateRetryBackoff(%d) = %v, want between %v and %v",
					tt.retryCount, got, tt.wantAtLeast, tt.wantAtMost)
			}
		})
	}
}
```

**Step 2: Run tests**

Run:
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./components/transaction/internal/adapters/rabbitmq/... -v -run TestCalculateRetryBackoff -count=1
```

**Expected output:**
```
=== RUN   TestCalculateRetryBackoff_Extended
--- PASS: TestCalculateRetryBackoff_Extended
PASS
```

---

### Task 19: Run full test suite

**Prerequisites:**
- Task 18 completed

**Step 1: Run all unit tests**

Run:
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./... -count=1 -timeout 5m 2>&1 | tail -50
```

**Expected output:**
```
ok  	github.com/LerianStudio/midaz/v3/...
...
PASS
```

**Step 2: Run linter**

Run:
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && make lint 2>&1 | tail -20
```

**Expected output:**
```
(no errors or only expected warnings)
```

---

### Task 20: Final commit and summary

**Step 1: Commit tests**

Run:
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && git add -A && git commit -m "test(transaction): add tests for balance_status and retry backoff

- Add skipped integration tests for UpdateBalanceStatus
- Add unit tests for extended retry backoff

Part of hybrid transaction consistency implementation."
```

**Step 2: Verify all commits**

Run:
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && git log --oneline -5
```

**Expected output:**
```
XXXXXXX test(transaction): add tests for balance_status and retry backoff
XXXXXXX feat(transaction): integrate balance_status into async flow
XXXXXXX feat(rabbitmq): increase retry tolerance to 24h with capped backoff
XXXXXXX feat(transaction): add balance_status column for saga-like consistency
...
```

---

## Phase 5: Reconciliation Job (Recovery Mechanism)

### Task 21: Add background reconciliation job for stuck PENDING transactions

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/reconcile-pending-transactions.go`
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/bootstrap/service.go` (or appropriate bootstrap)

**Prerequisites:**
- Phase 4 completed

**Why this matters:** If CONFIRMED status updates fail repeatedly (DB down during status update window), transactions stay PENDING forever despite balances being updated. This job provides a safety net.

**Step 1: Add FindOrphanedPending method to Repository**

In `transaction.postgresql.go`, add:

```go
// FindOrphanedPending returns transactions with balance_status=PENDING that are older than the threshold.
// These are candidates for reconciliation (either mark CONFIRMED if balance exists, or FAILED if not).
func (r *TransactionPostgreSQLRepository) FindOrphanedPending(ctx context.Context, olderThan time.Duration, limit int) ([]*Transaction, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_orphaned_pending")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		return nil, pkg.ValidateInternalError(err, "Transaction")
	}

	threshold := time.Now().Add(-olderThan)

	rows, err := db.QueryContext(ctx, `
		SELECT `+strings.Join(transactionColumnList, ", ")+`
		FROM transaction
		WHERE balance_status = 'PENDING'
		  AND created_at < $1
		  AND deleted_at IS NULL
		ORDER BY created_at ASC
		LIMIT $2
	`, threshold, limit)
	if err != nil {
		return nil, pkg.ValidateInternalError(err, "Transaction")
	}
	defer rows.Close()

	return scanTransactionRows(rows, logger)
}
```

**Step 2: Create reconciliation service**

Create `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/reconcile-pending-transactions.go`:

```go
package command

import (
	"context"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mlog"
)

// ReconcilePendingTransactionsConfig configures the reconciliation job.
type ReconcilePendingTransactionsConfig struct {
	// ThresholdAge is how old a PENDING transaction must be to be considered orphaned.
	// Should be > 24h (the retry window) + buffer.
	ThresholdAge time.Duration
	// BatchSize limits how many transactions to process per run.
	BatchSize int
}

// DefaultReconcileConfig returns sensible defaults for reconciliation.
func DefaultReconcileConfig() ReconcilePendingTransactionsConfig {
	return ReconcilePendingTransactionsConfig{
		ThresholdAge: 25 * time.Hour, // 24h retry window + 1h buffer
		BatchSize:    100,
	}
}

// ReconcilePendingTransactions finds and fixes orphaned PENDING transactions.
// This should be run periodically (e.g., every hour via cron/scheduler).
func (uc *UseCase) ReconcilePendingTransactions(ctx context.Context, config ReconcilePendingTransactionsConfig) error {
	logger := mlog.NewLoggerFromContext(ctx)

	orphaned, err := uc.TransactionRepo.FindOrphanedPending(ctx, config.ThresholdAge, config.BatchSize)
	if err != nil {
		logger.Errorf("Failed to find orphaned PENDING transactions: %v", err)
		return err
	}

	if len(orphaned) == 0 {
		logger.Debug("No orphaned PENDING transactions found")
		return nil
	}

	logger.Infof("Found %d orphaned PENDING transactions to reconcile", len(orphaned))

	var reconciled, failed int
	for _, tx := range orphaned {
		// For each orphaned transaction, check if operations exist
		// If operations exist -> balance was applied -> mark CONFIRMED
		// If no operations -> something went wrong -> mark FAILED
		ops, err := uc.OperationRepo.FindAllByTransactionID(ctx, tx.OrganizationID, tx.LedgerID, tx.ID)
		if err != nil {
			logger.Warnf("Failed to check operations for orphaned tx %s: %v", tx.ID, err)
			continue
		}

		var targetStatus string
		if len(ops) > 0 {
			// Operations exist = balance was applied = should be CONFIRMED
			targetStatus = constant.BalanceStatusConfirmed
		} else {
			// No operations = transaction never completed = FAILED
			targetStatus = constant.BalanceStatusFailed
		}

		if err := uc.TransactionRepo.UpdateBalanceStatus(ctx, tx.OrganizationID, tx.LedgerID, tx.ID, targetStatus); err != nil {
			logger.Warnf("Failed to reconcile orphaned tx %s to %s: %v", tx.ID, targetStatus, err)
			failed++
		} else {
			logger.Infof("Reconciled orphaned tx %s: PENDING -> %s", tx.ID, targetStatus)
			reconciled++
		}
	}

	logger.WithFields(map[string]interface{}{
		"total_found": len(orphaned),
		"reconciled":  reconciled,
		"failed":      failed,
	}).Info("Reconciliation job completed")

	return nil
}
```

**Step 3: Add interface method**

In the Repository interface, add:
```go
FindOrphanedPending(ctx context.Context, olderThan time.Duration, limit int) ([]*Transaction, error)
```

**Step 4: Wire up as scheduled job (implementation depends on your scheduler)**

Example using a simple ticker (add to bootstrap or separate worker):

```go
// In main.go or worker bootstrap
go func() {
	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for range ticker.C {
		ctx := context.Background()
		config := command.DefaultReconcileConfig()
		if err := useCase.ReconcilePendingTransactions(ctx, config); err != nil {
			logger.Errorf("Reconciliation job failed: %v", err)
		}
	}
}()
```

**Step 5: Verify compilation**

Run:
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./components/transaction/...
```

**Step 6: Commit reconciliation job**

Run:
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && git add -A && git commit -m "feat(transaction): add reconciliation job for stuck PENDING transactions

- Add FindOrphanedPending repository method
- Add ReconcilePendingTransactions service method
- Run hourly to fix transactions where CONFIRMED status update failed
- Uses 25h threshold (24h retry window + 1h buffer)

Part of hybrid transaction consistency implementation."
```

---

## Summary of Changes

### Files Modified

| File | Changes |
|------|---------|
| `components/transaction/migrations/000020_add_balance_status_to_transaction.up.sql` | NEW: Migration adding balance_status column |
| `components/transaction/migrations/000020_add_balance_status_to_transaction.down.sql` | NEW: Rollback migration |
| `pkg/constant/transaction.go` | ADD: BalanceStatus constants (PENDING/CONFIRMED/FAILED) |
| `pkg/mmodel/transaction.go` | ADD: BalanceStatus field to Transaction struct |
| `components/transaction/internal/adapters/postgres/transaction/transaction.go` | ADD: BalanceStatus to PostgreSQL model, ToEntity/FromEntity mappings |
| `components/transaction/internal/adapters/postgres/transaction/transaction.postgresql.go` | ADD: Column lists, Scan updates, ValidateBalanceStatus helper, UpdateBalanceStatus method with state machine |
| `components/transaction/internal/adapters/rabbitmq/consumer.rabbitmq.go` | MOD: Increase maxRetries to 288, extend backoff delays, ADD: TransactionRepo field, markTransactionsAsFailed method for DLQ routing |
| `components/transaction/internal/adapters/rabbitmq/metrics.go` | ADD: balance_status_failed metric, balance_status_update_failed metric, RecordBalanceStatusUpdateFailure function |
| `components/transaction/internal/bootstrap/rabbitmq.server.go` | MOD: Wire TransactionRepo into Consumer constructor |
| `components/transaction/internal/services/command/create-balance-transaction-operations-async.go` | MOD: Add idempotency check, set PENDING on create, CONFIRMED on success with retry |

### API Changes

The existing GET `/v1/organizations/{org}/ledgers/{ledger}/transactions/{id}` endpoint now includes:

```json
{
  "id": "...",
  "balanceStatus": "CONFIRMED"  // NEW: PENDING | CONFIRMED | FAILED | null
}
```

- `PENDING` - Balance update queued but not yet confirmed
- `CONFIRMED` - Balance update completed successfully
- `FAILED` - Balance update failed after max retries (in DLQ)
- `null` - Sync transaction (balance updated synchronously)

### Metrics Added

- `transaction_balance_status_failed_total` - Counter for transactions marked FAILED (DLQ routing)
- `transaction_balance_status_update_failed_total` - Counter for status update failures after retries (with `target_status` label)

### Outstanding Work (Post-Implementation)

1. ~~**DLQ Integration**~~: ✅ DONE - Consumer now marks transactions FAILED when routing to DLQ
2. **Admin API**: Optional endpoint to manually retry FAILED transactions (future feature)
3. **Reconciliation Job**: See Task 21 below - Background job to clean up stuck PENDING transactions

---

## If Task Fails (Global Recovery)

1. **Rollback to last good commit:**
   ```bash
   git log --oneline -10  # Find last good commit
   git reset --hard <commit-hash>
   ```

2. **Rollback migration if applied:**
   ```bash
   cd components/transaction && make migrate-down
   ```

3. **Clean build cache:**
   ```bash
   go clean -cache && go build ./...
   ```
