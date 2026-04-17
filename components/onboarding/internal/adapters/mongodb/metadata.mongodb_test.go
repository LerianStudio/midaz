// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mongodb

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/integration/mtest"

	libMongo "github.com/LerianStudio/lib-commons/v2/commons/mongo"

	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
)

func anyMongoTime() time.Time {
	t, _ := time.Parse(time.RFC3339, "2024-01-01T00:00:00Z")
	return t
}

// repoForClient wraps an mtest-provided *mongo.Client into the production
// repository so the real adapter methods can be exercised against a mock
// deployment. Closing is handled by mtest.
func repoForClient(client *mongo.Client, database string) *MetadataMongoDBRepository {
	return &MetadataMongoDBRepository{
		connection: &libMongo.MongoConnection{
			DB:        client,
			Connected: true,
			Database:  database,
		},
		Database: database,
	}
}

// withMockT wraps the mtest.T lifecycle behind a helper that keeps individual
// test cases focused on the mongo behaviour instead of mtest boilerplate.
func withMockT(t *testing.T, name string, body func(mt *mtest.T)) {
	t.Helper()

	opts := mtest.NewOptions().ClientType(mtest.Mock)
	mt := mtest.New(t, opts)
	mt.Run(name, body)
}

// commandErr returns a command error response suitable for mtest.AddMockResponses.
func commandErr() primitive.D {
	return mtest.CreateCommandErrorResponse(mtest.CommandError{
		Code: 1, Name: "InternalError", Message: "boom",
	})
}

func sampleDoc() bson.D {
	return bson.D{
		{Key: "_id", Value: primitive.NewObjectID()},
		{Key: "entity_id", Value: "ent-1"},
		{Key: "entity_name", Value: "segment"},
		{Key: "metadata", Value: bson.M{"k": "v"}},
	}
}

func TestMetadataRepository_Create(t *testing.T) {
	t.Parallel()

	withMockT(t, "insert_succeeds", func(mt *mtest.T) {
		mt.AddMockResponses(mtest.CreateSuccessResponse())

		r := repoForClient(mt.Client, strings.ToLower(mt.DB.Name()))

		err := r.Create(context.Background(), "segment", &Metadata{
			ID:       primitive.NewObjectID(),
			EntityID: "ent-1",
			Data:     JSON{"k": "v"},
		})
		require.NoError(t, err)
	})

	withMockT(t, "insert_error_wraps", func(mt *mtest.T) {
		mt.AddMockResponses(mtest.CreateWriteErrorsResponse(mtest.WriteError{
			Index: 0, Code: 11000, Message: "dup",
		}))

		r := repoForClient(mt.Client, strings.ToLower(mt.DB.Name()))

		err := r.Create(context.Background(), "segment", &Metadata{
			ID:       primitive.NewObjectID(),
			EntityID: "ent-2",
			Data:     JSON{"k": "v"},
		})
		require.Error(t, err)
	})
}

func TestMetadataRepository_FindByEntity(t *testing.T) {
	t.Parallel()

	withMockT(t, "found", func(mt *mtest.T) {
		mt.AddMockResponses(mtest.CreateCursorResponse(1, mt.DB.Name()+".segment", mtest.FirstBatch, sampleDoc()))

		r := repoForClient(mt.Client, strings.ToLower(mt.DB.Name()))

		got, err := r.FindByEntity(context.Background(), "segment", "ent-1")
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "ent-1", got.EntityID)
	})

	withMockT(t, "not_found_returns_nil", func(mt *mtest.T) {
		mt.AddMockResponses(mtest.CreateCursorResponse(0, mt.DB.Name()+".segment", mtest.FirstBatch))

		r := repoForClient(mt.Client, strings.ToLower(mt.DB.Name()))

		got, err := r.FindByEntity(context.Background(), "segment", "ent-none")
		require.NoError(t, err)
		assert.Nil(t, got)
	})

	withMockT(t, "query_error", func(mt *mtest.T) {
		mt.AddMockResponses(commandErr())

		r := repoForClient(mt.Client, strings.ToLower(mt.DB.Name()))

		_, err := r.FindByEntity(context.Background(), "segment", "ent-1")
		require.Error(t, err)
	})
}

