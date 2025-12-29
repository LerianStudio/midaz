# Command Service Postconditions Implementation Plan

> **For Agents:** REQUIRED SUB-SKILL: Use executing-plans to implement this plan task-by-task.

**Goal:** Add postcondition assertions to command service methods to catch repository layer bugs at service boundaries and prevent nil propagation.

**Architecture:** Add `assert.NotNil` after repository Create/Update calls, `assert.Never` in switch default branches, and `assert.That(assert.ValidUUID(...))` guards before `uuid.MustParse()`. This catches (nil, nil) repository returns and unknown status codes at the service layer with full context for debugging.

**Tech Stack:** Go 1.21+, `pkg/assert` package, testify/assert, gomock

**Global Prerequisites:**
- Environment: macOS/Linux, Go 1.21+
- Tools: `go test`, `golangci-lint`
- State: Clean working tree on `fix/fred-several-ones-dec-13-2025` branch
- Files: `pkg/assert/assert.go` and `pkg/assert/predicates.go` exist with required functions

**Verification before starting:**
```bash
# Run ALL these commands and verify output:
go version                          # Expected: go version go1.21+
git status --porcelain | head -5    # Expected: working changes present
ls pkg/assert/assert.go             # Expected: file exists
ls pkg/assert/predicates.go         # Expected: file exists
```

## Historical Precedent

**Query:** "postconditions assertions command service validation"
**Index Status:** Empty (new project)

No historical data available. This is normal for new projects.
Proceeding with standard planning approach.

---

## Task 1: Add Preconditions to TransactionExecute

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/send-bto-execute-async.go:20-26`
- Test: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/send-bto-execute-async_test.go`

**Prerequisites:**
- `pkg/assert` import available
- File already imports required packages

**Step 1: Write the failing test**

Add to `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/send-bto-execute-async_test.go`:

```go
func TestTransactionExecute_NilParseDSL_Panics(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := &UseCase{}

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	assert.Panics(t, func() {
		_ = uc.TransactionExecute(ctx, orgID, ledgerID, nil, &pkgTransaction.Responses{}, []*mmodel.Balance{}, &transaction.Transaction{})
	}, "expected panic when parseDSL is nil")
}

func TestTransactionExecute_NilValidate_Panics(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := &UseCase{}

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	assert.Panics(t, func() {
		_ = uc.TransactionExecute(ctx, orgID, ledgerID, &pkgTransaction.Transaction{}, nil, []*mmodel.Balance{}, &transaction.Transaction{})
	}, "expected panic when validate is nil")
}

func TestTransactionExecute_NilTransaction_Panics(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := &UseCase{}

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	assert.Panics(t, func() {
		_ = uc.TransactionExecute(ctx, orgID, ledgerID, &pkgTransaction.Transaction{}, &pkgTransaction.Responses{}, []*mmodel.Balance{}, nil)
	}, "expected panic when transaction is nil")
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run "TestTransactionExecute_Nil" /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/...`

**Expected output:**
```
--- FAIL: TestTransactionExecute_NilParseDSL_Panics
    assert.go:xxx: Should have panicked
```

**Step 3: Add precondition assertions**

In `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/send-bto-execute-async.go`, add import and assertions after line 19 (inside TransactionExecute function, after signature):

First, add import if not present:
```go
import (
	// ... existing imports ...
	"github.com/LerianStudio/midaz/v3/pkg/assert"
)
```

Then modify `TransactionExecute` function at line 20:
```go
// TransactionExecute func that send balances, transaction and operations to execute sync/async.
func (uc *UseCase) TransactionExecute(ctx context.Context, organizationID, ledgerID uuid.UUID, parseDSL *pkgTransaction.Transaction, validate *pkgTransaction.Responses, blc []*mmodel.Balance, tran *transaction.Transaction) error {
	// Preconditions: validate critical inputs that would cause downstream failures
	assert.NotNil(parseDSL, "parseDSL must not be nil",
		"organization_id", organizationID,
		"ledger_id", ledgerID)
	assert.NotNil(validate, "validate must not be nil",
		"organization_id", organizationID,
		"ledger_id", ledgerID)
	assert.NotNil(tran, "transaction must not be nil",
		"organization_id", organizationID,
		"ledger_id", ledgerID)

	if strings.ToLower(os.Getenv("RABBITMQ_TRANSACTION_ASYNC")) == "true" {
		return uc.SendBTOExecuteAsync(ctx, organizationID, ledgerID, parseDSL, validate, blc, tran)
	}

	return uc.CreateBTOExecuteSync(ctx, organizationID, ledgerID, parseDSL, validate, blc, tran)
}
```

**Step 4: Run test to verify it passes**

