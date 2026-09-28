// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// setupReaperRepo wires a ReservationReaperRepository whose read sweep resolves
// through a mock Connection backed by sqlmock.
func setupReaperRepo(t *testing.T) (*ReservationReaperRepository, sqlmock.Sqlmock, func()) {
	t.Helper()

	ctrl := gomock.NewController(t)

	db, sqlMock, err := sqlmock.New()
	require.NoError(t, err)

	mockConn := mocks.NewMockConnection(ctrl)
	mockConn.EXPECT().GetDB(gomock.Any()).Return(db, nil).AnyTimes()

	reaper := NewReservationReaperRepository(mockConn)

	cleanup := func() {
		db.Close()
		ctrl.Finish()
	}

	return reaper, sqlMock, cleanup
}

func TestReservationReaperRepository_FindExpiredReservations(t *testing.T) {
	testutil.SetupTestTracing(t)

	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	expiresAt := now.Add(-time.Minute)
	columns := []string{"id", "reservation_expires_at", "integration_id", "transaction_id"}

	const findExpired = `SELECT r.id, r.reservation_expires_at, d.integration_id, d.transaction_id FROM usage_reservations AS r LEFT JOIN reserve_decisions AS d`

	t.Run("Success - returns expired reserved rows with their operations", func(t *testing.T) {
		reaper, mock, cleanup := setupReaperRepo(t)
		defer cleanup()

		idA := testutil.MustDeterministicUUID(7001)
		idB := testutil.MustDeterministicUUID(7002)
		transactionA := testutil.MustDeterministicUUID(7004)
		transactionB := testutil.MustDeterministicUUID(7005)

		mock.ExpectQuery(findExpired).
			WithArgs(now.UTC()).
			WillReturnRows(sqlmock.NewRows(columns).AddRow(idA, expiresAt, "producer", transactionA).AddRow(idB, expiresAt, "other-producer", transactionB))

		expired, err := reaper.FindExpiredReservations(context.Background(), now, nil, 100)
		require.NoError(t, err)
		require.Len(t, expired, 2)
		assert.Equal(t, model.ExpiredReservation{ID: idA, ExpiresAt: expiresAt, Operation: model.ReserveOperationIdentity{IntegrationID: "producer", TransactionID: transactionA}}, expired[0])
		assert.Equal(t, model.ExpiredReservation{ID: idB, ExpiresAt: expiresAt, Operation: model.ReserveOperationIdentity{IntegrationID: "other-producer", TransactionID: transactionB}}, expired[1])
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Success - status is a literal and the sweep is capped", func(t *testing.T) {
		reaper, mock, cleanup := setupReaperRepo(t)
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
		reaper, mock, cleanup := setupReaperRepo(t)
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
		reaper, mock, cleanup := setupReaperRepo(t)
		defer cleanup()

		expired, err := reaper.FindExpiredReservations(context.Background(), now, nil, 0)
		require.ErrorIs(t, err, constant.ErrInvalidRequestBody)
		assert.Nil(t, expired)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Success - no expired rows returns nil slice", func(t *testing.T) {
		reaper, mock, cleanup := setupReaperRepo(t)
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
		reaper, mock, cleanup := setupReaperRepo(t)
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
		reaper, mock, cleanup := setupReaperRepo(t)
		defer cleanup()

		mock.ExpectQuery(findExpired).
			WithArgs(now.UTC()).
			WillReturnRows(sqlmock.NewRows(columns).AddRow("not-a-uuid", expiresAt, "producer", testutil.MustDeterministicUUID(7009)))

		expired, err := reaper.FindExpiredReservations(context.Background(), now, nil, 100)
		require.Error(t, err)
		assert.Nil(t, expired)
		assert.Contains(t, err.Error(), "failed to scan expired reservation")
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Error - row without its decision fails closed", func(t *testing.T) {
		reaper, mock, cleanup := setupReaperRepo(t)
		defer cleanup()

		mock.ExpectQuery(findExpired).
			WithArgs(now.UTC()).
			WillReturnRows(sqlmock.NewRows(columns).AddRow(testutil.MustDeterministicUUID(7006), expiresAt, nil, nil))

		expired, err := reaper.FindExpiredReservations(context.Background(), now, nil, 100)
		require.ErrorIs(t, err, constant.ErrInternalServer)
		assert.Nil(t, expired)
		require.NoError(t, mock.ExpectationsWereMet())
	})

	t.Run("Error - rows iteration error", func(t *testing.T) {
		reaper, mock, cleanup := setupReaperRepo(t)
		defer cleanup()

		idA := testutil.MustDeterministicUUID(7003)

		mock.ExpectQuery(findExpired).
			WithArgs(now.UTC()).
			WillReturnRows(
				sqlmock.NewRows(columns).
					AddRow(idA, expiresAt, "producer", testutil.MustDeterministicUUID(7007)).
					RowError(0, errors.New("read error mid-stream")),
			)

		expired, err := reaper.FindExpiredReservations(context.Background(), now, nil, 100)
		require.Error(t, err)
		assert.Nil(t, expired)
		assert.Contains(t, err.Error(), "failed to iterate expired reservations")
		require.NoError(t, mock.ExpectationsWereMet())
	})
}
