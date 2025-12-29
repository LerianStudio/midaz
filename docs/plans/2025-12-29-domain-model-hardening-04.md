# Domain Model Hardening Implementation Plan

> **For Agents:** REQUIRED SUB-SKILL: Use executing-plans to implement this plan task-by-task.

**Goal:** Enforce business invariants at the core domain level - balance operations, share calculations, status transitions, and DSL parsing.

**Architecture:** Add ~20-25 defensive assertions throughout the domain layer to catch programming errors early. These assertions use the existing `pkg/assert` package and panic on invariant violations (programming errors, not user input errors). User validation remains via error returns.

**Tech Stack:**
- Go 1.23+
- `pkg/assert` package (existing)
- `github.com/shopspring/decimal` for financial calculations
- `stretchr/testify` for tests

**Global Prerequisites:**
- Environment: macOS/Linux with Go 1.23+
- Tools: Go toolchain installed
- Access: Write access to repository
- State: Working from branch `fix/fred-several-ones-dec-13-2025`

**Verification before starting:**
```bash
# Run ALL these commands and verify output:
go version          # Expected: go version go1.23+
git status          # Expected: on branch fix/fred-several-ones-dec-13-2025
cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./...  # Expected: no errors
```

## Historical Precedent

**Query:** "domain validation assertions invariants balance share"
**Index Status:** Empty (new project)

No historical data available. This is normal for new projects.
Proceeding with standard planning approach.

---

## Task 1: Add Share Sum Tracking Variable in CalculateTotal

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/transaction/validations.go:288-364`
- Test: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/transaction/validations_test.go`

**Prerequisites:**
- Go toolchain installed
- Repository cloned and on correct branch

**Step 1: Write the failing test**

Add this test to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/transaction/validations_test.go`:

```go
func TestCalculateTotal_ShareSumExceeds100_Panics(t *testing.T) {
	// Arrange: Create fromTos with shares exceeding 100%
	fromTos := []FromTo{
		{
			AccountAlias: "@account1",
			IsFrom:       true,
			Share:        &Share{Percentage: 60, PercentageOfPercentage: 0},
		},
		{
			AccountAlias: "@account2",
			IsFrom:       true,
			Share:        &Share{Percentage: 50, PercentageOfPercentage: 0}, // Total: 110%
		},
	}
	transaction := Transaction{
		Send: Send{
			Asset: "USD",
			Value: decimal.NewFromInt(1000),
		},
	}

	// Act & Assert: Should panic because shares exceed 100%
	defer func() {
		r := recover()
		if r == nil {
			t.Errorf("Expected panic for shares exceeding 100%%, got none")
		}
		panicMsg := fmt.Sprintf("%v", r)
		if !strings.Contains(panicMsg, "total share percentages cannot exceed 100") {
			t.Errorf("Expected panic about share percentages, got: %v", r)
		}
	}()

	CalculateTotal(fromTos, transaction, "CREATED")
}
```

**Step 2: Add required imports to test file**

Ensure these imports exist at the top of `/Users/fredamaral/repos/lerianstudio/midaz/pkg/transaction/validations_test.go`:

```go
import (
	"fmt"
	"strings"
	// ... existing imports
)
```

**Step 3: Run test to verify it fails**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./pkg/transaction/... -run TestCalculateTotal_ShareSumExceeds100_Panics`

**Expected output:**
```
--- FAIL: TestCalculateTotal_ShareSumExceeds100_Panics
    Expected panic for shares exceeding 100%, got none
```

**Step 4: Add share sum assertion to CalculateTotal**

Modify `/Users/fredamaral/repos/lerianstudio/midaz/pkg/transaction/validations.go`. Find the `CalculateTotal` function (around line 288) and add the assertion:

```go
// CalculateTotal Calculate total for sources/destinations based on shares, amounts and remains
func CalculateTotal(fromTos []FromTo, transaction Transaction, transactionType string) (
	total decimal.Decimal,
	amounts map[string]Amount,
	aliases []string,
	operationRoutes map[string]string,
) {
	amounts = make(map[string]Amount)
	aliases = make([]string, 0)
	operationRoutes = make(map[string]string)

	total = decimal.NewFromInt(0)

	remaining := Amount{
		Asset:           transaction.Send.Asset,
		Value:           transaction.Send.Value,
		TransactionType: transactionType,
	}

	// Track total share percentage for validation
	var totalSharePercentage int64 = 0

	for i := range fromTos {
		operationRoutes[fromTos[i].AccountAlias] = fromTos[i].Route

		operation := DetermineOperation(transaction.Pending, fromTos[i].IsFrom, transactionType)

		if fromTos[i].Share != nil && fromTos[i].Share.Percentage != 0 {
			// Accumulate share percentages
			totalSharePercentage += fromTos[i].Share.Percentage

			oneHundred := decimal.NewFromInt(percentageMultiplier)

			percentage := decimal.NewFromInt(fromTos[i].Share.Percentage)

			percentageOfPercentage := decimal.NewFromInt(fromTos[i].Share.PercentageOfPercentage)
			if percentageOfPercentage.IsZero() {
				percentageOfPercentage = oneHundred
			}

			firstPart := percentage.Div(oneHundred)
			secondPart := percentageOfPercentage.Div(oneHundred)
			shareValue := transaction.Send.Value.Mul(firstPart).Mul(secondPart)

			amounts[fromTos[i].AccountAlias] = Amount{
				Asset:           transaction.Send.Asset,
				Value:           shareValue,
				Operation:       operation,
				TransactionType: transactionType,
			}

			total = total.Add(shareValue)
			remaining.Value = remaining.Value.Sub(shareValue)

			// Assert remaining never goes negative during distribution
			assert.That(assert.NonNegativeDecimal(remaining.Value),
				"remaining value cannot go negative during distribution",
				"index", i,
				"remaining", remaining.Value.String(),
				"accountAlias", fromTos[i].AccountAlias)
		}

		if fromTos[i].Amount != nil && fromTos[i].Amount.Value.IsPositive() {
			amount := Amount{
				Asset:           fromTos[i].Amount.Asset,
				Value:           fromTos[i].Amount.Value,
				Operation:       operation,
				TransactionType: transactionType,
			}

			amounts[fromTos[i].AccountAlias] = amount
			total = total.Add(amount.Value)

			remaining.Value = remaining.Value.Sub(amount.Value)
		}

		if !commons.IsNilOrEmpty(&fromTos[i].Remaining) {
			total = total.Add(remaining.Value)

			remaining.Operation = operation

			amounts[fromTos[i].AccountAlias] = remaining
			fromTos[i].Amount = &remaining
		}

		aliases = append(aliases, AliasKey(fromTos[i].SplitAlias(), fromTos[i].BalanceKey))
	}

	// Assert total shares don't exceed 100%
	assert.That(totalSharePercentage <= 100,
		"total share percentages cannot exceed 100",
		"total_percentage", totalSharePercentage)

	return total, amounts, aliases, operationRoutes
}
```

