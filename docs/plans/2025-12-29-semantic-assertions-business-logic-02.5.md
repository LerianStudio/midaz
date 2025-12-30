# Semantic Business Logic Assertions Implementation Plan

> **For Agents:** REQUIRED SUB-SKILL: Use executing-plans to implement this plan task-by-task.

**Goal:** Add semantic assertions that validate business invariants beyond null checks - enforcing double-entry accounting, transaction state machine rules, balance sufficiency, and temporal constraints.

**Architecture:** Extend `pkg/assert/predicates.go` with new business-logic predicates, then apply assertions at critical business rule enforcement points in transaction, onboarding, and CRM components. Each assertion guards an invariant that, if violated, indicates a programming bug - not user input errors.

**Tech Stack:** Go, `pkg/assert` package, `github.com/shopspring/decimal`, `pkg/constant`

**Global Prerequisites:**
- Environment: Go 1.22+, macOS/Linux
- Tools: `go`, `golangci-lint`
- State: Working from branch `fix/fred-several-ones-dec-13-2025`

**Verification before starting:**
```bash
# Run ALL these commands and verify output:
go version       # Expected: go version go1.22+ darwin/arm64
git status       # Expected: clean working tree or known changes
go build ./...   # Expected: builds successfully
go test ./pkg/assert/... -v  # Expected: all tests pass
```

## Historical Precedent

**Query:** "semantic assertions business logic transaction validation"
**Index Status:** Empty (new project)

No historical data available. This is normal for new projects.
Proceeding with standard planning approach.

---

## Overview

This plan adds ~25 business logic assertions across transaction, onboarding, and CRM components. Tasks are organized by priority:

| Priority | Tasks | Focus |
|----------|-------|-------|
| CRITICAL | 1-6 | Double-entry accounting invariant |
| HIGH | 7-20 | Transaction state machine, balance sufficiency, parent account |
| MEDIUM | 21-31 | Temporal constraints, balance deletion, external account rules |

**New Predicates to Add:**
- `ValidTransactionStatus(status string) bool`
- `TransactionCanTransitionTo(current, target string) bool`
- `BalanceSufficientForRelease(onHold, releaseAmount decimal.Decimal) bool`
- `DebitsEqualCredits(debits, credits decimal.Decimal) bool`
- `DateNotInFuture(t time.Time) bool`
- `DateAfter(date, reference time.Time) bool`
- `BalanceIsZero(available, onHold decimal.Decimal) bool`
- `TransactionHasOperations(operations []*mmodel.Operation) bool`

---

# CRITICAL PRIORITY: Double-Entry Accounting Invariant

---

### Task 1: Add DebitsEqualCredits Predicate - Test First

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates_test.go`

**Prerequisites:**
- Go 1.22+
- `pkg/assert` package exists

**Step 1: Write the failing test**

Add to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates_test.go`:

```go
// TestDebitsEqualCredits tests the DebitsEqualCredits predicate for double-entry accounting.
func TestDebitsEqualCredits(t *testing.T) {
	tests := []struct {
		name     string
		debits   decimal.Decimal
		credits  decimal.Decimal
		expected bool
	}{
		{"equal positive amounts", decimal.NewFromInt(100), decimal.NewFromInt(100), true},
		{"equal with decimals", decimal.NewFromFloat(123.45), decimal.NewFromFloat(123.45), true},
		{"equal zero", decimal.Zero, decimal.Zero, true},
		{"debits greater", decimal.NewFromInt(100), decimal.NewFromInt(99), false},
		{"credits greater", decimal.NewFromInt(99), decimal.NewFromInt(100), false},
		{"tiny difference", decimal.NewFromFloat(100.001), decimal.NewFromFloat(100.002), false},
		{"large equal", decimal.NewFromInt(1000000000), decimal.NewFromInt(1000000000), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, DebitsEqualCredits(tt.debits, tt.credits))
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run "TestDebitsEqualCredits" /Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/`

**Expected output:**
```
# github.com/LerianStudio/midaz/v3/pkg/assert [github.com/LerianStudio/midaz/v3/pkg/assert.test]
./predicates_test.go:XX:XX: undefined: DebitsEqualCredits
FAIL
```

**If you see different error:** Check that the test is in the correct package (`assert`) and imports are correct.

**Step 3: Commit**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates_test.go
git commit -m "$(cat <<'EOF'
test(assert): add failing test for DebitsEqualCredits predicate

TDD red phase - test for double-entry accounting invariant validation.
EOF
)"
```

---

### Task 2: Implement DebitsEqualCredits Predicate

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates.go`

**Prerequisites:**
- Task 1 completed (test written)
- Go 1.22+

**Step 1: Add the implementation**

Add to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates.go` after the existing predicates (after line 179):

```go
// DebitsEqualCredits returns true if debits and credits are exactly equal.
// This validates the fundamental double-entry accounting invariant:
// for every transaction, total debits MUST equal total credits.
//
// Note: Uses decimal.Equal() for exact comparison without floating point issues.
// Even a tiny difference indicates a bug in amount calculation.
//
// Example:
//
//	assert.That(assert.DebitsEqualCredits(debitTotal, creditTotal),
//	    "double-entry violation: debits must equal credits",
//	    "debits", debitTotal, "credits", creditTotal)
func DebitsEqualCredits(debits, credits decimal.Decimal) bool {
	return debits.Equal(credits)
}
```

**Step 2: Run test to verify it passes**

Run: `go test -v -run "TestDebitsEqualCredits" /Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/`

**Expected output:**
```
=== RUN   TestDebitsEqualCredits
=== RUN   TestDebitsEqualCredits/equal_positive_amounts
=== RUN   TestDebitsEqualCredits/equal_with_decimals
=== RUN   TestDebitsEqualCredits/equal_zero
=== RUN   TestDebitsEqualCredits/debits_greater
=== RUN   TestDebitsEqualCredits/credits_greater
=== RUN   TestDebitsEqualCredits/tiny_difference
=== RUN   TestDebitsEqualCredits/large_equal
--- PASS: TestDebitsEqualCredits (0.00s)
PASS
```

**Step 3: Commit**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates.go
git commit -m "$(cat <<'EOF'
feat(assert): implement DebitsEqualCredits predicate

TDD green phase - validates double-entry accounting invariant.
Uses decimal.Equal() for exact comparison.
EOF
)"
```

---

### Task 3: Add NonZeroTotals Predicate - Test First

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates_test.go`

**Prerequisites:**
- Task 2 completed

**Step 1: Write the failing test**

Add to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates_test.go`:

