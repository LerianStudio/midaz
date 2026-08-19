// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	pgdbMocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	servicesMocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/services/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

type reservationDeps struct {
	conn        *pgdbMocks.MockTxBeginner
	tx          *pgdbMocks.MockTx
	resolver    *servicesMocks.MockLimitResolver
	repo        *servicesMocks.MockReservationRepository
	auditWriter *servicesMocks.MockReservationAuditWriter
	clock       clock.Clock
}

func newReservationServiceDeps(t *testing.T) (*ReservationService, *reservationDeps) {
	t.Helper()

	testutil.SetupTestTracing(t)

	ctrl := gomock.NewController(t)

	deps := &reservationDeps{
		conn:        pgdbMocks.NewMockTxBeginner(ctrl),
		tx:          pgdbMocks.NewMockTx(ctrl),
		resolver:    servicesMocks.NewMockLimitResolver(ctrl),
		repo:        servicesMocks.NewMockReservationRepository(ctrl),
		auditWriter: servicesMocks.NewMockReservationAuditWriter(ctrl),
		clock:       testutil.NewMockClock(testutil.FixedTime()),
	}

	svc, err := NewReservationService(deps.conn, deps.resolver, deps.repo, deps.auditWriter, deps.clock)
	require.NoError(t, err)

	return svc, deps
}

// expectTxCommit wires the mock TxBeginner to hand out the mock Tx and expects a
// single Commit (the success path). The mocked repo/audit ignore the tx handle, so
// the test only verifies the tx lifecycle, not SQL.
func (d *reservationDeps) expectTxCommit() {
	d.conn.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(d.tx, nil).Times(1)
	d.tx.EXPECT().Commit().Return(nil).Times(1)
}

// expectTxRollback wires the success-less path: BeginTx then Rollback (no Commit).
func (d *reservationDeps) expectTxRollback() {
	d.conn.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(d.tx, nil).Times(1)
	d.tx.EXPECT().Rollback().Return(nil).Times(1)
}

func TestNewReservationService_NilDeps(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	conn := pgdbMocks.NewMockTxBeginner(ctrl)
	resolver := servicesMocks.NewMockLimitResolver(ctrl)
	repo := servicesMocks.NewMockReservationRepository(ctrl)
	audit := servicesMocks.NewMockReservationAuditWriter(ctrl)

	_, err := NewReservationService(nil, resolver, repo, audit, nil)
	require.ErrorIs(t, err, ErrNilReservationConn)

	_, err = NewReservationService(conn, nil, repo, audit, nil)
	require.ErrorIs(t, err, ErrNilLimitResolver)

	_, err = NewReservationService(conn, resolver, nil, audit, nil)
	require.ErrorIs(t, err, ErrNilReservationRepo)

	_, err = NewReservationService(conn, resolver, repo, nil, nil)
	require.ErrorIs(t, err, ErrNilReservationAuditWriter)
}

func testCheckLimitsInput(t *testing.T) *model.CheckLimitsInput {
	t.Helper()

	input, err := model.NewCheckLimitsInput(
		decimal.NewFromInt(400),
		"USD",
		testutil.MustDeterministicUUID(7001),
		nil, nil, nil, nil, nil,
		testutil.FixedTime(),
	)
	require.NoError(t, err)

	return input
}

// decEq matches a decimal.Decimal argument by value (decimal.Equal, never ==).
func decEq(want decimal.Decimal) gomock.Matcher {
	return gomock.Cond(func(got decimal.Decimal) bool { return got.Equal(want) })
}

func twoSpecs() []query.ReservationSpec {
	return []query.ReservationSpec{
		{
			LimitID:   testutil.MustDeterministicUUID(7101),
			ScopeKey:  "acct:7001",
			PeriodKey: "2026-06",
			Amount:    decimal.NewFromInt(400),
			MaxAmount: decimal.NewFromInt(10000),
		},
		{
			LimitID:   testutil.MustDeterministicUUID(7102),
			ScopeKey:  "global",
			PeriodKey: "2026-06-05",
			Amount:    decimal.NewFromInt(400),
			MaxAmount: decimal.NewFromInt(5000),
		},
	}
}

