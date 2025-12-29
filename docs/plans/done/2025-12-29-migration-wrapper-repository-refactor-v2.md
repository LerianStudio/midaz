# Migration Wrapper Repository Refactor V2 Implementation Plan

> **For Agents:** REQUIRED SUB-SKILL: Use executing-plans to implement this plan task-by-task.

**Goal:** Refactor all PostgreSQL repositories to receive MigrationWrapper for future health check integration while maintaining fast-path database operations.

**Architecture:** Repositories receive MigrationWrapper at construction time, extract the underlying PostgresConnection via `GetConnection()` for normal database operations (fast path), and optionally retain the wrapper reference for future health check integration. This approach ensures bootstrap validation happens once at startup, while normal operations use the standard connection pool with zero overhead.

**Tech Stack:** Go 1.21+, PostgreSQL, lib-commons v2 (libPostgres.PostgresConnection), mmigration package

**Global Prerequisites:**
- Environment: macOS or Linux, Go 1.21+
- Tools: `go test`, `golangci-lint`
- Access: Local development database running
- State: Working from `main` branch or current feature branch

**Verification before starting:**
```bash
# Run ALL these commands and verify output:
go version        # Expected: go1.21+
git status        # Expected: clean working tree or known changes
cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./...  # Expected: no errors
```

## Historical Precedent

**Query:** "migration wrapper repository refactor getconnection bootstrap"
**Index Status:** Empty (new project or no artifact index available)

### Successful Patterns to Reference
- Current bootstrap already creates MigrationWrapper and validates with SafeGetDBWithRetry()
- Transaction repos already have `getExecutor` pattern that abstracts DB access
- MigrationWrapper.GetConnection() method already exists at line 670-672

### Failure Patterns to AVOID
- **CRITICAL:** Previous plan proposed calling SafeGetDB() per-operation - this would cause 60-80% performance degradation
- SafeGetDB() is a heavyweight startup operation (opens connection, acquires advisory lock, queries schema_migrations)
- DO NOT call SafeGetDB() in constructors or getExecutor - bootstrap already validates

### Reviewer Recommendations (V2 Review)
- **getExecutor() Must Remain Unchanged:** DO NOT modify getExecutor() methods - they should continue using `r.connection.GetDB()` which is the fast-path for normal operations
- **Wrapper Field Justification:** The `wrapper` field is retained for future health check integration (e.g., exposing `wrapper.GetStatus()` via health endpoints)
- **Onboarding Note:** Onboarding component does not use DBProvider pattern like Transaction does - no additional changes needed beyond repository updates

### Related Past Plans
- `docs/plans/2025-12-29-migration-wrapper-repository-refactor.md` - Original plan (SUPERSEDED by this one due to SafeGetDB performance issue)

---

## Phase 1: Transaction Component Refactoring

### Task 1: Update Transaction Repository Constructor

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/postgres/transaction/transaction.postgresql.go:88-107`

**Prerequisites:**
- Go 1.21+
- File must exist at specified path

**Step 1: Update imports**

Add mmigration import if not present. Locate the import block (lines 3-32) and ensure it includes:

```go
"github.com/LerianStudio/midaz/v3/pkg/mmigration"
```

**Step 2: Update struct to include wrapper field**

Replace the struct definition at lines 88-92:

```go
// TransactionPostgreSQLRepository is a Postgresql-specific implementation of the TransactionRepository.
type TransactionPostgreSQLRepository struct {
	// connection is cached from wrapper.GetConnection() for fast-path DB operations.
	// Normal operations use connection.GetDB() - zero overhead.
	connection *libPostgres.PostgresConnection
	// wrapper is retained for future health check integration via GetStatus().
	// DO NOT use wrapper.SafeGetDB() in hot paths - it's heavyweight (advisory lock + preflight check).
	wrapper   *mmigration.MigrationWrapper
	tableName string
}
```

**Step 3: Update constructor to accept MigrationWrapper**

Replace the constructor at lines 94-107:

```go
// NewTransactionPostgreSQLRepository returns a new instance of TransactionPostgreSQLRepository using the given MigrationWrapper.
// The wrapper provides access to the underlying PostgresConnection for normal operations
// and can be used for future health check integration.
func NewTransactionPostgreSQLRepository(mw *mmigration.MigrationWrapper) *TransactionPostgreSQLRepository {
	assert.NotNil(mw, "MigrationWrapper must not be nil", "repository", "TransactionPostgreSQLRepository")

	// Extract underlying connection for normal DB operations (fast path)
	// NOTE: Do NOT call SafeGetDB() here - bootstrap already validated migrations
	pc := mw.GetConnection()
	assert.NotNil(pc, "PostgresConnection from wrapper must not be nil", "repository", "TransactionPostgreSQLRepository")

	return &TransactionPostgreSQLRepository{
		connection: pc,
		wrapper:    mw,
		tableName:  "transaction",
	}
}
```

**Step 4: Verify compilation**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./components/transaction/...`

