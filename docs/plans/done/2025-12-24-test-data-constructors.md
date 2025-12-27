# Test Data Constructor Functions Implementation Plan

> **For Agents:** REQUIRED SUB-SKILL: Use executing-plans to implement this plan task-by-task.

**Goal:** Add constructor functions (`NewTestAmount`, `NewTestDebitAmount`, `NewTestCreditAmount`, `NewTestResponses`, `NewTestBalance`) to enforce required fields in transaction test data, preventing panic-causing incomplete structs.

**Architecture:** These constructors belong in `pkg/transaction/testutil.go` as they provide test utilities for the `pkgTransaction.Amount`, `pkgTransaction.Responses`, and `pkgTransaction.Balance` structs defined in that package. The constructors will enforce that all required fields (especially `TransactionType` and `Operation`) are always set, preventing the `assert.Never` panic in `applyBalanceOperation`.

**Tech Stack:** Go 1.24.2+, testify, shopspring/decimal, lib-commons constants

**Global Prerequisites:**
- Environment: Go 1.24.2+ installed
- Tools: `go test`, `golangci-lint`
- Access: Read/write access to the Midaz repository
- State: Clean working tree on branch `fix/fred-several-ones-dec-13-2025`

**Verification before starting:**
```bash
# Run ALL these commands and verify output:
go version                    # Expected: go version go1.24.2+ (or higher)
git status                    # Expected: On branch fix/fred-several-ones-dec-13-2025
cd /Users/fredamaral/repos/lerianstudio/midaz && go mod verify  # Expected: all modules verified
```

---

## Batch 1: Create Test Utility File with Core Constructors

### Task 1: Create testutil.go with NewTestAmount constructor

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/transaction/testutil.go`

**Prerequisites:**
- Go compiler available
- Repository cloned

**Step 1: Create the test utility file with NewTestAmount**

Create file `/Users/fredamaral/repos/lerianstudio/midaz/pkg/transaction/testutil.go`:

```go
package transaction

import (
	constant "github.com/LerianStudio/lib-commons/v2/commons/constants"
	"github.com/shopspring/decimal"
)

// NewTestAmount creates a fully-initialized Amount struct for testing.
// This constructor ensures all required fields are set, preventing panics
// in applyBalanceOperation caused by missing TransactionType.
//
// Parameters:
//   - asset: The asset code (e.g., "USD", "BRL", "EUR")
//   - value: The decimal value of the amount
//   - operation: The operation type (constant.DEBIT, constant.CREDIT, constant.ONHOLD, constant.RELEASE)
//   - transactionType: The transaction type (constant.CREATED, constant.PENDING, constant.APPROVED, constant.CANCELED)
//
// Example:
//
//	amount := NewTestAmount("USD", decimal.NewFromInt(100), constant.DEBIT, constant.CREATED)
func NewTestAmount(asset string, value decimal.Decimal, operation, transactionType string) Amount {
	return Amount{
		Asset:           asset,
		Value:           value,
		Operation:       operation,
		TransactionType: transactionType,
	}
}

// NewTestDebitAmount creates a DEBIT Amount with CREATED transaction type.
// This is a convenience constructor for the most common debit scenario.
//
// Parameters:
//   - asset: The asset code (e.g., "USD", "BRL", "EUR")
//   - value: The decimal value of the amount
//
// Example:
//
//	amount := NewTestDebitAmount("USD", decimal.NewFromInt(100))
func NewTestDebitAmount(asset string, value decimal.Decimal) Amount {
	return Amount{
		Asset:           asset,
		Value:           value,
		Operation:       constant.DEBIT,
		TransactionType: constant.CREATED,
	}
}

// NewTestCreditAmount creates a CREDIT Amount with CREATED transaction type.
// This is a convenience constructor for the most common credit scenario.
//
// Parameters:
//   - asset: The asset code (e.g., "USD", "BRL", "EUR")
//   - value: The decimal value of the amount
//
// Example:
//
//	amount := NewTestCreditAmount("USD", decimal.NewFromInt(100))
func NewTestCreditAmount(asset string, value decimal.Decimal) Amount {
	return Amount{
		Asset:           asset,
		Value:           value,
		Operation:       constant.CREDIT,
		TransactionType: constant.CREATED,
	}
}
```

**Step 2: Verify the file compiles**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./pkg/transaction/...`

**Expected output:**
```
(no output - successful compilation)
```

**If you see errors:** Check import paths and ensure `lib-commons` dependency is available.

**Step 3: Commit the initial file**

```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && git add pkg/transaction/testutil.go && git commit -m "$(cat <<'EOF'
feat(transaction): add test data constructors for Amount struct

Add NewTestAmount, NewTestDebitAmount, and NewTestCreditAmount constructors
to enforce required fields in test data, preventing panics from incomplete
Amount structs missing TransactionType.
EOF
)"
```

**If Task Fails:**

1. **File won't compile:**
   - Check: Import paths are correct
   - Fix: Verify `lib-commons` is in go.mod
   - Rollback: `rm pkg/transaction/testutil.go`

2. **Import not found:**
   - Run: `go mod tidy`
   - Fix: Ensure dependency is in go.mod

---

### Task 2: Add NewTestResponses constructor

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/transaction/testutil.go`

**Prerequisites:**
- Task 1 completed
- testutil.go exists

**Step 1: Add NewTestResponses function**

Append to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/transaction/testutil.go`:

```go

// NewTestResponses creates a fully-initialized Responses struct for testing.
// This constructor ensures From and To maps are properly initialized.
//
// Parameters:
//   - from: Map of account aliases/keys to their debit Amounts
//   - to: Map of account aliases/keys to their credit Amounts
//
// Example:
//
//	responses := NewTestResponses(
//	    map[string]Amount{"@account1": NewTestDebitAmount("USD", decimal.NewFromInt(100))},
//	    map[string]Amount{"@account2": NewTestCreditAmount("USD", decimal.NewFromInt(100))},
//	)
func NewTestResponses(from, to map[string]Amount) *Responses {
	// Extract aliases from maps
	aliases := make([]string, 0, len(from)+len(to))
	sources := make([]string, 0, len(from))
	destinations := make([]string, 0, len(to))

	for k := range from {
		aliases = append(aliases, k)
		sources = append(sources, k)
	}

	for k := range to {
		aliases = append(aliases, k)
		destinations = append(destinations, k)
	}

	// Determine asset from first Amount (assumes all amounts use same asset)
	var asset string
	for _, v := range from {
		asset = v.Asset
		break
	}

	if asset == "" {
		for _, v := range to {
			asset = v.Asset
			break
		}
	}

	return &Responses{
		Asset:        asset,
		From:         from,
		To:           to,
		Aliases:      aliases,
		Sources:      sources,
		Destinations: destinations,
	}
}

// NewTestResponsesWithTotal creates a Responses struct with explicit total.
// Use this when you need to specify a total that differs from the sum of amounts.
//
// Parameters:
//   - total: The total transaction amount
//   - asset: The asset code
//   - from: Map of account aliases/keys to their debit Amounts
//   - to: Map of account aliases/keys to their credit Amounts
//
// Example:
//
//	responses := NewTestResponsesWithTotal(
//	    decimal.NewFromInt(100),
//	    "USD",
//	    map[string]Amount{"@account1": NewTestDebitAmount("USD", decimal.NewFromInt(100))},
//	    map[string]Amount{"@account2": NewTestCreditAmount("USD", decimal.NewFromInt(100))},
//	)
func NewTestResponsesWithTotal(total decimal.Decimal, asset string, from, to map[string]Amount) *Responses {
	resp := NewTestResponses(from, to)
	resp.Total = total
	resp.Asset = asset

	return resp
}
```

