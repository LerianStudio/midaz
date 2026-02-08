# Primary-Read Fallback Pattern Implementation Plan

> **For Agents:** REQUIRED SUB-SKILL: Use ring:executing-plans to implement this plan task-by-task.

**Goal:** Add fallback-to-primary read methods in the transaction repository so that cancel/commit operations succeed even when the read replica has not yet replicated a newly created transaction (eliminates 404 errors caused by replication lag).

**Architecture:** New `FindWithFallback` and `FindWithOperationsWithFallback` methods on the repository try the replica first (via `GetDB()`). If the replica returns `EntityNotFoundError`, the methods retry the same query against the primary database (via `GetDB().PrimaryDBs()[0]`). All existing methods remain unchanged. The query service and HTTP handler layers gain thin `WithFallback` variants that delegate to these new repository methods.

**Tech Stack:** Go 1.21+, PostgreSQL (primary + replica via `lib-commons/v2/commons/postgres`), `bxcodec/dbresolver/v2`, `squirrel` query builder, `gomock` for tests, `testify` for assertions.

**Global Prerequisites:**
- Environment: macOS or Linux, Go 1.21+
- Tools: Go compiler (`go version`), `mockgen` (`mockgen --version`)
- Repository: `github.com/LerianStudio/midaz` checked out at branch `main`
- Working tree: clean (`git status` shows no uncommitted changes)
- Design document read: `docs/plans/2026-02-08-primary-read-fallback-pattern-design.md`

**Verification before starting:**
```bash
# Run ALL these commands and verify output:
go version          # Expected: go1.21+ (any 1.21.x or higher)
git status          # Expected: clean working tree on main or feature branch
mockgen --version   # Expected: version line (any version)
```

**Key Insight -- How Primary/Replica Works in This Codebase:**
- `r.connection.GetDB()` returns a `dbresolver.DB` from `github.com/bxcodec/dbresolver/v2`
- Read queries (`QueryContext`, `QueryRowContext`) are automatically routed to the **replica**
- Write queries (`ExecContext`) are automatically routed to the **primary**
- To force a read from the **primary**, use: `r.connection.GetDB().PrimaryDBs()[0].QueryRowContext(...)`
- The `PrimaryDBs()` method returns `[]*sql.DB` -- the first element is the primary database
- Error detection: When `Find()` gets `sql.ErrNoRows`, it returns `pkg.EntityNotFoundError` via `pkg.ValidateBusinessError(constant.ErrEntityNotFound, ...)`
- To check if an error is "not found", use: `errors.As(err, &pkg.EntityNotFoundError{})`

---

## Task 1: Add `FindWithFallback` to the Repository Interface

**Files:**
- Modify: `/Users/lazari/github/_0000_ext_repositories/midaz/components/transaction/internal/adapters/postgres/transaction/transaction.postgresql.go` (line 67-77, the `Repository` interface)

**Prerequisites:**
- File must exist and be readable
- No other uncommitted changes to this file

**Step 1: Write the failing test**

This task only adds interface methods -- the test is that the code compiles with the new interface. The actual test for the implementation comes in Task 3. For now, we verify the mock generation fails (proving we need to regenerate mocks).

**Step 2: Add two new methods to the Repository interface**

Open `/Users/lazari/github/_0000_ext_repositories/midaz/components/transaction/internal/adapters/postgres/transaction/transaction.postgresql.go` and locate the `Repository` interface (lines 67-77). Add two new methods at the end, before the closing `}`:

```go
// Repository provides an interface for operations related to transaction template entities.
// It defines methods for creating, retrieving, updating, and deleting transactions.
type Repository interface {
	Create(ctx context.Context, transaction *Transaction) (*Transaction, error)
	FindAll(ctx context.Context, organizationID, ledgerID uuid.UUID, filter http.Pagination) ([]*Transaction, libHTTP.CursorPagination, error)
	Find(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*Transaction, error)
	FindByParentID(ctx context.Context, organizationID, ledgerID, parentID uuid.UUID) (*Transaction, error)
	ListByIDs(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID) ([]*Transaction, error)
	Update(ctx context.Context, organizationID, ledgerID, id uuid.UUID, transaction *Transaction) (*Transaction, error)
	Delete(ctx context.Context, organizationID, ledgerID, id uuid.UUID) error
	FindWithOperations(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*Transaction, error)
	FindOrListAllWithOperations(ctx context.Context, organizationID, ledgerID uuid.UUID, ids []uuid.UUID, filter http.Pagination) ([]*Transaction, libHTTP.CursorPagination, error)
	FindWithFallback(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*Transaction, error)
	FindWithOperationsWithFallback(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*Transaction, error)
}
```

