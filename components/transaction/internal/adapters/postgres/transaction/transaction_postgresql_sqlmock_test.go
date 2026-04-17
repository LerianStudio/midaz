// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"

	"github.com/LerianStudio/midaz/v3/pkg/net/http"
)

// errTestBoom is a sentinel injected into sqlmock to simulate database failures.
var errTestBoom = errors.New("boom")

func newTxRepoWithMock(t *testing.T) (*TransactionPostgreSQLRepository, sqlmock.Sqlmock) {
	t.Helper()

	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	require.NoError(t, err)

	t.Cleanup(func() { _ = db.Close() })

	resolver := dbresolver.New(
		dbresolver.WithPrimaryDBs(db),
		dbresolver.WithReplicaDBs(db),
		dbresolver.WithLoadBalancer(dbresolver.RoundRobinLB),
	)

	conn := &libPostgres.PostgresConnection{
		ConnectionDB: &resolver,
		Connected:    true,
	}

	return &TransactionPostgreSQLRepository{
		connection: conn,
		tableName:  "transaction",
	}, mock
}

func txRowCols() []string {
	return []string{
		"id", "parent_transaction_id", "description", "status", "status_description",
		"amount", "asset_code", "chart_of_accounts_group_name", "ledger_id", "organization_id",
		"body", "created_at", "updated_at", "deleted_at", "route",
	}
}

func txValidInput() *Transaction {
	amt := decimal.NewFromInt(100)

	return &Transaction{
		ID:                       uuid.NewString(),
		Description:              "test transaction",
		Status:                   Status{Code: "ACTIVE"},
		Amount:                   &amt,
		AssetCode:                "USD",
		ChartOfAccountsGroupName: "FUNDING",
		LedgerID:                 uuid.NewString(),
		OrganizationID:           uuid.NewString(),
	}
}

