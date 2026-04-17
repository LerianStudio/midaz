// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package organization

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

func newRepoWithMock(t *testing.T) (*OrganizationPostgreSQLRepository, sqlmock.Sqlmock) {
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

	return &OrganizationPostgreSQLRepository{
		connection: conn,
		tableName:  "organization",
	}, mock
}

func orgRow() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "parent_organization_id", "legal_name", "doing_business_as", "legal_document",
		"address", "status", "status_description",
		"created_at", "updated_at", "deleted_at",
	})
}

func anyTime() time.Time {
	t, _ := time.Parse(time.RFC3339, "2024-01-01T00:00:00Z")
	return t
}

func emptyAddressJSON() []byte {
	return []byte(`{"line1":"","line2":null,"zipCode":"","city":"","state":"","country":""}`)
}

func validOrg() *mmodel.Organization {
	return &mmodel.Organization{
		ID:            uuid.NewString(),
		LegalName:     "Acme Corp",
		LegalDocument: "123",
		Status:        mmodel.Status{Code: "ACTIVE"},
		Address: mmodel.Address{
			Line1:   "Main St",
			ZipCode: "00000",
			City:    "Springfield",
			State:   "SP",
			Country: "BR",
		},
	}
}

func TestOrgRepository_Create(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("INSERT INTO organization").WillReturnResult(sqlmock.NewResult(1, 1))

		got, err := r.Create(context.Background(), validOrg())
		require.NoError(t, err)
		require.NotNil(t, got)
	})

	t.Run("no_rows_affected", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("INSERT INTO organization").WillReturnResult(sqlmock.NewResult(0, 0))

		_, err := r.Create(context.Background(), validOrg())
		require.Error(t, err)
	})

	t.Run("exec_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("INSERT INTO organization").WillReturnError(errTestBoom)

		_, err := r.Create(context.Background(), validOrg())
		require.Error(t, err)
	})

	t.Run("pg_error_mapped", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("INSERT INTO organization").
			WillReturnError(&pgconn.PgError{ConstraintName: "organization_parent_organization_id_fkey"})

		_, err := r.Create(context.Background(), validOrg())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "validate pg error")
	})
}

func TestOrgRepository_Update(t *testing.T) {
	t.Parallel()

	parent := uuid.NewString()
	dba := "Acme"

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("UPDATE organization SET").WillReturnResult(sqlmock.NewResult(0, 1))

		_, err := r.Update(context.Background(), uuid.New(), &mmodel.Organization{
			ParentOrganizationID: &parent,
			LegalName:            "Acme",
			DoingBusinessAs:      &dba,
			Address: mmodel.Address{
				Line1: "x", ZipCode: "x", City: "x", State: "x", Country: "x",
			},
			Status: mmodel.Status{Code: "ACTIVE"},
		})
		require.NoError(t, err)
	})

	t.Run("no_rows_affected", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("UPDATE organization SET").WillReturnResult(sqlmock.NewResult(0, 0))

		_, err := r.Update(context.Background(), uuid.New(), &mmodel.Organization{LegalName: "x"})
		require.Error(t, err)
	})

	t.Run("exec_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("UPDATE organization SET").WillReturnError(errTestBoom)

		_, err := r.Update(context.Background(), uuid.New(), &mmodel.Organization{LegalName: "x"})
		require.Error(t, err)
	})

	t.Run("pg_error_mapped", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("UPDATE organization SET").
			WillReturnError(&pgconn.PgError{ConstraintName: "organization_parent_organization_id_fkey"})

		_, err := r.Update(context.Background(), uuid.New(), &mmodel.Organization{LegalName: "x"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "validate pg error")
	})
}

func TestOrgRepository_Find(t *testing.T) {
	t.Parallel()

	t.Run("found", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM organization").WillReturnRows(
			orgRow().AddRow(uuid.NewString(), nil, "Acme", nil, "123",
				emptyAddressJSON(), "ACTIVE", nil,
				anyTime(), anyTime(), sql.NullTime{}),
		)

		_, err := r.Find(context.Background(), uuid.New())
		require.NoError(t, err)
	})

	t.Run("not_found", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM organization").WillReturnError(sql.ErrNoRows)

		_, err := r.Find(context.Background(), uuid.New())
		require.Error(t, err)
	})

	t.Run("scan_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM organization").WillReturnError(errTestBoom)

		_, err := r.Find(context.Background(), uuid.New())
		require.Error(t, err)
	})
}

func TestOrgRepository_FindAll(t *testing.T) {
	t.Parallel()

	t.Run("returns_rows", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM organization").WillReturnRows(
			orgRow().
				AddRow(uuid.NewString(), nil, "a", nil, "1", emptyAddressJSON(), "ACTIVE", nil,
					anyTime(), anyTime(), sql.NullTime{}).
				AddRow(uuid.NewString(), nil, "b", nil, "2", emptyAddressJSON(), "ACTIVE", nil,
					anyTime(), anyTime(), sql.NullTime{}),
		)

		got, err := r.FindAll(context.Background(), http.Pagination{Limit: 10, Page: 1, SortOrder: "asc"})
		require.NoError(t, err)
		assert.Len(t, got, 2)
	})

	t.Run("query_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM organization").WillReturnError(errTestBoom)

		_, err := r.FindAll(context.Background(), http.Pagination{Limit: 10, Page: 1, SortOrder: "desc"})
		require.Error(t, err)
	})
}

func TestOrgRepository_ListByIDs(t *testing.T) {
	t.Parallel()

	t.Run("returns_rows", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM organization").WillReturnRows(
			orgRow().AddRow(uuid.NewString(), nil, "a", nil, "1", emptyAddressJSON(), "ACTIVE", nil,
				anyTime(), anyTime(), sql.NullTime{}),
		)

		got, err := r.ListByIDs(context.Background(), []uuid.UUID{uuid.New()})
		require.NoError(t, err)
		assert.Len(t, got, 1)
	})

	t.Run("query_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM organization").WillReturnError(errTestBoom)

		_, err := r.ListByIDs(context.Background(), []uuid.UUID{uuid.New()})
		require.Error(t, err)
	})
}

func TestOrgRepository_Delete(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("UPDATE organization SET deleted_at").
			WillReturnResult(sqlmock.NewResult(0, 1))

		require.NoError(t, r.Delete(context.Background(), uuid.New()))
	})

	t.Run("no_rows_affected", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("UPDATE organization SET deleted_at").
			WillReturnResult(sqlmock.NewResult(0, 0))

		require.Error(t, r.Delete(context.Background(), uuid.New()))
	})

	t.Run("exec_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectExec("UPDATE organization SET deleted_at").WillReturnError(errTestBoom)

		require.Error(t, r.Delete(context.Background(), uuid.New()))
	})
}

func TestOrgRepository_Count(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM organization`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(3)))

		got, err := r.Count(context.Background())
		require.NoError(t, err)
		assert.Equal(t, int64(3), got)
	})

	t.Run("query_error", func(t *testing.T) {
		t.Parallel()

		r, mock := newRepoWithMock(t)

		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM organization`).WillReturnError(errTestBoom)

		_, err := r.Count(context.Background())
		require.Error(t, err)
	})
}
