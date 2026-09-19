// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"time"

	"github.com/LerianStudio/lib-observability/v4/metrics"
	"go.opentelemetry.io/otel/attribute"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

type bulkMetricKey struct {
	organizationID string
	ledgerID       string
}

type bulkMetricCounts struct {
	transactionsAttempted int64
	transactionsInserted  int64
	transactionsIgnored   int64
	operationsAttempted   int64
	operationsInserted    int64
	operationsIgnored     int64
	payloadCount          int64
}

func recordBulkOTelMetrics(
	ctx context.Context,
	factory *metrics.MetricsFactory,
	result *command.BulkResult,
	payloads []transaction.TransactionProcessingPayload,
	duration time.Duration,
) {
	if factory == nil {
		return
	}

	for key, counts := range aggregatePayloadsByOrgLedger(payloads, result) {
		attrs := []attribute.KeyValue{
			attribute.String("organization_id", key.organizationID),
			attribute.String("ledger_id", key.ledgerID),
		}

		recordBulkCounter(ctx, factory, utils.BulkRecorderTransactionsAttempted, counts.transactionsAttempted, attrs)
		recordBulkCounter(ctx, factory, utils.BulkRecorderTransactionsInserted, counts.transactionsInserted, attrs)
		recordBulkCounter(ctx, factory, utils.BulkRecorderTransactionsIgnored, counts.transactionsIgnored, attrs)
		recordBulkCounter(ctx, factory, utils.BulkRecorderOperationsAttempted, counts.operationsAttempted, attrs)
		recordBulkCounter(ctx, factory, utils.BulkRecorderOperationsInserted, counts.operationsInserted, attrs)
		recordBulkCounter(ctx, factory, utils.BulkRecorderOperationsIgnored, counts.operationsIgnored, attrs)
		recordBulkCounter(ctx, factory, utils.BulkRecorderBulkSize, counts.payloadCount, attrs)
		recordBulkHistogram(ctx, factory, utils.BulkRecorderBulkDuration, duration.Milliseconds(), attrs)
	}

	if result.FallbackUsed && result.FallbackCount > 0 {
		recordBulkCounter(ctx, factory, utils.BulkRecorderFallbackTotal, result.FallbackCount, nil)
	}
}

func aggregatePayloadsByOrgLedger(
	payloads []transaction.TransactionProcessingPayload,
	result *command.BulkResult,
) map[bulkMetricKey]*bulkMetricCounts {
	type payloadInfo struct {
		payloadCount   int64
		operationCount int64
	}

	infoByKey := make(map[bulkMetricKey]*payloadInfo)

	var totalPayloads, totalOperations int64

	for _, payload := range payloads {
		if payload.Transaction == nil {
			continue
		}

		key := bulkMetricKey{
			organizationID: payload.Transaction.OrganizationID,
			ledgerID:       payload.Transaction.LedgerID,
		}

		info, exists := infoByKey[key]
		if !exists {
			info = &payloadInfo{}
			infoByKey[key] = info
		}

		info.payloadCount++
		totalPayloads++

		operationCount := int64(len(payload.Transaction.Operations))
		info.operationCount += operationCount
		totalOperations += operationCount
	}

	counts := make(map[bulkMetricKey]*bulkMetricCounts, len(infoByKey))
	for key, info := range infoByKey {
		current := &bulkMetricCounts{payloadCount: info.payloadCount}
		if totalPayloads > 0 {
			ratio := float64(info.payloadCount) / float64(totalPayloads)
			current.transactionsAttempted = int64(float64(result.TransactionsAttempted) * ratio)
			current.transactionsInserted = int64(float64(result.TransactionsInserted) * ratio)
			current.transactionsIgnored = int64(float64(result.TransactionsIgnored) * ratio)
		}

		if totalOperations > 0 {
			ratio := float64(info.operationCount) / float64(totalOperations)
			current.operationsAttempted = int64(float64(result.OperationsAttempted) * ratio)
			current.operationsInserted = int64(float64(result.OperationsInserted) * ratio)
			current.operationsIgnored = int64(float64(result.OperationsIgnored) * ratio)
		}

		counts[key] = current
	}

	return counts
}

func recordBulkCounter(
	ctx context.Context,
	factory *metrics.MetricsFactory,
	metric metrics.Metric,
	value int64,
	attrs []attribute.KeyValue,
) {
	if factory == nil || value == 0 {
		return
	}

	counter, err := factory.Counter(metric)
	if err != nil {
		return
	}

	if len(attrs) > 0 {
		_ = counter.WithAttributes(attrs...).Add(ctx, value)
		return
	}

	_ = counter.Add(ctx, value)
}

func recordBulkHistogram(
	ctx context.Context,
	factory *metrics.MetricsFactory,
	metric metrics.Metric,
	value int64,
	attrs []attribute.KeyValue,
) {
	if factory == nil {
		return
	}

	histogram, err := factory.Histogram(metric)
	if err != nil {
		return
	}

	if len(attrs) > 0 {
		_ = histogram.WithAttributes(attrs...).Record(ctx, value)
		return
	}

	_ = histogram.Record(ctx, value)
}
