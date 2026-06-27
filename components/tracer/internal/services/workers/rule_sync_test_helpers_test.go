// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package workers

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/cache"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/resilience"
)

// NOTE: For mock clock, use testutil.MockClock directly.
// It provides FixedTime, TickerChan fields and sync.Once-protected stop.
// Example: clk := testutil.MockClock{FixedTime: testutil.FixedTime()}

// newSyncTestRule creates a model.Rule with sensible defaults for sync worker tests.
func newSyncTestRule(base int64, status model.RuleStatus) *model.Rule {
	id := testutil.MustDeterministicUUID(base)
	now := testutil.FixedTime()

	return &model.Rule{
		ID:         id,
		Name:       "sync-test-rule-" + id.String()[:8],
		Expression: "amount > 1000",
		Action:     model.DecisionDeny,
		Status:     status,
		Scopes:     []model.Scope{},
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

// newSyncTestActiveRule creates an ACTIVE rule.
func newSyncTestActiveRule(base int64) *model.Rule {
	return newSyncTestRule(base, model.RuleStatusActive)
}

// newSyncTestCachedRule creates a CachedRule from a rule with a fake compiled program.
func newSyncTestCachedRule(rule *model.Rule) *cache.CachedRule {
	return &cache.CachedRule{
		Rule:    rule,
		Program: "compiled:" + rule.Expression,
	}
}

// buildCachedMap converts a slice of CachedRules into a map keyed by Rule.ID.
// Used to prepare the `cached` argument for ClassifyChanges.
func buildCachedMap(t *testing.T, rules ...*cache.CachedRule) map[uuid.UUID]*cache.CachedRule {
	t.Helper()

	m := make(map[uuid.UUID]*cache.CachedRule, len(rules))

	for _, r := range rules {
		if r == nil || r.Rule == nil {
			t.Fatal("buildCachedMap: received nil CachedRule or nil CachedRule.Rule")
		}

		m[r.Rule.ID] = r
	}

	return m
}

// defaultTestCircuitBreaker returns a circuit breaker with permissive test settings.
func defaultTestCircuitBreaker() *resilience.CircuitBreaker {
	cfg := resilience.CircuitBreakerConfig{
		Name:          "test",
		MaxRequests:   1,
		Interval:      0,
		Timeout:       1 * time.Second,
		FailureThresh: 5,
		FailureRatio:  0,
		MinRequests:   0,
	}

	return resilience.NewCircuitBreaker(cfg, testutil.NewMockLogger())
}

// defaultSyncConfig returns a test-friendly config with short intervals.
func defaultSyncConfig() RuleSyncWorkerConfig {
	return RuleSyncWorkerConfig{
		PollInterval:       100 * time.Millisecond,
		StalenessThreshold: 500 * time.Millisecond,
		OverlapBuffer:      50 * time.Millisecond,
	}
}

// assertChangeSetEmpty verifies a ChangeSet has no changes.
func assertChangeSetEmpty(t *testing.T, cs ChangeSet) {
	t.Helper()

	if !cs.IsEmpty() {
		t.Errorf("expected empty ChangeSet, got New=%d Updated=%d Deleted=%d",
			len(cs.New), len(cs.Updated), len(cs.Deleted))
	}
}
