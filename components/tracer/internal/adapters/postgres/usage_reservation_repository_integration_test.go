// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"errors"
	"sync"
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

var errForceOutcomeRollback = errors.New("force outcome rollback")

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
		INSERT INTO limits (id, name, limit_type, max_amount, currency, scopes, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, limitID, "Test Limit "+name, "DAILY", decimal.NewFromInt(10000), "USD", "[]", "ACTIVE")
	require.NoError(t, err, "Failed to create named test limit")

	return limitID
}

func readCounter(t *testing.T, db *sql.DB, limitID uuid.UUID, scopeKey, periodKey string) (current, reserved int64) {
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

func newV2Reservation(t *testing.T, limitID, transactionID uuid.UUID, scopeKey, periodKey string, amount int64, now time.Time) *model.Reservation {
	t.Helper()

	reservation, err := model.NewReservationWithDeliveryMode(
		limitID, transactionID, scopeKey, periodKey, amount,
		now.Add(-time.Hour), now.Add(-45*24*time.Hour), model.DeliveryModeLedgerOutcomeV2,
	)
	require.NoError(t, err)

	return reservation
}

func cleanupOutcomeTransaction(t *testing.T, db *sql.DB, transactionID uuid.UUID) {
	t.Helper()
	_, _ = db.Exec("DELETE FROM reservation_outcome_receipts WHERE transaction_id = $1", transactionID)
	_, _ = db.Exec("DELETE FROM usage_reservations WHERE transaction_id = $1", transactionID)
}

func TestIntegration_UsageReservationRepository_ApplyOutcomeV2_MovesAllAndReplays(t *testing.T) {
	testutil.SetupTestTracing(t)
	db := testutil.SetupIntegrationDB(t)
	repo := newReservationRepoIntegration(db)

	limitA := createTestLimitNamed(t, db, 9901, "outcome-v2-A")
	limitB := createTestLimitNamed(t, db, 9902, "outcome-v2-B")
	transactionID := testutil.MustDeterministicUUID(9903)
	outcomeID := testutil.MustDeterministicUUID(9904)
	now := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	scopeA, scopeB, period := "v2:9901", "v2:9902", "2026-08"
	t.Cleanup(func() {
		cleanupOutcomeTransaction(t, db, transactionID)
		cleanupTestLimit(t, db, limitA)
		cleanupTestLimit(t, db, limitB)
	})

	for _, reservation := range []*model.Reservation{
		newV2Reservation(t, limitA, transactionID, scopeA, period, 120, now),
		newV2Reservation(t, limitB, transactionID, scopeB, period, 230, now),
	} {
		require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
			_, _, err := repo.ReserveWithTx(t.Context(), tx, reservation, 10000, nil)
			return err
		}))
	}

	var receipt *model.ReservationOutcomeReceipt
	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		var err error
		receipt, _, _, err = repo.ApplyOutcomeWithTx(t.Context(), tx, transactionID, outcomeID, model.OutcomeCommitted, now)
		return err
	}))
	require.Equal(t, 2, receipt.ReservationCount)

	currentA, reservedA := readCounter(t, db, limitA, scopeA, period)
	currentB, reservedB := readCounter(t, db, limitB, scopeB, period)
	require.Equal(t, int64(120), currentA)
	require.Equal(t, int64(0), reservedA)
	require.Equal(t, int64(230), currentB)
	require.Equal(t, int64(0), reservedB)

	var replayed bool
	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		var err error
		receipt, _, replayed, err = repo.ApplyOutcomeWithTx(t.Context(), tx, transactionID, outcomeID, model.OutcomeCommitted, now.Add(time.Minute))
		return err
	}))
	require.True(t, replayed)
	require.Equal(t, 2, receipt.ReservationCount)

	err := inRealTx(t, db, func(tx *sql.Tx) error {
		_, _, _, applyErr := repo.ApplyOutcomeWithTx(t.Context(), tx, transactionID, outcomeID, model.OutcomeAborted, now)
		return applyErr
	})
	require.ErrorIs(t, err, constant.ErrReservationOutcomeConflict)

	currentA, reservedA = readCounter(t, db, limitA, scopeA, period)
	require.Equal(t, int64(120), currentA)
	require.Equal(t, int64(0), reservedA)
}