func TestTransactionRepo_Create(t *testing.T) {
	t.Parallel()

	t.Run("success_returns_transaction", func(t *testing.T) {
		t.Parallel()

		r, mock := newTxRepoWithMock(t)
		mock.ExpectExec("INSERT INTO transaction").WillReturnResult(sqlmock.NewResult(1, 1))

		got, err := r.Create(context.Background(), txValidInput())
		require.NoError(t, err)
		require.NotNil(t, got)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("idempotent_on_conflict_returns_nil_nil", func(t *testing.T) {
		t.Parallel()

		r, mock := newTxRepoWithMock(t)
		mock.ExpectExec("INSERT INTO transaction").WillReturnResult(sqlmock.NewResult(0, 0))

		got, err := r.Create(context.Background(), txValidInput())
		require.NoError(t, err)
		require.Nil(t, got)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("exec_error_returns_wrapped_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newTxRepoWithMock(t)
		mock.ExpectExec("INSERT INTO transaction").WillReturnError(errTestBoom)

		_, err := r.Create(context.Background(), txValidInput())
		require.Error(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

func TestTransactionRepo_Find(t *testing.T) {
	t.Parallel()

	t.Run("success_scans_row_into_entity", func(t *testing.T) {
		t.Parallel()

		r, mock := newTxRepoWithMock(t)
		txID := uuid.New()
		orgID := uuid.New()
		ledgerID := uuid.New()
		amt := decimal.NewFromInt(250)
		now := time.Now().UTC()

		rows := sqlmock.NewRows(txRowCols()).
			AddRow(txID.String(), nil, "desc", "ACTIVE", nil,
				&amt, "USD", "GRP", ledgerID.String(), orgID.String(),
				nil, now, now, nil, nil)

		mock.ExpectQuery("SELECT .* FROM transaction").
			WithArgs(orgID, ledgerID, txID).
			WillReturnRows(rows)

		got, err := r.Find(context.Background(), orgID, ledgerID, txID)
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Equal(t, txID.String(), got.ID)
		require.Equal(t, "ACTIVE", got.Status.Code)
		require.Equal(t, "USD", got.AssetCode)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("no_rows_returns_business_not_found_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newTxRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM transaction").
			WillReturnRows(sqlmock.NewRows(txRowCols()))

		_, err := r.Find(context.Background(), uuid.New(), uuid.New(), uuid.New())
		require.Error(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("scan_error_returns_wrapped_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newTxRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM transaction").WillReturnError(errTestBoom)

		_, err := r.Find(context.Background(), uuid.New(), uuid.New(), uuid.New())
		require.Error(t, err)
	})
}

func TestTransactionRepo_FindByParentID(t *testing.T) {
	t.Parallel()

	t.Run("success_returns_transaction", func(t *testing.T) {
		t.Parallel()

		r, mock := newTxRepoWithMock(t)
		txID := uuid.New()
		parentID := uuid.New()
		orgID := uuid.New()
		ledgerID := uuid.New()
		amt := decimal.NewFromInt(50)
		now := time.Now().UTC()
		parentIDStr := parentID.String()

		rows := sqlmock.NewRows(txRowCols()).
			AddRow(txID.String(), &parentIDStr, "child tx", "ACTIVE", nil,
				&amt, "BRL", "GRP", ledgerID.String(), orgID.String(),
				nil, now, now, nil, nil)

		mock.ExpectQuery("SELECT .* FROM transaction").
			WithArgs(orgID, ledgerID, parentID).
			WillReturnRows(rows)

		got, err := r.FindByParentID(context.Background(), orgID, ledgerID, parentID)
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Equal(t, "BRL", got.AssetCode)
	})

	t.Run("no_rows_returns_nil_nil", func(t *testing.T) {
		t.Parallel()

		r, mock := newTxRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM transaction").
			WillReturnRows(sqlmock.NewRows(txRowCols()))

		got, err := r.FindByParentID(context.Background(), uuid.New(), uuid.New(), uuid.New())
		require.NoError(t, err)
		require.Nil(t, got)
	})

	t.Run("query_error_returns_wrapped_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newTxRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM transaction").WillReturnError(errTestBoom)

		_, err := r.FindByParentID(context.Background(), uuid.New(), uuid.New(), uuid.New())
		require.Error(t, err)
	})
}

func TestTransactionRepo_Delete(t *testing.T) {
	t.Parallel()

	t.Run("success_soft_deletes_one_row", func(t *testing.T) {
		t.Parallel()

		r, mock := newTxRepoWithMock(t)
		orgID := uuid.New()
		ledgerID := uuid.New()
		txID := uuid.New()

		mock.ExpectExec("UPDATE transaction SET deleted_at").
			WithArgs(orgID, ledgerID, txID).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := r.Delete(context.Background(), orgID, ledgerID, txID)
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("zero_rows_returns_not_found", func(t *testing.T) {
		t.Parallel()

		r, mock := newTxRepoWithMock(t)
		mock.ExpectExec("UPDATE transaction SET deleted_at").
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := r.Delete(context.Background(), uuid.New(), uuid.New(), uuid.New())
		require.Error(t, err)
	})

	t.Run("exec_error_returns_wrapped_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newTxRepoWithMock(t)
		mock.ExpectExec("UPDATE transaction SET deleted_at").WillReturnError(errTestBoom)

		err := r.Delete(context.Background(), uuid.New(), uuid.New(), uuid.New())
		require.Error(t, err)
	})
}

func TestTransactionRepo_Update(t *testing.T) {
	t.Parallel()

	t.Run("success_updates_description_and_status", func(t *testing.T) {
		t.Parallel()

		r, mock := newTxRepoWithMock(t)
		orgID := uuid.New()
		ledgerID := uuid.New()
		txID := uuid.New()

		input := &Transaction{
			Description: "new desc",
			Status:      Status{Code: "COMPLETED"},
		}

		mock.ExpectExec("UPDATE transaction SET").
			WillReturnResult(sqlmock.NewResult(0, 1))

		got, err := r.Update(context.Background(), orgID, ledgerID, txID, input)
		require.NoError(t, err)
		require.NotNil(t, got)
	})

	t.Run("zero_rows_returns_not_found", func(t *testing.T) {
		t.Parallel()

		r, mock := newTxRepoWithMock(t)
		input := &Transaction{Description: "x"}

		mock.ExpectExec("UPDATE transaction SET").
			WillReturnResult(sqlmock.NewResult(0, 0))

		_, err := r.Update(context.Background(), uuid.New(), uuid.New(), uuid.New(), input)
		require.Error(t, err)
	})

	t.Run("exec_error_returns_wrapped_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newTxRepoWithMock(t)
		input := &Transaction{Description: "x"}

		mock.ExpectExec("UPDATE transaction SET").WillReturnError(errTestBoom)

		_, err := r.Update(context.Background(), uuid.New(), uuid.New(), uuid.New(), input)
		require.Error(t, err)
	})
}

func TestTransactionRepo_FindAll(t *testing.T) {
	t.Parallel()

	t.Run("success_returns_rows", func(t *testing.T) {
		t.Parallel()

		r, mock := newTxRepoWithMock(t)
		now := time.Now().UTC()
		amt := decimal.NewFromInt(10)

		rows := sqlmock.NewRows(txRowCols()).
			AddRow(uuid.NewString(), nil, "t1", "ACTIVE", nil,
				&amt, "USD", "G", uuid.NewString(), uuid.NewString(),
				nil, now, now, nil, nil)

		mock.ExpectQuery("SELECT .* FROM transaction").WillReturnRows(rows)

		txs, _, err := r.FindAll(context.Background(), uuid.New(), uuid.New(), http.Pagination{Limit: 10, SortOrder: "desc"})
		require.NoError(t, err)
		require.Len(t, txs, 1)
	})

	t.Run("empty_result_returns_empty_slice", func(t *testing.T) {
		t.Parallel()

		r, mock := newTxRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM transaction").
			WillReturnRows(sqlmock.NewRows(txRowCols()))

		txs, _, err := r.FindAll(context.Background(), uuid.New(), uuid.New(), http.Pagination{Limit: 10, SortOrder: "desc"})
		require.NoError(t, err)
		require.Empty(t, txs)
	})

	t.Run("query_error_returns_wrapped_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newTxRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM transaction").WillReturnError(errTestBoom)

		_, _, err := r.FindAll(context.Background(), uuid.New(), uuid.New(), http.Pagination{Limit: 10, SortOrder: "desc"})
		require.Error(t, err)
	})
}

func TestTransactionRepo_ListByIDs(t *testing.T) {
	t.Parallel()

	t.Run("success_returns_rows", func(t *testing.T) {
		t.Parallel()

		r, mock := newTxRepoWithMock(t)
		id1 := uuid.New()
		now := time.Now().UTC()
		amt := decimal.NewFromInt(10)

		rows := sqlmock.NewRows(txRowCols()).
			AddRow(id1.String(), nil, "t1", "ACTIVE", nil,
				&amt, "USD", "G", uuid.NewString(), uuid.NewString(),
				nil, now, now, nil, nil)

		mock.ExpectQuery("SELECT .* FROM transaction").WillReturnRows(rows)

		got, err := r.ListByIDs(context.Background(), uuid.New(), uuid.New(), []uuid.UUID{id1})
		require.NoError(t, err)
		require.Len(t, got, 1)
	})

	t.Run("query_error_returns_wrapped_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newTxRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM transaction").WillReturnError(errTestBoom)

		_, err := r.ListByIDs(context.Background(), uuid.New(), uuid.New(), []uuid.UUID{uuid.New()})
		require.Error(t, err)
	})
}

func TestTransactionRepo_CreateBatch(t *testing.T) {
	t.Parallel()

	t.Run("empty_slice_noop_returns_nil", func(t *testing.T) {
		t.Parallel()

		r, _ := newTxRepoWithMock(t)
		err := r.CreateBatch(context.Background(), nil)
		require.NoError(t, err)
	})

	t.Run("success_commits_transaction", func(t *testing.T) {
		t.Parallel()

		r, mock := newTxRepoWithMock(t)
		mock.ExpectBegin()
		mock.ExpectExec("INSERT INTO transaction").WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := r.CreateBatch(context.Background(), []*Transaction{txValidInput()})
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("exec_error_rolls_back", func(t *testing.T) {
		t.Parallel()

		r, mock := newTxRepoWithMock(t)
		mock.ExpectBegin()
		mock.ExpectExec("INSERT INTO transaction").WillReturnError(errTestBoom)
		mock.ExpectRollback()

		err := r.CreateBatch(context.Background(), []*Transaction{txValidInput()})
		require.Error(t, err)
	})
}
