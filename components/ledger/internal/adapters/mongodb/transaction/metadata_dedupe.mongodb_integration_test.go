//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mongodb

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/event"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	mongotestutil "github.com/LerianStudio/midaz/v4/tests/utils/mongodb"
)

// The production shape: a completion document beside the unnamed one a racing PATCH inserted. The
// dedupe folds them into the oldest, deletes the other, leaves entity_id unique, and a re-run
// changes nothing but a temporary index a crashed run left behind.
func TestIntegration_DedupeEntityIDAndMakeUnique_FoldsDuplicatesAndIsIdempotent(t *testing.T) {
	container := mongotestutil.SetupReusableContainer(t)
	ctx := context.Background()
	coll := container.Database.Collection("transaction")

	_, err := coll.Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "entity_id", Value: 1}}})
	require.NoError(t, err)

	completedAt := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	patchedAt := completedAt.Add(time.Minute)
	completed, patched := bson.NewObjectIDFromTimestamp(completedAt), bson.NewObjectIDFromTimestamp(patchedAt)

	_, err = coll.InsertMany(ctx, []any{
		bson.M{
			"_id": patched, "entity_id": "txn-dup", "metadata": bson.M{"client": "patched", "purpose": "patched", "feeApplied": "client"},
			"created_at": patchedAt, "updated_at": patchedAt,
		},
		bson.M{
			"_id": completed, "entity_id": "txn-dup", "entity_name": "Transaction", "metadata": bson.M{"purpose": "frozen", "feeApplied": "true"},
			"created_at": completedAt, "updated_at": completedAt,
		},
		bson.M{"entity_id": "txn-single", "entity_name": "Transaction", "metadata": bson.M{"k": "v"}, "created_at": completedAt, "updated_at": completedAt},
	})
	require.NoError(t, err)

	folded, raced, err := DedupeEntityIDAndMakeUnique(ctx, coll)
	require.NoError(t, err)
	assert.Equal(t, 1, folded)
	assert.False(t, raced)

	merged := readAll(t, coll, "txn-dup")
	require.Len(t, merged, 1, "one document per entity")
	assert.Equal(t, MetadataMongoDBModel{
		ID: completed, EntityID: "txn-dup", EntityName: "Transaction",
		Data:      JSON{"client": "patched", "purpose": "patched", "feeApplied": "true"},
		CreatedAt: completedAt, UpdatedAt: patchedAt,
	}, merged[0])
	assert.Equal(t, map[string]bool{"_id_": false, entityIDIndexName: true}, indexUniqueness(t, coll))

	_, err = coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{{Key: "entity_id", Value: 1}, {Key: "_id", Value: 1}}, Options: options.Index().SetName(tempEntityIDIndexName),
	})
	require.NoError(t, err)

	before := readAll(t, coll, "")

	folded, raced, err = DedupeEntityIDAndMakeUnique(ctx, coll)
	require.NoError(t, err)
	assert.Zero(t, folded, "a re-run finds nothing to fold")
	assert.False(t, raced)
	assert.Equal(t, before, readAll(t, coll, ""), "a re-run writes nothing")
	assert.Equal(t, map[string]bool{"_id_": false, entityIDIndexName: true}, indexUniqueness(t, coll), "the leftover temporary index is dropped")
}

// A duplicate written between the fold's read and its writes is neither overwritten nor deleted:
// the entity is reported as raced, entity_id stays non-unique, and a re-run folds it.
func TestIntegration_DedupeEntityIDAndMakeUnique_KeepsAWriteThatLandsAfterTheRead(t *testing.T) {
	container := mongotestutil.SetupReusableContainer(t)
	ctx := context.Background()
	coll := container.Database.Collection("transaction")
	readAt := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	writtenAt := readAt.Add(time.Hour)

	_, err := coll.InsertMany(ctx, []any{
		bson.M{"entity_id": "txn-dup", "entity_name": "Transaction", "metadata": bson.M{"purpose": "frozen"}, "created_at": readAt, "updated_at": readAt},
		bson.M{"entity_id": "txn-dup", "metadata": bson.M{"client": "first"}, "created_at": readAt, "updated_at": readAt},
	})
	require.NoError(t, err)

	read, err := readEntity(ctx, coll, "txn-dup")
	require.NoError(t, err)

	_, err = coll.UpdateByID(ctx, read[1].ID, bson.D{{Key: "$set", Value: bson.D{
		{Key: "metadata.client", Value: "second"}, {Key: "updated_at", Value: writtenAt},
	}}})
	require.NoError(t, err)

	written, err := foldEntity(ctx, coll, read)
	require.NoError(t, err)
	assert.False(t, written, "the entity is reported as raced")

	survivors := readAll(t, coll, "txn-dup")
	require.Len(t, survivors, 2, "the later write survives")
	assert.Equal(t, JSON{"client": "second"}, survivors[1].Data)

	_, raced, err := DedupeEntityIDAndMakeUnique(ctx, coll)
	require.NoError(t, err, "a re-run folds the later write")
	assert.False(t, raced)
	assert.Equal(t, JSON{"purpose": "frozen", "client": "second"}, readAll(t, coll, "txn-dup")[0].Data)
}

