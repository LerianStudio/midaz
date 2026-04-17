// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package operation

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

// errOpTestBoom is a sentinel injected into sqlmock to simulate database failures.
var errOpTestBoom = errors.New("op boom")

func newOpRepoWithMock(t *testing.T) (*OperationPostgreSQLRepository, sqlmock.Sqlmock) {
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

	return &OperationPostgreSQLRepository{
		connection: conn,
		tableName:  "public.operation",
	}, mock
}

func opRowCols() []string {
	return []string{
		"id", "transaction_id", "description", "type", "asset_code",
		"amount", "available_balance", "on_hold_balance",
		"available_balance_after", "on_hold_balance_after",
		"status", "status_description", "account_id", "account_alias",
		"balance_id", "chart_of_accounts", "organization_id", "ledger_id",
		"created_at", "updated_at", "deleted_at", "route",
		"balance_affected", "balance_key", "balance_version_before", "balance_version_after",
	}
}

func opValidInput() *Operation {
	amt := decimal.NewFromInt(100)
	avail := decimal.NewFromInt(1000)
	onHold := decimal.NewFromInt(0)

	return &Operation{
		ID:              uuid.NewString(),
		TransactionID:   uuid.NewString(),
		Description:     "op",
		Type:            "DEBIT",
		AssetCode:       "USD",
		ChartOfAccounts: "GRP",
		Amount:          Amount{Value: &amt},
		Balance:         Balance{Available: &avail, OnHold: &onHold},
		BalanceAfter:    Balance{Available: &avail, OnHold: &onHold},
		Status:          Status{Code: "ACTIVE"},
		AccountID:       uuid.NewString(),
		AccountAlias:    "@a",
		BalanceID:       uuid.NewString(),
		BalanceKey:      "default",
		OrganizationID:  uuid.NewString(),
		LedgerID:        uuid.NewString(),
	}
}

func opRow(id, txID, orgID, ledgerID uuid.UUID) *sqlmock.Rows {
	amt := decimal.NewFromInt(100)
	now := time.Now().UTC()

	return sqlmock.NewRows(opRowCols()).
		AddRow(
			id.String(), txID.String(), "op desc", "DEBIT", "USD",
			&amt, &amt, &amt, &amt, &amt,
			"ACTIVE", nil, uuid.NewString(), "@a",
			uuid.NewString(), "GRP", orgID.String(), ledgerID.String(),
			now, now, nil, nil,
			true, "default", int64(1), int64(2),
		)
}

func TestOperationRepo_Create(t *testing.T) {
	t.Parallel()

	t.Run("success_returns_operation", func(t *testing.T) {
		t.Parallel()

		r, mock := newOpRepoWithMock(t)
		mock.ExpectExec("INSERT INTO public.operation").
			WillReturnResult(sqlmock.NewResult(1, 1))

		got, err := r.Create(context.Background(), opValidInput())
		require.NoError(t, err)
		require.NotNil(t, got)
	})

	t.Run("zero_rows_returns_not_found_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newOpRepoWithMock(t)
		mock.ExpectExec("INSERT INTO public.operation").
			WillReturnResult(sqlmock.NewResult(0, 0))

		_, err := r.Create(context.Background(), opValidInput())
		require.Error(t, err)
	})

	t.Run("exec_error_returns_wrapped_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newOpRepoWithMock(t)
		mock.ExpectExec("INSERT INTO public.operation").WillReturnError(errOpTestBoom)

		_, err := r.Create(context.Background(), opValidInput())
		require.Error(t, err)
	})
}

