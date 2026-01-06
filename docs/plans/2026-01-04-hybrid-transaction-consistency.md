# Hybrid Transaction Consistency Implementation Plan

> **For Agents:** REQUIRED SUB-SKILL: Use executing-plans to implement this plan task-by-task.

**Goal:** Implement saga-like balance status tracking with higher retry tolerance to support the "201 promise" - once a client receives 201, the system will retry balance persistence for up to 24 hours before marking as FAILED.

**SLA:** The "201 promise" has a **24-hour SLA**. After 288 retries (~24h), transactions are marked `FAILED` and routed to DLQ for manual intervention. This is a pragmatic balance between guaranteed persistence and operational reality.

**Architecture:** Add a `balance_status` field to transactions that tracks whether async balance updates completed (distinct from the existing transaction lifecycle `status`). Increase retry tolerance from 5 attempts (~50s) to time-based (~24h) with exponential backoff capped at 5 minutes. Update balance_status on success/failure. Expose status via existing transaction GET endpoint only when explicitly requested and authorized (Task 3a).

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
-- NULL for sync transactions (balance updated synchronously)
ALTER TABLE "transaction"
    ADD COLUMN balance_status TEXT
    CHECK (balance_status IS NULL OR balance_status IN ('PENDING', 'CONFIRMED', 'FAILED'));

-- Durable proof that balances were persisted successfully.
-- This prevents reconciliation from guessing based on secondary signals.
ALTER TABLE "transaction"
    ADD COLUMN balance_persisted_at TIMESTAMPTZ NULL;

-- Index for efficient status queries.
-- NOTE: Put created_at first to match queries like:
--   WHERE balance_status='PENDING' AND created_at < $1 ORDER BY created_at ASC
CREATE INDEX idx_transaction_balance_status_pending
    ON "transaction" (created_at, balance_status)
    WHERE balance_status = 'PENDING';

-- Index for failed transactions requiring attention
CREATE INDEX idx_transaction_balance_status_failed
    ON "transaction" (updated_at, balance_status)
    WHERE balance_status = 'FAILED';

COMMENT ON COLUMN "transaction".balance_status IS 'Tracks async balance update state: PENDING=queued, CONFIRMED=completed, FAILED=DLQ. NULL for sync transactions.';
COMMENT ON COLUMN "transaction".balance_persisted_at IS 'Timestamp set only when balances are durably persisted; used as proof for reconciliation.';

-- IMPORTANT: balance_persisted_at is internal-only and must NOT be exposed via public APIs.

COMMIT;
```

**Step 2: Write the DOWN migration**

Create `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/migrations/000020_add_balance_status_to_transaction.down.sql`:

```sql
BEGIN;

DROP INDEX IF EXISTS idx_transaction_balance_status_failed;
DROP INDEX IF EXISTS idx_transaction_balance_status_pending;
ALTER TABLE "transaction" DROP COLUMN IF EXISTS balance_persisted_at;
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
// NOTE: These values intentionally overlap with other status strings (e.g., transaction lifecycle PENDING).
// Always reference them via the BalanceStatus* constants to avoid confusion.
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

### Task 3a: Gate `balanceStatus` exposure (security)

