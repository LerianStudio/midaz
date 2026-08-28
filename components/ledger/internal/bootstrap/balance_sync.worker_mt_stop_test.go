// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	"github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/tenantcache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStopTenantCollector_CancelsAndForgets covers the eviction path: the
// collector is cancelled, awaited, and removed from the worker.
func TestStopTenantCollector_CancelsAndForgets(t *testing.T) {
	t.Parallel()

	w := newTestWorkerWithCache(t, tenantcache.NewTenantCache())

	tc := newFakeTenantCollector("tenant-1")
	w.collectors["tenant-1"] = tc

	require.True(t, w.HasCollector("tenant-1"), "the seeded collector must be visible")

	w.StopTenantCollector("tenant-1")

	select {
	case <-tc.done:
	default:
		t.Fatal("StopTenantCollector must wait for the collector goroutine to exit")
	}

	assert.False(t, w.HasCollector("tenant-1"), "the stopped collector must be forgotten")
	assert.Empty(t, w.collectorTenantIDs(), "no collector must remain")
}

// TestStopTenantCollector_UnknownTenantIsNoop covers the ordinary case of an
// eviction for a tenant this worker never collected.
func TestStopTenantCollector_UnknownTenantIsNoop(t *testing.T) {
	t.Parallel()

	w := newTestWorkerWithCache(t, tenantcache.NewTenantCache())

	require.NotPanics(t, func() { w.StopTenantCollector("never-started") })

	assert.Empty(t, w.collectorTenantIDs(), "a no-op must not invent a collector")
}

// TestStopTenantCollector_LeavesOtherTenantsRunning proves the eviction is scoped
// to one tenant.
func TestStopTenantCollector_LeavesOtherTenantsRunning(t *testing.T) {
	t.Parallel()

	w := newTestWorkerWithCache(t, tenantcache.NewTenantCache())

	stopped := newFakeTenantCollector("tenant-1")
	kept := newFakeTenantCollector("tenant-2")
	w.collectors["tenant-1"] = stopped
	w.collectors["tenant-2"] = kept

	t.Cleanup(func() { w.StopTenantCollector("tenant-2") })

	w.StopTenantCollector("tenant-1")

	assert.False(t, w.HasCollector("tenant-1"))
	assert.True(t, w.HasCollector("tenant-2"), "the other tenant must keep collecting")

	select {
	case <-kept.done:
		t.Fatal("the other tenant's collector must not be cancelled")
	default:
	}
}

// TestStopTenantCollector_ReconcileRecreatesCollector is the point of the whole
// epic: a tenant.removed event stops the collector, and because the tenant is
// still in the cache the next reconcile brings it back on a freshly resolved
// pool — no process restart.
func TestStopTenantCollector_ReconcileRecreatesCollector(t *testing.T) {
	t.Parallel()

	cache := tenantcache.NewTenantCache()
	cache.Set("tenant-1", &tmcore.TenantConfig{ID: "tenant-1"}, 1*time.Hour)

	resolver := &fakePGResolver{db: newStubDB()}

	w, _, _ := newResolveTestWorker(t, resolver)
	w.tenantCache = cache

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w.reconcileCollectors(ctx)
	require.True(t, w.HasCollector("tenant-1"), "the first reconcile must start the collector")

	w.StopTenantCollector("tenant-1")
	require.False(t, w.HasCollector("tenant-1"), "the eviction must stop the collector")

	callsAfterStop := resolver.calls.Load()

	w.reconcileCollectors(ctx)

	assert.True(t, w.HasCollector("tenant-1"),
		"the tenant is still cached, so the reconcile must bring the collector back")
	assert.Greater(t, resolver.calls.Load(), callsAfterStop,
		"the restarted collector must resolve the pool again rather than reuse the evicted one")

	w.stopAllCollectors(ctx)
}

// TestHasCollector_FalseForUnknownAndZeroValue guards the ownership checker's
// reads against a worker that never entered multi-tenant mode.
func TestHasCollector_FalseForUnknownAndZeroValue(t *testing.T) {
	t.Parallel()

	w := newTestWorkerWithCache(t, tenantcache.NewTenantCache())
	assert.False(t, w.HasCollector("nobody"))

	var zero BalanceSyncWorker
	assert.False(t, zero.HasCollector("nobody"), "a nil map must read as no collector")
}

// TestStopTenantCollector_DrainsBeforeReturning is the contract the tenant-eviction
// wiring rests on: the callback calls StopTenantCollector and then closes the
// tenant's pools, so the final flush must have finished by the time the call
// returns — not eventually, afterwards.
func TestStopTenantCollector_DrainsBeforeReturning(t *testing.T) {
	t.Parallel()

	resolver := &fakePGResolver{db: newStubDB()}

	w, rec, _ := newResolveTestWorker(t, resolver)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tc := w.startTenantCollector(ctx, "tenant-1")
	require.NotNil(t, tc)

	w.collectorsMu.Lock()
	w.collectors["tenant-1"] = tc
	w.collectorsMu.Unlock()

	// Let the collector claim keys so its shutdown has a buffer worth draining.
	require.Eventually(t, func() bool { return rec.count() >= 1 }, resolveTestTimeout, 10*time.Millisecond,
		"the collector must be flushing before the eviction")

	flushesBeforeStop := rec.count()

	w.StopTenantCollector("tenant-1")

	// Read the goroutine's state with no waiting at all: anything StopTenantCollector
	// left running would make these racy, and -race would say so.
	select {
	case <-tc.done:
	default:
		t.Fatal("StopTenantCollector must not return while the collector is still running")
	}

	assert.GreaterOrEqual(t, rec.count(), flushesBeforeStop,
		"the final flush must have run before the pools are closed")
	assert.False(t, w.HasCollector("tenant-1"))
}
