// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/LerianStudio/lib-observability/v4/metrics"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestAtomicTransactionBatchMetricLabelsAreClosed(t *testing.T) {
	t.Parallel()

	require.Equal(t, atomicTransactionBatchOutcomeRejected, atomicTransactionBatchMetricOutcome("@private-alias"))
	require.Equal(t, "total", atomicTransactionBatchMetricPhase("01994f13-29b7-7000-8000-000000000001"))
	require.Equal(t, atomicTransactionBatchMetricCodeTechnical, atomicTransactionBatchMetricCodeLabel("idempotency-secret"))
	require.Equal(t, atomicTransactionBatchMetricDimensionNone, atomicTransactionBatchMetricDimensionLabel("987654321.99"))
	require.Equal(t, constant.ErrTransactionBatchBudgetExceeded.Error(), atomicTransactionBatchMetricCode(
		pkg.ValidateBusinessError(constant.ErrTransactionBatchBudgetExceeded, constant.EntityTransaction, "recoveryBytes", 0, 2, 1),
	))
	require.Equal(t, atomicTransactionBatchMetricCodeOtherBusiness, atomicTransactionBatchMetricCode(
		pkg.ValidationError{Code: "private-code", Message: "@private-alias"},
	))
	require.Equal(t, atomicTransactionBatchMetricCodeTechnical, atomicTransactionBatchMetricCode(
		errors.New("raw payload with idempotency-secret"),
	))
}

func TestAtomicTransactionBatchMetricsExposeOnlyCountsAndClosedLabels(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
	factory, err := metrics.NewMetricsFactory(provider.Meter("atomic-batch-telemetry-test"), nil)
	require.NoError(t, err)

	organizationID := uuid.MustParse("01994f13-29b7-7000-8000-000000000101")
	ledgerID := uuid.MustParse("01994f13-29b7-7000-8000-000000000102")
	first := atomicTransactionBatchItemInput(organizationID, ledgerID, "@private-source", "@private-shared")
	second := atomicTransactionBatchItemInput(organizationID, ledgerID, "@private-shared", "@private-destination")
	input := CreateAtomicTransactionBatchV2Input{
		Transactions:   []CreateAtomicTransactionBatchV2ItemInput{first, second},
		IdempotencyKey: "idempotency-secret",
	}
	run := &atomicTransactionBatchRun{
		batchID:            uuid.MustParse("01994f13-29b7-7000-8000-000000000103"),
		rejectionDimension: atomicTransactionBatchBudgetRecoveryBytes,
		items: []atomicTransactionBatchItemRun{
			{input: first.Transaction},
			{input: second.Transaction},
		},
		budgetMeasurements: &atomicTransactionBatchBudgetMeasurements{
			executionBalances:      []int{2, 3},
			completionPlanBytes:    []int{1024, 2048},
			accountingRequestBytes: []int{2048, 4096},
			preparedResponseBytes:  []int{4096, 8192},
			recoveryBytes:          []int{8192, 16384},
			cachedResponseBytes:    []int{16384, 32768},
		},
	}
	useCase := &UseCase{MetricsFactory: factory}
	ctx := context.Background()
	useCase.recordAtomicTransactionBatchReceived(ctx, input)
	useCase.recordAtomicTransactionBatchPhaseDuration(ctx, "preparation", 17*time.Millisecond)
	useCase.recordAtomicTransactionBatchCompleted(
		ctx,
		nil,
		run,
		pkg.ValidateBusinessError(
			constant.ErrTransactionBatchBudgetExceeded,
			constant.EntityTransaction,
			atomicTransactionBatchBudgetRecoveryBytes,
			1,
			16384,
			8192,
		),
		23*time.Millisecond,
	)
	useCase.recordAtomicTransactionBatchRecovering(ctx)
	useCase.recordAtomicTransactionBatchCompleted(
		ctx,
		&CreateAtomicTransactionBatchV2Result{},
		nil,
		nil,
		11*time.Millisecond,
	)
	useCase.recordAtomicTransactionBatchCompleted(
		ctx,
		&CreateAtomicTransactionBatchV2Result{Replayed: true},
		nil,
		nil,
		7*time.Millisecond,
	)

	var data metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(ctx, &data))
	observed := atomicBatchMetricData(t, data)
	for _, name := range []string{
		"atomic_transaction_batches_total",
		"atomic_transaction_batch_size",
		"atomic_transaction_batch_input_legs",
		"atomic_transaction_batch_expanded_postings",
		"atomic_transaction_batch_execution_balances",
		"atomic_transaction_batch_internal_bytes",
		"atomic_transaction_batch_duration_ms",
	} {
		require.Contains(t, observed, name)
	}

	outcomes := observed["atomic_transaction_batches_total"].Data.(metricdata.Sum[int64])
	require.Len(t, outcomes.DataPoints, 5)
	for _, point := range outcomes.DataPoints {
		require.Equal(t, 3, point.Attributes.Len())
		requireAtomicBatchMetricLabel(t, point.Attributes, "outcome", map[string]struct{}{
			atomicTransactionBatchOutcomeReceived:   {},
			atomicTransactionBatchOutcomeApplied:    {},
			atomicTransactionBatchOutcomeReplayed:   {},
			atomicTransactionBatchOutcomeRejected:   {},
			atomicTransactionBatchOutcomeRecovering: {},
		})
		requireAtomicBatchMetricLabel(t, point.Attributes, "code", map[string]struct{}{
			atomicTransactionBatchMetricCodeNone:               {},
			constant.ErrTransactionBatchBudgetExceeded.Error(): {},
		})
		requireAtomicBatchMetricLabel(t, point.Attributes, "dimension", map[string]struct{}{
			atomicTransactionBatchMetricDimensionNone: {},
			atomicTransactionBatchBudgetRecoveryBytes: {},
		})
	}

	internal := observed["atomic_transaction_batch_internal_bytes"].Data.(metricdata.Histogram[int64])
	require.Len(t, internal.DataPoints, 5)
	allowedKinds := map[string]struct{}{
		"completion_plan": {}, "accounting_request": {}, "prepared_response": {},
		"recovery": {}, "cached_response": {},
	}
	for _, point := range internal.DataPoints {
		requireAtomicBatchMetricLabel(t, point.Attributes, "kind", allowedKinds)
	}

	sensitive := []string{
		"@private-source",
		"@private-shared",
		"@private-destination",
		"01994f13-29b7-7000-8000-000000000101",
		"01994f13-29b7-7000-8000-000000000102",
		"01994f13-29b7-7000-8000-000000000103",
		"idempotency-secret",
		"10",
	}
	for _, metric := range observed {
		for _, labels := range atomicBatchMetricAttributeSets(metric.Data) {
			for _, label := range labels.ToSlice() {
				for _, forbidden := range sensitive {
					require.NotContains(t, label.Value.AsString(), forbidden)
				}
			}
		}
	}
}