**Goal:** Avoid leaking operational health signals to unprivileged clients.

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/http/in/transaction.go`

**Approach:**
- Default behavior: omit `balanceStatus` (set to `nil` so `omitempty` removes it)
- Opt-in: `GET .../transactions/{transaction_id}?includeBalanceStatus=true`
- Authorization: require explicit scope `transactions:read_balance_status`

**Step 1: Add query param handling**
- Parse `includeBalanceStatus` from query params

**Step 2: Authorization gate (explicit 403)**
- If `includeBalanceStatus=true` and caller lacks `transactions:read_balance_status`, return **HTTP 403**
- If `includeBalanceStatus` is not set, always omit `balanceStatus`

**Step 3: Verify swagger/docs reflect param**
- Add swagger annotation example:
  - `@Param includeBalanceStatus query boolean false "Include balance status (requires transactions:read_balance_status)"`
- Regenerate API docs if the project uses generated swagger artifacts

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
	BalancePersistedAt       *time.Time                  // INTERNAL ONLY: proof timestamp (do not expose in public API)
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

Note: `BalancePersistedAt` should NOT be set from the entity on create; it is written by the repository when setting `CONFIRMED`.

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

**Important:** Do NOT add `balance_persisted_at` to `transactionColumnList` because scan targets are the public `Transaction` model. `balance_persisted_at` is internal-only and should be handled via a dedicated reconciliation query (see Phase 5).

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
	result, err := executor.ExecContext(ctx, `INSERT INTO transaction (
	id,
	parent_transaction_id,
	description,
	status,
	status_description,
	amount,
	asset_code,
	chart_of_accounts_group_name,
	ledger_id,
	organization_id,
	body,
	created_at,
	updated_at,
	deleted_at,
	route,
	balance_status,
	balance_persisted_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
ON CONFLICT (id) DO NOTHING`,
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
		record.BalancePersistedAt,
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

**Step 1: Add interface methods to Repository**

Find the `Repository` interface in `transaction.postgresql.go` (near the other CRUD methods and `FindOrListAllWithOperations`). Add BOTH methods so the concrete repository satisfies all call sites:

```go
	UpdateBalanceStatus(ctx context.Context, organizationID, ledgerID, id uuid.UUID, status string) error
	FindPendingForReconciliation(ctx context.Context, olderThan time.Duration, limit int) ([]PendingBalanceStatusCandidate, error)
```

Note: `FindPendingForReconciliation` is intentionally a narrow internal query because `balance_persisted_at` must not be added to the public scan paths.

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
// Valid transitions:
// - NULL/PENDING -> CONFIRMED (balance persisted; set balance_persisted_at)
// - NULL/PENDING -> FAILED (max retries exceeded, message routed to DLQ)
// - FAILED -> PENDING (manual retry via admin action)
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
		`UPDATE transaction
		 SET balance_status = $1,
		     balance_persisted_at = CASE
		       WHEN $1 = 'CONFIRMED' THEN now()
		       ELSE balance_persisted_at
		     END,
		     updated_at = now()
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
// before routing to DLQ. Increased from 5 to 288 to span ~24 hours (≈23h 34m) with
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

### Task 10a: Add retry-storm guardrails (anti-DoS)

**Goal:** Preserve the 24h retry window for genuine outages while preventing unbounded resource consumption during poison-message storms or abuse.

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/rabbitmq/consumer.rabbitmq.go`
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/rabbitmq/metrics.go`
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/create-balance-transaction-operations-async.go` (extract key for budgeting)

**Step 1: Add hybrid retry budgets (Redis/Valkey with TTL + time buckets)**

Design goals:
- Protect the system from poison-message storms on hot balance keys
- Avoid punishing legitimate traffic forever (use time buckets)
- Keep correctness independent from budgets (budgets are guardrails, not idempotency)

Keys:
- Per-transaction cap (prevents churn): `retry_budget:tx:{org}:{ledger}:{tx_id}`
- Per-balance-key cap (prevents hot-key storms): `retry_budget:key:{org}:{ledger}:{balance_key}:{bucket_10m}`
  - `bucket_10m = unix_timestamp / 600`

Behavior:
- Per-tx budget TTL: **48h** (match Lua idempotency TTL)
- Per-balance-key bucket TTL: **10m + jitter** (e.g., 10m + [0..60s])
- On each retryable business error, increment both counters
- If either counter exceeds threshold:
  - Prefer **cooldown** first (delay republish longer)
  - Escalate to DLQ if budget remains exhausted across multiple cooldowns
- If Redis/Valkey is unavailable: **fail open** (skip budget enforcement) and emit a metric

**Step 2: Emit low-cardinality metrics**
- `transaction_retry_budget_exhausted_total{queue="...", budget="tx|balance_key"}`
- `transaction_retry_budget_check_failed_total{queue="..."}`
- `transaction_retry_routed_to_dlq_total{queue="...", reason="budget_exhausted|max_retries"}`

**Step 3: Add operational alerting thresholds**
- Alert on sustained `retry_budget_exhausted_total` growth
- Alert on `retry_budget_check_failed_total` spikes (Redis issues)
- Alert on DLQ growth and DLQ publish failures

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

After RecordMessageRetry method (around line 140), add (requires `github.com/LerianStudio/midaz/v3/pkg/mlog` import for structured logging):

```go
// RecordBalanceStatusFailed increments the transaction_balance_status_failed_total counter.
// This is called when a transaction's balance update fails after max retries and is marked FAILED.
func (dm *DLQMetrics) RecordBalanceStatusFailed(ctx context.Context, queue string) {
	if dm == nil || dm.factory == nil {
		return
	}

	// IMPORTANT: Do NOT include transaction_id as a metric label (high cardinality / DoS risk).
	// Use structured logs for correlation instead.
	dm.factory.Counter(balanceStatusFailedMetric).
		WithLabels(map[string]string{
			"queue": sanitizeLabelValue(queue),
		}).
		AddOne(ctx)
}

// recordBalanceStatusFailed is a package-level helper that records a balance status failure.
// Keep transactionID only for logs (not metric labels).
func recordBalanceStatusFailed(ctx context.Context, queue, transactionID string) {
	dm := GetDLQMetrics()
	if dm != nil {
		dm.RecordBalanceStatusFailed(ctx, queue)
	}

	logger := mlog.NewLoggerFromContext(ctx)
	logger.WithFields(map[string]interface{}{
		"event":          "balance_status_failed",
		"queue":          queue,
		"transaction_id": transactionID,
	}).Warn("Transaction marked FAILED (DLQ)")
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
- Add retry-storm guardrails (per-key retry budget)
- Add balance_status_failed metric for observability (no high-cardinality labels)
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

**CRITICAL (exactly-once):** A Postgres `Find()` check alone is not sufficient because the flow can mutate Redis before Postgres is durable. To prevent the crash window (Redis applied → worker dies → redelivery applies again), we must make the **Redis Lua script idempotent by `transaction_id`**.

Idempotency rules:
- The Lua script must atomically acquire an idempotency key before applying any balance deltas.
- If the idempotency key already exists, the script must do **nothing** and return a normal balances JSON response built from current Redis cache (so Go can proceed without special-casing).
- Set TTL to **48h** (24h retry window + buffer).

**Concrete implementation steps (Redis Lua + Go call path):**

**A) Lua script location and contract**
- File: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/redis/scripts/add_sub.lua`
- Loaded via: `//go:embed scripts/add_sub.lua` in `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/redis/consumer.redis.go`
- Called by: `(*RedisConsumerRepository).executeBalanceScript` in `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/redis/consumer.redis.go`
- Return type requirement (existing invariant): script MUST return `string`/`[]byte` containing JSON array of `[]mmodel.BalanceRedis`

**B) Extend KEYS contract**
- Existing:
  - `KEYS[1]` = `backup_queue:{transactions}`
  - `KEYS[2]` = `transaction:{transactions}:<org>:<ledger>:<tx_id>`
  - `KEYS[3]` = `schedule:{transactions}:balance-sync`