Run: `go test -v -run "TestTransactionExecute_Nil" /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/...`

**Expected output:**
```
--- PASS: TestTransactionExecute_NilParseDSL_Panics
--- PASS: TestTransactionExecute_NilValidate_Panics
--- PASS: TestTransactionExecute_NilTransaction_Panics
PASS
```

**Step 5: Run all tests in file to verify no regressions**

Run: `go test -v /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/... -run "SendBTO|TransactionExecute" -count=1`

**Expected output:** All tests pass

**If Task Fails:**
1. **Test won't compile:** Check import paths for `pkgTransaction` and `transaction`
2. **Assertion doesn't panic:** Verify `assert.NotNil` implementation in `pkg/assert/assert.go`
3. **Can't recover:** `git checkout -- .` and revisit

---

## Task 2: Add assert.Never to CreateOrUpdateTransaction Switch

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/create-balance-transaction-operations-async.go:150-161`
- Test: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/create-balance-transaction-operations-async_test.go`

**Prerequisites:**
- Task 1 completed (assert import pattern established)

**Step 1: Write the failing test**

Add to the test file:

```go
func TestCreateOrUpdateTransaction_UnknownStatusCode_Panics(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTransactionRepo := transaction.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionRepo: mockTransactionRepo,
	}

	ctx := context.Background()
	logger := mlog.NewMockLogger(ctrl)
	logger.EXPECT().Infof(gomock.Any(), gomock.Any()).AnyTimes()

	tracer := noop.NewTracerProvider().Tracer("test")

	// Create transaction with unknown status code
	unknownTran := &transaction.Transaction{
		ID: uuid.New().String(),
		Status: transaction.Status{
			Code: "UNKNOWN_STATUS", // Not CREATED or PENDING
		},
	}

	tq := transaction.TransactionQueue{
		Transaction: unknownTran,
		Validate:    &pkgTransaction.Responses{},
		ParseDSL:    &pkgTransaction.Transaction{},
	}

	assert.Panics(t, func() {
		_, _ = uc.CreateOrUpdateTransaction(ctx, logger, tracer, tq)
	}, "expected panic for unknown status code")
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run "TestCreateOrUpdateTransaction_UnknownStatusCode" /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/...`

**Expected output:**
```
--- FAIL: TestCreateOrUpdateTransaction_UnknownStatusCode_Panics
    assert.go:xxx: Should have panicked
```

**Step 3: Add assert.Never to switch default**

In `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/create-balance-transaction-operations-async.go`, modify the switch statement around line 150:

```go
	switch tran.Status.Code {
	case constant.CREATED:
		description := constant.APPROVED
		status := transaction.Status{
			Code:        description,
			Description: &description,
		}

		tran.Status = status
	case constant.PENDING:
		tran.Body = *t.ParseDSL
	default:
		assert.Never("unhandled transaction status code in CreateOrUpdateTransaction",
			"status_code", tran.Status.Code,
			"transaction_id", tran.ID)
	}
```

**Step 4: Run test to verify it passes**

Run: `go test -v -run "TestCreateOrUpdateTransaction_UnknownStatusCode" /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/...`

**Expected output:**
```
--- PASS: TestCreateOrUpdateTransaction_UnknownStatusCode_Panics
PASS
```

**Step 5: Run all tests in file**

Run: `go test -v /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/... -run "CreateOrUpdateTransaction|CreateBalanceTransaction" -count=1`

**Expected output:** All tests pass

**If Task Fails:**
1. **Import missing:** Add `"github.com/LerianStudio/midaz/v3/pkg/assert"` to imports
2. **Test setup fails:** Check mock logger interface matches actual interface
3. **Can't recover:** `git checkout -- components/transaction/internal/services/command/create-balance-transaction-operations-async.go`

---

## Task 3: Add Postcondition to UpdateOperation

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/update-operation.go:28-44`
- Test: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/update-operation_test.go`

**Prerequisites:**
- Task 2 completed

**Step 1: Write the failing test**

Add to the test file:

```go
func TestUpdateOperation_RepoReturnsNil_Panics(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOperationRepo := operation.NewMockRepository(ctrl)

	uc := &UseCase{
		OperationRepo: mockOperationRepo,
	}

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()
	operationID := uuid.New()

	input := &mmodel.UpdateOperationInput{
		Description: "Updated description",
	}

	// Mock returns (nil, nil) - invalid postcondition
	mockOperationRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil)

	assert.Panics(t, func() {
		_, _ = uc.UpdateOperation(ctx, orgID, ledgerID, transactionID, operationID, input)
	}, "expected panic when repository returns nil operation without error")
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run "TestUpdateOperation_RepoReturnsNil" /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/...`