func TestCreateAtomicTransactionBatchV2EmitsReceivedAndRejectedMetrics(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })
	factory, err := metrics.NewMetricsFactory(provider.Meter("atomic-batch-command-telemetry-test"), nil)
	require.NoError(t, err)

	result, err := (&UseCase{MetricsFactory: factory}).CreateAtomicTransactionBatchV2(
		context.Background(),
		CreateAtomicTransactionBatchV2Input{},
	)
	require.Nil(t, result)
	require.Error(t, err)

	var data metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &data))
	observed := atomicBatchMetricData(t, data)
	outcomes := observed["atomic_transaction_batches_total"].Data.(metricdata.Sum[int64])
	require.Len(t, outcomes.DataPoints, 2)

	want := map[string]string{
		atomicTransactionBatchOutcomeReceived: atomicTransactionBatchMetricCodeNone,
		atomicTransactionBatchOutcomeRejected: constant.ErrTransactionBatchCardinality.Error(),
	}
	for _, point := range outcomes.DataPoints {
		outcome, ok := point.Attributes.Value(attribute.Key("outcome"))
		require.True(t, ok)
		code, ok := point.Attributes.Value(attribute.Key("code"))
		require.True(t, ok)
		require.Equal(t, want[outcome.AsString()], code.AsString())
		delete(want, outcome.AsString())
	}
	require.Empty(t, want)
}

func atomicBatchMetricData(
	t *testing.T,
	data metricdata.ResourceMetrics,
) map[string]metricdata.Metrics {
	t.Helper()

	result := make(map[string]metricdata.Metrics)
	for _, scope := range data.ScopeMetrics {
		for _, metric := range scope.Metrics {
			result[metric.Name] = metric
		}
	}

	return result
}

func atomicBatchMetricAttributeSets(data metricdata.Aggregation) []attribute.Set {
	switch value := data.(type) {
	case metricdata.Sum[int64]:
		sets := make([]attribute.Set, len(value.DataPoints))
		for index := range value.DataPoints {
			sets[index] = value.DataPoints[index].Attributes
		}
		return sets
	case metricdata.Histogram[int64]:
		sets := make([]attribute.Set, len(value.DataPoints))
		for index := range value.DataPoints {
			sets[index] = value.DataPoints[index].Attributes
		}
		return sets
	default:
		return nil
	}
}

func requireAtomicBatchMetricLabel(
	t *testing.T,
	labels attribute.Set,
	key string,
	allowed map[string]struct{},
) {
	t.Helper()

	value, ok := labels.Value(attribute.Key(key))
	require.True(t, ok, "missing metric label %s", key)
	_, ok = allowed[value.AsString()]
	require.True(t, ok, "metric label %s has unbounded value %q", key, value.AsString())
}
