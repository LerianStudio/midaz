package mongodb

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/LerianStudio/midaz/common"

	"github.com/LerianStudio/midaz/common/mmongo"
	commonHTTP "github.com/LerianStudio/midaz/common/net/http"
	m "github.com/LerianStudio/midaz/components/ledger/internal/domain/metadata"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MetadataMongoDBRepository is a MongoDD-specific implementation of the MetadataRepository.
type MetadataMongoDBRepository struct {
	connection *mmongo.MongoConnection
	Database   string
}

// NewMetadataMongoDBRepository returns a new instance of MetadataMongoDBLRepository using the given MongoDB connection.
func NewMetadataMongoDBRepository(mc *mmongo.MongoConnection) *MetadataMongoDBRepository {
	r := &MetadataMongoDBRepository{
		connection: mc,
		Database:   mc.Database,
	}
	if _, err := r.connection.GetDB(context.Background()); err != nil {
		panic("Failed to connect mongodb")
	}

	return r
}

// Create inserts a new metadata entity into mongodb.
func (mmr *MetadataMongoDBRepository) Create(ctx context.Context, collection string, metadata *m.Metadata) error {
	db, err := mmr.connection.GetDB(ctx)
	if err != nil {
		return err
	}

	coll := db.Database(strings.ToLower(mmr.Database)).Collection(strings.ToLower(collection))
	record := &m.MetadataMongoDBModel{}

	if err := record.FromEntity(metadata); err != nil {
		return err
	}

	_, err = coll.InsertOne(ctx, record)
	if err != nil {
		return err
	}

	return nil
}

// FindList retrieves metadata from the mongodb all metadata or a list by specify metadata.
func (mmr *MetadataMongoDBRepository) FindList(ctx context.Context, collection string, filter commonHTTP.QueryHeader) ([]*m.Metadata, error) {
	db, err := mmr.connection.GetDB(ctx)
	if err != nil {
		return nil, err
	}

	coll := db.Database(strings.ToLower(mmr.Database)).Collection(strings.ToLower(collection))

	opts := options.FindOptions{}

	if filter.UseMetadata {
		limit := int64(filter.Limit)
		skip := int64(filter.Page*filter.Limit - filter.Limit)
		opts = options.FindOptions{Limit: &limit, Skip: &skip}
	}

	cur, err := coll.Find(ctx, filter.Metadata, &opts)
	if err != nil {
		return nil, err
	}

	var meta []*m.MetadataMongoDBModel

	for cur.Next(ctx) {
		var record m.MetadataMongoDBModel
		if err := cur.Decode(&record); err != nil {
			return nil, err
		}

		meta = append(meta, &record)
	}

	if err := cur.Err(); err != nil {
		return nil, err
	}

	if err := cur.Close(ctx); err != nil {
		return nil, err
	}

	metadata := make([]*m.Metadata, 0, len(meta))
	for i := range meta {
		metadata = append(metadata, meta[i].ToEntity())
	}

	return metadata, nil
}

// FindByEntity retrieves a metadata from the mongodb using the provided entity_id.
func (mmr *MetadataMongoDBRepository) FindByEntity(ctx context.Context, collection, id string) (*m.Metadata, error) {
	db, err := mmr.connection.GetDB(ctx)
	if err != nil {
		return nil, err
	}

	coll := db.Database(strings.ToLower(mmr.Database)).Collection(strings.ToLower(collection))

	var record m.MetadataMongoDBModel
	if err = coll.FindOne(ctx, bson.M{"entity_id": id}).Decode(&record); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}

		return nil, err
	}

	return record.ToEntity(), nil
}

// Update an metadata entity into mongodb.
func (mmr *MetadataMongoDBRepository) Update(ctx context.Context, collection, id string, metadata map[string]any) error {
	db, err := mmr.connection.GetDB(ctx)
	if err != nil {
		return err
	}

	coll := db.Database(strings.ToLower(mmr.Database)).Collection(strings.ToLower(collection))
	opts := options.Update().SetUpsert(true)
	filter := bson.M{"entity_id": id}
	update := bson.D{{Key: "$set", Value: bson.D{{Key: "metadata", Value: metadata}, {Key: "updated_at", Value: time.Now()}}}}

	updated, err := coll.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return common.EntityNotFoundError{
				EntityType: collection,
				Code:       "0007",
				Title:      "Entity Not Found",
				Message:    "No entity was found for the given ID. Please make sure to use the correct ID for the entity you are trying to manage.",
				Err:        err,
			}
		}

		return err
	}

	if updated.ModifiedCount > 0 {
		fmt.Println("updated a document with entity_id: ", id)
	}

	return nil
}

// Delete an metadata entity into mongodb.
func (mmr *MetadataMongoDBRepository) Delete(ctx context.Context, collection, id string) error {
	db, err := mmr.connection.GetDB(ctx)
	if err != nil {
		return err
	}

	opts := options.Delete()

	coll := db.Database(strings.ToLower(mmr.Database)).Collection(strings.ToLower(collection))

	deleted, err := coll.DeleteOne(ctx, bson.D{{Key: "entity_id", Value: id}}, opts)
	if err != nil {
		return err
	}

	if deleted.DeletedCount > 0 {
		fmt.Println("deleted a document with entity_id: ", id)
	}

	return nil
}
