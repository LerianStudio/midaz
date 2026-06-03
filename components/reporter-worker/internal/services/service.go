// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"

	"github.com/LerianStudio/lib-observability/log"
	"go.opentelemetry.io/otel/trace"

	"github.com/LerianStudio/midaz/v3/components/reporter/pkg"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/fetcher"
	extractionRepo "github.com/LerianStudio/midaz/v3/components/reporter/pkg/mongodb/extraction"
	reportData "github.com/LerianStudio/midaz/v3/components/reporter/pkg/mongodb/report"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/pdf"
	reportSeaweedFS "github.com/LerianStudio/midaz/v3/components/reporter/pkg/seaweedfs/report"
	templateSeaweedFS "github.com/LerianStudio/midaz/v3/components/reporter/pkg/seaweedfs/template"
)

// ExtractionJobCreator abstracts the Fetcher client's extraction job creation
// so the worker can be tested without a real HTTP client.
type ExtractionJobCreator interface {
	CreateExtractionJob(ctx context.Context, jobReq fetcher.CreateExtractionJobRequest) (*fetcher.ExtractionJobResponse, error)
}

// UseCase is a struct that coordinates the handling of template files, report storage, external data sources, and report data.
type UseCase struct {
	// Logger is the structured logger injected at construction time.
	Logger log.Logger

	// Tracer is the OpenTelemetry tracer injected at construction time.
	Tracer trace.Tracer

	// TemplateSeaweedFS is a repository used to retrieve template files from SeaweedFS storage.
	TemplateSeaweedFS templateSeaweedFS.Repository

	// ReportSeaweedFS is a repository interface for storing report files in SeaweedFS.
	ReportSeaweedFS reportSeaweedFS.Repository

	// ExternalDataSources holds a thread-safe map of external data sources identified by their names.
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

	// FetcherClient provides extraction job creation via the Fetcher API.
	// When non-nil, the worker uses Fetcher mode (dual-mode dispatch).
	// When nil, the worker uses direct datasource querying (legacy mode).
	FetcherClient ExtractionJobCreator

	// ExtractionMappingRepo is a repository for tracking extraction job mappings in MongoDB.
	ExtractionMappingRepo extractionRepo.Repository

	// AppEncKey is the shared master key (APP_ENC_KEY) between Reporter and Fetcher.
	// Kept for backward compatibility. Derived keys below are preferred.
	AppEncKey string

	// StorageDecryptKey is the HKDF-derived AES-256 key for decrypting extracted data
	// from SeaweedFS. Derived from APP_ENC_KEY with context "fetcher-storage-encryption-v1".
	StorageDecryptKey []byte

	// ExternalHMACKey is the HKDF-derived key for verifying HMAC signatures on
	// extracted data. Derived from APP_ENC_KEY with context "fetcher-external-hmac-v1".
	ExternalHMACKey []byte

	// FetcherDataStorage provides access to the Fetcher's object storage (S3)
	// for downloading extracted data files in the notification flow.
	FetcherDataStorage FetcherDataDownloader
}

// FetcherDataDownloader abstracts downloading extracted data from Fetcher's storage.
type FetcherDataDownloader interface {
	DownloadFile(ctx context.Context, path string) ([]byte, error)
}