The two new lines are:
```go
	FindWithFallback(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*Transaction, error)
	FindWithOperationsWithFallback(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*Transaction, error)
```

**Step 3: Verify the file compiles (it will fail because the struct does not implement the new methods yet)**

Run:
```bash
cd /Users/lazari/github/_0000_ext_repositories/midaz && go build ./components/transaction/...
```

**Expected output:**
```
# github.com/LerianStudio/midaz/v3/components/transaction/...
... TransactionPostgreSQLRepository does not implement Repository (missing method FindWithFallback)
```

This failure is expected. We will fix it in Task 2.

**Step 4: Commit the interface change**

```bash
git add components/transaction/internal/adapters/postgres/transaction/transaction.postgresql.go
git commit -m "feat(transaction): add FindWithFallback and FindWithOperationsWithFallback to Repository interface"
```

**If Task Fails:**

1. **File not found:** Verify path is correct: `ls components/transaction/internal/adapters/postgres/transaction/transaction.postgresql.go`
2. **Syntax error:** Check that the two new lines end with correct parentheses and return types
3. **Rollback:** `git checkout -- components/transaction/internal/adapters/postgres/transaction/transaction.postgresql.go`

---

## Task 2: Implement `FindWithFallback` on the PostgreSQL Repository

**Files:**
- Modify: `/Users/lazari/github/_0000_ext_repositories/midaz/components/transaction/internal/adapters/postgres/transaction/transaction.postgresql.go` (add new method after the existing `Find` method, around line 505)

**Prerequisites:**
- Task 1 is complete (interface has new methods)

**Step 1: Add the `FindWithFallback` method**

Add the following method to `TransactionPostgreSQLRepository` after the existing `Find` method (after line 505 in the file). This method calls the existing `Find()` (which reads from the replica via `GetDB()`), and if it gets an `EntityNotFoundError`, it retries against the primary database directly.

```go
// FindWithFallback retrieves a Transaction entity, trying the replica first and falling back to the primary on NotFound.
// This handles replication lag scenarios where a transaction was just created on the primary but not yet visible on the replica.
func (r *TransactionPostgreSQLRepository) FindWithFallback(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*Transaction, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_transaction_with_fallback")
	defer span.End()

	// Step 1: Try reading from the replica (default behavior of Find)
	tran, err := r.Find(ctx, organizationID, ledgerID, id)
	if err == nil {
		return tran, nil
	}

	// Step 2: If the error is NOT a "not found" error, return it immediately (e.g., connection error)
	var entityNotFoundErr pkg.EntityNotFoundError
	if !errors.As(err, &entityNotFoundErr) {
		return nil, err
	}

	// Step 3: Replica returned "not found" -- fall back to primary
	logger.Infof("Replica miss for transaction %s, falling back to primary read", id.String())

	db, dbErr := r.connection.GetDB()
	if dbErr != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection for primary fallback", dbErr)
		logger.Errorf("Failed to get database connection for primary fallback: %v", dbErr)
		return nil, dbErr
	}

	primaryDBs := db.PrimaryDBs()
	if len(primaryDBs) == 0 {
		libOpentelemetry.HandleSpanError(&span, "No primary database available for fallback", err)
		logger.Errorf("No primary database available for fallback")
		return nil, err
	}

	primaryDB := primaryDBs[0]

	transaction := &TransactionPostgreSQLModel{}
	var body *string

	ctx, spanPrimary := tracer.Start(ctx, "postgres.find_transaction_with_fallback.primary_query")
	defer spanPrimary.End()

	find := squirrel.Select(transactionColumnList...).
		From(r.tableName).
		Where(squirrel.Expr("organization_id = ?", organizationID)).
		Where(squirrel.Expr("ledger_id = ?", ledgerID)).
		Where(squirrel.Expr("id = ?", id)).
		Where(squirrel.Eq{"deleted_at": nil}).
		PlaceholderFormat(squirrel.Dollar)

	query, args, queryErr := find.ToSql()
	if queryErr != nil {
		libOpentelemetry.HandleSpanError(&spanPrimary, "Failed to build primary fallback query", queryErr)
		logger.Errorf("Failed to build primary fallback query: %v", queryErr)
		return nil, queryErr
	}

	row := primaryDB.QueryRowContext(ctx, query, args...)

	if scanErr := row.Scan(
		&transaction.ID,
		&transaction.ParentTransactionID,
		&transaction.Description,
		&transaction.Status,
		&transaction.StatusDescription,
		&transaction.Amount,
		&transaction.AssetCode,
		&transaction.ChartOfAccountsGroupName,
		&transaction.LedgerID,
		&transaction.OrganizationID,
		&body,
		&transaction.CreatedAt,
		&transaction.UpdatedAt,
		&transaction.DeletedAt,
		&transaction.Route,
	); scanErr != nil {
		if errors.Is(scanErr, sql.ErrNoRows) {
			// Not found on primary either -- return the original not-found error
			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanPrimary, "Transaction not found on primary fallback", err)
			logger.Infof("Transaction %s not found on primary fallback either", id.String())
			return nil, err
		}

		libOpentelemetry.HandleSpanError(&spanPrimary, "Failed to scan row from primary fallback", scanErr)
		logger.Errorf("Failed to scan row from primary fallback: %v", scanErr)
		return nil, scanErr
	}

	if !libCommons.IsNilOrEmpty(body) {
		if unmarshalErr := json.Unmarshal([]byte(*body), &transaction.Body); unmarshalErr != nil {
			libOpentelemetry.HandleSpanError(&spanPrimary, "Failed to unmarshal body from primary fallback", unmarshalErr)
			logger.Errorf("Failed to unmarshal body from primary fallback: %v", unmarshalErr)
			return nil, unmarshalErr
		}
	}

	logger.Infof("Transaction %s found on primary fallback (replica lag detected)", id.String())

	return transaction.ToEntity(), nil
}
```