- Add:
  - `KEYS[4]` = `idemp:{transactions}:<org>:<ledger>:<tx_id>` (48h TTL)

**C) Lua idempotency behavior**
- At the start of `main()` (after reading KEYS/ARGV), attempt:
  - `SET idempKey "1" NX EX 172800`
- If SETNX fails (already applied):
  - DO NOT modify any balance keys
  - Return balances from current Redis cache, preserving alias from ARGV

**Pseudo-code snippet to add near the start of `main()` in `add_sub.lua`:**
```lua
local idempKey = KEYS[4]
local ttlIdemp = 172800 -- 48h

local function returnBalancesFromCache()
  local groupSize = 16
  local returnBalances = {}
  for i = 2, #ARGV, groupSize do
    local redisBalanceKey = ARGV[i]
    local alias = ARGV[i + 5]
    local currentBalance = redis.call("GET", redisBalanceKey)
    if not currentBalance then
      -- If cache is missing, fall back to normal path (or treat as corruption)
      return nil
    end
    local b = cjson.decode(currentBalance)
    b.Alias = alias
    table.insert(returnBalances, b)
  end
  return cjson.encode(returnBalances)
end

local ok = redis.call("SET", idempKey, "1", "NX", "EX", ttlIdemp)
if not ok then
  local cached = returnBalancesFromCache()
  if cached then
    return cached
  end
end
```

