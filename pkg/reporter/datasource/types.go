// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package datasource

import "time"

// DataSourceInfo provides summary metadata about a registered data source.
// Used by ListDataSources to enumerate available data sources without
// exposing connection details or schema internals.
type DataSourceInfo struct {
	// ID is the unique identifier for the data source (e.g., "midaz_onboarding").
	ID string `json:"id"`

	// Name is the human-readable display name for the data source.
	Name string `json:"name"`

	// Type identifies the database engine (e.g., "postgresql", "mongodb").
	Type string `json:"type"`

	// Status indicates current availability (e.g., "available", "unavailable").
	Status string `json:"status"`
}

// DataSourceSchema describes the full schema of a data source, including all
// tables and their fields. Used for template field validation and schema
// introspection by the Manager component.
type DataSourceSchema struct {
	// DataSourceID is the identifier of the data source this schema belongs to.
	DataSourceID string `json:"dataSourceId"`

	// Tables contains the list of tables (or collections) available in the data source.
	Tables []SchemaTable `json:"tables"`
}

// SchemaTable represents a single table within a data source schema.
// For PostgreSQL, this maps to a database table; for MongoDB, a collection.
type SchemaTable struct {
	// Name is the table or collection name.
	Name string `json:"name"`

	// Schema is the database schema containing this table (e.g., "public").
	// For MongoDB, this may be empty.
	Schema string `json:"schema"`

	// Fields contains the list of columns or document fields in this table.
	Fields []SchemaField `json:"fields"`
}

// SchemaField represents a single field (column) within a SchemaTable.
type SchemaField struct {
	// Name is the field or column name.
	Name string `json:"name"`

	// Type is the data type of the field (e.g., "uuid", "varchar", "int").
	Type string `json:"type"`
}

// ValidationResult contains the outcome of validating requested table/field
// references against a data source schema. Per D7 decision, unavailable data
// sources produce warnings (not errors) so template creation can proceed with
// partial validation.
type ValidationResult struct {
	// Valid indicates whether all requested tables and fields are present in the schema.
	Valid bool `json:"valid"`

	// Warnings contains non-fatal issues discovered during validation.
	// For example, a DATA_SOURCE_UNAVAILABLE warning when a data source
	// cannot be reached.
	Warnings []ValidationWarning `json:"warnings"`

	// MissingTables lists table/collection names that were not found in the schema.
	MissingTables []string `json:"missingTables,omitempty"`

	// MissingFields lists fields that were not found, grouped by table.
	MissingFields []MissingFieldDetail `json:"missingFields,omitempty"`

	// Ambiguous lists tables that exist in multiple schemas without a "public"
	// fallback, making the reference ambiguous (PostgreSQL only).
	Ambiguous []AmbiguousTable `json:"ambiguous,omitempty"`
}

// MissingFieldDetail describes fields that were not found in a specific table.
type MissingFieldDetail struct {
	// Table is the table name where fields were expected.
	Table string `json:"table"`

	// Fields lists the field names that were not found.
	Fields []string `json:"fields"`
}

// AmbiguousTable describes a table reference that exists in multiple schemas
// without a "public" fallback, making it ambiguous.
type AmbiguousTable struct {
	// Table is the ambiguous table name.
	Table string `json:"table"`

	// Schemas lists the schema names where this table was found.
	Schemas []string `json:"schemas"`
}

// ValidationWarning represents a non-fatal issue discovered during schema
// validation. Warnings do not block report generation but indicate that
// results may be incomplete or degraded.
type ValidationWarning struct {
	// Field identifies the affected field or data source reference.
	Field string `json:"field"`

	// Code is a machine-readable warning code (e.g., "DATA_SOURCE_UNAVAILABLE").
	Code string `json:"code"`

	// Message is a human-readable description of the warning.
	Message string `json:"message"`
}

// ExtractionJobRequest represents a request to extract data from a data source.
type ExtractionJobRequest struct {
	// DataSourceID identifies which data source to extract from.
	DataSourceID string `json:"dataSourceId"`

	// ReportID is the report that triggered this extraction.
	ReportID string `json:"reportId"`

	// TemplateID is the template associated with this extraction.
	TemplateID string `json:"templateId"`

	// TenantID is the tenant identity resolved from JWT context (D3 decision).
	// This is deliberately TenantID (not OrganizationID) for multi-tenant isolation.
	TenantID string `json:"tenantId"`

	// Fields lists the specific fields to extract from the data source.
	Fields []string `json:"fields,omitempty"`

	// Filters contains key-value pairs for filtering extracted data.
	Filters map[string]string `json:"filters,omitempty"`
}

// ExtractionMapping tracks the relationship between a Fetcher extraction job
// and a report. This is the minimal field set per requirements: JobID, ReportID,
// TemplateID, TenantID, Status, CreatedAt, CompletedAt.
type ExtractionMapping struct {
	// JobID is the unique identifier assigned by the Fetcher service.
	JobID string `json:"jobId"`

	// ReportID is the report this extraction is associated with.
	ReportID string `json:"reportId"`

	// TemplateID is the template used for this extraction.
	TemplateID string `json:"templateId"`

	// TenantID is the tenant identity resolved from JWT context (D3 decision).
	// This is deliberately TenantID (not OrganizationID) for multi-tenant isolation.
	TenantID string `json:"tenantId"`

	// OutputFormat is the original report output format (e.g., "html", "pdf").
	OutputFormat string `json:"outputFormat"`

	// Status tracks the extraction job lifecycle (e.g., "pending", "completed", "failed").
	Status string `json:"status"`

	// CreatedAt is the timestamp when the extraction job was created.
	CreatedAt time.Time `json:"createdAt"`

	// CompletedAt is the timestamp when the extraction job finished.
	// Nil when the job is still in progress.
	CompletedAt *time.Time `json:"completedAt,omitempty"`
}
