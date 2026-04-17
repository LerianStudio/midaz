// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package asset

import (
	"context"
	"database/sql"
	"errors"
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

// errTestBoom is a sentinel so tests don't litter the binary with errors.New calls.
var errTestBoom = errors.New("boom")

func newRepoWithMock(t *testing.T) (*AssetPostgreSQLRepository, sqlmock.Sqlmock) {
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

	return &AssetPostgreSQLRepository{
		connection: conn,
		tableName:  "asset",
	}, mock
}

func assetRow() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "name", "type", "code", "status", "status_description",
		"ledger_id", "organization_id",
		"created_at", "updated_at", "deleted_at",
	})
}

func anyTime() time.Time {
	t, _ := time.Parse(time.RFC3339, "2024-01-01T00:00:00Z")
	return t
}

func TestAssetRepository_Create(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("INSERT INTO asset").WillReturnResult(sqlmock.NewResult(1, 1))

		_, err := r.Create(context.Background(), &mmodel.Asset{
			ID: uuid.NewString(), Name: "USD", Code: "USD", Type: "currency",
			LedgerID: uuid.NewString(), OrganizationID: uuid.NewString(),
			Status: mmodel.Status{Code: "ACTIVE"},
		})
		require.NoError(t, err)
	})

	t.Run("no_rows", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("INSERT INTO asset").WillReturnResult(sqlmock.NewResult(0, 0))

		_, err := r.Create(context.Background(), &mmodel.Asset{})
		require.Error(t, err)
	})

	t.Run("exec_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("INSERT INTO asset").WillReturnError(errTestBoom)

		_, err := r.Create(context.Background(), &mmodel.Asset{})
		require.Error(t, err)
	})

	t.Run("pg_error_mapped", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("INSERT INTO asset").
			WillReturnError(&pgconn.PgError{ConstraintName: "asset_ledger_id_fkey"})

		_, err := r.Create(context.Background(), &mmodel.Asset{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "validate pg error")
	})
}

func TestAssetRepository_FindByNameOrCode(t *testing.T) {
	t.Parallel()

	t.Run("name_taken", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM asset").WillReturnRows(
			assetRow().AddRow(uuid.NewString(), "USD", "currency", "USD", "ACTIVE", nil,
				uuid.NewString(), uuid.NewString(), anyTime(), anyTime(), sql.NullTime{}),
		)

		taken, err := r.FindByNameOrCode(context.Background(), uuid.New(), uuid.New(), "USD", "USD")
		require.Error(t, err)
		assert.True(t, taken)
	})

	t.Run("available", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM asset").WillReturnRows(assetRow())

		taken, err := r.FindByNameOrCode(context.Background(), uuid.New(), uuid.New(), "NEW", "NEW")
		require.NoError(t, err)
		assert.False(t, taken)
	})

	t.Run("query_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM asset").WillReturnError(errTestBoom)

		_, err := r.FindByNameOrCode(context.Background(), uuid.New(), uuid.New(), "n", "c")
		require.Error(t, err)
	})
}

func TestAssetRepository_FindAll(t *testing.T) {
	t.Parallel()

	t.Run("returns_rows", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM asset").WillReturnRows(
			assetRow().
				AddRow(uuid.NewString(), "a", "currency", "USD", "ACTIVE", nil,
					uuid.NewString(), uuid.NewString(), anyTime(), anyTime(), sql.NullTime{}).
				AddRow(uuid.NewString(), "b", "crypto", "BTC", "ACTIVE", nil,
					uuid.NewString(), uuid.NewString(), anyTime(), anyTime(), sql.NullTime{}),
		)

		got, err := r.FindAll(context.Background(), uuid.New(), uuid.New(), http.Pagination{Limit: 10, Page: 1, SortOrder: "asc"})
		require.NoError(t, err)
		assert.Len(t, got, 2)
	})

	t.Run("query_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM asset").WillReturnError(errTestBoom)

		_, err := r.FindAll(context.Background(), uuid.New(), uuid.New(), http.Pagination{Limit: 10, Page: 1, SortOrder: "desc"})
		require.Error(t, err)
	})
}