**D) Failure semantics (avoid "poisoning" idempotency key)**
- If the script returns an error after acquiring the idempotency key, the idempotency key must be deleted so the message can be retried safely.
- Recommended pattern:
  - introduce a local helper `fail(code)` that:
    - calls `rollback(rollbackBalances, ttl)` (existing)
    - `DEL idempKey`
    - returns `redis.error_reply(code)`
  - replace existing `rollback(...); return redis.error_reply("...")` sites inside `main()` with `return fail("...")`

**E) Update Go invocation to pass KEYS[4]**
- File: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/redis/consumer.redis.go`
- In `executeBalanceScript`, compute:
  - `idempKey := fmt.Sprintf("idemp:{transactions}:%s:%s:%s", organizationID, ledgerID, transactionID)`
- Update `script.Run` keys list to include it:
  - `[]string{TransactionBackupQueue, transactionKey, utils.BalanceSyncScheduleKey, idempKey}`

**F) Keep Postgres `Find()` check**
- Keep the Postgres `Find()` check as an optimization + to converge `balance_status`, but it is not the correctness gate.

```go
func (uc *UseCase) CreateBalanceTransactionOperationsAsync(ctx context.Context, data mmodel.Queue) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)
	// ... existing setup code ...

	transactionID := data.QueueData[0].ID // Adjust based on actual structure

	// Optional optimization: if an async tx already exists, do not re-apply balances.
	existing, err := uc.TransactionRepo.Find(ctx, data.OrganizationID, data.LedgerID, transactionID)
	if err == nil && existing.BalanceStatus != nil {
		if *existing.BalanceStatus == constant.BalanceStatusPending {
			// IMPORTANT: If this convergence fails, return error so RabbitMQ retries.
			if statusErr := uc.TransactionRepo.UpdateBalanceStatus(
				ctx, data.OrganizationID, data.LedgerID, transactionID,
				constant.BalanceStatusConfirmed,
			); statusErr != nil {
				return statusErr
			}
		}

		logger.Infof("Async transaction %s already exists (balanceStatus=%s), skipping duplicate message", transactionID, *existing.BalanceStatus)
		return nil
	}

	// ... rest of existing logic ...
	// Balance update step MUST pass transactionID into Lua script for dedup.
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

**Why retry matters:** If balances are persisted but CONFIRMED status update fails, clients see PENDING and may retry.

With Lua-level idempotency (Task 13), retries must not create duplicate balance effects, but status convergence is still required to satisfy the visibility promise and reduce client retry pressure.

**Step 1: Update balance_status after successful balance update with retry**

After the balance update succeeds (after `UpdateBalances` returns nil), add a call to update the status with retry logic.

Also ensure these imports exist in the file:
```go
import (
	"math/rand"
	"time"

	rabbitmq "github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/rabbitmq"
)
```

```go
// Mark transaction as CONFIRMED after successful balance update
// CRITICAL: If this fails, return an error so RabbitMQ retries.
// Idempotency guarantees ensure a retry won't re-apply balances; it will primarily converge the status.
var statusErr error
for attempts := 0; attempts < 3; attempts++ {
	statusErr = uc.TransactionRepo.UpdateBalanceStatus(ctx, message.OrganizationID, message.LedgerID, transactionID, constant.BalanceStatusConfirmed)
	if statusErr == nil {
		break
	}
	logger.Warnf("Attempt %d/3: Failed to update balance_status to CONFIRMED for transaction %s: %v", attempts+1, transactionID, statusErr)
	if attempts < 2 {
		// short backoff + jitter: avoid thundering herd on transient DB blips
		time.Sleep(time.Duration(attempts+1)*100*time.Millisecond + time.Duration(rand.Intn(50))*time.Millisecond)
	}
}

if statusErr != nil {
	logger.Errorf("Failed to update balance_status to CONFIRMED for transaction %s after 3 attempts: %v", transactionID, statusErr)
	// Emit metric + structured log for alerting
	rabbitmq.RecordBalanceStatusUpdateFailure(ctx, "CONFIRMED", transactionID.String())
	return statusErr
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

	balanceStatusDLQUpdateFailedMetric = metrics.Metric{
		Name:        "transaction_balance_status_dlq_update_failed_total",
		Unit:        "1",
		Description: "Total number of failures updating balance_status during DLQ hook",
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

// RecordBalanceStatusDLQUpdateFailure records when a DLQ hook cannot mark a transaction as FAILED.
// This should be rare; it usually indicates DB outage during DLQ routing.
// Note: transaction_id is logged, not in Prometheus label.
func RecordBalanceStatusDLQUpdateFailure(ctx context.Context, queue, targetStatus, transactionID string) {
	dm := GetDLQMetrics()
	if dm != nil && dm.factory != nil {
		dm.factory.Counter(balanceStatusDLQUpdateFailedMetric).
			WithLabels(map[string]string{
				"queue":         sanitizeLabelValue(queue),
				"target_status": targetStatus,
			}).
			AddOne(ctx)
	}

	logger := mlog.NewLoggerFromContext(ctx)
	logger.WithFields(map[string]interface{}{
		"event":          "balance_status_dlq_update_failure",
		"queue":          queue,
		"target_status":  targetStatus,
		"transaction_id": transactionID,
		"severity":       "critical",
	}).Error("Balance status update failed during DLQ hook")
}
```

