// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package engine

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/LerianStudio/lib-observability/v4/metrics"
	"github.com/stretchr/testify/require"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	core "github.com/LerianStudio/midaz/v4/components/ledger/internal/engine"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
)

func TestExecutionOutcome_ClosedLabels(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name, outcome, code string
		err                 error
	}{
		{"success", "success", "", nil},
		{"refusal", "refused", core.FailureInsufficientFunds, &core.Failure{Code: core.FailureInsufficientFunds}},
		{"integrity refusal", "refused", core.FailureOnHoldUnderflow, &core.Failure{Code: core.FailureOnHoldUnderflow}},
		{"wrapped CAS", "refused", core.FailureStaleVersion, fmt.Errorf("wrapper: %w", &core.Failure{Code: core.FailureStaleVersion})},
		{"technical", "technical_error", "connection_unavailable", technical("connection_unavailable", false, errors.New("sensitive detail"))},
		{"uncertain", "indeterminate", "transport", fmt.Errorf("wrapper: %w", technical("transport", true, errors.New("sensitive detail")))},
		{"foreign error", "technical_error", "unknown", errors.New("sensitive detail")},
		{"foreign refusal", "technical_error", "unknown", &core.Failure{Code: "@private-alias#default"}},
		{"foreign technical", "indeterminate", "unknown", technical("@private-alias#default", true, nil)},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			outcome, code := executionOutcome(test.err)
			require.Equal(t, test.outcome, outcome)
			require.Equal(t, test.code, code)
		})
	}
	var nilTechnical *TechnicalError
	var nilRefusal *core.Failure
	for _, err := range []error{nilTechnical, nilRefusal} {
		outcome, code := executionOutcome(err)
		require.Equal(t, "technical_error", outcome)
		require.Equal(t, "unknown", code)
	}

	require.NotPanics(t, func() { recordExecutionOutcome(context.Background(), nil, nil, time.Second, nil) })
}

func TestAdapterExecute_CanceledRequestMetrics(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	meter := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { require.NoError(t, meter.Shutdown(context.Background())) })
	factory, err := metrics.NewMetricsFactory(meter.Meter("engine-test"), nil)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(libObservability.ContextWithMetricFactory(context.Background(), factory))
	cancel()
	_, limits, _ := validWireExecution()
	provider := &countingProvider{}
	adapter, err := NewAdapter(provider, limits)
	require.NoError(t, err)
	result, err := adapter.Execute(ctx, command.EngineExecution{})
	require.Nil(t, result)
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, provider.calls)
	var collected metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &collected))
	observed := map[string]metricdata.Metrics{}
	for _, scope := range collected.ScopeMetrics {
		for _, metric := range scope.Metrics {
			observed[metric.Name] = metric
		}
	}
	require.Contains(t, observed, "balance_engine_requests_total")
	requests, ok := observed["balance_engine_requests_total"].Data.(metricdata.Sum[int64])
	require.True(t, ok)
	require.Len(t, requests.DataPoints, 1)
	require.Equal(t, int64(1), requests.DataPoints[0].Value)
	labels := requests.DataPoints[0].Attributes.ToSlice()
	require.Len(t, labels, 1)
	require.Equal(t, "outcome", string(labels[0].Key))
	require.Equal(t, "technical_error", labels[0].Value.AsString())
	failures, ok := observed["balance_engine_failures_total"].Data.(metricdata.Sum[int64])
	require.True(t, ok)
	require.Len(t, failures.DataPoints, 1)
	require.Equal(t, int64(1), failures.DataPoints[0].Value)
	failureLabels := failures.DataPoints[0].Attributes.ToSlice()
	require.Len(t, failureLabels, 1)
	require.Equal(t, "code", string(failureLabels[0].Key))
	require.Equal(t, "context_canceled", failureLabels[0].Value.AsString())
	duration, ok := observed["balance_engine_duration_ms"].Data.(metricdata.Histogram[int64])
	require.True(t, ok)
	require.Len(t, duration.DataPoints, 1)
	require.Equal(t, uint64(1), duration.DataPoints[0].Count)
	require.Zero(t, duration.DataPoints[0].Attributes.Len())
	require.NotContains(t, observed, "balance_engine_cas_attempts_total")
	require.NotContains(t, observed, "balance_engine_postings_total")
	require.NotContains(t, observed, "balance_engine_request_size_bytes")
}