func TestReservationService_Reserve(t *testing.T) {
	txID := testutil.MustDeterministicUUID(7050)

	t.Run("Resolves limits ONCE and reserves one row per applicable limit", func(t *testing.T) {
		svc, deps := newReservationServiceDeps(t)

		input := testCheckLimitsInput(t)

		// Single resolution call (R38 / resolve-once invariant).
		deps.resolver.EXPECT().
			ResolveReservations(gomock.Any(), input).
			Return(twoSpecs(), false, nil).
			Times(1)

		deps.expectTxCommit()

		// One reserve + one audit per applicable limit.
		deps.repo.EXPECT().
			ReserveWithTx(gomock.Any(), deps.tx, gomock.AssignableToTypeOf(&model.Reservation{}), decEq(decimal.NewFromInt(10000)), gomock.Any()).
			Return(testutil.MustDeterministicUUID(7061), true, nil).
			Times(1)
		deps.repo.EXPECT().
			ReserveWithTx(gomock.Any(), deps.tx, gomock.AssignableToTypeOf(&model.Reservation{}), decEq(decimal.NewFromInt(5000)), gomock.Any()).
			Return(testutil.MustDeterministicUUID(7062), true, nil).
			Times(1)
		deps.auditWriter.EXPECT().
			RecordReservationEventWithTx(gomock.Any(), deps.tx, model.AuditEventReservationReserved, model.AuditActionReserve, gomock.Any(), gomock.Any()).
			Return(nil).
			Times(2)

		result, err := svc.Reserve(context.Background(), txID, input, false)
		require.NoError(t, err)
		require.False(t, result.Denied)
		assert.Len(t, result.ReservationIDs, 2)
	})

	t.Run("Idempotent retry returns persisted id without a second audit", func(t *testing.T) {
		svc, deps := newReservationServiceDeps(t)
		input := testCheckLimitsInput(t)
		persistedID := testutil.MustDeterministicUUID(7063)

		deps.resolver.EXPECT().
			ResolveReservations(gomock.Any(), input).
			Return(oneSpec(), false, nil).
			Times(1)
		deps.expectTxCommit()
		deps.repo.EXPECT().
			ReserveWithTx(gomock.Any(), deps.tx, gomock.Any(), decEq(decimal.NewFromInt(10000)), gomock.Any()).
			Return(persistedID, false, nil).
			Times(1)

		result, err := svc.Reserve(context.Background(), txID, input, false)
		require.NoError(t, err)
		assert.Equal(t, []uuid.UUID{persistedID}, result.ReservationIDs)
	})

	t.Run("Fractional spec amount reaches the reservation row intact", func(t *testing.T) {
		svc, deps := newReservationServiceDeps(t)

		input := testCheckLimitsInput(t)

		fractional := decimal.RequireFromString("10.50")
		spec := []query.ReservationSpec{
			{
				LimitID:   testutil.MustDeterministicUUID(7101),
				ScopeKey:  "acct:7001",
				PeriodKey: "2026-06",
				Amount:    fractional,
				MaxAmount: decimal.NewFromInt(20),
			},
		}

		deps.resolver.EXPECT().
			ResolveReservations(gomock.Any(), input).
			Return(spec, false, nil).
			Times(1)

		deps.expectTxCommit()

		var captured decimal.Decimal
		deps.repo.EXPECT().
			ReserveWithTx(gomock.Any(), deps.tx, gomock.Any(), decEq(decimal.NewFromInt(20)), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ any, r *model.Reservation, _ decimal.Decimal, _ *time.Time) (uuid.UUID, bool, error) {
				captured = r.Amount

				return r.ID, true, nil
			}).
			Times(1)
		deps.auditWriter.EXPECT().
			RecordReservationEventWithTx(gomock.Any(), deps.tx, model.AuditEventReservationReserved, model.AuditActionReserve, gomock.Any(), gomock.Any()).
			Return(nil).
			Times(1)

		_, err := svc.Reserve(context.Background(), txID, input, false)
		require.NoError(t, err)

		// The pre-fix int64 path would have persisted 10 here.
		assert.True(t, fractional.Equal(captured), "expected 10.50 held, got %s", captured)
	})

	t.Run("Denied by resolver (per-transaction cap) returns Denied without a tx", func(t *testing.T) {
		svc, deps := newReservationServiceDeps(t)

		input := testCheckLimitsInput(t)

		deps.resolver.EXPECT().
			ResolveReservations(gomock.Any(), input).
			Return(nil, true, nil).
			Times(1)
		// No BeginTx expected — denial short-circuits before the transaction.

		result, err := svc.Reserve(context.Background(), txID, input, false)
		require.NoError(t, err)
		assert.True(t, result.Denied)
		assert.Empty(t, result.ReservationIDs)
	})

	t.Run("Reserve guard denies mid-tx -> rollback, Denied decision", func(t *testing.T) {
		svc, deps := newReservationServiceDeps(t)

		input := testCheckLimitsInput(t)

		deps.resolver.EXPECT().
			ResolveReservations(gomock.Any(), input).
			Return(twoSpecs(), false, nil).
			Times(1)

		deps.expectTxRollback()

		// First reserve trips the over-limit guard; the whole tx rolls back and no
		// further reserve/audit runs.
		deps.repo.EXPECT().
			ReserveWithTx(gomock.Any(), deps.tx, gomock.Any(), decEq(decimal.NewFromInt(10000)), gomock.Any()).
			Return(uuid.Nil, false, constant.ErrUsageCounterExceedsLimit).
			Times(1)

		result, err := svc.Reserve(context.Background(), txID, input, false)
		require.NoError(t, err)
		assert.True(t, result.Denied, "guard-denied reserve must surface the limit-exceeded decision")
		assert.Empty(t, result.ReservationIDs)
	})

	for _, tc := range []struct {
		name    string
		repoErr error
	}{
		{name: "idempotency conflict fails closed", repoErr: constant.ErrIdempotencyKey},
		{name: "terminal retry is not an active handle", repoErr: constant.ErrReservationAlreadyTerminal},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, deps := newReservationServiceDeps(t)
			input := testCheckLimitsInput(t)

			deps.resolver.EXPECT().ResolveReservations(gomock.Any(), input).Return(oneSpec(), false, nil).Times(1)
			deps.expectTxRollback()
			deps.repo.EXPECT().
				ReserveWithTx(gomock.Any(), deps.tx, gomock.Any(), decEq(decimal.NewFromInt(10000)), gomock.Any()).
				Return(uuid.Nil, false, tc.repoErr).
				Times(1)

			result, err := svc.Reserve(context.Background(), txID, input, false)
			require.ErrorIs(t, err, tc.repoErr)
			assert.Nil(t, result)
		})
	}

	t.Run("No applicable limits -> allow with empty handle", func(t *testing.T) {
		svc, deps := newReservationServiceDeps(t)

		input := testCheckLimitsInput(t)

		deps.resolver.EXPECT().
			ResolveReservations(gomock.Any(), input).
			Return(nil, false, nil).
			Times(1)

		result, err := svc.Reserve(context.Background(), txID, input, false)
		require.NoError(t, err)
		assert.False(t, result.Denied)
		assert.Empty(t, result.ReservationIDs)
	})

	t.Run("Missing transaction id is rejected", func(t *testing.T) {
		svc, _ := newReservationServiceDeps(t)

		_, err := svc.Reserve(context.Background(), uuid.Nil, testCheckLimitsInput(t), false)
		require.ErrorIs(t, err, ErrNilReservationTransationID)
	})

	t.Run("longLived=false sets the short direct TTL on the reservation", func(t *testing.T) {
		svc, deps := newReservationServiceDeps(t)

		input := testCheckLimitsInput(t)
		now := testutil.FixedTime()

		deps.resolver.EXPECT().
			ResolveReservations(gomock.Any(), input).
			Return(oneSpec(), false, nil).
			Times(1)

		deps.expectTxCommit()

		var capturedReservationExpiry time.Time
		var capturedCounterExpiry *time.Time
		deps.repo.EXPECT().
			ReserveWithTx(gomock.Any(), deps.tx, gomock.Any(), decEq(decimal.NewFromInt(10000)), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ any, r *model.Reservation, _ decimal.Decimal, counterExpiry *time.Time) (uuid.UUID, bool, error) {
				capturedReservationExpiry = r.ReservationExpiresAt
				capturedCounterExpiry = counterExpiry

				return r.ID, true, nil
			}).
			Times(1)
		deps.auditWriter.EXPECT().
			RecordReservationEventWithTx(gomock.Any(), deps.tx, model.AuditEventReservationReserved, model.AuditActionReserve, gomock.Any(), gomock.Any()).
			Return(nil).
			Times(1)

		_, err := svc.Reserve(context.Background(), txID, input, false)
		require.NoError(t, err)

		// Direct transactions use the fixed short TTL, NOT the long-lived knob.
		assert.Equal(t, now.UTC().Add(reservationTTL), capturedReservationExpiry)
		require.NotNil(t, capturedCounterExpiry)
		assert.Equal(t, *testCounterExpiry(), *capturedCounterExpiry)
		assert.NotEqual(t, capturedReservationExpiry, *capturedCounterExpiry,
			"counter retention must be independent from the reservation abandonment TTL")
	})

	t.Run("longLived=true sets the configured long-lived TTL on the reservation", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		testutil.SetupTestTracing(t)

		const longLivedTTL = 48 * time.Hour

		conn := pgdbMocks.NewMockTxBeginner(ctrl)
		tx := pgdbMocks.NewMockTx(ctrl)
		resolver := servicesMocks.NewMockLimitResolver(ctrl)
		repo := servicesMocks.NewMockReservationRepository(ctrl)
		auditWriter := servicesMocks.NewMockReservationAuditWriter(ctrl)
		clk := testutil.NewMockClock(testutil.FixedTime())

		svc, err := NewReservationServiceWithLongLivedTTL(conn, resolver, repo, auditWriter, clk, longLivedTTL)
		require.NoError(t, err)

		input := testCheckLimitsInput(t)
		now := testutil.FixedTime()

		resolver.EXPECT().
			ResolveReservations(gomock.Any(), input).
			Return(oneSpec(), false, nil).
			Times(1)

		conn.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(tx, nil).Times(1)
		tx.EXPECT().Commit().Return(nil).Times(1)

		var captured time.Time
		repo.EXPECT().
			ReserveWithTx(gomock.Any(), tx, gomock.Any(), decEq(decimal.NewFromInt(10000)), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ any, r *model.Reservation, _ decimal.Decimal, _ *time.Time) (uuid.UUID, bool, error) {
				captured = r.ReservationExpiresAt

				return r.ID, true, nil
			}).
			Times(1)
		auditWriter.EXPECT().
			RecordReservationEventWithTx(gomock.Any(), tx, model.AuditEventReservationReserved, model.AuditActionReserve, gomock.Any(), gomock.Any()).
			Return(nil).
			Times(1)

		_, err = svc.Reserve(context.Background(), txID, input, true)
		require.NoError(t, err)

		// PENDING reservations expire far out (the configured long-lived TTL), well
		// beyond the short direct TTL the reaper sweeps on (R18).
		assert.Equal(t, now.UTC().Add(longLivedTTL), captured)
		assert.True(t, captured.After(now.UTC().Add(reservationTTL)), "long-lived TTL must outlive the direct TTL")
	})

	t.Run("longLived=true with default service TTL uses the 30-day ceiling", func(t *testing.T) {
		svc, deps := newReservationServiceDeps(t)

		input := testCheckLimitsInput(t)
		now := testutil.FixedTime()

		deps.resolver.EXPECT().
			ResolveReservations(gomock.Any(), input).
			Return(oneSpec(), false, nil).
			Times(1)

		deps.expectTxCommit()

		var captured time.Time
		deps.repo.EXPECT().
			ReserveWithTx(gomock.Any(), deps.tx, gomock.Any(), decEq(decimal.NewFromInt(10000)), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ any, r *model.Reservation, _ decimal.Decimal, _ *time.Time) (uuid.UUID, bool, error) {
				captured = r.ReservationExpiresAt

				return r.ID, true, nil
			}).
			Times(1)
		deps.auditWriter.EXPECT().
			RecordReservationEventWithTx(gomock.Any(), deps.tx, model.AuditEventReservationReserved, model.AuditActionReserve, gomock.Any(), gomock.Any()).
			Return(nil).
			Times(1)

		_, err := svc.Reserve(context.Background(), txID, input, true)
		require.NoError(t, err)

		// newReservationServiceDeps passes longLivedTTL=0, so the service falls back
		// to defaultLongLivedReservationTTL (30 days).
		assert.Equal(t, now.UTC().Add(defaultLongLivedReservationTTL), captured)
	})
}