**Step 2: Verify the method compiles (will still fail because `FindWithOperationsWithFallback` is missing)**

Run:
```bash
cd /Users/lazari/github/_0000_ext_repositories/midaz && go build ./components/transaction/...
```

**Expected output:**
```
... TransactionPostgreSQLRepository does not implement Repository (missing method FindWithOperationsWithFallback)
```

This is expected -- Task 3 adds the other method.

**Step 3: Commit**

```bash
git add components/transaction/internal/adapters/postgres/transaction/transaction.postgresql.go
git commit -m "feat(transaction): implement FindWithFallback with primary database fallback on replica miss"
```

**If Task Fails:**

1. **Import missing:** Ensure `"github.com/LerianStudio/midaz/v3/pkg"` is imported at the top of the file (it is already imported as `pkg` on line 19)
2. **PrimaryDBs() not found:** This method is from `dbresolver.DB`. Verify the import by checking go.mod: `github.com/bxcodec/dbresolver/v2`
3. **Rollback:** `git checkout -- components/transaction/internal/adapters/postgres/transaction/transaction.postgresql.go`

---

## Task 3: Implement `FindWithOperationsWithFallback` on the PostgreSQL Repository

**Files:**
- Modify: `/Users/lazari/github/_0000_ext_repositories/midaz/components/transaction/internal/adapters/postgres/transaction/transaction.postgresql.go` (add new method after `FindWithFallback`)

**Prerequisites:**
- Task 2 is complete

**Step 1: Add the `FindWithOperationsWithFallback` method**

Add the following method after `FindWithFallback`. This method calls the existing `FindWithOperations()` first, and if it returns an empty transaction (ID is empty string -- meaning no rows), it retries on the primary.

**Important behavioral difference:** `FindWithOperations` does NOT return an error when no transaction is found. Instead, it returns a `*Transaction` with empty fields (ID = "", no operations). The fallback must detect this by checking `tran.ID == ""`.

