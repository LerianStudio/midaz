// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package recovery

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/repository"
)

type transactionStoreStub struct {
	db *sql.DB
}

func (stub transactionStoreStub) BeginTx(ctx context.Context) (repository.DBTransaction, error) {
	return stub.db.BeginTx(ctx, nil)
}

func (transactionStoreStub) CreateBulkTx(ctx context.Context, tx repository.DBExecutor, transactions []*transaction.Transaction) (*repository.BulkInsertResult, error) {
	result, err := tx.ExecContext(ctx, "INSERT recovery_transaction")
	if err != nil {
		return nil, err
	}

	count, err := result.RowsAffected()
	inserted := &repository.BulkInsertResult{Attempted: 1, Inserted: count}
	if count == 1 {
		inserted.InsertedIDs = []string{transactions[0].ID}
	}

	return inserted, err
}

type operationStoreStub struct{}

func (operationStoreStub) CreateBulkTx(ctx context.Context, tx repository.DBExecutor, operations []*operation.Operation) (*repository.BulkInsertResult, error) {
	if len(operations) > 1 {
		operations[0], operations[1] = operations[1], operations[0]
	}

	_, err := tx.ExecContext(ctx, "INSERT recovery_operations")
	return &repository.BulkInsertResult{}, err
}

func newStoreTest(t *testing.T) (*Store, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() {
		assert.NoError(t, mock.ExpectationsWereMet())
		_ = db.Close()
	})

	return NewStore(transactionStoreStub{db: db}, operationStoreStub{}), mock
}

func frozenStoreRecord() command.BalanceEnginePersistenceRecord {
	date := time.Date(2026, time.September, 4, 12, 0, 0, 0, time.UTC)
	amount := decimal.RequireFromString("0.00000000000000000001")
	before, after, hold := decimal.RequireFromString("1.00000000000000000001"), decimal.NewFromInt(1), decimal.Zero
	beforeVersion, afterVersion := int64(9007199254740993), int64(9007199254740994)
	tran := &transaction.Transaction{
		ID: "11111111-1111-4111-8111-111111111111", OrganizationID: "22222222-2222-4222-8222-222222222222", LedgerID: "33333333-3333-4333-8333-333333333333",
		Amount: &amount, AssetCode: "USD", Status: transaction.Status{Code: constant.APPROVED}, CreatedAt: date, UpdatedAt: date,
		Description: "editable transaction description", Metadata: map[string]any{"purpose": "frozen"},
		FeesSkipped: true, TracerSkipped: true,
	}
	tran.Operations = []*operation.Operation{{
		ID: "44444444-4444-5444-8444-444444444444", TransactionID: tran.ID, OrganizationID: tran.OrganizationID, LedgerID: tran.LedgerID,
		AccountID: "55555555-5555-4555-8555-555555555555", BalanceID: "66666666-6666-4666-8666-666666666666",
		AccountAlias: "@source", AssetCode: "USD", Type: constant.DEBIT, Direction: constant.DirectionDebit, BalanceAffected: true,
		Amount: operation.Amount{Value: &amount}, Balance: operation.Balance{Available: &before, OnHold: &hold, Version: &beforeVersion},
		BalanceAfter: operation.Balance{Available: &after, OnHold: &hold, Version: &afterVersion}, CreatedAt: date, UpdatedAt: date,
		Snapshot: mmodel.OperationSnapshot{OverdraftUsedBefore: "0", OverdraftUsedAfter: "0"},
	}}

	return command.BalanceEnginePersistenceRecord{Transaction: tran, Action: "direct"}
}

func expectTransaction(mock sqlmock.Sqlmock, status string, active, matches bool) {
	rows := sqlmock.NewRows([]string{"status", "active", "matches"})
	if status != "" {
		rows.AddRow(status, active, matches)
	}

	mock.ExpectQuery(`SELECT status, deleted_at IS NULL, .* FROM transaction WHERE id = \$1 FOR UPDATE`).WillReturnRows(rows).RowsWillBeClosed()
}