**Step 5: Run test to verify it passes**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./pkg/transaction/... -run TestCalculateTotal_ShareSumExceeds100_Panics`

**Expected output:**
```
--- PASS: TestCalculateTotal_ShareSumExceeds100_Panics
```

**Step 6: Commit**

```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && git add pkg/transaction/validations.go pkg/transaction/validations_test.go && git commit -m "$(cat <<'EOF'
feat(transaction): add share sum validation assertion in CalculateTotal

Add assertions to enforce that total share percentages cannot exceed 100%
and remaining value cannot go negative during distribution. These are
programming error checks, not user input validation.
EOF
)"
```

**If Task Fails:**

1. **Test won't compile:**
   - Check: Import statements for `fmt`, `strings` present
   - Fix: Add missing imports
   - Rollback: `git checkout -- pkg/transaction/`

2. **Assertion import missing:**
   - Check: `"github.com/LerianStudio/midaz/v3/pkg/assert"` in imports
   - Fix: It's already imported in validations.go

---

## Task 2: Add OnHold Non-Negativity Assertion in applyCanceledOperation

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/transaction/validations.go:163-170`
- Test: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/transaction/validations_test.go`

**Prerequisites:**
- Task 1 completed
- Go toolchain installed

**Step 1: Write the failing test**

Add this test to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/transaction/validations_test.go`:

```go
func TestApplyCanceledOperation_NegativeOnHold_Panics(t *testing.T) {
	// Arrange: Try to release more than what's on hold
	amount := Amount{
		Value:     decimal.NewFromInt(500),
		Operation: constant.RELEASE,
	}
	available := decimal.NewFromInt(1000)
	onHold := decimal.NewFromInt(100) // Less than release amount

	// Act & Assert: Should panic because onHold would go negative
	defer func() {
		r := recover()
		if r == nil {
			t.Errorf("Expected panic for negative onHold, got none")
		}
		panicMsg := fmt.Sprintf("%v", r)
		if !strings.Contains(panicMsg, "onHold cannot go negative during RELEASE") {
			t.Errorf("Expected panic about negative onHold, got: %v", r)
		}
	}()

	applyCanceledOperation(amount, available, onHold)
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./pkg/transaction/... -run TestApplyCanceledOperation_NegativeOnHold_Panics`

**Expected output:**
```
--- FAIL: TestApplyCanceledOperation_NegativeOnHold_Panics
    Expected panic for negative onHold, got none
```

**Step 3: Add assertion to applyCanceledOperation**

Modify `/Users/fredamaral/repos/lerianstudio/midaz/pkg/transaction/validations.go`, find `applyCanceledOperation` (around line 163):

Replace the function with:

```go
// applyCanceledOperation applies canceled transaction operations
func applyCanceledOperation(amount Amount, available, onHold decimal.Decimal) (decimal.Decimal, decimal.Decimal, bool) {
	if amount.Operation == constant.RELEASE {
		newOnHold := onHold.Sub(amount.Value)
		// OnHold can never be negative - this indicates a programming error
		// (trying to release more than was held)
		assert.That(assert.NonNegativeDecimal(newOnHold),
			"onHold cannot go negative during RELEASE",
			"original_onHold", onHold.String(),
			"release_amount", amount.Value.String(),
			"result", newOnHold.String())
		return available.Add(amount.Value), newOnHold, true
	}

	return available, onHold, false
}
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./pkg/transaction/... -run TestApplyCanceledOperation_NegativeOnHold_Panics`

**Expected output:**
```
--- PASS: TestApplyCanceledOperation_NegativeOnHold_Panics
```

**Step 5: Commit**

```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && git add pkg/transaction/validations.go pkg/transaction/validations_test.go && git commit -m "$(cat <<'EOF'
feat(transaction): add onHold non-negativity assertion in applyCanceledOperation

Assert that onHold balance cannot go negative during RELEASE operations.
This catches programming errors where release amount exceeds held amount.
EOF
)"
```

**If Task Fails:**

1. **Function not found:**
   - Check: Search for `func applyCanceledOperation` in validations.go
   - Fix: Verify line numbers, function may have moved

---

## Task 3: Add Decimal Scale Validation in OperateBalances

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/transaction/validations.go:223-243`
- Test: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/transaction/validations_test.go`

**Prerequisites:**
- Task 2 completed

**Step 1: Write the failing test**

