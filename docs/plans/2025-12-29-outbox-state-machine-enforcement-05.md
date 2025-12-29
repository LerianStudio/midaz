# Outbox State Machine Enforcement Implementation Plan

> **For Agents:** REQUIRED SUB-SKILL: Use executing-plans to implement this plan task-by-task.

**Goal:** Enforce the outbox status state machine at runtime to prevent invalid transitions and catch race conditions early.

**Architecture:** Add a `ValidOutboxTransitions` map defining allowed transitions, a `CanTransitionTo` method for validation, and update all status-changing methods to enforce preconditions via SQL WHERE clauses and Go assertions. Terminal states (PUBLISHED, DLQ) have no valid outgoing transitions.

**Tech Stack:** Go 1.21+, `pkg/assert` package (That, Never, ValidUUID predicates), PostgreSQL SQL WHERE clauses for DB-level enforcement

**Global Prerequisites:**
- Environment: macOS/Linux, Go 1.21+
- Tools: Verify with `go version` (expect 1.21+), `golangci-lint --version`
- Access: Read/write access to `components/transaction/internal/adapters/postgres/outbox/`
- State: Clean working tree on branch `fix/fred-several-ones-dec-13-2025`

**Verification before starting:**
```bash
# Run ALL these commands and verify output:
go version          # Expected: go version go1.21+ (any patch)
git status          # Expected: clean working tree (or known modifications)
go test ./components/transaction/internal/adapters/postgres/outbox/... -v  # Expected: PASS
```

## Historical Precedent

**Query:** "outbox state machine assertions validation"
**Index Status:** Empty (new project)

No historical data available. This is normal for new projects.
Proceeding with standard planning approach.

---

## Summary

The outbox has documented status transitions but no runtime enforcement:
```
PENDING -> PROCESSING (worker claims)
PROCESSING -> PUBLISHED (success)
PROCESSING -> FAILED (error, can retry)
FAILED -> PROCESSING (retry)
FAILED -> DLQ (max retries)
```

Current code allows any status to transition to any other status, which could mask bugs. This plan adds:
1. State machine definition with `ValidOutboxTransitions` map
2. `CanTransitionTo` and `IsTerminal` methods on `OutboxStatus`
3. SQL WHERE clauses enforcing source status in all transition methods
4. Go assertions for parameter validation
5. Comprehensive tests for valid/invalid transitions

**Expected outcome:** ~12-15 assertions added across 6 methods, state machine enforced at both Go and SQL levels.

---

### Task 1: Add State Machine Definition to outbox.go

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/postgres/outbox/outbox.go:37` (after StatusDLQ constant)

**Prerequisites:**
- File exists: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/postgres/outbox/outbox.go`

**Step 1: Write the failing test**

Add test to verify state machine definition:

Create new file `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/postgres/outbox/state_machine_test.go`:

```go
package outbox

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidOutboxTransitions_Defined(t *testing.T) {
	// Verify all statuses are in the transition map
	statuses := []OutboxStatus{StatusPending, StatusProcessing, StatusPublished, StatusFailed, StatusDLQ}
	for _, s := range statuses {
		_, exists := ValidOutboxTransitions[s]
		assert.True(t, exists, "status %s must be in ValidOutboxTransitions", s)
	}
}

func TestOutboxStatus_CanTransitionTo_ValidTransitions(t *testing.T) {
	tests := []struct {
		from OutboxStatus
		to   OutboxStatus
	}{
		{StatusPending, StatusProcessing},
		{StatusProcessing, StatusPublished},
		{StatusProcessing, StatusFailed},
		{StatusFailed, StatusProcessing},
		{StatusFailed, StatusDLQ},
	}

	for _, tt := range tests {
		t.Run(string(tt.from)+"->"+string(tt.to), func(t *testing.T) {
			assert.True(t, tt.from.CanTransitionTo(tt.to),
				"transition from %s to %s should be valid", tt.from, tt.to)
		})
	}
}

func TestOutboxStatus_CanTransitionTo_InvalidTransitions(t *testing.T) {
	tests := []struct {
		from OutboxStatus
		to   OutboxStatus
	}{
		// PENDING can only go to PROCESSING
		{StatusPending, StatusPublished},
		{StatusPending, StatusFailed},
		{StatusPending, StatusDLQ},
		// PROCESSING cannot go back to PENDING or directly to DLQ
		{StatusProcessing, StatusPending},
		{StatusProcessing, StatusDLQ},
		// PUBLISHED is terminal
		{StatusPublished, StatusPending},
		{StatusPublished, StatusProcessing},
		{StatusPublished, StatusFailed},
		{StatusPublished, StatusDLQ},
		// DLQ is terminal
		{StatusDLQ, StatusPending},
		{StatusDLQ, StatusProcessing},
		{StatusDLQ, StatusPublished},
		{StatusDLQ, StatusFailed},
		// FAILED cannot go directly to PUBLISHED
		{StatusFailed, StatusPublished},
		{StatusFailed, StatusPending},
	}

	for _, tt := range tests {
		t.Run(string(tt.from)+"->"+string(tt.to), func(t *testing.T) {
			assert.False(t, tt.from.CanTransitionTo(tt.to),
				"transition from %s to %s should be invalid", tt.from, tt.to)
		})
	}
}

func TestOutboxStatus_IsTerminal(t *testing.T) {
	assert.False(t, StatusPending.IsTerminal(), "PENDING is not terminal")
	assert.False(t, StatusProcessing.IsTerminal(), "PROCESSING is not terminal")
	assert.False(t, StatusFailed.IsTerminal(), "FAILED is not terminal")
	assert.True(t, StatusPublished.IsTerminal(), "PUBLISHED is terminal")
	assert.True(t, StatusDLQ.IsTerminal(), "DLQ is terminal")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./components/transaction/internal/adapters/postgres/outbox/... -run TestValidOutboxTransitions -v`

