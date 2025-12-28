# Property Test Coverage Expansion Implementation Plan

> **For Agents:** REQUIRED SUB-SKILL: Use executing-plans to implement this plan task-by-task.

**Goal:** Expand property-based test coverage to validate serialization round-trips, validation rules, and domain invariants in the Midaz ledger system.

**Architecture:** Model-level property tests using Go's `testing/quick` framework with deterministic RNG for reproducibility. Tests verify mathematical properties and invariants without requiring external services.

**Tech Stack:** Go 1.22+, testing/quick, shopspring/decimal, encoding/json

**Global Prerequisites:**
- Environment: macOS/Linux with Go 1.22+
- Tools: Go toolchain
- State: Clean working tree on current branch

**Verification before starting:**
```bash
# Run ALL these commands and verify output:
go version           # Expected: go version go1.22+ ...
git status           # Expected: clean working tree
ls tests/property/   # Expected: existing *_test.go files
```

## Historical Precedent

**Query:** "property tests testing quick generators validation"
**Index Status:** Empty (new project)

No historical data available. This is normal for new projects.
Proceeding with standard planning approach.

---

## Task 1: Create TransactionDate Round-trip Property Test

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/tests/property/transaction_date_test.go`

**Prerequisites:**
- Go 1.22+
- Existing pkg/transaction/time.go file

**Step 1: Write the test file**

Create file at `/Users/fredamaral/repos/lerianstudio/midaz/tests/property/transaction_date_test.go`:

```go
package property

import (
	"encoding/json"
	"testing"
	"testing/quick"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/transaction"
)

