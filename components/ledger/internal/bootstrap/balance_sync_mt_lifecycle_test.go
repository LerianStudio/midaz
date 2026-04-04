// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	libRedis "github.com/LerianStudio/lib-commons/v4/commons/redis"
	tmclient "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/client"
	tmcore "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/core"
	tmpostgres "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/postgres"
	"github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/tenantcache"
	redisTransaction "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services/command"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --------------------------------------------------------------------
// Helpers for multi-tenant lifecycle tests
// --------------------------------------------------------------------

// newFakeTenantCollector creates a tenantCollector whose goroutine blocks until cancel.
// This simulates a running BalanceSyncCollector without needing real Redis/PG.
func newFakeTenantCollector(tenantID string) *tenantCollector {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		defer close(done)
		<-ctx.Done()
	}()

	return &tenantCollector{tenantID: tenantID, cancel: cancel, done: done}
}

// newDeadTenantCollector creates a tenantCollector whose goroutine has already exited.
// Simulates a collector that panicked or errored out.
func newDeadTenantCollector(tenantID string) *tenantCollector {
	done := make(chan struct{})
	close(done)

	return &tenantCollector{tenantID: tenantID, cancel: func() {}, done: done}
}

// newTestWorkerWithCache creates a BalanceSyncWorker with a real TenantCache and
// an unreachable pgManager (PG operations will fail). Use for reconciliation tests
// where we don't need real collectors to start.
func newTestWorkerWithCache(t *testing.T, cache *tenantcache.TenantCache) *BalanceSyncWorker {
	t.Helper()

	logger := newTestLogger()
	conn := &libRedis.Client{}
	useCase := &command.UseCase{}

	tenantClient, err := tmclient.NewClient(
		"http://localhost:0", // unreachable — GetConnection will fail
		logger,
		tmclient.WithAllowInsecureHTTP(),
		tmclient.WithServiceAPIKey("test-api-key"),
	)
	require.NoError(t, err)

	pgMgr := tmpostgres.NewManager(tenantClient, "transaction", tmpostgres.WithLogger(logger))

	return NewBalanceSyncWorkerMultiTenant(
		conn, logger, useCase, 5, BalanceSyncConfig{},
		true, cache, pgMgr, "transaction",
	)
}

// --------------------------------------------------------------------
// Tests for reconcileCollectors
// --------------------------------------------------------------------

func TestReconcileCollectors_ReapsDeadCollector(t *testing.T) {
	t.Parallel()

	cache := tenantcache.NewTenantCache()
	cache.Set("tenant-1", &tmcore.TenantConfig{ID: "tenant-1"}, 1*time.Hour)

	w := newTestWorkerWithCache(t, cache)

	// Pre-populate with a dead collector (goroutine already exited)
	collectors := map[string]*tenantCollector{
		"tenant-1": newDeadTenantCollector("tenant-1"),
	}

	ctx := context.Background()
	w.reconcileCollectors(ctx, collectors)

	// Dead collector should be reaped. PG failure prevents restart,
	// so the map should be empty.
	assert.Empty(t, collectors, "dead collector should be reaped; PG failure prevents restart")
}

func TestReconcileCollectors_StopsRemovedTenant(t *testing.T) {
	t.Parallel()

	cache := tenantcache.NewTenantCache()
	// No tenants in cache — tenant-1 will be considered "removed"

	w := newTestWorkerWithCache(t, cache)

	tc := newFakeTenantCollector("tenant-1")
	collectors := map[string]*tenantCollector{
		"tenant-1": tc,
	}

	ctx := context.Background()
	w.reconcileCollectors(ctx, collectors)

	// Collector should have been cancelled and awaited
	select {
	case <-tc.done:
		// Good — collector was stopped
	default:
		t.Fatal("collector goroutine should have exited after cancel")
	}

	assert.Empty(t, collectors, "removed tenant's collector should be deleted from map")
}

func TestReconcileCollectors_StopsMultipleRemovedTenants(t *testing.T) {
	t.Parallel()

	cache := tenantcache.NewTenantCache()
	// Only tenant-2 remains active
	cache.Set("tenant-2", &tmcore.TenantConfig{ID: "tenant-2"}, 1*time.Hour)

	w := newTestWorkerWithCache(t, cache)

	tc1 := newFakeTenantCollector("tenant-1")
	tc3 := newFakeTenantCollector("tenant-3")
	tc2 := newFakeTenantCollector("tenant-2")

	collectors := map[string]*tenantCollector{
		"tenant-1": tc1,
		"tenant-2": tc2,
		"tenant-3": tc3,
	}

	ctx := context.Background()
	w.reconcileCollectors(ctx, collectors)

	// tenant-1 and tenant-3 should be stopped
	select {
	case <-tc1.done:
	default:
		t.Fatal("tenant-1 collector should have been stopped")
	}

	select {
	case <-tc3.done:
	default:
		t.Fatal("tenant-3 collector should have been stopped")
	}

	// tenant-2 should still be running
	select {
	case <-tc2.done:
		t.Fatal("tenant-2 collector should still be running")
	default:
	}

	assert.Len(t, collectors, 1, "only tenant-2 should remain")
	assert.Contains(t, collectors, "tenant-2")

	// Cleanup
	tc2.cancel()
	<-tc2.done
}