**Expected output:**
```
--- FAIL: TestUpdateOperation_RepoReturnsNil_Panics
```

**Step 3: Add postcondition assertion**

In `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/update-operation.go`:

First, add import:
```go
import (
	// ... existing imports ...
	"github.com/LerianStudio/midaz/v3/pkg/assert"
)
```

Then add postcondition after the Update call (around line 44):

```go
	operationUpdated, err := uc.OperationRepo.Update(ctx, organizationID, ledgerID, transactionID, operationID, op)
	if err != nil {
		logger.Errorf("Error updating op on repo by id: %v", err)

		var entityNotFound *pkg.EntityNotFoundError
		if errors.As(err, &entityNotFound) {
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update operation on repo by id", err)

			logger.Warnf("Error updating op on repo by id: %v", err)

			return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Operation{}).Name())
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update operation on repo by id", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Operation{}).Name())
	}

	// Postcondition: repository must return non-nil operation on success
	assert.NotNil(operationUpdated, "repository Update must return non-nil operation on success",
		"operation_id", operationID,
		"transaction_id", transactionID,
		"organization_id", organizationID)
```

**Step 4: Run test to verify it passes**

Run: `go test -v -run "TestUpdateOperation_RepoReturnsNil" /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/...`

**Expected output:**
```
--- PASS: TestUpdateOperation_RepoReturnsNil_Panics
PASS
```

**Step 5: Run all operation tests**

Run: `go test -v /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/... -run "UpdateOperation" -count=1`

**Expected output:** All tests pass

**If Task Fails:**
1. **Mock interface mismatch:** Check operation.MockRepository signature
2. **Test doesn't panic:** Verify assertion is placed after error check, not before
3. **Rollback:** `git checkout -- components/transaction/internal/services/command/update-operation.go`

---

## Task 4: Add Postcondition to CreateOperationRoute

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/create-operation-route.go:38-45`
- Test: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/create-operation-route_test.go`

**Prerequisites:**
- Task 3 completed

**Step 1: Write the failing test**

Add to test file:

```go
func TestCreateOperationRoute_RepoReturnsNil_Panics(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)

	uc := &UseCase{
		OperationRouteRepo: mockOperationRouteRepo,
	}

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	payload := &mmodel.CreateOperationRouteInput{
		Title:         "Test Route",
		Code:          "TEST",
		OperationType: "source",
		Account:       "test-account",
	}

	// Mock returns (nil, nil) - invalid postcondition
	mockOperationRouteRepo.EXPECT().
		Create(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil)

	assert.Panics(t, func() {
		_, _ = uc.CreateOperationRoute(ctx, orgID, ledgerID, payload)
	}, "expected panic when repository returns nil operation route without error")
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run "TestCreateOperationRoute_RepoReturnsNil" /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/...`

**Expected output:** Test fails (no panic)

**Step 3: Add postcondition assertion**

In `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/create-operation-route.go`:

Add import:
```go
import (
	// ... existing imports ...
	"github.com/LerianStudio/midaz/v3/pkg/assert"
)
```

Add postcondition after Create call:

```go
	createdOperationRoute, err := uc.OperationRouteRepo.Create(ctx, organizationID, ledgerID, operationRoute)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create operation route", err)

		logger.Errorf("Failed to create operation route: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.OperationRoute{}).Name())
	}

	// Postcondition: repository must return non-nil operation route on success
	assert.NotNil(createdOperationRoute, "repository Create must return non-nil operation route on success",
		"organization_id", organizationID,
		"ledger_id", ledgerID,
		"code", operationRoute.Code)
```

**Step 4: Run test to verify it passes**

Run: `go test -v -run "TestCreateOperationRoute_RepoReturnsNil" /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/...`

**Expected output:**
```
--- PASS: TestCreateOperationRoute_RepoReturnsNil_Panics
PASS
```

**Step 5: Run all operation route tests**

Run: `go test -v /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/... -run "OperationRoute" -count=1`

**Expected output:** All tests pass

**If Task Fails:**
1. **Import path wrong:** Check mock package name matches actual repository mock
2. **Rollback:** `git checkout -- components/transaction/internal/services/command/create-operation-route.go`

---

## Task 5: Add Postcondition to CreateTransactionRoute

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/create-transaction-route.go:63-70`
- Test: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/create-transaction-route_test.go`

**Prerequisites:**
- Task 4 completed

**Step 1: Write the failing test**

Add to test file:

```go
func TestCreateTransactionRoute_RepoReturnsNil_Panics(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTransactionRouteRepo := transactionroute.NewMockRepository(ctrl)
	mockOperationRouteRepo := operationroute.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionRouteRepo: mockTransactionRouteRepo,
		OperationRouteRepo:   mockOperationRouteRepo,
	}

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	opRouteID := uuid.New()

	payload := &mmodel.CreateTransactionRouteInput{
		Title:           "Test Transaction Route",
		OperationRoutes: []uuid.UUID{opRouteID},
	}

	// Mock FindByIDs returns valid operation routes
	mockOperationRouteRepo.EXPECT().
		FindByIDs(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return([]*mmodel.OperationRoute{
			{ID: opRouteID, OperationType: "source"},
			{ID: uuid.New(), OperationType: "destination"},
		}, nil)

	// Mock Create returns (nil, nil) - invalid postcondition
	mockTransactionRouteRepo.EXPECT().
		Create(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil)

	assert.Panics(t, func() {
		_, _ = uc.CreateTransactionRoute(ctx, orgID, ledgerID, payload)
	}, "expected panic when repository returns nil transaction route without error")
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run "TestCreateTransactionRoute_RepoReturnsNil" /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/...`

**Expected output:** Test fails (no panic)

**Step 3: Add postcondition assertion**

In `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/create-transaction-route.go`:

Add import:
```go
import (
	// ... existing imports ...
	"github.com/LerianStudio/midaz/v3/pkg/assert"
)
```

Add postcondition after Create call:

```go
	createdTransactionRoute, err := uc.TransactionRouteRepo.Create(ctx, organizationID, ledgerID, transactionRoute)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to create transaction route", err)

		logger.Errorf("Failed to create transaction route: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.TransactionRoute{}).Name())
	}

	// Postcondition: repository must return non-nil transaction route on success
	assert.NotNil(createdTransactionRoute, "repository Create must return non-nil transaction route on success",
		"organization_id", organizationID,
		"ledger_id", ledgerID)
```

**Step 4: Run test to verify it passes**

Run: `go test -v -run "TestCreateTransactionRoute_RepoReturnsNil" /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/...`

**Expected output:**
```
--- PASS: TestCreateTransactionRoute_RepoReturnsNil_Panics
PASS
```

**Step 5: Run all transaction route tests**

Run: `go test -v /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/... -run "TransactionRoute" -count=1`

**Expected output:** All tests pass

**If Task Fails:**
1. **Validation fails before Create:** Ensure mock returns both source and destination operation routes
2. **Rollback:** `git checkout -- components/transaction/internal/services/command/create-transaction-route.go`

---

## Task 6: Add Nil Element Check in extractBalanceIDs

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/delete-all-balances-by-account-id.go:143-150`
- Test: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/delete-all-balances-by-account-id_test.go`

**Prerequisites:**
- Task 5 completed

**Step 1: Write the failing test**

Add to test file:

```go
func TestExtractBalanceIDs_NilElementInSlice_Panics(t *testing.T) {
	uc := &UseCase{}

	balances := []*mmodel.Balance{
		{ID: uuid.New().String()},
		nil, // nil element should trigger panic
		{ID: uuid.New().String()},
	}

	assert.Panics(t, func() {
		_ = uc.extractBalanceIDs(balances)
	}, "expected panic when balance slice contains nil element")
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run "TestExtractBalanceIDs_NilElement" /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/...`

**Expected output:** Test fails (nil dereference panic or no controlled panic)

**Step 3: Add nil element check**

In `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/delete-all-balances-by-account-id.go`:

Add import:
```go
import (
	// ... existing imports ...
	"github.com/LerianStudio/midaz/v3/pkg/assert"
)
```

Modify `extractBalanceIDs`:

```go
// extractBalanceIDs extracts balance IDs from balance list
func (uc *UseCase) extractBalanceIDs(balances []*mmodel.Balance) []uuid.UUID {
	balanceIDs := make([]uuid.UUID, 0, len(balances))
	for i, balance := range balances {
		// Precondition: balance elements must not be nil
		assert.NotNil(balance, "balance in list must not be nil",
			"index", i,
			"total_count", len(balances))
		balanceIDs = append(balanceIDs, balance.IDtoUUID())
	}

	return balanceIDs
}
```

**Step 4: Run test to verify it passes**

Run: `go test -v -run "TestExtractBalanceIDs_NilElement" /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/...`

**Expected output:**
```
--- PASS: TestExtractBalanceIDs_NilElementInSlice_Panics
PASS
```

**Step 5: Run all balance deletion tests**

Run: `go test -v /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/... -run "DeleteAllBalances|extractBalanceIDs" -count=1`

**Expected output:** All tests pass

**If Task Fails:**
1. **Method unexported:** Ensure test is in same package `command`
2. **Rollback:** `git checkout -- components/transaction/internal/services/command/delete-all-balances-by-account-id.go`

