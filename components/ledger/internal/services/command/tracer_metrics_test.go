// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func collectTracerCounters(t *testing.T, reader *sdkmetric.ManualReader) map[string]int64 {
	t.Helper()
	var collected metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(t.Context(), &collected))
	result := make(map[string]int64)
	for _, scope := range collected.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != "tracer_coordination_total" {
				continue
			}
			sum, ok := metric.Data.(metricdata.Sum[int64])
			require.True(t, ok)
			for _, point := range sum.DataPoints {
				require.Equal(t, 2, point.Attributes.Len(), "no financial facts or identities in labels")
				operation, _ := point.Attributes.Value("operation")
				outcome, _ := point.Attributes.Value("result")
				result[operation.AsString()+"/"+outcome.AsString()] += point.Value
			}
		}
	}
	return result
}

func TestTracerMetricsBoundLabelsAndRecordDuration(t *testing.T) {
	reader, factory := newReaderFactory(t)
	emitTracerMetric(t.Context(), factory, "sensitive-transaction-id", "sensitive-error-payload", 7*time.Millisecond)
	require.Equal(t, map[string]int64{"unknown/unknown": 1}, collectTracerCounters(t, reader))
	var collected metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(t.Context(), &collected))
	found := false
	for _, scope := range collected.ScopeMetrics {
		for _, metric := range scope.Metrics {
			if metric.Name != "tracer_coordination_duration_ms" {
				continue
			}
			histogram, ok := metric.Data.(metricdata.Histogram[int64])
			require.True(t, ok)
			require.Len(t, histogram.DataPoints, 1)
			require.Equal(t, int64(7), histogram.DataPoints[0].Sum)
			require.Equal(t, 2, histogram.DataPoints[0].Attributes.Len())
			found = true
		}
	}
	require.True(t, found)
	emitTracerMetric(t.Context(), nil, "admission", "allow", time.Millisecond)
}