```go
// TestNonZeroTotals tests the NonZeroTotals predicate for transaction validation.
func TestNonZeroTotals(t *testing.T) {
	tests := []struct {
		name     string
		debits   decimal.Decimal
		credits  decimal.Decimal
		expected bool
	}{
		{"both positive", decimal.NewFromInt(100), decimal.NewFromInt(100), true},
		{"both zero", decimal.Zero, decimal.Zero, false},
		{"debits zero", decimal.Zero, decimal.NewFromInt(100), false},
		{"credits zero", decimal.NewFromInt(100), decimal.Zero, false},
		{"small positive", decimal.NewFromFloat(0.01), decimal.NewFromFloat(0.01), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, NonZeroTotals(tt.debits, tt.credits))
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run "TestNonZeroTotals" /Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/`

**Expected output:**
```
# github.com/LerianStudio/midaz/v3/pkg/assert [github.com/LerianStudio/midaz/v3/pkg/assert.test]
./predicates_test.go:XX:XX: undefined: NonZeroTotals
FAIL
```

**Step 3: Commit**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates_test.go
git commit -m "$(cat <<'EOF'
test(assert): add failing test for NonZeroTotals predicate

TDD red phase - validates transaction totals are non-zero.
EOF
)"
```

---

### Task 4: Implement NonZeroTotals Predicate

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates.go`

**Prerequisites:**
- Task 3 completed

**Step 1: Add the implementation**

Add to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates.go`:

```go
// NonZeroTotals returns true if both debits and credits are non-zero.
// A transaction with zero totals is meaningless and indicates a bug.
//
// Example:
//
//	assert.That(assert.NonZeroTotals(debitTotal, creditTotal),
//	    "transaction totals must be non-zero",
//	    "debits", debitTotal, "credits", creditTotal)
func NonZeroTotals(debits, credits decimal.Decimal) bool {
	return !debits.IsZero() && !credits.IsZero()
}
```

**Step 2: Run test to verify it passes**

Run: `go test -v -run "TestNonZeroTotals" /Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/`

**Expected output:**
```
=== RUN   TestNonZeroTotals
--- PASS: TestNonZeroTotals (0.00s)
PASS
```

**Step 3: Commit**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates.go
git commit -m "$(cat <<'EOF'
feat(assert): implement NonZeroTotals predicate

TDD green phase - validates transaction totals are non-zero.
EOF
)"
```

---

### Task 5: Add Double-Entry Assertion to computeOperationTotals - Test First

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/http/in/transaction_assertions_test.go`

**Prerequisites:**
- Tasks 1-4 completed
- `pkg/assert` predicates available

**Step 1: Write the failing test**

Create file `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/http/in/transaction_assertions_test.go`:

```go
package in

import (
	"fmt"
	"strings"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestValidateDoubleEntry_DebitsNotEqualCredits_Panics(t *testing.T) {
	// Create operations where debits != credits (invalid double-entry)
	debitAmount := decimal.NewFromInt(100)
	creditAmount := decimal.NewFromInt(99) // Mismatched!

	operations := []*mmodel.Operation{
		{
			Type:   constant.DEBIT,
			Amount: &mmodel.Amount{Value: &debitAmount},
		},
		{
			Type:   constant.CREDIT,
			Amount: &mmodel.Amount{Value: &creditAmount},
		},
	}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on debits != credits")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "double-entry") || strings.Contains(panicMsg, "debits must equal credits"),
			"panic message should mention double-entry violation, got: %s", panicMsg)
	}()

	validateDoubleEntry(operations)
}

func TestValidateDoubleEntry_DebitsEqualCredits_NoPanic(t *testing.T) {
	amount := decimal.NewFromInt(100)

	operations := []*mmodel.Operation{
		{
			Type:   constant.DEBIT,
			Amount: &mmodel.Amount{Value: &amount},
		},
		{
			Type:   constant.CREDIT,
			Amount: &mmodel.Amount{Value: &amount},
		},
	}

	assert.NotPanics(t, func() {
		validateDoubleEntry(operations)
	})
}

func TestValidateDoubleEntry_MultipleOperations_DebitsEqualCredits(t *testing.T) {
	fifty := decimal.NewFromInt(50)
	hundred := decimal.NewFromInt(100)

	operations := []*mmodel.Operation{
		{Type: constant.DEBIT, Amount: &mmodel.Amount{Value: &fifty}},
		{Type: constant.DEBIT, Amount: &mmodel.Amount{Value: &fifty}},
		{Type: constant.CREDIT, Amount: &mmodel.Amount{Value: &hundred}},
	}

	assert.NotPanics(t, func() {
		validateDoubleEntry(operations)
	})
}

func TestValidateDoubleEntry_ZeroTotals_Panics(t *testing.T) {
	// Empty operations means zero totals
	operations := []*mmodel.Operation{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on zero totals")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "non-zero") || strings.Contains(panicMsg, "totals"),
			"panic message should mention non-zero totals, got: %s", panicMsg)
	}()

	validateDoubleEntry(operations)
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run "TestValidateDoubleEntry" /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/http/in/`

**Expected output:**
```
# github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/http/in
./transaction_assertions_test.go:XX:XX: undefined: validateDoubleEntry
FAIL
```

**Step 3: Commit**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/http/in/transaction_assertions_test.go
git commit -m "$(cat <<'EOF'
test(transaction): add failing tests for double-entry validation

TDD red phase - tests for validateDoubleEntry function that will
enforce the double-entry accounting invariant.
EOF
)"
```

---

### Task 6: Implement validateDoubleEntry Function and Integrate

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/http/in/transaction.go`

**Prerequisites:**
- Task 5 completed
- Import path for assert package confirmed

**Step 1: Add the validateDoubleEntry function**

Add to `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/http/in/transaction.go` after line 694 (after `totalsMatchAmount` function):

```go
// validateDoubleEntry enforces the double-entry accounting invariant.
// Panics if debits != credits or if totals are zero.
// This is a programming bug assertion - user input errors should be handled before this point.
func validateDoubleEntry(operations []*mmodel.Operation) {
	debitTotal := decimal.Zero
	creditTotal := decimal.Zero

	for _, op := range operations {
		if op == nil || op.Amount == nil || op.Amount.Value == nil {
			continue
		}
		switch op.Type {
		case constant.DEBIT:
			debitTotal = debitTotal.Add(*op.Amount.Value)
		case constant.CREDIT:
			creditTotal = creditTotal.Add(*op.Amount.Value)
		}
	}

	assert.That(assert.NonZeroTotals(debitTotal, creditTotal),
		"double-entry violation: transaction totals must be non-zero",
		"debitTotal", debitTotal, "creditTotal", creditTotal)

	assert.That(assert.DebitsEqualCredits(debitTotal, creditTotal),
		"double-entry violation: debits must equal credits",
		"debitTotal", debitTotal, "creditTotal", creditTotal,
		"difference", debitTotal.Sub(creditTotal))
}
```

**Step 2: Ensure assert import exists**

Verify the import exists at the top of the file (should already be there from previous assertions):
```go
"github.com/LerianStudio/midaz/v3/pkg/assert"
```

**Step 3: Run tests to verify they pass**

Run: `go test -v -run "TestValidateDoubleEntry" /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/http/in/`

**Expected output:**
```
=== RUN   TestValidateDoubleEntry_DebitsNotEqualCredits_Panics
--- PASS: TestValidateDoubleEntry_DebitsNotEqualCredits_Panics (0.00s)
=== RUN   TestValidateDoubleEntry_DebitsEqualCredits_NoPanic
--- PASS: TestValidateDoubleEntry_DebitsEqualCredits_NoPanic (0.00s)
=== RUN   TestValidateDoubleEntry_MultipleOperations_DebitsEqualCredits
--- PASS: TestValidateDoubleEntry_MultipleOperations_DebitsEqualCredits (0.00s)
=== RUN   TestValidateDoubleEntry_ZeroTotals_Panics
--- PASS: TestValidateDoubleEntry_ZeroTotals_Panics (0.00s)
PASS
```

**Step 4: Commit**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/http/in/transaction.go
git commit -m "$(cat <<'EOF'
feat(transaction): implement validateDoubleEntry function

TDD green phase - enforces double-entry accounting invariant.
Validates debits == credits and totals are non-zero.
EOF
)"
```

