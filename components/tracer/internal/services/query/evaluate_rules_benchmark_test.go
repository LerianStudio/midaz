// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"go.uber.org/mock/gomock"

	"tracer/internal/testutil"
	"tracer/pkg/model"
)

// benchSink prevents compiler from optimizing away benchmark results.
var benchSink any

// Benchmark test data seeds - avoid magic numbers in test setup.
const (
	benchmarkRequestIDSeed = 999
	benchmarkAccountIDSeed = 1000
)

// BenchmarkEvaluateRules benchmarks the complete rule evaluation pipeline.
// Performance targets: <20ms for 100 rules, <50ms for 1000 rules.
func BenchmarkEvaluateRules(b *testing.B) {
	ruleCounts := []int{10, 100, 1000}

	// Scenario 1: All rules match (baseline - best case)
	for _, count := range ruleCounts {
		b.Run(fmt.Sprintf("all_match_%d_rules", count), func(b *testing.B) {
			ctrl := gomock.NewController(b)

			rules, txReq := setupBenchmarkData(count, model.DecisionAllow)
			mockGetActive, mockEvaluator := setupBenchmarkMocks(ctrl, rules)

			// All rules match
			mockEvaluator.EXPECT().
				EvaluateAll(gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, rules []*model.Rule, _ *model.ValidationRequest) (*EvaluationCollector, error) {
					collector := &EvaluationCollector{
						DenyRuleIDs:      make([]uuid.UUID, 0),
						AllowRuleIDs:     make([]uuid.UUID, 0, len(rules)),
						ReviewRuleIDs:    make([]uuid.UUID, 0),
						EvaluatedRuleIDs: make([]uuid.UUID, 0, len(rules)),
					}
					for _, r := range rules {
						collector.AllowRuleIDs = append(collector.AllowRuleIDs, r.ID)
						collector.EvaluatedRuleIDs = append(collector.EvaluatedRuleIDs, r.ID)
					}

					return collector, nil
				}).AnyTimes()

			config := defaultBenchConfig()
			query, err := NewEvaluateRulesQuery(mockGetActive, mockEvaluator, config)
			if err != nil {
				b.Fatal(err)
			}
			ctx := context.Background()

			b.ResetTimer()

			for b.Loop() {
				result, err := query.Execute(ctx, txReq)
				if err != nil {
					b.Fatal(err)
				}
				benchSink = result
			}
		})
	}

	// Scenario 2: Partial matches (50% match rate - realistic scenario)
	for _, count := range ruleCounts {
		b.Run(fmt.Sprintf("partial_match_%d_rules", count), func(b *testing.B) {
			ctrl := gomock.NewController(b)

			rules, txReq := setupBenchmarkData(count, model.DecisionAllow)
			mockGetActive, mockEvaluator := setupBenchmarkMocks(ctrl, rules)

			// 50% of rules match
			mockEvaluator.EXPECT().
				EvaluateAll(gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, rules []*model.Rule, _ *model.ValidationRequest) (*EvaluationCollector, error) {
					collector := &EvaluationCollector{
						DenyRuleIDs:      make([]uuid.UUID, 0),
						AllowRuleIDs:     make([]uuid.UUID, 0, len(rules)/2),
						ReviewRuleIDs:    make([]uuid.UUID, 0),
						EvaluatedRuleIDs: make([]uuid.UUID, 0, len(rules)),
					}

					for i, r := range rules {
						collector.EvaluatedRuleIDs = append(collector.EvaluatedRuleIDs, r.ID)
						if i%2 == 0 {
							collector.AllowRuleIDs = append(collector.AllowRuleIDs, r.ID)
						}
					}

					return collector, nil
				}).AnyTimes()

			config := defaultBenchConfig()
			query, err := NewEvaluateRulesQuery(mockGetActive, mockEvaluator, config)
			if err != nil {
				b.Fatal(err)
			}
			ctx := context.Background()

			b.ResetTimer()

			for b.Loop() {
				result, err := query.Execute(ctx, txReq)
				if err != nil {
					b.Fatal(err)
				}
				benchSink = result
			}
		})
	}

	// Scenario 3: Early DENY detection (DENY rule at position 5)
	for _, count := range ruleCounts {
		if count < 10 {
			continue
		}

		b.Run(fmt.Sprintf("early_deny_%d_rules", count), func(b *testing.B) {
			ctrl := gomock.NewController(b)

			rules, txReq := setupBenchmarkDataWithEarlyDeny(count, 5)
			mockGetActive, mockEvaluator := setupBenchmarkMocks(ctrl, rules)

			// All rules evaluated, one DENY at position 5
			mockEvaluator.EXPECT().
				EvaluateAll(gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, rules []*model.Rule, _ *model.ValidationRequest) (*EvaluationCollector, error) {
					collector := &EvaluationCollector{
						DenyRuleIDs:      make([]uuid.UUID, 0, 1),
						AllowRuleIDs:     make([]uuid.UUID, 0, len(rules)-1),
						ReviewRuleIDs:    make([]uuid.UUID, 0),
						EvaluatedRuleIDs: make([]uuid.UUID, 0, len(rules)),
					}

					for i, r := range rules {
						collector.EvaluatedRuleIDs = append(collector.EvaluatedRuleIDs, r.ID)
						if r.Action == model.DecisionDeny {
							collector.DenyRuleIDs = append(collector.DenyRuleIDs, r.ID)
						} else if i%2 == 0 {
							collector.AllowRuleIDs = append(collector.AllowRuleIDs, r.ID)
						}
					}

					return collector, nil
				}).AnyTimes()

			config := defaultBenchConfig()
			query, err := NewEvaluateRulesQuery(mockGetActive, mockEvaluator, config)
			if err != nil {
				b.Fatal(err)
			}
			ctx := context.Background()

			b.ResetTimer()

			for b.Loop() {
				result, err := query.Execute(ctx, txReq)
				if err != nil {
					b.Fatal(err)
				}
				benchSink = result
			}
		})
	}

	// Scenario 4: Simulated CEL latency (adds ~100us per evaluation)
	for _, count := range ruleCounts {
		if count > 100 {
			continue // Skip large counts for latency simulation
		}

		b.Run(fmt.Sprintf("cel_latency_%d_rules", count), func(b *testing.B) {
			ctrl := gomock.NewController(b)

			rules, txReq := setupBenchmarkData(count, model.DecisionAllow)
			mockGetActive, mockEvaluator := setupBenchmarkMocks(ctrl, rules)

			// Simulate realistic CEL evaluation cost (~100 microseconds total)
			mockEvaluator.EXPECT().
				EvaluateAll(gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, rules []*model.Rule, _ *model.ValidationRequest) (*EvaluationCollector, error) {
					// Simulate CEL latency
					time.Sleep(time.Duration(len(rules)) * 100 * time.Microsecond)

					collector := &EvaluationCollector{
						DenyRuleIDs:      make([]uuid.UUID, 0),
						AllowRuleIDs:     make([]uuid.UUID, 0, len(rules)),
						ReviewRuleIDs:    make([]uuid.UUID, 0),
						EvaluatedRuleIDs: make([]uuid.UUID, 0, len(rules)),
					}

					for _, r := range rules {
						collector.AllowRuleIDs = append(collector.AllowRuleIDs, r.ID)
						collector.EvaluatedRuleIDs = append(collector.EvaluatedRuleIDs, r.ID)
					}

					return collector, nil
				}).AnyTimes()

			config := defaultBenchConfig()
			query, err := NewEvaluateRulesQuery(mockGetActive, mockEvaluator, config)
			if err != nil {
				b.Fatal(err)
			}
			ctx := context.Background()

			b.ResetTimer()

			for b.Loop() {
				result, err := query.Execute(ctx, txReq)
				if err != nil {
					b.Fatal(err)
				}
				benchSink = result
			}
		})
	}
}