Add this test to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/transaction/validations_test.go`:

```go
func TestOperateBalances_InvalidPrecision_Panics(t *testing.T) {
	// Arrange: Create amount with extreme precision (exponent < -18)
	// This creates a value with exponent -30 which is outside valid range
	extremeValue := decimal.NewFromFloat(0.000000000000000000000000000001)

	amount := Amount{
		Value:           extremeValue,
		Operation:       constant.DEBIT,
		TransactionType: constant.CREATED,
	}
	balance := Balance{
		Available: decimal.NewFromInt(1000),
		OnHold:    decimal.Zero,
		Version:   1,
	}

	// Act & Assert: Should panic because precision is invalid
	defer func() {
		r := recover()
		if r == nil {
			t.Errorf("Expected panic for invalid precision, got none")
		}
		panicMsg := fmt.Sprintf("%v", r)
		if !strings.Contains(panicMsg, "amount value has invalid precision") {
			t.Errorf("Expected panic about invalid precision, got: %v", r)
		}
	}()

	OperateBalances(amount, balance)
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./pkg/transaction/... -run TestOperateBalances_InvalidPrecision_Panics`

**Expected output:**
```
--- FAIL: TestOperateBalances_InvalidPrecision_Panics
    Expected panic for invalid precision, got none
```

**Step 3: Add precision validation to OperateBalances**

Modify `/Users/fredamaral/repos/lerianstudio/midaz/pkg/transaction/validations.go`, find `OperateBalances` (around line 223):

Replace the function with:

```go
// OperateBalances Function to sum or sub two balances and Normalize the scale
func OperateBalances(amount Amount, balance Balance) (Balance, error) {
	// Validate input precision - catches malformed amounts before processing
	assert.That(assert.ValidAmount(amount.Value),
		"amount value has invalid precision",
		"value", amount.Value.String(),
		"exponent", amount.Value.Exponent())

	total, totalOnHold, changed := applyBalanceOperation(amount, balance.Available, balance.OnHold)

	if !changed {
		// For no-op transactions (e.g., NOTED), return the original balance without changing the version.
		return balance, nil
	}

	// Validate output precision - ensures results are within valid bounds
	assert.That(assert.ValidAmount(total),
		"resulting available has invalid precision",
		"value", total.String(),
		"exponent", total.Exponent())
	assert.That(assert.ValidAmount(totalOnHold),
		"resulting onHold has invalid precision",
		"value", totalOnHold.String(),
		"exponent", totalOnHold.Exponent())

	newVersion := balance.Version + 1
	assert.That(assert.Positive(newVersion),
		"balance version must be positive after increment",
		"previousVersion", balance.Version,
		"newVersion", newVersion)

	return Balance{
		Available: total,
		OnHold:    totalOnHold,
		Version:   newVersion,
	}, nil
}
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./pkg/transaction/... -run TestOperateBalances_InvalidPrecision_Panics`

**Expected output:**
```
--- PASS: TestOperateBalances_InvalidPrecision_Panics
```

**Step 5: Commit**

```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && git add pkg/transaction/validations.go pkg/transaction/validations_test.go && git commit -m "$(cat <<'EOF'
feat(transaction): add decimal precision validation in OperateBalances

Validate that input and output amounts have valid precision (exponent
between -18 and 18) to prevent overflow and precision issues.
EOF
)"
```

**If Task Fails:**

1. **Exponent method not available:**
   - Check: `decimal.Decimal` has `.Exponent()` method
   - The shopspring/decimal library provides this

---

## Task 4: Add Exhaustive Switch in TransactionRevert

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/transaction.go:221-254`
- Test: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/transaction_test.go`

**Prerequisites:**
- Task 3 completed

**Step 1: Write the failing test**

First, check if the test file exists. If not, create it. Add this test:

```go
// Add to /Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/transaction_test.go

package mmodel

import (
	"fmt"
	"strings"
	"testing"

	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/shopspring/decimal"
)