```go
// FindWithOperationsWithFallback retrieves a Transaction with its Operations, trying the replica first
// and falling back to the primary on empty result (replication lag scenario).
func (r *TransactionPostgreSQLRepository) FindWithOperationsWithFallback(ctx context.Context, organizationID, ledgerID, id uuid.UUID) (*Transaction, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "postgres.find_transaction_with_operations_with_fallback")
	defer span.End()

	// Step 1: Try reading from the replica (default behavior of FindWithOperations)
	tran, err := r.FindWithOperations(ctx, organizationID, ledgerID, id)
	if err != nil {
		// Non-recoverable error (connection issue, query error) -- return immediately
		return nil, err
	}

	// Step 2: If transaction was found on replica, return it
	if tran != nil && tran.ID != "" {
		return tran, nil
	}

	// Step 3: Replica returned empty result -- fall back to primary
	logger.Infof("Replica miss for transaction with operations %s, falling back to primary read", id.String())

	db, dbErr := r.connection.GetDB()
	if dbErr != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection for primary fallback", dbErr)
		logger.Errorf("Failed to get database connection for primary fallback: %v", dbErr)
		return nil, dbErr
	}

	primaryDBs := db.PrimaryDBs()
	if len(primaryDBs) == 0 {
		libOpentelemetry.HandleSpanError(&span, "No primary database available for fallback", nil)
		logger.Errorf("No primary database available for fallback")
		return tran, nil
	}

	primaryDB := primaryDBs[0]

	ctx, spanPrimary := tracer.Start(ctx, "postgres.find_transaction_with_operations_with_fallback.primary_query")
	defer spanPrimary.End()

	operationColumnListPrefixed := []string{
		"o.id", "o.transaction_id", "o.description", "o.type", "o.asset_code",
		"o.amount", "o.available_balance", "o.on_hold_balance", "o.available_balance_after",
		"o.on_hold_balance_after", "o.status", "o.status_description", "o.account_id",
		"o.account_alias", "o.balance_id", "o.chart_of_accounts", "o.organization_id",
		"o.ledger_id", "o.created_at", "o.updated_at", "o.deleted_at", "o.route",
		"o.balance_affected", "o.balance_key", "o.balance_version_before", "o.balance_version_after",
	}

	selectColumns := append(transactionColumnListPrefixed, operationColumnListPrefixed...)

	findWithOps := squirrel.Select(selectColumns...).
		From(r.tableName + " t").
		InnerJoin("operation o ON t.id = o.transaction_id").
		Where(squirrel.Expr("t.organization_id = ?", organizationID)).
		Where(squirrel.Expr("t.ledger_id = ?", ledgerID)).
		Where(squirrel.Expr("t.id = ?", id)).
		Where(squirrel.Eq{"t.deleted_at": nil}).
		PlaceholderFormat(squirrel.Dollar)

	query, args, queryErr := findWithOps.ToSql()
	if queryErr != nil {
		libOpentelemetry.HandleSpanError(&spanPrimary, "Failed to build primary fallback query", queryErr)
		logger.Errorf("Failed to build primary fallback query: %v", queryErr)
		return nil, queryErr
	}

	rows, queryExecErr := primaryDB.QueryContext(ctx, query, args...)
	if queryExecErr != nil {
		libOpentelemetry.HandleSpanError(&spanPrimary, "Failed to execute primary fallback query", queryExecErr)
		logger.Errorf("Failed to execute primary fallback query: %v", queryExecErr)
		return nil, queryExecErr
	}
	defer rows.Close()

	newTransaction := &Transaction{}
	operations := make([]*operation.Operation, 0)

	for rows.Next() {
		tranModel := &TransactionPostgreSQLModel{}
		op := operation.OperationPostgreSQLModel{}

		var body *string

		if scanErr := rows.Scan(
			&tranModel.ID,
			&tranModel.ParentTransactionID,
			&tranModel.Description,
			&tranModel.Status,
			&tranModel.StatusDescription,
			&tranModel.Amount,
			&tranModel.AssetCode,
			&tranModel.ChartOfAccountsGroupName,
			&tranModel.LedgerID,
			&tranModel.OrganizationID,
			&body,
			&tranModel.CreatedAt,
			&tranModel.UpdatedAt,
			&tranModel.DeletedAt,
			&tranModel.Route,
			&op.ID,
			&op.TransactionID,
			&op.Description,
			&op.Type,
			&op.AssetCode,
			&op.Amount,
			&op.AvailableBalance,
			&op.OnHoldBalance,
			&op.AvailableBalanceAfter,
			&op.OnHoldBalanceAfter,
			&op.Status,
			&op.StatusDescription,
			&op.AccountID,
			&op.AccountAlias,
			&op.BalanceID,
			&op.ChartOfAccounts,
			&op.OrganizationID,
			&op.LedgerID,
			&op.CreatedAt,
			&op.UpdatedAt,
			&op.DeletedAt,
			&op.Route,
			&op.BalanceAffected,
			&op.BalanceKey,
			&op.VersionBalance,
			&op.VersionBalanceAfter,
		); scanErr != nil {
			libOpentelemetry.HandleSpanError(&spanPrimary, "Failed to scan rows from primary fallback", scanErr)
			logger.Errorf("Failed to scan rows from primary fallback: %v", scanErr)
			return nil, scanErr
		}

		if !libCommons.IsNilOrEmpty(body) {
			if unmarshalErr := json.Unmarshal([]byte(*body), &tranModel.Body); unmarshalErr != nil {
				libOpentelemetry.HandleSpanError(&spanPrimary, "Failed to unmarshal body from primary fallback", unmarshalErr)
				logger.Errorf("Failed to unmarshal body from primary fallback: %v", unmarshalErr)
				return nil, unmarshalErr
			}
		}

		newTransaction = tranModel.ToEntity()
		operations = append(operations, op.ToEntity())
	}

	if rowErr := rows.Err(); rowErr != nil {
		libOpentelemetry.HandleSpanError(&spanPrimary, "Failed to get rows from primary fallback", rowErr)
		logger.Errorf("Failed to get rows from primary fallback: %v", rowErr)
		return nil, rowErr
	}

	newTransaction.Operations = operations

	if newTransaction.ID != "" {
		logger.Infof("Transaction %s with operations found on primary fallback (replica lag detected)", id.String())
	} else {
		logger.Infof("Transaction %s with operations not found on primary fallback either", id.String())
	}

	return newTransaction, nil
}
```