**If Task Fails:**

1. **Test won't compile:**
   - Check: Import paths for `mmodel`, `constant`, `decimal`
   - Fix: Verify package names match

2. **Tests fail unexpectedly:**
   - Run: `go test -v` with specific test name
   - Check: Assert message substring matching

3. **Can't recover:**
   - Rollback: `git checkout -- .`
   - Return to human partner

---

### Task 7: Run Code Review Checkpoint - CRITICAL Priority

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

# HIGH PRIORITY: Transaction State Machine

---

### Task 8: Add ValidTransactionStatus Predicate - Test First

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates_test.go`

**Prerequisites:**
- CRITICAL priority tasks completed

**Step 1: Write the failing test**

Add to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates_test.go`:

```go
// TestValidTransactionStatus tests the ValidTransactionStatus predicate.
func TestValidTransactionStatus(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		expected bool
	}{
		{"CREATED valid", "CREATED", true},
		{"APPROVED valid", "APPROVED", true},
		{"PENDING valid", "PENDING", true},
		{"CANCELED valid", "CANCELED", true},
		{"NOTED valid", "NOTED", true},
		{"empty invalid", "", false},
		{"lowercase invalid", "pending", false},
		{"unknown invalid", "UNKNOWN", false},
		{"partial invalid", "APPROV", false},
		{"with spaces invalid", " PENDING ", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, ValidTransactionStatus(tt.status))
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run "TestValidTransactionStatus" /Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/`

**Expected output:**
```
# github.com/LerianStudio/midaz/v3/pkg/assert [github.com/LerianStudio/midaz/v3/pkg/assert.test]
./predicates_test.go:XX:XX: undefined: ValidTransactionStatus
FAIL
```

**Step 3: Commit**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates_test.go
git commit -m "$(cat <<'EOF'
test(assert): add failing test for ValidTransactionStatus predicate

TDD red phase - validates transaction status is one of known values.
EOF
)"
```

---

### Task 9: Implement ValidTransactionStatus Predicate

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates.go`

**Prerequisites:**
- Task 8 completed

**Step 1: Add the implementation**

Add to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates.go`:

```go
// validTransactionStatuses contains valid transaction status values.
// Package-level for zero-allocation lookups.
var validTransactionStatuses = map[string]bool{
	"CREATED":  true,
	"APPROVED": true,
	"PENDING":  true,
	"CANCELED": true,
	"NOTED":    true,
}

// ValidTransactionStatus returns true if status is a valid transaction status.
// Valid statuses are: CREATED, APPROVED, PENDING, CANCELED, NOTED.
//
// Note: Statuses are case-sensitive and must match exactly.
//
// Example:
//
//	assert.That(assert.ValidTransactionStatus(tran.Status.Code),
//	    "invalid transaction status",
//	    "status", tran.Status.Code)
func ValidTransactionStatus(status string) bool {
	return validTransactionStatuses[status]
}
```

**Step 2: Run test to verify it passes**

Run: `go test -v -run "TestValidTransactionStatus" /Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/`

**Expected output:**
```
=== RUN   TestValidTransactionStatus
--- PASS: TestValidTransactionStatus (0.00s)
PASS
```

**Step 3: Commit**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates.go
git commit -m "$(cat <<'EOF'
feat(assert): implement ValidTransactionStatus predicate

TDD green phase - validates transaction status against known values.
Uses map lookup for O(1) validation.
EOF
)"
```

---

### Task 10: Add TransactionCanTransitionTo Predicate - Test First

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates_test.go`

**Prerequisites:**
- Task 9 completed

**Step 1: Write the failing test**

Add to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates_test.go`:

```go
// TestTransactionCanTransitionTo tests the TransactionCanTransitionTo predicate.
func TestTransactionCanTransitionTo(t *testing.T) {
	tests := []struct {
		name     string
		current  string
		target   string
		expected bool
	}{
		// Valid transitions from PENDING
		{"PENDING to APPROVED", "PENDING", "APPROVED", true},
		{"PENDING to CANCELED", "PENDING", "CANCELED", true},
		// Invalid transitions from PENDING
		{"PENDING to CREATED", "PENDING", "CREATED", false},
		{"PENDING to PENDING", "PENDING", "PENDING", false},
		// Invalid transitions from APPROVED (terminal state for forward)
		{"APPROVED to CANCELED", "APPROVED", "CANCELED", false},
		{"APPROVED to PENDING", "APPROVED", "PENDING", false},
		{"APPROVED to CREATED", "APPROVED", "CREATED", false},
		// Invalid transitions from CANCELED (terminal state)
		{"CANCELED to APPROVED", "CANCELED", "APPROVED", false},
		{"CANCELED to PENDING", "CANCELED", "PENDING", false},
		// Invalid transitions from CREATED
		{"CREATED to APPROVED", "CREATED", "APPROVED", false},
		{"CREATED to CANCELED", "CREATED", "CANCELED", false},
		// Invalid statuses
		{"invalid current", "INVALID", "APPROVED", false},
		{"invalid target", "PENDING", "INVALID", false},
		{"both invalid", "INVALID", "UNKNOWN", false},
		{"empty current", "", "APPROVED", false},
		{"empty target", "PENDING", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, TransactionCanTransitionTo(tt.current, tt.target))
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run "TestTransactionCanTransitionTo" /Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/`

**Expected output:**
```
# github.com/LerianStudio/midaz/v3/pkg/assert [github.com/LerianStudio/midaz/v3/pkg/assert.test]
./predicates_test.go:XX:XX: undefined: TransactionCanTransitionTo
FAIL
```

**Step 3: Commit**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates_test.go
git commit -m "$(cat <<'EOF'
test(assert): add failing test for TransactionCanTransitionTo predicate

TDD red phase - validates transaction state machine transitions.
Only PENDING can transition to APPROVED or CANCELED.
EOF
)"
```

---

### Task 11: Implement TransactionCanTransitionTo Predicate

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates.go`

