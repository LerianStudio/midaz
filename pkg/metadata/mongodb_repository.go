package metadata

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MongoDBRepository implements the Repository interface using MongoDB
type MongoDBRepository struct {
	db         *mongo.Database
	collection string
}

// NewMongoDBRepository creates a new MongoDB metadata repository
func NewMongoDBRepository(db *mongo.Database, collection string) *MongoDBRepository {
	return &MongoDBRepository{
		db:         db,
		collection: collection,
	}
}

// getCollection returns the collection for a specific entity type
func (r *MongoDBRepository) getCollection(entityName string) *mongo.Collection {
	collectionName := fmt.Sprintf("%s_%s", r.collection, entityName)
	return r.db.Collection(collectionName)
}

// Create stores metadata for an entity
func (r *MongoDBRepository) Create(ctx context.Context, entityName string, metadata *Metadata) error {
	collection := r.getCollection(entityName)
	
	// Create unique index on entity_id
	indexModel := mongo.IndexModel{
		Keys:    bson.D{{Key: "entity_id", Value: 1}},
		Options: options.Index().SetUnique(true),
	}
	if _, err := collection.Indexes().CreateOne(ctx, indexModel); err != nil {
		// Ignore error if index already exists
		if !mongo.IsDuplicateKeyError(err) {
			return err
		}
	}

	_, err := collection.InsertOne(ctx, metadata)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return fmt.Errorf("metadata already exists for entity %s", metadata.EntityID)
		}
		return err
	}

	return nil
}

// Update modifies existing metadata for an entity
func (r *MongoDBRepository) Update(ctx context.Context, entityName string, entityID string, data map[string]interface{}) error {
	collection := r.getCollection(entityName)

	filter := bson.M{"entity_id": entityID}
	update := bson.M{
		"$set": bson.M{
			"data":       data,
			"updated_at": time.Now(),
		},
	}

	result, err := collection.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}

	if result.MatchedCount == 0 {
		return fmt.Errorf("metadata not found for entity %s", entityID)
	}

	return nil
}

// Find retrieves metadata for a specific entity
func (r *MongoDBRepository) Find(ctx context.Context, entityName string, entityID string) (*Metadata, error) {
	collection := r.getCollection(entityName)

	filter := bson.M{"entity_id": entityID}
	
	var metadata Metadata
	err := collection.FindOne(ctx, filter).Decode(&metadata)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("metadata not found for entity %s", entityID)
		}
		return nil, err
	}

	return &metadata, nil
}

// FindByQuery retrieves metadata matching specific criteria
func (r *MongoDBRepository) FindByQuery(ctx context.Context, entityName string, query map[string]interface{}) ([]*Metadata, error) {
	collection := r.getCollection(entityName)

	// Build filter from query
	filter := bson.M{}
	for key, value := range query {
		// Support nested queries in data field
		if key == "data" {
			if dataMap, ok := value.(map[string]interface{}); ok {
				for dataKey, dataValue := range dataMap {
					filter[fmt.Sprintf("data.%s", dataKey)] = dataValue
				}
			}
		} else {
			filter[key] = value
		}
	}

	cursor, err := collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []*Metadata
	for cursor.Next(ctx) {
		var metadata Metadata
		if err := cursor.Decode(&metadata); err != nil {
			return nil, err
		}
		results = append(results, &metadata)
	}

	if err := cursor.Err(); err != nil {
		return nil, err
	}

	return results, nil
}

// Delete removes metadata for an entity
func (r *MongoDBRepository) Delete(ctx context.Context, entityName string, entityID string) error {
	collection := r.getCollection(entityName)

	filter := bson.M{"entity_id": entityID}
	
	result, err := collection.DeleteOne(ctx, filter)
	if err != nil {
		return err
	}

	if result.DeletedCount == 0 {
		return fmt.Errorf("metadata not found for entity %s", entityID)
	}

	return nil
}

// BulkCreate creates multiple metadata entries in a single operation
func (r *MongoDBRepository) BulkCreate(ctx context.Context, entityName string, metadataList []*Metadata) error {
	if len(metadataList) == 0 {
		return nil
	}

	collection := r.getCollection(entityName)

	// Convert to interface slice for InsertMany
	docs := make([]interface{}, len(metadataList))
	for i, metadata := range metadataList {
		docs[i] = metadata
	}

	_, err := collection.InsertMany(ctx, docs)
	return err
}

// DeleteByQuery removes metadata matching specific criteria
func (r *MongoDBRepository) DeleteByQuery(ctx context.Context, entityName string, query map[string]interface{}) (int64, error) {
	collection := r.getCollection(entityName)

	// Build filter from query
	filter := bson.M{}
	for key, value := range query {
		filter[key] = value
	}

	result, err := collection.DeleteMany(ctx, filter)
	if err != nil {
		return 0, err
	}

	return result.DeletedCount, nil
}