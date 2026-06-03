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

	tmclient "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/client"
	tmcore "github.com/LerianStudio/lib-commons/v5/commons/tenant-manager/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"tracer/internal/services/cache"
	"tracer/internal/services/workers/mocks"
	"tracer/internal/testutil"
	"tracer/pkg/clock"
	"tracer/pkg/constant"
	"tracer/pkg/model"
)

// fakeTenantLister is a test double for the subset of tmclient.Client used by
// the supervisor's InitialTenantSync path. It lets tests inject deterministic
// tenant lists and failures without standing up a real HTTP server.
type fakeTenantLister struct {
	mu      sync.Mutex
	tenants []*tmclient.TenantSummary
	err     error
	calls   int
}

func (f *fakeTenantLister) GetActiveTenantsByService(_ context.Context, _ string) ([]*tmclient.TenantSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.calls++

	if f.err != nil {
		return nil, f.err
	}

	return f.tenants, nil
}

// newSupervisorTestDeps returns a WorkerSupervisorDeps wired with unit-test
// friendly fakes. The repos return empty results so spawned workers don't
// block on real I/O; the in-memory cache is real so eviction can be asserted.
func newSupervisorTestDeps(t *testing.T, lister TenantLister, maxTenants int) (WorkerSupervisorDeps, *cache.RuleCache) {
	t.Helper()

	ctrl := gomock.NewController(t)
	syncRepo := mocks.NewMockRuleSyncRepository(ctrl)
	syncRepo.EXPECT().
		GetRulesUpdatedSince(gomock.Any(), gomock.Any()).
		Return([]*model.Rule{}, nil).
		AnyTimes()

	usageRepo := mocks.NewMockUsageCounterCleanupRepository(ctrl)
	usageRepo.EXPECT().
		DeleteExpiredCounters(gomock.Any(), gomock.Any()).
		Return(int64(0), nil).
		AnyTimes()

	compiler := mocks.NewMockExpressionCompiler(ctrl)
	compiler.EXPECT().
		Compile(gomock.Any(), gomock.Any()).
		Return("compiled", nil).
		AnyTimes()

	clk := clock.RealClock{}
	ruleCache := cache.NewRuleCache(clk)
	logger := testutil.NewMockLogger()

	deps := WorkerSupervisorDeps{
		RuleCache: ruleCache,
		SyncRepo:  syncRepo,
		UsageRepo: usageRepo,
		Compiler:  compiler,
		SyncConfig: RuleSyncWorkerConfig{
			PollInterval:       50 * time.Millisecond,
			StalenessThreshold: 500 * time.Millisecond,
			OverlapBuffer:      10 * time.Millisecond,
		},
		CleanupConfig: UsageCleanupWorkerConfig{
			CleanupInterval: 1 * time.Hour,
		},
		// Preserve the historical test behaviour (both workers spawn). The
		// H8-specific "disabled cleanup" path has its own dedicated test.
		CleanupWorkerEnabled: true,
		CBTemplate:           newSupervisorTestCBTemplate(),
		TenantList:           lister,
		Clock:                clk,
		MaxTenants:           maxTenants,
		Logger:               logger,
	}

	return deps, ruleCache
}

func newSupervisorTestCBTemplate() CircuitBreakerTemplate {
	return CircuitBreakerTemplate{
		NamePrefix:    "test_supervisor",
		MaxRequests:   1,
		Interval:      0,
		Timeout:       1 * time.Second,
		FailureThresh: 5,
		FailureRatio:  0,
		MinRequests:   0,
	}
}

func TestWorkerSupervisor_EnsureWorkers_Idempotent(t *testing.T) {
	t.Parallel()
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	deps, _ := newSupervisorTestDeps(t, &fakeTenantLister{}, 10)

	sup, err := NewWorkerSupervisor(deps)
	require.NoError(t, err)
	t.Cleanup(sup.Shutdown)

	ctx := context.Background()

	require.NoError(t, sup.EnsureWorkers(ctx, "tenant-a"))
	require.NoError(t, sup.EnsureWorkers(ctx, "tenant-a"))
	require.NoError(t, sup.EnsureWorkers(ctx, "tenant-a"))

	assert.Equal(t, 1, sup.tenantCount(), "only one worker set should exist")
}