// oneSpec returns a single counter-backed reservation spec with MaxAmount 10000,
// used by the TTL-assertion subtests so exactly one ReserveWithTx is captured.
func oneSpec() []query.ReservationSpec {
	return []query.ReservationSpec{
		{
			LimitID:          testutil.MustDeterministicUUID(7101),
			ScopeKey:         "acct:7001",
			PeriodKey:        "2026-06",
			Amount:           decimal.NewFromInt(400),
			MaxAmount:        decimal.NewFromInt(10000),
			CounterExpiresAt: testCounterExpiry(),
		},
	}
}

func testCounterExpiry() *time.Time {
	expiresAt := testutil.FixedTime().AddDate(0, 0, 90)

	return &expiresAt
}

func TestReservationService_Confirm(t *testing.T) {
	resID := testutil.MustDeterministicUUID(7200)

	t.Run("Success - counter move + row flip + audit in one tx", func(t *testing.T) {
		svc, deps := newReservationServiceDeps(t)

		deps.expectTxCommit()

		deps.repo.EXPECT().
			ConfirmWithTx(gomock.Any(), deps.tx, resID).
			Return(nil).
			Times(1)
		deps.auditWriter.EXPECT().
			RecordReservationEventWithTx(gomock.Any(), deps.tx, model.AuditEventReservationConfirmed, model.AuditActionConfirm, resID, gomock.Any()).
			Return(nil).
			Times(1)

		require.NoError(t, svc.Confirm(context.Background(), resID))
	})

	t.Run("Idempotent double-confirm - terminal row maps to success, NO second counter move", func(t *testing.T) {
		svc, deps := newReservationServiceDeps(t)

		// Repo reports already-terminal; the service rolls back and returns nil
		// WITHOUT recording a second audit event or moving the counter again.
		deps.expectTxRollback()
		deps.repo.EXPECT().
			ConfirmWithTx(gomock.Any(), deps.tx, resID).
			Return(constant.ErrReservationAlreadyTerminal).
			Times(1)
		// No audit call expected on the idempotent path.

		require.NoError(t, svc.Confirm(context.Background(), resID),
			"retried confirm against a terminal reservation must be an idempotent success")
	})

	t.Run("Not found propagates", func(t *testing.T) {
		svc, deps := newReservationServiceDeps(t)

		deps.expectTxRollback()
		deps.repo.EXPECT().
			ConfirmWithTx(gomock.Any(), deps.tx, resID).
			Return(constant.ErrReservationNotFound).
			Times(1)

		err := svc.Confirm(context.Background(), resID)
		require.ErrorIs(t, err, constant.ErrReservationNotFound)
	})
}

