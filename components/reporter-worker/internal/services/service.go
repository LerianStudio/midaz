// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"github.com/LerianStudio/lib-observability/log"
	"github.com/LerianStudio/lib-observability/metrics"
	"go.opentelemetry.io/otel/trace"

	fetcherengine "github.com/LerianStudio/fetcher/pkg/engine"

	pkg "github.com/LerianStudio/midaz/v4/pkg/reporter"
	reportData "github.com/LerianStudio/midaz/v4/pkg/reporter/mongodb/report"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/pdf"
	reportSeaweedFS "github.com/LerianStudio/midaz/v4/pkg/reporter/seaweedfs/report"
	templateSeaweedFS "github.com/LerianStudio/midaz/v4/pkg/reporter/seaweedfs/template"
)

// UseCase is a struct that coordinates the handling of template files, report storage, external data sources, and report data.
type UseCase struct {
	// Logger is the structured logger injected at construction time.
	Logger log.Logger

	// Tracer is the OpenTelemetry tracer injected at construction time.
	Tracer trace.Tracer

	// MetricsFactory emits the D6 domain operation metrics. Nil when telemetry
	// is disabled — RecordDomainOperation treats a nil factory as a no-op.
	MetricsFactory *metrics.MetricsFactory

	// TemplateSeaweedFS is a repository used to retrieve template files from SeaweedFS storage.
	TemplateSeaweedFS templateSeaweedFS.Repository

	// ReportSeaweedFS is a repository interface for storing report files in SeaweedFS.
	ReportSeaweedFS reportSeaweedFS.Repository

	// ExternalDataSources holds a thread-safe map of external data sources identified by their names.
	// It backs the plugin_crm org fan-out path; the embedded engine resolves
	// every other datasource through its tenant resolver.
	ExternalDataSources *pkg.SafeDataSources

	// ReportDataRepo is an interface for operations related to report data storage used in the reporting use case
	ReportDataRepo reportData.Repository

	// CircuitBreakerManager manages circuit breakers for external datasources
	CircuitBreakerManager pkg.CircuitBreakerExecutor

	// HealthChecker performs periodic health checks and reconnection attempts
	HealthChecker pkg.HealthCheckRunner

	// ReportTTL defines the Time To Live for reports (e.g., "1m", "1h", "7d", "30d"). Empty means no TTL.
	ReportTTL string

	// PdfPool provides PDF generation capabilities using Chrome headless
	PdfPool pdf.PDFGenerator

	// CryptoHashSecretKeyPluginCRM is the hash secret key for plugin_crm data operations.
	CryptoHashSecretKeyPluginCRM string

	// CryptoEncryptSecretKeyPluginCRM is the encryption secret key for plugin_crm data operations.
	CryptoEncryptSecretKeyPluginCRM string

	// Engine is the embedded in-process extraction engine
	// (github.com/LerianStudio/fetcher/pkg/engine). The generate-report handler
	// drives it (PlanExtraction / ExecuteExtraction) for every non-plugin_crm
	// datasource. A nil Engine fails the extraction closed rather than silently
	// returning empty data.
	Engine *fetcherengine.Engine

	// EngineMultiTenant mirrors the engine resolver's tenancy mode (set at
	// bootstrap from resolver.IsMultiTenant()). It gates the single-tenant tenant
	// placeholder: in multi-tenant mode an empty request tenant MUST fail closed
	// (tenant is the isolation boundary), never be substituted with a synthetic
	// tenant that would read a wrong/shared database.
	EngineMultiTenant bool
}
