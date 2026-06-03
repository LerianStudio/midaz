// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package fetcher

import (
	"encoding/json"
	"time"
)

// ConnectionResponse represents a single connection (datasource) returned by
// the Fetcher management API (GET /v1/management/connections).
type ConnectionResponse struct {
	// ID is the unique identifier for the connection in Fetcher.
	ID string `json:"id"`

	// ConfigName is the datasource config name (e.g., "midaz_onboarding", "plugin_crm").
	ConfigName string `json:"configName"`

	// Type identifies the database engine (e.g., "postgresql", "mongodb").
	Type string `json:"type"`

	// DatabaseName is the target database name.
	DatabaseName string `json:"databaseName"`
}

// ConnectionListResponse wraps the paginated list of connections returned by
// GET /v1/management/connections. The fetcher returns a Pagination envelope
// with items in the "items" field.
type ConnectionListResponse struct {
	Connections []ConnectionResponse `json:"items"`
}

// ConnectionSchemaResponse wraps the schema information returned by
// GET /v1/management/connections/{id}/schema.
type ConnectionSchemaResponse struct {
	// ID is the connection identifier (fetcher uses "id", not "dataSourceId").
	ID string `json:"id"`

	// ConfigName is the datasource config name.
	ConfigName string `json:"configName"`

	// DatabaseName is the target database name.
	DatabaseName string `json:"databaseName"`

	// Type identifies the database engine.
	Type string `json:"type"`

	// Tables contains the list of tables (or collections) available.
	Tables []SchemaTableResponse `json:"tables"`
}

// SchemaTableResponse represents a single table in the Fetcher schema response.
type SchemaTableResponse struct {
	// Name is the table or collection name.
	Name string `json:"name"`

	// Fields contains the columns or document fields.
	// The fetcher returns fields as []string for all database types.
	Fields []SchemaFieldResponse `json:"fields"`
}

// SchemaFieldResponse represents a single field within a SchemaTableResponse.
type SchemaFieldResponse struct {
	// Name is the field or column name.
	Name string `json:"name"`

	// Type is the data type (e.g., "uuid", "varchar", "int").
	Type string `json:"type"`
}

// UnmarshalJSON handles fields as either []string (fetcher format) or
// []SchemaFieldResponse (structured format).
func (t *SchemaTableResponse) UnmarshalJSON(data []byte) error {
	// Try structured format first ([]object with name/type)
	type Alias SchemaTableResponse

	var structured struct {
		Alias
	}

	if err := json.Unmarshal(data, &structured); err == nil && len(structured.Fields) > 0 {
		*t = SchemaTableResponse(structured.Alias)
		return nil
	}

	// Fallback: fields as []string (fetcher returns field names only)
	var raw struct {
		Name   string   `json:"name"`
		Fields []string `json:"fields"`
	}

	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	t.Name = raw.Name
	t.Fields = make([]SchemaFieldResponse, len(raw.Fields))

	for i, f := range raw.Fields {
		t.Fields[i] = SchemaFieldResponse{Name: f}
	}

	return nil
}

// ValidateSchemaRequest is the request body for
// POST /v1/management/connections/validate-schema.
// Matches the Fetcher API's SchemaValidationRequest contract.
type ValidateSchemaRequest struct {
	// MappedFields maps datasource config names to their tables and fields.
	MappedFields map[string]map[string][]string `json:"mappedFields"`
}

// ValidateSchemaResponse wraps the validation result from the Fetcher API.
// Matches the Fetcher's SchemaValidationResponse contract.
type ValidateSchemaResponse struct {
	// Status is "success" or "failure".
	Status string `json:"status"`

	// Message is a human-readable description of the result.
	Message string `json:"message"`

	// Errors contains validation failures (only present on failure).
	Errors []SchemaValidationError `json:"errors,omitempty"`
}

// SchemaValidationError represents a single validation error from the Fetcher API.
type SchemaValidationError struct {
	Type         string `json:"type"`
	DataSourceID string `json:"dataSourceId,omitempty"`
	Table        string `json:"table,omitempty"`
	Field        string `json:"field,omitempty"`
	Message      string `json:"message,omitempty"`
}

// IsSuccess returns true if the validation passed.
func (r *ValidateSchemaResponse) IsSuccess() bool {
	return r.Status == "success"
}

// CreateExtractionJobRequest is the request body for
// POST /v1/fetcher (create extraction job).
// Matches the Fetcher's FetcherRequest contract with nested DataRequest.
type CreateExtractionJobRequest struct {
	// DataRequest encapsulates field mappings and optional filters for extraction.
	DataRequest ExtractionDataRequest `json:"dataRequest"`

	// Metadata contains additional metadata for the extraction job.
	// Must include "source" to identify the requesting service (used by Fetcher
	// to build the RabbitMQ routing key: job.{status}.{source}).
	// Also include "reportId" and "templateId" for traceability.
	Metadata map[string]any `json:"metadata,omitempty"`
}

