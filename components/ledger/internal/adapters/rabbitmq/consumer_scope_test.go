// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package rabbitmq

import (
	"context"
	"testing"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// Scenario: Consumer de write-behind com scope do serviço. The consumer loop's root
// context carries the service tracer, so the per-message spans opened from it are
// recorded under the ledger's instrumentation scope.
func TestConsumerRoutes_RootContextUsesTheServiceScope(t *testing.T) {
	t.Parallel()

	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	cr := &ConsumerRoutes{Telemetry: libOpentelemetry.Telemetry{
		TelemetryConfig: libOpentelemetry.TelemetryConfig{LibraryName: "ledger-service-scope"},
		TracerProvider:  tp,
	}}

	ctx := cr.rootContext()
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)
	_, span := tracer.Start(ctx, "rabbitmq.consumer.process_message")
	span.End()

	ended := recorder.Ended()
	require.Len(t, ended, 1)
	assert.Equal(t, "ledger-service-scope", ended[0].InstrumentationScope().Name)
}

// Without a tracer provider the root context stays empty, keeping the default fallback.
func TestConsumerRoutes_RootContextWithoutTelemetryIsEmpty(t *testing.T) {
	t.Parallel()

	assert.Equal(t, context.Background(), (&ConsumerRoutes{}).rootContext())
}