**Step 2: Verify the code compiles**

Run:
```bash
cd /Users/lazari/github/_0000_ext_repositories/midaz && go build ./components/transaction/...
```

**Expected output:** Clean build with no errors. The `TransactionPostgreSQLRepository` now satisfies the full `Repository` interface.

**Step 3: Commit**

```bash
git add components/transaction/internal/adapters/postgres/transaction/transaction.postgresql.go
git commit -m "feat(transaction): implement FindWithOperationsWithFallback with primary database fallback"
```

**If Task Fails:**

1. **Import for `operation` package:** Ensure `"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"` is imported (already imported at line 18)
2. **Column count mismatch in Scan:** Compare the Scan fields with the existing `FindWithOperations` method (line 800-848) -- they must match exactly
3. **Rollback:** `git checkout -- components/transaction/internal/adapters/postgres/transaction/transaction.postgresql.go`

---

## Task 4: Regenerate the Mock for the Repository Interface

**Files:**
- Regenerate: `/Users/lazari/github/_0000_ext_repositories/midaz/components/transaction/internal/adapters/postgres/transaction/transaction.postgresql_mock.go`

**Prerequisites:**
- Tasks 1-3 are complete (interface has new methods, implementations exist, code compiles)
- `mockgen` is installed

**Step 1: Regenerate the mock**

Run from the repository root:
```bash
cd /Users/lazari/github/_0000_ext_repositories/midaz && mockgen -source=./components/transaction/internal/adapters/postgres/transaction/transaction.postgresql.go -destination=./components/transaction/internal/adapters/postgres/transaction/transaction.postgresql_mock.go -package=transaction
```

**Expected output:** No output (success). The mock file is regenerated.

**Step 2: Verify the mock has the new methods**

Run:
```bash
grep -n "FindWithFallback\|FindWithOperationsWithFallback" components/transaction/internal/adapters/postgres/transaction/transaction.postgresql_mock.go
```

**Expected output:** Multiple lines showing mock methods for both `FindWithFallback` and `FindWithOperationsWithFallback`.

**Step 3: Verify the code still compiles**

Run:
```bash
cd /Users/lazari/github/_0000_ext_repositories/midaz && go build ./components/transaction/...
```

**Expected output:** Clean build.

**Step 4: Commit**

```bash
git add components/transaction/internal/adapters/postgres/transaction/transaction.postgresql_mock.go
git commit -m "chore(transaction): regenerate Repository mock with FindWithFallback methods"
```

**If Task Fails:**

1. **mockgen not found:** Install it: `go install go.uber.org/mock/mockgen@latest`
2. **Mock has errors:** Delete the mock file and regenerate: `rm components/transaction/internal/adapters/postgres/transaction/transaction.postgresql_mock.go` then rerun the mockgen command
3. **Rollback:** `git checkout -- components/transaction/internal/adapters/postgres/transaction/transaction.postgresql_mock.go`

---

## Task 5: Add `GetTransactionByIDWithFallback` to the Query Service

**Files:**
- Modify: `/Users/lazari/github/_0000_ext_repositories/midaz/components/transaction/internal/services/query/get-id-transaction.go`

**Prerequisites:**
- Task 4 is complete (mock regenerated)

**Step 1: Add the `GetTransactionByIDWithFallback` method**

Open `/Users/lazari/github/_0000_ext_repositories/midaz/components/transaction/internal/services/query/get-id-transaction.go` and add the following method after the existing `GetTransactionWithOperationsByID` method (after line 83):

```go
// GetTransactionByIDWithFallback gets data in the repository, falling back to primary on replica miss.
// Use this for state-change operations (cancel, commit) where replication lag may cause false 404s.
func (uc *UseCase) GetTransactionByIDWithFallback(ctx context.Context, organizationID, ledgerID, transactionID uuid.UUID) (*transaction.Transaction, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "query.get_transaction_by_id_with_fallback")
	defer span.End()

	logger.Infof("Trying to get transaction with primary fallback")

	tran, err := uc.TransactionRepo.FindWithFallback(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get transaction on repo by id with fallback", err)

		logger.Errorf("Error getting transaction with fallback: %v", err)

		return nil, err
	}

	if tran != nil {
		metadata, err := uc.MetadataRepo.FindByEntity(ctx, reflect.TypeOf(transaction.Transaction{}).Name(), transactionID.String())
		if err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to get metadata on mongodb account", err)

			logger.Errorf("Error get metadata on mongodb account: %v", err)

			return nil, err
		}

		if metadata != nil {
			tran.Metadata = metadata.Data
		}
	}

	return tran, nil
}
```

