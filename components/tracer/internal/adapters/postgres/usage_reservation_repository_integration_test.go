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
		return repo.ReserveWithTx(ctx, tx, res, decimal.NewFromInt(10000))
	}))

	current, reserved := readCounter(t, db, limitID, scopeKey, periodKey)
	assert.Equal(t, int64(0), current, "reserve must not touch current_usage")
	assert.Equal(t, int64(400), reserved, "reserve must seed reserved_usage")

	// First confirm: moves 400 reserved -> current.
	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		return repo.ConfirmWithTx(ctx, tx, res.ID)
	}))

	current, reserved = readCounter(t, db, limitID, scopeKey, periodKey)
	assert.Equal(t, int64(400), current, "confirm must move amount into current_usage")
	assert.Equal(t, int64(0), reserved, "confirm must drain reserved_usage")
	assert.Equal(t, string(model.StatusConfirmed), readReservationStatus(t, db, res.ID))

	// Second confirm: idempotent — no double-move, counter unchanged.
	err = inRealTx(t, db, func(tx *sql.Tx) error {
		return repo.ConfirmWithTx(ctx, tx, res.ID)
	})
	require.ErrorIs(t, err, constant.ErrReservationAlreadyTerminal,
		"retried confirm against a terminal row must be an idempotent no-op")

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
		return repo.ReserveWithTx(ctx, tx, res, decimal.NewFromInt(20))
	}))

	current, reserved := readCounterDecimal(t, db, limitID, scopeKey, periodKey)
	assert.True(t, current.IsZero(), "reserve must not touch current_usage")
	assert.True(t, want.Equal(reserved), "reserve must hold the exact fraction, got %s", reserved)

	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		return repo.ConfirmWithTx(ctx, tx, res.ID)
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
		return repo.ReserveWithTx(ctx, tx, res, decimal.NewFromInt(10000))
	}))
	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		return repo.ReleaseWithTx(ctx, tx, res.ID, model.StatusReleased)
	}))

	current, reserved := readCounter(t, db, limitID, scopeKey, periodKey)
	assert.Equal(t, int64(0), current, "release must NOT credit current_usage")
	assert.Equal(t, int64(0), reserved, "release must drain reserved_usage")
	assert.Equal(t, string(model.StatusReleased), readReservationStatus(t, db, res.ID))

	// Confirm after release: terminal no-op, counter untouched.
	err = inRealTx(t, db, func(tx *sql.Tx) error {
		return repo.ConfirmWithTx(ctx, tx, res.ID)
	})
	require.ErrorIs(t, err, constant.ErrReservationAlreadyTerminal)

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
		if rErr := repo.ReserveWithTx(ctx, tx, resA, decimal.NewFromInt(10000)); rErr != nil {
			return rErr
		}

		return repo.ReserveWithTx(ctx, tx, resB, decimal.NewFromInt(10000))
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
		flipped, cErr = repo.ConfirmByTransactionWithTx(ctx, tx, txID)

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
		flipped, cErr = repo.ConfirmByTransactionWithTx(ctx, tx, txID)

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
		return repo.ReserveWithTx(ctx, tx, res, decimal.NewFromInt(10000))
	}))

	// Re-reserve the SAME row id and 4-tuple: ON CONFLICT DO NOTHING keeps a single
	// row.
	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		return repo.ReserveWithTx(ctx, tx, res, decimal.NewFromInt(10000))
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
		return repo.ReserveWithTx(ctx, tx, res, decimal.NewFromInt(20))
	}))

	current, reserved := readCounterDecimal(t, db, limitID, scopeKey, periodKey)
	assert.True(t, current.IsZero(), "reserve must not touch current_usage")
	assert.True(t, want.Equal(reserved),
		"sub-unitary reserve must hold the exact 0.99, not truncate to 0; got %s", reserved)

	// Confirm moves the exact sub-unitary fraction into current_usage.
	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		return repo.ConfirmWithTx(ctx, tx, res.ID)
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
		return repo.ReserveWithTx(ctx, tx, res1, cap075)
	}))

	current, reserved := readCounterDecimal(t, db, limitID, scopeKey, periodKey)
	assert.True(t, current.IsZero(), "reserve must not touch current_usage")
	assert.True(t, half.Equal(reserved), "first 0.50 must be held exactly; got %s", reserved)

	// Second 0.50 would push held usage to 1.00 > 0.75 — the guard denies it, and
	// inRealTx rolls the transaction back so no RESERVED row survives.
	err = inRealTx(t, db, func(tx *sql.Tx) error {
		return repo.ReserveWithTx(ctx, tx, res2, cap075)
	})
	require.ErrorIs(t, err, constant.ErrUsageCounterExceedsLimit,
		"0.50 + 0.50 = 1.00 over a 0.75 cap must be denied")

	// The denied reserve left the counter untouched at the first 0.50.
	current, reserved = readCounterDecimal(t, db, limitID, scopeKey, periodKey)
	assert.True(t, current.IsZero(), "denied reserve must not credit current_usage")
	assert.True(t, half.Equal(reserved),
		"denied reserve must leave reserved_usage at the first 0.50; got %s", reserved)
}
