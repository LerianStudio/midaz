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
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// EntityTelemetry handles all telemetry operations for business entities
type EntityTelemetry struct {
	// ServiceName is used to identify the service in telemetry data
	ServiceName string
}

// EntityOperation represents an operation being performed on an entity
type EntityOperation struct {
	// Entity is the name of the entity (e.g., "transaction", "balance", "operation")
	Entity string

	// Action is the operation being performed (e.g., "create", "update", "delete")
	Action string

	// ID is the identifier of the entity instance (e.g., transaction ID, account ID)
	ID string

	// IDLabel is the label for the ID attribute (e.g., "transaction_id", "account_id")
	IDLabel string

	// StartTime is when the operation started, used for duration calculation
	StartTime time.Time

	// Status represents the outcome of the operation (e.g., "success", "failure")
	Status string

	// Additional attributes to include in telemetry
	Attributes []attribute.KeyValue

	// Span for tracing, created when StartTrace is called
	span trace.Span

	// Context with the span, created when StartTrace is called
	ctx context.Context
}

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

	// Telemetry for easy access in use cases
	Telemetry *EntityTelemetry
}

// NewUseCase creates a new UseCase with initialized telemetry
func NewUseCase(serviceName string, transactionRepo transaction.Repository, operationRepo operation.Repository,
	assetRateRepo assetrate.Repository, balanceRepo balance.Repository, metadataRepo mongodb.Repository,
	rabbitMQRepo rabbitmq.ProducerRepository, redisRepo redis.RedisRepository) *UseCase {
	uc := &UseCase{
		ServiceName:     serviceName,
		TransactionRepo: transactionRepo,
		OperationRepo:   operationRepo,
		AssetRateRepo:   assetRateRepo,
		BalanceRepo:     balanceRepo,
		MetadataRepo:    metadataRepo,
		RabbitMQRepo:    rabbitMQRepo,
		RedisRepo:       redisRepo,
		Telemetry:       &EntityTelemetry{ServiceName: serviceName},
	}

	return uc
}

// NewEntityOperation creates a new EntityOperation instance
func (et *EntityTelemetry) NewEntityOperation(entity, action, id string) *EntityOperation {
	return &EntityOperation{
		Entity:     entity,
		Action:     action,
		ID:         id,
		IDLabel:    entity + "_id",
		StartTime:  time.Now(),
		Attributes: []attribute.KeyValue{},
	}
}

// WithAttribute adds a custom attribute to the EntityOperation
func (eo *EntityOperation) WithAttribute(key string, value string) *EntityOperation {
	eo.Attributes = append(eo.Attributes, attribute.String(key, value))
	return eo
}

// WithAttributes adds multiple custom attributes to the EntityOperation
func (eo *EntityOperation) WithAttributes(attributes ...attribute.KeyValue) *EntityOperation {
	eo.Attributes = append(eo.Attributes, attributes...)
	return eo
}

// StartTrace begins a trace span for this operation
func (eo *EntityOperation) StartTrace(ctx context.Context) context.Context {
	// Create standard attributes with entity and operation information
	attrs := append(eo.Attributes,
		attribute.String("entity", eo.Entity),
		attribute.String("action", eo.Action),
		attribute.String(eo.IDLabel, eo.ID))

	// Create a span for the operation
	tracer := otel.Tracer("business." + eo.Entity)
	eo.ctx, eo.span = tracer.Start(
		ctx,
		eo.Entity+"."+eo.Action,
		trace.WithAttributes(attrs...),
	)

	return eo.ctx
}

// RecordSystemicMetric records a count-based metric for this operation
func (eo *EntityOperation) RecordSystemicMetric(ctx context.Context) {
	// Create meter
	meter := otel.Meter("business." + eo.Entity)

	// Create metric name for systemic events
	metricName := mopentelemetry.GetMetricName("business", eo.Entity, "count", "total")

	// Create counter
	counter, _ := meter.Int64Counter(
		metricName,
		metric.WithDescription(eo.Entity+" operations count by type"),
		metric.WithUnit("{count}"),
	)

	// Prepare base attributes
	baseAttrs := []attribute.KeyValue{
		attribute.String("action", eo.Action),
		attribute.String(eo.IDLabel, eo.ID),
	}

	// Combine with additional attributes
	allAttrs := append(baseAttrs, eo.Attributes...)

	// Record the metric
	counter.Add(ctx, 1, metric.WithAttributes(allAttrs...))
}

