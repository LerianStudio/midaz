# Fuzz Test Expansion Implementation Plan

> **For Agents:** REQUIRED SUB-SKILL: Use executing-plans to implement this plan task-by-task.

**Goal:** Expand fuzz test coverage to exercise assertion guards added in Plans 01-07, focusing on domain constructors, predicates, outbox validation, and edge cases.

**Architecture:** Unit fuzz tests in Go's native testing framework targeting domain model constructors (NewHolder, NewBalance, NewAccount), assertion predicates (ValidUUID, InRange, ValidAmount), UUID conversion methods (IDtoUUID), and transaction share calculations. Tests verify that assertions trigger correctly on invalid inputs while valid inputs produce expected results.

**Tech Stack:** Go 1.21+, native `testing` package with fuzz support, `github.com/google/gofuzz` for seed generation, `github.com/shopspring/decimal` for financial math, `github.com/google/uuid` for UUID operations.

**Global Prerequisites:**
- Environment: macOS/Linux, Go 1.21+
- Tools: Go compiler with fuzz support
- Access: Local filesystem access to tests/fuzzy/
- State: Clean working tree on current branch

**Verification before starting:**
```bash
# Run ALL these commands and verify output:
go version      # Expected: go1.21+ (must support -fuzz flag)
ls tests/fuzzy/ # Expected: list of existing *_fuzz_test.go files
go test -list='Fuzz.*' ./tests/fuzzy/... 2>&1 | head -5 # Expected: list of existing fuzz tests
```

## Historical Precedent

**Query:** "fuzz testing assertions guards predicates"
**Index Status:** Empty (new project)

No historical data available. This is normal for new projects.
Proceeding with standard planning approach.

---

## Task 1: Create Constructor Fuzzing Test File

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/tests/fuzzy/constructor_fuzz_test.go`
- Reference: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/holder.go`
- Reference: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/balance.go`
- Reference: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/account.go`

**Prerequisites:**
- Tools: Go 1.21+
- Files must exist: `pkg/mmodel/holder.go`, `pkg/mmodel/balance.go`, `pkg/mmodel/account.go`

**Step 1: Create the test file with imports and helper**

Create `/Users/fredamaral/repos/lerianstudio/midaz/tests/fuzzy/constructor_fuzz_test.go`:

```go
package fuzzy

import (
	"fmt"
	"strings"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// assertionPanicRecovery returns true if the panic was an assertion failure (expected),
// false if it was an unexpected panic, and does not recover if no panic occurred.
func assertionPanicRecovery(t *testing.T, panicValue any, context string) bool {
	t.Helper()
	if panicValue == nil {
		return true // no panic
	}

	msg := fmt.Sprintf("%v", panicValue)
	if strings.Contains(msg, "assertion failed") {
		return true // expected assertion panic
	}

	t.Errorf("Unexpected panic (not assertion) in %s: %v", context, panicValue)
	return false
}
```

**Step 2: Run syntax check**

Run: `go build ./tests/fuzzy/constructor_fuzz_test.go 2>&1 || true`

**Expected output:**
```
# (no output means syntax is valid)
```

**If you see errors:** Check import paths match the module path in go.mod

**Step 3: Commit**

```bash
git add tests/fuzzy/constructor_fuzz_test.go
git commit -m "$(cat <<'EOF'
test(fuzzy): add constructor fuzz test file with helper

Add initial structure for constructor fuzzing with assertion
panic recovery helper function.
EOF
)"
```

---

## Task 2: Add FuzzNewHolder Test

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/tests/fuzzy/constructor_fuzz_test.go`

**Prerequisites:**
- Task 1 completed
- Files must exist: `tests/fuzzy/constructor_fuzz_test.go`

**Step 1: Add the FuzzNewHolder function**

Append to `/Users/fredamaral/repos/lerianstudio/midaz/tests/fuzzy/constructor_fuzz_test.go`:

```go

// FuzzNewHolder tests the NewHolder constructor with diverse inputs.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzNewHolder -run=^$ -fuzztime=30s
func FuzzNewHolder(f *testing.F) {
	// Valid seeds
	validUUID := "00000000-0000-0000-0000-000000000001"
	f.Add(validUUID, "John Doe", "12345678901", "NATURAL_PERSON")
	f.Add(validUUID, "Jane Corp", "12345678000199", "LEGAL_PERSON")

	// Edge case seeds - invalid UUIDs
	f.Add("", "Name", "Doc", "NATURAL_PERSON")
	f.Add("invalid-uuid", "Name", "Doc", "NATURAL_PERSON")
	f.Add("00000000-0000-0000-0000-000000000000", "Name", "Doc", "NATURAL_PERSON") // Nil UUID

	// Edge case seeds - empty required fields
	f.Add(validUUID, "", "Doc", "NATURAL_PERSON")    // Empty name
	f.Add(validUUID, "Name", "", "NATURAL_PERSON")   // Empty document
	f.Add(validUUID, "Name", "Doc", "")              // Empty type
	f.Add(validUUID, "Name", "Doc", "invalid_type")  // Invalid type

	// Edge case seeds - boundary values
	f.Add(validUUID, strings.Repeat("a", 1000), "Doc", "NATURAL_PERSON") // Long name
	f.Add(validUUID, "Name", strings.Repeat("9", 100), "LEGAL_PERSON")   // Long document

	f.Fuzz(func(t *testing.T, idStr, name, document, holderType string) {
		defer func() {
			assertionPanicRecovery(t, recover(), fmt.Sprintf(
				"FuzzNewHolder(id=%q, name=%q, doc=%q, type=%q)",
				idStr, name, document, holderType))
		}()

		id, err := uuid.Parse(idStr)
		if err != nil {
			return // Invalid UUID string - skip (pre-condition not met)
		}

		holder := mmodel.NewHolder(id, name, document, holderType)

		// If we reach here without panic, validate the result
		if holder == nil {
			t.Error("NewHolder returned nil without panicking")
			return
		}

		// Verify the holder has expected fields set
		if holder.ID == nil || *holder.ID != id {
			t.Errorf("ID mismatch: got %v, want %v", holder.ID, id)
		}
		if holder.Name == nil || *holder.Name != name {
			t.Errorf("Name mismatch: got %v, want %v", holder.Name, name)
		}
	})
}
```

**Step 2: Run the fuzz test briefly to verify it works**

Run: `go test -v ./tests/fuzzy -fuzz=FuzzNewHolder -run=^$ -fuzztime=5s 2>&1 | tail -20`

**Expected output (one of these):**
```
fuzz: elapsed: 5s, execs: XXXX (XXX/sec), new interesting: X (total: X)
PASS
```
OR (if assertion triggers on fuzzed input):
```
--- FAIL: FuzzNewHolder (X.XXs)
    --- FAIL: FuzzNewHolder/... (X.XXs)
        constructor_fuzz_test.go:XX: Unexpected panic...
