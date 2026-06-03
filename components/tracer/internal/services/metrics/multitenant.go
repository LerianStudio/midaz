// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package metrics provides multi-tenant-aware OpenTelemetry metrics helpers.
//
// The four canonical metrics exposed here (tenant_connections_total,
// tenant_connection_errors_total, tenant_consumers_active,
// tenant_messages_processed_total) are required by the Lerian multi-tenant
// standard. They are labelled with tenant_id + module so dashboards can
// slice by tenant and by module within a multi-module deployment.
//
// In single-tenant mode (MULTI_TENANT_ENABLED=false), MultiTenantMetrics
// returns a no-op implementation from NewMultiTenantMetrics so hot paths
// pay zero cost and no metrics are emitted.
package metrics

import (
	"context"
	"sync"
	"sync/atomic"

	libLog "github.com/LerianStudio/lib-observability/log"
	libMetrics "github.com/LerianStudio/lib-observability/metrics"
)

// Canonical multi-tenant metric definitions.
//
// Names and labels MUST match the Lerian standard exactly — drift breaks
// Grafana dashboards, alerts, and SLO calculations that are shared across
// Lerian services.
var (
	// MetricTenantConnectionsTotal counts per-tenant connection creations.
	// Labels: tenant_id, module.
	MetricTenantConnectionsTotal = libMetrics.Metric{
		Name:        "tenant_connections_total",
		Unit:        "1",
		Description: "Per-tenant connection creations (includes worker set spawns)",
	}

	// MetricTenantConnectionErrors counts per-tenant connection failures.
	// Labels: tenant_id, module, error_type.
	MetricTenantConnectionErrors = libMetrics.Metric{
		Name:        "tenant_connection_errors_total",
		Unit:        "1",
		Description: "Per-tenant connection failures by error_type",
	}

	// MetricTenantConsumersActive tracks active per-tenant workers as a gauge.
	// Labels: tenant_id, module.
	MetricTenantConsumersActive = libMetrics.Metric{
		Name:        "tenant_consumers_active",
		Unit:        "1",
		Description: "Currently active per-tenant worker sets",
	}

	// MetricTenantMessagesProcessed counts per-tenant validations processed.
	// For tracer this is repurposed from the generic "messages processed"
	// meaning in the multi-tenant standard — each validation completion
	// increments this counter with result ∈ {ALLOW, DENY, REVIEW, ERROR}.
	// Labels: tenant_id, module, result.
	MetricTenantMessagesProcessed = libMetrics.Metric{
		Name:        "tenant_messages_processed_total",
		Unit:        "1",
		Description: "Per-tenant validations processed by result",
	}
)

// MultiTenantMetrics is the call-site interface every hot path depends on.
// Two concrete implementations satisfy it: noopMultiTenantMetrics (used in
// single-tenant mode or when enabled=true but factory=nil) and
// realMultiTenantMetrics (used in multi-tenant mode with a live
// MetricsFactory).
//
// every method is safe to call with an empty tenantID — the emitted series
// will simply carry an empty tenant_id label, which matches the
// single-tenant pass-through contract in tmcore.GetTenantIDContext.
type MultiTenantMetrics interface {
	IncConnectionsTotal(ctx context.Context, tenantID, module string)
	IncConnectionErrors(ctx context.Context, tenantID, module, errorType string)
	IncConsumersActive(ctx context.Context, tenantID, module string)
	DecConsumersActive(ctx context.Context, tenantID, module string)
	IncMessagesProcessed(ctx context.Context, tenantID, module, result string)
}

// NewMultiTenantMetrics returns an implementation of MultiTenantMetrics.
//
// When enabled is false, returns a no-op. When enabled is true but the
// factory is nil, falls back to the no-op (with a warning if logger is
// non-nil). This graceful fallback means the supervisor does not need to
// special-case failed factory initialisation — it just keeps running
// without metrics.
func NewMultiTenantMetrics(enabled bool, factory *libMetrics.MetricsFactory, logger libLog.Logger) MultiTenantMetrics {
	if !enabled {
		return noopMultiTenantMetrics{}
	}

	if factory == nil {
		if logger != nil {
			logger.Log(context.Background(), libLog.LevelWarn,
				"MultiTenantMetrics: enabled=true but factory=nil, falling back to no-op")
		}

		return noopMultiTenantMetrics{}
	}

	return newRealMultiTenantMetrics(factory, logger)
}

// noopMultiTenantMetrics is the zero-cost implementation used in
// single-tenant mode.
type noopMultiTenantMetrics struct{}

func (noopMultiTenantMetrics) IncConnectionsTotal(_ context.Context, _, _ string) {}