**Why these retry values (short backoff + jitter)?**
- Primary goal: converge `balance_status=CONFIRMED` quickly without thundering-herd effects
- Backoff stays small to avoid blocking worker throughput
- Jitter reduces synchronized retries when DB briefly flaps
- If the status update still fails after retries, we return an error to trigger RabbitMQ retry (idempotent)

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
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/create-balance-transaction-operations-async.go` (new helper method)

**Prerequisites:**
- Task 14 completed

**Why this matters:** Without updating status to FAILED, transactions stay PENDING forever after DLQ routing. Clients cannot distinguish "still processing" from "permanently failed".

**Design goal:** Avoid coupling RabbitMQ adapter to Postgres repository types. Use an explicit DLQ callback (hook) wired in bootstrap.

**Step 1: Add an optional DLQ hook to the RabbitMQ consumer layer**

In `consumer.rabbitmq.go`, add a callback type and field to `ConsumerRoutes`:

```go
// OnDLQPublishedFunc is invoked after a message is successfully published to DLQ.
// Best-effort: must never block DLQ publishing.
type OnDLQPublishedFunc func(ctx context.Context, originalQueue string, body []byte)

type ConsumerRoutes struct {
	// ... existing fields ...
	OnDLQPublished OnDLQPublishedFunc
}
```

**Step 2: Invoke the hook after successful DLQ publish**

In `businessErrorContext.routeToDLQIfMaxRetries` (after DLQ publish succeeds, before/after Ack), add:

```go
if bec.onDLQPublished != nil {
	bec.onDLQPublished(bec.ctx, bec.queue, bec.msg.Body)
}
```

Implementation note:
- Add `onDLQPublished OnDLQPublishedFunc` to `messageProcessingContext` and `businessErrorContext`
- Set it from `ConsumerRoutes.OnDLQPublished` when building the message processing context

**Step 3: Implement the hook handler in the transaction UseCase**

In `create-balance-transaction-operations-async.go` (or a nearby file in the same package), add:

```go
// MarkBalanceStatusFailedFromDLQ marks async transactions as FAILED after DLQ routing.
// Best-effort: failures are logged and do not affect DLQ publishing.
func (uc *UseCase) MarkBalanceStatusFailedFromDLQ(ctx context.Context, originalQueue string, body []byte) {
	logger := mlog.NewLoggerFromContext(ctx)

	var msg mmodel.Queue
	if err := msgpack.Unmarshal(body, &msg); err != nil {
		logger.Warnf("Failed to unmarshal DLQ body for FAILED status update: %v", err)
		return
	}
	if len(msg.QueueData) == 0 {
		logger.Warn("Empty QueueData in DLQ body for FAILED status update")
		return
	}

	for _, item := range msg.QueueData {
		if item.ID == uuid.Nil {
			continue
		}

		// Best-effort with retries: DB might be flapping during outage.
		var lastErr error
		for attempts := 0; attempts < 3; attempts++ {
			lastErr = uc.TransactionRepo.UpdateBalanceStatus(ctx, msg.OrganizationID, msg.LedgerID, item.ID, constant.BalanceStatusFailed)
			if lastErr == nil {
				break
			}
			time.Sleep(time.Duration(attempts+1)*200*time.Millisecond + time.Duration(rand.Intn(100))*time.Millisecond)
		}
		if lastErr != nil {
			rabbitmq.RecordBalanceStatusDLQUpdateFailure(ctx, originalQueue, "FAILED", item.ID.String())
			logger.Warnf("Failed to mark transaction %s as FAILED after DLQ (will be handled by reconciliation if still PENDING after threshold): %v", item.ID, lastErr)
			continue
		}

		recordBalanceStatusFailed(ctx, originalQueue, item.ID.String())
	}
}
```

**Step 4: Wire the hook in bootstrap**

In `rabbitmq.server.go`, after creating `consumerRoutes`, set:

```go
consumerRoutes.OnDLQPublished = useCase.MarkBalanceStatusFailedFromDLQ
```

**Step 5: Verify compilation**

Run:
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./components/transaction/internal/adapters/rabbitmq/...
cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./components/transaction/internal/bootstrap/...
```

