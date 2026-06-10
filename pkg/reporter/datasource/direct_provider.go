// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package datasource

import (
	"context"
	"fmt"
	"strings"

	pkgErr "github.com/LerianStudio/midaz/v4/pkg"

	constant "github.com/LerianStudio/midaz/v4/pkg/constant"
	pkg "github.com/LerianStudio/midaz/v4/pkg/reporter"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/ctxutil"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/mongodb"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/postgres"

	libConstants "github.com/LerianStudio/lib-commons/v5/commons/constants"
	"github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"go.opentelemetry.io/otel/attribute"
)

// Compile-time interface satisfaction check.
var _ DataSourceProvider = (*DirectProvider)(nil)

// DirectProvider implements DataSourceProvider by running schema discovery and
// validation in-process. It delegates schema introspection to the existing
// PostgreSQL and MongoDB schema logic, and serves both single-tenant and
// multi-tenant deployments:
//
//   - Single-tenant: introspects the global SafeDataSources map built at
//     startup, using the repository handles stored on each DataSource entry.
//   - Multi-tenant: resolves a per-tenant connection through the optional
//     tenantSchemaSource (backed by the lib-commons tenant managers) and runs
//     the SAME field-matching, schema-ambiguity, and CRM logic over the
//     tenant-scoped snapshot. The global pool is never read in this mode.
//
// The registry of datasource IDs (for ListDataSources and the CRM/org-scope
// decision) comes from SafeDataSources in both modes; only the SOURCE of the
// schema snapshot changes under multi-tenancy.
type DirectProvider struct {
	safeDatasources       *pkg.SafeDataSources
	circuitBreakerManager *pkg.CircuitBreakerManager
	healthChecker         *pkg.HealthChecker
	// tenantSchema is non-nil only in multi-tenant mode. When set, schema
	// discovery resolves the per-tenant pool through it (fail-closed) instead
	// of the global SafeDataSources connections.
	tenantSchema schemaSource
}

// NewDirectProvider creates a single-tenant DirectProvider wrapping the given
// SafeDataSources. circuitBreakerManager and healthChecker are optional (may be
// nil).
func NewDirectProvider(
	safeDatasources *pkg.SafeDataSources,
	circuitBreakerManager *pkg.CircuitBreakerManager,
	healthChecker *pkg.HealthChecker,
) *DirectProvider {
	return &DirectProvider{
		safeDatasources:       safeDatasources,
		circuitBreakerManager: circuitBreakerManager,
		healthChecker:         healthChecker,
	}
}

// NewMultiTenantDirectProvider creates a DirectProvider whose schema discovery
// resolves per-tenant connections through the lib-commons tenant managers. The
// SafeDataSources map still supplies the immutable datasource registry (IDs,
// types, CRM/org-scope configuration); the tenant managers supply the live
// per-tenant connection a schema read runs against. Both managers are required
// — multi-tenant mode cannot fail open to a shared pool.
func NewMultiTenantDirectProvider(
	safeDatasources *pkg.SafeDataSources,
	pg TenantPostgresManager,
	mongo TenantMongoManager,
	healthChecker *pkg.HealthChecker,
	logger log.Logger,
) *DirectProvider {
	// Fail closed at construction: a nil manager would survive here and nil-deref
	// on the first per-tenant schema read (the db==nil guards catch a nil RETURNED
	// connection, not a nil manager). Multi-tenant mode must never fall open to a
	// shared pool, so a missing manager is a bootstrap programmer error — die
	// before serving rather than risk a cross-tenant read. Panic is the sanctioned
	// fail-closed init-guard exception (E12-c).
	if pg == nil || mongo == nil {
		panic("NewMultiTenantDirectProvider: both tenant managers are required (multi-tenant mode cannot fail open to a shared pool)")
	}

	return &DirectProvider{
		safeDatasources: safeDatasources,
		healthChecker:   healthChecker,
		tenantSchema:    newTenantSchemaSource(pg, mongo, logger),
	}
}

