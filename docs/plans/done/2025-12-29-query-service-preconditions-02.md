# Query Service Preconditions Implementation Plan

> **For Agents:** REQUIRED SUB-SKILL: Use executing-plans to implement this plan task-by-task.

**Goal:** Add precondition assertions to all query service methods to fail fast on invalid inputs (nil UUIDs, invalid pagination, malformed aliases) and catch caller bugs early.

**Architecture:** Apply the existing `pkg/assert` package at method entry points in query services. Each UUID parameter must be validated against `uuid.Nil`. The critical alias split bug in `get-balances.go` must be fixed with an assertion before array access.

**Tech Stack:** Go, `pkg/assert` package, `github.com/google/uuid`

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
```

## Historical Precedent

**Query:** "precondition assertion validation uuid query service"
**Index Status:** Empty (new project)

No historical data available. This is normal for new projects.
Proceeding with standard planning approach.

---

## Overview

This plan adds ~25-30 assertions across 15 files in transaction and onboarding query services. Each task follows TDD: write a test that expects panic on nil UUID, then add the assertion.

**Files to modify:**
- Transaction query services (8 files)
- Onboarding query services (7 files)

**CRITICAL BUG FIX:** `get-balances.go:144-151` has a latent panic when alias doesn't contain "#"

---

### Task 1: Add Preconditions to GetBalanceByID (Transaction)

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/get-id-balance.go:18-19`
- Test: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/get-id-balance_test.go` (create)

**Prerequisites:**
- Go 1.22+
- `pkg/assert` package available

**Step 1: Write the failing test**

Create file `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/get-id-balance_test.go`:

```go
package query

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestGetBalanceByID_NilOrganizationID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil organizationID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "organizationID must not be nil UUID"),
			"panic message should mention organizationID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetBalanceByID(ctx, uuid.Nil, uuid.New(), uuid.New())
}

func TestGetBalanceByID_NilLedgerID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil ledgerID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "ledgerID must not be nil UUID"),
			"panic message should mention ledgerID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetBalanceByID(ctx, uuid.New(), uuid.Nil, uuid.New())
}

func TestGetBalanceByID_NilBalanceID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil balanceID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "balanceID must not be nil UUID"),
			"panic message should mention balanceID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetBalanceByID(ctx, uuid.New(), uuid.New(), uuid.Nil)
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run "TestGetBalanceByID_Nil" /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/`

**Expected output:**
```
--- FAIL: TestGetBalanceByID_NilOrganizationID_Panics
    get-id-balance_test.go:XX: expected panic on nil organizationID
```

**If you see different error:** Check that test file is in correct package and imports are correct.

**Step 3: Add assertions to implementation**

Modify `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/get-id-balance.go`:

Add import:
```go
import (
	// ... existing imports
	"github.com/LerianStudio/midaz/v3/pkg/assert"
)
```

Add preconditions at the start of `GetBalanceByID` function, immediately after the function signature line 18:

```go
// GetBalanceByID gets data in the repository.
func (uc *UseCase) GetBalanceByID(ctx context.Context, organizationID, ledgerID, balanceID uuid.UUID) (*mmodel.Balance, error) {
	// Preconditions: validate required UUID inputs
	assert.That(organizationID != uuid.Nil, "organizationID must not be nil UUID",
		"organizationID", organizationID)
	assert.That(ledgerID != uuid.Nil, "ledgerID must not be nil UUID",
		"ledgerID", ledgerID)
	assert.That(balanceID != uuid.Nil, "balanceID must not be nil UUID",
		"balanceID", balanceID)

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)
	// ... rest of method unchanged
```

**Step 4: Run test to verify it passes**

Run: `go test -v -run "TestGetBalanceByID_Nil" /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/`

**Expected output:**
```
=== RUN   TestGetBalanceByID_NilOrganizationID_Panics
--- PASS: TestGetBalanceByID_NilOrganizationID_Panics
=== RUN   TestGetBalanceByID_NilLedgerID_Panics
--- PASS: TestGetBalanceByID_NilLedgerID_Panics
=== RUN   TestGetBalanceByID_NilBalanceID_Panics
--- PASS: TestGetBalanceByID_NilBalanceID_Panics
PASS
```

**Step 5: Commit**

```bash
git add components/transaction/internal/services/query/get-id-balance.go components/transaction/internal/services/query/get-id-balance_test.go
git commit -m "$(cat <<'EOF'
feat(transaction): add precondition assertions to GetBalanceByID

Add UUID nil checks at method entry to fail fast on invalid inputs.
Includes tests verifying panic behavior for each UUID parameter.
EOF
)"
```

**If Task Fails:**

1. **Test won't compile:**
   - Check: imports include `github.com/google/uuid` and `github.com/stretchr/testify/assert`
   - Fix: Add missing imports
   - Rollback: `git checkout -- .`

2. **Assertion doesn't trigger panic:**
   - Check: `assert.That` is from `pkg/assert`, not `testify/assert`
   - Fix: Ensure import is `"github.com/LerianStudio/midaz/v3/pkg/assert"`

---

### Task 2: Add Preconditions to GetTransactionByID (Transaction)

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/get-id-transaction.go:18-19`
- Test: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/get-id-transaction_test.go` (create)

**Prerequisites:**
- Task 1 completed
- Go 1.22+

**Step 1: Write the failing test**

Create file `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/get-id-transaction_test.go`:

```go
package query

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestGetTransactionByID_NilOrganizationID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil organizationID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "organizationID must not be nil UUID"),
			"panic message should mention organizationID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetTransactionByID(ctx, uuid.Nil, uuid.New(), uuid.New())
}

func TestGetTransactionByID_NilLedgerID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil ledgerID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "ledgerID must not be nil UUID"),
			"panic message should mention ledgerID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetTransactionByID(ctx, uuid.New(), uuid.Nil, uuid.New())
}

func TestGetTransactionByID_NilTransactionID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil transactionID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "transactionID must not be nil UUID"),
			"panic message should mention transactionID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetTransactionByID(ctx, uuid.New(), uuid.New(), uuid.Nil)
}

func TestGetTransactionWithOperationsByID_NilOrganizationID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil organizationID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "organizationID must not be nil UUID"),
			"panic message should mention organizationID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetTransactionWithOperationsByID(ctx, uuid.Nil, uuid.New(), uuid.New())
}