**Expected output:**
```
(no output - successful compilation)
```

**If you see import errors:** Ensure mmigration import is added correctly.

**Step 5: Verify getExecutor() is UNCHANGED (IMPORTANT)**

Confirm that the `getExecutor()` method still uses `r.connection.GetDB()` and NOT `r.wrapper.SafeGetDB()`. The getExecutor method should look like:

```go
func (r *TransactionPostgreSQLRepository) getExecutor(ctx context.Context) (dbtx.Executor, error) {
	db, err := r.connection.GetDB()  // ← Must use connection, NOT wrapper.SafeGetDB()
	if err != nil {
		return nil, err
	}
	return dbtx.GetExecutor(ctx, db), nil
}
```

**DO NOT modify getExecutor()** - bootstrap already validated migrations via SafeGetDBWithRetry().

---

### Task 2: Update Operation Repository Constructor

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/postgres/operation/operation.postgresql.go:44-92`

**Prerequisites:**
- Task 1 completed
- Go 1.21+

**Step 1: Update imports**

Ensure the import block includes:

```go
"github.com/LerianStudio/midaz/v3/pkg/mmigration"
```

**Step 2: Update struct definition**

Replace the struct at lines 44-48:

```go
// OperationPostgreSQLRepository is a Postgresql-specific implementation of the OperationRepository.
type OperationPostgreSQLRepository struct {
	connection *libPostgres.PostgresConnection
	wrapper    *mmigration.MigrationWrapper // For future health checks
	tableName  string
}
```

**Step 3: Update constructor**

Replace the constructor at lines 79-92:

```go
// NewOperationPostgreSQLRepository returns a new instance of OperationPostgreSQLRepository using the given MigrationWrapper.
func NewOperationPostgreSQLRepository(mw *mmigration.MigrationWrapper) *OperationPostgreSQLRepository {
	assert.NotNil(mw, "MigrationWrapper must not be nil", "repository", "OperationPostgreSQLRepository")

	pc := mw.GetConnection()
	assert.NotNil(pc, "PostgresConnection from wrapper must not be nil", "repository", "OperationPostgreSQLRepository")

	return &OperationPostgreSQLRepository{
		connection: pc,
		wrapper:    mw,
		tableName:  "operation",
	}
}
```

**Step 4: Verify compilation**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./components/transaction/...`

**Expected output:**
```
(no output - successful compilation)
```

---

### Task 3: Update Balance Repository Constructor

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/postgres/balance/balance.postgresql.go`

**Prerequisites:**
- Tasks 1-2 completed

**Step 1: Add mmigration import**

**Step 2: Update struct to include wrapper field**

Find the struct definition and add:
```go
wrapper    *mmigration.MigrationWrapper // For future health checks
```

**Step 3: Update constructor signature and body**

Change from `NewBalancePostgreSQLRepository(pc *libPostgres.PostgresConnection)` to:

```go
func NewBalancePostgreSQLRepository(mw *mmigration.MigrationWrapper) *BalancePostgreSQLRepository {
	assert.NotNil(mw, "MigrationWrapper must not be nil", "repository", "BalancePostgreSQLRepository")

	pc := mw.GetConnection()
	assert.NotNil(pc, "PostgresConnection from wrapper must not be nil", "repository", "BalancePostgreSQLRepository")

	return &BalancePostgreSQLRepository{
		connection: pc,
		wrapper:    mw,
		tableName:  "balance",
	}
}
```

**Step 4: Verify compilation**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./components/transaction/...`