func TestReservationService_Release(t *testing.T) {
	resID := testutil.MustDeterministicUUID(7300)

	t.Run("Success - RELEASED flip + audit in one tx", func(t *testing.T) {
		svc, deps := newReservationServiceDeps(t)

		deps.expectTxCommit()

		deps.repo.EXPECT().
			ReleaseWithTx(gomock.Any(), deps.tx, resID, model.StatusReleased).
			Return(nil).
			Times(1)
		deps.auditWriter.EXPECT().
			RecordReservationEventWithTx(gomock.Any(), deps.tx, model.AuditEventReservationReleased, model.AuditActionRelease, resID, gomock.Any()).
			Return(nil).
			Times(1)

		require.NoError(t, svc.Release(context.Background(), resID))
	})

	t.Run("Idempotent double-release - terminal row maps to success", func(t *testing.T) {
		svc, deps := newReservationServiceDeps(t)

		deps.expectTxRollback()
		deps.repo.EXPECT().
			ReleaseWithTx(gomock.Any(), deps.tx, resID, model.StatusReleased).
			Return(constant.ErrReservationAlreadyTerminal).
			Times(1)

		require.NoError(t, svc.Release(context.Background(), resID))
	})
}

func twoReservations(txID uuid.UUID) []*model.Reservation {
	res1, _ := model.NewReservation(
		testutil.MustDeterministicUUID(7401), txID, "acct:7401", "2026-06", decimal.NewFromInt(400),
		testutil.FixedTime().Add(5*time.Minute), testutil.FixedTime(),
	)
	res2, _ := model.NewReservation(
		testutil.MustDeterministicUUID(7402), txID, "global", "2026-06-05", decimal.NewFromInt(400),
		testutil.FixedTime().Add(5*time.Minute), testutil.FixedTime(),
	)

	return []*model.Reservation{res1, res2}
}