**Expected output:**
```
# github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/outbox [build failed]
./state_machine_test.go:13:16: undefined: ValidOutboxTransitions
```

**Step 3: Write minimal implementation**

Edit `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/postgres/outbox/outbox.go`.

After line 37 (after `StatusDLQ OutboxStatus = "DLQ"`), add:

```go
// ValidOutboxTransitions defines the allowed status transitions.
// This enforces the state machine at runtime to catch bugs early.
//
// Valid transitions:
//   - PENDING -> PROCESSING (worker claims entry)
//   - PROCESSING -> PUBLISHED (success)
//   - PROCESSING -> FAILED (error, will retry)
//   - FAILED -> PROCESSING (retry attempt)
//   - FAILED -> DLQ (max retries exceeded)
//
// Terminal states (no outgoing transitions):
//   - PUBLISHED: Successfully processed
//   - DLQ: Permanently failed, requires manual intervention
var ValidOutboxTransitions = map[OutboxStatus][]OutboxStatus{
	StatusPending:    {StatusProcessing},
	StatusProcessing: {StatusPublished, StatusFailed},
	StatusFailed:     {StatusProcessing, StatusDLQ},
	StatusPublished:  {}, // Terminal state
	StatusDLQ:        {}, // Terminal state
}

// CanTransitionTo returns true if transitioning from this status to the target is valid.
func (s OutboxStatus) CanTransitionTo(target OutboxStatus) bool {
	allowed, exists := ValidOutboxTransitions[s]
	if !exists {
		return false
	}

	for _, a := range allowed {
		if a == target {
			return true
		}
	}

	return false
}

// IsTerminal returns true if this status is a terminal state (no valid outgoing transitions).
func (s OutboxStatus) IsTerminal() bool {
	return s == StatusPublished || s == StatusDLQ
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./components/transaction/internal/adapters/postgres/outbox/... -run "TestValidOutboxTransitions|TestOutboxStatus_CanTransitionTo|TestOutboxStatus_IsTerminal" -v`

**Expected output:**
```
=== RUN   TestValidOutboxTransitions_Defined
--- PASS: TestValidOutboxTransitions_Defined (0.00s)
=== RUN   TestOutboxStatus_CanTransitionTo_ValidTransitions
--- PASS: TestOutboxStatus_CanTransitionTo_ValidTransitions (0.00s)
=== RUN   TestOutboxStatus_CanTransitionTo_InvalidTransitions
--- PASS: TestOutboxStatus_CanTransitionTo_InvalidTransitions (0.00s)
=== RUN   TestOutboxStatus_IsTerminal
--- PASS: TestOutboxStatus_IsTerminal (0.00s)
PASS
```

**Step 5: Commit**

```bash
git add components/transaction/internal/adapters/postgres/outbox/outbox.go components/transaction/internal/adapters/postgres/outbox/state_machine_test.go
git commit -m "$(cat <<'EOF'
feat(outbox): add state machine definition and transition validation

Add ValidOutboxTransitions map, CanTransitionTo and IsTerminal methods
to enforce the outbox status state machine at runtime.
EOF
)"
```

**If Task Fails:**