---

### Task 4: Update AssetRate Repository Constructor

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/postgres/assetrate/assetrate.postgresql.go`

**Prerequisites:**
- Tasks 1-3 completed

**Step 1: Add mmigration import**

**Step 2: Update struct to include wrapper field**

**Step 3: Update constructor**

```go
func NewAssetRatePostgreSQLRepository(mw *mmigration.MigrationWrapper) *AssetRatePostgreSQLRepository {
	assert.NotNil(mw, "MigrationWrapper must not be nil", "repository", "AssetRatePostgreSQLRepository")

	pc := mw.GetConnection()
	assert.NotNil(pc, "PostgresConnection from wrapper must not be nil", "repository", "AssetRatePostgreSQLRepository")

	return &AssetRatePostgreSQLRepository{
		connection: pc,
		wrapper:    mw,
		tableName:  "asset_rate",
	}
}
```

**Step 4: Verify compilation**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./components/transaction/...`

---

### Task 5: Update OperationRoute Repository Constructor

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/postgres/operationroute/operationroute.postgresql.go`

**Prerequisites:**
- Tasks 1-4 completed

**Step 1-4: Same pattern as previous tasks**

Update imports, struct, and constructor to accept MigrationWrapper:

```go
func NewOperationRoutePostgreSQLRepository(mw *mmigration.MigrationWrapper) *OperationRoutePostgreSQLRepository {
	assert.NotNil(mw, "MigrationWrapper must not be nil", "repository", "OperationRoutePostgreSQLRepository")

	pc := mw.GetConnection()
	assert.NotNil(pc, "PostgresConnection from wrapper must not be nil", "repository", "OperationRoutePostgreSQLRepository")

	return &OperationRoutePostgreSQLRepository{
		connection: pc,
		wrapper:    mw,
		tableName:  "operation_route",
	}
}
```

**Verify compilation after changes.**

---

### Task 6: Update TransactionRoute Repository Constructor

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/postgres/transactionroute/transactionroute.postgresql.go`

**Prerequisites:**
- Tasks 1-5 completed

**Step 1-4: Same pattern as previous tasks**

```go
func NewTransactionRoutePostgreSQLRepository(mw *mmigration.MigrationWrapper) *TransactionRoutePostgreSQLRepository {
	assert.NotNil(mw, "MigrationWrapper must not be nil", "repository", "TransactionRoutePostgreSQLRepository")

	pc := mw.GetConnection()
	assert.NotNil(pc, "PostgresConnection from wrapper must not be nil", "repository", "TransactionRoutePostgreSQLRepository")

	return &TransactionRoutePostgreSQLRepository{
		connection: pc,
		wrapper:    mw,
		tableName:  "transaction_route",
	}
}
```

---

### Task 7: Update Outbox Repository Constructor

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/adapters/postgres/outbox/outbox.postgresql.go`

**Prerequisites:**
- Tasks 1-6 completed

**Step 1-4: Same pattern as previous tasks**

```go
func NewOutboxPostgreSQLRepository(mw *mmigration.MigrationWrapper) *OutboxPostgreSQLRepository {
	assert.NotNil(mw, "MigrationWrapper must not be nil", "repository", "OutboxPostgreSQLRepository")

	pc := mw.GetConnection()
	assert.NotNil(pc, "PostgresConnection from wrapper must not be nil", "repository", "OutboxPostgreSQLRepository")

	return &OutboxPostgreSQLRepository{
		connection: pc,
		wrapper:    mw,
		tableName:  "outbox",
	}
}
```

---

### Task 8: Update Transaction Bootstrap Configuration

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/internal/bootstrap/config.go:280-289`

**Prerequisites:**
- Tasks 1-7 completed (all transaction repos updated)

**Step 1: Update repository instantiation**

Replace lines 280-289 where repositories are created:

```go
	transactionPostgreSQLRepository := transaction.NewTransactionPostgreSQLRepository(migrationWrapper)
	operationPostgreSQLRepository := operation.NewOperationPostgreSQLRepository(migrationWrapper)
	assetRatePostgreSQLRepository := assetrate.NewAssetRatePostgreSQLRepository(migrationWrapper)
	balancePostgreSQLRepository := balance.NewBalancePostgreSQLRepository(migrationWrapper)
	operationRoutePostgreSQLRepository := operationroute.NewOperationRoutePostgreSQLRepository(migrationWrapper)
	transactionRoutePostgreSQLRepository := transactionroute.NewTransactionRoutePostgreSQLRepository(migrationWrapper)
```

**Step 2: Update outbox repository instantiation**

Find line 289 and update:

```go
	outboxPostgreSQLRepository := outbox.NewOutboxPostgreSQLRepository(migrationWrapper)
```

**Step 3: Update DBProvider to use wrapper's connection**

Replace lines 324-331:

```go
	// Get DB connection from migration wrapper for transaction management in UseCase
	// This ensures DBProvider uses the same validated connection as repositories
	dbConn, err := migrationWrapper.GetConnection().GetDB()
	assert.NoError(err, "database connection required for UseCase DBProvider",
		"package", "bootstrap",
		"function", "InitServers")

	// Wrap dbresolver.DB to implement dbtx.TxBeginner interface
	dbProvider := &dbProviderAdapter{db: dbConn}
```

**Step 4: Verify compilation**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./components/transaction/...`

**Expected output:**
```
(no output - successful compilation)
```

---

### Task 9: Run Transaction Component Tests

**Files:**
- Test files in: `/Users/fredamaral/repos/lerianstudio/midaz/components/transaction/...`

**Prerequisites:**
- Task 8 completed

**Step 1: Run unit tests**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./components/transaction/... -v -short 2>&1 | head -100`

**Expected output:**
```
=== RUN   TestXxx
--- PASS: TestXxx (0.xxs)
...
PASS
ok      github.com/LerianStudio/midaz/v3/components/transaction/...
```

**If tests fail:**
1. Regenerate mocks: `go generate ./components/transaction/internal/adapters/postgres/...`
2. Check test files for direct PostgresConnection usage that needs updating
3. Verify test setup creates MigrationWrapper mocks correctly

---

### Task 10: Run Code Review - Transaction Phase

**Prerequisites:**
- Task 9 completed (tests passing)

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

**If Task Fails:**
1. **Review finds Critical issues:** Fix immediately, re-run review
2. **Can't resolve issue:** Document blocker and stop

---

## Phase 2: Onboarding Component Refactoring

### Task 11: Update Organization Repository Constructor

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/adapters/postgres/organization/organization.postgresql.go:56-75`

**Prerequisites:**
- Phase 1 completed (Transaction component)

**Step 1: Add mmigration import**

```go
"github.com/LerianStudio/midaz/v3/pkg/mmigration"
```

**Step 2: Update struct**

```go
// OrganizationPostgreSQLRepository is a Postgresql-specific implementation of the OrganizationRepository.
type OrganizationPostgreSQLRepository struct {
	connection *libPostgres.PostgresConnection
	wrapper    *mmigration.MigrationWrapper // For future health checks
	tableName  string
}
```

**Step 3: Update constructor**

```go
// NewOrganizationPostgreSQLRepository returns a new instance of OrganizationPostgresRepository using the given MigrationWrapper.
func NewOrganizationPostgreSQLRepository(mw *mmigration.MigrationWrapper) *OrganizationPostgreSQLRepository {
	assert.NotNil(mw, "MigrationWrapper must not be nil", "repository", "OrganizationPostgreSQLRepository")

	pc := mw.GetConnection()
	assert.NotNil(pc, "PostgresConnection from wrapper must not be nil", "repository", "OrganizationPostgreSQLRepository")

	return &OrganizationPostgreSQLRepository{
		connection: pc,
		wrapper:    mw,
		tableName:  "organization",
	}
}
```

**Step 4: Verify compilation**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./components/onboarding/...`

---

