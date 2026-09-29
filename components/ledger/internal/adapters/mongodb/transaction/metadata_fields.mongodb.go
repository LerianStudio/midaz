// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mongodb

import (
	"context"
	"strings"
	"time"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// UpdateFields sets each non-nil field of an entity's metadata and removes each nil one in one
// write, keeping every other stored key; keys stay literal and flat. It returns the resulting
// document and upserts a missing one like Update.
func (mmr *MetadataMongoDBRepository) UpdateFields(ctx context.Context, collection, id string, fields map[string]any) (*Metadata, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.update_metadata_fields")
	defer span.End()

	db, err := mmr.getDatabase(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)

		return nil, err
	}

	touched, written := make(bson.A, 0, len(fields)), make(bson.A, 0, len(fields))
	for key, value := range fields {
		touched = append(touched, key)

		if value != nil {
			written = append(written, bson.M{"k": key, "v": value})
		}
	}

	kept := bson.M{"$filter": bson.M{
		"input": bson.M{"$objectToArray": bson.M{"$ifNull": bson.A{"$metadata", bson.M{}}}},
		"cond":  bson.M{"$not": bson.A{bson.M{"$in": bson.A{"$$this.k", bson.M{"$literal": touched}}}}},
	}}
	metadata := bson.M{"$arrayToObject": bson.A{bson.M{"$concatArrays": bson.A{kept, bson.M{"$literal": written}}}}}
	update := mongo.Pipeline{{{Key: "$set", Value: bson.M{"metadata": metadata, "updated_at": time.Now()}}}}
	opts := options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After)

	_, spanUpdate := tracer.Start(ctx, "mongodb.update_metadata_fields.find_one_and_update")
	defer spanUpdate.End()

	var record MetadataMongoDBModel
	if err := db.Collection(strings.ToLower(collection)).FindOneAndUpdate(ctx, bson.M{"entity_id": id}, update, opts).Decode(&record); err != nil {
		libOpentelemetry.HandleSpanError(spanUpdate, "Failed to update metadata fields", err)

		return nil, err
	}

	return record.ToEntity(), nil
}
