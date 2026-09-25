// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query_test

import (
	"context"
	"sync"
	"testing"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/cel"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

func policyEngine(t testing.TB) *cel.ContextAdapter {
	t.Helper()
	engine, err := cel.NewContextAdapter(cel.ContextAdapterConfig{
		Limits:    tracercontract.Limits{MaxAccounts: 10, MaxEntries: 20, MaxTextBytes: 256, MaxIntegerDigits: 128, MaxFractionDigits: 128},
		CostLimit: 100000, MaxExpressionBytes: 5000,
	})
	require.NoError(t, err)
	return engine
}

func policyFacts() tracercontract.Context {
	blocked := false
	id := uuid.MustParse("550e8400-e29b-41d4-a716-446655440001")
	asset := tracercontract.AssetRef{Namespace: "producer", ID: "asset-1", Code: "BTC"}
	return tracercontract.Context{
		Accounts: []tracercontract.Account{{ID: id, Type: "deposit", Status: "ACTIVE", Blocked: &blocked, Asset: asset}},
		Entries:  []tracercontract.Entry{{AccountID: id, Direction: tracercontract.Debit, Amount: "0.00000001", Asset: asset}},
	}
}

func policySnapshot() model.ContextPolicy {
	return model.ContextPolicy{
		ID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440010"), Revision: 7, DefaultDecision: model.DecisionDeny,
		Rules: []model.ContextPolicyRule{
			{ID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440002"), Revision: 3, Action: model.DecisionReview, Expression: `accounts.exists(a, a.type == "deposit")`},
			{ID: uuid.MustParse("550e8400-e29b-41d4-a716-446655440001"), Revision: 2, Action: model.DecisionAllow, Expression: `accounts.exists(a, a.type == "deposit")`},
		},
	}
}

func policyEvaluator(t testing.TB, engine *cel.ContextAdapter, budget uint64) *query.ContextPolicyEvaluator {
	t.Helper()
	evaluator, err := query.NewContextPolicyEvaluator(engine, query.ContextPolicyConfig{MaxRules: 10, TotalCost: budget})
	require.NoError(t, err)
	return evaluator
}

func TestContextPolicyPrecedenceAndExplicitDefault(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name            string
		actions         []model.Decision
		defaultDecision model.Decision
		want            model.Decision
	}{
		{"deny beats review and allow", []model.Decision{model.DecisionAllow, model.DecisionReview, model.DecisionDeny}, model.DecisionAllow, model.DecisionDeny},
		{"review beats allow", []model.Decision{model.DecisionAllow, model.DecisionReview}, model.DecisionDeny, model.DecisionReview},
		{"matching allow beats deny default", []model.Decision{model.DecisionAllow}, model.DecisionDeny, model.DecisionAllow},
		{"no match uses explicit deny", nil, model.DecisionDeny, model.DecisionDeny},
		{"no match uses explicit allow", nil, model.DecisionAllow, model.DecisionAllow},
	} {
		t.Run(tc.name, func(t *testing.T) {
			evaluator := policyEvaluator(t, policyEngine(t), 100000)
			input := policySnapshot()
			input.DefaultDecision = tc.defaultDecision
			input.Rules = nil
			for i, action := range tc.actions {
				id := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
				id[15] = byte(i + 1)
				input.Rules = append(input.Rules, model.ContextPolicyRule{ID: id, Revision: 1, Action: action, Expression: "true"})
			}
			compiled, err := evaluator.Compile(context.Background(), input)
			require.NoError(t, err)
			result, err := evaluator.Execute(context.Background(), compiled, policyFacts(), "producer")
			require.NoError(t, err)
			require.Equal(t, tc.want, result.Decision)
			require.Len(t, result.EvaluatedRules, len(tc.actions))
			require.Len(t, result.MatchedRules, len(tc.actions))
			require.Equal(t, input.ID, result.PolicyID)
			require.Equal(t, input.Revision, result.PolicyRevision)
		})
	}
}

func TestContextPolicySnapshotAndRevisionOrder(t *testing.T) {
	t.Parallel()
	evaluator := policyEvaluator(t, policyEngine(t), 100000)
	input := policySnapshot()
	compiled, err := evaluator.Compile(context.Background(), input)
	require.NoError(t, err)
	input.DefaultDecision = model.DecisionAllow
	input.Rules[0].Action = model.DecisionDeny
	input.Rules[0].Revision = 99
	input.Rules[0].Expression = "false"
	result, err := evaluator.Execute(context.Background(), compiled, policyFacts(), "producer")
	require.NoError(t, err)
	require.Equal(t, model.DecisionReview, result.Decision)
	require.Len(t, result.EvaluatedRules, 2)
	require.Equal(t, input.Rules[1].ID, result.EvaluatedRules[0].ID)
	require.EqualValues(t, 3, result.EvaluatedRules[1].Revision)
	require.Equal(t, result.EvaluatedRules, result.MatchedRules)
	result.MatchedRules[0].Revision = 90
	again, err := evaluator.Execute(context.Background(), compiled, policyFacts(), "producer")
	require.NoError(t, err)
	require.EqualValues(t, 2, again.MatchedRules[0].Revision)
}

func TestContextPolicyTotalBudgetAcrossRules(t *testing.T) {
	t.Parallel()
	engine := policyEngine(t)
	input := policySnapshot()
	program, err := engine.Compile(context.Background(), input.Rules[0].Expression)
	require.NoError(t, err)
	activation, err := engine.Prepare(context.Background(), policyFacts(), "producer")
	require.NoError(t, err)
	_, perRule, err := engine.Evaluate(context.Background(), program, activation, 100000)
	require.NoError(t, err)
	require.Positive(t, perRule)
	estimatedTotal := program.EstimatedMaxCost() * 2
	require.GreaterOrEqual(t, estimatedTotal, perRule*2)
	for _, budget := range []uint64{program.EstimatedMaxCost(), estimatedTotal - 1, estimatedTotal} {
		evaluator := policyEvaluator(t, engine, budget)
		compiled, err := evaluator.Compile(context.Background(), input)
		if budget < estimatedTotal {
			require.ErrorIs(t, err, constant.ErrExpressionCostExceeded)
			require.Nil(t, compiled)
			continue
		}
		require.NoError(t, err)
		for range 2 { // Cached policies retain independent runtime budgets.
			result, err := evaluator.Execute(context.Background(), compiled, policyFacts(), "producer")
			require.NoError(t, err)
			require.Equal(t, perRule*2, result.Cost)
		}
	}
}

func TestContextPolicyRejectsAggregateCostBeforeActivation(t *testing.T) {
	evaluator := policyEvaluator(t, policyEngine(t), 1)
	compiled, err := evaluator.Compile(t.Context(), policySnapshot())
	require.ErrorIs(t, err, constant.ErrExpressionCostExceeded)
	require.Nil(t, compiled)
}

func TestContextPolicyErrorsNeverBecomeDecisions(t *testing.T) {
	t.Parallel()
	evaluator := policyEvaluator(t, policyEngine(t), 100000)
	input := policySnapshot()
	input.Rules[1].Action = model.DecisionDeny // First by ID, but cannot mask an error later.
	input.Rules[0].Expression = `{"present": 1}["missing"] == 1`
	compiled, err := evaluator.Compile(context.Background(), input)
	require.NoError(t, err)
	result, err := evaluator.Execute(context.Background(), compiled, policyFacts(), "producer")
	require.ErrorIs(t, err, constant.ErrExpressionEvaluation)
	require.Nil(t, result)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = evaluator.Execute(ctx, compiled, policyFacts(), "producer")
	require.ErrorIs(t, err, context.Canceled)
	_, err = evaluator.Execute(context.Background(), nil, policyFacts(), "producer")
	require.Error(t, err)
	other := policyEvaluator(t, policyEngine(t), 100000)
	_, err = other.Execute(context.Background(), compiled, policyFacts(), "producer")
	require.Error(t, err)
	input.Rules = nil
	input.DefaultDecision = model.DecisionAllow
	compiled, err = evaluator.Compile(context.Background(), input)
	require.NoError(t, err)
	facts := policyFacts()
	facts.Accounts[0].Blocked = nil
	_, err = evaluator.Execute(context.Background(), compiled, facts, "producer")
	require.ErrorIs(t, err, constant.ErrInvalidRequestBody)
}

func TestContextPolicyRejectsInvalidConfigurationWithoutTruncation(t *testing.T) {
	t.Parallel()
	evaluator := policyEvaluator(t, policyEngine(t), 100000)
	for _, mutate := range []func(*model.ContextPolicy){
		func(p *model.ContextPolicy) { p.ID = uuid.Nil },
		func(p *model.ContextPolicy) { p.Revision = 0 },
		func(p *model.ContextPolicy) { p.DefaultDecision = "" },
		func(p *model.ContextPolicy) { p.DefaultDecision = model.DecisionReview },
		func(p *model.ContextPolicy) { p.Rules[0].Revision = 0 },
		func(p *model.ContextPolicy) { p.Rules[0].ID = uuid.Nil },
		func(p *model.ContextPolicy) { p.Rules[0].ID = p.Rules[1].ID },
		func(p *model.ContextPolicy) { p.Rules[0].Action = "UNKNOWN" },
		func(p *model.ContextPolicy) { p.Rules[0].Expression = `amount > 10` },
		func(p *model.ContextPolicy) {
			p.Rules = make([]model.ContextPolicyRule, 11)
			for i := range p.Rules {
				id := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
				id[15] = byte(i + 1)
				p.Rules[i] = model.ContextPolicyRule{ID: id, Revision: 1, Action: model.DecisionAllow, Expression: "true"}
			}
			p.Rules[10].Action = model.DecisionDeny
		},
	} {
		input := policySnapshot()
		mutate(&input)
		compiled, err := evaluator.Compile(context.Background(), input)
		require.Error(t, err)
		require.Nil(t, compiled)
	}
}

func TestContextPolicySpanErrorClassification(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name          string
		invalidPolicy bool
		want          codes.Code
	}{
		{"invalid snapshot is a business error", true, codes.Unset},
		{"runtime failure is a technical error", false, codes.Error},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := tracetest.NewSpanRecorder()
			provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
			t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
			ctx := libObservability.ContextWithTracer(context.Background(), provider.Tracer("policy-test"))
			evaluator := policyEvaluator(t, policyEngine(t), 100000)
			input := policySnapshot()
			spanName := "query.evaluate_context_policy"
			if tc.invalidPolicy {
				input.Revision = 0
				_, err := evaluator.Compile(ctx, input)
				require.Error(t, err)
				spanName = "query.compile_context_policy"
			} else {
				input.Rules[0].Expression = `{"present": 1}["missing"] == 1`
				compiled, err := evaluator.Compile(ctx, input)
				require.NoError(t, err)
				_, err = evaluator.Execute(ctx, compiled, policyFacts(), "producer")
				require.Error(t, err)
			}
			found := false
			for _, span := range recorder.Ended() {
				if span.Name() == spanName {
					found = true
					require.Equal(t, tc.want, span.Status().Code)
				}
			}
			require.True(t, found)
		})
	}
}

func TestContextPolicyConcurrentRequestsKeepIndependentFactsAndBudgets(t *testing.T) {
	t.Parallel()
	evaluator := policyEvaluator(t, policyEngine(t), 100000)
	compiled, err := evaluator.Compile(context.Background(), policySnapshot())
	require.NoError(t, err)
	var workers sync.WaitGroup
	for i := range 8 {
		workers.Go(func() {
			facts := policyFacts()
			want := model.DecisionReview
			if i%2 == 0 {
				facts.Accounts[0].Type = "future-account-type"
				want = model.DecisionDeny // No matching rule, use this policy's default.
			}
			result, err := evaluator.Execute(context.Background(), compiled, facts, "producer")
			if err != nil {
				t.Errorf("concurrent policy evaluation failed: %v", err)
				return
			}
			if result.Decision != want || len(result.EvaluatedRules) != 2 || result.Cost == 0 {
				t.Errorf("unexpected decision, evaluated rules or cost: %+v", result)
			}
		})
	}
	workers.Wait()
}
