// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// setupReaperRepo wires a ReservationReaperRepository whose read sweep resolves
// through a mock Connection backed by sqlmock, and whose per-row release opens a
// real *sql.Tx over the SAME sqlmock DB via a mock TxBeginner. The composed
// UsageReservationRepository runs its real SQL against the sqlmock expectations,
// so the reaper's own begin/release/commit/rollback branches are exercised
// end-to-end without a real database.
func setupReaperRepo(t *testing.T) (*ReservationReaperRepository, *sql.DB, sqlmock.Sqlmock, *mocks.MockTxBeginner, func()) {
	t.Helper()

	ctrl := gomock.NewController(t)

	db, sqlMock, err := sqlmock.New()
	require.NoError(t, err)

	mockConn := mocks.NewMockConnection(ctrl)
	mockConn.EXPECT().GetDB(gomock.Any()).Return(db, nil).AnyTimes()

	mockBeginner := mocks.NewMockTxBeginner(ctrl)

	counterRepo := NewUsageCounterRepositoryWithConnection(nil)
	resRepo := NewUsageReservationRepositoryWithConnection(counterRepo)

	reaper := NewReservationReaperRepository(mockConn, mockBeginner, resRepo)

	cleanup := func() {
		db.Close()
		ctrl.Finish()
	}

	return reaper, db, sqlMock, mockBeginner, cleanup
}

