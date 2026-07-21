// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"testing"

	"github.com/LerianStudio/lib-observability/metrics"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

// TestBalanceSyncWorker_WithMetricsFactory verifies the fluent setter wires the
// factory onto the worker.
func TestBalanceSyncWorker_WithMetricsFactory(t *testing.T) {
	t.Parallel()

	meter := sdkmetric.NewMeterProvider().Meter("test")
	factory, err := metrics.NewMetricsFactory(meter, nil)
	require.NoError(t, err)

	worker := NewBalanceSyncWorker(newTestLogger(), &command.UseCase{}, BalanceSyncConfig{}).
		WithMetricsFactory(factory)

	assert.Same(t, factory, worker.metricsFactory, "WithMetricsFactory must set the factory")
}

// TestBalanceSyncWorker_EmitTenantSkip verifies the tenant-skip counter is
// nil-safe and emits without error (with a bounded tenant_id label) when a
// factory is wired.
func TestBalanceSyncWorker_EmitTenantSkip(t *testing.T) {
	t.Parallel()

	t.Run("nil factory is a no-op", func(t *testing.T) {
		t.Parallel()

		worker := NewBalanceSyncWorker(newTestLogger(), &command.UseCase{}, BalanceSyncConfig{})

		require.NotPanics(t, func() {
			worker.emitTenantSkip(context.Background(), "tenant-123")
		})
	})

	t.Run("with factory emits", func(t *testing.T) {
		t.Parallel()

		meter := sdkmetric.NewMeterProvider().Meter("test")
		factory, err := metrics.NewMetricsFactory(meter, nil)
		require.NoError(t, err)

		worker := NewBalanceSyncWorker(newTestLogger(), &command.UseCase{}, BalanceSyncConfig{}).
			WithMetricsFactory(factory)

		require.NotPanics(t, func() {
			worker.emitTenantSkip(context.Background(), "tenant-123")
		})
	})
}