func TestIntegration_UsageReservationRepository_ApplyOutcomeV2_RollbackAndZeroLimits(t *testing.T) {
	testutil.SetupTestTracing(t)
	db := testutil.SetupIntegrationDB(t)
	repo := newReservationRepoIntegration(db)

	limitID := createTestLimitNamed(t, db, 9911, "outcome-rollback")
	transactionID := testutil.MustDeterministicUUID(9912)
	zeroTransactionID := testutil.MustDeterministicUUID(9913)
	outcomeID := testutil.MustDeterministicUUID(9914)
	zeroOutcomeID := testutil.MustDeterministicUUID(9915)
	now := time.Date(2026, 8, 18, 13, 0, 0, 0, time.UTC)
	scope, period := "v2:9911", "2026-08"
	t.Cleanup(func() {
		cleanupOutcomeTransaction(t, db, transactionID)
		cleanupOutcomeTransaction(t, db, zeroTransactionID)
		cleanupTestLimit(t, db, limitID)
	})

	reservation := newV2Reservation(t, limitID, transactionID, scope, period, 500, now)
	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		_, _, err := repo.ReserveWithTx(t.Context(), tx, reservation, 10000, nil)
		return err
	}))

	err := inRealTx(t, db, func(tx *sql.Tx) error {
		_, _, _, applyErr := repo.ApplyOutcomeWithTx(t.Context(), tx, transactionID, outcomeID, model.OutcomeCommitted, now)
		if applyErr != nil {
			return applyErr
		}
		return errForceOutcomeRollback
	})
	require.ErrorIs(t, err, errForceOutcomeRollback)
	require.Equal(t, string(model.StatusReserved), readReservationStatus(t, db, reservation.ID))
	current, reserved := readCounter(t, db, limitID, scope, period)
	require.Equal(t, int64(0), current)
	require.Equal(t, int64(500), reserved)

	var receiptCount int
	require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM reservation_outcome_receipts WHERE transaction_id = $1", transactionID).Scan(&receiptCount))
	require.Zero(t, receiptCount)

	var zeroReceipt *model.ReservationOutcomeReceipt
	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		var applyErr error
		zeroReceipt, _, _, applyErr = repo.ApplyOutcomeWithTx(t.Context(), tx, zeroTransactionID, zeroOutcomeID, model.OutcomeAborted, now)
		return applyErr
	}))
	require.Zero(t, zeroReceipt.ReservationCount)
}

