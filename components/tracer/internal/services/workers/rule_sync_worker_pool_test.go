// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package workers

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/services/cache"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/clock"
	"github.com/LerianStudio/midaz/v4/components/tracer/pkg/model"
)

// fakeTenantDB is a minimal dbresolver.DB stub. We never execute against it —
// the tests only need a non-nil, type-checkable value to assert the worker
// stashed it onto the cycle context via tmcore.ContextWithPG.
type fakeTenantDB struct {
	dbresolver.DB
	tag string
}

// stubPoolResolver records the tenantID passed to GetTenantDB and returns a
// pre-seeded response. Lets the test assert the per-cycle resolution path
// without needing lib-commons' Manager.
type stubPoolResolver struct {
	mu    sync.Mutex
	db    dbresolver.DB
	err   error
	calls []string
}

func (s *stubPoolResolver) GetTenantDB(_ context.Context, tenantID string) (dbresolver.DB, error) {
	s.mu.Lock()
	s.calls = append(s.calls, tenantID)

	db, err := s.db, s.err
	s.mu.Unlock()

	return db, err
}

func (s *stubPoolResolver) recordedCalls() []string {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]string, len(s.calls))
	copy(out, s.calls)

	return out
}

// captureSyncRepo is a minimal RuleSyncRepository that records the ctx it
// received and returns an empty rule list so the cycle exits cleanly.
type captureSyncRepo struct {
	mu          sync.Mutex
	capturedCtx context.Context
}

var _ RuleSyncRepository = (*captureSyncRepo)(nil)

func (c *captureSyncRepo) GetRulesUpdatedSince(ctx context.Context, _ time.Time) ([]*model.Rule, error) {
	c.mu.Lock()
	c.capturedCtx = ctx
	c.mu.Unlock()

	return nil, nil
}

// TestRuleSyncWorker_InjectsTenantPoolIntoContext is the load-bearing unit
// test for CRITICAL A: per-tenant workers MUST resolve their tenant's pool
// per-cycle and inject it onto the cycle context via tmcore.ContextWithPG,
// so downstream repo.GetRulesUpdatedSince(ctx) lands on the tenant DB and
// not the root default pool.
func TestRuleSyncWorker_InjectsTenantPoolIntoContext(t *testing.T) {
	t.Parallel()

	clk := clock.RealClock{}
	ruleCache := cache.NewRuleCache(clk)
	logger := testutil.NewMockLogger()

	stubRepo := &captureSyncRepo{}
	seed := &fakeTenantDB{tag: "tenant-a-pool"}
	resolver := &stubPoolResolver{db: seed}

	worker, err := NewRuleSyncWorkerWithPoolResolver(
		ruleCache, stubRepo, &stubReadyCompiler{}, defaultSyncConfig(), logger,
		defaultTestCircuitBreaker(), clk, "tenant-a", resolver,
	)
	require.NoError(t, err)

	// Run one cycle with a context that already carries the tenantID (what
	// runLoop would do in production).
	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-a")
	worker.runSyncCycle(ctx)

	// 1. Pool resolver was called exactly once for the correct tenant.
	assert.Equal(t, []string{"tenant-a"}, resolver.recordedCalls(),
		"pool resolver must be invoked once per cycle for the worker's tenantID")

	// 2. The injected DB propagated through to the repository's ctx.
	// GetPGContext returns nil if injection was missed — that was the bug.
	require.NotNil(t, stubRepo.capturedCtx, "repository must be called")

	got := tmcore.GetPGContext(stubRepo.capturedCtx)
	require.NotNil(t, got,
		"tmcore.GetPGContext(repoCtx) must be non-nil — the worker must inject "+
			"the tenant pool via tmcore.ContextWithPG each cycle")

	gotDB, ok := got.(*fakeTenantDB)
	require.True(t, ok, "injected db must be the one the resolver returned")
	assert.Equal(t, "tenant-a-pool", gotDB.tag)
}

