// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"testing"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	otelCodes "go.opentelemetry.io/otel/codes"
	"go.uber.org/mock/gomock"

	pgdb "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db"
	pgdbMocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	servicesMocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/services/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

type reservationDeps struct {
	ctrl        *gomock.Controller
	conn        *pgdbMocks.MockTxBeginner
	tx          *pgdbMocks.MockTx
	resolver    *servicesMocks.MockLimitResolver
	repo        *servicesMocks.MockReservationRepository
	auditWriter *servicesMocks.MockReservationAuditWriter
	clock       clock.Clock
	tracing     *testutil.TestTracer
	logger      *testutil.MockLogger
}

func newReservationServiceDeps(t *testing.T) (*ReservationService, *reservationDeps) {
	t.Helper()

	ctrl := gomock.NewController(t)

	deps := &reservationDeps{
		ctrl:        ctrl,
		conn:        pgdbMocks.NewMockTxBeginner(ctrl),
		tx:          pgdbMocks.NewMockTx(ctrl),
		resolver:    servicesMocks.NewMockLimitResolver(ctrl),
		repo:        servicesMocks.NewMockReservationRepository(ctrl),
		auditWriter: servicesMocks.NewMockReservationAuditWriter(ctrl),
		clock:       testutil.NewMockClock(testutil.FixedTime()),
		tracing:     testutil.SetupTestTracing(t),
		logger:      testutil.NewMockLogger(),
	}

	svc, err := NewReservationService(deps.conn, deps.resolver, deps.repo, deps.auditWriter, allowRuleEvaluator{}, deps.clock)
	require.NoError(t, err)

	return svc, deps
}

// ctx returns a context whose tracking logger is the capturing MockLogger, so a
// subtest can assert on the level and fields the service logs at.
func (d *reservationDeps) ctx() context.Context {
	return libObservability.ContextWithLogger(context.Background(), d.logger)
}

// logCalls returns the captured log calls at the given level.
func (d *reservationDeps) logCalls(level string) []testutil.LogCall {
	var out []testutil.LogCall

	for _, call := range d.logger.Calls {
		if call.Level == level {
			out = append(out, call)
		}
	}

	return out
}

// spanEvents returns the names of the events recorded on the span with the
// given name, together with that span's status code.
func (d *reservationDeps) spanEvents(t *testing.T, spanName string) ([]string, otelCodes.Code) {
	t.Helper()

	for _, span := range d.tracing.GetSpans() {
		if span.Name != spanName {
			continue
		}

		names := make([]string, 0, len(span.Events))
		for _, event := range span.Events {
			names = append(names, event.Name)
		}

		return names, span.Status.Code
	}

	require.Failf(t, "span not recorded", "no span named %q was exported", spanName)

	return nil, otelCodes.Unset
}