func TestMetadataRepository_FindByEntityIDs(t *testing.T) {
	t.Parallel()

	withMockT(t, "returns_rows", func(mt *mtest.T) {
		first := mtest.CreateCursorResponse(1, mt.DB.Name()+".segment", mtest.FirstBatch, sampleDoc())
		second := mtest.CreateCursorResponse(0, mt.DB.Name()+".segment", mtest.NextBatch)
		mt.AddMockResponses(first, second)

		r := repoForClient(mt.Client, strings.ToLower(mt.DB.Name()))

		got, err := r.FindByEntityIDs(context.Background(), "segment", []string{"ent-1"})
		require.NoError(t, err)
		assert.Len(t, got, 1)
	})

	withMockT(t, "cursor_error", func(mt *mtest.T) {
		mt.AddMockResponses(commandErr())

		r := repoForClient(mt.Client, strings.ToLower(mt.DB.Name()))

		_, err := r.FindByEntityIDs(context.Background(), "segment", []string{"ent-1"})
		require.Error(t, err)
	})
}

func TestMetadataRepository_FindList(t *testing.T) {
	t.Parallel()

	withMockT(t, "returns_rows", func(mt *mtest.T) {
		first := mtest.CreateCursorResponse(1, mt.DB.Name()+".segment", mtest.FirstBatch, sampleDoc())
		second := mtest.CreateCursorResponse(0, mt.DB.Name()+".segment", mtest.NextBatch)
		mt.AddMockResponses(first, second)

		r := repoForClient(mt.Client, strings.ToLower(mt.DB.Name()))

		got, err := r.FindList(context.Background(), "segment", http.QueryHeader{
			Limit: 10, Page: 1, SortOrder: "asc", UseMetadata: true,
			Metadata: &map[string]any{},
		})
		require.NoError(t, err)
		assert.Len(t, got, 1)
	})

	withMockT(t, "query_error", func(mt *mtest.T) {
		mt.AddMockResponses(commandErr())

		r := repoForClient(mt.Client, strings.ToLower(mt.DB.Name()))

		_, err := r.FindList(context.Background(), "segment", http.QueryHeader{
			Limit: 10, Page: 1, SortOrder: "desc", UseMetadata: true,
			Metadata: &map[string]any{},
		})
		require.Error(t, err)
	})
}

func TestMetadataRepository_Update(t *testing.T) {
	t.Parallel()

	withMockT(t, "updates_document", func(mt *mtest.T) {
		mt.AddMockResponses(bson.D{
			{Key: "ok", Value: 1},
			{Key: "n", Value: 1},
			{Key: "nModified", Value: 1},
		})

		r := repoForClient(mt.Client, strings.ToLower(mt.DB.Name()))

		err := r.Update(context.Background(), "segment", "ent-1", map[string]any{"k": "v"})
		require.NoError(t, err)
	})

	withMockT(t, "update_error", func(mt *mtest.T) {
		mt.AddMockResponses(commandErr())

		r := repoForClient(mt.Client, strings.ToLower(mt.DB.Name()))

		err := r.Update(context.Background(), "segment", "ent-1", map[string]any{"k": "v"})
		require.Error(t, err)
	})
}

func TestMetadataRepository_Delete(t *testing.T) {
	t.Parallel()

	withMockT(t, "deletes_document", func(mt *mtest.T) {
		mt.AddMockResponses(bson.D{{Key: "ok", Value: 1}, {Key: "n", Value: 1}})

		r := repoForClient(mt.Client, strings.ToLower(mt.DB.Name()))

		err := r.Delete(context.Background(), "segment", "ent-1")
		require.NoError(t, err)
	})

	withMockT(t, "delete_error", func(mt *mtest.T) {
		mt.AddMockResponses(commandErr())

		r := repoForClient(mt.Client, strings.ToLower(mt.DB.Name()))

		err := r.Delete(context.Background(), "segment", "ent-1")
		require.Error(t, err)
	})
}

func TestMetadataRepository_CreateIndex(t *testing.T) {
	t.Parallel()

	withMockT(t, "create_index_error", func(mt *mtest.T) {
		mt.AddMockResponses(commandErr())

		r := repoForClient(mt.Client, strings.ToLower(mt.DB.Name()))

		_, err := r.CreateIndex(context.Background(), "segment", &mmodel.CreateMetadataIndexInput{
			MetadataKey: "tier",
		})
		require.Error(t, err)
	})
}

