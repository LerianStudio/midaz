# Reconciliation Refactoring Implementation Plan

> **For Agents:** REQUIRED SUB-SKILL: Use executing-plans to implement this plan task-by-task.

**Goal:** Refactor the reconciliation component to improve testability, reduce code duplication, and add missing test coverage while maintaining the existing behavior.

**Architecture:**
- Introduce a `ReconciliationChecker` interface to unify all checkers with different signatures
- Use a `CheckerConfig` struct to pass configuration parameters to checkers uniformly
- Extract common row iteration and status determination logic into helper functions
- Define sentinel errors for consistent error handling

**Tech Stack:** Go 1.24, PostgreSQL, sqlmock (testing), testify (assertions)

**Global Prerequisites:**
- Environment: macOS/Linux, Go 1.24+
- Tools: `go`, `make`
- Access: No external services required (all tests use mocks)
- State: Working from branch `fix/fred-several-ones-dec-13-2025`

**Verification before starting:**
```bash
# Run ALL these commands and verify output:
go version                    # Expected: go version go1.24.x
cd /Users/fredamaral/repos/lerianstudio/midaz && git status  # Expected: on branch fix/fred-several-ones-dec-13-2025
go build ./components/reconciliation/...  # Expected: success (no output)
go test ./components/reconciliation/... -count=1  # Expected: all tests pass
```

## Historical Precedent

**Query:** "reconciliation refactoring checker interface"
**Index Status:** Empty (new project)

### Successful Patterns to Reference
- Original plan: `docs/plans/2025-12-29-reconciliation-worker.md` established the component structure
- Test patterns in `balance_check_test.go` and `double_entry_check_test.go` use sqlmock effectively

### Related Past Plans
- `docs/plans/2025-12-29-reconciliation-worker.md`: Created the reconciliation component from scratch

---

## Phase 1: Quick Wins (Low Risk)

### Task 1: Remove empty mongodb directory

**Files:**
- Delete: `components/reconciliation/internal/adapters/mongodb/`

**Prerequisites:**
- Tools: bash
- Files must exist: the empty directory

**Agent:** `backend-engineer-golang`

**Step 1: Verify directory is empty and remove**

Run: `ls -la /Users/fredamaral/repos/lerianstudio/midaz/components/reconciliation/internal/adapters/mongodb/ && rm -rf /Users/fredamaral/repos/lerianstudio/midaz/components/reconciliation/internal/adapters/mongodb/`

**Expected output:**
```
total 0
drwxr-xr-x@ - fredamaral ...  .
drwxr-xr-x@ - fredamaral ...  ..
```

**Step 2: Verify removal**

Run: `ls /Users/fredamaral/repos/lerianstudio/midaz/components/reconciliation/internal/adapters/mongodb/ 2>&1`

**Expected output:**
```
ls: /Users/fredamaral/repos/lerianstudio/midaz/components/reconciliation/internal/adapters/mongodb/: No such file or directory
```

**If Task Fails:**
1. **Directory not empty:** List contents with `ls -la`, review files before deleting
2. **Permission denied:** Check file permissions with `ls -la ..`
3. **Can't recover:** Document what failed and return to human partner

---

### Task 2: Remove empty metrics directory

**Files:**
- Delete: `components/reconciliation/internal/adapters/metrics/`

**Prerequisites:**
- Tools: bash
- Files must exist: the empty directory

**Agent:** `backend-engineer-golang`

**Step 1: Verify directory is empty and remove**

Run: `ls -la /Users/fredamaral/repos/lerianstudio/midaz/components/reconciliation/internal/adapters/metrics/ && rm -rf /Users/fredamaral/repos/lerianstudio/midaz/components/reconciliation/internal/adapters/metrics/`

**Expected output:**
```
total 0
drwxr-xr-x@ - fredamaral ...  .
drwxr-xr-x@ - fredamaral ...  ..
```

**Step 2: Verify removal**

Run: `ls /Users/fredamaral/repos/lerianstudio/midaz/components/reconciliation/internal/adapters/metrics/ 2>&1`

**Expected output:**
```
ls: /Users/fredamaral/repos/lerianstudio/midaz/components/reconciliation/internal/adapters/metrics/: No such file or directory
```

**If Task Fails:**
1. **Directory not empty:** List contents with `ls -la`, review files before deleting
2. **Permission denied:** Check file permissions with `ls -la ..`
3. **Can't recover:** Document what failed and return to human partner

---

### Task 3: Add error wrapping to settlement.go GetUnsettledCount

**Files:**
- Modify: `components/reconciliation/internal/adapters/postgres/settlement.go:36`

**Prerequisites:**
- Tools: Go 1.24+
- Files must exist: `components/reconciliation/internal/adapters/postgres/settlement.go`

**Agent:** `backend-engineer-golang`

**Step 1: Add fmt import if not present**

The file already has `fmt` import, so skip this step.

**Step 2: Wrap the error in GetUnsettledCount**

Replace line 36 (the return statement in GetUnsettledCount):
```go
	return count, err
```

With:
```go
	if err != nil {
		return 0, fmt.Errorf("query unsettled transactions count: %w", err)
	}
	return count, nil
```

**Step 3: Verify file compiles**

Run: `go build /Users/fredamaral/repos/lerianstudio/midaz/components/reconciliation/internal/adapters/postgres/`

**Expected output:** (no output means success)

**Step 4: Run existing tests**

Run: `go test /Users/fredamaral/repos/lerianstudio/midaz/components/reconciliation/internal/adapters/postgres/ -run TestSettlement -v -count=1`

