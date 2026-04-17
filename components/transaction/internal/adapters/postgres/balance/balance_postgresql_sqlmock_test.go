// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package balance

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

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
)

var errBalTestBoom = errors.New("bal boom")

func newBalRepoWithMock(t *testing.T) (*BalancePostgreSQLRepository, sqlmock.Sqlmock) {
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

	return &BalancePostgreSQLRepository{
		connection: conn,
		tableName:  "public.balance",
	}, mock
}

func balRowColsFind() []string {
	// Matches balanceColumnList order used by Find/ListAll
	return []string{
		"id", "organization_id", "ledger_id", "account_id", "alias",
		"asset_code", "available", "on_hold", "version", "account_type",
		"allow_sending", "allow_receiving", "created_at", "updated_at",
		"deleted_at", "key",
	}
}

func balRowColsFindByAccountKey() []string {
	// FindByAccountIDAndKey uses a different order: key BEFORE asset_code, deleted_at LAST
	return []string{
		"id", "organization_id", "ledger_id", "account_id", "alias", "key",
		"asset_code", "available", "on_hold", "version", "account_type",
		"allow_sending", "allow_receiving", "created_at", "updated_at", "deleted_at",
	}
}

func validBalance() *mmodel.Balance {
	return &mmodel.Balance{
		ID:             uuid.NewString(),
		OrganizationID: uuid.NewString(),
		LedgerID:       uuid.NewString(),
		AccountID:      uuid.NewString(),
		Alias:          "@acc",
		Key:            "default",
		AssetCode:      "USD",
		Available:      decimal.NewFromInt(1000),
		OnHold:         decimal.NewFromInt(0),
		Version:        1,
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
	}
}

func balFindRow() *sqlmock.Rows {
	now := time.Now().UTC()

	return sqlmock.NewRows(balRowColsFind()).
		AddRow(uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), "@acc",
			"USD", decimal.NewFromInt(1000), decimal.NewFromInt(0), int64(1), "deposit",
			true, true, now, now, nil, "default")
}

func TestBalanceRepo_Create(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		r, mock := newBalRepoWithMock(t)
		mock.ExpectExec("INSERT INTO public.balance").
			WillReturnResult(sqlmock.NewResult(1, 1))

		err := r.Create(context.Background(), validBalance())
		require.NoError(t, err)
	})

	t.Run("zero_rows_returns_not_found", func(t *testing.T) {
		t.Parallel()

		r, mock := newBalRepoWithMock(t)
		mock.ExpectExec("INSERT INTO public.balance").
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := r.Create(context.Background(), validBalance())
		require.Error(t, err)
	})

	t.Run("exec_error_returns_wrapped", func(t *testing.T) {
		t.Parallel()

		r, mock := newBalRepoWithMock(t)
		mock.ExpectExec("INSERT INTO public.balance").WillReturnError(errBalTestBoom)

		err := r.Create(context.Background(), validBalance())
		require.Error(t, err)
	})
}

func TestBalanceRepo_CreateIfNotExists(t *testing.T) {
	t.Parallel()

	t.Run("success_inserts", func(t *testing.T) {
		t.Parallel()

		r, mock := newBalRepoWithMock(t)
		mock.ExpectExec("INSERT INTO public.balance").
			WillReturnResult(sqlmock.NewResult(1, 1))

		err := r.CreateIfNotExists(context.Background(), validBalance())
		require.NoError(t, err)
	})

	t.Run("zero_rows_is_idempotent_no_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newBalRepoWithMock(t)
		mock.ExpectExec("INSERT INTO public.balance").
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := r.CreateIfNotExists(context.Background(), validBalance())
		require.NoError(t, err)
	})

	t.Run("exec_error_returns_wrapped", func(t *testing.T) {
		t.Parallel()

		r, mock := newBalRepoWithMock(t)
		mock.ExpectExec("INSERT INTO public.balance").WillReturnError(errBalTestBoom)

		err := r.CreateIfNotExists(context.Background(), validBalance())
		require.Error(t, err)
	})
}

