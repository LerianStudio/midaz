// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	pkgErr "github.com/LerianStudio/midaz/v4/pkg"

	cnErr "github.com/LerianStudio/midaz/v4/pkg/constant"
	pkg "github.com/LerianStudio/midaz/v4/pkg/reporter"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/constant"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/ctxutil"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/model"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/postgres"

	libConstants "github.com/LerianStudio/lib-commons/v5/commons/constants"
	libCrypto "github.com/LerianStudio/lib-commons/v5/commons/crypto"
	"github.com/LerianStudio/lib-observability/log"
	libOtel "github.com/LerianStudio/lib-observability/tracing"

	// otel/attribute is used for span attribute types (no lib-commons wrapper available)
	"go.opentelemetry.io/otel/attribute"
)

// sectionFailure records a per-database (section) data-retrieval failure with its
// canonical error code (E9). Used to classify a report as PARTIAL when only some
// sections fail.
type sectionFailure struct {
	database  string
	errorCode string
}

// queryExternalData retrieves data from external data sources specified in the
// message and populates the result map. Per-section (per-database) failures are
// accumulated and returned rather than aborting the whole report: a section that
// fails is skipped, succeeded sections still populate result. The returned error
// is reserved for fatal conditions (e.g. context cancellation); a non-nil slice of
// sectionFailure indicates sections that could not be retrieved.
func (uc *UseCase) queryExternalData(ctx context.Context, message GenerateReportMessage, result map[string]map[string][]map[string]any) ([]sectionFailure, error) {
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := uc.Tracer.Start(ctx, "service.report.query_external_data")
	defer span.End()

	span.SetAttributes(attribute.String("app.request.request_id", reqId))

	var failures []sectionFailure

	for databaseName, tables := range message.DataQueries {
		if err := ctx.Err(); err != nil {
			return failures, err
		}

		if err := uc.queryDatabase(ctx, databaseName, tables, message.Filters, result); err != nil {
			uc.Logger.Log(ctx, log.LevelWarn, "Data section failed, recording for partial classification",
				log.String("database", databaseName), log.Err(err))

			failures = append(failures, sectionFailure{
				database:  databaseName,
				errorCode: classifyReporterErrorCode(err),
			})

			// Drop any partial map seeded for this section so it is not rendered.
			delete(result, databaseName)
		}
	}

	span.SetAttributes(attribute.Int("app.report.failed_section_count", len(failures)))

	return failures, nil
}

// classifyReporterErrorCode extracts the canonical numeric error code carried by a
// typed reporter error (E9: classified code, never raw text). It falls back to the
// generic internal-server code when the error is untyped.
func classifyReporterErrorCode(err error) string {
	var (
		validationErr   pkgErr.ValidationError
		preconditionErr pkgErr.FailedPreconditionError
		notFoundErr     pkgErr.EntityNotFoundError
		unauthorizedErr pkgErr.UnauthorizedError
		internalErr     pkgErr.InternalServerError
	)

	switch {
	case errors.As(err, &validationErr):
		return validationErr.Code
	case errors.As(err, &preconditionErr):
		return preconditionErr.Code
	case errors.As(err, &notFoundErr):
		return notFoundErr.Code
	case errors.As(err, &unauthorizedErr):
		return unauthorizedErr.Code
	case errors.As(err, &internalErr):
		return internalErr.Code
	default:
		return cnErr.ErrInternalServer.Error()
	}
}

// decideReportStatus classifies the terminal report status from the number of
// attempted sections and the accumulated per-section failures:
//
//	all sections ok -> FinishedStatus (no failure metadata)
//	all sections failed -> ErrorStatus (with per-section codes)
//	mixed -> PartialStatus (with per-section codes for the failed sections)
//
// When failures exist, the returned metadata carries a "sections" map keyed by
// database name, each value an {"error_code": <canonical>} entry (E9).
func decideReportStatus(attempted int, failures []sectionFailure) (string, map[string]any) {
	if len(failures) == 0 {
		return constant.FinishedStatus, nil
	}

	sections := make(map[string]any, len(failures))
	for _, f := range failures {
		sections[f.database] = map[string]any{"error_code": f.errorCode}
	}

	metadata := map[string]any{"sections": sections}

	if len(failures) >= attempted {
		metadata["error"] = "All report data sections failed"
		metadata["error_code"] = cnErr.ErrExtractionJobFailed.Error()

		return constant.ErrorStatus, metadata
	}

	metadata["error"] = "Some report data sections failed"
	metadata["error_code"] = cnErr.ErrExtractionJobFailed.Error()

	return constant.PartialStatus, metadata
}