**Expected output:**
```
=== RUN   TestSettlementDetector_GetUnsettledCount
...
--- PASS: TestSettlementDetector_GetUnsettledCount
...
PASS
```

**If Task Fails:**
1. **Compile error:** Check syntax, ensure `fmt` is imported
2. **Test fails:** Review test expectations, they may need updating for new error message
3. **Can't recover:** `git checkout -- components/reconciliation/internal/adapters/postgres/settlement.go`

---

### Task 4: Add error wrapping to settlement.go GetSettledCount

**Files:**
- Modify: `components/reconciliation/internal/adapters/postgres/settlement.go:58`

**Prerequisites:**
- Tools: Go 1.24+
- Files must exist: `components/reconciliation/internal/adapters/postgres/settlement.go`
- Task 3 completed

**Agent:** `backend-engineer-golang`

**Step 1: Wrap the error in GetSettledCount**

Replace line 58 (the return statement in GetSettledCount):
```go
	return count, err
```

With:
```go
	if err != nil {
		return 0, fmt.Errorf("query settled transactions count: %w", err)
	}
	return count, nil
```

**Step 2: Verify file compiles**

Run: `go build /Users/fredamaral/repos/lerianstudio/midaz/components/reconciliation/internal/adapters/postgres/`

**Expected output:** (no output means success)

**Step 3: Run existing tests**

Run: `go test /Users/fredamaral/repos/lerianstudio/midaz/components/reconciliation/internal/adapters/postgres/ -run TestSettlement -v -count=1`

**Expected output:**
```
=== RUN   TestSettlementDetector_GetSettledCount
...
--- PASS: TestSettlementDetector_GetSettledCount
...
PASS
```

**If Task Fails:**
1. **Compile error:** Check syntax, ensure `fmt` is imported
2. **Test fails:** Review test expectations, they may need updating for new error message
3. **Can't recover:** `git checkout -- components/reconciliation/internal/adapters/postgres/settlement.go`

---

### Task 5: Add error wrapping to counts.go GetOnboardingCounts

**Files:**
- Modify: `components/reconciliation/internal/adapters/postgres/counts.go:58`

**Prerequisites:**
- Tools: Go 1.24+
- Files must exist: `components/reconciliation/internal/adapters/postgres/counts.go`

**Agent:** `backend-engineer-golang`

**Step 1: Add fmt import**

Add `"fmt"` to the imports section. The file currently has:
```go
import (
	"context"
	"database/sql"
)
```

Change to:
```go
import (
	"context"
	"database/sql"
	"fmt"
)
```

**Step 2: Wrap the error in GetOnboardingCounts**

Replace line 58 (the return statement in GetOnboardingCounts):
```go
	return counts, err
```

With:
```go
	if err != nil {
		return nil, fmt.Errorf("query onboarding entity counts: %w", err)
	}
	return counts, nil
```

**Step 3: Verify file compiles**

Run: `go build /Users/fredamaral/repos/lerianstudio/midaz/components/reconciliation/internal/adapters/postgres/`

**Expected output:** (no output means success)

**Step 4: Run existing tests**

Run: `go test /Users/fredamaral/repos/lerianstudio/midaz/components/reconciliation/internal/adapters/postgres/ -run TestEntityCounter -v -count=1`

**Expected output:**
```
=== RUN   TestEntityCounter_GetOnboardingCounts
...
--- PASS: TestEntityCounter_GetOnboardingCounts
...
PASS
```

**If Task Fails:**
1. **Compile error:** Check syntax, ensure `fmt` is imported correctly
2. **Test fails:** Review test expectations, they may need updating for new error message
3. **Can't recover:** `git checkout -- components/reconciliation/internal/adapters/postgres/counts.go`

---

### Task 6: Add error wrapping to counts.go GetTransactionCounts

**Files:**
- Modify: `components/reconciliation/internal/adapters/postgres/counts.go:77`

**Prerequisites:**
- Tools: Go 1.24+
- Files must exist: `components/reconciliation/internal/adapters/postgres/counts.go`
- Task 5 completed

**Agent:** `backend-engineer-golang`

**Step 1: Wrap the error in GetTransactionCounts**

Replace line 77 (the return statement in GetTransactionCounts):
```go
	return counts, err
```

With:
```go
	if err != nil {
		return nil, fmt.Errorf("query transaction entity counts: %w", err)
	}
	return counts, nil
```

**Step 2: Verify file compiles**

Run: `go build /Users/fredamaral/repos/lerianstudio/midaz/components/reconciliation/internal/adapters/postgres/`

**Expected output:** (no output means success)

**Step 3: Run existing tests**

Run: `go test /Users/fredamaral/repos/lerianstudio/midaz/components/reconciliation/internal/adapters/postgres/ -run TestEntityCounter -v -count=1`

**Expected output:**
```
=== RUN   TestEntityCounter_GetTransactionCounts
...
--- PASS: TestEntityCounter_GetTransactionCounts
...
PASS
```

**If Task Fails:**
1. **Compile error:** Check syntax
2. **Test fails:** Review test expectations, they may need updating for new error message
3. **Can't recover:** `git checkout -- components/reconciliation/internal/adapters/postgres/counts.go`

---

### Task 7: Create sentinel errors file

**Files:**
- Create: `components/reconciliation/internal/errors.go`

**Prerequisites:**
- Tools: Go 1.24+
- Directory must exist: `components/reconciliation/internal/`

**Agent:** `backend-engineer-golang`

**Step 1: Create the errors file**

Create file `components/reconciliation/internal/errors.go` with content:

