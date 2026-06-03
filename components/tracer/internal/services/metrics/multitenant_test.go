// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package metrics

import (
	"context"
	"strconv"
	"sync"
	"testing"

	libMetrics "github.com/LerianStudio/lib-observability/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMultiTenantMetrics_CanonicalNames verifies the 4 canonical metric names
// match the spec exactly. Name drift is a production incident because
// dashboards, alerts, and SLOs depend on these specific names.
func TestMultiTenantMetrics_CanonicalNames(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "tenant_connections_total", MetricTenantConnectionsTotal.Name)
	assert.Equal(t, "tenant_connection_errors_total", MetricTenantConnectionErrors.Name)
	assert.Equal(t, "tenant_consumers_active", MetricTenantConsumersActive.Name)
	assert.Equal(t, "tenant_messages_processed_total", MetricTenantMessagesProcessed.Name)
}

// TestMultiTenantMetrics_MetricDescriptions verifies every metric has a
// non-empty description — required for Grafana dashboards and operator
// documentation.
func TestMultiTenantMetrics_MetricDescriptions(t *testing.T) {
	t.Parallel()

	for _, m := range []libMetrics.Metric{
		MetricTenantConnectionsTotal,
		MetricTenantConnectionErrors,
		MetricTenantConsumersActive,
		MetricTenantMessagesProcessed,
	} {
		assert.NotEmpty(t, m.Description, "metric %q needs a description", m.Name)
		assert.Equal(t, "1", m.Unit, "metric %q unit should be '1' for counters/gauges", m.Name)
	}
}

// TestNewMultiTenantMetrics_Disabled returns a no-op implementation that
// accepts every call without panicking. This is the single-tenant path —
// hot paths MUST remain free of any work.
func TestNewMultiTenantMetrics_Disabled(t *testing.T) {
	t.Parallel()

	m := NewMultiTenantMetrics(false, nil, nil)
	require.NotNil(t, m, "disabled metrics should still return a usable instance")

	ctx := context.Background()

	assert.NotPanics(t, func() {
		m.IncConnectionsTotal(ctx, "tenant-a", "tracer")
		m.IncConnectionErrors(ctx, "tenant-a", "tracer", "timeout")
		m.IncConsumersActive(ctx, "tenant-a", "tracer")
		m.DecConsumersActive(ctx, "tenant-a", "tracer")
		m.IncMessagesProcessed(ctx, "tenant-a", "tracer", "ALLOW")
	})
}

// TestNewMultiTenantMetrics_Enabled_NilFactoryFallsBackToNoop verifies that
// passing enabled=true with a nil factory does not panic — we log a warning
// (via the provided logger if non-nil) and fall back to the no-op.
func TestNewMultiTenantMetrics_Enabled_NilFactoryFallsBackToNoop(t *testing.T) {
	t.Parallel()

	m := NewMultiTenantMetrics(true, nil, nil)
	require.NotNil(t, m)

	ctx := context.Background()

	assert.NotPanics(t, func() {
		m.IncConnectionsTotal(ctx, "tenant-a", "tracer")
	})
}

// TestNewMultiTenantMetrics_Enabled_RealFactory verifies the real
// implementation emits metrics through the lib-commons MetricsFactory.
// We use NewNopFactory so the call sites run end-to-end without requiring
// a real OTel SDK, but every registration and emit path is exercised.
func TestNewMultiTenantMetrics_Enabled_RealFactory(t *testing.T) {
	t.Parallel()

	factory := libMetrics.NewNopFactory()

	m := NewMultiTenantMetrics(true, factory, nil)
	require.NotNil(t, m)

	ctx := context.Background()

	// Each call path must complete without error or panic.
	m.IncConnectionsTotal(ctx, "tenant-a", "tracer")
	m.IncConnectionErrors(ctx, "tenant-a", "tracer", "timeout")
	m.IncConsumersActive(ctx, "tenant-a", "tracer")
	m.IncMessagesProcessed(ctx, "tenant-a", "tracer", "ALLOW")
	m.IncMessagesProcessed(ctx, "tenant-a", "tracer", "DENY")
	m.IncMessagesProcessed(ctx, "tenant-a", "tracer", "ERROR")
	m.DecConsumersActive(ctx, "tenant-a", "tracer")
}

// TestMultiTenantMetrics_ConsumersActive_StateTracking verifies that the
// gauge is incremented/decremented in lock-step across concurrent goroutines.
// The gauge value in the real impl is computed from an internal state map
// keyed by (tenant_id, module). State drift would surface as incorrect
// consumer counts in Grafana and trigger false alerts.
func TestMultiTenantMetrics_ConsumersActive_StateTracking(t *testing.T) {
	t.Parallel()

	impl := newRealMultiTenantMetrics(libMetrics.NewNopFactory(), nil)
	require.NotNil(t, impl)

	ctx := context.Background()

	// Spin up 10 tenants with 3 concurrent increments each, then decrement
	// twice — every tenant should end with count=1.
	var wg sync.WaitGroup

	const tenants = 10
	for i := 0; i < tenants; i++ {
		wg.Add(1)

		tenantID := "tenant-" + strconv.Itoa(i)

		go func() {
			defer wg.Done()

			for k := 0; k < 3; k++ {
				impl.IncConsumersActive(ctx, tenantID, "tracer")
			}

			for k := 0; k < 2; k++ {
				impl.DecConsumersActive(ctx, tenantID, "tracer")
			}
		}()
	}

	wg.Wait()

	for i := 0; i < tenants; i++ {
		tenantID := "tenant-" + strconv.Itoa(i)
		assert.EqualValues(t, 1, impl.activeCount(tenantID, "tracer"),
			"tenant %q should have net 1 active consumer", tenantID)
	}
}

// TestMultiTenantMetrics_DecBelowZero_DoesNotUnderflow verifies that calling
// DecConsumersActive more times than IncConsumersActive does not produce a
// negative gauge value. Underflow would leak into Grafana as a huge uint
// (~18 quintillion) and trip alerts.
func TestMultiTenantMetrics_DecBelowZero_DoesNotUnderflow(t *testing.T) {
	t.Parallel()

	impl := newRealMultiTenantMetrics(libMetrics.NewNopFactory(), nil)
	ctx := context.Background()

	impl.DecConsumersActive(ctx, "tenant-a", "tracer")
	impl.DecConsumersActive(ctx, "tenant-a", "tracer")

	assert.EqualValues(t, 0, impl.activeCount("tenant-a", "tracer"),
		"decrementing below zero must clamp at 0")
}

// TestMultiTenantMetrics_EmptyTenantID_NotPanic mirrors the single-tenant
// path where GetTenantIDContext returns "". The metric impl must pass this
// through without panicking; the tenant label will be "" in emitted series.
func TestMultiTenantMetrics_EmptyTenantID_NotPanic(t *testing.T) {
	t.Parallel()

	m := NewMultiTenantMetrics(true, libMetrics.NewNopFactory(), nil)

	assert.NotPanics(t, func() {
		m.IncConnectionsTotal(context.Background(), "", "tracer")
	})
}
