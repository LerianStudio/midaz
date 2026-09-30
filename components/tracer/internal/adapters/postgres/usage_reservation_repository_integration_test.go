// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// newReservationRepoIntegration wires the reservation repository plus the shared
// usage-counter repository over a real PostgreSQL connection.
func newReservationRepoIntegration(db *sql.DB) *UsageReservationRepository {
	adapter := &testutil.IntegrationDBAdapter{DB: db}
	counterRepo := NewUsageCounterRepositoryWithConnection(adapter)

	return NewUsageReservationRepositoryWithConnection(counterRepo)
}

// inRealTx runs fn inside a real *sql.Tx, committing on success and rolling back on
// error — mimicking the reservation service's tx ownership so the repo's *WithTx
// methods are exercised atomically.
func inRealTx(t *testing.T, db *sql.DB, fn func(tx *sql.Tx) error) error {
	t.Helper()

	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)

	if err := fn(tx); err != nil {
		require.NoError(t, tx.Rollback())
		return err
	}

	return tx.Commit()
}

// createTestLimitNamed seeds an ACTIVE limit with an explicit, unique name so
// multiple limits can coexist in one test without colliding on the global
// idx_limits_name_active partial unique index (the shared createTestLimit derives
// the name from the UUID prefix, which is identical across deterministic seeds).
func createTestLimitNamed(t *testing.T, db *sql.DB, seed int64, name string) uuid.UUID {
	t.Helper()

	limitID := testutil.MustDeterministicUUID(seed)

	_, err := db.Exec(`
		INSERT INTO limits (id, name, limit_type, max_amount, asset, scopes, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, limitID, "Test Limit "+name, "DAILY", decimal.NewFromInt(10000), "USD", "[]", "ACTIVE")
	require.NoError(t, err, "Failed to create named test limit")

	return limitID
}

func readCounter(t *testing.T, db *sql.DB, limitID uuid.UUID, scopeKey, periodKey string) (current, reserved int64) {
	t.Helper()

	cur, rsv := readCounterDecimal(t, db, limitID, scopeKey, periodKey)

	return cur.IntPart(), rsv.IntPart()
}

// readCounterDecimal reads the counter buckets as exact decimals, so fractional
// amounts survive the round-trip (the DECIMAL columns landed in migration 000021).
func readCounterDecimal(t *testing.T, db *sql.DB, limitID uuid.UUID, scopeKey, periodKey string) (current, reserved decimal.Decimal) {
	t.Helper()

	err := db.QueryRow(
		"SELECT current_usage, reserved_usage FROM usage_counters WHERE limit_id = $1 AND scope_key = $2 AND period_key = $3",
		limitID, scopeKey, periodKey,
	).Scan(&current, &reserved)
	require.NoError(t, err, "failed to read counter buckets")

	return current, reserved
}

func readReservationStatus(t *testing.T, db *sql.DB, reservationID uuid.UUID) string {
	t.Helper()

	var status string

	err := db.QueryRow("SELECT status FROM usage_reservations WHERE id = $1", reservationID).Scan(&status)
	require.NoError(t, err, "failed to read reservation status")

	return status
}

// TestIntegration_UsageReservationRepository_DoubleConfirm_Idempotent proves the
// core idempotency invariant: a second confirm against an already-CONFIRMED
// reservation performs NO second counter move. After reserve (reserved=400) and
// confirm (current=400, reserved=0), a retried confirm must leave the counter at
// current=400, reserved=0 and return ErrReservationAlreadyTerminal.
func TestIntegration_UsageReservationRepository_DoubleConfirm_Idempotent(t *testing.T) {
	testutil.SetupTestTracing(t)

	db := testutil.SetupIntegrationDB(t)
	repo := newReservationRepoIntegration(db)

	limitID := createTestLimit(t, db, 8501)
	t.Cleanup(func() { cleanupTestLimit(t, db, limitID) })

	scopeKey := "acct:8501-" + testutil.MustDeterministicUUID(8511).String()[:8]
	periodKey := "2026-06"

	ctx := context.Background()
	now := testutil.FixedTime()

	res, err := model.NewReservation(
		limitID,
		testutil.MustDeterministicUUID(8521), // transactionID
		scopeKey,
		periodKey,
		decimal.NewFromInt(400),
		now.Add(5*time.Minute),
		now,
	)
	require.NoError(t, err)

	// Reserve: seeds reserved_usage = 400, current_usage = 0.
	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		_, err := repo.ReserveWithTx(ctx, tx, res, decimal.NewFromInt(10000))
		return err
	}))

	current, reserved := readCounter(t, db, limitID, scopeKey, periodKey)
	assert.Equal(t, int64(0), current, "reserve must not touch current_usage")
	assert.Equal(t, int64(400), reserved, "reserve must seed reserved_usage")

	// First confirm: moves 400 reserved -> current and reports the RESERVED it found.
	var found model.ReservationStatus

	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		var err error

		found, err = repo.ConfirmWithTx(ctx, tx, res.ID)

		return err
	}))
	assert.Equal(t, model.StatusReserved, found, "a settling confirm must report the status it locked")

	current, reserved = readCounter(t, db, limitID, scopeKey, periodKey)
	assert.Equal(t, int64(400), current, "confirm must move amount into current_usage")
	assert.Equal(t, int64(0), reserved, "confirm must drain reserved_usage")
	assert.Equal(t, string(model.StatusConfirmed), readReservationStatus(t, db, res.ID))

	// Second confirm: idempotent — no double-move, counter unchanged, and the
	// caller learns the row was already CONFIRMED (not RELEASED).
	err = inRealTx(t, db, func(tx *sql.Tx) error {
		var err error

		found, err = repo.ConfirmWithTx(ctx, tx, res.ID)

		return err
	})
	require.ErrorIs(t, err, constant.ErrReservationAlreadyTerminal,
		"retried confirm against a terminal row must be an idempotent no-op")
	assert.Equal(t, model.StatusConfirmed, found, "a terminal confirm must report the status it found")

	current, reserved = readCounter(t, db, limitID, scopeKey, periodKey)
	assert.Equal(t, int64(400), current, "double-confirm must NOT double-move into current_usage")
	assert.Equal(t, int64(0), reserved, "double-confirm must NOT drive reserved_usage negative")
}

// TestIntegration_UsageReservationRepository_FractionalAmount_Preserved proves the
// money-path fix end-to-end against real DECIMAL columns: a 10.50 reserve seeds
// reserved_usage=10.50 (not 10), and confirm moves the exact fraction into
// current_usage. Under the pre-fix int64 seam this truncated to 10.
func TestIntegration_UsageReservationRepository_FractionalAmount_Preserved(t *testing.T) {
	testutil.SetupTestTracing(t)

	db := testutil.SetupIntegrationDB(t)
	repo := newReservationRepoIntegration(db)

	limitID := createTestLimit(t, db, 8504)
	t.Cleanup(func() { cleanupTestLimit(t, db, limitID) })

	scopeKey := "acct:8504-" + testutil.MustDeterministicUUID(8514).String()[:8]
	periodKey := "2026-06"

	ctx := context.Background()
	now := testutil.FixedTime()

	want := decimal.RequireFromString("10.50")

	res, err := model.NewReservation(
		limitID,
		testutil.MustDeterministicUUID(8524),
		scopeKey,
		periodKey,
		want,
		now.Add(5*time.Minute),
		now,
	)
	require.NoError(t, err)

	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		_, err := repo.ReserveWithTx(ctx, tx, res, decimal.NewFromInt(20))
		return err
	}))

	current, reserved := readCounterDecimal(t, db, limitID, scopeKey, periodKey)
	assert.True(t, current.IsZero(), "reserve must not touch current_usage")
	assert.True(t, want.Equal(reserved), "reserve must hold the exact fraction, got %s", reserved)

	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		_, err := repo.ConfirmWithTx(ctx, tx, res.ID)
		return err
	}))

	current, reserved = readCounterDecimal(t, db, limitID, scopeKey, periodKey)
	assert.True(t, want.Equal(current), "confirm must move the exact fraction into current_usage, got %s", current)
	assert.True(t, reserved.IsZero(), "confirm must drain reserved_usage")
}

// TestIntegration_UsageReservationRepository_ReleaseThenConfirm_Idempotent proves
// release drains reserved_usage without crediting current_usage, and a confirm
// after release is a terminal no-op.
func TestIntegration_UsageReservationRepository_ReleaseThenConfirm_Idempotent(t *testing.T) {
	testutil.SetupTestTracing(t)

	db := testutil.SetupIntegrationDB(t)
	repo := newReservationRepoIntegration(db)

	limitID := createTestLimit(t, db, 8502)
	t.Cleanup(func() { cleanupTestLimit(t, db, limitID) })

	scopeKey := "acct:8502-" + testutil.MustDeterministicUUID(8512).String()[:8]
	periodKey := "2026-06"

	ctx := context.Background()
	now := testutil.FixedTime()

	res, err := model.NewReservation(
		limitID,
		testutil.MustDeterministicUUID(8522),
		scopeKey,
		periodKey,
		decimal.NewFromInt(250),
		now.Add(5*time.Minute),
		now,
	)
	require.NoError(t, err)

	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		_, err := repo.ReserveWithTx(ctx, tx, res, decimal.NewFromInt(10000))
		return err
	}))
	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		return repo.ReleaseWithTx(ctx, tx, res.ID, model.StatusReleased)
	}))

	current, reserved := readCounter(t, db, limitID, scopeKey, periodKey)
	assert.Equal(t, int64(0), current, "release must NOT credit current_usage")
	assert.Equal(t, int64(0), reserved, "release must drain reserved_usage")
	assert.Equal(t, string(model.StatusReleased), readReservationStatus(t, db, res.ID))

	// Confirm after release: terminal no-op, counter untouched, and the caller
	// learns the row was RELEASED so it can report spend that was never counted.
	var found model.ReservationStatus

	err = inRealTx(t, db, func(tx *sql.Tx) error {
		var err error

		found, err = repo.ConfirmWithTx(ctx, tx, res.ID)

		return err
	})
	require.ErrorIs(t, err, constant.ErrReservationAlreadyTerminal)
	assert.Equal(t, model.StatusReleased, found, "a confirm onto a released row must report RELEASED")

	current, reserved = readCounter(t, db, limitID, scopeKey, periodKey)
	assert.Equal(t, int64(0), current)
	assert.Equal(t, int64(0), reserved)
}

// TestIntegration_UsageReservationRepository_ConfirmByTransaction_FlipsAll proves
// the by-transaction confirm flips EVERY RESERVED reservation a transaction holds
// across two distinct limits, moving each counter ONCE (reserved -> current), and
// that a re-run is an idempotent no-op (flipped=0, counters unchanged). This is the
// PENDING /commit lifecycle path: the ledger addresses the tracer by transaction id
// because the per-reservation handle does not survive the separate commit request.
func TestIntegration_UsageReservationRepository_ConfirmByTransaction_FlipsAll(t *testing.T) {
	testutil.SetupTestTracing(t)

	db := testutil.SetupIntegrationDB(t)
	repo := newReservationRepoIntegration(db)

	// createTestLimit names the limit from the UUID prefix, which is identical for
	// all deterministic seeds (the seed lives in the trailing bytes), so two limits
	// in one test collide on idx_limits_name_active. This test needs two distinct
	// limits under one transaction, so it seeds them with explicitly unique names.
	limitA := createTestLimitNamed(t, db, 8601, "by-txn-confirm-A")
	limitB := createTestLimitNamed(t, db, 8602, "by-txn-confirm-B")
	t.Cleanup(func() {
		cleanupTestLimit(t, db, limitA)
		cleanupTestLimit(t, db, limitB)
	})

	txID := testutil.MustDeterministicUUID(8650)
	scopeA := "acct:8601-" + testutil.MustDeterministicUUID(8611).String()[:8]
	scopeB := "global-" + testutil.MustDeterministicUUID(8612).String()[:8]
	periodKey := "2026-06"

	ctx := context.Background()
	now := testutil.FixedTime()

	// Two reservations under ONE transaction, on two different limits.
	resA, err := model.NewReservation(limitA, txID, scopeA, periodKey, decimal.NewFromInt(400),
		now.Add(5*time.Minute), now)
	require.NoError(t, err)

	resB, err := model.NewReservation(limitB, txID, scopeB, periodKey, decimal.NewFromInt(250),
		now.Add(5*time.Minute), now)
	require.NoError(t, err)

	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		if _, rErr := repo.ReserveWithTx(ctx, tx, resA, decimal.NewFromInt(10000)); rErr != nil {
			return rErr
		}

		_, err := repo.ReserveWithTx(ctx, tx, resB, decimal.NewFromInt(10000))
		return err
	}))

	// Both counters hold their amounts in reserved_usage.
	curA, rsvA := readCounter(t, db, limitA, scopeA, periodKey)
	curB, rsvB := readCounter(t, db, limitB, scopeB, periodKey)
	assert.Equal(t, int64(0), curA)
	assert.Equal(t, int64(400), rsvA)
	assert.Equal(t, int64(0), curB)
	assert.Equal(t, int64(250), rsvB)

	// ConfirmByTransaction flips BOTH in one tx; each counter moves once.
	var flipped []*model.Reservation

	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		var cErr error
		flipped, _, cErr = repo.ConfirmByTransactionWithTx(ctx, tx, txID)

		return cErr
	}))
	assert.Len(t, flipped, 2, "both reservations of the transaction are confirmed")

	curA, rsvA = readCounter(t, db, limitA, scopeA, periodKey)
	curB, rsvB = readCounter(t, db, limitB, scopeB, periodKey)
	assert.Equal(t, int64(400), curA, "limit A amount moved into current_usage")
	assert.Equal(t, int64(0), rsvA)
	assert.Equal(t, int64(250), curB, "limit B amount moved into current_usage")
	assert.Equal(t, int64(0), rsvB)
	assert.Equal(t, string(model.StatusConfirmed), readReservationStatus(t, db, resA.ID))
	assert.Equal(t, string(model.StatusConfirmed), readReservationStatus(t, db, resB.ID))

	// Re-run: no RESERVED rows remain, so it is an idempotent no-op and the counters
	// do NOT double-move.
	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		var cErr error
		flipped, _, cErr = repo.ConfirmByTransactionWithTx(ctx, tx, txID)

		return cErr
	}))
	assert.Empty(t, flipped, "re-run over an already-confirmed transaction flips nothing")

	curA, rsvA = readCounter(t, db, limitA, scopeA, periodKey)
	curB, rsvB = readCounter(t, db, limitB, scopeB, periodKey)
	assert.Equal(t, int64(400), curA, "double-confirm-by-transaction must NOT double-move")
	assert.Equal(t, int64(0), rsvA)
	assert.Equal(t, int64(250), curB)
	assert.Equal(t, int64(0), rsvB)
}

// TestIntegration_UsageReservationRepository_Reserve_RowIdempotent proves a retried
// reserve for the same 4-tuple collapses onto the existing row (ON CONFLICT DO
// NOTHING) and does not duplicate the reservation row.
func TestIntegration_UsageReservationRepository_Reserve_RowIdempotent(t *testing.T) {
	testutil.SetupTestTracing(t)

	db := testutil.SetupIntegrationDB(t)
	repo := newReservationRepoIntegration(db)

	limitID := createTestLimit(t, db, 8503)
	t.Cleanup(func() { cleanupTestLimit(t, db, limitID) })

	scopeKey := "acct:8503-" + testutil.MustDeterministicUUID(8513).String()[:8]
	periodKey := "2026-06"

	ctx := context.Background()
	now := testutil.FixedTime()

	res, err := model.NewReservation(
		limitID,
		testutil.MustDeterministicUUID(8523),
		scopeKey,
		periodKey,
		decimal.NewFromInt(100),
		now.Add(5*time.Minute),
		now,
	)
	require.NoError(t, err)

	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		_, err := repo.ReserveWithTx(ctx, tx, res, decimal.NewFromInt(10000))
		return err
	}))

	// Re-reserve the SAME row id and 4-tuple: ON CONFLICT DO NOTHING keeps a single
	// row.
	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		_, err := repo.ReserveWithTx(ctx, tx, res, decimal.NewFromInt(10000))
		return err
	}))

	var rowCount int

	err = db.QueryRow(
		"SELECT COUNT(*) FROM usage_reservations WHERE transaction_id = $1 AND limit_id = $2 AND scope_key = $3 AND period_key = $4",
		res.TransactionID, limitID, scopeKey, periodKey,
	).Scan(&rowCount)
	require.NoError(t, err)
	assert.Equal(t, 1, rowCount, "retried reserve must not duplicate the reservation row")

	// The replay must be a counter no-op: reserved_usage stays at the single held
	// amount, never doubled. This is the regression lock for the insert-first gate.
	current, reserved := readCounterDecimal(t, db, limitID, scopeKey, periodKey)
	assert.True(t, current.IsZero(), "replayed reserve must not touch current_usage; got %s", current)
	assert.True(t, decimal.NewFromInt(100).Equal(reserved),
		"replayed reserve must not increase reserved_usage")
}

// TestIntegration_UsageReservationRepository_SubUnitaryAmount_Preserved proves a
// sub-unitary reserve (0 < amount < 1) survives the real DECIMAL columns intact. This
// is the case the pre-fix int64 IntPart() seam destroyed WHOLLY: 0.99 collapsed to 0,
// so the reserve held nothing while the transaction believed capacity was reserved.
// Every fractional test before this used amounts > 1, where truncation only shaved
// the cents; only a sub-unitary amount exercises total loss.
func TestIntegration_UsageReservationRepository_SubUnitaryAmount_Preserved(t *testing.T) {
	testutil.SetupTestTracing(t)

	db := testutil.SetupIntegrationDB(t)
	repo := newReservationRepoIntegration(db)

	limitID := createTestLimit(t, db, 8505)
	t.Cleanup(func() { cleanupTestLimit(t, db, limitID) })

	scopeKey := "acct:8505-" + testutil.MustDeterministicUUID(8515).String()[:8]
	periodKey := "2026-06"

	ctx := context.Background()
	now := testutil.FixedTime()

	want := decimal.RequireFromString("0.99")

	res, err := model.NewReservation(
		limitID,
		testutil.MustDeterministicUUID(8525),
		scopeKey,
		periodKey,
		want,
		now.Add(5*time.Minute),
		now,
	)
	require.NoError(t, err)

	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		_, err := repo.ReserveWithTx(ctx, tx, res, decimal.NewFromInt(20))
		return err
	}))

	current, reserved := readCounterDecimal(t, db, limitID, scopeKey, periodKey)
	assert.True(t, current.IsZero(), "reserve must not touch current_usage")
	assert.True(t, want.Equal(reserved),
		"sub-unitary reserve must hold the exact 0.99, not truncate to 0; got %s", reserved)

	// Confirm moves the exact sub-unitary fraction into current_usage.
	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		_, err := repo.ConfirmWithTx(ctx, tx, res.ID)
		return err
	}))

	current, reserved = readCounterDecimal(t, db, limitID, scopeKey, periodKey)
	assert.True(t, want.Equal(current),
		"confirm must move the exact 0.99 into current_usage; got %s", current)
	assert.True(t, reserved.IsZero(), "confirm must drain reserved_usage")
}

// TestIntegration_UsageReservationRepository_FractionalCap_Denies proves the reserve
// CTE's over-limit guard (current_usage + reserved_usage + amount <= maxAmount) holds
// at sub-unitary precision. Against a 0.75 cap: a first 0.50 reserve succeeds, and a
// second 0.50 reserve (0.50 + 0.50 = 1.00 > 0.75) is denied with
// ErrUsageCounterExceedsLimit, leaving reserved_usage at exactly 0.50. Under the
// pre-fix integer seam both amounts truncated to 0 and the cap could never bind.
func TestIntegration_UsageReservationRepository_FractionalCap_Denies(t *testing.T) {
	testutil.SetupTestTracing(t)

	db := testutil.SetupIntegrationDB(t)
	repo := newReservationRepoIntegration(db)

	limitID := createTestLimit(t, db, 8506)
	t.Cleanup(func() { cleanupTestLimit(t, db, limitID) })

	scopeKey := "acct:8506-" + testutil.MustDeterministicUUID(8516).String()[:8]
	periodKey := "2026-06"

	ctx := context.Background()
	now := testutil.FixedTime()

	// The reserve guard checks against the maxAmount the caller passes, not the limit
	// column, so the cap is set here to a sub-unitary 0.75.
	cap075 := decimal.RequireFromString("0.75")
	half := decimal.RequireFromString("0.50")

	// Two reservations under DISTINCT transactions but the SAME counter (limit +
	// scope + period), so the second accumulates onto the first's reserved_usage.
	res1, err := model.NewReservation(
		limitID,
		testutil.MustDeterministicUUID(8526),
		scopeKey,
		periodKey,
		half,
		now.Add(5*time.Minute),
		now,
	)
	require.NoError(t, err)

	res2, err := model.NewReservation(
		limitID,
		testutil.MustDeterministicUUID(8527),
		scopeKey,
		periodKey,
		half,
		now.Add(5*time.Minute),
		now,
	)
	require.NoError(t, err)

	// First 0.50 fits under the 0.75 cap.
	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		_, err := repo.ReserveWithTx(ctx, tx, res1, cap075)
		return err
	}))

	current, reserved := readCounterDecimal(t, db, limitID, scopeKey, periodKey)
	assert.True(t, current.IsZero(), "reserve must not touch current_usage")
	assert.True(t, half.Equal(reserved), "first 0.50 must be held exactly; got %s", reserved)

	// Second 0.50 would push held usage to 1.00 > 0.75 — the guard denies it, and
	// inRealTx rolls the transaction back so no RESERVED row survives.
	err = inRealTx(t, db, func(tx *sql.Tx) error {
		_, err := repo.ReserveWithTx(ctx, tx, res2, cap075)
		return err
	})
	require.ErrorIs(t, err, constant.ErrUsageCounterExceedsLimit,
		"0.50 + 0.50 = 1.00 over a 0.75 cap must be denied")

	// The denied reserve left the counter untouched at the first 0.50.
	current, reserved = readCounterDecimal(t, db, limitID, scopeKey, periodKey)
	assert.True(t, current.IsZero(), "denied reserve must not credit current_usage")
	assert.True(t, half.Equal(reserved),
		"denied reserve must leave reserved_usage at the first 0.50; got %s", reserved)
}

// TestIntegration_UsageReservationRepository_ReserveReplay_HandleOwnsExistingRow
// reproduces the ledger's ordinary at-least-once retry end to end: reserve,
// retry the SAME reserve, then confirm with the handle the retry returned, and
// assert what the cap counted.
//
// The retry builds a SECOND model.NewReservation for the same 4-tuple, which is
// exactly what the reservation service does per call — a FRESH row id. Before the
// fix ReserveWithTx left that fresh id untouched on the replay branch, so the
// caller held a handle matching zero rows: the confirm failed with
// ErrReservationNotFound, the ledger swallowed it at Warn, and a spend that
// actually committed was never counted against the customer's cap.
func TestIntegration_UsageReservationRepository_ReserveReplay_HandleOwnsExistingRow(t *testing.T) {
	testutil.SetupTestTracing(t)

	db := testutil.SetupIntegrationDB(t)
	repo := newReservationRepoIntegration(db)

	limitID := createTestLimit(t, db, 8531)
	t.Cleanup(func() { cleanupTestLimit(t, db, limitID) })

	scopeKey := "acct:8531-" + testutil.MustDeterministicUUID(8541).String()[:8]
	periodKey := "2026-06"

	ctx := context.Background()
	now := testutil.FixedTime()
	txID := testutil.MustDeterministicUUID(8551)
	amount := decimal.NewFromInt(400)
	maxAmount := decimal.NewFromInt(1000)

	newReserve := func() *model.Reservation {
		res, err := model.NewReservation(limitID, txID, scopeKey, periodKey, amount, now.Add(5*time.Minute), now)
		require.NoError(t, err)

		return res
	}

	first := newReserve()

	var replayed bool

	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		var err error

		replayed, err = repo.ReserveWithTx(ctx, tx, first, maxAmount)

		return err
	}))
	assert.False(t, replayed, "a first insert is not a replay")

	// The retry: same 4-tuple, fresh row id, exactly as the service generates it.
	retry := newReserve()
	require.NotEqual(t, first.ID, retry.ID, "the retry must start with a fresh id, as the service generates it")

	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		var err error

		replayed, err = repo.ReserveWithTx(ctx, tx, retry, maxAmount)

		return err
	}))

	assert.True(t, replayed, "a reserve onto an existing RESERVED row must report the replay")
	assert.Equal(t, first.ID, retry.ID,
		"a replayed reserve must hand back the id of the row that owns the held capacity")

	current, reserved := readCounterDecimal(t, db, limitID, scopeKey, periodKey)
	assert.True(t, current.IsZero(), "replay must not touch current_usage; got %s", current)
	assert.True(t, amount.Equal(reserved), "replay must not double-hold; want 400 got %s", reserved)

	// The ledger commits and confirms with the handle the RETRY returned.
	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		_, err := repo.ConfirmWithTx(ctx, tx, retry.ID)
		return err
	}), "confirm with the retried reserve's handle must find the row")

	current, reserved = readCounterDecimal(t, db, limitID, scopeKey, periodKey)
	assert.True(t, amount.Equal(current),
		"the committed spend must be counted against the cap; want 400 got %s", current)
	assert.True(t, reserved.IsZero(), "confirm must drain the hold; got %s", reserved)
	assert.Equal(t, string(model.StatusConfirmed), readReservationStatus(t, db, first.ID))
}

// TestIntegration_UsageReservationRepository_ConfirmAfterExpiry_CountsTheSpendOnce
// covers the window the sweep opens: the sweep returns a hold's capacity at its
// stated expiry without waiting for the ledger, and then the ledger's confirm
// arrives for a transaction that really did commit.
//
// The spend happened, so the cap must reflect it exactly once. The expiry already
// returned the hold, so the late confirm adds the amount to counted spending and
// must NOT touch the hold bucket again.
func TestIntegration_UsageReservationRepository_ConfirmAfterExpiry_CountsTheSpendOnce(t *testing.T) {
	testutil.SetupTestTracing(t)

	db := testutil.SetupIntegrationDB(t)
	repo := newReservationRepoIntegration(db)

	limitID := createTestLimit(t, db, 8561)
	t.Cleanup(func() { cleanupTestLimit(t, db, limitID) })

	scopeKey := "acct:8561-" + testutil.MustDeterministicUUID(8571).String()[:8]
	periodKey := "2026-06"

	ctx := context.Background()
	now := testutil.FixedTime()
	amount := decimal.NewFromInt(400)

	res, err := model.NewReservation(
		limitID,
		testutil.MustDeterministicUUID(8581),
		scopeKey,
		periodKey,
		amount,
		now.Add(5*time.Minute),
		now,
	)
	require.NoError(t, err)

	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		_, err := repo.ReserveWithTx(ctx, tx, res, decimal.NewFromInt(1000))
		return err
	}))

	current, held := readCounterDecimal(t, db, limitID, scopeKey, periodKey)
	assert.True(t, current.IsZero(), "after reserve: counted spending must be 0, got %s", current)
	assert.True(t, amount.Equal(held), "after reserve: hold must be 400, got %s", held)

	// The sweep expires it. This is exactly what the reaper does per row.
	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		return repo.ReleaseWithTx(ctx, tx, res.ID, model.StatusExpired)
	}))

	current, held = readCounterDecimal(t, db, limitID, scopeKey, periodKey)
	assert.True(t, current.IsZero(), "after expiry: counted spending must still be 0, got %s", current)
	assert.True(t, held.IsZero(), "after expiry: the hold must be returned, got %s", held)
	assert.Equal(t, string(model.StatusExpired), readReservationStatus(t, db, res.ID))

	// The ledger committed and confirms late. The spend must land on the cap.
	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		_, err := repo.ConfirmWithTx(ctx, tx, res.ID)
		return err
	}), "a confirm for a transaction that committed must not be discarded because its hold expired")

	current, held = readCounterDecimal(t, db, limitID, scopeKey, periodKey)
	assert.True(t, amount.Equal(current),
		"after the late confirm: the committed spend must be counted; want 400 got %s", current)
	assert.True(t, held.IsZero(), "after the late confirm: the hold must stay returned, got %s", held)
	assert.Equal(t, string(model.StatusConfirmed), readReservationStatus(t, db, res.ID))

	// Retrying the late confirm must not count the spend twice.
	err = inRealTx(t, db, func(tx *sql.Tx) error {
		_, err := repo.ConfirmWithTx(ctx, tx, res.ID)
		return err
	})
	require.ErrorIs(t, err, constant.ErrReservationAlreadyTerminal)

	current, held = readCounterDecimal(t, db, limitID, scopeKey, periodKey)
	assert.True(t, amount.Equal(current), "a retried late confirm must not double-count; got %s", current)
	assert.True(t, held.IsZero())
}

// TestIntegration_UsageReservationRepository_ConfirmByTransactionAfterExpiry_CountsTheSpendOnce
// is the same window on the pending-transaction route, where the ledger holds only
// the transaction id at commit time.
func TestIntegration_UsageReservationRepository_ConfirmByTransactionAfterExpiry_CountsTheSpendOnce(t *testing.T) {
	testutil.SetupTestTracing(t)

	db := testutil.SetupIntegrationDB(t)
	repo := newReservationRepoIntegration(db)

	limitID := createTestLimit(t, db, 8562)
	t.Cleanup(func() { cleanupTestLimit(t, db, limitID) })

	scopeKey := "acct:8562-" + testutil.MustDeterministicUUID(8572).String()[:8]
	periodKey := "2026-06"

	ctx := context.Background()
	now := testutil.FixedTime()
	txID := testutil.MustDeterministicUUID(8582)
	amount := decimal.NewFromInt(250)

	res, err := model.NewReservation(limitID, txID, scopeKey, periodKey, amount, now.Add(5*time.Minute), now)
	require.NoError(t, err)

	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		_, err := repo.ReserveWithTx(ctx, tx, res, decimal.NewFromInt(1000))
		return err
	}))

	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		return repo.ReleaseWithTx(ctx, tx, res.ID, model.StatusExpired)
	}))

	current, held := readCounterDecimal(t, db, limitID, scopeKey, periodKey)
	require.True(t, current.IsZero())
	require.True(t, held.IsZero())

	var flipped []*model.Reservation

	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		var cErr error
		flipped, _, cErr = repo.ConfirmByTransactionWithTx(ctx, tx, txID)

		return cErr
	}))
	assert.Len(t, flipped, 1, "the pending commit must find the expired hold and settle it")

	current, held = readCounterDecimal(t, db, limitID, scopeKey, periodKey)
	assert.True(t, amount.Equal(current),
		"the committed pending spend must be counted; want 250 got %s", current)
	assert.True(t, held.IsZero())
	assert.Equal(t, string(model.StatusConfirmed), readReservationStatus(t, db, res.ID))

	// Re-running the commit confirm must be a no-op.
	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		var cErr error
		flipped, _, cErr = repo.ConfirmByTransactionWithTx(ctx, tx, txID)

		return cErr
	}))
	assert.Empty(t, flipped)

	current, _ = readCounterDecimal(t, db, limitID, scopeKey, periodKey)
	assert.True(t, amount.Equal(current), "a re-run must not double-count; got %s", current)
}

// TestIntegration_UsageReservationRepository_ReserveReplay_OntoSettledRow_Rejected
// proves that a reserve which lands on a row that already left RESERVED is not an
// idempotent success: it returns ErrReservationAlreadySettled, adopts no handle,
// leaves both counter buckets and the row status exactly as the settlement left
// them, and — for the released case — a later reserve by another transaction
// still sees the full remaining capacity.
func TestIntegration_UsageReservationRepository_ReserveReplay_OntoSettledRow_Rejected(t *testing.T) {
	testutil.SetupTestTracing(t)

	db := testutil.SetupIntegrationDB(t)
	repo := newReservationRepoIntegration(db)

	ctx := context.Background()
	now := testutil.FixedTime()
	amount := decimal.NewFromInt(400)
	maxAmount := decimal.NewFromInt(1000)

	cases := []struct {
		name        string
		limitSeed   int64
		txSeed      int64
		settle      func(tx *sql.Tx, reservationID uuid.UUID) error
		wantStatus  model.ReservationStatus
		wantCurrent decimal.Decimal
	}{
		{
			name:      "released",
			limitSeed: 8801,
			txSeed:    8811,
			settle: func(tx *sql.Tx, id uuid.UUID) error {
				return repo.ReleaseWithTx(ctx, tx, id, model.StatusReleased)
			},
			wantStatus:  model.StatusReleased,
			wantCurrent: decimal.Zero,
		},
		{
			name:      "expired",
			limitSeed: 8802,
			txSeed:    8812,
			settle: func(tx *sql.Tx, id uuid.UUID) error {
				return repo.ReleaseWithTx(ctx, tx, id, model.StatusExpired)
			},
			wantStatus:  model.StatusExpired,
			wantCurrent: decimal.Zero,
		},
		{
			name:      "confirmed",
			limitSeed: 8803,
			txSeed:    8813,
			settle: func(tx *sql.Tx, id uuid.UUID) error {
				_, err := repo.ConfirmWithTx(ctx, tx, id)
				return err
			},
			wantStatus:  model.StatusConfirmed,
			wantCurrent: amount,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			limitID := createTestLimit(t, db, tc.limitSeed)
			t.Cleanup(func() { cleanupTestLimit(t, db, limitID) })

			scopeKey := "acct:" + testutil.MustDeterministicUUID(tc.limitSeed).String()[:8]
			periodKey := "2026-06"
			txID := testutil.MustDeterministicUUID(tc.txSeed)

			first, err := model.NewReservation(limitID, txID, scopeKey, periodKey, amount, now.Add(5*time.Minute), now)
			require.NoError(t, err)

			require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
				_, err := repo.ReserveWithTx(ctx, tx, first, maxAmount)
				return err
			}))
			require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
				return tc.settle(tx, first.ID)
			}))

			current, reserved := readCounterDecimal(t, db, limitID, scopeKey, periodKey)
			require.True(t, tc.wantCurrent.Equal(current), "after settle: current_usage want %s got %s", tc.wantCurrent, current)
			require.True(t, reserved.IsZero(), "after settle: reserved_usage must be drained, got %s", reserved)

			// The replay: same 4-tuple, fresh row id, exactly as the service generates it.
			retry, err := model.NewReservation(limitID, txID, scopeKey, periodKey, amount, now.Add(5*time.Minute), now)
			require.NoError(t, err)

			retryID := retry.ID

			var replayed bool

			err = inRealTx(t, db, func(tx *sql.Tx) error {
				var err error

				replayed, err = repo.ReserveWithTx(ctx, tx, retry, maxAmount)

				return err
			})
			require.ErrorIs(t, err, constant.ErrReservationAlreadySettled,
				"a reserve onto a %s row must be rejected, not replayed", tc.wantStatus)
			assert.False(t, replayed, "a rejected reserve is not a replay")
			assert.Equal(t, retryID, retry.ID, "a rejected reserve must not adopt the settled row's handle")

			current, reserved = readCounterDecimal(t, db, limitID, scopeKey, periodKey)
			assert.True(t, tc.wantCurrent.Equal(current), "rejected reserve must not touch current_usage; want %s got %s", tc.wantCurrent, current)
			assert.True(t, reserved.IsZero(), "rejected reserve must not re-hold capacity; got %s", reserved)
			assert.Equal(t, string(tc.wantStatus), readReservationStatus(t, db, first.ID),
				"rejected reserve must leave the settled row as it was")
		})
	}
}

// TestIntegration_UsageReservationRepository_ReserveReplay_OntoReleased_LeavesCapacityFree
// is the released case seen from the next customer: tx A holds 400 and releases
// it, A's replay is rejected, and tx B can then take the whole 1000 cap — the
// rejection moved no counter and the released capacity really is free.
func TestIntegration_UsageReservationRepository_ReserveReplay_OntoReleased_LeavesCapacityFree(t *testing.T) {
	testutil.SetupTestTracing(t)

	db := testutil.SetupIntegrationDB(t)
	repo := newReservationRepoIntegration(db)

	limitID := createTestLimit(t, db, 8821)
	t.Cleanup(func() { cleanupTestLimit(t, db, limitID) })

	scopeKey := "acct:8821-" + testutil.MustDeterministicUUID(8822).String()[:8]
	periodKey := "2026-06"

	ctx := context.Background()
	now := testutil.FixedTime()
	maxAmount := decimal.NewFromInt(1000)
	txA := testutil.MustDeterministicUUID(8831)
	txB := testutil.MustDeterministicUUID(8832)

	resA, err := model.NewReservation(limitID, txA, scopeKey, periodKey, decimal.NewFromInt(400), now.Add(5*time.Minute), now)
	require.NoError(t, err)

	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		_, err := repo.ReserveWithTx(ctx, tx, resA, maxAmount)
		return err
	}))
	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		return repo.ReleaseWithTx(ctx, tx, resA.ID, model.StatusReleased)
	}))

	replayA, err := model.NewReservation(limitID, txA, scopeKey, periodKey, decimal.NewFromInt(400), now.Add(5*time.Minute), now)
	require.NoError(t, err)

	err = inRealTx(t, db, func(tx *sql.Tx) error {
		_, err := repo.ReserveWithTx(ctx, tx, replayA, maxAmount)
		return err
	})
	require.ErrorIs(t, err, constant.ErrReservationAlreadySettled)

	resB, err := model.NewReservation(limitID, txB, scopeKey, periodKey, decimal.NewFromInt(1000), now.Add(5*time.Minute), now)
	require.NoError(t, err)

	var replayed bool

	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		var err error

		replayed, err = repo.ReserveWithTx(ctx, tx, resB, maxAmount)

		return err
	}), "tx B must be able to take the full cap after A released and A's replay was rejected")
	assert.False(t, replayed)

	current, reserved := readCounterDecimal(t, db, limitID, scopeKey, periodKey)
	assert.True(t, current.IsZero(), "nothing is confirmed yet; got %s", current)
	assert.True(t, decimal.NewFromInt(1000).Equal(reserved), "B holds the whole cap; got %s", reserved)
	assert.Equal(t, string(model.StatusReleased), readReservationStatus(t, db, resA.ID))
	assert.Equal(t, string(model.StatusReserved), readReservationStatus(t, db, resB.ID))
}

// TestIntegration_UsageReservationRepository_ConfirmByTransaction_CountsReleased
// proves the by-transaction confirm learns, from its own locked read, how much of
// a transaction's spend will never be counted: one RELEASED and one CONFIRMED row
// report a released count of 1 and settle nothing, and a transaction with no rows
// reports 0.
func TestIntegration_UsageReservationRepository_ConfirmByTransaction_CountsReleased(t *testing.T) {
	testutil.SetupTestTracing(t)

	db := testutil.SetupIntegrationDB(t)
	repo := newReservationRepoIntegration(db)

	limitA := createTestLimitNamed(t, db, 8841, "count-released-a")
	t.Cleanup(func() { cleanupTestLimit(t, db, limitA) })

	limitB := createTestLimitNamed(t, db, 8842, "count-released-b")
	t.Cleanup(func() { cleanupTestLimit(t, db, limitB) })

	scopeKey := "acct:8841-" + testutil.MustDeterministicUUID(8843).String()[:8]
	periodKey := "2026-06"

	ctx := context.Background()
	now := testutil.FixedTime()
	txID := testutil.MustDeterministicUUID(8851)
	maxAmount := decimal.NewFromInt(10000)

	resA, err := model.NewReservation(limitA, txID, scopeKey, periodKey, decimal.NewFromInt(100), now.Add(5*time.Minute), now)
	require.NoError(t, err)

	resB, err := model.NewReservation(limitB, txID, scopeKey, periodKey, decimal.NewFromInt(200), now.Add(5*time.Minute), now)
	require.NoError(t, err)

	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		if _, err := repo.ReserveWithTx(ctx, tx, resA, maxAmount); err != nil {
			return err
		}

		_, err := repo.ReserveWithTx(ctx, tx, resB, maxAmount)

		return err
	}))

	confirmByTransaction := func(id uuid.UUID) (flipped []*model.Reservation, released int) {
		t.Helper()

		require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
			var err error

			flipped, released, err = repo.ConfirmByTransactionWithTx(ctx, tx, id)

			return err
		}))

		return flipped, released
	}

	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		return repo.ReleaseWithTx(ctx, tx, resA.ID, model.StatusReleased)
	}))
	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		_, err := repo.ConfirmWithTx(ctx, tx, resB.ID)
		return err
	}))

	currentB, _ := readCounter(t, db, limitB, scopeKey, periodKey)

	flipped, released := confirmByTransaction(txID)
	assert.Empty(t, flipped, "neither a RELEASED nor a CONFIRMED row settles again")
	assert.Equal(t, 1, released, "one RELEASED and one CONFIRMED row count as 1")

	currentA, reservedA := readCounter(t, db, limitA, scopeKey, periodKey)
	assert.Equal(t, int64(0), currentA, "the released spend is never counted")
	assert.Equal(t, int64(0), reservedA)

	currentBAfter, _ := readCounter(t, db, limitB, scopeKey, periodKey)
	assert.Equal(t, currentB, currentBAfter, "the confirmed row does not double-move")

	flipped, released = confirmByTransaction(testutil.MustDeterministicUUID(8852))
	assert.Empty(t, flipped)
	assert.Equal(t, 0, released, "a transaction with no rows counts as 0")
}
