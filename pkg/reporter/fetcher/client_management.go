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
	neturl "net/url"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/ctxutil"

	"github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"go.opentelemetry.io/otel/attribute"
)

// ListConnections retrieves all registered connections from the Fetcher
// management API (GET /v1/management/connections).
func (c *FetcherClient) ListConnections(ctx context.Context) ([]ConnectionResponse, error) {
	logger := ctxutil.NewLoggerFromContext(ctx)
	tracer := ctxutil.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "fetcher.client.list_connections")
	defer span.End()

	ctx, cancel := context.WithTimeout(ctx, fetcherManagementTimeout)
	defer cancel()

	url := fmt.Sprintf("%s/v1/management/connections", c.baseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to create request for list connections", err)
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// doWithAuthRetry applies M2M auth, executes the request, and re-attempts
	// exactly once on a 401 (with a fresh token after credential invalidation).
	// On non-401 errors the original response/error flows through unchanged.
	resp, err := c.doWithAuthRetry(ctx, req)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to execute request for list connections", err)
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// doWithAuthRetry already invalidated credentials and retried on 401;
		// at this point the status is either non-401 or the second attempt's
		// final status. Either way, propagate as a transport-style error.
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorResponseSize))
		err := fmt.Errorf("list connections failed with status %d: %s", resp.StatusCode, string(body))
		libOpentelemetry.HandleSpanError(span, "List connections returned non-OK status", err)

		return nil, err
	}

	var result ConnectionListResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to decode list connections response", err)
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	span.SetAttributes(attribute.Int("fetcher.connections.count", len(result.Connections)))
	logger.Log(ctx, log.LevelInfo, "Listed connections from Fetcher API",
		log.Int("count", len(result.Connections)))

	return result.Connections, nil
}

// GetConnectionSchema retrieves the schema for a specific connection from the
// Fetcher management API (GET /v1/management/connections/{id}/schema).
func (c *FetcherClient) GetConnectionSchema(ctx context.Context, connectionID string) (*ConnectionSchemaResponse, error) {
	logger := ctxutil.NewLoggerFromContext(ctx)
	tracer := ctxutil.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "fetcher.client.get_connection_schema")
	defer span.End()

	span.SetAttributes(attribute.String("fetcher.connection.id", connectionID))

	ctx, cancel := context.WithTimeout(ctx, fetcherManagementTimeout)
	defer cancel()

	url := fmt.Sprintf("%s/v1/management/connections/%s/schema", c.baseURL, neturl.PathEscape(connectionID))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to create request for get connection schema", err)
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// doWithAuthRetry handles M2M auth application + 401 re-attempt internally.
	resp, err := c.doWithAuthRetry(ctx, req)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to execute request for get connection schema", err)
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// On 401, doWithAuthRetry already invalidated and retried; do not
		// invalidate a second time here.
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorResponseSize))
		err := fmt.Errorf("get connection schema failed with status %d: %s", resp.StatusCode, string(body))
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Get connection schema returned non-OK status", err)

		return nil, err
	}

	var result ConnectionSchemaResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to decode get connection schema response", err)
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	logger.Log(ctx, log.LevelInfo, "Retrieved schema for connection",
		log.String("connection_id", connectionID),
		log.Int("table_count", len(result.Tables)))

	return &result, nil
}

// ValidateSchema validates requested fields against connection schemas via
// the Fetcher management API (POST /v1/management/connections/validate-schema).
func (c *FetcherClient) ValidateSchema(ctx context.Context, mappedFields map[string]map[string][]string) (*ValidateSchemaResponse, error) {
	logger := ctxutil.NewLoggerFromContext(ctx)
	tracer := ctxutil.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "fetcher.client.validate_schema")
	defer span.End()

	span.SetAttributes(
		attribute.Int("fetcher.datasource.count", len(mappedFields)),
	)

	ctx, cancel := context.WithTimeout(ctx, fetcherManagementTimeout)
	defer cancel()

	reqBody := ValidateSchemaRequest{
		MappedFields: mappedFields,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to marshal validate schema request", err)
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	url := fmt.Sprintf("%s/v1/management/connections/validate-schema", c.baseURL)

	// bytes.NewReader is used so http.NewRequestWithContext configures
	// req.GetBody automatically. doWithAuthRetry relies on GetBody to
	// re-stream the body on the second attempt.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to create request for validate schema", err)
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// doWithAuthRetry handles M2M auth application + 401 re-attempt internally.
	resp, err := c.doWithAuthRetry(ctx, req)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to execute request for validate schema", err)
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// On 401, doWithAuthRetry already invalidated and retried; do not
		// invalidate a second time here. Other 4xx/5xx flow through the
		// existing structured-error parsing below.
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorResponseSize))

		// Fetcher returns 422 when schema validation finds inconsistencies.
		// This is a successful validation with a failure result, not a transport error.
		// Parse it as a ValidateSchemaResponse so callers can process structured errors
		// through the normal validation path (translateValidationResult).
		if resp.StatusCode == http.StatusUnprocessableEntity {
			var errResp struct {
				Message string                  `json:"message"`
				Errors  []SchemaValidationError `json:"errors"`
			}

			if json.Unmarshal(body, &errResp) == nil && len(errResp.Errors) > 0 {
				libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Schema validation found inconsistencies",
					fmt.Errorf("validation failure: %s", errResp.Message))

				return &ValidateSchemaResponse{
					Status:  "failure",
					Message: errResp.Message,
					Errors:  errResp.Errors,
				}, nil
			}
		}

		// For specific fetcher error codes, map to the appropriate domain type.
		// Only FET-1062 (no connections found) maps to DATA_SOURCE_NOT_FOUND;
		// other 4xx errors (auth, rate-limit, bad request) are returned as-is.
		if resp.StatusCode >= http.StatusBadRequest && resp.StatusCode < http.StatusInternalServerError {
			var errResp struct {
				Code    string `json:"code"`
				Message string `json:"message"`
				Title   string `json:"title"`
			}

			if json.Unmarshal(body, &errResp) == nil && errResp.Code == "FET-1062" {
				libOpentelemetry.HandleSpanBusinessErrorEvent(span, "No connections found for datasources",
					fmt.Errorf("%s: %s", errResp.Code, errResp.Message))

				return &ValidateSchemaResponse{
					Status:  "failure",
					Message: errResp.Message,
					Errors: []SchemaValidationError{{
						Type:    "DATA_SOURCE_NOT_FOUND",
						Message: errResp.Message,
					}},
				}, nil
			}
		}

		err := fmt.Errorf("validate schema failed with status %d: %s", resp.StatusCode, string(body))
		libOpentelemetry.HandleSpanError(span, "Validate schema returned non-OK status", err)

		return nil, err
	}

	var result ValidateSchemaResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to decode validate schema response", err)
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	logger.Log(ctx, log.LevelInfo, "Validated schema via Fetcher API",
		log.Int("datasource_count", len(mappedFields)),
		log.String("status", result.Status),
		log.Int("error_count", len(result.Errors)))

	return &result, nil
}
