// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	pkgErr "github.com/LerianStudio/midaz/v4/pkg"

	cnErr "github.com/LerianStudio/midaz/v4/pkg/constant"
	pkg "github.com/LerianStudio/midaz/v4/pkg/reporter"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/constant"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/ctxutil"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/datasource"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/model"
	"github.com/LerianStudio/midaz/v4/pkg/reporter/mongodb"

	libConstants "github.com/LerianStudio/lib-commons/v5/commons/constants"
	"github.com/LerianStudio/lib-observability/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"go.opentelemetry.io/otel/attribute"
)

const pluginCRMDataSourceID = "plugin_crm"

var (
	// Define encrypted fields that should be excluded
	encryptedFields = map[string]bool{
		"document": true,
		"name":     true,
	}

	// Define search fields that should be included (these are hashes, not encrypted)
	searchFields = map[string]bool{
		"search.document":                true,
		"search.banking_details_account": true,
		"search.banking_details_iban":    true,
		"search.contact_primary_email":   true,
		"search.contact_secondary_email": true,
		"search.contact_mobile_phone":    true,
		"search.contact_other_phone":     true,
		"search":                         true, // Include the search object itself
	}

	// Define nested encrypted fields that should be excluded
	nestedEncryptedFields = map[string]bool{
		"contact.primary_email":                  true,
		"contact.secondary_email":                true,
		"contact.mobile_phone":                   true,
		"contact.other_phone":                    true,
		"banking_details.account":                true,
		"banking_details.iban":                   true,
		"legal_person.representative.name":       true,
		"legal_person.representative.document":   true,
		"legal_person.representative.email":      true,
		"natural_person.mother_name":             true,
		"natural_person.father_name":             true,
		"regulatory_fields.participant_document": true,
		"related_parties.document":               true,
		"related_parties.name":                   true,
	}
)

// GetDataSourceDetailsByID retrieves the data source information by data source id.
// When a DataSourceProvider is set, it delegates to provider.GetDataSourceSchema()
// and maps the result to the existing model.DataSourceDetails format. Otherwise,
// falls back to legacy direct repository access.
func (uc *UseCase) GetDataSourceDetailsByID(ctx context.Context, dataSourceID string) (_ *model.DataSourceDetails, err error) {
	start := time.Now()

	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := uc.Tracer.Start(ctx, "service.data_source.get_details_by_id")
	defer span.End()
	defer func() { uc.recordDomainOp(ctx, opGetDataSourceDetails, start, err) }()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.data_source_id", dataSourceID),
	)
	uc.Logger.Log(ctx, log.LevelDebug, "Retrieving data source details", log.String("data_source_id", dataSourceID))

	cacheKey := constant.DataSourceDetailsKeyPrefix + ":" + dataSourceID
	if cached, ok := uc.getDataSourceDetailsFromCache(ctx, cacheKey); ok {
		uc.Logger.Log(ctx, log.LevelDebug, "Cache hit for data source details", log.String("data_source_id", dataSourceID))
		return cached, nil
	}

	if uc.DataSourceProvider != nil {
		return uc.getDataSourceDetailsByIDFromProvider(ctx, dataSourceID, cacheKey)
	}

	return uc.getDataSourceDetailsByIDLegacy(ctx, dataSourceID, cacheKey)
}