func expectOperations(mock sqlmock.Sqlmock, count int) {
	mock.ExpectExec("INSERT recovery_operations").WillReturnResult(sqlmock.NewResult(0, int64(count)))
	expectVerifiedOperations(mock, count)
}

func expectVerifiedOperations(mock sqlmock.Sqlmock, count int) {
	for range count {
		mock.ExpectQuery(`SELECT .* FROM operation WHERE id = \$1 FOR SHARE`).WillReturnRows(sqlmock.NewRows([]string{"matches"}).AddRow(true)).RowsWillBeClosed()
	}
}

func TestStorePersistCreatesAtomically(t *testing.T) {
	store, mock := newStoreTest(t)
	record := frozenStoreRecord()
	second := *record.Transaction.Operations[0]
	second.ID = "77777777-7777-5777-8777-777777777777"
	record.Transaction.Operations = append(record.Transaction.Operations, &second)
	mock.ExpectBegin()
	expectTransaction(mock, "", true, true)
	mock.ExpectExec("INSERT recovery_transaction").WillReturnResult(sqlmock.NewResult(0, 1))
	expectTransaction(mock, constant.APPROVED, true, true)
	expectOperations(mock, 2)
	mock.ExpectCommit()

	require.NoError(t, store.Persist(context.Background(), record))
	assert.Equal(t, "44444444-4444-5444-8444-444444444444", record.Transaction.Operations[0].ID)
	assert.Equal(t, second.ID, record.Transaction.Operations[1].ID)
}

func TestStorePersistReplayVerifiesRows(t *testing.T) {
	store, mock := newStoreTest(t)
	mock.ExpectBegin()
	expectTransaction(mock, constant.APPROVED, true, true)
	expectVerifiedOperations(mock, 1)
	mock.ExpectCommit()

	require.NoError(t, store.Persist(context.Background(), frozenStoreRecord()))
}

func TestStorePersistCreationRaceDoesNotAppendAnotherExecution(t *testing.T) {
	for _, existingOperation := range []bool{false, true} {
		t.Run(map[bool]string{false: "missing operation", true: "matching operation"}[existingOperation], func(t *testing.T) {
			store, mock := newStoreTest(t)
			mock.ExpectBegin()
			expectTransaction(mock, "", true, true)
			mock.ExpectExec("INSERT recovery_transaction").WillReturnResult(sqlmock.NewResult(0, 0))
			expectTransaction(mock, constant.APPROVED, true, true)
			if existingOperation {
				expectVerifiedOperations(mock, 1)
				mock.ExpectCommit()
				require.NoError(t, store.Persist(context.Background(), frozenStoreRecord()))
				return
			}

			mock.ExpectQuery("SELECT .* FROM operation").WillReturnRows(sqlmock.NewRows([]string{"matches"}))
			mock.ExpectRollback()
			require.ErrorIs(t, store.Persist(context.Background(), frozenStoreRecord()), command.ErrBalanceEnginePersistenceConflict)
		})
	}
}

func TestStorePersistRequiresConfirmedStatusUpdate(t *testing.T) {
	for _, changedRows := range []int64{0, 2} {
		t.Run(map[int64]string{0: "no transition", 2: "ambiguous transition"}[changedRows], func(t *testing.T) {
			store, mock := newStoreTest(t)
			record := frozenStoreRecord()
			record.Action, record.ExpectedStatus = "commit", constant.PENDING
			mock.ExpectBegin()
			expectTransaction(mock, constant.PENDING, true, true)
			mock.ExpectExec("UPDATE transaction").WillReturnResult(sqlmock.NewResult(0, changedRows))
			mock.ExpectRollback()
			require.ErrorIs(t, store.Persist(context.Background(), record), command.ErrBalanceEnginePersistenceConflict)
		})
	}
}

