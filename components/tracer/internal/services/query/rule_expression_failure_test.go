// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"errors"
	"fmt"
	"testing"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/cel"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func expressionFailure() error {
	return fmt.Errorf("failed to evaluate expression: %w: no such overload", constant.ErrExpressionEvaluation)
}

func precisionFailure() error {
	return fmt.Errorf("failed to evaluate expression: %w: failed to build activation: %w", constant.ErrExpressionEvaluation,
		fmt.Errorf("amount exceeds safe precision for CEL evaluation: %w", constant.ErrAmountExceedsPrecision))
}

func failureTestRequest() *model.ValidationRequest {
	return &model.ValidationRequest{
		RequestID: testutil.MustDeterministicUUID(8001),
		Amount:    decimal.RequireFromString("100"),
		Asset:     "USD",
	}
}

func failureTestRule(seed int64, action model.Decision) *model.Rule {
	return &model.Rule{ID: testutil.MustDeterministicUUID(seed), Name: "rule", Expression: "true", Action: action}
}

func TestRuleExpressionFailurePredicates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		err             error
		wantExpression  bool
		wantUnevaluable bool
		wantClass       string
	}{
		{name: "runtime evaluation failure", err: expressionFailure(), wantExpression: true, wantUnevaluable: true, wantClass: constant.ErrExpressionEvaluation.Error()},
		{name: "non-bool result", err: fmt.Errorf("%w: expected bool", constant.ErrExpressionType), wantExpression: true, wantUnevaluable: true, wantClass: constant.ErrExpressionType.Error()},
		{name: "compile failure", err: fmt.Errorf("failed to compile expression: %w", constant.ErrExpressionSyntax), wantExpression: true, wantUnevaluable: true, wantClass: constant.ErrExpressionSyntax.Error()},
		{name: "amount beyond CEL precision", err: precisionFailure(), wantExpression: true, wantUnevaluable: false, wantClass: constant.ErrAmountExceedsPrecision.Error()},
		{name: "infrastructure failure", err: errors.New("connection reset"), wantExpression: false, wantUnevaluable: false},
		{name: "nil", err: nil, wantExpression: false, wantUnevaluable: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.wantExpression, IsRuleExpressionFailure(tt.err))
			assert.Equal(t, tt.wantUnevaluable, IsUnevaluableRule(tt.err))
			assert.Equal(t, tt.wantClass, RuleExpressionFailureClass(tt.err))
		})
	}
}

func TestFailingRuleIDs(t *testing.T) {
	t.Parallel()

	first := testutil.MustDeterministicUUID(8010)
	second := testutil.MustDeterministicUUID(8011)

	tests := []struct {
		name string
		err  error
		want []uuid.UUID
	}{
		{name: "single failure", err: &RuleEvaluationError{RuleID: first, Err: expressionFailure()}, want: []uuid.UUID{first}},
		{
			name: "wrapped failures of several rules",
			err: fmt.Errorf("failed to evaluate rules: %w", &RuleEvaluationFailures{Failures: []*RuleEvaluationError{
				{RuleID: first, Err: expressionFailure()},
				{RuleID: second, Err: expressionFailure()},
			}}),
			want: []uuid.UUID{first, second},
		},
		{name: "failure without a rule id", err: &RuleEvaluationError{Err: expressionFailure()}, want: nil},
		{name: "unattributed error", err: expressionFailure(), want: nil},
		{name: "nil", err: nil, want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, FailingRuleIDs(tt.err))
		})
	}
}