// queryDatabase handles data retrieval for a specific database
func (uc *UseCase) queryDatabase(
	ctx context.Context,
	databaseName string,
	tables map[string][]string,
	allFilters map[string]map[string]map[string]model.FilterCondition,
	result map[string]map[string][]map[string]any,
) error {
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, dbSpan := uc.Tracer.Start(ctx, "service.report.query_database")
	defer dbSpan.End()

	dbSpan.SetAttributes(attribute.String("app.request.request_id", reqId))
	uc.Logger.Log(ctx, log.LevelDebug, "Querying database", log.String("database", databaseName))

	dataSource, exists := uc.ExternalDataSources.Get(databaseName)
	if !exists {
		err := pkgErr.ValidationError{Code: cnErr.ErrCodeDataSourceNotFound.Error(), Title: "Data Source Not Found", Message: fmt.Sprintf("data source not found: %s", databaseName)}
		libOtel.HandleSpanError(dbSpan, "Referenced datasource is missing", err)
		uc.Logger.Log(ctx, log.LevelError, "Referenced datasource is missing, failing report",
			log.String("database", databaseName))

		return err
	}

	// Check circuit breaker state before attempting query
	if !uc.CircuitBreakerManager.IsHealthy(databaseName) {
		cbState := uc.CircuitBreakerManager.GetState(databaseName)
		err := fmt.Errorf("datasource %s is unhealthy - circuit breaker state: %s", databaseName, cbState)
		libOtel.HandleSpanError(dbSpan, "Circuit breaker blocking request", err)
		uc.Logger.Log(ctx, log.LevelError, "Circuit breaker blocking request to datasource",
			log.String("database", databaseName), log.String("circuit_breaker_state", cbState))

		return err
	}

	// Check datasource initialization status
	if !dataSource.Initialized {
		// Check if datasource is marked as unavailable from initialization
		if dataSource.Status == libConstants.DataSourceStatusUnavailable {
			err := pkgErr.FailedPreconditionError{Code: cnErr.ErrCodeDataSourceUnavailable.Error(), Title: "Data Source Unavailable", Message: fmt.Sprintf("datasource %s is unavailable (initialization failed)", databaseName)}
			libOtel.HandleSpanError(dbSpan, "Datasource unavailable", err)
			uc.Logger.Log(ctx, log.LevelError, "Datasource is unavailable",
				log.String("database", databaseName), log.String("last_error", pkg.SanitizeExternalError(dataSource.LastError)))

			return err
		}

		// Attempt to connect
		if err := uc.ExternalDataSources.ConnectDataSource(ctx, databaseName, &dataSource, uc.Logger); err != nil {
			libOtel.HandleSpanError(dbSpan, "Error initializing database connection.", err)
			return err
		}
	}

	// Prepare a result map for this database
	if _, databaseExists := result[databaseName]; !databaseExists {
		result[databaseName] = make(map[string][]map[string]any)
	}

	// Get filters for this database
	databaseFilters := allFilters[databaseName]

	switch dataSource.DatabaseType {
	case pkg.PostgreSQLType:
		return uc.queryPostgresDatabase(ctx, &dataSource, databaseName, tables, databaseFilters, result)
	case pkg.MongoDBType:
		return uc.queryMongoDatabase(ctx, &dataSource, databaseName, tables, databaseFilters, result)
	default:
		return pkgErr.ValidationError{Code: cnErr.ErrCodeUnsupportedDatabaseType.Error(), Title: "Unsupported Database Type", Message: fmt.Sprintf("unsupported database type: %s for database: %s", dataSource.DatabaseType, databaseName)}
	}
}

