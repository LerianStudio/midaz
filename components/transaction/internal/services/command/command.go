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

// recordBusinessMetrics records business metrics related to transactions
func (uc *UseCase) recordBusinessMetrics(ctx context.Context, action string, attributes ...attribute.KeyValue) {
	// Create meter
	meter := otel.Meter(uc.ServiceName)

	// Create metric for transaction counts by type
	txCounter, _ := meter.Int64Counter(
		mopentelemetry.GetMetricName("business", "transaction", "count", "total"),
		metric.WithDescription("Number of transactions by type"),
		metric.WithUnit("{transaction}"),
	)

	// Set base attributes
	baseAttrs := []attribute.KeyValue{
		attribute.String("action", action),
	}

	// Combine with additional attributes
	allAttrs := append(baseAttrs, attributes...)

	// Record the metric
	txCounter.Add(ctx, 1, metric.WithAttributes(allAttrs...))
}

// recordTransactionDuration records duration metrics for transactions
func (uc *UseCase) recordTransactionDuration(ctx context.Context, startTime time.Time, _ string, status string, attributes ...attribute.KeyValue) {
	// Calculate duration
	duration := time.Since(startTime).Milliseconds()

	// Create meter
	meter := otel.Meter(uc.ServiceName)

	// Create duration histogram
	txDuration, _ := meter.Int64Histogram(
		mopentelemetry.GetMetricName("business", "transaction", "duration", "milliseconds"),
		metric.WithDescription("Duration of transaction processing"),
		metric.WithUnit("ms"),
	)

	// Set base attributes
	baseAttrs := []attribute.KeyValue{
		attribute.String("transaction_type", "create"), // hardcoded since it's always "create"
		attribute.String("status", status),
	}

	// Combine with additional attributes
	allAttrs := append(baseAttrs, attributes...)

	// Record the metric
	txDuration.Record(ctx, duration, metric.WithAttributes(allAttrs...))
}

// recordBalanceUpdates records metrics for balance updates
func (uc *UseCase) recordBalanceUpdates(ctx context.Context, assetCode string, amount float64) {
	// Create meter
	meter := otel.Meter(uc.ServiceName)

	// Create counters for total value moved by asset
	balanceValueCounter, _ := meter.Float64Counter(
		mopentelemetry.GetMetricName("business", "balance", "value", "moved"),
		metric.WithDescription("Total value moved by asset"),
		metric.WithUnit("unit"),
	)

	// Record the metric with asset code
	balanceValueCounter.Add(ctx, amount, metric.WithAttributes(
		attribute.String("asset_code", assetCode),
	))
}

// recordTransactionError records error metrics for transactions
func (uc *UseCase) recordTransactionError(ctx context.Context, errorType string, attributes ...attribute.KeyValue) {
	// Create meter
	meter := otel.Meter(uc.ServiceName)

	// Create error counter
	txErrorCounter, _ := meter.Int64Counter(
		mopentelemetry.GetMetricName("business", "transaction", "errors", "count"),
		metric.WithDescription("Number of transaction errors by type"),
		metric.WithUnit("{error}"),
	)

	// Set base attributes
	baseAttrs := []attribute.KeyValue{
		attribute.String("error_type", errorType),
	}

	// Combine with additional attributes
	allAttrs := append(baseAttrs, attributes...)

	// Record the metric
	txErrorCounter.Add(ctx, 1, metric.WithAttributes(allAttrs...))
}
