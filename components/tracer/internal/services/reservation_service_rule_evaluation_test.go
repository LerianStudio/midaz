// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"fmt"
	"testing"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	pgdbMocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/postgres/db/mocks"
	servicesMocks "github.com/LerianStudio/midaz/v4/components/tracer/internal/services/mocks"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// newReservationServiceWithRuleQuery wires a ReservationService over the real
// rule pipeline. Its persistence collaborators are mocks with no expectation,
// so a reserve that reaches limits or storage fails the test.
func newReservationServiceWithRuleQuery(t *testing.T, ctrl *gomock.Controller, rules ...*model.Rule) *ReservationService {
	t.Helper()

	svc, err := NewReservationService(
		pgdbMocks.NewMockTxBeginner(ctrl),
		servicesMocks.NewMockLimitResolver(ctrl),
		servicesMocks.NewMockReservationRepository(ctrl),
		servicesMocks.NewMockReservationAuditWriter(ctrl),
		realRuleQuery(t, ctrl, rules...),
		testutil.NewMockClock(testutil.FixedTime()),
	)
	require.NoError(t, err)

	return svc
}

func reserveRuleEvaluationRequest(t *testing.T, metadata map[string]any) *model.ValidationRequest {
	t.Helper()

	req := testReserveRequest(t)
	req.Metadata = metadata

	return req
}

// A newer rule that fails at runtime must not hide an older DENY that matches.
func TestReserve_RealCEL_FailingRuleDoesNotHideOlderDeny(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	failingID := testutil.MustDeterministicUUID(7300)
	denyID := testutil.MustDeterministicUUID(7301)

	svc := newReservationServiceWithRuleQuery(t, ctrl, tierRule(failingID), denyRule(denyID))

	result, err := svc.Reserve(context.Background(), testutil.MustDeterministicUUID(7302),
		reserveRuleEvaluationRequest(t, map[string]any{"tier": "1"}), ReserveOptions{})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.True(t, result.Denied)
	assert.Equal(t, model.DecisionDeny, result.Decision)
	assert.Equal(t, []uuid.UUID{denyID}, result.MatchedRuleIDs)
	assert.NotEqual(t, reasonRuleEvaluationError, result.Reason)
	assert.Empty(t, result.ReservationIDs)
}

func TestReserve_RealCEL_FailingRulesWithoutDenyRouteToReview(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	firstFailing := testutil.MustDeterministicUUID(7310)
	allowID := testutil.MustDeterministicUUID(7311)
	secondFailing := testutil.MustDeterministicUUID(7312)

	svc := newReservationServiceWithRuleQuery(t, ctrl, tierRule(firstFailing), allowRule(allowID), tierRule(secondFailing))

	logger := testutil.NewMockLogger()
	ctx := libObservability.ContextWithLogger(context.Background(), logger)

	result, err := svc.Reserve(ctx, testutil.MustDeterministicUUID(7313),
		reserveRuleEvaluationRequest(t, map[string]any{"tier": "secret-value"}), ReserveOptions{})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.True(t, result.Denied)
	assert.Equal(t, model.DecisionReview, result.Decision)
	assert.Equal(t, reasonRuleEvaluationError, result.Reason)
	assert.Equal(t, []uuid.UUID{firstFailing, secondFailing}, result.MatchedRuleIDs)
	assert.Empty(t, result.ReservationIDs)

	warn := findLogCall(t, logger, "Reservation routed to review: rule evaluation failed")
	fields := testutil.FieldsToMap(warn.Fields)
	assert.Equal(t, constant.ErrExpressionEvaluation.Error(), fields["error.class"])
	assert.Equal(t, []string{firstFailing.String(), secondFailing.String()}, fields["rule_ids"])
	assert.NotContains(t, fields, "error")

	for _, call := range logger.Calls {
		for _, field := range call.Fields {
			assert.NotContains(t, fmt.Sprint(field.Value), "secret-value", "log %q leaks request metadata", call.Message)
		}
	}
}

