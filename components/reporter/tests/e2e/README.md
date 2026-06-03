# E2E Tests

End-to-end tests for the Reporter application using the `itestkit` framework. These tests validate the complete flow from API requests through report generation to result storage.

## Table of Contents

- [Quick Start](#quick-start)
- [Environment Variables](#environment-variables)
- [Prerequisites](#prerequisites)
- [Test Architecture](#test-architecture)
- [Project Structure](#project-structure)
- [Important Patterns](#important-patterns)
  - [Template Creation](#template-creation)
  - [Report Generation Flow](#report-generation-flow)
  - [Polling for Completion](#polling-for-completion)
- [Creating New Tests](#creating-new-tests)
- [Infrastructure Components](#infrastructure-components)
- [Troubleshooting](#troubleshooting)
- [Best Practices](#best-practices)
- [Test Catalog](#test-catalog)
  - [Infrastructure - Health](#infrastructure---health)
  - [Infrastructure - Security](#infrastructure---security)
  - [Infrastructure - Data Sources](#infrastructure---data-sources)
  - [Report - Management](#report---management)
  - [Report - Generation Pipeline](#report---generation-pipeline)
  - [Report - Filters](#report---filters)
  - [Template - CRUD](#template---crud)
  - [Template - Report Validation](#template---report-validation)
  - [Validation - Query Parameters](#validation---query-parameters)
  - [Resilience - Error & Retry](#resilience---error--retry)
  - [Tenant - Isolation](#tenant---isolation)

## Quick Start

```bash
# Run all e2e tests (pre-built images)
make test-e2e

# Build images from source (requires GitHub token for private deps)
make test-e2e E2E_SKIP_BUILD=false GITHUB_TOKEN=`cat .secrets/github_token.txt`

# Run with custom images
make test-e2e MANAGER_IMAGE=reporter-manager:v1.0 WORKER_IMAGE=reporter-worker:v1.0

# Run a specific test
go test -v -tags e2e ./tests/e2e -run TestTemplate_CreateHTML -timeout 10m

# Run with fixed ports (useful for debugging)
FIXED_PORT=true go test -v -tags e2e ./tests/e2e -timeout 30m

# Infrastructure-only mode (start infra, then debug Manager/Worker in IDE)
FIXED_PORT=true E2E_INFRA_ONLY=true go test -v -tags e2e ./tests/e2e -timeout 30m

# Save generated reports to disk for manual inspection
E2E_SAVE_REPORTS=true make test-e2e

# Save reports to a custom directory
E2E_SAVE_REPORTS=true E2E_REPORTS_DIR=./debug-reports make test-e2e
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `GITHUB_TOKEN` | `""` | GitHub token for building images with private dependencies |
| `E2E_SKIP_BUILD` | `true` | Skip Docker build, use pre-built images |
| `MANAGER_IMAGE` | `reporter-manager:latest` | Docker image for Manager container |
| `WORKER_IMAGE` | `reporter-worker:latest` | Docker image for Worker container |
| `FIXED_PORT` | `false` | Use fixed ports for infrastructure (MongoDB 27017, PostgreSQL 5432, RabbitMQ 5672, Redis 6379, MinIO 9000) |
| `E2E_INFRA_ONLY` | `false` | Start infrastructure only and block (for debugging Manager/Worker in IDE) |
| `E2E_SKIP_MANAGER` | `false` | Skip Manager container, use external Manager |
| `E2E_SKIP_WORKER` | `false` | Skip Worker container (for debugging Worker locally) |
| `E2E_ENABLE_MT` | `false` | Enable multi-tenant tests |
| `E2E_SAVE_REPORTS` | `false` | Save downloaded reports to disk for manual inspection |
| `E2E_REPORTS_DIR` | `/tmp/e2e-reports/` | Directory for saved reports (used with `E2E_SAVE_REPORTS=true`) |
| `E2E_DEBUG_LOG` | `false` | Print all HTTP requests/responses to stderr |

## Prerequisites

- Go 1.25+
- Docker with Docker Compose
- Pre-built Reporter images (`reporter-manager:latest`, `reporter-worker:latest`)

To build the images before running tests:

```bash
# Option 1: Build via make target
make test-e2e E2E_SKIP_BUILD=false GITHUB_TOKEN=`cat .secrets/github_token.txt`

# Option 2: Build images manually
cd components/manager && docker build -t reporter-manager:latest .
cd components/worker && docker build -t reporter-worker:latest .
```

## Test Architecture

The E2E tests spin up a complete test environment using [testcontainers-go](https://golang.testcontainers.org/):

```
+---------------------------------------------------------------+
|                        Test Environment                        |
+---------------------------------------------------------------+
|                                                                |
|  +----------+         +----------+                             |
|  | Manager  |<------->| MongoDB  |  (Reporter metadata)       |
|  +----------+         +----------+                             |
|       |                                                        |
|       | HTTP API (:4005)                                       |
|       v                                                        |
|  +----------+         +----------+                             |
|  |  Worker  |<------->| RabbitMQ |  (Report queue)             |
|  +----------+         +----------+                             |
|       |                                                        |
|       | Query & Generate                                       |
|       v                                                        |
|  +------------+       +----------+                             |
|  | PostgreSQL |       |  Redis   |  (Cache/Locking)            |
|  | (midaz_    |       +----------+                             |
|  | onboarding)|                                                |
|  +------------+       +----------+                             |
|                       |  MinIO   |  (S3 report storage)        |
|  +------------+       +----------+                             |
|  |  MongoDB   |                                                |
|  | (plugin_   |                                                |
|  |  crm)      |                                                |
|  +------------+                                                |
|   (Data Sources)                                               |
|                                                                |
+---------------------------------------------------------------+
```

## Project Structure

```
tests/e2e/
├── main_test.go                        # TestMain setup/teardown
├── infra_datasources_test.go           # Data source discovery and access
├── infra_health_test.go                # Health/readiness endpoint checks
├── infra_security_test.go              # Security headers, rate limiting, CORS
├── report_filters_test.go              # Filter operators (eq, gt, lt, between, in, nin)
├── report_generation_test.go           # Full report generation pipeline (all formats)
├── report_management_test.go           # Report CRUD, idempotency, downloads
├── resilience_error_retry_test.go      # Retry logic, DLQ, circuit breakers
├── template_crud_test.go               # Template CRUD, validation, injection prevention
├── template_report_validation_test.go  # Business template data validation (ACCS005, CADOC 4111, etc.)
├── tenant_isolation_test.go            # Multi-tenant isolation (E2E_ENABLE_MT=true)
├── validation_query_params_test.go     # Query parameter validation
├── shared/                             # Test utilities
│   ├── apps.go                         # StartManager, StartWorker, AppEnv
│   ├── assertions.go                   # Custom assertion helpers
│   ├── client.go                       # ManagerClient HTTP wrapper
│   ├── constants.go                    # Timeouts, ports, credentials, fixture paths
│   ├── helpers.go                      # Helper functions
│   └── infra.go                        # Infrastructure setup (MongoDB, PG, RabbitMQ, Redis, MinIO)
└── testdata/
    ├── init_postgres.sql               # PostgreSQL seed data (midaz_onboarding)
    ├── init_mongo.js                   # MongoDB seed data (plugin_crm)
    └── templates/                      # Template fixtures (21 files)
        ├── valid_html.tpl
        ├── valid_csv.tpl
        ├── valid_pdf.tpl
        ├── valid_txt.tpl
        ├── valid_xml.tpl
        ├── multi-source_html.tpl
        ├── schema-qualified_html.tpl
        ├── script-injection_html.tpl   # XSS test fixtures
        ├── event-handler-injection_html.tpl
        └── ...
```

### File Naming Convention

Test files use category prefixes to group related tests:

| Prefix | Category | Description |
|--------|----------|-------------|
| `infra_` | Infrastructure | Health checks, security, data sources |
| `report_` | Reports | CRUD, generation pipeline, filters |
| `template_` | Templates | Template CRUD and validation |
| `validation_` | Validation | Input validation and query parameters |
| `resilience_` | Resilience | Error handling, retry logic, circuit breakers |
| `tenant_` | Tenancy | Multi-tenant isolation |

## Important Patterns

### Template Creation

Templates are created via multipart form upload. Use the helper that reads fixture files from `testdata/templates/`:

```go
func TestMyFeature(t *testing.T) {
    t.Parallel()

    ctx, cancel := context.WithTimeout(context.Background(), shared.DefaultPollTimeout)
    defer cancel()

    // Create a template from a fixture file
    tpl := createTemplateForFormat(t, ctx, shared.FormatHTML, shared.FixtureValidHTML)

    // Template is now available for report generation
    t.Logf("Created template: %s", tpl.ID)
}
```

### Report Generation Flow

The full report generation flow is: create template -> create report -> poll status -> download result.

```go
// 1. Create template
tpl := createTemplateForFormat(t, ctx, shared.FormatCSV, shared.FixtureValidCSV)

// 2. Create report referencing the template
report := createTestReport(t, ctx, tpl.ID, nil)

// 3. Poll until finished
waitForReportStatus(t, ctx, report.ID, shared.StatusFinished, shared.DefaultPollTimeout)

// 4. Download the generated report
body := downloadReport(t, ctx, report.ID)
```

### Polling for Completion

Reports are generated asynchronously via RabbitMQ. Use the polling pattern to wait for completion:

```go
waitForReportStatus(t, ctx, reportID, shared.StatusFinished, shared.DefaultPollTimeout)
```

For PDF reports, use a longer timeout due to browser rendering:

```go
waitForReportStatus(t, ctx, reportID, shared.StatusFinished, shared.DefaultPollTimeout+shared.PDFExtraTimeout)
```

## Creating New Tests

### 1. Test File Structure

Create a new file `*_test.go` with the `e2e` build tag:

```go
//go:build e2e

package e2e

import (
    "context"
    "testing"

    shared "github.com/LerianStudio/reporter/tests/e2e/shared"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestMyFeature(t *testing.T) {
    t.Parallel()

    ctx, cancel := context.WithTimeout(context.Background(), shared.DefaultPollTimeout)
    defer cancel()

    // Use apiClient (global from main_test.go) to interact with the Manager API
    resp, err := apiClient.GetHealth(ctx)
    require.NoError(t, err)
    assert.Equal(t, 200, resp.StatusCode())
}
```

### 2. File Naming

Choose the appropriate prefix for your test file:

- `infra_*.go` - Infrastructure/platform concerns
- `report_*.go` - Report domain operations
- `template_*.go` - Template domain operations
- `validation_*.go` - Input validation
- `resilience_*.go` - Error handling and fault tolerance
- `tenant_*.go` - Multi-tenancy

### 3. Test Function Naming

Follow the pattern `Test<Feature>_<ScenarioName>`:

```go
func TestReport_CreateWithFilters(t *testing.T) { ... }
func TestTemplate_CreateMissingFile(t *testing.T) { ... }
func TestFilter_EqSingleValue(t *testing.T) { ... }
```

## Infrastructure Components

| Component | Purpose | Default Port |
|-----------|---------|--------------|
| MongoDB | Reporter metadata storage | 27017 |
| PostgreSQL | midaz_onboarding data source | 5432 |
| MongoDB (plugin_crm) | plugin_crm data source | 27018 |
| RabbitMQ | Report generation queue | 5672 |
| Redis | Cache and distributed locking | 6379 |
| MinIO | S3-compatible report storage | 9000 |

## Troubleshooting

### Build fails with "GITHUB_TOKEN not set"

Set `E2E_SKIP_BUILD=true` to use pre-built images, or provide the token:

```bash
# Option 1: Use pre-built images (default)
make test-e2e

# Option 2: Build from source
make test-e2e E2E_SKIP_BUILD=false GITHUB_TOKEN=`cat .secrets/github_token.txt`
```

### Tests timeout waiting for report completion

1. Increase timeout in `waitForReportStatus()`
2. Check Worker logs for processing errors
3. Verify RabbitMQ queue has consumers

### Container can't connect to host services

The framework rewrites `localhost` to `host.docker.internal` for container environments.

### Debugging with infrastructure-only mode

Start infrastructure only, then run Manager/Worker in your IDE:

```bash
# Terminal 1: Start infrastructure
FIXED_PORT=true E2E_INFRA_ONLY=true go test -v -tags e2e ./tests/e2e -timeout 30m

# Terminal 2: Run tests against local Manager/Worker
E2E_SKIP_MANAGER=true E2E_SKIP_WORKER=true go test -v -tags e2e ./tests/e2e -run TestMyTest -timeout 10m
```

### Saving reports for manual inspection

The template validation tests (`template_report_validation_test.go`) support saving downloaded
reports to disk so you can open and inspect them manually:

```bash
# Save to /tmp/e2e-reports/ (default)
E2E_SAVE_REPORTS=true go test -v -tags e2e ./tests/e2e -run TestTemplateReport -timeout 30m

# Save to a custom directory
E2E_SAVE_REPORTS=true E2E_REPORTS_DIR=./my-reports go test -v -tags e2e ./tests/e2e -run TestTemplateReport -timeout 30m
```

Files are saved as `<TestName>.<ext>` (e.g., `TestTemplateReport_AccountPDF.pdf`,
`TestTemplateReport_ACCS005.xml`). The output path is logged in the test output.

### Cleanup: Remove orphan containers

```bash
docker ps -a | grep -E "(mongo|rabbit|redis|minio|postgres)" | awk '{print $1}' | xargs docker rm -f
```

## Best Practices

1. **Always use `t.Parallel()`** as the first line in every test
2. **Use build tag `//go:build e2e`** on every test file
3. **Use `t.Cleanup()`** for resource cleanup to ensure cleanup runs even on failure
4. **Keep tests independent** - each test should set up its own templates and reports
5. **Use unique names** to avoid conflicts between parallel tests
6. **Use longer timeouts for PDF** - PDF rendering requires a headless browser
7. **Follow naming conventions** - `Test<Feature>_<Scenario>` for functions, category prefix for files

## Test Catalog

### Infrastructure - Health

| File | Test | Description |
|------|------|-------------|
| `infra_health_test.go` | `TestHealth_ManagerHealthEndpoint` | GET /health returns 200 |
| `infra_health_test.go` | `TestHealth_ManagerReadyEndpoint` | GET /ready returns 200 or 503 |
| `infra_health_test.go` | `TestHealth_ManagerReadyDependencies` | /ready checks all dependencies |
| `infra_health_test.go` | `TestHealth_ManagerVersionEndpoint` | Version endpoint returns valid data |
| `infra_health_test.go` | `TestHealth_WorkerHealthEndpoint` | Worker /health returns 200 |
| `infra_health_test.go` | `TestHealth_WorkerReadyEndpoint` | Worker /ready returns 200 or 503 |
| `infra_health_test.go` | `TestHealth_ManagerHealthResponseFormat` | Response JSON format validation |
| `infra_health_test.go` | `TestHealth_ManagerReadyResponseBody` | Valid JSON body response |
| `infra_health_test.go` | `TestHealth_MultipleHealthChecks` | Sequential health check consistency |
| `infra_health_test.go` | `TestHealth_ConcurrentHealthChecks` | Concurrent health check stability |

### Infrastructure - Security

| File | Test | Description |
|------|------|-------------|
| `infra_security_test.go` | `TestSec_XContentTypeOptions` | X-Content-Type-Options: nosniff |
| `infra_security_test.go` | `TestSec_XFrameOptions` | X-Frame-Options: DENY |
| `infra_security_test.go` | `TestSec_XXSSProtection` | XSS protection header |
| `infra_security_test.go` | `TestSec_StrictTransportSecurity` | HSTS header validation |
| `infra_security_test.go` | `TestSec_CORSAllowedOrigin` | CORS allowed origin |
| `infra_security_test.go` | `TestSec_CORSDisallowedOrigin` | CORS rejection |
| `infra_security_test.go` | `TestSec_CORSPreflight` | OPTIONS preflight handling |
| `infra_security_test.go` | `TestSec_AuthDisabledByDefault` | Auth disabled without config |
| `infra_security_test.go` | `TestSec_PathTraversalBlocked` | Path traversal attack prevention |

### Infrastructure - Data Sources

| File | Test | Description |
|------|------|-------------|
| `infra_datasources_test.go` | `TestDS_ListDataSources` | List returns midaz_onboarding and plugin_crm |
| `infra_datasources_test.go` | `TestDS_GetMidazOnboarding` | Get PostgreSQL data source details |
| `infra_datasources_test.go` | `TestDS_GetPluginCRM` | Get MongoDB data source details |
| `infra_datasources_test.go` | `TestDS_GetNotFound` | 404 for non-existent data source |
| `infra_datasources_test.go` | `TestDS_GetPathTraversal` | Path traversal attack prevention |
| `infra_datasources_test.go` | `TestDS_SchemaCaching` | Schema caching mechanisms |

### Report - Management

| File | Test | Description |
|------|------|-------------|
| `report_management_test.go` | `TestReport_CreateHTML` | Create HTML report |
| `report_management_test.go` | `TestReport_CreateCSV` | Create CSV report |
| `report_management_test.go` | `TestReport_CreateXML` | Create XML report |
| `report_management_test.go` | `TestReport_CreatePDF` | Create PDF report |
| `report_management_test.go` | `TestReport_CreateTXT` | Create TXT report |
| `report_management_test.go` | `TestReport_CreateWithFilters` | Report with applied filters |
| `report_management_test.go` | `TestReport_CreateMissingTemplateID` | Missing templateID returns 400 |
| `report_management_test.go` | `TestReport_CreateInvalidTemplateID` | Invalid templateID format |
| `report_management_test.go` | `TestReport_CreateTemplateNotFound` | Non-existent template returns 404 |
| `report_management_test.go` | `TestReport_IdempotencyHeader` | Idempotency-Key header support |
| `report_management_test.go` | `TestReport_IdempotencyBodyHash` | Body hash-based idempotency |
| `report_management_test.go` | `TestReport_IdempotencyConcurrentDuplicate` | Concurrent duplicate handling |
| `report_management_test.go` | `TestReport_GetByID` | GET report by ID |
| `report_management_test.go` | `TestReport_GetNotFound` | 404 for non-existent report |

### Report - Generation Pipeline

| File | Test | Description |
|------|------|-------------|
| `report_generation_test.go` | `TestGen_HTMLPipeline` | Full HTML generation flow |
| `report_generation_test.go` | `TestGen_CSVPipeline` | Full CSV generation flow |
| `report_generation_test.go` | `TestGen_XMLPipeline` | Full XML generation flow |
| `report_generation_test.go` | `TestGen_PDFPipeline` | Full PDF generation flow |
| `report_generation_test.go` | `TestGen_TXTPipeline` | Full TXT generation flow |
| `report_generation_test.go` | `TestGen_PostgreSQLDataSource` | Query PostgreSQL and generate report |
| `report_generation_test.go` | `TestGen_MongoDBDataSource` | Query MongoDB and generate report |
| `report_generation_test.go` | `TestGen_MultiSourceReport` | Report using both PG and MongoDB |
| `report_generation_test.go` | `TestGen_FilterEq` | Equality filter in pipeline |
| `report_generation_test.go` | `TestGen_FilterGtDate` | Greater-than date filter |
| `report_generation_test.go` | `TestGen_FilterBetweenDates` | Date range filter |
| `report_generation_test.go` | `TestGen_StatusTransitions` | Report status progression |
| `report_generation_test.go` | `TestGen_PDFTimeoutHandling` | PDF generation timeout handling |

### Report - Filters

| File | Test | Description |
|------|------|-------------|
| `report_filters_test.go` | `TestFilter_EqSingleValue` | Equality: eq operator |
| `report_filters_test.go` | `TestFilter_GtNumeric` | Greater-than numeric |
| `report_filters_test.go` | `TestFilter_GteDate` | Greater-than-or-equal date |
| `report_filters_test.go` | `TestFilter_LtNumeric` | Less-than numeric |
| `report_filters_test.go` | `TestFilter_LteDate` | Less-than-or-equal date |
| `report_filters_test.go` | `TestFilter_BetweenDates` | Date range filter |
| `report_filters_test.go` | `TestFilter_BetweenNumeric` | Numeric range filter |
| `report_filters_test.go` | `TestFilter_InList` | IN operator |
| `report_filters_test.go` | `TestFilter_NinExclusion` | NOT IN operator |
| `report_filters_test.go` | `TestFilter_CombinedSameField` | Multiple filters on same field |
| `report_filters_test.go` | `TestFilter_AcrossMultipleTables` | Filters across tables |
| `report_filters_test.go` | `TestFilter_EmptyFiltersAllData` | Empty filter returns all data |

### Template - CRUD

| File | Test | Description |
|------|------|-------------|
| `template_crud_test.go` | `TestTemplate_CreateHTML` | Create HTML template |
| `template_crud_test.go` | `TestTemplate_CreateCSV` | Create CSV template |
| `template_crud_test.go` | `TestTemplate_CreateXML` | Create XML template |
| `template_crud_test.go` | `TestTemplate_CreatePDF` | Create PDF template |
| `template_crud_test.go` | `TestTemplate_CreateTXT` | Create TXT template |
| `template_crud_test.go` | `TestTemplate_CreateMissingFile` | Missing file in multipart upload |
| `template_crud_test.go` | `TestTemplate_CreateInvalidExtension` | Invalid file extension |
| `template_crud_test.go` | `TestTemplate_CreateEmptyFile` | Empty template file |
| `template_crud_test.go` | `TestTemplate_CreateScriptInjection` | XSS script injection blocked |
| `template_crud_test.go` | `TestTemplate_CreateEventHandlerInjection` | Event handler injection blocked |
| `template_crud_test.go` | `TestTemplate_IdempotencyHeader` | Idempotency-Key header |
| `template_crud_test.go` | `TestTemplate_GetByID` | GET template by ID |
| `template_crud_test.go` | `TestTemplate_GetNotFound` | 404 for non-existent template |
| `template_crud_test.go` | `TestTemplate_ListNoFilters` | List all templates |
| `template_crud_test.go` | `TestTemplate_ListPagination` | Pagination support |
| `template_crud_test.go` | `TestTemplate_UpdateFullUpdate` | Full template update |
| `template_crud_test.go` | `TestTemplate_DeleteSuccess` | Successful deletion |
| `template_crud_test.go` | `TestTemplate_DeleteAlreadyDeleted` | Delete already deleted template |

### Template - Report Validation

These tests validate that specific business templates produce correctly structured reports
with the expected internal data. Reports can be saved to disk for manual inspection using
`E2E_SAVE_REPORTS=true`.

| File | Test | Description |
|------|------|-------------|
| `template_report_validation_test.go` | `TestTemplateReport_AccountPDF` | account_pdf.tpl — PDF pipeline with account data validation |
| `template_report_validation_test.go` | `TestTemplateReport_ACCS005` | ACCS005.tpl — Brazilian CCS XML with holder/account/alias data |
| `template_report_validation_test.go` | `TestTemplateReport_CADOC4111` | cadoc-4111.tpl — CADOC 4111 XML with operation balances |

### Validation - Query Parameters

| File | Test | Description |
|------|------|-------------|
| `validation_query_params_test.go` | `TestQP_InvalidLimitNonNumeric` | Non-numeric limit returns 400 |
| `validation_query_params_test.go` | `TestQP_InvalidLimitZero` | Zero limit returns 400 |
| `validation_query_params_test.go` | `TestQP_InvalidLimitNegative` | Negative limit validation |
| `validation_query_params_test.go` | `TestQP_LimitExceedsMax` | Limit exceeds maximum |
| `validation_query_params_test.go` | `TestQP_InvalidPageZero` | Page zero validation |
| `validation_query_params_test.go` | `TestQP_InvalidSortOrder` | Invalid sort order rejection |
| `validation_query_params_test.go` | `TestQP_InvalidOutputFormatInQuery` | Invalid output format |
| `validation_query_params_test.go` | `TestQP_DefaultSortOrderDesc` | Default sort order is descending |

### Resilience - Error & Retry

| File | Test | Description |
|------|------|-------------|
| `resilience_error_retry_test.go` | `TestErr_WorkerRetryTransient` | Transient error retry |
| `resilience_error_retry_test.go` | `TestErr_DLQNonRetryable` | Non-retryable errors to DLQ |
| `resilience_error_retry_test.go` | `TestErr_DLQMaxRetries` | Max retry limit enforcement |
| `resilience_error_retry_test.go` | `TestErr_CircuitBreakerOpens` | Circuit breaker opens on failures |
| `resilience_error_retry_test.go` | `TestErr_CircuitBreakerHalfOpen` | Half-open state recovery |
| `resilience_error_retry_test.go` | `TestErr_ExponentialBackoff` | Exponential backoff strategy |
| `resilience_error_retry_test.go` | `TestErr_RabbitMQReconnection` | RabbitMQ connection recovery |
| `resilience_error_retry_test.go` | `TestErr_CompensatingTransactionS3Failure` | S3 failure compensation |

### Tenant - Isolation

Requires `E2E_ENABLE_MT=true` to run.

| File | Test | Description |
|------|------|-------------|
| `tenant_isolation_test.go` | `TestMT_TenantATemplateNotVisibleToB` | Cross-tenant template isolation |
| `tenant_isolation_test.go` | `TestMT_TenantBCannotDownloadTenantAReport` | Cross-tenant download blocked |
| `tenant_isolation_test.go` | `TestMT_ReportUsesTenantSpecificDB` | Tenant-scoped database usage |
| `tenant_isolation_test.go` | `TestMT_RabbitMQMessageIncludesTenantID` | Messages include tenant ID |
| `tenant_isolation_test.go` | `TestMT_SingleTenantModeWithoutHeaders` | Backward compatibility |
| `tenant_isolation_test.go` | `TestMT_TenantIsolationTemplateList` | Template lists are tenant-scoped |
| `tenant_isolation_test.go` | `TestMT_TenantIsolationReportList` | Report lists are tenant-scoped |