**Step 2: Verify the file compiles**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./pkg/transaction/...`

**Expected output:**
```
(no output - successful compilation)
```

**Step 3: Commit the addition**

```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && git add pkg/transaction/testutil.go && git commit -m "$(cat <<'EOF'
feat(transaction): add NewTestResponses constructor for test data

Add constructors for creating Responses structs with properly initialized
fields, reducing boilerplate in tests and ensuring consistency.
EOF
)"
```

**If Task Fails:**

1. **Compilation error:**
   - Check: `decimal.Decimal` import
   - Fix: Ensure shopspring/decimal import exists

---

### Task 3: Add NewTestBalance constructor

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/transaction/testutil.go`

**Prerequisites:**
- Task 2 completed

**Step 1: Add NewTestBalance function**

Append to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/transaction/testutil.go`:

```go

// NewTestBalance creates a fully-initialized Balance struct for testing.
// This constructor sets sensible defaults for testing scenarios.
//
// Parameters:
//   - id: The balance ID (UUID string)
//   - alias: The account alias (e.g., "@account1")
//   - assetCode: The asset code (e.g., "USD")
//   - available: The available balance amount
//
// Example:
//
//	balance := NewTestBalance(uuid.New().String(), "@account1", "USD", decimal.NewFromInt(1000))
func NewTestBalance(id, alias, assetCode string, available decimal.Decimal) *Balance {
	return &Balance{
		ID:             id,
		Alias:          alias,
		Key:            "default",
		AssetCode:      assetCode,
		Available:      available,
		OnHold:         decimal.Zero,
		Version:        1,
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
	}
}

// NewTestBalanceWithOrg creates a Balance with organization and ledger IDs.
// Use this when tests require full organizational context.
//
// Parameters:
//   - id: The balance ID (UUID string)
//   - organizationID: The organization UUID string
//   - ledgerID: The ledger UUID string
//   - accountID: The account UUID string
//   - alias: The account alias (e.g., "@account1")
//   - assetCode: The asset code (e.g., "USD")
//   - available: The available balance amount
//
// Example:
//
//	balance := NewTestBalanceWithOrg(
//	    uuid.New().String(),
//	    orgID.String(),
//	    ledgerID.String(),
//	    accountID.String(),
//	    "@account1",
//	    "USD",
//	    decimal.NewFromInt(1000),
//	)
func NewTestBalanceWithOrg(id, organizationID, ledgerID, accountID, alias, assetCode string, available decimal.Decimal) *Balance {
	balance := NewTestBalance(id, alias, assetCode, available)
	balance.OrganizationID = organizationID
	balance.LedgerID = ledgerID
	balance.AccountID = accountID

	return balance
}

// NewTestExternalBalance creates a Balance for an external account type.
// External accounts have special validation rules in the transaction system.
//
// Parameters:
//   - id: The balance ID (UUID string)
//   - alias: The account alias (e.g., "@external/BRL")
//   - assetCode: The asset code (e.g., "USD")
//
// Example:
//
//	balance := NewTestExternalBalance(uuid.New().String(), "@external/BRL", "BRL")
func NewTestExternalBalance(id, alias, assetCode string) *Balance {
	return &Balance{
		ID:             id,
		Alias:          alias,
		Key:            "default",
		AssetCode:      assetCode,
		Available:      decimal.Zero, // External accounts typically have zero or negative balance
		OnHold:         decimal.Zero,
		Version:        1,
		AccountType:    constant.ExternalAccountType,
		AllowSending:   true,
		AllowReceiving: true,
	}
}
```

**Step 2: Verify the file compiles**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./pkg/transaction/...`

**Expected output:**
```
(no output - successful compilation)
```

**Step 3: Commit the addition**

```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && git add pkg/transaction/testutil.go && git commit -m "$(cat <<'EOF'
feat(transaction): add NewTestBalance constructors for test data

Add constructors for Balance structs with sensible defaults,
including support for external accounts and full org context.
EOF
)"
```

---

### Task 4: Add pending transaction constructors

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/transaction/testutil.go`

**Prerequisites:**
- Task 3 completed

**Step 1: Add pending transaction Amount constructors**

Append to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/transaction/testutil.go`:

```go

// NewTestPendingDebitAmount creates a DEBIT Amount with PENDING transaction type.
// Use for testing pending/on-hold transactions.
//
// Example:
//
//	amount := NewTestPendingDebitAmount("USD", decimal.NewFromInt(100))
func NewTestPendingDebitAmount(asset string, value decimal.Decimal) Amount {
	return Amount{
		Asset:           asset,
		Value:           value,
		Operation:       constant.DEBIT,
		TransactionType: constant.PENDING,
	}
}

// NewTestPendingCreditAmount creates a CREDIT Amount with PENDING transaction type.
// Use for testing pending transactions.
//
// Example:
//
//	amount := NewTestPendingCreditAmount("USD", decimal.NewFromInt(100))
func NewTestPendingCreditAmount(asset string, value decimal.Decimal) Amount {
	return Amount{
		Asset:           asset,
		Value:           value,
		Operation:       constant.CREDIT,
		TransactionType: constant.PENDING,
	}
}

// NewTestOnHoldAmount creates an ONHOLD Amount for pending source transactions.
// The ONHOLD operation is used when a pending transaction holds funds.
//
// Example:
//
//	amount := NewTestOnHoldAmount("USD", decimal.NewFromInt(100))
func NewTestOnHoldAmount(asset string, value decimal.Decimal) Amount {
	return Amount{
		Asset:           asset,
		Value:           value,
		Operation:       constant.ONHOLD,
		TransactionType: constant.PENDING,
	}
}

// NewTestReleaseAmount creates a RELEASE Amount for canceled transactions.
// The RELEASE operation is used when releasing held funds.
//
// Example:
//
//	amount := NewTestReleaseAmount("USD", decimal.NewFromInt(100))
func NewTestReleaseAmount(asset string, value decimal.Decimal) Amount {
	return Amount{
		Asset:           asset,
		Value:           value,
		Operation:       constant.RELEASE,
		TransactionType: constant.CANCELED,
	}
}
```