// queryPostgresDatabase handles querying PostgresSQL databases
func (uc *UseCase) queryPostgresDatabase(
	ctx context.Context,
	dataSource *pkg.DataSource,
	databaseName string,
	tables map[string][]string,
	databaseFilters map[string]map[string]model.FilterCondition,
	result map[string]map[string][]map[string]any,
) error {
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := uc.Tracer.Start(ctx, "service.report.query_postgres_database")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.database_name", databaseName),
	)

	// Use configured schemas or default to public
	configuredSchemas := dataSource.Schemas
	if len(configuredSchemas) == 0 {
		configuredSchemas = []string{"public"}
	}

	uc.Logger.Log(ctx, log.LevelDebug, "Querying database with schemas",
		log.String("database", databaseName), log.Any("schemas", configuredSchemas))

	// Execute schema query with circuit breaker protection
	schemaResult, err := uc.CircuitBreakerManager.Execute(databaseName, func() (any, error) {
		return dataSource.PostgresRepository.GetDatabaseSchema(ctx, configuredSchemas)
	})
	if err != nil {
		uc.Logger.Log(ctx, log.LevelError, "Error getting database schema (circuit breaker)",
			log.String("database", databaseName), log.Err(err))

		return err
	}

	schema, ok := schemaResult.([]postgres.TableSchema)
	if !ok {
		uc.Logger.Log(ctx, log.LevelError, "Unexpected schema result type",
			log.String("database", databaseName), log.Any("actual_type", schemaResult))

		return pkgErr.FailedPreconditionError{Code: cnErr.ErrCodeUnexpectedSchemaResult.Error(), Title: "Unexpected Schema Result", Message: fmt.Sprintf("unexpected schema result type for database %s", databaseName)}
	}

	// Initialize SchemaResolver with discovered tables
	resolver := pkg.NewSchemaResolver()
	resolver.RegisterDatabase(databaseName, schema)

	for tableKey, fields := range tables {
		tableFilters := getTableFilters(databaseFilters, tableKey)

		// Parse table key to extract explicit schema if present
		// Supports multiple formats:
		// - "schema__table" (Pongo2 compatible format from CleanPath)
		// - "schema.table" (explicit qualified format)
		// - "table" (autodiscovery)
		var explicitSchema, tableName string

		if strings.Contains(tableKey, "__") {
			// Pongo2 format: schema__table -> split by double underscore
			parts := strings.SplitN(tableKey, "__", constant.SplitKeyValueParts)
			explicitSchema = parts[0]
			tableName = parts[1]
		} else if strings.Contains(tableKey, ".") {
			// Qualified format: schema.table -> split by dot
			parts := strings.SplitN(tableKey, ".", constant.SplitKeyValueParts)
			explicitSchema = parts[0]
			tableName = parts[1]
		} else {
			tableName = tableKey
		}

		// Resolve schema name for this table
		schemaName, err := resolver.ResolveSchema(databaseName, explicitSchema, tableName)
		if err != nil {
			// Check if it's an ambiguity error for actionable message
			if ambiguityErr, ok := err.(*pkg.SchemaAmbiguityError); ok {
				uc.Logger.Log(ctx, log.LevelError, "Schema ambiguity for table",
					log.String("table", tableName), log.String("database", databaseName), log.Err(ambiguityErr))
			} else {
				uc.Logger.Log(ctx, log.LevelError, "Error resolving schema for table",
					log.String("table", tableName), log.String("database", databaseName), log.Err(err))
			}

			return err
		}

		uc.Logger.Log(ctx, log.LevelDebug, "Resolved schema for table",
			log.String("schema", schemaName), log.String("table", tableName), log.String("database", databaseName))

		// Execute query with circuit breaker protection
		var tableResult []map[string]any

		queryResult, err := uc.CircuitBreakerManager.Execute(databaseName, func() (any, error) {
			if len(tableFilters) > 0 {
				return dataSource.PostgresRepository.QueryWithAdvancedFilters(ctx, schema, schemaName, tableName, fields, tableFilters)
			}

			return dataSource.PostgresRepository.Query(ctx, schema, schemaName, tableName, fields, nil)
		})
		if err != nil {
			uc.Logger.Log(ctx, log.LevelError, "Error querying table (circuit breaker)",
				log.String("schema", schemaName), log.String("table", tableName),
				log.String("database", databaseName), log.Err(err))

			return err
		}

		tableResult, ok := queryResult.([]map[string]any)
		if !ok {
			return pkgErr.FailedPreconditionError{Code: cnErr.ErrCodeUnexpectedTableResult.Error(), Title: "Unexpected Query Result", Message: fmt.Sprintf("unexpected query result type for table %s.%s in %s", schemaName, tableName, databaseName)}
		}

		if len(tableFilters) > 0 {
			uc.Logger.Log(ctx, log.LevelDebug, "Successfully queried table with advanced filters",
				log.String("schema", schemaName), log.String("table", tableName),
				log.String("circuit_breaker_state", uc.CircuitBreakerManager.GetState(databaseName)))
		} else {
			uc.Logger.Log(ctx, log.LevelDebug, "Successfully queried table",
				log.String("schema", schemaName), log.String("table", tableName),
				log.String("circuit_breaker_state", uc.CircuitBreakerManager.GetState(databaseName)))
		}

		// Store result using the original tableKey which is already in Pongo2-compatible format
		// (schema__table from CleanPath) for dot notation access in templates
		result[databaseName][tableKey] = tableResult
	}

	return nil
}

// queryMongoDatabase handles querying MongoDB databases
func (uc *UseCase) queryMongoDatabase(
	ctx context.Context,
	dataSource *pkg.DataSource,
	databaseName string,
	collections map[string][]string,
	databaseFilters map[string]map[string]model.FilterCondition,
	result map[string]map[string][]map[string]any,
) error {
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := uc.Tracer.Start(ctx, "service.report.query_mongo_database")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.database_name", databaseName),
	)

	for collection, fields := range collections {
		collectionFilters := getTableFilters(databaseFilters, collection)

		if err := uc.processMongoCollection(ctx, dataSource, databaseName, collection, fields, collectionFilters, result); err != nil {
			libOtel.HandleSpanError(span, "Error processing MongoDB collection", err)
			return err
		}
	}

	return nil
}

