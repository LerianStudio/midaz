// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package account

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
)

// newRepoWithMock builds an AccountPostgreSQLRepository backed by go-sqlmock, wrapped
// through dbresolver so that the production code path (r.connection.GetDB()) is exercised
// without touching a real Postgres instance. Closing the returned *sql.DB is the caller's
// responsibility (handled via t.Cleanup).
func newRepoWithMock(t *testing.T) (*AccountPostgreSQLRepository, sqlmock.Sqlmock) {
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

	return &AccountPostgreSQLRepository{
		connection: conn,
		tableName:  "account",
	}, mock
}

// accountRow is the 17-column row layout emitted by AccountPostgreSQLModel Scan calls.
func accountRow() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "name", "parent_account_id", "entity_id", "asset_code",
		"organization_id", "ledger_id", "portfolio_id", "segment_id",
		"status", "status_description", "alias", "type",
		"created_at", "updated_at", "deleted_at", "blocked",
	})
}

func TestAccountRepository_Create(t *testing.T) {
	t.Parallel()

	t.Run("success_happy_path", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("INSERT INTO account").
			WillReturnResult(sqlmock.NewResult(1, 1))

		acc := &mmodel.Account{
			ID:             uuid.NewString(),
			Name:           "wallet",
			OrganizationID: uuid.NewString(),
			LedgerID:       uuid.NewString(),
			AssetCode:      "USD",
			Type:           "deposit",
			Status:         mmodel.Status{Code: "ACTIVE"},
		}

		got, err := r.Create(context.Background(), acc)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "wallet", got.Name)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("no_rows_affected_maps_to_not_found", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("INSERT INTO account").
			WillReturnResult(sqlmock.NewResult(0, 0))

		_, err := r.Create(context.Background(), &mmodel.Account{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no rows affected")
	})

	t.Run("exec_error_is_wrapped", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("INSERT INTO account").
			WillReturnError(errTestBoom)

		_, err := r.Create(context.Background(), &mmodel.Account{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "execute insert")
	})

	t.Run("pg_constraint_error_is_mapped", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		pgErr := &pgconn.PgError{ConstraintName: "account_asset_code_fkey"}
		mock.ExpectExec("INSERT INTO account").WillReturnError(pgErr)

		_, err := r.Create(context.Background(), &mmodel.Account{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "validate pg error on create")
	})
}

func TestAccountRepository_FindAll(t *testing.T) {
	t.Parallel()

	t.Run("returns_rows", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		rows := accountRow().
			AddRow(
				uuid.NewString(), "acc1", nil, nil, "USD",
				uuid.NewString(), uuid.NewString(), nil, nil,
				"ACTIVE", nil, nil, "deposit",
				anyTime(), anyTime(), sql.NullTime{}, false,
			).
			AddRow(
				uuid.NewString(), "acc2", nil, nil, "BRL",
				uuid.NewString(), uuid.NewString(), nil, nil,
				"ACTIVE", nil, nil, "savings",
				anyTime(), anyTime(), sql.NullTime{}, false,
			)

		mock.ExpectQuery("SELECT .* FROM account").WillReturnRows(rows)

		portfolioID := uuid.New()
		got, err := r.FindAll(context.Background(), uuid.New(), uuid.New(), &portfolioID, http.Pagination{
			Limit: 10, Page: 1, SortOrder: "asc",
		})
		require.NoError(t, err)
		require.Len(t, got, 2)
	})

	t.Run("query_error_is_wrapped", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM account").WillReturnError(errTestDBUnavailable)

		_, err := r.FindAll(context.Background(), uuid.New(), uuid.New(), nil, http.Pagination{
			Limit: 10, Page: 1, SortOrder: "desc",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "execute find all")
	})

	t.Run("scan_error_is_wrapped", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		// Row with wrong column count will trigger scan error.
		badRows := sqlmock.NewRows([]string{"id"}).AddRow("only-one-col")
		mock.ExpectQuery("SELECT .* FROM account").WillReturnRows(badRows)

		_, err := r.FindAll(context.Background(), uuid.New(), uuid.New(), nil, http.Pagination{
			Limit: 10, Page: 1, SortOrder: "asc",
		})
		require.Error(t, err)
	})
}