func TestTransactionRevert_UnknownOperationType_Panics(t *testing.T) {
	// Arrange: Create transaction with unknown operation type
	amount := decimal.NewFromInt(100)
	transaction := Transaction{
		ID:        "123e4567-e89b-12d3-a456-426614174000",
		AssetCode: "USD",
		Amount:    &amount,
		Operations: []*Operation{
			{
				ID:           "op-1",
				Type:         "UNKNOWN_TYPE", // Invalid type
				AccountAlias: "@account1",
				AssetCode:    "USD",
				Amount:       OperationAmount{Value: &amount},
			},
		},
	}

	// Act & Assert: Should panic for unknown operation type
	defer func() {
		r := recover()
		if r == nil {
			t.Errorf("Expected panic for unknown operation type, got none")
		}
		panicMsg := fmt.Sprintf("%v", r)
		if !strings.Contains(panicMsg, "unhandled operation type in TransactionRevert") {
			t.Errorf("Expected panic about unhandled operation type, got: %v", r)
		}
	}()

	transaction.TransactionRevert()
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./pkg/mmodel/... -run TestTransactionRevert_UnknownOperationType_Panics`

**Expected output:**
```
--- FAIL: TestTransactionRevert_UnknownOperationType_Panics
    Expected panic for unknown operation type, got none
```

(Note: Currently unknown types are silently ignored)

**Step 3: Add exhaustive switch to TransactionRevert**

Modify `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/transaction.go`, find `TransactionRevert` (around line 216):

Replace the function with:

```go
// TransactionRevert is a func that revert transaction
func (t Transaction) TransactionRevert() pkgTransaction.Transaction {
	froms := make([]pkgTransaction.FromTo, 0)
	tos := make([]pkgTransaction.FromTo, 0)

	for _, op := range t.Operations {
		switch op.Type {
		case libConstant.CREDIT:
			from := pkgTransaction.FromTo{
				IsFrom:       true,
				AccountAlias: op.AccountAlias,
				Amount: &pkgTransaction.Amount{
					Asset: op.AssetCode,
					Value: *op.Amount.Value,
				},
				Description:     op.Description,
				ChartOfAccounts: op.ChartOfAccounts,
				Metadata:        op.Metadata,
				Route:           op.Route,
			}

			froms = append(froms, from)
		case libConstant.DEBIT:
			to := pkgTransaction.FromTo{
				IsFrom:       false,
				AccountAlias: op.AccountAlias,
				Amount: &pkgTransaction.Amount{
					Asset: op.AssetCode,
					Value: *op.Amount.Value,
				},
				Description:     op.Description,
				ChartOfAccounts: op.ChartOfAccounts,
				Metadata:        op.Metadata,
				Route:           op.Route,
			}

			tos = append(tos, to)
		default:
			// Unknown operation types indicate a programming error
			assert.Never("unhandled operation type in TransactionRevert",
				"operation_type", op.Type,
				"operation_id", op.ID,
				"transaction_id", t.ID)
		}
	}

	send := pkgTransaction.Send{
		Asset: t.AssetCode,
		Value: *t.Amount,
		Source: pkgTransaction.Source{
			From: froms,
		},
		Distribute: pkgTransaction.Distribute{
			To: tos,
		},
	}

	transaction := pkgTransaction.Transaction{
		ChartOfAccountsGroupName: t.ChartOfAccountsGroupName,
		Description:              t.Description,
		Pending:                  false,
		Metadata:                 t.Metadata,
		Route:                    t.Route,
		Send:                     send,
	}

	return transaction
}
```

**Step 4: Run test to verify it passes**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./pkg/mmodel/... -run TestTransactionRevert_UnknownOperationType_Panics`

**Expected output:**
```
--- PASS: TestTransactionRevert_UnknownOperationType_Panics
```

**Step 5: Commit**

```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && git add pkg/mmodel/transaction.go pkg/mmodel/transaction_test.go && git commit -m "$(cat <<'EOF'
feat(mmodel): add exhaustive switch assertion in TransactionRevert

Add assert.Never for default case to catch unknown operation types.
This ensures all operation types are explicitly handled.
EOF
)"
```

**If Task Fails:**

1. **Test file doesn't exist:**
   - Create: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/transaction_test.go`
   - Add package declaration and imports

---

## Task 5: Add Share Percentage Range Validation in DSL Parser

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/gold/transaction/parse.go:397-416`
- Test: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/gold/transaction/parse_test.go`

**Prerequisites:**
- Task 4 completed

**Step 1: Write the failing test**

Add this test to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/gold/transaction/parse_test.go`:

```go
func TestParse_SharePercentageOver100_Panics(t *testing.T) {
	// Arrange: DSL with share percentage > 100
	dsl := `(transaction V1 (chart-of-accounts-group-name FUNDING) (send USD 1000|2 (source (from @A :share 150%)) (distribute (to @B :remaining))))`

	// Act & Assert: Should panic for invalid percentage range
	defer func() {
		r := recover()
		if r == nil {
			t.Errorf("Expected panic for share percentage > 100, got none")
		}
		panicMsg := fmt.Sprintf("%v", r)
		if !strings.Contains(panicMsg, "share percentage must be between 0 and 100") {
			t.Errorf("Expected panic about share percentage range, got: %v", r)
		}
	}()

	Parse(dsl)
}

func TestParse_SharePercentageNegative_Panics(t *testing.T) {
	// Note: The lexer likely won't allow negative numbers, but we test the assertion anyway
	// This test documents the expected behavior
	t.Skip("Negative percentages are rejected by the lexer before reaching the assertion")
}
```

Also add the required imports:

```go
import (
	"fmt"
	"strings"
	"testing"
)
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./pkg/gold/transaction/... -run TestParse_SharePercentageOver100_Panics`

**Expected output:**
```
--- FAIL: TestParse_SharePercentageOver100_Panics
    Expected panic for share percentage > 100, got none
```

(Currently percentages over 100 are accepted)

**Step 3: Add range assertion to VisitShareInt**

Modify `/Users/fredamaral/repos/lerianstudio/midaz/pkg/gold/transaction/parse.go`.

First, add the assert import at the top:

```go
import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/gold/parser"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/antlr4-go/antlr/v4"
	"github.com/shopspring/decimal"
)
```

Then find `VisitShareInt` (around line 397) and replace:

```go
// VisitShareInt visits a share int context and builds a Share object with percentage.
func (v *TransactionVisitor) VisitShareInt(ctx *parser.ShareIntContext) any {
	if ctx == nil {
		v.setError(ErrShareContextNil)
		return pkgTransaction.Share{}
	}

	percentageStr := v.VisitNumericValue(numericValueContext(ctx.NumericValue())).(string)

	percentage, err := strconv.ParseInt(percentageStr, 10, 64)
	if err != nil {
		v.setError(fmt.Errorf("%w: %w", ErrInvalidSharePercentage, err))
		return pkgTransaction.Share{}
	}

	// Validate percentage is within valid range [0, 100]
	assert.That(assert.InRange(percentage, 0, 100),
		"share percentage must be between 0 and 100",
		"value", percentage)

	return pkgTransaction.Share{
		Percentage:             percentage,
		PercentageOfPercentage: 0,
	}
}
```

**Step 4: Also update VisitShareIntOfInt**

Find `VisitShareIntOfInt` (around line 418) and add the same validation:

```go
// VisitShareIntOfInt visits a share int of int context and builds a Share object with percentage and percentageOfPercentage.
func (v *TransactionVisitor) VisitShareIntOfInt(ctx *parser.ShareIntOfIntContext) any {
	if ctx == nil {
		v.setError(ErrShareContextNil)
		return pkgTransaction.Share{}
	}

	percentageStr := v.VisitNumericValue(numericValueContext(ctx.NumericValue(0))).(string)

	percentage, err := strconv.ParseInt(percentageStr, 10, 64)
	if err != nil {
		v.setError(fmt.Errorf("%w: %w", ErrInvalidSharePercentage, err))
		return pkgTransaction.Share{}
	}

	// Validate percentage is within valid range [0, 100]
	assert.That(assert.InRange(percentage, 0, 100),
		"share percentage must be between 0 and 100",
		"value", percentage)

	percentageOfPercentageStr := v.VisitNumericValue(numericValueContext(ctx.NumericValue(1))).(string)

	percentageOfPercentage, err := strconv.ParseInt(percentageOfPercentageStr, 10, 64)
	if err != nil {
		v.setError(fmt.Errorf("%w: %w", ErrInvalidSharePercentageOfPercent, err))
		return pkgTransaction.Share{}
	}

	// Validate percentageOfPercentage is within valid range [0, 100]
	assert.That(assert.InRange(percentageOfPercentage, 0, 100),
		"share percentageOfPercentage must be between 0 and 100",
		"value", percentageOfPercentage)

	return pkgTransaction.Share{
		Percentage:             percentage,
		PercentageOfPercentage: percentageOfPercentage,
	}
}
```

**Step 5: Run test to verify it passes**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./pkg/gold/transaction/... -run TestParse_SharePercentageOver100_Panics`

**Expected output:**
```
--- PASS: TestParse_SharePercentageOver100_Panics
```

**Step 6: Commit**

```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && git add pkg/gold/transaction/parse.go pkg/gold/transaction/parse_test.go && git commit -m "$(cat <<'EOF'
feat(gold): add share percentage range validation in DSL parser

Assert that share percentages and percentageOfPercentage values are
within valid range [0, 100]. Catches invalid DSL at parse time.
EOF
)"
```

**If Task Fails:**

1. **Import not found:**
   - Check: Import path is `"github.com/LerianStudio/midaz/v3/pkg/assert"`
   - Fix: Verify module path in go.mod

---

## Task 6: Add Post-Parse AST Invariants in Parse Function

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/gold/transaction/parse.go:597-628`
- Test: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/gold/transaction/parse_test.go`

**Prerequisites:**
- Task 5 completed

**Step 1: Write the failing test**

Add this test to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/gold/transaction/parse_test.go`:

```go
func TestParse_ValidDSL_ReturnsTransaction(t *testing.T) {
	// Arrange: Valid DSL
	dsl := `(transaction V1 (chart-of-accounts-group-name FUNDING) (send USD 1000|2 (source (from @A :amount USD 1000|2)) (distribute (to @B :amount USD 1000|2))))`

	// Act
	result := Parse(dsl)

	// Assert: Should return valid transaction
	if result == nil {
		t.Errorf("Expected valid transaction, got nil")
		return
	}

	tx, ok := result.(pkgTransaction.Transaction)
	if !ok {
		t.Errorf("Expected pkgTransaction.Transaction, got %T", result)
		return
	}

	if len(tx.Send.Source.From) == 0 {
		t.Errorf("Expected at least one source, got none")
	}
	if len(tx.Send.Distribute.To) == 0 {
		t.Errorf("Expected at least one destination, got none")
	}
}
```

Add the import at the top of the test file:

```go
import (
	"fmt"
	"strings"
	"testing"

	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
)
```

**Step 2: Run test to verify current behavior**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./pkg/gold/transaction/... -run TestParse_ValidDSL_ReturnsTransaction`

**Expected output:**
```
--- PASS: TestParse_ValidDSL_ReturnsTransaction
```

(This test should pass with current code)

**Step 3: Add helper function and post-parse assertions**

Modify `/Users/fredamaral/repos/lerianstudio/midaz/pkg/gold/transaction/parse.go`.

First, add the helper function at the end of the file (before the last closing brace):

```go
// truncateString truncates a string to maxLen characters for logging
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
```

Then modify the `Parse` function (around line 597):

```go
// Parse parses a transaction DSL string and returns a Transaction object or nil if parsing fails.
func Parse(dsl string) any {
	input := antlr.NewInputStream(dsl)
	lexer := parser.NewTransactionLexer(input)
	stream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)

	p := parser.NewTransactionParser(stream)
	lexerErrors := &Error{}
	parserErrors := &Error{}

	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(lexerErrors)
	p.RemoveErrorListeners()
	p.AddErrorListener(parserErrors)
	p.BuildParseTrees = true
	p.AddErrorListener(antlr.NewDiagnosticErrorListener(true))

	tree := p.Transaction()

	if len(lexerErrors.Errors) > 0 || len(parserErrors.Errors) > 0 {
		return nil
	}

	visitor := NewTransactionVisitor()
	transaction := visitor.Visit(tree)

	if visitor.err != nil {
		return nil
	}

	// Validate parsed transaction invariants
	if t, ok := transaction.(pkgTransaction.Transaction); ok {
		assert.That(len(t.Send.Source.From) > 0,
			"parsed transaction must have at least one source",
			"dsl_preview", truncateString(dsl, 100))
		assert.That(len(t.Send.Distribute.To) > 0,
			"parsed transaction must have at least one destination",
			"dsl_preview", truncateString(dsl, 100))
		assert.That(assert.PositiveDecimal(t.Send.Value),
			"send value must be positive",
			"value", t.Send.Value.String(),
			"dsl_preview", truncateString(dsl, 100))
	}

	return transaction
}
```

**Step 4: Write test for zero send value**

Add another test:

```go
func TestParse_ZeroSendValue_Panics(t *testing.T) {
	// Arrange: DSL with zero send value
	dsl := `(transaction V1 (chart-of-accounts-group-name FUNDING) (send USD 0|2 (source (from @A :amount USD 0|2)) (distribute (to @B :amount USD 0|2))))`

	// Act & Assert: Should panic for zero send value
	defer func() {
		r := recover()
		if r == nil {
			t.Errorf("Expected panic for zero send value, got none")
		}
		panicMsg := fmt.Sprintf("%v", r)
		if !strings.Contains(panicMsg, "send value must be positive") {
			t.Errorf("Expected panic about positive send value, got: %v", r)
		}
	}()

	Parse(dsl)
}
```

**Step 5: Run all parse tests**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./pkg/gold/transaction/... -run TestParse`