// processMongoCollection processes a single MongoDB collection
func (uc *UseCase) processMongoCollection(
	ctx context.Context,
	dataSource *pkg.DataSource,
	databaseName, collection string,
	fields []string,
	collectionFilters map[string]model.FilterCondition,
	result map[string]map[string][]map[string]any,
) error {
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := uc.Tracer.Start(ctx, "service.report.process_mongo_collection")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.database_name", databaseName),
		attribute.String("app.request.collection", collection),
	)

	// Handle plugin_crm special cases
	if databaseName == "plugin_crm" {
		// Skip "organization" collection - it's not a real collection, just stores the organizationID for template context
		if collection == "organization" {
			uc.Logger.Log(ctx, log.LevelDebug, "Skipping organization collection for plugin_crm - it's a metadata field, not a queryable collection")
			return nil
		}

		if err := uc.processPluginCRMCollection(ctx, dataSource, collection, fields, collectionFilters, result); err != nil {
			libOtel.HandleSpanError(span, "Error processing plugin_crm collection", err)
			return err
		}

		return nil
	}

	// Handle regular collections
	if err := uc.processRegularMongoCollection(ctx, dataSource, databaseName, collection, fields, collectionFilters, result); err != nil {
		libOtel.HandleSpanError(span, "Error processing regular MongoDB collection", err)
		return err
	}

	return nil
}

// processPluginCRMCollection handles plugin_crm specific collection processing.
// It discovers all org-scoped collections matching the logical name (e.g. "holders_*"),
// queries each one individually, merges the results into a single slice under the
// logical name, and decrypts sensitive fields.
func (uc *UseCase) processPluginCRMCollection(
	ctx context.Context,
	dataSource *pkg.DataSource,
	collection string,
	fields []string,
	collectionFilters map[string]model.FilterCondition,
	result map[string]map[string][]map[string]any,
) error {
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := uc.Tracer.Start(ctx, "service.report.process_plugin_crm_collection")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.collection", collection),
	)

	// Discover all physical collections matching the logical prefix (e.g. "holders_*").
	allCollections, err := dataSource.MongoDBRepository.ListCollectionNames(ctx)
	if err != nil {
		libOtel.HandleSpanError(span, "Failed to list collection names for plugin_crm", err)

		return fmt.Errorf("failed to list plugin_crm collections: %w", err)
	}

	prefix := collection + "_"

	// Filter and sort matching collections so the merge order is deterministic.
	// Without sorting, templates using index-based access (e.g. holders.0.document)
	// would render different organizations depending on ListCollectionNames order.
	var matchingCollections []string

	for _, coll := range allCollections {
		if strings.HasPrefix(coll, prefix) {
			matchingCollections = append(matchingCollections, coll)
		}
	}

	sort.Strings(matchingCollections)

	var allResults []map[string]any

	for _, physColl := range matchingCollections {
		orgID := strings.TrimPrefix(physColl, prefix)

		uc.Logger.Log(ctx, log.LevelDebug, "Querying plugin_crm org-scoped collection",
			log.String("physical", physColl),
			log.String("logical", collection),
			log.String("organization_id", orgID),
		)

		collectionResult, queryErr := uc.queryMongoCollectionWithFilters(ctx, dataSource, physColl, fields, collectionFilters, "plugin_crm")
		if queryErr != nil {
			uc.Logger.Log(ctx, log.LevelError, "Failed to query plugin_crm collection",
				log.String("collection", physColl), log.String("organization_id", orgID), log.Err(queryErr))

			return fmt.Errorf("failed to query plugin_crm collection %s: %w", physColl, queryErr)
		}

		// Inject organization_id into each record so the template can identify the source org.
		for i := range collectionResult {
			collectionResult[i]["organization_id"] = orgID
		}

		allResults = append(allResults, collectionResult...)
	}

	span.SetAttributes(attribute.Int("app.request.plugin_crm.total_records", len(allResults)))

	uc.Logger.Log(ctx, log.LevelDebug, "Merged plugin_crm results",
		log.String("collection", collection),
		log.Int("total_records", len(allResults)),
	)

	result["plugin_crm"][collection] = allResults

	// Decrypt sensitive fields.
	decryptedResult, err := uc.decryptPluginCRMData(result["plugin_crm"][collection], fields)
	if err != nil {
		uc.Logger.Log(ctx, log.LevelError, "Error decrypting data for collection",
			log.String("collection", collection), log.Err(err))

		return pkgErr.ValidateBusinessError(cnErr.ErrDecryptionData, "", err)
	}

	result["plugin_crm"][collection] = decryptedResult

	return nil
}