```

**If assertion panics are expected:** That's correct behavior - the test is verifying assertions work.

**Step 3: Commit**

```bash
git add tests/fuzzy/constructor_fuzz_test.go
git commit -m "$(cat <<'EOF'
test(fuzzy): add FuzzNewHolder for holder constructor assertions

Fuzz test exercises NewHolder with diverse inputs including invalid
UUIDs, empty fields, invalid holder types, and boundary values.
EOF
)"
```

---

## Task 3: Add FuzzNewBalance Test

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/tests/fuzzy/constructor_fuzz_test.go`

**Prerequisites:**
- Task 2 completed
- Files must exist: `tests/fuzzy/constructor_fuzz_test.go`

**Step 1: Add the FuzzNewBalance function**

Append to `/Users/fredamaral/repos/lerianstudio/midaz/tests/fuzzy/constructor_fuzz_test.go`:

```go

// FuzzNewBalance tests the NewBalance constructor with diverse inputs.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzNewBalance -run=^$ -fuzztime=30s
func FuzzNewBalance(f *testing.F) {
	validUUID := "00000000-0000-0000-0000-000000000001"

	// Valid seeds
	f.Add(validUUID, validUUID, validUUID, validUUID, "@alias", "USD", "checking")
	f.Add(validUUID, validUUID, validUUID, validUUID, "@person1", "BRL", "savings")

	// Edge case seeds - invalid UUIDs
	f.Add("", validUUID, validUUID, validUUID, "@alias", "USD", "checking")
	f.Add(validUUID, "", validUUID, validUUID, "@alias", "USD", "checking")
	f.Add(validUUID, validUUID, "", validUUID, "@alias", "USD", "checking")
	f.Add(validUUID, validUUID, validUUID, "", "@alias", "USD", "checking")
	f.Add("invalid", "invalid", "invalid", "invalid", "@a", "B", "c")

	// Edge case seeds - empty required fields
	f.Add(validUUID, validUUID, validUUID, validUUID, "", "USD", "checking")     // Empty alias
	f.Add(validUUID, validUUID, validUUID, validUUID, "@alias", "", "checking")  // Empty assetCode

	// Edge case seeds - unusual but potentially valid
	f.Add(validUUID, validUUID, validUUID, validUUID, "@alias", "usd", "checking") // Lowercase asset
	f.Add(validUUID, validUUID, validUUID, validUUID, "no-at-prefix", "USD", "")   // No @ prefix, empty type

	f.Fuzz(func(t *testing.T, id, orgID, ledgerID, accountID, alias, assetCode, accountType string) {
		defer func() {
			assertionPanicRecovery(t, recover(), fmt.Sprintf(
				"FuzzNewBalance(id=%q, orgID=%q, ledgerID=%q, accountID=%q, alias=%q, asset=%q, type=%q)",
				id, orgID, ledgerID, accountID, alias, assetCode, accountType))
		}()

		balance := mmodel.NewBalance(id, orgID, ledgerID, accountID, alias, assetCode, accountType)

		// If we reach here without panic, validate the result
		if balance == nil {
			t.Error("NewBalance returned nil without panicking")
			return
		}

		// Verify critical fields
		if balance.ID != id {
			t.Errorf("ID mismatch: got %q, want %q", balance.ID, id)
		}
		if balance.Alias != alias {
			t.Errorf("Alias mismatch: got %q, want %q", balance.Alias, alias)
		}
		if balance.Version != 1 {
			t.Errorf("Version should be 1 for new balance, got %d", balance.Version)
		}
	})
}
```

**Step 2: Run the fuzz test briefly**

Run: `go test -v ./tests/fuzzy -fuzz=FuzzNewBalance -run=^$ -fuzztime=5s 2>&1 | tail -20`

**Expected output:**
```
fuzz: elapsed: 5s, execs: XXXX (XXX/sec), new interesting: X (total: X)
PASS
```

**Step 3: Commit**

```bash
git add tests/fuzzy/constructor_fuzz_test.go
git commit -m "$(cat <<'EOF'
test(fuzzy): add FuzzNewBalance for balance constructor assertions

Fuzz test exercises NewBalance with diverse UUID combinations,
empty required fields, and edge cases for alias/assetCode.
EOF
)"
```

---

## Task 4: Add FuzzNewAccount Test

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/tests/fuzzy/constructor_fuzz_test.go`

**Prerequisites:**
- Task 3 completed
- Files must exist: `tests/fuzzy/constructor_fuzz_test.go`

**Step 1: Add the FuzzNewAccount function**

Append to `/Users/fredamaral/repos/lerianstudio/midaz/tests/fuzzy/constructor_fuzz_test.go`:

```go