func TestStorePersistLifecycle(t *testing.T) {
	for _, scenario := range []struct {
		name, action, existing, target string
		update, conflict               bool
	}{
		{"commit", "commit", constant.PENDING, constant.APPROVED, true, false},
		{"cancel", "cancel", constant.PENDING, constant.CANCELED, true, false},
		{"commit replay", "commit", constant.APPROVED, constant.APPROVED, false, false},
		{"cancel replay", "cancel", constant.CANCELED, constant.CANCELED, false, false},
		{"commit before pending", "commit", "", constant.APPROVED, false, true},
		{"cancel after commit", "cancel", constant.APPROVED, constant.CANCELED, false, true},
		{"commit after cancel", "commit", constant.CANCELED, constant.APPROVED, false, true},
		{"unknown persisted status", "commit", "UNKNOWN", constant.APPROVED, false, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			store, mock := newStoreTest(t)
			record := frozenStoreRecord()
			record.Action, record.ExpectedStatus, record.Transaction.Status.Code = scenario.action, constant.PENDING, scenario.target
			mock.ExpectBegin()
			expectTransaction(mock, scenario.existing, true, true)
			if scenario.conflict {
				mock.ExpectRollback()
				require.ErrorIs(t, store.Persist(context.Background(), record), command.ErrBalanceEnginePersistenceConflict)
				return
			}

			if scenario.update {
				mock.ExpectExec(`UPDATE transaction SET status = \$1, status_description = \$2, updated_at = \$3\s+WHERE id = \$4 AND organization_id = \$5 AND ledger_id = \$6 AND status = \$7 AND deleted_at IS NULL`).
					WithArgs(scenario.target, nil, record.Transaction.UpdatedAt, record.Transaction.ID, record.Transaction.OrganizationID, record.Transaction.LedgerID, constant.PENDING).
					WillReturnResult(sqlmock.NewResult(0, 1))
			}

			if scenario.update {
				expectOperations(mock, 1)
			} else {
				expectVerifiedOperations(mock, 1)
			}
			mock.ExpectCommit()
			require.NoError(t, store.Persist(context.Background(), record))
		})
	}
}

func TestStorePersistWithOutcomeReportsDurableTransactionStatus(t *testing.T) {
	for _, scenario := range []struct {
		name, action, existing, target, expected string
		create, update                           bool
	}{
		{name: "new pending hold", action: "hold", target: constant.PENDING, expected: constant.PENDING, create: true},
		{name: "new direct approval", action: "direct", target: constant.APPROVED, expected: constant.APPROVED, create: true},
		{name: "new revert approval", action: "revert", target: constant.APPROVED, expected: constant.APPROVED, create: true},
		{name: "commit approval", action: "commit", existing: constant.PENDING, target: constant.APPROVED, expected: constant.APPROVED, update: true},
		{name: "cancel", action: "cancel", existing: constant.PENDING, target: constant.CANCELED, expected: constant.CANCELED, update: true},
		{name: "late hold after approval", action: "hold", existing: constant.APPROVED, target: constant.PENDING, expected: constant.APPROVED},
		{name: "late hold after cancellation", action: "hold", existing: constant.CANCELED, target: constant.PENDING, expected: constant.CANCELED},
		{name: "direct replay", action: "direct", existing: constant.APPROVED, target: constant.APPROVED, expected: constant.APPROVED},
		{name: "commit replay", action: "commit", existing: constant.APPROVED, target: constant.APPROVED, expected: constant.APPROVED},
		{name: "cancel replay", action: "cancel", existing: constant.CANCELED, target: constant.CANCELED, expected: constant.CANCELED},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			store, mock := newStoreTest(t)
			record := frozenStoreRecord()
			record.Action, record.Transaction.Status.Code = scenario.action, scenario.target
			if scenario.action == "hold" {
				record.Transaction.Body = mtransaction.Transaction{Pending: true, Send: mtransaction.Send{Asset: "USD", Value: *record.Transaction.Amount}}
			}
			if scenario.action == "commit" || scenario.action == "cancel" {
				record.ExpectedStatus = constant.PENDING
			}

			mock.ExpectBegin()
			expectTransaction(mock, scenario.existing, true, true)
			if scenario.create {
				mock.ExpectExec("INSERT recovery_transaction").WillReturnResult(sqlmock.NewResult(0, 1))
				expectTransaction(mock, scenario.target, true, true)
			}
			if scenario.update {
				mock.ExpectExec("UPDATE transaction").WillReturnResult(sqlmock.NewResult(0, 1))
			}
			if scenario.create || scenario.update {
				expectOperations(mock, 1)
			} else {
				expectVerifiedOperations(mock, 1)
			}
			mock.ExpectCommit()

			outcome, err := store.PersistWithOutcome(t.Context(), record)
			require.NoError(t, err)
			assert.Equal(t, scenario.expected, outcome.TransactionStatus)
			expectedPhase := command.TransactionLifecyclePhaseNoop
			if scenario.create {
				expectedPhase = command.TransactionLifecyclePhaseCreated
			} else if scenario.update {
				expectedPhase = command.TransactionLifecyclePhaseUpdated
			}
			assert.Equal(t, expectedPhase, outcome.LifecyclePhase)
		})
	}
}

