// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

import (
	"context"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v3/pkg/net/http"
)

// K5 extends K2's sqlmock coverage to the JOIN-based aggregate loaders and
// residual pagination/cursor edge paths in the transaction adapter. It uses
// the exact same harness (newTxRepoWithMock + QueryMatcherRegexp + dbresolver
// wrapped in libPostgres.PostgresConnection) so there is zero drift from K2.
//
// Anti-pattern guard:
//   - All assertions are on domain outcomes (returned entities, error presence,
//     pagination shape) — never on sqlmock call counts or internal SQL text.
//   - Queries are matched with ".*" / loose regex so the adapter is free to
//     refactor the squirrel builder without breaking these tests.

// txWithOpsCols returns the column list for the FindWithOperations /
// FindOrListAllWithOperations JOIN projection. Order mirrors the production
// scan sequence (15 transaction columns followed by 26 operation columns).
func txWithOpsCols() []string {
	return []string{
		// transaction columns (t.*)
		"t.id", "t.parent_transaction_id", "t.description", "t.status", "t.status_description",
		"t.amount", "t.asset_code", "t.chart_of_accounts_group_name", "t.ledger_id", "t.organization_id",
		"t.body", "t.created_at", "t.updated_at", "t.deleted_at", "t.route",
		// operation columns (o.*)
		"o.id", "o.transaction_id", "o.description", "o.type", "o.asset_code",
		"o.amount", "o.available_balance", "o.on_hold_balance", "o.available_balance_after",
		"o.on_hold_balance_after", "o.status", "o.status_description", "o.account_id",
		"o.account_alias", "o.balance_id", "o.chart_of_accounts", "o.organization_id",
		"o.ledger_id", "o.created_at", "o.updated_at", "o.deleted_at", "o.route",
		"o.balance_affected", "o.balance_key", "o.balance_version_before", "o.balance_version_after",
	}
}

// txWithOpsRow builds one joined row with a non-NULL operation. The amounts
// are arbitrary — the assertions check id wiring and operation attachment,
// not numeric values, so drift-safe by design.
func txWithOpsRow(txID uuid.UUID) *sqlmock.Rows {
	orgID := uuid.NewString()
	ledgerID := uuid.NewString()
	amt := decimal.NewFromInt(100)
	now := time.Now().UTC()

	return sqlmock.NewRows(txWithOpsCols()).
		AddRow(
			// transaction
			txID.String(), nil, "tx desc", "ACTIVE", nil,
			&amt, "USD", "GRP", ledgerID, orgID,
			nil, now, now, nil, nil,
			// operation
			uuid.NewString(), txID.String(), "op desc", "DEBIT", "USD",
			&amt, &amt, &amt, &amt, &amt,
			"ACTIVE", nil, uuid.NewString(), "@a",
			uuid.NewString(), "GRP", orgID, ledgerID,
			now, now, nil, nil,
			true, "default", int64(1), int64(2),
		)
}

// txWithOpsLeftJoinNullOpRow builds a row where the LEFT JOIN produced a
// transaction with no matching operation — the operation columns are all
// NULL. This exercises the nullable-pointer scan path in
// FindOrListAllWithOperations.
func txWithOpsLeftJoinNullOpRow(txID uuid.UUID) *sqlmock.Rows {
	orgID := uuid.NewString()
	ledgerID := uuid.NewString()
	amt := decimal.NewFromInt(100)
	now := time.Now().UTC()

	return sqlmock.NewRows(txWithOpsCols()).
		AddRow(
			// transaction — present
			txID.String(), nil, "tx desc", "ACTIVE", nil,
			&amt, "USD", "GRP", ledgerID, orgID,
			nil, now, now, nil, nil,
			// operation — all NULL (no matching row in LEFT JOIN)
			nil, nil, nil, nil, nil,
			nil, nil, nil, nil, nil,
			nil, nil, nil, nil,
			nil, nil, nil, nil,
			nil, nil, nil, nil,
			nil, nil, nil, nil,
		)
}