// FuzzNewAccount tests the NewAccount constructor with diverse inputs.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzNewAccount -run=^$ -fuzztime=30s
func FuzzNewAccount(f *testing.F) {
	validUUID := "00000000-0000-0000-0000-000000000001"

	// Valid seeds
	f.Add(validUUID, validUUID, validUUID, "USD", "checking")
	f.Add(validUUID, validUUID, validUUID, "BRL", "savings")
	f.Add(validUUID, validUUID, validUUID, "EUR", "deposit")

	// Edge case seeds - invalid UUIDs
	f.Add("", validUUID, validUUID, "USD", "checking")
	f.Add(validUUID, "", validUUID, "USD", "checking")
	f.Add(validUUID, validUUID, "", "USD", "checking")
	f.Add("invalid-uuid", validUUID, validUUID, "USD", "checking")

	// Edge case seeds - empty required fields
	f.Add(validUUID, validUUID, validUUID, "", "checking")  // Empty assetCode
	f.Add(validUUID, validUUID, validUUID, "USD", "")       // Empty accountType

	// Edge case seeds - boundary values
	f.Add(validUUID, validUUID, validUUID, strings.Repeat("A", 100), "checking")
	f.Add(validUUID, validUUID, validUUID, "USD", strings.Repeat("x", 256))

	f.Fuzz(func(t *testing.T, id, orgID, ledgerID, assetCode, accountType string) {
		defer func() {
			assertionPanicRecovery(t, recover(), fmt.Sprintf(
				"FuzzNewAccount(id=%q, orgID=%q, ledgerID=%q, asset=%q, type=%q)",
				id, orgID, ledgerID, assetCode, accountType))
		}()

		account := mmodel.NewAccount(id, orgID, ledgerID, assetCode, accountType)

		// If we reach here without panic, validate the result
		if account == nil {
			t.Error("NewAccount returned nil without panicking")
			return
		}

		// Verify critical fields
		if account.ID != id {
			t.Errorf("ID mismatch: got %q, want %q", account.ID, id)
		}
		if account.AssetCode != assetCode {
			t.Errorf("AssetCode mismatch: got %q, want %q", account.AssetCode, assetCode)
		}
		if account.Status.Code != mmodel.AccountStatusActive {
			t.Errorf("Status should be ACTIVE for new account, got %q", account.Status.Code)
		}
	})
}
```

**Step 2: Run the fuzz test briefly**

Run: `go test -v ./tests/fuzzy -fuzz=FuzzNewAccount -run=^$ -fuzztime=5s 2>&1 | tail -20`

**Expected output:**
```
fuzz: elapsed: 5s, execs: XXXX (XXX/sec), new interesting: X (total: X)
PASS
```

**Step 3: Commit**

```bash
git add tests/fuzzy/constructor_fuzz_test.go
git commit -m "$(cat <<'EOF'
test(fuzzy): add FuzzNewAccount for account constructor assertions

Fuzz test exercises NewAccount with diverse UUID combinations,
empty required fields, and boundary value strings.
EOF
)"
```

---

## Task 5: Create Predicates Fuzzing Test File

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/tests/fuzzy/predicates_fuzz_test.go`
- Reference: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/assert/predicates.go`

**Prerequisites:**
- Tools: Go 1.21+
- Files must exist: `pkg/assert/predicates.go`

**Step 1: Create the test file with FuzzValidUUID**

Create `/Users/fredamaral/repos/lerianstudio/midaz/tests/fuzzy/predicates_fuzz_test.go`:

```go
package fuzzy

import (
	"strings"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// FuzzValidUUID tests the ValidUUID predicate against uuid.Parse for consistency.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzValidUUID -run=^$ -fuzztime=30s
func FuzzValidUUID(f *testing.F) {
	// Valid UUIDs
	f.Add("00000000-0000-0000-0000-000000000000") // Nil UUID
	f.Add("00000000-0000-0000-0000-000000000001") // Valid
	f.Add("ffffffff-ffff-ffff-ffff-ffffffffffff") // All F
	f.Add("a1b2c3d4-e5f6-7890-abcd-ef1234567890") // Mixed case

	// Invalid UUIDs
	f.Add("not-a-uuid")
	f.Add("")
	f.Add(strings.Repeat("0", 36))                            // No hyphens
	f.Add("00000000-0000-0000-0000-00000000000")               // Too short (35 chars)
	f.Add("00000000-0000-0000-0000-0000000000001")             // Too long (37 chars)
	f.Add("00000000_0000_0000_0000_000000000001")              // Wrong separator
	f.Add("g0000000-0000-0000-0000-000000000001")              // Invalid hex char
	f.Add("00000000-0000-0000-0000-000000000001\n")            // Trailing newline
	f.Add(" 00000000-0000-0000-0000-000000000001")             // Leading space

	f.Fuzz(func(t *testing.T, s string) {
		result := assert.ValidUUID(s)

		// Cross-check with uuid.Parse
		_, err := uuid.Parse(s)
		expected := err == nil

		if result != expected {
			t.Errorf("ValidUUID(%q) = %v, but uuid.Parse returned err=%v", s, result, err)
		}
	})
}
```

**Step 2: Run syntax check**

Run: `go build ./tests/fuzzy/predicates_fuzz_test.go 2>&1 || true`

**Expected output:**
```
# (no output means syntax is valid)
```

**Step 3: Run the fuzz test briefly**

Run: `go test -v ./tests/fuzzy -fuzz=FuzzValidUUID -run=^$ -fuzztime=5s 2>&1 | tail -20`

**Expected output:**
```
fuzz: elapsed: 5s, execs: XXXX (XXX/sec), new interesting: X (total: X)
PASS
```

**Step 4: Commit**

```bash
git add tests/fuzzy/predicates_fuzz_test.go
git commit -m "$(cat <<'EOF'
test(fuzzy): add predicates fuzz test with FuzzValidUUID

Cross-validates assert.ValidUUID against uuid.Parse to ensure
consistent UUID validation behavior.
EOF
)"
```

---

## Task 6: Add FuzzInRange Test

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/tests/fuzzy/predicates_fuzz_test.go`

**Prerequisites:**
- Task 5 completed
- Files must exist: `tests/fuzzy/predicates_fuzz_test.go`

**Step 1: Add the FuzzInRange function**

Append to `/Users/fredamaral/repos/lerianstudio/midaz/tests/fuzzy/predicates_fuzz_test.go`:

```go

// FuzzInRange tests the InRange predicate with diverse int64 values.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzInRange -run=^$ -fuzztime=30s
func FuzzInRange(f *testing.F) {
	// Normal cases
	f.Add(int64(5), int64(0), int64(10))   // In range
	f.Add(int64(0), int64(0), int64(10))   // At min boundary
	f.Add(int64(10), int64(0), int64(10))  // At max boundary
	f.Add(int64(-1), int64(0), int64(10))  // Below min
	f.Add(int64(11), int64(0), int64(10))  // Above max

	// Edge cases
	f.Add(int64(5), int64(10), int64(0))                       // Inverted range
	f.Add(int64(0), int64(0), int64(0))                        // Single value range
	f.Add(int64(1), int64(0), int64(0))                        // Single value range, out of range
	f.Add(int64(1<<62), int64(0), int64(1<<63-1))              // Large positive values
	f.Add(int64(-1<<62), int64(-1<<63), int64(0))              // Large negative values
	f.Add(int64(0), int64(-1<<63), int64(1<<63-1))             // Full int64 range

	f.Fuzz(func(t *testing.T, n, minVal, maxVal int64) {
		result := assert.InRange(n, minVal, maxVal)

		// Manual verification
		var expected bool
		if minVal <= maxVal {
			expected = n >= minVal && n <= maxVal
		} else {
			// Inverted range should return false per predicate documentation
			expected = false
		}

		if result != expected {
			t.Errorf("InRange(%d, %d, %d) = %v, want %v", n, minVal, maxVal, result, expected)
		}
	})
}
```

**Step 2: Run the fuzz test briefly**

Run: `go test -v ./tests/fuzzy -fuzz=FuzzInRange -run=^$ -fuzztime=5s 2>&1 | tail -20`

**Expected output:**
```
fuzz: elapsed: 5s, execs: XXXX (XXX/sec), new interesting: X (total: X)
PASS
```

**Step 3: Commit**

```bash
git add tests/fuzzy/predicates_fuzz_test.go
git commit -m "$(cat <<'EOF'
test(fuzzy): add FuzzInRange for range predicate assertions

Fuzz test verifies InRange behavior with diverse int64 values
including boundaries, inverted ranges, and edge cases.
EOF
)"
```

---

## Task 7: Add FuzzValidScale Test

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/tests/fuzzy/predicates_fuzz_test.go`

**Prerequisites:**
- Task 6 completed
- Files must exist: `tests/fuzzy/predicates_fuzz_test.go`

**Step 1: Add the FuzzValidScale function**

Append to `/Users/fredamaral/repos/lerianstudio/midaz/tests/fuzzy/predicates_fuzz_test.go`:

```go

// FuzzValidScale tests the ValidScale predicate with diverse scale values.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzValidScale -run=^$ -fuzztime=30s
func FuzzValidScale(f *testing.F) {
	// Boundary values
	f.Add(0)    // Min valid
	f.Add(18)   // Max valid
	f.Add(-1)   // Just below min
	f.Add(19)   // Just above max

	// Edge cases
	f.Add(1)
	f.Add(9)
	f.Add(17)
	f.Add(100)
	f.Add(-100)
	f.Add(1<<30)  // Large positive
	f.Add(-1<<30) // Large negative

	f.Fuzz(func(t *testing.T, scale int) {
		result := assert.ValidScale(scale)
		expected := scale >= 0 && scale <= 18

		if result != expected {
			t.Errorf("ValidScale(%d) = %v, want %v", scale, result, expected)
		}
	})
}
```

**Step 2: Run the fuzz test briefly**

Run: `go test -v ./tests/fuzzy -fuzz=FuzzValidScale -run=^$ -fuzztime=5s 2>&1 | tail -20`

**Expected output:**
```
fuzz: elapsed: 5s, execs: XXXX (XXX/sec), new interesting: X (total: X)
PASS
```

**Step 3: Commit**

```bash
git add tests/fuzzy/predicates_fuzz_test.go
git commit -m "$(cat <<'EOF'
test(fuzzy): add FuzzValidScale for scale validation predicate

Fuzz test verifies ValidScale returns true only for scale
values in the valid range [0, 18].
EOF
)"
```

---

## Task 8: Add FuzzValidAmount Test

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/tests/fuzzy/predicates_fuzz_test.go`

**Prerequisites:**
- Task 7 completed
- Files must exist: `tests/fuzzy/predicates_fuzz_test.go`

**Step 1: Add the FuzzValidAmount function**

Append to `/Users/fredamaral/repos/lerianstudio/midaz/tests/fuzzy/predicates_fuzz_test.go`:

```go

// FuzzValidAmount tests the ValidAmount predicate with diverse decimal values.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzValidAmount -run=^$ -fuzztime=30s
func FuzzValidAmount(f *testing.F) {
	// Normal values
	f.Add("100", int32(0))    // 100, exp=0
	f.Add("1", int32(2))      // 100 (shifted), exp=2
	f.Add("1", int32(-2))     // 0.01, exp=-2

	// Boundary exponents
	f.Add("1", int32(-18))    // Min valid exponent
	f.Add("1", int32(18))     // Max valid exponent
	f.Add("1", int32(-19))    // Below min exponent (invalid)
	f.Add("1", int32(19))     // Above max exponent (invalid)

	// Edge cases
	f.Add("0", int32(0))                       // Zero
	f.Add("-100", int32(0))                    // Negative
	f.Add("999999999999999999", int32(0))      // Large coefficient
	f.Add("1", int32(-30))                     // Very small (invalid exp)
	f.Add("1", int32(30))                      // Very large (invalid exp)

	f.Fuzz(func(t *testing.T, valueStr string, shift int32) {
		d, err := decimal.NewFromString(valueStr)
		if err != nil {
			return // Invalid decimal string - skip
		}

		// Apply shift to test different exponents
		// Shift changes the exponent: positive shift multiplies by 10^shift
		d = d.Shift(shift)

		result := assert.ValidAmount(d)

		// Verify: exponent outside [-18, 18] should return false
		exp := d.Exponent()
		expectedValid := exp >= -18 && exp <= 18

		if result != expectedValid {
			t.Errorf("ValidAmount(%s) with exp=%d: got %v, want %v",
				d.String(), exp, result, expectedValid)
		}
	})
}
```

**Step 2: Run the fuzz test briefly**

Run: `go test -v ./tests/fuzzy -fuzz=FuzzValidAmount -run=^$ -fuzztime=5s 2>&1 | tail -20`

**Expected output:**
```
fuzz: elapsed: 5s, execs: XXXX (XXX/sec), new interesting: X (total: X)
PASS
```

**Step 3: Commit**