// spanIntAttribute returns the int attribute key recorded on the span with the
// given name, and whether it was set.
func (d *reservationDeps) spanIntAttribute(t *testing.T, spanName, key string) (int64, bool) {
	t.Helper()

	for _, span := range d.tracing.GetSpans() {
		if span.Name != spanName {
			continue
		}

		for _, attr := range span.Attributes {
			if string(attr.Key) == key {
				return attr.Value.AsInt64(), true
			}
		}

		return 0, false
	}

	require.Failf(t, "span not recorded", "no span named %q was exported", spanName)

	return 0, false
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

// expectScopeLock wires the per-account advisory lock the reserve closure acquires
// on the shared mock tx before any counter row is touched. Only the
// Reserve path takes it; confirm/release do not, so it is opt-in per subtest.
func (d *reservationDeps) expectScopeLock() {
	d.repo.EXPECT().AcquireReserveScopeLock(gomock.Any(), d.tx, gomock.Any()).Return(nil).Times(1)
}

func TestNewReservationService_NilDeps(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	conn := pgdbMocks.NewMockTxBeginner(ctrl)
	resolver := servicesMocks.NewMockLimitResolver(ctrl)
	repo := servicesMocks.NewMockReservationRepository(ctrl)
	audit := servicesMocks.NewMockReservationAuditWriter(ctrl)

	evaluator := allowRuleEvaluator{}

	_, err := NewReservationService(nil, resolver, repo, audit, evaluator, nil)
	require.ErrorIs(t, err, ErrNilReservationConn)

	_, err = NewReservationService(conn, nil, repo, audit, evaluator, nil)
	require.ErrorIs(t, err, ErrNilLimitResolver)

	_, err = NewReservationService(conn, resolver, nil, audit, evaluator, nil)
	require.ErrorIs(t, err, ErrNilReservationRepo)

	_, err = NewReservationService(conn, resolver, repo, nil, evaluator, nil)
	require.ErrorIs(t, err, ErrNilReservationAuditWriter)

	_, err = NewReservationService(conn, resolver, repo, audit, nil, nil)
	require.ErrorIs(t, err, ErrNilRuleEvaluator)
}

func testReserveRequest(t *testing.T) *model.ValidationRequest {
	t.Helper()

	return &model.ValidationRequest{
		RequestID:            testutil.MustDeterministicUUID(7000),
		Amount:               decimal.NewFromInt(400),
		Asset:                "USD",
		TransactionTimestamp: testutil.FixedTime(),
		Account:              model.AccountContext{ID: testutil.MustDeterministicUUID(7001)},
	}
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

		req := testReserveRequest(t)
		input := req.ToCheckLimitsInput()

		// Single resolution call (R38 / resolve-once invariant).
		deps.resolver.EXPECT().
			ResolveReservations(gomock.Any(), input).
			Return(twoSpecs(), false, nil).
			Times(1)

		deps.expectTxCommit()
		deps.expectScopeLock()

		// One reserve + one audit per applicable limit.
		deps.repo.EXPECT().
			ReserveWithTx(gomock.Any(), deps.tx, gomock.AssignableToTypeOf(&model.Reservation{}), decEq(decimal.NewFromInt(10000))).
			Return(false, nil).
			Times(1)
		deps.repo.EXPECT().
			ReserveWithTx(gomock.Any(), deps.tx, gomock.AssignableToTypeOf(&model.Reservation{}), decEq(decimal.NewFromInt(5000))).
			Return(false, nil).
			Times(1)
		deps.auditWriter.EXPECT().
			RecordReservationEventWithTx(gomock.Any(), deps.tx, model.AuditEventReservationReserved, model.AuditActionReserve, gomock.Any(), gomock.Any()).
			Return(nil).
			Times(2)

		result, err := svc.Reserve(context.Background(), txID, req, ReserveOptions{})
		require.NoError(t, err)
		require.False(t, result.Denied)
		assert.Len(t, result.ReservationIDs, 2)
	})

	t.Run("A replay onto a RESERVED row hands back its id and writes no second audit row", func(t *testing.T) {
		svc, deps := newReservationServiceDeps(t)

		req := testReserveRequest(t)
		input := req.ToCheckLimitsInput()
		specs := twoSpecs()

		// The id the retried reserve collapses onto. The repository overwrites the
		// reservation's id with the row that owns the capacity, so the handle the
		// ledger confirms or releases with must be THIS id, not the one the service
		// generated for the retry.
		owningID := testutil.MustDeterministicUUID(7099)

		deps.resolver.EXPECT().
			ResolveReservations(gomock.Any(), input).
			Return(specs[:1], false, nil).
			Times(1)

		deps.expectTxCommit()
		deps.expectScopeLock()

		deps.repo.EXPECT().
			ReserveWithTx(gomock.Any(), deps.tx, gomock.Any(), decEq(decimal.NewFromInt(10000))).
			DoAndReturn(func(_ context.Context, _ any, r *model.Reservation, _ decimal.Decimal) (bool, error) {
				r.ID = owningID

				return true, nil
			}).
			Times(1)

		// The original reserve already wrote the RESERVED audit row: a replay must
		// not write a second one. gomock fails the test on any unexpected call.
		deps.auditWriter.EXPECT().
			RecordReservationEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Times(0)

		result, err := svc.Reserve(context.Background(), txID, req, ReserveOptions{})
		require.NoError(t, err)
		require.False(t, result.Denied)
		require.Len(t, result.ReservationIDs, 1)
		assert.Equal(t, owningID, result.ReservationIDs[0],
			"the handle returned must address the row that owns the held capacity")
	})

	t.Run("A replay onto a settled row fails with 0533, no audit row, no id", func(t *testing.T) {
		svc, deps := newReservationServiceDeps(t)

		req := testReserveRequest(t)
		input := req.ToCheckLimitsInput()

		deps.resolver.EXPECT().
			ResolveReservations(gomock.Any(), input).
			Return(oneSpec(), false, nil).
			Times(1)

		// Exactly one BeginTx: 0533 is a business refusal, never a transient retry.
		deps.expectTxRollback()
		deps.expectScopeLock()

		deps.repo.EXPECT().
			ReserveWithTx(gomock.Any(), deps.tx, gomock.Any(), decEq(decimal.NewFromInt(10000))).
			Return(false, constant.ErrReservationAlreadySettled).
			Times(1)
		deps.auditWriter.EXPECT().
			RecordReservationEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Times(0)

		result, err := svc.Reserve(deps.ctx(), txID, req, ReserveOptions{})
		require.ErrorIs(t, err, constant.ErrReservationAlreadySettled)
		assert.Nil(t, result, "a settled replay holds nothing and returns no handle")

		warns := deps.logCalls("warn")
		require.Len(t, warns, 1, "a settled replay is logged once at Warn")
		assert.Equal(t, txID.String(), testutil.FieldsToMap(warns[0].Fields)["transaction_id"])

		_, status := deps.spanEvents(t, "service.reservation.reserve")
		assert.NotEqual(t, otelCodes.Error, status, "a settled replay is a business refusal: the span stays green")
	})

	t.Run("Fractional spec amount reaches the reservation row intact", func(t *testing.T) {
		svc, deps := newReservationServiceDeps(t)

		req := testReserveRequest(t)
		input := req.ToCheckLimitsInput()

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
		deps.expectScopeLock()

		var captured decimal.Decimal
		deps.repo.EXPECT().
			ReserveWithTx(gomock.Any(), deps.tx, gomock.Any(), decEq(decimal.NewFromInt(20))).
			DoAndReturn(func(_ context.Context, _ any, r *model.Reservation, _ decimal.Decimal) (bool, error) {
				captured = r.Amount

				return false, nil
			}).
			Times(1)
		deps.auditWriter.EXPECT().
			RecordReservationEventWithTx(gomock.Any(), deps.tx, model.AuditEventReservationReserved, model.AuditActionReserve, gomock.Any(), gomock.Any()).
			Return(nil).
			Times(1)

		_, err := svc.Reserve(context.Background(), txID, req, ReserveOptions{})
		require.NoError(t, err)

		// The pre-fix int64 path would have persisted 10 here.
		assert.True(t, fractional.Equal(captured), "expected 10.50 held, got %s", captured)
	})

	t.Run("Denied by resolver (per-transaction cap) returns Denied without a tx", func(t *testing.T) {
		svc, deps := newReservationServiceDeps(t)

		req := testReserveRequest(t)
		input := req.ToCheckLimitsInput()

		deps.resolver.EXPECT().
			ResolveReservations(gomock.Any(), input).
			Return(nil, true, nil).
			Times(1)
		// No BeginTx expected — denial short-circuits before the transaction.

		result, err := svc.Reserve(context.Background(), txID, req, ReserveOptions{})
		require.NoError(t, err)
		assert.True(t, result.Denied)
		assert.Empty(t, result.ReservationIDs)
	})

	t.Run("Reserve guard denies mid-tx -> rollback, Denied decision", func(t *testing.T) {
		svc, deps := newReservationServiceDeps(t)

		req := testReserveRequest(t)
		input := req.ToCheckLimitsInput()

		deps.resolver.EXPECT().
			ResolveReservations(gomock.Any(), input).
			Return(twoSpecs(), false, nil).
			Times(1)

		deps.expectTxRollback()
		deps.expectScopeLock()

		// First reserve trips the over-limit guard; the whole tx rolls back and no
		// further reserve/audit runs.
		deps.repo.EXPECT().
			ReserveWithTx(gomock.Any(), deps.tx, gomock.Any(), decEq(decimal.NewFromInt(10000))).
			Return(false, constant.ErrUsageCounterExceedsLimit).
			Times(1)

		result, err := svc.Reserve(context.Background(), txID, req, ReserveOptions{})
		require.NoError(t, err)
		assert.True(t, result.Denied, "guard-denied reserve must surface the limit-exceeded decision")
		assert.Empty(t, result.ReservationIDs)
	})

	t.Run("Scope-lock acquisition failure aborts the reserve", func(t *testing.T) {
		svc, deps := newReservationServiceDeps(t)

		req := testReserveRequest(t)
		input := req.ToCheckLimitsInput()

		deps.resolver.EXPECT().
			ResolveReservations(gomock.Any(), input).
			Return(twoSpecs(), false, nil).
			Times(1)

		deps.expectTxRollback()

		lockErr := errors.New("advisory lock failed")

		// The scope lock is taken FIRST; its failure rolls the tx back and no
		// reserve/audit runs.
		deps.repo.EXPECT().
			AcquireReserveScopeLock(gomock.Any(), deps.tx, gomock.Any()).
			Return(lockErr).
			Times(1)

		_, err := svc.Reserve(context.Background(), txID, req, ReserveOptions{})
		require.ErrorIs(t, err, lockErr)
	})

	t.Run("No applicable limits -> allow with empty handle", func(t *testing.T) {
		svc, deps := newReservationServiceDeps(t)

		req := testReserveRequest(t)
		input := req.ToCheckLimitsInput()

		deps.resolver.EXPECT().
			ResolveReservations(gomock.Any(), input).
			Return(nil, false, nil).
			Times(1)

		result, err := svc.Reserve(context.Background(), txID, req, ReserveOptions{})
		require.NoError(t, err)
		assert.False(t, result.Denied)
		assert.Empty(t, result.ReservationIDs)
	})

	t.Run("Missing transaction id is rejected", func(t *testing.T) {
		svc, _ := newReservationServiceDeps(t)

		_, err := svc.Reserve(context.Background(), uuid.Nil, testReserveRequest(t), ReserveOptions{})
		require.ErrorIs(t, err, ErrNilReservationTransationID)
	})

	t.Run("longLived=false sets the short direct TTL on the reservation", func(t *testing.T) {
		svc, deps := newReservationServiceDeps(t)

		req := testReserveRequest(t)
		input := req.ToCheckLimitsInput()
		now := testutil.FixedTime()

		deps.resolver.EXPECT().
			ResolveReservations(gomock.Any(), input).
			Return(oneSpec(), false, nil).
			Times(1)

		deps.expectTxCommit()

		var captured time.Time
		deps.repo.EXPECT().
			ReserveWithTx(gomock.Any(), deps.tx, gomock.Any(), decEq(decimal.NewFromInt(10000))).
			DoAndReturn(func(_ context.Context, _ any, r *model.Reservation, _ decimal.Decimal) (bool, error) {
				captured = r.ReservationExpiresAt

				return false, nil
			}).
			Times(1)
		deps.auditWriter.EXPECT().
			RecordReservationEventWithTx(gomock.Any(), deps.tx, model.AuditEventReservationReserved, model.AuditActionReserve, gomock.Any(), gomock.Any()).
			Return(nil).
			Times(1)

		deps.expectScopeLock()

		_, err := svc.Reserve(context.Background(), txID, req, ReserveOptions{})
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

		svc, err := NewReservationServiceWithLongLivedTTL(conn, resolver, repo, auditWriter, allowRuleEvaluator{}, clk, longLivedTTL)
		require.NoError(t, err)

		req := testReserveRequest(t)
		input := req.ToCheckLimitsInput()
		now := testutil.FixedTime()

		resolver.EXPECT().
			ResolveReservations(gomock.Any(), input).
			Return(oneSpec(), false, nil).
			Times(1)

		conn.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(tx, nil).Times(1)
		tx.EXPECT().Commit().Return(nil).Times(1)
		repo.EXPECT().AcquireReserveScopeLock(gomock.Any(), tx, gomock.Any()).Return(nil).Times(1)

		var captured time.Time
		repo.EXPECT().
			ReserveWithTx(gomock.Any(), tx, gomock.Any(), decEq(decimal.NewFromInt(10000))).
			DoAndReturn(func(_ context.Context, _ any, r *model.Reservation, _ decimal.Decimal) (bool, error) {
				captured = r.ReservationExpiresAt

				return false, nil
			}).
			Times(1)
		auditWriter.EXPECT().
			RecordReservationEventWithTx(gomock.Any(), tx, model.AuditEventReservationReserved, model.AuditActionReserve, gomock.Any(), gomock.Any()).
			Return(nil).
			Times(1)

		_, err = svc.Reserve(context.Background(), txID, req, ReserveOptions{LongLived: true})
		require.NoError(t, err)

		// PENDING reservations expire far out (the configured long-lived TTL), well
		// beyond the short direct TTL the reaper sweeps on (R18).
		assert.Equal(t, now.UTC().Add(longLivedTTL), captured)
		assert.True(t, captured.After(now.UTC().Add(reservationTTL)), "long-lived TTL must outlive the direct TTL")
	})

	t.Run("longLived=true with default service TTL uses the 30-day ceiling", func(t *testing.T) {
		svc, deps := newReservationServiceDeps(t)

		req := testReserveRequest(t)
		input := req.ToCheckLimitsInput()
		now := testutil.FixedTime()

		deps.resolver.EXPECT().
			ResolveReservations(gomock.Any(), input).
			Return(oneSpec(), false, nil).
			Times(1)

		deps.expectTxCommit()

		var captured time.Time
		deps.repo.EXPECT().
			ReserveWithTx(gomock.Any(), deps.tx, gomock.Any(), decEq(decimal.NewFromInt(10000))).
			DoAndReturn(func(_ context.Context, _ any, r *model.Reservation, _ decimal.Decimal) (bool, error) {
				captured = r.ReservationExpiresAt

				return false, nil
			}).
			Times(1)
		deps.auditWriter.EXPECT().
			RecordReservationEventWithTx(gomock.Any(), deps.tx, model.AuditEventReservationReserved, model.AuditActionReserve, gomock.Any(), gomock.Any()).
			Return(nil).
			Times(1)

		deps.expectScopeLock()

		_, err := svc.Reserve(context.Background(), txID, req, ReserveOptions{LongLived: true})
		require.NoError(t, err)

		// newReservationServiceDeps passes longLivedTTL=0, so the service falls back
		// to defaultLongLivedReservationTTL (30 days).
		assert.Equal(t, now.UTC().Add(defaultLongLivedReservationTTL), captured)
	})
}

