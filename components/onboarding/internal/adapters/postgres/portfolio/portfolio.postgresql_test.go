// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package portfolio

import (
	"context"
	"database/sql"
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

func newRepoWithMock(t *testing.T) (*PortfolioPostgreSQLRepository, sqlmock.Sqlmock) {
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

	return &PortfolioPostgreSQLRepository{
		connection: conn,
		tableName:  "portfolio",
	}, mock
}

func portfolioRow() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "name", "entity_id", "ledger_id", "organization_id",
		"status", "status_description",
		"created_at", "updated_at", "deleted_at",
	})
}

func anyTime() time.Time {
	t, _ := time.Parse(time.RFC3339, "2024-01-01T00:00:00Z")
	return t
}

func TestPortfolioRepository_Create(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("INSERT INTO portfolio").WillReturnResult(sqlmock.NewResult(1, 1))

		got, err := r.Create(context.Background(), &mmodel.Portfolio{
			ID:             uuid.NewString(),
			Name:           "default",
			EntityID:       uuid.NewString(),
			LedgerID:       uuid.NewString(),
			OrganizationID: uuid.NewString(),
			Status:         mmodel.Status{Code: "ACTIVE"},
		})
		require.NoError(t, err)
		require.NotNil(t, got)
	})

	t.Run("no_rows", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("INSERT INTO portfolio").WillReturnResult(sqlmock.NewResult(0, 0))

		_, err := r.Create(context.Background(), &mmodel.Portfolio{})
		require.Error(t, err)
	})

	t.Run("exec_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("INSERT INTO portfolio").WillReturnError(errTestBoom)

		_, err := r.Create(context.Background(), &mmodel.Portfolio{})
		require.Error(t, err)
	})

	t.Run("pg_error_mapped", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("INSERT INTO portfolio").
			WillReturnError(&pgconn.PgError{ConstraintName: "portfolio_ledger_id_fkey"})

		_, err := r.Create(context.Background(), &mmodel.Portfolio{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "validate pg error")
	})
}

func TestPortfolioRepository_FindByIDEntity(t *testing.T) {
	t.Parallel()

	t.Run("found", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM portfolio").WillReturnRows(
			portfolioRow().AddRow(uuid.NewString(), "default", uuid.NewString(),
				uuid.NewString(), uuid.NewString(), "ACTIVE", nil,
				anyTime(), anyTime(), sql.NullTime{}),
		)

		_, err := r.FindByIDEntity(context.Background(), uuid.New(), uuid.New(), uuid.New())
		require.NoError(t, err)
	})

	t.Run("not_found", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM portfolio").WillReturnError(sql.ErrNoRows)

		_, err := r.FindByIDEntity(context.Background(), uuid.New(), uuid.New(), uuid.New())
		require.Error(t, err)
	})

	t.Run("other_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM portfolio").WillReturnError(errTestBoom)

		_, err := r.FindByIDEntity(context.Background(), uuid.New(), uuid.New(), uuid.New())
		require.Error(t, err)
	})
}

func TestPortfolioRepository_FindAll(t *testing.T) {
	t.Parallel()

	t.Run("returns_rows", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM portfolio").WillReturnRows(
			portfolioRow().
				AddRow(uuid.NewString(), "a", uuid.NewString(), uuid.NewString(), uuid.NewString(), "ACTIVE", nil,
					anyTime(), anyTime(), sql.NullTime{}).
				AddRow(uuid.NewString(), "b", uuid.NewString(), uuid.NewString(), uuid.NewString(), "ACTIVE", nil,
					anyTime(), anyTime(), sql.NullTime{}),
		)

		got, err := r.FindAll(context.Background(), uuid.New(), uuid.New(), http.Pagination{Limit: 10, Page: 1, SortOrder: "asc"})
		require.NoError(t, err)
		assert.Len(t, got, 2)
	})

	t.Run("query_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM portfolio").WillReturnError(errTestBoom)

		_, err := r.FindAll(context.Background(), uuid.New(), uuid.New(), http.Pagination{Limit: 10, Page: 1, SortOrder: "desc"})
		require.Error(t, err)
	})
}

