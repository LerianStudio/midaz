// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package cache_test

import (
	"context"
	"testing"

	"tracer/internal/services/cache"
	"tracer/internal/testutil"
	"tracer/pkg/clock"
	"tracer/pkg/model"
)

// benchResult prevents compiler from optimizing away GetActiveRules calls.
var benchResult []*cache.CachedRule

func benchmarkGetActiveRules(b *testing.B, count int) {
	ctx := context.Background()
	c := cache.NewRuleCache(clock.New())
	rules := make([]*cache.CachedRule, count)
	for i := range rules {
		rules[i] = newTestCachedRule(newTestRule(int64(i + 1)))
	}
	c.SetRules(ctx, rules)
	c.MarkReady(ctx)

	for b.Loop() {
		benchResult = c.GetActiveRules(ctx, nil)
	}
}

func BenchmarkRuleCache_GetActiveRules_100Rules(b *testing.B)   { benchmarkGetActiveRules(b, 100) }
func BenchmarkRuleCache_GetActiveRules_1000Rules(b *testing.B)  { benchmarkGetActiveRules(b, 1000) }
func BenchmarkRuleCache_GetActiveRules_10000Rules(b *testing.B) { benchmarkGetActiveRules(b, 10000) }

func BenchmarkRuleCache_GetActiveRules_ScopeFiltered_1000Rules(b *testing.B) {
	ctx := context.Background()
	c := cache.NewRuleCache(clock.New())
	accountID := testutil.MustDeterministicUUID(100)

	rules := make([]*cache.CachedRule, 1000)
	for i := range rules {
		if i%10 == 0 {
			// 10% of rules scoped to target account
			rules[i] = newTestCachedRule(newTestRuleWithScope(int64(i+1), model.Scope{
				AccountID: testutil.UUIDPtr(accountID),
			}))
		} else {
			rules[i] = newTestCachedRule(newTestRule(int64(i + 1)))
		}
	}

	c.SetRules(ctx, rules)
	c.MarkReady(ctx)

	txScope := &model.Scope{AccountID: testutil.UUIDPtr(accountID)}

	for b.Loop() {
		benchResult = c.GetActiveRules(ctx, txScope)
	}
}