func TestReservationService_ConfirmByTransaction(t *testing.T) {
	txID := testutil.MustDeterministicUUID(7400)

	t.Run("Flips ALL reserved rows in one tx, audits each", func(t *testing.T) {
		svc, deps := newReservationServiceDeps(t)

		reservations := twoReservations(txID)

		deps.expectTxCommit()

		deps.repo.EXPECT().
			ConfirmByTransactionWithTx(gomock.Any(), deps.tx, txID).
			Return(reservations, nil).
			Times(1)
		// One audit row per flipped reservation, same tx.
		deps.auditWriter.EXPECT().
			RecordReservationEventWithTx(gomock.Any(), deps.tx, model.AuditEventReservationConfirmed, model.AuditActionConfirm, gomock.Any(), gomock.Any()).
			Return(nil).
			Times(2)

		flipped, err := svc.ConfirmByTransaction(context.Background(), txID)
		require.NoError(t, err)
		assert.Equal(t, 2, flipped, "every reserved row of the transaction is confirmed")
	})

	t.Run("No reserved rows is an idempotent no-op success (re-run), NO audit", func(t *testing.T) {
		svc, deps := newReservationServiceDeps(t)

		deps.expectTxCommit()

		deps.repo.EXPECT().
			ConfirmByTransactionWithTx(gomock.Any(), deps.tx, txID).
			Return(nil, nil).
			Times(1)
		// No audit call expected on the empty path.

		flipped, err := svc.ConfirmByTransaction(context.Background(), txID)
		require.NoError(t, err)
		assert.Equal(t, 0, flipped, "re-run over an already-confirmed transaction is a clean no-op")
	})

	t.Run("Missing transaction id is rejected before a tx", func(t *testing.T) {
		svc, _ := newReservationServiceDeps(t)

		_, err := svc.ConfirmByTransaction(context.Background(), uuid.Nil)
		require.ErrorIs(t, err, ErrNilReservationTransationID)
	})
}

