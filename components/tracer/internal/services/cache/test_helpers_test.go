// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package cache_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/cache"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// newTestRule creates a model.Rule with sensible defaults and a deterministic UUID.
// Override fields after creation for specific test scenarios.
func newTestRule(base int64) *model.Rule {
	id := testutil.MustDeterministicUUID(base)
	now := testutil.FixedTime()

	return &model.Rule{
		ID:         id,
		Name:       "test-rule-" + id.String()[:8],
		Expression: "amount > 1000",
		Action:     model.DecisionDeny,
		Status:     model.RuleStatusActive,
		Scopes:     []model.Scope{},
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

// newTestCachedRule creates a CachedRule from a Rule and a fake program.
func newTestCachedRule(rule *model.Rule) *cache.CachedRule {
	return &cache.CachedRule{
		Rule:    rule,
		Program: "compiled:" + rule.Expression,
	}
}

// newTestRuleWithScope creates a rule with a specific scope for scope-filtering tests.
func newTestRuleWithScope(base int64, scope model.Scope) *model.Rule {
	rule := newTestRule(base)
	rule.Scopes = []model.Scope{scope}

	return rule
}

// assertCacheSize verifies the cache contains the expected number of rules.
func assertCacheSize(t *testing.T, c *cache.RuleCache, expected int) {
	t.Helper()
	assert.Equal(t, expected, c.Size(context.Background()), "cache size mismatch")
}

// assertCacheContains verifies the cache contains rules with the given IDs.
func assertCacheContains(t *testing.T, c *cache.RuleCache, ids ...uuid.UUID) {
	t.Helper()

	rules := c.GetActiveRules(context.Background(), nil)
	ruleIDs := make(map[uuid.UUID]bool, len(rules))

	for _, r := range rules {
		ruleIDs[r.Rule.ID] = true
	}

	for _, id := range ids {
		assert.True(t, ruleIDs[id], "cache should contain rule %s", id)
	}
}

// assertCacheReady verifies the cache reports as ready.
func assertCacheReady(t *testing.T, c *cache.RuleCache) {
	t.Helper()
	require.True(t, c.IsReady(context.Background()), "cache should be ready")
}

// assertCacheNotReady verifies the cache reports as not ready.
func assertCacheNotReady(t *testing.T, c *cache.RuleCache) {
	t.Helper()
	require.False(t, c.IsReady(context.Background()), "cache should not be ready")
}
