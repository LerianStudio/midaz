// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"testing"

	libMetrics "github.com/LerianStudio/lib-observability/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/LerianStudio/lib-observability/log"
)

// findDomainMetric locates a metric by name within a collected ResourceMetrics.
func findDomainMetric(t *testing.T, rm metricdata.ResourceMetrics, name string) (metricdata.Metrics, bool) {
	t.Helper()

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				return m, true
			}
		}
	}

	return metricdata.Metrics{}, false
}

// newDomainReaderAndFactory builds a ManualReader-backed MetricsFactory so a
// test can drive RecordDomainOperation through a real meter and inspect the
// emitted instruments.
func newDomainReaderAndFactory(t *testing.T) (*sdkmetric.ManualReader, *libMetrics.MetricsFactory) {
	t.Helper()

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	factory, err := libMetrics.NewMetricsFactory(mp.Meter("reporter-worker-domain-metrics-test"), log.NewNop())
	require.NoError(t, err)
	require.NotNil(t, factory)

	return reader, factory
}

// TestGenerateReport_EmitsDomainMetric verifies that GenerateReport records the
// D6 domain operation metrics (counter + duration histogram) at its exit
// boundary, with the bounded component/operation/result labels. A malformed
// message body drives the parse-failure path, which returns a technical error
// without touching any repository — keeping the emission contract under test in
// isolation.
func TestGenerateReport_EmitsDomainMetric(t *testing.T) {
	t.Parallel()

	reader, factory := newDomainReaderAndFactory(t)

	uc := &UseCase{
		Logger:         log.NewNop(),
		Tracer:         noop.NewTracerProvider().Tracer("test"),
		MetricsFactory: factory,
	}

	ctx := context.Background()

	err := uc.GenerateReport(ctx, []byte("not-json"))
	require.Error(t, err)

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(ctx, &rm))

	counter, ok := findDomainMetric(t, rm, "domain_operations_total")
	require.True(t, ok, "domain_operations_total must be registered")

	sum, ok := counter.Data.(metricdata.Sum[int64])
	require.True(t, ok, "data type must be Sum[int64], got %T", counter.Data)
	require.Len(t, sum.DataPoints, 1)

	dp := sum.DataPoints[0]
	assert.Equal(t, int64(1), dp.Value)

	component, _ := dp.Attributes.Value("component")
	operation, _ := dp.Attributes.Value("operation")
	result, _ := dp.Attributes.Value("result")

	assert.Equal(t, "reporter", component.AsString())
	assert.Equal(t, opGenerateReport, operation.AsString())
	assert.Equal(t, "technical_error", result.AsString())

	_, durOK := findDomainMetric(t, rm, "domain_operation_duration_ms")
	assert.True(t, durOK, "domain_operation_duration_ms must be registered")
}

// TestRecordDomainOp_NilFactory_NoPanic guards the disabled-telemetry path: a
// UseCase with a nil MetricsFactory must treat emission as a no-op.
func TestRecordDomainOp_NilFactory_NoPanic(t *testing.T) {
	t.Parallel()

	uc := &UseCase{
		Logger:         log.NewNop(),
		Tracer:         noop.NewTracerProvider().Tracer("test"),
		MetricsFactory: nil,
	}

	assert.NotPanics(t, func() {
		_ = uc.GenerateReport(context.Background(), []byte("not-json"))
	})
}