func TestGetTransactionWithOperationsByID_NilLedgerID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil ledgerID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "ledgerID must not be nil UUID"),
			"panic message should mention ledgerID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetTransactionWithOperationsByID(ctx, uuid.New(), uuid.Nil, uuid.New())
}

func TestGetTransactionWithOperationsByID_NilTransactionID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil transactionID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "transactionID must not be nil UUID"),
			"panic message should mention transactionID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetTransactionWithOperationsByID(ctx, uuid.New(), uuid.New(), uuid.Nil)
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run "TestGetTransaction.*_Nil" /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/`

**Expected output:**
```
--- FAIL: TestGetTransactionByID_NilOrganizationID_Panics
    get-id-transaction_test.go:XX: expected panic on nil organizationID
```

**Step 3: Add assertions to implementation**

Modify `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/get-id-transaction.go`:

Add import:
```go
import (
	// ... existing imports
	"github.com/LerianStudio/midaz/v3/pkg/assert"
)
```

Add preconditions to `GetTransactionByID` (after line 18):

```go
// GetTransactionByID gets data in the repository.
func (uc *UseCase) GetTransactionByID(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID) (*transaction.Transaction, error) {
	// Preconditions: validate required UUID inputs
	assert.That(organizationID != uuid.Nil, "organizationID must not be nil UUID",
		"organizationID", organizationID)
	assert.That(ledgerID != uuid.Nil, "ledgerID must not be nil UUID",
		"ledgerID", ledgerID)
	assert.That(transactionID != uuid.Nil, "transactionID must not be nil UUID",
		"transactionID", transactionID)

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)
	// ... rest of method unchanged
```

Add preconditions to `GetTransactionWithOperationsByID` (after line 46):

```go
// GetTransactionWithOperationsByID gets data in the repository.
func (uc *UseCase) GetTransactionWithOperationsByID(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID) (*transaction.Transaction, error) {
	// Preconditions: validate required UUID inputs
	assert.That(organizationID != uuid.Nil, "organizationID must not be nil UUID",
		"organizationID", organizationID)
	assert.That(ledgerID != uuid.Nil, "ledgerID must not be nil UUID",
		"ledgerID", ledgerID)
	assert.That(transactionID != uuid.Nil, "transactionID must not be nil UUID",
		"transactionID", transactionID)

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)
	// ... rest of method unchanged
```

**Step 4: Run test to verify it passes**

Run: `go test -v -run "TestGetTransaction.*_Nil" /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/`

**Expected output:**
```
=== RUN   TestGetTransactionByID_NilOrganizationID_Panics
--- PASS: TestGetTransactionByID_NilOrganizationID_Panics
...
PASS
```

**Step 5: Commit**

```bash
git add components/transaction/internal/services/query/get-id-transaction.go components/transaction/internal/services/query/get-id-transaction_test.go
git commit -m "$(cat <<'EOF'
feat(transaction): add precondition assertions to GetTransactionByID methods

Add UUID nil checks for both GetTransactionByID and
GetTransactionWithOperationsByID to fail fast on invalid inputs.
EOF
)"
```

**If Task Fails:**

1. **Import conflict:**
   - Check: No duplicate import of `assert`
   - Fix: Use alias if needed: `pkgAssert "github.com/LerianStudio/midaz/v3/pkg/assert"`

---

### Task 3: Add Preconditions to GetOperationByID (Transaction)

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/get-id-operation.go:15-16`
- Test: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/get-id-operation_test.go` (create)

**Prerequisites:**
- Task 2 completed

**Step 1: Write the failing test**

Create file `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/get-id-operation_test.go`:

```go
package query

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestGetOperationByID_NilOrganizationID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil organizationID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "organizationID must not be nil UUID"),
			"panic message should mention organizationID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetOperationByID(ctx, uuid.Nil, uuid.New(), uuid.New(), uuid.New())
}

func TestGetOperationByID_NilLedgerID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil ledgerID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "ledgerID must not be nil UUID"),
			"panic message should mention ledgerID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetOperationByID(ctx, uuid.New(), uuid.Nil, uuid.New(), uuid.New())
}

func TestGetOperationByID_NilTransactionID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil transactionID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "transactionID must not be nil UUID"),
			"panic message should mention transactionID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetOperationByID(ctx, uuid.New(), uuid.New(), uuid.Nil, uuid.New())
}

func TestGetOperationByID_NilOperationID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil operationID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "operationID must not be nil UUID"),
			"panic message should mention operationID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetOperationByID(ctx, uuid.New(), uuid.New(), uuid.New(), uuid.Nil)
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run "TestGetOperationByID_Nil" /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/`

**Expected output:**
```
--- FAIL: TestGetOperationByID_NilOrganizationID_Panics
```

**Step 3: Add assertions to implementation**

Modify `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/get-id-operation.go`:

Add import:
```go
import (
	// ... existing imports
	"github.com/LerianStudio/midaz/v3/pkg/assert"
)
```

Add preconditions to `GetOperationByID` (after line 15):

```go
// GetOperationByID gets data in the repository.
func (uc *UseCase) GetOperationByID(ctx context.Context, organizationID, ledgerID, transactionID, operationID uuid.UUID) (*operation.Operation, error) {
	// Preconditions: validate required UUID inputs
	assert.That(organizationID != uuid.Nil, "organizationID must not be nil UUID",
		"organizationID", organizationID)
	assert.That(ledgerID != uuid.Nil, "ledgerID must not be nil UUID",
		"ledgerID", ledgerID)
	assert.That(transactionID != uuid.Nil, "transactionID must not be nil UUID",
		"transactionID", transactionID)
	assert.That(operationID != uuid.Nil, "operationID must not be nil UUID",
		"operationID", operationID)

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)
	// ... rest of method unchanged
```

**Step 4: Run test to verify it passes**

Run: `go test -v -run "TestGetOperationByID_Nil" /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/`

**Expected output:**
```
PASS
```

**Step 5: Commit**

```bash
git add components/transaction/internal/services/query/get-id-operation.go components/transaction/internal/services/query/get-id-operation_test.go
git commit -m "$(cat <<'EOF'
feat(transaction): add precondition assertions to GetOperationByID

Add UUID nil checks for all 4 UUID parameters (organizationID, ledgerID,
transactionID, operationID) to fail fast on invalid inputs.
EOF
)"
```

---

### Task 4: Add Preconditions to GetParentByTransactionID (Transaction)

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/get-parent-id-transaction.go:15-16`
- Test: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/get-parent-id-transaction_test.go` (create)

**Prerequisites:**
- Task 3 completed

**Step 1: Write the failing test**

Create file `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/get-parent-id-transaction_test.go`:

```go
package query

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestGetParentByTransactionID_NilOrganizationID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil organizationID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "organizationID must not be nil UUID"),
			"panic message should mention organizationID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetParentByTransactionID(ctx, uuid.Nil, uuid.New(), uuid.New())
}