**Step 2: Verify the file compiles**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./pkg/transaction/...`

**Expected output:**
```
(no output - successful compilation)
```

**Step 3: Commit the addition**

```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && git add pkg/transaction/testutil.go && git commit -m "$(cat <<'EOF'
feat(transaction): add pending transaction Amount constructors

Add constructors for PENDING, ONHOLD, and RELEASE operations
to support testing of pending/canceled transaction flows.
EOF
)"
```

---

### Task 5: Run Code Review (Batch 1)

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

3. **Proceed only when:**
   - Zero Critical/High/Medium issues remain
   - All Low issues have TODO(review): comments added
   - All Cosmetic issues have FIXME(nitpick): comments added

---

## Batch 2: Add Unit Tests for Constructors

### Task 6: Create testutil_test.go with Amount tests

**Files:**
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/transaction/testutil_test.go`

**Prerequisites:**
- Batch 1 completed

**Step 1: Create test file**

Create file `/Users/fredamaral/repos/lerianstudio/midaz/pkg/transaction/testutil_test.go`:

```go
package transaction

import (
	"testing"

	constant "github.com/LerianStudio/lib-commons/v2/commons/constants"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
)

func TestNewTestAmount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		asset           string
		value           decimal.Decimal
		operation       string
		transactionType string
	}{
		{
			name:            "debit created",
			asset:           "USD",
			value:           decimal.NewFromInt(100),
			operation:       constant.DEBIT,
			transactionType: constant.CREATED,
		},
		{
			name:            "credit created",
			asset:           "EUR",
			value:           decimal.NewFromFloat(50.5),
			operation:       constant.CREDIT,
			transactionType: constant.CREATED,
		},
		{
			name:            "onhold pending",
			asset:           "BRL",
			value:           decimal.NewFromInt(200),
			operation:       constant.ONHOLD,
			transactionType: constant.PENDING,
		},
		{
			name:            "release canceled",
			asset:           "GBP",
			value:           decimal.NewFromInt(75),
			operation:       constant.RELEASE,
			transactionType: constant.CANCELED,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			amount := NewTestAmount(tt.asset, tt.value, tt.operation, tt.transactionType)

			assert.Equal(t, tt.asset, amount.Asset)
			assert.True(t, tt.value.Equal(amount.Value))
			assert.Equal(t, tt.operation, amount.Operation)
			assert.Equal(t, tt.transactionType, amount.TransactionType)
		})
	}
}

func TestNewTestDebitAmount(t *testing.T) {
	t.Parallel()

	amount := NewTestDebitAmount("USD", decimal.NewFromInt(100))

	assert.Equal(t, "USD", amount.Asset)
	assert.True(t, decimal.NewFromInt(100).Equal(amount.Value))
	assert.Equal(t, constant.DEBIT, amount.Operation)
	assert.Equal(t, constant.CREATED, amount.TransactionType)
}

func TestNewTestCreditAmount(t *testing.T) {
	t.Parallel()

	amount := NewTestCreditAmount("EUR", decimal.NewFromInt(50))

	assert.Equal(t, "EUR", amount.Asset)
	assert.True(t, decimal.NewFromInt(50).Equal(amount.Value))
	assert.Equal(t, constant.CREDIT, amount.Operation)
	assert.Equal(t, constant.CREATED, amount.TransactionType)
}

func TestNewTestPendingDebitAmount(t *testing.T) {
	t.Parallel()

	amount := NewTestPendingDebitAmount("USD", decimal.NewFromInt(100))

	assert.Equal(t, "USD", amount.Asset)
	assert.True(t, decimal.NewFromInt(100).Equal(amount.Value))
	assert.Equal(t, constant.DEBIT, amount.Operation)
	assert.Equal(t, constant.PENDING, amount.TransactionType)
}

func TestNewTestPendingCreditAmount(t *testing.T) {
	t.Parallel()

	amount := NewTestPendingCreditAmount("EUR", decimal.NewFromInt(50))

	assert.Equal(t, "EUR", amount.Asset)
	assert.True(t, decimal.NewFromInt(50).Equal(amount.Value))
	assert.Equal(t, constant.CREDIT, amount.Operation)
	assert.Equal(t, constant.PENDING, amount.TransactionType)
}

func TestNewTestOnHoldAmount(t *testing.T) {
	t.Parallel()

	amount := NewTestOnHoldAmount("USD", decimal.NewFromInt(100))

	assert.Equal(t, "USD", amount.Asset)
	assert.True(t, decimal.NewFromInt(100).Equal(amount.Value))
	assert.Equal(t, constant.ONHOLD, amount.Operation)
	assert.Equal(t, constant.PENDING, amount.TransactionType)
}

func TestNewTestReleaseAmount(t *testing.T) {
	t.Parallel()

	amount := NewTestReleaseAmount("USD", decimal.NewFromInt(100))

	assert.Equal(t, "USD", amount.Asset)
	assert.True(t, decimal.NewFromInt(100).Equal(amount.Value))
	assert.Equal(t, constant.RELEASE, amount.Operation)
	assert.Equal(t, constant.CANCELED, amount.TransactionType)
}
```

**Step 2: Run the tests**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./pkg/transaction/... -run TestNewTest`

**Expected output:**
```
=== RUN   TestNewTestAmount
=== RUN   TestNewTestAmount/debit_created
=== RUN   TestNewTestAmount/credit_created
=== RUN   TestNewTestAmount/onhold_pending
=== RUN   TestNewTestAmount/release_canceled
--- PASS: TestNewTestAmount (0.00s)
    --- PASS: TestNewTestAmount/debit_created (0.00s)
    --- PASS: TestNewTestAmount/credit_created (0.00s)
    --- PASS: TestNewTestAmount/onhold_pending (0.00s)
    --- PASS: TestNewTestAmount/release_canceled (0.00s)