func TestWorkerSupervisor_EnsureWorkers_MultipleTenants(t *testing.T) {
	t.Parallel()
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	deps, _ := newSupervisorTestDeps(t, &fakeTenantLister{}, 10)

	sup, err := NewWorkerSupervisor(deps)
	require.NoError(t, err)
	t.Cleanup(sup.Shutdown)

	ctx := context.Background()
	require.NoError(t, sup.EnsureWorkers(ctx, "tenant-a"))
	require.NoError(t, sup.EnsureWorkers(ctx, "tenant-b"))

	assert.Equal(t, 2, sup.tenantCount())
}

func TestWorkerSupervisor_EnsureWorkers_CapReached(t *testing.T) {
	t.Parallel()
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	deps, _ := newSupervisorTestDeps(t, &fakeTenantLister{}, 2)

	sup, err := NewWorkerSupervisor(deps)
	require.NoError(t, err)
	t.Cleanup(sup.Shutdown)

	ctx := context.Background()
	require.NoError(t, sup.EnsureWorkers(ctx, "tenant-a"))
	require.NoError(t, sup.EnsureWorkers(ctx, "tenant-b"))

	err = sup.EnsureWorkers(ctx, "tenant-c")
	require.Error(t, err, "third tenant should hit the cap")

	assert.Equal(t, 2, sup.tenantCount(), "tenant-c must NOT be registered after cap failure")
}

func TestWorkerSupervisor_EnsureWorkers_EmptyTenantID(t *testing.T) {
	t.Parallel()
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	deps, _ := newSupervisorTestDeps(t, &fakeTenantLister{}, 10)

	sup, err := NewWorkerSupervisor(deps)
	require.NoError(t, err)
	t.Cleanup(sup.Shutdown)

	err = sup.EnsureWorkers(context.Background(), "")
	require.Error(t, err, "empty tenantID must be rejected")
	assert.Equal(t, 0, sup.tenantCount())
}

func TestWorkerSupervisor_StopWorkers_EvictsCache(t *testing.T) {
	t.Parallel()
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	deps, ruleCache := newSupervisorTestDeps(t, &fakeTenantLister{}, 10)

	sup, err := NewWorkerSupervisor(deps)
	require.NoError(t, err)
	t.Cleanup(sup.Shutdown)

	ctx := context.Background()
	require.NoError(t, sup.EnsureWorkers(ctx, "tenant-a"))
	require.NoError(t, sup.EnsureWorkers(ctx, "tenant-b"))

	// Populate both tenants' cache buckets directly (no dependence on workers).
	ctxA := tmcore.ContextWithTenantID(ctx, "tenant-a")
	ctxB := tmcore.ContextWithTenantID(ctx, "tenant-b")
	ruleA := newSyncTestActiveRule(1)
	ruleB := newSyncTestActiveRule(2)
	ruleCache.UpsertRule(ctxA, ruleA, "compiled-a")
	ruleCache.UpsertRule(ctxB, ruleB, "compiled-b")
	require.Len(t, ruleCache.GetActiveRules(ctxA, nil), 1)
	require.Len(t, ruleCache.GetActiveRules(ctxB, nil), 1)

	sup.StopWorkers("tenant-a")

	assert.Equal(t, 1, sup.tenantCount(), "tenant-a must be removed from supervisor")
	assert.Empty(t, ruleCache.GetActiveRules(ctxA, nil), "tenant-a cache must be evicted")
	assert.Len(t, ruleCache.GetActiveRules(ctxB, nil), 1, "tenant-b cache must NOT be touched")
}

