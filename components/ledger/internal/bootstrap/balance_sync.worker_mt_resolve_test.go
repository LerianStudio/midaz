// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	"github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/tenantcache"
	"github.com/LerianStudio/lib-observability/v4/metrics"
	"github.com/bxcodec/dbresolver/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/mock/gomock"

	redisTransaction "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// resolveTestTimeout bounds every Eventually in this file. The collector's default
// flush timeout is 500ms, so a handful of flush cycles fit comfortably inside it.
const resolveTestTimeout = 5 * time.Second

// flushHandleRecorder captures the PG handle each flush saw, in order.
type flushHandleRecorder struct {
	mu      sync.Mutex
	handles []dbresolver.DB
}

// record stores the transaction-module handle carried by a flush context.
func (r *flushHandleRecorder) record(ctx context.Context) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.handles = append(r.handles, tmcore.GetPGContext(ctx, constant.ModuleTransaction))
}

func (r *flushHandleRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()

	return len(r.handles)
}

// sawHandle reports whether any recorded flush received want.
func (r *flushHandleRecorder) sawHandle(want dbresolver.DB) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, h := range r.handles {
		if h == want {
			return true
		}
	}

	return false
}

func (r *flushHandleRecorder) first() dbresolver.DB {
	r.mu.Lock()
	defer r.mu.Unlock()

	if len(r.handles) == 0 {
		return nil
	}

	return r.handles[0]
}

// newAlwaysOneKeyRepo returns a RedisRepository mock whose GetBalanceSyncKeys
// always yields exactly one key, so a collector with BatchSize 1 flushes on
// every fetch and the loop never goes idle.
func newAlwaysOneKeyRepo(t *testing.T) *redisTransaction.MockRedisRepository {
	t.Helper()

	ctrl := gomock.NewController(t)
	repo := redisTransaction.NewMockRedisRepository(ctrl)

	repo.EXPECT().
		GetBalanceSyncKeys(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ int64) ([]redisTransaction.SyncKey, error) {
			return []redisTransaction.SyncKey{{Key: "balance:{transactions}:org:ledger:alias#key"}}, nil
		}).
		AnyTimes()

	return repo
}

// newResolveTestWorker wires an MT-ready worker whose flushes are recorded rather
// than executed, so a test observes exactly which PG handle each flush received.
func newResolveTestWorker(
	t *testing.T,
	resolver tenantPGResolver,
) (*BalanceSyncWorker, *flushHandleRecorder, *sdkmetric.ManualReader) {
	t.Helper()

	reader, factory := newBalanceSyncReaderFactory(t)

	w := newTestWorkerWithResolver(t, tenantcache.NewTenantCache(), resolver).
		WithMetricsFactory(factory)
	w.useCase = &command.UseCase{TransactionRedisRepo: newAlwaysOneKeyRepo(t)}

	// BatchSize 1 makes every fetch fire the SIZE trigger, so flushes are immediate.
	w.syncConfig.BatchSize = 1

	rec := &flushHandleRecorder{}
	w.flushBatchFn = func(ctx context.Context, _ []redisTransaction.SyncKey) bool {
		rec.record(ctx)

		return true
	}

	return w, rec, reader
}

// newBalanceSyncReaderFactory builds a MetricsFactory backed by a manual reader so
// a test can read back the counters the worker emitted.
func newBalanceSyncReaderFactory(t *testing.T) (*sdkmetric.ManualReader, *metrics.MetricsFactory) {
	t.Helper()

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	factory, err := metrics.NewMetricsFactory(mp.Meter("balance-sync-resolve-test"), nil)
	require.NoError(t, err)

	return reader, factory
}

// tenantSkipCount sums the tenant-skip counter across all label sets.
func tenantSkipCount(t *testing.T, reader *sdkmetric.ManualReader) int64 {
	t.Helper()

	var rm metricdata.ResourceMetrics

	require.NoError(t, reader.Collect(context.Background(), &rm))

	var total int64

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != utils.BalanceSyncTenantSkip.Name {
				continue
			}

			sum, ok := m.Data.(metricdata.Sum[int64])
			require.True(t, ok, "tenant skip data type must be Sum[int64], got %T", m.Data)

			for _, dp := range sum.DataPoints {
				total += dp.Value
			}
		}
	}

	return total
}