**Expected output:**
```
(no output - success)
```

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

- Add Lua-level idempotency (transaction_id dedup)
- Set balance_status=PENDING when creating async transaction
- Update to CONFIRMED on successful balance persistence (sets balance_persisted_at)
- Mark as FAILED when routing to DLQ after max retries (best-effort + metrics)
- Add DLQ hook to mark FAILED without tight coupling

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

**Step 2: Add integration tests for UpdateBalanceStatus (real Postgres)**

Create a new file with an integration build tag:
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/postgres/transaction/transaction.postgresql_integ_test.go`
- Add: `//go:build integration`

Use `testcontainers-go` to start a PostgreSQL container and run real SQL against it.
- Follow the existing pattern used in `components/transaction/internal/adapters/redis/balance_sync_integ_test.go`
- Ensure migrations are applied before exercising `UpdateBalanceStatus`

Integration cases to implement (no `t.Skip`):
- `TestUpdateBalanceStatus_Success`: create async transaction with `PENDING`, update to `CONFIRMED`, verify persisted value, `balance_persisted_at` is set, and `updated_at` changes
- `TestUpdateBalanceStatus_NotFound`: update a random UUID and assert `ErrEntityNotFound`
- `TestUpdateBalanceStatus_InvalidTransition`: create `CONFIRMED`, attempt update to `FAILED`, verify no change and no error (idempotent)

**Step 3: Run tests**

Run:
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go test -tags=integration ./components/transaction/internal/adapters/postgres/transaction/... -v -run "ValidateBalanceStatus|UpdateBalanceStatus" -count=1
```

**Expected output:**
```
=== RUN   TestValidateBalanceStatus
--- PASS: TestValidateBalanceStatus (0.00s)
=== RUN   TestUpdateBalanceStatus_Success
--- PASS: TestUpdateBalanceStatus_Success
=== RUN   TestUpdateBalanceStatus_NotFound
--- PASS: TestUpdateBalanceStatus_NotFound
=== RUN   TestUpdateBalanceStatus_InvalidTransition
--- PASS: TestUpdateBalanceStatus_InvalidTransition
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

**Step 1: Add reconciliation query method (proof-based) to Repository**

In `transaction.postgresql.go`, add a dedicated query that returns only what reconciliation needs, including the internal proof field `balance_persisted_at`.

```go
type PendingBalanceStatusCandidate struct {
	ID               uuid.UUID
	OrganizationID   uuid.UUID
	LedgerID         uuid.UUID
	CreatedAt        time.Time
	BalancePersistedAt *time.Time
}

// FindPendingForReconciliation returns async transactions stuck in PENDING beyond the threshold.
// This query is the ONLY place we read balance_persisted_at; it must not be exposed via public APIs.
func (r *TransactionPostgreSQLRepository) FindPendingForReconciliation(ctx context.Context, olderThan time.Duration, limit int) ([]PendingBalanceStatusCandidate, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_pending_for_reconciliation")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		return nil, pkg.ValidateInternalError(err, "Transaction")
	}

	threshold := time.Now().Add(-olderThan)

	rows, err := db.QueryContext(ctx, `
		SELECT id, organization_id, ledger_id, created_at, balance_persisted_at
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

	candidates := make([]PendingBalanceStatusCandidate, 0)
	for rows.Next() {
		var c PendingBalanceStatusCandidate
		if scanErr := rows.Scan(&c.ID, &c.OrganizationID, &c.LedgerID, &c.CreatedAt, &c.BalancePersistedAt); scanErr != nil {
			logger.Errorf("Failed to scan reconciliation candidate: %v", scanErr)
			continue
		}
		candidates = append(candidates, c)
	}
	return candidates, nil
}
```

**Step 2: Create reconciliation service (proof-based + guardrails)**

Create `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/reconcile-pending-transactions.go`:

```go
package command