func TestStorePersistWithOutcomeReportsNoopAfterConcurrentInsert(t *testing.T) {
	store, mock := newStoreTest(t)
	mock.ExpectBegin()
	expectTransaction(mock, "", true, true)
	mock.ExpectExec("INSERT recovery_transaction").WillReturnResult(sqlmock.NewResult(0, 0))
	expectTransaction(mock, constant.APPROVED, true, true)
	expectVerifiedOperations(mock, 1)
	mock.ExpectCommit()

	outcome, err := store.PersistWithOutcome(t.Context(), frozenStoreRecord())

	require.NoError(t, err)
	assert.Equal(t, constant.APPROVED, outcome.TransactionStatus)
	assert.Equal(t, command.TransactionLifecyclePhaseNoop, outcome.LifecyclePhase)
}

func TestStorePersistWithOutcomeReturnsZeroOnPersistenceFailure(t *testing.T) {
	for _, stage := range []string{"operation insert", "commit"} {
		t.Run(stage, func(t *testing.T) {
			store, mock := newStoreTest(t)
			failure := errors.New("SQL failure")
			mock.ExpectBegin()
			expectTransaction(mock, "", true, true)
			mock.ExpectExec("INSERT recovery_transaction").WillReturnResult(sqlmock.NewResult(0, 1))
			expectTransaction(mock, constant.APPROVED, true, true)
			if stage == "operation insert" {
				mock.ExpectExec("INSERT recovery_operations").WillReturnError(failure)
				mock.ExpectRollback()
			} else {
				expectOperations(mock, 1)
				mock.ExpectCommit().WillReturnError(failure)
			}

			outcome, err := store.PersistWithOutcome(t.Context(), frozenStoreRecord())
			require.ErrorIs(t, err, failure)
			assert.Empty(t, outcome.TransactionStatus)
		})
	}
}

func TestStorePersistLateHoldRequiresEveryFrozenRow(t *testing.T) {
	for _, status := range []string{constant.APPROVED, constant.CANCELED} {
		for _, proof := range []string{"matching", "missing", "different"} {
			t.Run(status+"/"+proof, func(t *testing.T) {
				store, mock := newStoreTest(t)
				record := frozenStoreRecord()
				record.Action, record.Transaction.Status.Code = "hold", constant.PENDING
				record.Transaction.Body = mtransaction.Transaction{Pending: true, Send: mtransaction.Send{Asset: "USD", Value: *record.Transaction.Amount}}
				second := *record.Transaction.Operations[0]
				second.ID = "77777777-7777-5777-8777-777777777777"
				record.Transaction.Operations = append(record.Transaction.Operations, &second)
				mock.ExpectBegin()
				expectTransaction(mock, status, true, true)
				expectVerifiedOperations(mock, 1)
				rows := sqlmock.NewRows([]string{"matches"})
				if proof != "missing" {
					rows.AddRow(proof == "matching")
				}
				mock.ExpectQuery(`SELECT .* FROM operation WHERE id = \$1 FOR SHARE`).WillReturnRows(rows).RowsWillBeClosed()
				if proof == "matching" {
					mock.ExpectCommit()
					require.NoError(t, store.Persist(t.Context(), record))
				} else {
					mock.ExpectRollback()
					require.ErrorIs(t, store.Persist(t.Context(), record), command.ErrBalanceEnginePersistenceConflict)
				}
			})
		}
	}
}