// Rules arrive newest first; a newer rule failing on the request must not hide
// an older DENY that matches it.
func TestCompleteEvaluator_EvaluateAll_FailingRuleDoesNotHideDeny(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	request := failureTestRequest()
	failing := failureTestRule(8020, model.DecisionDeny)
	deny := failureTestRule(8021, model.DecisionDeny)

	mockEval := NewMockSingleRuleEvaluator(ctrl)
	gomock.InOrder(
		mockEval.EXPECT().Evaluate(gomock.Any(), failing, request).Return(false, expressionFailure()),
		mockEval.EXPECT().Evaluate(gomock.Any(), deny, request).Return(true, nil),
	)

	evaluator, err := NewCompleteEvaluator(mockEval)
	require.NoError(t, err)

	collector, err := evaluator.EvaluateAll(context.Background(), []*model.Rule{failing, deny}, request)
	require.NoError(t, err)
	require.NotNil(t, collector)
	assert.Equal(t, []uuid.UUID{deny.ID}, collector.DenyRuleIDs)
	assert.Equal(t, []uuid.UUID{deny.ID}, collector.EvaluatedRuleIDs)
	assert.Equal(t, []uuid.UUID{failing.ID}, collector.FailedRuleIDs)
}

func TestCompleteEvaluator_EvaluateAll_FailingRulesWithoutDenySurfaceEveryFailure(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	request := failureTestRequest()
	firstFailing := failureTestRule(8030, model.DecisionReview)
	allow := failureTestRule(8031, model.DecisionAllow)
	secondFailing := failureTestRule(8032, model.DecisionDeny)

	mockEval := NewMockSingleRuleEvaluator(ctrl)
	gomock.InOrder(
		mockEval.EXPECT().Evaluate(gomock.Any(), firstFailing, request).Return(false, expressionFailure()),
		mockEval.EXPECT().Evaluate(gomock.Any(), allow, request).Return(true, nil),
		mockEval.EXPECT().Evaluate(gomock.Any(), secondFailing, request).
			Return(false, fmt.Errorf("%w: expected bool", constant.ErrExpressionType)),
	)

	evaluator, err := NewCompleteEvaluator(mockEval)
	require.NoError(t, err)

	collector, err := evaluator.EvaluateAll(context.Background(), []*model.Rule{firstFailing, allow, secondFailing}, request)
	require.Error(t, err)
	assert.Nil(t, collector)
	assert.True(t, IsUnevaluableRule(err))
	assert.ErrorIs(t, err, constant.ErrExpressionEvaluation)
	assert.ErrorIs(t, err, constant.ErrExpressionType)
	assert.Equal(t, []uuid.UUID{firstFailing.ID, secondFailing.ID}, FailingRuleIDs(err))
	assert.Contains(t, err.Error(), firstFailing.ID.String())
	assert.Contains(t, err.Error(), secondFailing.ID.String())
}

func TestCompleteEvaluator_EvaluateAll_AbortsOnNonExpressionFailures(t *testing.T) {
	t.Parallel()

	infra := errors.New("connection reset")

	tests := []struct {
		name  string
		cause error
	}{
		{name: "infrastructure failure", cause: infra},
		{name: "amount beyond CEL precision", cause: precisionFailure()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			request := failureTestRequest()
			failing := failureTestRule(8040, model.DecisionAllow)
			deny := failureTestRule(8041, model.DecisionDeny)

			mockEval := NewMockSingleRuleEvaluator(ctrl)
			// The later DENY is never evaluated: any call to it fails the test.
			mockEval.EXPECT().Evaluate(gomock.Any(), failing, request).Return(false, tt.cause)

			evaluator, err := NewCompleteEvaluator(mockEval)
			require.NoError(t, err)

			collector, err := evaluator.EvaluateAll(context.Background(), []*model.Rule{failing, deny}, request)
			require.ErrorIs(t, err, tt.cause)
			assert.Nil(t, collector)
			assert.Equal(t, []uuid.UUID{failing.ID}, FailingRuleIDs(err))
		})
	}
}

// spanStatus returns the status code of the first exported span named name.
func spanStatus(t *testing.T, tt *testutil.TestTracer, name string) codes.Code {
	t.Helper()

	for _, span := range tt.Exporter.GetSpans() {
		if span.Name == name {
			return span.Status.Code
		}
	}

	require.Failf(t, "span not exported", "no span named %q", name)

	return codes.Unset
}

