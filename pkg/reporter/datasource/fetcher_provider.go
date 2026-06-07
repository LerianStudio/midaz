// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package datasource

import (
	"context"
	"fmt"
	"strings"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/ctxutil"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/fetcher"

	"github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"go.opentelemetry.io/otel/attribute"
)

// fetcherWarningCodeDataSourceDown is the warning code returned by the Fetcher
// API when a data source is unreachable. Per D7 decision, this maps to
// WarningCodeDataSourceUnavailable in the Reporter domain.
const fetcherWarningCodeDataSourceDown = "DATA_SOURCE_DOWN"

// Compile-time interface satisfaction check.
var _ DataSourceProvider = (*FetcherProvider)(nil)

// FetcherManagementClient defines the subset of FetcherClient methods used by
// FetcherProvider. This interface enables mock-based unit testing without
// requiring a live Fetcher API.
type FetcherManagementClient interface {
	ListConnections(ctx context.Context) ([]fetcher.ConnectionResponse, error)
	GetConnectionSchema(ctx context.Context, connectionID string) (*fetcher.ConnectionSchemaResponse, error)
	ValidateSchema(ctx context.Context, mappedFields map[string]map[string][]string) (*fetcher.ValidateSchemaResponse, error)
	// Ping issues a readiness probe (GET /readyz) against the Fetcher API.
	// Returns nil on a 2xx response; returns an error otherwise. Used by the
	// /readyz handler to surface Fetcher reachability without coupling to
	// any business endpoint.
	Ping(ctx context.Context) error
}

// FetcherProvider implements DataSourceProvider by delegating to the Fetcher
// API via a FetcherManagementClient. Used when FETCHER_ENABLED=true (dual-mode).
//
// This provider does NOT contain any multi-tenant logic. Tenant context is
// handled by the FetcherClient's M2M token provider.
type FetcherProvider struct {
	client FetcherManagementClient
}

// NewFetcherProvider creates a FetcherProvider wrapping the given client.
func NewFetcherProvider(client FetcherManagementClient) *FetcherProvider {
	return &FetcherProvider{
		client: client,
	}
}

// Ping delegates to the underlying client's Ping method. Used by the /readyz
// handler when this provider is detected via type-assertion. Returns nil if
// the Fetcher API responds with 2xx, or an error otherwise.
func (fp *FetcherProvider) Ping(ctx context.Context) error {
	return fp.client.Ping(ctx)
}

// ListDataSources returns metadata for all registered data sources by querying
// the Fetcher management API and mapping ConnectionResponse to DataSourceInfo.
func (fp *FetcherProvider) ListDataSources(ctx context.Context) ([]DataSourceInfo, error) {
	logger := ctxutil.NewLoggerFromContext(ctx)
	tracer := ctxutil.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "provider.fetcher.list_data_sources")
	defer span.End()

	connections, err := fp.client.ListConnections(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to list data sources from fetcher", err)

		return nil, fmt.Errorf("failed to list data sources from fetcher: %w", err)
	}

	span.SetAttributes(attribute.Int("app.datasource.count", len(connections)))

	result := make([]DataSourceInfo, 0, len(connections))

	for _, conn := range connections {
		result = append(result, DataSourceInfo{
			ID:   conn.ID,
			Name: conn.ConfigName,
			Type: conn.Type,
		})
	}

	logger.Log(ctx, log.LevelDebug, "Listed data sources from Fetcher API",
		log.Int("count", len(result)))

	return result, nil
}

// GetDataSourceSchema retrieves the full schema for the specified data source
// by querying the Fetcher management API and mapping the response to internal
// DataSourceSchema types.
func (fp *FetcherProvider) GetDataSourceSchema(ctx context.Context, dataSourceID string) (*DataSourceSchema, error) {
	logger := ctxutil.NewLoggerFromContext(ctx)
	tracer := ctxutil.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "provider.fetcher.get_data_source_schema")
	defer span.End()

	span.SetAttributes(attribute.String("app.datasource.id", dataSourceID))

	if dataSourceID == "" {
		err := fmt.Errorf("data source ID must not be empty")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Empty data source ID", err)

		return nil, err
	}

	// Translate configName to UUID if a mapping exists (internal datasources
	// use configName as public ID but the fetcher API requires UUID).
	fetcherID := fp.resolveConnectionID(ctx, dataSourceID)

	schemaResp, err := fp.client.GetConnectionSchema(ctx, fetcherID)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get schema from fetcher", err)

		return nil, fmt.Errorf("failed to get schema from fetcher for %q: %w", dataSourceID, err)
	}

	schema := mapConnectionSchemaToDataSourceSchema(schemaResp)

	logger.Log(ctx, log.LevelDebug, "Retrieved schema from Fetcher API",
		log.String("data_source_id", dataSourceID),
		log.Int("table_count", len(schema.Tables)))

	return schema, nil
}