**Prerequisites:**
- Task 10 completed

**Step 1: Add the implementation**

Add to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates.go`:

```go
// validTransitions defines the allowed state machine transitions.
// Key: current state, Value: set of valid target states.
// Only PENDING transactions can be committed (APPROVED) or canceled (CANCELED).
var validTransitions = map[string]map[string]bool{
	"PENDING": {
		"APPROVED": true,
		"CANCELED": true,
	},
	// CREATED, APPROVED, CANCELED, NOTED are terminal states - no forward transitions
}

// TransactionCanTransitionTo returns true if transitioning from current to target is valid.
// The transaction state machine only allows: PENDING -> APPROVED or PENDING -> CANCELED.
//
// Note: This is for forward transitions only. Revert is a separate operation.
//
// Example:
//
//	assert.That(assert.TransactionCanTransitionTo(tran.Status.Code, targetStatus),
//	    "invalid transaction state transition",
//	    "current", tran.Status.Code, "target", targetStatus)
func TransactionCanTransitionTo(current, target string) bool {
	allowed, exists := validTransitions[current]
	if !exists {
		return false
	}
	return allowed[target]
}
```

**Step 2: Run test to verify it passes**

Run: `go test -v -run "TestTransactionCanTransitionTo" /Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/`

**Expected output:**
```
=== RUN   TestTransactionCanTransitionTo
--- PASS: TestTransactionCanTransitionTo (0.00s)
PASS
```

**Step 3: Commit**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates.go
git commit -m "$(cat <<'EOF'
feat(assert): implement TransactionCanTransitionTo predicate

TDD green phase - enforces transaction state machine.
Only PENDING -> APPROVED or PENDING -> CANCELED allowed.
EOF
)"
```

---