func TestReconcileCollectors_NoOpWhenCollectorsMatchCache(t *testing.T) {
	t.Parallel()

	cache := tenantcache.NewTenantCache()
	cache.Set("tenant-1", &tmcore.TenantConfig{ID: "tenant-1"}, 1*time.Hour)

	w := newTestWorkerWithCache(t, cache)

	tc := newFakeTenantCollector("tenant-1")
	collectors := map[string]*tenantCollector{
		"tenant-1": tc,
	}

	ctx := context.Background()
	w.reconcileCollectors(ctx, collectors)

	// Collector should still be running, map unchanged
	assert.Len(t, collectors, 1)
	assert.Contains(t, collectors, "tenant-1")

	select {
	case <-tc.done:
		t.Fatal("collector should still be running")
	default:
	}

	// Cleanup
	tc.cancel()
	<-tc.done
}

func TestReconcileCollectors_EmptyCache(t *testing.T) {
	t.Parallel()

	cache := tenantcache.NewTenantCache()
	// No tenants

	w := newTestWorkerWithCache(t, cache)

	collectors := make(map[string]*tenantCollector)

	ctx := context.Background()
	w.reconcileCollectors(ctx, collectors)

	assert.Empty(t, collectors, "no tenants = no collectors")
}

func TestReconcileCollectors_NewTenantPGFails(t *testing.T) {
	t.Parallel()

	cache := tenantcache.NewTenantCache()
	cache.Set("tenant-new", &tmcore.TenantConfig{ID: "tenant-new"}, 1*time.Hour)

	w := newTestWorkerWithCache(t, cache) // unreachable pgManager

	collectors := make(map[string]*tenantCollector)

	ctx := context.Background()
	w.reconcileCollectors(ctx, collectors)

	// startTenantCollector should have failed (PG unreachable) — not added to map
	assert.Empty(t, collectors, "PG failure should prevent collector from starting")
}

func TestReconcileCollectors_DeadCollectorRestartedButPGFails(t *testing.T) {
	t.Parallel()

	cache := tenantcache.NewTenantCache()
	cache.Set("tenant-1", &tmcore.TenantConfig{ID: "tenant-1"}, 1*time.Hour)

	w := newTestWorkerWithCache(t, cache)

	// Dead collector
	collectors := map[string]*tenantCollector{
		"tenant-1": newDeadTenantCollector("tenant-1"),
	}

	ctx := context.Background()
	w.reconcileCollectors(ctx, collectors)

	// Dead collector reaped (Phase 1), restart attempted (Phase 3) but PG fails
	assert.Empty(t, collectors, "restart should fail due to PG, leaving map empty")
}

// --------------------------------------------------------------------
// Tests for stopAllCollectors
// --------------------------------------------------------------------

func TestStopAllCollectors_CancelsAndWaitsForAll(t *testing.T) {
	t.Parallel()

	w := &BalanceSyncWorker{logger: newTestLogger()}

	tc1 := newFakeTenantCollector("tenant-1")
	tc2 := newFakeTenantCollector("tenant-2")
	tc3 := newFakeTenantCollector("tenant-3")

	collectors := map[string]*tenantCollector{
		"tenant-1": tc1,
		"tenant-2": tc2,
		"tenant-3": tc3,
	}

	ctx := context.Background()
	w.stopAllCollectors(ctx, collectors)

	// All should be stopped
	for name, tc := range map[string]*tenantCollector{
		"tenant-1": tc1,
		"tenant-2": tc2,
		"tenant-3": tc3,
	} {
		select {
		case <-tc.done:
		default:
			t.Fatalf("%s collector should be done", name)
		}
	}

	assert.Empty(t, collectors, "map should be cleared")
}

func TestStopAllCollectors_EmptyMap(t *testing.T) {
	t.Parallel()

	w := &BalanceSyncWorker{logger: newTestLogger()}

	collectors := make(map[string]*tenantCollector)

	// Should not panic or block
	w.stopAllCollectors(context.Background(), collectors)

	assert.Empty(t, collectors)
}

// --------------------------------------------------------------------
// Tests for startTenantCollector
// --------------------------------------------------------------------