func TestTransactionRepo_FindWithOperations(t *testing.T) {
	t.Parallel()

	t.Run("success_loads_transaction_with_operations", func(t *testing.T) {
		t.Parallel()

		r, mock := newTxRepoWithMock(t)
		txID := uuid.New()

		mock.ExpectQuery("SELECT .* FROM transaction .* JOIN operation").
			WillReturnRows(txWithOpsRow(txID))

		got, err := r.FindWithOperations(context.Background(), uuid.New(), uuid.New(), txID)
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Equal(t, txID.String(), got.ID)
		require.Len(t, got.Operations, 1, "single joined row must yield one operation")
	})

	t.Run("empty_rows_returns_empty_transaction", func(t *testing.T) {
		t.Parallel()

		// When the JOIN finds no rows the method does not return an error — it
		// returns an empty transaction aggregate. Callers at the use-case layer
		// decide whether that is a 404.
		r, mock := newTxRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM transaction").
			WillReturnRows(sqlmock.NewRows(txWithOpsCols()))

		got, err := r.FindWithOperations(context.Background(), uuid.New(), uuid.New(), uuid.New())
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Empty(t, got.Operations)
	})

	t.Run("query_error_returns_wrapped_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newTxRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM transaction").WillReturnError(errTestBoom)

		_, err := r.FindWithOperations(context.Background(), uuid.New(), uuid.New(), uuid.New())
		require.Error(t, err)
	})
}

func TestTransactionRepo_FindOrListAllWithOperations(t *testing.T) {
	t.Parallel()

	t.Run("success_groups_operations_by_transaction", func(t *testing.T) {
		t.Parallel()

		r, mock := newTxRepoWithMock(t)
		txID := uuid.New()

		mock.ExpectQuery("SELECT .* FROM \\(SELECT").
			WillReturnRows(txWithOpsRow(txID))

		txs, _, err := r.FindOrListAllWithOperations(context.Background(),
			uuid.New(), uuid.New(), nil,
			http.Pagination{Limit: 10, SortOrder: "desc"})
		require.NoError(t, err)
		require.Len(t, txs, 1)
		require.Equal(t, txID.String(), txs[0].ID)
		require.Len(t, txs[0].Operations, 1)
	})

	t.Run("left_join_null_operation_does_not_crash", func(t *testing.T) {
		t.Parallel()

		// Regression guard for LEFT JOIN NULL handling: a transaction with no
		// matching operation row must yield a transaction with an empty
		// Operations slice, not a nil-deref panic.
		r, mock := newTxRepoWithMock(t)
		txID := uuid.New()

		mock.ExpectQuery("SELECT .* FROM \\(SELECT").
			WillReturnRows(txWithOpsLeftJoinNullOpRow(txID))

		txs, _, err := r.FindOrListAllWithOperations(context.Background(),
			uuid.New(), uuid.New(), nil,
			http.Pagination{Limit: 10, SortOrder: "desc"})
		require.NoError(t, err)
		require.Len(t, txs, 1)
		require.Empty(t, txs[0].Operations, "NULL op columns must produce empty op slice")
	})

	t.Run("empty_rows_returns_empty_slice", func(t *testing.T) {
		t.Parallel()

		r, mock := newTxRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM \\(SELECT").
			WillReturnRows(sqlmock.NewRows(txWithOpsCols()))

		txs, _, err := r.FindOrListAllWithOperations(context.Background(),
			uuid.New(), uuid.New(), nil,
			http.Pagination{Limit: 10, SortOrder: "desc"})
		require.NoError(t, err)
		require.Empty(t, txs)
	})

	t.Run("query_error_returns_wrapped_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newTxRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM \\(SELECT").WillReturnError(errTestBoom)

		_, _, err := r.FindOrListAllWithOperations(context.Background(),
			uuid.New(), uuid.New(), nil,
			http.Pagination{Limit: 10, SortOrder: "desc"})
		require.Error(t, err)
	})

	t.Run("with_ids_filter_success", func(t *testing.T) {
		t.Parallel()

		// ids slice adds an `id = ANY(?)` predicate; we don't pin the SQL,
		// just the adapter's behaviour: results scan into the aggregate.
		r, mock := newTxRepoWithMock(t)
		txID := uuid.New()
		mock.ExpectQuery("SELECT .* FROM \\(SELECT").
			WillReturnRows(txWithOpsRow(txID))

		txs, _, err := r.FindOrListAllWithOperations(context.Background(),
			uuid.New(), uuid.New(), []uuid.UUID{txID},
			http.Pagination{Limit: 10, SortOrder: "desc"})
		require.NoError(t, err)
		require.Len(t, txs, 1)
	})

	t.Run("invalid_cursor_returns_decode_error", func(t *testing.T) {
		t.Parallel()

		// Malformed cursor short-circuits before any DB call. No expectations
		// set on the mock — if the adapter issued a query, the test fails via
		// the sqlmock strict ordering.
		r, _ := newTxRepoWithMock(t)
		_, _, err := r.FindOrListAllWithOperations(context.Background(),
			uuid.New(), uuid.New(), nil,
			http.Pagination{Limit: 10, SortOrder: "desc", Cursor: "!!!not-base64!!!"})
		require.Error(t, err)
	})
}