### Task 12: Add State Machine Assertion to commitOrCancelTransaction - Test First

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/http/in/transaction_assertions_test.go`

**Prerequisites:**
- Tasks 10-11 completed

**Step 1: Write the failing test**

Add to `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/http/in/transaction_assertions_test.go`:

```go
func TestValidateTransactionStateTransition_InvalidTransition_Panics(t *testing.T) {
	tests := []struct {
		name        string
		current     string
		target      string
		shouldPanic bool
	}{
		{"PENDING to APPROVED valid", "PENDING", "APPROVED", false},
		{"PENDING to CANCELED valid", "PENDING", "CANCELED", false},
		{"APPROVED to CANCELED invalid", "APPROVED", "CANCELED", true},
		{"CANCELED to APPROVED invalid", "CANCELED", "APPROVED", true},
		{"CREATED to APPROVED invalid", "CREATED", "APPROVED", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.shouldPanic {
				defer func() {
					r := recover()
					assert.NotNil(t, r, "expected panic on invalid transition")
					panicMsg := fmt.Sprintf("%v", r)
					assert.True(t, strings.Contains(panicMsg, "state transition") || strings.Contains(panicMsg, "transition"),
						"panic message should mention transition, got: %s", panicMsg)
				}()
			}

			validateTransactionStateTransition(tt.current, tt.target)

			if tt.shouldPanic {
				t.Fatal("expected panic but none occurred")
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run "TestValidateTransactionStateTransition" /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/http/in/`

**Expected output:**
```
# github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/http/in
./transaction_assertions_test.go:XX:XX: undefined: validateTransactionStateTransition
FAIL
```

**Step 3: Commit**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/http/in/transaction_assertions_test.go
git commit -m "$(cat <<'EOF'
test(transaction): add failing tests for state transition validation

TDD red phase - tests for validateTransactionStateTransition function.
EOF
)"
```

---

### Task 13: Implement validateTransactionStateTransition Function

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/http/in/transaction.go`

**Prerequisites:**
- Task 12 completed

**Step 1: Add the function**

Add to `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/http/in/transaction.go` after `validateDoubleEntry`:

```go
// validateTransactionStateTransition enforces the transaction state machine.
// Panics if the transition from current to target is not allowed.
// Valid transitions: PENDING -> APPROVED, PENDING -> CANCELED
func validateTransactionStateTransition(current, target string) {
	assert.That(assert.ValidTransactionStatus(current),
		"current transaction status must be valid",
		"current", current)

	assert.That(assert.ValidTransactionStatus(target),
		"target transaction status must be valid",
		"target", target)

	assert.That(assert.TransactionCanTransitionTo(current, target),
		"invalid transaction state transition",
		"current", current, "target", target)
}
```

**Step 2: Run tests to verify they pass**

Run: `go test -v -run "TestValidateTransactionStateTransition" /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/http/in/`

**Expected output:**
```
=== RUN   TestValidateTransactionStateTransition_InvalidTransition_Panics
=== RUN   TestValidateTransactionStateTransition_InvalidTransition_Panics/PENDING_to_APPROVED_valid
=== RUN   TestValidateTransactionStateTransition_InvalidTransition_Panics/PENDING_to_CANCELED_valid
=== RUN   TestValidateTransactionStateTransition_InvalidTransition_Panics/APPROVED_to_CANCELED_invalid
=== RUN   TestValidateTransactionStateTransition_InvalidTransition_Panics/CANCELED_to_APPROVED_invalid
=== RUN   TestValidateTransactionStateTransition_InvalidTransition_Panics/CREATED_to_APPROVED_invalid
--- PASS: TestValidateTransactionStateTransition_InvalidTransition_Panics (0.00s)
PASS
```

**Step 3: Commit**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/http/in/transaction.go
git commit -m "$(cat <<'EOF'
feat(transaction): implement validateTransactionStateTransition function

TDD green phase - enforces transaction state machine invariant.
Only PENDING can transition to APPROVED or CANCELED.
EOF
)"
```

---

### Task 14: Integrate State Transition Assertion into commitOrCancelTransaction

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/http/in/transaction.go`

**Prerequisites:**
- Task 13 completed

**Step 1: Add assertion call in commitOrCancelTransaction**

Locate `commitOrCancelTransaction` function (around line 1524) and add the assertion after the existing UUID assertions (after line 1537):

Find this section:
```go
	assert.That(assert.ValidUUID(tran.OrganizationID),
		"transaction organization ID must be valid UUID",
		"organizationID", tran.OrganizationID)
	assert.That(assert.ValidUUID(tran.LedgerID),
		"transaction ledger ID must be valid UUID",
		"ledgerID", tran.LedgerID)
```

Add immediately after:
```go
	// Validate state machine: only PENDING transactions can be committed/canceled
	validateTransactionStateTransition(tran.Status.Code, transactionStatus)
```

**Step 2: Run existing tests to ensure no regression**

Run: `go test -v /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/http/in/... 2>&1 | head -50`

**Expected output:** All existing tests should pass (or be skipped if they require mocks).

**Step 3: Commit**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/http/in/transaction.go
git commit -m "$(cat <<'EOF'
feat(transaction): integrate state transition assertion in commitOrCancelTransaction

Validates state machine invariant before processing commit/cancel.
Catches invalid transitions early with descriptive panic.
EOF
)"
```

---

### Task 15: Add TransactionCanBeReverted Predicate - Test First

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates_test.go`

**Prerequisites:**
- Task 14 completed

**Step 1: Write the failing test**

Add to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates_test.go`:

```go
// TestTransactionCanBeReverted tests the TransactionCanBeReverted predicate.
func TestTransactionCanBeReverted(t *testing.T) {
	tests := []struct {
		name              string
		status            string
		hasParent         bool
		expected          bool
	}{
		{"APPROVED without parent can revert", "APPROVED", false, true},
		{"APPROVED with parent cannot revert", "APPROVED", true, false},
		{"PENDING cannot revert", "PENDING", false, false},
		{"CANCELED cannot revert", "CANCELED", false, false},
		{"CREATED cannot revert", "CREATED", false, false},
		{"NOTED cannot revert", "NOTED", false, false},
		{"invalid status cannot revert", "INVALID", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, TransactionCanBeReverted(tt.status, tt.hasParent))
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run "TestTransactionCanBeReverted" /Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/`

**Expected output:**
```
# github.com/LerianStudio/midaz/v3/pkg/assert [github.com/LerianStudio/midaz/v3/pkg/assert.test]
./predicates_test.go:XX:XX: undefined: TransactionCanBeReverted
FAIL
```

**Step 3: Commit**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates_test.go
git commit -m "$(cat <<'EOF'
test(assert): add failing test for TransactionCanBeReverted predicate

TDD red phase - validates revert eligibility.
Only APPROVED transactions without parent can be reverted.
EOF
)"
```

---

### Task 16: Implement TransactionCanBeReverted Predicate

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates.go`

**Prerequisites:**
- Task 15 completed

**Step 1: Add the implementation**

Add to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates.go`:

```go
// TransactionCanBeReverted returns true if transaction is eligible for revert.
// A transaction can be reverted only if:
// 1. Status is APPROVED (other statuses cannot be reversed)
// 2. Has no parent transaction (already a revert - no double-revert)
//
// Example:
//
//	hasParent := tran.ParentTransactionID != nil
//	assert.That(assert.TransactionCanBeReverted(tran.Status.Code, hasParent),
//	    "transaction cannot be reverted",
//	    "status", tran.Status.Code, "hasParent", hasParent)
func TransactionCanBeReverted(status string, hasParent bool) bool {
	return status == "APPROVED" && !hasParent
}
```

**Step 2: Run test to verify it passes**

Run: `go test -v -run "TestTransactionCanBeReverted" /Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/`

**Expected output:**
```
=== RUN   TestTransactionCanBeReverted
--- PASS: TestTransactionCanBeReverted (0.00s)
PASS
```

**Step 3: Commit**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates.go
git commit -m "$(cat <<'EOF'
feat(assert): implement TransactionCanBeReverted predicate

TDD green phase - validates revert eligibility.
Only APPROVED without parent can be reverted.
EOF
)"
```

---

### Task 17: Add BalanceSufficientForRelease Predicate - Test First

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates_test.go`

**Prerequisites:**
- Task 16 completed

**Step 1: Write the failing test**

Add to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates_test.go`:

```go
// TestBalanceSufficientForRelease tests the BalanceSufficientForRelease predicate.
func TestBalanceSufficientForRelease(t *testing.T) {
	tests := []struct {
		name          string
		onHold        decimal.Decimal
		releaseAmount decimal.Decimal
		expected      bool
	}{
		{"sufficient onHold", decimal.NewFromInt(100), decimal.NewFromInt(50), true},
		{"exactly sufficient", decimal.NewFromInt(100), decimal.NewFromInt(100), true},
		{"insufficient onHold", decimal.NewFromInt(50), decimal.NewFromInt(100), false},
		{"zero onHold zero release", decimal.Zero, decimal.Zero, true},
		{"zero onHold positive release", decimal.Zero, decimal.NewFromInt(1), false},
		{"decimal precision sufficient", decimal.NewFromFloat(100.50), decimal.NewFromFloat(100.49), true},
		{"decimal precision insufficient", decimal.NewFromFloat(100.49), decimal.NewFromFloat(100.50), false},
		{"negative onHold always fails", decimal.NewFromInt(-10), decimal.NewFromInt(5), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, BalanceSufficientForRelease(tt.onHold, tt.releaseAmount))
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run "TestBalanceSufficientForRelease" /Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/`

**Expected output:**
```
# github.com/LerianStudio/midaz/v3/pkg/assert [github.com/LerianStudio/midaz/v3/pkg/assert.test]
./predicates_test.go:XX:XX: undefined: BalanceSufficientForRelease
FAIL
```

**Step 3: Commit**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates_test.go
git commit -m "$(cat <<'EOF'
test(assert): add failing test for BalanceSufficientForRelease predicate

TDD red phase - validates onHold >= releaseAmount.
Prevents negative balance after release operation.
EOF
)"
```

---

### Task 18: Implement BalanceSufficientForRelease Predicate

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates.go`

**Prerequisites:**
- Task 17 completed

**Step 1: Add the implementation**

Add to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates.go`:

```go
// BalanceSufficientForRelease returns true if onHold >= releaseAmount.
// This ensures a release operation won't result in negative onHold balance.
//
// Note: Also returns false if onHold is negative (invalid state).
//
// Example:
//
//	assert.That(assert.BalanceSufficientForRelease(balance.OnHold, releaseAmount),
//	    "insufficient onHold balance for release",
//	    "onHold", balance.OnHold, "releaseAmount", releaseAmount)
func BalanceSufficientForRelease(onHold, releaseAmount decimal.Decimal) bool {
	if onHold.IsNegative() {
		return false
	}
	return onHold.GreaterThanOrEqual(releaseAmount)
}
```

**Step 2: Run test to verify it passes**

Run: `go test -v -run "TestBalanceSufficientForRelease" /Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/`

**Expected output:**
```
=== RUN   TestBalanceSufficientForRelease
--- PASS: TestBalanceSufficientForRelease (0.00s)
PASS
```

**Step 3: Commit**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates.go
git commit -m "$(cat <<'EOF'
feat(assert): implement BalanceSufficientForRelease predicate

TDD green phase - validates onHold balance sufficiency.
Ensures release won't result in negative balance.
EOF
)"
```

---

### Task 19: Add Assertion to applyCanceledOperation

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/transaction/validations.go`

**Prerequisites:**
- Task 18 completed

**Step 1: Add the import**

Add to the imports section in `/Users/fredamaral/repos/lerianstudio/midaz/pkg/transaction/validations.go`:
```go
"github.com/LerianStudio/midaz/v3/pkg/assert"
```

The import section should look like:
```go
import (
	"context"
	"strconv"
	"strings"

	"github.com/LerianStudio/lib-commons/v2/commons"
	constant "github.com/LerianStudio/lib-commons/v2/commons/constants"
	"github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	localConstant "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/shopspring/decimal"
)
```

**Step 2: Add assertion to applyCanceledOperation**

Modify the `applyCanceledOperation` function (around line 163-170). Find:

```go
// applyCanceledOperation applies canceled transaction operations
func applyCanceledOperation(amount Amount, available, onHold decimal.Decimal) (decimal.Decimal, decimal.Decimal, bool) {
	if amount.Operation == constant.RELEASE {
		return available.Add(amount.Value), onHold.Sub(amount.Value), true
	}

	return available, onHold, false
}
```

Replace with:

```go
// applyCanceledOperation applies canceled transaction operations
func applyCanceledOperation(amount Amount, available, onHold decimal.Decimal) (decimal.Decimal, decimal.Decimal, bool) {
	if amount.Operation == constant.RELEASE {
		// Precondition: onHold must be sufficient for release
		assert.That(assert.BalanceSufficientForRelease(onHold, amount.Value),
			"insufficient onHold balance for release operation",
			"onHold", onHold, "releaseAmount", amount.Value,
			"deficit", amount.Value.Sub(onHold))

		return available.Add(amount.Value), onHold.Sub(amount.Value), true
	}

	return available, onHold, false
}
```

**Step 3: Run tests to verify no regression**

Run: `go test -v /Users/fredamaral/repos/lerianstudio/midaz/pkg/transaction/...`

**Expected output:** Tests pass (or skip if require external dependencies)

**Step 4: Commit**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/pkg/transaction/validations.go
git commit -m "$(cat <<'EOF'
feat(transaction): add balance sufficiency assertion to applyCanceledOperation

Validates onHold >= releaseAmount before release operation.
Catches bugs that would result in negative onHold balance.
EOF
)"
```

---

### Task 20: Add Parent Account Asset Code Match Assertion

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/command/create-account.go`

**Prerequisites:**
- Task 19 completed

**Step 1: Add assertion to validateParentAccount**

Locate the `validateParentAccount` function (around line 68-106). Find this section:

```go
	if acc.AssetCode != cai.AssetCode {
		businessErr := pkg.ValidateBusinessError(constant.ErrMismatchedAssetCode, reflect.TypeOf(mmodel.Account{}).Name())
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to validate parent account", businessErr)

		return businessErr
	}
```

Add an assertion BEFORE the business error check (after the `assert.NotNil` on line 87-88):

```go
	assert.NotNil(acc, "parent account must exist after successful Find",
		"parent_account_id", parsedParentID.String())

	// Assert: If parent exists, the asset code relationship is a business rule
	// but the fact that we got here with a valid parent is an invariant
	assert.NotEmpty(acc.AssetCode, "parent account asset code must not be empty",
		"parent_account_id", parsedParentID.String())

	if acc.AssetCode != cai.AssetCode {
```

**Step 2: Verify the assert import exists**

Ensure the import exists (should already be present):
```go
"github.com/LerianStudio/midaz/v3/pkg/assert"
```

**Step 3: Run tests**

Run: `go build ./components/onboarding/...`

**Expected output:** Build succeeds without errors

**Step 4: Commit**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/command/create-account.go
git commit -m "$(cat <<'EOF'
feat(onboarding): add asset code assertion in validateParentAccount

Asserts parent account has non-empty asset code before comparison.
Catches data corruption early.
EOF
)"
```

---

### Task 21: Run Code Review Checkpoint - HIGH Priority

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

**Cosmetic/Nitpick Issues:**
- Add `FIXME(nitpick):` comments in code at the relevant location

3. **Proceed only when:**
   - Zero Critical/High/Medium issues remain
   - All Low issues have TODO(review): comments added
   - All Cosmetic issues have FIXME(nitpick): comments added

---

# MEDIUM PRIORITY: Temporal Constraints and Balance Rules

---

### Task 22: Add DateNotInFuture Predicate - Test First

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates_test.go`

**Prerequisites:**
- HIGH priority tasks completed

**Step 1: Write the failing test**

Add to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates_test.go`:

```go
// TestDateNotInFuture tests the DateNotInFuture predicate.
func TestDateNotInFuture(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		date     time.Time
		expected bool
	}{
		{"past date valid", now.Add(-24 * time.Hour), true},
		{"now valid", now, true},
		{"one second ago valid", now.Add(-time.Second), true},
		{"one second future invalid", now.Add(time.Second), false},
		{"one hour future invalid", now.Add(time.Hour), false},
		{"far future invalid", now.Add(365 * 24 * time.Hour), false},
		{"zero time valid", time.Time{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DateNotInFuture(tt.date)
			// Allow slight timing variance for "now" test
			if tt.name == "now valid" {
				require.True(t, result || time.Since(now) < time.Millisecond)
			} else {
				require.Equal(t, tt.expected, result)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run "TestDateNotInFuture" /Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/`

**Expected output:**
```
# github.com/LerianStudio/midaz/v3/pkg/assert [github.com/LerianStudio/midaz/v3/pkg/assert.test]
./predicates_test.go:XX:XX: undefined: DateNotInFuture
FAIL
```

**Step 3: Commit**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates_test.go
git commit -m "$(cat <<'EOF'
test(assert): add failing test for DateNotInFuture predicate

TDD red phase - validates date is not in the future.
EOF
)"
```

---

### Task 23: Implement DateNotInFuture Predicate

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates.go`

**Prerequisites:**
- Task 22 completed

**Step 1: Add import**

Add to imports section:
```go
"time"
```

**Step 2: Add the implementation**

Add to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates.go`:

```go
// DateNotInFuture returns true if the date is not in the future.
// Zero time is considered valid (not in future).
//
// Example:
//
//	assert.That(assert.DateNotInFuture(transactionDate),
//	    "transaction date cannot be in the future",
//	    "date", transactionDate)
func DateNotInFuture(t time.Time) bool {
	if t.IsZero() {
		return true
	}
	return !t.After(time.Now())
}
```

**Step 3: Run test to verify it passes**

Run: `go test -v -run "TestDateNotInFuture" /Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/`

**Expected output:**
```
=== RUN   TestDateNotInFuture
--- PASS: TestDateNotInFuture (0.00s)
PASS
```

**Step 4: Commit**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates.go
git commit -m "$(cat <<'EOF'
feat(assert): implement DateNotInFuture predicate

TDD green phase - validates date is not in the future.
Zero time treated as valid.
EOF
)"
```

---

### Task 24: Add DateAfter Predicate - Test First

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates_test.go`

**Prerequisites:**
- Task 23 completed

**Step 1: Write the failing test**

Add to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates_test.go`:

```go
// TestDateAfter tests the DateAfter predicate.
func TestDateAfter(t *testing.T) {
	base := time.Date(2024, 1, 15, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		date      time.Time
		reference time.Time
		expected  bool
	}{
		{"date after reference", base.Add(24 * time.Hour), base, true},
		{"date equal to reference", base, base, false},
		{"date before reference", base.Add(-24 * time.Hour), base, false},
		{"date one second after", base.Add(time.Second), base, true},
		{"date one second before", base.Add(-time.Second), base, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, DateAfter(tt.date, tt.reference))
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run "TestDateAfter" /Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/`

**Expected output:**
```
./predicates_test.go:XX:XX: undefined: DateAfter
FAIL
```

**Step 3: Commit**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates_test.go
git commit -m "$(cat <<'EOF'
test(assert): add failing test for DateAfter predicate

TDD red phase - validates date is after reference.
EOF
)"
```

---

### Task 25: Implement DateAfter Predicate

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates.go`

**Prerequisites:**
- Task 24 completed

**Step 1: Add the implementation**

Add to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates.go`:

```go
// DateAfter returns true if date is strictly after reference.
//
// Example:
//
//	assert.That(assert.DateAfter(closingDate, createdAt),
//	    "closing date must be after creation date",
//	    "closingDate", closingDate, "createdAt", createdAt)
func DateAfter(date, reference time.Time) bool {
	return date.After(reference)
}
```

**Step 2: Run test to verify it passes**

Run: `go test -v -run "TestDateAfter" /Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/`

**Expected output:**
```
=== RUN   TestDateAfter
--- PASS: TestDateAfter (0.00s)
PASS
```

**Step 3: Commit**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates.go
git commit -m "$(cat <<'EOF'
feat(assert): implement DateAfter predicate

TDD green phase - validates date ordering.
EOF
)"
```

---

### Task 26: Add BalanceIsZero Predicate - Test First

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates_test.go`

**Prerequisites:**
- Task 25 completed

**Step 1: Write the failing test**

Add to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates_test.go`:

```go
// TestBalanceIsZero tests the BalanceIsZero predicate.
func TestBalanceIsZero(t *testing.T) {
	tests := []struct {
		name      string
		available decimal.Decimal
		onHold    decimal.Decimal
		expected  bool
	}{
		{"both zero", decimal.Zero, decimal.Zero, true},
		{"available non-zero", decimal.NewFromInt(1), decimal.Zero, false},
		{"onHold non-zero", decimal.Zero, decimal.NewFromInt(1), false},
		{"both non-zero", decimal.NewFromInt(1), decimal.NewFromInt(1), false},
		{"tiny available", decimal.NewFromFloat(0.001), decimal.Zero, false},
		{"negative available still not zero", decimal.NewFromInt(-1), decimal.Zero, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, BalanceIsZero(tt.available, tt.onHold))
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run "TestBalanceIsZero" /Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/`

**Expected output:**
```
./predicates_test.go:XX:XX: undefined: BalanceIsZero
FAIL
```

**Step 3: Commit**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates_test.go
git commit -m "$(cat <<'EOF'
test(assert): add failing test for BalanceIsZero predicate

TDD red phase - validates balance has zero funds.
EOF
)"
```

---

### Task 27: Implement BalanceIsZero Predicate

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates.go`

