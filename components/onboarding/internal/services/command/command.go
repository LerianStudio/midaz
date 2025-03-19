package command

import (
	"context"
	"time"

	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/organization"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/portfolio"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/segment"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/redis"
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
	// Entity is the name of the entity (e.g., "organization", "ledger", "segment")
	Entity string

	// Action is the operation being performed (e.g., "create", "update", "delete")
	Action string

	// ID is the identifier of the entity instance (e.g., organization ID, ledger ID)
	ID string

	// IDLabel is the label for the ID attribute (e.g., "organization_id", "ledger_id")
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

// UseCase is a struct that aggregates various repositories for simplified access in use case implementation.
type UseCase struct {
	// OrganizationRepo provides an abstraction on top of the organization data source.
	OrganizationRepo organization.Repository

	// LedgerRepo provides an abstraction on top of the ledger data source.
	LedgerRepo ledger.Repository

	// SegmentRepo provides an abstraction on top of the segment data source.
	SegmentRepo segment.Repository

	// PortfolioRepo provides an abstraction on top of the portfolio data source.
	PortfolioRepo portfolio.Repository

	// AccountRepo provides an abstraction on top of the account data source.
	AccountRepo account.Repository

	// AssetRepo provides an abstraction on top of the asset data source.
	AssetRepo asset.Repository

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
func NewUseCase(serviceName string, organizationRepo organization.Repository, ledgerRepo ledger.Repository,
	segmentRepo segment.Repository, portfolioRepo portfolio.Repository, accountRepo account.Repository,
	assetRepo asset.Repository, metadataRepo mongodb.Repository, rabbitMQRepo rabbitmq.ProducerRepository,
	redisRepo redis.RedisRepository) *UseCase {
	uc := &UseCase{
		ServiceName:      serviceName,
		OrganizationRepo: organizationRepo,
		LedgerRepo:       ledgerRepo,
		SegmentRepo:      segmentRepo,
		PortfolioRepo:    portfolioRepo,
		AccountRepo:      accountRepo,
		AssetRepo:        assetRepo,
		MetadataRepo:     metadataRepo,
		RabbitMQRepo:     rabbitMQRepo,
		RedisRepo:        redisRepo,
		Telemetry:        &EntityTelemetry{ServiceName: serviceName},
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

// NewOrganizationOperation creates an operation for organization entity
func (et *EntityTelemetry) NewOrganizationOperation(action, organizationID string) *EntityOperation {
	return et.NewEntityOperation("organization", action, organizationID)
}

// NewLedgerOperation creates an operation for ledger entity
func (et *EntityTelemetry) NewLedgerOperation(action, ledgerID string) *EntityOperation {
	return et.NewEntityOperation("ledger", action, ledgerID)
}

// NewSegmentOperation creates an operation for segment entity
func (et *EntityTelemetry) NewSegmentOperation(action, segmentID string) *EntityOperation {
	return et.NewEntityOperation("segment", action, segmentID)
}

// NewPortfolioOperation creates an operation for portfolio entity
func (et *EntityTelemetry) NewPortfolioOperation(action, portfolioID string) *EntityOperation {
	return et.NewEntityOperation("portfolio", action, portfolioID)
}

// NewAccountOperation creates an operation for account entity
func (et *EntityTelemetry) NewAccountOperation(action, accountID string) *EntityOperation {
	return et.NewEntityOperation("account", action, accountID)
}

// NewAssetOperation creates an operation for asset entity
func (et *EntityTelemetry) NewAssetOperation(action, assetID string) *EntityOperation {
	return et.NewEntityOperation("asset", action, assetID)
}
