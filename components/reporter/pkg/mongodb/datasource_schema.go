// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mongodb

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/LerianStudio/reporter/pkg/constant"
	"github.com/LerianStudio/reporter/pkg/ctxutil"

	"github.com/LerianStudio/lib-observability/log"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.opentelemetry.io/otel/attribute"
)

// GetDatabaseSchema retrieves all collections and infers their schema from sample documents.
func (ds *ExternalDataSource) GetDatabaseSchema(ctx context.Context) ([]CollectionSchema, error) {
	logger := ds.connection.Logger
	tracer := ctxutil.NewTracerFromContext(ctx)
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.datasource.get_database_schema")
	defer span.End()

	span.SetAttributes(attribute.String("app.request.request_id", reqID))
	logger.Log(ctx, log.LevelInfo, "Retrieving MongoDB schema information using hybrid approach")

	schemaCtx, cancel := context.WithTimeout(ctx, constant.SchemaDiscoveryTimeout)
	defer cancel()

	client, err := ds.connection.GetDB(ctx)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("mongodb schema discovery timeout while getting database: %w", err)
		}

		return nil, err
	}

	database := client.Database(ds.Database)

	collections, err := database.ListCollectionNames(schemaCtx, bson.M{})
	if err != nil {
		if schemaCtx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("mongodb schema discovery timeout after %v while listing collections: %w", constant.SchemaDiscoveryTimeout, err)
		}

		return nil, err
	}

	schema := make([]CollectionSchema, 0, len(collections))
	for _, collName := range collections {
		coll := database.Collection(collName)
		logger.Log(ctx, log.LevelInfo, "Analyzing collection", log.String("collection", collName))

		allFields, err := ds.discoverCollectionFields(schemaCtx, coll)
		if err != nil {
			return nil, err
		}

		fieldTypes, additionalFields, err := ds.sampleMultipleDocuments(schemaCtx, coll)
		if err != nil {
			logger.Log(ctx, log.LevelWarn, "Document sampling failed for collection", log.String("collection", collName), log.Err(err))

			fieldTypes = make(map[string]string)
			additionalFields = make(map[string]bool)
		}

		for field := range additionalFields {
			allFields[field] = true
		}

		collSchema := CollectionSchema{CollectionName: collName, Fields: []FieldInformation{}}

		for fieldName := range allFields {
			dataType := fieldTypes[fieldName]
			if dataType == "" {
				dataType = unknownDataType
			}

			collSchema.Fields = append(collSchema.Fields, FieldInformation{Name: fieldName, DataType: dataType})
		}

		logger.Log(ctx, log.LevelInfo, "Discovered fields in collection", log.Int("field_count", len(collSchema.Fields)), log.String("collection", collName))
		schema = append(schema, collSchema)
	}

	logger.Log(ctx, log.LevelInfo, "Retrieved schema for collections", log.Int("collection_count", len(schema)))

	return schema, nil
}

// GetDatabaseSchemaForOrganization retrieves collections filtered by organization ID suffix.
func (ds *ExternalDataSource) GetDatabaseSchemaForOrganization(ctx context.Context, organizationID string) ([]CollectionSchema, error) {
	logger := ds.connection.Logger
	tracer := ctxutil.NewTracerFromContext(ctx)
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.datasource.get_database_schema_for_organization")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.organization_id", organizationID),
	)
	logger.Log(ctx, log.LevelInfo, "Retrieving MongoDB schema for organization", log.String("organization_id", organizationID))

	schemaCtx, cancel := context.WithTimeout(ctx, constant.SchemaDiscoveryTimeout)
	defer cancel()

	client, err := ds.connection.GetDB(ctx)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("mongodb schema discovery timeout while getting database: %w", err)
		}

		return nil, err
	}

	database := client.Database(ds.Database)
	filter := bson.M{"name": bson.M{"$regex": "_" + regexp.QuoteMeta(organizationID) + "$"}}

	collections, err := database.ListCollectionNames(schemaCtx, filter)
	if err != nil {
		if schemaCtx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("mongodb schema discovery timeout after %v while listing collections: %w", constant.SchemaDiscoveryTimeout, err)
		}

		return nil, err
	}

	logger.Log(ctx, log.LevelInfo, "Found collections for organization", log.Int("collection_count", len(collections)), log.String("organization_id", organizationID))

	schema := make([]CollectionSchema, 0, len(collections))
	for _, collName := range collections {
		coll := database.Collection(collName)
		logger.Log(ctx, log.LevelInfo, "Analyzing collection", log.String("collection", collName))

		allFields, err := ds.discoverCollectionFields(schemaCtx, coll)
		if err != nil {
			return nil, err
		}

		fieldTypes, additionalFields, err := ds.sampleMultipleDocuments(schemaCtx, coll)
		if err != nil {
			logger.Log(ctx, log.LevelWarn, "Document sampling failed for collection", log.String("collection", collName), log.Err(err))

			fieldTypes = make(map[string]string)
			additionalFields = make(map[string]bool)
		}

		for field := range additionalFields {
			allFields[field] = true
		}

		collSchema := CollectionSchema{CollectionName: collName, Fields: []FieldInformation{}}

		for fieldName := range allFields {
			dataType := fieldTypes[fieldName]
			if dataType == "" {
				dataType = unknownDataType
			}

			collSchema.Fields = append(collSchema.Fields, FieldInformation{Name: fieldName, DataType: dataType})
		}

		logger.Log(ctx, log.LevelInfo, "Discovered fields in collection", log.Int("field_count", len(collSchema.Fields)), log.String("collection", collName))
		schema = append(schema, collSchema)
	}

	logger.Log(ctx, log.LevelInfo, "Retrieved schema for collections for organization", log.Int("collection_count", len(schema)), log.String("organization_id", organizationID))

	return schema, nil
}