**Expected output:**
```
--- PASS: TestParse_ValidDSL_ReturnsTransaction
--- PASS: TestParse_ZeroSendValue_Panics
--- PASS: TestParse (other existing tests)
```

**Step 6: Commit**

```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && git add pkg/gold/transaction/parse.go pkg/gold/transaction/parse_test.go && git commit -m "$(cat <<'EOF'
feat(gold): add post-parse AST invariant assertions

Validate that parsed transactions have at least one source,
at least one destination, and a positive send value.
EOF
)"
```

**If Task Fails:**

1. **Type assertion fails:**
   - Check: The transaction type is `pkgTransaction.Transaction`
   - Verify import path

---

## Task 7: Create NewOperation Constructor with Validated Invariants

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/operation.go`
- Test: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/operation_test.go`

**Prerequisites:**
- Task 6 completed

**Step 1: Create test file and write failing test**

Create `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/operation_test.go`:

```go
package mmodel

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestNewOperation_ValidInputs_ReturnsOperation(t *testing.T) {
	// Arrange
	id := "123e4567-e89b-12d3-a456-426614174000"
	transactionID := "123e4567-e89b-12d3-a456-426614174001"
	opType := "DEBIT"
	assetCode := "USD"
	amountValue := decimal.NewFromInt(100)

	// Act
	op := NewOperation(id, transactionID, opType, assetCode, amountValue)

	// Assert
	if op == nil {
		t.Fatal("Expected non-nil operation")
	}
	if op.ID != id {
		t.Errorf("Expected ID %s, got %s", id, op.ID)
	}
	if op.TransactionID != transactionID {
		t.Errorf("Expected TransactionID %s, got %s", transactionID, op.TransactionID)
	}
	if op.Type != opType {
		t.Errorf("Expected Type %s, got %s", opType, op.Type)
	}
	if op.AssetCode != assetCode {
		t.Errorf("Expected AssetCode %s, got %s", assetCode, op.AssetCode)
	}
	if !op.Amount.Value.Equal(amountValue) {
		t.Errorf("Expected Amount %s, got %s", amountValue.String(), op.Amount.Value.String())
	}
	if op.CreatedAt.IsZero() {
		t.Error("Expected CreatedAt to be set")
	}
}

func TestNewOperation_InvalidID_Panics(t *testing.T) {
	// Arrange
	invalidID := "not-a-uuid"
	transactionID := "123e4567-e89b-12d3-a456-426614174001"

	// Act & Assert
	defer func() {
		r := recover()
		if r == nil {
			t.Error("Expected panic for invalid ID")
		}
		panicMsg := fmt.Sprintf("%v", r)
		if !strings.Contains(panicMsg, "operation ID must be valid UUID") {
			t.Errorf("Expected panic about invalid UUID, got: %v", r)
		}
	}()

	NewOperation(invalidID, transactionID, "DEBIT", "USD", decimal.NewFromInt(100))
}

func TestNewOperation_InvalidType_Panics(t *testing.T) {
	// Arrange
	id := "123e4567-e89b-12d3-a456-426614174000"
	transactionID := "123e4567-e89b-12d3-a456-426614174001"

	// Act & Assert
	defer func() {
		r := recover()
		if r == nil {
			t.Error("Expected panic for invalid type")
		}
		panicMsg := fmt.Sprintf("%v", r)
		if !strings.Contains(panicMsg, "operation type must be DEBIT or CREDIT") {
			t.Errorf("Expected panic about operation type, got: %v", r)
		}
	}()

	NewOperation(id, transactionID, "INVALID", "USD", decimal.NewFromInt(100))
}

func TestNewOperation_NegativeAmount_Panics(t *testing.T) {
	// Arrange
	id := "123e4567-e89b-12d3-a456-426614174000"
	transactionID := "123e4567-e89b-12d3-a456-426614174001"
	negativeAmount := decimal.NewFromInt(-100)

	// Act & Assert
	defer func() {
		r := recover()
		if r == nil {
			t.Error("Expected panic for negative amount")
		}
		panicMsg := fmt.Sprintf("%v", r)
		if !strings.Contains(panicMsg, "amount must be non-negative") {
			t.Errorf("Expected panic about non-negative amount, got: %v", r)
		}
	}()

	NewOperation(id, transactionID, "DEBIT", "USD", negativeAmount)
}

func TestNewOperation_EmptyAssetCode_Panics(t *testing.T) {
	// Arrange
	id := "123e4567-e89b-12d3-a456-426614174000"
	transactionID := "123e4567-e89b-12d3-a456-426614174001"

	// Act & Assert
	defer func() {
		r := recover()
		if r == nil {
			t.Error("Expected panic for empty asset code")
		}
		panicMsg := fmt.Sprintf("%v", r)
		if !strings.Contains(panicMsg, "assetCode must not be empty") {
			t.Errorf("Expected panic about empty asset code, got: %v", r)
		}
	}()

	NewOperation(id, transactionID, "DEBIT", "", decimal.NewFromInt(100))
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./pkg/mmodel/... -run TestNewOperation`

**Expected output:**
```
# github.com/LerianStudio/midaz/v3/pkg/mmodel [pkg/mmodel.test]
./operation_test.go:XX: undefined: NewOperation
```

**Step 3: Implement NewOperation constructor**

Add to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/operation.go`. First add imports:

```go
package mmodel