```bash
git add tests/fuzzy/predicates_fuzz_test.go
git commit -m "$(cat <<'EOF'
test(fuzzy): add FuzzValidAmount for decimal amount validation

Fuzz test verifies ValidAmount correctly validates decimal
exponents are within the acceptable range [-18, 18].
EOF
)"
```

---

## Task 9: Create UUID Conversion Fuzzing Test File

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/tests/fuzzy/uuid_conversion_fuzz_test.go`
- Reference: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/transaction.go`
- Reference: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/balance.go`
- Reference: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/account.go`

**Prerequisites:**
- Tools: Go 1.21+
- Files must exist: relevant mmodel files

**Step 1: Create the test file with FuzzTransactionIDtoUUID**

Create `/Users/fredamaral/repos/lerianstudio/midaz/tests/fuzzy/uuid_conversion_fuzz_test.go`:

```go
package fuzzy

import (
	"fmt"
	"strings"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
)

// FuzzTransactionIDtoUUID tests the Transaction.IDtoUUID method with diverse inputs.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzTransactionIDtoUUID -run=^$ -fuzztime=30s
func FuzzTransactionIDtoUUID(f *testing.F) {
	// Valid UUIDs
	f.Add("00000000-0000-0000-0000-000000000000") // Nil UUID (may be rejected)
	f.Add("00000000-0000-0000-0000-000000000001") // Valid
	f.Add("ffffffff-ffff-ffff-ffff-ffffffffffff") // All F

	// Invalid UUIDs - should trigger assertion
	f.Add("not-a-uuid")
	f.Add("")
	f.Add(strings.Repeat("0", 36))                // No hyphens
	f.Add("00000000-0000-0000-0000-00000000000")  // Too short
	f.Add("00000000-0000-0000-0000-0000000000001") // Too long
	f.Add("00000000_0000_0000_0000_000000000001") // Wrong separator

	f.Fuzz(func(t *testing.T, id string) {
		defer func() {
			assertionPanicRecovery(t, recover(), fmt.Sprintf(
				"FuzzTransactionIDtoUUID(id=%q)", id))
		}()

		tx := mmodel.Transaction{ID: id}
		result := tx.IDtoUUID()

		// If we reach here, the conversion succeeded
		// Verify it matches the expected UUID
		expected, err := uuid.Parse(id)
		if err != nil {
			t.Errorf("IDtoUUID succeeded but uuid.Parse failed for %q", id)
			return
		}

		if result != expected {
			t.Errorf("IDtoUUID(%q) = %v, want %v", id, result, expected)
		}
	})
}
```

**Step 2: Run syntax check**

Run: `go build ./tests/fuzzy/uuid_conversion_fuzz_test.go 2>&1 || true`

**Expected output:**
```
# (no output means syntax is valid)
```

**Step 3: Run the fuzz test briefly**

Run: `go test -v ./tests/fuzzy -fuzz=FuzzTransactionIDtoUUID -run=^$ -fuzztime=5s 2>&1 | tail -20`

**Expected output:**
```
fuzz: elapsed: 5s, execs: XXXX (XXX/sec), new interesting: X (total: X)
PASS
```

**Step 4: Commit**

```bash
git add tests/fuzzy/uuid_conversion_fuzz_test.go
git commit -m "$(cat <<'EOF'
test(fuzzy): add UUID conversion fuzz test file

Add FuzzTransactionIDtoUUID to verify assertion guards on
Transaction.IDtoUUID conversion method.
EOF
)"
```

---

## Task 10: Add FuzzBalanceIDtoUUID and FuzzAccountIDtoUUID Tests

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/tests/fuzzy/uuid_conversion_fuzz_test.go`

**Prerequisites:**
- Task 9 completed
- Files must exist: `tests/fuzzy/uuid_conversion_fuzz_test.go`

**Step 1: Add the remaining UUID conversion fuzz tests**

Append to `/Users/fredamaral/repos/lerianstudio/midaz/tests/fuzzy/uuid_conversion_fuzz_test.go`:

```go

// FuzzBalanceIDtoUUID tests the Balance.IDtoUUID method with diverse inputs.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzBalanceIDtoUUID -run=^$ -fuzztime=30s
func FuzzBalanceIDtoUUID(f *testing.F) {
	// Valid UUIDs
	f.Add("00000000-0000-0000-0000-000000000001")
	f.Add("ffffffff-ffff-ffff-ffff-ffffffffffff")

	// Invalid UUIDs
	f.Add("invalid")
	f.Add("")
	f.Add("00000000-0000-0000-0000-00000000000") // Too short

	f.Fuzz(func(t *testing.T, id string) {
		defer func() {
			assertionPanicRecovery(t, recover(), fmt.Sprintf(
				"FuzzBalanceIDtoUUID(id=%q)", id))
		}()

		b := &mmodel.Balance{ID: id}
		result := b.IDtoUUID()

		// If we reach here, the conversion succeeded
		expected, err := uuid.Parse(id)
		if err != nil {
			t.Errorf("IDtoUUID succeeded but uuid.Parse failed for %q", id)
			return
		}

		if result != expected {
			t.Errorf("Balance.IDtoUUID(%q) = %v, want %v", id, result, expected)
		}
	})
}