// GetDatabaseSchemaForPluginCRM groups org-scoped collections by prefix (e.g. holders_*)
// and returns a union schema per logical name.
func (ds *ExternalDataSource) GetDatabaseSchemaForPluginCRM(ctx context.Context) ([]CollectionSchema, error) {
	logger := ds.connection.Logger
	tracer := ctxutil.NewTracerFromContext(ctx)
	reqID := ctxutil.HeaderIDFromContext(ctx)

	ctx, span := tracer.Start(ctx, "repository.datasource.get_database_schema_for_plugin_crm")
	defer span.End()

	span.SetAttributes(attribute.String("app.request.request_id", reqID))
	logger.Log(ctx, log.LevelInfo, "Discovering plugin_crm schema via prefix-based collection grouping")

	schemaCtx, cancel := context.WithTimeout(ctx, constant.SchemaDiscoveryTimeout)
	defer cancel()

	client, err := ds.connection.GetDB(ctx)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("mongodb plugin_crm schema discovery timeout while getting database: %w", err)
		}

		return nil, err
	}

	database := client.Database(ds.Database)

	allCollections, err := database.ListCollectionNames(schemaCtx, bson.M{})
	if err != nil {
		if schemaCtx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("mongodb plugin_crm schema discovery timeout after %v while listing collections: %w", constant.SchemaDiscoveryTimeout, err)
		}

		return nil, fmt.Errorf("failed to list collections for plugin_crm schema: %w", err)
	}

	logicalGroups := groupCollectionsByPrefix(allCollections)

	logger.Log(ctx, log.LevelInfo, "Grouped plugin_crm collections by prefix",
		log.Int("logical_groups", len(logicalGroups)),
		log.Int("total_physical", len(allCollections)),
	)

	schema := make([]CollectionSchema, 0, len(logicalGroups))

	for logicalName, physicalCollections := range logicalGroups {
		unionFields := ds.sampleAndUnionFields(schemaCtx, database, physicalCollections, logger)

		if len(unionFields) == 0 {
			logger.Log(ctx, log.LevelWarn, "Skipping logical collection with no discovered fields (all sampling failed)",
				log.String("logical_name", logicalName),
			)

			continue
		}

		collSchema := CollectionSchema{
			CollectionName: logicalName,
			Fields:         make([]FieldInformation, 0, len(unionFields)),
		}

		for fieldName, dataType := range unionFields {
			collSchema.Fields = append(collSchema.Fields, FieldInformation{Name: fieldName, DataType: dataType})
		}

		logger.Log(ctx, log.LevelInfo, "Union schema for logical collection",
			log.String("logical_name", logicalName),
			log.Int("field_count", len(collSchema.Fields)),
			log.Int("orgs_sampled", len(physicalCollections)),
		)

		schema = append(schema, collSchema)
	}

	return schema, nil
}

// groupCollectionsByPrefix groups physical collection names by their logical prefix.
// "holders_orgA", "holders_orgB" → {"holders": ["holders_orgA", "holders_orgB"]}.
// Collections without an underscore (e.g. "system.indexes") are skipped.
func groupCollectionsByPrefix(collections []string) map[string][]string {
	groups := make(map[string][]string)

	for _, coll := range collections {
		idx := strings.Index(coll, "_")
		if idx <= 0 {
			continue
		}

		logicalName := coll[:idx]
		groups[logicalName] = append(groups[logicalName], coll)
	}

	return groups
}

// maxSampleCollections limits how many physical collections are sampled per logical group.
const maxSampleCollections = 5

// sampleAndUnionFields samples up to maxSampleCollections physical collections and
// returns a union of all discovered fields (fieldName → dataType).
func (ds *ExternalDataSource) sampleAndUnionFields(
	ctx context.Context,
	database *mongo.Database,
	physicalCollections []string,
	logger log.Logger,
) map[string]string {
	unionFields := make(map[string]string)

	// Sort for deterministic sampling — ListCollectionNames order is not guaranteed.
	sorted := make([]string, len(physicalCollections))
	copy(sorted, physicalCollections)
	sort.Strings(sorted)

	sampled := sorted
	if len(sampled) > maxSampleCollections {
		sampled = sampled[:maxSampleCollections]
	}

	for _, physColl := range sampled {
		coll := database.Collection(physColl)

		fieldTypes, additionalFields, err := ds.sampleMultipleDocuments(ctx, coll)
		if err != nil {
			logger.Log(ctx, log.LevelWarn, "Sampling failed for collection, skipping",
				log.String("collection", physColl), log.Err(err))

			continue
		}

		for field, dtype := range fieldTypes {
			if _, exists := unionFields[field]; !exists {
				unionFields[field] = dtype
			}
		}

		for field := range additionalFields {
			if _, exists := unionFields[field]; !exists {
				unionFields[field] = unknownDataType
			}
		}
	}

	return unionFields
}