// TestStartTenantCollector_ResolvesPGPerFlush proves the handle is resolved on
// every flush rather than captured once at spawn: swapping the resolver's return
// value mid-flight makes the next flush see the new handle.
func TestStartTenantCollector_ResolvesPGPerFlush(t *testing.T) {
	t.Parallel()

	dbA, dbB := newStubDB(), newStubDB()
	resolver := &fakePGResolver{db: dbA}

	w, rec, _ := newResolveTestWorker(t, resolver)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tc := w.startTenantCollector(ctx, "tenant-1")
	require.NotNil(t, tc, "preflight succeeds, so the collector must start")

	require.Eventually(t, func() bool { return rec.count() >= 1 }, resolveTestTimeout, 10*time.Millisecond,
		"the first flush must run")
	assert.Equal(t, dbA, rec.first(), "the first flush must see the handle resolved for it")

	resolver.set(dbB, nil)

	require.Eventually(t, func() bool { return rec.sawHandle(dbB) }, resolveTestTimeout, 10*time.Millisecond,
		"a flush after the swap must see the new handle, which only per-flush resolution gives")

	// One preflight plus at least one resolution per observed flush.
	assert.GreaterOrEqual(t, resolver.calls.Load(), int32(3),
		"resolution must happen once per flush, not once per collector")

	cancel()
	<-tc.done
}

// TestStartTenantCollector_ResolveFailsSkipsFlushKeepsCollectorAlive proves a
// resolution failure is degraded-but-recoverable: the batch is skipped and
// counted, and the collector keeps running.
func TestStartTenantCollector_ResolveFailsSkipsFlushKeepsCollectorAlive(t *testing.T) {
	t.Parallel()

	dbA := newStubDB()
	resolver := &fakePGResolver{db: dbA}

	w, rec, reader := newResolveTestWorker(t, resolver)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tc := w.startTenantCollector(ctx, "tenant-1")
	require.NotNil(t, tc, "preflight succeeds, so the collector must start")

	require.Eventually(t, func() bool { return rec.count() >= 1 }, resolveTestTimeout, 10*time.Millisecond,
		"the first flush must run while the resolver is healthy")

	resolver.set(nil, errors.New("pool gone"))

	flushesBeforeFailure := rec.count()
	callsBeforeFailure := resolver.calls.Load()

	// Wait for several further resolution attempts to confirm the loop is still
	// turning, then assert none of them reached the flush body.
	require.Eventually(t, func() bool { return resolver.calls.Load() >= callsBeforeFailure+3 },
		resolveTestTimeout, 10*time.Millisecond,
		"the collector must keep attempting resolution after a failure")

	assert.LessOrEqual(t, rec.count(), flushesBeforeFailure+1,
		"a failed resolution must skip the flush rather than call it with no handle")
	assert.Positive(t, tenantSkipCount(t, reader),
		"a skipped flush must be counted on the tenant-skip counter")

	select {
	case <-tc.done:
		t.Fatal("the collector must stay alive across resolution failures")
	default:
	}

	cancel()
	<-tc.done
}

// TestStartTenantCollector_RecoversWhenResolverHeals proves the auto-heal: once
// the resolver hands back a working pool the same collector resumes flushing,
// with no restart and no external action.
func TestStartTenantCollector_RecoversWhenResolverHeals(t *testing.T) {
	t.Parallel()

	dbA := newStubDB()
	resolver := &fakePGResolver{err: errors.New("pool gone"), db: newStubDB()}

	w, rec, _ := newResolveTestWorker(t, resolver)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// The preflight resolves before the failure is installed, mirroring a pool
	// that dies underneath a collector that started healthy.
	resolver.set(newStubDB(), nil)

	tc := w.startTenantCollector(ctx, "tenant-1")
	require.NotNil(t, tc, "preflight succeeds, so the collector must start")

	resolver.set(nil, errors.New("pool gone"))

	callsAtFailure := resolver.calls.Load()
	require.Eventually(t, func() bool { return resolver.calls.Load() >= callsAtFailure+2 },
		resolveTestTimeout, 10*time.Millisecond,
		"at least two flushes must fail to resolve before the pool heals")

	resolver.set(dbA, nil)

	require.Eventually(t, func() bool { return rec.sawHandle(dbA) }, resolveTestTimeout, 10*time.Millisecond,
		"the collector must flush again on the healed handle without being restarted")

	select {
	case <-tc.done:
		t.Fatal("recovery must happen inside the original collector, not after a restart")
	default:
	}

	cancel()
	<-tc.done
}
