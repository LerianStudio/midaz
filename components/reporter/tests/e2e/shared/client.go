// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build e2e

package shared

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/go-resty/resty/v2"
)

// ManagerClient provides a typed HTTP client for the Reporter Manager API.
// It wraps resty.Client with Reporter-specific operations and automatic JSON serialization.
// All methods include both a typed version (returns parsed response) and a Raw version
// (returns *resty.Response for testing error scenarios).
type ManagerClient struct {
	client  *resty.Client
	baseURL string
}

// NewManagerClient creates a new Manager API client configured with the given base URL.
//
// Debug logging can be enabled by setting E2E_DEBUG_LOG=true in the environment.
// When enabled, all HTTP requests and responses (including headers and bodies)
// are printed to stderr, which is useful for diagnosing test failures.
func NewManagerClient(baseURL string) *ManagerClient {
	client := resty.New().
		SetBaseURL(baseURL).
		SetTimeout(HTTPClientTimeout)

	if os.Getenv("E2E_DEBUG_LOG") == "true" {
		client.SetDebug(true)
	}

	return &ManagerClient{client: client, baseURL: baseURL}
}

// PaginatedResponse represents a paginated list response from the Manager API.
type PaginatedResponse struct {
	Items []map[string]any `json:"items"`
	Page  int              `json:"page"`
	Limit int              `json:"limit"`
	Total int              `json:"total"`
}

// ReportResponse represents the API response for report operations.
type ReportResponse struct {
	ID         string         `json:"id"`
	TemplateID string         `json:"templateId"`
	Status     string         `json:"status"`
	Metadata   map[string]any `json:"metadata"`
	CreatedAt  string         `json:"createdAt"`
	UpdatedAt  string         `json:"updatedAt"`
}

// CreateReportRequest represents the request body for creating a report.
type CreateReportRequest struct {
	TemplateID string                                           `json:"templateId"`
	Filters    map[string]map[string]map[string]FilterCondition `json:"filters"`
}

// DataSourceInfo represents a data source summary from the list endpoint.
type DataSourceInfo struct {
	ID           string `json:"id"`
	ExternalName string `json:"externalName"`
	Type         string `json:"type"`
}

// DataSourceDetails represents the detailed data source information including tables.
type DataSourceDetails struct {
	ID           string            `json:"id"`
	ExternalName string            `json:"externalName"`
	Type         string            `json:"type"`
	Tables       []DataSourceTable `json:"tables"`
}

// DataSourceTable represents a table or collection within a data source.
type DataSourceTable struct {
	Name   string   `json:"name"`
	Fields []string `json:"fields"`
}

// ReadinessResponse represents the API response for the /readyz endpoint.
type ReadinessResponse struct {
	Status       string         `json:"status"`
	Dependencies map[string]any `json:"checks"`
}

// idempotencyHeader is the header name used for idempotency keys.
const idempotencyHeader = "X-Idempotency"

// checkStatus validates the HTTP response status code against expected values.
// Returns nil if the response status matches any expected code, otherwise returns an error
// with the method name, actual status code, and response body for debugging.
func checkStatus(resp *resty.Response, method string, expected ...int) error {
	for _, code := range expected {
		if resp.StatusCode() == code {
			return nil
		}
	}

	return fmt.Errorf("%s: unexpected status %d: %s", method, resp.StatusCode(), resp.String())
}

// ############################################################################
// Template Methods
// ############################################################################

// CreateTemplate uploads a template via multipart form POST /v1/templates.
// Returns the HTTP status code, parsed response body, and any error.
func (c *ManagerClient) CreateTemplate(ctx context.Context, tplBytes []byte, fileName, outputFormat, description string) (int, map[string]any, error) {
	resp, err := c.createTemplateRequest(ctx, tplBytes, fileName, outputFormat, description, "")
	if err != nil {
		return 0, nil, fmt.Errorf("CreateTemplate: %w", err)
	}

	var result map[string]any
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return resp.StatusCode(), nil, fmt.Errorf("CreateTemplate: unmarshal: %w", err)
	}

	return resp.StatusCode(), result, nil
}