// ensureConnected checks whether the given DataSource has an active database
// connection and, if not, triggers on-demand connection via SafeDataSources.
// After a successful connection the updated DataSource is re-read from the map
// so the caller receives the entry with initialized repositories.
func (dp *DirectProvider) ensureConnected(ctx context.Context, dataSourceID string, ds pkg.DataSource) (pkg.DataSource, error) {
	logger := ctxutil.NewLoggerFromContext(ctx)
	tracer := ctxutil.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "provider.direct.ensure_connected")
	defer span.End()

	needsConnection := false

	switch ds.DatabaseType {
	case pkg.PostgreSQLType:
		needsConnection = !ds.Initialized || ds.PostgresRepository == nil
	case pkg.MongoDBType:
		needsConnection = !ds.Initialized || ds.MongoDBRepository == nil
	}

	// D7: if the datasource is already known to be unavailable, skip reconnection
	// so callers can apply graceful degradation (warning instead of error).
	if ds.Status == libConstants.DataSourceStatusUnavailable {
		return ds, nil
	}

	if !needsConnection {
		return ds, nil
	}

	logger.Log(ctx, log.LevelDebug, "Connecting to datasource on-demand",
		log.String("data_source_id", dataSourceID),
		log.String("database_type", ds.DatabaseType))

	if err := dp.safeDatasources.ConnectDataSource(ctx, dataSourceID, &ds, logger); err != nil {
		logger.Log(ctx, log.LevelWarn, "On-demand datasource connection failed, datasource remains uninitialized",
			log.String("data_source_id", dataSourceID),
			log.Any("error", err))
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "On-demand connection failed (graceful degradation)", err)

		// Graceful degradation: return the datasource as-is (Initialized=false)
		// so the caller can decide how to handle the unavailable state.
		return ds, nil
	}

	// Re-read from SafeDataSources to get the updated entry with initialized repositories.
	updated, ok := dp.safeDatasources.Get(dataSourceID)
	if !ok {
		err := fmt.Errorf("datasource %q disappeared after on-demand connection", dataSourceID)
		libOpentelemetry.HandleSpanError(span, "Datasource not found after connection", err)

		return ds, err
	}

	logger.Log(ctx, log.LevelDebug, "Datasource connected on-demand successfully",
		log.String("data_source_id", dataSourceID))

	return updated, nil
}

// ListDataSources returns metadata for all registered data sources by iterating
// the SafeDataSources map. Both available and unavailable datasources are listed.
func (dp *DirectProvider) ListDataSources(ctx context.Context) ([]DataSourceInfo, error) {
	logger := ctxutil.NewLoggerFromContext(ctx)
	tracer := ctxutil.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "provider.direct.list_data_sources")
	defer span.End()

	allDS := dp.safeDatasources.GetAll()
	span.SetAttributes(attribute.Int("app.datasource.count", len(allDS)))

	logger.Log(ctx, log.LevelDebug, "Listing datasources via DirectProvider",
		log.Int("datasource_count", len(allDS)))

	result := make([]DataSourceInfo, 0, len(allDS))

	for id, ds := range allDS {
		info := DataSourceInfo{
			ID:     id,
			Name:   dp.resolveDisplayName(ds),
			Type:   ds.DatabaseType,
			Status: ds.Status,
		}

		result = append(result, info)
	}

	return result, nil
}