**Step 2: Verify it compiles**

Run:
```bash
cd /Users/lazari/github/_0000_ext_repositories/midaz && go build ./components/transaction/...
```

**Expected output:** Clean build.

**Step 3: Commit**

```bash
git add components/transaction/internal/services/query/get-id-transaction.go
git commit -m "feat(transaction): add GetTransactionByIDWithFallback to query service"
```

**If Task Fails:**

1. **Imports missing:** The file already imports `reflect`, `libCommons`, `libOpentelemetry`, `transaction`, and `uuid` -- no new imports needed
2. **Rollback:** `git checkout -- components/transaction/internal/services/query/get-id-transaction.go`

---

## Task 6: Update `CommitTransaction` Handler to Use Fallback

**Files:**
- Modify: `/Users/lazari/github/_0000_ext_repositories/midaz/components/transaction/internal/adapters/http/in/transaction.go` (lines 310-332, `CommitTransaction` method)

**Prerequisites:**
- Task 5 is complete

**Step 1: Change the query call in CommitTransaction**

In the `CommitTransaction` method (line 322), change:
```go
	tran, err := handler.Query.GetTransactionByID(ctx, organizationID, ledgerID, transactionID)
```

to:
```go
	tran, err := handler.Query.GetTransactionByIDWithFallback(ctx, organizationID, ledgerID, transactionID)
```

This is a single-line change. The method signature and return types are identical.

**Step 2: Verify it compiles**

Run:
```bash
cd /Users/lazari/github/_0000_ext_repositories/midaz && go build ./components/transaction/...
```

**Expected output:** Clean build.

**Step 3: Commit**

```bash
git add components/transaction/internal/adapters/http/in/transaction.go
git commit -m "feat(transaction): use GetTransactionByIDWithFallback in CommitTransaction handler"
```

**If Task Fails:**

1. **Method not found:** Ensure Task 5 is complete and the query UseCase has the method
2. **Rollback:** `git checkout -- components/transaction/internal/adapters/http/in/transaction.go`

---

## Task 7: Update `CancelTransaction` Handler to Use Fallback

**Files:**
- Modify: `/Users/lazari/github/_0000_ext_repositories/midaz/components/transaction/internal/adapters/http/in/transaction.go` (lines 354-376, `CancelTransaction` method)

**Prerequisites:**
- Task 5 is complete

**Step 1: Change the query call in CancelTransaction**

In the `CancelTransaction` method (line 366), change:
```go
	tran, err := handler.Query.GetTransactionByID(ctx, organizationID, ledgerID, transactionID)
```

to:
```go
	tran, err := handler.Query.GetTransactionByIDWithFallback(ctx, organizationID, ledgerID, transactionID)
```

This is a single-line change.

**Step 2: Verify it compiles**

Run:
```bash
cd /Users/lazari/github/_0000_ext_repositories/midaz && go build ./components/transaction/...
```

**Expected output:** Clean build.

**Step 3: Commit**

```bash
git add components/transaction/internal/adapters/http/in/transaction.go
git commit -m "feat(transaction): use GetTransactionByIDWithFallback in CancelTransaction handler"
```

**If Task Fails:**

1. **Rollback:** `git checkout -- components/transaction/internal/adapters/http/in/transaction.go`

---

## Task 8: Run Code Review (Checkpoint 1 -- Repository and Handler Changes)

1. **Dispatch all 3 reviewers in parallel:**
   - REQUIRED SUB-SKILL: Use ring:requesting-code-review
   - All reviewers run simultaneously (ring:code-reviewer, ring:business-logic-reviewer, ring:security-reviewer)
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

## Task 9: Write Unit Tests for `GetTransactionByIDWithFallback` in the Query Service

**Files:**
- Modify: `/Users/lazari/github/_0000_ext_repositories/midaz/components/transaction/internal/services/query/get-id-transaction_test.go`

**Prerequisites:**
- Task 4 is complete (mock has `FindWithFallback`)
- Task 5 is complete (query method exists)

**Step 1: Add the test function**

Add the following test function at the end of `/Users/lazari/github/_0000_ext_repositories/midaz/components/transaction/internal/services/query/get-id-transaction_test.go`, after `TestGetTransactionWithOperationsByID`:

```go
func TestGetTransactionByIDWithFallback(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()

	newBaseTran := func() *transaction.Transaction {
		return &transaction.Transaction{
			ID:             transactionID.String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
		}
	}

	testMetadata := map[string]any{"key": "value", "env": "test"}

	tests := []struct {
		name            string
		setupMocks      func(mockTxRepo *transaction.MockRepository, mockMetaRepo *mongodb.MockRepository)
		expectedErr     error
		expectNilResult bool
		expectedMeta    map[string]any
	}{
		{
			name: "fallback returns transaction successfully",
			setupMocks: func(mockTxRepo *transaction.MockRepository, mockMetaRepo *mongodb.MockRepository) {
				mockTxRepo.EXPECT().
					FindWithFallback(gomock.Any(), organizationID, ledgerID, transactionID).
					Return(newBaseTran(), nil)
				mockMetaRepo.EXPECT().
					FindByEntity(gomock.Any(), reflect.TypeOf(transaction.Transaction{}).Name(), transactionID.String()).
					Return(nil, nil)
			},
			expectedErr:     nil,
			expectNilResult: false,
			expectedMeta:    nil,
		},
		{
			name: "fallback returns transaction with metadata",
			setupMocks: func(mockTxRepo *transaction.MockRepository, mockMetaRepo *mongodb.MockRepository) {
				mockTxRepo.EXPECT().
					FindWithFallback(gomock.Any(), organizationID, ledgerID, transactionID).
					Return(newBaseTran(), nil)
				mockMetaRepo.EXPECT().
					FindByEntity(gomock.Any(), reflect.TypeOf(transaction.Transaction{}).Name(), transactionID.String()).
					Return(&mongodb.Metadata{EntityID: transactionID.String(), Data: testMetadata}, nil)
			},
			expectedErr:     nil,
			expectNilResult: false,
			expectedMeta:    testMetadata,
		},
		{
			name: "fallback returns nil transaction",
			setupMocks: func(mockTxRepo *transaction.MockRepository, mockMetaRepo *mongodb.MockRepository) {
				mockTxRepo.EXPECT().
					FindWithFallback(gomock.Any(), organizationID, ledgerID, transactionID).
					Return(nil, nil)
			},
			expectedErr:     nil,
			expectNilResult: true,
			expectedMeta:    nil,
		},
		{
			name: "fallback returns repo error",
			setupMocks: func(mockTxRepo *transaction.MockRepository, mockMetaRepo *mongodb.MockRepository) {
				mockTxRepo.EXPECT().
					FindWithFallback(gomock.Any(), organizationID, ledgerID, transactionID).
					Return(nil, errors.New("database connection error"))
			},
			expectedErr:     errors.New("database connection error"),
			expectNilResult: true,
			expectedMeta:    nil,
		},
		{
			name: "fallback returns transaction but metadata repo error",
			setupMocks: func(mockTxRepo *transaction.MockRepository, mockMetaRepo *mongodb.MockRepository) {
				mockTxRepo.EXPECT().
					FindWithFallback(gomock.Any(), organizationID, ledgerID, transactionID).
					Return(newBaseTran(), nil)
				mockMetaRepo.EXPECT().
					FindByEntity(gomock.Any(), reflect.TypeOf(transaction.Transaction{}).Name(), transactionID.String()).
					Return(nil, errors.New("mongodb connection error"))
			},
			expectedErr:     errors.New("mongodb connection error"),
			expectNilResult: true,
			expectedMeta:    nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockTxRepo := transaction.NewMockRepository(ctrl)
			mockMetaRepo := mongodb.NewMockRepository(ctrl)

			tt.setupMocks(mockTxRepo, mockMetaRepo)

			uc := &UseCase{
				TransactionRepo: mockTxRepo,
				MetadataRepo:    mockMetaRepo,
			}

			result, err := uc.GetTransactionByIDWithFallback(context.Background(), organizationID, ledgerID, transactionID)

			if tt.expectedErr != nil {
				require.Error(t, err)
				assert.Equal(t, tt.expectedErr.Error(), err.Error())
				assert.Nil(t, result)
				return
			}

			require.NoError(t, err)

			if tt.expectNilResult {
				assert.Nil(t, result)
				return
			}

			require.NotNil(t, result)
			assert.Equal(t, transactionID.String(), result.ID)
			assert.Equal(t, organizationID.String(), result.OrganizationID)
			assert.Equal(t, ledgerID.String(), result.LedgerID)

			if tt.expectedMeta != nil {
				assert.Equal(t, tt.expectedMeta, result.Metadata)
			} else {
				assert.Nil(t, result.Metadata)
			}
		})
	}
}
```

**Step 2: Run the test**

Run:
```bash
cd /Users/lazari/github/_0000_ext_repositories/midaz && go test ./components/transaction/internal/services/query/ -run TestGetTransactionByIDWithFallback -v
```