// ExtractionDataRequest encapsulates field mappings and optional filters
// for the Fetcher extraction API. Matches Fetcher's DataRequest struct.
type ExtractionDataRequest struct {
	// MappedFields maps datasource config names to their tables and fields.
	// Key: configName (e.g., "midaz_onboarding")
	// Value: map of table names to field names
	MappedFields map[string]map[string][]string `json:"mappedFields"`

	// Filters contains nested filtering criteria matching Fetcher's NestedFilters type:
	// map[datasource]map[table]map[field]FilterCondition
	Filters map[string]map[string]map[string]FilterCondition `json:"filters,omitempty"`
}

// FilterCondition matches the Fetcher's job.FilterCondition for extraction filters.
// JSON tags MUST match the Fetcher's pkg/model/job/job_queue.go tags exactly.
type FilterCondition struct {
	Equals         []any `json:"eq,omitempty"`
	GreaterThan    []any `json:"gt,omitempty"`
	GreaterOrEqual []any `json:"gte,omitempty"`
	LessThan       []any `json:"lt,omitempty"`
	LessOrEqual    []any `json:"lte,omitempty"`
	Between        []any `json:"between,omitempty"`
	In             []any `json:"in,omitempty"`
	NotIn          []any `json:"nin,omitempty"`
	NotEquals      []any `json:"ne,omitempty"`
	Like           []any `json:"like,omitempty"`
}

// ExtractionJobResponse represents the response from
// POST /v1/fetcher or GET /v1/fetcher/{id}.
type ExtractionJobResponse struct {
	// JobID is the unique identifier assigned by the Fetcher service.
	JobID string `json:"jobId"`

	// Status tracks the extraction job lifecycle.
	Status string `json:"status"`

	// CreatedAt is the timestamp when the extraction job was created.
	CreatedAt time.Time `json:"createdAt"`

	// CompletedAt is the timestamp when the job finished (nil if still running).
	CompletedAt *time.Time `json:"completedAt,omitempty"`

	// Result contains extraction result data (path, size, hmac, etc.).
	// Populated when status is "completed" on GET /v1/fetcher/{id}.
	Result *FetcherResultData `json:"result,omitempty"`

	// Error contains error details if the job failed.
	Error string `json:"error,omitempty"`
}

// ErrorResponse represents an error response from the Fetcher API.
type ErrorResponse struct {
	// Code is a machine-readable error code.
	Code string `json:"code"`

	// Message is a human-readable error description.
	Message string `json:"message"`
}

// FetcherNotification represents a job event notification published by the
// Fetcher service via RabbitMQ (exchange: fetcher.job.events, topic type).
// This struct mirrors the Fetcher's JobNotificationMessage exactly.
// Consumer 2 receives these messages on the reporter.fetcher.job.events queue
// (bound with routing keys job.completed.reporter / job.failed.reporter).
type FetcherNotification struct {
	// JobID is the extraction job identifier assigned by the Fetcher service.
	JobID string `json:"jobId"`

	// Status is the final status of the extraction job ("completed" or "failed").
	Status string `json:"status"`

	// Metadata contains additional metadata for the notification.
	// Includes "source" (requesting service) and optionally "error" (map with details).
	Metadata map[string]any `json:"metadata,omitempty"`

	// Result contains extraction result data (path, size, rowCount, format, hmac).
	// Only populated when Status is "completed".
	Result *FetcherResultData `json:"result,omitempty"`

	// ExecutionTimeMs is the total execution time in milliseconds.
	ExecutionTimeMs int64 `json:"executionTimeMs,omitempty"`

	// CompletedAt is the timestamp when the job completed.
	CompletedAt *time.Time `json:"completedAt,omitempty"`
}

// FetcherResultData contains information about the extraction result,
// mirroring the Fetcher's JobResultData struct.
type FetcherResultData struct {
	// Path is the storage path where result data is stored.
	Path string `json:"path,omitempty"`

	// SizeBytes is the size of the result data in bytes (before encryption).
	SizeBytes int64 `json:"sizeBytes,omitempty"`

	// RowCount is the total number of records extracted across all tables.
	RowCount int64 `json:"rowCount,omitempty"`

	// Format is the output format (e.g., "json").
	Format string `json:"format,omitempty"`

	// HMAC is the HMAC-SHA256 signature of the result data (before encryption).
	HMAC string `json:"hmac,omitempty"`
}

// GetDataPath returns the storage path for the extracted data.
// Returns empty string if result is nil.
func (n *FetcherNotification) GetDataPath() string {
	if n.Result != nil {
		return n.Result.Path
	}

	return ""
}

// GetHMAC returns the HMAC signature from the result data.
// Returns empty string if result is nil.
func (n *FetcherNotification) GetHMAC() string {
	if n.Result != nil {
		return n.Result.HMAC
	}

	return ""
}

// GetErrorMessage extracts the error message from metadata.
// Fetcher stores errors as metadata["error"]["message"] (map structure).
// Returns empty string if no error metadata exists.
func (n *FetcherNotification) GetErrorMessage() string {
	if n.Metadata == nil {
		return ""
	}

	errData, ok := n.Metadata["error"]
	if !ok {
		return ""
	}

	switch v := errData.(type) {
	case map[string]any:
		if msg, ok := v["message"].(string); ok {
			return msg
		}
	case string:
		return v
	}

	return ""
}
