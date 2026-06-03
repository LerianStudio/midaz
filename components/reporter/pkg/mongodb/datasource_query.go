// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mongodb

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/constant"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/ctxutil"
	"github.com/LerianStudio/midaz/v3/components/reporter/pkg/model"

	"github.com/LerianStudio/lib-observability/log"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.opentelemetry.io/otel/attribute"
)

// Query executes a query on the specified collection with the given fields and filter criteria.
func (ds *ExternalDataSource) Query(ctx context.Context, collection string, fields []string, filter map[string][]any) ([]map[string]any, error) {
	logger := ds.connection.Logger
	tracer := ctxutil.NewTracerFromContext(ctx)
	reqID := ctxutil.HeaderIDFromContext(ctx)
	logger.Log(ctx, log.LevelInfo, "Querying MongoDB collection",
		log.String("collection", collection),
		log.Int("field_count", len(fields)),
		log.Int("filter_count", len(filter)),
	)

	ctx, span := tracer.Start(ctx, "repository.datasource.query")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.collection", collection),
		attribute.Int("app.request.field_count", len(fields)),
		attribute.Int("app.request.filter_count", len(filter)),
	)

	client, err := ds.connection.GetDB(ctx)
	if err != nil {
		return nil, err
	}

	validatedFields, err := ds.validateCollectionAndFields(ctx, client, collection, fields, filterFieldNames(filter))
	if err != nil {
		return nil, err
	}

	mongoFilter := buildSimpleMongoFilter(filter)
	findOptions := ds.buildFindOptions(validatedFields)

	queryCtx, cancel := context.WithTimeout(ctx, constant.QueryTimeoutMedium)
	defer cancel()

	database := client.Database(ds.Database)

	cursor, err := database.Collection(collection).Find(queryCtx, mongoFilter, findOptions)
	if err != nil {
		return nil, wrapQueryError(queryCtx, constant.QueryTimeoutMedium, collection, "mongodb query timeout after %v for collection %s: %w", err)
	}
	defer cursor.Close(queryCtx)

	results := decodeCursorResults(queryCtx, cursor, logger)
	if err := cursor.Err(); err != nil {
		return nil, wrapQueryError(queryCtx, constant.QueryTimeoutMedium, collection, "mongodb query result iteration timeout after %v for collection %s: %w", err)
	}

	return results, nil
}

func buildSimpleMongoFilter(filter map[string][]any) bson.M {
	mongoFilter := bson.M{}

	for key, values := range filter {
		if len(values) == 1 {
			mongoFilter[key] = values[0]
		} else if len(values) > 1 {
			mongoFilter[key] = bson.M{"$in": values}
		}
	}

	return mongoFilter
}

func decodeCursorResults(ctx context.Context, cursor *mongo.Cursor, logger log.Logger) []map[string]any {
	var results []map[string]any

	for cursor.Next(ctx) {
		var result bson.M
		if err := cursor.Decode(&result); err != nil {
			logger.Log(ctx, log.LevelWarn, "Error decoding document", log.Err(err))
			continue
		}

		results = append(results, convertBsonToMap(result))
	}

	return results
}

func wrapQueryError(queryCtx context.Context, timeout time.Duration, collection string, timeoutMsg string, err error) error {
	if queryCtx.Err() == context.DeadlineExceeded {
		return fmt.Errorf(timeoutMsg, timeout, collection, err)
	}

	return err
}

// QueryWithAdvancedFilters executes a query with advanced FilterCondition support.
func (ds *ExternalDataSource) QueryWithAdvancedFilters(ctx context.Context, collection string, fields []string, filter map[string]model.FilterCondition) ([]map[string]any, error) {
	logger := ds.connection.Logger
	tracer := ctxutil.NewTracerFromContext(ctx)
	reqID := ctxutil.HeaderIDFromContext(ctx)
	logger.Log(ctx, log.LevelInfo, "Querying collection with advanced filters", log.String("collection", collection), log.Any("fields", fields))

	ctx, span := tracer.Start(ctx, "repository.datasource.query_with_advanced_filters")
	defer span.End()

	span.SetAttributes(
		attribute.String("app.request.request_id", reqID),
		attribute.String("app.request.collection", collection),
		attribute.Int("app.request.field_count", len(fields)),
		attribute.Int("app.request.filter_count", len(filter)),
	)

	client, err := ds.connection.GetDB(ctx)
	if err != nil {
		return nil, err
	}

	validatedFields, err := ds.validateCollectionAndFields(ctx, client, collection, fields, advancedFilterFieldNames(filter))
	if err != nil {
		return nil, err
	}

	mongoFilter, err := ds.buildMongoFilter(filter)
	if err != nil {
		return nil, err
	}

	findOptions := ds.buildFindOptions(validatedFields)

	cursor, queryCtx, cancel, err := ds.executeFindQuery(ctx, client, collection, mongoFilter, findOptions)
	if err != nil {
		return nil, err
	}
	defer cancel()
	defer cursor.Close(queryCtx)

	return ds.processQueryResults(queryCtx, cursor, collection, logger)
}

// buildFindOptions creates MongoDB find options with field projection.
func (ds *ExternalDataSource) buildFindOptions(fields []string) *options.FindOptionsBuilder {
	projection := bson.M{}

	if len(fields) > 0 && fields[0] != "*" {
		filteredFields := FilterNestedFields(fields)
		for _, field := range filteredFields {
			projection[field] = 1
		}
	}

	findOptions := options.Find()
	if len(projection) > 0 {
		findOptions.SetProjection(projection)
	}

	return findOptions
}

