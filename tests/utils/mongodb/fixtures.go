//go:build integration

package mongodb

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// MetadataFixture represents test metadata for insertion.
type MetadataFixture struct {
	ID         primitive.ObjectID `bson:"_id,omitempty"`
	EntityID   string             `bson:"entity_id"`
	EntityName string             `bson:"entity_name"`
	Data       map[string]any     `bson:"metadata"`
	CreatedAt  time.Time          `bson:"created_at"`
	UpdatedAt  time.Time          `bson:"updated_at"`
}

// InsertMetadata inserts a metadata fixture into the specified collection.
func InsertMetadata(t *testing.T, db *mongo.Database, collection string, fixture MetadataFixture) primitive.ObjectID {
	t.Helper()

	if fixture.ID.IsZero() {
		fixture.ID = primitive.NewObjectID()
	}

	if fixture.CreatedAt.IsZero() {
		fixture.CreatedAt = time.Now()
	}

	if fixture.UpdatedAt.IsZero() {
		fixture.UpdatedAt = time.Now()
	}

	coll := db.Collection(collection)
	_, err := coll.InsertOne(context.Background(), fixture)
	require.NoError(t, err, "failed to insert metadata fixture")

	return fixture.ID
}

// InsertManyMetadata inserts multiple metadata fixtures into the specified collection.
func InsertManyMetadata(t *testing.T, db *mongo.Database, collection string, fixtures []MetadataFixture) []primitive.ObjectID {
	t.Helper()

	ids := make([]primitive.ObjectID, len(fixtures))
	for i, f := range fixtures {
		ids[i] = InsertMetadata(t, db, collection, f)
	}

	return ids
}

// CountDocuments counts documents in a collection with optional filter.
func CountDocuments(t *testing.T, db *mongo.Database, collection string, filter bson.M) int64 {
	t.Helper()

	if filter == nil {
		filter = bson.M{}
	}

	count, err := db.Collection(collection).CountDocuments(context.Background(), filter)
	require.NoError(t, err, "failed to count documents")

	return count
}

// ClearCollection removes all documents from a collection.
func ClearCollection(t *testing.T, db *mongo.Database, collection string) {
	t.Helper()

	_, err := db.Collection(collection).DeleteMany(context.Background(), bson.M{})
	require.NoError(t, err, "failed to clear collection")
}
