// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
)

// setupUsageReservationRepository wires the reservation repository plus the shared
// usage-counter repository over a sqlmock DB, asserting all expectations were met
// on cleanup. The service owns the transaction, so the test passes the raw *sql.DB
// (which satisfies pgdb.DB) directly as the tx handle — no Begin/Commit on the repo.
func setupUsageReservationRepository(t *testing.T) (*UsageReservationRepository, *sql.DB, sqlmock.Sqlmock, func()) {
	t.Helper()

	db, sqlMock, err := sqlmock.New()
	require.NoError(t, err)

	counterRepo := NewUsageCounterRepositoryWithConnection(nil)
	repo := NewUsageReservationRepositoryWithConnection(counterRepo)

	cleanup := func() {
		require.NoError(t, sqlMock.ExpectationsWereMet())

		if err := db.Close(); err != nil {
			t.Logf("failed to close mock db: %v", err)
		}
	}

	return repo, db, sqlMock, cleanup
}

func TestUsageReservationRepository_AcquireReserveScopeLock(t *testing.T) {
	testutil.SetupTestTracing(t)

	t.Run("bounds the lock wait then issues the advisory lock on the supplied handle", func(t *testing.T) {
		repo, db, mock, cleanup := setupUsageReservationRepository(t)
		defer cleanup()

		// lock_timeout is set FIRST (transaction-local), then the advisory lock.
		mock.ExpectExec(regexp.QuoteMeta(`SELECT set_config('lock_timeout', $1, true)`)).
			WithArgs(reserveLockTimeout.String()).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock($1)`)).
			WithArgs(int64(4242)).
			WillReturnResult(sqlmock.NewResult(0, 0))

		require.NoError(t, repo.AcquireReserveScopeLock(context.Background(), db, 4242))
	})

	t.Run("nil handle returns the connection sentinel", func(t *testing.T) {
		repo, _, _, cleanup := setupUsageReservationRepository(t)
		defer cleanup()

		require.ErrorIs(t, repo.AcquireReserveScopeLock(context.Background(), nil, 1), pgdb.ErrNilConnection)
	})

	t.Run("wraps a lock_timeout driver error", func(t *testing.T) {
		repo, db, mock, cleanup := setupUsageReservationRepository(t)
		defer cleanup()

		mock.ExpectExec(regexp.QuoteMeta(`SELECT set_config('lock_timeout', $1, true)`)).
			WithArgs(reserveLockTimeout.String()).
			WillReturnError(assert.AnError)

		err := repo.AcquireReserveScopeLock(context.Background(), db, 9)
		require.Error(t, err)
		assert.ErrorIs(t, err, assert.AnError)
	})

	t.Run("wraps an advisory-lock driver error", func(t *testing.T) {
		repo, db, mock, cleanup := setupUsageReservationRepository(t)
		defer cleanup()

		mock.ExpectExec(regexp.QuoteMeta(`SELECT set_config('lock_timeout', $1, true)`)).
			WithArgs(reserveLockTimeout.String()).
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock($1)`)).
			WithArgs(int64(7)).
			WillReturnError(assert.AnError)

		err := repo.AcquireReserveScopeLock(context.Background(), db, 7)
		require.Error(t, err)
		assert.ErrorIs(t, err, assert.AnError)
	})
}