func TestGetParentByTransactionID_NilLedgerID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil ledgerID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "ledgerID must not be nil UUID"),
			"panic message should mention ledgerID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetParentByTransactionID(ctx, uuid.New(), uuid.Nil, uuid.New())
}

func TestGetParentByTransactionID_NilParentID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil parentID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "parentID must not be nil UUID"),
			"panic message should mention parentID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetParentByTransactionID(ctx, uuid.New(), uuid.New(), uuid.Nil)
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run "TestGetParentByTransactionID_Nil" /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/`

**Expected output:**
```
--- FAIL: TestGetParentByTransactionID_NilOrganizationID_Panics
```

**Step 3: Add assertions to implementation**

Modify `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/get-parent-id-transaction.go`:

Add import:
```go
import (
	// ... existing imports
	"github.com/LerianStudio/midaz/v3/pkg/assert"
)
```

Add preconditions to `GetParentByTransactionID` (after line 15):

```go
// GetParentByTransactionID gets data in the repository.
func (uc *UseCase) GetParentByTransactionID(ctx context.Context, organizationID, ledgerID, parentID uuid.UUID) (*transaction.Transaction, error) {
	// Preconditions: validate required UUID inputs
	assert.That(organizationID != uuid.Nil, "organizationID must not be nil UUID",
		"organizationID", organizationID)
	assert.That(ledgerID != uuid.Nil, "ledgerID must not be nil UUID",
		"ledgerID", ledgerID)
	assert.That(parentID != uuid.Nil, "parentID must not be nil UUID",
		"parentID", parentID)

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)
	// ... rest of method unchanged
```

**Step 4: Run test to verify it passes**

Run: `go test -v -run "TestGetParentByTransactionID_Nil" /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/`

**Expected output:**
```
PASS
```

**Step 5: Commit**

```bash
git add components/transaction/internal/services/query/get-parent-id-transaction.go components/transaction/internal/services/query/get-parent-id-transaction_test.go
git commit -m "$(cat <<'EOF'
feat(transaction): add precondition assertions to GetParentByTransactionID

Add UUID nil checks for organizationID, ledgerID, and parentID
to fail fast on invalid inputs.
EOF
)"
```

---

### Task 5: CRITICAL - Fix Alias Split Panic in ValidateIfBalanceExistsOnRedis

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/get-balances.go:144`
- Test: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/get-balances_test.go` (create)

**Prerequisites:**
- Task 4 completed
- This is a CRITICAL bug fix - must be done before adding other preconditions to this file

**Step 1: Write the failing test**

Create file `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/get-balances_test.go`:

```go
package query

import (
	"context"
	"fmt"
	"strings"
	"testing"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestValidateIfBalanceExistsOnRedis_MalformedAlias_Panics(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRedisRepo := redis.NewMockRedisRepository(ctrl)
	uc := &UseCase{
		RedisRepo: mockRedisRepo,
	}

	// Mock Redis to return a valid balance JSON for a malformed alias (no # separator)
	malformedAlias := "alias_without_separator"
	orgID := uuid.New()
	ledgerID := uuid.New()

	// The key format includes the alias, so we need to match any key
	mockRedisRepo.EXPECT().
		Get(gomock.Any(), gomock.Any()).
		Return(`{"id":"test-id","account_id":"acc-123","available":"100","on_hold":"0","version":1}`, nil)

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on malformed alias without # separator")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "alias must contain exactly one '#' separator"),
			"panic message should mention alias separator, got: %s", panicMsg)
	}()

	ctx := context.Background()
	logger := libLog.NewStdLogger()
	_, _ = uc.ValidateIfBalanceExistsOnRedis(ctx, logger, orgID, ledgerID, []string{malformedAlias})
}

func TestGetBalances_NilOrganizationID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil organizationID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "organizationID must not be nil UUID"),
			"panic message should mention organizationID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetBalances(ctx, uuid.Nil, uuid.New(), uuid.New(), nil, nil, "")
}

func TestGetBalances_NilLedgerID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil ledgerID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "ledgerID must not be nil UUID"),
			"panic message should mention ledgerID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetBalances(ctx, uuid.New(), uuid.Nil, uuid.New(), nil, nil, "")
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run "TestValidateIfBalanceExistsOnRedis_MalformedAlias_Panics|TestGetBalances_Nil" /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/`

**Expected output:**
```
--- FAIL: TestValidateIfBalanceExistsOnRedis_MalformedAlias_Panics
    get-balances_test.go:XX: expected panic on malformed alias without # separator
    (or may actually panic with index out of range instead of our assertion)