func TestStorePersistRejectsDivergentRows(t *testing.T) {
	for _, scenario := range []struct {
		name                  string
		active, matchingTx    bool
		operation, missingRow bool
	}{
		{"different immutable transaction", true, false, false, false},
		{"deleted transaction", false, true, false, false},
		{"different operation", true, true, true, false},
		{"same target with another execution operation", true, true, true, true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			store, mock := newStoreTest(t)
			mock.ExpectBegin()
			expectTransaction(mock, constant.APPROVED, scenario.active, scenario.matchingTx)
			if scenario.operation {
				rows := sqlmock.NewRows([]string{"matches"})
				if !scenario.missingRow {
					rows.AddRow(false)
				}

				mock.ExpectQuery(`SELECT .* FROM operation`).WillReturnRows(rows)
			}

			mock.ExpectRollback()
			require.ErrorIs(t, store.Persist(context.Background(), frozenStoreRecord()), command.ErrBalanceEnginePersistenceConflict)
		})
	}
}

func TestStorePersistPropagatesSQLFailures(t *testing.T) {
	for _, stage := range []string{"begin", "read", "transaction insert", "operation insert", "operation verification", "commit"} {
		t.Run(stage, func(t *testing.T) {
			store, mock := newStoreTest(t)
			failure := errors.New("SQL failure")
			if stage == "begin" {
				mock.ExpectBegin().WillReturnError(failure)
				require.ErrorIs(t, store.Persist(context.Background(), frozenStoreRecord()), failure)
				return
			}

			mock.ExpectBegin()
			switch stage {
			case "read":
				mock.ExpectQuery("SELECT status").WillReturnError(failure)
			case "transaction insert":
				expectTransaction(mock, "", true, true)
				mock.ExpectExec("INSERT recovery_transaction").WillReturnError(failure)
			case "operation insert":
				expectTransaction(mock, "", true, true)
				mock.ExpectExec("INSERT recovery_transaction").WillReturnResult(sqlmock.NewResult(0, 1))
				expectTransaction(mock, constant.APPROVED, true, true)
				mock.ExpectExec("INSERT recovery_operations").WillReturnError(failure)
			case "operation verification":
				expectTransaction(mock, constant.APPROVED, true, true)
				mock.ExpectQuery("SELECT .* FROM operation").WillReturnError(failure)
			case "commit":
				expectTransaction(mock, constant.APPROVED, true, true)
				expectVerifiedOperations(mock, 1)
				mock.ExpectCommit().WillReturnError(failure)
			}

			// database/sql marks the transaction done after any Commit result.
			if stage != "commit" {
				mock.ExpectRollback()
			}

			require.ErrorIs(t, store.Persist(context.Background(), frozenStoreRecord()), failure)
		})
	}
}

type execOnlyTransaction struct {
	rolledBack bool
}

func (*execOnlyTransaction) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	return nil, errors.New("unexpected write")
}

func (*execOnlyTransaction) Commit() error { return errors.New("unexpected commit") }

func (tx *execOnlyTransaction) Rollback() error { tx.rolledBack = true; return nil }

type execOnlyRepository struct {
	transactionStoreStub
	tx *execOnlyTransaction
}

func (stub execOnlyRepository) BeginTx(context.Context) (repository.DBTransaction, error) {
	return stub.tx, nil
}

func TestStorePersistRequiresQuerierBeforeWrites(t *testing.T) {
	tx := &execOnlyTransaction{}
	store := NewStore(execOnlyRepository{tx: tx}, operationStoreStub{})
	require.ErrorIs(t, store.Persist(context.Background(), frozenStoreRecord()), repository.ErrQueryContextNotSupported)
	assert.True(t, tx.rolledBack)
}