// processRegularMongoCollection handles regular MongoDB collection processing
func (uc *UseCase) processRegularMongoCollection(
	ctx context.Context,
	dataSource *pkg.DataSource,
	databaseName string,
	collection string,
	fields []string,
	collectionFilters map[string]model.FilterCondition,
	result map[string]map[string][]map[string]any,
) error {
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := uc.Tracer.Start(ctx, "service.report.process_regular_mongo_collection")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.database_name", databaseName),
		attribute.String("app.request.collection", collection),
	)

	collectionResult, err := uc.queryMongoCollectionWithFilters(ctx, dataSource, collection, fields, collectionFilters, databaseName)
	if err != nil {
		return err
	}

	result[databaseName][collection] = collectionResult

	return nil
}

// queryMongoCollectionWithFilters queries a MongoDB collection with or without filters
func (uc *UseCase) queryMongoCollectionWithFilters(
	ctx context.Context,
	dataSource *pkg.DataSource,
	collection string,
	fields []string,
	collectionFilters map[string]model.FilterCondition,
	databaseName string,
) ([]map[string]any, error) {
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := uc.Tracer.Start(ctx, "service.report.query_mongo_filters")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.database_name", databaseName),
		attribute.String("app.request.collection", collection),
	)

	// Execute query with circuit breaker protection
	queryResult, err := uc.CircuitBreakerManager.Execute(databaseName, func() (any, error) {
		if len(collectionFilters) > 0 {
			// Check if this is plugin_crm and needs filter transformation
			if databaseName == "plugin_crm" {
				transformedFilter, err := uc.transformPluginCRMAdvancedFilters(collectionFilters)
				if err != nil {
					return nil, fmt.Errorf("error transforming advanced filters for collection %s: %w", collection, err)
				}

				collectionFilters = transformedFilter
			}

			return dataSource.MongoDBRepository.QueryWithAdvancedFilters(ctx, collection, fields, collectionFilters)
		}

		// No filters, use legacy method
		return dataSource.MongoDBRepository.Query(ctx, collection, fields, nil)
	})
	if err != nil {
		libOtel.HandleSpanError(span, "Error querying MongoDB collection", err)
		uc.Logger.Log(ctx, log.LevelError, "Error querying collection (circuit breaker)",
			log.String("collection", collection), log.String("database", databaseName), log.Err(err))

		return nil, err
	}

	collectionResult, ok := queryResult.([]map[string]any)
	if !ok {
		return nil, pkgErr.FailedPreconditionError{Code: cnErr.ErrCodeUnexpectedCollectionResult.Error(), Title: "Unexpected Query Result", Message: fmt.Sprintf("unexpected query result type for collection %s in %s", collection, databaseName)}
	}

	if len(collectionFilters) > 0 {
		uc.Logger.Log(ctx, log.LevelDebug, "Successfully queried collection with advanced filters",
			log.String("collection", collection),
			log.String("circuit_breaker_state", uc.CircuitBreakerManager.GetState(databaseName)))
	} else {
		uc.Logger.Log(ctx, log.LevelDebug, "Successfully queried collection",
			log.String("collection", collection),
			log.String("circuit_breaker_state", uc.CircuitBreakerManager.GetState(databaseName)))
	}

	return collectionResult, nil
}

// getTableFilters extracts filters for a specific table/collection
// Supports multiple table name formats:
// - "schema__table" (Pongo2 format)
// - "schema.table" (qualified format)
// - "table" (simple format, will try with "public." prefix)
func getTableFilters(databaseFilters map[string]map[string]model.FilterCondition, tableName string) map[string]model.FilterCondition {
	if databaseFilters == nil {
		return nil
	}

	// Try exact match first
	if filters, ok := databaseFilters[tableName]; ok {
		return filters
	}

	// Try alternative formats
	var alternativeKeys []string

	if strings.Contains(tableName, "__") {
		// Pongo2 format: schema__table -> try schema.table
		alternativeKeys = append(alternativeKeys, strings.Replace(tableName, "__", ".", 1))
	} else if strings.Contains(tableName, ".") {
		// Qualified format: schema.table -> try schema__table
		alternativeKeys = append(alternativeKeys, strings.Replace(tableName, ".", "__", 1))
	} else {
		// Simple table name without schema -> try with public schema
		// This handles the case where template has "organization" but filter has "public.organization"
		alternativeKeys = append(alternativeKeys, "public."+tableName)
		alternativeKeys = append(alternativeKeys, "public__"+tableName)
	}

	for _, altKey := range alternativeKeys {
		if filters, ok := databaseFilters[altKey]; ok {
			return filters
		}
	}

	return nil
}