func TestIntegration_UsageReservationRepository_ApplyOutcomeV2_ConcurrentOppositesSerialize(t *testing.T) {
	testutil.SetupTestTracing(t)
	db := testutil.SetupIntegrationDB(t)
	repo := newReservationRepoIntegration(db)

	limitID := createTestLimitNamed(t, db, 9921, "outcome-race")
	transactionID := testutil.MustDeterministicUUID(9922)
	committedID := testutil.MustDeterministicUUID(9923)
	abortedID := testutil.MustDeterministicUUID(9924)
	now := time.Date(2026, 8, 18, 14, 0, 0, 0, time.UTC)
	scope, period := "v2:9921", "2026-08"
	t.Cleanup(func() {
		cleanupOutcomeTransaction(t, db, transactionID)
		cleanupTestLimit(t, db, limitID)
	})

	reservation := newV2Reservation(t, limitID, transactionID, scope, period, 75, now)
	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		_, _, err := repo.ReserveWithTx(t.Context(), tx, reservation, 10000, nil)
		return err
	}))

	type result struct {
		outcome model.ReservationOutcome
		err     error
	}
	results := make(chan result, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for _, tc := range []struct {
		id      uuid.UUID
		outcome model.ReservationOutcome
	}{{committedID, model.OutcomeCommitted}, {abortedID, model.OutcomeAborted}} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			err := inRealTx(t, db, func(tx *sql.Tx) error {
				_, _, _, applyErr := repo.ApplyOutcomeWithTx(t.Context(), tx, transactionID, tc.id, tc.outcome, now)
				return applyErr
			})
			results <- result{outcome: tc.outcome, err: err}
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	var success, conflicts int
	for got := range results {
		if got.err == nil {
			success++
		} else if errors.Is(got.err, constant.ErrReservationOutcomeConflict) {
			conflicts++
		} else {
			t.Fatalf("unexpected concurrent outcome error for %s: %v", got.outcome, got.err)
		}
	}
	require.Equal(t, 1, success)
	require.Equal(t, 1, conflicts)

	var outcome string
	require.NoError(t, db.QueryRow("SELECT outcome FROM reservation_outcome_receipts WHERE transaction_id = $1", transactionID).Scan(&outcome))
	status := readReservationStatus(t, db, reservation.ID)
	if outcome == string(model.OutcomeCommitted) {
		require.Equal(t, string(model.StatusConfirmed), status)
	} else {
		require.Equal(t, string(model.StatusReleased), status)
	}
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

	res, err := model.NewReservation(
		limitID,
		testutil.MustDeterministicUUID(8521), // transactionID
		scopeKey,
		periodKey,
		400,
		time.Now().UTC().Add(5*time.Minute),
		time.Now().UTC(),
	)
	require.NoError(t, err)

	// Reserve: seeds reserved_usage = 400, current_usage = 0.
	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		_, _, reserveErr := repo.ReserveWithTx(ctx, tx, res, 10000, nil)
		return reserveErr
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

	res, err := model.NewReservation(
		limitID,
		testutil.MustDeterministicUUID(8522),
		scopeKey,
		periodKey,
		250,
		time.Now().UTC().Add(5*time.Minute),
		time.Now().UTC(),
	)
	require.NoError(t, err)

	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		_, _, reserveErr := repo.ReserveWithTx(ctx, tx, res, 10000, nil)
		return reserveErr
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

	// Two reservations under ONE transaction, on two different limits.
	resA, err := model.NewReservation(limitA, txID, scopeA, periodKey, 400,
		time.Now().UTC().Add(5*time.Minute), time.Now().UTC())
	require.NoError(t, err)

	resB, err := model.NewReservation(limitB, txID, scopeB, periodKey, 250,
		time.Now().UTC().Add(5*time.Minute), time.Now().UTC())
	require.NoError(t, err)

	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		if _, _, rErr := repo.ReserveWithTx(ctx, tx, resA, 10000, nil); rErr != nil {
			return rErr
		}

		_, _, rErr := repo.ReserveWithTx(ctx, tx, resB, 10000, nil)
		return rErr
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
// reserve for the same 4-tuple returns the persisted id and does not duplicate
// either the reservation row or the held capacity.
func TestIntegration_UsageReservationRepository_Reserve_RowIdempotent(t *testing.T) {
	testutil.SetupTestTracing(t)

	db := testutil.SetupIntegrationDB(t)
	repo := newReservationRepoIntegration(db)

	limitID := createTestLimit(t, db, 8503)
	t.Cleanup(func() { cleanupTestLimit(t, db, limitID) })

	scopeKey := "acct:8503-" + testutil.MustDeterministicUUID(8513).String()[:8]
	periodKey := "2026-06"

	ctx := context.Background()

	res, err := model.NewReservation(
		limitID,
		testutil.MustDeterministicUUID(8523),
		scopeKey,
		periodKey,
		100,
		time.Now().UTC().Add(5*time.Minute),
		time.Now().UTC(),
	)
	require.NoError(t, err)

	var firstID uuid.UUID
	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		var created bool
		var reserveErr error
		firstID, created, reserveErr = repo.ReserveWithTx(ctx, tx, res, 10000, nil)
		require.True(t, created)
		return reserveErr
	}))

	retry, err := model.NewReservation(
		limitID,
		res.TransactionID,
		scopeKey,
		periodKey,
		100,
		time.Now().UTC().Add(5*time.Minute),
		time.Now().UTC(),
	)
	require.NoError(t, err)
	require.NotEqual(t, res.ID, retry.ID)

	var retryID uuid.UUID
	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		var created bool
		var reserveErr error
		retryID, created, reserveErr = repo.ReserveWithTx(ctx, tx, retry, 10000, nil)
		require.False(t, created)
		return reserveErr
	}))
	assert.Equal(t, firstID, retryID, "retry must return the persisted reservation id")

	conflicting, err := model.NewReservation(
		limitID,
		res.TransactionID,
		scopeKey,
		periodKey,
		101,
		time.Now().UTC().Add(5*time.Minute),
		time.Now().UTC(),
	)
	require.NoError(t, err)

	err = inRealTx(t, db, func(tx *sql.Tx) error {
		_, _, reserveErr := repo.ReserveWithTx(ctx, tx, conflicting, 10000, nil)
		return reserveErr
	})
	require.ErrorIs(t, err, constant.ErrIdempotencyKey, "same tuple with a different amount must fail closed")

	var rowCount int

	err = db.QueryRow(
		"SELECT COUNT(*) FROM usage_reservations WHERE transaction_id = $1 AND limit_id = $2 AND scope_key = $3 AND period_key = $4",
		res.TransactionID, limitID, scopeKey, periodKey,
	).Scan(&rowCount)
	require.NoError(t, err)
	assert.Equal(t, 1, rowCount, "retried reserve must not duplicate the reservation row")

	current, reserved := readCounter(t, db, limitID, scopeKey, periodKey)
	assert.Equal(t, int64(0), current)
	assert.Equal(t, int64(100), reserved, "retried reserve must not hold capacity twice")

	require.NoError(t, inRealTx(t, db, func(tx *sql.Tx) error {
		return repo.ConfirmWithTx(ctx, tx, firstID)
	}))

	err = inRealTx(t, db, func(tx *sql.Tx) error {
		_, _, reserveErr := repo.ReserveWithTx(ctx, tx, retry, 10000, nil)
		return reserveErr
	})
	require.ErrorIs(t, err, constant.ErrReservationAlreadyTerminal, "a terminal row is not an active idempotent handle")

	current, reserved = readCounter(t, db, limitID, scopeKey, periodKey)
	assert.Equal(t, int64(100), current)
	assert.Equal(t, int64(0), reserved)
}