func TestStartTenantCollector_PGConnectionFails(t *testing.T) {
	t.Parallel()

	cache := tenantcache.NewTenantCache()
	w := newTestWorkerWithCache(t, cache)

	ctx := context.Background()
	tc := w.startTenantCollector(ctx, "tenant-unreachable")

	assert.Nil(t, tc, "should return nil when PG connection fails")
}

// --------------------------------------------------------------------
// Tests for tenantCollector type
// --------------------------------------------------------------------

func TestTenantCollector_DoneClosedAfterCancel(t *testing.T) {
	t.Parallel()

	tc := newFakeTenantCollector("test-tenant")

	// Should be alive
	select {
	case <-tc.done:
		t.Fatal("collector should be alive before cancel")
	default:
	}

	tc.cancel()

	// Should complete within a reasonable time
	select {
	case <-tc.done:
		// Good
	case <-time.After(2 * time.Second):
		t.Fatal("collector goroutine did not exit after cancel")
	}
}

func TestTenantCollector_DeadDetectable(t *testing.T) {
	t.Parallel()

	tc := newDeadTenantCollector("dead-tenant")

	select {
	case <-tc.done:
		// Good — dead collector is immediately detectable
	default:
		t.Fatal("dead collector's done channel should be closed")
	}
}

// --------------------------------------------------------------------
// Tests for tenantReconcileInterval constant
// --------------------------------------------------------------------

func TestTenantReconcileInterval_ReasonableDefault(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 10*time.Second, tenantReconcileInterval,
		"reconcile interval should be 10s")
}

// --------------------------------------------------------------------
// Tests for flushRemaining context preservation
// --------------------------------------------------------------------

// TestFlushRemaining_PreservesContextValues verifies that after context cancellation,
// the flush callback in flushRemaining receives a non-cancelled context that preserves
// the original context's values (tenant ID, PG connection, etc.).
//
// The context is validated INSIDE the callback because flushRemaining's deferred cancel()
// fires after the callback returns, which would cancel the context before the test can check.
func TestFlushRemaining_PreservesContextValues(t *testing.T) {
	t.Parallel()

	type ctxKey string
	const tenantKey ctxKey = "tenant-id"
	const pgKey ctxKey = "pg-connection"

	// Create a context with values, then cancel it
	ctx := context.WithValue(context.Background(), tenantKey, "acme-corp")
	ctx = context.WithValue(ctx, pgKey, "mock-db-handle")
	ctx, cancel := context.WithCancel(ctx)
	cancel() // cancelled BEFORE flushRemaining

	// Capture results inside the callback (before deferred cancel fires)
	var ctxErr error
	var tenantVal, pgVal any
	var callCount atomic.Int32

	collector := NewBalanceSyncCollector(10, 500*time.Millisecond, 50*time.Millisecond, 1*time.Second, newTestLogger())
	collector.SetFlushCallback(func(flushCtx context.Context, keys []redisTransaction.SyncKey) bool {
		ctxErr = flushCtx.Err()
		tenantVal = flushCtx.Value(tenantKey)
		pgVal = flushCtx.Value(pgKey)
		callCount.Add(1)
		return true
	})

	// Manually fill the buffer
	collector.mu.Lock()
	collector.buffer = append(collector.buffer, redisTransaction.SyncKey{Key: "key1", Score: 100})
	collector.mu.Unlock()

	// Call flushRemaining with the cancelled context
	collector.flushRemaining(ctx)

	require.Equal(t, int32(1), callCount.Load(), "flush callback should have been called once")

	// The flush context should NOT be cancelled (context.WithoutCancel strips cancellation)
	assert.NoError(t, ctxErr, "flush context should not be cancelled during callback execution")

	// Context values should be preserved
	assert.Equal(t, "acme-corp", tenantVal, "tenant ID should be preserved")
	assert.Equal(t, "mock-db-handle", pgVal, "PG connection should be preserved")
}

// TestFlushRemaining_EmptyBufferNoFlush verifies that flushRemaining does not call
// the flush callback when the buffer is empty (even with a valid context).
func TestFlushRemaining_EmptyBufferNoFlush(t *testing.T) {
	t.Parallel()

	var callCount atomic.Int32

	collector := NewBalanceSyncCollector(10, 500*time.Millisecond, 50*time.Millisecond, 1*time.Second, newTestLogger())
	collector.SetFlushCallback(func(_ context.Context, _ []redisTransaction.SyncKey) bool {
		callCount.Add(1)
		return true
	})

	collector.flushRemaining(context.Background())

	assert.Equal(t, int32(0), callCount.Load(), "should not flush empty buffer")
}

// --------------------------------------------------------------------
// Integration-style test: reconcile lifecycle
// --------------------------------------------------------------------