// CreateTemplateRaw uploads a template and returns the raw *resty.Response.
// Use this for testing error scenarios where you need access to the raw response.
func (c *ManagerClient) CreateTemplateRaw(ctx context.Context, tplBytes []byte, fileName, outputFormat, description string) (*resty.Response, error) {
	return c.createTemplateRequest(ctx, tplBytes, fileName, outputFormat, description, "")
}

// CreateTemplateWithIdempotency uploads a template with an idempotency key.
// Returns the HTTP status code, parsed response body, and any error.
func (c *ManagerClient) CreateTemplateWithIdempotency(ctx context.Context, tplBytes []byte, fileName, outputFormat, description, idempotencyKey string) (int, map[string]any, error) {
	resp, err := c.createTemplateRequest(ctx, tplBytes, fileName, outputFormat, description, idempotencyKey)
	if err != nil {
		return 0, nil, fmt.Errorf("CreateTemplateWithIdempotency: %w", err)
	}

	var result map[string]any
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return resp.StatusCode(), nil, fmt.Errorf("CreateTemplateWithIdempotency: unmarshal: %w", err)
	}

	return resp.StatusCode(), result, nil
}

// createTemplateRequest builds and sends the multipart template upload request.
func (c *ManagerClient) createTemplateRequest(ctx context.Context, tplBytes []byte, fileName, outputFormat, description, idempotencyKey string) (*resty.Response, error) {
	req := c.client.R().
		SetContext(ctx).
		SetFileReader("template", fileName, bytes.NewReader(tplBytes)).
		SetFormData(map[string]string{
			"outputFormat": outputFormat,
			"description":  description,
		})

	if idempotencyKey != "" {
		req.SetHeader(idempotencyHeader, idempotencyKey)
	}

	return req.Post("/v1/templates")
}

// GetTemplate retrieves a template by ID via GET /v1/templates/:id.
// Returns the HTTP status code, parsed response body, and any error.
func (c *ManagerClient) GetTemplate(ctx context.Context, id string) (int, map[string]any, error) {
	var result map[string]any

	resp, err := c.client.R().
		SetContext(ctx).
		SetResult(&result).
		Get("/v1/templates/" + id)
	if err != nil {
		return 0, nil, fmt.Errorf("GetTemplate: %w", err)
	}

	return resp.StatusCode(), result, nil
}

// GetTemplateRaw retrieves a template by ID and returns the raw *resty.Response.
func (c *ManagerClient) GetTemplateRaw(ctx context.Context, id string) (*resty.Response, error) {
	return c.client.R().
		SetContext(ctx).
		Get("/v1/templates/" + id)
}

// GetAllTemplates retrieves all templates via GET /v1/templates with optional query params.
// Returns the HTTP status code, paginated response, and any error.
func (c *ManagerClient) GetAllTemplates(ctx context.Context, params map[string]string) (int, PaginatedResponse, error) {
	var result PaginatedResponse

	resp, err := c.client.R().
		SetContext(ctx).
		SetQueryParams(params).
		SetResult(&result).
		Get("/v1/templates")
	if err != nil {
		return 0, PaginatedResponse{}, fmt.Errorf("GetAllTemplates: %w", err)
	}

	return resp.StatusCode(), result, nil
}

// GetAllTemplatesRaw retrieves all templates and returns the raw *resty.Response.
// Use this for testing error scenarios where you need access to the raw response body.
func (c *ManagerClient) GetAllTemplatesRaw(ctx context.Context, params map[string]string) (*resty.Response, error) {
	return c.client.R().
		SetContext(ctx).
		SetQueryParams(params).
		Get("/v1/templates")
}

// UpdateTemplate performs a partial update on a template via PATCH /v1/templates/:id.
// The body is sent as multipart form data (same as create: template file + outputFormat + description).
// Pass nil for tplBytes to skip the file upload.
func (c *ManagerClient) UpdateTemplate(ctx context.Context, id string, tplBytes []byte, fileName, outputFormat, description string) (int, map[string]any, error) {
	req := c.client.R().
		SetContext(ctx).
		SetFormData(map[string]string{
			"outputFormat": outputFormat,
			"description":  description,
		})

	if tplBytes != nil {
		req.SetFileReader("template", fileName, bytes.NewReader(tplBytes))
	}

	resp, err := req.Patch("/v1/templates/" + id)
	if err != nil {
		return 0, nil, fmt.Errorf("UpdateTemplate: %w", err)
	}

	var result map[string]any
	if err := json.Unmarshal(resp.Body(), &result); err != nil {
		return resp.StatusCode(), nil, fmt.Errorf("UpdateTemplate: unmarshal: %w", err)
	}

	return resp.StatusCode(), result, nil
}

