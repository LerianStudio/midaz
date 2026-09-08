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

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/tenantcache"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	redisTransaction "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
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
//
// Neither trigger is allowed to fire during the run: the batch size is far above
// what the collector can accumulate and the flush timeout outlasts the test. The
// buffer therefore only grows, and the single flush the recorder sees is
// unambiguously the one flushRemaining drove. Sizing it any other way makes the
// assertion a coin toss, because with an immediate SIZE trigger the buffer is
// empty most of the time and flushRemaining skips an empty buffer.
func TestStopTenantCollector_DrainsBeforeReturning(t *testing.T) {
	t.Parallel()

	resolver := &fakePGResolver{db: newStubDB()}

	w, rec, _ := newResolveTestWorker(t, resolver)
	w.syncConfig.BatchSize = 1_000_000
	w.syncConfig.FlushTimeoutMs = int(resolveTestTimeout.Milliseconds()) * 10

	fetches := &atomic.Int32{}
	w.useCase.TransactionRedisRepo = newCountingOneKeyRepo(t, fetches)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tc := w.startTenantCollector(ctx, "tenant-1")
	require.NotNil(t, tc)

	w.collectorsMu.Lock()
	w.collectors["tenant-1"] = tc
	w.collectorsMu.Unlock()

	// Two fetches mean at least one key is buffered and no flush has been triggered.
	require.Eventually(t, func() bool { return fetches.Load() >= 2 }, resolveTestTimeout, 10*time.Millisecond,
		"the collector must accumulate keys before the eviction")
	require.Zero(t, rec.count(), "no trigger may fire before the eviction")

	w.StopTenantCollector("tenant-1")

	// Read the goroutine's state with no waiting at all: anything StopTenantCollector
	// left running would make these racy, and -race would say so.
	select {
	case <-tc.done:
	default:
		t.Fatal("StopTenantCollector must not return while the collector is still running")
	}

	assert.Equal(t, 1, rec.count(),
		"the buffered keys must be flushed before StopTenantCollector returns")
	assert.False(t, w.HasCollector("tenant-1"))
}

// newCountingOneKeyRepo is newAlwaysOneKeyRepo with an observable fetch count, so a
// test can wait for the collector to have buffered keys without a flush to observe.
func newCountingOneKeyRepo(t *testing.T, fetches *atomic.Int32) *redisTransaction.MockRedisRepository {
	t.Helper()

	ctrl := gomock.NewController(t)
	repo := redisTransaction.NewMockRedisRepository(ctrl)

	repo.EXPECT().
		GetBalanceSyncKeys(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ int64) ([]redisTransaction.SyncKey, error) {
			fetches.Add(1)

			return []redisTransaction.SyncKey{{Key: "balance:{transactions}:org:ledger:alias#key"}}, nil
		}).
		AnyTimes()

	return repo
}

// blockingPGResolver holds its first GetDB open until released, which is how a test
// lands an eviction inside the reconcile's spawn window.
type blockingPGResolver struct {
	db      dbresolver.DB
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (b *blockingPGResolver) GetDB(context.Context, string) (dbresolver.DB, error) {
	b.once.Do(func() {
		close(b.entered)
		<-b.release
	})

	return b.db, nil
}

// TestStopTenantCollector_EvictionDuringSpawnWins covers the registration race the
// spawn-outside-the-lock design opens: reconcileCollectors spawns without holding
// collectorsMu, so an eviction can land while a collector is being built. That
// eviction finds no collector to cancel, and registering the in-flight one
// afterwards would resurrect a collector whose pool the eviction callback is about
// to close.
func TestStopTenantCollector_EvictionDuringSpawnWins(t *testing.T) {
	t.Parallel()

	cache := tenantcache.NewTenantCache()
	cache.Set("tenant-1", &tmcore.TenantConfig{ID: "tenant-1"}, 1*time.Hour)

	blocker := &blockingPGResolver{
		db:      newStubDB(),
		entered: make(chan struct{}),
		release: make(chan struct{}),
	}

	w, _, _ := newResolveTestWorker(t, &fakePGResolver{db: newStubDB()})
	w.tenantCache = cache
	w.pgResolver = blocker

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reconciled := make(chan struct{})

	go func() {
		defer close(reconciled)

		w.reconcileCollectors(ctx)
	}()

	// The reconcile is now inside startTenantCollector's preflight.
	<-blocker.entered

	// The eviction finds no collector yet — exactly the window under test.
	w.StopTenantCollector("tenant-1")

	close(blocker.release)
	<-reconciled

	assert.False(t, w.HasCollector("tenant-1"),
		"the eviction must win the race; registering the in-flight collector would resurrect it")
	assert.Empty(t, w.collectorTenantIDs(), "no collector may survive the eviction")

	// The next reconcile is a fresh decision and may start a collector again.
	w.reconcileCollectors(ctx)
	assert.True(t, w.HasCollector("tenant-1"),
		"the tenant is still cached, so a later reconcile must be free to start one")

	w.stopAllCollectors(ctx)
}
