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
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/constant"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
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

func twoSpecs() []query.ReservationSpec {
	return []query.ReservationSpec{
		{
			LimitID:   testutil.MustDeterministicUUID(7101),
			ScopeKey:  "acct:7001",
			PeriodKey: "2026-06",
			Amount:    400,
			MaxAmount: 10000,
		},
		{
			LimitID:   testutil.MustDeterministicUUID(7102),
			ScopeKey:  "global",
			PeriodKey: "2026-06-05",
			Amount:    400,
			MaxAmount: 5000,
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
			ReserveWithTx(gomock.Any(), deps.tx, gomock.AssignableToTypeOf(&model.Reservation{}), int64(10000)).
			Return(nil).
			Times(1)
		deps.repo.EXPECT().
			ReserveWithTx(gomock.Any(), deps.tx, gomock.AssignableToTypeOf(&model.Reservation{}), int64(5000)).
			Return(nil).
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
			ReserveWithTx(gomock.Any(), deps.tx, gomock.Any(), int64(10000)).
			Return(constant.ErrUsageCounterExceedsLimit).
			Times(1)

		result, err := svc.Reserve(context.Background(), txID, input, false)
		require.NoError(t, err)
		assert.True(t, result.Denied, "guard-denied reserve must surface the limit-exceeded decision")
		assert.Empty(t, result.ReservationIDs)
	})

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

		var captured time.Time
		deps.repo.EXPECT().
			ReserveWithTx(gomock.Any(), deps.tx, gomock.Any(), int64(10000)).
			DoAndReturn(func(_ context.Context, _ any, r *model.Reservation, _ int64) error {
				captured = r.ReservationExpiresAt

				return nil
			}).
			Times(1)
		deps.auditWriter.EXPECT().
			RecordReservationEventWithTx(gomock.Any(), deps.tx, model.AuditEventReservationReserved, model.AuditActionReserve, gomock.Any(), gomock.Any()).
			Return(nil).
			Times(1)

		_, err := svc.Reserve(context.Background(), txID, input, false)
		require.NoError(t, err)

		// Direct transactions use the fixed short TTL, NOT the long-lived knob.
		assert.Equal(t, now.UTC().Add(reservationTTL), captured)
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
			ReserveWithTx(gomock.Any(), tx, gomock.Any(), int64(10000)).
			DoAndReturn(func(_ context.Context, _ any, r *model.Reservation, _ int64) error {
				captured = r.ReservationExpiresAt

				return nil
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
			ReserveWithTx(gomock.Any(), deps.tx, gomock.Any(), int64(10000)).
			DoAndReturn(func(_ context.Context, _ any, r *model.Reservation, _ int64) error {
				captured = r.ReservationExpiresAt

				return nil
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
			LimitID:   testutil.MustDeterministicUUID(7101),
			ScopeKey:  "acct:7001",
			PeriodKey: "2026-06",
			Amount:    400,
			MaxAmount: 10000,
		},
	}
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
		testutil.MustDeterministicUUID(7401), txID, "acct:7401", "2026-06", 400,
		testutil.FixedTime().Add(5*time.Minute), testutil.FixedTime(),
	)
	res2, _ := model.NewReservation(
		testutil.MustDeterministicUUID(7402), txID, "global", "2026-06-05", 400,
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
