// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package operationroute

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

var errORTestBoom = errors.New("or boom")

func newORRepoWithMock(t *testing.T) (*OperationRoutePostgreSQLRepository, sqlmock.Sqlmock) {
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

	return &OperationRoutePostgreSQLRepository{
		connection: conn,
		tableName:  "operation_route",
	}, mock
}

func orRowCols() []string {
	return []string{
		"id", "organization_id", "ledger_id", "title", "description",
		"code", "operation_type", "account_rule_type", "account_rule_valid_if",
		"created_at", "updated_at", "deleted_at",
	}
}

func validOpRoute() *mmodel.OperationRoute {
	return &mmodel.OperationRoute{
		ID:             uuid.New(),
		OrganizationID: uuid.New(),
		LedgerID:       uuid.New(),
		Title:          "route 1",
		Description:    "desc",
		Code:           "CODE_A",
		OperationType:  "source",
	}
}

func orRow(id, orgID, ledgerID uuid.UUID) *sqlmock.Rows {
	now := time.Now().UTC()

	return sqlmock.NewRows(orRowCols()).
		AddRow(id, orgID, ledgerID, "title", "desc",
			"CODE_A", "source", "", "",
			now, now, nil)
}

func TestOpRouteRepo_Create(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		r, mock := newORRepoWithMock(t)
		mock.ExpectExec("INSERT INTO operation_route").
			WillReturnResult(sqlmock.NewResult(1, 1))

		got, err := r.Create(context.Background(), uuid.New(), uuid.New(), validOpRoute())
		require.NoError(t, err)
		require.NotNil(t, got)
	})

	t.Run("zero_rows_returns_not_found", func(t *testing.T) {
		t.Parallel()

		r, mock := newORRepoWithMock(t)
		mock.ExpectExec("INSERT INTO operation_route").
			WillReturnResult(sqlmock.NewResult(0, 0))

		_, err := r.Create(context.Background(), uuid.New(), uuid.New(), validOpRoute())
		require.Error(t, err)
	})

	t.Run("exec_error_returns_wrapped_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newORRepoWithMock(t)
		mock.ExpectExec("INSERT INTO operation_route").WillReturnError(errORTestBoom)

		_, err := r.Create(context.Background(), uuid.New(), uuid.New(), validOpRoute())
		require.Error(t, err)
	})
}

func TestOpRouteRepo_FindByID(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		r, mock := newORRepoWithMock(t)
		id := uuid.New()
		orgID := uuid.New()
		ledgerID := uuid.New()

		mock.ExpectQuery("SELECT .* FROM operation_route").
			WithArgs(orgID, ledgerID, id).
			WillReturnRows(orRow(id, orgID, ledgerID))

		got, err := r.FindByID(context.Background(), orgID, ledgerID, id)
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Equal(t, id, got.ID)
	})

	t.Run("no_rows_returns_not_found_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newORRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM operation_route").
			WillReturnRows(sqlmock.NewRows(orRowCols()))

		_, err := r.FindByID(context.Background(), uuid.New(), uuid.New(), uuid.New())
		require.Error(t, err)
	})

	t.Run("query_error_returns_wrapped_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newORRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM operation_route").WillReturnError(errORTestBoom)

		_, err := r.FindByID(context.Background(), uuid.New(), uuid.New(), uuid.New())
		require.Error(t, err)
	})
}

func TestOpRouteRepo_FindByIDs(t *testing.T) {
	t.Parallel()

	t.Run("empty_ids_returns_empty_slice", func(t *testing.T) {
		t.Parallel()

		r, _ := newORRepoWithMock(t)
		got, err := r.FindByIDs(context.Background(), uuid.New(), uuid.New(), nil)
		require.NoError(t, err)
		require.Empty(t, got)
	})

	t.Run("success_returns_rows", func(t *testing.T) {
		t.Parallel()

		r, mock := newORRepoWithMock(t)
		id := uuid.New()
		orgID := uuid.New()
		ledgerID := uuid.New()

		mock.ExpectQuery("SELECT .* FROM operation_route").
			WillReturnRows(orRow(id, orgID, ledgerID))

		got, err := r.FindByIDs(context.Background(), orgID, ledgerID, []uuid.UUID{id})
		require.NoError(t, err)
		require.Len(t, got, 1)
	})

	t.Run("query_error_returns_wrapped_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newORRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM operation_route").WillReturnError(errORTestBoom)

		_, err := r.FindByIDs(context.Background(), uuid.New(), uuid.New(), []uuid.UUID{uuid.New()})
		require.Error(t, err)
	})
}

