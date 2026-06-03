// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build unit

package multitenant

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// TestNewMetrics_CreatesAllFourInstruments verifies that NewMetrics creates a Metrics
// struct with all four canonical multi-tenant instruments initialized (non-nil).
func TestNewMetrics_CreatesAllFourInstruments(t *testing.T) {
	t.Parallel()

	mp := sdkmetric.NewMeterProvider()
	defer func() { _ = mp.Shutdown(context.Background()) }()

	meter := mp.Meter("test-library")

	m, err := NewMetrics(meter)

	require.NoError(t, err)
	require.NotNil(t, m)
	assert.NotNil(t, m.TenantConnectionsTotal, "TenantConnectionsTotal must be initialized")
	assert.NotNil(t, m.TenantConnectionErrorsTotal, "TenantConnectionErrorsTotal must be initialized")
	assert.NotNil(t, m.TenantConsumersActive, "TenantConsumersActive must be initialized")
	assert.NotNil(t, m.TenantMessagesProcessedTotal, "TenantMessagesProcessedTotal must be initialized")
}

// TestNoopMetrics_ReturnsNonNilInstruments verifies that NoopMetrics returns a Metrics
// struct where all instruments are non-nil noop implementations. This ensures callers
// can always call Record/Add without nil checks.
func TestNoopMetrics_ReturnsNonNilInstruments(t *testing.T) {
	t.Parallel()

	m := NoopMetrics()

	require.NotNil(t, m)
	assert.NotNil(t, m.TenantConnectionsTotal, "noop TenantConnectionsTotal must not be nil")
	assert.NotNil(t, m.TenantConnectionErrorsTotal, "noop TenantConnectionErrorsTotal must not be nil")
	assert.NotNil(t, m.TenantConsumersActive, "noop TenantConsumersActive must not be nil")
	assert.NotNil(t, m.TenantMessagesProcessedTotal, "noop TenantMessagesProcessedTotal must not be nil")
}

// TestNoopMetrics_RecordDoesNotPanic verifies that recording values on noop instruments
// does not panic. This is the core safety guarantee: code paths that call metrics
// when MULTI_TENANT_ENABLED=false must not crash.
func TestNoopMetrics_RecordDoesNotPanic(t *testing.T) {
	t.Parallel()

	m := NoopMetrics()

	ctx := context.Background()
	tenantAttr := metric.WithAttributes(attribute.String("tenant_id", "test-tenant"))

	assert.NotPanics(t, func() {
		m.TenantConnectionsTotal.Add(ctx, 1, tenantAttr)
	}, "Add on noop TenantConnectionsTotal must not panic")

	assert.NotPanics(t, func() {
		m.TenantConnectionErrorsTotal.Add(ctx, 1, tenantAttr)
	}, "Add on noop TenantConnectionErrorsTotal must not panic")

	assert.NotPanics(t, func() {
		m.TenantConsumersActive.Add(ctx, 1, tenantAttr)
	}, "Add on noop TenantConsumersActive must not panic")

	assert.NotPanics(t, func() {
		m.TenantMessagesProcessedTotal.Add(ctx, 1, tenantAttr)
	}, "Add on noop TenantMessagesProcessedTotal must not panic")
}

// TestNewMetrics_RecordDoesNotPanic verifies that recording values on real OTel instruments
// does not panic. This mirrors the noop safety test but uses a real SDK meter provider,
// ensuring production metrics paths are safe to call.
func TestNewMetrics_RecordDoesNotPanic(t *testing.T) {
	t.Parallel()

	mp := sdkmetric.NewMeterProvider()
	defer func() { _ = mp.Shutdown(context.Background()) }()

	meter := mp.Meter("test-library")

	m, err := NewMetrics(meter)
	require.NoError(t, err)

	ctx := context.Background()
	tenantAttr := metric.WithAttributes(attribute.String("tenant_id", "test-tenant"))

	assert.NotPanics(t, func() {
		m.TenantConnectionsTotal.Add(ctx, 1, tenantAttr)
	}, "Add on real TenantConnectionsTotal must not panic")

	assert.NotPanics(t, func() {
		m.TenantConnectionErrorsTotal.Add(ctx, 1, tenantAttr)
	}, "Add on real TenantConnectionErrorsTotal must not panic")

	assert.NotPanics(t, func() {
		m.TenantConsumersActive.Add(ctx, 1, tenantAttr)
	}, "Add on real TenantConsumersActive must not panic")

	assert.NotPanics(t, func() {
		m.TenantMessagesProcessedTotal.Add(ctx, 1, tenantAttr)
	}, "Add on real TenantMessagesProcessedTotal must not panic")
}