import (
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/shopspring/decimal"
)
```

Then add the constructor at the end of the file:

```go
// NewOperation creates a new Operation with validated invariants.
// Panics if any invariant is violated (programming errors).
func NewOperation(id, transactionID, opType, assetCode string, amountValue decimal.Decimal) *Operation {
	assert.That(assert.ValidUUID(id),
		"operation ID must be valid UUID",
		"id", id)
	assert.That(assert.ValidUUID(transactionID),
		"transaction ID must be valid UUID",
		"transactionID", transactionID)
	assert.That(opType == constant.DEBIT || opType == constant.CREDIT,
		"operation type must be DEBIT or CREDIT",
		"type", opType)
	assert.NotEmpty(assetCode,
		"assetCode must not be empty")
	assert.That(assert.NonNegativeDecimal(amountValue),
		"amount must be non-negative",
		"value", amountValue.String())

	now := time.Now()
	return &Operation{
		ID:            id,
		TransactionID: transactionID,
		Type:          opType,
		AssetCode:     assetCode,
		Amount:        OperationAmount{Value: &amountValue},
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}
```

**Step 4: Run tests to verify they pass**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./pkg/mmodel/... -run TestNewOperation`

**Expected output:**
```
--- PASS: TestNewOperation_ValidInputs_ReturnsOperation
--- PASS: TestNewOperation_InvalidID_Panics
--- PASS: TestNewOperation_InvalidType_Panics
--- PASS: TestNewOperation_NegativeAmount_Panics
--- PASS: TestNewOperation_EmptyAssetCode_Panics
```

**Step 5: Commit**

```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && git add pkg/mmodel/operation.go pkg/mmodel/operation_test.go && git commit -m "$(cat <<'EOF'
feat(mmodel): add NewOperation constructor with validated invariants

Create factory function that validates:
- Valid UUID for ID and TransactionID
- Operation type is DEBIT or CREDIT
- Non-empty asset code
- Non-negative amount value
EOF
)"
```

**If Task Fails:**

1. **Import conflict:**
   - Check: May need to alias constant import if there's a conflict
   - Fix: Use `pkgConstant "github.com/LerianStudio/midaz/v3/pkg/constant"`

---

## Task 8: Create Status Transition Helpers

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/constant/status_transitions.go`
- Test: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/constant/status_transitions_test.go`

**Prerequisites:**
- Task 7 completed

**Step 1: Create test file first**

Create `/Users/fredamaral/repos/lerianstudio/midaz/pkg/constant/status_transitions_test.go`:

```go
package constant

import (
	"fmt"
	"strings"
	"testing"
)

func TestAssertValidStatusCode_ValidCodes(t *testing.T) {
	validCodes := []string{CREATED, PENDING, APPROVED, CANCELED, NOTED}

	for _, code := range validCodes {
		t.Run(code, func(t *testing.T) {
			// Should not panic
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Unexpected panic for valid code %s: %v", code, r)
				}
			}()
			AssertValidStatusCode(code)
		})
	}
}

func TestAssertValidStatusCode_InvalidCode_Panics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Error("Expected panic for invalid status code")
		}
		panicMsg := fmt.Sprintf("%v", r)
		if !strings.Contains(panicMsg, "unknown transaction status code") {
			t.Errorf("Expected panic about unknown status code, got: %v", r)
		}
	}()

	AssertValidStatusCode("INVALID_STATUS")
}

func TestAssertValidStatusTransition_ValidTransitions(t *testing.T) {
	validTransitions := []struct {
		from string
		to   string
	}{
		{CREATED, PENDING},
		{CREATED, APPROVED},
		{CREATED, NOTED},
		{PENDING, APPROVED},
		{PENDING, CANCELED},
	}

	for _, tt := range validTransitions {
		t.Run(tt.from+"->"+tt.to, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Unexpected panic for valid transition %s->%s: %v", tt.from, tt.to, r)
				}
			}()
			AssertValidStatusTransition(tt.from, tt.to)
		})
	}
}

func TestAssertValidStatusTransition_InvalidTransition_Panics(t *testing.T) {
	invalidTransitions := []struct {
		from string
		to   string
	}{
		{APPROVED, PENDING},  // Terminal state
		{CANCELED, APPROVED}, // Terminal state
		{PENDING, CREATED},   // Backward transition
	}

	for _, tt := range invalidTransitions {
		t.Run(tt.from+"->"+tt.to, func(t *testing.T) {
			defer func() {
				r := recover()
				if r == nil {
					t.Errorf("Expected panic for invalid transition %s->%s", tt.from, tt.to)
				}
				panicMsg := fmt.Sprintf("%v", r)
				if !strings.Contains(panicMsg, "invalid status transition") {
					t.Errorf("Expected panic about invalid transition, got: %v", r)
				}
			}()
			AssertValidStatusTransition(tt.from, tt.to)
		})
	}
}

func TestIsTerminalStatus(t *testing.T) {
	tests := []struct {
		status   string
		expected bool
	}{
		{CREATED, false},
		{PENDING, false},
		{APPROVED, true},
		{CANCELED, true},
		{NOTED, true},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			result := IsTerminalStatus(tt.status)
			if result != tt.expected {
				t.Errorf("IsTerminalStatus(%s) = %v, want %v", tt.status, result, tt.expected)
			}
		})
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./pkg/constant/... -run TestAssertValidStatusCode`

**Expected output:**
```
# github.com/LerianStudio/midaz/v3/pkg/constant [pkg/constant.test]
./status_transitions_test.go:XX: undefined: AssertValidStatusCode
```

**Step 3: Create status_transitions.go**

Create `/Users/fredamaral/repos/lerianstudio/midaz/pkg/constant/status_transitions.go`:

```go
package constant

import "github.com/LerianStudio/midaz/v3/pkg/assert"

// ValidTransactionStatuses is the set of valid transaction status codes.
var ValidTransactionStatuses = map[string]bool{
	CREATED:  true,
	PENDING:  true,
	APPROVED: true,
	CANCELED: true,
	NOTED:    true,
}

// ValidStatusTransitions defines allowed status transitions.
// Terminal states (APPROVED, CANCELED, NOTED) have no valid transitions.
var ValidStatusTransitions = map[string][]string{
	CREATED:  {PENDING, APPROVED, NOTED},
	PENDING:  {APPROVED, CANCELED},
	APPROVED: {}, // Terminal
	CANCELED: {}, // Terminal
	NOTED:    {}, // Terminal
}

// TerminalStatuses are statuses that cannot transition to other statuses.
var TerminalStatuses = map[string]bool{
	APPROVED: true,
	CANCELED: true,
	NOTED:    true,
}

// AssertValidStatusCode panics if status code is unknown.
// Use for validating status codes from internal sources (programming errors).
func AssertValidStatusCode(code string) {
	assert.That(ValidTransactionStatuses[code],
		"unknown transaction status code",
		"code", code)
}

// AssertValidStatusTransition panics if transition is not allowed.
// Use for validating state machine transitions (programming errors).
func AssertValidStatusTransition(from, to string) {
	// First validate both codes
	AssertValidStatusCode(from)
	AssertValidStatusCode(to)

	allowed := ValidStatusTransitions[from]
	for _, s := range allowed {
		if s == to {
			return
		}
	}

	assert.Never("invalid status transition",
		"from", from,
		"to", to,
		"allowed", allowed)
}

// IsTerminalStatus returns true if the status is a terminal state
// (cannot transition to any other status).
func IsTerminalStatus(status string) bool {
	return TerminalStatuses[status]
}

// GetAllowedTransitions returns the list of statuses that can be transitioned
// to from the given status. Returns nil for terminal states.
func GetAllowedTransitions(status string) []string {
	return ValidStatusTransitions[status]
}
```

**Step 4: Run all tests to verify they pass**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./pkg/constant/...`

**Expected output:**
```
--- PASS: TestAssertValidStatusCode_ValidCodes
--- PASS: TestAssertValidStatusCode_InvalidCode_Panics
--- PASS: TestAssertValidStatusTransition_ValidTransitions
--- PASS: TestAssertValidStatusTransition_InvalidTransition_Panics
--- PASS: TestIsTerminalStatus
```

**Step 5: Commit**

```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && git add pkg/constant/status_transitions.go pkg/constant/status_transitions_test.go && git commit -m "$(cat <<'EOF'
feat(constant): add status transition validation helpers

Add functions to validate transaction status codes and transitions:
- AssertValidStatusCode: validates status is known
- AssertValidStatusTransition: validates transition is allowed
- IsTerminalStatus: checks if status is terminal
- GetAllowedTransitions: returns valid transitions from status
EOF
)"
```

**If Task Fails:**

1. **Circular import:**
   - Check: assert package doesn't import constant
   - This should not be an issue based on current structure

---

## Task 9: Run Code Review

**Prerequisites:**
- Tasks 1-8 completed

**Step 1: Dispatch all 3 reviewers in parallel**

**REQUIRED SUB-SKILL:** Use requesting-code-review

Run all reviewers simultaneously:
- code-reviewer
- business-logic-reviewer
- security-reviewer

Wait for all to complete.

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

**Step 3: Proceed only when:**
- Zero Critical/High/Medium issues remain
- All Low issues have TODO(review): comments added
- All Cosmetic issues have FIXME(nitpick): comments added

---

## Task 10: Run Full Test Suite and Verify

**Files:**
- All modified files from Tasks 1-8

**Prerequisites:**
- Task 9 (Code Review) completed

**Step 1: Run full test suite**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./pkg/transaction/... ./pkg/mmodel/... ./pkg/gold/transaction/... ./pkg/constant/...`