func TestOpRouteRepo_Update(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		r, mock := newORRepoWithMock(t)
		input := &mmodel.OperationRoute{Title: "new title", Description: "new desc"}

		mock.ExpectExec("UPDATE operation_route SET").
			WillReturnResult(sqlmock.NewResult(0, 1))

		got, err := r.Update(context.Background(), uuid.New(), uuid.New(), uuid.New(), input)
		require.NoError(t, err)
		require.NotNil(t, got)
	})

	t.Run("zero_rows_returns_not_found", func(t *testing.T) {
		t.Parallel()

		r, mock := newORRepoWithMock(t)
		input := &mmodel.OperationRoute{Title: "x"}

		mock.ExpectExec("UPDATE operation_route SET").
			WillReturnResult(sqlmock.NewResult(0, 0))

		_, err := r.Update(context.Background(), uuid.New(), uuid.New(), uuid.New(), input)
		require.Error(t, err)
	})

	t.Run("exec_error_returns_wrapped_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newORRepoWithMock(t)
		input := &mmodel.OperationRoute{Title: "x"}

		mock.ExpectExec("UPDATE operation_route SET").WillReturnError(errORTestBoom)

		_, err := r.Update(context.Background(), uuid.New(), uuid.New(), uuid.New(), input)
		require.Error(t, err)
	})
}

func TestOpRouteRepo_Delete(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		r, mock := newORRepoWithMock(t)
		mock.ExpectExec("UPDATE operation_route SET deleted_at").
			WithArgs(uuid.Nil, uuid.Nil, uuid.Nil).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := r.Delete(context.Background(), uuid.Nil, uuid.Nil, uuid.Nil)
		require.NoError(t, err)
	})

	t.Run("exec_error_returns_wrapped_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newORRepoWithMock(t)
		mock.ExpectExec("UPDATE operation_route SET deleted_at").WillReturnError(errORTestBoom)

		err := r.Delete(context.Background(), uuid.New(), uuid.New(), uuid.New())
		require.Error(t, err)
	})
}

func orFindAllRow(id, orgID, ledgerID uuid.UUID) *sqlmock.Rows {
	now := time.Now().UTC()
	// FindAll uses a different column order than FindByID: code is LAST.
	return sqlmock.NewRows([]string{
		"id", "organization_id", "ledger_id", "title", "description", "operation_type",
		"account_rule_type", "account_rule_valid_if", "created_at", "updated_at", "deleted_at", "code",
	}).AddRow(id, orgID, ledgerID, "title", "desc", "source",
		"", "", now, now, nil, "CODE_A")
}

func TestOpRouteRepo_FindAll(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		r, mock := newORRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM operation_route").
			WillReturnRows(orFindAllRow(uuid.New(), uuid.New(), uuid.New()))

		got, _, err := r.FindAll(context.Background(), uuid.New(), uuid.New(), http.Pagination{Limit: 10, SortOrder: "desc"})
		require.NoError(t, err)
		require.Len(t, got, 1)
	})

	t.Run("query_error_returns_wrapped_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newORRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM operation_route").WillReturnError(errORTestBoom)

		_, _, err := r.FindAll(context.Background(), uuid.New(), uuid.New(), http.Pagination{Limit: 10, SortOrder: "desc"})
		require.Error(t, err)
	})
}

func TestOpRouteRepo_HasTransactionRouteLinks(t *testing.T) {
	t.Parallel()

	t.Run("returns_true", func(t *testing.T) {
		t.Parallel()

		r, mock := newORRepoWithMock(t)
		mock.ExpectQuery("SELECT EXISTS").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

		has, err := r.HasTransactionRouteLinks(context.Background(), uuid.New())
		require.NoError(t, err)
		require.True(t, has)
	})

	t.Run("returns_false", func(t *testing.T) {
		t.Parallel()

		r, mock := newORRepoWithMock(t)
		mock.ExpectQuery("SELECT EXISTS").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

		has, err := r.HasTransactionRouteLinks(context.Background(), uuid.New())
		require.NoError(t, err)
		require.False(t, has)
	})

	t.Run("scan_error_returns_wrapped_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newORRepoWithMock(t)
		mock.ExpectQuery("SELECT EXISTS").WillReturnError(errORTestBoom)

		_, err := r.HasTransactionRouteLinks(context.Background(), uuid.New())
		require.Error(t, err)
	})
}

func TestOpRouteRepo_FindTransactionRouteIDs(t *testing.T) {
	t.Parallel()

	t.Run("success_returns_ids", func(t *testing.T) {
		t.Parallel()

		r, mock := newORRepoWithMock(t)
		id1 := uuid.New()
		id2 := uuid.New()
		mock.ExpectQuery("SELECT transaction_route_id FROM operation_transaction_route").
			WillReturnRows(sqlmock.NewRows([]string{"transaction_route_id"}).
				AddRow(id1).AddRow(id2))

		ids, err := r.FindTransactionRouteIDs(context.Background(), uuid.New())
		require.NoError(t, err)
		require.Len(t, ids, 2)
	})

	t.Run("empty_result_returns_empty_slice", func(t *testing.T) {
		t.Parallel()

		r, mock := newORRepoWithMock(t)
		mock.ExpectQuery("SELECT transaction_route_id FROM operation_transaction_route").
			WillReturnRows(sqlmock.NewRows([]string{"transaction_route_id"}))

		ids, err := r.FindTransactionRouteIDs(context.Background(), uuid.New())
		require.NoError(t, err)
		require.Empty(t, ids)
	})

	t.Run("query_error_returns_wrapped_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newORRepoWithMock(t)
		mock.ExpectQuery("SELECT transaction_route_id FROM operation_transaction_route").
			WillReturnError(errORTestBoom)

		_, err := r.FindTransactionRouteIDs(context.Background(), uuid.New())
		require.Error(t, err)
	})
}
