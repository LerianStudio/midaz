// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"testing"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

const testServiceScope = "ledger-service-scope"

// testServiceTelemetry builds a telemetry whose tracer provider records into recorder, the
// same shape config.go hands the workers.
func testServiceTelemetry(t *testing.T) (*libOpentelemetry.Telemetry, *tracetest.SpanRecorder) {
	t.Helper()

	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	return &libOpentelemetry.Telemetry{
		TelemetryConfig: libOpentelemetry.TelemetryConfig{LibraryName: testServiceScope},
		TracerProvider:  tp,
	}, recorder
}

// scopeOfSpanFrom starts and ends one span the way worker code does, through the tracker
// on ctx, and returns the instrumentation scope it was recorded under.
func scopeOfSpanFrom(t *testing.T, ctx context.Context, recorder *tracetest.SpanRecorder) string {
	t.Helper()

	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)
	_, span := tracer.Start(ctx, "worker.span")
	span.End()

	ended := recorder.Ended()
	require.NotEmpty(t, ended, "the span was not recorded by the service tracer provider")

	return ended[len(ended)-1].InstrumentationScope().Name
}

func TestServiceTracer_NilWithoutTracerProvider(t *testing.T) {
	t.Parallel()

	assert.Nil(t, serviceTracer(nil))
	assert.Nil(t, serviceTracer(&libOpentelemetry.Telemetry{}))

	ctx := context.Background()
	assert.Equal(t, ctx, contextWithServiceTracer(ctx, nil), "a nil tracer leaves the context unchanged")
}

// Scenario: Consumer de write-behind com scope do serviço. Every worker, cron and
// consumer root context carries the service tracer, so their spans are exported under the
// ledger's instrumentation scope and not under observability.default.
func TestServiceTracer_WorkerRootContextsUseTheServiceScope(t *testing.T) {
	t.Parallel()

	roots := map[string]func(trace.Tracer) context.Context{
		"redis consumer": func(tr trace.Tracer) context.Context {
			return (&RedisQueueConsumer{}).WithServiceTracer(tr).rootContext()
		},
		"balance sync worker": func(tr trace.Tracer) context.Context {
			return (&BalanceSyncWorker{}).WithServiceTracer(tr).rootContext()
		},
		"legacy balance sync drainer": func(tr trace.Tracer) context.Context {
			return (&LegacyBalanceSyncDrainer{}).WithServiceTracer(tr).rootContext()
		},
		"engine recovery cleanup": func(tr trace.Tracer) context.Context {
			return (&EngineRecoveryCleanupRunner{}).WithServiceTracer(tr).rootContext()
		},
		"balance reconciler": func(tr trace.Tracer) context.Context {
			return (&BalanceReconcilerRunner{}).WithServiceTracer(tr).rootContext()
		},
		"tenant health prober": func(tr trace.Tracer) context.Context {
			return (&TenantHealthProber{}).WithServiceTracer(tr).rootContext()
		},
		"multi-tenant rabbitmq consumer": func(tr trace.Tracer) context.Context {
			return (&multiTenantConsumerRunnable{tracer: tr}).rootContext()
		},
	}

	for name, root := range roots {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			telemetry, recorder := testServiceTelemetry(t)

			scope := scopeOfSpanFrom(t, root(serviceTracer(telemetry)), recorder)

			assert.Equal(t, testServiceScope, scope)
			assert.NotEqual(t, "observability.default", scope)
		})
	}
}

// The builders are nil-safe because the optional runners are nil when disabled.
func TestServiceTracer_BuildersAreNilSafe(t *testing.T) {
	t.Parallel()

	var (
		consumer   *RedisQueueConsumer
		worker     *BalanceSyncWorker
		drainer    *LegacyBalanceSyncDrainer
		cleanup    *EngineRecoveryCleanupRunner
		reconciler *BalanceReconcilerRunner
		prober     *TenantHealthProber
	)

	assert.Nil(t, consumer.WithServiceTracer(nil))
	assert.Nil(t, worker.WithServiceTracer(nil))
	assert.Nil(t, drainer.WithServiceTracer(nil))
	assert.Nil(t, cleanup.WithServiceTracer(nil))
	assert.Nil(t, reconciler.WithServiceTracer(nil))
	assert.Nil(t, prober.WithServiceTracer(nil))
}