=== RUN   TestNewTestDebitAmount
--- PASS: TestNewTestDebitAmount (0.00s)
=== RUN   TestNewTestCreditAmount
--- PASS: TestNewTestCreditAmount (0.00s)
=== RUN   TestNewTestPendingDebitAmount
--- PASS: TestNewTestPendingDebitAmount (0.00s)
=== RUN   TestNewTestPendingCreditAmount
--- PASS: TestNewTestPendingCreditAmount (0.00s)
=== RUN   TestNewTestOnHoldAmount
--- PASS: TestNewTestOnHoldAmount (0.00s)
=== RUN   TestNewTestReleaseAmount
--- PASS: TestNewTestReleaseAmount (0.00s)
PASS
ok      github.com/LerianStudio/midaz/v3/pkg/transaction
```

**Step 3: Commit the tests**

```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && git add pkg/transaction/testutil_test.go && git commit -m "$(cat <<'EOF'
test(transaction): add unit tests for Amount constructors

Verify NewTestAmount, NewTestDebitAmount, NewTestCreditAmount,
and pending/release constructors set all fields correctly.
EOF
)"
```

---

### Task 7: Add Responses and Balance constructor tests

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/transaction/testutil_test.go`

**Prerequisites:**
- Task 6 completed

**Step 1: Add Responses and Balance tests**

Append to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/transaction/testutil_test.go`:

```go

func TestNewTestResponses(t *testing.T) {
	t.Parallel()

	from := map[string]Amount{
		"@account1": NewTestDebitAmount("USD", decimal.NewFromInt(100)),
	}
	to := map[string]Amount{
		"@account2": NewTestCreditAmount("USD", decimal.NewFromInt(100)),
	}

	responses := NewTestResponses(from, to)

	assert.NotNil(t, responses)
	assert.Equal(t, "USD", responses.Asset)
	assert.Len(t, responses.From, 1)
	assert.Len(t, responses.To, 1)
	assert.Contains(t, responses.Aliases, "@account1")
	assert.Contains(t, responses.Aliases, "@account2")
	assert.Contains(t, responses.Sources, "@account1")
	assert.Contains(t, responses.Destinations, "@account2")
}

func TestNewTestResponsesWithTotal(t *testing.T) {
	t.Parallel()

	from := map[string]Amount{
		"@account1": NewTestDebitAmount("USD", decimal.NewFromInt(100)),
	}
	to := map[string]Amount{
		"@account2": NewTestCreditAmount("USD", decimal.NewFromInt(100)),
	}

	responses := NewTestResponsesWithTotal(decimal.NewFromInt(100), "USD", from, to)

	assert.NotNil(t, responses)
	assert.True(t, decimal.NewFromInt(100).Equal(responses.Total))
	assert.Equal(t, "USD", responses.Asset)
}

func TestNewTestBalance(t *testing.T) {
	t.Parallel()

	balance := NewTestBalance("test-id", "@account1", "USD", decimal.NewFromInt(1000))

	assert.NotNil(t, balance)
	assert.Equal(t, "test-id", balance.ID)
	assert.Equal(t, "@account1", balance.Alias)
	assert.Equal(t, "default", balance.Key)
	assert.Equal(t, "USD", balance.AssetCode)
	assert.True(t, decimal.NewFromInt(1000).Equal(balance.Available))
	assert.True(t, decimal.Zero.Equal(balance.OnHold))
	assert.Equal(t, int64(1), balance.Version)
	assert.Equal(t, "deposit", balance.AccountType)
	assert.True(t, balance.AllowSending)
	assert.True(t, balance.AllowReceiving)
}

func TestNewTestBalanceWithOrg(t *testing.T) {
	t.Parallel()

	balance := NewTestBalanceWithOrg(
		"balance-id",
		"org-id",
		"ledger-id",
		"account-id",
		"@account1",
		"USD",
		decimal.NewFromInt(500),
	)

	assert.NotNil(t, balance)
	assert.Equal(t, "balance-id", balance.ID)
	assert.Equal(t, "org-id", balance.OrganizationID)
	assert.Equal(t, "ledger-id", balance.LedgerID)
	assert.Equal(t, "account-id", balance.AccountID)
	assert.Equal(t, "@account1", balance.Alias)
	assert.Equal(t, "USD", balance.AssetCode)
	assert.True(t, decimal.NewFromInt(500).Equal(balance.Available))
}

func TestNewTestExternalBalance(t *testing.T) {
	t.Parallel()

	balance := NewTestExternalBalance("ext-id", "@external/BRL", "BRL")

	assert.NotNil(t, balance)
	assert.Equal(t, "ext-id", balance.ID)
	assert.Equal(t, "@external/BRL", balance.Alias)
	assert.Equal(t, "BRL", balance.AssetCode)
	assert.True(t, decimal.Zero.Equal(balance.Available))
	assert.Equal(t, constant.ExternalAccountType, balance.AccountType)
}

// TestAmountWorksWithOperateBalances verifies that Amount structs created by
// constructors work correctly with the OperateBalances function.
func TestAmountWorksWithOperateBalances(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		amount            Amount
		initialAvailable  decimal.Decimal
		initialOnHold     decimal.Decimal
		expectedAvailable decimal.Decimal
		expectedOnHold    decimal.Decimal
	}{
		{
			name:              "debit created reduces available",
			amount:            NewTestDebitAmount("USD", decimal.NewFromInt(50)),
			initialAvailable:  decimal.NewFromInt(100),
			initialOnHold:     decimal.Zero,
			expectedAvailable: decimal.NewFromInt(50),
			expectedOnHold:    decimal.Zero,
		},
		{
			name:              "credit created increases available",
			amount:            NewTestCreditAmount("USD", decimal.NewFromInt(50)),
			initialAvailable:  decimal.NewFromInt(100),
			initialOnHold:     decimal.Zero,
			expectedAvailable: decimal.NewFromInt(150),
			expectedOnHold:    decimal.Zero,
		},
		{
			name:              "onhold pending moves to onHold",
			amount:            NewTestOnHoldAmount("USD", decimal.NewFromInt(30)),
			initialAvailable:  decimal.NewFromInt(100),
			initialOnHold:     decimal.Zero,
			expectedAvailable: decimal.NewFromInt(70),
			expectedOnHold:    decimal.NewFromInt(30),
		},
		{
			name:              "release canceled returns from onHold",
			amount:            NewTestReleaseAmount("USD", decimal.NewFromInt(30)),
			initialAvailable:  decimal.NewFromInt(70),
			initialOnHold:     decimal.NewFromInt(30),
			expectedAvailable: decimal.NewFromInt(100),
			expectedOnHold:    decimal.Zero,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			balance := Balance{
				Available: tt.initialAvailable,
				OnHold:    tt.initialOnHold,
				Version:   1,
			}

			result, err := OperateBalances(tt.amount, balance)

			assert.NoError(t, err)
			assert.True(t, tt.expectedAvailable.Equal(result.Available),
				"expected available %s, got %s", tt.expectedAvailable, result.Available)
			assert.True(t, tt.expectedOnHold.Equal(result.OnHold),
				"expected onHold %s, got %s", tt.expectedOnHold, result.OnHold)
		})
	}
}
```

**Step 2: Run all constructor tests**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./pkg/transaction/... -run TestNewTest`