func TestReservationService_ReleaseByTransaction(t *testing.T) {
	txID := testutil.MustDeterministicUUID(7500)

	t.Run("Releases ALL reserved rows in one tx, audits each", func(t *testing.T) {
		svc, deps := newReservationServiceDeps(t)

		reservations := twoReservations(txID)

		deps.expectTxCommit()

		deps.repo.EXPECT().
			ReleaseByTransactionWithTx(gomock.Any(), deps.tx, txID, model.StatusReleased).
			Return(reservations, nil).
			Times(1)
		deps.auditWriter.EXPECT().
			RecordReservationEventWithTx(gomock.Any(), deps.tx, model.AuditEventReservationReleased, model.AuditActionRelease, gomock.Any(), gomock.Any()).
			Return(nil).
			Times(2)

		flipped, err := svc.ReleaseByTransaction(context.Background(), txID)
		require.NoError(t, err)
		assert.Equal(t, 2, flipped)
	})

	t.Run("No reserved rows is an idempotent no-op success", func(t *testing.T) {
		svc, deps := newReservationServiceDeps(t)

		deps.expectTxCommit()

		deps.repo.EXPECT().
			ReleaseByTransactionWithTx(gomock.Any(), deps.tx, txID, model.StatusReleased).
			Return(nil, nil).
			Times(1)

		flipped, err := svc.ReleaseByTransaction(context.Background(), txID)
		require.NoError(t, err)
		assert.Equal(t, 0, flipped)
	})
}