// TestRuleSyncWorker_SkipsCycleOnPoolResolutionError verifies the fail-closed
// path: if the pool resolver errors out, the worker MUST skip this cycle
// rather than continue with the root default pool — otherwise the tenant's
// cycle could silently land on the wrong database.
func TestRuleSyncWorker_SkipsCycleOnPoolResolutionError(t *testing.T) {
	t.Parallel()

	clk := clock.RealClock{}
	ruleCache := cache.NewRuleCache(clk)

	stubRepo := &captureSyncRepo{}
	resolver := &stubPoolResolver{err: errors.New("tenant manager unreachable")}

	worker, err := NewRuleSyncWorkerWithPoolResolver(
		ruleCache, stubRepo, &stubReadyCompiler{}, defaultSyncConfig(),
		testutil.NewMockLogger(), defaultTestCircuitBreaker(), clk,
		"tenant-a", resolver,
	)
	require.NoError(t, err)

	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-a")
	worker.runSyncCycle(ctx)

	// The repo must NOT have been called — a missed injection would land
	// the cycle on the default pool.
	assert.Nil(t, stubRepo.capturedCtx,
		"repository must NOT be called when pool resolution fails")
}

// TestRuleSyncWorker_SingleTenantSkipsInjection verifies that single-tenant
// workers (tenantID == "" or nil resolver) do not touch ContextWithPG, which
// preserves the pre-MT behaviour where repositories fall through to the
// static connection. This protects the SINGLE-TENANT canary in CI.
func TestRuleSyncWorker_SingleTenantSkipsInjection(t *testing.T) {
	t.Parallel()

	clk := clock.RealClock{}
	ruleCache := cache.NewRuleCache(clk)

	stubRepo := &captureSyncRepo{}
	// Resolver is set but should never be called because tenantID == "".
	resolver := &stubPoolResolver{db: &fakeTenantDB{tag: "should-not-be-used"}}

	worker, err := NewRuleSyncWorkerWithPoolResolver(
		ruleCache, stubRepo, &stubReadyCompiler{}, defaultSyncConfig(),
		testutil.NewMockLogger(), defaultTestCircuitBreaker(), clk,
		"", resolver,
	)
	require.NoError(t, err)

	worker.runSyncCycle(context.Background())

	assert.Empty(t, resolver.recordedCalls(),
		"pool resolver MUST NOT be called when tenantID is empty (single-tenant mode)")

	require.NotNil(t, stubRepo.capturedCtx,
		"repository must still be called in single-tenant mode")
	assert.Nil(t, tmcore.GetPGContext(stubRepo.capturedCtx),
		"no ContextWithPG injection in single-tenant mode — repositories fall back to static conn")
}

// TestUsageCleanupWorker_InjectsTenantPoolIntoContext mirrors the rule-sync
// worker test for the cleanup worker path. DeleteExpiredCounters is the only
// I/O the cleanup cycle performs, so that's where we verify the pool
// injection.
func TestUsageCleanupWorker_InjectsTenantPoolIntoContext(t *testing.T) {
	t.Parallel()

	captured := &captureCleanupRepo{}
	seed := &fakeTenantDB{tag: "cleanup-pool"}
	resolver := &stubPoolResolver{db: seed}

	worker, err := NewUsageCleanupWorkerWithPoolResolver(
		captured, UsageCleanupWorkerConfig{CleanupInterval: 1 * time.Hour},
		testutil.NewMockLogger(), clock.RealClock{}, "tenant-c", resolver,
	)
	require.NoError(t, err)

	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-c")
	worker.runCleanupCycle(ctx)

	assert.Equal(t, []string{"tenant-c"}, resolver.recordedCalls())
	require.NotNil(t, captured.capturedCtx)

	got := tmcore.GetPGContext(captured.capturedCtx)
	require.NotNil(t, got,
		"cleanup worker must inject the tenant pool via tmcore.ContextWithPG each cycle")

	gotDB, ok := got.(*fakeTenantDB)
	require.True(t, ok)
	assert.Equal(t, "cleanup-pool", gotDB.tag)
}

type captureCleanupRepo struct {
	mu          sync.Mutex
	capturedCtx context.Context
}

var _ UsageCounterCleanupRepository = (*captureCleanupRepo)(nil)

func (c *captureCleanupRepo) DeleteExpiredCounters(ctx context.Context, _ time.Time) (int64, error) {
	c.mu.Lock()
	c.capturedCtx = ctx
	c.mu.Unlock()

	return 0, nil
}