```go
// Package internal provides internal types and errors for the reconciliation component.
package internal

import "errors"

// Sentinel errors for reconciliation operations.
var (
	// ErrQueryFailed indicates a database query failed.
	ErrQueryFailed = errors.New("query failed")

	// ErrScanFailed indicates a row scan operation failed.
	ErrScanFailed = errors.New("row scan failed")

	// ErrIterationFailed indicates row iteration failed.
	ErrIterationFailed = errors.New("row iteration failed")

	// ErrNoReport indicates no reconciliation report is available.
	ErrNoReport = errors.New("no reconciliation report available")

	// ErrReconciliationInProgress indicates a reconciliation is already running.
	ErrReconciliationInProgress = errors.New("reconciliation already in progress")
)
```

**Step 2: Verify file compiles**

Run: `go build /Users/fredamaral/repos/lerianstudio/midaz/components/reconciliation/internal/`

**Expected output:** (no output means success)

**If Task Fails:**
1. **Compile error:** Check package name matches directory
2. **Directory not found:** Verify path is correct
3. **Can't recover:** Remove the file and document the issue

---

### Code Review Checkpoint - Phase 1

**Dispatch all 3 reviewers in parallel:**
- REQUIRED SUB-SKILL: Use requesting-code-review
- All reviewers run simultaneously (code-reviewer, business-logic-reviewer, security-reviewer)
- Wait for all to complete

**Handle findings by severity (MANDATORY):**

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

**Proceed only when:**
- Zero Critical/High/Medium issues remain
- All Low issues have TODO(review): comments added
- All Cosmetic issues have FIXME(nitpick): comments added

---

## Phase 2: Interface Abstraction (Medium Risk)

### Task 8: Create ReconciliationChecker interface and CheckerConfig

**Files:**
- Create: `components/reconciliation/internal/adapters/postgres/checker.go`

**Prerequisites:**
- Tools: Go 1.24+
- Directory must exist: `components/reconciliation/internal/adapters/postgres/`

**Agent:** `backend-engineer-golang`

**Step 1: Create the checker interface file**

Create file `components/reconciliation/internal/adapters/postgres/checker.go` with content:

```go
package postgres

import (
	"context"
)

// CheckerConfig holds configuration parameters for reconciliation checkers.
// This allows uniform passing of config to checkers with different signatures.
type CheckerConfig struct {
	// DiscrepancyThreshold is the minimum discrepancy amount to report (for balance checker).
	DiscrepancyThreshold int64

	// MaxResults limits the number of detailed results returned.
	MaxResults int

	// StaleThresholdSeconds is the staleness threshold for sync checks.
	StaleThresholdSeconds int
}

// CheckResult is the common interface for all check results.
// Each checker returns its specific result type that implements this interface.
type CheckResult interface {
	// GetStatus returns the status of the check (HEALTHY, WARNING, CRITICAL, ERROR).
	GetStatus() string
}

// ReconciliationChecker is the common interface for all reconciliation checkers.
// It allows the engine to treat all checkers uniformly despite their different parameters.
type ReconciliationChecker interface {
	// Name returns the unique name of this checker.
	Name() string

	// Check performs the reconciliation check and returns the result.
	// The config provides all necessary parameters.
	Check(ctx context.Context, config CheckerConfig) (CheckResult, error)
}
```

**Step 2: Verify file compiles**

Run: `go build /Users/fredamaral/repos/lerianstudio/midaz/components/reconciliation/internal/adapters/postgres/`

**Expected output:** (no output means success)

**If Task Fails:**
1. **Compile error:** Check syntax and package name
2. **Import errors:** Ensure context is imported
3. **Can't recover:** Remove the file and document the issue

---

### Task 9: Create status determination helper

**Files:**
- Create: `components/reconciliation/internal/adapters/postgres/helpers.go`

**Prerequisites:**
- Tools: Go 1.24+
- Directory must exist: `components/reconciliation/internal/adapters/postgres/`

**Agent:** `backend-engineer-golang`

**Step 1: Create the helpers file**

Create file `components/reconciliation/internal/adapters/postgres/helpers.go` with content:

```go
package postgres

// StatusThresholds defines the thresholds for status determination.
type StatusThresholds struct {
	// WarningThreshold: if issues <= this, status is WARNING (else CRITICAL).
	// If issues == 0, status is HEALTHY.
	WarningThreshold int

	// CriticalOnAny: if true, any issue is CRITICAL (used for double-entry).
	CriticalOnAny bool
}

// DetermineStatus calculates the status based on issue count and thresholds.
// Returns "HEALTHY", "WARNING", or "CRITICAL".
func DetermineStatus(issueCount int, thresholds StatusThresholds) string {
	if issueCount == 0 {
		return "HEALTHY"
	}

	if thresholds.CriticalOnAny {
		return "CRITICAL"
	}

	if issueCount <= thresholds.WarningThreshold {
		return "WARNING"
	}

	return "CRITICAL"
}

// DetermineStatusWithPartial calculates status when there are both critical and partial issues.
// criticalCount: issues that are critical (e.g., orphan transactions)
// partialCount: issues that are warnings (e.g., partial transactions)
func DetermineStatusWithPartial(criticalCount, partialCount int) string {
	if criticalCount == 0 && partialCount == 0 {
		return "HEALTHY"
	}

	if criticalCount > 0 {
		return "CRITICAL"
	}

	return "WARNING"
}
```

**Step 2: Verify file compiles**

Run: `go build /Users/fredamaral/repos/lerianstudio/midaz/components/reconciliation/internal/adapters/postgres/`