**Expected output:**
```
=== RUN   TestNewTestAmount
--- PASS: TestNewTestAmount (0.00s)
...
=== RUN   TestNewTestResponses
--- PASS: TestNewTestResponses (0.00s)
=== RUN   TestNewTestResponsesWithTotal
--- PASS: TestNewTestResponsesWithTotal (0.00s)
=== RUN   TestNewTestBalance
--- PASS: TestNewTestBalance (0.00s)
=== RUN   TestNewTestBalanceWithOrg
--- PASS: TestNewTestBalanceWithOrg (0.00s)
=== RUN   TestNewTestExternalBalance
--- PASS: TestNewTestExternalBalance (0.00s)
PASS
```

**Step 3: Run integration test with OperateBalances**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./pkg/transaction/... -run TestAmountWorksWithOperateBalances`

**Expected output:**
```
=== RUN   TestAmountWorksWithOperateBalances
=== RUN   TestAmountWorksWithOperateBalances/debit_created_reduces_available
=== RUN   TestAmountWorksWithOperateBalances/credit_created_increases_available
=== RUN   TestAmountWorksWithOperateBalances/onhold_pending_moves_to_onHold
=== RUN   TestAmountWorksWithOperateBalances/release_canceled_returns_from_onHold
--- PASS: TestAmountWorksWithOperateBalances (0.00s)
PASS
```

**Step 4: Commit the tests**

```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && git add pkg/transaction/testutil_test.go && git commit -m "$(cat <<'EOF'
test(transaction): add tests for Responses and Balance constructors

Verify constructors set all fields correctly and that created
Amount structs work with OperateBalances without panics.
EOF
)"
```

---

### Task 8: Run Code Review (Batch 2)

1. **Dispatch all 3 reviewers in parallel:**
   - REQUIRED SUB-SKILL: Use requesting-code-review
   - All reviewers run simultaneously
   - Wait for all to complete

2. **Handle findings by severity (MANDATORY):**

**Critical/High/Medium Issues:**
- Fix immediately
- Re-run all 3 reviewers in parallel after fixes
- Repeat until zero Critical/High/Medium issues remain

**Low Issues:**
- Add `TODO(review):` comments in code

**Cosmetic/Nitpick Issues:**
- Add `FIXME(nitpick):` comments in code

---

## Batch 3: Update Existing Tests to Use Constructors

### Task 9: Update get-balances_test.go

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/get-balances_test.go`

**Prerequisites:**
- Batch 2 completed
- Constructors tested

**Step 1: Update Amount creation with missing TransactionType**

In `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/get-balances_test.go`, find and replace Amount struct literals that are missing `TransactionType`:

Find this pattern (around line 43-57):
```go
		fromAmount := pkgTransaction.Amount{
			Asset:     "USD",
			Value:     decimal.NewFromFloat(50),
			Operation: constant.DEBIT,
		}
		toAmount2 := pkgTransaction.Amount{
			Asset:     "EUR",
			Value:     decimal.NewFromFloat(40),
			Operation: constant.CREDIT,
		}
		toAmount3 := pkgTransaction.Amount{
			Asset:     "GBP",
			Value:     decimal.NewFromFloat(30),
			Operation: constant.CREDIT,
		}
```

Replace with:
```go
		fromAmount := pkgTransaction.NewTestDebitAmount("USD", decimal.NewFromFloat(50))
		toAmount2 := pkgTransaction.NewTestCreditAmount("EUR", decimal.NewFromFloat(40))
		toAmount3 := pkgTransaction.NewTestCreditAmount("GBP", decimal.NewFromFloat(30))
```

Continue with similar replacements for:
- Lines 194-203 (fromAmount and toAmount in "all balances from redis" test)
- Lines 324-328 (fromAmount in "lock balances successfully" test)

**Step 2: Verify tests pass**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./components/transaction/internal/services/query/... -run TestGetBalances`

**Expected output:**
```
=== RUN   TestGetBalances
=== RUN   TestGetBalances/get_balances_from_redis_and_database
=== RUN   TestGetBalances/all_balances_from_redis
--- PASS: TestGetBalances (0.00s)
PASS
```

**Step 3: Commit the update**

```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && git add components/transaction/internal/services/query/get-balances_test.go && git commit -m "$(cat <<'EOF'
refactor(transaction): use test constructors in get-balances_test

Replace manual Amount struct creation with NewTestDebitAmount
and NewTestCreditAmount to ensure TransactionType is always set.
EOF
)"
```

---

### Task 10: Update send-bto-execute-async_test.go

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/send-bto-execute-async_test.go`

**Prerequisites:**
- Task 9 completed

**Step 1: Update Amount creation**

In `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/send-bto-execute-async_test.go`, find the Amount struct literals (around line 56-68):

Find:
```go
	validate := &pkgTransaction.Responses{
		Aliases: []string{"alias1", "alias2"},
		From: map[string]pkgTransaction.Amount{
			"alias1": {
				Asset: "USD",
				Value: decimal.NewFromInt(50),
			},
		},
		To: map[string]pkgTransaction.Amount{
			"alias2": {
				Asset: "EUR",
				Value: decimal.NewFromInt(40),
			},
		},
	}
```

Replace with:
```go
	validate := pkgTransaction.NewTestResponses(
		map[string]pkgTransaction.Amount{
			"alias1": pkgTransaction.NewTestDebitAmount("USD", decimal.NewFromInt(50)),
		},
		map[string]pkgTransaction.Amount{
			"alias2": pkgTransaction.NewTestCreditAmount("EUR", decimal.NewFromInt(40)),
		},
	)
```

**Step 2: Verify tests pass**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./components/transaction/internal/services/command/... -run TestSendBTOExecuteAsync`

**Expected output:**
```
--- PASS: TestSendBTOExecuteAsync (0.00s)
PASS
```

**Step 3: Commit the update**

```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && git add components/transaction/internal/services/command/send-bto-execute-async_test.go && git commit -m "$(cat <<'EOF'
refactor(transaction): use test constructors in send-bto-execute-async_test

Replace manual Amount/Responses creation with constructors
to ensure required fields are always set.
EOF
)"
```

---

### Task 11: Update parity_test.go in pkg/gold/transaction

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/gold/transaction/parity_test.go`

**Prerequisites:**
- Task 10 completed

**Step 1: Review and update Amount creation**

In `/Users/fredamaral/repos/lerianstudio/midaz/pkg/gold/transaction/parity_test.go`, the Amount structs are used within FromTo.Amount pointers. Since these are pointer fields, we need to be careful with the update.