// A duplicate written after the folds refuses the unique build. That is the same race as a raced
// fold: the temporary index served entity_id lookups meanwhile, the non-unique entity_id_1 is back,
// and the collection is reported for a re-run instead of failing the run.
func TestIntegration_DedupeEntityIDAndMakeUnique_ReportsADuplicateThatRefusesTheBuildAsRaced(t *testing.T) {
	container := mongotestutil.SetupReusableContainer(t)
	plain := container.Database.Collection("transaction")

	_, err := plain.Indexes().CreateOne(context.Background(), mongo.IndexModel{Keys: bson.D{{Key: "entity_id", Value: 1}}})
	require.NoError(t, err)

	var (
		inserted    bool
		duringBuild []mongo.IndexSpecification
		hookErr     error
	)

	coll := monitoredCollection(t, container, &event.CommandMonitor{
		Succeeded: func(_ context.Context, e *event.CommandSucceededEvent) {
			if e.CommandName == "dropIndexes" && !inserted {
				inserted = true
				_, hookErr = plain.InsertMany(context.Background(), []any{bson.M{"entity_id": "txn-late"}, bson.M{"entity_id": "txn-late"}})
			}
		},
		Failed: func(_ context.Context, e *event.CommandFailedEvent) {
			if e.CommandName == "createIndexes" && duringBuild == nil {
				duringBuild, hookErr = plain.Indexes().ListSpecifications(context.Background())
			}
		},
	})

	folded, raced, err := DedupeEntityIDAndMakeUnique(context.Background(), coll)
	require.NoError(t, err)
	require.NoError(t, hookErr)
	assert.Zero(t, folded)
	assert.True(t, raced, "the collection is reported for a re-run")
	assert.Equal(t, map[string]bool{"_id_": false, tempEntityIDIndexName: false}, uniqueness(duringBuild), "entity_id stays indexed mid-rebuild")
	assert.Equal(t, map[string]bool{"_id_": false, entityIDIndexName: false}, indexUniqueness(t, plain), "the non-unique index is back")
}

// A context that ends after the drop (SIGTERM, the run's deadline) fails the unique build, and the
// non-unique entity_id_1 is still restored; a temporary index a crashed run left is reused.
func TestIntegration_MakeEntityIDIndexUnique_RestoresTheIndexWhenTheContextEndsAfterTheDrop(t *testing.T) {
	container := mongotestutil.SetupReusableContainer(t)
	plain := container.Database.Collection("transaction")

	for _, model := range []mongo.IndexModel{
		{Keys: bson.D{{Key: "entity_id", Value: 1}}},
		{Keys: bson.D{{Key: "entity_id", Value: 1}, {Key: "_id", Value: 1}}, Options: options.Index().SetName(tempEntityIDIndexName)},
	} {
		_, err := plain.Indexes().CreateOne(context.Background(), model)
		require.NoError(t, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	coll := monitoredCollection(t, container, &event.CommandMonitor{Succeeded: func(_ context.Context, e *event.CommandSucceededEvent) {
		if e.CommandName == "dropIndexes" {
			cancel()
		}
	}})

	err := makeEntityIDIndexUnique(ctx, coll)
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, map[string]bool{"_id_": false, entityIDIndexName: false}, indexUniqueness(t, plain), "the non-unique index is back")
}

// monitoredCollection opens the container's transaction collection through a client reporting to monitor.
func monitoredCollection(t *testing.T, container *mongotestutil.ContainerResult, monitor *event.CommandMonitor) *mongo.Collection {
	t.Helper()

	client, err := mongo.Connect(options.Client().ApplyURI(container.URI).SetMonitor(monitor))
	require.NoError(t, err)

	t.Cleanup(func() { require.NoError(t, client.Disconnect(context.Background())) })

	return client.Database(container.DBName).Collection("transaction")
}

// readAll returns the documents of entityID, or every document when it is empty, ordered by _id.
func readAll(t *testing.T, coll *mongo.Collection, entityID string) []MetadataMongoDBModel {
	t.Helper()

	filter := bson.D{}
	if entityID != "" {
		filter = bson.D{{Key: "entity_id", Value: entityID}}
	}

	cur, err := coll.Find(context.Background(), filter, options.Find().SetSort(bson.D{{Key: "_id", Value: 1}}))
	require.NoError(t, err)

	var docs []MetadataMongoDBModel
	require.NoError(t, cur.All(context.Background(), &docs))

	for i := range docs {
		docs[i].CreatedAt, docs[i].UpdatedAt = docs[i].CreatedAt.UTC(), docs[i].UpdatedAt.UTC()
	}

	return docs
}

func indexUniqueness(t *testing.T, coll *mongo.Collection) map[string]bool {
	t.Helper()

	specs, err := coll.Indexes().ListSpecifications(context.Background())
	require.NoError(t, err)

	return uniqueness(specs)
}

// uniqueness maps each index name to whether the index is unique.
func uniqueness(specs []mongo.IndexSpecification) map[string]bool {
	names := make(map[string]bool, len(specs))

	for _, spec := range specs {
		names[spec.Name] = spec.Unique != nil && *spec.Unique
	}

	return names
}