// BenchmarkRuleScopesMatch benchmarks scope matching performance.
func BenchmarkRuleScopesMatch(b *testing.B) {
	accountID := testutil.MustDeterministicUUID(1)
	segmentID := testutil.MustDeterministicUUID(2)
	txType := model.TransactionTypeCard

	ruleScopes := []model.Scope{
		{AccountID: &accountID},
		{SegmentID: &segmentID},
		{TransactionType: &txType},
	}

	txScope := &model.Scope{
		AccountID:       &accountID,
		TransactionType: &txType,
	}

	b.ResetTimer()

	for b.Loop() {
		benchSink = model.RuleScopesMatch(ruleScopes, txScope)
	}
}

// BenchmarkDecisionMaker benchmarks decision making with various input sizes.
func BenchmarkDecisionMaker(b *testing.B) {
	sizes := []int{4, 10, 100, 1000}

	for _, size := range sizes {
		b.Run(fmt.Sprintf("decision_%d_rules", size), func(b *testing.B) {
			// Ensure minimum counts for each category
			denyCount := max(size/4, 1)
			allowCount := max(size/2, 1)
			reviewCount := max(size/4, 1)

			denyIDs := testutil.MustDeterministicUUIDs(denyCount, 0)
			allowIDs := testutil.MustDeterministicUUIDs(allowCount, int64(size))
			reviewIDs := testutil.MustDeterministicUUIDs(reviewCount, int64(size*2))
			evaluatedIDs := testutil.MustDeterministicUUIDs(size, int64(size*3))

			maker := model.NewDecisionMaker()

			b.ResetTimer()

			for b.Loop() {
				result, err := maker.MakeDecision(denyIDs, allowIDs, reviewIDs, evaluatedIDs, model.DecisionAllow)
				if err != nil {
					b.Fatal(err)
				}

				benchSink = result
			}
		})
	}
}