func TestReservationReaperRepository_FindExpiredReservations(t *testing.T) {
	testutil.SetupTestTracing(t)

	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	expiresAt := now.Add(-time.Minute)
	columns := []string{"id", "reservation_expires_at", "decision_id", "integration_id", "transaction_id"}

	const findExpired = `SELECT r.id, r.reservation_expires_at, r.decision_id, d.integration_id, d.transaction_id FROM usage_reservations AS r LEFT JOIN reserve_decisions AS d`

	t.Run("Success - returns expired reserved rows with decision operations", func(t *testing.T) {
		reaper, _, mock, _, cleanup := setupReaperRepo(t)
		defer cleanup()

		idA := testutil.MustDeterministicUUID(7001)
		idB := testutil.MustDeterministicUUID(7002)
		decisionID := testutil.MustDeterministicUUID(7004)
		transactionID := testutil.MustDeterministicUUID(7005)

		mock.ExpectQuery(findExpired).
			WithArgs(now.UTC()).
			WillReturnRows(sqlmock.NewRows(columns).AddRow(idA, expiresAt, nil, nil, nil).AddRow(idB, expiresAt, decisionID, "producer", transactionID))

		expired, err := reaper.FindExpiredReservations(context.Background(), now, nil, 100)
		require.NoError(t, err)
		require.Len(t, expired, 2)
		assert.Equal(t, model.ExpiredReservation{ID: idA, ExpiresAt: expiresAt}, expired[0])
		assert.Equal(t, model.ExpiredReservation{ID: idB, ExpiresAt: expiresAt, Operation: &model.ReserveOperationIdentity{IntegrationID: "producer", TransactionID: transactionID}}, expired[1])
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Success - status is a literal and the sweep is capped", func(t *testing.T) {
		reaper, _, mock, _, cleanup := setupReaperRepo(t)
		defer cleanup()

		mock.ExpectQuery(`WHERE r.status = 'RESERVED' AND r.reservation_expires_at < \$1 ORDER BY r.reservation_expires_at, r.id LIMIT 7`).
			WithArgs(now.UTC()).
			WillReturnRows(sqlmock.NewRows(columns))

		expired, err := reaper.FindExpiredReservations(context.Background(), now, nil, 7)
		require.NoError(t, err)
		assert.Empty(t, expired)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Success - a resume position reads strictly past it", func(t *testing.T) {
		reaper, _, mock, _, cleanup := setupReaperRepo(t)
		defer cleanup()

		after := model.ReservationExpiryPosition{ExpiresAt: expiresAt, ID: testutil.MustDeterministicUUID(7008)}

		mock.ExpectQuery(`AND r.reservation_expires_at < \$1 AND \(r.reservation_expires_at, r.id\) > \(\$2, \$3\) ORDER BY r.reservation_expires_at, r.id LIMIT 7`).
			WithArgs(now.UTC(), after.ExpiresAt, after.ID).
			WillReturnRows(sqlmock.NewRows(columns))

		expired, err := reaper.FindExpiredReservations(context.Background(), now, &after, 7)
		require.NoError(t, err)
		assert.Empty(t, expired)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Error - non-positive limit is rejected before the query", func(t *testing.T) {
		reaper, _, mock, _, cleanup := setupReaperRepo(t)
		defer cleanup()

		expired, err := reaper.FindExpiredReservations(context.Background(), now, nil, 0)
		require.ErrorIs(t, err, constant.ErrInvalidRequestBody)
		assert.Nil(t, expired)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Success - no expired rows returns nil slice", func(t *testing.T) {
		reaper, _, mock, _, cleanup := setupReaperRepo(t)
		defer cleanup()

		mock.ExpectQuery(findExpired).
			WithArgs(now.UTC()).
			WillReturnRows(sqlmock.NewRows(columns))

		expired, err := reaper.FindExpiredReservations(context.Background(), now, nil, 100)
		require.NoError(t, err)
		assert.Empty(t, expired)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Error - query fails", func(t *testing.T) {
		reaper, _, mock, _, cleanup := setupReaperRepo(t)
		defer cleanup()

		mock.ExpectQuery(findExpired).
			WillReturnError(errors.New("connection reset"))

		expired, err := reaper.FindExpiredReservations(context.Background(), now, nil, 100)
		require.Error(t, err)
		assert.Nil(t, expired)
		assert.Contains(t, err.Error(), "failed to query expired reservations")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Error - scan fails on malformed id", func(t *testing.T) {
		reaper, _, mock, _, cleanup := setupReaperRepo(t)
		defer cleanup()

		mock.ExpectQuery(findExpired).
			WithArgs(now.UTC()).
			WillReturnRows(sqlmock.NewRows(columns).AddRow("not-a-uuid", expiresAt, nil, nil, nil))

		expired, err := reaper.FindExpiredReservations(context.Background(), now, nil, 100)
		require.Error(t, err)
		assert.Nil(t, expired)
		assert.Contains(t, err.Error(), "failed to scan expired reservation")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Error - decision row without its decision fails closed", func(t *testing.T) {
		reaper, _, mock, _, cleanup := setupReaperRepo(t)
		defer cleanup()

		mock.ExpectQuery(findExpired).
			WithArgs(now.UTC()).
			WillReturnRows(sqlmock.NewRows(columns).AddRow(testutil.MustDeterministicUUID(7006), expiresAt, testutil.MustDeterministicUUID(7007), nil, nil))

		expired, err := reaper.FindExpiredReservations(context.Background(), now, nil, 100)
		require.ErrorIs(t, err, constant.ErrInternalServer)
		assert.Nil(t, expired)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Error - rows iteration error", func(t *testing.T) {
		reaper, _, mock, _, cleanup := setupReaperRepo(t)
		defer cleanup()

		idA := testutil.MustDeterministicUUID(7003)

		mock.ExpectQuery(findExpired).
			WithArgs(now.UTC()).
			WillReturnRows(
				sqlmock.NewRows(columns).
					AddRow(idA, expiresAt, nil, nil, nil).
					RowError(0, errors.New("read error mid-stream")),
			)

		expired, err := reaper.FindExpiredReservations(context.Background(), now, nil, 100)
		require.Error(t, err)
		assert.Nil(t, expired)
		assert.Contains(t, err.Error(), "failed to iterate expired reservations")
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

// reaperLockRow scripts the FOR UPDATE lock select that ReleaseWithTx issues,
// returning a row in the supplied status.
func reaperLockRow(mock sqlmock.Sqlmock, resID, limitID, txID uuid.UUID, status string) {
	mock.ExpectQuery(`SELECT id, limit_id`).
		WithArgs(resID).
		WillReturnRows(sqlmock.NewRows(reservationLockColumns()).AddRow(
			resID, limitID, "acct:7100", "2026-06", int64(400), status,
			txID, testutil.FixedTime(), testutil.FixedTime(), nil, nil,
		))
}

func TestReservationReaperRepository_ReleaseExpired(t *testing.T) {
	testutil.SetupTestTracing(t)

	resID := testutil.MustDeterministicUUID(7101)
	limitID := testutil.MustDeterministicUUID(7102)
	txID := testutil.MustDeterministicUUID(7103)

	t.Run("Success - flips RESERVED to EXPIRED and commits", func(t *testing.T) {
		reaper, db, mock, beginner, cleanup := setupReaperRepo(t)
		defer cleanup()

		mock.ExpectBegin()
		reaperLockRow(mock, resID, limitID, txID, "RESERVED")
		mock.ExpectExec(`UPDATE usage_counters SET reserved_usage`).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`UPDATE usage_reservations SET status`).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit()

		beginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, opts *sql.TxOptions) (pgdb.Tx, error) {
				return db.BeginTx(ctx, opts)
			})

		err := reaper.ReleaseExpired(context.Background(), resID)
		require.NoError(t, err)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Idempotent no-op - already terminal row rolls back as success", func(t *testing.T) {
		reaper, db, mock, beginner, cleanup := setupReaperRepo(t)
		defer cleanup()

		mock.ExpectBegin()
		// Row already CONFIRMED: ReleaseWithTx returns ErrReservationAlreadyTerminal,
		// the reaper maps it to success, and the deferred rollback fires (no commit).
		reaperLockRow(mock, resID, limitID, txID, "CONFIRMED")
		mock.ExpectRollback()

		beginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, opts *sql.TxOptions) (pgdb.Tx, error) {
				return db.BeginTx(ctx, opts)
			})

		err := reaper.ReleaseExpired(context.Background(), resID)
		require.NoError(t, err, "an already-terminal reservation must be an idempotent success")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Error - BeginTx fails", func(t *testing.T) {
		reaper, _, _, beginner, cleanup := setupReaperRepo(t)
		defer cleanup()

		beginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).
			Return(nil, errors.New("pool exhausted"))

		err := reaper.ReleaseExpired(context.Background(), resID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to begin reaper transaction")
	})

	t.Run("Error - BeginTx returns nil tx without error", func(t *testing.T) {
		reaper, _, _, beginner, cleanup := setupReaperRepo(t)
		defer cleanup()

		beginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).
			Return(nil, nil)

		err := reaper.ReleaseExpired(context.Background(), resID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "BeginTx returned nil transaction without error")
	})

	t.Run("Error - release fails (non-terminal) propagates and rolls back", func(t *testing.T) {
		reaper, db, mock, beginner, cleanup := setupReaperRepo(t)
		defer cleanup()

		mock.ExpectBegin()
		// Reservation not found: ReleaseWithTx returns ErrReservationNotFound,
		// which is NOT ErrReservationAlreadyTerminal, so it propagates and the
		// transaction rolls back.
		mock.ExpectQuery(`SELECT id, limit_id`).
			WithArgs(resID).
			WillReturnError(sql.ErrNoRows)
		mock.ExpectRollback()

		beginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, opts *sql.TxOptions) (pgdb.Tx, error) {
				return db.BeginTx(ctx, opts)
			})

		err := reaper.ReleaseExpired(context.Background(), resID)
		require.Error(t, err)
		assert.ErrorIs(t, err, constant.ErrReservationNotFound)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Error - commit fails", func(t *testing.T) {
		reaper, db, mock, beginner, cleanup := setupReaperRepo(t)
		defer cleanup()

		mock.ExpectBegin()
		reaperLockRow(mock, resID, limitID, txID, "RESERVED")
		mock.ExpectExec(`UPDATE usage_counters SET reserved_usage`).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`UPDATE usage_reservations SET status`).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectCommit().WillReturnError(errors.New("commit conflict"))

		beginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, opts *sql.TxOptions) (pgdb.Tx, error) {
				return db.BeginTx(ctx, opts)
			})

		err := reaper.ReleaseExpired(context.Background(), resID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to commit reaper transaction")
		require.NoError(t, mock.ExpectationsWereMet())
	})
}