**Expected output:** (no output means success)

**If Task Fails:**
1. **Compile error:** Check syntax and package name
2. **Can't recover:** Remove the file and document the issue

---

### Task 10: Create helpers test file

**Files:**
- Create: `components/reconciliation/internal/adapters/postgres/helpers_test.go`

**Prerequisites:**
- Tools: Go 1.24+
- Task 9 completed

**Agent:** `backend-engineer-golang`

**Step 1: Create the test file**

Create file `components/reconciliation/internal/adapters/postgres/helpers_test.go` with content:

```go
package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDetermineStatus_Healthy(t *testing.T) {
	t.Parallel()

	result := DetermineStatus(0, StatusThresholds{WarningThreshold: 10})
	assert.Equal(t, "HEALTHY", result)
}

func TestDetermineStatus_Warning(t *testing.T) {
	t.Parallel()

	result := DetermineStatus(5, StatusThresholds{WarningThreshold: 10})
	assert.Equal(t, "WARNING", result)
}

func TestDetermineStatus_Critical(t *testing.T) {
	t.Parallel()

	result := DetermineStatus(15, StatusThresholds{WarningThreshold: 10})
	assert.Equal(t, "CRITICAL", result)
}

func TestDetermineStatus_CriticalOnAny(t *testing.T) {
	t.Parallel()

	result := DetermineStatus(1, StatusThresholds{CriticalOnAny: true})
	assert.Equal(t, "CRITICAL", result)
}

func TestDetermineStatus_CriticalOnAnyZero(t *testing.T) {
	t.Parallel()

	// Even with CriticalOnAny, zero issues is HEALTHY
	result := DetermineStatus(0, StatusThresholds{CriticalOnAny: true})
	assert.Equal(t, "HEALTHY", result)
}

func TestDetermineStatusWithPartial_Healthy(t *testing.T) {
	t.Parallel()

	result := DetermineStatusWithPartial(0, 0)
	assert.Equal(t, "HEALTHY", result)
}

func TestDetermineStatusWithPartial_Warning(t *testing.T) {
	t.Parallel()

	result := DetermineStatusWithPartial(0, 5)
	assert.Equal(t, "WARNING", result)
}

func TestDetermineStatusWithPartial_Critical(t *testing.T) {
	t.Parallel()

	result := DetermineStatusWithPartial(3, 5)
	assert.Equal(t, "CRITICAL", result)
}
```

**Step 2: Run the tests**

Run: `go test /Users/fredamaral/repos/lerianstudio/midaz/components/reconciliation/internal/adapters/postgres/ -run TestDetermineStatus -v -count=1`

**Expected output:**
```
=== RUN   TestDetermineStatus_Healthy
--- PASS: TestDetermineStatus_Healthy (0.00s)
=== RUN   TestDetermineStatus_Warning
--- PASS: TestDetermineStatus_Warning (0.00s)
=== RUN   TestDetermineStatus_Critical
--- PASS: TestDetermineStatus_Critical (0.00s)
=== RUN   TestDetermineStatus_CriticalOnAny
--- PASS: TestDetermineStatus_CriticalOnAny (0.00s)
=== RUN   TestDetermineStatus_CriticalOnAnyZero
--- PASS: TestDetermineStatus_CriticalOnAnyZero (0.00s)
=== RUN   TestDetermineStatusWithPartial_Healthy
--- PASS: TestDetermineStatusWithPartial_Healthy (0.00s)
=== RUN   TestDetermineStatusWithPartial_Warning
--- PASS: TestDetermineStatusWithPartial_Warning (0.00s)
=== RUN   TestDetermineStatusWithPartial_Critical
--- PASS: TestDetermineStatusWithPartial_Critical (0.00s)
PASS
```

**If Task Fails:**
1. **Test fails:** Check helper logic matches test expectations
2. **Compile error:** Check imports and function signatures
3. **Can't recover:** Remove the file and document the issue

---

### Task 11: Add GetStatus method to BalanceCheckResult

**Files:**
- Modify: `components/reconciliation/internal/domain/report.go`

**Prerequisites:**
- Tools: Go 1.24+
- Task 8 completed (CheckResult interface exists)

**Agent:** `backend-engineer-golang`

**Step 1: Add GetStatus method to BalanceCheckResult**

Add after the `BalanceCheckResult` struct definition (around line 35):

```go
// GetStatus implements CheckResult interface.
func (r *BalanceCheckResult) GetStatus() string {
	return r.Status
}
```

**Step 2: Verify file compiles**

Run: `go build /Users/fredamaral/repos/lerianstudio/midaz/components/reconciliation/internal/domain/`

**Expected output:** (no output means success)

**If Task Fails:**
1. **Compile error:** Check method receiver syntax
2. **Can't recover:** `git checkout -- components/reconciliation/internal/domain/report.go`

---

### Task 12: Add GetStatus method to DoubleEntryCheckResult

**Files:**
- Modify: `components/reconciliation/internal/domain/report.go`

**Prerequisites:**
- Tools: Go 1.24+
- Task 11 completed

**Agent:** `backend-engineer-golang`

**Step 1: Add GetStatus method to DoubleEntryCheckResult**

Add after the `DoubleEntryCheckResult` struct definition (around line 58):

```go
// GetStatus implements CheckResult interface.
func (r *DoubleEntryCheckResult) GetStatus() string {
	return r.Status
}
```

**Step 2: Verify file compiles**

Run: `go build /Users/fredamaral/repos/lerianstudio/midaz/components/reconciliation/internal/domain/`