func (noopMultiTenantMetrics) IncConnectionErrors(_ context.Context, _, _, _ string) {}

func (noopMultiTenantMetrics) IncConsumersActive(_ context.Context, _, _ string) {}

func (noopMultiTenantMetrics) DecConsumersActive(_ context.Context, _, _ string) {}

func (noopMultiTenantMetrics) IncMessagesProcessed(_ context.Context, _, _, _ string) {}

// realMultiTenantMetrics emits through the lib-commons MetricsFactory.
//
// The lib-commons v4 Int64Gauge has no native Inc/Dec — only Set. To expose
// an Inc/Dec style API here (which matches how the supervisor thinks about
// per-tenant worker lifecycle), we track the running count per (tenant_id,
// module) pair locally and push the updated value via Set on every change.
// Keys are "tenant_id|module" — simple, allocation-lean, and impossible to
// collide because neither component contains '|'.
type realMultiTenantMetrics struct {
	factory *libMetrics.MetricsFactory
	logger  libLog.Logger

	// activeMu guards active; individual int64 reads/writes go through the
	// atomic package so the fast path avoids the map lookup + mutex on
	// repeated increments for the same tenant.
	activeMu sync.RWMutex
	active   map[string]*int64
}

// newRealMultiTenantMetrics is exported to the test file via lower-case — it
// lives in the same package and is used directly to assert state-tracking
// correctness without going through the interface fallback branches.
func newRealMultiTenantMetrics(factory *libMetrics.MetricsFactory, logger libLog.Logger) *realMultiTenantMetrics {
	return &realMultiTenantMetrics{
		factory: factory,
		logger:  logger,
		active:  make(map[string]*int64),
	}
}

// activeKey joins tenantID and module without allocating a format string.
// Chosen over fmt.Sprintf because this is called on every Inc/Dec —
// hot-path hygiene matters here.
func activeKey(tenantID, module string) string {
	buf := make([]byte, 0, len(tenantID)+1+len(module))
	buf = append(buf, tenantID...)
	buf = append(buf, '|')
	buf = append(buf, module...)

	return string(buf)
}

// activeCount returns the current net count for a (tenant_id, module) pair.
// Exported via lower-case to keep it within the package for tests.
//
//nolint:unused // exercised by multitenant_test.go; unused linter misses that path.
func (m *realMultiTenantMetrics) activeCount(tenantID, module string) int64 {
	m.activeMu.RLock()
	p, ok := m.active[activeKey(tenantID, module)]
	m.activeMu.RUnlock()

	if !ok {
		return 0
	}

	return atomic.LoadInt64(p)
}

// adjustActive mutates the counter by delta and returns the new value.
// Clamps at zero so decrements past zero do not underflow — a real
// production incident I want to avoid is a Grafana gauge showing a huge
// uint value because a decrement raced ahead of its matching increment.
//
// When the new value is zero AND delta < 0 (i.e. we are decrementing toward
// zero, not just observing a tenant that never had activity), the (tenant_id,
// module) entry is evicted from the active map so long-lived deployments with
// tenant churn do not accumulate dead-tenant entries indefinitely.
func (m *realMultiTenantMetrics) adjustActive(tenantID, module string, delta int64) int64 {
	key := activeKey(tenantID, module)

	m.activeMu.RLock()
	p, ok := m.active[key]
	m.activeMu.RUnlock()

	if !ok {
		m.activeMu.Lock()

		if p, ok = m.active[key]; !ok {
			var z int64

			p = &z
			m.active[key] = p
		}

		m.activeMu.Unlock()
	}

	for {
		cur := atomic.LoadInt64(p)

		next := cur + delta
		if next < 0 {
			next = 0
		}

		if atomic.CompareAndSwapInt64(p, cur, next) {
			// Evict zero-count entries on the decrement path so the active
			// map does not grow unbounded with churning tenants. We only
			// evict when delta < 0 to avoid removing a freshly-created entry
			// that another goroutine is about to increment. We re-check the
			// pointer + value under the write lock to avoid a TOCTOU vs.
			// concurrent increments.
			if next == 0 && delta < 0 {
				m.activeMu.Lock()
				if existing, exists := m.active[key]; exists && existing == p && atomic.LoadInt64(existing) == 0 {
					delete(m.active, key)
				}
				m.activeMu.Unlock()
			}

			return next
		}
	}
}

// labelsPool keeps a small reuse pool for the map allocated on every metric
// emission. lib-commons' CounterBuilder.WithLabels / GaugeBuilder.WithLabels
// range over the input map and build their own attribute slice — they do NOT
// retain the map — so pooling is safe (M15).
//
// The pool's New function allocates a size-3 map to accommodate the common
// (tenant_id, module) + optional (result | error_type) shape without growing.
var labelsPool = sync.Pool{
	New: func() any {
		m := make(map[string]string, 3)
		return &m
	},
}