// Benchmark helper functions

func setupBenchmarkData(count int, action model.Decision) ([]*model.Rule, *model.ValidationRequest) {
	rules := make([]*model.Rule, count)

	for i := range count {
		rules[i] = &model.Rule{
			ID:         testutil.MustDeterministicUUID(int64(i)),
			Name:       fmt.Sprintf("Rule %d", i),
			Action:     action,
			Expression: "amount > 10",
			Scopes:     []model.Scope{},
		}
	}

	txReq := &model.ValidationRequest{
		RequestID:       testutil.MustDeterministicUUID(benchmarkRequestIDSeed),
		TransactionType: model.TransactionTypeCard,
		Amount:          decimal.RequireFromString("50"),
		Currency:        "USD",
		Account:         model.AccountContext{ID: testutil.MustDeterministicUUID(benchmarkAccountIDSeed), Type: "checking"},
	}

	return rules, txReq
}

func setupBenchmarkDataWithEarlyDeny(count, denyPosition int) ([]*model.Rule, *model.ValidationRequest) {
	rules, txReq := setupBenchmarkData(count, model.DecisionAllow)

	if denyPosition < count {
		rules[denyPosition].Action = model.DecisionDeny
	}

	return rules, txReq
}

func setupBenchmarkMocks(ctrl *gomock.Controller, rules []*model.Rule) (*MockGetActiveRulesExecutor, *MockCompleteRuleEvaluator) {
	mockGetActive := NewMockGetActiveRulesExecutor(ctrl)
	mockEvaluator := NewMockCompleteRuleEvaluator(ctrl)

	mockGetActive.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(rules, nil).AnyTimes()

	return mockGetActive, mockEvaluator
}

func defaultBenchConfig() *EvaluationConfig {
	return &EvaluationConfig{
		DefaultDecisionWhenNoMatch: model.DecisionAllow,
		MaxRulesPerRequest:         100000,
	}
}
