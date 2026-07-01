// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package workers

import (
	"context"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/cache"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// stubReadyRepo is a minimal RuleSyncRepository used by the MarkReady tests.
// Returns a fixed slice of rules; no I/O.
type stubReadyRepo struct {
	rules []*model.Rule
}

func (s *stubReadyRepo) GetRulesUpdatedSince(_ context.Context, _ time.Time) ([]*model.Rule, error) {
	return s.rules, nil
}

// stubReadyCompiler always succeeds — cache-readiness is orthogonal to
// compilation, so we keep the compiler path trivial.
type stubReadyCompiler struct{}

func (s *stubReadyCompiler) Compile(_ context.Context, _ string) (any, error) {
	return "compiled", nil
}

// TestRuleSyncWorker_MarksReadyPerTenant verifies that the per-tenant sync
// worker flips the cache-readiness flag for its tenant bucket as soon as its
// first cycle completes. Without this, a freshly-spawned tenant worker would
// populate the cache (via ApplyChanges) but leave the IsReady gate closed,
// and every POST /v1/validations would return TRC-0281 until a manual rule
// activation fired the backstop in ActivateRuleService.
//
// The test uses a real *cache.RuleCache (not a mock) so it exercises the
// actual MarkReady implementation and the tenant-bucket partitioning.
func TestRuleSyncWorker_MarksReadyPerTenant(t *testing.T) {
	t.Parallel()

	clk := clock.RealClock{}
	ruleCache := cache.NewRuleCache(clk)
	logger := testutil.NewMockLogger()

	// Use stub repo + compiler so the cycle completes without I/O.
	repo := &stubReadyRepo{rules: []*model.Rule{}}
	compiler := &stubReadyCompiler{}

	worker, err := NewRuleSyncWorker(
		ruleCache, repo, compiler, defaultSyncConfig(), logger,
		defaultTestCircuitBreaker(), clk, "tenant-ready-a",
	)
	require.NoError(t, err)

	// Build a tenant-scoped context exactly the way runLoop would.
	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-ready-a")

	// Before the cycle, the tenant bucket MUST be unready — this is the
	// pre-condition for the bug (validation would return TRC-0281).
	require.False(t, ruleCache.IsReady(ctx),
		"pre-condition: tenant-ready-a cache bucket starts unready")

	// Other tenants must NOT be ready either — readiness is per-tenant.
	otherCtx := tmcore.ContextWithTenantID(context.Background(), "tenant-ready-b")
	require.False(t, ruleCache.IsReady(otherCtx),
		"pre-condition: unrelated tenant must stay unready")

	// Single cycle with no DB results — still MUST flip the readiness gate
	// because a tenant with zero rules should serve ALLOW via the
	// default-decision path, not block on TRC-0281.
	worker.runSyncCycle(ctx)

	assert.True(t, ruleCache.IsReady(ctx),
		"runSyncCycle must MarkReady the tenant-ready-a bucket even with empty fetch")
	assert.False(t, ruleCache.IsReady(otherCtx),
		"readiness MUST stay per-tenant — tenant-ready-b bucket must remain unready")
}

// TestRuleSyncWorker_MarksReadyAfterUpsertCycle verifies MarkReady is also
// called on the happy path where the cycle actually applies new rules.
func TestRuleSyncWorker_MarksReadyAfterUpsertCycle(t *testing.T) {
	t.Parallel()

	clk := clock.RealClock{}
	ruleCache := cache.NewRuleCache(clk)
	logger := testutil.NewMockLogger()

	rule := newSyncTestActiveRule(1)
	repo := &stubReadyRepo{rules: []*model.Rule{rule}}
	compiler := &stubReadyCompiler{}

	worker, err := NewRuleSyncWorker(
		ruleCache, repo, compiler, defaultSyncConfig(), logger,
		defaultTestCircuitBreaker(), clk, "tenant-upsert",
	)
	require.NoError(t, err)

	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-upsert")
	require.False(t, ruleCache.IsReady(ctx))

	worker.runSyncCycle(ctx)

	assert.True(t, ruleCache.IsReady(ctx),
		"apply-upserts path must flip readiness")
	assert.Equal(t, 1, ruleCache.Size(ctx),
		"the rule must actually land in the cache")
}