import (
	"context"
	"math/rand"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mlog"
)

// ReconcilePendingTransactionsConfig configures the reconciliation job.
type ReconcilePendingTransactionsConfig struct {
	ThresholdAge time.Duration
	BatchSize    int

	// Guardrails
	MaxTotalPerRun     int
	MaxPerOrgPerRun    int
	MaxErrorRate       float64
	SleepBetweenMin    time.Duration
	SleepBetweenJitter time.Duration
}

func DefaultReconcileConfig() ReconcilePendingTransactionsConfig {
	return ReconcilePendingTransactionsConfig{
		ThresholdAge: 25 * time.Hour, // 24h retry window + 1h buffer
		BatchSize:    100,

		MaxTotalPerRun:     500,
		MaxPerOrgPerRun:    200,
		MaxErrorRate:       0.3,
		SleepBetweenMin:    50 * time.Millisecond,
		SleepBetweenJitter: 50 * time.Millisecond,
	}
}

// ReconcilePendingTransactions fixes transactions stuck in PENDING beyond the retry window.
// Rule (no guessing):
// - If balance_persisted_at IS NOT NULL => mark CONFIRMED
// - If balance_persisted_at IS NULL     => mark FAILED (terminal)
func (uc *UseCase) ReconcilePendingTransactions(ctx context.Context, config ReconcilePendingTransactionsConfig) error {
	logger := mlog.NewLoggerFromContext(ctx)

	candidates, err := uc.TransactionRepo.FindPendingForReconciliation(ctx, config.ThresholdAge, config.BatchSize)
	if err != nil {
		logger.Errorf("Failed to find PENDING reconciliation candidates: %v", err)
		return err
	}
	if len(candidates) == 0 {
		logger.Debug("No PENDING reconciliation candidates found")
		return nil
	}

	seenPerOrg := map[string]int{}
	var reconciled, failed, errors int
	processed := 0

	for _, c := range candidates {
		if processed >= config.MaxTotalPerRun {
			break
		}
		orgKey := c.OrganizationID.String()
		if seenPerOrg[orgKey] >= config.MaxPerOrgPerRun {
			continue
		}

		time.Sleep(config.SleepBetweenMin + time.Duration(rand.Int63n(int64(config.SleepBetweenJitter))))

		targetStatus := constant.BalanceStatusFailed
		if c.BalancePersistedAt != nil {
			targetStatus = constant.BalanceStatusConfirmed
		}

		if err := uc.TransactionRepo.UpdateBalanceStatus(ctx, c.OrganizationID, c.LedgerID, c.ID, targetStatus); err != nil {
			errors++
			logger.Warnf("Failed to reconcile tx %s to %s: %v", c.ID, targetStatus, err)
		} else {
			if targetStatus == constant.BalanceStatusConfirmed {
				reconciled++
			} else {
				failed++
			}
		}

		processed++
		seenPerOrg[orgKey]++

		// Avoid aborting too early on small samples.
		if processed > 10 && float64(errors)/float64(processed) > config.MaxErrorRate {
			logger.Errorf("Reconciliation aborting due to error rate: errors=%d processed=%d", errors, processed)
			break
		}
	}

	logger.WithFields(map[string]interface{}{
		"total_found": len(candidates),
		"processed":   processed,
		"confirmed":   reconciled,
		"failed":      failed,
		"errors":      errors,
	}).Info("Reconciliation job completed")

	return nil
}
```

**Step 3: Add interface method**

In the Repository interface, add:
```go
FindPendingForReconciliation(ctx context.Context, olderThan time.Duration, limit int) ([]PendingBalanceStatusCandidate, error)
```

**Step 4: Wire up as scheduled job (implementation depends on your scheduler)**

Important (multi-instance deployments): acquire a distributed lock before running reconciliation so only one instance processes candidates at a time (e.g., Redis `SETNX lock:{transactions}:reconcile_balance_status EX 3600`).

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

- Add FindPendingForReconciliation repository method (includes balance_persisted_at)
- Add ReconcilePendingTransactions service method
- Run hourly to reconcile transactions beyond retry window using balance_persisted_at proof
- Uses 25h threshold (24h retry window + 1h buffer)

Part of hybrid transaction consistency implementation."
```