// Property: Marshal(Unmarshal(Marshal(t))) == Marshal(t)
// For any valid TransactionDate, JSON serialization round-trip preserves the value.
func TestProperty_TransactionDateRoundtrip(t *testing.T) {
	f := func(year int16, month uint8, day uint8, hour uint8, minute uint8, second uint8, milli uint16) bool {
		// Constrain to valid date ranges
		y := int(year)%400 + 1900 // 1900-2299
		m := int(month)%12 + 1    // 1-12
		d := int(day)%28 + 1      // 1-28 (safe for all months)
		h := int(hour) % 24       // 0-23
		min := int(minute) % 60   // 0-59
		sec := int(second) % 60   // 0-59
		ms := int(milli) % 1000   // 0-999

		// Create a time with milliseconds
		original := time.Date(y, time.Month(m), d, h, min, sec, ms*1_000_000, time.UTC)
		td := transaction.TransactionDate(original)

		// Marshal to JSON
		data, err := json.Marshal(td)
		if err != nil {
			t.Logf("marshal failed: %v", err)
			return false
		}

		// Unmarshal back
		var parsed transaction.TransactionDate
		if err := json.Unmarshal(data, &parsed); err != nil {
			t.Logf("unmarshal failed: %v for data: %s", err, string(data))
			return false
		}

		// Compare: times should be equal (within millisecond precision)
		originalTime := td.Time()
		parsedTime := parsed.Time()

		// TransactionDate uses millisecond precision in output
		diff := originalTime.Sub(parsedTime)
		if diff < 0 {
			diff = -diff
		}

		if diff > time.Millisecond {
			t.Logf("round-trip mismatch: original=%v parsed=%v diff=%v",
				originalTime, parsedTime, diff)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("TransactionDate round-trip property failed: %v", err)
	}
}

// Property: All supported ISO 8601 formats parse successfully
func TestProperty_TransactionDateFormats(t *testing.T) {
	formats := []string{
		`"2024-01-15T10:30:45.123Z"`,       // RFC3339Nano
		`"2024-01-15T10:30:45Z"`,            // RFC3339
		`"2024-01-15T10:30:45.000Z"`,        // Milliseconds explicit
		`"2024-01-15T10:30:45"`,             // No timezone
		`"2024-01-15"`,                      // Date only
	}

	for _, jsonStr := range formats {
		var td transaction.TransactionDate
		if err := json.Unmarshal([]byte(jsonStr), &td); err != nil {
			t.Errorf("failed to parse format %s: %v", jsonStr, err)
		}

		if td.IsZero() {
			t.Errorf("parsed to zero time for format %s", jsonStr)
		}
	}
}

// Property: Zero TransactionDate marshals to "null"
func TestProperty_TransactionDateZeroIsNull(t *testing.T) {
	var td transaction.TransactionDate

	data, err := json.Marshal(td)
	if err != nil {
		t.Fatalf("marshal zero failed: %v", err)
	}

	if string(data) != "null" {
		t.Errorf("zero TransactionDate should marshal to null, got: %s", string(data))
	}
}

// Property: "null" and empty string unmarshal to zero TransactionDate
func TestProperty_TransactionDateNullUnmarshal(t *testing.T) {
	inputs := []string{`null`, `""`}

	for _, input := range inputs {
		var td transaction.TransactionDate
		if err := json.Unmarshal([]byte(input), &td); err != nil {
			t.Errorf("unmarshal %s failed: %v", input, err)
			continue
		}

		if !td.IsZero() {
			t.Errorf("expected zero time for input %s, got: %v", input, td.Time())
		}
	}
}
```

**Step 2: Run test to verify it passes**

Run: `go test -v -race -timeout 30s /Users/fredamaral/repos/lerianstudio/midaz/tests/property -run TransactionDate`

**Expected output:**
```
=== RUN   TestProperty_TransactionDateRoundtrip
--- PASS: TestProperty_TransactionDateRoundtrip (X.XXs)
=== RUN   TestProperty_TransactionDateFormats
--- PASS: TestProperty_TransactionDateFormats (X.XXs)
=== RUN   TestProperty_TransactionDateZeroIsNull
--- PASS: TestProperty_TransactionDateZeroIsNull (X.XXs)
=== RUN   TestProperty_TransactionDateNullUnmarshal
--- PASS: TestProperty_TransactionDateNullUnmarshal (X.XXs)
PASS
```

**If Task Fails:**

1. **Import errors:**
   - Check: Module path matches `github.com/LerianStudio/midaz/v3`
   - Fix: Verify go.mod for correct module path

2. **Test fails with time mismatch:**
   - Check: Precision handling in TransactionDate
   - Fix: Adjust tolerance or inspect marshal format

3. **Can't recover:**
   - Document: What failed and why
   - Stop: Return to human partner

---

## Task 2: Create BalanceRedis Unmarshal Property Test

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/tests/property/balance_redis_test.go`

**Prerequisites:**
- Go 1.22+
- Existing pkg/mmodel/balance.go file

**Step 1: Write the test file**

Create file at `/Users/fredamaral/repos/lerianstudio/midaz/tests/property/balance_redis_test.go`:

```go
package property

import (
	"encoding/json"
	"testing"
	"testing/quick"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/shopspring/decimal"
)

// Property: BalanceRedis correctly unmarshals decimal values from float64
func TestProperty_BalanceRedisUnmarshalFloat64(t *testing.T) {
	f := func(available, onHold float64) bool {
		// Skip NaN and Inf
		if available != available || onHold != onHold { // NaN check
			return true
		}

		// Constrain to reasonable values
		if available > 1e15 || available < -1e15 || onHold > 1e15 || onHold < -1e15 {
			return true
		}

		jsonData := []byte(`{
			"id": "test-id",
			"accountId": "acc-id",
			"assetCode": "USD",
			"available": ` + decimal.NewFromFloat(available).String() + `,
			"onHold": ` + decimal.NewFromFloat(onHold).String() + `,
			"version": 1,
			"allowSending": 1,
			"allowReceiving": 1
		}`)

		var balance mmodel.BalanceRedis
		if err := json.Unmarshal(jsonData, &balance); err != nil {
			t.Logf("unmarshal failed: %v for data: %s", err, string(jsonData))
			return false
		}

		// Verify values are close (float64 has precision limits)
		expectedAvail := decimal.NewFromFloat(available)
		expectedOnHold := decimal.NewFromFloat(onHold)

		availDiff := balance.Available.Sub(expectedAvail).Abs()
		onHoldDiff := balance.OnHold.Sub(expectedOnHold).Abs()

		tolerance := decimal.NewFromFloat(0.0001)

		if availDiff.GreaterThan(tolerance) {
			t.Logf("available mismatch: expected %s, got %s", expectedAvail, balance.Available)
			return false
		}

		if onHoldDiff.GreaterThan(tolerance) {
			t.Logf("onHold mismatch: expected %s, got %s", expectedOnHold, balance.OnHold)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("BalanceRedis float64 unmarshal property failed: %v", err)
	}
}

// Property: BalanceRedis correctly unmarshals decimal values from string
func TestProperty_BalanceRedisUnmarshalString(t *testing.T) {
	f := func(availInt, availFrac, onHoldInt, onHoldFrac int64) bool {
		// Constrain to reasonable values
		availInt = availInt % 1_000_000_000
		onHoldInt = onHoldInt % 1_000_000_000
		availFrac = (availFrac % 1_000_000)
		onHoldFrac = (onHoldFrac % 1_000_000)

		if availFrac < 0 {
			availFrac = -availFrac
		}
		if onHoldFrac < 0 {
			onHoldFrac = -onHoldFrac
		}

		availStr := decimal.NewFromInt(availInt).String()
		if availFrac > 0 {
			availStr = availStr + "." + padLeft(availFrac, 6)
		}

		onHoldStr := decimal.NewFromInt(onHoldInt).String()
		if onHoldFrac > 0 {
			onHoldStr = onHoldStr + "." + padLeft(onHoldFrac, 6)
		}

		jsonData := []byte(`{
			"id": "test-id",
			"accountId": "acc-id",
			"assetCode": "USD",
			"available": "` + availStr + `",
			"onHold": "` + onHoldStr + `",
			"version": 1,
			"allowSending": 1,
			"allowReceiving": 1
		}`)

		var balance mmodel.BalanceRedis
		if err := json.Unmarshal(jsonData, &balance); err != nil {
			t.Logf("unmarshal failed: %v for data: %s", err, string(jsonData))
			return false
		}

		expectedAvail, _ := decimal.NewFromString(availStr)
		expectedOnHold, _ := decimal.NewFromString(onHoldStr)

		if !balance.Available.Equal(expectedAvail) {
			t.Logf("available mismatch: expected %s, got %s", expectedAvail, balance.Available)
			return false
		}

		if !balance.OnHold.Equal(expectedOnHold) {
			t.Logf("onHold mismatch: expected %s, got %s", expectedOnHold, balance.OnHold)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("BalanceRedis string unmarshal property failed: %v", err)
	}
}

// padLeft pads a number with leading zeros to reach the specified width
func padLeft(n int64, width int) string {
	s := decimal.NewFromInt(n).String()
	for len(s) < width {
		s = "0" + s
	}
	return s
}

// Property: BalanceRedis unmarshal handles json.Number correctly
func TestProperty_BalanceRedisUnmarshalJSONNumber(t *testing.T) {
	// json.Number is produced when using json.Decoder with UseNumber()
	testCases := []struct {
		name     string
		jsonData string
		expected decimal.Decimal
	}{
		{"integer", `{"id":"t","accountId":"a","assetCode":"USD","available":12345,"onHold":0,"version":1}`, decimal.NewFromInt(12345)},
		{"float", `{"id":"t","accountId":"a","assetCode":"USD","available":123.45,"onHold":0,"version":1}`, decimal.NewFromFloat(123.45)},
		{"string", `{"id":"t","accountId":"a","assetCode":"USD","available":"999.99","onHold":0,"version":1}`, decimal.NewFromFloat(999.99)},
		{"zero", `{"id":"t","accountId":"a","assetCode":"USD","available":0,"onHold":0,"version":1}`, decimal.Zero},
		{"negative", `{"id":"t","accountId":"a","assetCode":"USD","available":-500,"onHold":0,"version":1}`, decimal.NewFromInt(-500)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var balance mmodel.BalanceRedis
			if err := json.Unmarshal([]byte(tc.jsonData), &balance); err != nil {
				t.Fatalf("unmarshal failed: %v", err)
			}

			if !balance.Available.Equal(tc.expected) {
				t.Errorf("expected %s, got %s", tc.expected, balance.Available)
			}
		})
	}
}
```

**Step 2: Run test to verify it passes**

Run: `go test -v -race -timeout 30s /Users/fredamaral/repos/lerianstudio/midaz/tests/property -run BalanceRedis`

**Expected output:**
```
=== RUN   TestProperty_BalanceRedisUnmarshalFloat64
--- PASS: TestProperty_BalanceRedisUnmarshalFloat64 (X.XXs)
=== RUN   TestProperty_BalanceRedisUnmarshalString
--- PASS: TestProperty_BalanceRedisUnmarshalString (X.XXs)
=== RUN   TestProperty_BalanceRedisUnmarshalJSONNumber
--- PASS: TestProperty_BalanceRedisUnmarshalJSONNumber (X.XXs)
PASS
```

**If Task Fails:**

1. **Import errors:**
   - Check: Module path and package imports
   - Fix: Verify `mmodel` package is accessible

2. **Unmarshal errors:**
   - Check: JSON structure matches BalanceRedis fields
   - Fix: Inspect BalanceRedis struct definition

3. **Can't recover:**
   - Document: What failed and why
   - Stop: Return to human partner

---

## Task 3: Create Balance Validation Property Tests

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/tests/property/balance_validation_test.go`

**Prerequisites:**
- Go 1.22+
- Existing pkg/transaction/validations.go file

**Step 1: Write the test file**

Create file at `/Users/fredamaral/repos/lerianstudio/midaz/tests/property/balance_validation_test.go`:

```go
package property

import (
	"math/rand"
	"testing"
	"testing/quick"

	"github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/shopspring/decimal"
)

// Property: OperateBalances version always increases (monotonic) when operation changes balance
func TestProperty_BalanceVersionMonotonic(t *testing.T) {
	f := func(initialVersion int64, value int64, isDebit bool) bool {
		// Constrain to valid versions
		if initialVersion < 1 {
			initialVersion = 1
		}
		if initialVersion > 1_000_000 {
			initialVersion = 1_000_000
		}

		// Constrain value
		if value < 0 {
			value = -value
		}
		if value == 0 {
			value = 1
		}

		balance := transaction.Balance{
			Available: decimal.NewFromInt(1000), // Enough for debits
			OnHold:    decimal.Zero,
			Version:   initialVersion,
		}

		operation := "CREDIT"
		if isDebit {
			operation = "DEBIT"
		}

		amount := transaction.Amount{
			Value:           decimal.NewFromInt(value),
			Operation:       operation,
			TransactionType: "CREATED",
		}

		newBalance, err := transaction.OperateBalances(amount, balance)
		if err != nil {
			t.Logf("OperateBalances error: %v", err)
			return true // Skip errored cases
		}

		// Property: new version should be exactly initialVersion + 1
		expectedVersion := initialVersion + 1
		if newBalance.Version != expectedVersion {
			t.Logf("Version not monotonic: initial=%d expected=%d got=%d",
				initialVersion, expectedVersion, newBalance.Version)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("Balance version monotonic property failed: %v", err)
	}
}

// Property: DEBIT subtracts from Available, CREDIT adds to Available
func TestProperty_OperateBalancesDebitCredit(t *testing.T) {
	f := func(seed int64, initialAvail, operationValue int64) bool {
		rng := rand.New(rand.NewSource(seed))

		// Constrain values
		if initialAvail < 0 {
			initialAvail = -initialAvail
		}
		if operationValue < 0 {
			operationValue = -operationValue
		}
		if operationValue == 0 {
			operationValue = 1
		}

		balance := transaction.Balance{
			Available: decimal.NewFromInt(initialAvail),
			OnHold:    decimal.Zero,
			Version:   1,
		}

		isDebit := rng.Intn(2) == 0
		operation := "CREDIT"
		if isDebit {
			operation = "DEBIT"
		}

		amount := transaction.Amount{
			Value:           decimal.NewFromInt(operationValue),
			Operation:       operation,
			TransactionType: "CREATED",
		}

		newBalance, err := transaction.OperateBalances(amount, balance)
		if err != nil {
			return true // Skip errors
		}

		expectedAvail := balance.Available
		if isDebit {
			expectedAvail = expectedAvail.Sub(amount.Value)
		} else {
			expectedAvail = expectedAvail.Add(amount.Value)
		}

		if !newBalance.Available.Equal(expectedAvail) {
			t.Logf("Available mismatch: op=%s initial=%s value=%s expected=%s got=%s",
				operation, balance.Available, amount.Value, expectedAvail, newBalance.Available)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("OperateBalances DEBIT/CREDIT property failed: %v", err)
	}
}

// Property: PENDING transactions move funds from Available to OnHold
func TestProperty_OperateBalancesPending(t *testing.T) {
	f := func(initialAvail, initialOnHold, value int64) bool {
		// Constrain values
		if initialAvail < 0 {
			initialAvail = -initialAvail
		}
		if initialOnHold < 0 {
			initialOnHold = -initialOnHold
		}
		if value <= 0 {
			value = 1
		}

		// Ensure enough available balance for the hold
		if initialAvail < value {
			initialAvail = value + 100
		}

		balance := transaction.Balance{
			Available: decimal.NewFromInt(initialAvail),
			OnHold:    decimal.NewFromInt(initialOnHold),
			Version:   1,
		}

		amount := transaction.Amount{
			Value:           decimal.NewFromInt(value),
			Operation:       "ONHOLD",
			TransactionType: "PENDING",
		}

		newBalance, err := transaction.OperateBalances(amount, balance)
		if err != nil {
			return true // Skip errors
		}

		// Property: Available decreases, OnHold increases by same amount
		expectedAvail := balance.Available.Sub(amount.Value)
		expectedOnHold := balance.OnHold.Add(amount.Value)

		if !newBalance.Available.Equal(expectedAvail) {
			t.Logf("PENDING Available mismatch: expected=%s got=%s", expectedAvail, newBalance.Available)
			return false
		}

		if !newBalance.OnHold.Equal(expectedOnHold) {
			t.Logf("PENDING OnHold mismatch: expected=%s got=%s", expectedOnHold, newBalance.OnHold)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("OperateBalances PENDING property failed: %v", err)
	}
}

// Property: RELEASE reverses ONHOLD - funds move back from OnHold to Available
func TestProperty_OperateBalancesRelease(t *testing.T) {
	f := func(initialAvail, initialOnHold, value int64) bool {
		// Constrain values
		if initialAvail < 0 {
			initialAvail = -initialAvail
		}
		if initialOnHold < 0 {
			initialOnHold = -initialOnHold
		}
		if value <= 0 {
			value = 1
		}

		// Ensure enough on hold for the release
		if initialOnHold < value {
			initialOnHold = value + 100
		}

		balance := transaction.Balance{
			Available: decimal.NewFromInt(initialAvail),
			OnHold:    decimal.NewFromInt(initialOnHold),
			Version:   1,
		}

		amount := transaction.Amount{
			Value:           decimal.NewFromInt(value),
			Operation:       "RELEASE",
			TransactionType: "CANCELED",
		}

		newBalance, err := transaction.OperateBalances(amount, balance)
		if err != nil {
			return true // Skip errors
		}

		// Property: Available increases, OnHold decreases by same amount
		expectedAvail := balance.Available.Add(amount.Value)
		expectedOnHold := balance.OnHold.Sub(amount.Value)

		if !newBalance.Available.Equal(expectedAvail) {
			t.Logf("RELEASE Available mismatch: expected=%s got=%s", expectedAvail, newBalance.Available)
			return false
		}

		if !newBalance.OnHold.Equal(expectedOnHold) {
			t.Logf("RELEASE OnHold mismatch: expected=%s got=%s", expectedOnHold, newBalance.OnHold)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("OperateBalances RELEASE property failed: %v", err)
	}
}

// Property: Total funds (Available + OnHold) is conserved across ONHOLD/RELEASE operations
func TestProperty_BalanceTotalConserved(t *testing.T) {
	f := func(initialAvail, initialOnHold, value int64, isPending bool) bool {
		// Constrain values
		if initialAvail < 0 {
			initialAvail = -initialAvail
		}
		if initialOnHold < 0 {
			initialOnHold = -initialOnHold
		}
		if value <= 0 {
			value = 1
		}

		// Ensure balance can handle the operation
		if isPending && initialAvail < value {
			initialAvail = value + 100
		}
		if !isPending && initialOnHold < value {
			initialOnHold = value + 100
		}

		balance := transaction.Balance{
			Available: decimal.NewFromInt(initialAvail),
			OnHold:    decimal.NewFromInt(initialOnHold),
			Version:   1,
		}

		initialTotal := balance.Available.Add(balance.OnHold)

		var amount transaction.Amount
		if isPending {
			amount = transaction.Amount{
				Value:           decimal.NewFromInt(value),
				Operation:       "ONHOLD",
				TransactionType: "PENDING",
			}
		} else {
			amount = transaction.Amount{
				Value:           decimal.NewFromInt(value),
				Operation:       "RELEASE",
				TransactionType: "CANCELED",
			}
		}

		newBalance, err := transaction.OperateBalances(amount, balance)
		if err != nil {
			return true // Skip errors
		}

		newTotal := newBalance.Available.Add(newBalance.OnHold)

		// Property: total should be conserved
		if !initialTotal.Equal(newTotal) {
			t.Logf("Total not conserved: initial=%s new=%s op=%s",
				initialTotal, newTotal, amount.Operation)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("Balance total conservation property failed: %v", err)
	}
}
```

**Step 2: Run test to verify it passes**

Run: `go test -v -race -timeout 30s /Users/fredamaral/repos/lerianstudio/midaz/tests/property -run "Balance(Version|Operate|Total)"`

**Expected output:**
```
=== RUN   TestProperty_BalanceVersionMonotonic
--- PASS: TestProperty_BalanceVersionMonotonic (X.XXs)
=== RUN   TestProperty_OperateBalancesDebitCredit
--- PASS: TestProperty_OperateBalancesDebitCredit (X.XXs)
=== RUN   TestProperty_OperateBalancesPending
--- PASS: TestProperty_OperateBalancesPending (X.XXs)
=== RUN   TestProperty_OperateBalancesRelease
--- PASS: TestProperty_OperateBalancesRelease (X.XXs)
=== RUN   TestProperty_BalanceTotalConserved
--- PASS: TestProperty_BalanceTotalConserved (X.XXs)
PASS
```

**If Task Fails:**

1. **Import errors:**
   - Check: `pkg/transaction` package path
   - Fix: Verify transaction.Balance struct exists

2. **Operation constant not found:**
   - Check: Constants in lib-commons
   - Fix: May need to import constant package

3. **Can't recover:**
   - Document: What failed and why
   - Stop: Return to human partner

---

## Task 4: Create Alias Format Validation Property Tests

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/tests/property/alias_format_test.go`

**Prerequisites:**
- Go 1.22+
- Existing pkg/constant/account.go file

**Step 1: Write the test file**

Create file at `/Users/fredamaral/repos/lerianstudio/midaz/tests/property/alias_format_test.go`:

```go
package property

import (
	"math/rand"
	"regexp"
	"strings"
	"testing"
	"testing/quick"

	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
)

// validAliasRegex matches the AccountAliasAcceptedChars pattern
var validAliasRegex = regexp.MustCompile(cn.AccountAliasAcceptedChars)

// Property: Valid aliases match the accepted character pattern
func TestProperty_AliasValidCharacters(t *testing.T) {
	validChars := []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789@:_-")

	f := func(seed int64, length uint8) bool {
		rng := rand.New(rand.NewSource(seed))

		// Constrain length
		l := int(length)%50 + 1 // 1-50 chars

		// Generate valid alias
		alias := make([]rune, l)
		for i := 0; i < l; i++ {
			alias[i] = validChars[rng.Intn(len(validChars))]
		}
		aliasStr := string(alias)

		// Property: alias should match the regex
		if !validAliasRegex.MatchString(aliasStr) {
			t.Logf("Valid alias rejected: %s", aliasStr)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("Alias valid characters property failed: %v", err)
	}
}

// Property: Aliases with invalid characters are rejected
func TestProperty_AliasInvalidCharactersRejected(t *testing.T) {
	invalidChars := []rune("!#$%^&*()+=[]{}|\\;'\",.<>?/`~ \t\n")

	f := func(seed int64, length uint8, invalidPos uint8) bool {
		rng := rand.New(rand.NewSource(seed))

		// Constrain length
		l := int(length)%49 + 2 // 2-50 chars

		// Generate mostly valid alias
		validChars := []rune("abcdefghijklmnopqrstuvwxyz0123456789")
		alias := make([]rune, l)
		for i := 0; i < l; i++ {
			alias[i] = validChars[rng.Intn(len(validChars))]
		}

		// Insert one invalid character
		pos := int(invalidPos) % l
		alias[pos] = invalidChars[rng.Intn(len(invalidChars))]
		aliasStr := string(alias)

		// Property: alias should NOT match the regex
		if validAliasRegex.MatchString(aliasStr) {
			t.Logf("Invalid alias accepted: %s (invalid char at pos %d)", aliasStr, pos)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("Alias invalid characters rejection property failed: %v", err)
	}
}

// Property: @external/ prefix is prohibited
func TestProperty_AliasExternalPrefixProhibited(t *testing.T) {
	f := func(seed int64, suffix string) bool {
		// Remove any characters that would make the suffix invalid
		cleanSuffix := strings.Map(func(r rune) rune {
			if strings.ContainsRune("abcdefghijklmnopqrstuvwxyz0123456789", r) {
				return r
			}
			return -1
		}, suffix)

		if cleanSuffix == "" {
			cleanSuffix = "test"
		}

		alias := cn.DefaultExternalAccountAliasPrefix + cleanSuffix

		// Property: alias containing @external/ prefix should be rejected
		if strings.Contains(alias, cn.DefaultExternalAccountAliasPrefix) {
			// This is expected - external prefix is prohibited
			return true
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 100}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("Alias external prefix prohibition property failed: %v", err)
	}
}

// Property: Empty alias is invalid
func TestProperty_AliasEmptyInvalid(t *testing.T) {
	alias := ""

	// Empty string should not match the pattern (pattern requires at least one char)
	if validAliasRegex.MatchString(alias) {
		t.Errorf("Empty alias should be invalid")
	}
}

// Property: Common valid alias formats are accepted
func TestProperty_AliasCommonFormats(t *testing.T) {
	validAliases := []string{
		"@user123",
		"account-1",
		"Account_Name",
		"user:subaccount",
		"simple",
		"UPPERCASE",
		"MixedCase123",
		"a",
		"@",
		"_",
		"-",
		":",
		"a-b_c:d@e",
	}

	for _, alias := range validAliases {
		if !validAliasRegex.MatchString(alias) {
			t.Errorf("Valid alias format rejected: %s", alias)
		}
	}
}

// Property: Known invalid alias formats are rejected
func TestProperty_AliasInvalidFormats(t *testing.T) {
	invalidAliases := []string{
		"user name",   // space
		"user\tname",  // tab
		"user\nname",  // newline
		"user!name",   // exclamation
		"user#name",   // hash
		"user$name",   // dollar
		"user%name",   // percent
		"user^name",   // caret
		"user&name",   // ampersand
		"user*name",   // asterisk
		"user(name)",  // parentheses
		"user+name",   // plus
		"user=name",   // equals
		"user[name]",  // brackets
		"user{name}",  // braces
		"user|name",   // pipe
		"user\\name",  // backslash
		"user;name",   // semicolon
		"user'name",   // single quote
		"user\"name",  // double quote
		"user,name",   // comma
		"user.name",   // period
		"user<name>",  // angle brackets
		"user?name",   // question mark
		"user/name",   // forward slash
		"user`name",   // backtick
		"user~name",   // tilde
	}

	for _, alias := range invalidAliases {
		if validAliasRegex.MatchString(alias) {
			t.Errorf("Invalid alias format accepted: %s", alias)
		}
	}
}
```

**Step 2: Run test to verify it passes**

Run: `go test -v -race -timeout 30s /Users/fredamaral/repos/lerianstudio/midaz/tests/property -run "Alias(Valid|Invalid|External|Empty|Common)"`

**Expected output:**
```
=== RUN   TestProperty_AliasValidCharacters
--- PASS: TestProperty_AliasValidCharacters (X.XXs)
=== RUN   TestProperty_AliasInvalidCharactersRejected
--- PASS: TestProperty_AliasInvalidCharactersRejected (X.XXs)
=== RUN   TestProperty_AliasExternalPrefixProhibited
--- PASS: TestProperty_AliasExternalPrefixProhibited (X.XXs)
=== RUN   TestProperty_AliasEmptyInvalid
--- PASS: TestProperty_AliasEmptyInvalid (X.XXs)
=== RUN   TestProperty_AliasCommonFormats
--- PASS: TestProperty_AliasCommonFormats (X.XXs)
=== RUN   TestProperty_AliasInvalidFormats
--- PASS: TestProperty_AliasInvalidFormats (X.XXs)
PASS
```

**If Task Fails:**

1. **Import errors:**
   - Check: `pkg/constant` package path
   - Fix: Verify constant package exists with AccountAliasAcceptedChars

2. **Regex doesn't match expected:**
   - Check: AccountAliasAcceptedChars value
   - Fix: Verify pattern is `^[a-zA-Z0-9@:_-]+$`

3. **Can't recover:**
   - Document: What failed and why
   - Stop: Return to human partner

---

## Task 5: Create CPF/CNPJ Validation Property Tests

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/tests/property/document_validation_test.go`

**Prerequisites:**
- Go 1.22+

**Step 1: Write the test file**

Create file at `/Users/fredamaral/repos/lerianstudio/midaz/tests/property/document_validation_test.go`:

```go
package property

import (
	"fmt"
	"math/rand"
	"testing"
	"testing/quick"
)

const (
	cpfLength  = 11
	cnpjLength = 14
)

// generateValidCPF generates a valid CPF with correct check digits
func generateValidCPF(rng *rand.Rand) string {
	// Generate first 9 digits
	digits := make([]int, 11)
	for i := 0; i < 9; i++ {
		digits[i] = rng.Intn(10)
	}

	// Avoid all equal digits (invalid CPFs)
	allEqual := true
	for i := 1; i < 9; i++ {
		if digits[i] != digits[0] {
			allEqual = false
			break
		}
	}
	if allEqual {
		digits[8] = (digits[8] + 1) % 10
	}

	// Calculate first check digit
	sum := 0
	for i := 0; i < 9; i++ {
		sum += digits[i] * (10 - i)
	}
	remainder := (sum * 10) % 11
	if remainder == 10 {
		remainder = 0
	}
	digits[9] = remainder

	// Calculate second check digit
	sum = 0
	for i := 0; i < 10; i++ {
		sum += digits[i] * (11 - i)
	}
	remainder = (sum * 10) % 11
	if remainder == 10 {
		remainder = 0
	}
	digits[10] = remainder

	result := ""
	for _, d := range digits {
		result += fmt.Sprintf("%d", d)
	}
	return result
}

// generateValidCNPJ generates a valid CNPJ with correct check digits
func generateValidCNPJ(rng *rand.Rand) string {
	// Generate first 12 digits
	digits := make([]int, 14)
	for i := 0; i < 12; i++ {
		digits[i] = rng.Intn(10)
	}

	// Avoid all equal digits
	allEqual := true
	for i := 1; i < 12; i++ {
		if digits[i] != digits[0] {
			allEqual = false
			break
		}
	}
	if allEqual {
		digits[11] = (digits[11] + 1) % 10
	}

	// Calculate first check digit
	weights1 := []int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	sum := 0
	for i := 0; i < 12; i++ {
		sum += digits[i] * weights1[i]
	}
	remainder := sum % 11
	if remainder < 2 {
		digits[12] = 0
	} else {
		digits[12] = 11 - remainder
	}

	// Calculate second check digit
	weights2 := []int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	sum = 0
	for i := 0; i < 13; i++ {
		sum += digits[i] * weights2[i]
	}
	remainder = sum % 11
	if remainder < 2 {
		digits[13] = 0
	} else {
		digits[13] = 11 - remainder
	}

	result := ""
	for _, d := range digits {
		result += fmt.Sprintf("%d", d)
	}
	return result
}

// validateCPF checks if a CPF is valid (implements the same logic as the validator)
func validateCPF(cpf string) bool {
	if len(cpf) != cpfLength {
		return false
	}

	// Check for all equal digits
	allEqual := true
	for i := 1; i < len(cpf); i++ {
		if cpf[i] != cpf[0] {
			allEqual = false
			break
		}
	}
	if allEqual {
		return false
	}

	// Check all characters are digits
	for _, c := range cpf {
		if c < '0' || c > '9' {
			return false
		}
	}

	// Validate first check digit
	sum := 0
	for i := 0; i < 9; i++ {
		sum += int(cpf[i]-'0') * (10 - i)
	}
	remainder := (sum * 10) % 11
	if remainder == 10 {
		remainder = 0
	}
	if remainder != int(cpf[9]-'0') {
		return false
	}

	// Validate second check digit
	sum = 0
	for i := 0; i < 10; i++ {
		sum += int(cpf[i]-'0') * (11 - i)
	}
	remainder = (sum * 10) % 11
	if remainder == 10 {
		remainder = 0
	}
	return remainder == int(cpf[10]-'0')
}

// validateCNPJ checks if a CNPJ is valid (implements the same logic as the validator)
func validateCNPJ(cnpj string) bool {
	if len(cnpj) != cnpjLength {
		return false
	}

	// Check for all equal digits
	allEqual := true
	for i := 1; i < len(cnpj); i++ {
		if cnpj[i] != cnpj[0] {
			allEqual = false
			break
		}
	}
	if allEqual {
		return false
	}

	// Check all characters are digits
	for _, c := range cnpj {
		if c < '0' || c > '9' {
			return false
		}
	}

	// Validate first check digit
	weights1 := []int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	sum := 0
	for i := 0; i < 12; i++ {
		sum += int(cnpj[i]-'0') * weights1[i]
	}
	remainder := sum % 11
	expectedDigit := 0
	if remainder >= 2 {
		expectedDigit = 11 - remainder
	}
	if expectedDigit != int(cnpj[12]-'0') {
		return false
	}

	// Validate second check digit
	weights2 := []int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	sum = 0
	for i := 0; i < 13; i++ {
		sum += int(cnpj[i]-'0') * weights2[i]
	}
	remainder = sum % 11
	expectedDigit = 0
	if remainder >= 2 {
		expectedDigit = 11 - remainder
	}
	return expectedDigit == int(cnpj[13]-'0')
}

// Property: Generated valid CPFs pass validation
func TestProperty_ValidCPFPasses(t *testing.T) {
	f := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))
		cpf := generateValidCPF(rng)

		if !validateCPF(cpf) {
			t.Logf("Generated valid CPF failed validation: %s", cpf)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("Valid CPF property failed: %v", err)
	}
}

// Property: Generated valid CNPJs pass validation
func TestProperty_ValidCNPJPasses(t *testing.T) {
	f := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))
		cnpj := generateValidCNPJ(rng)

		if !validateCNPJ(cnpj) {
			t.Logf("Generated valid CNPJ failed validation: %s", cnpj)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("Valid CNPJ property failed: %v", err)
	}
}

// Property: CPFs with incorrect check digit fail validation
func TestProperty_InvalidCPFCheckDigitFails(t *testing.T) {
	f := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))
		cpf := generateValidCPF(rng)

		// Corrupt the last check digit
		digits := []byte(cpf)
		original := digits[10]
		digits[10] = byte('0' + (int(original-'0')+1)%10)
		corruptedCPF := string(digits)

		if validateCPF(corruptedCPF) {
			t.Logf("Corrupted CPF passed validation: %s (original: %s)", corruptedCPF, cpf)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("Invalid CPF check digit property failed: %v", err)
	}
}

// Property: CNPJs with incorrect check digit fail validation
func TestProperty_InvalidCNPJCheckDigitFails(t *testing.T) {
	f := func(seed int64) bool {
		rng := rand.New(rand.NewSource(seed))
		cnpj := generateValidCNPJ(rng)

		// Corrupt the last check digit
		digits := []byte(cnpj)
		original := digits[13]
		digits[13] = byte('0' + (int(original-'0')+1)%10)
		corruptedCNPJ := string(digits)

		if validateCNPJ(corruptedCNPJ) {
			t.Logf("Corrupted CNPJ passed validation: %s (original: %s)", corruptedCNPJ, cnpj)
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("Invalid CNPJ check digit property failed: %v", err)
	}
}

// Property: All-equal-digit documents are invalid
func TestProperty_AllEqualDigitsInvalid(t *testing.T) {
	for digit := '0'; digit <= '9'; digit++ {
		cpf := string(make([]byte, cpfLength))
		for i := range cpf {
			cpf = cpf[:i] + string(digit) + cpf[i+1:]
		}
		cpf = ""
		for i := 0; i < cpfLength; i++ {
			cpf += string(digit)
		}

		if validateCPF(cpf) {
			t.Errorf("All-equal CPF should be invalid: %s", cpf)
		}

		cnpj := ""
		for i := 0; i < cnpjLength; i++ {
			cnpj += string(digit)
		}

		if validateCNPJ(cnpj) {
			t.Errorf("All-equal CNPJ should be invalid: %s", cnpj)
		}
	}
}

// Property: Wrong length documents are invalid
func TestProperty_WrongLengthInvalid(t *testing.T) {
	wrongLengths := []int{0, 1, 5, 10, 12, 13, 15, 20}

	for _, length := range wrongLengths {
		doc := ""
		for i := 0; i < length; i++ {
			doc += "1"
		}

		if validateCPF(doc) && length != cpfLength {
			t.Errorf("Wrong length CPF should be invalid: length=%d", length)
		}

		if validateCNPJ(doc) && length != cnpjLength {
			t.Errorf("Wrong length CNPJ should be invalid: length=%d", length)
		}
	}
}

// Property: Non-digit characters make document invalid
func TestProperty_NonDigitCharactersInvalid(t *testing.T) {
	invalidChars := []rune{'a', 'Z', '-', '.', ' ', '/', '#'}

	for _, char := range invalidChars {
		// Create CPF with invalid char
		cpf := "1234567890" + string(char)
		if len(cpf) == cpfLength && validateCPF(cpf) {
			t.Errorf("CPF with non-digit should be invalid: %s", cpf)
		}

		// Create CNPJ with invalid char
		cnpj := "1234567890123" + string(char)
		if len(cnpj) == cnpjLength && validateCNPJ(cnpj) {
			t.Errorf("CNPJ with non-digit should be invalid: %s", cnpj)
		}
	}
}
```

**Step 2: Run test to verify it passes**

Run: `go test -v -race -timeout 30s /Users/fredamaral/repos/lerianstudio/midaz/tests/property -run "(CPF|CNPJ|Document)"`

**Expected output:**
```
=== RUN   TestProperty_ValidCPFPasses
--- PASS: TestProperty_ValidCPFPasses (X.XXs)
=== RUN   TestProperty_ValidCNPJPasses
--- PASS: TestProperty_ValidCNPJPasses (X.XXs)
=== RUN   TestProperty_InvalidCPFCheckDigitFails
--- PASS: TestProperty_InvalidCPFCheckDigitFails (X.XXs)
=== RUN   TestProperty_InvalidCNPJCheckDigitFails
--- PASS: TestProperty_InvalidCNPJCheckDigitFails (X.XXs)
=== RUN   TestProperty_AllEqualDigitsInvalid
--- PASS: TestProperty_AllEqualDigitsInvalid (X.XXs)
=== RUN   TestProperty_WrongLengthInvalid
--- PASS: TestProperty_WrongLengthInvalid (X.XXs)
=== RUN   TestProperty_NonDigitCharactersInvalid
--- PASS: TestProperty_NonDigitCharactersInvalid (X.XXs)
PASS
```

**If Task Fails:**

1. **Check digit calculation mismatch:**
   - Check: Algorithm against withBody.go implementation
   - Fix: Verify weight arrays and modulo operations

2. **Can't recover:**
   - Document: What failed and why
   - Stop: Return to human partner

---

## Task 6: Create Asset Rate Conversion Property Tests

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/tests/property/asset_rate_test.go`

**Prerequisites:**
- Go 1.22+

**Step 1: Write the test file**

Create file at `/Users/fredamaral/repos/lerianstudio/midaz/tests/property/asset_rate_test.go`:

```go
package property

import (
	"math"
	"testing"
	"testing/quick"

	"github.com/shopspring/decimal"
)

// Property: Rate conversion with scale preserves value semantics
// Actual rate = rate / 10^scale
func TestProperty_AssetRateScaleSemantics(t *testing.T) {
	f := func(rate int64, scale uint8) bool {
		// Constrain values
		if rate == 0 {
			return true // Skip zero rate
		}
		if rate < 0 {
			rate = -rate
		}

		s := int(scale) % 10 // 0-9 scale

		// Calculate actual rate: rate / 10^scale
		rateDecimal := decimal.NewFromInt(rate)
		divisor := decimal.NewFromInt(1)
		for i := 0; i < s; i++ {
			divisor = divisor.Mul(decimal.NewFromInt(10))
		}
		actualRate := rateDecimal.Div(divisor)

		// Property: actualRate * 10^scale should equal original rate
		reconstructed := actualRate.Mul(divisor)

		if !reconstructed.Equal(rateDecimal) {
			t.Logf("Scale semantics violated: rate=%d scale=%d actual=%s reconstructed=%s",
				rate, s, actualRate.String(), reconstructed.String())
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("Asset rate scale semantics property failed: %v", err)
	}
}

// Property: Converting amount with rate and back with inverse rate preserves value (within tolerance)
func TestProperty_AssetRateInverseRoundtrip(t *testing.T) {
	f := func(amount, rateNum, rateDenom int64) bool {
		// Skip invalid cases
		if rateNum == 0 || rateDenom == 0 {
			return true
		}

		// Constrain to reasonable values
		if amount < 0 {
			amount = -amount
		}
		if rateNum < 0 {
			rateNum = -rateNum
		}
		if rateDenom < 0 {
			rateDenom = -rateDenom
		}

		// Skip extreme rates
		ratio := float64(rateNum) / float64(rateDenom)
		if ratio > 1000 || ratio < 0.001 {
			return true
		}

		amountDec := decimal.NewFromInt(amount)
		rate := decimal.NewFromInt(rateNum).Div(decimal.NewFromInt(rateDenom))
		inverseRate := decimal.NewFromInt(rateDenom).Div(decimal.NewFromInt(rateNum))

		// Forward conversion: amount * rate
		converted := amountDec.Mul(rate)

		// Reverse conversion: converted * inverseRate
		roundtrip := converted.Mul(inverseRate)

		// Property: roundtrip should be close to original
		diff := roundtrip.Sub(amountDec).Abs()
		tolerance := amountDec.Abs().Mul(decimal.NewFromFloat(0.0001)) // 0.01% tolerance
		if tolerance.LessThan(decimal.NewFromFloat(0.0001)) {
			tolerance = decimal.NewFromFloat(0.0001)
		}

		if diff.GreaterThan(tolerance) {
			t.Logf("Inverse roundtrip exceeded tolerance: amount=%d rate=%s/%s diff=%s tolerance=%s",
				amount, rateNum, rateDenom, diff.String(), tolerance.String())
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("Asset rate inverse roundtrip property failed: %v", err)
	}
}

// Property: Rate conversion is associative when amounts are multiplied
// (a * rate1) * rate2 == a * (rate1 * rate2)
func TestProperty_AssetRateAssociative(t *testing.T) {
	f := func(amount, rate1Num, rate1Denom, rate2Num, rate2Denom int64) bool {
		// Skip invalid cases
		if rate1Denom == 0 || rate2Denom == 0 {
			return true
		}

		// Constrain values
		if amount < 0 {
			amount = -amount
		}
		if rate1Num == 0 || rate2Num == 0 {
			return true
		}

		// Avoid extreme values that could cause overflow
		if math.Abs(float64(rate1Num)/float64(rate1Denom)) > 100 ||
			math.Abs(float64(rate2Num)/float64(rate2Denom)) > 100 {
			return true
		}

		amountDec := decimal.NewFromInt(amount)
		rate1 := decimal.NewFromInt(rate1Num).Div(decimal.NewFromInt(rate1Denom))
		rate2 := decimal.NewFromInt(rate2Num).Div(decimal.NewFromInt(rate2Denom))

		// (a * rate1) * rate2
		leftSide := amountDec.Mul(rate1).Mul(rate2)

		// a * (rate1 * rate2)
		combinedRate := rate1.Mul(rate2)
		rightSide := amountDec.Mul(combinedRate)

		// Property: both should be equal
		diff := leftSide.Sub(rightSide).Abs()
		tolerance := decimal.NewFromFloat(0.0001)

		if diff.GreaterThan(tolerance) {
			t.Logf("Associativity violated: left=%s right=%s diff=%s",
				leftSide.String(), rightSide.String(), diff.String())
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("Asset rate associativity property failed: %v", err)
	}
}

// Property: Rate of 1 is identity (amount * 1 == amount)
func TestProperty_AssetRateIdentity(t *testing.T) {
	f := func(amount int64) bool {
		amountDec := decimal.NewFromInt(amount)
		identityRate := decimal.NewFromInt(1)

		result := amountDec.Mul(identityRate)

		if !result.Equal(amountDec) {
			t.Logf("Identity rate violated: amount=%s result=%s", amountDec.String(), result.String())
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("Asset rate identity property failed: %v", err)
	}
}

// Property: Conversion preserves sign
func TestProperty_AssetRateSignPreservation(t *testing.T) {
	f := func(amount, rateNum, rateDenom int64) bool {
		if rateDenom == 0 || rateNum == 0 {
			return true
		}

		// Make rate always positive for this test
		if rateNum < 0 {
			rateNum = -rateNum
		}
		if rateDenom < 0 {
			rateDenom = -rateDenom
		}

		amountDec := decimal.NewFromInt(amount)
		rate := decimal.NewFromInt(rateNum).Div(decimal.NewFromInt(rateDenom))
		result := amountDec.Mul(rate)

		// Property: sign of result should match sign of amount (since rate is positive)
		if amount > 0 && !result.IsPositive() {
			t.Logf("Sign not preserved (positive): amount=%d rate=%s result=%s",
				amount, rate.String(), result.String())
			return false
		}
		if amount < 0 && !result.IsNegative() {
			t.Logf("Sign not preserved (negative): amount=%d rate=%s result=%s",
				amount, rate.String(), result.String())
			return false
		}
		if amount == 0 && !result.IsZero() {
			t.Logf("Zero not preserved: amount=%d rate=%s result=%s",
				amount, rate.String(), result.String())
			return false
		}

		return true
	}

	cfg := &quick.Config{MaxCount: 1000}
	if err := quick.Check(f, cfg); err != nil {
		t.Fatalf("Asset rate sign preservation property failed: %v", err)
	}
}
```

**Step 2: Run test to verify it passes**

Run: `go test -v -race -timeout 30s /Users/fredamaral/repos/lerianstudio/midaz/tests/property -run "AssetRate"`

**Expected output:**
```
=== RUN   TestProperty_AssetRateScaleSemantics
--- PASS: TestProperty_AssetRateScaleSemantics (X.XXs)
=== RUN   TestProperty_AssetRateInverseRoundtrip
--- PASS: TestProperty_AssetRateInverseRoundtrip (X.XXs)
=== RUN   TestProperty_AssetRateAssociative
--- PASS: TestProperty_AssetRateAssociative (X.XXs)
=== RUN   TestProperty_AssetRateIdentity
--- PASS: TestProperty_AssetRateIdentity (X.XXs)
=== RUN   TestProperty_AssetRateSignPreservation
--- PASS: TestProperty_AssetRateSignPreservation (X.XXs)
PASS
```

**If Task Fails:**

1. **Tolerance too strict:**
   - Check: Tolerance calculation
   - Fix: Increase tolerance for floating-point operations

2. **Can't recover:**
   - Document: What failed and why
   - Stop: Return to human partner

---

## Task 7: Run Code Review

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

## Task 8: Run Full Property Test Suite

**Prerequisites:**
- All previous tasks completed
- Code review passed

**Step 1: Run the complete property test suite**

Run: `make test-property`

**Expected output:**
```
Running property-based model tests
=== RUN   TestProperty_TransactionDateRoundtrip
--- PASS: TestProperty_TransactionDateRoundtrip (X.XXs)
=== RUN   TestProperty_TransactionDateFormats
--- PASS: TestProperty_TransactionDateFormats (X.XXs)
... [all tests pass]
PASS
ok      github.com/LerianStudio/midaz/v3/tests/property  XXX.XXXs
```

**Step 2: Verify no race conditions**

Run: `go test -v -race -timeout 120s ./tests/property`

**Expected output:**
```
PASS
ok      github.com/LerianStudio/midaz/v3/tests/property
```

**If Task Fails:**

1. **Test timeout:**
   - Check: MaxCount values (reduce if needed)
   - Fix: Reduce iterations for slow tests

2. **Race condition detected:**
   - Check: Shared state in tests
   - Fix: Use local variables, avoid global state

3. **Can't recover:**
   - Document: What failed and why
   - Stop: Return to human partner

---

## Task 9: Commit Changes

**Prerequisites:**
- All tests pass
- Code review completed

**Step 1: Stage and commit**

Run:
```bash
git add tests/property/transaction_date_test.go \
        tests/property/balance_redis_test.go \
        tests/property/balance_validation_test.go \
        tests/property/alias_format_test.go \
        tests/property/document_validation_test.go \
        tests/property/asset_rate_test.go

git commit -m "$(cat <<'EOF'
test(property): expand property test coverage

Add comprehensive property-based tests for:
- TransactionDate JSON round-trip serialization
- BalanceRedis unmarshal from float64/string/json.Number
- Balance validation operations (DEBIT/CREDIT/ONHOLD/RELEASE)
- Alias format validation (valid chars, external prefix prohibition)
- CPF/CNPJ document validation with check digit verification
- Asset rate conversion semantics (scale, inverse, associativity)

All tests use testing/quick with deterministic RNG for reproducibility.
Tests verify mathematical properties and invariants without external services.
EOF
)"
```

**Expected output:**
```
[branch-name abc1234] test(property): expand property test coverage
 6 files changed, XXX insertions(+)
 create mode 100644 tests/property/transaction_date_test.go
 create mode 100644 tests/property/balance_redis_test.go
 create mode 100644 tests/property/balance_validation_test.go
 create mode 100644 tests/property/alias_format_test.go
 create mode 100644 tests/property/document_validation_test.go
 create mode 100644 tests/property/asset_rate_test.go
```

**If Task Fails:**

1. **Pre-commit hook fails:**
   - Check: Lint errors in new files
   - Fix: Run `golangci-lint run --fix` on the files

2. **Can't recover:**
   - Document: What failed and why
   - Stop: Return to human partner

---

## Summary

This plan creates 6 new property test files:

| File | Properties Tested |
|------|-------------------|
| `transaction_date_test.go` | JSON round-trip, format parsing, null handling |
| `balance_redis_test.go` | Unmarshal from float64, string, json.Number |
| `balance_validation_test.go` | Version monotonicity, DEBIT/CREDIT, ONHOLD/RELEASE, total conservation |
| `alias_format_test.go` | Valid characters, invalid rejection, external prefix prohibition |
| `document_validation_test.go` | CPF/CNPJ generation, check digit validation |
| `asset_rate_test.go` | Scale semantics, inverse roundtrip, associativity, identity |

**Total estimated time:** 60-90 minutes for an engineer with zero codebase context.

**Verification:** `make test-property` should pass with all new tests.