func TestAccountRepository_Find(t *testing.T) {
	t.Parallel()

	t.Run("found", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		id := uuid.NewString()
		rows := accountRow().AddRow(
			id, "wallet", nil, nil, "USD",
			uuid.NewString(), uuid.NewString(), nil, nil,
			"ACTIVE", nil, nil, "deposit",
			anyTime(), anyTime(), sql.NullTime{}, false,
		)
		mock.ExpectQuery("SELECT .* FROM account").WillReturnRows(rows)

		got, err := r.Find(context.Background(), uuid.New(), uuid.New(), nil, uuid.New())
		require.NoError(t, err)
		require.NotNil(t, got)
	})

	t.Run("not_found_maps_to_entity_not_found", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM account").WillReturnError(sql.ErrNoRows)

		_, err := r.Find(context.Background(), uuid.New(), uuid.New(), nil, uuid.New())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("other_scan_error_is_wrapped", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM account").WillReturnError(errTestBoom)

		_, err := r.Find(context.Background(), uuid.New(), uuid.New(), nil, uuid.New())
		require.Error(t, err)
	})

	t.Run("with_portfolio_id_filter", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		rows := accountRow().AddRow(
			uuid.NewString(), "wallet", nil, nil, "USD",
			uuid.NewString(), uuid.NewString(), nil, nil,
			"ACTIVE", nil, nil, "deposit",
			anyTime(), anyTime(), sql.NullTime{}, false,
		)
		mock.ExpectQuery("SELECT .* FROM account .* portfolio_id = ").WillReturnRows(rows)

		pid := uuid.New()
		_, err := r.Find(context.Background(), uuid.New(), uuid.New(), &pid, uuid.New())
		require.NoError(t, err)
	})
}

func TestAccountRepository_FindWithDeleted(t *testing.T) {
	t.Parallel()

	t.Run("found", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		rows := accountRow().AddRow(
			uuid.NewString(), "del", nil, nil, "USD",
			uuid.NewString(), uuid.NewString(), nil, nil,
			"ACTIVE", nil, nil, "deposit",
			anyTime(), anyTime(), sql.NullTime{}, false,
		)
		mock.ExpectQuery("SELECT .* FROM account").WillReturnRows(rows)

		pid := uuid.New()
		_, err := r.FindWithDeleted(context.Background(), uuid.New(), uuid.New(), &pid, uuid.New())
		require.NoError(t, err)
	})

	t.Run("not_found", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM account").WillReturnError(sql.ErrNoRows)

		_, err := r.FindWithDeleted(context.Background(), uuid.New(), uuid.New(), nil, uuid.New())
		require.Error(t, err)
	})
}

func TestAccountRepository_FindAlias(t *testing.T) {
	t.Parallel()

	t.Run("found", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		rows := accountRow().AddRow(
			uuid.NewString(), "wallet", nil, nil, "USD",
			uuid.NewString(), uuid.NewString(), nil, nil,
			"ACTIVE", nil, nil, "deposit",
			anyTime(), anyTime(), sql.NullTime{}, false,
		)
		mock.ExpectQuery("SELECT .* FROM account").WillReturnRows(rows)

		pid := uuid.New()
		_, err := r.FindAlias(context.Background(), uuid.New(), uuid.New(), &pid, "wallet-alias")
		require.NoError(t, err)
	})

	t.Run("not_found_returns_alias_not_found", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM account").WillReturnError(sql.ErrNoRows)

		_, err := r.FindAlias(context.Background(), uuid.New(), uuid.New(), nil, "alias")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "alias")
	})

	t.Run("generic_scan_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM account").WillReturnError(errTestBoom)

		_, err := r.FindAlias(context.Background(), uuid.New(), uuid.New(), nil, "alias")
		require.Error(t, err)
	})
}

