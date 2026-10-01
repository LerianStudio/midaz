// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"fmt"
	"testing"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/cel"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/observability"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

type ruleOutcome struct {
	rule    *model.Rule
	matched bool
	err     error
}

func syntaxFailure() error {
	return fmt.Errorf("failed to compile expression: %w", constant.ErrExpressionSyntax)
}

func assertDisjoint(t *testing.T, evaluated, failing []uuid.UUID) {
	t.Helper()

	failed := make(map[uuid.UUID]struct{}, len(failing))
	for _, id := range failing {
		failed[id] = struct{}{}
	}

	for _, id := range evaluated {
		_, ok := failed[id]
		assert.False(t, ok, "rule %s is both evaluated and failing", id)
	}
}

// Without a matched DENY, the failure carries what the evaluation still
// established: the rules that did evaluate and the REVIEW rules that matched.
func TestCompleteEvaluator_EvaluateAll_IncompleteEvaluationState(t *testing.T) {
	t.Parallel()

	review := failureTestRule(8100, model.DecisionReview)
	failing := failureTestRule(8101, model.DecisionDeny)
	allow := failureTestRule(8102, model.DecisionAllow)
	secondFailing := failureTestRule(8103, model.DecisionReview)
	syntaxFailing := failureTestRule(8104, model.DecisionDeny)
	precisionFailing := failureTestRule(8105, model.DecisionDeny)
	unreached := failureTestRule(8106, model.DecisionDeny)

	tests := []struct {
		name          string
		outcomes      []ruleOutcome
		wantReview    []uuid.UUID
		wantEvaluated []uuid.UUID
		wantFailing   []uuid.UUID
		wantMatched   []uuid.UUID
		wantIs        error
		unevaluable   bool
	}{
		{
			name: "matched REVIEW and a failing rule",
			outcomes: []ruleOutcome{
				{rule: review, matched: true},
				{rule: failing, err: expressionFailure()},
				{rule: allow, matched: true},
			},
			wantReview:    []uuid.UUID{review.ID},
			wantEvaluated: []uuid.UUID{review.ID, allow.ID},
			wantFailing:   []uuid.UUID{failing.ID},
			wantMatched:   []uuid.UUID{review.ID, failing.ID},
			wantIs:        constant.ErrExpressionEvaluation,
			unevaluable:   true,
		},
		{
			name: "matched ALLOW and failing rules",
			outcomes: []ruleOutcome{
				{rule: failing, err: expressionFailure()},
				{rule: allow, matched: true},
				{rule: secondFailing, err: fmt.Errorf("%w: expected bool", constant.ErrExpressionType)},
			},
			wantReview:    []uuid.UUID{},
			wantEvaluated: []uuid.UUID{allow.ID},
			wantFailing:   []uuid.UUID{failing.ID, secondFailing.ID},
			wantMatched:   []uuid.UUID{failing.ID, secondFailing.ID},
			wantIs:        constant.ErrExpressionType,
			unevaluable:   true,
		},
		{
			// A precision failure is a fault of the request: it aborts the
			// evaluation and names only its own rule, dropping the unevaluable
			// rules collected before it.
			name: "syntax failure then precision failure",
			outcomes: []ruleOutcome{
				{rule: allow, matched: true},
				{rule: syntaxFailing, err: syntaxFailure()},
				{rule: precisionFailing, err: precisionFailure()},
				{rule: unreached},
			},
			wantReview:    []uuid.UUID{},
			wantEvaluated: []uuid.UUID{allow.ID},
			wantFailing:   []uuid.UUID{precisionFailing.ID},
			wantMatched:   []uuid.UUID{precisionFailing.ID},
			wantIs:        constant.ErrAmountExceedsPrecision,
			unevaluable:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			request := failureTestRequest()
			mockEval := NewMockSingleRuleEvaluator(ctrl)

			rules := make([]*model.Rule, 0, len(tt.outcomes))

			for _, o := range tt.outcomes {
				rules = append(rules, o.rule)

				if o.rule == unreached {
					continue
				}

				mockEval.EXPECT().Evaluate(gomock.Any(), o.rule, request).Return(o.matched, o.err).Times(1)
			}

			evaluator, err := NewCompleteEvaluator(mockEval)
			require.NoError(t, err)

			collector, err := evaluator.EvaluateAll(context.Background(), rules, request)
			require.Error(t, err)
			assert.Nil(t, collector)
			require.ErrorIs(t, err, tt.wantIs)
			assert.Equal(t, tt.unevaluable, IsUnevaluableRule(err))
			assert.Equal(t, tt.wantFailing, FailingRuleIDs(err))

			incomplete, ok := IncompleteEvaluationOf(err)
			require.True(t, ok, "an expression failure must carry the evaluation state")
			assert.Equal(t, tt.wantReview, incomplete.ReviewRuleIDs)
			assert.Equal(t, tt.wantEvaluated, incomplete.EvaluatedRuleIDs)
			assert.Equal(t, tt.wantMatched, incomplete.MatchedRuleIDs())
			assertDisjoint(t, incomplete.EvaluatedRuleIDs, FailingRuleIDs(err))
		})
	}
}

func TestIncompleteEvaluationOf_NotAnExpressionFailure(t *testing.T) {
	t.Parallel()

	for _, err := range []error{nil, fmt.Errorf("connection reset"), expressionFailure()} {
		incomplete, ok := IncompleteEvaluationOf(err)
		assert.False(t, ok)
		assert.Nil(t, incomplete)
	}
}