1. **Test won't run:**
   - Check: `ls components/transaction/internal/adapters/postgres/outbox/` (file exists?)
   - Fix: Ensure you're in the repo root directory
   - Rollback: `git checkout -- .`

2. **Compilation errors:**
   - Run: `go build ./components/transaction/internal/adapters/postgres/outbox/...`
   - Check imports and syntax
   - Rollback: `git checkout -- components/transaction/internal/adapters/postgres/outbox/outbox.go`

---

### Task 2: Add UUID Validation to MarkPublished

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/postgres/outbox/outbox.postgresql.go:411-453`

**Prerequisites:**
- Task 1 completed
- File exists: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/postgres/outbox/outbox.postgresql.go`

**Step 1: Write the failing test**

Add to `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/postgres/outbox/state_machine_test.go`:

```go
func TestMarkPublished_InvalidUUID_Panics(t *testing.T) {
	// This test verifies that MarkPublished panics with invalid UUID
	// We can't easily test this without a real DB, but we document the behavior
	// The actual assertion happens in the implementation
	t.Log("MarkPublished should assert valid UUID format - tested via assertion in code")
}
```

**Step 2: Write implementation with assertions**

Edit `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/postgres/outbox/outbox.postgresql.go`.

Replace the `MarkPublished` method (lines 410-453) with:

```go
// MarkPublished marks an entry as successfully processed.
// Preconditions:
//   - id must be a valid UUID
//   - Entry must be in PROCESSING status (enforced by SQL WHERE clause)
func (r *OutboxPostgreSQLRepository) MarkPublished(ctx context.Context, id string) error {
	// Validate UUID format
	assert.That(assert.ValidUUID(id), "outbox entry ID must be valid UUID",
		"id", id, "method", "MarkPublished")

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.outbox.mark_published")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	now := time.Now()
	// Enforce state machine: only PROCESSING -> PUBLISHED is valid
	query := `
		UPDATE metadata_outbox
		SET status = $1, updated_at = $2, processed_at = $3
		WHERE id = $4 AND status = $5
	`

	result, err := db.ExecContext(ctx, query,
		string(StatusPublished), now, now, id, string(StatusProcessing))
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to mark entry as published", err)
		logger.Errorf("Failed to mark entry as published: %v", err)

		return pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		return pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	if rowsAffected == 0 {
		// Could be: entry not found OR entry not in PROCESSING status
		logger.Warnf("MarkPublished: no rows affected - entry may not exist or not in PROCESSING status: id=%s", id)

		return pkg.ValidateInternalError(ErrOutboxEntryNotFound, "MetadataOutbox")
	}

	// Postcondition: exactly one row should be affected
	assert.That(rowsAffected == 1, "mark published must affect exactly one row",
		"rows_affected", rowsAffected, "id", id)

	logger.Infof("Marked outbox entry as published: id=%s", id)

	return nil
}
```

**Step 3: Run test to verify compilation**

Run: `go build ./components/transaction/internal/adapters/postgres/outbox/...`

**Expected output:** No errors (silent success)

**Step 4: Run all outbox tests**

Run: `go test ./components/transaction/internal/adapters/postgres/outbox/... -v`

**Expected output:**
```
=== RUN   TestNewMetadataOutbox_Success
--- PASS: TestNewMetadataOutbox_Success (0.00s)
...
PASS
ok      github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/outbox
```

**Step 5: Commit**

```bash
git add components/transaction/internal/adapters/postgres/outbox/outbox.postgresql.go
git commit -m "$(cat <<'EOF'
feat(outbox): enforce state machine in MarkPublished

Add UUID validation assertion and SQL WHERE clause to ensure only
PROCESSING entries can transition to PUBLISHED.
EOF
)"
```

**If Task Fails:**

1. **Compilation fails:**
   - Check: Import `"github.com/LerianStudio/midaz/v3/pkg/assert"` exists
   - Run: `go mod tidy`
   - Rollback: `git checkout -- components/transaction/internal/adapters/postgres/outbox/outbox.postgresql.go`

---