---

## Summary of Changes

### Files Modified

| File | Changes |
|------|---------|
| `components/transaction/migrations/000020_add_balance_status_to_transaction.up.sql` | NEW: Migration adding balance_status + balance_persisted_at columns |
| `components/transaction/migrations/000020_add_balance_status_to_transaction.down.sql` | NEW: Rollback migration |
| `pkg/constant/transaction.go` | ADD: BalanceStatus constants (PENDING/CONFIRMED/FAILED) |
| `pkg/mmodel/transaction.go` | ADD: BalanceStatus field to Transaction struct (gated exposure in handler) |
| `components/transaction/internal/adapters/http/in/transaction.go` | MOD: Gate balanceStatus behind `includeBalanceStatus=true` + scope `transactions:read_balance_status` (403 if unauthorized) | 
| `components/transaction/internal/adapters/postgres/transaction/transaction.go` | ADD: BalanceStatus mapping; ADD: BalancePersistedAt internal field (not exposed) |
| `components/transaction/internal/adapters/postgres/transaction/transaction.postgresql.go` | MOD: safer INSERT with explicit columns; ADD: ValidateBalanceStatus; ADD: UpdateBalanceStatus sets balance_persisted_at on CONFIRMED; ADD: reconciliation query for balance_persisted_at |
| `components/transaction/internal/adapters/rabbitmq/consumer.rabbitmq.go` | MOD: Increase maxRetries to 288, extend backoff delays, add retry-storm guardrails, invoke DLQ hook after successful DLQ publish |
| `components/transaction/internal/adapters/rabbitmq/metrics.go` | ADD: balance_status_failed metric (no high-cardinality labels), balance_status_update_failed metric, balance_status_dlq_update_failed metric, retry-budget metrics, structured logs for correlation |
| `components/transaction/internal/bootstrap/rabbitmq.server.go` | MOD: Wire DLQ hook (`OnDLQPublished`) to UseCase handler |
| `components/transaction/internal/services/command/create-balance-transaction-operations-async.go` | MOD: Add Lua-level idempotency (transaction_id dedup), set PENDING on create, set CONFIRMED with retry and error-on-failure, add DLQ FAILED hook handler |

### API Changes

The existing GET `/v1/organizations/{org}/ledgers/{ledger}/transactions/{id}` endpoint can include `balanceStatus` when explicitly requested and authorized:

- Request: `?includeBalanceStatus=true`
- Authorization: scope `transactions:read_balance_status` (see Task 3a)

Example (authorized response):
```json
{
  "id": "...",
  "balanceStatus": "CONFIRMED"  // NEW: PENDING | CONFIRMED | FAILED
}
```

If `includeBalanceStatus` is not set, `balanceStatus` is omitted.

If `includeBalanceStatus=true` but caller is not authorized, return **HTTP 403** (do not silently omit).

Semantics:
- `PENDING` - Balance update queued but not yet confirmed
- `CONFIRMED` - Balance update completed successfully
- `FAILED` - Balance update failed after max retries (in DLQ)
- Omitted - Sync transaction

### Metrics Added

- `transaction_balance_status_failed_total` - Counter for transactions marked FAILED (DLQ routing)
- `transaction_balance_status_update_failed_total` - Counter for status update failures after retries (with `target_status` label)

### Outstanding Work (Post-Implementation)

1. ~~**DLQ Integration**~~: ✅ DONE - DLQ publish hook marks async transactions FAILED (best-effort)
2. **Admin API**: Add audited operator action to transition `FAILED -> PENDING` for manual retries (future feature)
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
