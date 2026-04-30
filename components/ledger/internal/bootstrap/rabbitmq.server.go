// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"os"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
	"github.com/LerianStudio/lib-commons/v5/commons/opentelemetry/metrics"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/vmihailenco/msgpack/v5"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type MultiQueueConsumer struct {
	consumerRoutes *rabbitmq.ConsumerRoutes
	UseCase        *command.UseCase
	metricsFactory *metrics.MetricsFactory
}

// NewMultiQueueConsumer create a new instance of MultiQueueConsumer.
// The metricsFactory parameter can be nil when telemetry is disabled.
func NewMultiQueueConsumer(routes *rabbitmq.ConsumerRoutes, useCase *command.UseCase, metricsFactory *metrics.MetricsFactory) *MultiQueueConsumer {
	consumer := &MultiQueueConsumer{
		consumerRoutes: routes,
		UseCase:        useCase,
		metricsFactory: metricsFactory,
	}

	queueName := os.Getenv("RABBITMQ_TRANSACTION_BALANCE_OPERATION_QUEUE")

	// Register individual handler (used for non-bulk mode and as fallback)
	routes.Register(queueName, consumer.handlerBTOQueue)

	// Register bulk handler (used when bulk mode is enabled)
	routes.RegisterBulk(queueName, consumer.handlerBTOBulkQueue)

	return consumer
}

// Run starts consumers for all registered queues.
func (mq *MultiQueueConsumer) Run(l *libCommons.Launcher) error {
	return mq.consumerRoutes.RunConsumers()
}

// handlerBTOQueue processes messages from the balance fifo queue, unmarshal the JSON, and update balances on database.
func (mq *MultiQueueConsumer) handlerBTOQueue(ctx context.Context, body []byte) error {
	return handlerBTO(ctx, body, mq.UseCase)
}

// handlerBTO is the standalone balance-transaction-operation handler.
// It unmarshals the message and delegates to the use case for async processing.
// Extracted as a package-level function so both the single-tenant MultiQueueConsumer
// and the multi-tenant consumer can reuse the same logic.
func handlerBTO(ctx context.Context, body []byte, useCase *command.UseCase) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "consumer.handler_balance_update")
	defer span.End()

	logger.Log(ctx, libLog.LevelInfo, "Processing message from balance_retry_queue_fifo")

	var message mmodel.Queue

	err := msgpack.Unmarshal(body, &message)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Error unmarshalling message JSON", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error unmarshalling balance message JSON: %v", err))

		return err
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Transaction message consumed: %s", message.QueueData[0].ID))

	err = useCase.CreateBalanceTransactionOperationsAsync(ctx, message)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Error creating transaction", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error creating transaction: %v", err))

		return err
	}

	return nil
}

// handlerBTOBulkQueue processes a batch of messages from the balance queue.
// Returns per-message results for acknowledgment handling.
func (mq *MultiQueueConsumer) handlerBTOBulkQueue(ctx context.Context, messages []amqp.Delivery) ([]rabbitmq.BulkMessageResult, error) {
	return handlerBTOBulk(ctx, messages, mq.UseCase, mq.metricsFactory)
}

// handlerBTOBulk is the bulk handler for balance-transaction-operation processing.
// It unmarshals all messages, extracts payloads, and calls CreateBulkTransactionOperationsAsync.
// The metricsFactory parameter can be nil when telemetry is disabled.
func handlerBTOBulk(ctx context.Context, messages []amqp.Delivery, useCase *command.UseCase, metricsFactory *metrics.MetricsFactory) ([]rabbitmq.BulkMessageResult, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "consumer.handler_balance_update_bulk")
	defer span.End()

	startTime := time.Now()

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Processing bulk of %d messages from balance_queue", len(messages)))

	// Extract payloads from all messages
	payloads := make([]transaction.TransactionProcessingPayload, 0, len(messages))

	for i, msg := range messages {
		var queueMsg mmodel.Queue

		if err := msgpack.Unmarshal(msg.Body, &queueMsg); err != nil {
			libOpentelemetry.HandleSpanError(span, "Error unmarshalling message in bulk", err)
			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error unmarshalling message %d in bulk: %v", i, err))

			// Return error to trigger fallback processing
			return nil, fmt.Errorf("failed to unmarshal message %d: %w", i, err)
		}

		// Extract payload from queue data
		if len(queueMsg.QueueData) == 0 {
			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Message %d has empty QueueData, skipping", i))

			continue
		}

		var payload transaction.TransactionProcessingPayload
		if err := msgpack.Unmarshal(queueMsg.QueueData[0].Value, &payload); err != nil {
			libOpentelemetry.HandleSpanError(span, "Error unmarshalling payload in bulk", err)
			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Error unmarshalling payload %d in bulk: %v", i, err))

			// Return error to trigger fallback processing
			return nil, fmt.Errorf("failed to unmarshal payload %d: %w", i, err)
		}

		payloads = append(payloads, payload)
	}

	if len(payloads) == 0 {
		logger.Log(ctx, libLog.LevelWarn, "No valid payloads extracted from bulk")

		// All messages processed (skipped), return empty results (all ack)
		return nil, nil
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Processing %d payloads in bulk", len(payloads)))

	// Call bulk processing
	result, err := useCase.CreateBulkTransactionOperationsAsync(ctx, payloads)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Bulk transaction processing failed", err)
		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Bulk transaction processing failed: %v", err))

		// Return error to trigger fallback processing
		return nil, err
	}

	duration := time.Since(startTime)

	// Record metrics as span attributes (if result is available)
	if result != nil {
		recordBulkMetrics(span, result, len(messages), len(payloads))
		// Record OpenTelemetry metrics with organization_id/ledger_id labels
		recordBulkOTelMetrics(ctx, metricsFactory, result, payloads, duration)
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Bulk processing completed successfully for %d payloads", len(payloads)),
		libLog.Any("transactions_attempted", result.TransactionsAttempted),
		libLog.Any("transactions_inserted", result.TransactionsInserted),
		libLog.Any("transactions_ignored", result.TransactionsIgnored),
		libLog.Any("operations_attempted", result.OperationsAttempted),
		libLog.Any("operations_inserted", result.OperationsInserted),
		libLog.Any("operations_ignored", result.OperationsIgnored),
	)

	// All succeeded - return nil results to indicate bulk ack
	return nil, nil
}