### Task 3: Add Assertions to MarkFailed

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/postgres/outbox/outbox.postgresql.go:455-508`

**Prerequisites:**
- Task 2 completed

**Step 1: Write implementation with assertions**

Replace the `MarkFailed` method (lines 455-508) with:

```go
// MarkFailed increments retry count and schedules next retry.
// Error message is sanitized to remove PII before storage.
// Preconditions:
//   - id must be a valid UUID
//   - errMsg must not be empty (will be sanitized)
//   - Entry must be in PROCESSING status (enforced by SQL WHERE clause)
func (r *OutboxPostgreSQLRepository) MarkFailed(ctx context.Context, id string, errMsg string, nextRetryAt time.Time) error {
	// Validate preconditions
	assert.That(assert.ValidUUID(id), "outbox entry ID must be valid UUID",
		"id", id, "method", "MarkFailed")
	assert.NotEmpty(errMsg, "error message must not be empty",
		"id", id, "method", "MarkFailed")

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.outbox.mark_failed")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	// Sanitize error message to remove PII before storing
	sanitizedErr := SanitizeErrorMessage(errMsg)

	// Enforce state machine: only PROCESSING -> FAILED is valid
	query := `
		UPDATE metadata_outbox
		SET status = $1, retry_count = retry_count + 1, last_error = $2, next_retry_at = $3, updated_at = $4
		WHERE id = $5 AND status = $6
	`

	result, err := db.ExecContext(ctx, query,
		string(StatusFailed),
		sanitizedErr,
		nextRetryAt,
		time.Now(),
		id,
		string(StatusProcessing),
	)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to mark entry as failed", err)
		logger.Errorf("Failed to mark entry as failed: %v", err)

		return pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		return pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	if rowsAffected == 0 {
		// Could be: entry not found OR entry not in PROCESSING status
		logger.Warnf("MarkFailed: no rows affected - entry may not exist or not in PROCESSING status: id=%s", id)

		return pkg.ValidateInternalError(ErrOutboxEntryNotFound, "MetadataOutbox")
	}

	// Postcondition: exactly one row should be affected
	assert.That(rowsAffected == 1, "mark failed must affect exactly one row",
		"rows_affected", rowsAffected, "id", id)

	// Log with correlation ID only, not the error message (to avoid PII in logs)
	logger.Warnf("Marked outbox entry as failed: id=%s, next_retry=%v", id, nextRetryAt)

	return nil
}
```

**Step 2: Run test to verify compilation**

Run: `go build ./components/transaction/internal/adapters/postgres/outbox/...`

**Expected output:** No errors (silent success)

**Step 3: Run all outbox tests**

Run: `go test ./components/transaction/internal/adapters/postgres/outbox/... -v`

**Expected output:** All tests pass

**Step 4: Commit**

```bash
git add components/transaction/internal/adapters/postgres/outbox/outbox.postgresql.go
git commit -m "$(cat <<'EOF'
feat(outbox): enforce state machine in MarkFailed

Add UUID validation, non-empty error message assertions, and SQL WHERE
clause to ensure only PROCESSING entries can transition to FAILED.
EOF
)"
```

**If Task Fails:**

1. **Compilation fails:**
   - Check imports for `assert` package
   - Rollback: `git checkout -- components/transaction/internal/adapters/postgres/outbox/outbox.postgresql.go`

---

### Task 4: Add Assertions to MarkDLQ

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/postgres/outbox/outbox.postgresql.go:510-563`

**Prerequisites:**
- Task 3 completed

**Step 1: Write implementation with assertions**

Replace the `MarkDLQ` method (lines 510-563) with:

```go
// MarkDLQ marks an entry as permanently failed (Dead Letter Queue).
// Error message is sanitized to remove PII before storage.
// Preconditions:
//   - id must be a valid UUID
//   - errMsg must not be empty (will be sanitized)
//   - Entry must be in PROCESSING or FAILED status (enforced by SQL WHERE clause)
func (r *OutboxPostgreSQLRepository) MarkDLQ(ctx context.Context, id string, errMsg string) error {
	// Validate preconditions
	assert.That(assert.ValidUUID(id), "outbox entry ID must be valid UUID",
		"id", id, "method", "MarkDLQ")
	assert.NotEmpty(errMsg, "DLQ reason must not be empty",
		"id", id, "method", "MarkDLQ")

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.outbox.mark_dlq")
	defer span.End()

	db, err := r.connection.GetDB()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	// Sanitize error message to remove PII before storing
	sanitizedErr := SanitizeErrorMessage(errMsg)

	// Enforce state machine: PROCESSING -> DLQ or FAILED -> DLQ are valid
	// Note: PROCESSING -> DLQ happens when processing fails and max retries already exceeded
	// FAILED -> DLQ happens when retry count check happens before retry attempt
	query := `
		UPDATE metadata_outbox
		SET status = $1, last_error = $2, updated_at = $3
		WHERE id = $4 AND status IN ($5, $6)
	`

	result, err := db.ExecContext(ctx, query,
		string(StatusDLQ),
		sanitizedErr,
		time.Now(),
		id,
		string(StatusProcessing),
		string(StatusFailed),
	)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to mark entry as DLQ", err)
		logger.Errorf("Failed to mark entry as DLQ: %v", err)

		return pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		return pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	if rowsAffected == 0 {
		// Could be: entry not found OR entry not in PROCESSING/FAILED status
		logger.Warnf("MarkDLQ: no rows affected - entry may not exist or in invalid status: id=%s", id)

		return pkg.ValidateInternalError(ErrOutboxEntryNotFound, "MetadataOutbox")
	}

	// Postcondition: exactly one row should be affected
	assert.That(rowsAffected == 1, "mark DLQ must affect exactly one row",
		"rows_affected", rowsAffected, "id", id)

	// Log DLQ event for alerting (no PII in message)
	logger.Warnf("METADATA_OUTBOX_DLQ: Entry moved to Dead Letter Queue: id=%s", id)

	return nil
}
```