// DeleteTemplate removes a template by ID via DELETE /v1/templates/:id.
// Returns the HTTP status code and any error.
func (c *ManagerClient) DeleteTemplate(ctx context.Context, id string) (int, error) {
	resp, err := c.client.R().
		SetContext(ctx).
		Delete("/v1/templates/" + id)
	if err != nil {
		return 0, fmt.Errorf("DeleteTemplate: %w", err)
	}

	return resp.StatusCode(), nil
}

// DeleteTemplateRaw removes a template and returns the raw *resty.Response.
func (c *ManagerClient) DeleteTemplateRaw(ctx context.Context, id string) (*resty.Response, error) {
	return c.client.R().
		SetContext(ctx).
		Delete("/v1/templates/" + id)
}

// ############################################################################
// Report Methods
// ############################################################################

// CreateReport creates a new report via POST /v1/reports.
// Returns the HTTP status code, parsed response body, and any error.
// On error responses (4xx/5xx), the error body is returned instead of the success body.
func (c *ManagerClient) CreateReport(ctx context.Context, req CreateReportRequest) (int, map[string]any, error) {
	var result map[string]any
	var errResult map[string]any

	resp, err := c.client.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(req).
		SetResult(&result).
		SetError(&errResult).
		Post("/v1/reports")
	if err != nil {
		return 0, nil, fmt.Errorf("CreateReport: %w", err)
	}

	if resp.IsError() && errResult != nil {
		return resp.StatusCode(), errResult, nil
	}

	return resp.StatusCode(), result, nil
}

// CreateReportRaw creates a report and returns the raw *resty.Response.
// Use this for testing error scenarios with arbitrary body payloads.
func (c *ManagerClient) CreateReportRaw(ctx context.Context, body any) (*resty.Response, error) {
	return c.client.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(body).
		Post("/v1/reports")
}

// CreateReportWithIdempotency creates a report with an idempotency key.
// Returns the HTTP status code, parsed response body, and any error.
func (c *ManagerClient) CreateReportWithIdempotency(ctx context.Context, req CreateReportRequest, key string) (int, map[string]any, error) {
	var result map[string]any
	var errResult map[string]any

	resp, err := c.client.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetHeader(idempotencyHeader, key).
		SetBody(req).
		SetResult(&result).
		SetError(&errResult).
		Post("/v1/reports")
	if err != nil {
		return 0, nil, fmt.Errorf("CreateReportWithIdempotency: %w", err)
	}

	if resp.IsError() && errResult != nil {
		return resp.StatusCode(), errResult, nil
	}

	return resp.StatusCode(), result, nil
}

// GetReport retrieves a report by ID via GET /v1/reports/:id.
// Returns the HTTP status code, parsed report response, and any error.
func (c *ManagerClient) GetReport(ctx context.Context, id string) (int, ReportResponse, error) {
	var result ReportResponse

	resp, err := c.client.R().
		SetContext(ctx).
		SetResult(&result).
		Get("/v1/reports/" + id)
	if err != nil {
		return 0, ReportResponse{}, fmt.Errorf("GetReport: %w", err)
	}

	return resp.StatusCode(), result, nil
}

// GetReportRaw retrieves a report and returns the raw *resty.Response.
func (c *ManagerClient) GetReportRaw(ctx context.Context, id string) (*resty.Response, error) {
	return c.client.R().
		SetContext(ctx).
		Get("/v1/reports/" + id)
}

// GetAllReports retrieves all reports via GET /v1/reports with optional query params.
// Returns the HTTP status code, paginated response, and any error.
func (c *ManagerClient) GetAllReports(ctx context.Context, params map[string]string) (int, PaginatedResponse, error) {
	var result PaginatedResponse

	resp, err := c.client.R().
		SetContext(ctx).
		SetQueryParams(params).
		SetResult(&result).
		Get("/v1/reports")
	if err != nil {
		return 0, PaginatedResponse{}, fmt.Errorf("GetAllReports: %w", err)
	}

	return resp.StatusCode(), result, nil
}