// TestReservationService_Reserve_TransientRetry_NoDuplicateHandles locks the
// per-attempt accumulator reset (reservationIDs[:0] / guardDenied=false) at the top
// of the reserve closure. Attempt 1 reserves the first spec, then trips a transient
// 40P01 on the second and rolls back; attempt 2 begins fresh and reserves BOTH. Two
// BeginTx calls prove the retry; the result must carry exactly one handle per spec —
// never the three that a missing reset would accumulate across attempts.
func TestReservationService_Reserve_TransientRetry_NoDuplicateHandles(t *testing.T) {
	svc, deps := newReservationServiceDeps(t)

	req := testReserveRequest(t)
	input := req.ToCheckLimitsInput()
	specs := twoSpecs()

	deps.resolver.EXPECT().
		ResolveReservations(gomock.Any(), input).
		Return(specs, false, nil).
		Times(1)

	// DISTINCT transaction handles per attempt. tx1 is the attempt-1 handle that
	// trips the transient abort and is rolled back; tx2 is the fresh attempt-2
	// handle that commits. Wiring the whole lifecycle onto attempt-specific mocks
	// is what proves the rolled-back tx1 is never reused on attempt 2: every
	// attempt-2 expectation matches tx2 exactly, so any reuse of tx1 would trip a
	// gomock "unexpected call" (tx1 has no attempt-2 expectations and no Commit).
	tx1 := pgdbMocks.NewMockTx(deps.ctrl)
	tx2 := pgdbMocks.NewMockTx(deps.ctrl)

	// BeginTx hands out tx1 first, then tx2. Declaration order + Times(1) makes the
	// sequence deterministic: the first call exhausts the tx1 expectation.
	gomock.InOrder(
		deps.conn.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(tx1, nil).Times(1),
		deps.conn.EXPECT().BeginTx(gomock.Any(), gomock.Any()).Return(tx2, nil).Times(1),
	)

	// Attempt 1 (tx1): scope lock, then reserve spec0 ok + one audit row, then
	// reserve spec1 trips 40P01 and the whole tx1 rolls back. No Commit on tx1.
	tx1.EXPECT().Rollback().Return(nil).Times(1)
	deps.repo.EXPECT().
		AcquireReserveScopeLock(gomock.Any(), tx1, gomock.Any()).
		Return(nil).
		Times(1)

	tx1ReserveCalls := 0
	deps.repo.EXPECT().
		ReserveWithTx(gomock.Any(), tx1, gomock.AssignableToTypeOf(&model.Reservation{}), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ pgdb.DB, _ *model.Reservation, _ decimal.Decimal) (bool, error) {
			tx1ReserveCalls++
			if tx1ReserveCalls == 2 {
				return false, &pgconn.PgError{Code: "40P01"} // deadlock_detected, transient
			}

			return false, nil
		}).
		Times(2)
	deps.auditWriter.EXPECT().
		RecordReservationEventWithTx(gomock.Any(), tx1, model.AuditEventReservationReserved, model.AuditActionReserve, gomock.Any(), gomock.Any()).
		Return(nil).
		Times(1)

	// Attempt 2 (tx2): fresh begin, scope lock, both specs reserve + audit, commit.
	tx2.EXPECT().Commit().Return(nil).Times(1)
	deps.repo.EXPECT().
		AcquireReserveScopeLock(gomock.Any(), tx2, gomock.Any()).
		Return(nil).
		Times(1)
	deps.repo.EXPECT().
		ReserveWithTx(gomock.Any(), tx2, gomock.AssignableToTypeOf(&model.Reservation{}), gomock.Any()).
		Return(false, nil).
		Times(2)
	deps.auditWriter.EXPECT().
		RecordReservationEventWithTx(gomock.Any(), tx2, model.AuditEventReservationReserved, model.AuditActionReserve, gomock.Any(), gomock.Any()).
		Return(nil).
		Times(2)

	// Deterministic retry: no wall-clock backoff.
	svc.retrySleep = func(context.Context, time.Duration) error { return nil }

	res, err := svc.Reserve(context.Background(), testutil.MustDeterministicUUID(7050), req, ReserveOptions{})
	require.NoError(t, err)
	require.False(t, res.Denied)
	assert.Len(t, res.ReservationIDs, len(specs),
		"the retry must reset the per-attempt accumulator: exactly one handle per spec, never doubled")
}