### Task 12: Update Ledger Repository Constructor

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/adapters/postgres/ledger/ledger.postgresql.go`

**Prerequisites:**
- Task 11 completed

**Step 1-4: Same pattern**

```go
func NewLedgerPostgreSQLRepository(mw *mmigration.MigrationWrapper) *LedgerPostgreSQLRepository {
	assert.NotNil(mw, "MigrationWrapper must not be nil", "repository", "LedgerPostgreSQLRepository")

	pc := mw.GetConnection()
	assert.NotNil(pc, "PostgresConnection from wrapper must not be nil", "repository", "LedgerPostgreSQLRepository")

	return &LedgerPostgreSQLRepository{
		connection: pc,
		wrapper:    mw,
		tableName:  "ledger",
	}
}
```

---

### Task 13: Update Segment Repository Constructor

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/adapters/postgres/segment/segment.postgresql.go:58-77`

**Prerequisites:**
- Task 12 completed

**Step 1-4: Same pattern**

```go
func NewSegmentPostgreSQLRepository(mw *mmigration.MigrationWrapper) *SegmentPostgreSQLRepository {
	assert.NotNil(mw, "MigrationWrapper must not be nil", "repository", "SegmentPostgreSQLRepository")

	pc := mw.GetConnection()
	assert.NotNil(pc, "PostgresConnection from wrapper must not be nil", "repository", "SegmentPostgreSQLRepository")

	return &SegmentPostgreSQLRepository{
		connection: pc,
		wrapper:    mw,
		tableName:  "segment",
	}
}
```

---

### Task 14: Update Portfolio Repository Constructor

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/adapters/postgres/portfolio/portfolio.postgresql.go`

**Prerequisites:**
- Task 13 completed

**Step 1-4: Same pattern**

```go
func NewPortfolioPostgreSQLRepository(mw *mmigration.MigrationWrapper) *PortfolioPostgreSQLRepository {
	assert.NotNil(mw, "MigrationWrapper must not be nil", "repository", "PortfolioPostgreSQLRepository")

	pc := mw.GetConnection()
	assert.NotNil(pc, "PostgresConnection from wrapper must not be nil", "repository", "PortfolioPostgreSQLRepository")

	return &PortfolioPostgreSQLRepository{
		connection: pc,
		wrapper:    mw,
		tableName:  "portfolio",
	}
}
```

---

### Task 15: Update Account Repository Constructor

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/adapters/postgres/account/account.postgresql.go`

**Prerequisites:**
- Task 14 completed

**Step 1-4: Same pattern**

```go
func NewAccountPostgreSQLRepository(mw *mmigration.MigrationWrapper) *AccountPostgreSQLRepository {
	assert.NotNil(mw, "MigrationWrapper must not be nil", "repository", "AccountPostgreSQLRepository")

	pc := mw.GetConnection()
	assert.NotNil(pc, "PostgresConnection from wrapper must not be nil", "repository", "AccountPostgreSQLRepository")

	return &AccountPostgreSQLRepository{
		connection: pc,
		wrapper:    mw,
		tableName:  "account",
	}
}
```

---

### Task 16: Update Asset Repository Constructor

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/adapters/postgres/asset/asset.postgresql.go`

**Prerequisites:**
- Task 15 completed

**Step 1-4: Same pattern**

```go
func NewAssetPostgreSQLRepository(mw *mmigration.MigrationWrapper) *AssetPostgreSQLRepository {
	assert.NotNil(mw, "MigrationWrapper must not be nil", "repository", "AssetPostgreSQLRepository")

	pc := mw.GetConnection()
	assert.NotNil(pc, "PostgresConnection from wrapper must not be nil", "repository", "AssetPostgreSQLRepository")

	return &AssetPostgreSQLRepository{
		connection: pc,
		wrapper:    mw,
		tableName:  "asset",
	}
}
```

---

### Task 17: Update AccountType Repository Constructor

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/adapters/postgres/accounttype/accounttype.postgresql.go`

**Prerequisites:**
- Task 16 completed

**Step 1-4: Same pattern**