// acquireLabels returns a zeroed labels map from the pool populated with the
// canonical (tenant_id, module) keys. Callers MUST return the map via
// releaseLabels once the emission completes, even on the error path, or the
// pool degrades into a pure allocator.
func acquireLabels(tenantID, module string) *map[string]string {
	ptr := labelsPool.Get().(*map[string]string)
	m := *ptr

	// Clear stale keys from previous lease (e.g. "error_type", "result").
	for k := range m {
		delete(m, k)
	}

	m["tenant_id"] = tenantID
	m["module"] = module

	*ptr = m

	return ptr
}

// releaseLabels returns a labels map to the pool. Nil-safe so the defer
// doesn't crash when acquireLabels was never called (e.g. early error
// before the builder).
func releaseLabels(ptr *map[string]string) {
	if ptr == nil {
		return
	}

	labelsPool.Put(ptr)
}

func (m *realMultiTenantMetrics) IncConnectionsTotal(ctx context.Context, tenantID, module string) {
	counter, err := m.factory.Counter(MetricTenantConnectionsTotal)
	if err != nil || counter == nil {
		m.logEmitFailure(ctx, MetricTenantConnectionsTotal.Name, err)

		return
	}

	labels := acquireLabels(tenantID, module)
	defer releaseLabels(labels)

	if err := counter.WithLabels(*labels).AddOne(ctx); err != nil {
		m.logEmitFailure(ctx, MetricTenantConnectionsTotal.Name, err)
	}
}

func (m *realMultiTenantMetrics) IncConnectionErrors(ctx context.Context, tenantID, module, errorType string) {
	counter, err := m.factory.Counter(MetricTenantConnectionErrors)
	if err != nil || counter == nil {
		m.logEmitFailure(ctx, MetricTenantConnectionErrors.Name, err)

		return
	}

	labels := acquireLabels(tenantID, module)
	defer releaseLabels(labels)

	(*labels)["error_type"] = errorType

	if err := counter.WithLabels(*labels).AddOne(ctx); err != nil {
		m.logEmitFailure(ctx, MetricTenantConnectionErrors.Name, err)
	}
}

func (m *realMultiTenantMetrics) IncConsumersActive(ctx context.Context, tenantID, module string) {
	next := m.adjustActive(tenantID, module, 1)
	m.setConsumersActive(ctx, tenantID, module, next)
}

func (m *realMultiTenantMetrics) DecConsumersActive(ctx context.Context, tenantID, module string) {
	next := m.adjustActive(tenantID, module, -1)
	m.setConsumersActive(ctx, tenantID, module, next)
}

func (m *realMultiTenantMetrics) setConsumersActive(ctx context.Context, tenantID, module string, value int64) {
	gauge, err := m.factory.Gauge(MetricTenantConsumersActive)
	if err != nil || gauge == nil {
		m.logEmitFailure(ctx, MetricTenantConsumersActive.Name, err)

		return
	}

	labels := acquireLabels(tenantID, module)
	defer releaseLabels(labels)

	if err := gauge.WithLabels(*labels).Set(ctx, value); err != nil {
		m.logEmitFailure(ctx, MetricTenantConsumersActive.Name, err)
	}
}

func (m *realMultiTenantMetrics) IncMessagesProcessed(ctx context.Context, tenantID, module, result string) {
	counter, err := m.factory.Counter(MetricTenantMessagesProcessed)
	if err != nil || counter == nil {
		m.logEmitFailure(ctx, MetricTenantMessagesProcessed.Name, err)

		return
	}

	labels := acquireLabels(tenantID, module)
	defer releaseLabels(labels)

	(*labels)["result"] = result

	if err := counter.WithLabels(*labels).AddOne(ctx); err != nil {
		m.logEmitFailure(ctx, MetricTenantMessagesProcessed.Name, err)
	}
}

// logEmitFailure is best-effort — a missing metric emission is not fatal, so
// we log at Warn level and move on. If the logger is nil (which happens
// when the caller did not wire one), we swallow the error silently rather
// than panicking.
func (m *realMultiTenantMetrics) logEmitFailure(ctx context.Context, metricName string, err error) {
	if m.logger == nil || err == nil {
		return
	}

	m.logger.With(
		libLog.String("operation", "metrics.multitenant.emit"),
		libLog.String("metric.name", metricName),
		libLog.String("error.message", err.Error()),
	).Log(ctx, libLog.LevelWarn, "Failed to emit multi-tenant metric")
}
