// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"github.com/LerianStudio/lib-observability/log"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v3/components/reporter/pkg"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/datasource"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/mongodb/deadline"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/mongodb/report"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/mongodb/template"
	pkgRabbitmq "github.com/LerianStudio/midaz/v3/components/reporter/pkg/rabbitmq"
	pkgRedis "github.com/LerianStudio/midaz/v3/components/reporter/pkg/redis"
	reportSeaweedFS "github.com/LerianStudio/midaz/v3/components/reporter/pkg/seaweedfs/report"
	templateSeaweedFS "github.com/LerianStudio/midaz/v3/components/reporter/pkg/seaweedfs/template"
)

// UseCase is a struct to implement the services methods
type UseCase struct {
	// Logger is the structured logger injected at construction time.
	Logger log.Logger

	// Tracer is the OpenTelemetry tracer injected at construction time.
	Tracer trace.Tracer

	// DeadlineRepo provides an abstraction on top of the deadline data source.
	DeadlineRepo deadline.Repository

	// TemplateRepo provides an abstraction on top of the template data source.
	TemplateRepo template.Repository

	// TemplateSeaweedFS is a repository interface for storing template files in SeaweedFS.
	TemplateSeaweedFS templateSeaweedFS.Repository

	// ReportRepo provides an abstraction on top of the report data source.
	ReportRepo report.Repository

	// ReportSeaweed is a repository interface for storing report files in SeaweedFS.
	ReportSeaweedFS reportSeaweedFS.Repository

	// RabbitMQRepo provides an abstraction on top of the producer rabbitmq.
	RabbitMQRepo pkgRabbitmq.ProducerRepository

	// DataSourceProvider abstracts data source operations (listing, schema retrieval)
	// behind a mode-agnostic interface. When set, GetDataSourceInformation and
	// GetDataSourceDetailsByID delegate to the provider instead of accessing
	// ExternalDataSources directly. This enables switching between direct-query
	// (legacy) and Fetcher-based (dual-mode) providers transparently.
	DataSourceProvider datasource.DataSourceProvider

	// ExternalDataSources holds a thread-safe map of external data sources identified by their names.
	// Deprecated: Use DataSourceProvider for new code paths. Retained for backward
	// compatibility with services that have not yet migrated to the provider interface.
	ExternalDataSources *pkg.SafeDataSources

	// RedisRepo provides an abstraction on top of the redis consumer.
	RedisRepo pkgRedis.RedisRepository

	// RabbitMQExchange is the exchange name for publishing report generation messages.
	RabbitMQExchange string

	// RabbitMQGenerateReportKey is the routing key for report generation messages.
	RabbitMQGenerateReportKey string

	// FetcherEnabled indicates whether the reporter operates in fetcher mode
	// (delegated extraction) vs direct mode (query databases directly).
	FetcherEnabled bool
}

// isFetcherMode returns true when the reporter delegates data operations to the fetcher.
func (uc *UseCase) isFetcherMode() bool {
	return uc.FetcherEnabled
}
