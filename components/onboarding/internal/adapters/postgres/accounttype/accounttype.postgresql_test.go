// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package accounttype

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

var errTestBoom = errors.New("boom")

func newRepoWithMock(t *testing.T) (*AccountTypePostgreSQLRepository, sqlmock.Sqlmock) {
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

	return &AccountTypePostgreSQLRepository{
		connection: conn,
		tableName:  "account_type",
	}, mock
}

func accountTypeRow() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "organization_id", "ledger_id", "name", "description", "key_value",
		"created_at", "updated_at", "deleted_at",
	})
}

func anyTime() time.Time {
	t, _ := time.Parse(time.RFC3339, "2024-01-01T00:00:00Z")
	return t
}

func TestAccountTypeRepository_Create(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("INSERT INTO account_type").WillReturnResult(sqlmock.NewResult(1, 1))

		_, err := r.Create(context.Background(), uuid.New(), uuid.New(), &mmodel.AccountType{
			Name: "deposit", KeyValue: "deposit",
		})
		require.NoError(t, err)
	})

	t.Run("no_rows", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("INSERT INTO account_type").WillReturnResult(sqlmock.NewResult(0, 0))

		_, err := r.Create(context.Background(), uuid.New(), uuid.New(), &mmodel.AccountType{})
		require.Error(t, err)
	})

	t.Run("exec_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("INSERT INTO account_type").WillReturnError(errTestBoom)

		_, err := r.Create(context.Background(), uuid.New(), uuid.New(), &mmodel.AccountType{})
		require.Error(t, err)
	})

	t.Run("pg_error_mapped", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("INSERT INTO account_type").
			WillReturnError(&pgconn.PgError{ConstraintName: "account_type_ledger_id_fkey"})

		_, err := r.Create(context.Background(), uuid.New(), uuid.New(), &mmodel.AccountType{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "validate pg error")
	})
}

func TestAccountTypeRepository_FindByID(t *testing.T) {
	t.Parallel()

	t.Run("found", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT.*FROM account_type").WillReturnRows(
			accountTypeRow().AddRow(uuid.NewString(), uuid.NewString(), uuid.NewString(),
				"deposit", "desc", "deposit",
				anyTime(), anyTime(), sql.NullTime{}),
		)

		_, err := r.FindByID(context.Background(), uuid.New(), uuid.New(), uuid.New())
		require.NoError(t, err)
	})

	t.Run("not_found", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT.*FROM account_type").WillReturnError(sql.ErrNoRows)

		_, err := r.FindByID(context.Background(), uuid.New(), uuid.New(), uuid.New())
		require.Error(t, err)
	})

	t.Run("other_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT.*FROM account_type").WillReturnError(errTestBoom)

		_, err := r.FindByID(context.Background(), uuid.New(), uuid.New(), uuid.New())
		require.Error(t, err)
	})
}

func TestAccountTypeRepository_FindByKey(t *testing.T) {
	t.Parallel()

	t.Run("found", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT.*FROM account_type").WillReturnRows(
			accountTypeRow().AddRow(uuid.NewString(), uuid.NewString(), uuid.NewString(),
				"deposit", "desc", "deposit",
				anyTime(), anyTime(), sql.NullTime{}),
		)

		_, err := r.FindByKey(context.Background(), uuid.New(), uuid.New(), "deposit")
		require.NoError(t, err)
	})

	t.Run("not_found", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT.*FROM account_type").WillReturnError(sql.ErrNoRows)

		_, err := r.FindByKey(context.Background(), uuid.New(), uuid.New(), "unknown")
		require.Error(t, err)
	})

	t.Run("other_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT.*FROM account_type").WillReturnError(errTestBoom)

		_, err := r.FindByKey(context.Background(), uuid.New(), uuid.New(), "k")
		require.Error(t, err)
	})
}