**Prerequisites:**
- Task 26 completed

**Step 1: Add the implementation**

Add to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates.go`:

```go
// BalanceIsZero returns true if both available and onHold are exactly zero.
// This is required before deleting a balance - cannot delete balance with funds.
//
// Example:
//
//	assert.That(assert.BalanceIsZero(balance.Available, balance.OnHold),
//	    "balance must be zero before deletion",
//	    "available", balance.Available, "onHold", balance.OnHold)
func BalanceIsZero(available, onHold decimal.Decimal) bool {
	return available.IsZero() && onHold.IsZero()
}
```

**Step 2: Run test to verify it passes**

Run: `go test -v -run "TestBalanceIsZero" /Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/`

**Expected output:**
```
=== RUN   TestBalanceIsZero
--- PASS: TestBalanceIsZero (0.00s)
PASS
```

**Step 3: Commit**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates.go
git commit -m "$(cat <<'EOF'
feat(assert): implement BalanceIsZero predicate

TDD green phase - validates balance has zero funds.
Required for balance deletion validation.
EOF
)"
```

---

### Task 28: Add Assertion to DeleteBalance

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/delete-balance.go`

**Prerequisites:**
- Task 27 completed

**Step 1: Add import**

Add to imports in `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/delete-balance.go`:
```go
"github.com/LerianStudio/midaz/v3/pkg/assert"
```

**Step 2: Add assertion after balance retrieval**

Find the section (around line 33-38):

```go
	if balance != nil && (!balance.Available.IsZero() || !balance.OnHold.IsZero()) {
		err = pkg.ValidateBusinessError(constant.ErrBalancesCantBeDeleted, "DeleteBalance")
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Balance cannot be deleted because it still has funds in it.", err)
		logger.Warnf("Error deleting balance: %v", err)

		return err
	}