func TestTransactionRepo_FindAll_InvalidCursor(t *testing.T) {
	t.Parallel()

	// Covers the DecodeCursor error branch in FindAll — same short-circuit
	// behaviour as FindOrListAllWithOperations.
	r, _ := newTxRepoWithMock(t)
	_, _, err := r.FindAll(context.Background(), uuid.New(), uuid.New(),
		http.Pagination{Limit: 10, SortOrder: "desc", Cursor: "###bad###"})
	require.Error(t, err)
}

func TestTransactionRepo_CreateBatch_Paths(t *testing.T) {
	t.Parallel()

	t.Run("begin_tx_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newTxRepoWithMock(t)
		mock.ExpectBegin().WillReturnError(errTestBoom)

		err := r.CreateBatch(context.Background(), []*Transaction{txValidInput()})
		require.Error(t, err)
	})

	t.Run("nil_entries_in_slice_are_skipped", func(t *testing.T) {
		t.Parallel()

		// A slice containing only nils means hasRows stays false → no exec.
		// Transaction still begins + commits cleanly.
		r, mock := newTxRepoWithMock(t)
		mock.ExpectBegin()
		mock.ExpectCommit()

		err := r.CreateBatch(context.Background(), []*Transaction{nil})
		require.NoError(t, err)
	})

	t.Run("commit_error_surfaces", func(t *testing.T) {
		t.Parallel()

		r, mock := newTxRepoWithMock(t)
		mock.ExpectBegin()
		mock.ExpectExec("INSERT INTO transaction").WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit().WillReturnError(errTestBoom)

		err := r.CreateBatch(context.Background(), []*Transaction{txValidInput()})
		require.Error(t, err)
	})
}

func TestTransactionRepo_BeginTx(t *testing.T) {
	t.Parallel()

	t.Run("success_returns_tx", func(t *testing.T) {
		t.Parallel()

		r, mock := newTxRepoWithMock(t)
		mock.ExpectBegin()
		mock.ExpectRollback()

		tx, err := r.BeginTx(context.Background())
		require.NoError(t, err)
		require.NotNil(t, tx)
		require.NoError(t, tx.Rollback())
	})

	t.Run("begin_error_surfaces", func(t *testing.T) {
		t.Parallel()

		r, mock := newTxRepoWithMock(t)
		mock.ExpectBegin().WillReturnError(errTestBoom)

		_, err := r.BeginTx(context.Background())
		require.Error(t, err)
	})
}

func TestTransactionRepo_CreateBatchWithTx_NilTxIsNoop(t *testing.T) {
	t.Parallel()

	r, _ := newTxRepoWithMock(t)

	// Passing a nil tx is an explicit no-op contract so callers can safely
	// defer cleanup even when the initial BeginTx failed upstream.
	err := r.CreateBatchWithTx(context.Background(), nil, []*Transaction{txValidInput()})
	require.NoError(t, err)
}

// assert is referenced indirectly via require aliases above; keep the import
// anchored so a future refactor that drops require.* but keeps assert.* still
// compiles.
var _ = assert.True