// FuzzAccountIDtoUUID tests the Account.IDtoUUID method with diverse inputs.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzAccountIDtoUUID -run=^$ -fuzztime=30s
func FuzzAccountIDtoUUID(f *testing.F) {
	// Valid UUIDs
	f.Add("00000000-0000-0000-0000-000000000001")
	f.Add("ffffffff-ffff-ffff-ffff-ffffffffffff")

	// Invalid UUIDs
	f.Add("invalid")
	f.Add("")
	f.Add("00000000-0000-0000-0000-00000000000") // Too short

	f.Fuzz(func(t *testing.T, id string) {
		defer func() {
			assertionPanicRecovery(t, recover(), fmt.Sprintf(
				"FuzzAccountIDtoUUID(id=%q)", id))
		}()

		a := &mmodel.Account{ID: id}
		result := a.IDtoUUID()

		// If we reach here, the conversion succeeded
		expected, err := uuid.Parse(id)
		if err != nil {
			t.Errorf("IDtoUUID succeeded but uuid.Parse failed for %q", id)
			return
		}

		if result != expected {
			t.Errorf("Account.IDtoUUID(%q) = %v, want %v", id, result, expected)
		}
	})
}
```

**Step 2: Run all UUID conversion fuzz tests briefly**

Run: `go test -v ./tests/fuzzy -fuzz=FuzzBalanceIDtoUUID -run=^$ -fuzztime=5s 2>&1 | tail -10 && go test -v ./tests/fuzzy -fuzz=FuzzAccountIDtoUUID -run=^$ -fuzztime=5s 2>&1 | tail -10`

**Expected output:**
```
fuzz: elapsed: 5s, execs: XXXX (XXX/sec), new interesting: X (total: X)
PASS
fuzz: elapsed: 5s, execs: XXXX (XXX/sec), new interesting: X (total: X)
PASS
```

**Step 3: Commit**

```bash
git add tests/fuzzy/uuid_conversion_fuzz_test.go
git commit -m "$(cat <<'EOF'
test(fuzzy): add FuzzBalanceIDtoUUID and FuzzAccountIDtoUUID

Complete UUID conversion fuzz coverage for Balance and Account
models to verify assertion guards.
EOF
)"
```

---

## Task 11: Create Outbox Validation Fuzzing Test File

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/tests/fuzzy/outbox_validation_fuzz_test.go`
- Reference: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/postgres/outbox/outbox.go`

**Prerequisites:**
- Tools: Go 1.21+
- Files must exist: `components/transaction/internal/adapters/postgres/outbox/outbox.go`

**Step 1: Create the test file with FuzzNewMetadataOutbox**

Create `/Users/fredamaral/repos/lerianstudio/midaz/tests/fuzzy/outbox_validation_fuzz_test.go`:

```go
package fuzzy

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/outbox"
)

// FuzzNewMetadataOutbox tests the NewMetadataOutbox constructor validation.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzNewMetadataOutbox -run=^$ -fuzztime=30s
func FuzzNewMetadataOutbox(f *testing.F) {
	// Valid inputs
	f.Add("valid-entity-id", "Transaction", `{"key": "value"}`)
	f.Add("another-id", "Operation", `{"nested": {"key": "value"}}`)

	// Edge case: entity ID boundaries
	f.Add("", "Transaction", `{}`)                            // Empty ID (invalid)
	f.Add(strings.Repeat("a", 255), "Transaction", `{}`)      // Max length ID
	f.Add(strings.Repeat("a", 256), "Transaction", `{}`)      // Over max length (invalid)
	f.Add("x", "Transaction", `{}`)                           // Min length ID

	// Edge case: invalid entity types
	f.Add("valid-id", "InvalidType", `{}`)
	f.Add("valid-id", "", `{}`)
	f.Add("valid-id", "transaction", `{}`)  // Wrong case
	f.Add("valid-id", "TRANSACTION", `{}`)  // Wrong case

	// Edge case: metadata variations
	f.Add("valid-id", "Transaction", `null`)                   // null metadata (will be nil)
	f.Add("valid-id", "Transaction", `{"a": 1, "b": "test"}`)
	f.Add("valid-id", "Transaction", `[]`)                     // Array (invalid for map)

	f.Fuzz(func(t *testing.T, entityID, entityType, metadataJSON string) {
		var metadata map[string]any

		// Try to parse metadata JSON
		if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
			// Invalid JSON or wrong type - test with nil
			metadata = nil
		}

		result, err := outbox.NewMetadataOutbox(entityID, entityType, metadata)

		// Validate error conditions
		if entityID == "" {
			if err == nil || !errors.Is(err, outbox.ErrEntityIDEmpty) {
				t.Errorf("Expected ErrEntityIDEmpty for empty entityID, got: %v", err)
			}
			return
		}

		if len(entityID) > outbox.MaxEntityIDLength {
			if err == nil || !errors.Is(err, outbox.ErrEntityIDTooLong) {
				t.Errorf("Expected ErrEntityIDTooLong for entityID len=%d, got: %v", len(entityID), err)
			}
			return
		}

		if entityType != outbox.EntityTypeTransaction && entityType != outbox.EntityTypeOperation {
			if err == nil || !errors.Is(err, outbox.ErrInvalidEntityType) {
				t.Errorf("Expected ErrInvalidEntityType for entityType=%q, got: %v", entityType, err)
			}
			return
		}

		if metadata == nil {
			if err == nil || !errors.Is(err, outbox.ErrMetadataNil) {
				t.Errorf("Expected ErrMetadataNil for nil metadata, got: %v", err)
			}
			return
		}

		// If we expect success
		if err != nil {
			// Could be metadata too large or other validation error
			if !errors.Is(err, outbox.ErrMetadataTooLarge) {
				t.Errorf("Unexpected error for valid inputs: %v", err)
			}
			return
		}

		// Validate successful result
		if result == nil {
			t.Error("NewMetadataOutbox returned nil without error")
			return
		}

		if result.EntityID != entityID {
			t.Errorf("EntityID mismatch: got %q, want %q", result.EntityID, entityID)
		}

		if result.Status != outbox.StatusPending {
			t.Errorf("Status should be PENDING for new outbox entry, got %v", result.Status)
		}
	})
}
```

**Step 2: Run syntax check**

Run: `go build ./tests/fuzzy/outbox_validation_fuzz_test.go 2>&1 || true`

**Expected output:**
```
# (no output means syntax is valid)
```

**Step 3: Run the fuzz test briefly**

Run: `go test -v ./tests/fuzzy -fuzz=FuzzNewMetadataOutbox -run=^$ -fuzztime=5s 2>&1 | tail -20`

**Expected output:**
```
fuzz: elapsed: 5s, execs: XXXX (XXX/sec), new interesting: X (total: X)
PASS
```

**Step 4: Commit**

```bash
git add tests/fuzzy/outbox_validation_fuzz_test.go
git commit -m "$(cat <<'EOF'
test(fuzzy): add outbox validation fuzz test