// GetDataSourceSchema retrieves the full schema for the specified data source
// by delegating to the underlying PostgreSQL or MongoDB repository.
func (dp *DirectProvider) GetDataSourceSchema(ctx context.Context, dataSourceID string) (*DataSourceSchema, error) {
	logger := ctxutil.NewLoggerFromContext(ctx)
	tracer := ctxutil.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "provider.direct.get_data_source_schema")
	defer span.End()

	span.SetAttributes(attribute.String("app.datasource.id", dataSourceID))

	if dataSourceID == "" {
		err := fmt.Errorf("data source ID must not be empty")
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Empty data source ID", err)

		return nil, err
	}

	ds, ok := dp.safeDatasources.Get(dataSourceID)
	if !ok {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Data source not found", ErrDataSourceNotFound)
		logger.Log(ctx, log.LevelWarn, "Data source not found in DirectProvider",
			log.String("data_source_id", dataSourceID))

		return nil, fmt.Errorf("%w: %w", ErrDataSourceNotFound, pkgErr.ValidateBusinessError(constant.ErrMissingDataSource, "", dataSourceID))
	}

	logger.Log(ctx, log.LevelDebug, "Retrieving schema via DirectProvider",
		log.String("data_source_id", dataSourceID),
		log.String("database_type", ds.DatabaseType))

	// Multi-tenant mode resolves the per-tenant connection inside the schema
	// source (fail-closed), so the env-pool lazy-connect and initialized guards
	// below do not apply: the global SafeDataSources entry carries no live
	// per-tenant connection. Dispatch straight to the type-specific reader,
	// which routes through the tenant source.
	if dp.tenantSchema != nil {
		switch ds.DatabaseType {
		case pkg.PostgreSQLType:
			return dp.getPostgresSchema(ctx, dataSourceID, ds)
		case pkg.MongoDBType:
			return dp.getMongoDBSchema(ctx, dataSourceID, ds)
		default:
			err := fmt.Errorf("unsupported database type %q for datasource %q", ds.DatabaseType, dataSourceID)
			libOpentelemetry.HandleSpanError(span, "Unsupported database type", err)

			return nil, err
		}
	}

	// Ensure the datasource has an active connection (lazy initialization).
	// ensureConnected uses graceful degradation: it never returns an error,
	// but the datasource may remain uninitialized if connection failed.
	ds, _ = dp.ensureConnected(ctx, dataSourceID, ds)

	switch ds.DatabaseType {
	case pkg.PostgreSQLType:
		if !ds.Initialized || ds.PostgresRepository == nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "PostgreSQL datasource unavailable after connection attempt", ErrDataSourceUnavailable)

			return nil, pkgErr.ValidateBusinessError(constant.ErrDataSourceUnavailable, "", dataSourceID)
		}

		return dp.getPostgresSchema(ctx, dataSourceID, ds)
	case pkg.MongoDBType:
		if !ds.Initialized || ds.MongoDBRepository == nil {
			libOpentelemetry.HandleSpanBusinessErrorEvent(span, "MongoDB datasource unavailable after connection attempt", ErrDataSourceUnavailable)

			return nil, pkgErr.ValidateBusinessError(constant.ErrDataSourceUnavailable, "", dataSourceID)
		}

		return dp.getMongoDBSchema(ctx, dataSourceID, ds)
	default:
		err := fmt.Errorf("unsupported database type %q for datasource %q", ds.DatabaseType, dataSourceID)
		libOpentelemetry.HandleSpanError(span, "Unsupported database type", err)

		return nil, err
	}
}

// ValidateSchema performs table-aware validation of mapped fields against the
// data source's schema. For each table in tableFields, it verifies that the
// table exists and that all referenced fields are present.
//
// PostgreSQL-specific: supports 3 table name formats (pongo2, qualified, legacy)
// and detects schema ambiguity. MongoDB-specific: applies organization-scoped
// collection lookup for crm datasources.
//
// Per decision, unavailable datasources produce a warning (not error).
func (dp *DirectProvider) ValidateSchema(ctx context.Context, dataSourceID string, tableFields map[string][]string) (*ValidationResult, error) {
	logger := ctxutil.NewLoggerFromContext(ctx)
	tracer := ctxutil.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "provider.direct.validate_schema")
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

	ds, ok := dp.safeDatasources.Get(dataSourceID)
	if !ok {
		libOpentelemetry.HandleSpanBusinessErrorEvent(span, "Data source not found for validation", ErrDataSourceNotFound)

		// Match GetDataSourceSchema's not-found shape: wrap ErrDataSourceNotFound
		// alongside the business error so callers can errors.Is the sentinel and
		// WithError still classifies the business error to its HTTP status.
		return nil, fmt.Errorf("%w: %w", ErrDataSourceNotFound, pkgErr.ValidateBusinessError(constant.ErrMissingDataSource, "", dataSourceID))
	}

	// Multi-tenant mode resolves the per-tenant connection inside the schema
	// source (fail-closed). The env-pool lazy-connect and the D7 env-pool
	// availability guard below do not apply, because the global SafeDataSources
	// entry holds no live per-tenant connection. Dispatch straight to the
	// type-specific validator. A tenant-resolution failure surfaces as a hard
	// error (fail closed) rather than a D7 warning: under MT, an unresolvable
	// tenant pool is a real failure, not a degraded-but-known datasource.
	if dp.tenantSchema != nil {
		switch ds.DatabaseType {
		case pkg.PostgreSQLType:
			return dp.validatePostgresSchema(ctx, dataSourceID, ds, tableFields)
		case pkg.MongoDBType:
			return dp.validateMongoDBSchema(ctx, dataSourceID, ds, tableFields)
		default:
			err := fmt.Errorf("unsupported database type %q for datasource %q", ds.DatabaseType, dataSourceID)
			libOpentelemetry.HandleSpanError(span, "Unsupported database type", err)

			return nil, err
		}
	}

	// Ensure the datasource has an active connection (lazy initialization).
	// ensureConnected uses graceful degradation: it never returns an error,
	// but the datasource may remain uninitialized. The D7 check below
	// handles that case gracefully as a warning.
	ds, _ = dp.ensureConnected(ctx, dataSourceID, ds)

	// D7: unavailable datasources produce warnings, not errors
	if ds.Status == libConstants.DataSourceStatusUnavailable || !ds.Initialized {
		logger.Log(ctx, log.LevelWarn, "Data source is unavailable, returning warning per D7",
			log.String("data_source_id", dataSourceID),
			log.String("status", ds.Status))

		return &ValidationResult{
			Valid: true,
			Warnings: []ValidationWarning{
				{
					Field:   dataSourceID,
					Code:    WarningCodeDataSourceUnavailable,
					Message: fmt.Sprintf("Data source %q is currently unavailable; validation skipped", dataSourceID),
				},
			},
		}, nil
	}

	switch ds.DatabaseType {
	case pkg.PostgreSQLType:
		return dp.validatePostgresSchema(ctx, dataSourceID, ds, tableFields)
	case pkg.MongoDBType:
		return dp.validateMongoDBSchema(ctx, dataSourceID, ds, tableFields)
	default:
		err := fmt.Errorf("unsupported database type %q for datasource %q", ds.DatabaseType, dataSourceID)
		libOpentelemetry.HandleSpanError(span, "Unsupported database type", err)

		return nil, err
	}
}