// GetAllReportsRaw retrieves all reports and returns the raw *resty.Response.
// Use this for testing error scenarios where you need access to the raw response body.
func (c *ManagerClient) GetAllReportsRaw(ctx context.Context, params map[string]string) (*resty.Response, error) {
	return c.client.R().
		SetContext(ctx).
		SetQueryParams(params).
		Get("/v1/reports")
}

// DownloadReport downloads a report file via GET /v1/reports/:id/download.
// Returns the HTTP status code, raw file bytes, response headers, and any error.
func (c *ManagerClient) DownloadReport(ctx context.Context, id string) (int, []byte, http.Header, error) {
	resp, err := c.client.R().
		SetContext(ctx).
		Get("/v1/reports/" + id + "/download")
	if err != nil {
		return 0, nil, nil, fmt.Errorf("DownloadReport: %w", err)
	}

	headers := http.Header{}
	for k, v := range resp.Header() {
		headers[k] = v
	}

	return resp.StatusCode(), resp.Body(), headers, nil
}

// ############################################################################
// Deadline Methods
// ############################################################################

// CreateDeadline creates a new deadline via POST /v1/deadlines.
// Returns the HTTP status code, parsed response body, and any error.
func (c *ManagerClient) CreateDeadline(ctx context.Context, body any) (int, map[string]any, error) {
	var result map[string]any
	var errResult map[string]any

	resp, err := c.client.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(body).
		SetResult(&result).
		SetError(&errResult).
		Post("/v1/deadlines")
	if err != nil {
		return 0, nil, fmt.Errorf("CreateDeadline: %w", err)
	}

	if resp.IsError() && errResult != nil {
		return resp.StatusCode(), errResult, nil
	}

	return resp.StatusCode(), result, nil
}

// CreateDeadlineRaw creates a deadline and returns the raw *resty.Response.
func (c *ManagerClient) CreateDeadlineRaw(ctx context.Context, body any) (*resty.Response, error) {
	return c.client.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(body).
		Post("/v1/deadlines")
}

// GetAllDeadlines retrieves all deadlines via GET /v1/deadlines with optional query params.
// Returns the HTTP status code, paginated response, and any error.
func (c *ManagerClient) GetAllDeadlines(ctx context.Context, params map[string]string) (int, PaginatedResponse, error) {
	var result PaginatedResponse

	resp, err := c.client.R().
		SetContext(ctx).
		SetQueryParams(params).
		SetResult(&result).
		Get("/v1/deadlines")
	if err != nil {
		return 0, PaginatedResponse{}, fmt.Errorf("GetAllDeadlines: %w", err)
	}

	return resp.StatusCode(), result, nil
}

// GetAllDeadlinesRaw retrieves all deadlines and returns the raw *resty.Response.
func (c *ManagerClient) GetAllDeadlinesRaw(ctx context.Context, params map[string]string) (*resty.Response, error) {
	return c.client.R().
		SetContext(ctx).
		SetQueryParams(params).
		Get("/v1/deadlines")
}

// UpdateDeadline updates a deadline via PATCH /v1/deadlines/:id.
// Returns the HTTP status code, parsed response body, and any error.
func (c *ManagerClient) UpdateDeadline(ctx context.Context, id string, body any) (int, map[string]any, error) {
	var result map[string]any
	var errResult map[string]any

	resp, err := c.client.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(body).
		SetResult(&result).
		SetError(&errResult).
		Patch("/v1/deadlines/" + id)
	if err != nil {
		return 0, nil, fmt.Errorf("UpdateDeadline: %w", err)
	}

	if resp.IsError() && errResult != nil {
		return resp.StatusCode(), errResult, nil
	}

	return resp.StatusCode(), result, nil
}

// UpdateDeadlineRaw updates a deadline and returns the raw *resty.Response.
func (c *ManagerClient) UpdateDeadlineRaw(ctx context.Context, id string, body any) (*resty.Response, error) {
	return c.client.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(body).
		Patch("/v1/deadlines/" + id)
}

// DeleteDeadline removes a deadline via DELETE /v1/deadlines/:id.
// Returns the HTTP status code and any error.
func (c *ManagerClient) DeleteDeadline(ctx context.Context, id string) (int, error) {
	resp, err := c.client.R().
		SetContext(ctx).
		Delete("/v1/deadlines/" + id)
	if err != nil {
		return 0, fmt.Errorf("DeleteDeadline: %w", err)
	}

	return resp.StatusCode(), nil
}