// getDataSourceDetailsByIDFromProvider delegates schema retrieval to the
// DataSourceProvider interface and maps the result to model.DataSourceDetails.
func (uc *UseCase) getDataSourceDetailsByIDFromProvider(ctx context.Context, dataSourceID, cacheKey string) (*model.DataSourceDetails, error) {
	ctx, span := uc.Tracer.Start(ctx, "service.data_source.get_details_by_id_provider")
	defer span.End()

	span.SetAttributes(attribute.String("app.request.data_source_id", dataSourceID))

	uc.Logger.Log(ctx, log.LevelDebug, "Retrieving data source schema via DataSourceProvider",
		log.String("data_source_id", dataSourceID))

	schema, err := uc.DataSourceProvider.GetDataSourceSchema(ctx, dataSourceID)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to retrieve schema from provider", err)
		uc.Logger.Log(ctx, log.LevelError, "Error retrieving schema from DataSourceProvider",
			log.String("data_source_id", dataSourceID), log.Err(err))

		return nil, err
	}

	if schema == nil {
		return nil, fmt.Errorf("provider returned nil schema for data source %s", dataSourceID)
	}

	result := mapSchemaToDataSourceDetails(dataSourceID, schema)

	errSet := uc.setDataSourceDetailsToCache(ctx, cacheKey, result)
	if errSet != nil {
		uc.Logger.Log(ctx, log.LevelError, "Error to set data source details to cache", log.Err(errSet))
		return nil, errSet
	}

	return result, nil
}

// mapSchemaToDataSourceDetails converts the provider's DataSourceSchema to the
// existing model.DataSourceDetails format, preserving API backward compatibility.
func mapSchemaToDataSourceDetails(dataSourceID string, schema *datasource.DataSourceSchema) *model.DataSourceDetails {
	tables := make([]model.TableDetails, 0, len(schema.Tables))

	for _, table := range schema.Tables {
		fields := make([]string, 0, len(table.Fields))
		for _, field := range table.Fields {
			fields = append(fields, field.Name)
		}

		tables = append(tables, model.TableDetails{
			Name:   table.Name,
			Fields: fields,
		})
	}

	// Derive type from the schema tables (provider may not expose type directly).
	// We use the dataSourceID to look up type from the provider's ListDataSources
	// result, but for simplicity we set it from what the schema implies.
	dsType := deriveDataSourceType(schema)

	return &model.DataSourceDetails{
		Id:     dataSourceID,
		Type:   dsType,
		Tables: tables,
	}
}

// deriveDataSourceType infers the data source type from schema structure.
// If tables have a Schema field set, it's PostgreSQL; otherwise MongoDB.
func deriveDataSourceType(schema *datasource.DataSourceSchema) string {
	for _, table := range schema.Tables {
		if table.Schema != "" {
			return pkg.PostgreSQLType
		}
	}

	if len(schema.Tables) > 0 {
		return pkg.MongoDBType
	}

	return ""
}

// getDataSourceDetailsByIDLegacy is the original implementation that accesses
// ExternalDataSources directly. Retained for backward compatibility when no
// DataSourceProvider is configured.
func (uc *UseCase) getDataSourceDetailsByIDLegacy(ctx context.Context, dataSourceID, cacheKey string) (*model.DataSourceDetails, error) {
	if !pkg.IsValidDataSourceID(dataSourceID) {
		uc.Logger.Log(ctx, log.LevelError, "Unknown data source - not in immutable registry, rejecting request", log.String("data_source_id", dataSourceID))
		return nil, pkgErr.ValidateBusinessError(cnErr.ErrMissingDataSource, "", dataSourceID)
	}

	dataSource, ok := uc.ExternalDataSources.Get(dataSourceID)
	if !ok {
		return nil, pkgErr.ValidateBusinessError(cnErr.ErrMissingDataSource, "", dataSourceID)
	}

	if err := uc.ensureDataSourceConnected(ctx, uc.Logger, dataSourceID, &dataSource); err != nil {
		uc.Logger.Log(ctx, log.LevelError, "Error initializing database connection", log.String("data_source_id", dataSourceID), log.Err(err))
		return nil, err
	}

	var (
		result           *model.DataSourceDetails
		errGetDataSource error
	)

	switch dataSource.DatabaseType {
	case pkg.PostgreSQLType:
		result, errGetDataSource = uc.getDataSourceDetailsOfPostgresDatabase(ctx, uc.Logger, dataSourceID, dataSource)
	case pkg.MongoDBType:
		result, errGetDataSource = uc.getDataSourceDetailsOfMongoDBDatabase(ctx, uc.Logger, dataSourceID, dataSource)
	default:
		return nil, pkgErr.ValidateBusinessError(cnErr.ErrMissingDataSource, "", dataSourceID)
	}

	if errGetDataSource != nil {
		uc.Logger.Log(ctx, log.LevelError, "Error to get data source details", log.Err(errGetDataSource))
		return nil, pkgErr.ValidateBusinessError(cnErr.ErrMissingDataSource, "", dataSourceID)
	}

	errSet := uc.setDataSourceDetailsToCache(ctx, cacheKey, result)
	if errSet != nil {
		uc.Logger.Log(ctx, log.LevelError, "Error to set data source details to cache", log.Err(errSet))
		return nil, errSet
	}

	return result, nil
}

