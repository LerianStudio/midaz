// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package postgres

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pgdb "github.com/LerianStudio/midaz/v3/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v3/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v3/components/tracer/pkg/model"
)

const (
	reserveAmount = int64(400)
	maxAmountTest = int64(1000)
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

func newTestReservation(t *testing.T) *model.Reservation {
	t.Helper()

	res, err := model.NewReservation(
		testutil.MustDeterministicUUID(8001), // limitID
		testutil.MustDeterministicUUID(8002), // transactionID
		"acct:8001",
		"2026-06",
		reserveAmount,
		testutil.FixedTime().Add(5*time.Minute),
		testutil.FixedTime(),
	)
	require.NoError(t, err)

	return res
}

// reserveInsertSQL is the expected reservation-row INSERT, asserting the 4-tuple
// ON CONFLICT DO NOTHING idempotency grain.
const reserveInsertSQL = `
		INSERT INTO usage_reservations (
			id, limit_id, scope_key, period_key, amount, status,
			transaction_id, reservation_expires_at, created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (transaction_id, limit_id, scope_key, period_key) DO NOTHING
	`

func TestUsageReservationRepository_Reserve(t *testing.T) {
	testutil.SetupTestTracing(t)

	t.Run("Success - reserve seeds counter and inserts row", func(t *testing.T) {
		repo, db, mock, cleanup := setupUsageReservationRepository(t)
		defer cleanup()

		res := newTestReservation(t)

		// Reserve CTE (counter seed) returns succeeded=true.
		mock.ExpectQuery(regexp.QuoteMeta(upsertReserveSQL)).
			WillReturnRows(sqlmock.NewRows([]string{"reserved_usage", "succeeded"}).AddRow("400", true))
		// Reservation row insert with the 4-tuple ON CONFLICT grain.
		mock.ExpectExec(regexp.QuoteMeta(reserveInsertSQL)).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.ReserveWithTx(context.Background(), db, res, maxAmountTest)
		require.NoError(t, err)
	})

	t.Run("Guard denies - exceeds-limit error, no row inserted", func(t *testing.T) {
		repo, db, mock, cleanup := setupUsageReservationRepository(t)
		defer cleanup()

		res := newTestReservation(t)

		// Reserve CTE WHERE guard fails: succeeded=false -> ErrUsageCounterExceedsLimit.
		// No INSERT expected — the guard error returns before the row insert.
		mock.ExpectQuery(regexp.QuoteMeta(upsertReserveSQL)).
			WillReturnRows(sqlmock.NewRows([]string{"reserved_usage", "succeeded"}).AddRow("1000", false))

		err := repo.ReserveWithTx(context.Background(), db, res, maxAmountTest)
		require.ErrorIs(t, err, constant.ErrUsageCounterExceedsLimit)
	})

	t.Run("Nil db is rejected", func(t *testing.T) {
		repo, _, _, cleanup := setupUsageReservationRepository(t)
		defer cleanup()

		err := repo.ReserveWithTx(context.Background(), nil, newTestReservation(t), maxAmountTest)
		require.ErrorIs(t, err, pgdb.ErrNilConnection)
	})

	t.Run("Nil reservation is rejected", func(t *testing.T) {
		repo, db, _, cleanup := setupUsageReservationRepository(t)
		defer cleanup()

		err := repo.ReserveWithTx(context.Background(), db, nil, maxAmountTest)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "reservation cannot be nil")
	})
}

// reservationLockColumns is the row shape lockReservation scans.
func reservationLockColumns() []string {
	return []string{
		"id", "limit_id", "scope_key", "period_key", "amount", "status",
		"transaction_id", "reservation_expires_at", "created_at", "confirmed_at", "released_at",
	}
}