func TestAccountRepository_FindByAlias(t *testing.T) {
	t.Parallel()

	t.Run("alias_taken_returns_true_and_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		rows := sqlmock.NewRows([]string{"?column?"}).AddRow(1)
		mock.ExpectQuery("SELECT 1 FROM account").WillReturnRows(rows)

		taken, err := r.FindByAlias(context.Background(), uuid.New(), uuid.New(), "taken-alias")
		require.Error(t, err)
		assert.True(t, taken)
	})

	t.Run("alias_free_returns_false_nil", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT 1 FROM account").WillReturnError(sql.ErrNoRows)

		taken, err := r.FindByAlias(context.Background(), uuid.New(), uuid.New(), "free-alias")
		require.NoError(t, err)
		assert.False(t, taken)
	})

	t.Run("db_error_is_wrapped", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT 1 FROM account").WillReturnError(errTestDBDown)

		_, err := r.FindByAlias(context.Background(), uuid.New(), uuid.New(), "alias")
		require.Error(t, err)
	})
}

func TestAccountRepository_ListByIDs(t *testing.T) {
	t.Parallel()

	t.Run("returns_accounts", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		rows := accountRow().
			AddRow(uuid.NewString(), "a1", nil, nil, "USD", uuid.NewString(), uuid.NewString(),
				nil, nil, "ACTIVE", nil, nil, "deposit",
				anyTime(), anyTime(), sql.NullTime{}, false).
			AddRow(uuid.NewString(), "a2", nil, nil, "EUR", uuid.NewString(), uuid.NewString(),
				nil, nil, "ACTIVE", nil, nil, "deposit",
				anyTime(), anyTime(), sql.NullTime{}, false)

		mock.ExpectQuery("SELECT .* FROM account").WillReturnRows(rows)

		pid := uuid.New()
		got, err := r.ListByIDs(context.Background(), uuid.New(), uuid.New(), &pid, []uuid.UUID{uuid.New(), uuid.New()})
		require.NoError(t, err)
		assert.Len(t, got, 2)
	})

	t.Run("query_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM account").WillReturnError(errTestBoom)

		_, err := r.ListByIDs(context.Background(), uuid.New(), uuid.New(), nil, []uuid.UUID{uuid.New()})
		require.Error(t, err)
	})
}

func TestAccountRepository_ListByAlias(t *testing.T) {
	t.Parallel()

	t.Run("returns_accounts", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		rows := accountRow().AddRow(
			uuid.NewString(), "w", nil, nil, "USD",
			uuid.NewString(), uuid.NewString(), nil, nil,
			"ACTIVE", nil, nil, "deposit",
			anyTime(), anyTime(), sql.NullTime{}, false,
		)
		mock.ExpectQuery("SELECT .* FROM account").WillReturnRows(rows)

		got, err := r.ListByAlias(context.Background(), uuid.New(), uuid.New(), uuid.New(), []string{"x"})
		require.NoError(t, err)
		assert.Len(t, got, 1)
	})

	t.Run("query_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM account").WillReturnError(errTestBoom)

		_, err := r.ListByAlias(context.Background(), uuid.New(), uuid.New(), uuid.New(), []string{"x"})
		require.Error(t, err)
	})
}

func TestAccountRepository_Update(t *testing.T) {
	t.Parallel()

	blocked := true
	seg := "seg-1"
	ent := "ent-1"
	prt := "port-1"
	alias := "alias-1"

	t.Run("success_all_fields", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("UPDATE account").
			WillReturnResult(sqlmock.NewResult(0, 1))

		pid := uuid.New()
		got, err := r.Update(context.Background(), uuid.New(), uuid.New(), &pid, uuid.New(), &mmodel.Account{
			Name:      "x",
			Status:    mmodel.Status{Code: "ACTIVE"},
			Alias:     &alias,
			Blocked:   &blocked,
			SegmentID: &seg,
			EntityID:  &ent,
			// Exercise both portfolioID param and PortfolioID field.
			PortfolioID: &prt,
		})
		require.NoError(t, err)
		require.NotNil(t, got)
	})

	t.Run("no_rows_affected_is_not_found", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("UPDATE account").WillReturnResult(sqlmock.NewResult(0, 0))

		_, err := r.Update(context.Background(), uuid.New(), uuid.New(), nil, uuid.New(), &mmodel.Account{Name: "x"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no rows affected")
	})

	t.Run("exec_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("UPDATE account").WillReturnError(errTestBoom)

		_, err := r.Update(context.Background(), uuid.New(), uuid.New(), nil, uuid.New(), &mmodel.Account{Name: "x"})
		require.Error(t, err)
	})

	t.Run("pg_error_mapped", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("UPDATE account").WillReturnError(&pgconn.PgError{ConstraintName: "account_segment_id_fkey"})

		_, err := r.Update(context.Background(), uuid.New(), uuid.New(), nil, uuid.New(), &mmodel.Account{Name: "x"})
		require.Error(t, err)
		assert.Contains(t, strings.ToLower(err.Error()), "validate pg error")
	})
}