func TestBalanceRepo_Find(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		r, mock := newBalRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM public.balance").
			WillReturnRows(balFindRow())

		got, err := r.Find(context.Background(), uuid.New(), uuid.New(), uuid.New())
		require.NoError(t, err)
		require.NotNil(t, got)
	})

	t.Run("no_rows_returns_business_not_found", func(t *testing.T) {
		t.Parallel()

		r, mock := newBalRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM public.balance").
			WillReturnRows(sqlmock.NewRows(balRowColsFind()))

		_, err := r.Find(context.Background(), uuid.New(), uuid.New(), uuid.New())
		require.Error(t, err)
	})

	t.Run("query_error_returns_wrapped", func(t *testing.T) {
		t.Parallel()

		r, mock := newBalRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM public.balance").WillReturnError(errBalTestBoom)

		_, err := r.Find(context.Background(), uuid.New(), uuid.New(), uuid.New())
		require.Error(t, err)
	})
}

func TestBalanceRepo_FindByAccountIDAndKey(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		r, mock := newBalRepoWithMock(t)
		now := time.Now().UTC()
		rows := sqlmock.NewRows(balRowColsFindByAccountKey()).
			AddRow(uuid.NewString(), uuid.NewString(), uuid.NewString(), uuid.NewString(), "@acc", "default",
				"USD", decimal.NewFromInt(1000), decimal.NewFromInt(0), int64(1), "deposit",
				true, true, now, now, nil)

		mock.ExpectQuery("SELECT .* FROM public.balance").
			WillReturnRows(rows)

		got, err := r.FindByAccountIDAndKey(context.Background(), uuid.New(), uuid.New(), uuid.New(), "default")
		require.NoError(t, err)
		require.NotNil(t, got)
	})

	t.Run("no_rows_returns_business_not_found", func(t *testing.T) {
		t.Parallel()

		r, mock := newBalRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM public.balance").
			WillReturnRows(sqlmock.NewRows(balRowColsFindByAccountKey()))

		_, err := r.FindByAccountIDAndKey(context.Background(), uuid.New(), uuid.New(), uuid.New(), "default")
		require.Error(t, err)
	})

	t.Run("query_error_returns_wrapped", func(t *testing.T) {
		t.Parallel()

		r, mock := newBalRepoWithMock(t)
		mock.ExpectQuery("SELECT .* FROM public.balance").WillReturnError(errBalTestBoom)

		_, err := r.FindByAccountIDAndKey(context.Background(), uuid.New(), uuid.New(), uuid.New(), "default")
		require.Error(t, err)
	})
}

func TestBalanceRepo_ExistsByAccountIDAndKey(t *testing.T) {
	t.Parallel()

	t.Run("returns_true", func(t *testing.T) {
		t.Parallel()

		r, mock := newBalRepoWithMock(t)
		mock.ExpectQuery("SELECT EXISTS").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(true))

		got, err := r.ExistsByAccountIDAndKey(context.Background(), uuid.New(), uuid.New(), uuid.New(), "default")
		require.NoError(t, err)
		require.True(t, got)
	})

	t.Run("returns_false", func(t *testing.T) {
		t.Parallel()

		r, mock := newBalRepoWithMock(t)
		mock.ExpectQuery("SELECT EXISTS").
			WillReturnRows(sqlmock.NewRows([]string{"exists"}).AddRow(false))

		got, err := r.ExistsByAccountIDAndKey(context.Background(), uuid.New(), uuid.New(), uuid.New(), "default")
		require.NoError(t, err)
		require.False(t, got)
	})

	t.Run("query_error_returns_wrapped", func(t *testing.T) {
		t.Parallel()

		r, mock := newBalRepoWithMock(t)
		mock.ExpectQuery("SELECT EXISTS").WillReturnError(errBalTestBoom)

		_, err := r.ExistsByAccountIDAndKey(context.Background(), uuid.New(), uuid.New(), uuid.New(), "default")
		require.Error(t, err)
	})
}