```

Add assertion BEFORE the business error check (after line 31 where balance is checked):

```go
	if balance != nil && (!balance.Available.IsZero() || !balance.OnHold.IsZero()) {
		// Invariant check: if we got here with non-zero balance, log details for debugging
		assert.That(false,
			"balance deletion attempted with non-zero funds - this indicates a caller bug",
			"balanceID", balanceID,
			"available", balance.Available,
			"onHold", balance.OnHold)
```

Wait - this would always panic. Instead, we should add an assertion that documents the invariant at the point where we're about to delete. Let's add it after the check passes, before the delete:

Find line 41:
```go
	err = uc.BalanceRepo.Delete(ctx, organizationID, ledgerID, balanceID)
```

Add before it:
```go
	// Postcondition after validation: balance must be zero before deletion
	if balance != nil {
		assert.That(assert.BalanceIsZero(balance.Available, balance.OnHold),
			"balance must be zero before deletion - validation passed but invariant violated",
			"balanceID", balanceID,
			"available", balance.Available,
			"onHold", balance.OnHold)
	}

	err = uc.BalanceRepo.Delete(ctx, organizationID, ledgerID, balanceID)
```

**Step 3: Run build**

Run: `go build ./components/transaction/...`

**Expected output:** Build succeeds

**Step 4: Commit**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/delete-balance.go
git commit -m "$(cat <<'EOF'
feat(transaction): add balance zero assertion in DeleteBalance

Postcondition assertion after validation passes.
Catches any logic errors between validation and deletion.
EOF
)"
```

---

### Task 29: Add Assertion to validateAliasClosingDate

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/services/validate-alias-closing-date.go`

**Prerequisites:**
- Task 28 completed

**Step 1: Add import**

Add to imports:
```go
"github.com/LerianStudio/midaz/v3/pkg/assert"
```

**Step 2: Add assertion after alias retrieval**

Find the section (around line 28-33):
```go
	alias, err := uc.GetAliasByID(ctx, organizationID, holderID, aliasId, false)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to get alias", err)
		logger.Errorf("Failed to get alias: %v", err)

		return err
	}