```go
func NewAccountTypePostgreSQLRepository(mw *mmigration.MigrationWrapper) *AccountTypePostgreSQLRepository {
	assert.NotNil(mw, "MigrationWrapper must not be nil", "repository", "AccountTypePostgreSQLRepository")

	pc := mw.GetConnection()
	assert.NotNil(pc, "PostgresConnection from wrapper must not be nil", "repository", "AccountTypePostgreSQLRepository")

	return &AccountTypePostgreSQLRepository{
		connection: pc,
		wrapper:    mw,
		tableName:  "account_type",
	}
}
```

---

### Task 18: Update Onboarding Bootstrap - InitServers

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/bootstrap/config.go:235-241`

**Prerequisites:**
- Tasks 11-17 completed (all onboarding repos updated)

**Step 1: Update repository instantiation in InitServers()**

Replace lines 235-241:

```go
	organizationPostgreSQLRepository := organization.NewOrganizationPostgreSQLRepository(migrationWrapper)
	ledgerPostgreSQLRepository := ledger.NewLedgerPostgreSQLRepository(migrationWrapper)
	segmentPostgreSQLRepository := segment.NewSegmentPostgreSQLRepository(migrationWrapper)
	portfolioPostgreSQLRepository := portfolio.NewPortfolioPostgreSQLRepository(migrationWrapper)
	accountPostgreSQLRepository := account.NewAccountPostgreSQLRepository(migrationWrapper)
	assetPostgreSQLRepository := asset.NewAssetPostgreSQLRepository(migrationWrapper)
	accountTypePostgreSQLRepository := accounttype.NewAccountTypePostgreSQLRepository(migrationWrapper)
```

**Step 2: Verify compilation**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./components/onboarding/...`

---

### Task 19: Update Onboarding Bootstrap - InitServersWithOptions

**Files:**
- Modify: `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/internal/bootstrap/config.go:521-527`

**Prerequisites:**
- Task 18 completed

**Note:** Onboarding component does NOT use DBProvider pattern like Transaction does. Only repository instantiation needs updating - no DBProvider changes required.

**Step 1: Update repository instantiation in InitServersWithOptions()**

Replace lines 521-527:

```go
	organizationPostgreSQLRepository := organization.NewOrganizationPostgreSQLRepository(migrationWrapper)
	ledgerPostgreSQLRepository := ledger.NewLedgerPostgreSQLRepository(migrationWrapper)
	segmentPostgreSQLRepository := segment.NewSegmentPostgreSQLRepository(migrationWrapper)
	portfolioPostgreSQLRepository := portfolio.NewPortfolioPostgreSQLRepository(migrationWrapper)
	accountPostgreSQLRepository := account.NewAccountPostgreSQLRepository(migrationWrapper)
	assetPostgreSQLRepository := asset.NewAssetPostgreSQLRepository(migrationWrapper)
	accountTypePostgreSQLRepository := accounttype.NewAccountTypePostgreSQLRepository(migrationWrapper)
```

**Step 2: Verify compilation**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./components/onboarding/...`

---

### Task 20: Run Onboarding Component Tests

**Files:**
- Test files in: `/Users/fredamaral/repos/lerianstudio/midaz/components/onboarding/...`

**Prerequisites:**
- Task 19 completed

**Step 1: Run unit tests**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./components/onboarding/... -v -short 2>&1 | head -100`

**Expected output:**
```
=== RUN   TestXxx
--- PASS: TestXxx (0.xxs)
...
PASS
ok      github.com/LerianStudio/midaz/v3/components/onboarding/...
```

**If tests fail:**
1. Regenerate mocks: `go generate ./components/onboarding/internal/adapters/postgres/...`
2. Check test files for direct PostgresConnection usage that needs updating
3. Verify test setup creates MigrationWrapper mocks correctly

---

### Task 21: Run Code Review - Onboarding Phase

**Prerequisites:**
- Task 20 completed (tests passing)

1. **Dispatch all 3 reviewers in parallel:**
   - REQUIRED SUB-SKILL: Use requesting-code-review
   - All reviewers run simultaneously
   - Wait for all to complete

2. **Handle findings by severity (MANDATORY):**