func TestBalanceRepo_Delete(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		r, mock := newBalRepoWithMock(t)
		mock.ExpectExec("UPDATE public.balance[\\s\\S]*deleted_at = NOW").
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := r.Delete(context.Background(), uuid.New(), uuid.New(), uuid.New())
		require.NoError(t, err)
	})

	t.Run("zero_rows_returns_not_found", func(t *testing.T) {
		t.Parallel()

		r, mock := newBalRepoWithMock(t)
		mock.ExpectExec("UPDATE public.balance[\\s\\S]*deleted_at = NOW").
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := r.Delete(context.Background(), uuid.New(), uuid.New(), uuid.New())
		require.Error(t, err)
	})

	t.Run("exec_error_returns_wrapped", func(t *testing.T) {
		t.Parallel()

		r, mock := newBalRepoWithMock(t)
		mock.ExpectExec("UPDATE public.balance[\\s\\S]*deleted_at = NOW").WillReturnError(errBalTestBoom)

		err := r.Delete(context.Background(), uuid.New(), uuid.New(), uuid.New())
		require.Error(t, err)
	})
}

func TestBalanceRepo_DeleteAllByIDs(t *testing.T) {
	t.Parallel()

	t.Run("empty_ids_noop", func(t *testing.T) {
		t.Parallel()

		r, _ := newBalRepoWithMock(t)
		err := r.DeleteAllByIDs(context.Background(), uuid.New(), uuid.New(), nil)
		require.NoError(t, err)
	})

	t.Run("success_matches_rowcount", func(t *testing.T) {
		t.Parallel()

		r, mock := newBalRepoWithMock(t)
		ids := []uuid.UUID{uuid.New(), uuid.New()}

		mock.ExpectBegin()
		mock.ExpectExec("UPDATE public.balance").
			WillReturnResult(sqlmock.NewResult(0, 2))
		mock.ExpectCommit()

		err := r.DeleteAllByIDs(context.Background(), uuid.New(), uuid.New(), ids)
		require.NoError(t, err)
	})

	t.Run("rowcount_mismatch_rollbacks_and_errors", func(t *testing.T) {
		t.Parallel()

		r, mock := newBalRepoWithMock(t)
		ids := []uuid.UUID{uuid.New(), uuid.New()}

		mock.ExpectBegin()
		mock.ExpectExec("UPDATE public.balance").
			WillReturnResult(sqlmock.NewResult(0, 1)) // only 1 of 2
		mock.ExpectRollback()

		err := r.DeleteAllByIDs(context.Background(), uuid.New(), uuid.New(), ids)
		require.Error(t, err)
	})

	t.Run("exec_error_rolls_back", func(t *testing.T) {
		t.Parallel()

		r, mock := newBalRepoWithMock(t)
		ids := []uuid.UUID{uuid.New()}

		mock.ExpectBegin()
		mock.ExpectExec("UPDATE public.balance").WillReturnError(errBalTestBoom)
		mock.ExpectRollback()

		err := r.DeleteAllByIDs(context.Background(), uuid.New(), uuid.New(), ids)
		require.Error(t, err)
	})
}

func TestBalanceRepo_Update(t *testing.T) {
	t.Parallel()

	t.Run("success_returns_updated_row", func(t *testing.T) {
		t.Parallel()

		r, mock := newBalRepoWithMock(t)
		allow := false

		mock.ExpectQuery("UPDATE public.balance SET").
			WillReturnRows(balFindRow())

		got, err := r.Update(context.Background(), uuid.New(), uuid.New(), uuid.New(),
			mmodel.UpdateBalance{AllowSending: &allow, AllowReceiving: &allow})
		require.NoError(t, err)
		require.NotNil(t, got)
	})

	t.Run("no_rows_returns_not_found", func(t *testing.T) {
		t.Parallel()

		r, mock := newBalRepoWithMock(t)
		mock.ExpectQuery("UPDATE public.balance SET").
			WillReturnRows(sqlmock.NewRows(balRowColsFind()))

		_, err := r.Update(context.Background(), uuid.New(), uuid.New(), uuid.New(), mmodel.UpdateBalance{})
		require.Error(t, err)
	})

	t.Run("query_error_returns_wrapped", func(t *testing.T) {
		t.Parallel()

		r, mock := newBalRepoWithMock(t)
		mock.ExpectQuery("UPDATE public.balance SET").WillReturnError(errBalTestBoom)

		_, err := r.Update(context.Background(), uuid.New(), uuid.New(), uuid.New(), mmodel.UpdateBalance{})
		require.Error(t, err)
	})
}
