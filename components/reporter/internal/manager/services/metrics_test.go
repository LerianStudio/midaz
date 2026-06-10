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
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/lib-observability/log"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/mongodb/template"
	"github.com/google/uuid"
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

	factory, err := libMetrics.NewMetricsFactory(mp.Meter("reporter-manager-domain-metrics-test"), log.NewNop())
	require.NoError(t, err)
	require.NotNil(t, factory)

	return reader, factory
}

// TestGetTemplateByID_EmitsDomainMetric verifies that a flagship query records
// the D6 domain operation metrics at its exit boundary with the bounded
// component/operation/result labels. The repository returns a not-found error,
// which the use case converts to a business error — exercising the
// business_error result classification (the technical_error path is covered by
// the worker emission test).
func TestGetTemplateByID_EmitsDomainMetric(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	reader, factory := newDomainReaderAndFactory(t)

	mockTemplateRepo := template.NewMockRepository(ctrl)
	mockTemplateRepo.EXPECT().
		FindByID(gomock.Any(), gomock.Any()).
		Return(nil, pkg.EntityNotFoundError{EntityType: "template"})

	uc := &UseCase{
		Logger:         log.NewNop(),
		Tracer:         noop.NewTracerProvider().Tracer("test"),
		MetricsFactory: factory,
		TemplateRepo:   mockTemplateRepo,
	}

	ctx := context.Background()

	_, err := uc.GetTemplateByID(ctx, uuid.New())
	require.Error(t, err)
	require.True(t, pkg.IsBusinessError(err), "expected the not-found error to be classified as a business error")

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
	assert.Equal(t, opGetTemplate, operation.AsString())
	assert.Equal(t, "business_error", result.AsString())

	_, durOK := findDomainMetric(t, rm, "domain_operation_duration_ms")
	assert.True(t, durOK, "domain_operation_duration_ms must be registered")
}

// TestRecordDomainOp_NilFactory_NoPanic guards the disabled-telemetry path: a
// UseCase with a nil MetricsFactory must treat emission as a no-op.
func TestRecordDomainOp_NilFactory_NoPanic(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTemplateRepo := template.NewMockRepository(ctrl)
	mockTemplateRepo.EXPECT().
		FindByID(gomock.Any(), gomock.Any()).
		Return(nil, pkg.EntityNotFoundError{EntityType: "template"})

	uc := &UseCase{
		Logger:         log.NewNop(),
		Tracer:         noop.NewTracerProvider().Tracer("test"),
		MetricsFactory: nil,
		TemplateRepo:   mockTemplateRepo,
	}

	assert.NotPanics(t, func() {
		_, _ = uc.GetTemplateByID(context.Background(), uuid.New())
	})
}