// validatePostgresSchema validates tableFields against a PostgreSQL schema.
// Supports 3 table name formats: pongo2 (schema__table), qualified (schema.table), legacy (table).
// Detects schema ambiguity for unqualified table names.
func (dp *DirectProvider) validatePostgresSchema(
	ctx context.Context,
	dataSourceID string,
	ds pkg.DataSource,
	tableFields map[string][]string,
) (*ValidationResult, error) {
	tracer := ctxutil.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "provider.direct.validate_postgres_schema")
	defer span.End()

	dbSchema, err := dp.postgresTableSchemas(ctx, dataSourceID, ds)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get PostgreSQL schema", err)

		return nil, fmt.Errorf("failed to get PostgreSQL schema for %q: %w", dataSourceID, err)
	}

	result := &ValidationResult{Valid: true}

	// Check schema ambiguity for unqualified table names
	ambiguous := detectSchemaAmbiguity(dbSchema, tableFields)
	if len(ambiguous) > 0 {
		result.Valid = false
		result.Ambiguous = ambiguous

		return result, nil
	}

	// Build lookup: for each schema table, map all 3 format variants to the table schema
	matchedTables := make(map[string]bool)

	for _, s := range dbSchema {
		qualifiedName := s.QualifiedName()                         // "schema.table"
		pongo2Name := strings.Replace(qualifiedName, ".", "__", 1) // "schema__table"
		legacyName := s.TableName                                  // "table"

		// Find which format the caller used (if any)
		var (
			requestedFields []string
			matchedKey      string
		)

		switch {
		case tableFields[pongo2Name] != nil:
			requestedFields = tableFields[pongo2Name]
			matchedKey = pongo2Name
		case tableFields[qualifiedName] != nil:
			requestedFields = tableFields[qualifiedName]
			matchedKey = qualifiedName
		case tableFields[legacyName] != nil:
			requestedFields = tableFields[legacyName]
			matchedKey = legacyName
		}

		// An empty requested-field list means "table must exist, no fields to
		// check": mark it matched before the early-continue so it is not
		// mis-collected as a MissingTable. This mirrors the MongoDB validator,
		// which accepts an empty-field table as present.
		if matchedKey != "" {
			matchedTables[matchedKey] = true
		}

		if len(requestedFields) == 0 {
			continue
		}

		// Validate fields using existing helper
		var countIfTableExist int32

		missing := postgres.ValidateFieldsInSchemaPostgres(requestedFields, s, &countIfTableExist)
		if len(missing) > 0 {
			result.Valid = false
			result.MissingFields = append(result.MissingFields, MissingFieldDetail{
				Table:  matchedKey,
				Fields: missing,
			})
		}
	}

	// Collect unmatched tables
	for tableKey := range tableFields {
		if !matchedTables[tableKey] {
			result.Valid = false
			result.MissingTables = append(result.MissingTables, tableKey)
		}
	}

	return result, nil
}