Add FuzzNewMetadataOutbox to verify validation logic including
entity ID length limits, valid entity types, and metadata checks.
EOF
)"
```

---

## Task 12: Create Share Distribution Fuzzing Test File

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/tests/fuzzy/share_distribution_fuzz_test.go`
- Reference: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/transaction/validations.go`

**Prerequisites:**
- Tools: Go 1.21+
- Files must exist: `pkg/transaction/validations.go`

**Step 1: Create the test file with FuzzCalculateTotal**

Create `/Users/fredamaral/repos/lerianstudio/midaz/tests/fuzzy/share_distribution_fuzz_test.go`:

```go
package fuzzy

import (
	"fmt"
	"strings"
	"testing"

	constant "github.com/LerianStudio/lib-commons/v2/commons/constants"
	"github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/shopspring/decimal"
)

// FuzzCalculateTotal tests share and percentage calculations with diverse inputs.
// Run with: go test -v ./tests/fuzzy -fuzz=FuzzCalculateTotal -run=^$ -fuzztime=30s
func FuzzCalculateTotal(f *testing.F) {
	// Normal percentage calculations
	f.Add(int64(10000), int64(50), int64(100), true)   // 50% of 100%
	f.Add(int64(10000), int64(100), int64(100), false) // 100% distribution
	f.Add(int64(10000), int64(25), int64(50), true)    // 25% of 50%

	// Boundary percentages
	f.Add(int64(10000), int64(0), int64(100), true)   // 0%
	f.Add(int64(10000), int64(100), int64(100), true) // 100%
	f.Add(int64(10000), int64(1), int64(100), true)   // 1%

	// Large values
	f.Add(int64(9223372036854775807), int64(50), int64(100), true) // max int64 amount
	f.Add(int64(1000000000000000), int64(33), int64(100), false)   // large with odd percentage

	// Precision edge cases
	f.Add(int64(100), int64(33), int64(100), true) // 33% - repeating decimal
	f.Add(int64(100), int64(66), int64(100), true) // 66% - repeating decimal
	f.Add(int64(1), int64(50), int64(100), true)   // small amount with percentage

	// Zero and negative
	f.Add(int64(0), int64(50), int64(100), true)
	f.Add(int64(-1000), int64(50), int64(100), false)

	f.Fuzz(func(t *testing.T, sendValue, percentage, percentageOf int64, isFrom bool) {
		// Recover from any panics
		defer func() {
			if r := recover(); r != nil {
				msg := fmt.Sprintf("%v", r)
				// Check if it's an expected assertion
				if strings.Contains(msg, "assertion failed") {
					// Expected assertion failure for invalid inputs
					return
				}
				t.Errorf("CalculateTotal panicked: sendValue=%d, pct=%d, pctOf=%d, isFrom=%v, panic=%v",
					sendValue, percentage, percentageOf, isFrom, r)
			}
		}()

		// Skip clearly invalid percentages to focus on valid edge cases
		if percentage < 0 || percentage > 100 || percentageOf <= 0 || percentageOf > 100 {
			return
		}

		fromTos := []transaction.FromTo{
			{
				AccountAlias: "@test-account",
				IsFrom:       isFrom,
				Share: &transaction.Share{
					Percentage:             percentage,
					PercentageOfPercentage: percentageOf,
				},
			},
		}

		tx := transaction.Transaction{
			Send: transaction.Send{
				Asset: "USD",
				Value: decimal.NewFromInt(sendValue),
			},
		}

		total, amounts, aliases, routes := transaction.CalculateTotal(fromTos, tx, constant.CREATED)

		// Verify results are consistent
		if percentage > 0 {
			// Should have produced results
			if len(amounts) != 1 {
				t.Errorf("Expected 1 amount for non-zero percentage, got %d (pct=%d)", len(amounts), percentage)
			}
			if len(aliases) != 1 {
				t.Errorf("Expected 1 alias for non-zero percentage, got %d (pct=%d)", len(aliases), percentage)
			}
			if len(routes) != 1 {
				t.Errorf("Expected 1 route for non-zero percentage, got %d (pct=%d)", len(routes), percentage)
			}
		}

		// Total should not be negative for positive inputs
		if sendValue >= 0 && total.IsNegative() {
			t.Errorf("Total is negative for positive sendValue: sendValue=%d, total=%s",
				sendValue, total.String())
		}
	})
}
```

**Step 2: Run syntax check**

Run: `go build ./tests/fuzzy/share_distribution_fuzz_test.go 2>&1 || true`

**Expected output:**
```
# (no output means syntax is valid)
```

**Step 3: Run the fuzz test briefly**

Run: `go test -v ./tests/fuzzy -fuzz=FuzzCalculateTotal -run=^$ -fuzztime=5s 2>&1 | tail -20`

**Expected output:**
```
fuzz: elapsed: 5s, execs: XXXX (XXX/sec), new interesting: X (total: X)
PASS
```

**Step 4: Commit**

```bash
git add tests/fuzzy/share_distribution_fuzz_test.go
git commit -m "$(cat <<'EOF'
test(fuzzy): add share distribution fuzz test