// TestReservationReaperRepository_ReleaseExpired_StatusIsExpired pins the reaper
// to the EXPIRED transition specifically (not RELEASED): the row-flip guard must
// write status='EXPIRED' so audit consumers can distinguish a reaper sweep from
// a caller-initiated release.
func TestReservationReaperRepository_ReleaseExpired_StatusIsExpired(t *testing.T) {
	testutil.SetupTestTracing(t)

	resID := testutil.MustDeterministicUUID(7201)
	limitID := testutil.MustDeterministicUUID(7202)
	txID := testutil.MustDeterministicUUID(7203)

	reaper, db, mock, beginner, cleanup := setupReaperRepo(t)
	defer cleanup()

	mock.ExpectBegin()
	reaperLockRow(mock, resID, limitID, txID, "RESERVED")
	mock.ExpectExec(`UPDATE usage_counters SET reserved_usage`).
		WillReturnResult(sqlmock.NewResult(0, 1))
	// The row flip must carry the EXPIRED status value as the first SET arg.
	mock.ExpectExec(`UPDATE usage_reservations SET status`).
		WithArgs(
			string(model.StatusExpired),
			sqlmock.AnyArg(), // released_at
			resID,
			string(model.StatusReserved),
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	beginner.EXPECT().BeginTx(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, opts *sql.TxOptions) (pgdb.Tx, error) {
			return db.BeginTx(ctx, opts)
		})

	err := reaper.ReleaseExpired(context.Background(), resID)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}