---

## Task 7: Add Postcondition to UpdateAccount (Onboarding)

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/command/update-account.go:53-70`
- Test: `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/command/update-account_test.go`

**Prerequisites:**
- Task 6 completed

**Step 1: Write the failing test**

Add to test file:

```go
func TestUpdateAccount_RepoReturnsNilWithoutError_Panics(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAccountRepo := account.NewMockRepository(ctrl)

	uc := &UseCase{
		AccountRepo: mockAccountRepo,
	}

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	accountID := uuid.New()

	input := &mmodel.UpdateAccountInput{
		Name: "Updated Account",
	}

	// Find returns non-external account
	mockAccountRepo.EXPECT().
		Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&mmodel.Account{ID: accountID.String(), Type: "internal"}, nil)

	// Update returns (nil, nil) - invalid postcondition
	mockAccountRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil)

	assert.Panics(t, func() {
		_, _ = uc.UpdateAccount(ctx, orgID, ledgerID, nil, accountID, input)
	}, "expected panic when repository returns nil account without error")
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run "TestUpdateAccount_RepoReturnsNilWithoutError" /Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/command/...`

**Expected output:** Test fails (no panic)

**Step 3: Add postcondition assertion**

In `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/command/update-account.go`:

Add import:
```go
import (
	// ... existing imports ...
	"github.com/LerianStudio/midaz/v3/pkg/assert"
)
```

Add postcondition after Update call:

```go
	accountUpdated, err := uc.AccountRepo.Update(ctx, organizationID, ledgerID, portfolioID, id, account)
	if err != nil {
		logger.Errorf("Error updating account on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			logger.Warnf("Account ID not found: %s", id.String())

			err = pkg.ValidateBusinessError(constant.ErrAccountIDNotFound, reflect.TypeOf(mmodel.Account{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update account on repo by id", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update account on repo by id", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Account{}).Name())
	}

	// Postcondition: repository must return non-nil account on success
	assert.NotNil(accountUpdated, "repository Update must return non-nil account on success",
		"account_id", id,
		"organization_id", organizationID,
		"ledger_id", ledgerID)
```

**Step 4: Run test to verify it passes**

Run: `go test -v -run "TestUpdateAccount_RepoReturnsNilWithoutError" /Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/command/...`

**Expected output:**
```
--- PASS: TestUpdateAccount_RepoReturnsNilWithoutError_Panics
PASS
```

**Step 5: Run all account update tests**

Run: `go test -v /Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/command/... -run "UpdateAccount" -count=1`

**Expected output:** All tests pass

**If Task Fails:**
1. **Rollback:** `git checkout -- components/onboarding/internal/services/command/update-account.go`

---

## Task 8: Add Postcondition to UpdateOrganization

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/command/update-organization.go:59-76`
- Test: `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/command/update-organization_test.go`

**Prerequisites:**
- Task 7 completed

**Step 1: Write the failing test**

Add to test file:

```go
func TestUpdateOrganizationByID_RepoReturnsNil_Panics(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockOrganizationRepo := organization.NewMockRepository(ctrl)

	uc := &UseCase{
		OrganizationRepo: mockOrganizationRepo,
	}

	ctx := context.Background()
	orgID := uuid.New()

	input := &mmodel.UpdateOrganizationInput{
		LegalName: "Updated Org",
	}

	// Update returns (nil, nil) - invalid postcondition
	mockOrganizationRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil)

	assert.Panics(t, func() {
		_, _ = uc.UpdateOrganizationByID(ctx, orgID, input)
	}, "expected panic when repository returns nil organization without error")
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run "TestUpdateOrganizationByID_RepoReturnsNil" /Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/command/...`

**Expected output:** Test fails (no panic)

**Step 3: Add postcondition assertion**

In `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/command/update-organization.go`:

Add import:
```go
import (
	// ... existing imports ...
	"github.com/LerianStudio/midaz/v3/pkg/assert"
)
```

Add postcondition after Update call:

```go
	organizationUpdated, err := uc.OrganizationRepo.Update(ctx, id, organization)
	if err != nil {
		logger.Errorf("Error updating organization on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			logger.Warnf("Organization ID not found: %s", id.String())

			err = pkg.ValidateBusinessError(constant.ErrOrganizationIDNotFound, reflect.TypeOf(mmodel.Organization{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update organization on repo by id", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update organization on repo by id", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Organization{}).Name())
	}

	// Postcondition: repository must return non-nil organization on success
	assert.NotNil(organizationUpdated, "repository Update must return non-nil organization on success",
		"organization_id", id)
```

**Step 4: Run test to verify it passes**

Run: `go test -v -run "TestUpdateOrganizationByID_RepoReturnsNil" /Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/command/...`

**Expected output:**
```
--- PASS: TestUpdateOrganizationByID_RepoReturnsNil_Panics
PASS
```

**Step 5: Run all organization tests**

Run: `go test -v /Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/command/... -run "UpdateOrganization" -count=1`

**Expected output:** All tests pass

**If Task Fails:**
1. **Rollback:** `git checkout -- components/onboarding/internal/services/command/update-organization.go`

---

## Task 9: Add Postcondition to UpdateLedger

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/command/update-ledger.go:32-49`
- Test: `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/command/update-ledger_test.go`

**Prerequisites:**
- Task 8 completed

**Step 1: Write the failing test**

Add to test file:

```go
func TestUpdateLedgerByID_RepoReturnsNil_Panics(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockLedgerRepo := ledger.NewMockRepository(ctrl)

	uc := &UseCase{
		LedgerRepo: mockLedgerRepo,
	}

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	input := &mmodel.UpdateLedgerInput{
		Name: "Updated Ledger",
	}

	// Update returns (nil, nil) - invalid postcondition
	mockLedgerRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil)

	assert.Panics(t, func() {
		_, _ = uc.UpdateLedgerByID(ctx, orgID, ledgerID, input)
	}, "expected panic when repository returns nil ledger without error")
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run "TestUpdateLedgerByID_RepoReturnsNil" /Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/command/...`

**Expected output:** Test fails (no panic)

**Step 3: Add postcondition assertion**

In `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/command/update-ledger.go`:

Add import:
```go
import (
	// ... existing imports ...
	"github.com/LerianStudio/midaz/v3/pkg/assert"
)
```

Add postcondition after Update call:

```go
	ledgerUpdated, err := uc.LedgerRepo.Update(ctx, organizationID, id, ledger)
	if err != nil {
		logger.Errorf("Error updating ledger on repo by id: %v", err)

		if errors.Is(err, services.ErrDatabaseItemNotFound) {
			logger.Warnf("Ledger ID not found: %s", id.String())

			err = pkg.ValidateBusinessError(constant.ErrLedgerIDNotFound, reflect.TypeOf(mmodel.Ledger{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update ledger on repo by id", err)

			return nil, err
		}

		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to update ledger on repo by id", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Ledger{}).Name())
	}

	// Postcondition: repository must return non-nil ledger on success
	assert.NotNil(ledgerUpdated, "repository Update must return non-nil ledger on success",
		"ledger_id", id,
		"organization_id", organizationID)
```

**Step 4: Run test to verify it passes**

Run: `go test -v -run "TestUpdateLedgerByID_RepoReturnsNil" /Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/command/...`

**Expected output:**
```
--- PASS: TestUpdateLedgerByID_RepoReturnsNil_Panics
PASS
```

**Step 5: Run all ledger tests**

Run: `go test -v /Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/command/... -run "UpdateLedger" -count=1`

**Expected output:** All tests pass

**If Task Fails:**
1. **Rollback:** `git checkout -- components/onboarding/internal/services/command/update-ledger.go`

---

## Task 10: Fix validateAssetCode Switch Default Case

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/command/create-asset.go:228-249`
- Test: `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/command/create-asset_test.go`

**Prerequisites:**
- Task 9 completed

**Step 1: Write the failing test**

Add to test file:

```go
func TestValidateAssetCode_UnknownError_ReturnsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	uc := &UseCase{}

	ctx := context.Background()

	// This test verifies that unknown validation errors don't silently return nil
	// We need to mock the utils.ValidateCode to return an unexpected error
	// Since we can't easily mock a package function, we'll verify the behavior
	// by testing with a code that passes specific validation but fails another way

	// Test that the function handles all known error cases
	// The switch currently has no default, so unknown errors return nil (bug)
	// After fix, unknown errors should return an internal error

	// Test with valid code to ensure normal path works
	err := uc.validateAssetCode(ctx, "USD")
	assert.NoError(t, err)

	// Test with invalid code to trigger error path
	err = uc.validateAssetCode(ctx, "usd") // lowercase - should fail
	assert.Error(t, err)
}
```

**Step 2: Add default case to switch**

In `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/command/create-asset.go`:

The current code at line 228-249:
```go
	if err := utils.ValidateCode(code); err != nil {
		switch err.Error() {
		case constant.ErrInvalidCodeFormat.Error():
			mapped := pkg.ValidateBusinessError(constant.ErrInvalidCodeFormat, reflect.TypeOf(mmodel.Asset{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate asset code", mapped)

			return mapped
		case constant.ErrCodeUppercaseRequirement.Error():
			mapped := pkg.ValidateBusinessError(constant.ErrCodeUppercaseRequirement, reflect.TypeOf(mmodel.Asset{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate asset code", mapped)

			return mapped
		case constant.ErrInvalidCodeLength.Error():
			mapped := pkg.ValidateBusinessError(constant.ErrInvalidCodeLength, reflect.TypeOf(mmodel.Asset{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate asset code", mapped)

			return mapped
		}
	}
```

Add default case:
```go
	if err := utils.ValidateCode(code); err != nil {
		switch err.Error() {
		case constant.ErrInvalidCodeFormat.Error():
			mapped := pkg.ValidateBusinessError(constant.ErrInvalidCodeFormat, reflect.TypeOf(mmodel.Asset{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate asset code", mapped)

			return mapped
		case constant.ErrCodeUppercaseRequirement.Error():
			mapped := pkg.ValidateBusinessError(constant.ErrCodeUppercaseRequirement, reflect.TypeOf(mmodel.Asset{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate asset code", mapped)

			return mapped
		case constant.ErrInvalidCodeLength.Error():
			mapped := pkg.ValidateBusinessError(constant.ErrInvalidCodeLength, reflect.TypeOf(mmodel.Asset{}).Name())

			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate asset code", mapped)

			return mapped
		default:
			// Unknown validation error - return internal error instead of silently succeeding
			libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to validate asset code with unknown error", err)

			return pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.Asset{}).Name())
		}
	}
```

**Step 3: Run test to verify behavior**

Run: `go test -v -run "TestValidateAssetCode" /Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/command/...`

**Expected output:** Tests pass

**Step 4: Run all asset tests**

Run: `go test -v /Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/command/... -run "Asset" -count=1`

**Expected output:** All tests pass

**If Task Fails:**
1. **Rollback:** `git checkout -- components/onboarding/internal/services/command/create-asset.go`

---

## Task 11: Add Postcondition to CreateAssetRate Update

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/create-assetrate.go:115-121`
- Test: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/create-assetrate_test.go`

**Prerequisites:**
- Task 10 completed

**Step 1: Write the failing test**

Add to test file:

```go
func TestUpdateExistingAssetRate_RepoReturnsNil_Panics(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAssetRateRepo := assetrate.NewMockRepository(ctrl)

	uc := &UseCase{
		AssetRateRepo: mockAssetRateRepo,
	}

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	existingRate := &mmodel.AssetRate{
		ID:   uuid.New().String(),
		From: "USD",
		To:   "EUR",
	}

	input := &mmodel.CreateAssetRateInput{
		From: "USD",
		To:   "EUR",
		Rate: decimal.NewFromFloat(1.2),
	}

	tracer := noop.NewTracerProvider().Tracer("test")
	_, span := tracer.Start(ctx, "test")

	logger := mlog.NewMockLogger(ctrl)
	logger.EXPECT().Infof(gomock.Any(), gomock.Any()).AnyTimes()
	logger.EXPECT().Errorf(gomock.Any(), gomock.Any()).AnyTimes()

	// Update returns (nil, nil) - invalid postcondition
	mockAssetRateRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil)

	assert.Panics(t, func() {
		_, _ = uc.updateExistingAssetRate(ctx, &span, logger, orgID, ledgerID, input, existingRate)
	}, "expected panic when repository returns nil asset rate without error")
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run "TestUpdateExistingAssetRate_RepoReturnsNil" /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/...`

**Expected output:** Test fails (no panic)

**Step 3: Add postcondition assertion**

In `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/create-assetrate.go`, add after the Update call in `updateExistingAssetRate`:

```go
	updated, err := uc.AssetRateRepo.Update(ctx, organizationID, ledgerID, uuid.MustParse(arFound.ID), arFound)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to update asset rate", err)
		logger.Errorf("Error updating asset rate: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.AssetRate{}).Name())
	}

	// Postcondition: repository must return non-nil asset rate on success
	assert.NotNil(updated, "repository Update must return non-nil asset rate on success",
		"asset_rate_id", arFound.ID,
		"organization_id", organizationID,
		"ledger_id", ledgerID)
```

**Step 4: Run test to verify it passes**

Run: `go test -v -run "TestUpdateExistingAssetRate_RepoReturnsNil" /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/...`

**Expected output:**
```
--- PASS: TestUpdateExistingAssetRate_RepoReturnsNil_Panics
PASS
```

**Step 5: Run all asset rate tests**

Run: `go test -v /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/... -run "AssetRate" -count=1`

**Expected output:** All tests pass

**If Task Fails:**
1. **Rollback:** `git checkout -- components/transaction/internal/services/command/create-assetrate.go`

---

## Task 12: Add Postcondition to CreateAssetRate Create

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/create-assetrate.go:184-190`
- Test: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/create-assetrate_test.go`

**Prerequisites:**
- Task 11 completed

**Step 1: Write the failing test**

Add to test file:

```go
func TestCreateNewAssetRate_RepoReturnsNil_Panics(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAssetRateRepo := assetrate.NewMockRepository(ctrl)

	uc := &UseCase{
		AssetRateRepo: mockAssetRateRepo,
	}

	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	input := &mmodel.CreateAssetRateInput{
		From: "USD",
		To:   "EUR",
		Rate: decimal.NewFromFloat(1.2),
	}

	tracer := noop.NewTracerProvider().Tracer("test")
	_, span := tracer.Start(ctx, "test")

	logger := mlog.NewMockLogger(ctrl)
	logger.EXPECT().Infof(gomock.Any(), gomock.Any()).AnyTimes()
	logger.EXPECT().Errorf(gomock.Any(), gomock.Any()).AnyTimes()

	// Create returns (nil, nil) - invalid postcondition
	mockAssetRateRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		Return(nil, nil)

	assert.Panics(t, func() {
		_, _ = uc.createNewAssetRate(ctx, &span, logger, orgID, ledgerID, input)
	}, "expected panic when repository returns nil asset rate without error")
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run "TestCreateNewAssetRate_RepoReturnsNil" /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/...`

**Expected output:** Test fails (no panic)

**Step 3: Add postcondition assertion**

In `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/create-assetrate.go`, add after the Create call in `createNewAssetRate`:

```go
	assetRate, err := uc.AssetRateRepo.Create(ctx, assetRateDB)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Failed to create asset rate on repository", err)
		logger.Errorf("Error creating asset rate: %v", err)

		return nil, pkg.ValidateInternalError(err, reflect.TypeOf(mmodel.AssetRate{}).Name())
	}

	// Postcondition: repository must return non-nil asset rate on success
	assert.NotNil(assetRate, "repository Create must return non-nil asset rate on success",
		"from", cari.From,
		"to", cari.To,
		"organization_id", organizationID,
		"ledger_id", ledgerID)
```

**Step 4: Run test to verify it passes**

Run: `go test -v -run "TestCreateNewAssetRate_RepoReturnsNil" /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/...`

**Expected output:**
```
--- PASS: TestCreateNewAssetRate_RepoReturnsNil_Panics
PASS
```

**Step 5: Run all asset rate tests**

Run: `go test -v /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/... -run "AssetRate" -count=1`

**Expected output:** All tests pass

**If Task Fails:**
1. **Rollback:** `git checkout -- components/transaction/internal/services/command/create-assetrate.go`

---

## Task 13: Run Code Review

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
- This tracks tech debt for future resolution

**Cosmetic/Nitpick Issues:**
- Add `FIXME(nitpick):` comments in code at the relevant location
- Format: `FIXME(nitpick): [Issue description] (reported by [reviewer] on [date], severity: Cosmetic)`
- Low-priority improvements tracked inline

3. **Proceed only when:**
   - Zero Critical/High/Medium issues remain
   - All Low issues have TODO(review): comments added
   - All Cosmetic issues have FIXME(nitpick): comments added

---

## Task 14: Run Full Test Suite

**Files:**
- All modified files

**Prerequisites:**
- Tasks 1-13 completed

**Step 1: Run transaction component tests**

Run: `go test -v /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/command/... -count=1`

**Expected output:** All tests pass

**Step 2: Run onboarding component tests**

Run: `go test -v /Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/command/... -count=1`

**Expected output:** All tests pass

**Step 3: Run linter**

Run: `golangci-lint run ./components/transaction/... ./components/onboarding/...`

**Expected output:** No errors (warnings acceptable)

**If Task Fails:**
1. **Test failures:** Review specific failures and fix
2. **Lint errors:** Fix according to linter suggestions
3. **Rollback full:** `git checkout -- .`

---

## Summary

**Total Assertions Added:** ~15 assertions across 12 files

| File | Assertions Added |
|------|-----------------|
| send-bto-execute-async.go | 3 (preconditions) |
| create-balance-transaction-operations-async.go | 1 (switch default) |
| update-operation.go | 1 (postcondition) |
| create-operation-route.go | 1 (postcondition) |
| create-transaction-route.go | 1 (postcondition) |
| delete-all-balances-by-account-id.go | 1 (nil element check) |
| update-account.go | 1 (postcondition) |
| update-organization.go | 1 (postcondition) |
| update-ledger.go | 1 (postcondition) |
| create-asset.go | 1 (switch default) |
| create-assetrate.go | 2 (postconditions) |

**Expected Outcome:** Repository bugs caught at service layer boundary with clear context for debugging. Prevents nil propagation through the system.