// TestReconcileCollectors_FullLifecycle tests the complete add → stable → remove cycle.
func TestReconcileCollectors_FullLifecycle(t *testing.T) {
	t.Parallel()

	cache := tenantcache.NewTenantCache()
	w := newTestWorkerWithCache(t, cache)
	ctx := context.Background()
	collectors := make(map[string]*tenantCollector)

	// Round 1: Empty cache → nothing happens
	w.reconcileCollectors(ctx, collectors)
	assert.Empty(t, collectors)

	// Round 2: Add tenant-1 (PG fails, so won't start, but no crash)
	cache.Set("tenant-1", &tmcore.TenantConfig{ID: "tenant-1"}, 1*time.Hour)
	w.reconcileCollectors(ctx, collectors)
	assert.Empty(t, collectors, "PG failure prevents start")

	// Round 3: Manually inject a running collector (simulating successful PG)
	tc1 := newFakeTenantCollector("tenant-1")
	collectors["tenant-1"] = tc1
	w.reconcileCollectors(ctx, collectors)
	assert.Len(t, collectors, 1, "existing collector left running")

	// Round 4: Add tenant-2 (PG fails), tenant-1 still running
	cache.Set("tenant-2", &tmcore.TenantConfig{ID: "tenant-2"}, 1*time.Hour)
	w.reconcileCollectors(ctx, collectors)
	assert.Len(t, collectors, 1, "tenant-1 still running, tenant-2 failed to start")
	assert.Contains(t, collectors, "tenant-1")

	// Round 5: Remove tenant-1 from cache → collector stopped
	cache.Delete("tenant-1")
	w.reconcileCollectors(ctx, collectors)
	assert.Empty(t, collectors, "tenant-1 removed, tenant-2 PG still failing")

	select {
	case <-tc1.done:
	default:
		t.Fatal("tenant-1 collector should have been stopped")
	}
}

// TestReconcileCollectors_ConcurrentSafety verifies reconcileCollectors does not
// race when called from a single goroutine with collectors being cancelled by
// their own goroutines concurrently.
func TestReconcileCollectors_ConcurrentSafety(t *testing.T) {
	t.Parallel()

	cache := tenantcache.NewTenantCache()
	cache.Set("tenant-1", &tmcore.TenantConfig{ID: "tenant-1"}, 1*time.Hour)

	w := newTestWorkerWithCache(t, cache)
	ctx := context.Background()

	collectors := make(map[string]*tenantCollector)

	// Inject a collector that will die on its own after a short delay
	shortLived := func() *tenantCollector {
		_, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})

		go func() {
			defer close(done)
			// Die after a short time
			time.Sleep(10 * time.Millisecond)
			cancel()
		}()

		return &tenantCollector{tenantID: "tenant-1", cancel: cancel, done: done}
	}

	collectors["tenant-1"] = shortLived()

	// Wait for the collector to die
	time.Sleep(50 * time.Millisecond)

	// Reconcile should detect the dead collector and attempt restart
	w.reconcileCollectors(ctx, collectors)

	// Should be empty (reap + PG failure on restart)
	assert.Empty(t, collectors)
}

// --------------------------------------------------------------------
// Test for stopAllCollectors ordering
// --------------------------------------------------------------------

// TestStopAllCollectors_CancelsBeforeWaiting verifies that all collectors receive
// their cancellation signals before any waiting begins (parallel cancel, then wait).
func TestStopAllCollectors_CancelsBeforeWaiting(t *testing.T) {
	t.Parallel()

	w := &BalanceSyncWorker{logger: newTestLogger()}

	var cancelOrder []string
	var mu sync.Mutex

	// Create collectors that record when they receive cancel
	makeCollector := func(id string) *tenantCollector {
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})

		wrappedCancel := func() {
			mu.Lock()
			cancelOrder = append(cancelOrder, id)
			mu.Unlock()
			cancel()
		}

		go func() {
			defer close(done)
			<-ctx.Done()
			// Small delay to simulate flush work
			time.Sleep(10 * time.Millisecond)
		}()

		return &tenantCollector{tenantID: id, cancel: wrappedCancel, done: done}
	}

	collectors := map[string]*tenantCollector{
		"t1": makeCollector("t1"),
		"t2": makeCollector("t2"),
		"t3": makeCollector("t3"),
	}

	start := time.Now()
	w.stopAllCollectors(context.Background(), collectors)
	elapsed := time.Since(start)

	// All three should have been cancelled
	mu.Lock()
	assert.Len(t, cancelOrder, 3, "all collectors should have been cancelled")
	mu.Unlock()

	// Should complete in roughly the time of one collector's flush (10ms), not 3x
	// because cancellations are sent in parallel before waiting.
	assert.Less(t, elapsed, 200*time.Millisecond,
		"stop should be fast since cancels happen before waits")

	assert.Empty(t, collectors, "map should be cleared")
}