func TestReserveScopeLockKey(t *testing.T) {
	t.Parallel()

	acctA := testutil.MustDeterministicUUID(7401)
	acctB := testutil.MustDeterministicUUID(7402)

	// nilAccountScopeLockKey is the FNV-1a (64-bit) of 16 zero bytes, cast to int64 —
	// the fixed key every external-only (nil-account) reserve must map to. Pinning the
	// literal locks the hashing so a change to reserveScopeLockKey cannot silently move
	// the shared key.
	const nilAccountScopeLockKey = int64(-8637869204239850395)

	t.Run("distinct accounts map to distinct keys", func(t *testing.T) {
		t.Parallel()

		assert.NotEqual(t, reserveScopeLockKey(acctA), reserveScopeLockKey(acctB),
			"distinct accounts must not collapse onto one key (parallelism preserved)")
	})

	t.Run("nil account maps to the fixed precomputed key", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, nilAccountScopeLockKey, reserveScopeLockKey(uuid.Nil),
			"an external-only (nil-account) reserve must serialize on the fixed FNV-1a-of-zeroes key")
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
			Amount:    decimal.NewFromInt(400),
			MaxAmount: decimal.NewFromInt(10000),
		},
	}
}

func TestReservationService_Confirm(t *testing.T) {
	resID := testutil.MustDeterministicUUID(7200)

	t.Run("Success - counter move + row flip + audit in one tx reports one confirmed", func(t *testing.T) {
		svc, deps := newReservationServiceDeps(t)

		deps.expectTxCommit()

		deps.repo.EXPECT().
			ConfirmWithTx(gomock.Any(), deps.tx, resID).
			Return(model.StatusReserved, nil).
			Times(1)
		deps.auditWriter.EXPECT().
			RecordReservationEventWithTx(gomock.Any(), deps.tx, model.AuditEventReservationConfirmed, model.AuditActionConfirm, resID, gomock.Any()).
			Return(nil).
			Times(1)

		outcome, err := svc.Confirm(deps.ctx(), resID)
		require.NoError(t, err)
		assert.Equal(t, ConfirmOutcome{Confirmed: 1}, outcome)
		assert.Empty(t, deps.logCalls("warn"), "a clean confirm is not a divergence")
	})

	t.Run("Idempotent double-confirm - CONFIRMED row is a no-op success, NO second counter move", func(t *testing.T) {
		svc, deps := newReservationServiceDeps(t)

		// Repo reports already-terminal; the service rolls back and returns nil
		// WITHOUT recording a second audit event or moving the counter again.
		deps.expectTxRollback()
		deps.repo.EXPECT().
			ConfirmWithTx(gomock.Any(), deps.tx, resID).
			Return(model.StatusConfirmed, constant.ErrReservationAlreadyTerminal).
			Times(1)
		deps.auditWriter.EXPECT().
			RecordReservationEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Times(0)

		outcome, err := svc.Confirm(deps.ctx(), resID)
		require.NoError(t, err, "retried confirm against a CONFIRMED reservation must be an idempotent success")
		assert.Equal(t, ConfirmOutcome{}, outcome)
		assert.Empty(t, deps.logCalls("warn"), "a CONFIRMED replay is Debug, not a divergence")

		events, _ := deps.spanEvents(t, "service.reservation.confirm")
		assert.NotContains(t, events, "reservation.confirm.already_released")
	})

	t.Run("Confirm on a RELEASED row reports it as already released with a Warn and span event", func(t *testing.T) {
		svc, deps := newReservationServiceDeps(t)

		deps.expectTxRollback()
		deps.repo.EXPECT().
			ConfirmWithTx(gomock.Any(), deps.tx, resID).
			Return(model.StatusReleased, constant.ErrReservationAlreadyTerminal).
			Times(1)
		deps.auditWriter.EXPECT().
			RecordReservationEventWithTx(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Times(0)

		outcome, err := svc.Confirm(deps.ctx(), resID)
		require.NoError(t, err, "a confirm that finds the row RELEASED is an outcome, not a failure")
		assert.Equal(t, ConfirmOutcome{AlreadyReleased: 1}, outcome)

		warns := deps.logCalls("warn")
		require.Len(t, warns, 1)

		fields := testutil.FieldsToMap(warns[0].Fields)
		assert.Equal(t, resID.String(), fields["reservation_id"])
		assert.EqualValues(t, 1, fields["already_released"])

		events, status := deps.spanEvents(t, "service.reservation.confirm")
		assert.Contains(t, events, "reservation.confirm.already_released")
		assert.NotEqual(t, otelCodes.Error, status, "an already-released row is a business observation: the span stays green")
	})

	t.Run("Not found propagates", func(t *testing.T) {
		svc, deps := newReservationServiceDeps(t)

		deps.expectTxRollback()
		deps.repo.EXPECT().
			ConfirmWithTx(gomock.Any(), deps.tx, resID).
			Return(model.ReservationStatus(""), constant.ErrReservationNotFound).
			Times(1)

		outcome, err := svc.Confirm(deps.ctx(), resID)
		require.ErrorIs(t, err, constant.ErrReservationNotFound)
		assert.Equal(t, ConfirmOutcome{}, outcome)

		_, status := deps.spanEvents(t, "service.reservation.confirm")
		assert.NotEqual(t, otelCodes.Error, status, "an unknown reservation is a business outcome: the span stays green")
	})

	t.Run("Missing reservation id is rejected before a tx", func(t *testing.T) {
		svc, _ := newReservationServiceDeps(t)

		_, err := svc.Confirm(context.Background(), uuid.Nil)
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

	t.Run("Not found propagates with a green span", func(t *testing.T) {
		svc, deps := newReservationServiceDeps(t)

		deps.expectTxRollback()
		deps.repo.EXPECT().
			ReleaseWithTx(gomock.Any(), deps.tx, resID, model.StatusReleased).
			Return(constant.ErrReservationNotFound).
			Times(1)

		require.ErrorIs(t, svc.Release(deps.ctx(), resID), constant.ErrReservationNotFound)

		_, status := deps.spanEvents(t, "service.reservation.release")
		assert.NotEqual(t, otelCodes.Error, status, "an unknown reservation is a business outcome: the span stays green")
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

	t.Run("Flips ALL reserved rows in one tx, audits each, counts released from the lock", func(t *testing.T) {
		svc, deps := newReservationServiceDeps(t)

		reservations := twoReservations(txID)

		deps.expectTxCommit()

		// The released count comes from the confirm's own locked read: no query
		// runs after the audit inserts.
		deps.repo.EXPECT().
			ConfirmByTransactionWithTx(gomock.Any(), deps.tx, txID).
			Return(reservations, 0, nil).
			Times(1)
		// One audit row per flipped reservation, same tx.
		deps.auditWriter.EXPECT().
			RecordReservationEventWithTx(gomock.Any(), deps.tx, model.AuditEventReservationConfirmed, model.AuditActionConfirm, gomock.Any(), gomock.Any()).
			Return(nil).
			Times(2)

		outcome, err := svc.ConfirmByTransaction(deps.ctx(), txID)
		require.NoError(t, err)
		assert.Equal(t, ConfirmOutcome{Confirmed: 2}, outcome, "every reserved row of the transaction is confirmed")
		assert.Empty(t, deps.logCalls("warn"))
	})

	t.Run("One RELEASED row among the transaction's rows is reported with a Warn and span event", func(t *testing.T) {
		svc, deps := newReservationServiceDeps(t)

		reservations := twoReservations(txID)[:1]

		deps.expectTxCommit()

		deps.repo.EXPECT().
			ConfirmByTransactionWithTx(gomock.Any(), deps.tx, txID).
			Return(reservations, 1, nil).
			Times(1)
		deps.auditWriter.EXPECT().
			RecordReservationEventWithTx(gomock.Any(), deps.tx, model.AuditEventReservationConfirmed, model.AuditActionConfirm, reservations[0].ID, gomock.Any()).
			Return(nil).
			Times(1)

		outcome, err := svc.ConfirmByTransaction(deps.ctx(), txID)
		require.NoError(t, err, "a released row is an outcome the caller reads, not a failure")
		assert.Equal(t, ConfirmOutcome{Confirmed: 1, AlreadyReleased: 1}, outcome)

		warns := deps.logCalls("warn")
		require.Len(t, warns, 1)

		fields := testutil.FieldsToMap(warns[0].Fields)
		assert.Equal(t, txID.String(), fields["transaction_id"])
		assert.EqualValues(t, 1, fields["already_released"])

		events, status := deps.spanEvents(t, "service.reservation.confirm_by_transaction")
		assert.Contains(t, events, "reservation.confirm.already_released")
		assert.NotEqual(t, otelCodes.Error, status, "an already-released row is a business observation: the span stays green")

		releasedCount, ok := deps.spanIntAttribute(t, "service.reservation.confirm_by_transaction", "app.reservation.already_released")
		require.True(t, ok, "the confirm span carries the released count")
		assert.EqualValues(t, 1, releasedCount)
	})

	t.Run("No reserved rows is an idempotent no-op success (re-run), NO audit", func(t *testing.T) {
		svc, deps := newReservationServiceDeps(t)

		deps.expectTxCommit()

		deps.repo.EXPECT().
			ConfirmByTransactionWithTx(gomock.Any(), deps.tx, txID).
			Return(nil, 0, nil).
			Times(1)
		// No audit call expected on the empty path.

		outcome, err := svc.ConfirmByTransaction(deps.ctx(), txID)
		require.NoError(t, err)
		assert.Equal(t, ConfirmOutcome{}, outcome, "re-run over an already-confirmed transaction is a clean no-op")
		assert.Empty(t, deps.logCalls("warn"))
	})

	t.Run("Repository failure rolls the confirm back", func(t *testing.T) {
		svc, deps := newReservationServiceDeps(t)

		repoErr := errors.New("lock failed")

		deps.expectTxRollback()

		deps.repo.EXPECT().
			ConfirmByTransactionWithTx(gomock.Any(), deps.tx, txID).
			Return(nil, 0, repoErr).
			Times(1)

		outcome, err := svc.ConfirmByTransaction(context.Background(), txID)
		require.ErrorIs(t, err, repoErr)
		assert.Equal(t, ConfirmOutcome{}, outcome)
	})

	t.Run("Audit failure rolls the confirm back", func(t *testing.T) {
		svc, deps := newReservationServiceDeps(t)

		auditErr := errors.New("audit failed")

		deps.expectTxRollback()

		deps.repo.EXPECT().
			ConfirmByTransactionWithTx(gomock.Any(), deps.tx, txID).
			Return(twoReservations(txID), 0, nil).
			Times(1)
		deps.auditWriter.EXPECT().
			RecordReservationEventWithTx(gomock.Any(), deps.tx, model.AuditEventReservationConfirmed, model.AuditActionConfirm, gomock.Any(), gomock.Any()).
			Return(auditErr).
			Times(1)

		outcome, err := svc.ConfirmByTransaction(context.Background(), txID)
		require.ErrorIs(t, err, auditErr)
		assert.Equal(t, ConfirmOutcome{}, outcome)
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
