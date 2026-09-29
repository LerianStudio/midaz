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
)

// SetKeys sets each key inside an existing document's metadata and bumps updated_at, keeping
// every other key. Keys must be flat names without '.' or a leading '$'.
func (mmr *MetadataMongoDBRepository) SetKeys(ctx context.Context, collection, id string, keys map[string]any) error {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.set_metadata_keys")
	defer span.End()

	db, err := mmr.getDatabase(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)

		return err
	}

	set := bson.D{{Key: "updated_at", Value: time.Now()}}
	for key, value := range keys {
		set = append(set, bson.E{Key: "metadata." + key, Value: value})
	}

	if _, err := db.Collection(strings.ToLower(collection)).UpdateOne(ctx, bson.M{"entity_id": id}, bson.D{{Key: "$set", Value: set}}); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to set metadata keys", err)

		return err
	}

	return nil
}

// UpdateIfUnchanged replaces an existing document's metadata only while its updated_at still
// equals updatedAt, as read, and reports whether it did.
func (mmr *MetadataMongoDBRepository) UpdateIfUnchanged(ctx context.Context, collection, id string, metadata map[string]any, updatedAt time.Time) (bool, error) {
	_, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.update_metadata_if_unchanged")
	defer span.End()

	db, err := mmr.getDatabase(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)

		return false, err
	}

	filter := bson.D{{Key: "entity_id", Value: id}, {Key: "updated_at", Value: updatedAt}}
	update := bson.D{{Key: "$set", Value: bson.D{{Key: "metadata", Value: metadata}, {Key: "updated_at", Value: time.Now()}}}}

	result, err := db.Collection(strings.ToLower(collection)).UpdateOne(ctx, filter, update)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to update metadata", err)

		return false, err
	}

	return result.MatchedCount == 1, nil
}
