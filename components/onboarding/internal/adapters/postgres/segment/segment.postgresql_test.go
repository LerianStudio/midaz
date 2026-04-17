// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package segment

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

func newRepoWithMock(t *testing.T) (*SegmentPostgreSQLRepository, sqlmock.Sqlmock) {
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

	return &SegmentPostgreSQLRepository{
		connection: conn,
		tableName:  "segment",
	}, mock
}

func segmentRow() *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "name", "ledger_id", "organization_id",
		"status", "status_description",
		"created_at", "updated_at", "deleted_at",
	})
}

func anyTime() time.Time {
	t, _ := time.Parse(time.RFC3339, "2024-01-01T00:00:00Z")
	return t
}

func TestSegmentRepository_Create(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		p, mock := newRepoWithMock(t)

		mock.ExpectExec("INSERT INTO segment").WillReturnResult(sqlmock.NewResult(1, 1))

		_, err := p.Create(context.Background(), &mmodel.Segment{
			ID:             uuid.NewString(),
			Name:           "seg",
			LedgerID:       uuid.NewString(),
			OrganizationID: uuid.NewString(),
			Status:         mmodel.Status{Code: "ACTIVE"},
		})
		require.NoError(t, err)
	})

	t.Run("no_rows", func(t *testing.T) {
		t.Parallel()

		p, mock := newRepoWithMock(t)

		mock.ExpectExec("INSERT INTO segment").WillReturnResult(sqlmock.NewResult(0, 0))

		_, err := p.Create(context.Background(), &mmodel.Segment{})
		require.Error(t, err)
	})

	t.Run("exec_error", func(t *testing.T) {
		t.Parallel()

		p, mock := newRepoWithMock(t)

		mock.ExpectExec("INSERT INTO segment").WillReturnError(errTestBoom)

		_, err := p.Create(context.Background(), &mmodel.Segment{})
		require.Error(t, err)
	})

	t.Run("pg_error_mapped", func(t *testing.T) {
		t.Parallel()

		p, mock := newRepoWithMock(t)

		mock.ExpectExec("INSERT INTO segment").
			WillReturnError(&pgconn.PgError{ConstraintName: "segment_ledger_id_fkey"})

		_, err := p.Create(context.Background(), &mmodel.Segment{})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "validate pg error")
	})
}

func TestSegmentRepository_FindByName(t *testing.T) {
	t.Parallel()

	t.Run("duplicate", func(t *testing.T) {
		t.Parallel()

		p, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM segment").WillReturnRows(
			segmentRow().AddRow(uuid.NewString(), "seg", uuid.NewString(), uuid.NewString(),
				"ACTIVE", nil, anyTime(), anyTime(), sql.NullTime{}),
		)

		taken, err := p.FindByName(context.Background(), uuid.New(), uuid.New(), "seg")
		require.Error(t, err)
		assert.True(t, taken)
	})

	t.Run("available", func(t *testing.T) {
		t.Parallel()

		p, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM segment").WillReturnRows(segmentRow())

		taken, err := p.FindByName(context.Background(), uuid.New(), uuid.New(), "seg")
		require.NoError(t, err)
		assert.False(t, taken)
	})

	t.Run("query_error", func(t *testing.T) {
		t.Parallel()

		p, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM segment").WillReturnError(errTestBoom)

		_, err := p.FindByName(context.Background(), uuid.New(), uuid.New(), "seg")
		require.Error(t, err)
	})
}

func TestSegmentRepository_FindAll(t *testing.T) {
	t.Parallel()

	t.Run("returns_rows", func(t *testing.T) {
		t.Parallel()

		p, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM segment").WillReturnRows(
			segmentRow().
				AddRow(uuid.NewString(), "a", uuid.NewString(), uuid.NewString(), "ACTIVE", nil,
					anyTime(), anyTime(), sql.NullTime{}).
				AddRow(uuid.NewString(), "b", uuid.NewString(), uuid.NewString(), "ACTIVE", nil,
					anyTime(), anyTime(), sql.NullTime{}),
		)

		got, err := p.FindAll(context.Background(), uuid.New(), uuid.New(),
			http.Pagination{Limit: 10, Page: 1, SortOrder: "asc"})
		require.NoError(t, err)
		assert.Len(t, got, 2)
	})

	t.Run("query_error", func(t *testing.T) {
		t.Parallel()

		p, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM segment").WillReturnError(errTestBoom)

		_, err := p.FindAll(context.Background(), uuid.New(), uuid.New(),
			http.Pagination{Limit: 10, Page: 1, SortOrder: "desc"})
		require.Error(t, err)
	})
}

