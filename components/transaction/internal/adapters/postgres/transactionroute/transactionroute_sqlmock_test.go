// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transactionroute

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
)

var errTRTestBoom = errors.New("tr boom")

func newTRRepoWithMock(t *testing.T) (*TransactionRoutePostgreSQLRepository, sqlmock.Sqlmock) {
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

	return &TransactionRoutePostgreSQLRepository{
		connection: conn,
		tableName:  "transaction_route",
	}, mock
}

// trFindAllCols matches the SELECT column order used by FindAll (8 cols, no joins).
func trFindAllCols() []string {
	return []string{
		"id", "organization_id", "ledger_id", "title", "description",
		"created_at", "updated_at", "deleted_at",
	}
}

func trFindAllRow() *sqlmock.Rows {
	now := time.Now().UTC()

	return sqlmock.NewRows(trFindAllCols()).
		AddRow(uuid.New(), uuid.New(), uuid.New(), "Charge", "Settle", now, now, nil)
}

func validTxRoute() *mmodel.TransactionRoute {
	return &mmodel.TransactionRoute{
		ID:             uuid.New(),
		OrganizationID: uuid.New(),
		LedgerID:       uuid.New(),
		Title:          "Charge Settlement",
		Description:    "Settlement route for service charges",
	}
}

func TestTRRepo_Create(t *testing.T) {
	t.Parallel()

	t.Run("success_no_operation_routes", func(t *testing.T) {
		t.Parallel()
		r, mock := newTRRepoWithMock(t)
		mock.ExpectBegin()
		mock.ExpectExec("INSERT INTO transaction_route").WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		got, err := r.Create(context.Background(), uuid.New(), uuid.New(), validTxRoute())
		require.NoError(t, err)
		require.NotNil(t, got)
	})

	t.Run("zero_rows_returns_business_not_found", func(t *testing.T) {
		t.Parallel()
		r, mock := newTRRepoWithMock(t)
		mock.ExpectBegin()
		mock.ExpectExec("INSERT INTO transaction_route").WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectRollback()

		_, err := r.Create(context.Background(), uuid.New(), uuid.New(), validTxRoute())
		require.Error(t, err)
	})

	t.Run("exec_error_rolls_back", func(t *testing.T) {
		t.Parallel()
		r, mock := newTRRepoWithMock(t)
		mock.ExpectBegin()
		mock.ExpectExec("INSERT INTO transaction_route").WillReturnError(errTRTestBoom)
		mock.ExpectRollback()

		_, err := r.Create(context.Background(), uuid.New(), uuid.New(), validTxRoute())
		require.Error(t, err)
	})
}

func TestTRRepo_FindByID(t *testing.T) {
	t.Parallel()

	// FindByID executes a complex join. The scan columns are:
	// tr.{id,org,ledger,title,desc,created,updated,deleted} (8)
	// + otr.{id,opr_route_id,tr_route_id,created,deleted} (5)
	// + or.{id,org,ledger,title,desc,op_type,rule_type,rule_valid_if,created,updated,deleted,code} (12)
	joinCols := []string{
		"tr_id", "tr_org", "tr_ledger", "tr_title", "tr_desc", "tr_created", "tr_updated", "tr_deleted",
		"otr_id", "otr_opr", "otr_trr", "otr_created", "otr_deleted",
		"or_id", "or_org", "or_ledger", "or_title", "or_desc", "or_optype", "or_ruletype", "or_rulevalidif",
		"or_created", "or_updated", "or_deleted", "or_code",
	}

	t.Run("success_with_no_operation_routes", func(t *testing.T) {
		t.Parallel()
		r, mock := newTRRepoWithMock(t)
		now := time.Now().UTC()
		rows := sqlmock.NewRows(joinCols).
			AddRow(uuid.New(), uuid.New(), uuid.New(), "T", "D", now, now, nil,
				uuid.UUID{}, uuid.UUID{}, uuid.UUID{}, now, nil,
				uuid.UUID{}, uuid.UUID{}, uuid.UUID{}, "", "", "", "", "",
				now, now, nil, nil)
		mock.ExpectQuery("SELECT .*FROM").WillReturnRows(rows)

		got, err := r.FindByID(context.Background(), uuid.New(), uuid.New(), uuid.New())
		require.NoError(t, err)
		require.NotNil(t, got)
	})

	t.Run("empty_rows_returns_not_found", func(t *testing.T) {
		t.Parallel()
		r, mock := newTRRepoWithMock(t)
		mock.ExpectQuery("SELECT .*FROM").WillReturnRows(sqlmock.NewRows(joinCols))

		_, err := r.FindByID(context.Background(), uuid.New(), uuid.New(), uuid.New())
		require.Error(t, err)
	})

	t.Run("query_error", func(t *testing.T) {
		t.Parallel()
		r, mock := newTRRepoWithMock(t)
		mock.ExpectQuery("SELECT .*FROM").WillReturnError(errTRTestBoom)

		_, err := r.FindByID(context.Background(), uuid.New(), uuid.New(), uuid.New())
		require.Error(t, err)
	})
}

