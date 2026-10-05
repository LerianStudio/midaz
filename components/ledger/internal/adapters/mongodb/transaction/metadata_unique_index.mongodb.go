// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mongodb

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// entityIDIndexName is the name MongoDB derives for the ascending entity_id index of a metadata
// collection, and the name the tenant Mongo migrations (migrations/<module>/mongodb) create it under.
const entityIDIndexName = "entity_id_1"

// EnsureUniqueEntityIDIndex creates entity_id_1 unique on a metadata collection that has none, so
// one entity holds one document, and reports whether the collection's entity_id_1 is unique. It
// never drops an index: converting a non-unique one is the backfill runner's job.
func EnsureUniqueEntityIDIndex(ctx context.Context, coll *mongo.Collection) (bool, error) {
	specs, err := coll.Indexes().ListSpecifications(ctx)
	if err != nil {
		return false, err
	}

	for _, spec := range specs {
		if spec.Name == entityIDIndexName {
			return spec.Unique != nil && *spec.Unique, nil
		}
	}

	_, err = coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "entity_id", Value: 1}}, Options: options.Index().SetUnique(true),
	})

	return err == nil, err
}