func TestUsageReservationRepository_Confirm(t *testing.T) {
	testutil.SetupTestTracing(t)

	resID := testutil.MustDeterministicUUID(8101)
	limitID := testutil.MustDeterministicUUID(8102)
	txID := testutil.MustDeterministicUUID(8103)

	t.Run("Success - counter move + row flip", func(t *testing.T) {
		repo, db, mock, cleanup := setupUsageReservationRepository(t)
		defer cleanup()

		mock.ExpectQuery(`SELECT id, limit_id`).
			WithArgs(resID).
			WillReturnRows(sqlmock.NewRows(reservationLockColumns()).AddRow(
				resID, limitID, "acct:8101", "2026-06", int64(400), "RESERVED",
				txID, testutil.FixedTime(), testutil.FixedTime(), nil, nil,
			))
		// Counter move: current_usage += amount, reserved_usage -= amount.
		mock.ExpectExec(`UPDATE usage_counters SET current_usage`).
			WillReturnResult(sqlmock.NewResult(0, 1))
		// Row flip guarded WHERE status='RESERVED'.
		mock.ExpectExec(`UPDATE usage_reservations SET status`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.ConfirmWithTx(context.Background(), db, resID)
		require.NoError(t, err)
	})

	t.Run("Idempotent double-confirm - terminal row, NO counter move", func(t *testing.T) {
		repo, db, mock, cleanup := setupUsageReservationRepository(t)
		defer cleanup()

		// Row already CONFIRMED — lockReservation sees a terminal status, so the
		// counter move is NEVER issued (no double-move).
		mock.ExpectQuery(`SELECT id, limit_id`).
			WithArgs(resID).
			WillReturnRows(sqlmock.NewRows(reservationLockColumns()).AddRow(
				resID, limitID, "acct:8101", "2026-06", int64(400), "CONFIRMED",
				txID, testutil.FixedTime(), testutil.FixedTime(), testutil.FixedTime(), nil,
			))

		err := repo.ConfirmWithTx(context.Background(), db, resID)
		require.ErrorIs(t, err, constant.ErrReservationAlreadyTerminal)
	})

	t.Run("Not found - missing row maps to ErrReservationNotFound", func(t *testing.T) {
		repo, db, mock, cleanup := setupUsageReservationRepository(t)
		defer cleanup()

		mock.ExpectQuery(`SELECT id, limit_id`).
			WithArgs(resID).
			WillReturnError(sql.ErrNoRows)

		err := repo.ConfirmWithTx(context.Background(), db, resID)
		require.ErrorIs(t, err, constant.ErrReservationNotFound)
	})
}

// expectReservedByTransactionSelect scripts the FOR UPDATE select over every
// RESERVED row a transaction holds, returning the supplied (id, limitID, scope,
// period) tuples — one per reservation the by-transaction confirm/release flips.
func expectReservedByTransactionSelect(mock sqlmock.Sqlmock, txID uuid.UUID, rows ...[4]any) {
	r := sqlmock.NewRows(reservationLockColumns())

	for _, row := range rows {
		r = r.AddRow(
			row[0], row[1], row[2], row[3], int64(400), "RESERVED",
			txID, testutil.FixedTime(), testutil.FixedTime(), nil, nil,
		)
	}

	mock.ExpectQuery(`SELECT id, limit_id`).
		WithArgs(txID).
		WillReturnRows(r)
}

func TestUsageReservationRepository_ConfirmByTransaction(t *testing.T) {
	testutil.SetupTestTracing(t)

	txID := testutil.MustDeterministicUUID(8601)
	res1 := testutil.MustDeterministicUUID(8602)
	res2 := testutil.MustDeterministicUUID(8603)
	limit1 := testutil.MustDeterministicUUID(8604)
	limit2 := testutil.MustDeterministicUUID(8605)

	t.Run("Flips ALL reserved rows of the transaction - counter move + row flip each", func(t *testing.T) {
		repo, db, mock, cleanup := setupUsageReservationRepository(t)
		defer cleanup()

		// Two reservations for one transaction (two limits): the select returns both
		// and each gets a counter move + row flip in the SAME (caller-owned) tx.
		expectReservedByTransactionSelect(mock, txID,
			[4]any{res1, limit1, "acct:8601", "2026-06"},
			[4]any{res2, limit2, "global", "2026-06-05"},
		)

		for range []uuid.UUID{res1, res2} {
			mock.ExpectExec(`UPDATE usage_counters SET current_usage`).
				WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec(`UPDATE usage_reservations SET status`).
				WillReturnResult(sqlmock.NewResult(0, 1))
		}

		flipped, err := repo.ConfirmByTransactionWithTx(context.Background(), db, txID)
		require.NoError(t, err)
		assert.Len(t, flipped, 2, "every reserved row of the transaction is flipped")
	})

	t.Run("No reserved rows is an idempotent no-op success (re-run after confirm)", func(t *testing.T) {
		repo, db, mock, cleanup := setupUsageReservationRepository(t)
		defer cleanup()

		// A retried confirm-by-transaction sees no RESERVED rows (all already
		// CONFIRMED): the select returns empty, NO counter move issues, flipped=0.
		expectReservedByTransactionSelect(mock, txID)

		flipped, err := repo.ConfirmByTransactionWithTx(context.Background(), db, txID)
		require.NoError(t, err)
		assert.Empty(t, flipped, "re-run over an already-confirmed transaction does NOT double-move")
	})

	t.Run("Nil db is rejected", func(t *testing.T) {
		repo, _, _, cleanup := setupUsageReservationRepository(t)
		defer cleanup()

		_, err := repo.ConfirmByTransactionWithTx(context.Background(), nil, txID)
		require.ErrorIs(t, err, pgdb.ErrNilConnection)
	})
}

func TestUsageReservationRepository_ReleaseByTransaction(t *testing.T) {
	testutil.SetupTestTracing(t)

	txID := testutil.MustDeterministicUUID(8701)
	res1 := testutil.MustDeterministicUUID(8702)
	res2 := testutil.MustDeterministicUUID(8703)
	limit1 := testutil.MustDeterministicUUID(8704)
	limit2 := testutil.MustDeterministicUUID(8705)

	t.Run("Flips ALL reserved rows - reserved_usage decremented, current_usage untouched", func(t *testing.T) {
		repo, db, mock, cleanup := setupUsageReservationRepository(t)
		defer cleanup()

		expectReservedByTransactionSelect(mock, txID,
			[4]any{res1, limit1, "acct:8701", "2026-06"},
			[4]any{res2, limit2, "global", "2026-06-05"},
		)

		for range []uuid.UUID{res1, res2} {
			// Release counter move touches only reserved_usage.
			mock.ExpectExec(`UPDATE usage_counters SET reserved_usage`).
				WillReturnResult(sqlmock.NewResult(0, 1))
			mock.ExpectExec(`UPDATE usage_reservations SET status`).
				WillReturnResult(sqlmock.NewResult(0, 1))
		}

		flipped, err := repo.ReleaseByTransactionWithTx(context.Background(), db, txID, model.StatusReleased)
		require.NoError(t, err)
		assert.Len(t, flipped, 2)
	})

	t.Run("Invalid status rejected before any SQL", func(t *testing.T) {
		repo, db, _, cleanup := setupUsageReservationRepository(t)
		defer cleanup()

		_, err := repo.ReleaseByTransactionWithTx(context.Background(), db, txID, model.StatusConfirmed)
		require.ErrorIs(t, err, constant.ErrReservationInvalidStatus)
	})

	t.Run("No reserved rows is an idempotent no-op success", func(t *testing.T) {
		repo, db, mock, cleanup := setupUsageReservationRepository(t)
		defer cleanup()

		expectReservedByTransactionSelect(mock, txID)

		flipped, err := repo.ReleaseByTransactionWithTx(context.Background(), db, txID, model.StatusReleased)
		require.NoError(t, err)
		assert.Empty(t, flipped)
	})
}

func TestUsageReservationRepository_Release(t *testing.T) {
	testutil.SetupTestTracing(t)

	resID := testutil.MustDeterministicUUID(8201)
	limitID := testutil.MustDeterministicUUID(8202)
	txID := testutil.MustDeterministicUUID(8203)

	t.Run("Success - reserved_usage decremented, current_usage untouched", func(t *testing.T) {
		repo, db, mock, cleanup := setupUsageReservationRepository(t)
		defer cleanup()

		mock.ExpectQuery(`SELECT id, limit_id`).
			WithArgs(resID).
			WillReturnRows(sqlmock.NewRows(reservationLockColumns()).AddRow(
				resID, limitID, "acct:8201", "2026-06", int64(400), "RESERVED",
				txID, testutil.FixedTime(), testutil.FixedTime(), nil, nil,
			))
		// Release counter move: only reserved_usage decremented (no current_usage).
		mock.ExpectExec(`UPDATE usage_counters SET reserved_usage`).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`UPDATE usage_reservations SET status`).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.ReleaseWithTx(context.Background(), db, resID, model.StatusReleased)
		require.NoError(t, err)
	})

	t.Run("Invalid status rejected before any SQL", func(t *testing.T) {
		repo, db, _, cleanup := setupUsageReservationRepository(t)
		defer cleanup()

		// StatusConfirmed is not a valid release target; rejected before the read.
		err := repo.ReleaseWithTx(context.Background(), db, resID, model.StatusConfirmed)
		require.ErrorIs(t, err, constant.ErrReservationInvalidStatus)
	})

	t.Run("Expire path uses EXPIRED status flip", func(t *testing.T) {
		repo, db, mock, cleanup := setupUsageReservationRepository(t)
		defer cleanup()

		mock.ExpectQuery(`SELECT id, limit_id`).
			WithArgs(resID).
			WillReturnRows(sqlmock.NewRows(reservationLockColumns()).AddRow(
				resID, limitID, "acct:8201", "2026-06", int64(400), "RESERVED",
				txID, testutil.FixedTime(), testutil.FixedTime(), nil, nil,
			))
		mock.ExpectExec(`UPDATE usage_counters SET reserved_usage`).
			WillReturnResult(sqlmock.NewResult(0, 1))
		mock.ExpectExec(`UPDATE usage_reservations SET status`).
			WithArgs(string(model.StatusExpired), sqlmock.AnyArg(), resID, string(model.StatusReserved)).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.ReleaseWithTx(context.Background(), db, resID, model.StatusExpired)
		require.NoError(t, err)
	})
}