// validateMongoDBSchema validates tableFields against a MongoDB schema.
// For crm datasources with MidazOrganizationID, applies org-scoped
// collection lookup and table name transformation.
func (dp *DirectProvider) validateMongoDBSchema(
	ctx context.Context,
	dataSourceID string,
	ds pkg.DataSource,
	tableFields map[string][]string,
) (*ValidationResult, error) {
	tracer := ctxutil.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "provider.direct.validate_mongodb_schema")
	defer span.End()

	// Resolve the raw collection schema for validation: an org-scoped datasource
	// uses the org-suffix filter, others use plain discovery. Validation does NOT
	// use crm prefix-grouping — it relies on the org-suffix collection-name
	// transformation applied below. Single-tenant reads the env pool;
	// multi-tenant resolves the tenant-scoped database (fail-closed).
	collectionSchemas, schemaErr := dp.mongoSchemaForValidation(ctx, dataSourceID, ds)
	if schemaErr != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get MongoDB schema", schemaErr)

		return nil, fmt.Errorf("failed to get MongoDB schema for %q: %w", dataSourceID, schemaErr)
	}

	result := &ValidationResult{Valid: true}

	// Build a lookup from collection name → schema for matching
	collectionLookup := make(map[string]mongodb.CollectionSchema, len(collectionSchemas))
	for _, cs := range collectionSchemas {
		collectionLookup[cs.CollectionName] = cs
	}

	for tableKey, requestedFields := range tableFields {
		if len(requestedFields) == 0 {
			continue
		}

		// For crm, transform table name: tableName + "_" + orgID
		lookupName := tableKey
		if ds.MidazOrganizationID != "" {
			lookupName = tableKey + "_" + ds.MidazOrganizationID
		}

		cs, found := collectionLookup[lookupName]
		if !found {
			// Also try direct match without transformation
			cs, found = collectionLookup[tableKey]
		}

		if !found {
			result.Valid = false
			result.MissingTables = append(result.MissingTables, tableKey)

			continue
		}

		var countIfTableExist int32

		missing := mongodb.ValidateFieldsInSchemaMongo(requestedFields, cs, &countIfTableExist)
		if len(missing) > 0 {
			result.Valid = false
			result.MissingFields = append(result.MissingFields, MissingFieldDetail{
				Table:  tableKey,
				Fields: missing,
			})
		}
	}

	return result, nil
}

// detectSchemaAmbiguity checks for unqualified table names that exist in multiple
// schemas without "public" as one of them, making the reference ambiguous.
func detectSchemaAmbiguity(dbSchema []postgres.TableSchema, tableFields map[string][]string) []AmbiguousTable {
	// Build table name → list of schemas
	tableSchemas := make(map[string][]string)
	for _, s := range dbSchema {
		tableSchemas[s.TableName] = append(tableSchemas[s.TableName], s.SchemaName)
	}

	var ambiguous []AmbiguousTable

	for tableKey := range tableFields {
		// Skip tables with explicit schema (pongo2 or qualified format)
		if strings.Contains(tableKey, "__") || strings.Contains(tableKey, ".") {
			continue
		}

		schemaList, exists := tableSchemas[tableKey]
		if !exists || len(schemaList) <= 1 {
			continue
		}

		hasPublic := false

		for _, s := range schemaList {
			if s == "public" {
				hasPublic = true

				break
			}
		}

		if !hasPublic {
			ambiguous = append(ambiguous, AmbiguousTable{
				Table:   tableKey,
				Schemas: schemaList,
			})
		}
	}

	return ambiguous
}

// HealthCheck returns connectivity status for all registered datasources.
// When a HealthChecker is available, it delegates to GetHealthStatus.
// Otherwise, it derives status from the SafeDataSources map directly.
func (dp *DirectProvider) HealthCheck(ctx context.Context) (map[string]bool, error) {
	tracer := ctxutil.NewTracerFromContext(ctx)

	_, span := tracer.Start(ctx, "provider.direct.health_check")
	defer span.End()

	allDS := dp.safeDatasources.GetAll()
	result := make(map[string]bool, len(allDS))

	if dp.healthChecker != nil {
		healthStatus := dp.healthChecker.GetHealthStatus()

		for id := range allDS {
			status, ok := healthStatus[id]
			if ok {
				// HealthChecker returns "available (CB: closed)" format
				// Consider available if the status string starts with "available"
				result[id] = strings.HasPrefix(status, libConstants.DataSourceStatusAvailable)
			} else {
				result[id] = false
			}
		}

		return result, nil
	}

	// Fallback: derive from SafeDataSources status field
	for id, ds := range allDS {
		result[id] = ds.Status == libConstants.DataSourceStatusAvailable && ds.Initialized
	}

	return result, nil
}