func TestWorkerSupervisor_StopWorkers_Unknown(t *testing.T) {
	t.Parallel()
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	deps, _ := newSupervisorTestDeps(t, &fakeTenantLister{}, 10)

	sup, err := NewWorkerSupervisor(deps)
	require.NoError(t, err)
	t.Cleanup(sup.Shutdown)

	// Must not panic, must not fail.
	sup.StopWorkers("never-spawned")

	assert.Equal(t, 0, sup.tenantCount())
}

func TestWorkerSupervisor_Shutdown_StopsAll(t *testing.T) {
	t.Parallel()
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	deps, ruleCache := newSupervisorTestDeps(t, &fakeTenantLister{}, 10)

	sup, err := NewWorkerSupervisor(deps)
	require.NoError(t, err)

	ctx := context.Background()
	for _, id := range []string{"tenant-a", "tenant-b", "tenant-c"} {
		require.NoError(t, sup.EnsureWorkers(ctx, id))
		ruleCache.UpsertRule(tmcore.ContextWithTenantID(ctx, id), newSyncTestActiveRule(1), "compiled")
	}
	require.Equal(t, 3, sup.tenantCount())

	done := make(chan struct{})
	go func() {
		sup.Shutdown()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Shutdown did not complete within timeout")
	}

	assert.Equal(t, 0, sup.tenantCount())
	for _, id := range []string{"tenant-a", "tenant-b", "tenant-c"} {
		assert.Empty(t,
			ruleCache.GetActiveRules(tmcore.ContextWithTenantID(ctx, id), nil),
			"tenant %q cache must be evicted by Shutdown", id)
	}
}

func TestWorkerSupervisor_InitialTenantSync(t *testing.T) {
	t.Parallel()
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	lister := &fakeTenantLister{
		tenants: []*tmclient.TenantSummary{
			{ID: "tenant-a", Name: "A", Status: "active"},
			{ID: "tenant-b", Name: "B", Status: "active"},
			{ID: "tenant-c", Name: "C", Status: "active"},
		},
	}

	deps, _ := newSupervisorTestDeps(t, lister, 10)

	sup, err := NewWorkerSupervisor(deps)
	require.NoError(t, err)
	t.Cleanup(sup.Shutdown)

	require.NoError(t, sup.InitialTenantSync(context.Background()))

	assert.Equal(t, 3, sup.tenantCount())
}

func TestWorkerSupervisor_InitialTenantSync_ListerError(t *testing.T) {
	t.Parallel()
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	lister := &fakeTenantLister{err: errors.New("tenant manager unreachable")}

	deps, _ := newSupervisorTestDeps(t, lister, 10)

	sup, err := NewWorkerSupervisor(deps)
	require.NoError(t, err)
	t.Cleanup(sup.Shutdown)

	// Best-effort: InitialTenantSync surfaces the error but the supervisor stays usable.
	err = sup.InitialTenantSync(context.Background())
	require.Error(t, err)
	assert.Equal(t, 0, sup.tenantCount())

	// A later EnsureWorkers call must still succeed.
	require.NoError(t, sup.EnsureWorkers(context.Background(), "tenant-a"))
	assert.Equal(t, 1, sup.tenantCount())
}

func TestWorkerSupervisor_ConcurrentEnsure(t *testing.T) {
	t.Parallel()
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	deps, _ := newSupervisorTestDeps(t, &fakeTenantLister{}, 10)

	sup, err := NewWorkerSupervisor(deps)
	require.NoError(t, err)
	t.Cleanup(sup.Shutdown)

	const numGoroutines = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	start := make(chan struct{})

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			<-start
			_ = sup.EnsureWorkers(context.Background(), "tenant-a")
		}()
	}

	close(start)
	wg.Wait()

	assert.Equal(t, 1, sup.tenantCount(),
		"100 concurrent EnsureWorkers calls must produce exactly one worker set")
}