```

Add assertion after the error check:
```go
	alias, err := uc.GetAliasByID(ctx, organizationID, holderID, aliasId, false)
	if err != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to get alias", err)
		logger.Errorf("Failed to get alias: %v", err)

		return err
	}

	// Precondition: alias must have valid creation date
	assert.NotNil(alias, "alias must exist after successful GetAliasByID",
		"aliasId", aliasId.String())
	assert.That(!alias.CreatedAt.IsZero(),
		"alias creation date must be set",
		"aliasId", aliasId.String())

	if closingDate.Before(alias.CreatedAt) {
```

**Step 3: Run build**

Run: `go build ./components/crm/...`

**Expected output:** Build succeeds

**Step 4: Commit**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/services/validate-alias-closing-date.go
git commit -m "$(cat <<'EOF'
feat(crm): add alias creation date assertion in validateAliasClosingDate

Validates alias exists and has valid creation date before comparison.
EOF
)"
```

---

### Task 30: Add Assertion to UpdateAccount for External Account Check

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/command/update-account.go`

**Prerequisites:**
- Task 29 completed

**Step 1: Add import**

Add to imports:
```go
"github.com/LerianStudio/midaz/v3/pkg/assert"
```

**Step 2: Add assertion after account retrieval**

Find the section (around line 30-41):
```go
	accFound, err := uc.AccountRepo.Find(ctx, organizationID, ledgerID, nil, id)
	if err != nil {
		logger.Errorf("Error finding account by alias: %v", err)

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to find account by alias", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Account{}).Name())
	}

	if accFound != nil && accFound.ID == id.String() && accFound.Type == accountTypeExternal {
		return nil, pkg.ValidateBusinessError(constant.ErrForbiddenExternalAccountManipulation, reflect.TypeOf(mmodel.Account{}).Name())
	}
```

Add assertion after the error check and before the external account check:
```go
	accFound, err := uc.AccountRepo.Find(ctx, organizationID, ledgerID, nil, id)
	if err != nil {
		logger.Errorf("Error finding account by alias: %v", err)

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to find account by alias", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Account{}).Name())
	}

	// Precondition: if account found, ID should match the requested ID
	if accFound != nil {
		assert.That(accFound.ID == id.String(),
			"found account ID must match requested ID",
			"requestedID", id.String(),
			"foundID", accFound.ID)
	}

	if accFound != nil && accFound.ID == id.String() && accFound.Type == accountTypeExternal {
```

**Step 3: Run build**

Run: `go build ./components/onboarding/...`

**Expected output:** Build succeeds

**Step 4: Commit**

```bash
git add /Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/command/update-account.go
git commit -m "$(cat <<'EOF'
feat(onboarding): add account ID match assertion in UpdateAccount

Validates found account ID matches requested ID.
Catches potential data inconsistency bugs.
EOF
)"
```

---

### Task 31: Run All Predicate Tests

**Files:**
- None (verification only)

**Prerequisites:**
- All previous tasks completed

**Step 1: Run full predicate test suite**

Run: `go test -v /Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/...`

**Expected output:**
```
=== RUN   TestThat_Pass
--- PASS: TestThat_Pass (0.00s)
...
=== RUN   TestDebitsEqualCredits
--- PASS: TestDebitsEqualCredits (0.00s)
=== RUN   TestNonZeroTotals
--- PASS: TestNonZeroTotals (0.00s)
=== RUN   TestValidTransactionStatus
--- PASS: TestValidTransactionStatus (0.00s)
=== RUN   TestTransactionCanTransitionTo
--- PASS: TestTransactionCanTransitionTo (0.00s)
=== RUN   TestTransactionCanBeReverted
--- PASS: TestTransactionCanBeReverted (0.00s)
=== RUN   TestBalanceSufficientForRelease
--- PASS: TestBalanceSufficientForRelease (0.00s)
=== RUN   TestDateNotInFuture
--- PASS: TestDateNotInFuture (0.00s)
=== RUN   TestDateAfter
--- PASS: TestDateAfter (0.00s)
=== RUN   TestBalanceIsZero
--- PASS: TestBalanceIsZero (0.00s)
PASS
ok      github.com/LerianStudio/midaz/v3/pkg/assert
```

**Step 2: Run build on all components**

Run: `go build ./...`

**Expected output:** Build succeeds with no errors

**Step 3: Commit (if any fixes needed)**

Only commit if fixes were required.

---

### Task 32: Run Code Review Checkpoint - MEDIUM Priority (Final)

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

**Cosmetic/Nitpick Issues:**
- Add `FIXME(nitpick):` comments in code at the relevant location

3. **Proceed only when:**
   - Zero Critical/High/Medium issues remain
   - All Low issues have TODO(review): comments added
   - All Cosmetic issues have FIXME(nitpick): comments added

---

## Summary

### New Predicates Added to `pkg/assert/predicates.go`:
1. `DebitsEqualCredits(debits, credits decimal.Decimal) bool`
2. `NonZeroTotals(debits, credits decimal.Decimal) bool`
3. `ValidTransactionStatus(status string) bool`
4. `TransactionCanTransitionTo(current, target string) bool`
5. `TransactionCanBeReverted(status string, hasParent bool) bool`
6. `BalanceSufficientForRelease(onHold, releaseAmount decimal.Decimal) bool`
7. `DateNotInFuture(t time.Time) bool`
8. `DateAfter(date, reference time.Time) bool`
9. `BalanceIsZero(available, onHold decimal.Decimal) bool`

### Assertions Added:
| Location | Invariant |
|----------|-----------|
| `transaction.go:validateDoubleEntry()` | Debits = Credits, non-zero totals |
| `transaction.go:validateTransactionStateTransition()` | State machine: PENDING -> APPROVED/CANCELED only |
| `transaction.go:commitOrCancelTransaction()` | Uses state transition validation |
| `validations.go:applyCanceledOperation()` | OnHold >= release amount |
| `create-account.go:validateParentAccount()` | Parent has non-empty asset code |
| `delete-balance.go:DeleteBalance()` | Balance is zero before deletion |
| `validate-alias-closing-date.go` | Alias exists, has valid creation date |
| `update-account.go:UpdateAccount()` | Found account ID matches requested ID |

### Files Modified:
- `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates.go` (9 new predicates)
- `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates_test.go` (9 new test suites)
- `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/http/in/transaction.go` (2 new functions, 1 integration)
- `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/http/in/transaction_assertions_test.go` (new file)
- `/Users/fredamaral/repos/lerianstudio/midaz/pkg/transaction/validations.go` (1 assertion)
- `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/command/create-account.go` (1 assertion)
- `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/delete-balance.go` (1 assertion)
- `/Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/services/validate-alias-closing-date.go` (2 assertions)
- `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/command/update-account.go` (1 assertion)
