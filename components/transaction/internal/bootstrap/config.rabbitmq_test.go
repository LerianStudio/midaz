// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"testing"

	"github.com/LerianStudio/lib-commons/v3/commons/opentelemetry/metrics"
	tmcore "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// TestRabbitMQComponents_MetricsFactoryField verifies that the rabbitMQComponents
// struct has a metricsFactory field that can be nil in single-tenant mode and
// non-nil when telemetry is enabled in multi-tenant mode.
func TestRabbitMQComponents_MetricsFactoryField(t *testing.T) {
	t.Parallel()

	mp := sdkmetric.NewMeterProvider()
	meter := mp.Meter("test")
	factory := metrics.NewMetricsFactory(meter, nil)

	tests := []struct {
		name           string
		metricsFactory *metrics.MetricsFactory
		wantNil        bool
	}{
		{
			name:           "nil_factory_in_single_tenant_mode",
			metricsFactory: nil,
			wantNil:        true,
		},
		{
			name:           "non_nil_factory_in_multi_tenant_mode_with_telemetry",
			metricsFactory: factory,
			wantNil:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rmq := &rabbitMQComponents{
				metricsFactory: tt.metricsFactory,
			}

			if tt.wantNil {
				assert.Nil(t, rmq.metricsFactory,
					"metricsFactory should be nil in single-tenant or telemetry-disabled mode")
			} else {
				assert.NotNil(t, rmq.metricsFactory,
					"metricsFactory should be non-nil when telemetry is enabled")
			}
		})
	}
}

// TestResolveTenantConnections_EmitsMetrics_OnNilManagersNoError verifies that
// resolveTenantConnections does not panic when metricsFactory is set on the
// rabbitMQComponents but both managers are nil (graceful degradation).
func TestResolveTenantConnections_EmitsMetrics_OnNilManagersNoError(t *testing.T) {
	t.Parallel()

	mp := sdkmetric.NewMeterProvider()
	meter := mp.Meter("test")
	factory := metrics.NewMetricsFactory(meter, nil)

	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-001")
	rmq := &rabbitMQComponents{
		pgManager:      nil,
		mongoManager:   nil,
		metricsFactory: factory,
	}

	// With nil managers and a tenant ID, resolveTenantConnections should
	// skip resolution without error and without emitting metrics.
	result, err := resolveTenantConnections(ctx, rmq)
	require.NoError(t, err, "should not error with nil managers")
	assert.Equal(t, "tenant-001", tmcore.GetTenantIDFromContext(result),
		"tenant ID should be preserved")
}

// TestResolveTenantConnections_NilMetricsFactory_NoPanic verifies that
// resolveTenantConnections does not panic when metricsFactory is nil.
// This is the single-tenant mode path.
func TestResolveTenantConnections_NilMetricsFactory_NoPanic(t *testing.T) {
	t.Parallel()

	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-002")
	rmq := &rabbitMQComponents{
		metricsFactory: nil,
	}

	require.NotPanics(t, func() {
		_, err := resolveTenantConnections(ctx, rmq)
		require.NoError(t, err, "should not error with nil metricsFactory")
	}, "must not panic with nil metricsFactory")
}

// TestMultiTenantConsumerRunnable_MetricsFactoryField verifies that the
// multiTenantConsumerRunnable struct has a metricsFactory field.
func TestMultiTenantConsumerRunnable_MetricsFactoryField(t *testing.T) {
	t.Parallel()

	mp := sdkmetric.NewMeterProvider()
	meter := mp.Meter("test")
	factory := metrics.NewMetricsFactory(meter, nil)

	tests := []struct {
		name           string
		metricsFactory *metrics.MetricsFactory
		wantNil        bool
	}{
		{
			name:           "nil_factory",
			metricsFactory: nil,
			wantNil:        true,
		},
		{
			name:           "non_nil_factory",
			metricsFactory: factory,
			wantNil:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			runnable := &multiTenantConsumerRunnable{
				metricsFactory: tt.metricsFactory,
			}

			if tt.wantNil {
				assert.Nil(t, runnable.metricsFactory,
					"metricsFactory should be nil")
			} else {
				assert.NotNil(t, runnable.metricsFactory,
					"metricsFactory should be non-nil")
			}
		})
	}
}
