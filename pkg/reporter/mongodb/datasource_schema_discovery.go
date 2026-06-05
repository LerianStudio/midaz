// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mongodb

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/LerianStudio/midaz/v4/pkg/reporter/constant"
)

// discoverAllFieldsWithAggregation uses MongoDB aggregation with sampling for large collections.
func (ds *ExternalDataSource) discoverAllFieldsWithAggregation(ctx context.Context, coll *mongo.Collection) (map[string]bool, error) {
	count, err := coll.CountDocuments(ctx, bson.M{})
	if err != nil {
		return nil, err
	}

	if count > constant.MongoLargeCollectionThreshold {
		return ds.discoverFieldsWithSampling(ctx, coll, count)
	}

	pipeline := []bson.M{{"$limit": func() int64 {
		if count > constant.MongoSmallCollectionLimit {
			return constant.MongoSmallCollectionLimit
		}

		return count
	}()}, {"$project": bson.M{"arrayofkeyvalue": bson.M{"$objectToArray": "$$ROOT"}}}, {"$unwind": "$arrayofkeyvalue"}, {"$group": bson.M{"_id": nil, "allkeys": bson.M{"$addToSet": "$arrayofkeyvalue.k"}}}}

	cursor, err := coll.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	allFields := make(map[string]bool)

	if cursor.Next(ctx) {
		var result struct {
			AllKeys []string `bson:"allkeys"`
		}
		if err := cursor.Decode(&result); err != nil {
			return nil, fmt.Errorf("failed to decode aggregation result for collection %q: %w", coll.Name(), err)
		}

		for _, key := range result.AllKeys {
			allFields[key] = true
		}
	}

	return allFields, nil
}

// discoverFieldsWithSampling uses intelligent sampling for large collections.
func (ds *ExternalDataSource) discoverFieldsWithSampling(ctx context.Context, coll *mongo.Collection, totalDocs int64) (map[string]bool, error) {
	sampleSize := ds.calculateOptimalSampleSize(totalDocs)
	pipeline := []bson.M{{"$sample": bson.M{"size": sampleSize}}, {"$project": bson.M{"arrayofkeyvalue": bson.M{"$objectToArray": "$$ROOT"}}}, {"$unwind": "$arrayofkeyvalue"}, {"$group": bson.M{"_id": nil, "allkeys": bson.M{"$addToSet": "$arrayofkeyvalue.k"}}}}

	cursor, err := coll.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	allFields := make(map[string]bool)

	if cursor.Next(ctx) {
		var result struct {
			AllKeys []string `bson:"allkeys"`
		}
		if err := cursor.Decode(&result); err != nil {
			return nil, fmt.Errorf("failed to decode sampling result for collection %q: %w", coll.Name(), err)
		}

		for _, key := range result.AllKeys {
			allFields[key] = true
		}
	}

	return allFields, nil
}

// calculateOptimalSampleSize calculates the optimal sample size based on collection size.
func (ds *ExternalDataSource) calculateOptimalSampleSize(totalDocs int64) int {
	switch {
	case totalDocs <= constant.MongoSmallCollectionDocLimit:
		return int(totalDocs)
	case totalDocs <= constant.MongoMediumCollectionDocLimit:
		return constant.MongoDefaultSampleSize
	case totalDocs <= constant.MongoLargeCollectionDocLimit:
		return constant.MongoMediumSampleSize
	case totalDocs <= constant.MongoVeryLargeCollectionDocLimit:
		return constant.MongoLargeSampleSize
	default:
		return constant.MongoMaxSampleSize
	}
}

// sampleMultipleDocuments samples multiple documents to discover field types and additional fields.
func (ds *ExternalDataSource) sampleMultipleDocuments(ctx context.Context, coll *mongo.Collection) (map[string]string, map[string]bool, error) {
	count, err := coll.CountDocuments(ctx, bson.M{})
	if err != nil {
		return nil, nil, err
	}

	sampleSize := constant.MongoMinDocsForSampling
	if count < constant.MongoMinDocsForSampling {
		sampleSize = int(count)
	}

	var cursor *mongo.Cursor
	if count > constant.MongoMaxDocsForSmallSample {
		cursor, err = coll.Aggregate(ctx, []bson.M{{"$sample": bson.M{"size": sampleSize}}})
	} else {
		cursor, err = coll.Find(ctx, bson.M{}, options.Find().SetLimit(int64(sampleSize)))
	}

	if err != nil {
		return nil, nil, err
	}

	defer cursor.Close(ctx)

	fieldTypes := make(map[string]string)
	allFields := make(map[string]bool)

	for cursor.Next(ctx) {
		var doc bson.M
		if err := cursor.Decode(&doc); err != nil {
			continue
		}

		for fieldName, value := range doc {
			allFields[fieldName] = true

			dataType := ds.inferDataType(value)
			if currentType, exists := fieldTypes[fieldName]; !exists || ds.isMoreSpecificType(dataType, currentType) {
				fieldTypes[fieldName] = dataType
			}
		}
	}

	return fieldTypes, allFields, nil
}

// inferDataType determines the MongoDB data type from a Go value.
func (ds *ExternalDataSource) inferDataType(value any) string {
	switch value.(type) {
	case string:
		return "string"
	case int, int32, int64, float32, float64:
		return "number"
	case bool:
		return "boolean"
	case bson.A:
		return "array"
	case bson.M, bson.D:
		return "object"
	case bson.DateTime:
		return "date"
	case bson.ObjectID:
		return "objectId"
	case bson.Binary:
		return "binData"
	case bson.Regex:
		return "regex"
	case bson.Timestamp:
		return "timestamp"
	case bson.Decimal128:
		return "decimal"
	case bson.MinKey, bson.MaxKey:
		return "minKey/maxKey"
	default:
		return unknownDataType
	}
}

// isMoreSpecificType determines if one type is more specific than another.
func (ds *ExternalDataSource) isMoreSpecificType(newType, currentType string) bool {
	typeHierarchy := map[string]int{
		"objectId":      constant.BSONPriorityObjectID,
		"date":          constant.BSONPriorityDate,
		"timestamp":     constant.BSONPriorityTimestamp,
		"decimal":       constant.BSONPriorityDecimal,
		"binData":       constant.BSONPriorityBinData,
		"regex":         constant.BSONPriorityRegex,
		"minKey/maxKey": constant.BSONPriorityMinMaxKey,
		"number":        constant.BSONPriorityNumber,
		"string":        constant.BSONPriorityDefault,
		"boolean":       constant.BSONPriorityDefault,
		"array":         constant.BSONPriorityDefault,
		"object":        constant.BSONPriorityDefault,
		"unknown":       constant.BSONPriorityUnknown,
	}

	return typeHierarchy[newType] > typeHierarchy[currentType]
}