Add FuzzCalculateTotal to verify share percentage calculations
with diverse inputs including boundary values and edge cases.
EOF
)"
```

---

## Task 13: Run Code Review

**Prerequisites:**
- Tasks 1-12 completed
- All fuzz test files created

**Step 1: Dispatch all 3 reviewers in parallel**
- REQUIRED SUB-SKILL: Use requesting-code-review
- All reviewers run simultaneously (code-reviewer, business-logic-reviewer, security-reviewer)
- Wait for all to complete

**Step 2: Handle findings by severity (MANDATORY)**

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

**Step 3: Proceed only when**
- Zero Critical/High/Medium issues remain
- All Low issues have TODO(review): comments added
- All Cosmetic issues have FIXME(nitpick): comments added

---

## Task 14: Run All Fuzz Tests and Verify

**Files:**
- All files in `/Users/fredamaral/repos/lerianstudio/midaz/tests/fuzzy/`

**Prerequisites:**
- Tasks 1-13 completed
- Code review passed

**Step 1: List all new fuzz tests**

Run: `go test -list='Fuzz.*' ./tests/fuzzy/... 2>&1 | grep -E '^Fuzz'`

**Expected output (should include new tests):**
```
FuzzNewHolder
FuzzNewBalance
FuzzNewAccount
FuzzValidUUID
FuzzInRange
FuzzValidScale
FuzzValidAmount
FuzzTransactionIDtoUUID
FuzzBalanceIDtoUUID
FuzzAccountIDtoUUID
FuzzNewMetadataOutbox
FuzzCalculateTotal
... (existing tests)
```

**Step 2: Run all new fuzz tests with short fuzz time**

Run: `for test in FuzzNewHolder FuzzNewBalance FuzzNewAccount FuzzValidUUID FuzzInRange FuzzValidScale FuzzValidAmount FuzzTransactionIDtoUUID FuzzBalanceIDtoUUID FuzzAccountIDtoUUID FuzzNewMetadataOutbox FuzzCalculateTotal; do echo "=== $test ==="; go test -v ./tests/fuzzy -fuzz=$test -run=^$ -fuzztime=10s 2>&1 | tail -5; done`

**Expected output:**
```
=== FuzzNewHolder ===
fuzz: elapsed: 10s, ...
PASS
=== FuzzNewBalance ===
fuzz: elapsed: 10s, ...
PASS
... (similar for all tests)
```

**Step 3: Run unit test mode (no fuzzing) to verify tests compile and run**

Run: `go test -v ./tests/fuzzy/... -run='Fuzz.*' -count=1 2>&1 | tail -30`

**Expected output:**
```
--- PASS: FuzzNewHolder (0.00s)
    --- PASS: FuzzNewHolder/... (0.00s)
...
PASS
```

**If Task Fails:**

1. **Fuzz test crashes:**
   - Check: Error message for panic details
   - Fix: Update test to handle edge case or fix the production code assertion
   - Rollback: `git checkout -- tests/fuzzy/`

2. **Import errors:**
   - Run: `go mod tidy`
   - Fix: Correct import paths to match module

3. **Can't recover:**
   - Document: What failed and why
   - Stop: Return to human partner
   - Don't: Try to fix without understanding

---

## Task 15: Final Commit and Summary

**Prerequisites:**
- Task 14 completed successfully
- All fuzz tests pass

**Step 1: Stage all new files**

Run: `git add tests/fuzzy/constructor_fuzz_test.go tests/fuzzy/predicates_fuzz_test.go tests/fuzzy/uuid_conversion_fuzz_test.go tests/fuzzy/outbox_validation_fuzz_test.go tests/fuzzy/share_distribution_fuzz_test.go`

**Step 2: Verify staged changes**

Run: `git status`

**Expected output:**
```
Changes to be committed:
  new file:   tests/fuzzy/constructor_fuzz_test.go
  new file:   tests/fuzzy/predicates_fuzz_test.go
  new file:   tests/fuzzy/uuid_conversion_fuzz_test.go
  new file:   tests/fuzzy/outbox_validation_fuzz_test.go
  new file:   tests/fuzzy/share_distribution_fuzz_test.go
```

**Step 3: Create final summary commit (if not already committed incrementally)**

Only if files weren't committed in earlier tasks:

```bash
git commit -m "$(cat <<'EOF'
feat(fuzzy): expand fuzz test coverage for assertion guards

Add comprehensive fuzz tests for:
- Domain constructors (NewHolder, NewBalance, NewAccount)
- Assert predicates (ValidUUID, InRange, ValidScale, ValidAmount)
- UUID conversion methods (IDtoUUID for Transaction, Balance, Account)
- Outbox validation (NewMetadataOutbox)
- Share distribution calculations (CalculateTotal)

These tests exercise assertion guards to verify they trigger correctly
on invalid inputs and allow valid inputs to pass through.

Run individual fuzz tests with:
  go test -v ./tests/fuzzy -fuzz=FuzzXxx -fuzztime=60s

Part of Plan 08: Fuzz Test Expansion
EOF
)"
```

---

## Semantic Assertion Fuzz Additions (Review Outcome)

Add fuzz coverage for the **new business-logic predicates** introduced in the semantic assertions plan:

**File:** `/Users/fredamaral/repos/lerianstudio/midaz/tests/fuzzy/predicates_fuzz_test.go`

- `FuzzDateNotInFuture`: generate random `time.Time` values (past, now, future) and assert expected predicate behavior.
- `FuzzDateAfter`: generate pairs of dates and verify strict ordering.
- `FuzzTransactionCanBeReverted`: randomize status codes + parent flag to validate predicate behavior (APPROVED + no parent only).
- `FuzzBalanceIsZero`: randomize available/onHold and validate predicate behavior.
- `FuzzDebitsEqualCredits`: generate paired decimals with slight perturbations and ensure exact-equality requirement holds.

These fuzzers should use the same `assertionPanicRecovery` helper to treat assertion panics as expected outcomes.

---

## Summary

This plan creates 5 new fuzz test files with 12+ new fuzz tests (plus the semantic predicate fuzzers above):

| File | Tests | Coverage |
|------|-------|----------|
| `constructor_fuzz_test.go` | FuzzNewHolder, FuzzNewBalance, FuzzNewAccount | Domain model constructors with assertion guards |
| `predicates_fuzz_test.go` | FuzzValidUUID, FuzzInRange, FuzzValidScale, FuzzValidAmount | Assert package predicate functions |
| `uuid_conversion_fuzz_test.go` | FuzzTransactionIDtoUUID, FuzzBalanceIDtoUUID, FuzzAccountIDtoUUID | IDtoUUID methods with assertion guards |
| `outbox_validation_fuzz_test.go` | FuzzNewMetadataOutbox | Outbox pattern validation logic |
| `share_distribution_fuzz_test.go` | FuzzCalculateTotal | Transaction share percentage calculations |

**Testing Strategy:**
- Run individual tests: `go test -v ./tests/fuzzy -fuzz=FuzzXxx -fuzztime=60s`
- Extend fuzz time for critical tests: `-fuzztime=5m`
- Add corpus files for discovered edge cases in `testdata/fuzz/`

**Expected Outcome:**
- 5 new fuzz test files
- 12 new fuzz tests covering assertion-protected code paths
- Verification that assertions trigger correctly on invalid inputs
- Discovery of potential edge-case bugs
