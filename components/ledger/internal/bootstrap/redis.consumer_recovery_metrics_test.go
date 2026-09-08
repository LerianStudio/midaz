// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/lib-observability/v4/metrics"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
)

func TestFinalizeRecoveryRecord_EmitsBoundedOutcomeMetrics(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		consumer    func(*metrics.MetricsFactory) *RedisQueueConsumer
		ctx         func() context.Context
		status      int64
		finalizeErr error
		ackErr      error
		want        string
		wantErr     bool
	}{
		{name: "deleted", status: 1, want: recoveryMetricOutcomeCompleted},
		{name: "already absent", status: 0, want: recoveryMetricOutcomeCompleted},
		{name: "record changed", status: 2, want: recoveryMetricOutcomeRecordChanged, wantErr: true},
		{name: "invalid ack", status: 3, want: recoveryMetricOutcomeInvalidAck, wantErr: true},
		{name: "finalization failed", finalizeErr: errors.New("durability uncertain"), want: recoveryMetricOutcomeFinalizationFailed, wantErr: true},
		{name: "ack failed", status: 1, ackErr: errors.New("response lost"), want: recoveryMetricOutcomeAckFailed, wantErr: true},
		{name: "finalizer deadline", finalizeErr: context.DeadlineExceeded, want: recoveryMetricOutcomeContextCanceled, wantErr: true},
		{name: "ack canceled", ackErr: context.Canceled, want: recoveryMetricOutcomeContextCanceled, wantErr: true},
		{name: "missing finalizer", consumer: func(factory *metrics.MetricsFactory) *RedisQueueConsumer {
			return (&RedisQueueConsumer{metricsFactory: factory}).WithBalanceEngineFinalizer(nil)
		}, want: recoveryMetricOutcomeNotConfigured, wantErr: true},
		{name: "missing acknowledgment", consumer: func(factory *metrics.MetricsFactory) *RedisQueueConsumer {
			return (&RedisQueueConsumer{metricsFactory: factory}).WithBalanceEngineFinalizer(&recoveryFinalizerStub{})
		}, want: recoveryMetricOutcomeNotConfigured, wantErr: true},
		{name: "context canceled", ctx: func() context.Context {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			return ctx
		}, want: recoveryMetricOutcomeContextCanceled, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			reader := sdkmetric.NewManualReader()
			provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
			t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
			factory, err := metrics.NewMetricsFactory(provider.Meter("recovery-metrics-test"), nil)
			require.NoError(t, err)
			order := []string{}
			finalizer := &recoveryFinalizerStub{err: test.finalizeErr, order: &order}
			queue := &recoveryQueueStub{status: test.status, err: test.ackErr, order: &order}
			var consumer *RedisQueueConsumer
			if test.consumer != nil {
				consumer = test.consumer(factory)
			}
			if consumer == nil {
				consumer = (&RedisQueueConsumer{queue: queue}).WithBalanceEngineFinalizer(finalizer).WithMetricsFactory(factory)
			}
			ctx := context.Background()
			if test.ctx != nil {
				ctx = test.ctx()
			}
			gotErr := consumer.finalizeRecoveryRecord(ctx, "field", "raw", &command.BalanceEngineRecoveryEnvelope{})
			if test.wantErr {
				require.Error(t, gotErr)
			} else {
				require.NoError(t, gotErr)
			}
			switch {
			case test.consumer != nil || ctx.Err() != nil:
				require.Zero(t, finalizer.calls)
				require.Zero(t, queue.calls)
				require.Empty(t, order)
				if ctx.Err() != nil {
					require.ErrorIs(t, gotErr, ctx.Err())
				}
			case test.finalizeErr != nil:
				require.ErrorIs(t, gotErr, test.finalizeErr)
				require.Equal(t, 1, finalizer.calls)
				require.Zero(t, queue.calls)
				require.Equal(t, []string{"durable-finalization"}, order)
			default:
				require.Equal(t, 1, finalizer.calls)
				require.Equal(t, 1, queue.calls)
				require.Equal(t, []string{"durable-finalization", "conditional-ack"}, order)
				require.Equal(t, "field", queue.field)
				require.Equal(t, "raw", queue.payload)
				if test.ackErr != nil {
					require.ErrorIs(t, gotErr, test.ackErr)
				}
			}
			var data metricdata.ResourceMetrics
			require.NoError(t, reader.Collect(context.Background(), &data))
			counter := findRecoveryCounter(data)
			require.NotNil(t, counter)
			require.Len(t, counter.DataPoints, 1)
			require.Equal(t, int64(1), counter.DataPoints[0].Value)
			require.Equal(t, 1, counter.DataPoints[0].Attributes.Len())
			value, ok := counter.DataPoints[0].Attributes.Value(attribute.Key(recoveryMetricOutcomeLabel))
			require.True(t, ok)
			require.Equal(t, test.want, value.AsString())
			histogram := findRecoveryHistogram(data)
			require.NotNil(t, histogram)
			require.Len(t, histogram.DataPoints, 1)
			require.Equal(t, uint64(1), histogram.DataPoints[0].Count)
			require.Equal(t, 1, histogram.DataPoints[0].Attributes.Len())
			histogramOutcome, ok := histogram.DataPoints[0].Attributes.Value(attribute.Key(recoveryMetricOutcomeLabel))
			require.True(t, ok)
			require.Equal(t, test.want, histogramOutcome.AsString())
			require.Equal(t, recoveryMetricDurationBuckets, histogram.DataPoints[0].Bounds)
		})
	}
}

func TestFinalizeRecoveryRecord_MetricsAreNilFactorySafe(t *testing.T) {
	order := []string{}
	finalizer := &recoveryFinalizerStub{order: &order}
	queue := &recoveryQueueStub{status: 1, order: &order}
	consumer := (&RedisQueueConsumer{queue: queue}).WithBalanceEngineFinalizer(finalizer)
	require.NoError(t, consumer.finalizeRecoveryRecord(context.Background(), "field", "raw", &command.BalanceEngineRecoveryEnvelope{}))
	require.Equal(t, []string{"durable-finalization", "conditional-ack"}, order)
	require.Equal(t, 1, finalizer.calls)
	require.Equal(t, 1, queue.calls)
	require.Equal(t, "field", queue.field)
	require.Equal(t, "raw", queue.payload)
}

func findRecoveryCounter(data metricdata.ResourceMetrics) *metricdata.Sum[int64] {
	for _, scope := range data.ScopeMetrics {
		for i := range scope.Metrics {
			if scope.Metrics[i].Name == recoveryMetricName {
				if value, ok := scope.Metrics[i].Data.(metricdata.Sum[int64]); ok {
					return &value
				}
			}
		}
	}

	return nil
}

func findRecoveryHistogram(data metricdata.ResourceMetrics) *metricdata.Histogram[int64] {
	for _, scope := range data.ScopeMetrics {
		for i := range scope.Metrics {
			if scope.Metrics[i].Name == recoveryMetricDurationName {
				if value, ok := scope.Metrics[i].Data.(metricdata.Histogram[int64]); ok {
					return &value
				}
			}
		}
	}

	return nil
}