func TestTRRepo_Delete(t *testing.T) {
	t.Parallel()

	t.Run("success_no_remove", func(t *testing.T) {
		t.Parallel()
		r, mock := newTRRepoWithMock(t)
		mock.ExpectBegin()
		mock.ExpectExec("UPDATE transaction_route SET deleted_at").
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		err := r.Delete(context.Background(), uuid.New(), uuid.New(), uuid.New(), nil)
		require.NoError(t, err)
	})

	t.Run("exec_error_rolls_back", func(t *testing.T) {
		t.Parallel()
		r, mock := newTRRepoWithMock(t)
		mock.ExpectBegin()
		mock.ExpectExec("UPDATE transaction_route SET deleted_at").
			WillReturnError(errTRTestBoom)
		mock.ExpectRollback()

		err := r.Delete(context.Background(), uuid.New(), uuid.New(), uuid.New(), nil)
		require.Error(t, err)
	})
}

func TestTRRepo_Update(t *testing.T) {
	t.Parallel()

	t.Run("success_no_relationship_changes", func(t *testing.T) {
		t.Parallel()
		r, mock := newTRRepoWithMock(t)
		mock.ExpectBegin()
		mock.ExpectExec("UPDATE transaction_route SET").
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		got, err := r.Update(context.Background(), uuid.New(), uuid.New(), uuid.New(), validTxRoute(), nil, nil)
		require.NoError(t, err)
		require.NotNil(t, got)
	})

	t.Run("zero_rows_returns_not_found", func(t *testing.T) {
		t.Parallel()
		r, mock := newTRRepoWithMock(t)
		mock.ExpectBegin()
		mock.ExpectExec("UPDATE transaction_route SET").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectRollback()

		_, err := r.Update(context.Background(), uuid.New(), uuid.New(), uuid.New(), validTxRoute(), nil, nil)
		require.Error(t, err)
	})

	t.Run("exec_error_rolls_back", func(t *testing.T) {
		t.Parallel()
		r, mock := newTRRepoWithMock(t)
		mock.ExpectBegin()
		mock.ExpectExec("UPDATE transaction_route SET").WillReturnError(errTRTestBoom)
		mock.ExpectRollback()

		_, err := r.Update(context.Background(), uuid.New(), uuid.New(), uuid.New(), validTxRoute(), nil, nil)
		require.Error(t, err)
	})
}

func TestTRRepo_FindAll(t *testing.T) {
	t.Parallel()

	filter := http.Pagination{Limit: 10, SortOrder: "DESC"}

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		r, mock := newTRRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM transaction_route").WillReturnRows(trFindAllRow())

		got, _, err := r.FindAll(context.Background(), uuid.New(), uuid.New(), filter)
		require.NoError(t, err)
		require.Len(t, got, 1)
	})

	t.Run("empty_rows", func(t *testing.T) {
		t.Parallel()
		r, mock := newTRRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM transaction_route").
			WillReturnRows(sqlmock.NewRows(trFindAllCols()))

		got, _, err := r.FindAll(context.Background(), uuid.New(), uuid.New(), filter)
		require.NoError(t, err)
		require.Empty(t, got)
	})

	t.Run("query_error", func(t *testing.T) {
		t.Parallel()
		r, mock := newTRRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM transaction_route").WillReturnError(errTRTestBoom)

		_, _, err := r.FindAll(context.Background(), uuid.New(), uuid.New(), filter)
		require.Error(t, err)
	})

	t.Run("invalid_cursor_returns_error", func(t *testing.T) {
		t.Parallel()
		r, _ := newTRRepoWithMock(t)
		bad := http.Pagination{Limit: 10, SortOrder: "DESC", Cursor: "!!!bad!!!"}

		_, _, err := r.FindAll(context.Background(), uuid.New(), uuid.New(), bad)
		require.Error(t, err)
	})
}
