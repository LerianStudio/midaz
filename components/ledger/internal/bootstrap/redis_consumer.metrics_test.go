// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/LerianStudio/lib-observability/metrics"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
)

func newTestMetricsFactory(t *testing.T) *metrics.MetricsFactory {
	t.Helper()

	meter := sdkmetric.NewMeterProvider().Meter("test")
	factory, err := metrics.NewMetricsFactory(meter, nil)
	require.NoError(t, err)

	return factory
}

// TestEmitQueueGauges_NilFactory verifies the queue gauges are a no-op (no panic)
// when no metrics factory is configured.
func TestEmitQueueGauges_NilFactory(t *testing.T) {
	t.Parallel()

	consumer := NewRedisQueueConsumer(newTestLogger(), in.TransactionHandler{})

	require.NotPanics(t, func() {
		consumer.emitQueueGauges(context.Background(), map[string]string{"k": "{}"})
	})
}

// TestEmitQueueGauges_WithFactory verifies the depth and oldest-age gauges emit
// without error when a real factory is wired, including with an empty queue.
func TestEmitQueueGauges_WithFactory(t *testing.T) {
	t.Parallel()

	consumer := NewRedisQueueConsumer(newTestLogger(), in.TransactionHandler{}).
		WithMetricsFactory(newTestMetricsFactory(t))

	old := mmodel.TransactionRedisQueue{TTL: time.Now().Add(-2 * time.Hour)}

	raw, err := json.Marshal(old)
	require.NoError(t, err)

	payload := string(raw)

	require.NotPanics(t, func() {
		// Non-empty queue: exercises both depth and oldest-age paths.
		consumer.emitQueueGauges(context.Background(), map[string]string{"key": payload})
		// Empty queue: depth=0, oldest-age path short-circuits.
		consumer.emitQueueGauges(context.Background(), map[string]string{})
		// Unparseable record: depth still emits, oldest-age skipped.
		consumer.emitQueueGauges(context.Background(), map[string]string{"key": "not-json"})
	})
}

// TestEmitQuarantineMetric verifies the quarantine counter is nil-safe and emits
// without error when a factory is present.
func TestEmitQuarantineMetric(t *testing.T) {
	t.Parallel()

	t.Run("nil factory is a no-op", func(t *testing.T) {
		t.Parallel()

		consumer := NewRedisQueueConsumer(newTestLogger(), in.TransactionHandler{})

		require.NotPanics(t, func() {
			consumer.emitQuarantineMetric(context.Background(), newTestLogger())
		})
	})

	t.Run("with factory emits", func(t *testing.T) {
		t.Parallel()

		consumer := NewRedisQueueConsumer(newTestLogger(), in.TransactionHandler{}).
			WithMetricsFactory(newTestMetricsFactory(t))

		require.NotPanics(t, func() {
			consumer.emitQuarantineMetric(context.Background(), newTestLogger())
		})
	})
}