**Expected output:** (no output means success)

**If Task Fails:**
1. **Compile error:** Check method receiver syntax
2. **Can't recover:** `git checkout -- components/reconciliation/internal/domain/report.go`

---

### Task 13: Add GetStatus method to ReferentialCheckResult

**Files:**
- Modify: `components/reconciliation/internal/domain/report.go`

**Prerequisites:**
- Tools: Go 1.24+
- Task 12 completed

**Agent:** `backend-engineer-golang`

**Step 1: Add GetStatus method to ReferentialCheckResult**

Add after the `ReferentialCheckResult` struct definition (around line 80):

```go
// GetStatus implements CheckResult interface.
func (r *ReferentialCheckResult) GetStatus() string {
	return r.Status
}
```

**Step 2: Verify file compiles**

Run: `go build /Users/fredamaral/repos/lerianstudio/midaz/components/reconciliation/internal/domain/`

**Expected output:** (no output means success)

**If Task Fails:**
1. **Compile error:** Check method receiver syntax
2. **Can't recover:** `git checkout -- components/reconciliation/internal/domain/report.go`

---

### Task 14: Add GetStatus method to SyncCheckResult

**Files:**
- Modify: `components/reconciliation/internal/domain/report.go`

**Prerequisites:**
- Tools: Go 1.24+
- Task 13 completed

**Agent:** `backend-engineer-golang`

**Step 1: Add GetStatus method to SyncCheckResult**

Add after the `SyncCheckResult` struct definition (around line 96):

```go
// GetStatus implements CheckResult interface.
func (r *SyncCheckResult) GetStatus() string {
	return r.Status
}
```

**Step 2: Verify file compiles**

Run: `go build /Users/fredamaral/repos/lerianstudio/midaz/components/reconciliation/internal/domain/`

**Expected output:** (no output means success)

**If Task Fails:**
1. **Compile error:** Check method receiver syntax
2. **Can't recover:** `git checkout -- components/reconciliation/internal/domain/report.go`

---

### Task 15: Add GetStatus method to OrphanCheckResult

**Files:**
- Modify: `components/reconciliation/internal/domain/report.go`

**Prerequisites:**
- Tools: Go 1.24+
- Task 14 completed

**Agent:** `backend-engineer-golang`

**Step 1: Add GetStatus method to OrphanCheckResult**

Add after the `OrphanCheckResult` struct definition (around line 114):

```go
// GetStatus implements CheckResult interface.
func (r *OrphanCheckResult) GetStatus() string {
	return r.Status
}
```

**Step 2: Verify file compiles**

Run: `go build /Users/fredamaral/repos/lerianstudio/midaz/components/reconciliation/internal/domain/`

**Expected output:** (no output means success)

**If Task Fails:**
1. **Compile error:** Check method receiver syntax
2. **Can't recover:** `git checkout -- components/reconciliation/internal/domain/report.go`

---

### Task 16: Add GetStatus method to MetadataCheckResult

**Files:**
- Modify: `components/reconciliation/internal/domain/report.go`

**Prerequisites:**
- Tools: Go 1.24+
- Task 15 completed

**Agent:** `backend-engineer-golang`

**Step 1: Add GetStatus method to MetadataCheckResult**

Add after the `MetadataCheckResult` struct definition (around line 134):

```go
// GetStatus implements CheckResult interface.
func (r *MetadataCheckResult) GetStatus() string {
	return r.Status
}
```

**Step 2: Verify file compiles**

Run: `go build /Users/fredamaral/repos/lerianstudio/midaz/components/reconciliation/internal/domain/`

**Expected output:** (no output means success)

**Step 3: Run all domain tests**

Run: `go test /Users/fredamaral/repos/lerianstudio/midaz/components/reconciliation/internal/domain/ -v -count=1`

**Expected output:**
```
=== RUN   TestDetermineOverallStatus
...
PASS
```

**If Task Fails:**
1. **Compile error:** Check method receiver syntax
2. **Test fails:** Review test logic
3. **Can't recover:** `git checkout -- components/reconciliation/internal/domain/report.go`

---

### Code Review Checkpoint - Phase 2

**Dispatch all 3 reviewers in parallel:**
- REQUIRED SUB-SKILL: Use requesting-code-review
- All reviewers run simultaneously (code-reviewer, business-logic-reviewer, security-reviewer)
- Wait for all to complete

**Handle findings by severity (MANDATORY):**

**Critical/High/Medium Issues:**
- Fix immediately (do NOT add TODO comments for these severities)
- Re-run all 3 reviewers in parallel after fixes
- Repeat until zero Critical/High/Medium issues remain

**Low Issues:**
- Add `TODO(review):` comments in code at the relevant location

**Cosmetic/Nitpick Issues:**
- Add `FIXME(nitpick):` comments in code at the relevant location

**Proceed only when:**
- Zero Critical/High/Medium issues remain
- All Low issues have TODO(review): comments added
- All Cosmetic issues have FIXME(nitpick): comments added

---

## Phase 3: Test Coverage (Medium-High Risk)

### Task 17: Create SyncChecker test file

**Files:**
- Create: `components/reconciliation/internal/adapters/postgres/sync_check_test.go`

**Prerequisites:**
- Tools: Go 1.24+, sqlmock
- Files must exist: `components/reconciliation/internal/adapters/postgres/sync_check.go`

**Agent:** `backend-engineer-golang`

**Step 1: Create the test file**

Create file `components/reconciliation/internal/adapters/postgres/sync_check_test.go` with content:

```go
package postgres

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSyncChecker_Check_NoIssues(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Empty result set - no version mismatches or stale balances
	rows := sqlmock.NewRows([]string{
		"balance_id", "alias", "asset_code", "db_version",
		"max_op_version", "staleness_seconds",
	})
	mock.ExpectQuery(`WITH balance_ops AS`).
		WithArgs(300, 10).
		WillReturnRows(rows)

	checker := NewSyncChecker(db)
	result, err := checker.Check(context.Background(), 300, 10)

	require.NoError(t, err)
	assert.Equal(t, "HEALTHY", result.Status)
	assert.Equal(t, 0, result.VersionMismatches)
	assert.Equal(t, 0, result.StaleBalances)
	assert.Empty(t, result.Issues)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSyncChecker_Check_VersionMismatch(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	rows := sqlmock.NewRows([]string{
		"balance_id", "alias", "asset_code", "db_version",
		"max_op_version", "staleness_seconds",
	}).AddRow("bal-1", "account1", "USD", int32(5), int32(10), int64(100))

	mock.ExpectQuery(`WITH balance_ops AS`).
		WithArgs(300, 10).
		WillReturnRows(rows)

	checker := NewSyncChecker(db)
	result, err := checker.Check(context.Background(), 300, 10)

	require.NoError(t, err)
	assert.Equal(t, "WARNING", result.Status)
	assert.Equal(t, 1, result.VersionMismatches)
	assert.Equal(t, 0, result.StaleBalances)
	assert.Len(t, result.Issues, 1)
	assert.Equal(t, "bal-1", result.Issues[0].BalanceID)
	assert.Equal(t, int32(5), result.Issues[0].DBVersion)
	assert.Equal(t, int32(10), result.Issues[0].MaxOpVersion)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSyncChecker_Check_StaleBalance(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Balance version matches but staleness exceeds threshold
	rows := sqlmock.NewRows([]string{
		"balance_id", "alias", "asset_code", "db_version",
		"max_op_version", "staleness_seconds",
	}).AddRow("bal-2", "account2", "EUR", int32(10), int32(10), int64(500))

	mock.ExpectQuery(`WITH balance_ops AS`).
		WithArgs(300, 10).
		WillReturnRows(rows)

	checker := NewSyncChecker(db)
	result, err := checker.Check(context.Background(), 300, 10)

	require.NoError(t, err)
	assert.Equal(t, "WARNING", result.Status)
	assert.Equal(t, 0, result.VersionMismatches)
	assert.Equal(t, 1, result.StaleBalances)
	assert.Len(t, result.Issues, 1)
	assert.Equal(t, "bal-2", result.Issues[0].BalanceID)
	assert.Equal(t, int64(500), result.Issues[0].StalenessSeconds)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSyncChecker_Check_CriticalIssues(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// 10+ issues triggers CRITICAL status
	rows := sqlmock.NewRows([]string{
		"balance_id", "alias", "asset_code", "db_version",
		"max_op_version", "staleness_seconds",
	})
	for i := 0; i < 12; i++ {
		rows.AddRow("bal-"+string(rune('a'+i)), "account", "USD", int32(i), int32(i+1), int64(100))
	}

	mock.ExpectQuery(`WITH balance_ops AS`).
		WithArgs(300, 20).
		WillReturnRows(rows)

	checker := NewSyncChecker(db)
	result, err := checker.Check(context.Background(), 300, 20)

	require.NoError(t, err)
	assert.Equal(t, "CRITICAL", result.Status)
	assert.Equal(t, 12, result.VersionMismatches)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSyncChecker_Check_QueryError(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	mock.ExpectQuery(`WITH balance_ops AS`).
		WithArgs(300, 10).
		WillReturnError(assert.AnError)

	checker := NewSyncChecker(db)
	result, err := checker.Check(context.Background(), 300, 10)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "sync check query failed")
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSyncChecker_Check_BothVersionMismatchAndStale(t *testing.T) {
	t.Parallel()

	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()

	// Balance has both version mismatch AND staleness issue
	rows := sqlmock.NewRows([]string{
		"balance_id", "alias", "asset_code", "db_version",
		"max_op_version", "staleness_seconds",
	}).AddRow("bal-both", "account-both", "USD", int32(5), int32(10), int64(500))

	mock.ExpectQuery(`WITH balance_ops AS`).
		WithArgs(300, 10).
		WillReturnRows(rows)

	checker := NewSyncChecker(db)
	result, err := checker.Check(context.Background(), 300, 10)

	require.NoError(t, err)
	assert.Equal(t, "WARNING", result.Status)
	assert.Equal(t, 1, result.VersionMismatches)
	assert.Equal(t, 1, result.StaleBalances)
	assert.Len(t, result.Issues, 1)
	assert.NoError(t, mock.ExpectationsWereMet())
}
```

**Step 2: Run the tests**

Run: `go test /Users/fredamaral/repos/lerianstudio/midaz/components/reconciliation/internal/adapters/postgres/ -run TestSyncChecker -v -count=1`

**Expected output:**
```
=== RUN   TestSyncChecker_Check_NoIssues
--- PASS: TestSyncChecker_Check_NoIssues (0.00s)
=== RUN   TestSyncChecker_Check_VersionMismatch
--- PASS: TestSyncChecker_Check_VersionMismatch (0.00s)
=== RUN   TestSyncChecker_Check_StaleBalance
--- PASS: TestSyncChecker_Check_StaleBalance (0.00s)
=== RUN   TestSyncChecker_Check_CriticalIssues
--- PASS: TestSyncChecker_Check_CriticalIssues (0.00s)
=== RUN   TestSyncChecker_Check_QueryError
--- PASS: TestSyncChecker_Check_QueryError (0.00s)
=== RUN   TestSyncChecker_Check_BothVersionMismatchAndStale
--- PASS: TestSyncChecker_Check_BothVersionMismatchAndStale (0.00s)
PASS
```