// transformPluginCRMAdvancedFilters transforms advanced FilterCondition filters for plugin_crm to use search fields
func (uc *UseCase) transformPluginCRMAdvancedFilters(filter map[string]model.FilterCondition) (map[string]model.FilterCondition, error) {
	if filter == nil {
		return nil, nil
	}

	if uc.CryptoHashSecretKeyPluginCRM == "" {
		return nil, pkgErr.FailedPreconditionError{Code: cnErr.ErrCodeCRMHashKeyNotConfigured.Error(), Title: "CRM Crypto Not Configured", Message: "CRYPTO_HASH_SECRET_KEY_PLUGIN_CRM not configured"}
	}

	crypto := &libCrypto.Crypto{
		HashSecretKey: uc.CryptoHashSecretKeyPluginCRM,
		Logger:        uc.Logger,
	}

	transformedFilter := make(map[string]model.FilterCondition)

	// Define field mappings: encrypted field -> search field
	fieldMappings := map[string]string{
		"document":                               "search.document",
		"name":                                   "search.name",
		"banking_details.account":                "search.banking_details_account",
		"banking_details.iban":                   "search.banking_details_iban",
		"contact.primary_email":                  "search.contact_primary_email",
		"contact.secondary_email":                "search.contact_secondary_email",
		"contact.mobile_phone":                   "search.contact_mobile_phone",
		"contact.other_phone":                    "search.contact_other_phone",
		"regulatory_fields.participant_document": "search.regulatory_fields_participant_document",
		"related_parties.document":               "search.related_party_documents",
	}

	for fieldName, condition := range filter {
		if searchField, exists := fieldMappings[fieldName]; exists {
			// Transform the condition by hashing string values
			transformedCondition := model.FilterCondition{}

			// Transform Equals values
			if len(condition.Equals) > 0 {
				transformedCondition.Equals = uc.hashFilterValues(condition.Equals, crypto)
			}

			// Transform GreaterThan values
			if len(condition.GreaterThan) > 0 {
				transformedCondition.GreaterThan = uc.hashFilterValues(condition.GreaterThan, crypto)
			}

			// Transform GreaterOrEqual values
			if len(condition.GreaterOrEqual) > 0 {
				transformedCondition.GreaterOrEqual = uc.hashFilterValues(condition.GreaterOrEqual, crypto)
			}

			// Transform LessThan values
			if len(condition.LessThan) > 0 {
				transformedCondition.LessThan = uc.hashFilterValues(condition.LessThan, crypto)
			}

			// Transform LessOrEqual values
			if len(condition.LessOrEqual) > 0 {
				transformedCondition.LessOrEqual = uc.hashFilterValues(condition.LessOrEqual, crypto)
			}

			// Transform Between values
			if len(condition.Between) > 0 {
				transformedCondition.Between = uc.hashFilterValues(condition.Between, crypto)
			}

			// Transform In values
			if len(condition.In) > 0 {
				transformedCondition.In = uc.hashFilterValues(condition.In, crypto)
			}

			// Transform NotIn values
			if len(condition.NotIn) > 0 {
				transformedCondition.NotIn = uc.hashFilterValues(condition.NotIn, crypto)
			}

			transformedFilter[searchField] = transformedCondition
			uc.Logger.Log(context.Background(), log.LevelDebug, "Transformed advanced filter",
				log.String("from", fieldName), log.String("to", searchField))
		} else {
			// Keep non-mapped fields as-is
			transformedFilter[fieldName] = condition
		}
	}

	return transformedFilter, nil
}

// hashFilterValues hashes string values in a filter condition array
func (uc *UseCase) hashFilterValues(values []any, crypto *libCrypto.Crypto) []any {
	hashedValues := make([]any, len(values))

	for i, value := range values {
		if strValue, ok := value.(string); ok && strValue != "" {
			hash := crypto.GenerateHash(&strValue)
			hashedValues[i] = hash
		} else {
			hashedValues[i] = value // Keep non-string values as-is
		}
	}

	return hashedValues
}