```

**Step 3: Add assertions to implementation**

Modify `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/get-balances.go`:

Add import:
```go
import (
	// ... existing imports
	"github.com/LerianStudio/midaz/v3/pkg/assert"
)
```

Fix the critical bug at line 144 by adding assertion before array access:

Find this code (around line 144):
```go
			aliasAndKey := strings.Split(alias, "#")
			newBalances = append(newBalances, &mmodel.Balance{
				ID:             b.ID,
				AccountID:      b.AccountID,
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Alias:          aliasAndKey[0],
				Key:            aliasAndKey[1],
```

Replace with:
```go
			aliasAndKey := strings.Split(alias, "#")
			assert.That(len(aliasAndKey) == 2,
				"alias must contain exactly one '#' separator",
				"alias", alias,
				"parts", len(aliasAndKey))
			newBalances = append(newBalances, &mmodel.Balance{
				ID:             b.ID,
				AccountID:      b.AccountID,
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Alias:          aliasAndKey[0],
				Key:            aliasAndKey[1],
```

Add preconditions to `GetBalances` (after line 27):

```go
// GetBalances methods responsible to get balances from a database.
func (uc *UseCase) GetBalances(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, parserDSL *pkgTransaction.Transaction, validate *pkgTransaction.Responses, transactionStatus string) ([]*mmodel.Balance, error) {
	// Preconditions: validate required UUID inputs
	assert.That(organizationID != uuid.Nil, "organizationID must not be nil UUID",
		"organizationID", organizationID)
	assert.That(ledgerID != uuid.Nil, "ledgerID must not be nil UUID",
		"ledgerID", ledgerID)

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)
	// ... rest of method unchanged
```

**Step 4: Run test to verify it passes**

Run: `go test -v -run "TestValidateIfBalanceExistsOnRedis_MalformedAlias_Panics|TestGetBalances_Nil" /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/`

**Expected output:**
```
=== RUN   TestValidateIfBalanceExistsOnRedis_MalformedAlias_Panics
--- PASS: TestValidateIfBalanceExistsOnRedis_MalformedAlias_Panics
=== RUN   TestGetBalances_NilOrganizationID_Panics
--- PASS: TestGetBalances_NilOrganizationID_Panics
=== RUN   TestGetBalances_NilLedgerID_Panics
--- PASS: TestGetBalances_NilLedgerID_Panics
PASS
```

**Step 5: Commit**

```bash
git add components/transaction/internal/services/query/get-balances.go components/transaction/internal/services/query/get-balances_test.go
git commit -m "$(cat <<'EOF'
fix(transaction): prevent panic on malformed alias in ValidateIfBalanceExistsOnRedis

CRITICAL BUG FIX: The code at line 144 would panic with index out of range
if an alias without '#' separator was passed. This adds an assertion to
fail fast with a clear error message.

Also adds UUID nil checks to GetBalances method entry point.
EOF
)"
```

**If Task Fails:**

1. **Mock not matching:**
   - Check: Redis mock expectation matches the key format used
   - Fix: Use `gomock.Any()` for flexible matching

---

### Task 6: Add Preconditions to GetAllBalances (Transaction)

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/get-all-balances.go:20-21`
- Test: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/get-all-balances_test.go` (create)

**Prerequisites:**
- Task 5 completed

**Step 1: Write the failing test**

Create file `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/get-all-balances_test.go`:

```go
package query

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestGetAllBalances_NilOrganizationID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil organizationID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "organizationID must not be nil UUID"),
			"panic message should mention organizationID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _, _ = uc.GetAllBalances(ctx, uuid.Nil, uuid.New(), http.QueryHeader{})
}

func TestGetAllBalances_NilLedgerID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil ledgerID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "ledgerID must not be nil UUID"),
			"panic message should mention ledgerID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _, _ = uc.GetAllBalances(ctx, uuid.New(), uuid.Nil, http.QueryHeader{})
}

func TestGetAllBalancesByAlias_NilOrganizationID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil organizationID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "organizationID must not be nil UUID"),
			"panic message should mention organizationID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetAllBalancesByAlias(ctx, uuid.Nil, uuid.New(), "test-alias")
}

func TestGetAllBalancesByAlias_NilLedgerID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil ledgerID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "ledgerID must not be nil UUID"),
			"panic message should mention ledgerID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetAllBalancesByAlias(ctx, uuid.New(), uuid.Nil, "test-alias")
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run "TestGetAllBalances.*_Nil" /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/`

**Expected output:**
```
--- FAIL: TestGetAllBalances_NilOrganizationID_Panics
```

**Step 3: Add assertions to implementation**

Modify `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/get-all-balances.go`:

Add import:
```go
import (
	// ... existing imports
	"github.com/LerianStudio/midaz/v3/pkg/assert"
)
```

Add preconditions to `GetAllBalances` (after line 20):

```go
// GetAllBalances methods responsible to get all balances from a database.
func (uc *UseCase) GetAllBalances(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*mmodel.Balance, libHTTP.CursorPagination, error) {
	// Preconditions: validate required UUID inputs
	assert.That(organizationID != uuid.Nil, "organizationID must not be nil UUID",
		"organizationID", organizationID)
	assert.That(ledgerID != uuid.Nil, "ledgerID must not be nil UUID",
		"ledgerID", ledgerID)

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)
	// ... rest of method unchanged
```

Add preconditions to `GetAllBalancesByAlias` (after line 76):

```go
// GetAllBalancesByAlias methods responsible to get all balances from a database by alias.
func (uc *UseCase) GetAllBalancesByAlias(ctx context.Context, organizationID, ledgerID uuid.UUID, alias string) ([]*mmodel.Balance, error) {
	// Preconditions: validate required UUID inputs
	assert.That(organizationID != uuid.Nil, "organizationID must not be nil UUID",
		"organizationID", organizationID)
	assert.That(ledgerID != uuid.Nil, "ledgerID must not be nil UUID",
		"ledgerID", ledgerID)

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)
	// ... rest of method unchanged
```

**Step 4: Run test to verify it passes**

Run: `go test -v -run "TestGetAllBalances.*_Nil" /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/`

**Expected output:**
```
PASS
```

**Step 5: Commit**

```bash
git add components/transaction/internal/services/query/get-all-balances.go components/transaction/internal/services/query/get-all-balances_test.go
git commit -m "$(cat <<'EOF'
feat(transaction): add precondition assertions to GetAllBalances methods

Add UUID nil checks for both GetAllBalances and GetAllBalancesByAlias
to fail fast on invalid inputs.
EOF
)"
```

---

### Task 7: Add Preconditions to GetAllTransactions and GetAllOperations (Transaction)

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/get-all-transactions.go:23-24`
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/get-all-operations.go:16-17`
- Test: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/get-all-transactions_test.go` (create)
- Test: Extend existing `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/get-all-operations_test.go`

**Prerequisites:**
- Task 6 completed

**Step 1: Write the failing tests**

Create file `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/get-all-transactions_test.go`:

```go
package query

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestGetAllTransactions_NilOrganizationID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil organizationID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "organizationID must not be nil UUID"),
			"panic message should mention organizationID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _, _ = uc.GetAllTransactions(ctx, uuid.Nil, uuid.New(), http.QueryHeader{})
}

func TestGetAllTransactions_NilLedgerID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil ledgerID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "ledgerID must not be nil UUID"),
			"panic message should mention ledgerID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _, _ = uc.GetAllTransactions(ctx, uuid.New(), uuid.Nil, http.QueryHeader{})
}
```

Add tests to existing file - append to `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/get-all-operations_test.go`:

```go
// Add these test functions at the end of the file

func TestGetAllOperations_NilOrganizationID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil organizationID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "organizationID must not be nil UUID"),
			"panic message should mention organizationID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _, _ = uc.GetAllOperations(ctx, uuid.Nil, uuid.New(), uuid.New(), http.QueryHeader{})
}

func TestGetAllOperations_NilLedgerID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil ledgerID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "ledgerID must not be nil UUID"),
			"panic message should mention ledgerID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _, _ = uc.GetAllOperations(ctx, uuid.New(), uuid.Nil, uuid.New(), http.QueryHeader{})
}

func TestGetAllOperations_NilTransactionID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil transactionID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "transactionID must not be nil UUID"),
			"panic message should mention transactionID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _, _ = uc.GetAllOperations(ctx, uuid.New(), uuid.New(), uuid.Nil, http.QueryHeader{})
}
```

**Step 2: Run tests to verify they fail**

Run: `go test -v -run "TestGetAllTransactions_Nil|TestGetAllOperations_Nil" /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/`

**Expected output:**
```
--- FAIL: TestGetAllTransactions_NilOrganizationID_Panics
--- FAIL: TestGetAllOperations_NilOrganizationID_Panics
```

**Step 3: Add assertions to implementations**

Modify `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/get-all-transactions.go`:

Add import:
```go
import (
	// ... existing imports
	"github.com/LerianStudio/midaz/v3/pkg/assert"
)
```

Add preconditions to `GetAllTransactions` (after line 23):

```go
// GetAllTransactions fetch all Transactions from the repository
func (uc *UseCase) GetAllTransactions(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.QueryHeader) ([]*transaction.Transaction, libHTTP.CursorPagination, error) {
	// Preconditions: validate required UUID inputs
	assert.That(organizationID != uuid.Nil, "organizationID must not be nil UUID",
		"organizationID", organizationID)
	assert.That(ledgerID != uuid.Nil, "ledgerID must not be nil UUID",
		"ledgerID", ledgerID)

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)
	// ... rest of method unchanged
```

Modify `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/get-all-operations.go`:

Add import:
```go
import (
	// ... existing imports
	"github.com/LerianStudio/midaz/v3/pkg/assert"
)
```

Add preconditions to `GetAllOperations` (after line 16):

```go
// GetAllOperations retrieves all operations for a specific transaction with pagination support.
func (uc *UseCase) GetAllOperations(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID, filter http.QueryHeader) ([]*operation.Operation, libHTTP.CursorPagination, error) {
	// Preconditions: validate required UUID inputs
	assert.That(organizationID != uuid.Nil, "organizationID must not be nil UUID",
		"organizationID", organizationID)
	assert.That(ledgerID != uuid.Nil, "ledgerID must not be nil UUID",
		"ledgerID", ledgerID)
	assert.That(transactionID != uuid.Nil, "transactionID must not be nil UUID",
		"transactionID", transactionID)

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)
	// ... rest of method unchanged
```

**Step 4: Run tests to verify they pass**

Run: `go test -v -run "TestGetAllTransactions_Nil|TestGetAllOperations_Nil" /Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/services/query/`

**Expected output:**
```
PASS
```

**Step 5: Commit**

```bash
git add components/transaction/internal/services/query/get-all-transactions.go components/transaction/internal/services/query/get-all-transactions_test.go components/transaction/internal/services/query/get-all-operations.go components/transaction/internal/services/query/get-all-operations_test.go
git commit -m "$(cat <<'EOF'
feat(transaction): add precondition assertions to GetAllTransactions and GetAllOperations

Add UUID nil checks to list methods for fail-fast behavior on invalid inputs.
EOF
)"
```

---

### Task 8: Run Code Review Checkpoint

**Prerequisites:**
- Tasks 1-7 completed

**Step 1: Dispatch all 3 reviewers in parallel**

- REQUIRED SUB-SKILL: Use requesting-code-review
- Run code-reviewer, business-logic-reviewer, security-reviewer simultaneously
- Wait for all to complete

**Step 2: Handle findings by severity**

**Critical/High/Medium Issues:**
- Fix immediately (do NOT add TODO comments for these severities)
- Re-run all 3 reviewers in parallel after fixes
- Repeat until zero Critical/High/Medium issues remain

**Low Issues:**
- Add `TODO(review):` comments in code at the relevant location

**Cosmetic/Nitpick Issues:**
- Add `FIXME(nitpick):` comments in code at the relevant location

**Step 3: Proceed only when:**
- Zero Critical/High/Medium issues remain
- All Low issues have TODO(review): comments added
- All Cosmetic issues have FIXME(nitpick): comments added

---

### Task 9: Add Preconditions to GetOrganizationByID (Onboarding)

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/query/get-id-organization.go:18-19`
- Test: Extend existing `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/query/get-id-organization_test.go`

**Prerequisites:**
- Task 8 (code review) completed

**Step 1: Write the failing test**

Add to existing file `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/query/get-id-organization_test.go`:

```go
// Add these imports if not present
import (
	"fmt"
	"strings"
)

// Add this test function
func TestGetOrganizationByID_NilID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil organizationID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "organizationID must not be nil UUID"),
			"panic message should mention organizationID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetOrganizationByID(ctx, uuid.Nil)
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run "TestGetOrganizationByID_NilID_Panics" /Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/query/`

**Expected output:**
```
--- FAIL: TestGetOrganizationByID_NilID_Panics
```

**Step 3: Add assertions to implementation**

Modify `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/query/get-id-organization.go`:

Add import:
```go
import (
	// ... existing imports
	"github.com/LerianStudio/midaz/v3/pkg/assert"
)
```

Add preconditions to `GetOrganizationByID` (after line 18):

```go
// GetOrganizationByID fetch a new organization from the repository
func (uc *UseCase) GetOrganizationByID(ctx context.Context, id uuid.UUID) (*mmodel.Organization, error) {
	// Preconditions: validate required UUID inputs
	assert.That(id != uuid.Nil, "organizationID must not be nil UUID",
		"organizationID", id)

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)
	// ... rest of method unchanged
```

**Step 4: Run test to verify it passes**

Run: `go test -v -run "TestGetOrganizationByID_NilID_Panics" /Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/query/`

**Expected output:**
```
PASS
```

**Step 5: Commit**

```bash
git add components/onboarding/internal/services/query/get-id-organization.go components/onboarding/internal/services/query/get-id-organization_test.go
git commit -m "$(cat <<'EOF'
feat(onboarding): add precondition assertions to GetOrganizationByID

Add UUID nil check for organizationID to fail fast on invalid inputs.
EOF
)"
```

---

### Task 10: Add Preconditions to GetLedgerByID (Onboarding)

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/query/get-id-ledger.go:17-18`
- Test: Extend existing `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/query/get-id-ledger_test.go`

