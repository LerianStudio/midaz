package mongodb

import (
	"context"
	"errors"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libMongo "github.com/LerianStudio/lib-commons/commons/mongo"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/LerianStudio/midaz/pkg"
	"github.com/LerianStudio/midaz/pkg/constant"
	"github.com/LerianStudio/midaz/pkg/net/http"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"strings"
	"time"
)

// Repository provides an interface for operations related on mongodb a metadata entities.
// It defines methods for creating, finding, updating, and deleting metadata entities.
type Repository interface {
	Create(ctx context.Context, collection string, metadata *Metadata) error
	FindList(ctx context.Context, collection string, filter http.QueryHeader) ([]*Metadata, error)
	FindByEntity(ctx context.Context, collection, id string) (*Metadata, error)
	Update(ctx context.Context, collection, id string, metadata map[string]any) error
	Delete(ctx context.Context, collection, id string) error
}

// MetadataMongoDBRepository is a MongoDD-specific implementation of the MetadataRepository.
type MetadataMongoDBRepository struct {
	database *mongo.Database
}

// NewMetadataMongoDBRepository returns a new instance of MetadataMongoDBLRepository using the given MongoDB connection.
func NewMetadataMongoDBRepository(mc *libMongo.MongoConnection) *MetadataMongoDBRepository {
	db, err := mc.GetDB(context.Background())
	if err != nil {
		panic("Failed to connect mongodb")
	}

	return &MetadataMongoDBRepository{
		database: db.Database(strings.ToLower(mc.Database)),
	}
}

// Create inserts a new metadata entity into mongodb.
func (mmr *MetadataMongoDBRepository) Create(ctx context.Context, collection string, metadata *Metadata) error {
	tracer := libCommons.NewTracerFromContext(ctx)
	logger := libCommons.NewLoggerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.create_metadata")
	defer span.End()

	coll := mmr.database.Collection(strings.ToLower(collection))

	insertResult, err := coll.InsertOne(ctx, metadata.ToEntity())
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to insert metadata", err)

		logger.Errorf("Failed to insert metadata to collection %s: %v", collection, err)

		return err
	}

	logger.Infoln("Inserted a document: ", insertResult.InsertedID)

	return nil
}

// FindList retrieves metadata from the mongodb all metadata or a list by specify metadata.
func (mmr *MetadataMongoDBRepository) FindList(ctx context.Context, collection string, filter http.QueryHeader) ([]*Metadata, error) {
	tracer := libCommons.NewTracerFromContext(ctx)
	logger := libCommons.NewLoggerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.find_list")
	defer span.End()

	coll := mmr.database.Collection(strings.ToLower(collection))

	cur, err := coll.Find(ctx, filter.Metadata, options.Find())
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to find metadata", err)

		logger.Errorf("Failed to find metadata to collection %s: %v", collection, err)

		return nil, err
	}

	metadata := make([]*Metadata, 0)

	for cur.Next(ctx) {
		var record MetadataMongoDBModel
		if err := cur.Decode(&record); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to decode metadata", err)

			return nil, err
		}

		metadata = append(metadata, record.ToDTO())
	}

	if err := cur.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate metadata", err)

		logger.Errorf("Failed to iterate metadata to collection %s: %v", collection, err)

		return nil, err
	}

	if err := cur.Close(ctx); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to close cursor", err)

		logger.Errorf("Failed to close cursor to collection %s: %v", collection, err)

		return nil, err
	}

	return metadata, nil
}

// FindByEntity retrieves a metadata from the mongodb using the provided entity_id.
func (mmr *MetadataMongoDBRepository) FindByEntity(ctx context.Context, collection, id string) (*Metadata, error) {
	tracer := libCommons.NewTracerFromContext(ctx)
	logger := libCommons.NewLoggerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.find_by_entity")
	defer span.End()

	coll := mmr.database.Collection(strings.ToLower(collection))

	var record MetadataMongoDBModel

	if err := coll.FindOne(ctx, bson.M{"entity_id": id}).Decode(&record); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to find metadata", err)

		logger.Errorf("Failed to find metadata by entity to collection %s: %v", collection, err)

		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}

		return nil, err
	}

	return record.ToDTO(), nil
}

// Update an metadata entity into mongodb.
func (mmr *MetadataMongoDBRepository) Update(ctx context.Context, collection, id string, metadata map[string]any) error {
	tracer := libCommons.NewTracerFromContext(ctx)
	logger := libCommons.NewLoggerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.update_metadata")
	defer span.End()

	coll := mmr.database.Collection(strings.ToLower(collection))

	opts := options.Update().SetUpsert(true)
	filter := bson.M{"entity_id": id}
	update := bson.D{{Key: "$set", Value: bson.D{{Key: "metadata", Value: metadata}, {Key: "updated_at", Value: time.Now()}}}}

	updated, err := coll.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to update metadata", err)

		logger.Errorf("Failed to update metadata to collection %s: %v", collection, err)

		if errors.Is(err, mongo.ErrNoDocuments) {
			return pkg.ValidateBusinessError(constant.ErrEntityNotFound, collection)
		}

		return err
	}

	if updated.ModifiedCount > 0 {
		logger.Infoln("updated a document with entity_id: ", id)
	}

	return nil
}

// Delete an metadata entity into mongodb.
func (mmr *MetadataMongoDBRepository) Delete(ctx context.Context, collection, id string) error {
	tracer := libCommons.NewTracerFromContext(ctx)
	logger := libCommons.NewLoggerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.delete_metadata")
	defer span.End()

	coll := mmr.database.Collection(strings.ToLower(collection))

	deleted, err := coll.DeleteOne(ctx, bson.D{{Key: "entity_id", Value: id}}, options.Delete())
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to delete metadata", err)

		return err
	}

	if deleted.DeletedCount > 0 {
		logger.Infoln("deleted a document with entity_id: ", id)
	}

	return nil
}