**If Task Fails:**
1. **Test fails:** Check mock expectations match actual SQL patterns
2. **Compile error:** Check imports and test syntax
3. **Can't recover:** Remove the file and document the issue

---

### Task 18: Create Engine test file with mock checkers

**Files:**
- Create: `components/reconciliation/internal/engine/reconciliation_test.go`

**Prerequisites:**
- Tools: Go 1.24+
- Files must exist: `components/reconciliation/internal/engine/reconciliation.go`

**Agent:** `backend-engineer-golang`

**Step 1: Create the test file**

Create file `components/reconciliation/internal/engine/reconciliation_test.go` with content:

```go
package engine

import (
	"sync"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/domain"
	"github.com/stretchr/testify/assert"
)

// mockLogger implements the Logger interface for testing
type mockLogger struct {
	mu       sync.Mutex
	infos    []string
	errors   []string
	warnings []string
}

func newMockLogger() *mockLogger {
	return &mockLogger{
		infos:    make([]string, 0),
		errors:   make([]string, 0),
		warnings: make([]string, 0),
	}
}

func (l *mockLogger) Info(args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.infos = append(l.infos, "info")
}

func (l *mockLogger) Infof(format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.infos = append(l.infos, format)
}

func (l *mockLogger) Error(args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.errors = append(l.errors, "error")
}

func (l *mockLogger) Errorf(format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.errors = append(l.errors, format)
}

func (l *mockLogger) Warnf(format string, args ...interface{}) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.warnings = append(l.warnings, format)
}

func TestReconciliationEngine_GetLastReport_Nil(t *testing.T) {
	t.Parallel()

	engine := &ReconciliationEngine{
		logger: newMockLogger(),
	}

	report := engine.GetLastReport()
	assert.Nil(t, report)
}

func TestReconciliationEngine_GetLastReport_NotNil(t *testing.T) {
	t.Parallel()

	engine := &ReconciliationEngine{
		logger: newMockLogger(),
		lastReport: &domain.ReconciliationReport{
			Status: "HEALTHY",
		},
	}

	report := engine.GetLastReport()
	assert.NotNil(t, report)
	assert.Equal(t, "HEALTHY", report.Status)
}

func TestReconciliationEngine_IsHealthy_NoReport(t *testing.T) {
	t.Parallel()

	engine := &ReconciliationEngine{
		logger: newMockLogger(),
	}

	assert.False(t, engine.IsHealthy())
}

func TestReconciliationEngine_IsHealthy_Healthy(t *testing.T) {
	t.Parallel()

	engine := &ReconciliationEngine{
		logger: newMockLogger(),
		lastReport: &domain.ReconciliationReport{
			Status: "HEALTHY",
		},
	}

	assert.True(t, engine.IsHealthy())
}

func TestReconciliationEngine_IsHealthy_Warning(t *testing.T) {
	t.Parallel()

	engine := &ReconciliationEngine{
		logger: newMockLogger(),
		lastReport: &domain.ReconciliationReport{
			Status: "WARNING",
		},
	}

	// WARNING is not CRITICAL, so considered healthy
	assert.True(t, engine.IsHealthy())
}

func TestReconciliationEngine_IsHealthy_Critical(t *testing.T) {
	t.Parallel()

	engine := &ReconciliationEngine{
		logger: newMockLogger(),
		lastReport: &domain.ReconciliationReport{
			Status: "CRITICAL",
		},
	}

	assert.False(t, engine.IsHealthy())
}

func TestReconciliationEngine_GetLastReport_ThreadSafe(t *testing.T) {
	t.Parallel()

	engine := &ReconciliationEngine{
		logger: newMockLogger(),
		lastReport: &domain.ReconciliationReport{
			Status: "HEALTHY",
		},
	}

	// Run multiple concurrent reads
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			report := engine.GetLastReport()
			assert.NotNil(t, report)
		}()
	}
	wg.Wait()
}

func TestReconciliationEngine_IsHealthy_ThreadSafe(t *testing.T) {
	t.Parallel()

	engine := &ReconciliationEngine{
		logger: newMockLogger(),
		lastReport: &domain.ReconciliationReport{
			Status: "HEALTHY",
		},
	}

	// Run multiple concurrent reads
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = engine.IsHealthy()
		}()
	}
	wg.Wait()
}
```

**Step 2: Run the tests**

Run: `go test /Users/fredamaral/repos/lerianstudio/midaz/components/reconciliation/internal/engine/ -v -count=1`

**Expected output:**
```
=== RUN   TestReconciliationEngine_GetLastReport_Nil
--- PASS: TestReconciliationEngine_GetLastReport_Nil (0.00s)
=== RUN   TestReconciliationEngine_GetLastReport_NotNil
--- PASS: TestReconciliationEngine_GetLastReport_NotNil (0.00s)
=== RUN   TestReconciliationEngine_IsHealthy_NoReport
--- PASS: TestReconciliationEngine_IsHealthy_NoReport (0.00s)
=== RUN   TestReconciliationEngine_IsHealthy_Healthy
--- PASS: TestReconciliationEngine_IsHealthy_Healthy (0.00s)
=== RUN   TestReconciliationEngine_IsHealthy_Warning
--- PASS: TestReconciliationEngine_IsHealthy_Warning (0.00s)
=== RUN   TestReconciliationEngine_IsHealthy_Critical
--- PASS: TestReconciliationEngine_IsHealthy_Critical (0.00s)
=== RUN   TestReconciliationEngine_GetLastReport_ThreadSafe
--- PASS: TestReconciliationEngine_GetLastReport_ThreadSafe (0.00s)
=== RUN   TestReconciliationEngine_IsHealthy_ThreadSafe
--- PASS: TestReconciliationEngine_IsHealthy_ThreadSafe (0.00s)
PASS
```

