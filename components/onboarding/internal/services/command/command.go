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
	"go.opentelemetry.io/otel/metric"
)

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

	// ServiceName is used for OpenTelemetry metrics
	ServiceName string
}

// recordOnboardingMetrics records count metrics for onboarding operations
func (uc *UseCase) recordOnboardingMetrics(ctx context.Context, entityType, action string, attributes ...attribute.KeyValue) {
	// Create meter
	meter := otel.Meter(uc.ServiceName)

	// Create metric for onboarding entity counts by type
	onboardingCounter, _ := meter.Int64Counter(
		mopentelemetry.GetMetricName("business", "onboarding", "count", "total"),
		metric.WithDescription("Number of onboarding operations by entity type"),
		metric.WithUnit("{operation}"),
	)

	// Set base attributes
	baseAttrs := []attribute.KeyValue{
		attribute.String("entity_type", entityType),
		attribute.String("action", action),
	}

	// Combine with additional attributes
	allAttrs := append(baseAttrs, attributes...)

	// Record the metric
	onboardingCounter.Add(ctx, 1, metric.WithAttributes(allAttrs...))
}

// recordOnboardingDuration records duration metrics for onboarding operations
func (uc *UseCase) recordOnboardingDuration(ctx context.Context, startTime time.Time, entityType, action, status string, attributes ...attribute.KeyValue) {
	// Calculate duration
	duration := time.Since(startTime).Milliseconds()

	// Create meter
	meter := otel.Meter(uc.ServiceName)

	// Create duration histogram
	onboardingDuration, _ := meter.Int64Histogram(
		mopentelemetry.GetMetricName("business", "onboarding", "duration", "milliseconds"),
		metric.WithDescription("Duration of onboarding operations"),
		metric.WithUnit("ms"),
	)

	// Set base attributes
	baseAttrs := []attribute.KeyValue{
		attribute.String("entity_type", entityType),
		attribute.String("action", action),
		attribute.String("status", status),
	}

	// Combine with additional attributes
	allAttrs := append(baseAttrs, attributes...)

	// Record the metric
	onboardingDuration.Record(ctx, duration, metric.WithAttributes(allAttrs...))
}

// recordOnboardingError records error metrics for onboarding operations
func (uc *UseCase) recordOnboardingError(ctx context.Context, entityType, errorType string, attributes ...attribute.KeyValue) {
	// Create meter
	meter := otel.Meter(uc.ServiceName)

	// Create error counter
	onboardingErrorCounter, _ := meter.Int64Counter(
		mopentelemetry.GetMetricName("business", "onboarding", "errors", "count"),
		metric.WithDescription("Number of onboarding errors by entity type"),
		metric.WithUnit("{error}"),
	)

	// Set base attributes
	baseAttrs := []attribute.KeyValue{
		attribute.String("entity_type", entityType),
		attribute.String("error_type", errorType),
	}

	// Combine with additional attributes
	allAttrs := append(baseAttrs, attributes...)

	// Record the metric
	onboardingErrorCounter.Add(ctx, 1, metric.WithAttributes(allAttrs...))
}