func TestAccountTypeRepository_Update(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("UPDATE account_type SET").WillReturnResult(sqlmock.NewResult(0, 1))

		_, err := r.Update(context.Background(), uuid.New(), uuid.New(), uuid.New(), &mmodel.AccountType{
			Name: "new", Description: "d",
		})
		require.NoError(t, err)
	})

	t.Run("no_rows_affected", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("UPDATE account_type SET").WillReturnResult(sqlmock.NewResult(0, 0))

		_, err := r.Update(context.Background(), uuid.New(), uuid.New(), uuid.New(), &mmodel.AccountType{Name: "n"})
		require.Error(t, err)
	})

	t.Run("exec_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("UPDATE account_type SET").WillReturnError(errTestBoom)

		_, err := r.Update(context.Background(), uuid.New(), uuid.New(), uuid.New(), &mmodel.AccountType{Name: "n"})
		require.Error(t, err)
	})

	t.Run("pg_error_mapped", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("UPDATE account_type SET").
			WillReturnError(&pgconn.PgError{ConstraintName: "account_type_ledger_id_fkey"})

		_, err := r.Update(context.Background(), uuid.New(), uuid.New(), uuid.New(), &mmodel.AccountType{Name: "n"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "validate pg error")
	})
}

func TestAccountTypeRepository_FindAll(t *testing.T) {
	t.Parallel()

	t.Run("returns_rows", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM account_type").WillReturnRows(
			accountTypeRow().
				AddRow(uuid.NewString(), uuid.NewString(), uuid.NewString(), "a", "d", "k1",
					anyTime(), anyTime(), sql.NullTime{}).
				AddRow(uuid.NewString(), uuid.NewString(), uuid.NewString(), "b", "d", "k2",
					anyTime(), anyTime(), sql.NullTime{}),
		)

		got, _, err := r.FindAll(context.Background(), uuid.New(), uuid.New(),
			http.Pagination{Limit: 10, SortOrder: "asc"})
		require.NoError(t, err)
		assert.Len(t, got, 2)
	})

	t.Run("query_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM account_type").WillReturnError(errTestBoom)

		_, _, err := r.FindAll(context.Background(), uuid.New(), uuid.New(),
			http.Pagination{Limit: 10, SortOrder: "desc"})
		require.Error(t, err)
	})

	t.Run("invalid_cursor", func(t *testing.T) {
		t.Parallel()

		r, _ := newRepoWithMock(t)

		_, _, err := r.FindAll(context.Background(), uuid.New(), uuid.New(),
			http.Pagination{Limit: 10, SortOrder: "desc", Cursor: "not-a-base64-cursor"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "decode cursor")
	})
}

func TestAccountTypeRepository_ListByIDs(t *testing.T) {
	t.Parallel()

	t.Run("returns_rows", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT.*FROM account_type").WillReturnRows(
			accountTypeRow().AddRow(uuid.NewString(), uuid.NewString(), uuid.NewString(),
				"a", "d", "k", anyTime(), anyTime(), sql.NullTime{}),
		)

		got, err := r.ListByIDs(context.Background(), uuid.New(), uuid.New(), []uuid.UUID{uuid.New()})
		require.NoError(t, err)
		assert.Len(t, got, 1)
	})

	t.Run("query_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT.*FROM account_type").WillReturnError(errTestBoom)

		_, err := r.ListByIDs(context.Background(), uuid.New(), uuid.New(), []uuid.UUID{uuid.New()})
		require.Error(t, err)
	})
}

func TestAccountTypeRepository_Delete(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("UPDATE account_type SET deleted_at").WillReturnResult(sqlmock.NewResult(0, 1))

		require.NoError(t, r.Delete(context.Background(), uuid.New(), uuid.New(), uuid.New()))
	})

	t.Run("no_rows_affected", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("UPDATE account_type SET deleted_at").WillReturnResult(sqlmock.NewResult(0, 0))

		require.Error(t, r.Delete(context.Background(), uuid.New(), uuid.New(), uuid.New()))
	})

	t.Run("exec_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("UPDATE account_type SET deleted_at").WillReturnError(errTestBoom)

		require.Error(t, r.Delete(context.Background(), uuid.New(), uuid.New(), uuid.New()))
	})
}