func TestStorePersistRejectsInvalidInputBeforeBegin(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		mutate func(*command.BalanceEnginePersistenceRecord)
	}{
		{"nil transaction", func(r *command.BalanceEnginePersistenceRecord) { r.Transaction = nil }},
		{"foreign operation", func(r *command.BalanceEnginePersistenceRecord) {
			r.Transaction.Operations[0].OrganizationID = r.Transaction.LedgerID
		}},
		{"duplicate operation", func(r *command.BalanceEnginePersistenceRecord) {
			r.Transaction.Operations = append(r.Transaction.Operations, r.Transaction.Operations[0])
		}},
		{"unknown lifecycle", func(r *command.BalanceEnginePersistenceRecord) { r.Action = "unknown" }},
		{"unguarded commit", func(r *command.BalanceEnginePersistenceRecord) { r.Action = "commit" }},
		{"missing amount", func(r *command.BalanceEnginePersistenceRecord) { r.Transaction.Operations[0].Amount.Value = nil }},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			store, _ := newStoreTest(t)
			record := frozenStoreRecord()
			scenario.mutate(&record)
			require.ErrorIs(t, store.Persist(context.Background(), record), command.ErrBalanceEnginePersistenceConflict)
		})
	}
}

func TestStorePersistCancelledBeforeBegin(t *testing.T) {
	store, _ := newStoreTest(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, store.Persist(ctx, frozenStoreRecord()), context.Canceled)
}

func TestVerifyTransactionPreservesAuditAndExactAmount(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	record := frozenStoreRecord()
	parent, emptyRoute := "88888888-8888-4888-8888-888888888888", ""
	record.Transaction.ParentTransactionID, record.Transaction.RouteID = &parent, &emptyRoute
	query := `SELECT status, deleted_at IS NULL, \(organization_id IS NOT DISTINCT FROM \$2 AND ledger_id IS NOT DISTINCT FROM \$3 AND parent_transaction_id IS NOT DISTINCT FROM \$4 AND amount IS NOT DISTINCT FROM \$5 AND asset_code IS NOT DISTINCT FROM \$6 AND chart_of_accounts_group_name IS NOT DISTINCT FROM \$7 AND created_at IS NOT DISTINCT FROM \$8 AND route IS NOT DISTINCT FROM \$9 AND route_id IS NOT DISTINCT FROM \$10 AND fees_skipped IS NOT DISTINCT FROM \$11 AND tracer_skipped IS NOT DISTINCT FROM \$12\) FROM transaction WHERE id = \$1 FOR UPDATE`
	mock.ExpectQuery(query).WithArgs(record.Transaction.ID, record.Transaction.OrganizationID, record.Transaction.LedgerID, parent,
		"0.00000000000000000001", "USD", "", record.Transaction.CreatedAt, nil, "", true, true).
		WillReturnRows(sqlmock.NewRows([]string{"status", "active", "matches"}).AddRow(constant.APPROVED, true, true))
	status, exists, err := verifyTransaction(context.Background(), db, record)
	require.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, constant.APPROVED, status)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestVerifyOperationAllPersistedFields(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	row := frozenStoreRecord().Transaction.Operations[0]
	empty := ""
	row.RouteID, row.RouteCode, row.RouteDescription = &empty, &empty, &empty
	snapshot, err := json.Marshal(row.Snapshot)
	require.NoError(t, err)
	args := []driver.Value{
		row.ID, row.TransactionID, "", constant.DEBIT, "USD", "0.00000000000000000001", "1.00000000000000000001", "0", "1", "0", "", nil,
		row.AccountID, "@source", row.BalanceID, "", row.OrganizationID, row.LedgerID, row.CreatedAt, row.UpdatedAt, nil, nil, true, "default",
		int64(9007199254740993), int64(9007199254740994), constant.DirectionDebit, "", "", "", string(snapshot),
	}
	mock.ExpectQuery(`SELECT .*balance_version_before IS NOT DISTINCT FROM \$25 AND balance_version_after IS NOT DISTINCT FROM \$26.*` + regexp.QuoteMeta("snapshot IS NOT DISTINCT FROM $31::jsonb) FROM operation WHERE id = $1 FOR SHARE")).
		WithArgs(args...).WillReturnRows(sqlmock.NewRows([]string{"matches"}).AddRow(true))
	require.NoError(t, verifyOperation(context.Background(), db, row))
	require.NoError(t, mock.ExpectationsWereMet())
}