func TestReservationService_ReserveLedgerOutcomeV2PersistsDeliveryMode(t *testing.T) {
	txID := testutil.MustDeterministicUUID(7600)
	svc, deps := newReservationServiceDeps(t)
	input := testCheckLimitsInput(t)

	deps.resolver.EXPECT().ResolveReservations(gomock.Any(), input).Return(oneSpec(), false, nil).Times(1)
	deps.expectTxCommit()
	deps.repo.EXPECT().
		ReserveWithTx(gomock.Any(), deps.tx, gomock.Any(), decEq(decimal.NewFromInt(10000)), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ any, reservation *model.Reservation, _ decimal.Decimal, _ *time.Time) (uuid.UUID, bool, error) {
			assert.Equal(t, model.DeliveryModeLedgerOutcomeV2, reservation.DeliveryMode)

			return reservation.ID, true, nil
		}).
		Times(1)
	deps.auditWriter.EXPECT().
		RecordReservationEventWithTx(gomock.Any(), deps.tx, model.AuditEventReservationReserved, model.AuditActionReserve, gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	result, err := svc.Reserve(context.Background(), txID, input, true, model.DeliveryModeLedgerOutcomeV2)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, model.DeliveryModeLedgerOutcomeV2, result.DeliveryMode)
}

func TestReservationService_ReserveEchoesNormalizedLegacyModeWithoutReservations(t *testing.T) {
	txID := testutil.MustDeterministicUUID(7650)
	svc, deps := newReservationServiceDeps(t)
	input := testCheckLimitsInput(t)

	deps.resolver.EXPECT().ResolveReservations(gomock.Any(), input).Return(nil, false, nil).Times(1)

	result, err := svc.Reserve(context.Background(), txID, input, false)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, model.DeliveryModeLegacy, result.DeliveryMode)
}

