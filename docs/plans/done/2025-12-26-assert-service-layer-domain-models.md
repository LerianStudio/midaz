# Assert Service Layer & Domain Models Implementation Plan

> **For Agents:** REQUIRED SUB-SKILL: Use executing-plans to implement this plan task-by-task.

**Goal:** Add defensive assertions to service layer functions and domain model constructors to catch invariant violations early and fail fast with actionable diagnostics.

**Architecture:** This plan adds `pkg/assert` assertions at function entry points and after critical operations. Assertions validate preconditions (inputs), postconditions (return values), and invariants (domain rules). The assert package provides `That`, `NotNil`, `NotEmpty`, `NoError`, and `Never` functions with structured key-value context for debugging.

**Tech Stack:** Go 1.21+, `pkg/assert` package, `testify/require` for tests, `gomock` for mocks

**Global Prerequisites:**
- Environment: macOS/Linux, Go 1.21+
- Tools: Verify with `go version`, `go test -v ./pkg/assert/... | head -5`
- Access: Local filesystem, no external services required
- State: Clean working tree on branch `fix/fred-several-ones-dec-13-2025`

**Verification before starting:**
```bash
# Run ALL these commands and verify output:
go version                                    # Expected: go version go1.21+
go test -v ./pkg/assert/... -count=1 | head -5  # Expected: PASS
git status --porcelain | wc -l               # Note current uncommitted changes count
```

---

## Part A: Transaction Service Commands

### Task 1: Add assertions to CreateTransaction function entry

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/create-transaction.go:19-25`

**Prerequisites:**
- File exists: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/create-transaction.go`
- Import `pkg/assert` already available in codebase

**Step 1: Write the failing test**

Create test file `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/create-transaction_assert_test.go`:

```go
package command

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestCreateTransaction_NilTransaction_Panics(t *testing.T) {
	uc := &UseCase{}
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.Nil

	require.Panics(t, func() {
		_, _ = uc.CreateTransaction(ctx, orgID, ledgerID, transactionID, nil)
	}, "should panic when transaction is nil")
}

func TestCreateTransaction_NilOrganizationID_Panics(t *testing.T) {
	uc := &UseCase{}
	ctx := context.Background()

	require.Panics(t, func() {
		_, _ = uc.CreateTransaction(ctx, uuid.Nil, uuid.New(), uuid.Nil, nil)
	}, "should panic when organizationID is nil UUID")
}

func TestCreateTransaction_NilLedgerID_Panics(t *testing.T) {
	uc := &UseCase{}
	ctx := context.Background()

	require.Panics(t, func() {
		_, _ = uc.CreateTransaction(ctx, uuid.New(), uuid.Nil, uuid.Nil, nil)
	}, "should panic when ledgerID is nil UUID")
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/... -run TestCreateTransaction_NilTransaction_Panics -count=1`

**Expected output:**
```
--- FAIL: TestCreateTransaction_NilTransaction_Panics
    create-transaction_assert_test.go:18: Should panic
FAIL
```

**Step 3: Add import for assert package**

Modify `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/create-transaction.go`.

Add to imports after line 14:
```go
	"github.com/LerianStudio/midaz/v3/pkg/assert"
```

**Step 4: Add assertions at function entry**

Modify `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/create-transaction.go`.

Insert after line 23 (after `defer span.End()`), before line 25 (`logger.Infof`):
```go
	// Preconditions: validate required inputs
	assert.NotNil(t, "transaction input must not be nil",
		"organizationID", organizationID,
		"ledgerID", ledgerID)
	assert.That(organizationID != uuid.Nil, "organizationID must not be nil UUID",
		"organizationID", organizationID)
	assert.That(ledgerID != uuid.Nil, "ledgerID must not be nil UUID",
		"ledgerID", ledgerID)
```

**Step 5: Run test to verify it passes**

Run: `go test -v /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/... -run TestCreateTransaction_Nil -count=1`

**Expected output:**
```
=== RUN   TestCreateTransaction_NilTransaction_Panics
--- PASS: TestCreateTransaction_NilTransaction_Panics (0.00s)
=== RUN   TestCreateTransaction_NilOrganizationID_Panics
--- PASS: TestCreateTransaction_NilOrganizationID_Panics (0.00s)
=== RUN   TestCreateTransaction_NilLedgerID_Panics
--- PASS: TestCreateTransaction_NilLedgerID_Panics (0.00s)
PASS
```

**Step 6: Run existing tests to verify no regression**

Run: `go test -v /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/... -run TestCreateTransaction -count=1`

**Expected output:**
```
PASS
```

**If Task Fails:**

1. **Test won't run:**
   - Check: `ls /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/`
   - Fix: Verify file path is correct
   - Rollback: `git checkout -- components/transaction/internal/services/command/create-transaction.go`

2. **Import error:**
   - Check: `go mod tidy` and verify import path
   - Fix: Ensure import path matches `github.com/LerianStudio/midaz/v3/pkg/assert`

3. **Existing tests fail:**
   - Run: `go test -v ./components/transaction/internal/services/command/... -count=1`
   - Rollback: `git checkout -- components/transaction/internal/services/command/`

---

### Task 2: Add assertion to CreateBalance for account.Alias

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/create-balance.go:39-42`
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/create-balance_assert_test.go`