**Critical/High/Medium Issues:** Fix immediately
**Low Issues:** Add TODO(review): comments
**Cosmetic Issues:** Add FIXME(nitpick): comments

3. **Proceed only when:** Zero Critical/High/Medium issues remain

---

## Phase 3: Integration and Final Verification

### Task 22: Run Full Build

**Prerequisites:**
- Tasks 1-21 completed

**Step 1: Build entire project**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go build ./...`

**Expected output:**
```
(no output - successful compilation)
```

**Step 2: Run linter**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && golangci-lint run --timeout=5m ./... 2>&1 | head -50`

**Expected output:**
```
(no new lint errors introduced by these changes)
```

---

### Task 23: Run Integration Tests

**Prerequisites:**
- Task 22 completed
- Local database running

**Step 1: Run integration tests**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && go test ./... -v -tags=integration -short 2>&1 | head -200`

**Expected output:**
```
=== RUN   TestXxx
--- PASS: TestXxx
...
```

**If tests fail:**
- Check database connectivity
- Verify migrations have run
- Check test logs for specific errors

---

### Task 24: Final Code Review

**Prerequisites:**
- Task 23 completed (integration tests passing)

1. **Dispatch all 3 reviewers in parallel:**
   - REQUIRED SUB-SKILL: Use requesting-code-review
   - Focus on: constructor patterns, import consistency, no SafeGetDB() in hot paths
   - Wait for all to complete

2. **Handle findings by severity (MANDATORY):**

**Critical/High/Medium Issues:** Fix immediately
**Low Issues:** Add TODO(review): comments
**Cosmetic Issues:** Add FIXME(nitpick): comments

3. **Proceed only when:** Zero Critical/High/Medium issues remain

---

### Task 25: Commit Changes

**Prerequisites:**
- Task 24 completed (code review passed)

**Step 1: Stage changes**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && git add components/transaction/internal/adapters/postgres/*/\*.go components/onboarding/internal/adapters/postgres/*/\*.go components/*/internal/bootstrap/config.go`

**Step 2: Create commit**

Run:
```bash
cd /Users/fredamaral/repos/lerianstudio/midaz && git commit -m "$(cat <<'EOF'
refactor(postgres): update repository constructors to accept MigrationWrapper

Update all 14 PostgreSQL repository constructors across transaction and
onboarding components to receive MigrationWrapper instead of PostgresConnection.

Architecture:
- Repositories extract underlying PostgresConnection via GetConnection()
- Normal DB operations use fast-path connection.GetDB()
- Wrapper retained for future health check integration
- No SafeGetDB() calls in hot paths (bootstrap validates once at startup)

This change enables future health check integration while maintaining
zero overhead for normal database operations.

Affected repositories:
- Transaction: transaction, operation, balance, assetrate, operationroute,
  transactionroute, outbox
- Onboarding: organization, ledger, segment, portfolio, account, asset,
  accounttype
EOF
)"
```

**Step 3: Verify commit**

Run: `cd /Users/fredamaral/repos/lerianstudio/midaz && git log -1 --oneline`

**Expected output:**
```
<hash> refactor(postgres): update repository constructors to accept MigrationWrapper
```

---

## Summary

**Total Tasks:** 25
**Estimated Time:** 2-3 hours

**Key Architecture Points:**
1. MigrationWrapper is passed to constructors for DI
2. PostgresConnection extracted via GetConnection() for normal operations
3. NO SafeGetDB() calls except during bootstrap validation
4. Normal operations use connection.GetDB() - zero overhead fast path
5. Wrapper retained for future health check integration

**Files Modified:**
- 7 transaction repositories
- 7 onboarding repositories
- 2 bootstrap configurations

**If Task Fails:**

1. **Compilation errors:**
   - Check import statements
   - Verify struct field names match
   - Check constructor return types

2. **Test failures:**
   - Run `go generate ./...` to update mocks
   - Check test files for direct PostgresConnection usage
   - Update test setup to use MigrationWrapper

3. **Can't recover:**
   - Document what failed and why
   - Stop and return to human partner
   - Don't try to fix without understanding root cause