func TestOperationRepo_Find(t *testing.T) {
	t.Parallel()

	t.Run("success_returns_entity", func(t *testing.T) {
		t.Parallel()

		r, mock := newOpRepoWithMock(t)
		opID := uuid.New()
		txID := uuid.New()
		orgID := uuid.New()
		ledgerID := uuid.New()

		mock.ExpectQuery("SELECT .* FROM public.operation").
			WillReturnRows(opRow(opID, txID, orgID, ledgerID))

		got, err := r.Find(context.Background(), orgID, ledgerID, txID, opID)
		require.NoError(t, err)
		require.NotNil(t, got)
		require.Equal(t, opID.String(), got.ID)
	})

	t.Run("no_rows_returns_business_not_found_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newOpRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM public.operation").
			WillReturnRows(sqlmock.NewRows(opRowCols()))

		_, err := r.Find(context.Background(), uuid.New(), uuid.New(), uuid.New(), uuid.New())
		require.Error(t, err)
	})

	t.Run("query_error_returns_wrapped_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newOpRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM public.operation").WillReturnError(errOpTestBoom)

		_, err := r.Find(context.Background(), uuid.New(), uuid.New(), uuid.New(), uuid.New())
		require.Error(t, err)
	})
}

func TestOperationRepo_FindByAccount(t *testing.T) {
	t.Parallel()

	t.Run("success_returns_entity", func(t *testing.T) {
		t.Parallel()

		r, mock := newOpRepoWithMock(t)
		opID := uuid.New()
		mock.ExpectQuery("SELECT .* FROM public.operation").
			WillReturnRows(opRow(opID, uuid.New(), uuid.New(), uuid.New()))

		got, err := r.FindByAccount(context.Background(), uuid.New(), uuid.New(), uuid.New(), opID)
		require.NoError(t, err)
		require.NotNil(t, got)
	})

	t.Run("no_rows_returns_business_not_found", func(t *testing.T) {
		t.Parallel()

		r, mock := newOpRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM public.operation").
			WillReturnRows(sqlmock.NewRows(opRowCols()))

		_, err := r.FindByAccount(context.Background(), uuid.New(), uuid.New(), uuid.New(), uuid.New())
		require.Error(t, err)
	})
}

func TestOperationRepo_Delete(t *testing.T) {
	t.Parallel()

	t.Run("success_soft_deletes_one", func(t *testing.T) {
		t.Parallel()

		r, mock := newOpRepoWithMock(t)
		mock.ExpectExec("UPDATE public.operation SET deleted_at").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := r.Delete(context.Background(), uuid.New(), uuid.New(), uuid.New())
		require.NoError(t, err)
	})

	t.Run("zero_rows_returns_not_found", func(t *testing.T) {
		t.Parallel()

		r, mock := newOpRepoWithMock(t)
		mock.ExpectExec("UPDATE public.operation SET deleted_at").
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := r.Delete(context.Background(), uuid.New(), uuid.New(), uuid.New())
		require.Error(t, err)
	})

	t.Run("exec_error_returns_wrapped_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newOpRepoWithMock(t)
		mock.ExpectExec("UPDATE public.operation SET deleted_at").WillReturnError(errOpTestBoom)

		err := r.Delete(context.Background(), uuid.New(), uuid.New(), uuid.New())
		require.Error(t, err)
	})
}

func TestOperationRepo_Update(t *testing.T) {
	t.Parallel()

	t.Run("success_updates_one_row", func(t *testing.T) {
		t.Parallel()

		r, mock := newOpRepoWithMock(t)
		input := &Operation{Description: "updated"}

		mock.ExpectExec("UPDATE public.operation SET").
			WillReturnResult(sqlmock.NewResult(0, 1))

		got, err := r.Update(context.Background(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), input)
		require.NoError(t, err)
		require.NotNil(t, got)
	})

	t.Run("zero_rows_returns_not_found", func(t *testing.T) {
		t.Parallel()

		r, mock := newOpRepoWithMock(t)
		input := &Operation{Description: "x"}

		mock.ExpectExec("UPDATE public.operation SET").
			WillReturnResult(sqlmock.NewResult(0, 0))

		_, err := r.Update(context.Background(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), input)
		require.Error(t, err)
	})

	t.Run("exec_error_returns_wrapped_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newOpRepoWithMock(t)
		input := &Operation{Description: "x"}

		mock.ExpectExec("UPDATE public.operation SET").WillReturnError(errOpTestBoom)

		_, err := r.Update(context.Background(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), input)
		require.Error(t, err)
	})
}

