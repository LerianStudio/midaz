// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	pkgHTTP "github.com/LerianStudio/midaz/v4/components/tracer/pkg/net/http"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// setupAuditEventRepoTx returns a bare *AuditEventRepository (conn left nil
// because InsertWithTx uses the supplied db handle directly) plus a sqlmock
// *sql.DB that satisfies pgdb.DB.
func setupAuditEventRepoTx(t *testing.T) (*AuditEventRepository, *sql.DB, sqlmock.Sqlmock, func()) {
	t.Helper()

	db, sqlMock, err := sqlmock.New()
	require.NoError(t, err)

	repo := &AuditEventRepository{}

	cleanup := func() {
		require.NoError(t, sqlMock.ExpectationsWereMet())
		sqlMock.ExpectClose()
		require.NoError(t, db.Close())
	}

	return repo, db, sqlMock, cleanup
}

func TestAuditEventRepository_InsertWithTx(t *testing.T) {
	testutil.SetupTestTracing(t)

	t.Run("Success - inserts using supplied tx handle", func(t *testing.T) {
		repo, db, mock, cleanup := setupAuditEventRepoTx(t)
		defer cleanup()

		event := createTestAuditEvent(t)

		mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO audit_events`)).
			WithArgs(
				event.EventID,
				string(event.EventType),
				event.CreatedAt,
				string(event.Action),
				string(event.Result),
				event.ResourceID,
				string(event.ResourceType),
				string(event.Actor.ActorType),
				event.Actor.ID,
				event.Actor.Name,
				event.Actor.Role,
				event.Actor.IPAddress,
				sqlmock.AnyArg(),           // contextJSON
				sqlmock.AnyArg(),           // metadataJSON
				event.ResourceID,           // WHERE resource_id
				string(event.EventType),    // WHERE event_type
				string(event.ResourceType), // WHERE resource_type
			).
			WillReturnResult(sqlmock.NewResult(1, 1))

		err := repo.InsertWithTx(context.Background(), db, event)
		require.NoError(t, err)
	})

	t.Run("Success - dedup no-op (zero rows) on transaction event is still success", func(t *testing.T) {
		repo, db, mock, cleanup := setupAuditEventRepoTx(t)
		defer cleanup()

		event := createTestAuditEvent(t) // ResourceTypeTransaction

		// WHERE NOT EXISTS suppressed the insert: zero rows affected. The dedup
		// path logs but must NOT return an error.
		mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO audit_events`)).
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := repo.InsertWithTx(context.Background(), db, event)
		require.NoError(t, err, "deduplicated transaction audit event must be an idempotent success")
	})

	t.Run("Error - nil event rejected before touching db", func(t *testing.T) {
		repo, db, _, cleanup := setupAuditEventRepoTx(t)
		defer cleanup()

		err := repo.InsertWithTx(context.Background(), db, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "event cannot be nil")
	})

	t.Run("Error - nil db handle rejected with ErrNilConnection", func(t *testing.T) {
		repo := &AuditEventRepository{}
		event := createTestAuditEvent(t)

		err := repo.InsertWithTx(context.Background(), nil, event)
		require.Error(t, err)
		assert.ErrorIs(t, err, pgdb.ErrNilConnection)
	})

	t.Run("Error - exec failure is wrapped", func(t *testing.T) {
		repo, db, mock, cleanup := setupAuditEventRepoTx(t)
		defer cleanup()

		event := createTestAuditEvent(t)

		mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO audit_events`)).
			WillReturnError(errors.New("deadlock detected"))

		err := repo.InsertWithTx(context.Background(), db, event)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to insert audit event")
	})
}

// TestAuditEventRepository_List_CursorPagination drives applyCursorPagination
// through List: it pins the invalid-cursor branches (decode failure, sortBy
// mismatch, sortOrder mismatch, non-numeric ID) and the two valid tuple-cursor
// directions (DESC and ASC), asserting the emitted WHERE clause carries the
// keyset comparison operator for each direction.
func TestAuditEventRepository_List_CursorPagination(t *testing.T) {
	testutil.SetupTestTracing(t)

	// validCursor builds an encoded cursor matching created_at/<order> sorting.
	validCursor := func(t *testing.T, id, order string) string {
		t.Helper()

		enc, err := pkgHTTP.EncodeCursor(pkgHTTP.Cursor{
			ID:         id,
			SortValue:  "2024-01-15T10:00:00Z",
			SortBy:     "created_at",
			SortOrder:  order,
			PointsNext: true,
		})
		require.NoError(t, err)

		return enc
	}

	t.Run("Error - undecodable cursor maps to ErrInvalidCursor", func(t *testing.T) {
		repo, mock, cleanup := setupAuditEventRepositoryMockDB(t)
		defer cleanup()

		filters := &model.AuditEventFilters{
			Limit:     10,
			SortBy:    "created_at",
			SortOrder: "DESC",
			Cursor:    "!!!not-base64!!!",
		}

		result, err := repo.List(context.Background(), filters)
		require.Error(t, err)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, constant.ErrInvalidCursor)
		// The query must never be issued when the cursor is rejected.
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Error - cursor sortBy mismatch rejected", func(t *testing.T) {
		repo, mock, cleanup := setupAuditEventRepositoryMockDB(t)
		defer cleanup()

		enc, err := pkgHTTP.EncodeCursor(pkgHTTP.Cursor{
			ID:        "100",
			SortValue: "2024-01-15T10:00:00Z",
			SortBy:    "event_type", // mismatched with request below
			SortOrder: "DESC",
		})
		require.NoError(t, err)

		filters := &model.AuditEventFilters{
			Limit:     10,
			SortBy:    "created_at",
			SortOrder: "DESC",
			Cursor:    enc,
		}

		result, err := repo.List(context.Background(), filters)
		require.Error(t, err)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, constant.ErrInvalidCursor)
		assert.Contains(t, err.Error(), "sortBy mismatch")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Error - cursor sortOrder mismatch rejected", func(t *testing.T) {
		repo, mock, cleanup := setupAuditEventRepositoryMockDB(t)
		defer cleanup()

		enc, err := pkgHTTP.EncodeCursor(pkgHTTP.Cursor{
			ID:        "100",
			SortValue: "2024-01-15T10:00:00Z",
			SortBy:    "created_at",
			SortOrder: "ASC", // mismatched with request below
		})
		require.NoError(t, err)

		filters := &model.AuditEventFilters{
			Limit:     10,
			SortBy:    "created_at",
			SortOrder: "DESC",
			Cursor:    enc,
		}

		result, err := repo.List(context.Background(), filters)
		require.Error(t, err)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, constant.ErrInvalidCursor)
		assert.Contains(t, err.Error(), "sortOrder mismatch")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Error - non-numeric cursor ID rejected", func(t *testing.T) {
		repo, mock, cleanup := setupAuditEventRepositoryMockDB(t)
		defer cleanup()

		filters := &model.AuditEventFilters{
			Limit:     10,
			SortBy:    "created_at",
			SortOrder: "DESC",
			Cursor:    validCursor(t, "not-an-int", "DESC"),
		}

		result, err := repo.List(context.Background(), filters)
		require.Error(t, err)
		assert.Nil(t, result)
		assert.ErrorIs(t, err, constant.ErrInvalidCursor)
		assert.Contains(t, err.Error(), "invalid ID format")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Success - DESC cursor emits less-than keyset predicate", func(t *testing.T) {
		repo, mock, cleanup := setupAuditEventRepositoryMockDB(t)
		defer cleanup()

		// DESC keyset: created_at < $ OR (created_at = $ AND id < $).
		mock.ExpectQuery(`created_at <`).
			WillReturnRows(auditEventRow(t, createTestAuditEvent(t)))

		filters := &model.AuditEventFilters{
			Limit:     10,
			SortBy:    "created_at",
			SortOrder: "DESC",
			Cursor:    validCursor(t, "100", "DESC"),
		}

		result, err := repo.List(context.Background(), filters)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Len(t, result.AuditEvents, 1)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Success - ASC cursor emits greater-than keyset predicate", func(t *testing.T) {
		repo, mock, cleanup := setupAuditEventRepositoryMockDB(t)
		defer cleanup()

		// ASC keyset: created_at > $ OR (created_at = $ AND id > $).
		mock.ExpectQuery(`created_at >`).
			WillReturnRows(auditEventRow(t, createTestAuditEvent(t)))

		filters := &model.AuditEventFilters{
			Limit:     10,
			SortBy:    "created_at",
			SortOrder: "ASC",
			Cursor:    validCursor(t, "100", "ASC"),
		}

		result, err := repo.List(context.Background(), filters)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Len(t, result.AuditEvents, 1)
		require.NoError(t, mock.ExpectationsWereMet())
	})
}
