// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package holder

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// indexModels returns the index definitions for the holder collection.
func indexModels() []mongo.IndexModel {
	return []mongo.IndexModel{
		{
			// Document uniqueness is enforced on the active-primary search token; it holds within a
			// single keyset version. Cross-version uniqueness after PRF key rotation is a rotation-time
			// concern (rotation is currently out of scope).
			Keys: bson.D{{Key: "search.document", Value: 1}},
			Options: options.Index().
				SetUnique(true).
				SetPartialFilterExpression(bson.D{{Key: "deleted_at", Value: nil}}),
		},
		{
			Keys: bson.D{{Key: "deleted_at", Value: 1}},
			Options: options.Index().
				SetPartialFilterExpression(bson.D{{Key: "deleted_at", Value: nil}}),
		},
		{
			Keys: bson.D{{Key: "external_id", Value: 1}},
			Options: options.Index().
				SetPartialFilterExpression(bson.D{{Key: "deleted_at", Value: nil}}),
		},
		{
			Keys: bson.D{
				{Key: "search.document", Value: 1},
				{Key: "external_id", Value: 1},
			},
			Options: options.Index().
				SetUnique(true).
				SetPartialFilterExpression(bson.D{{Key: "deleted_at", Value: nil}}),
		},
		{
			Keys: bson.D{
				{Key: "type", Value: 1},
				{Key: "deleted_at", Value: 1},
			},
		},
	}
}

// createIndexes creates indexes for specific fields, if it not exists.
func createIndexes(ctx context.Context, collection *mongo.Collection) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, err := collection.Indexes().CreateMany(ctx, indexModels())

	return err
}
