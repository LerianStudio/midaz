// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package fetcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"

	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/ctxutil"

	"github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"go.opentelemetry.io/otel/attribute"
)

// countExtractionFilters returns the total number of filter conditions across
// the nested (datasource → table → field) tree. Used for log metadata only —
// it tells operators the SHAPE of the request without leaking filter values
// (which can carry CPF/CNPJ/account IDs in fintech payloads).
func countExtractionFilters(filters map[string]map[string]map[string]FilterCondition) int {
	total := 0

	for _, byDatasource := range filters {
		for _, byTable := range byDatasource {
			total += len(byTable)
		}
	}

	return total
}

// countExtractionMappedFields returns the total field count flattened across
// the (datasource → table → []field) tree. Reveals payload size at a glance
// without leaking individual field names.
func countExtractionMappedFields(mapped map[string]map[string][]string) int {
	total := 0

	for _, byDatasource := range mapped {
		for _, fields := range byDatasource {
			total += len(fields)
		}
	}

	return total
}

// sortedMetadataKeys returns the keys of the extraction-request metadata map
// in deterministic order. Keys are well-known and operationally meaningful
// (source, reportId, templateId) — values are not logged because they can
// carry tenant- or report-scoped identifiers we treat as restricted-scope.
func sortedMetadataKeys(metadata map[string]any) []string {
	if len(metadata) == 0 {
		return nil
	}

	keys := make([]string, 0, len(metadata))
	for k := range metadata {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	return keys
}

// CreateExtractionJob creates a new extraction job in the Fetcher service
// (POST /v1/fetcher). The caller provides the data source, report, and template
// identifiers along with optional field filters. Returns the job metadata
// including the assigned JobID for status polling.
func (c *FetcherClient) CreateExtractionJob(ctx context.Context, jobReq CreateExtractionJobRequest) (*ExtractionJobResponse, error) {
	logger := ctxutil.NewLoggerFromContext(ctx)
	tracer := ctxutil.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "fetcher.client.create_extraction_job")
	defer span.End()

	span.SetAttributes(
		attribute.Int("fetcher.datasource.count", len(jobReq.DataRequest.MappedFields)),
	)

	ctx, cancel := context.WithTimeout(ctx, fetcherExtractionTimeout)
	defer cancel()

	bodyBytes, err := json.Marshal(jobReq)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to marshal extraction job request", err)
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	// M2 (security-reviewer): emit structural metadata, never the body.
	// The verbatim payload contains DataRequest.Filters values, which in
	// fintech can be CPF/CNPJ/account IDs — DEBUG is off in production today
	// but a temporary LOG_LEVEL=debug during incident triage must not turn
	// into a PCI/LGPD exposure (CWE-532, OWASP A09). Operators get the SHAPE
	// of the request (count of datasources, fields, filters, metadata keys,
	// serialized size) which is what diagnostics actually need.
	logger.Log(ctx, log.LevelDebug, "Fetcher extraction request prepared",
		log.Int("datasource_count", len(jobReq.DataRequest.MappedFields)),
		log.Int("mapped_field_count", countExtractionMappedFields(jobReq.DataRequest.MappedFields)),
		log.Int("filter_count", countExtractionFilters(jobReq.DataRequest.Filters)),
		log.Any("metadata_keys", sortedMetadataKeys(jobReq.Metadata)),
		log.Int("body_size_bytes", len(bodyBytes)),
	)

	reqURL := fmt.Sprintf("%s/v1/fetcher", c.baseURL)

	// bytes.NewReader is used so http.NewRequestWithContext configures
	// req.GetBody automatically. doWithAuthRetry relies on GetBody to
	// re-stream the body on the second attempt.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(bodyBytes))
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to create request for extraction job", err)
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// doWithAuthRetry handles M2M auth application + 401 re-attempt internally.
	resp, err := c.doWithAuthRetry(ctx, req)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to execute request for extraction job", err)
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		// On 401, doWithAuthRetry already invalidated and retried; do not
		// invalidate a second time here.
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorResponseSize))
		err := fmt.Errorf("create extraction job failed with status %d: %s", resp.StatusCode, string(body))
		libOpentelemetry.HandleSpanError(span, "Create extraction job returned unexpected status", err)

		return nil, err
	}

	var result ExtractionJobResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to decode extraction job response", err)
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	logger.Log(ctx, log.LevelInfo, "Created extraction job",
		log.String("job_id", result.JobID))

	return &result, nil
}

// GetExtractionJobStatus retrieves the current status of an extraction job
// from the Fetcher service (GET /v1/fetcher/{id}). This is used for fallback
// polling when webhook notifications are not available.
func (c *FetcherClient) GetExtractionJobStatus(ctx context.Context, jobID string) (*ExtractionJobResponse, error) {
	logger := ctxutil.NewLoggerFromContext(ctx)
	tracer := ctxutil.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "fetcher.client.get_extraction_job_status")
	defer span.End()

	span.SetAttributes(attribute.String("fetcher.job.id", jobID))

	ctx, cancel := context.WithTimeout(ctx, fetcherStatusTimeout)
	defer cancel()

	reqURL := fmt.Sprintf("%s/v1/fetcher/%s", c.baseURL, url.PathEscape(jobID))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to create request for job status", err)
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// doWithAuthRetry handles M2M auth application + 401 re-attempt internally.
	resp, err := c.doWithAuthRetry(ctx, req)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to execute request for job status", err)
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// On 401, doWithAuthRetry already invalidated and retried; do not
		// invalidate a second time here.
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorResponseSize))
		err := fmt.Errorf("get extraction job status failed with status %d: %s", resp.StatusCode, string(body))
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Get extraction job status returned non-OK status", err)

		return nil, err
	}

	var result ExtractionJobResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to decode job status response", err)
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	logger.Log(ctx, log.LevelInfo, "Retrieved extraction job status",
		log.String("job_id", jobID),
		log.String("status", result.Status))

	return &result, nil
}