// DeliverDeadline marks (or clears) a deadline delivery via PATCH /v1/deadlines/:id/deliver.
// Returns the HTTP status code, parsed response body, and any error.
func (c *ManagerClient) DeliverDeadline(ctx context.Context, id string, delivered bool) (int, map[string]any, error) {
	var result map[string]any
	var errResult map[string]any

	resp, err := c.client.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(map[string]bool{"delivered": delivered}).
		SetResult(&result).
		SetError(&errResult).
		Patch("/v1/deadlines/" + id + "/deliver")
	if err != nil {
		return 0, nil, fmt.Errorf("DeliverDeadline: %w", err)
	}

	if resp.IsError() && errResult != nil {
		return resp.StatusCode(), errResult, nil
	}

	return resp.StatusCode(), result, nil
}

// DeliverDeadlineRaw marks a deadline as delivered and returns the raw *resty.Response.
func (c *ManagerClient) DeliverDeadlineRaw(ctx context.Context, id string, body any) (*resty.Response, error) {
	return c.client.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(body).
		Patch("/v1/deadlines/" + id + "/deliver")
}

// ############################################################################
// Template Builder Methods
// ############################################################################

// GetBlocksConfig retrieves block definitions via GET /v1/templates/blocks-config.
// Returns the HTTP status code, parsed response body, and any error.
func (c *ManagerClient) GetBlocksConfig(ctx context.Context) (int, map[string]any, error) {
	var result map[string]any

	resp, err := c.client.R().
		SetContext(ctx).
		SetResult(&result).
		Get("/v1/templates/blocks-config")
	if err != nil {
		return 0, nil, fmt.Errorf("GetBlocksConfig: %w", err)
	}

	return resp.StatusCode(), result, nil
}

// GetFiltersConfig retrieves filter definitions via GET /v1/templates/filters.
// Returns the HTTP status code, parsed response body, and any error.
func (c *ManagerClient) GetFiltersConfig(ctx context.Context) (int, map[string]any, error) {
	var result map[string]any

	resp, err := c.client.R().
		SetContext(ctx).
		SetResult(&result).
		Get("/v1/templates/filters")
	if err != nil {
		return 0, nil, fmt.Errorf("GetFiltersConfig: %w", err)
	}

	return resp.StatusCode(), result, nil
}

// ValidateBlocks validates template blocks via POST /v1/templates/validate.
// Returns the HTTP status code, parsed response body, and any error.
func (c *ManagerClient) ValidateBlocks(ctx context.Context, body any) (int, map[string]any, error) {
	var result map[string]any

	resp, err := c.client.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(body).
		SetResult(&result).
		Post("/v1/templates/validate")
	if err != nil {
		return 0, nil, fmt.Errorf("ValidateBlocks: %w", err)
	}

	return resp.StatusCode(), result, nil
}

// ValidateBlocksRaw validates template blocks and returns the raw *resty.Response.
func (c *ManagerClient) ValidateBlocksRaw(ctx context.Context, body any) (*resty.Response, error) {
	return c.client.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(body).
		Post("/v1/templates/validate")
}

// GenerateCode generates template code via POST /v1/templates/generate-code.
// Returns the HTTP status code, parsed response body, and any error.
func (c *ManagerClient) GenerateCode(ctx context.Context, body any) (int, map[string]any, error) {
	var result map[string]any

	resp, err := c.client.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(body).
		SetResult(&result).
		Post("/v1/templates/generate-code")
	if err != nil {
		return 0, nil, fmt.Errorf("GenerateCode: %w", err)
	}

	return resp.StatusCode(), result, nil
}

// GenerateCodeRaw generates template code and returns the raw *resty.Response.
func (c *ManagerClient) GenerateCodeRaw(ctx context.Context, body any) (*resty.Response, error) {
	return c.client.R().
		SetContext(ctx).
		SetHeader("Content-Type", "application/json").
		SetBody(body).
		Post("/v1/templates/generate-code")
}

// ############################################################################
// DataSource Methods
// ############################################################################