Find patterns like:
```go
Amount: &pkgTransaction.Amount{Asset: "USD", Value: decimal.RequireFromString("3")}
```

These can be updated to use constructors with pointer conversion:
```go
Amount: func() *pkgTransaction.Amount { a := pkgTransaction.NewTestDebitAmount("USD", decimal.RequireFromString("3")); return &a }()
```

Or create a helper for pointer amounts. Add to testutil.go first (Task 12), then update.

**Step 2: Verify tests pass**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./pkg/gold/transaction/... -run TestParity`

**Expected output:**
```
--- PASS: TestParity... (0.00s)
PASS
```

**Step 3: Commit the update**

```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && git add pkg/gold/transaction/parity_test.go && git commit -m "$(cat <<'EOF'
refactor(gold): use test constructors in parity_test

Ensure Amount structs have required fields set.
EOF
)"
```

---

### Task 12: Add pointer helper constructors

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/transaction/testutil.go`

**Prerequisites:**
- Task 11 identified need for pointer helpers

**Step 1: Add pointer helper functions**

Append to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/transaction/testutil.go`:

```go

// NewTestAmountPtr creates a pointer to a fully-initialized Amount struct.
// Use this when the Amount needs to be assigned to a pointer field.
//
// Example:
//
//	fromTo := FromTo{
//	    Amount: NewTestAmountPtr("USD", decimal.NewFromInt(100), constant.DEBIT, constant.CREATED),
//	}
func NewTestAmountPtr(asset string, value decimal.Decimal, operation, transactionType string) *Amount {
	amount := NewTestAmount(asset, value, operation, transactionType)
	return &amount
}

// NewTestDebitAmountPtr creates a pointer to a DEBIT Amount with CREATED transaction type.
//
// Example:
//
//	fromTo := FromTo{Amount: NewTestDebitAmountPtr("USD", decimal.NewFromInt(100))}
func NewTestDebitAmountPtr(asset string, value decimal.Decimal) *Amount {
	amount := NewTestDebitAmount(asset, value)
	return &amount
}

// NewTestCreditAmountPtr creates a pointer to a CREDIT Amount with CREATED transaction type.
//
// Example:
//
//	fromTo := FromTo{Amount: NewTestCreditAmountPtr("USD", decimal.NewFromInt(100))}
func NewTestCreditAmountPtr(asset string, value decimal.Decimal) *Amount {
	amount := NewTestCreditAmount(asset, value)
	return &amount
}
```

**Step 2: Verify the file compiles**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./pkg/transaction/...`

**Expected output:**
```
(no output - successful compilation)
```

**Step 3: Commit the addition**

```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && git add pkg/transaction/testutil.go && git commit -m "$(cat <<'EOF'
feat(transaction): add pointer helper constructors for Amount

Add NewTestAmountPtr, NewTestDebitAmountPtr, NewTestCreditAmountPtr
for use with FromTo.Amount pointer fields.
EOF
)"
```

---

### Task 13: Run Code Review (Batch 3)

1. **Dispatch all 3 reviewers in parallel:**
   - REQUIRED SUB-SKILL: Use requesting-code-review
   - All reviewers run simultaneously
   - Wait for all to complete

2. **Handle findings by severity (MANDATORY):**

**Critical/High/Medium Issues:**
- Fix immediately
- Re-run all 3 reviewers in parallel after fixes

**Low Issues:**
- Add `TODO(review):` comments

---

## Batch 4: Final Verification

### Task 14: Run full test suite for transaction package

**Files:**
- None (verification only)

**Prerequisites:**
- All previous tasks completed

**Step 1: Run all transaction package tests**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test -v ./pkg/transaction/...`

**Expected output:**
```
=== RUN   TestValidateBalancesRules
--- PASS: TestValidateBalancesRules (0.00s)
=== RUN   TestValidateFromBalances
--- PASS: TestValidateFromBalances (0.00s)
...
=== RUN   TestNewTestAmount
--- PASS: TestNewTestAmount (0.00s)
...
=== RUN   TestAmountWorksWithOperateBalances
--- PASS: TestAmountWorksWithOperateBalances (0.00s)
PASS
ok      github.com/LerianStudio/midaz/v3/pkg/transaction
```

**Step 2: Run all transaction component tests**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./components/transaction/...`

**Expected output:**
```
ok      github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command
ok      github.com/LerianStudio/midaz/v3/components/transaction/internal/services/query
...
```

**Step 3: Run linting**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && make lint`

**Expected output:**
```
(should pass without errors related to new files)
```

---

### Task 15: Final commit and summary

**Files:**
- None (documentation only)

**Prerequisites:**
- Task 14 verification passed

**Step 1: View git log for new commits**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && git log --oneline -10`

**Expected output:** Should show the commits from this plan

**Step 2: Create summary of changes**

The following files were created/modified:

**New files:**
- `/Users/fredamaral/repos/lerianstudio/midaz/pkg/transaction/testutil.go` - Test data constructors
- `/Users/fredamaral/repos/lerianstudio/midaz/pkg/transaction/testutil_test.go` - Constructor tests

**Modified files:**
- `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/get-balances_test.go`
- `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/send-bto-execute-async_test.go`
- `/Users/fredamaral/repos/lerianstudio/midaz/pkg/gold/transaction/parity_test.go`

**If Task Fails:**

1. **Tests fail:**
   - Run: `go test -v ./...` to identify failing tests
   - Check: Import paths are correct
   - Rollback: `git reset --hard HEAD~N` where N is number of commits to undo

---

## Summary of Constructors Created

| Constructor | Purpose | Default Operation | Default TransactionType |
|-------------|---------|-------------------|------------------------|
| `NewTestAmount` | Full control over all fields | Provided | Provided |
| `NewTestDebitAmount` | Quick debit amount | DEBIT | CREATED |
| `NewTestCreditAmount` | Quick credit amount | CREDIT | CREATED |
| `NewTestPendingDebitAmount` | Pending debit | DEBIT | PENDING |
| `NewTestPendingCreditAmount` | Pending credit | CREDIT | PENDING |
| `NewTestOnHoldAmount` | Hold funds | ONHOLD | PENDING |
| `NewTestReleaseAmount` | Release held funds | RELEASE | CANCELED |
| `NewTestAmountPtr` | Pointer to Amount | Provided | Provided |
| `NewTestDebitAmountPtr` | Pointer to debit | DEBIT | CREATED |
| `NewTestCreditAmountPtr` | Pointer to credit | CREDIT | CREATED |
| `NewTestResponses` | Create Responses struct | N/A | N/A |
| `NewTestResponsesWithTotal` | Responses with explicit total | N/A | N/A |
| `NewTestBalance` | Basic Balance | N/A | N/A |
| `NewTestBalanceWithOrg` | Balance with org context | N/A | N/A |
| `NewTestExternalBalance` | External account Balance | N/A | N/A |