// decryptPluginCRMData decrypts sensitive fields for plugin_crm database
func (uc *UseCase) decryptPluginCRMData(collectionResult []map[string]any, fields []string) ([]map[string]any, error) {
	// Check if we need to decrypt any fields
	needsDecryption := false

	for _, field := range fields {
		// Check for top-level encrypted fields
		if isEncryptedField(field) {
			needsDecryption = true
			break
		}
		// Check for nested fields that might need decryption
		if strings.Contains(field, ".") {
			needsDecryption = true
			break
		}
	}

	if !needsDecryption {
		return collectionResult, nil
	}

	// Initialize crypto instance using centralized configuration
	if uc.CryptoEncryptSecretKeyPluginCRM == "" {
		return nil, pkgErr.FailedPreconditionError{Code: cnErr.ErrCodeCRMEncryptKeyNotConfigured.Error(), Title: "CRM Crypto Not Configured", Message: "CRYPTO_ENCRYPT_SECRET_KEY_PLUGIN_CRM not configured"}
	}

	if uc.CryptoHashSecretKeyPluginCRM == "" {
		return nil, pkgErr.FailedPreconditionError{Code: cnErr.ErrCodeCRMHashKeyNotConfigured.Error(), Title: "CRM Crypto Not Configured", Message: "CRYPTO_HASH_SECRET_KEY_PLUGIN_CRM not configured"}
	}

	crypto := &libCrypto.Crypto{
		HashSecretKey:    uc.CryptoHashSecretKeyPluginCRM,
		EncryptSecretKey: uc.CryptoEncryptSecretKeyPluginCRM,
		Logger:           uc.Logger,
	}

	err := crypto.InitializeCipher()
	if err != nil {
		return nil, pkgErr.FailedPreconditionError{Code: cnErr.ErrCodeCipherInitFailed.Error(), Title: "Cipher Initialization Failed", Message: fmt.Sprintf("failed to initialize cipher: %s", err.Error()), Err: err}
	}

	// Process each record in the collection
	for i, record := range collectionResult {
		decryptedRecord, err := uc.decryptRecord(record, crypto)
		if err != nil {
			return nil, pkgErr.FailedPreconditionError{Code: cnErr.ErrCodeRecordDecryptionFailed.Error(), Title: "Decryption Failed", Message: fmt.Sprintf("failed to decrypt record %d: %s", i, err.Error()), Err: err}
		}

		collectionResult[i] = decryptedRecord
	}

	return collectionResult, nil
}

// isEncryptedField checks if a field is known to be encrypted in plugin_crm
func isEncryptedField(field string) bool {
	encryptedFields := map[string]bool{
		"document": true,
		"name":     true,
	}

	return encryptedFields[field]
}

// decryptRecord decrypts a single record's encrypted fields
func (uc *UseCase) decryptRecord(record map[string]any, crypto *libCrypto.Crypto) (map[string]any, error) {
	// Create a copy of the record to avoid modifying the original
	decryptedRecord := make(map[string]any)
	for k, v := range record {
		decryptedRecord[k] = v
	}

	// Decrypt top-level fields
	if err := uc.decryptTopLevelFields(decryptedRecord, crypto); err != nil {
		return nil, err
	}

	// Decrypt nested fields
	if err := uc.decryptNestedFields(decryptedRecord, crypto); err != nil {
		return nil, err
	}

	return decryptedRecord, nil
}

// decryptTopLevelFields decrypts top-level encrypted fields
func (uc *UseCase) decryptTopLevelFields(record map[string]any, crypto *libCrypto.Crypto) error {
	for fieldName, fieldValue := range record {
		if isEncryptedField(fieldName) && fieldValue != nil {
			if err := uc.decryptFieldValue(record, fieldName, fieldValue, crypto); err != nil {
				return fmt.Errorf("failed to decrypt field %s: %w", fieldName, err)
			}
		}
	}

	return nil
}

// decryptNestedFields decrypts nested encrypted fields in the record
func (uc *UseCase) decryptNestedFields(record map[string]any, crypto *libCrypto.Crypto) error {
	if err := uc.decryptContactFields(record, crypto); err != nil {
		return err
	}

	if err := uc.decryptBankingDetailsFields(record, crypto); err != nil {
		return err
	}

	if err := uc.decryptLegalPersonFields(record, crypto); err != nil {
		return err
	}

	if err := uc.decryptNaturalPersonFields(record, crypto); err != nil {
		return err
	}

	if err := uc.decryptRegulatoryFieldsFields(record, crypto); err != nil {
		return err
	}

	if err := uc.decryptRelatedPartiesFields(record, crypto); err != nil {
		return err
	}

	return nil
}

// decryptContactFields decrypts fields within the contact object
func (uc *UseCase) decryptContactFields(record map[string]any, crypto *libCrypto.Crypto) error {
	contact, ok := record["contact"].(map[string]any)
	if !ok {
		return nil
	}

	contactFields := []string{"primary_email", "secondary_email", "mobile_phone", "other_phone"}
	for _, fieldName := range contactFields {
		if fieldValue, exists := contact[fieldName]; exists && fieldValue != nil {
			if err := uc.decryptFieldValue(contact, fieldName, fieldValue, crypto); err != nil {
				return fmt.Errorf("failed to decrypt contact.%s: %w", fieldName, err)
			}
		}
	}

	record["contact"] = contact

	return nil
}