func TestAssetRepository_ListByIDs(t *testing.T) {
	t.Parallel()

	t.Run("returns_rows", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM asset").WillReturnRows(
			assetRow().AddRow(uuid.NewString(), "a", "currency", "USD", "ACTIVE", nil,
				uuid.NewString(), uuid.NewString(), anyTime(), anyTime(), sql.NullTime{}),
		)

		got, err := r.ListByIDs(context.Background(), uuid.New(), uuid.New(), []uuid.UUID{uuid.New()})
		require.NoError(t, err)
		assert.Len(t, got, 1)
	})

	t.Run("query_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM asset").WillReturnError(errTestBoom)

		_, err := r.ListByIDs(context.Background(), uuid.New(), uuid.New(), []uuid.UUID{uuid.New()})
		require.Error(t, err)
	})
}

func TestAssetRepository_Find(t *testing.T) {
	t.Parallel()

	t.Run("found", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM asset").WillReturnRows(
			assetRow().AddRow(uuid.NewString(), "USD", "currency", "USD", "ACTIVE", nil,
				uuid.NewString(), uuid.NewString(), anyTime(), anyTime(), sql.NullTime{}),
		)

		_, err := r.Find(context.Background(), uuid.New(), uuid.New(), uuid.New())
		require.NoError(t, err)
	})

	t.Run("not_found", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM asset").WillReturnError(sql.ErrNoRows)

		_, err := r.Find(context.Background(), uuid.New(), uuid.New(), uuid.New())
		require.Error(t, err)
	})

	t.Run("other_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM asset").WillReturnError(errTestBoom)

		_, err := r.Find(context.Background(), uuid.New(), uuid.New(), uuid.New())
		require.Error(t, err)
	})
}

func TestAssetRepository_Update(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("UPDATE asset SET").WillReturnResult(sqlmock.NewResult(0, 1))

		_, err := r.Update(context.Background(), uuid.New(), uuid.New(), uuid.New(), &mmodel.Asset{
			Name: "new", Status: mmodel.Status{Code: "ACTIVE"},
		})
		require.NoError(t, err)
	})

	t.Run("no_rows_affected", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("UPDATE asset SET").WillReturnResult(sqlmock.NewResult(0, 0))

		_, err := r.Update(context.Background(), uuid.New(), uuid.New(), uuid.New(), &mmodel.Asset{Name: "n"})
		require.Error(t, err)
	})

	t.Run("exec_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("UPDATE asset SET").WillReturnError(errTestBoom)

		_, err := r.Update(context.Background(), uuid.New(), uuid.New(), uuid.New(), &mmodel.Asset{Name: "n"})
		require.Error(t, err)
	})

	t.Run("pg_error_mapped", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("UPDATE asset SET").
			WillReturnError(&pgconn.PgError{ConstraintName: "asset_ledger_id_fkey"})

		_, err := r.Update(context.Background(), uuid.New(), uuid.New(), uuid.New(), &mmodel.Asset{Name: "n"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "validate pg error")
	})
}

func TestAssetRepository_Delete(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("UPDATE asset SET deleted_at").WillReturnResult(sqlmock.NewResult(0, 1))

		require.NoError(t, r.Delete(context.Background(), uuid.New(), uuid.New(), uuid.New()))
	})

	t.Run("no_rows_affected", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("UPDATE asset SET deleted_at").WillReturnResult(sqlmock.NewResult(0, 0))

		require.Error(t, r.Delete(context.Background(), uuid.New(), uuid.New(), uuid.New()))
	})

	t.Run("exec_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("UPDATE asset SET deleted_at").WillReturnError(errTestBoom)

		require.Error(t, r.Delete(context.Background(), uuid.New(), uuid.New(), uuid.New()))
	})
}

func TestAssetRepository_Count(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM asset`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(9)))

		got, err := r.Count(context.Background(), uuid.New(), uuid.New())
		require.NoError(t, err)
		assert.Equal(t, int64(9), got)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM asset`).WillReturnError(errTestBoom)

		_, err := r.Count(context.Background(), uuid.New(), uuid.New())
		require.Error(t, err)
	})
}