---

## Batch 5: RabbitMQ Consumer Improvements (Review Findings)

> **Review Finding Source:** Code review conducted 2025-12-27

### Task 16: Add empty queue name validation in buildDLQName (Low)

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/rabbitmq/consumer.rabbitmq.go`

**Finding:** `buildDLQName()` returns ".dlq" for empty queue names, which may indicate a programming error.

**Step 1: Update buildDLQName function**

Find (around line 55-62):
```go
// TODO(review): Consider adding validation for empty queueName - returns ".dlq" which may indicate programming error (reported by code-reviewer and business-logic-reviewer on 2025-12-14, severity: Low)
func buildDLQName(queueName string) string {
	return queueName + dlqSuffix
}
```

Replace with:
```go
// buildDLQName creates the Dead Letter Queue name for a given queue.
// Panics if queueName is empty to catch programming errors early.
func buildDLQName(queueName string) string {
	assert.NotEmpty(queueName, "queueName must not be empty for DLQ routing")
	return queueName + dlqSuffix
}
```

**Step 2: Verify and commit**

```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./components/transaction/...
git add components/transaction/internal/adapters/rabbitmq/consumer.rabbitmq.go
git commit -m "fix(rabbitmq): add validation for empty queue name in buildDLQName"
```

---

### Task 17: Add context-aware sleep for graceful shutdown (Medium)

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/rabbitmq/consumer.rabbitmq.go`

**Finding:** Blocking `time.Sleep()` may delay graceful shutdown by up to 30s during backoff.

**Step 1: Add sleepWithContext helper function**

Add after `calculateRetryBackoff` function (around line 75):

```go
// sleepWithContext waits for the specified duration or until context is cancelled.
// Returns true if the sleep completed, false if context was cancelled.
func sleepWithContext(ctx context.Context, duration time.Duration) bool {
	if duration <= 0 {
		return true
	}

	select {
	case <-ctx.Done():
		return false
	case <-time.After(duration):
		return true
	}
}
```

**Step 2: Update republishWithRetry to use context-aware sleep**

Find in `republishWithRetry` method (around line 315-325):
```go
	// Apply backoff delay before republishing
	// TODO(review): Consider using context-aware sleep (select with ctx.Done()) to support
	// graceful shutdown during backoff. Current blocking sleep may delay shutdown by up to 30s.
	// (reported by security-reviewer on 2025-12-14, severity: Low)
	if backoffDelay > 0 {
		time.Sleep(backoffDelay)
	}
```

Replace with:
```go
	// Apply backoff delay before republishing with context awareness for graceful shutdown
	if backoffDelay > 0 {
		if !sleepWithContext(context.Background(), backoffDelay) {
			bec.logger.Warnf("Worker %d: backoff sleep interrupted by shutdown, proceeding with republish", bec.workerID)
		}
	}
```

**Step 3: Verify and commit**

```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./components/transaction/...
git add components/transaction/internal/adapters/rabbitmq/consumer.rabbitmq.go
git commit -m "fix(rabbitmq): use context-aware sleep for graceful shutdown during backoff"
```

---

### Task 18: Add retry.action span attribute for observability (Low)

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/rabbitmq/consumer.rabbitmq.go`

**Finding:** Span attribute `retry.backoff_seconds` is set even when not applied; need `retry.action` for clarity.

**Step 1: Update handleBusinessError to add retry.action attribute**

Find in `handleBusinessError` where span attributes are set and add action attribute.

For DLQ path (when `retryCount >= maxRetries-1`), add:
```go
span.SetAttributes(
	attribute.String("retry.action", "dlq"),
	attribute.Int("retry.total_attempts", bec.retryCount+1),
)
```

For retry path (in `republishWithRetry`), add:
```go
span.SetAttributes(
	attribute.String("retry.action", "retry"),
	attribute.Float64("retry.backoff_seconds", backoffDelay.Seconds()),
)
```

**Step 2: Verify and commit**

```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./components/transaction/...
git add components/transaction/internal/adapters/rabbitmq/consumer.rabbitmq.go
git commit -m "feat(rabbitmq): add retry.action span attribute for observability clarity"
```

---

## Batch 6: Operations Polling Improvements (Review Findings)

### Task 19: Add timeout indication to operations polling response (Medium)

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/operations_wait.go`

**Finding:** When timeout expires without finding operations, client receives empty results without indication that timeout occurred.

**Step 1: Add sentinel error for timeout**

Update imports and add error after imports:

```go
import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
)

// ErrOperationsWaitTimeout indicates that operations polling timed out without finding results.
// This is not necessarily an error - async operations may still be processing.
// Callers can check errors.Is(err, ErrOperationsWaitTimeout) to distinguish timeout from actual errors.
var ErrOperationsWaitTimeout = errors.New("operations wait timeout: async operations may still be processing")
```

**Step 2: Update waitForOperations to return timeout indication**

Replace the function:

```go
func waitForOperations(ctx context.Context, fetch func(context.Context) ([]*operation.Operation, libHTTP.CursorPagination, error)) ([]*operation.Operation, libHTTP.CursorPagination, error) {
	ops, cur, err := fetch(ctx)
	if err != nil || len(ops) > 0 || !asyncTransactionsEnabled() {
		return ops, cur, err
	}

	deadline := time.Now().Add(operationsWaitTimeout)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ops, cur, nil // Context cancelled - return current state without error
		case <-time.After(operationsWaitPollBackoff):
		}

		ops, cur, err = fetch(ctx)
		if err != nil || len(ops) > 0 {
			return ops, cur, err
		}
	}

	// Timeout reached - return sentinel error so caller can distinguish
	// between "no operations exist" and "operations may still be processing"
	return ops, cur, ErrOperationsWaitTimeout
}
```

**Step 3: Update callers to handle timeout gracefully**

Search for callers of `waitForOperations` and ensure they handle `ErrOperationsWaitTimeout`:

```go
ops, cur, err := waitForOperations(ctx, fetchFunc)
if err != nil && !errors.Is(err, ErrOperationsWaitTimeout) {
	// Real error - propagate it
	return nil, err
}
// ErrOperationsWaitTimeout is informational - log and continue with empty results
if errors.Is(err, ErrOperationsWaitTimeout) {
	logger.Warnf("Operations polling timed out after %v, returning current results", operationsWaitTimeout)
	err = nil // Clear the error since we're handling it gracefully
}
```

**Step 4: Verify and commit**

```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./components/transaction/internal/services/query/...
git add components/transaction/internal/services/query/
git commit -m "feat(transaction): add timeout indication to operations polling response"
```