// getDataSourceDetailsFromCache tries to get and unmarshal DataSourceDetails from Redis
func (uc *UseCase) getDataSourceDetailsFromCache(ctx context.Context, cacheKey string) (*model.DataSourceDetails, bool) {
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := uc.Tracer.Start(ctx, "service.data_source.get_cache")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.cache.key", cacheKey),
	)

	if uc.RedisRepo == nil {
		return nil, false
	}

	cached, err := uc.RedisRepo.Get(ctx, cacheKey)
	if err != nil || cached == "" {
		return nil, false
	}

	var details model.DataSourceDetails
	if err := json.Unmarshal([]byte(cached), &details); err != nil {
		return nil, false
	}

	return &details, true
}

// setDataSourceDetailsToCache marshals and sets DataSourceDetails in Redis
func (uc *UseCase) setDataSourceDetailsToCache(ctx context.Context, cacheKey string, details *model.DataSourceDetails) error {
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := uc.Tracer.Start(ctx, "service.data_source.set_cache")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.cache.key", cacheKey),
	)

	if uc.RedisRepo == nil || details == nil {
		return nil
	}

	if marshaled, err := json.Marshal(details); err == nil {
		if errCache := uc.RedisRepo.Set(ctx, cacheKey, string(marshaled), time.Second*constant.RedisTTL); errCache != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to set data source details to cache", errCache)

			return errCache
		}
	}

	return nil
}

// ensureDataSourceConnected ensures the data source is initialized/connected.
func (uc *UseCase) ensureDataSourceConnected(ctx context.Context, logger log.Logger, dataSourceID string, dataSource *pkg.DataSource) error {
	// Check if datasource is marked as unavailable
	if dataSource.Status == libConstants.DataSourceStatusUnavailable {
		logger.Log(ctx, log.LevelWarn, "Datasource is marked as unavailable - attempting to connect anyway", log.String("data_source_id", dataSourceID))
	}

	switch dataSource.DatabaseType {
	case pkg.PostgreSQLType:
		if !dataSource.Initialized || !dataSource.DatabaseConfig.Connected {
			logger.Log(ctx, log.LevelDebug, "Connecting to PostgreSQL datasource on-demand...", log.String("data_source_id", dataSourceID))
			return uc.ExternalDataSources.ConnectDataSource(ctx, dataSourceID, dataSource, logger)
		}
	case pkg.MongoDBType:
		if !dataSource.Initialized {
			logger.Log(ctx, log.LevelDebug, "Connecting to MongoDB datasource on-demand...", log.String("data_source_id", dataSourceID))
			return uc.ExternalDataSources.ConnectDataSource(ctx, dataSourceID, dataSource, logger)
		}
	}

	return nil
}

