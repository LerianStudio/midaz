//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"testing"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/LerianStudio/lib-observability/v4/metrics"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
)

func TestIntegration_AdapterExecute_IndeterminateMetrics(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	meter := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { require.NoError(t, meter.Shutdown(context.Background())) })
	factory, err := metrics.NewMetricsFactory(meter.Meter("engine-test"), nil)
	require.NoError(t, err)
	ctx := libObservability.ContextWithMetricFactory(context.Background(), factory)
	_, address, password := newAdapterValkey(t)
	proxy := newAccountingProxy(t, address, true)
	client := redis.NewClient(&redis.Options{
		Addr: proxy.listener.Addr().String(), Password: password, DB: 2, Protocol: 2,
		TLSConfig: proxy.clientTLS, MaxRetries: 3,
	})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	input, limits := richAdapterExecution(t)
	adapter, err := NewAdapter(&integrationClientProvider{client: client}, limits)
	require.NoError(t, err)
	result, err := adapter.Execute(ctx, input)
	require.Nil(t, result)
	assertAdapterTechnical(t, err, "transport", true)
	require.Equal(t, 1, proxy.count("EVAL"))
	var collected metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &collected))
	wanted := map[string]string{
		"balance_engine_requests_total":      "indeterminate",
		"balance_engine_failures_total":      "transport",
		"balance_engine_indeterminate_total": "",
	}
	for _, scope := range collected.ScopeMetrics {
		for _, metric := range scope.Metrics {
			label, needed := wanted[metric.Name]
			if !needed {
				continue
			}
			counter, ok := metric.Data.(metricdata.Sum[int64])
			require.True(t, ok)
			require.Len(t, counter.DataPoints, 1)
			require.Equal(t, int64(1), counter.DataPoints[0].Value)
			labels := counter.DataPoints[0].Attributes.ToSlice()
			if label == "" {
				require.Empty(t, labels)
			} else {
				require.Len(t, labels, 1)
				require.Equal(t, label, labels[0].Value.AsString())
			}
			delete(wanted, metric.Name)
		}
	}
	require.Empty(t, wanted, "all outcome metrics must be emitted even after a lost response")
}

func TestIntegration_AdapterExecute_PreparedMetricsAndReplay(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	meter := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { require.NoError(t, meter.Shutdown(context.Background())) })
	factory, err := metrics.NewMetricsFactory(meter.Meter("engine-test"), nil)
	require.NoError(t, err)
	ctx := libObservability.ContextWithMetricFactory(context.Background(), factory)
	client, _, _ := newAdapterValkey(t)
	hook := &integrationCommandHook{}
	client.AddHook(hook)
	input, limits := richAdapterExecution(t)
	adapter, err := NewAdapter(&integrationClientProvider{client: client}, limits)
	require.NoError(t, err)
	first, err := adapter.Execute(ctx, input)
	require.NoError(t, err)
	replayed, err := adapter.Execute(ctx, input)
	require.NoError(t, err)
	require.Equal(t, first, replayed)
	require.Equal(t, int32(2), hook.evalSHA.Load())
	require.Equal(t, int32(1), hook.eval.Load())
	var collected metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &collected))
	observed := map[string]metricdata.Metrics{}
	for _, scope := range collected.ScopeMetrics {
		for _, metric := range scope.Metrics {
			observed[metric.Name] = metric
		}
	}
	for _, name := range []string{"balance_engine_requests_total", "balance_engine_postings_total", "balance_engine_cas_attempts_total"} {
		require.Contains(t, observed, name)
		counter, ok := observed[name].Data.(metricdata.Sum[int64])
		require.True(t, ok)
		require.Len(t, counter.DataPoints, 1)
		require.Equal(t, int64(2), counter.DataPoints[0].Value, "requests and requested postings count replay, NOSCRIPT does not add a CAS attempt")
		labels := counter.DataPoints[0].Attributes.ToSlice()
		switch name {
		case "balance_engine_requests_total":
			require.Len(t, labels, 1)
			require.Equal(t, "outcome", string(labels[0].Key))
			require.Equal(t, "success", labels[0].Value.AsString())
		case "balance_engine_postings_total":
			require.Len(t, labels, 1)
			require.Equal(t, "type", string(labels[0].Key))
			require.Equal(t, "debit", labels[0].Value.AsString())
		default:
			require.Empty(t, labels)
		}
	}
	resolved, err := resolveAdapterKeys(ctx, input.Request)
	require.NoError(t, err)
	prepared, err := prepareExecution(ctx, input, limits, resolved)
	require.NoError(t, err)
	require.Contains(t, observed, "balance_engine_request_size_bytes")
	size, ok := observed["balance_engine_request_size_bytes"].Data.(metricdata.Histogram[int64])
	require.True(t, ok)
	require.Len(t, size.DataPoints, 1)
	require.Equal(t, uint64(2), size.DataPoints[0].Count)
	require.Equal(t, int64(2*len(prepared.Payload)), size.DataPoints[0].Sum)
	require.Zero(t, size.DataPoints[0].Attributes.Len())
	require.NotContains(t, observed, "balance_engine_failures_total")
	require.NotContains(t, observed, "balance_engine_indeterminate_total")
}