**If Task Fails:**
1. **Test fails:** Check mock logger implementation
2. **Compile error:** Check imports and test syntax
3. **Can't recover:** Remove the file and document the issue

---

### Code Review Checkpoint - Phase 3

**Dispatch all 3 reviewers in parallel:**
- REQUIRED SUB-SKILL: Use requesting-code-review
- All reviewers run simultaneously (code-reviewer, business-logic-reviewer, security-reviewer)
- Wait for all to complete

**Handle findings by severity (MANDATORY):**

**Critical/High/Medium Issues:**
- Fix immediately
- Re-run all 3 reviewers in parallel after fixes
- Repeat until zero Critical/High/Medium issues remain

**Low Issues:**
- Add `TODO(review):` comments in code at the relevant location

**Cosmetic/Nitpick Issues:**
- Add `FIXME(nitpick):` comments in code at the relevant location

**Proceed only when:**
- Zero Critical/High/Medium issues remain
- All Low issues have TODO(review): comments added
- All Cosmetic issues have FIXME(nitpick): comments added

---

## Phase 4: Final Verification

### Task 19: Run full test suite

**Files:**
- No file changes

**Prerequisites:**
- All previous tasks completed
- Tools: Go 1.24+

**Agent:** `backend-engineer-golang`

**Step 1: Run all reconciliation tests**

Run: `go test /Users/fredamaral/repos/lerianstudio/midaz/components/reconciliation/... -v -count=1 -race`

**Expected output:**
```
...
ok      github.com/LerianStudio/midaz/v3/components/reconciliation/internal/adapters/postgres   X.XXXs
ok      github.com/LerianStudio/midaz/v3/components/reconciliation/internal/domain              X.XXXs
ok      github.com/LerianStudio/midaz/v3/components/reconciliation/internal/engine              X.XXXs
ok      github.com/LerianStudio/midaz/v3/components/reconciliation/pkg/safego                   X.XXXs
```

All packages should pass.

**Step 2: Verify build**

Run: `go build /Users/fredamaral/repos/lerianstudio/midaz/components/reconciliation/...`

**Expected output:** (no output means success)

**Step 3: Check for test coverage improvement**

Run: `go test /Users/fredamaral/repos/lerianstudio/midaz/components/reconciliation/internal/adapters/postgres/ -cover`

**Expected output:**
```
ok      github.com/LerianStudio/midaz/v3/components/reconciliation/internal/adapters/postgres   X.XXXs  coverage: XX.X% of statements
```

Coverage should be higher than before (previously some checkers had 0% coverage).

**If Task Fails:**
1. **Tests fail:** Review failed test output, fix issues
2. **Build fails:** Check for syntax errors introduced
3. **Coverage lower:** Review which tests are missing

---

### Task 20: Verify no regressions in other components

**Files:**
- No file changes

**Prerequisites:**
- Task 19 completed
- Tools: Go 1.24+

**Agent:** `backend-engineer-golang`

**Step 1: Build entire project**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./...`

**Expected output:** (no output means success)

**Step 2: Run linting**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && golangci-lint run ./components/reconciliation/...`

**Expected output:** (no output or only minor warnings)

**If Task Fails:**
1. **Build fails:** Check for import issues or syntax errors
2. **Lint fails:** Fix lint issues, they are usually minor
3. **Can't recover:** Document what failed

---

## Final Summary

### Files Created
- `components/reconciliation/internal/errors.go` - Sentinel errors
- `components/reconciliation/internal/adapters/postgres/checker.go` - Interface definitions
- `components/reconciliation/internal/adapters/postgres/helpers.go` - Status determination helpers
- `components/reconciliation/internal/adapters/postgres/helpers_test.go` - Helper tests
- `components/reconciliation/internal/adapters/postgres/sync_check_test.go` - SyncChecker tests
- `components/reconciliation/internal/engine/reconciliation_test.go` - Engine tests

### Files Modified
- `components/reconciliation/internal/adapters/postgres/settlement.go` - Error wrapping
- `components/reconciliation/internal/adapters/postgres/counts.go` - Error wrapping
- `components/reconciliation/internal/domain/report.go` - GetStatus methods

### Files Deleted
- `components/reconciliation/internal/adapters/mongodb/` - Empty directory
- `components/reconciliation/internal/adapters/metrics/` - Empty directory

### Improvements Made
1. Removed dead code (empty directories)
2. Added proper error wrapping with context
3. Created sentinel errors for consistent error handling
4. Added ReconciliationChecker interface for future testability improvements
5. Added GetStatus() method to all check result types
6. Added status determination helper functions
7. Added SyncChecker tests (previously 0% coverage)
8. Added Engine tests (previously 0% coverage)

### Future Work (Not in this plan)
- Refactor engine to accept checker map via dependency injection
- Update bootstrap to inject checkers as interfaces
- Refactor checkers to implement ReconciliationChecker interface
- Extract more duplicated code using helper functions

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
- [x] Plan avoids known failure patterns (none found in precedent)