// recordBulkMetrics sets span attributes for bulk processing metrics.
func recordBulkMetrics(span trace.Span, result *command.BulkResult, messageCount, payloadCount int) {
	span.SetAttributes(
		// Message-level counts
		attribute.Int("bulk.messages_received", messageCount),
		attribute.Int("bulk.payloads_extracted", payloadCount),

		// Transaction counts
		attribute.Int64("bulk.transactions_attempted", result.TransactionsAttempted),
		attribute.Int64("bulk.transactions_inserted", result.TransactionsInserted),
		attribute.Int64("bulk.transactions_ignored", result.TransactionsIgnored),

		// Operation counts
		attribute.Int64("bulk.operations_attempted", result.OperationsAttempted),
		attribute.Int64("bulk.operations_inserted", result.OperationsInserted),
		attribute.Int64("bulk.operations_ignored", result.OperationsIgnored),

		// Fallback tracking
		attribute.Bool("bulk.fallback_used", result.FallbackUsed),
		attribute.Int64("bulk.fallback_count", result.FallbackCount),
	)
}

// bulkMetricKey groups payloads by organization and ledger for metric aggregation.
type bulkMetricKey struct {
	organizationID string
	ledgerID       string
}

// bulkMetricCounts holds aggregated counts for a single org/ledger pair.
type bulkMetricCounts struct {
	transactionsAttempted int64
	transactionsInserted  int64
	transactionsIgnored   int64
	operationsAttempted   int64
	operationsInserted    int64
	operationsIgnored     int64
	payloadCount          int64
}

// recordBulkOTelMetrics records OpenTelemetry counter and histogram metrics for bulk processing.
// Metrics are aggregated by organization_id and ledger_id labels as required by PRD.
// The metricsFactory parameter can be nil when telemetry is disabled.
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

	// Aggregate counts by organization_id/ledger_id
	countsByKey := aggregatePayloadsByOrgLedger(payloads, result)

	// Record counters for each org/ledger pair
	for key, counts := range countsByKey {
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

		// Record duration histogram per org/ledger (in milliseconds for precision)
		recordBulkHistogram(ctx, factory, utils.BulkRecorderBulkDuration, duration.Milliseconds(), attrs)
	}

	// Record fallback counter if fallback was used
	if result.FallbackUsed && result.FallbackCount > 0 {
		// Fallback is recorded without org/ledger labels since it applies to the entire bulk
		recordBulkCounter(ctx, factory, utils.BulkRecorderFallbackTotal, result.FallbackCount, nil)
	}
}

// aggregatePayloadsByOrgLedger groups payloads by organization_id and ledger_id.
// It distributes the bulk result counts proportionally based on payload distribution.
func aggregatePayloadsByOrgLedger(
	payloads []transaction.TransactionProcessingPayload,
	result *command.BulkResult,
) map[bulkMetricKey]*bulkMetricCounts {
	counts := make(map[bulkMetricKey]*bulkMetricCounts)

	// Count payloads per org/ledger and operations per payload
	type payloadInfo struct {
		payloadCount   int64
		operationCount int64
	}

	infoByKey := make(map[bulkMetricKey]*payloadInfo)
	totalPayloads := int64(0)
	totalOperations := int64(0)

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

		opCount := int64(len(payload.Transaction.Operations))
		info.operationCount += opCount
		totalOperations += opCount
	}

	// Distribute result counts proportionally
	for key, info := range infoByKey {
		c := &bulkMetricCounts{
			payloadCount: info.payloadCount,
		}

		// Distribute transaction counts proportionally by payload count
		if totalPayloads > 0 {
			ratio := float64(info.payloadCount) / float64(totalPayloads)
			c.transactionsAttempted = int64(float64(result.TransactionsAttempted) * ratio)
			c.transactionsInserted = int64(float64(result.TransactionsInserted) * ratio)
			c.transactionsIgnored = int64(float64(result.TransactionsIgnored) * ratio)
		}

		// Distribute operation counts proportionally by operation count
		if totalOperations > 0 {
			ratio := float64(info.operationCount) / float64(totalOperations)
			c.operationsAttempted = int64(float64(result.OperationsAttempted) * ratio)
			c.operationsInserted = int64(float64(result.OperationsInserted) * ratio)
			c.operationsIgnored = int64(float64(result.OperationsIgnored) * ratio)
		}

		counts[key] = c
	}

	return counts
}

// recordBulkCounter records a counter metric with the given value and attributes.
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
	} else {
		_ = counter.Add(ctx, value)
	}
}

// recordBulkHistogram records a histogram metric with the given value and attributes.
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
	} else {
		_ = histogram.Record(ctx, value)
	}
}