func TestReservationService_ApplyOutcome(t *testing.T) {
	txID := testutil.MustDeterministicUUID(7700)
	outcomeID := testutil.MustDeterministicUUID(7701)
	now := testutil.FixedTime()

	newReceipt := func(outcome model.ReservationOutcome, count int) *model.ReservationOutcomeReceipt {
		return &model.ReservationOutcomeReceipt{
			TransactionID:    txID,
			OutcomeID:        outcomeID,
			Outcome:          outcome,
			ReservationCount: count,
			AppliedAt:        now,
		}
	}

	t.Run("committed moves every reservation and audits in the receipt transaction", func(t *testing.T) {
		svc, deps := newReservationServiceDeps(t)
		reservations := twoReservations(txID)
		for _, reservation := range reservations {
			reservation.DeliveryMode = model.DeliveryModeLedgerOutcomeV2
		}

		deps.expectTxCommit()
		deps.repo.EXPECT().
			ApplyOutcomeWithTx(gomock.Any(), deps.tx, txID, outcomeID, model.OutcomeCommitted, now).
			Return(newReceipt(model.OutcomeCommitted, 2), reservations, false, nil).
			Times(1)
		deps.auditWriter.EXPECT().
			RecordReservationEventWithTx(gomock.Any(), deps.tx, model.AuditEventReservationConfirmed, model.AuditActionConfirm, gomock.Any(), gomock.Any()).
			Return(nil).
			Times(2)

		result, err := svc.ApplyOutcome(context.Background(), txID, outcomeID, model.OutcomeCommitted)
		require.NoError(t, err)
		assert.Equal(t, 2, result.ReservationCount)
		assert.False(t, result.Replayed)
		assert.Equal(t, model.OutcomeCommitted, result.Outcome)
	})

	t.Run("aborted releases every reservation and audits", func(t *testing.T) {
		svc, deps := newReservationServiceDeps(t)
		reservations := twoReservations(txID)

		deps.expectTxCommit()
		deps.repo.EXPECT().
			ApplyOutcomeWithTx(gomock.Any(), deps.tx, txID, outcomeID, model.OutcomeAborted, now).
			Return(newReceipt(model.OutcomeAborted, 2), reservations, false, nil).
			Times(1)
		deps.auditWriter.EXPECT().
			RecordReservationEventWithTx(gomock.Any(), deps.tx, model.AuditEventReservationReleased, model.AuditActionRelease, gomock.Any(), gomock.Any()).
			Return(nil).
			Times(2)

		result, err := svc.ApplyOutcome(context.Background(), txID, outcomeID, model.OutcomeAborted)
		require.NoError(t, err)
		assert.Equal(t, model.OutcomeAborted, result.Outcome)
		assert.Equal(t, 2, result.ReservationCount)
	})

	t.Run("exact replay returns persisted receipt without counters or audit", func(t *testing.T) {
		svc, deps := newReservationServiceDeps(t)
		receipt := newReceipt(model.OutcomeCommitted, 2)

		deps.expectTxCommit()
		deps.repo.EXPECT().
			ApplyOutcomeWithTx(gomock.Any(), deps.tx, txID, outcomeID, model.OutcomeCommitted, now).
			Return(receipt, nil, true, nil).
			Times(1)

		result, err := svc.ApplyOutcome(context.Background(), txID, outcomeID, model.OutcomeCommitted)
		require.NoError(t, err)
		assert.True(t, result.Replayed)
		assert.Equal(t, 2, result.ReservationCount)
	})

	t.Run("zero limits still commits a durable receipt", func(t *testing.T) {
		svc, deps := newReservationServiceDeps(t)

		deps.expectTxCommit()
		deps.repo.EXPECT().
			ApplyOutcomeWithTx(gomock.Any(), deps.tx, txID, outcomeID, model.OutcomeCommitted, now).
			Return(newReceipt(model.OutcomeCommitted, 0), nil, false, nil).
			Times(1)

		result, err := svc.ApplyOutcome(context.Background(), txID, outcomeID, model.OutcomeCommitted)
		require.NoError(t, err)
		assert.Zero(t, result.ReservationCount)
		assert.False(t, result.Replayed)
	})

	t.Run("opposite outcome conflicts and rolls back", func(t *testing.T) {
		svc, deps := newReservationServiceDeps(t)

		deps.expectTxRollback()
		deps.repo.EXPECT().
			ApplyOutcomeWithTx(gomock.Any(), deps.tx, txID, outcomeID, model.OutcomeAborted, now).
			Return(nil, nil, false, constant.ErrReservationOutcomeConflict).
			Times(1)

		result, err := svc.ApplyOutcome(context.Background(), txID, outcomeID, model.OutcomeAborted)
		require.ErrorIs(t, err, constant.ErrReservationOutcomeConflict)
		assert.Nil(t, result)
	})

	t.Run("audit failure rolls back receipt and every counter move", func(t *testing.T) {
		svc, deps := newReservationServiceDeps(t)
		reservations := twoReservations(txID)
		auditErr := assert.AnError

		deps.expectTxRollback()
		deps.repo.EXPECT().
			ApplyOutcomeWithTx(gomock.Any(), deps.tx, txID, outcomeID, model.OutcomeCommitted, now).
			Return(newReceipt(model.OutcomeCommitted, 2), reservations, false, nil).
			Times(1)
		deps.auditWriter.EXPECT().
			RecordReservationEventWithTx(gomock.Any(), deps.tx, model.AuditEventReservationConfirmed, model.AuditActionConfirm, gomock.Any(), gomock.Any()).
			Return(auditErr).
			Times(1)

		result, err := svc.ApplyOutcome(context.Background(), txID, outcomeID, model.OutcomeCommitted)
		require.ErrorIs(t, err, auditErr)
		assert.Nil(t, result)
	})

	t.Run("invalid identifiers and outcome fail before opening a transaction", func(t *testing.T) {
		svc, _ := newReservationServiceDeps(t)

		_, err := svc.ApplyOutcome(context.Background(), uuid.Nil, outcomeID, model.OutcomeCommitted)
		require.ErrorIs(t, err, ErrNilReservationTransationID)

		_, err = svc.ApplyOutcome(context.Background(), txID, uuid.Nil, model.OutcomeCommitted)
		require.ErrorIs(t, err, constant.ErrReservationOutcomeIDRequired)

		_, err = svc.ApplyOutcome(context.Background(), txID, outcomeID, model.OutcomeUnspecified)
		require.ErrorIs(t, err, constant.ErrReservationOutcomeInvalid)
	})
}
