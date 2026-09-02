// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package utils

import (
	"context"
	"testing"
	"time"

	"github.com/LerianStudio/lib-observability/v2/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func readerFactory(t *testing.T, name string) (*sdkmetric.ManualReader, *metrics.MetricsFactory) {
	t.Helper()

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { _ = mp.Shutdown(context.Background()) })

	factory, err := metrics.NewMetricsFactory(mp.Meter(name), nil)
	require.NoError(t, err)

	return reader, factory
}

func counterValue(t *testing.T, reader *sdkmetric.ManualReader, metricName string, labelKey, labelVal string) int64 {
	t.Helper()

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))

	var total int64

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != metricName {
				continue
			}

			sum, ok := m.Data.(metricdata.Sum[int64])
			require.True(t, ok)

			for _, dp := range sum.DataPoints {
				if labelKey == "" {
					total += dp.Value
					continue
				}

				if v, present := dp.Attributes.Value(attribute.Key(labelKey)); present && v.AsString() == labelVal {
					total += dp.Value
				}
			}
		}
	}

	return total
}

func histogramCount(t *testing.T, reader *sdkmetric.ManualReader, metricName string) uint64 {
	t.Helper()

	var rm metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &rm))

	var count uint64

	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != metricName {
				continue
			}

			h, ok := m.Data.(metricdata.Histogram[int64])
			require.True(t, ok)

			for _, dp := range h.DataPoints {
				count += dp.Count
			}
		}
	}

	return count
}

// TestRecordAccountExceptionEvaluation covers the evaluation counter helper for a
// real factory (emits the labelled point) and a nil factory (no-op, no panic).
func TestRecordAccountExceptionEvaluation(t *testing.T) {
	t.Parallel()

	reader, factory := readerFactory(t, "eval-counter-test")
	ctx := context.Background()

	RecordAccountExceptionEvaluation(ctx, factory, nil, "ledger", "granted")
	RecordAccountExceptionEvaluation(ctx, factory, nil, "ledger", "store_error")

	assert.Equal(t, int64(1), counterValue(t, reader, "account_exception_evaluations_total", "result", "granted"))
	assert.Equal(t, int64(1), counterValue(t, reader, "account_exception_evaluations_total", "result", "store_error"))

	// Nil factory is a no-op.
	RecordAccountExceptionEvaluation(ctx, nil, nil, "ledger", "granted")
}

// TestRecordAccountExceptionEvaluationDuration covers the duration histogram
// helper and the nil-factory no-op.
func TestRecordAccountExceptionEvaluationDuration(t *testing.T) {
	t.Parallel()

	reader, factory := readerFactory(t, "eval-duration-test")
	ctx := context.Background()

	RecordAccountExceptionEvaluationDuration(ctx, factory, nil, "ledger", time.Now().Add(-2*time.Millisecond))

	assert.Equal(t, uint64(1), histogramCount(t, reader, "account_exception_evaluation_duration_ms"))

	RecordAccountExceptionEvaluationDuration(ctx, nil, nil, "ledger", time.Now())
}

// TestRecordBlockedAccountRejection covers the blocked-rejection counter helper
// and the nil-factory no-op.
func TestRecordBlockedAccountRejection(t *testing.T) {
	t.Parallel()

	reader, factory := readerFactory(t, "blocked-rejection-helper-test")
	ctx := context.Background()

	RecordBlockedAccountRejection(ctx, factory, nil, "ledger")
	RecordBlockedAccountRejection(ctx, factory, nil, "ledger")

	assert.Equal(t, int64(2), counterValue(t, reader, "blocked_account_rejections_total", "", ""))

	RecordBlockedAccountRejection(ctx, nil, nil, "ledger")
}

// TestAccountExceptionMetricDeclarations asserts the metric metadata for the
// three account-exception metrics.
func TestAccountExceptionMetricDeclarations(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "account_exception_evaluations_total", AccountExceptionEvaluationsTotal.Name)
	assert.Equal(t, "1", AccountExceptionEvaluationsTotal.Unit)
	assert.Equal(t, "account_exception_evaluation_duration_ms", AccountExceptionEvaluationDuration.Name)
	assert.Equal(t, "ms", AccountExceptionEvaluationDuration.Unit)
	assert.Equal(t, "blocked_account_rejections_total", BlockedAccountRejectionsTotal.Name)
	assert.Equal(t, "1", BlockedAccountRejectionsTotal.Unit)
}
