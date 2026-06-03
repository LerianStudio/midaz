// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package datasource

//go:generate mockgen --destination=provider.mock.go --package=datasource --copyright_file=../../COPYRIGHT . DataSourceProvider

import "context"

// DataSourceProvider defines the contract for interacting with data sources
// in a mode-agnostic way. Implementations include a direct-query provider
// (legacy mode, querying databases directly) and a Fetcher provider (dual-mode,
// delegating extraction to the Fetcher API).
//
// The interface supports four operations:
//   - ListDataSources: enumerate registered data sources with metadata
//   - GetDataSourceSchema: retrieve the schema for a specific data source
//   - ValidateSchema: validate that requested fields exist in a data source schema
//   - HealthCheck: check connectivity to all registered data sources
type DataSourceProvider interface {
	// ListDataSources returns metadata for all registered data sources.
	// Returns an error if the context is cancelled or the provider cannot
	// enumerate data sources.
	ListDataSources(ctx context.Context) ([]DataSourceInfo, error)

	// GetDataSourceSchema returns the full schema (tables and fields) for the
	// specified data source. Returns an error if the data source ID is unknown
	// or empty, or if the schema cannot be retrieved.
	GetDataSourceSchema(ctx context.Context, dataSourceID string) (*DataSourceSchema, error)

	// ValidateSchema checks whether the requested table/field references exist
	// in the specified data source's schema. The tableFields parameter maps
	// table names (in any supported format: legacy, qualified, pongo2) to
	// field names for that table.
	//
	// Returns a ValidationResult with structured details:
	//   - MissingTables: tables not found in schema
	//   - MissingFields: fields not found per table
	//   - Ambiguous: tables that exist in multiple schemas without "public"
	//   - Warnings: non-fatal issues (e.g., DATA_SOURCE_UNAVAILABLE per D7)
	//
	// Returns an error if tableFields is empty or the data source ID is invalid.
	ValidateSchema(ctx context.Context, dataSourceID string, tableFields map[string][]string) (*ValidationResult, error)

	// HealthCheck returns a map of data source IDs to their connectivity status.
	// A value of true indicates the data source is reachable; false indicates
	// it is unavailable.
	HealthCheck(ctx context.Context) (map[string]bool, error)
}