// TestWorkerSupervisor_CleanupDisabled verifies H8: when
// CleanupWorkerEnabled is false, EnsureWorkers spawns ONLY the sync worker
// (not the cleanup worker). The tenant worker set still registers and
// StopWorkers tears it down cleanly with a single goroutine.
func TestWorkerSupervisor_CleanupDisabled(t *testing.T) {
	t.Parallel()
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	deps, _ := newSupervisorTestDeps(t, &fakeTenantLister{}, 10)
	// Flip the flag off — this is the path operators hit with
	// CLEANUP_WORKER_ENABLED=false. Historically the supervisor overwrote
	// this signal with DefaultUsageCleanupWorkerConfig.
	deps.CleanupWorkerEnabled = false

	sup, err := NewWorkerSupervisor(deps)
	require.NoError(t, err)

	ctx := context.Background()
	require.NoError(t, sup.EnsureWorkers(ctx, "tenant-a"))
	require.Equal(t, 1, sup.tenantCount(),
		"tenant set must register even when only the sync worker runs")

	// StopWorkers MUST complete cleanly even with only one goroutine in the
	// set. The old code path depended on the wait group being Add(2), which
	// the new code sets to Add(1) in this branch.
	done := make(chan struct{})
	go func() {
		defer close(done)
		sup.StopWorkers("tenant-a")
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("StopWorkers did not return within 2s when cleanup worker is disabled")
	}

	assert.Equal(t, 0, sup.tenantCount(), "tenant set must be removed after StopWorkers")
}

func TestNewWorkerSupervisor_ValidatesRequiredDeps(t *testing.T) {
	t.Parallel()

	baseDeps, _ := newSupervisorTestDeps(t, &fakeTenantLister{}, 10)

	tests := []struct {
		name    string
		mutate  func(*WorkerSupervisorDeps)
		wantErr error
	}{
		{
			name:    "nil rule cache",
			mutate:  func(d *WorkerSupervisorDeps) { d.RuleCache = nil },
			wantErr: constant.ErrSupervisorNilRuleCache,
		},
		{
			name:    "nil sync repo",
			mutate:  func(d *WorkerSupervisorDeps) { d.SyncRepo = nil },
			wantErr: constant.ErrSupervisorNilSyncRepo,
		},
		{
			// Cleanup is enabled in baseDeps so a nil UsageRepo MUST fail.
			// The cleanup-disabled path is covered by the dedicated test
			// below.
			name:    "nil usage repo with cleanup enabled",
			mutate:  func(d *WorkerSupervisorDeps) { d.UsageRepo = nil },
			wantErr: constant.ErrSupervisorNilUsageRepo,
		},
		{
			name:    "nil compiler",
			mutate:  func(d *WorkerSupervisorDeps) { d.Compiler = nil },
			wantErr: constant.ErrSupervisorNilCompiler,
		},
		{
			name:    "nil logger",
			mutate:  func(d *WorkerSupervisorDeps) { d.Logger = nil },
			wantErr: constant.ErrSupervisorNilLogger,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := baseDeps
			tt.mutate(&deps)

			_, err := NewWorkerSupervisor(deps)
			require.Error(t, err, "expected validation to fail for %s", tt.name)
			require.ErrorIs(t, err, tt.wantErr,
				"sentinel must be the registered TRC code so callers can match via errors.Is")
		})
	}
}

// TestNewWorkerSupervisor_NilUsageRepoAllowedWhenCleanupDisabled verifies that
// the supervisor accepts a nil UsageRepo when the cleanup worker is gated off
// at the feature-flag layer. Operators running with CLEANUP_WORKER_ENABLED=false
// would otherwise be forced to thread a no-op repository through every layer
// even though the dependency is never consumed — the nil-check belongs behind
// the same flag that gates the cleanup goroutine.
func TestNewWorkerSupervisor_NilUsageRepoAllowedWhenCleanupDisabled(t *testing.T) {
	t.Parallel()

	deps, _ := newSupervisorTestDeps(t, &fakeTenantLister{}, 10)
	deps.CleanupWorkerEnabled = false
	deps.UsageRepo = nil

	sup, err := NewWorkerSupervisor(deps)
	require.NoError(t, err,
		"cleanup-disabled supervisor must accept a nil UsageRepo; the cleanup goroutine never spawns so the dep is unused")
	require.NotNil(t, sup, "supervisor must construct successfully")
}