**Prerequisites:**
- Task 9 completed

**Step 1: Write the failing tests**

Add to existing file `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/query/get-id-ledger_test.go`:

```go
// Add these imports if not present
import (
	"fmt"
	"strings"
)

// Add these test functions
func TestGetLedgerByID_NilOrganizationID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil organizationID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "organizationID must not be nil UUID"),
			"panic message should mention organizationID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetLedgerByID(ctx, uuid.Nil, uuid.New())
}

func TestGetLedgerByID_NilLedgerID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil ledgerID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "ledgerID must not be nil UUID"),
			"panic message should mention ledgerID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetLedgerByID(ctx, uuid.New(), uuid.Nil)
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run "TestGetLedgerByID_Nil" /Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/query/`

**Expected output:**
```
--- FAIL: TestGetLedgerByID_NilOrganizationID_Panics
```

**Step 3: Add assertions to implementation**

Modify `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/query/get-id-ledger.go`:

Add import:
```go
import (
	// ... existing imports
	"github.com/LerianStudio/midaz/v3/pkg/assert"
)
```

Add preconditions to `GetLedgerByID` (after line 17):

```go
// GetLedgerByID Get a ledger from the repository by given id.
func (uc *UseCase) GetLedgerByID(ctx context.Context, organizationID, id uuid.UUID) (*mmodel.Ledger, error) {
	// Preconditions: validate required UUID inputs
	assert.That(organizationID != uuid.Nil, "organizationID must not be nil UUID",
		"organizationID", organizationID)
	assert.That(id != uuid.Nil, "ledgerID must not be nil UUID",
		"ledgerID", id)

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)
	// ... rest of method unchanged
```

**Step 4: Run test to verify it passes**

Run: `go test -v -run "TestGetLedgerByID_Nil" /Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/query/`

**Expected output:**
```
PASS
```

**Step 5: Commit**

```bash
git add components/onboarding/internal/services/query/get-id-ledger.go components/onboarding/internal/services/query/get-id-ledger_test.go
git commit -m "$(cat <<'EOF'
feat(onboarding): add precondition assertions to GetLedgerByID

Add UUID nil checks for organizationID and ledgerID to fail fast on invalid inputs.
EOF
)"
```

---

### Task 11: Add Preconditions to GetAccountByID (Onboarding)

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/query/get-id-account.go:15-16`
- Test: Extend existing `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/query/get-id-account_test.go`

**Prerequisites:**
- Task 10 completed

**Step 1: Write the failing tests**

Add to existing file `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/query/get-id-account_test.go`:

```go
// Add these imports if not present
import (
	"fmt"
	"strings"
)

// Add these test functions
func TestGetAccountByID_NilOrganizationID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil organizationID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "organizationID must not be nil UUID"),
			"panic message should mention organizationID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetAccountByID(ctx, uuid.Nil, uuid.New(), nil, uuid.New())
}

func TestGetAccountByID_NilLedgerID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil ledgerID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "ledgerID must not be nil UUID"),
			"panic message should mention ledgerID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetAccountByID(ctx, uuid.New(), uuid.Nil, nil, uuid.New())
}

func TestGetAccountByID_NilAccountID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil accountID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "accountID must not be nil UUID"),
			"panic message should mention accountID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetAccountByID(ctx, uuid.New(), uuid.New(), nil, uuid.Nil)
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run "TestGetAccountByID_Nil.*_Panics" /Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/query/`

**Expected output:**
```
--- FAIL: TestGetAccountByID_NilOrganizationID_Panics
```

**Step 3: Add assertions to implementation**

Modify `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/query/get-id-account.go`:

Add import:
```go
import (
	// ... existing imports
	"github.com/LerianStudio/midaz/v3/pkg/assert"
)
```

Add preconditions to `GetAccountByID` (after line 15):

```go
// GetAccountByID get an Account from the repository by given id.
func (uc *UseCase) GetAccountByID(ctx context.Context, organizationID, ledgerID uuid.UUID, portfolioID *uuid.UUID, id uuid.UUID) (*mmodel.Account, error) {
	// Preconditions: validate required UUID inputs
	assert.That(organizationID != uuid.Nil, "organizationID must not be nil UUID",
		"organizationID", organizationID)
	assert.That(ledgerID != uuid.Nil, "ledgerID must not be nil UUID",
		"ledgerID", ledgerID)
	// portfolioID is optional (can be nil pointer)
	assert.That(id != uuid.Nil, "accountID must not be nil UUID",
		"accountID", id)

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)
	// ... rest of method unchanged
```

**Step 4: Run test to verify it passes**

Run: `go test -v -run "TestGetAccountByID_Nil.*_Panics" /Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/query/`

**Expected output:**
```
PASS
```

**Step 5: Commit**

```bash
git add components/onboarding/internal/services/query/get-id-account.go components/onboarding/internal/services/query/get-id-account_test.go
git commit -m "$(cat <<'EOF'
feat(onboarding): add precondition assertions to GetAccountByID

Add UUID nil checks for organizationID, ledgerID, and accountID.
Note: portfolioID is optional and can be nil.
EOF
)"
```

---

### Task 12: Add Preconditions to GetAssetByID (Onboarding)

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/query/get-id-asset.go:17-18`
- Test: Extend existing `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/query/get-id-asset_test.go`

**Prerequisites:**
- Task 11 completed

**Step 1: Write the failing tests**

Add to existing file `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/query/get-id-asset_test.go`:

```go
// Add these imports if not present
import (
	"fmt"
	"strings"
)

// Add these test functions
func TestGetAssetByID_NilOrganizationID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil organizationID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "organizationID must not be nil UUID"),
			"panic message should mention organizationID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetAssetByID(ctx, uuid.Nil, uuid.New(), uuid.New())
}

func TestGetAssetByID_NilLedgerID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil ledgerID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "ledgerID must not be nil UUID"),
			"panic message should mention ledgerID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetAssetByID(ctx, uuid.New(), uuid.Nil, uuid.New())
}

func TestGetAssetByID_NilAssetID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil assetID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "assetID must not be nil UUID"),
			"panic message should mention assetID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetAssetByID(ctx, uuid.New(), uuid.New(), uuid.Nil)
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run "TestGetAssetByID_Nil" /Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/query/`

**Expected output:**
```
--- FAIL: TestGetAssetByID_NilOrganizationID_Panics
```

**Step 3: Add assertions to implementation**

Modify `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/query/get-id-asset.go`:

Add import:
```go
import (
	// ... existing imports
	"github.com/LerianStudio/midaz/v3/pkg/assert"
)
```

Add preconditions to `GetAssetByID` (after line 17):

```go
// GetAssetByID get an Asset from the repository by given id.
func (uc *UseCase) GetAssetByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.Asset, error) {
	// Preconditions: validate required UUID inputs
	assert.That(organizationID != uuid.Nil, "organizationID must not be nil UUID",
		"organizationID", organizationID)
	assert.That(ledgerID != uuid.Nil, "ledgerID must not be nil UUID",
		"ledgerID", ledgerID)
	assert.That(id != uuid.Nil, "assetID must not be nil UUID",
		"assetID", id)

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)
	// ... rest of method unchanged
```

**Step 4: Run test to verify it passes**

Run: `go test -v -run "TestGetAssetByID_Nil" /Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/query/`

**Expected output:**
```
PASS
```

**Step 5: Commit**

```bash
git add components/onboarding/internal/services/query/get-id-asset.go components/onboarding/internal/services/query/get-id-asset_test.go
git commit -m "$(cat <<'EOF'
feat(onboarding): add precondition assertions to GetAssetByID

Add UUID nil checks for organizationID, ledgerID, and assetID.
EOF
)"
```

---

### Task 13: Add Preconditions to GetPortfolioByID (Onboarding)

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/query/get-id-portfolio.go:17-18`
- Test: Extend existing `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/query/get-id-portfolio_test.go`

**Prerequisites:**
- Task 12 completed

**Step 1: Write the failing tests**

Add to existing file `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/query/get-id-portfolio_test.go`:

```go
// Add these imports if not present
import (
	"fmt"
	"strings"
)

// Add these test functions
func TestGetPortfolioByID_NilOrganizationID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil organizationID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "organizationID must not be nil UUID"),
			"panic message should mention organizationID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetPortfolioByID(ctx, uuid.Nil, uuid.New(), uuid.New())
}

func TestGetPortfolioByID_NilLedgerID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil ledgerID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "ledgerID must not be nil UUID"),
			"panic message should mention ledgerID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetPortfolioByID(ctx, uuid.New(), uuid.Nil, uuid.New())
}

func TestGetPortfolioByID_NilPortfolioID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil portfolioID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "portfolioID must not be nil UUID"),
			"panic message should mention portfolioID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetPortfolioByID(ctx, uuid.New(), uuid.New(), uuid.Nil)
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run "TestGetPortfolioByID_Nil" /Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/query/`

**Expected output:**
```
--- FAIL: TestGetPortfolioByID_NilOrganizationID_Panics
```

**Step 3: Add assertions to implementation**

Modify `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/query/get-id-portfolio.go`:

Add import:
```go
import (
	// ... existing imports
	"github.com/LerianStudio/midaz/v3/pkg/assert"
)
```

Add preconditions to `GetPortfolioByID` (after line 17):

```go
// GetPortfolioByID get a Portfolio from the repository by given id.
func (uc *UseCase) GetPortfolioByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.Portfolio, error) {
	// Preconditions: validate required UUID inputs
	assert.That(organizationID != uuid.Nil, "organizationID must not be nil UUID",
		"organizationID", organizationID)
	assert.That(ledgerID != uuid.Nil, "ledgerID must not be nil UUID",
		"ledgerID", ledgerID)
	assert.That(id != uuid.Nil, "portfolioID must not be nil UUID",
		"portfolioID", id)

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)
	// ... rest of method unchanged
```

**Step 4: Run test to verify it passes**

Run: `go test -v -run "TestGetPortfolioByID_Nil" /Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/query/`

**Expected output:**
```
PASS
```

**Step 5: Commit**

```bash
git add components/onboarding/internal/services/query/get-id-portfolio.go components/onboarding/internal/services/query/get-id-portfolio_test.go
git commit -m "$(cat <<'EOF'
feat(onboarding): add precondition assertions to GetPortfolioByID

Add UUID nil checks for organizationID, ledgerID, and portfolioID.
EOF
)"
```

---

### Task 14: Add Preconditions to GetSegmentByID (Onboarding)

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/query/get-id-segment.go:18-19`
- Test: Extend existing `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/query/get-id-segment_test.go`

**Prerequisites:**
- Task 13 completed

**Step 1: Write the failing tests**

Add to existing file `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/query/get-id-segment_test.go`:

```go
// Add these imports if not present
import (
	"fmt"
	"strings"
)

// Add these test functions
func TestGetSegmentByID_NilOrganizationID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil organizationID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "organizationID must not be nil UUID"),
			"panic message should mention organizationID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetSegmentByID(ctx, uuid.Nil, uuid.New(), uuid.New())
}

func TestGetSegmentByID_NilLedgerID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil ledgerID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "ledgerID must not be nil UUID"),
			"panic message should mention ledgerID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetSegmentByID(ctx, uuid.New(), uuid.Nil, uuid.New())
}

func TestGetSegmentByID_NilSegmentID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil segmentID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "segmentID must not be nil UUID"),
			"panic message should mention segmentID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetSegmentByID(ctx, uuid.New(), uuid.New(), uuid.Nil)
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run "TestGetSegmentByID_Nil" /Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/query/`

**Expected output:**
```
--- FAIL: TestGetSegmentByID_NilOrganizationID_Panics
```

**Step 3: Add assertions to implementation**

Modify `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/query/get-id-segment.go`:

Add import:
```go
import (
	// ... existing imports
	"github.com/LerianStudio/midaz/v3/pkg/assert"
)
```

Add preconditions to `GetSegmentByID` (after line 18):

```go
// GetSegmentByID get a Segment from the repository by given id.
func (uc *UseCase) GetSegmentByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.Segment, error) {
	// Preconditions: validate required UUID inputs
	assert.That(organizationID != uuid.Nil, "organizationID must not be nil UUID",
		"organizationID", organizationID)
	assert.That(ledgerID != uuid.Nil, "ledgerID must not be nil UUID",
		"ledgerID", ledgerID)
	assert.That(id != uuid.Nil, "segmentID must not be nil UUID",
		"segmentID", id)

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)
	// ... rest of method unchanged
```

**Step 4: Run test to verify it passes**

Run: `go test -v -run "TestGetSegmentByID_Nil" /Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/query/`

**Expected output:**
```
PASS
```

**Step 5: Commit**