func TestEvaluateRulesQuery_Execute_IncompleteEvaluationCarriesTruncation(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	review := failureTestRule(8110, model.DecisionReview)
	failing := failureTestRule(8111, model.DecisionDeny)
	dropped := failureTestRule(8112, model.DecisionDeny)

	mockEval := NewMockSingleRuleEvaluator(ctrl)
	mockEval.EXPECT().Evaluate(gomock.Any(), review, gomock.Any()).Return(true, nil).Times(1)
	mockEval.EXPECT().Evaluate(gomock.Any(), failing, gomock.Any()).Return(false, expressionFailure()).Times(1)

	complete, err := NewCompleteEvaluator(mockEval)
	require.NoError(t, err)

	activeRules := NewMockGetActiveRulesExecutor(ctrl)
	activeRules.EXPECT().Execute(gomock.Any(), gomock.Any()).Return([]*model.Rule{review, failing, dropped}, nil).Times(1)

	q, err := NewEvaluateRulesQuery(activeRules, complete, &EvaluationConfig{
		DefaultDecisionWhenNoMatch: model.DecisionAllow,
		MaxRulesPerRequest:         2,
	})
	require.NoError(t, err)

	result, err := q.Execute(context.Background(), failureTestRequest())
	require.Error(t, err)
	assert.Nil(t, result)
	assert.True(t, IsUnevaluableRule(err))

	incomplete, ok := IncompleteEvaluationOf(err)
	require.True(t, ok)
	assert.Equal(t, 3, incomplete.TotalRulesLoaded)
	assert.True(t, incomplete.Truncated)
	assert.Equal(t, []uuid.UUID{review.ID}, incomplete.EvaluatedRuleIDs)
	assert.Equal(t, []uuid.UUID{review.ID, failing.ID}, incomplete.MatchedRuleIDs())
}

// Sequential: SetupTestTracing swaps the process-global tracer provider.
func TestRuleEvaluationLayers_SpanEventsCarryClassNotText(t *testing.T) {
	const secret = "secret-metadata-value"

	for _, tc := range []struct {
		name      string
		cause     error
		wantClass string
	}{
		{
			name:      "runtime evaluation failure",
			cause:     fmt.Errorf("%w: no such overload: %s", constant.ErrExpressionEvaluation, secret),
			wantClass: constant.ErrExpressionEvaluation.Error(),
		},
		{
			name: "amount beyond CEL precision",
			cause: fmt.Errorf("%w: failed to build activation: %w", constant.ErrExpressionEvaluation,
				fmt.Errorf("amount %s: %w", secret, constant.ErrAmountExceedsPrecision)),
			wantClass: constant.ErrAmountExceedsPrecision.Error(),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tracing := testutil.SetupTestTracing(t)
			ctrl := gomock.NewController(t)

			rule := failureTestRule(8120, model.DecisionDeny)
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

			_, err = q.Execute(context.Background(), failureTestRequest())
			require.ErrorIs(t, err, tc.cause)

			var events int

			for _, span := range tracing.Exporter.GetSpans() {
				for _, event := range span.Events {
					for _, attr := range event.Attributes {
						events++

						assert.NotContains(t, attr.Value.Emit(), secret, "span %s event %q leaks the error text", span.Name, event.Name)

						if attr.Key == "error" {
							assert.Equal(t, tc.wantClass, attr.Value.AsString(), "span %s event %q", span.Name, event.Name)
						}
					}
				}
			}

			assert.Positive(t, events, "the failure must still be recorded on the spans")
		})
	}
}

func domainOpResult(t *testing.T, reg *prometheus.Registry, operation string) map[string]float64 {
	t.Helper()

	families, err := reg.Gather()
	require.NoError(t, err)

	out := map[string]float64{}

	for _, family := range families {
		if family.GetName() != "domain_operations_total" {
			continue
		}

		for _, m := range family.GetMetric() {
			labels := labelMap(m)
			if labels["operation"] == operation {
				out[labels["result"]] += m.GetCounter().GetValue()
			}
		}
	}

	return out
}

func labelMap(m *dto.Metric) map[string]string {
	labels := map[string]string{}
	for _, l := range m.GetLabel() {
		labels[l.GetName()] = l.GetValue()
	}

	return labels
}

func TestEvaluateRulesQuery_Execute_ExpressionFailureIsBusinessMetric(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		cause      error
		wantResult string
	}{
		{name: "runtime evaluation failure", cause: expressionFailure(), wantResult: "business_error"},
		{name: "amount beyond CEL precision", cause: precisionFailure(), wantResult: "business_error"},
		{name: "infrastructure failure", cause: fmt.Errorf("connection reset"), wantResult: "technical_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			reg := prometheus.NewRegistry()

			factory, shutdown, err := observability.NewPrometheusBackedFactory(reg, nil)
			require.NoError(t, err)

			t.Cleanup(func() { _ = shutdown() })

			ctrl := gomock.NewController(t)
			rule := failureTestRule(8130, model.DecisionDeny)

			mockEval := NewMockSingleRuleEvaluator(ctrl)
			mockEval.EXPECT().Evaluate(gomock.Any(), rule, gomock.Any()).Return(false, tc.cause).Times(1)

			complete, err := NewCompleteEvaluator(mockEval)
			require.NoError(t, err)

			activeRules := NewMockGetActiveRulesExecutor(ctrl)
			activeRules.EXPECT().Execute(gomock.Any(), gomock.Any()).Return([]*model.Rule{rule}, nil).Times(1)

			q, err := NewEvaluateRulesQuery(activeRules, complete, &EvaluationConfig{DefaultDecisionWhenNoMatch: model.DecisionAllow})
			require.NoError(t, err)

			ctx := libObservability.ContextWithMetricFactory(context.Background(), factory)

			_, err = q.Execute(ctx, failureTestRequest())
			require.ErrorIs(t, err, tc.cause)

			assert.Equal(t, map[string]float64{tc.wantResult: 1}, domainOpResult(t, reg, "rules_evaluate"))
		})
	}
}
