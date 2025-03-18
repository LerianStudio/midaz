package command

import (
	"context"
	"time"

	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/pkg/mopentelemetry"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// UseCase is a struct that aggregates various repositories for simplified access in use case implementations.
type UseCase struct {
	// TransactionRepo provides an abstraction on top of the transaction data source.
	TransactionRepo transaction.Repository

	// OperationRepo provides an abstraction on top of the operation data source.
	OperationRepo operation.Repository

	// AssetRateRepo provides an abstraction on top of the asset rate data source.
	AssetRateRepo assetrate.Repository

	// BalanceRepo provides an abstraction on top of the balance data source.
	BalanceRepo balance.Repository

	// MetadataRepo provides an abstraction on top of the metadata data source.
	MetadataRepo mongodb.Repository

	// RabbitMQRepo provides an abstraction on top of the producer rabbitmq.
	RabbitMQRepo rabbitmq.ProducerRepository

	// RedisRepo provides an abstraction on top of the redis consumer.
	RedisRepo redis.RedisRepository

	// ServiceName is used for telemetry
	ServiceName string
}

// recordEntityMetric is a generalized function to record metrics for any entity
func (uc *UseCase) recordEntityMetric(ctx context.Context, entity string, action string, metricType string, value int64, attributes ...attribute.KeyValue) {
	// Create meter
	meter := otel.Meter(uc.ServiceName)

	// Create metric name based on entity and metric type
	metricName := mopentelemetry.GetMetricName("business", entity, metricType, "total")

	// Create counter
	counter, _ := meter.Int64Counter(
		metricName,
		metric.WithDescription(entity+" "+metricType+" by type"),
		metric.WithUnit("{count}"),
	)

	// Set base attributes
	baseAttrs := []attribute.KeyValue{
		attribute.String("action", action),
	}

	// Combine with additional attributes
	allAttrs := append(baseAttrs, attributes...)

	// Record the metric
	counter.Add(ctx, value, metric.WithAttributes(allAttrs...))
}

// recordEntityDuration records duration metrics for any entity
func (uc *UseCase) recordEntityDuration(ctx context.Context, entity string, startTime time.Time, action string, status string, attributes ...attribute.KeyValue) {
	// Calculate duration
	duration := time.Since(startTime).Milliseconds()

	// Create meter
	meter := otel.Meter(uc.ServiceName)

	// Create duration histogram
	durationHistogram, _ := meter.Int64Histogram(
		mopentelemetry.GetMetricName("business", entity, "duration", "milliseconds"),
		metric.WithDescription("Duration of "+entity+" processing"),
		metric.WithUnit("ms"),
	)

	// Set base attributes
	baseAttrs := []attribute.KeyValue{
		attribute.String("action", action),
		attribute.String("status", status),
	}

	// Combine with additional attributes
	allAttrs := append(baseAttrs, attributes...)

	// Record the metric
	durationHistogram.Record(ctx, duration, metric.WithAttributes(allAttrs...))
}

// recordEntityFloatValue records float value metrics for any entity
func (uc *UseCase) recordEntityFloatValue(ctx context.Context, entity string, metricType string, value float64, attributes ...attribute.KeyValue) {
	// Create meter
	meter := otel.Meter(uc.ServiceName)

	// Create float value counter
	valueCounter, _ := meter.Float64Counter(
		mopentelemetry.GetMetricName("business", entity, metricType, "value"),
		metric.WithDescription("Value metrics for "+entity),
		metric.WithUnit("unit"),
	)

	// Record the metric
	valueCounter.Add(ctx, value, metric.WithAttributes(attributes...))
}

// recordEntityError records error metrics for any entity
func (uc *UseCase) recordEntityError(ctx context.Context, entity string, errorType string, attributes ...attribute.KeyValue) {
	// Create meter
	meter := otel.Meter(uc.ServiceName)

	// Create error counter
	errorCounter, _ := meter.Int64Counter(
		mopentelemetry.GetMetricName("business", entity, "errors", "count"),
		metric.WithDescription("Number of "+entity+" errors by type"),
		metric.WithUnit("{error}"),
	)

	// Set base attributes
	baseAttrs := []attribute.KeyValue{
		attribute.String("error_type", errorType),
	}

	// Combine with additional attributes
	allAttrs := append(baseAttrs, attributes...)

	// Record the metric
	errorCounter.Add(ctx, 1, metric.WithAttributes(allAttrs...))
}

// Entity-specific recording functions

// RecordTransactionMetric records metrics for transaction entity
func (uc *UseCase) RecordTransactionMetric(ctx context.Context, action string, transactionID string, attributes ...attribute.KeyValue) {
	txAttrs := append(attributes, attribute.String("transaction_id", transactionID))
	uc.recordEntityMetric(ctx, "transaction", action, "count", 1, txAttrs...)
}

// RecordTransactionDuration records duration for transaction processing
func (uc *UseCase) RecordTransactionDuration(ctx context.Context, startTime time.Time, action string, status string, transactionID string, attributes ...attribute.KeyValue) {
	txAttrs := append(attributes, attribute.String("transaction_id", transactionID))
	uc.recordEntityDuration(ctx, "transaction", startTime, action, status, txAttrs...)
}

// RecordOperationMetric records metrics for operation entity
func (uc *UseCase) RecordOperationMetric(ctx context.Context, action string, operationID string, attributes ...attribute.KeyValue) {
	opAttrs := append(attributes, attribute.String("operation_id", operationID))
	uc.recordEntityMetric(ctx, "operation", action, "count", 1, opAttrs...)
}

// RecordBalanceMetric records metrics for balance entity
func (uc *UseCase) RecordBalanceMetric(ctx context.Context, action string, accountID string, attributes ...attribute.KeyValue) {
	balAttrs := append(attributes, attribute.String("account_id", accountID))
	uc.recordEntityMetric(ctx, "balance", action, "count", 1, balAttrs...)
}

// RecordBalanceUpdate records value updates for balance
func (uc *UseCase) RecordBalanceUpdate(ctx context.Context, assetCode string, amount float64, accountID string) {
	uc.recordEntityFloatValue(ctx, "balance", "update", amount,
		attribute.String("asset_code", assetCode),
		attribute.String("account_id", accountID))
}

// RecordAssetRateMetric records metrics for assetrate entity
func (uc *UseCase) RecordAssetRateMetric(ctx context.Context, action string, assetCode string, attributes ...attribute.KeyValue) {
	arAttrs := append(attributes, attribute.String("asset_code", assetCode))
	uc.recordEntityMetric(ctx, "assetrate", action, "count", 1, arAttrs...)
}

// Record entity errors
func (uc *UseCase) RecordEntityError(ctx context.Context, entity string, errorType string, entityID string, attributes ...attribute.KeyValue) {
	errAttrs := append(attributes, attribute.String(entity+"_id", entityID))
	uc.recordEntityError(ctx, entity, errorType, errAttrs...)
}
