// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/adapters/cel"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/query"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/pkg/tracercontract"
)

// These synthetic shapes measure the real exact-decimal evaluator. They are not
// production limits or latency targets. Entry counts 2/10/50 follow the Ledger
// engine report, 100 follows the batch posting cap, 1000 the v2 public leg cap,
// and 10000 the engine's expanded posting cap. These are work shapes, not typical
// traffic distributions. Each entry uses a distinct account to measure context
// preparation conservatively; real postings can share accounts. Every rule
// scans every entry and debit;
// there is no short-circuit on a matching decision. Network, database, locks,
// durable replay and Ledger enrichment are outside this CPU/allocation baseline.
func BenchmarkContextPolicySynthetic(b *testing.B) {
	for _, shape := range []struct{ entries, rules, integerDigits, fractionDigits int }{
		{2, 10, 18, 8},
		{4, 10, 18, 8},
		{10, 10, 18, 8},
		{50, 10, 18, 8},
		{100, 10, 18, 8},
		{1000, 10, 18, 8},
		{10000, 1, 18, 8},
		{64, 100, 18, 8},
		{256, 100, 128, 128},
		{64, 1000, 18, 8},
	} {
		name := fmt.Sprintf("accounts_%d_entries_%d_rules_%d_digits_%d_%d", shape.entries, shape.entries, shape.rules, shape.integerDigits, shape.fractionDigits)
		b.Run(name, func(b *testing.B) {
			facts, policy := syntheticPolicyFixture(shape.entries, shape.rules, shape.integerDigits, shape.fractionDigits)
			engine, err := cel.NewContextAdapter(cel.ContextAdapterConfig{
				Limits: tracercontract.Limits{
					MaxAccounts: shape.entries, MaxEntries: shape.entries, MaxTextBytes: 256,
					MaxIntegerDigits: shape.integerDigits + 4, MaxFractionDigits: shape.fractionDigits,
				},
				CostLimit: 1000000000, MaxExpressionBytes: 4096,
			})
			require.NoError(b, err)
			evaluator, err := query.NewContextPolicyEvaluator(engine, query.ContextPolicyConfig{MaxRules: shape.rules, TotalCost: 1000000000})
			require.NoError(b, err)
			wire, err := json.Marshal(facts)
			require.NoError(b, err)
			ctx := context.Background()
			compiled, err := evaluator.Compile(ctx, policy)
			require.NoError(b, err)
			probe, err := evaluator.Execute(ctx, compiled, facts, "synthetic")
			require.NoError(b, err)
			require.Equal(b, model.DecisionAllow, probe.Decision)
			require.Len(b, probe.EvaluatedRules, shape.rules)
			b.Run("evaluate", func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					if _, err := evaluator.Execute(ctx, compiled, facts, "synthetic"); err != nil {
						b.Fatal(err)
					}
				}
				b.ReportMetric(float64(probe.Cost), "CEL-cost/op")
				b.ReportMetric(float64(len(wire)), "context-bytes/op")
			})
			b.Run("compile", func(b *testing.B) {
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					if _, err := evaluator.Compile(ctx, policy); err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}

func syntheticPolicyFixture(accountCount, ruleCount, integerDigits, fractionDigits int) (tracercontract.Context, model.ContextPolicy) {
	asset := tracercontract.AssetRef{Namespace: "synthetic", ID: "native-asset", Code: "TOKEN"}
	amount := tracercontract.Amount(strings.Repeat("1", integerDigits) + "." + strings.Repeat("1", fractionDigits))
	facts := tracercontract.Context{Accounts: make([]tracercontract.Account, 0, accountCount), Entries: make([]tracercontract.Entry, 0, accountCount)}
	blocked := false
	for i := range accountCount {
		id := uuid.MustParse("11000000-0000-4000-8000-000000000000")
		id[14], id[15] = byte((i+1)>>8), byte(i+1)
		facts.Accounts = append(facts.Accounts, tracercontract.Account{ID: id, Type: "deposit", Status: "ACTIVE", Blocked: &blocked, Asset: asset})
		direction := tracercontract.Debit
		if i%2 != 0 {
			direction = tracercontract.Credit
		}
		facts.Entries = append(facts.Entries, tracercontract.Entry{AccountID: id, Direction: direction, Amount: amount, Asset: asset})
	}
	policy := model.ContextPolicy{
		ID: uuid.MustParse("22000000-0000-4000-8000-000000000001"), Revision: 1, DefaultDecision: model.DecisionDeny,
		Rules: make([]model.ContextPolicyRule, 0, ruleCount),
	}
	for i := range ruleCount {
		id := uuid.MustParse("33000000-0000-4000-8000-000000000000")
		id[14], id[15] = byte((i+1)>>8), byte(i+1)
		policy.Rules = append(policy.Rules, model.ContextPolicyRule{
			ID: id, Revision: 1, Action: model.DecisionAllow,
			Expression: `entries.all(e, e.amount.greaterThan(decimal("0"))) && debits.all(d, d.amount.greaterThan(decimal("0")))`,
		})
	}
	return facts, policy
}
