// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	pgdbMocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	servicesMocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/services/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// newReservationServiceWithRules builds a service whose rule step is driven by a
// mock evaluator, so each case controls the rule decision independently of limits.
func newReservationServiceWithRules(t *testing.T) (*ReservationService, *reservationDeps, *servicesMocks.MockRuleEvaluator) {
	t.Helper()

	testutil.SetupTestTracing(t)

	ctrl := gomock.NewController(t)

	deps := &reservationDeps{
		ctrl:        ctrl,
		conn:        pgdbMocks.NewMockTxBeginner(ctrl),
		tx:          pgdbMocks.NewMockTx(ctrl),
		resolver:    servicesMocks.NewMockLimitResolver(ctrl),
		repo:        servicesMocks.NewMockReservationRepository(ctrl),
		auditWriter: servicesMocks.NewMockReservationAuditWriter(ctrl),
		clock:       testutil.NewMockClock(testutil.FixedTime()),
	}

	evaluator := servicesMocks.NewMockRuleEvaluator(ctrl)

	svc, err := NewReservationService(deps.conn, deps.resolver, deps.repo, deps.auditWriter, evaluator, deps.clock)
	require.NoError(t, err)

	return svc, deps, evaluator
}

func TestReservationService_Reserve_Rules(t *testing.T) {
	txID := testutil.MustDeterministicUUID(7250)
	ruleID := testutil.MustDeterministicUUID(7251)

	for _, decision := range []model.Decision{model.DecisionDeny, model.DecisionReview} {
		t.Run(string(decision)+" by rule refuses without resolving limits or holding capacity", func(t *testing.T) {
			svc, _, evaluator := newReservationServiceWithRules(t)

			req := testReserveRequest(t)

			evaluator.EXPECT().
				Execute(gomock.Any(), req).
				Return(&model.EvaluationResult{
					Decision:       decision,
					MatchedRuleIDs: []uuid.UUID{ruleID},
					Reason:         "blocked by rule",
				}, nil).
				Times(1)
			// No resolver, BeginTx, repo or audit expectation: any call fails the test.

			result, err := svc.Reserve(context.Background(), txID, req, false)
			require.NoError(t, err)
			assert.True(t, result.Denied)
			assert.Equal(t, decision, result.Decision)
			assert.Equal(t, "blocked by rule", result.Reason)
			assert.Equal(t, []uuid.UUID{ruleID}, result.MatchedRuleIDs)
			assert.Empty(t, result.ReservationIDs)
		})
	}

	t.Run("ALLOW by rule proceeds to limits and reserves", func(t *testing.T) {
		svc, deps, evaluator := newReservationServiceWithRules(t)

		req := testReserveRequest(t)

		gomock.InOrder(
			evaluator.EXPECT().
				Execute(gomock.Any(), req).
				Return(&model.EvaluationResult{Decision: model.DecisionAllow}, nil).
				Times(1),
			deps.resolver.EXPECT().
				ResolveReservations(gomock.Any(), req.ToCheckLimitsInput()).
				Return(oneSpec(), false, nil).
				Times(1),
		)

		deps.expectTxCommit()
		deps.expectScopeLock()
		deps.repo.EXPECT().
			ReserveWithTx(gomock.Any(), deps.tx, gomock.Any(), decEq(decimal.NewFromInt(10000))).
			Return(nil).
			Times(1)
		deps.auditWriter.EXPECT().
			RecordReservationEventWithTx(gomock.Any(), deps.tx, model.AuditEventReservationReserved, model.AuditActionReserve, gomock.Any(), gomock.Any()).
			Return(nil).
			Times(1)

		result, err := svc.Reserve(context.Background(), txID, req, false)
		require.NoError(t, err)
		assert.False(t, result.Denied)
		assert.Equal(t, model.DecisionAllow, result.Decision)
		assert.Empty(t, result.Reason)
		assert.Empty(t, result.MatchedRuleIDs)
		assert.Len(t, result.ReservationIDs, 1)
	})

	t.Run("ALLOW by rule then denied by limit reports limit_exceeded", func(t *testing.T) {
		svc, deps, evaluator := newReservationServiceWithRules(t)

		req := testReserveRequest(t)

		evaluator.EXPECT().
			Execute(gomock.Any(), req).
			Return(&model.EvaluationResult{Decision: model.DecisionAllow}, nil).
			Times(1)
		deps.resolver.EXPECT().
			ResolveReservations(gomock.Any(), req.ToCheckLimitsInput()).
			Return(nil, true, nil).
			Times(1)

		result, err := svc.Reserve(context.Background(), txID, req, false)
		require.NoError(t, err)
		assert.True(t, result.Denied)
		assert.Equal(t, model.DecisionDeny, result.Decision)
		assert.Equal(t, reserveReasonLimitExceeded, result.Reason)
	})

	t.Run("ALLOW by rule then denied by the reserve guard reports limit_exceeded", func(t *testing.T) {
		svc, deps, evaluator := newReservationServiceWithRules(t)

		req := testReserveRequest(t)

		evaluator.EXPECT().
			Execute(gomock.Any(), req).
			Return(&model.EvaluationResult{Decision: model.DecisionAllow}, nil).
			Times(1)
		deps.resolver.EXPECT().
			ResolveReservations(gomock.Any(), req.ToCheckLimitsInput()).
			Return(oneSpec(), false, nil).
			Times(1)
		deps.expectTxRollback()
		deps.expectScopeLock()
		deps.repo.EXPECT().
			ReserveWithTx(gomock.Any(), deps.tx, gomock.Any(), gomock.Any()).
			Return(constant.ErrUsageCounterExceedsLimit).
			Times(1)

		result, err := svc.Reserve(context.Background(), txID, req, false)
		require.NoError(t, err)
		assert.True(t, result.Denied)
		assert.Equal(t, model.DecisionDeny, result.Decision)
		assert.Equal(t, reserveReasonLimitExceeded, result.Reason)
	})

	t.Run("nil evaluator skips the rule step", func(t *testing.T) {
		svc, deps := newReservationServiceDeps(t)

		req := testReserveRequest(t)

		deps.resolver.EXPECT().
			ResolveReservations(gomock.Any(), req.ToCheckLimitsInput()).
			Return(nil, false, nil).
			Times(1)

		result, err := svc.Reserve(context.Background(), txID, req, false)
		require.NoError(t, err)
		assert.False(t, result.Denied)
		assert.Equal(t, model.DecisionAllow, result.Decision)
	})

	t.Run("evaluator error propagates and no limit is resolved", func(t *testing.T) {
		svc, _, evaluator := newReservationServiceWithRules(t)

		req := testReserveRequest(t)
		evalErr := errors.New("rule store unavailable")

		evaluator.EXPECT().
			Execute(gomock.Any(), req).
			Return(nil, evalErr).
			Times(1)

		result, err := svc.Reserve(context.Background(), txID, req, false)
		require.ErrorIs(t, err, evalErr)
		assert.Nil(t, result)
	})

	t.Run("evaluator nil result is an error", func(t *testing.T) {
		svc, _, evaluator := newReservationServiceWithRules(t)

		req := testReserveRequest(t)

		evaluator.EXPECT().
			Execute(gomock.Any(), req).
			Return(nil, nil).
			Times(1)

		result, err := svc.Reserve(context.Background(), txID, req, false)
		require.ErrorIs(t, err, ErrNilRuleEvaluationResult)
		assert.Nil(t, result)
	})

	t.Run("nil request is rejected before any rule evaluation", func(t *testing.T) {
		svc, _, _ := newReservationServiceWithRules(t)

		result, err := svc.Reserve(context.Background(), txID, nil, false)
		require.ErrorIs(t, err, ErrNilReservationRequest)
		assert.Nil(t, result)
	})
}