// getDataSourceDetailsOfMongoDBDatabase retrieves the data source information of a MongoDB database
func (uc *UseCase) getDataSourceDetailsOfMongoDBDatabase(ctx context.Context, logger log.Logger, dataSourceID string, dataSource pkg.DataSource) (*model.DataSourceDetails, error) {
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := uc.Tracer.Start(ctx, "service.data_source.get_details_mongodb")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.data_source_id", dataSourceID),
	)

	var (
		schema []mongodb.CollectionSchema
		err    error
	)

	// If MidazOrganizationID is configured (e.g., for plugin_crm), fetch only collections for that organization
	if dataSource.MidazOrganizationID != "" {
		logger.Log(ctx, log.LevelDebug, "Fetching schema for Midaz organization",
			log.String("organization_id", dataSource.MidazOrganizationID),
			log.String("data_source_id", dataSourceID),
		)
		schema, err = dataSource.MongoDBRepository.GetDatabaseSchemaForOrganization(ctx, dataSource.MidazOrganizationID)
	} else {
		schema, err = dataSource.MongoDBRepository.GetDatabaseSchema(ctx)
	}

	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get MongoDB schema", err)
		logger.Log(ctx, log.LevelError, "Error get schemas of mongo db", log.Err(err))

		return nil, err
	}

	tableDetails := uc.processCollectionsForDataSource(schema, dataSourceID)

	result := &model.DataSourceDetails{
		Id:           dataSourceID,
		ExternalName: dataSource.MongoDBName,
		Type:         dataSource.DatabaseType,
		Tables:       tableDetails,
	}

	return result, nil
}

// processCollectionsForDataSource processes collections and returns table details
func (uc *UseCase) processCollectionsForDataSource(schema []mongodb.CollectionSchema, dataSourceID string) []model.TableDetails {
	tableDetails := make([]model.TableDetails, 0, len(schema))

	for _, collection := range schema {
		fields := uc.getFieldsForCollection(collection, dataSourceID)
		displayName := uc.getDisplayNameForCollection(collection.CollectionName, dataSourceID)

		tableSchema := model.TableDetails{
			Name:   displayName,
			Fields: fields,
		}

		tableDetails = append(tableDetails, tableSchema)
	}

	return tableDetails
}

// getFieldsForCollection determines which fields to include for a collection
func (uc *UseCase) getFieldsForCollection(collection mongodb.CollectionSchema, dataSourceID string) []string {
	if dataSourceID == pluginCRMDataSourceID {
		return uc.getFieldsForPluginCRM(collection)
	}

	// For other databases, include all fields
	fields := make([]string, 0, len(collection.Fields))
	for _, collectionField := range collection.Fields {
		fields = append(fields, collectionField.Name)
	}

	return fields
}

// getFieldsForPluginCRM gets fields for plugin_crm collections with special handling
func (uc *UseCase) getFieldsForPluginCRM(collection mongodb.CollectionSchema) []string {
	baseCollectionName := uc.getBaseCollectionName(collection.CollectionName)

	expandedFields := uc.getExpandedFieldsForPluginCRM(baseCollectionName)
	if expandedFields != nil {
		return expandedFields
	}

	// Fallback to filtering raw schema fields
	fields := make([]string, 0)

	for _, collectionField := range collection.Fields {
		if uc.shouldIncludeFieldForPluginCRM(collectionField.Name, baseCollectionName) {
			fields = append(fields, collectionField.Name)
		}
	}

	return fields
}

// getBaseCollectionName extracts the base collection name by removing organization suffix
func (uc *UseCase) getBaseCollectionName(collectionName string) string {
	if !strings.Contains(collectionName, "_") {
		return collectionName
	}

	parts := strings.Split(collectionName, "_")
	if len(parts) > 1 {
		return strings.Join(parts[:len(parts)-1], "_")
	}

	return collectionName
}

// getDisplayNameForCollection gets the display name for a collection
func (uc *UseCase) getDisplayNameForCollection(collectionName, dataSourceID string) string {
	if dataSourceID == pluginCRMDataSourceID {
		return uc.getBaseCollectionName(collectionName)
	}

	return collectionName
}