func TestMetadataRepository_FindAllIndexes(t *testing.T) {
	t.Parallel()

	withMockT(t, "aggregate_error", func(mt *mtest.T) {
		mt.AddMockResponses(commandErr())

		r := repoForClient(mt.Client, strings.ToLower(mt.DB.Name()))

		_, err := r.FindAllIndexes(context.Background(), "segment")
		require.Error(t, err)
	})

	withMockT(t, "list_indexes_error_after_stats", func(mt *mtest.T) {
		// First: $indexStats aggregate succeeds with empty cursor.
		statsFirst := mtest.CreateCursorResponse(0, mt.DB.Name()+".segment", mtest.FirstBatch)
		// Second: listIndexes command fails.
		mt.AddMockResponses(statsFirst, commandErr())

		r := repoForClient(mt.Client, strings.ToLower(mt.DB.Name()))

		_, err := r.FindAllIndexes(context.Background(), "segment")
		require.Error(t, err)
	})

	withMockT(t, "happy_path_filters_metadata_prefix", func(mt *mtest.T) {
		// $indexStats aggregate: return one entry.
		statsFirst := mtest.CreateCursorResponse(1, mt.DB.Name()+".segment", mtest.FirstBatch, bson.D{
			{Key: "name", Value: "metadata.tier_1"},
			{Key: "accesses", Value: bson.D{
				{Key: "ops", Value: int64(42)},
				{Key: "since", Value: anyTimeMongo()},
			}},
		})
		statsNext := mtest.CreateCursorResponse(0, mt.DB.Name()+".segment", mtest.NextBatch)

		// listIndexes: return one index on "metadata.tier" and one on "_id" (should be skipped).
		idxFirst := mtest.CreateCursorResponse(1, mt.DB.Name()+".segment.$cmd.listIndexes", mtest.FirstBatch,
			bson.D{
				{Key: "v", Value: int32(2)},
				{Key: "name", Value: "metadata.tier_1"},
				{Key: "key", Value: bson.D{{Key: "metadata.tier", Value: int32(1)}}},
				{Key: "unique", Value: true},
				{Key: "sparse", Value: true},
			},
			bson.D{
				{Key: "v", Value: int32(2)},
				{Key: "name", Value: "_id_"},
				{Key: "key", Value: bson.D{{Key: "_id", Value: int32(1)}}},
			},
		)
		idxNext := mtest.CreateCursorResponse(0, mt.DB.Name()+".segment.$cmd.listIndexes", mtest.NextBatch)

		mt.AddMockResponses(statsFirst, statsNext, idxFirst, idxNext)

		r := repoForClient(mt.Client, strings.ToLower(mt.DB.Name()))

		got, err := r.FindAllIndexes(context.Background(), "segment")
		require.NoError(t, err)
		// Only the metadata-prefixed index survives the filter.
		require.Len(t, got, 1)
		assert.Equal(t, "tier", got[0].MetadataKey)
	})
}

// anyTimeMongo returns a mongo-friendly timestamp for fixture stats responses.
func anyTimeMongo() primitive.DateTime {
	return primitive.NewDateTimeFromTime(anyMongoTime())
}

func TestMetadataRepository_DeleteIndex(t *testing.T) {
	t.Parallel()

	withMockT(t, "drops_index", func(mt *mtest.T) {
		mt.AddMockResponses(bson.D{{Key: "ok", Value: 1}, {Key: "nIndexesWas", Value: 2}})

		r := repoForClient(mt.Client, strings.ToLower(mt.DB.Name()))

		err := r.DeleteIndex(context.Background(), "segment", "entity_id_1")
		require.NoError(t, err)
	})

	withMockT(t, "drop_index_error", func(mt *mtest.T) {
		mt.AddMockResponses(commandErr())

		r := repoForClient(mt.Client, strings.ToLower(mt.DB.Name()))

		err := r.DeleteIndex(context.Background(), "segment", "entity_id_1")
		require.Error(t, err)
	})
}
