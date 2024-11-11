package mongodb

import (
	"context"
	"errors"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"strings"
	"time"

	"github.com/LerianStudio/midaz/common"
	cn "github.com/LerianStudio/midaz/common/constant"

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
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.create_metadata")
	defer span.End()

	ctx, spanGetDB := tracer.Start(ctx, "mongodb.create_metadata.get_db")

	db, err := mmr.connection.GetDB(ctx)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanGetDB, "Failed to get database", err)

		return err
	}

	spanGetDB.End()

	coll := db.Database(strings.ToLower(mmr.Database)).Collection(strings.ToLower(collection))
	record := &m.MetadataMongoDBModel{}

	if err := record.FromEntity(metadata); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to convert metadata to model", err)

		return err
	}

	ctx, spanInsert := tracer.Start(ctx, "mongodb.create_metadata.insert")

	_, err = coll.InsertOne(ctx, record)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanInsert, "Failed to insert metadata", err)

		return err
	}

	spanInsert.End()

	return nil
}

// FindList retrieves metadata from the mongodb all metadata or a list by specify metadata.
func (mmr *MetadataMongoDBRepository) FindList(ctx context.Context, collection string, filter commonHTTP.QueryHeader) ([]*m.Metadata, error) {
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.find_list")
	defer span.End()

	db, err := mmr.connection.GetDB(ctx)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database", err)

		return nil, err
	}

	coll := db.Database(strings.ToLower(mmr.Database)).Collection(strings.ToLower(collection))

	opts := options.FindOptions{}

	if filter.UseMetadata {
		limit := int64(filter.Limit)
		skip := int64(filter.Page*filter.Limit - filter.Limit)
		opts = options.FindOptions{Limit: &limit, Skip: &skip}
	}

	ctx, spanFind := tracer.Start(ctx, "mongodb.find_list.find")

	cur, err := coll.Find(ctx, filter.Metadata, &opts)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanFind, "Failed to find metadata", err)

		return nil, err
	}

	spanFind.End()

	var meta []*m.MetadataMongoDBModel

	for cur.Next(ctx) {
		var record m.MetadataMongoDBModel
		if err := cur.Decode(&record); err != nil {
			mopentelemetry.HandleSpanError(&span, "Failed to decode metadata", err)

			return nil, err
		}

		meta = append(meta, &record)
	}

	if err := cur.Err(); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to iterate metadata", err)

		return nil, err
	}

	if err := cur.Close(ctx); err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to close cursor", err)

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
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.find_by_entity")
	defer span.End()

	db, err := mmr.connection.GetDB(ctx)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database", err)

		return nil, err
	}

	coll := db.Database(strings.ToLower(mmr.Database)).Collection(strings.ToLower(collection))

	var record m.MetadataMongoDBModel

	ctx, spanFindOne := tracer.Start(ctx, "mongodb.find_by_entity.find_one")

	if err = coll.FindOne(ctx, bson.M{"entity_id": id}).Decode(&record); err != nil {
		mopentelemetry.HandleSpanError(&spanFindOne, "Failed to find metadata by entity", err)

		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}

		return nil, err
	}

	spanFindOne.End()

	return record.ToEntity(), nil
}

// Update an metadata entity into mongodb.
func (mmr *MetadataMongoDBRepository) Update(ctx context.Context, collection, id string, metadata map[string]any) error {
	logger := common.NewLoggerFromContext(ctx)
	tracer := common.NewTracerFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.update_metadata")
	defer span.End()

	db, err := mmr.connection.GetDB(ctx)
	if err != nil {
		mopentelemetry.HandleSpanError(&span, "Failed to get database", err)

		return err
	}

	coll := db.Database(strings.ToLower(mmr.Database)).Collection(strings.ToLower(collection))
	opts := options.Update().SetUpsert(true)
	filter := bson.M{"entity_id": id}
	update := bson.D{{Key: "$set", Value: bson.D{{Key: "metadata", Value: metadata}, {Key: "updated_at", Value: time.Now()}}}}

	ctx, spanUpdate := tracer.Start(ctx, "mongodb.update_metadata.update_one")

	updated, err := coll.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		mopentelemetry.HandleSpanError(&spanUpdate, "Failed to update metadata", err)

		if errors.Is(err, mongo.ErrNoDocuments) {
			return common.ValidateBusinessError(cn.ErrEntityNotFound, collection)
		}

		return err
	}

	spanUpdate.End()

	if updated.ModifiedCount > 0 {
		logger.Infoln("updated a document with entity_id: ", id)
	}

	return nil
}

// Delete an metadata entity into mongodb.
func (mmr *MetadataMongoDBRepository) Delete(ctx context.Context, collection, id string) error {
	db, err := mmr.connection.GetDB(ctx)
	if err != nil {
		return err
	}

	logger := common.NewLoggerFromContext(ctx)

	opts := options.Delete()

	coll := db.Database(strings.ToLower(mmr.Database)).Collection(strings.ToLower(collection))

	deleted, err := coll.DeleteOne(ctx, bson.D{{Key: "entity_id", Value: id}}, opts)
	if err != nil {
		return err
	}

	if deleted.DeletedCount > 0 {
		logger.Infoln("deleted a document with entity_id: ", id)
	}

	return nil
}