func TestOperationRepo_FindAll(t *testing.T) {
	t.Parallel()

	t.Run("success_returns_rows", func(t *testing.T) {
		t.Parallel()

		r, mock := newOpRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM public.operation").
			WillReturnRows(opRow(uuid.New(), uuid.New(), uuid.New(), uuid.New()))

		ops, _, err := r.FindAll(context.Background(), uuid.New(), uuid.New(), uuid.New(), http.Pagination{Limit: 10, SortOrder: "desc"})
		require.NoError(t, err)
		require.Len(t, ops, 1)
	})

	t.Run("empty_returns_empty_slice", func(t *testing.T) {
		t.Parallel()

		r, mock := newOpRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM public.operation").
			WillReturnRows(sqlmock.NewRows(opRowCols()))

		ops, _, err := r.FindAll(context.Background(), uuid.New(), uuid.New(), uuid.New(), http.Pagination{Limit: 10, SortOrder: "desc"})
		require.NoError(t, err)
		require.Empty(t, ops)
	})

	t.Run("query_error_returns_wrapped_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newOpRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM public.operation").WillReturnError(errOpTestBoom)

		_, _, err := r.FindAll(context.Background(), uuid.New(), uuid.New(), uuid.New(), http.Pagination{Limit: 10, SortOrder: "desc"})
		require.Error(t, err)
	})
}

func TestOperationRepo_FindAllByAccount(t *testing.T) {
	t.Parallel()

	t.Run("success_returns_rows", func(t *testing.T) {
		t.Parallel()

		r, mock := newOpRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM public.operation").
			WillReturnRows(opRow(uuid.New(), uuid.New(), uuid.New(), uuid.New()))

		ops, _, err := r.FindAllByAccount(context.Background(), uuid.New(), uuid.New(), uuid.New(), nil, http.Pagination{Limit: 10, SortOrder: "desc"})
		require.NoError(t, err)
		require.Len(t, ops, 1)
	})

	t.Run("query_error_returns_wrapped_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newOpRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM public.operation").WillReturnError(errOpTestBoom)

		_, _, err := r.FindAllByAccount(context.Background(), uuid.New(), uuid.New(), uuid.New(), nil, http.Pagination{Limit: 10, SortOrder: "desc"})
		require.Error(t, err)
	})
}

func TestOperationRepo_ListByIDs(t *testing.T) {
	t.Parallel()

	t.Run("success_returns_rows", func(t *testing.T) {
		t.Parallel()

		r, mock := newOpRepoWithMock(t)
		id1 := uuid.New()
		mock.ExpectQuery("SELECT .* FROM public.operation").
			WillReturnRows(opRow(id1, uuid.New(), uuid.New(), uuid.New()))

		got, err := r.ListByIDs(context.Background(), uuid.New(), uuid.New(), []uuid.UUID{id1})
		require.NoError(t, err)
		require.Len(t, got, 1)
	})

	t.Run("query_error_returns_wrapped_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newOpRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM public.operation").WillReturnError(errOpTestBoom)

		_, err := r.ListByIDs(context.Background(), uuid.New(), uuid.New(), []uuid.UUID{uuid.New()})
		require.Error(t, err)
	})
}

func TestOperationRepo_CreateBatch(t *testing.T) {
	t.Parallel()

	t.Run("empty_slice_noop", func(t *testing.T) {
		t.Parallel()

		r, _ := newOpRepoWithMock(t)
		err := r.CreateBatch(context.Background(), nil)
		require.NoError(t, err)
	})

	t.Run("success_commits", func(t *testing.T) {
		t.Parallel()

		r, mock := newOpRepoWithMock(t)
		mock.ExpectBegin()
		mock.ExpectExec("INSERT INTO public.operation").
			WillReturnResult(sqlmock.NewResult(1, 1))
		mock.ExpectCommit()

		err := r.CreateBatch(context.Background(), []*Operation{opValidInput()})
		require.NoError(t, err)
	})

	t.Run("exec_error_rolls_back", func(t *testing.T) {
		t.Parallel()

		r, mock := newOpRepoWithMock(t)
		mock.ExpectBegin()
		mock.ExpectExec("INSERT INTO public.operation").WillReturnError(errOpTestBoom)
		mock.ExpectRollback()

		err := r.CreateBatch(context.Background(), []*Operation{opValidInput()})
		require.Error(t, err)
	})
}