---

## Batch 7: CRM Service Improvements (Review Findings)

### Task 20: Improve holder link deletion error handling (Medium)

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/services/delete-holder.go`

**Finding:** Sequential link deletion without transaction boundary may leave inconsistent state on partial failure.

**Step 1: Update DeleteHolderByID with better error aggregation**

Find the holder link deletion loop (around line 45-58):
```go
	for _, holderLink := range holderLinks {
		err = uc.HolderLinkRepo.Delete(ctx, organizationID, *holderLink.ID, hardDelete)
		if err != nil {
			libOpenTelemetry.HandleSpanError(&span, "Failed to delete holder link by id: %v", err)

			logger.Errorf("Failed to delete holder link by id: %v", err)

			return pkg.ValidateInternalError(err, "CRM")
		}
	}
```

Replace with:
```go
	// Delete all holder links - log all failures but return first error
	// Note: For better atomicity, consider implementing DeleteByHolderID in the repository layer
	var firstErr error
	var failedCount int
	for _, holderLink := range holderLinks {
		err = uc.HolderLinkRepo.Delete(ctx, organizationID, *holderLink.ID, hardDelete)
		if err != nil {
			failedCount++
			logger.Errorf("Failed to delete holder link %s: %v", holderLink.ID.String(), err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	if firstErr != nil {
		libOpenTelemetry.HandleSpanError(&span, "Failed to delete %d of %d holder links", failedCount, len(holderLinks))
		return pkg.ValidateInternalError(firstErr, "CRM")
	}
```

**Step 2: Verify and commit**

```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./components/crm/...
git add components/crm/internal/services/delete-holder.go
git commit -m "fix(crm): improve holder link deletion error handling and logging"
```

---

### Task 21: Document race condition in alias creation (Medium - Documentation)

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/services/create-alias.go`

**Finding:** Race condition window exists in alias+holder link creation but is correctly handled by rollback pattern. Needs documentation.

**Step 1: Add documentation comment to createAliasWithHolderLink**

Find (around line 116):
```go
func (uc *UseCase) createAliasWithHolderLink(ctx context.Context, span *trace.Span, logger loggerInterface, organizationID string, holderID uuid.UUID, cai *mmodel.CreateAliasInput, alias *mmodel.Alias, createdAccount *mmodel.Alias) (*mmodel.Alias, error) {
```

Replace with:
```go
// createAliasWithHolderLink creates a holder link after alias creation.
//
// CONCURRENCY NOTE: A race condition window exists between validateAndCreateHolderLinkConstraints
// and createHolderLink. If two concurrent requests both pass validation for PRIMARY_HOLDER,
// the unique database index will reject the second insert. The rollback pattern correctly
// handles this by deleting the orphaned alias when holder link creation fails.
//
// The database unique index is the source of truth for PRIMARY_HOLDER uniqueness.
// Application-level validation (ValidateHolderLinkConstraints) provides early feedback
// but does not guarantee atomicity - the rollback compensates for any race conditions.
func (uc *UseCase) createAliasWithHolderLink(ctx context.Context, span *trace.Span, logger loggerInterface, organizationID string, holderID uuid.UUID, cai *mmodel.CreateAliasInput, alias *mmodel.Alias, createdAccount *mmodel.Alias) (*mmodel.Alias, error) {
```

**Step 2: Verify and commit**

```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./components/crm/...
git add components/crm/internal/services/create-alias.go
git commit -m "docs(crm): document race condition handling in alias creation"
```

---

### Task 22: Rename shadowed variable in buildAliasFromInput (Low)

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/services/create-alias.go`

**Finding:** Variable `alias` in `buildAliasFromInput` creates potential confusion with outer scope.

**Step 1: Rename variable**

Find in `buildAliasFromInput` (around line 64-100):
```go
func (uc *UseCase) buildAliasFromInput(ctx context.Context, span *trace.Span, logger loggerInterface, organizationID string, holderID uuid.UUID, cai *mmodel.CreateAliasInput) (*mmodel.Alias, error) {
	accountID := libCommons.GenerateUUIDv7()

	alias := &mmodel.Alias{
```

Replace `alias` with `newAlias` throughout the function:
```go
func (uc *UseCase) buildAliasFromInput(ctx context.Context, span *trace.Span, logger loggerInterface, organizationID string, holderID uuid.UUID, cai *mmodel.CreateAliasInput) (*mmodel.Alias, error) {
	accountID := libCommons.GenerateUUIDv7()

	newAlias := &mmodel.Alias{
```

And update all references within the function:
- `alias.BankingDetails` -> `newAlias.BankingDetails`
- `alias.Document` -> `newAlias.Document`
- `alias.Type` -> `newAlias.Type`
- `return alias, nil` -> `return newAlias, nil`

**Step 2: Verify and commit**

```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./components/crm/...
git add components/crm/internal/services/create-alias.go
git commit -m "refactor(crm): rename shadowed variable in buildAliasFromInput"
```

---

## Batch 8: Final Verification and Summary

### Task 23: Run full test suite

**Prerequisites:**
- All previous tasks completed

**Step 1: Run all affected package tests**

```bash
cd /Users/fredamaral/repos/lerianstudio/midaz
go test ./pkg/transaction/...
go test ./components/transaction/...
go test ./components/crm/...
```

**Step 2: Run linting**

```bash
make lint
```

**Step 3: Verify all changes compile**

```bash
go build ./...
```

---

### Task 24: Run Code Review (Final Batch)

1. **Dispatch all 3 reviewers in parallel:**
   - REQUIRED SUB-SKILL: Use requesting-code-review
   - Focus on Batches 5-7 changes

2. **Verify all Medium/Low findings are addressed:**
   - All TODO comments removed from addressed issues
   - No new issues introduced

---

## Review Findings Summary

| Finding | Severity | File | Task | Fix |
|---------|----------|------|------|-----|
| Empty queue name validation | Low | consumer.rabbitmq.go | Task 16 | Added assert.NotEmpty |
| Context-aware sleep | Medium | consumer.rabbitmq.go | Task 17 | Added sleepWithContext helper |
| retry.action span attribute | Low | consumer.rabbitmq.go | Task 18 | Added span attribute |
| Operations polling timeout | Medium | operations_wait.go | Task 19 | Added ErrOperationsWaitTimeout |
| Holder deletion partial failure | Medium | delete-holder.go | Task 20 | Improved error aggregation |
| Race condition documentation | Medium | create-alias.go | Task 21 | Added doc comment |
| Variable shadowing | Low | create-alias.go | Task 22 | Renamed to newAlias |
| Test data constructors | Low | testutil.go | Tasks 1-4 | Created new file |