func TestAccountRepository_Delete(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("UPDATE account").WillReturnResult(sqlmock.NewResult(0, 1))

		pid := uuid.New()
		err := r.Delete(context.Background(), uuid.New(), uuid.New(), &pid, uuid.New())
		require.NoError(t, err)
	})

	t.Run("exec_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("UPDATE account").WillReturnError(errTestBoom)

		err := r.Delete(context.Background(), uuid.New(), uuid.New(), nil, uuid.New())
		require.Error(t, err)
	})
}

func TestAccountRepository_ListAccountsByIDs(t *testing.T) {
	t.Parallel()

	t.Run("returns_rows", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		rows := accountRow().AddRow(
			uuid.NewString(), "a", nil, nil, "USD",
			uuid.NewString(), uuid.NewString(), nil, nil,
			"ACTIVE", nil, nil, "deposit",
			anyTime(), anyTime(), sql.NullTime{}, false,
		)
		mock.ExpectQuery("SELECT .* FROM account").WillReturnRows(rows)

		got, err := r.ListAccountsByIDs(context.Background(), uuid.New(), uuid.New(), []uuid.UUID{uuid.New()})
		require.NoError(t, err)
		assert.Len(t, got, 1)
	})

	t.Run("query_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM account").WillReturnError(errTestBoom)

		_, err := r.ListAccountsByIDs(context.Background(), uuid.New(), uuid.New(), []uuid.UUID{uuid.New()})
		require.Error(t, err)
	})
}

func TestAccountRepository_ListAccountsByAlias(t *testing.T) {
	t.Parallel()

	t.Run("returns_rows", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		rows := accountRow().AddRow(
			uuid.NewString(), "a", nil, nil, "USD",
			uuid.NewString(), uuid.NewString(), nil, nil,
			"ACTIVE", nil, nil, "deposit",
			anyTime(), anyTime(), sql.NullTime{}, false,
		)
		mock.ExpectQuery("SELECT .* FROM account").WillReturnRows(rows)

		got, err := r.ListAccountsByAlias(context.Background(), uuid.New(), uuid.New(), []string{"a"})
		require.NoError(t, err)
		assert.Len(t, got, 1)
	})

	t.Run("query_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM account").WillReturnError(errTestBoom)

		_, err := r.ListAccountsByAlias(context.Background(), uuid.New(), uuid.New(), []string{"a"})
		require.Error(t, err)
	})
}

func TestAccountRepository_Count(t *testing.T) {
	t.Parallel()

	t.Run("returns_count", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM account`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(42)))

		got, err := r.Count(context.Background(), uuid.New(), uuid.New())
		require.NoError(t, err)
		assert.Equal(t, int64(42), got)
	})

	t.Run("query_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM account`).WillReturnError(errTestBoom)

		got, err := r.Count(context.Background(), uuid.New(), uuid.New())
		require.Error(t, err)
		assert.Equal(t, int64(0), got)
	})
}

// anyTime returns a deterministic, non-zero time.Time placeholder for Scan rows.
func anyTime() time.Time {
	t, _ := time.Parse(time.RFC3339, "2024-01-01T00:00:00Z")
	return t
}