// shouldIncludeFieldForPluginCRM determines if a field should be included for plugin_crm based on encryption status
func (uc *UseCase) shouldIncludeFieldForPluginCRM(fieldName, collectionName string) bool {
	// Check if it's a search field (include these)
	if searchFields[fieldName] {
		return true
	}

	// Check if it's a top-level encrypted field (exclude these)
	if encryptedFields[fieldName] {
		return false
	}

	// Check if it's a nested encrypted field (exclude these)
	if nestedEncryptedFields[fieldName] {
		return false
	}

	// For holders and aliases collections, be more specific about what to include
	if collectionName == "holders" || collectionName == "aliases" {
		// Include all non-encrypted fields
		return true
	}

	// For other collections, include all fields
	return true
}

// getExpandedFieldsForPluginCRM returns the expanded field list for plugin_crm collections
func (uc *UseCase) getExpandedFieldsForPluginCRM(collectionName string) []string {
	switch collectionName {
	case "holders":
		return []string{
			"_id",
			"external_id",
			"type",
			"addresses",
			"created_at",
			"updated_at",
			"deleted_at",
			"metadata",
			"search.document",
			// Natural person fields (non-encrypted)
			"natural_person.favorite_name",
			"natural_person.social_name",
			"natural_person.gender",
			"natural_person.birth_date",
			"natural_person.civil_status",
			"natural_person.nationality",
			"natural_person.status",
			// Legal person fields (non-encrypted)
			"legal_person.trade_name",
			"legal_person.activity",
			"legal_person.type",
			"legal_person.founding_date",
			"legal_person.size",
			"legal_person.status",
			"legal_person.representative.role",
		}
	case "aliases":
		return []string{
			"_id",
			"account_id",
			"holder_id",
			"ledger_id",
			"type",
			"created_at",
			"updated_at",
			"deleted_at",
			"metadata",
			// Search fields (hashes, not encrypted)
			"search.document",
			"search.banking_details_account",
			"search.banking_details_iban",
			"search.regulatory_fields_participant_document",
			"search.related_party_documents",
			// Banking details fields (non-encrypted)
			"banking_details.branch",
			"banking_details.type",
			"banking_details.opening_date",
			"banking_details.closing_date",
			"banking_details.country_code",
			"banking_details.bank_id",
			// Regulatory fields (non-encrypted)
			"regulatory_fields",
			// Related parties fields (non-encrypted)
			"related_parties",
			"related_parties._id",
			"related_parties.role",
			"related_parties.start_date",
			"related_parties.end_date",
		}
	default:
		return nil
	}
}

// getDataSourceDetailsOfPostgresDatabase retrieves the data source information of a PostgresSQL database
func (uc *UseCase) getDataSourceDetailsOfPostgresDatabase(ctx context.Context, logger log.Logger, dataSourceID string, dataSource pkg.DataSource) (*model.DataSourceDetails, error) {
	reqId := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := uc.Tracer.Start(ctx, "service.data_source.get_details_postgres")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqId),
		attribute.String("app.request.data_source_id", dataSourceID),
	)

	// Use configured schemas or default to public
	configuredSchemas := dataSource.Schemas
	if len(configuredSchemas) == 0 {
		configuredSchemas = []string{"public"}
	}

	schemas, err := dataSource.PostgresRepository.GetDatabaseSchema(ctx, configuredSchemas)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get PostgreSQL schema", err)
		logger.Log(ctx, log.LevelError, "Error get schemas of postgres", log.Err(err))

		return nil, err
	}

	tableDetails := make([]model.TableDetails, 0)

	for _, tableSchema := range schemas {
		fields := make([]string, 0)
		for _, field := range tableSchema.Columns {
			fields = append(fields, field.Name)
		}

		tableDetail := model.TableDetails{
			Name:   tableSchema.QualifiedName(), // Returns "schema.table" format
			Fields: fields,
		}

		tableDetails = append(tableDetails, tableDetail)
	}

	result := &model.DataSourceDetails{
		Id:           dataSourceID,
		ExternalName: dataSource.MongoDBName,
		Type:         dataSource.DatabaseType,
		Tables:       tableDetails,
	}

	return result, nil
}