func (ds *ExternalDataSource) validateCollectionAndFields(ctx context.Context, client *mongo.Client, collection string, requestedFields []string, filterFields []string) ([]string, error) {
	database := client.Database(ds.Database)

	collections, err := database.ListCollectionNames(ctx, bson.M{"name": collection})
	if err != nil {
		return nil, fmt.Errorf("failed to validate collection %q: %w", collection, err)
	}

	if len(collections) == 0 {
		return nil, fmt.Errorf("collection %q does not exist in the database", collection)
	}

	coll := database.Collection(collection)

	count, err := coll.CountDocuments(ctx, bson.M{})
	if err != nil {
		return nil, fmt.Errorf("failed to inspect collection %q: %w", collection, err)
	}

	if count == 0 {
		return requestedFields, nil
	}

	knownFields, err := ds.discoverCollectionFields(ctx, coll)
	if err != nil {
		return nil, err
	}

	return validateMongoFieldNames(collection, knownFields, requestedFields, filterFields)
}

func (ds *ExternalDataSource) discoverCollectionFields(ctx context.Context, coll *mongo.Collection) (map[string]bool, error) {
	allFields, err := ds.discoverAllFieldsWithAggregation(ctx, coll)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect collection %q fields: %w", coll.Name(), err)
	}

	_, additionalFields, err := ds.sampleMultipleDocuments(ctx, coll)
	if err != nil {
		return nil, fmt.Errorf("failed to sample collection %q fields: %w", coll.Name(), err)
	}

	for field := range additionalFields {
		allFields[field] = true
	}

	return allFields, nil
}

func validateMongoFieldNames(collection string, knownFields map[string]bool, requestedFields []string, filterFields []string) ([]string, error) {
	if len(requestedFields) == 1 && requestedFields[0] == "*" {
		if invalid := invalidMongoFields(knownFields, filterFields); len(invalid) > 0 {
			return nil, fmt.Errorf("invalid filter fields for collection %q: %v", collection, invalid)
		}

		return requestedFields, nil
	}

	if invalid := invalidMongoFields(knownFields, requestedFields); len(invalid) > 0 {
		return nil, fmt.Errorf("invalid fields for collection %q: %v", collection, invalid)
	}

	if invalid := invalidMongoFields(knownFields, filterFields); len(invalid) > 0 {
		return nil, fmt.Errorf("invalid filter fields for collection %q: %v", collection, invalid)
	}

	return requestedFields, nil
}

func invalidMongoFields(knownFields map[string]bool, fields []string) []string {
	invalid := make([]string, 0)

	for _, field := range fields {
		rootField := field
		if dotIdx := strings.Index(field, "."); dotIdx != -1 {
			rootField = field[:dotIdx]
		}

		if !knownFields[rootField] {
			invalid = append(invalid, field)
		}
	}

	return invalid
}

func filterFieldNames(filter map[string][]any) []string {
	fields := make([]string, 0, len(filter))
	for field := range filter {
		fields = append(fields, field)
	}

	return fields
}

func advancedFilterFieldNames(filter map[string]model.FilterCondition) []string {
	fields := make([]string, 0, len(filter))
	for field := range filter {
		fields = append(fields, field)
	}

	return fields
}

// executeFindQuery executes the MongoDB find query with timeout.
func (ds *ExternalDataSource) executeFindQuery(ctx context.Context, client *mongo.Client, collection string, mongoFilter bson.M, findOptions *options.FindOptionsBuilder) (*mongo.Cursor, context.Context, context.CancelFunc, error) {
	queryCtx, cancel := context.WithTimeout(ctx, constant.QueryTimeoutSlow)
	database := client.Database(ds.Database)

	cursor, err := database.Collection(collection).Find(queryCtx, mongoFilter, findOptions)
	if err != nil {
		cancel()

		if queryCtx.Err() == context.DeadlineExceeded {
			return nil, nil, nil, fmt.Errorf("mongodb advanced filter query timeout after %v for collection %s: %w", constant.QueryTimeoutSlow, collection, err)
		}

		return nil, nil, nil, err
	}

	return cursor, queryCtx, cancel, nil
}

// processQueryResults iterates through cursor and converts results.
func (ds *ExternalDataSource) processQueryResults(queryCtx context.Context, cursor *mongo.Cursor, collection string, logger log.Logger) ([]map[string]any, error) {
	var results []map[string]any

	for cursor.Next(queryCtx) {
		var result bson.M
		if err := cursor.Decode(&result); err != nil {
			logger.Log(queryCtx, log.LevelWarn, "Error decoding document", log.Err(err))
			continue
		}

		results = append(results, convertBsonToMap(result))
	}

	if err := cursor.Err(); err != nil {
		if queryCtx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("mongodb advanced filter result iteration timeout after %v for collection %s: %w", constant.QueryTimeoutSlow, collection, err)
		}

		return nil, err
	}

	return results, nil
}

// ListCollectionNames returns all collection names in the database.
// Used by plugin_crm to discover org-scoped collections (e.g. holders_orgA, aliases_orgB).
func (ds *ExternalDataSource) ListCollectionNames(ctx context.Context) ([]string, error) {
	logger := ctxutil.NewLoggerFromContext(ctx)

	client, err := ds.connection.GetDB(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get MongoDB client for ListCollectionNames: %w", err)
	}

	listCtx, cancel := context.WithTimeout(ctx, constant.QueryTimeoutSlow)
	defer cancel()

	collections, err := client.Database(ds.Database).ListCollectionNames(listCtx, bson.M{})
	if err != nil {
		return nil, fmt.Errorf("failed to list collection names: %w", err)
	}

	logger.Log(ctx, log.LevelDebug, "Listed collection names",
		log.String("database", ds.Database),
		log.Int("count", len(collections)),
	)

	return collections, nil
}