// ValidateSchema checks whether the requested fields exist in the specified
// data source's schema via the Fetcher management API.
//
// The mappedFields are passed directly to the Fetcher API which expects
// datasource→table→fields structure.
//
// Per D7 decision, DATA_SOURCE_DOWN warnings from Fetcher are mapped to
// WarningCodeDataSourceUnavailable (not errors).
func (fp *FetcherProvider) ValidateSchema(ctx context.Context, dataSourceID string, tableFields map[string][]string) (*ValidationResult, error) {
	logger := ctxutil.NewLoggerFromContext(ctx)
	tracer := ctxutil.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "provider.fetcher.validate_schema")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.datasource.id", dataSourceID),
		attribute.Int("app.datasource.table_count", len(tableFields)),
	)

	if len(tableFields) == 0 {
		err := fmt.Errorf("tableFields must not be empty for validation")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Empty tableFields", err)

		return nil, err
	}

	// Convert Pongo2 double-underscore notation back to dot notation for the
	// Fetcher API (e.g., "schema__table" → "schema.table"). The reporter
	// stores table names with __ for Pongo2 compatibility, but the fetcher
	// expects standard dot-qualified names.
	normalizedFields := make(map[string][]string, len(tableFields))
	for tableName, fields := range tableFields {
		normalizedFields[strings.ReplaceAll(tableName, "__", ".")] = fields
	}

	// Build mappedFields using the configName as key (the fetcher resolves
	// configNames to connections internally via its registry/repository).
	mappedFields := map[string]map[string][]string{
		dataSourceID: normalizedFields,
	}

	resp, err := fp.client.ValidateSchema(ctx, mappedFields)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to validate schema via fetcher", err)

		return nil, fmt.Errorf("failed to validate schema via fetcher for %q: %w", dataSourceID, err)
	}

	result := &ValidationResult{
		Valid: resp.IsSuccess(),
	}

	// Map fetcher validation errors to reporter domain
	if !result.Valid {
		for _, e := range resp.Errors {
			switch e.Type {
			case "TABLE_NOT_FOUND":
				result.MissingTables = append(result.MissingTables, e.Table)
			case "FIELD_NOT_FOUND":
				result.MissingFields = append(result.MissingFields, MissingFieldDetail{
					Table:  e.Table,
					Fields: []string{e.Field},
				})
			case "DATA_SOURCE_DOWN":
				result.Warnings = append(result.Warnings, ValidationWarning{
					Field:   e.DataSourceID,
					Code:    WarningCodeDataSourceUnavailable,
					Message: "data source is currently unavailable",
				})
			}
		}
	}

	logger.Log(ctx, log.LevelDebug, "Validated schema via Fetcher API",
		log.String("data_source_id", dataSourceID),
		log.Bool("valid", result.Valid),
		log.Int("error_count", len(resp.Errors)))

	return result, nil
}

// HealthCheck verifies connectivity with the Fetcher API by attempting to list
// connections. Each returned connection's status determines the health map value.
func (fp *FetcherProvider) HealthCheck(ctx context.Context) (map[string]bool, error) {
	logger := ctxutil.NewLoggerFromContext(ctx)
	tracer := ctxutil.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "provider.fetcher.health_check")
	defer span.End()

	connections, err := fp.client.ListConnections(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to check fetcher health", err)

		return nil, fmt.Errorf("failed to check fetcher health: %w", err)
	}

	result := make(map[string]bool, len(connections))

	for _, conn := range connections {
		result[conn.ID] = true // connections returned by fetcher are available
	}

	span.SetAttributes(attribute.Int("app.datasource.health_count", len(result)))

	logger.Log(ctx, log.LevelDebug, "Health check completed via Fetcher API",
		log.Int("connection_count", len(result)))

	return result, nil
}

// ConvertSchemaNotation converts dot notation (schema.table) to double
// underscore notation (schema__table) for Pongo2 template compatibility.
// Per D1 decision, dots are not valid in Pongo2 variable names so we use
// double underscores as separators.
func ConvertSchemaNotation(name string) string {
	return strings.ReplaceAll(name, ".", "__")
}

// MapWarningCode translates Fetcher API warning codes to Reporter domain
// warning codes. Per D7 decision, DATA_SOURCE_DOWN from Fetcher maps to
// DATA_SOURCE_UNAVAILABLE in Reporter (a warning, not an error).
func MapWarningCode(code string) string {
	if code == fetcherWarningCodeDataSourceDown {
		return WarningCodeDataSourceUnavailable
	}

	return code
}

// mapConnectionSchemaToDataSourceSchema converts a Fetcher ConnectionSchemaResponse
// to the internal DataSourceSchema type.
func mapConnectionSchemaToDataSourceSchema(resp *fetcher.ConnectionSchemaResponse) *DataSourceSchema {
	tables := make([]SchemaTable, 0, len(resp.Tables))

	for _, t := range resp.Tables {
		fields := make([]SchemaField, 0, len(t.Fields))

		for _, f := range t.Fields {
			fields = append(fields, SchemaField{
				Name: f.Name,
				Type: f.Type,
			})
		}

		tables = append(tables, SchemaTable{
			Name:   t.Name,
			Fields: fields,
		})
	}

	return &DataSourceSchema{
		DataSourceID: resp.ID,
		Tables:       tables,
	}
}

// resolveConnectionID translates a public data source ID (configName) to the
// fetcher's internal UUID by querying the fetcher's ListConnections API and
// matching by configName. This is multi-tenant safe because the fetcher
// resolves tenant context from the M2M token/JWT in the request.
// If no match is found, returns the input unchanged (it may already be a UUID).
func (fp *FetcherProvider) resolveConnectionID(ctx context.Context, dataSourceID string) string {
	connections, err := fp.client.ListConnections(ctx)
	if err != nil {
		return dataSourceID
	}

	for _, conn := range connections {
		if conn.ConfigName == dataSourceID {
			return conn.ID
		}
	}

	return dataSourceID
}