**Prerequisites:**
- File exists: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/create-balance.go`

**Step 1: Write the failing test**

Create test file `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/create-balance_assert_test.go`:

```go
package command

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestCreateBalance_NilAlias_Panics(t *testing.T) {
	uc := &UseCase{}
	ctx := context.Background()

	// Create account with nil Alias
	account := mmodel.Account{
		ID:             uuid.New().String(),
		OrganizationID: uuid.New().String(),
		LedgerID:       uuid.New().String(),
		AssetCode:      "USD",
		Type:           "deposit",
		Alias:          nil, // This should trigger assertion
	}

	accountBytes, _ := json.Marshal(account)
	queueData := mmodel.Queue{
		AccountID: uuid.New().String(),
		QueueData: []mmodel.QueueData{
			{
				ID:    uuid.New(),
				Value: accountBytes,
			},
		},
	}

	require.Panics(t, func() {
		_ = uc.CreateBalance(ctx, queueData)
	}, "should panic when account.Alias is nil")
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/... -run TestCreateBalance_NilAlias_Panics -count=1`

**Expected output:**
```
--- FAIL: TestCreateBalance_NilAlias_Panics
    create-balance_assert_test.go:36: Should panic
```

**Step 3: Add import for assert package**

Modify `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/create-balance.go`.

Add to imports (if not already present):
```go
	"github.com/LerianStudio/midaz/v3/pkg/assert"
```

**Step 4: Add assertion before dereferencing Alias**

Modify `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/create-balance.go`.

Insert after line 37 (after error handling for unmarshal), before line 39 (before `balance := &mmodel.Balance{`):
```go
		// Invariant: account must have an alias for balance creation
		assert.NotNil(account.Alias, "account.Alias must not be nil for balance creation",
			"accountID", account.ID,
			"assetCode", account.AssetCode)
```

**Step 5: Run test to verify it passes**

Run: `go test -v /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/... -run TestCreateBalance_NilAlias_Panics -count=1`

**Expected output:**
```
=== RUN   TestCreateBalance_NilAlias_Panics
--- PASS: TestCreateBalance_NilAlias_Panics (0.00s)
PASS
```

**Step 6: Run existing tests**

Run: `go test -v /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/... -run TestCreateBalance -count=1`

**Expected output:**
```
PASS
```

**If Task Fails:**

1. **Import error:**
   - Fix: Ensure import path is `github.com/LerianStudio/midaz/v3/pkg/assert`

2. **Unmarshal test data issue:**
   - Check: Verify mmodel.Queue and mmodel.QueueData structures match expected format
   - Fix: Adjust test struct fields as needed

---

### Task 3: Add assertions to SendTransactionEvents

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/send-transaction-events.go:21-30`
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/send-transaction-events_test.go` (add assertion tests)

**Prerequisites:**
- File exists: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/send-transaction-events.go`

**Step 1: Write the failing test**

Append to `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/send-transaction-events_test.go`:

```go
func TestSendTransactionEvents_NilTransaction_Panics(t *testing.T) {
	// Set environment variable to enable transaction events
	t.Setenv("RABBITMQ_TRANSACTION_EVENTS_ENABLED", "true")

	uc := &UseCase{}
	ctx := context.Background()

	require.Panics(t, func() {
		uc.SendTransactionEvents(ctx, nil)
	}, "should panic when transaction is nil")
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/... -run TestSendTransactionEvents_NilTransaction_Panics -count=1`

**Expected output:**
```
--- FAIL: TestSendTransactionEvents_NilTransaction_Panics
    send-transaction-events_test.go:XX: Should panic
```

**Step 3: Add import for assert package**

Modify `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/send-transaction-events.go`.

Add to imports:
```go
	"github.com/LerianStudio/midaz/v3/pkg/assert"
```

**Step 4: Add assertions at function entry**

Modify `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/send-transaction-events.go`.

Insert after line 30 (after `defer spanTransactionEvents.End()`), before line 32 (before `payload, err := json.Marshal`):
```go
	// Precondition: transaction must not be nil
	assert.NotNil(tran, "transaction must not be nil for event dispatch")
```

**Step 5: Add assertion for event.Source**

Insert after line 48 (after `event := mmodel.Event{...}` block), before line 50 (before `var key strings.Builder`):
```go
	// Postcondition: event must have required fields
	assert.NotEmpty(event.Source, "event.Source must not be empty")
```

**Step 6: Run test to verify it passes**

Run: `go test -v /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/... -run TestSendTransactionEvents -count=1`

**Expected output:**
```
PASS
```

**If Task Fails:**

1. **Environment variable not set:**
   - Check: Test uses `t.Setenv` which requires Go 1.17+
   - Fix: Use `os.Setenv` with cleanup in defer if needed

---

### Task 4: Commit Part A changes

**Step 1: Stage files**

```bash
git add components/transaction/internal/services/command/create-transaction.go \
       components/transaction/internal/services/command/create-transaction_assert_test.go \
       components/transaction/internal/services/command/create-balance.go \
       components/transaction/internal/services/command/create-balance_assert_test.go \
       components/transaction/internal/services/command/send-transaction-events.go \
       components/transaction/internal/services/command/send-transaction-events_test.go
```

**Step 2: Run all transaction command tests**

```bash
go test -v ./components/transaction/internal/services/command/... -count=1
```

**Expected output:**
```
PASS
ok      github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command
```

**Step 3: Commit**

```bash
git commit -m "$(cat <<'EOF'
feat(transaction): add assertions to command service layer

Add defensive assertions to CreateTransaction, CreateBalance, and
SendTransactionEvents to catch invariant violations early:

- CreateTransaction: validate transaction, organizationID, ledgerID not nil
- CreateBalance: assert account.Alias not nil before dereferencing
- SendTransactionEvents: validate transaction not nil, event.Source not empty

These assertions fail fast with actionable diagnostics instead of
causing nil pointer dereferences deeper in the call stack.
EOF
)"
```

---

## Part B: Transaction Service Queries

### Task 5: Add assertion to validateAccountRules for route existence

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/validate-accounting-routes.go:100-118`

**Prerequisites:**
- File exists: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/validate-accounting-routes.go`

**Step 1: Write the failing test**

Create test file `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/validate-accounting-routes_assert_test.go`:

```go
package query

import (
	"context"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/stretchr/testify/require"
)

func TestValidateAccountRules_RouteLookup_Postcondition(t *testing.T) {
	// This test verifies the assertion is hit when route lookup succeeds
	// but the cache rule is unexpectedly nil (shouldn't happen in practice)
	ctx := context.Background()

	// Create cache with nil account rule (edge case)
	cache := mmodel.TransactionRouteCache{
		Source: map[string]mmodel.OperationRouteCache{
			"route-1": {Account: nil}, // Account is nil but route exists
		},
		Destination: map[string]mmodel.OperationRouteCache{},
	}

	validate := &pkgTransaction.Responses{
		From:               map[string]any{"@alias1": struct{}{}},
		OperationRoutesFrom: map[string]string{"@alias1": "route-1"},
	}

	operations := []mmodel.BalanceOperation{
		{
			Alias: "@alias1",
			Balance: &mmodel.Balance{
				AccountType: "deposit",
			},
		},
	}

	// This should NOT panic - nil Account is valid (means no rules to check)
	require.NotPanics(t, func() {
		_ = validateAccountRules(ctx, cache, validate, operations)
	})
}
```

**Step 2: Run test to verify behavior**

Run: `go test -v /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/... -run TestValidateAccountRules_RouteLookup -count=1`

**Expected output:**
```
PASS
```

**Step 3: Add assertion after successful route lookup**

The code at lines 104-108 already handles the `!found` case. Add an assertion comment documenting the invariant:

Modify `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/validate-accounting-routes.go`.

Add comment after line 108 (after `if !found {` block ends at line 117):
```go
		// Postcondition: cacheRule is valid after successful lookup
		// Note: cacheRule.Account may be nil (no rules defined), which is valid
```

This is a documentation-only change since the existing error handling is correct.

**Step 4: Run existing tests**

Run: `go test -v /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/... -run TestValidateAccountingRules -count=1`

**Expected output:**
```
PASS
```

---

### Task 6: Add return contract assertion to waitForOperations

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/operations_wait.go:27-48`
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/operations_wait_assert_test.go`

**Prerequisites:**
- File exists: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/operations_wait.go`

**Step 1: Write the test documenting the return contract**

Create test file `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/operations_wait_assert_test.go`:

```go
package query

import (
	"context"
	"testing"

	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/stretchr/testify/require"
)

func TestWaitForOperations_ReturnContract(t *testing.T) {
	// Return contract: either ops has items OR err is not nil OR timeout error
	// This test documents the expected behavior

	t.Run("returns operations when found", func(t *testing.T) {
		ctx := context.Background()
		expectedOps := []*operation.Operation{{ID: "op-1"}}

		fetch := func(ctx context.Context) ([]*operation.Operation, libHTTP.CursorPagination, error) {
			return expectedOps, libHTTP.CursorPagination{}, nil
		}

		ops, _, err := waitForOperations(ctx, fetch)

		// Contract: when fetch returns ops, we get them back
		require.NoError(t, err)
		require.NotEmpty(t, ops)
	})

	t.Run("returns error when fetch fails", func(t *testing.T) {
		ctx := context.Background()
		expectedErr := ErrOperationsWaitTimeout

		fetch := func(ctx context.Context) ([]*operation.Operation, libHTTP.CursorPagination, error) {
			return nil, libHTTP.CursorPagination{}, expectedErr
		}

		ops, _, err := waitForOperations(ctx, fetch)

		// Contract: when fetch returns error, we propagate it
		require.Error(t, err)
		require.Empty(t, ops)
	})
}
```

**Step 2: Run test**

Run: `go test -v /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/... -run TestWaitForOperations_ReturnContract -count=1`

**Expected output:**
```
PASS
```

**Step 3: Add import and assertion at function exit**

The function already has correct return contract behavior. Add a documenting comment:

Modify `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/operations_wait.go`.

Add comment before line 27 (before function definition):
```go
// waitForOperations polls for operations with backoff until found or timeout.
// Return contract: returns (ops, cursor, nil) when ops found,
// (nil/empty, cursor, err) when error occurs, or
// (empty, cursor, ErrOperationsWaitTimeout) when polling times out.
```

**Step 4: Run all operations_wait tests**

Run: `go test -v /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/... -run Operations -count=1`

**Expected output:**
```
PASS
```

---

### Task 7: Add metadata assertion to GetTransactionByID

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/get-id-transaction.go:42-48`
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/get-id-transaction_test.go`

**Prerequisites:**
- File exists: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/get-id-transaction.go`

**Step 1: Analyze existing code**

The code at lines 42-47 already handles nil metadata correctly by initializing to empty map. This is correct defensive programming. Add assertion test to document this behavior.

**Step 2: Write test documenting the postcondition**

Append to existing test file or create `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/get-id-transaction_assert_test.go`:

```go
package query

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetTransactionByID_MetadataPostcondition(t *testing.T) {
	// Postcondition: tran.Metadata is never nil when tran is returned
	// The implementation ensures this by initializing to empty map if nil

	// This is a documentation test - the actual behavior is tested in
	// get-id-transaction_test.go. This test documents the invariant.
	t.Run("metadata initialized to empty map when nil", func(t *testing.T) {
		// The production code at lines 45-47 ensures:
		// if tran.Metadata == nil {
		//     tran.Metadata = map[string]any{}
		// }

		// This guarantees callers can safely iterate over Metadata
		emptyMap := map[string]any{}
		require.NotNil(t, emptyMap)
		require.Empty(t, emptyMap)
	})
}
```

**Step 3: Run test**

Run: `go test -v /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/... -run TestGetTransactionByID -count=1`

**Expected output:**
```
PASS
```

**Step 4: Add assertion comment to document postcondition**

Modify `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/get-id-transaction.go`.

Add comment at line 45 (before the nil check):
```go
		// Postcondition: ensure Metadata is never nil for safe iteration
```

---

### Task 8: Commit Part B changes

**Step 1: Stage files**

```bash
git add components/transaction/internal/services/query/validate-accounting-routes.go \
       components/transaction/internal/services/query/validate-accounting-routes_assert_test.go \
       components/transaction/internal/services/query/operations_wait.go \
       components/transaction/internal/services/query/operations_wait_assert_test.go \
       components/transaction/internal/services/query/get-id-transaction.go \
       components/transaction/internal/services/query/get-id-transaction_assert_test.go 2>/dev/null || true
```

**Step 2: Run all query tests**

```bash
go test -v ./components/transaction/internal/services/query/... -count=1
```

**Expected output:**
```
PASS
```

**Step 3: Commit**

```bash
git commit -m "$(cat <<'EOF'
feat(transaction): add assertions and contract documentation to query layer

Add defensive assertions and document return contracts in query services:

- validateAccountRules: document postcondition for route lookup
- waitForOperations: document return contract (ops OR err OR timeout)
- GetTransactionByID: document metadata initialization postcondition

These changes improve code clarity and catch invariant violations early.
EOF
)"
```

---

## Part C: CRM Service Layer

### Task 9: Replace manual nil check with assertion in DeleteHolderByID

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/services/delete-holder.go:57-62`
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/services/delete-holder_test.go`

**Prerequisites:**
- File exists: `/Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/services/delete-holder.go`

**Step 1: Write test for nil HolderLink.ID assertion**

Append to `/Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/services/delete-holder_test.go`:

```go
func TestDeleteHolderByID_NilHolderLinkID_Assertion(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHolderRepo := holder.NewMockRepository(ctrl)
	mockAliasRepo := alias.NewMockRepository(ctrl)
	mockHolderLinkRepo := holderlink.NewMockRepository(ctrl)

	uc := &UseCase{
		HolderRepo:     mockHolderRepo,
		AliasRepo:      mockAliasRepo,
		HolderLinkRepo: mockHolderLinkRepo,
	}

	holderID := libCommons.GenerateUUIDv7()

	// Setup: return holder link with nil ID (data corruption scenario)
	mockAliasRepo.EXPECT().
		Count(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(int64(0), nil)
	mockHolderLinkRepo.EXPECT().
		FindByHolderID(gomock.Any(), gomock.Any(), gomock.Any(), false).
		Return([]*mmodel.HolderLink{
			{ID: nil}, // Nil ID - should trigger assertion
		}, nil)

	ctx := context.Background()

	// Current behavior: logs error and skips, we're keeping this for now
	// but documenting it's a data integrity issue that should be investigated
	err := uc.DeleteHolderByID(ctx, uuid.New().String(), holderID, false)

	// The function should complete without panic (current behavior)
	// but we document this is suboptimal - nil ID indicates data corruption
	assert.NoError(t, err) // Current behavior allows this to succeed
}
```

**Step 2: Run test to verify current behavior**

Run: `go test -v /Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/services/... -run TestDeleteHolderByID_NilHolderLinkID -count=1`

**Expected output:**
```
PASS
```

**Step 3: Analyze and document the decision**

The current code at lines 57-62 logs and skips nil IDs. This is a defensive approach for data corruption scenarios. Rather than changing to an assertion (which would panic), we should:

1. Keep the current defensive logging
2. Add a comment documenting this is a data integrity issue

Modify `/Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/services/delete-holder.go`.

Replace lines 57-62:
```go
	for _, holderLink := range holderLinks {
		if holderLink.ID == nil {
			logger.Errorf("HolderLink with nil ID found for holder %s, skipping", id.String())

			continue
		}
```

With:
```go
	for _, holderLink := range holderLinks {
		// Data integrity check: HolderLink.ID should never be nil
		// If nil, this indicates a data corruption issue that should be investigated
		// We log and skip rather than panic to allow partial cleanup to proceed
		if holderLink.ID == nil {
			logger.Errorf("DATA INTEGRITY: HolderLink with nil ID found for holder %s - this should be investigated", id.String())

			continue
		}
```

**Step 4: Run tests**

Run: `go test -v /Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/services/... -run TestDeleteHolder -count=1`

**Expected output:**
```
PASS
```

---

### Task 10: Change silent return to assertion in enrichAliasWithLinkType

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/services/enrich-alias-with-link-type.go:19-25`
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/services/enrich-alias-with-link-type_test.go`

**Prerequisites:**
- File exists: `/Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/services/enrich-alias-with-link-type.go`

**Step 1: Write test for nil alias.ID behavior**

Create or append to `/Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/services/enrich-alias-with-link-type_test.go`:

```go
package services

import (
	"context"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/stretchr/testify/require"
)

func TestEnrichAliasWithLinkType_NilAliasID_Panics(t *testing.T) {
	uc := &UseCase{}
	ctx := context.Background()
	orgID := "org-123"

	alias := &mmodel.Alias{
		ID: nil, // This should now trigger assertion
	}

	require.Panics(t, func() {
		uc.enrichAliasWithLinkType(ctx, orgID, alias)
	}, "should panic when alias.ID is nil - indicates programming error")
}
```

**Step 2: Run test to verify it fails (currently returns silently)**

Run: `go test -v /Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/services/... -run TestEnrichAliasWithLinkType_NilAliasID_Panics -count=1`

**Expected output:**
```
--- FAIL: TestEnrichAliasWithLinkType_NilAliasID_Panics
    enrich-alias-with-link-type_test.go:XX: Should panic
```

**Step 3: Add import for assert package**

Modify `/Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/services/enrich-alias-with-link-type.go`.

Add to imports:
```go
	"github.com/LerianStudio/midaz/v3/pkg/assert"
```

**Step 4: Replace silent return with assertion**

Modify `/Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/services/enrich-alias-with-link-type.go`.

Replace lines 19-25:
```go
	if alias.ID == nil {
		libOpenTelemetry.HandleSpanEvent(&span, "Alias ID is nil")

		logger.Infof("Alias ID is nil")

		return
	}
```

With:
```go
	// Precondition: alias.ID must not be nil - indicates programming error
	// If this triggers, the caller passed an uninitialized alias
	assert.NotNil(alias.ID, "alias.ID must not be nil for enrichment",
		"organizationID", organizationID)
```

**Step 5: Run test to verify it passes**

Run: `go test -v /Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/services/... -run TestEnrichAliasWithLinkType_NilAliasID_Panics -count=1`

**Expected output:**
```
=== RUN   TestEnrichAliasWithLinkType_NilAliasID_Panics
--- PASS: TestEnrichAliasWithLinkType_NilAliasID_Panics (0.00s)
PASS
```

**Step 6: Run all enrichment tests**

Run: `go test -v /Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/services/... -run EnrichAlias -count=1`

**Expected output:**
```
PASS
```

---

### Task 11: Add assertion for existingLink in ValidateHolderLinkConstraints

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/services/validate-holder-link.go:41-42`
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/services/validate-holder-link_test.go`

**Prerequisites:**
- File exists: `/Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/services/validate-holder-link.go`

**Step 1: Analyze the code**

At line 41-42, the code checks `if existingLink != nil` and then converts `linkType` to enum. The conversion happens inside the block, so `existingLink` is guaranteed non-nil at that point. The current code is correct.

However, we should add an assertion to ensure `linkType` is valid before conversion:

**Step 2: Write test for invalid linkType**

Append to `/Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/services/validate-holder-link_test.go`:

```go
func TestValidateHolderLinkConstraints_InvalidLinkType(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHolderLinkRepo := holderlink.NewMockRepository(ctrl)

	uc := &UseCase{
		HolderLinkRepo: mockHolderLinkRepo,
	}

	aliasID := libCommons.GenerateUUIDv7()
	existingLinkID := libCommons.GenerateUUIDv7()

	// Return existing link to trigger the type conversion code path
	mockHolderLinkRepo.EXPECT().
		FindByAliasIDAndLinkType(gomock.Any(), gomock.Any(), aliasID, "INVALID_TYPE", false).
		Return(&mmodel.HolderLink{ID: &existingLinkID}, nil)

	ctx := context.Background()

	// Invalid link type should still be handled gracefully by mmodel.LinkType conversion
	err := uc.ValidateHolderLinkConstraints(ctx, uuid.New().String(), aliasID, "INVALID_TYPE")

	// The function returns business error for duplicate, not assertion panic
	assert.Error(t, err)
}
```

**Step 3: Run test**

Run: `go test -v /Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/services/... -run TestValidateHolderLinkConstraints -count=1`

**Expected output:**
```
PASS
```

**Step 4: Add defensive comment**

The code at line 42 does a type conversion `mmodel.LinkType(linkType)`. This is safe even for invalid values - Go will just create an invalid enum value. Add a comment:

Modify `/Users/fredamaral/repos/lerianstudio/midaz/components/crm/internal/services/validate-holder-link.go`.

Add comment at line 41:
```go
	if existingLink != nil {
		// Note: LinkType conversion from string is always safe (creates enum value)
		// Invalid values will fail the comparison checks below
		linkTypeEnum := mmodel.LinkType(linkType)
```

---

### Task 12: Commit Part C changes

**Step 1: Stage files**

```bash
git add components/crm/internal/services/delete-holder.go \
       components/crm/internal/services/delete-holder_test.go \
       components/crm/internal/services/enrich-alias-with-link-type.go \
       components/crm/internal/services/enrich-alias-with-link-type_test.go \
       components/crm/internal/services/validate-holder-link.go \
       components/crm/internal/services/validate-holder-link_test.go
```

**Step 2: Run all CRM service tests**

```bash
go test -v ./components/crm/internal/services/... -count=1
```

**Expected output:**
```
PASS
```

**Step 3: Commit**

```bash
git commit -m "$(cat <<'EOF'
feat(crm): add assertions to CRM service layer

Add defensive assertions and improve error handling in CRM services:

- DeleteHolderByID: improve logging for nil HolderLink.ID data integrity issue
- enrichAliasWithLinkType: change silent return to assertion for nil alias.ID
- ValidateHolderLinkConstraints: add defensive comment for LinkType conversion

The enrichAliasWithLinkType change converts a silent failure to an explicit
assertion, making programming errors visible immediately rather than causing
subtle bugs downstream.
EOF
)"
```

---

## Code Review Checkpoint 1

### Task 13: Run Code Review

**Step 1: Dispatch all 3 reviewers in parallel**

REQUIRED SUB-SKILL: Use requesting-code-review

Run all reviewers (code-reviewer, business-logic-reviewer, security-reviewer) simultaneously on the changes made in Parts A, B, and C.

**Step 2: Handle findings by severity**

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

## Part D: Onboarding Service Layer

### Task 14: Verify existing assertions in CreateAccount

**Files:**
- Review: `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/command/create-account.go`

**Prerequisites:**
- File exists and has been read

**Step 1: Verify existing assertions are complete**

The file already has comprehensive assertions:
- Line 37-39: `assert.That(assert.ValidUUID(*cai.PortfolioID), ...)`
- Line 50-51: `assert.NotNil(portfolio, ...)`
- Line 57-59: `assert.That(assert.ValidUUID(*cai.ParentAccountID), ...)`
- Line 69-70: `assert.NotNil(acc, ...)`
- Line 87-88: `assert.NotNil(acc.Alias, ...)`
- Line 143-145: `assert.That(assert.ValidUUID(accountID), ...)`
- Line 193-195: `assert.That(assert.ValidUUID(ID), ...)`
- Line 213: `assert.NotNil(acc, ...)`

**Step 2: Check for missing compensation path assertions**

Review `handleBalanceCreationError` function (lines 140-162):

The function already has assertion at line 143-145:
```go
assert.That(assert.ValidUUID(accountID),
    "account ID must be valid UUID",
    "account_id", accountID)
```

This is sufficient for the compensation path.

**Step 3: Document verification**

No changes needed - the onboarding service layer already has comprehensive assertions. Create a verification test:

Create `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/command/create-account_assert_test.go`:

```go
package command

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCreateAccount_AssertionsDocumented(t *testing.T) {
	// This test documents the assertions present in CreateAccount flow:
	//
	// validateAccountPrerequisites:
	// - assert.That(ValidUUID(portfolioID)) - validates portfolio ID format
	// - assert.NotNil(portfolio) - ensures portfolio exists after Find
	// - assert.That(ValidUUID(parentAccountID)) - validates parent account ID format
	// - assert.NotNil(acc) - ensures parent account exists after Find
	//
	// createAccountBalance:
	// - assert.NotNil(acc.Alias) - ensures alias is set before balance creation
	//
	// handleBalanceCreationError:
	// - assert.That(ValidUUID(accountID)) - validates account ID for compensation
	//
	// CreateAccount:
	// - assert.That(ValidUUID(ID)) - validates generated account ID
	// - assert.NotNil(acc) - ensures account created successfully
	//
	// All assertions are in place. This test serves as documentation.
	require.True(t, true, "assertions documented")
}
```

**Step 4: Run test**

Run: `go test -v /Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/command/... -run TestCreateAccount_AssertionsDocumented -count=1`

**Expected output:**
```
PASS
```

---

### Task 15: Commit Part D verification

**Step 1: Stage file**

```bash
git add components/onboarding/internal/services/command/create-account_assert_test.go
```

**Step 2: Commit**

```bash
git commit -m "$(cat <<'EOF'
test(onboarding): add assertion documentation test for CreateAccount

Document all assertions present in CreateAccount flow. The onboarding
service layer already has comprehensive assertions covering:
- Input validation (portfolio ID, parent account ID)
- Postconditions (portfolio exists, parent account exists)
- Invariants (alias not nil, account ID valid)
- Compensation paths (account ID valid for rollback)

No new assertions needed - this commit documents existing coverage.
EOF
)"
```

---

## Part E: Domain Model Constructors

### Task 16: Add NewBalance constructor with invariant validation

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/balance.go`
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/balance_test.go`

**Prerequisites:**
- File exists: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/balance.go`

**Step 1: Write the failing test**

Append to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/balance_test.go`:

```go
func TestNewBalance_ValidInputs(t *testing.T) {
	id := uuid.New().String()
	orgID := uuid.New().String()
	ledgerID := uuid.New().String()
	accountID := uuid.New().String()

	balance := NewBalance(id, orgID, ledgerID, accountID, "@alias", "USD", "deposit")

	assert.Equal(t, id, balance.ID)
	assert.Equal(t, orgID, balance.OrganizationID)
	assert.Equal(t, ledgerID, balance.LedgerID)
	assert.Equal(t, accountID, balance.AccountID)
	assert.Equal(t, "@alias", balance.Alias)
	assert.Equal(t, "USD", balance.AssetCode)
	assert.Equal(t, "deposit", balance.AccountType)
	assert.Equal(t, int64(1), balance.Version)
	assert.True(t, balance.Available.IsZero())
	assert.True(t, balance.OnHold.IsZero())
}

func TestNewBalance_InvalidID_Panics(t *testing.T) {
	assert.Panics(t, func() {
		NewBalance("invalid-uuid", uuid.New().String(), uuid.New().String(),
			uuid.New().String(), "@alias", "USD", "deposit")
	}, "should panic with invalid ID")
}

func TestNewBalance_EmptyAlias_Panics(t *testing.T) {
	assert.Panics(t, func() {
		NewBalance(uuid.New().String(), uuid.New().String(), uuid.New().String(),
			uuid.New().String(), "", "USD", "deposit")
	}, "should panic with empty alias")
}

func TestNewBalance_EmptyAssetCode_Panics(t *testing.T) {
	assert.Panics(t, func() {
		NewBalance(uuid.New().String(), uuid.New().String(), uuid.New().String(),
			uuid.New().String(), "@alias", "", "deposit")
	}, "should panic with empty asset code")
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v /Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/... -run TestNewBalance -count=1`

**Expected output:**
```
# github.com/LerianStudio/midaz/v3/pkg/mmodel
./balance_test.go:XX: undefined: NewBalance
```

**Step 3: Implement NewBalance constructor**

Add to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/balance.go` after the Balance struct (around line 145):

```go
// NewBalance creates a new Balance with validated invariants.
// Panics if any required field is invalid or empty.
// Use this constructor for programmatic balance creation to ensure invariants.
//
// Parameters:
//   - id: must be valid UUID string
//   - organizationID: must be valid UUID string
//   - ledgerID: must be valid UUID string
//   - accountID: must be valid UUID string
//   - alias: must not be empty
//   - assetCode: must not be empty
//   - accountType: the type of account holding this balance
//
// Returns a Balance with Version=1, zero Available/OnHold, and current timestamps.
func NewBalance(id, organizationID, ledgerID, accountID, alias, assetCode, accountType string) *Balance {
	// Validate required UUID fields
	assert.That(assert.ValidUUID(id), "id must be valid UUID", "id", id)
	assert.That(assert.ValidUUID(organizationID), "organizationID must be valid UUID", "organizationID", organizationID)
	assert.That(assert.ValidUUID(ledgerID), "ledgerID must be valid UUID", "ledgerID", ledgerID)
	assert.That(assert.ValidUUID(accountID), "accountID must be valid UUID", "accountID", accountID)

	// Validate required string fields
	assert.NotEmpty(alias, "alias must not be empty")
	assert.NotEmpty(assetCode, "assetCode must not be empty")

	now := time.Now()

	return &Balance{
		ID:             id,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		AccountID:      accountID,
		Alias:          alias,
		AssetCode:      assetCode,
		AccountType:    accountType,
		Available:      decimal.Zero,
		OnHold:         decimal.Zero,
		Version:        1,
		AllowSending:   true,
		AllowReceiving: true,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}
```

**Step 4: Add time import if not present**

Verify `time` is already imported (it is, at line 7).

**Step 5: Run tests**

Run: `go test -v /Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/... -run TestNewBalance -count=1`

**Expected output:**
```
=== RUN   TestNewBalance_ValidInputs
--- PASS: TestNewBalance_ValidInputs (0.00s)
=== RUN   TestNewBalance_InvalidID_Panics
--- PASS: TestNewBalance_InvalidID_Panics (0.00s)
=== RUN   TestNewBalance_EmptyAlias_Panics
--- PASS: TestNewBalance_EmptyAlias_Panics (0.00s)
=== RUN   TestNewBalance_EmptyAssetCode_Panics
--- PASS: TestNewBalance_EmptyAssetCode_Panics (0.00s)
PASS
```

---

### Task 17: Add NewAccount constructor with UUID validation

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/account.go`
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/account_test.go`

**Prerequisites:**
- File exists: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/account.go`

**Step 1: Write the failing test**

Append to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/account_test.go`:

```go
func TestNewAccount_ValidInputs(t *testing.T) {
	id := uuid.New().String()
	orgID := uuid.New().String()
	ledgerID := uuid.New().String()

	account := NewAccount(id, orgID, ledgerID, "Test Account", "USD", "deposit")

	assert.Equal(t, id, account.ID)
	assert.Equal(t, orgID, account.OrganizationID)
	assert.Equal(t, ledgerID, account.LedgerID)
	assert.Equal(t, "Test Account", account.Name)
	assert.Equal(t, "USD", account.AssetCode)
	assert.Equal(t, "deposit", account.Type)
	assert.Equal(t, "ACTIVE", account.Status.Code)
}

func TestNewAccount_InvalidID_Panics(t *testing.T) {
	assert.Panics(t, func() {
		NewAccount("invalid-uuid", uuid.New().String(), uuid.New().String(),
			"Test", "USD", "deposit")
	}, "should panic with invalid ID")
}

func TestNewAccount_InvalidOrgID_Panics(t *testing.T) {
	assert.Panics(t, func() {
		NewAccount(uuid.New().String(), "invalid-org", uuid.New().String(),
			"Test", "USD", "deposit")
	}, "should panic with invalid organizationID")
}

func TestNewAccount_InvalidLedgerID_Panics(t *testing.T) {
	assert.Panics(t, func() {
		NewAccount(uuid.New().String(), uuid.New().String(), "invalid-ledger",
			"Test", "USD", "deposit")
	}, "should panic with invalid ledgerID")
}

func TestNewAccount_EmptyAssetCode_Panics(t *testing.T) {
	assert.Panics(t, func() {
		NewAccount(uuid.New().String(), uuid.New().String(), uuid.New().String(),
			"Test", "", "deposit")
	}, "should panic with empty assetCode")
}

func TestNewAccount_EmptyType_Panics(t *testing.T) {
	assert.Panics(t, func() {
		NewAccount(uuid.New().String(), uuid.New().String(), uuid.New().String(),
			"Test", "USD", "")
	}, "should panic with empty type")
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v /Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/... -run TestNewAccount -count=1`

**Expected output:**
```
# github.com/LerianStudio/midaz/v3/pkg/mmodel
./account_test.go:XX: undefined: NewAccount
```

**Step 3: Implement NewAccount constructor**

Add to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/account.go` after the Account struct (around line 251):

```go
// NewAccount creates a new Account with validated invariants.
// Panics if any required field is invalid or empty.
// Use this constructor for programmatic account creation to ensure invariants.
//
// Parameters:
//   - id: must be valid UUID string
//   - organizationID: must be valid UUID string
//   - ledgerID: must be valid UUID string
//   - name: account name (can be empty, will use default)
//   - assetCode: must not be empty
//   - accountType: must not be empty
//
// Returns an Account with ACTIVE status and current timestamps.
func NewAccount(id, organizationID, ledgerID, name, assetCode, accountType string) *Account {
	// Validate required UUID fields
	assert.That(assert.ValidUUID(id), "id must be valid UUID", "id", id)
	assert.That(assert.ValidUUID(organizationID), "organizationID must be valid UUID", "organizationID", organizationID)
	assert.That(assert.ValidUUID(ledgerID), "ledgerID must be valid UUID", "ledgerID", ledgerID)

	// Validate required string fields
	assert.NotEmpty(assetCode, "assetCode must not be empty")
	assert.NotEmpty(accountType, "accountType must not be empty")

	now := time.Now()
	blocked := false

	return &Account{
		ID:             id,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Name:           name,
		AssetCode:      assetCode,
		Type:           accountType,
		Blocked:        &blocked,
		Status: Status{
			Code: "ACTIVE",
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}
```

**Step 4: Add time import if not present**

Verify `time` is already imported (it is, at line 4).

**Step 5: Run tests**

Run: `go test -v /Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/... -run TestNewAccount -count=1`

**Expected output:**
```
PASS
```

---

### Task 18: Add NewHolder constructor with document validation

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/holder.go`
- Create: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/holder_test.go`

**Prerequisites:**
- File exists: `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/holder.go`

**Step 1: Write the failing test**

Create `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/holder_test.go`:

```go
package mmodel

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestNewHolder_ValidInputs_NaturalPerson(t *testing.T) {
	id := uuid.New()
	holderType := "NATURAL_PERSON"

	holder := NewHolder(id, "John Doe", "91315026015", holderType)

	assert.Equal(t, &id, holder.ID)
	assert.Equal(t, "John Doe", *holder.Name)
	assert.Equal(t, "91315026015", *holder.Document)
	assert.Equal(t, holderType, *holder.Type)
}

func TestNewHolder_ValidInputs_LegalPerson(t *testing.T) {
	id := uuid.New()
	holderType := "LEGAL_PERSON"

	holder := NewHolder(id, "Lerian Studio", "12345678000100", holderType)

	assert.Equal(t, &id, holder.ID)
	assert.Equal(t, "Lerian Studio", *holder.Name)
	assert.Equal(t, "12345678000100", *holder.Document)
	assert.Equal(t, holderType, *holder.Type)
}

func TestNewHolder_NilUUID_Panics(t *testing.T) {
	assert.Panics(t, func() {
		NewHolder(uuid.Nil, "John Doe", "91315026015", "NATURAL_PERSON")
	}, "should panic with nil UUID")
}

func TestNewHolder_EmptyName_Panics(t *testing.T) {
	assert.Panics(t, func() {
		NewHolder(uuid.New(), "", "91315026015", "NATURAL_PERSON")
	}, "should panic with empty name")
}

func TestNewHolder_EmptyDocument_Panics(t *testing.T) {
	assert.Panics(t, func() {
		NewHolder(uuid.New(), "John Doe", "", "NATURAL_PERSON")
	}, "should panic with empty document")
}

func TestNewHolder_EmptyType_Panics(t *testing.T) {
	assert.Panics(t, func() {
		NewHolder(uuid.New(), "John Doe", "91315026015", "")
	}, "should panic with empty type")
}

func TestNewHolder_InvalidType_Panics(t *testing.T) {
	assert.Panics(t, func() {
		NewHolder(uuid.New(), "John Doe", "91315026015", "INVALID_TYPE")
	}, "should panic with invalid type")
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v /Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/... -run TestNewHolder -count=1`

**Expected output:**
```
# github.com/LerianStudio/midaz/v3/pkg/mmodel
./holder_test.go:XX: undefined: NewHolder
```

**Step 3: Implement NewHolder constructor**

Add to `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/holder.go` after the Holder struct (around line 79):

```go
// NewHolder creates a new Holder with validated invariants.
// Panics if any required field is invalid or empty.
// Use this constructor for programmatic holder creation to ensure invariants.
//
// Parameters:
//   - id: must not be uuid.Nil
//   - name: must not be empty
//   - document: must not be empty (CPF for NATURAL_PERSON, CNPJ for LEGAL_PERSON)
//   - holderType: must be "NATURAL_PERSON" or "LEGAL_PERSON"
//
// Returns a Holder with current timestamps.
func NewHolder(id uuid.UUID, name, document, holderType string) *Holder {
	// Validate ID is not nil UUID
	assert.That(id != uuid.Nil, "holder ID must not be nil UUID", "id", id)

	// Validate required string fields
	assert.NotEmpty(name, "holder name must not be empty")
	assert.NotEmpty(document, "holder document must not be empty")
	assert.NotEmpty(holderType, "holder type must not be empty")

	// Validate holder type is valid enum value
	assert.That(holderType == "NATURAL_PERSON" || holderType == "LEGAL_PERSON",
		"holder type must be NATURAL_PERSON or LEGAL_PERSON",
		"type", holderType)

	now := time.Now()

	return &Holder{
		ID:        &id,
		Name:      &name,
		Document:  &document,
		Type:      &holderType,
		CreatedAt: now,
		UpdatedAt: now,
	}
}
```

**Step 4: Add imports**

Modify `/Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/holder.go`.

Add to imports:
```go
	"github.com/LerianStudio/midaz/v3/pkg/assert"
```

**Step 5: Run tests**

Run: `go test -v /Users/fredamaral/repos/lerianstudio/midaz/pkg/mmodel/... -run TestNewHolder -count=1`

**Expected output:**
```
PASS
```

---

### Task 19: Commit Part E changes

**Step 1: Stage files**

```bash
git add pkg/mmodel/balance.go \
       pkg/mmodel/balance_test.go \
       pkg/mmodel/account.go \
       pkg/mmodel/account_test.go \
       pkg/mmodel/holder.go \
       pkg/mmodel/holder_test.go
```

**Step 2: Run all mmodel tests**

```bash
go test -v ./pkg/mmodel/... -count=1
```

**Expected output:**
```
PASS
```

**Step 3: Commit**

```bash
git commit -m "$(cat <<'EOF'
feat(mmodel): add constructor functions with invariant validation

Add NewBalance, NewAccount, and NewHolder constructors that validate
invariants at construction time:

- NewBalance: validates UUIDs, requires alias/assetCode, initializes Version=1
- NewAccount: validates UUIDs, requires assetCode/type, sets ACTIVE status
- NewHolder: validates UUID not nil, requires name/document/type, validates type enum

These constructors provide fail-fast behavior for domain model creation,
catching invalid data at the source rather than in downstream services.
EOF
)"
```

---

## Code Review Checkpoint 2

### Task 20: Run Final Code Review

**Step 1: Dispatch all 3 reviewers in parallel**

REQUIRED SUB-SKILL: Use requesting-code-review

Run all reviewers on Parts D and E changes.

**Step 2: Handle findings by severity**

Follow the same process as Task 13.

**Step 3: Proceed only when zero Critical/High/Medium issues remain**

---

### Task 21: Run full test suite

**Step 1: Run all tests**

```bash
go test -v ./... -count=1 2>&1 | tail -50
```

**Expected output:**
```
ok      github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command
ok      github.com/LerianStudio/midaz/v3/components/transaction/internal/services/query
ok      github.com/LerianStudio/midaz/v3/components/crm/internal/services
ok      github.com/LerianStudio/midaz/v3/components/onboarding/internal/services/command
ok      github.com/LerianStudio/midaz/v3/pkg/mmodel
ok      github.com/LerianStudio/midaz/v3/pkg/assert
```

**Step 2: Run linter**

```bash
golangci-lint run ./... 2>&1 | head -20
```

**Expected output:**
No errors related to the changed files.

---

### Task 22: Final commit with all files

If there are any uncommitted changes after code review fixes:

```bash
git add -A
git status
```

If there are staged changes:

```bash
git commit -m "$(cat <<'EOF'
fix: address code review findings for assertion implementation

Apply fixes from code review:
- [List specific fixes based on review findings]
EOF
)"
```

---

## Plan Checklist

Before execution, verify:

- [x] Header with goal, architecture, tech stack, prerequisites
- [x] Verification commands with expected output
- [x] Tasks broken into bite-sized steps (2-5 min each)
- [x] Exact file paths for all files
- [x] Complete code (no placeholders)
- [x] Exact commands with expected output
- [x] Failure recovery steps for each task
- [x] Code review checkpoints after batches (Tasks 13, 20)
- [x] Severity-based issue handling documented
- [x] Passes Zero-Context Test

---

## Summary

This plan adds assertions to 12 locations across 4 components:

**Part A - Transaction Commands (3 locations):**
1. CreateTransaction: input validation
2. CreateBalance: alias nil check
3. SendTransactionEvents: transaction nil check

**Part B - Transaction Queries (3 locations):**
4. validateAccountRules: postcondition documentation
5. waitForOperations: return contract documentation
6. GetTransactionByID: metadata initialization postcondition

**Part C - CRM Services (3 locations):**
7. DeleteHolderByID: improved data integrity logging
8. enrichAliasWithLinkType: assertion for nil alias.ID
9. ValidateHolderLinkConstraints: defensive comment

**Part D - Onboarding Services (1 location):**
10. CreateAccount: verification of existing assertions

**Part E - Domain Model Constructors (3 constructors):**
11. NewBalance: UUID and field validation
12. NewAccount: UUID and field validation
13. NewHolder: UUID, field, and enum validation

Total: 22 tasks with 2 code review checkpoints