// GetDataSources retrieves all data sources via GET /v1/data-sources.
// Returns the HTTP status code, list of data sources, and any error.
func (c *ManagerClient) GetDataSources(ctx context.Context) (int, []DataSourceInfo, error) {
	var result []DataSourceInfo

	resp, err := c.client.R().
		SetContext(ctx).
		SetResult(&result).
		Get("/v1/data-sources")
	if err != nil {
		return 0, nil, fmt.Errorf("GetDataSources: %w", err)
	}

	return resp.StatusCode(), result, nil
}

// GetDataSourceByID retrieves a data source by ID via GET /v1/data-sources/:dataSourceId.
// Returns the HTTP status code, data source details, and any error.
func (c *ManagerClient) GetDataSourceByID(ctx context.Context, id string) (int, DataSourceDetails, error) {
	var result DataSourceDetails

	resp, err := c.client.R().
		SetContext(ctx).
		SetResult(&result).
		Get("/v1/data-sources/" + id)
	if err != nil {
		return 0, DataSourceDetails{}, fmt.Errorf("GetDataSourceByID: %w", err)
	}

	return resp.StatusCode(), result, nil
}

// GetDataSourceByIDRaw retrieves a data source and returns the raw *resty.Response.
func (c *ManagerClient) GetDataSourceByIDRaw(ctx context.Context, id string) (*resty.Response, error) {
	return c.client.R().
		SetContext(ctx).
		Get("/v1/data-sources/" + id)
}

// ############################################################################
// Metrics Methods
// ############################################################################

// GetMetricsRaw retrieves metrics via GET /v1/metrics and returns the raw *resty.Response.
func (c *ManagerClient) GetMetricsRaw(ctx context.Context) (*resty.Response, error) {
	return c.client.R().
		SetContext(ctx).
		Get("/v1/metrics")
}

// ############################################################################
// Health Methods
// ############################################################################

// Health performs a health check via GET /health.
// Returns the HTTP status code and any error.
func (c *ManagerClient) Health(ctx context.Context) (int, error) {
	resp, err := c.client.R().
		SetContext(ctx).
		Get("/health")
	if err != nil {
		return 0, fmt.Errorf("Health: %w", err)
	}

	return resp.StatusCode(), nil
}

// Ready performs a readiness check via GET /readyz.
// Returns the HTTP status code, readiness response, and any error.
func (c *ManagerClient) Ready(ctx context.Context) (int, ReadinessResponse, error) {
	var result ReadinessResponse

	resp, err := c.client.R().
		SetContext(ctx).
		SetResult(&result).
		Get("/readyz")
	if err != nil {
		return 0, ReadinessResponse{}, fmt.Errorf("Ready: %w", err)
	}

	return resp.StatusCode(), result, nil
}

// Version retrieves the service version via GET /version.
// Returns the HTTP status code, version info, and any error.
func (c *ManagerClient) Version(ctx context.Context) (int, map[string]any, error) {
	var result map[string]any

	resp, err := c.client.R().
		SetContext(ctx).
		SetResult(&result).
		Get("/version")
	if err != nil {
		return 0, nil, fmt.Errorf("Version: %w", err)
	}

	return resp.StatusCode(), result, nil
}

// WaitHealthy polls the /health endpoint until the service returns 200 or the timeout is reached.
// Returns nil on success or an error describing the last failure.
func (c *ManagerClient) WaitHealthy(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	pollInterval := 500 * time.Millisecond

	var lastErr error

	for time.Now().Before(deadline) {
		status, err := c.Health(ctx)
		if err == nil && status == http.StatusOK {
			return nil
		}

		if err != nil {
			lastErr = err
		} else {
			lastErr = fmt.Errorf("health returned status %d", status)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
			// Continue polling.
		}
	}

	return fmt.Errorf("service did not become healthy within %v: %w", timeout, lastErr)
}

// ############################################################################
// Worker Health
// ############################################################################

// WorkerHealth performs a health check on the worker service at the given base URL.
// Returns the HTTP status code and any error.
func WorkerHealth(ctx context.Context, workerBaseURL string) (int, error) {
	client := resty.New().
		SetBaseURL(workerBaseURL).
		SetTimeout(HTTPClientTimeout)

	resp, err := client.R().
		SetContext(ctx).
		Get("/health")
	if err != nil {
		return 0, fmt.Errorf("WorkerHealth: %w", err)
	}

	return resp.StatusCode(), nil
}