func TestReserve_RealCEL_MatchedReviewAndFailingRuleRouteToReview(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	reviewID := testutil.MustDeterministicUUID(7320)
	failingID := testutil.MustDeterministicUUID(7321)
	allowID := testutil.MustDeterministicUUID(7322)

	svc := newReservationServiceWithRuleQuery(t, ctrl, reviewRule(reviewID), tierRule(failingID), allowRule(allowID))

	result, err := svc.Reserve(context.Background(), testutil.MustDeterministicUUID(7323),
		reserveRuleEvaluationRequest(t, map[string]any{"tier": "1"}), ReserveOptions{})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.True(t, result.Denied)
	assert.Equal(t, model.DecisionReview, result.Decision)
	assert.Equal(t, reasonRuleEvaluationError, result.Reason)
	assert.Equal(t, []uuid.UUID{reviewID, failingID}, result.MatchedRuleIDs)
	assert.Empty(t, result.ReservationIDs)
}

func TestReserve_RealCEL_MatchedAllowAndFailingRuleRouteToReview(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	allowID := testutil.MustDeterministicUUID(7330)
	failingID := testutil.MustDeterministicUUID(7331)

	svc := newReservationServiceWithRuleQuery(t, ctrl, allowRule(allowID), tierRule(failingID))

	result, err := svc.Reserve(context.Background(), testutil.MustDeterministicUUID(7332),
		reserveRuleEvaluationRequest(t, map[string]any{"tier": "1"}), ReserveOptions{})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, model.DecisionReview, result.Decision)
	assert.Equal(t, reasonRuleEvaluationError, result.Reason)
	assert.Equal(t, []uuid.UUID{failingID}, result.MatchedRuleIDs)
}

// An amount beyond CEL's precision routes the reserve to review, attributed to
// the rule it aborted at.
func TestReserve_RealCEL_AmountBeyondPrecisionRoutesToReview(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	reviewID := testutil.MustDeterministicUUID(7340)

	svc := newReservationServiceWithRuleQuery(t, ctrl, reviewRule(reviewID))

	req := reserveRuleEvaluationRequest(t, nil)
	req.Amount = decimal.RequireFromString("9007199254740993")

	result, err := svc.Reserve(context.Background(), testutil.MustDeterministicUUID(7341), req, ReserveOptions{})
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, model.DecisionReview, result.Decision)
	assert.Equal(t, reasonRuleEvaluationError, result.Reason)
	assert.Equal(t, []uuid.UUID{reviewID}, result.MatchedRuleIDs)
}

// Sequential: SetupTestTracing swaps the process-global tracer provider.
func TestReserve_RuleExpressionFailure_SpanEventsOmitRawError(t *testing.T) {
	const secret = "secret-metadata-value"

	tracing := testutil.SetupTestTracing(t)
	ctrl := gomock.NewController(t)
	evaluator := servicesMocks.NewMockRuleEvaluator(ctrl)

	svc, err := NewReservationService(
		pgdbMocks.NewMockTxBeginner(ctrl),
		servicesMocks.NewMockLimitResolver(ctrl),
		servicesMocks.NewMockReservationRepository(ctrl),
		servicesMocks.NewMockReservationAuditWriter(ctrl),
		evaluator,
		testutil.NewMockClock(testutil.FixedTime()),
	)
	require.NoError(t, err)

	req := testReserveRequest(t)

	evaluator.EXPECT().Execute(gomock.Any(), req).Return(nil, &query.RuleEvaluationError{
		RuleID: testutil.MustDeterministicUUID(7350),
		Err:    fmt.Errorf("%w: no such overload: %s", constant.ErrExpressionEvaluation, secret),
	}).Times(1)

	result, err := svc.Reserve(context.Background(), testutil.MustDeterministicUUID(7351), req, ReserveOptions{})
	require.NoError(t, err)
	assert.Equal(t, model.DecisionReview, result.Decision)

	assertNoSpanEventCarries(t, tracing, secret)
}
