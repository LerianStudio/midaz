// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package workers

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"tracer/pkg/constant"
)

// fakeMultiTenantMetrics is a recording test double. Kept in the workers
// package so supervisor_metrics_test.go can verify the hook fires even
// though the concrete MultiTenantMetrics interface lives in the metrics
// package. Go allows this because the interface is satisfied structurally.
type fakeMultiTenantMetrics struct {
	mu                sync.Mutex
	connectionsTotal  int
	connectionErrors  []string // error_type values
	consumersIncCalls int
	consumersDecCalls int
	messagesProcessed map[string]int64 // result -> count
	lastTenantID      atomic.Value     // string
	lastModule        atomic.Value     // string
}

func newFakeMultiTenantMetrics() *fakeMultiTenantMetrics {
	f := &fakeMultiTenantMetrics{messagesProcessed: map[string]int64{}}
	f.lastTenantID.Store("")
	f.lastModule.Store("")

	return f
}

func (f *fakeMultiTenantMetrics) IncConnectionsTotal(_ context.Context, tenantID, module string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.connectionsTotal++
	f.lastTenantID.Store(tenantID)
	f.lastModule.Store(module)
}

func (f *fakeMultiTenantMetrics) IncConnectionErrors(_ context.Context, tenantID, module, errorType string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.connectionErrors = append(f.connectionErrors, errorType)
	f.lastTenantID.Store(tenantID)
	f.lastModule.Store(module)
}

func (f *fakeMultiTenantMetrics) IncConsumersActive(_ context.Context, tenantID, module string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.consumersIncCalls++
	f.lastTenantID.Store(tenantID)
	f.lastModule.Store(module)
}

func (f *fakeMultiTenantMetrics) DecConsumersActive(_ context.Context, tenantID, module string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.consumersDecCalls++
	f.lastTenantID.Store(tenantID)
	f.lastModule.Store(module)
}

func (f *fakeMultiTenantMetrics) IncMessagesProcessed(_ context.Context, tenantID, module, result string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.messagesProcessed[result]++
	f.lastTenantID.Store(tenantID)
	f.lastModule.Store(module)
}

func (f *fakeMultiTenantMetrics) snapshot() (int, int, int, []string) {
	f.mu.Lock()
	defer f.mu.Unlock()

	errs := append([]string(nil), f.connectionErrors...)

	return f.connectionsTotal, f.consumersIncCalls, f.consumersDecCalls, errs
}

// TestWorkerSupervisor_Metrics_SpawnIncrements verifies that EnsureWorkers
// emits one connection + one consumers_active increment per unique tenant,
// with the tracer module label.
func TestWorkerSupervisor_Metrics_SpawnIncrements(t *testing.T) {
	t.Parallel()
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	fake := newFakeMultiTenantMetrics()
	deps, _ := newSupervisorTestDeps(t, &fakeTenantLister{}, 10)
	deps.Metrics = fake

	sup, err := NewWorkerSupervisor(deps)
	require.NoError(t, err)
	t.Cleanup(sup.Shutdown)

	ctx := context.Background()
	require.NoError(t, sup.EnsureWorkers(ctx, "tenant-a"))

	// Idempotent re-spawn must NOT double-count.
	require.NoError(t, sup.EnsureWorkers(ctx, "tenant-a"))

	conns, inc, dec, _ := fake.snapshot()

	assert.Equal(t, 1, conns, "one connection emission per unique tenant")
	assert.Equal(t, 1, inc, "one consumers_active increment per unique tenant")
	assert.Equal(t, 0, dec, "no decrement until StopWorkers")
	assert.Equal(t, "tenant-a", fake.lastTenantID.Load())
	assert.Equal(t, constant.ModuleName, fake.lastModule.Load())
}

// TestWorkerSupervisor_Metrics_StopDecrements verifies that StopWorkers
// emits exactly one consumers_active decrement.
func TestWorkerSupervisor_Metrics_StopDecrements(t *testing.T) {
	t.Parallel()
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	fake := newFakeMultiTenantMetrics()
	deps, _ := newSupervisorTestDeps(t, &fakeTenantLister{}, 10)
	deps.Metrics = fake

	sup, err := NewWorkerSupervisor(deps)
	require.NoError(t, err)
	t.Cleanup(sup.Shutdown)

	require.NoError(t, sup.EnsureWorkers(context.Background(), "tenant-a"))
	sup.StopWorkers("tenant-a")

	_, inc, dec, _ := fake.snapshot()
	assert.Equal(t, 1, inc)
	assert.Equal(t, 1, dec)
}

// TestWorkerSupervisor_Metrics_CapReachedEmitsConnError verifies the cap
// failure path increments connection_errors with a well-known error_type,
// so operators can alert on tenant onboarding pressure.
func TestWorkerSupervisor_Metrics_CapReachedEmitsConnError(t *testing.T) {
	t.Parallel()
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	fake := newFakeMultiTenantMetrics()
	// maxTenants=1 so the second EnsureWorkers trips the cap.
	deps, _ := newSupervisorTestDeps(t, &fakeTenantLister{}, 1)
	deps.Metrics = fake

	sup, err := NewWorkerSupervisor(deps)
	require.NoError(t, err)
	t.Cleanup(sup.Shutdown)

	ctx := context.Background()
	require.NoError(t, sup.EnsureWorkers(ctx, "tenant-a"))

	err = sup.EnsureWorkers(ctx, "tenant-b")
	require.Error(t, err, "cap should reject the second tenant")

	_, _, _, errs := fake.snapshot()
	require.Len(t, errs, 1, "exactly one connection_errors emission on cap-reached")
	assert.Equal(t, "tenant_cap_reached", errs[0])
}

// TestWorkerSupervisor_Metrics_NilMetricsDefaultsToNoop verifies that
// leaving Metrics unset in the deps falls back to the no-op implementation.
// Without this, existing callers that pre-date the metrics plumbing would
// nil-deref on every EnsureWorkers call.
func TestWorkerSupervisor_Metrics_NilMetricsDefaultsToNoop(t *testing.T) {
	t.Parallel()
	_, cleanup := setupTestTracer(t)
	defer cleanup()

	deps, _ := newSupervisorTestDeps(t, &fakeTenantLister{}, 10)
	deps.Metrics = nil // explicit — confirm the constructor fills this in

	sup, err := NewWorkerSupervisor(deps)
	require.NoError(t, err)
	t.Cleanup(sup.Shutdown)

	require.NotPanics(t, func() {
		_ = sup.EnsureWorkers(context.Background(), "tenant-a")
		sup.StopWorkers("tenant-a")
	})
}