```bash
git add components/onboarding/internal/services/query/get-id-segment.go components/onboarding/internal/services/query/get-id-segment_test.go
git commit -m "$(cat <<'EOF'
feat(onboarding): add precondition assertions to GetSegmentByID

Add UUID nil checks for organizationID, ledgerID, and segmentID.
EOF
)"
```

---

### Task 15: Add Preconditions to GetAccountTypeByID (Onboarding)

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/query/get-id-account-type.go:18-19`
- Test: Create `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/query/get-id-account-type_test.go`

**Prerequisites:**
- Task 14 completed

**Step 1: Write the failing test**

Create file `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/query/get-id-account-type_test.go`:

```go
package query

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

func TestGetAccountTypeByID_NilOrganizationID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil organizationID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "organizationID must not be nil UUID"),
			"panic message should mention organizationID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetAccountTypeByID(ctx, uuid.Nil, uuid.New(), uuid.New())
}

func TestGetAccountTypeByID_NilLedgerID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil ledgerID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "ledgerID must not be nil UUID"),
			"panic message should mention ledgerID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetAccountTypeByID(ctx, uuid.New(), uuid.Nil, uuid.New())
}

func TestGetAccountTypeByID_NilAccountTypeID_Panics(t *testing.T) {
	uc := &UseCase{}

	defer func() {
		r := recover()
		assert.NotNil(t, r, "expected panic on nil accountTypeID")
		panicMsg := fmt.Sprintf("%v", r)
		assert.True(t, strings.Contains(panicMsg, "accountTypeID must not be nil UUID"),
			"panic message should mention accountTypeID, got: %s", panicMsg)
	}()

	ctx := context.Background()
	_, _ = uc.GetAccountTypeByID(ctx, uuid.New(), uuid.New(), uuid.Nil)
}
```

**Step 2: Run test to verify it fails**

Run: `go test -v -run "TestGetAccountTypeByID_Nil" /Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/query/`

**Expected output:**
```
--- FAIL: TestGetAccountTypeByID_NilOrganizationID_Panics
```

**Step 3: Add assertions to implementation**

Modify `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/query/get-id-account-type.go`:

Add import:
```go
import (
	// ... existing imports
	"github.com/LerianStudio/midaz/v3/pkg/assert"
)
```

Add preconditions to `GetAccountTypeByID` (after line 18):

```go
// GetAccountTypeByID get an Account Type from the repository by given id.
func (uc *UseCase) GetAccountTypeByID(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*mmodel.AccountType, error) {
	// Preconditions: validate required UUID inputs
	assert.That(organizationID != uuid.Nil, "organizationID must not be nil UUID",
		"organizationID", organizationID)
	assert.That(ledgerID != uuid.Nil, "ledgerID must not be nil UUID",
		"ledgerID", ledgerID)
	assert.That(id != uuid.Nil, "accountTypeID must not be nil UUID",
		"accountTypeID", id)

	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)
	// ... rest of method unchanged
```

**Step 4: Run test to verify it passes**

Run: `go test -v -run "TestGetAccountTypeByID_Nil" /Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/services/query/`

**Expected output:**
```
PASS
```

**Step 5: Commit**

```bash
git add components/onboarding/internal/services/query/get-id-account-type.go components/onboarding/internal/services/query/get-id-account-type_test.go
git commit -m "$(cat <<'EOF'
feat(onboarding): add precondition assertions to GetAccountTypeByID

Add UUID nil checks for organizationID, ledgerID, and accountTypeID.
EOF
)"
```

---

### Task 16: Final Code Review and Verification

**Prerequisites:**
- Tasks 9-15 completed

**Step 1: Run full test suite for modified packages**

```bash
go test -v ./components/transaction/internal/services/query/...
go test -v ./components/onboarding/internal/services/query/...
```

**Expected output:**
```
ok      github.com/LerianStudio/midaz/v3/components/transaction/internal/services/query
ok      github.com/LerianStudio/midaz/v3/components/onboarding/internal/services/query
```

**Step 2: Run lint checks**

```bash
golangci-lint run ./components/transaction/internal/services/query/...
golangci-lint run ./components/onboarding/internal/services/query/...
```

**Expected output:**
No lint errors (clean output)

**Step 3: Dispatch final code review**

- REQUIRED SUB-SKILL: Use requesting-code-review
- Run all 3 reviewers in parallel
- Handle any remaining findings

**Step 4: Create summary commit**

If all reviews pass:

```bash
git add -A
git commit -m "$(cat <<'EOF'
feat: add precondition assertions to query service methods

Adds ~30 UUID nil check assertions across transaction and onboarding
query services. Includes CRITICAL bug fix for alias split panic in
ValidateIfBalanceExistsOnRedis.

Changes:
- Transaction: GetBalanceByID, GetTransactionByID, GetOperationByID,
  GetParentByTransactionID, GetBalances, GetAllBalances, GetAllTransactions,
  GetAllOperations
- Onboarding: GetOrganizationByID, GetLedgerByID, GetAccountByID,
  GetAssetByID, GetPortfolioByID, GetSegmentByID, GetAccountTypeByID

All methods now fail fast with clear context when called with uuid.Nil.
EOF
)"
```

**If Task Fails:**

1. **Tests fail:**
   - Run: `go test -v ./... 2>&1 | grep -A 5 FAIL`
   - Fix: Address specific test failures

2. **Lint errors:**
   - Run: `golangci-lint run --fix`
   - Review auto-fixes

---

## Summary

This plan adds precondition assertions to 15 query service files:

**Transaction Component (8 files):**
1. `get-id-balance.go` - 3 UUID checks
2. `get-id-transaction.go` - 6 UUID checks (2 methods)
3. `get-id-operation.go` - 4 UUID checks
4. `get-parent-id-transaction.go` - 3 UUID checks
5. `get-balances.go` - 2 UUID checks + CRITICAL alias split fix
6. `get-all-balances.go` - 4 UUID checks (2 methods)
7. `get-all-transactions.go` - 2 UUID checks
8. `get-all-operations.go` - 3 UUID checks

**Onboarding Component (7 files):**
9. `get-id-organization.go` - 1 UUID check
10. `get-id-ledger.go` - 2 UUID checks
11. `get-id-account.go` - 3 UUID checks
12. `get-id-asset.go` - 3 UUID checks
13. `get-id-portfolio.go` - 3 UUID checks
14. `get-id-segment.go` - 3 UUID checks
15. `get-id-account-type.go` - 3 UUID checks

**Total: ~30 assertions** protecting query methods from nil UUID inputs.