// RecordBusinessMetric records a business-related metric with a float value
func (eo *EntityOperation) RecordBusinessMetric(ctx context.Context, metricType string, value float64) {
	// Create meter
	meter := otel.Meter("business." + eo.Entity)

	// Create metric name for business metrics
	metricName := mopentelemetry.GetMetricName("business", eo.Entity, metricType, "value")

	// Create value counter
	valueCounter, _ := meter.Float64Counter(
		metricName,
		metric.WithDescription("Business metrics for "+eo.Entity),
		metric.WithUnit("unit"),
	)

	// Prepare base attributes
	baseAttrs := []attribute.KeyValue{
		attribute.String("action", eo.Action),
		attribute.String(eo.IDLabel, eo.ID),
	}

	// Combine with additional attributes
	allAttrs := append(baseAttrs, eo.Attributes...)

	// Record the metric
	valueCounter.Add(ctx, value, metric.WithAttributes(allAttrs...))
}

// End completes the operation, recording duration and ending the trace span
func (eo *EntityOperation) End(ctx context.Context, status string) {
	// Set status for this operation
	eo.Status = status

	// Record duration metrics
	eo.recordDuration(ctx)

	// If we have an active span, finish it
	if eo.span != nil {
		defer eo.span.End()

		// Add final status to span
		eo.span.SetAttributes(attribute.String("status", status))

		// Set span status (success or error)
		if status != "success" {
			eo.span.SetStatus(codes.Error, "Operation failed with status: "+status)
		} else {
			eo.span.SetStatus(codes.Ok, "")
		}
	}
}

// RecordError records an error for this operation
func (eo *EntityOperation) RecordError(ctx context.Context, errorType string, err error) {
	// Create meter
	meter := otel.Meter("business." + eo.Entity)

	// Create error counter
	errorCounter, _ := meter.Int64Counter(
		mopentelemetry.GetMetricName("business", eo.Entity, "errors", "count"),
		metric.WithDescription("Number of "+eo.Entity+" errors by type"),
		metric.WithUnit("{error}"),
	)

	// Prepare base attributes
	baseAttrs := []attribute.KeyValue{
		attribute.String("error_type", errorType),
		attribute.String(eo.IDLabel, eo.ID),
	}

	// Combine with additional attributes
	allAttrs := append(baseAttrs, eo.Attributes...)

	// Record the error metric
	errorCounter.Add(ctx, 1, metric.WithAttributes(allAttrs...))

	// If we have an active span, record the error there too
	if eo.span != nil {
		eo.span.SetStatus(codes.Error, errorType+": "+err.Error())
		eo.span.RecordError(err)
	}
}

// recordDuration records the duration of this operation
func (eo *EntityOperation) recordDuration(ctx context.Context) {
	// Calculate duration
	duration := time.Since(eo.StartTime).Milliseconds()

	// Create meter
	meter := otel.Meter("business." + eo.Entity)

	// Create duration histogram
	durationHistogram, _ := meter.Int64Histogram(
		mopentelemetry.GetMetricName("business", eo.Entity, "duration", "milliseconds"),
		metric.WithDescription("Duration of "+eo.Entity+" processing"),
		metric.WithUnit("ms"),
	)

	// Prepare base attributes
	baseAttrs := []attribute.KeyValue{
		attribute.String("action", eo.Action),
		attribute.String("status", eo.Status),
		attribute.String(eo.IDLabel, eo.ID),
	}

	// Combine with additional attributes
	allAttrs := append(baseAttrs, eo.Attributes...)

	// Record the duration
	durationHistogram.Record(ctx, duration, metric.WithAttributes(allAttrs...))
}

// Convenience methods for common entities

// NewTransactionOperation creates an operation for transaction entity
func (et *EntityTelemetry) NewTransactionOperation(action, transactionID string) *EntityOperation {
	return et.NewEntityOperation("transaction", action, transactionID)
}

// NewBalanceOperation creates an operation for balance entity
func (et *EntityTelemetry) NewBalanceOperation(action, accountID string) *EntityOperation {
	return et.NewEntityOperation("balance", action, accountID)
}

// NewOperationOperation creates an operation for operation entity
func (et *EntityTelemetry) NewOperationOperation(action, operationID string) *EntityOperation {
	return et.NewEntityOperation("operation", action, operationID)
}

// NewAssetRateOperation creates an operation for assetrate entity
func (et *EntityTelemetry) NewAssetRateOperation(action, assetCode string) *EntityOperation {
	op := et.NewEntityOperation("assetrate", action, assetCode)
	op.IDLabel = "asset_code" // Override default ID label

	return op
}