// decryptBankingDetailsFields decrypts fields within the banking_details object
func (uc *UseCase) decryptBankingDetailsFields(record map[string]any, crypto *libCrypto.Crypto) error {
	bankingDetails, ok := record["banking_details"].(map[string]any)
	if !ok {
		return nil
	}

	bankingFields := []string{"account", "iban"}
	for _, fieldName := range bankingFields {
		if fieldValue, exists := bankingDetails[fieldName]; exists && fieldValue != nil {
			if err := uc.decryptFieldValue(bankingDetails, fieldName, fieldValue, crypto); err != nil {
				return fmt.Errorf("failed to decrypt banking_details.%s: %w", fieldName, err)
			}
		}
	}

	record["banking_details"] = bankingDetails

	return nil
}

// decryptLegalPersonFields decrypts fields within the legal_person object
func (uc *UseCase) decryptLegalPersonFields(record map[string]any, crypto *libCrypto.Crypto) error {
	legalPerson, ok := record["legal_person"].(map[string]any)
	if !ok {
		return nil
	}

	representative, ok := legalPerson["representative"].(map[string]any)
	if !ok {
		return nil
	}

	representativeFields := []string{"name", "document", "email"}
	for _, fieldName := range representativeFields {
		if fieldValue, exists := representative[fieldName]; exists && fieldValue != nil {
			if err := uc.decryptFieldValue(representative, fieldName, fieldValue, crypto); err != nil {
				return fmt.Errorf("failed to decrypt legal_person.representative.%s: %w", fieldName, err)
			}
		}
	}

	legalPerson["representative"] = representative
	record["legal_person"] = legalPerson

	return nil
}

// decryptNaturalPersonFields decrypts fields within the natural_person object
func (uc *UseCase) decryptNaturalPersonFields(record map[string]any, crypto *libCrypto.Crypto) error {
	naturalPerson, ok := record["natural_person"].(map[string]any)
	if !ok {
		return nil
	}

	naturalPersonFields := []string{"mother_name", "father_name"}
	for _, fieldName := range naturalPersonFields {
		if fieldValue, exists := naturalPerson[fieldName]; exists && fieldValue != nil {
			if err := uc.decryptFieldValue(naturalPerson, fieldName, fieldValue, crypto); err != nil {
				return fmt.Errorf("failed to decrypt natural_person.%s: %w", fieldName, err)
			}
		}
	}

	record["natural_person"] = naturalPerson

	return nil
}

// decryptRegulatoryFieldsFields decrypts fields within the regulatory_fields object
func (uc *UseCase) decryptRegulatoryFieldsFields(record map[string]any, crypto *libCrypto.Crypto) error {
	regulatoryFields, ok := record["regulatory_fields"].(map[string]any)
	if !ok {
		return nil
	}

	regulatoryFieldNames := []string{"participant_document"}
	for _, fieldName := range regulatoryFieldNames {
		if fieldValue, exists := regulatoryFields[fieldName]; exists && fieldValue != nil {
			if err := uc.decryptFieldValue(regulatoryFields, fieldName, fieldValue, crypto); err != nil {
				return fmt.Errorf("failed to decrypt regulatory_fields.%s: %w", fieldName, err)
			}
		}
	}

	record["regulatory_fields"] = regulatoryFields

	return nil
}

// decryptRelatedPartiesFields decrypts the document field within each related_parties array item
func (uc *UseCase) decryptRelatedPartiesFields(record map[string]any, crypto *libCrypto.Crypto) error {
	relatedParties, ok := record["related_parties"].([]any)
	if !ok {
		return nil
	}

	for i, party := range relatedParties {
		partyMap, ok := party.(map[string]any)
		if !ok {
			continue
		}

		if fieldValue, exists := partyMap["document"]; exists && fieldValue != nil {
			if err := uc.decryptFieldValue(partyMap, "document", fieldValue, crypto); err != nil {
				return fmt.Errorf("failed to decrypt related_parties[%d].document: %w", i, err)
			}
		}

		relatedParties[i] = partyMap
	}

	record["related_parties"] = relatedParties

	return nil
}

// decryptFieldValue decrypts a single field value if it's a non-empty string
func (uc *UseCase) decryptFieldValue(container map[string]any, fieldName string, fieldValue any, crypto *libCrypto.Crypto) error {
	strValue, ok := fieldValue.(string)
	if !ok || strValue == "" {
		return nil
	}

	decryptedValue, err := crypto.Decrypt(&strValue)
	if err != nil {
		return err
	}

	container[fieldName] = *decryptedValue

	return nil
}