func errorLogs(logger *testutil.MockLogger) []testutil.LogCall {
	var out []testutil.LogCall

	for _, call := range logger.Calls {
		if call.Level == "error" {
			out = append(out, call)
		}
	}

	return out
}

// Sequential: SetupTestTracing swaps the process-global tracer provider.
func TestRuleEvaluationLayers_ExpressionFailureIsBusinessClass(t *testing.T) {
	for _, tc := range []struct {
		name  string
		cause error
	}{
		{name: "runtime evaluation failure", cause: expressionFailure()},
		{name: "amount beyond CEL precision", cause: precisionFailure()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tracing := testutil.SetupTestTracing(t)
			ctrl := gomock.NewController(t)
			logger := testutil.NewMockLogger()
			ctx := libObservability.ContextWithLogger(context.Background(), logger)

			rule := failureTestRule(8050, model.DecisionDeny)
			rule.CompiledProgram = &cel.CompiledProgram{}

			exprEval := NewMockExpressionEvaluator(ctrl)
			exprEval.EXPECT().Evaluate(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, tc.cause)

			single, err := NewRuleEvaluator(exprEval)
			require.NoError(t, err)

			complete, err := NewCompleteEvaluator(single)
			require.NoError(t, err)

			activeRules := NewMockGetActiveRulesExecutor(ctrl)
			activeRules.EXPECT().Execute(gomock.Any(), gomock.Any()).Return([]*model.Rule{rule}, nil)

			q, err := NewEvaluateRulesQuery(activeRules, complete, &EvaluationConfig{DefaultDecisionWhenNoMatch: model.DecisionAllow})
			require.NoError(t, err)

			_, err = q.Execute(ctx, failureTestRequest())
			require.ErrorIs(t, err, tc.cause)

			for _, name := range []string{"service.rules.evaluate_expression", "service.rules.evaluate_all", "service.rules.evaluate"} {
				assert.Equal(t, codes.Unset, spanStatus(t, tracing, name), "span %s must stay green on an expression failure", name)
			}

			assert.Empty(t, errorLogs(logger), "an expression failure is not logged at Error below the service")
		})
	}
}

// Sequential: SetupTestTracing swaps the process-global tracer provider.
func TestRuleEvaluationLayers_InfrastructureFailureIsTechnicalClass(t *testing.T) {
	tracing := testutil.SetupTestTracing(t)
	ctrl := gomock.NewController(t)
	logger := testutil.NewMockLogger()
	ctx := libObservability.ContextWithLogger(context.Background(), logger)

	rule := failureTestRule(8060, model.DecisionDeny)
	rule.CompiledProgram = &cel.CompiledProgram{}
	infra := errors.New("program is required")

	exprEval := NewMockExpressionEvaluator(ctrl)
	exprEval.EXPECT().Evaluate(gomock.Any(), gomock.Any(), gomock.Any()).Return(false, infra)

	single, err := NewRuleEvaluator(exprEval)
	require.NoError(t, err)

	complete, err := NewCompleteEvaluator(single)
	require.NoError(t, err)

	activeRules := NewMockGetActiveRulesExecutor(ctrl)
	activeRules.EXPECT().Execute(gomock.Any(), gomock.Any()).Return([]*model.Rule{rule}, nil)

	q, err := NewEvaluateRulesQuery(activeRules, complete, &EvaluationConfig{DefaultDecisionWhenNoMatch: model.DecisionAllow})
	require.NoError(t, err)

	_, err = q.Execute(ctx, failureTestRequest())
	require.ErrorIs(t, err, infra)

	for _, name := range []string{"service.rules.evaluate_expression", "service.rules.evaluate_all", "service.rules.evaluate"} {
		assert.Equal(t, codes.Error, spanStatus(t, tracing, name), "span %s must flip red on an infrastructure failure", name)
	}

	assert.NotEmpty(t, errorLogs(logger))
}
