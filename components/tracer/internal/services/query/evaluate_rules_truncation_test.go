// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// truncationRequest is the same transaction on every call, so any difference in
// the evaluated set comes from the cut and not from the request.
func truncationRequest() *model.ValidationRequest {
	return &model.ValidationRequest{
		RequestID:       testutil.MustDeterministicUUID(900),
		TransactionType: model.TransactionTypeCard,
		Amount:          decimal.RequireFromString("150"),
		Asset:           "USD",
		Account:         model.AccountContext{ID: testutil.MustDeterministicUUID(901), Type: "checking"},
	}
}

// runTruncation drives one evaluation over the given candidate set and returns
// the rule set the evaluator actually received.
func runTruncation(t *testing.T, candidates []*model.Rule, ceiling int) []*model.Rule {
	t.Helper()

	ctrl := gomock.NewController(t)

	mockGetActive := NewMockGetActiveRulesExecutor(ctrl)
	mockEvaluator := NewMockCompleteRuleEvaluator(ctrl)

	// The query sorts the slice in place, so hand it a copy per call: a caller
	// that reuses one backing array would see the first call's ordering leak
	// into the second and hide a non-deterministic cut.
	handed := make([]*model.Rule, len(candidates))
	copy(handed, candidates)

	mockGetActive.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(handed, nil)

	var evaluated []*model.Rule

	mockEvaluator.EXPECT().
		EvaluateAll(gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, rules []*model.Rule, _ *model.ValidationRequest) (*EvaluationCollector, error) {
			evaluated = append([]*model.Rule(nil), rules...)

			return &EvaluationCollector{
				DenyRuleIDs:      []uuid.UUID{},
				AllowRuleIDs:     []uuid.UUID{},
				ReviewRuleIDs:    []uuid.UUID{},
				EvaluatedRuleIDs: []uuid.UUID{},
			}, nil
		})

	q, err := NewEvaluateRulesQuery(mockGetActive, mockEvaluator, &EvaluationConfig{
		DefaultDecisionWhenNoMatch: model.DecisionAllow,
		MaxRulesPerRequest:         ceiling,
	})
	require.NoError(t, err)

	_, err = q.Execute(context.Background(), truncationRequest())
	require.NoError(t, err)

	return evaluated
}

func ruleIDs(rules []*model.Rule) []uuid.UUID {
	ids := make([]uuid.UUID, 0, len(rules))
	for _, rule := range rules {
		ids = append(ids, rule.ID)
	}

	return ids
}

// TestEvaluateRulesQuery_TruncationKeepsTheDenyRules pins that the ceiling never
// discards a rule that denies the transaction while keeping a weaker one. The
// candidate set arrives in Go map order, so before the fix the single DENY rule
// survived the cut only when the map happened to iterate it early.
func TestEvaluateRulesQuery_TruncationKeepsTheDenyRules(t *testing.T) {
	const (
		total   = 20
		ceiling = 5
	)

	denyID := testutil.MustDeterministicUUID(999)

	candidates := make([]*model.Rule, 0, total)
	for i := range total - 1 {
		candidates = append(candidates, &model.Rule{
			ID:         testutil.MustDeterministicUUID(int64(i) + 1),
			Name:       fmt.Sprintf("allow-%d", i),
			Expression: "true",
			Action:     model.DecisionAllow,
			Scopes:     []model.Scope{},
		})
	}

	// The DENY rule sits last, which is exactly the position an unordered cut
	// discards.
	candidates = append(candidates, &model.Rule{
		ID:         denyID,
		Name:       "deny-high-value",
		Expression: "amount > 100",
		Action:     model.DecisionDeny,
		Scopes:     []model.Scope{},
	})

	evaluated := runTruncation(t, candidates, ceiling)

	require.Len(t, evaluated, ceiling)
	assert.Contains(t, ruleIDs(evaluated), denyID,
		"the ceiling discarded the rule that denies the transaction and kept rules that allow it")
	assert.Equal(t, model.DecisionDeny, evaluated[0].Action,
		"the strictest rule must lead the set the ceiling keeps")
}

// TestEvaluateRulesQuery_TruncationIsReproducible pins that two identical
// validations evaluate the same rules. The candidate set is handed over in a
// different input order each time, standing in for the randomised map order the
// rule cache produces.
func TestEvaluateRulesQuery_TruncationIsReproducible(t *testing.T) {
	const (
		total   = 30
		ceiling = 7
	)

	forward := make([]*model.Rule, 0, total)

	for i := range total {
		action := model.DecisionAllow
		if i%3 == 0 {
			action = model.DecisionReview
		}

		forward = append(forward, &model.Rule{
			ID:         testutil.MustDeterministicUUID(int64(i) + 1),
			Name:       fmt.Sprintf("rule-%d", i),
			Expression: "true",
			Action:     action,
			Scopes:     []model.Scope{},
		})
	}

	reversed := make([]*model.Rule, 0, total)
	for i := total - 1; i >= 0; i-- {
		reversed = append(reversed, forward[i])
	}

	first := runTruncation(t, forward, ceiling)
	second := runTruncation(t, reversed, ceiling)

	require.Len(t, first, ceiling)
	require.Len(t, second, ceiling)
	assert.Equal(t, ruleIDs(first), ruleIDs(second),
		"the same transaction evaluated a different rule subset on the second call")
}

// TestOrderRulesForTruncation_RanksByDecisionPrecedence pins the order itself:
// DENY, then REVIEW, then ALLOW, then anything unrecognised, with the rule id
// breaking ties. A nil entry must not panic and must sort last.
func TestOrderRulesForTruncation_RanksByDecisionPrecedence(t *testing.T) {
	allowA := &model.Rule{ID: testutil.MustDeterministicUUID(40), Action: model.DecisionAllow}
	allowB := &model.Rule{ID: testutil.MustDeterministicUUID(10), Action: model.DecisionAllow}
	review := &model.Rule{ID: testutil.MustDeterministicUUID(50), Action: model.DecisionReview}
	deny := &model.Rule{ID: testutil.MustDeterministicUUID(60), Action: model.DecisionDeny}
	unknown := &model.Rule{ID: testutil.MustDeterministicUUID(70), Action: model.Decision("SOMETHING_ELSE")}

	// The two ALLOW rules tie on rank, so the rule id decides which comes first.
	allowFirst, allowSecond := allowA, allowB
	if allowB.ID.String() < allowA.ID.String() {
		allowFirst, allowSecond = allowB, allowA
	}

	rules := []*model.Rule{allowA, nil, unknown, allowB, review, deny}

	orderRulesForTruncation(rules)

	assert.Equal(t, []*model.Rule{deny, review, allowFirst, allowSecond, unknown, nil}, rules)
}