func TestSegmentRepository_FindByIDs(t *testing.T) {
	t.Parallel()

	t.Run("returns_rows", func(t *testing.T) {
		t.Parallel()

		p, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM segment").WillReturnRows(
			segmentRow().AddRow(uuid.NewString(), "a", uuid.NewString(), uuid.NewString(), "ACTIVE", nil,
				anyTime(), anyTime(), sql.NullTime{}),
		)

		got, err := p.FindByIDs(context.Background(), uuid.New(), uuid.New(), []uuid.UUID{uuid.New()})
		require.NoError(t, err)
		assert.Len(t, got, 1)
	})

	t.Run("query_error", func(t *testing.T) {
		t.Parallel()

		p, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM segment").WillReturnError(errTestBoom)

		_, err := p.FindByIDs(context.Background(), uuid.New(), uuid.New(), []uuid.UUID{uuid.New()})
		require.Error(t, err)
	})
}

func TestSegmentRepository_Find(t *testing.T) {
	t.Parallel()

	t.Run("found", func(t *testing.T) {
		t.Parallel()

		p, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM segment").WillReturnRows(
			segmentRow().AddRow(uuid.NewString(), "s", uuid.NewString(), uuid.NewString(), "ACTIVE", nil,
				anyTime(), anyTime(), sql.NullTime{}),
		)

		_, err := p.Find(context.Background(), uuid.New(), uuid.New(), uuid.New())
		require.NoError(t, err)
	})

	t.Run("not_found", func(t *testing.T) {
		t.Parallel()

		p, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM segment").WillReturnError(sql.ErrNoRows)

		_, err := p.Find(context.Background(), uuid.New(), uuid.New(), uuid.New())
		require.Error(t, err)
	})

	t.Run("other_error", func(t *testing.T) {
		t.Parallel()

		p, mock := newRepoWithMock(t)

		mock.ExpectQuery("SELECT .* FROM segment").WillReturnError(errTestBoom)

		_, err := p.Find(context.Background(), uuid.New(), uuid.New(), uuid.New())
		require.Error(t, err)
	})
}

func TestSegmentRepository_Update(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		p, mock := newRepoWithMock(t)

		mock.ExpectExec("UPDATE segment SET").WillReturnResult(sqlmock.NewResult(0, 1))

		_, err := p.Update(context.Background(), uuid.New(), uuid.New(), uuid.New(), &mmodel.Segment{
			Name:   "new",
			Status: mmodel.Status{Code: "ACTIVE"},
		})
		require.NoError(t, err)
	})

	t.Run("no_rows_affected", func(t *testing.T) {
		t.Parallel()

		p, mock := newRepoWithMock(t)

		mock.ExpectExec("UPDATE segment SET").WillReturnResult(sqlmock.NewResult(0, 0))

		_, err := p.Update(context.Background(), uuid.New(), uuid.New(), uuid.New(), &mmodel.Segment{Name: "n"})
		require.Error(t, err)
	})

	t.Run("exec_error", func(t *testing.T) {
		t.Parallel()

		p, mock := newRepoWithMock(t)

		mock.ExpectExec("UPDATE segment SET").WillReturnError(errTestBoom)

		_, err := p.Update(context.Background(), uuid.New(), uuid.New(), uuid.New(), &mmodel.Segment{Name: "n"})
		require.Error(t, err)
	})

	t.Run("pg_error_mapped", func(t *testing.T) {
		t.Parallel()

		p, mock := newRepoWithMock(t)

		mock.ExpectExec("UPDATE segment SET").
			WillReturnError(&pgconn.PgError{ConstraintName: "segment_ledger_id_fkey"})

		_, err := p.Update(context.Background(), uuid.New(), uuid.New(), uuid.New(), &mmodel.Segment{Name: "n"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "validate pg error")
	})
}

func TestSegmentRepository_Delete(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		p, mock := newRepoWithMock(t)

		mock.ExpectExec("UPDATE segment SET deleted_at").WillReturnResult(sqlmock.NewResult(0, 1))

		require.NoError(t, p.Delete(context.Background(), uuid.New(), uuid.New(), uuid.New()))
	})

	t.Run("no_rows_affected", func(t *testing.T) {
		t.Parallel()

		p, mock := newRepoWithMock(t)

		mock.ExpectExec("UPDATE segment SET deleted_at").WillReturnResult(sqlmock.NewResult(0, 0))

		require.Error(t, p.Delete(context.Background(), uuid.New(), uuid.New(), uuid.New()))
	})

	t.Run("exec_error", func(t *testing.T) {
		t.Parallel()

		p, mock := newRepoWithMock(t)

		mock.ExpectExec("UPDATE segment SET deleted_at").WillReturnError(errTestBoom)

		require.Error(t, p.Delete(context.Background(), uuid.New(), uuid.New(), uuid.New()))
	})
}

func TestSegmentRepository_Count(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		p, mock := newRepoWithMock(t)

		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM segment`).
			WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(4)))

		got, err := p.Count(context.Background(), uuid.New(), uuid.New())
		require.NoError(t, err)
		assert.Equal(t, int64(4), got)
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		p, mock := newRepoWithMock(t)

		mock.ExpectQuery(`SELECT COUNT\(\*\) FROM segment`).WillReturnError(errTestBoom)

		_, err := p.Count(context.Background(), uuid.New(), uuid.New())
		require.Error(t, err)
	})
}