func TestPortfolioRepository_Find(t *testing.T) {
	t.Parallel()

	t.Run("found", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM portfolio").WillReturnRows(
			portfolioRow().AddRow(uuid.NewString(), "default", uuid.NewString(),
				uuid.NewString(), uuid.NewString(), "ACTIVE", nil,
				anyTime(), anyTime(), sql.NullTime{}),
		)

		_, err := r.Find(context.Background(), uuid.New(), uuid.New(), uuid.New())
		require.NoError(t, err)
	})

	t.Run("not_found", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM portfolio").WillReturnError(sql.ErrNoRows)

		_, err := r.Find(context.Background(), uuid.New(), uuid.New(), uuid.New())
		require.Error(t, err)
	})

	t.Run("generic_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM portfolio").WillReturnError(errTestBoom)

		_, err := r.Find(context.Background(), uuid.New(), uuid.New(), uuid.New())
		require.Error(t, err)
	})
}

func TestPortfolioRepository_ListByIDs(t *testing.T) {
	t.Parallel()

	t.Run("returns_rows", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM portfolio").WillReturnRows(
			portfolioRow().AddRow(uuid.NewString(), "a", uuid.NewString(), uuid.NewString(), uuid.NewString(), "ACTIVE", nil,
				anyTime(), anyTime(), sql.NullTime{}),
		)

		got, err := r.ListByIDs(context.Background(), uuid.New(), uuid.New(), []uuid.UUID{uuid.New()})
		require.NoError(t, err)
		assert.Len(t, got, 1)
	})

	t.Run("query_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM portfolio").WillReturnError(errTestBoom)

		_, err := r.ListByIDs(context.Background(), uuid.New(), uuid.New(), []uuid.UUID{uuid.New()})
		require.Error(t, err)
	})
}

func TestPortfolioRepository_Update(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("UPDATE portfolio SET").WillReturnResult(sqlmock.NewResult(0, 1))

		_, err := r.Update(context.Background(), uuid.New(), uuid.New(), uuid.New(), &mmodel.Portfolio{
			Name:   "new",
			Status: mmodel.Status{Code: "ACTIVE"},
		})
		require.NoError(t, err)
	})

	t.Run("no_rows_affected", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("UPDATE portfolio SET").WillReturnResult(sqlmock.NewResult(0, 0))

		_, err := r.Update(context.Background(), uuid.New(), uuid.New(), uuid.New(), &mmodel.Portfolio{Name: "n"})
		require.Error(t, err)
	})

	t.Run("exec_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("UPDATE portfolio SET").WillReturnError(errTestBoom)

		_, err := r.Update(context.Background(), uuid.New(), uuid.New(), uuid.New(), &mmodel.Portfolio{Name: "n"})
		require.Error(t, err)
	})

	t.Run("pg_error_mapped", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("UPDATE portfolio SET").
			WillReturnError(&pgconn.PgError{ConstraintName: "portfolio_ledger_id_fkey"})

		_, err := r.Update(context.Background(), uuid.New(), uuid.New(), uuid.New(), &mmodel.Portfolio{Name: "n"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "validate pg error")
	})
}

func TestPortfolioRepository_Delete(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("UPDATE portfolio SET deleted_at").WillReturnResult(sqlmock.NewResult(0, 1))

		require.NoError(t, r.Delete(context.Background(), uuid.New(), uuid.New(), uuid.New()))
	})

	t.Run("no_rows_affected", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("UPDATE portfolio SET deleted_at").WillReturnResult(sqlmock.NewResult(0, 0))

		require.Error(t, r.Delete(context.Background(), uuid.New(), uuid.New(), uuid.New()))
	})

	t.Run("exec_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("UPDATE portfolio SET deleted_at").WillReturnError(errTestBoom)

		require.Error(t, r.Delete(context.Background(), uuid.New(), uuid.New(), uuid.New()))
	})
}

func TestPortfolioRepository_Count(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM portfolio`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(5)))

		got, err := r.Count(context.Background(), uuid.New(), uuid.New())
		require.NoError(t, err)
		assert.Equal(t, int64(5), got)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM portfolio`).WillReturnError(errTestBoom)

		_, err := r.Count(context.Background(), uuid.New(), uuid.New())
		require.Error(t, err)
	})
}