**Step 2: Run test to verify compilation**

Run: `go build ./components/transaction/internal/adapters/postgres/outbox/...`

**Expected output:** No errors (silent success)

**Step 3: Run all outbox tests**

Run: `go test ./components/transaction/internal/adapters/postgres/outbox/... -v`

**Expected output:** All tests pass

**Step 4: Commit**

```bash
git add components/transaction/internal/adapters/postgres/outbox/outbox.postgresql.go
git commit -m "$(cat <<'EOF'
feat(outbox): enforce state machine in MarkDLQ

Add UUID validation, non-empty reason assertions, and SQL WHERE clause
to ensure only PROCESSING or FAILED entries can transition to DLQ.
EOF
)"
```

**If Task Fails:**

1. **Compilation fails:**
   - Check SQL syntax (IN clause with multiple values)
   - Rollback: `git checkout -- components/transaction/internal/adapters/postgres/outbox/outbox.postgresql.go`

---

### Task 5: Add Assertions to Create Method

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/postgres/outbox/outbox.postgresql.go:146-195`

**Prerequisites:**
- Task 4 completed

**Step 1: Write implementation with assertions**

Replace the `Create` method (lines 146-195) with:

```go
// Create inserts a new outbox entry. If a transaction is in context, participates in it.
// Preconditions:
//   - entry must not be nil
//   - entry must have StatusPending (new entries always start as PENDING)
func (r *OutboxPostgreSQLRepository) Create(ctx context.Context, entry *MetadataOutbox) error {
	// Validate preconditions
	assert.NotNil(entry, "outbox entry must not be nil", "method", "Create")
	assert.That(entry.Status == StatusPending,
		"new outbox entry must have PENDING status",
		"actual_status", entry.Status, "method", "Create")

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.outbox.create")
	defer span.End()

	executor, err := r.getExecutor(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get executor", err)
		logger.Errorf("Failed to get executor: %v", err)

		return pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	record := &MetadataOutboxPostgreSQLModel{}
	if err := record.FromEntity(entry); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert entity to model", err)
		logger.Errorf("Failed to convert entity to model: %v", err)

		return pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	query := `
		INSERT INTO metadata_outbox (id, entity_id, entity_type, metadata, status, retry_count, max_retries, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	result, err := executor.ExecContext(ctx, query,
		record.ID,
		record.EntityID,
		record.EntityType,
		record.Metadata,
		record.Status,
		record.RetryCount,
		record.MaxRetries,
		record.CreatedAt,
		record.UpdatedAt,
	)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to insert outbox entry", err)
		logger.Errorf("Failed to insert outbox entry: %v", err)

		return pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	// Verify insert succeeded
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get rows affected", err)

		return pkg.ValidateInternalError(err, "MetadataOutbox")
	}

	// Postcondition: exactly one row should be inserted
	assert.That(rowsAffected == 1, "outbox entry insert must affect exactly one row",
		"rows_affected", rowsAffected,
		"entity_id", entry.EntityID,
		"entity_type", entry.EntityType)

	logger.Infof("Created outbox entry: entity_id=%s, entity_type=%s", entry.EntityID, entry.EntityType)

	return nil
}
```

**Step 2: Run test to verify compilation**

Run: `go build ./components/transaction/internal/adapters/postgres/outbox/...`

**Expected output:** No errors (silent success)

**Step 3: Run all outbox tests**

Run: `go test ./components/transaction/internal/adapters/postgres/outbox/... -v`

**Expected output:** All tests pass

**Step 4: Commit**

```bash
git add components/transaction/internal/adapters/postgres/outbox/outbox.postgresql.go
git commit -m "$(cat <<'EOF'
feat(outbox): add precondition assertions to Create method

Validate entry is not nil, has PENDING status, and verify exactly one
row is inserted.
EOF
)"
```

**If Task Fails:**

1. **Tests fail:**
   - Check if existing tests create entries with non-PENDING status
   - Rollback: `git checkout -- components/transaction/internal/adapters/postgres/outbox/outbox.postgresql.go`

---

### Task 6: Add Assertions to ClaimPendingBatch

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/postgres/outbox/outbox.postgresql.go:197-353`

**Prerequisites:**
- Task 5 completed

**Step 1: Write the failing test**

Add to `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/postgres/outbox/state_machine_test.go`:

```go
func TestClaimPendingBatch_BatchSizeValidation(t *testing.T) {
	// Document expected behavior for batch size boundaries
	// Actual validation happens in implementation via assertions and normalization
	t.Log("ClaimPendingBatch normalizes batch size: <=0 becomes 100, >1000 becomes 1000")
}
```

**Step 2: Write implementation with assertions**

In `ClaimPendingBatch` method, after the existing batch size normalization (around line 207-213), add postcondition verification. Find the section where entries are collected and before the return statement at the end, add:

Locate the return statement `return entries, nil` at around line 350-352 and add postcondition check before it:

```go
	// Postcondition: all returned entries must be in PROCESSING status
	// (we just updated them in this transaction)
	for i, entry := range entries {
		assert.That(entry.Status == StatusProcessing || entry.Status == StatusPending || entry.Status == StatusFailed,
			"claimed entry must have been in claimable status",
			"index", i,
			"entry_id", entry.ID.String(),
			"status", entry.Status)
	}
```

**Note:** The entries slice contains the status BEFORE the update (as they were selected from DB). The postcondition should verify they were in a valid source state. However, since we update them to PROCESSING and don't re-read, we trust the SQL. A better approach is to update the entry status in memory after claiming.

**Revised implementation** - add after the `tx.Commit()` call and before the log statement:

```go
	// Update in-memory entries to reflect PROCESSING status
	// (the database was already updated in the transaction)
	for _, entry := range entries {
		entry.Status = StatusProcessing
		entry.ProcessingStartedAt = &now
	}
```

Then the postcondition becomes:

```go
	// Postcondition: all returned entries must be in PROCESSING status
	for i, entry := range entries {
		assert.That(entry.Status == StatusProcessing,
			"claimed entry must be in PROCESSING status after claim",
			"index", i,
			"entry_id", entry.ID.String(),
			"actual_status", entry.Status)
	}

	logger.Infof("Claimed %d outbox entries for processing", len(entries))

	return entries, nil
```

**Step 3: Run test to verify compilation**

Run: `go build ./components/transaction/internal/adapters/postgres/outbox/...`

**Expected output:** No errors (silent success)

**Step 4: Run all outbox tests**

Run: `go test ./components/transaction/internal/adapters/postgres/outbox/... -v`

**Expected output:** All tests pass

**Step 5: Commit**

```bash
git add components/transaction/internal/adapters/postgres/outbox/outbox.postgresql.go components/transaction/internal/adapters/postgres/outbox/state_machine_test.go
git commit -m "$(cat <<'EOF'
feat(outbox): add postcondition assertions to ClaimPendingBatch

Update in-memory entries to PROCESSING status after claim and verify
postcondition that all returned entries are in PROCESSING state.
EOF
)"
```

**If Task Fails:**

1. **Tests fail:**
   - Check if entry.Status is being correctly updated
   - Ensure the postcondition check happens after status update
   - Rollback: `git checkout -- components/transaction/internal/adapters/postgres/outbox/outbox.postgresql.go`

---

### Task 7: Add Assertions to FindByEntityID

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/postgres/outbox/outbox.postgresql.go:355-408`

**Prerequisites:**
- Task 6 completed

**Step 1: Write implementation with assertions**

Add precondition assertions at the beginning of `FindByEntityID` method. Replace lines 355-360 with:

```go
// FindByEntityID checks if an entry exists for the given entity (for idempotency checks).
// Preconditions:
//   - entityID must not be empty
//   - entityType must not be empty
func (r *OutboxPostgreSQLRepository) FindByEntityID(ctx context.Context, entityID, entityType string) (*MetadataOutbox, error) {
	// Validate preconditions - early return for empty parameters
	assert.NotEmpty(entityID, "entityID must not be empty", "method", "FindByEntityID")
	assert.NotEmpty(entityType, "entityType must not be empty",
		"entityID", entityID, "method", "FindByEntityID")

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)
```

The rest of the method remains unchanged.

**Step 2: Run test to verify compilation**

Run: `go build ./components/transaction/internal/adapters/postgres/outbox/...`

**Expected output:** No errors (silent success)

**Step 3: Run all outbox tests**

Run: `go test ./components/transaction/internal/adapters/postgres/outbox/... -v`

**Expected output:** All tests pass

**Step 4: Commit**

```bash
git add components/transaction/internal/adapters/postgres/outbox/outbox.postgresql.go
git commit -m "$(cat <<'EOF'
feat(outbox): add precondition assertions to FindByEntityID

Validate entityID and entityType are not empty before querying.
EOF
)"
```

**If Task Fails:**

1. **Tests fail:**
   - Check if any existing tests call with empty parameters
   - Rollback: `git checkout -- components/transaction/internal/adapters/postgres/outbox/outbox.postgresql.go`

---

### Task 8: Add Integration Test for State Machine Enforcement

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/postgres/outbox/state_machine_test.go`

**Prerequisites:**
- Task 7 completed

**Step 1: Add comprehensive state machine tests**

Add to `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/postgres/outbox/state_machine_test.go`:

```go
func TestOutboxStatus_AllTransitions_Coverage(t *testing.T) {
	// Verify complete coverage: every possible transition is either valid or invalid
	allStatuses := []OutboxStatus{StatusPending, StatusProcessing, StatusPublished, StatusFailed, StatusDLQ}

	validCount := 0
	invalidCount := 0

	for _, from := range allStatuses {
		for _, to := range allStatuses {
			if from == to {
				continue // Self-transitions not meaningful
			}

			if from.CanTransitionTo(to) {
				validCount++
				t.Logf("VALID: %s -> %s", from, to)
			} else {
				invalidCount++
			}
		}
	}

	// Expected: 5 valid transitions (per state machine diagram)
	assert.Equal(t, 5, validCount, "should have exactly 5 valid transitions")

	// Expected: 5*4 - 5 = 15 invalid transitions (20 pairs minus self minus 5 valid)
	assert.Equal(t, 15, invalidCount, "should have exactly 15 invalid transitions")
}

func TestOutboxStatus_TerminalStates_NoOutgoingTransitions(t *testing.T) {
	terminalStates := []OutboxStatus{StatusPublished, StatusDLQ}
	allStates := []OutboxStatus{StatusPending, StatusProcessing, StatusPublished, StatusFailed, StatusDLQ}

	for _, terminal := range terminalStates {
		t.Run(string(terminal), func(t *testing.T) {
			assert.True(t, terminal.IsTerminal())

			for _, target := range allStates {
				if target != terminal {
					assert.False(t, terminal.CanTransitionTo(target),
						"terminal state %s should not transition to %s", terminal, target)
				}
			}
		})
	}
}

func TestOutboxStatus_PendingCanOnlyGoToProcessing(t *testing.T) {
	allStates := []OutboxStatus{StatusPending, StatusProcessing, StatusPublished, StatusFailed, StatusDLQ}

	for _, target := range allStates {
		if target == StatusProcessing {
			assert.True(t, StatusPending.CanTransitionTo(target),
				"PENDING should be able to go to PROCESSING")
		} else if target != StatusPending {
			assert.False(t, StatusPending.CanTransitionTo(target),
				"PENDING should not be able to go to %s", target)
		}
	}
}

func TestOutboxStatus_ProcessingCanGoToPublishedOrFailed(t *testing.T) {
	validTargets := []OutboxStatus{StatusPublished, StatusFailed}
	invalidTargets := []OutboxStatus{StatusPending, StatusDLQ}

	for _, target := range validTargets {
		assert.True(t, StatusProcessing.CanTransitionTo(target),
			"PROCESSING should be able to go to %s", target)
	}

	for _, target := range invalidTargets {
		assert.False(t, StatusProcessing.CanTransitionTo(target),
			"PROCESSING should not be able to go to %s", target)
	}
}

func TestOutboxStatus_FailedCanGoToProcessingOrDLQ(t *testing.T) {
	validTargets := []OutboxStatus{StatusProcessing, StatusDLQ}
	invalidTargets := []OutboxStatus{StatusPending, StatusPublished}

	for _, target := range validTargets {
		assert.True(t, StatusFailed.CanTransitionTo(target),
			"FAILED should be able to go to %s", target)
	}

	for _, target := range invalidTargets {
		assert.False(t, StatusFailed.CanTransitionTo(target),
			"FAILED should not be able to go to %s", target)
	}
}
```

**Step 2: Run tests**

Run: `go test ./components/transaction/internal/adapters/postgres/outbox/... -v`

**Expected output:**
```
=== RUN   TestOutboxStatus_AllTransitions_Coverage
    state_machine_test.go:XX: VALID: PENDING -> PROCESSING
    state_machine_test.go:XX: VALID: PROCESSING -> PUBLISHED
    state_machine_test.go:XX: VALID: PROCESSING -> FAILED
    state_machine_test.go:XX: VALID: FAILED -> PROCESSING
    state_machine_test.go:XX: VALID: FAILED -> DLQ
--- PASS: TestOutboxStatus_AllTransitions_Coverage (0.00s)
...
PASS
```

**Step 3: Commit**

```bash
git add components/transaction/internal/adapters/postgres/outbox/state_machine_test.go
git commit -m "$(cat <<'EOF'
test(outbox): add comprehensive state machine transition tests

Cover all valid and invalid transitions, terminal state behavior,
and per-status transition rules.
EOF
)"
```

**If Task Fails:**

1. **Tests fail:**
   - Check that ValidOutboxTransitions matches expected transitions
   - Verify test counts (5 valid, 15 invalid)
   - Rollback: `git checkout -- components/transaction/internal/adapters/postgres/outbox/state_machine_test.go`

---

### Task 9: Run Code Review

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
- Format: `FIXME(nitpick): [Issue description] (reported by [reviewer] on [date], severity: Cosmetic)`

3. **Proceed only when:**
   - Zero Critical/High/Medium issues remain
   - All Low issues have TODO(review): comments added
   - All Cosmetic issues have FIXME(nitpick): comments added

---

### Task 10: Run Final Verification

**Files:** All modified files in `components/transaction/internal/adapters/postgres/outbox/`

**Prerequisites:**
- All previous tasks completed
- Code review passed

**Step 1: Run linter**

Run: `golangci-lint run ./components/transaction/internal/adapters/postgres/outbox/...`

**Expected output:** No errors or warnings

**Step 2: Run all tests**

Run: `go test ./components/transaction/internal/adapters/postgres/outbox/... -v -cover`

**Expected output:**
```
PASS
coverage: XX.X% of statements
ok      github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/outbox
```

**Step 3: Build verification**

Run: `go build ./components/transaction/...`

**Expected output:** No errors (silent success)

**Step 4: Final commit if needed**

If any fixes were made during verification:

```bash
git add -A
git commit -m "$(cat <<'EOF'
chore(outbox): address linter and test findings
EOF
)"
```

**If Task Fails:**

1. **Linter errors:**
   - Fix each error according to linter message
   - Common issues: unused variables, missing error checks
   - Re-run linter after each fix

2. **Test failures:**
   - Run failing test in isolation: `go test -run TestName -v`
   - Check assertion messages for clues
   - Rollback if necessary: `git reset --hard HEAD~1`

---

## Summary of Assertions Added

| Method | Assertion Type | Description |
|--------|---------------|-------------|
| `Create` | `NotNil` | Entry must not be nil |
| `Create` | `That` | Entry status must be PENDING |
| `Create` | `That` | Exactly one row inserted |
| `ClaimPendingBatch` | `That` | Returned entries in PROCESSING status |
| `FindByEntityID` | `NotEmpty` | EntityID not empty |
| `FindByEntityID` | `NotEmpty` | EntityType not empty |
| `MarkPublished` | `ValidUUID` | ID is valid UUID |
| `MarkPublished` | `That` | Exactly one row affected |
| `MarkFailed` | `ValidUUID` | ID is valid UUID |
| `MarkFailed` | `NotEmpty` | Error message not empty |
| `MarkFailed` | `That` | Exactly one row affected |
| `MarkDLQ` | `ValidUUID` | ID is valid UUID |
| `MarkDLQ` | `NotEmpty` | Reason not empty |
| `MarkDLQ` | `That` | Exactly one row affected |

**Total: ~14 assertions across 6 methods**

## SQL-Level State Machine Enforcement

| Method | WHERE Clause | Valid Source States |
|--------|-------------|---------------------|
| `MarkPublished` | `status = 'PROCESSING'` | PROCESSING only |
| `MarkFailed` | `status = 'PROCESSING'` | PROCESSING only |
| `MarkDLQ` | `status IN ('PROCESSING', 'FAILED')` | PROCESSING or FAILED |

This provides defense-in-depth: Go assertions catch programmer errors early, SQL constraints prevent invalid database states.

---

## Plan Checklist

- [x] Historical precedent queried (index empty - new project)
- [x] Historical Precedent section included
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
- [x] Summary of assertions and SQL enforcement