**Expected output:**
```
=== RUN   TestGetTransactionByIDWithFallback
=== RUN   TestGetTransactionByIDWithFallback/fallback_returns_transaction_successfully
=== RUN   TestGetTransactionByIDWithFallback/fallback_returns_transaction_with_metadata
=== RUN   TestGetTransactionByIDWithFallback/fallback_returns_nil_transaction
=== RUN   TestGetTransactionByIDWithFallback/fallback_returns_repo_error
=== RUN   TestGetTransactionByIDWithFallback/fallback_returns_transaction_but_metadata_repo_error
--- PASS: TestGetTransactionByIDWithFallback (0.00s)
    --- PASS: ... (0.00s)
    --- PASS: ... (0.00s)
    --- PASS: ... (0.00s)
    --- PASS: ... (0.00s)
    --- PASS: ... (0.00s)
PASS
```

**Step 3: Commit**

```bash
git add components/transaction/internal/services/query/get-id-transaction_test.go
git commit -m "test(transaction): add unit tests for GetTransactionByIDWithFallback query service method"
```

**If Task Fails:**

1. **Missing imports:** Ensure the test file imports `"reflect"` -- it should already be imported
2. **Mock method not found:** Ensure Task 4 (mock regeneration) was completed
3. **Rollback:** `git checkout -- components/transaction/internal/services/query/get-id-transaction_test.go`

---

## Task 10: Run All Existing Tests to Verify No Regressions

**Files:** No files modified -- this is a verification task.

**Prerequisites:**
- All previous tasks complete

**Step 1: Run the transaction service unit tests**

Run:
```bash
cd /Users/lazari/github/_0000_ext_repositories/midaz && go test ./components/transaction/... -count=1 -short
```

**Expected output:**
```
ok  	github.com/LerianStudio/midaz/v3/components/transaction/...
```

All tests should pass. The `-short` flag skips integration tests that need Docker.

**Step 2: Run the query service tests specifically**

Run:
```bash
cd /Users/lazari/github/_0000_ext_repositories/midaz && go test ./components/transaction/internal/services/query/ -v -count=1
```

**Expected output:** All tests pass, including the new `TestGetTransactionByIDWithFallback`.

**Step 3: Run the transaction model tests**

Run:
```bash
cd /Users/lazari/github/_0000_ext_repositories/midaz && go test ./components/transaction/internal/adapters/postgres/transaction/ -v -count=1
```

**Expected output:** All existing tests pass.

**If Tests Fail:**

1. **Compilation errors:** Go back to the task that introduced the error and fix
2. **Existing test failures:** Check if the failure is pre-existing by running on a clean checkout: `git stash && go test ./components/transaction/... -short && git stash pop`
3. **Mock mismatch:** Re-run Task 4 to regenerate the mock

---

## Task 11: Run Code Review (Checkpoint 2 -- Tests and Final Verification)

1. **Dispatch all 3 reviewers in parallel:**
   - REQUIRED SUB-SKILL: Use ring:requesting-code-review
   - All reviewers run simultaneously (ring:code-reviewer, ring:business-logic-reviewer, ring:security-reviewer)
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

## Summary of All Changes

| File | Change Type | Description |
|------|-------------|-------------|
| `components/transaction/internal/adapters/postgres/transaction/transaction.postgresql.go` | Modify | Add `FindWithFallback` and `FindWithOperationsWithFallback` to interface and implement them |
| `components/transaction/internal/adapters/postgres/transaction/transaction.postgresql_mock.go` | Regenerate | Regenerate mock with new interface methods |
| `components/transaction/internal/services/query/get-id-transaction.go` | Modify | Add `GetTransactionByIDWithFallback` method |
| `components/transaction/internal/adapters/http/in/transaction.go` | Modify | Update `CommitTransaction` and `CancelTransaction` to use `GetTransactionByIDWithFallback` |
| `components/transaction/internal/services/query/get-id-transaction_test.go` | Modify | Add `TestGetTransactionByIDWithFallback` test suite |

**Backwards Compatibility:**
- All existing methods remain unchanged
- New methods are additive (opt-in)
- Only `CommitTransaction` and `CancelTransaction` handlers use the new fallback path
- All other read paths continue using the replica (no performance impact)

**What Was NOT Changed (by design):**
- `GetTransaction` handler (GET /transactions/{id}) -- reads can tolerate replica lag for display
- `RevertTransaction` handler -- already calls `GetTransactionWithOperationsByID` which uses replica; revert is not a rapid-fire operation after create
- `UpdateTransaction` handler -- the update writes to primary; the subsequent read is for display only