**Expected output:**
```
ok  	github.com/LerianStudio/midaz/v3/pkg/transaction
ok  	github.com/LerianStudio/midaz/v3/pkg/mmodel
ok  	github.com/LerianStudio/midaz/v3/pkg/gold/transaction
ok  	github.com/LerianStudio/midaz/v3/pkg/constant
```

**Step 2: Run linter**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && golangci-lint run ./pkg/transaction/... ./pkg/mmodel/... ./pkg/gold/transaction/... ./pkg/constant/...`

**Expected output:**
```
(no output = no issues)
```

**Step 3: Build the project**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./...`

**Expected output:**
```
(no output = successful build)
```

**Step 4: Final commit if any fixes were needed**

```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && git status
# If there are any uncommitted changes from fixes:
git add -A && git commit -m "$(cat <<'EOF'
fix: address code review findings and linting issues
EOF
)"
```

**If Task Fails:**

1. **Test failures:**
   - Check: Which test failed
   - Fix: Review the specific assertion and test setup

2. **Lint errors:**
   - Check: Error message details
   - Fix: Address specific lint issues

---

## Summary

**Total assertions added:** ~20-25

**Files modified:**
1. `/Users/fredamaral/repos/lerianstudio/midaz/pkg/transaction/validations.go` - Share sum validation, onHold non-negativity, decimal precision
2. `/Users/fredamaral/repos/lerianstudio/midaz/pkg/transaction/validations_test.go` - Tests for above
3. `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/transaction.go` - Exhaustive switch in TransactionRevert
4. `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/transaction_test.go` - Tests for above
5. `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/operation.go` - NewOperation constructor
6. `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/operation_test.go` - Tests for above
7. `/Users/fredamaral/repos/lerianstudio/midaz/pkg/gold/transaction/parse.go` - Share percentage range, post-parse AST invariants
8. `/Users/fredamaral/repos/lerianstudio/midaz/pkg/gold/transaction/parse_test.go` - Tests for above
9. `/Users/fredamaral/repos/lerianstudio/midaz/pkg/constant/status_transitions.go` - Status transition helpers (new file)
10. `/Users/fredamaral/repos/lerianstudio/midaz/pkg/constant/status_transitions_test.go` - Tests for above (new file)

**Key invariants enforced:**
- Share percentages cannot exceed 100%
- Remaining value cannot go negative during distribution
- OnHold balance cannot go negative during RELEASE
- Amount precision must be within valid bounds
- Operation types must be exhaustively handled
- Share percentages must be in [0, 100] range
- Parsed transactions must have sources and destinations
- Send value must be positive
- Operation constructor validates all inputs
- Status codes must be known
- Status transitions must be valid
