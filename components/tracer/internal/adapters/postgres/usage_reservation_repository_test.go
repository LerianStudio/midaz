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
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

var (
	reserveAmount = decimal.NewFromInt(400)
	maxAmountTest = decimal.NewFromInt(1000)
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

func TestUsageReservationRepository_Reserve(t *testing.T) {
	testutil.SetupTestTracing(t)

	t.Run("Success - reserve inserts row then seeds counter", func(t *testing.T) {
		repo, db, mock, cleanup := setupUsageReservationRepository(t)
		defer cleanup()

		res := newTestReservation(t)

		// Reservation row insert (4-tuple ON CONFLICT grain) runs first; 1 row affected
		// means a new reservation.
		mock.ExpectExec(regexp.QuoteMeta(reserveInsertSQL)).
			WillReturnResult(sqlmock.NewResult(0, 1))
		// A new row was inserted, so the reserve CTE (counter seed) follows and returns
		// succeeded=true.
		mock.ExpectQuery(regexp.QuoteMeta(upsertReserveSQL)).
			WillReturnRows(sqlmock.NewRows([]string{"reserved_usage", "succeeded"}).AddRow("400", true))

		err := repo.ReserveWithTx(context.Background(), db, res, maxAmountTest)
		require.NoError(t, err)
	})

	t.Run("Replay - existing row suppresses the counter move", func(t *testing.T) {
		repo, db, mock, cleanup := setupUsageReservationRepository(t)
		defer cleanup()

		res := newTestReservation(t)

		// ON CONFLICT DO NOTHING suppresses the insert (0 rows affected): the 4-tuple
		// already exists. The counter CTE MUST NOT run, so the replay holds capacity
		// exactly once. ExpectationsWereMet on cleanup asserts no stray counter query.
		mock.ExpectExec(regexp.QuoteMeta(reserveInsertSQL)).
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := repo.ReserveWithTx(context.Background(), db, res, maxAmountTest)
		require.NoError(t, err)
	})

	t.Run("Fractional amount is inserted without truncation", func(t *testing.T) {
		repo, db, mock, cleanup := setupUsageReservationRepository(t)
		defer cleanup()

		res, err := model.NewReservation(
			testutil.MustDeterministicUUID(8001),
			testutil.MustDeterministicUUID(8002),
			"acct:8001",
			"2026-06",
			decimal.RequireFromString("10.50"),
			testutil.FixedTime().Add(5*time.Minute),
			testutil.FixedTime(),
		)
		require.NoError(t, err)

		// The reservation carries the exact decimal; the pre-fix int64 row would have
		// held 10.
		require.Equal(t, "10.5", res.Amount.String())

		maxAmount := decimal.NewFromInt(20)

		// WithArgs guards the changed line: the exact fractional amount (not a
		// truncated integer) MUST reach both the row insert and the reserve CTE. The row
		// insert runs first; its $5 amount carries the exact fraction on the row.
		mock.ExpectExec(regexp.QuoteMeta(reserveInsertSQL)).
			WithArgs(
				res.ID,                             // $1 reservation id
				res.LimitID,                        // $2 limit id
				res.ScopeKey,                       // $3 scope key
				res.PeriodKey,                      // $4 period key
				decimal.RequireFromString("10.50"), // $5 amount — exact fraction on the row
				string(res.Status),                 // $6 status
				res.TransactionID,                  // $7 transaction id
				sqlmock.AnyArg(),                   // $8 reservation_expires_at
				sqlmock.AnyArg(),                   // $9 created_at
			).
			WillReturnResult(sqlmock.NewResult(0, 1))
		// A new row was inserted, so the reserve CTE follows. It binds the amount three
		// times ($5 INSERT seed, $7 UPDATE increment, $9 WHERE-guard check) and the cap
		// once ($10); the counter id, timestamps and expiry are non-deterministic. A
		// hardcoded return row alone would pass even if the repo truncated the amount.
		mock.ExpectQuery(regexp.QuoteMeta(upsertReserveSQL)).
			WithArgs(
				sqlmock.AnyArg(),                   // $1 counter id (uuid.New)
				res.LimitID.String(),               // $2 limit id
				res.ScopeKey,                       // $3 scope key
				res.PeriodKey,                      // $4 period key
				decimal.RequireFromString("10.50"), // $5 INSERT reserved_usage seed
				sqlmock.AnyArg(),                   // $6 last_updated_at
				decimal.RequireFromString("10.50"), // $7 UPDATE reserved_usage increment
				sqlmock.AnyArg(),                   // $8 last_updated_at
				decimal.RequireFromString("10.50"), // $9 WHERE-guard amount
				maxAmount,                          // $10 WHERE-guard cap
				sqlmock.AnyArg(),                   // $11 reservation_expires_at
			).
			WillReturnRows(sqlmock.NewRows([]string{"reserved_usage", "succeeded"}).AddRow("10.5", true))

		require.NoError(t, repo.ReserveWithTx(context.Background(), db, res, maxAmount))
	})

	t.Run("Guard denies - exceeds-limit error after row insert", func(t *testing.T) {
		repo, db, mock, cleanup := setupUsageReservationRepository(t)
		defer cleanup()

		res := newTestReservation(t)

		// A new row inserts first (1 row affected); the reserve CTE then fails its WHERE
		// guard (succeeded=false) -> ErrUsageCounterExceedsLimit. The caller rolls the
		// transaction back, which unwinds the row inserted above.
		mock.ExpectExec(regexp.QuoteMeta(reserveInsertSQL)).
			WillReturnResult(sqlmock.NewResult(0, 1))
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
		expectReservedByTransactionSelect(
			mock, txID,
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

		expectReservedByTransactionSelect(
			mock, txID,
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
