// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mongodb

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libMongo "github.com/LerianStudio/lib-commons/v2/commons/mongo"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Repository provides an interface for operations related on mongodb a metadata entities.
// It defines methods for creating, finding, updating, and deleting metadata entities.
//
//go:generate mockgen --destination=metadata.mongodb_mock.go --package=mongodb . Repository
type Repository interface {
	Create(ctx context.Context, collection string, metadata *Metadata) error
	FindList(ctx context.Context, collection string, filter http.QueryHeader) ([]*Metadata, error)
	FindByEntity(ctx context.Context, collection, id string) (*Metadata, error)
	FindByEntityIDs(ctx context.Context, collection string, entityIDs []string) ([]*Metadata, error)
	Update(ctx context.Context, collection, id string, metadata map[string]any) error
	Delete(ctx context.Context, collection, id string) error
	CreateIndex(ctx context.Context, collection string, input *mmodel.CreateMetadataIndexInput) (*mmodel.MetadataIndex, error)
	FindAllIndexes(ctx context.Context, collection string) ([]*mmodel.MetadataIndex, error)
	DeleteIndex(ctx context.Context, collection, indexName string) error
}

// MetadataMongoDBRepository is a MongoDD-specific implementation of the MetadataRepository.
type MetadataMongoDBRepository struct {
	connection *libMongo.MongoConnection
	Database   string
}

// NewMetadataMongoDBRepository returns a new instance of MetadataMongoDBLRepository using the given MongoDB connection.
func NewMetadataMongoDBRepository(mc *libMongo.MongoConnection) *MetadataMongoDBRepository {
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
func (mmr *MetadataMongoDBRepository) Create(ctx context.Context, collection string, metadata *Metadata) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.create_metadata")
	defer span.End()

	db, err := mmr.connection.GetDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		return err
	}

	coll := db.Database(strings.ToLower(mmr.Database)).Collection(strings.ToLower(collection))
	record := &MetadataMongoDBModel{}

	if err := record.FromEntity(metadata); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert metadata to model", err)

		return err
	}

	ctx, spanInsert := tracer.Start(ctx, "mongodb.create_metadata.insert")

	insertResult, err := coll.InsertOne(ctx, record)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanInsert, "Failed to insert metadata", err)

		return err
	}

	spanInsert.End()

	logger.Infoln("Inserted a document: ", insertResult.InsertedID)

	return nil
}

// FindList retrieves metadata from the mongodb all metadata or a list by specify metadata.
func (mmr *MetadataMongoDBRepository) FindList(ctx context.Context, collection string, filter http.QueryHeader) ([]*Metadata, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.find_list")
	defer span.End()

	db, err := mmr.connection.GetDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, err
	}

	coll := db.Database(strings.ToLower(mmr.Database)).Collection(strings.ToLower(collection))

	opts := options.Find()

	mongoFilter := bson.M{}

	if filter.Metadata != nil {
		for key, value := range *filter.Metadata {
			mongoFilter[key] = value
		}
	}

	if !filter.StartDate.IsZero() || !filter.EndDate.IsZero() {
		dateFilter := bson.M{}

		if !filter.StartDate.IsZero() {
			dateFilter["$gte"] = filter.StartDate
		}

		if !filter.EndDate.IsZero() {
			dateFilter["$lte"] = filter.EndDate
		}

		mongoFilter["created_at"] = dateFilter
	}

	ctx, spanFind := tracer.Start(ctx, "mongodb.find_list.find")

	cur, err := coll.Find(ctx, mongoFilter, opts)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanFind, "Failed to find metadata", err)

		logger.Errorf("Failed to find metadata: %v", err)

		return nil, err
	}

	spanFind.End()

	var meta []*MetadataMongoDBModel

	for cur.Next(ctx) {
		var record MetadataMongoDBModel
		if err := cur.Decode(&record); err != nil {
			libOpentelemetry.HandleSpanError(&spanFind, "Failed to decode metadata", err)

			logger.Errorf("Failed to decode metadata: %v", err)

			return nil, err
		}

		meta = append(meta, &record)
	}

	if err := cur.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&spanFind, "Failed to iterate metadata", err)

		logger.Errorf("Failed to iterate metadata: %v", err)

		return nil, err
	}

	if err := cur.Close(ctx); err != nil {
		libOpentelemetry.HandleSpanError(&spanFind, "Failed to close cursor", err)

		logger.Errorf("Failed to close cursor: %v", err)

		return nil, err
	}

	metadata := make([]*Metadata, 0, len(meta))
	for i := range meta {
		metadata = append(metadata, meta[i].ToEntity())
	}

	return metadata, nil
}

// FindByEntity retrieves a metadata from the mongodb using the provided entity_id.
func (mmr *MetadataMongoDBRepository) FindByEntity(ctx context.Context, collection, id string) (*Metadata, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.find_by_entity")
	defer span.End()

	db, err := mmr.connection.GetDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database", err)

		logger.Errorf("Failed to get database: %v", err)

		return nil, err
	}

	coll := db.Database(strings.ToLower(mmr.Database)).Collection(strings.ToLower(collection))

	var record MetadataMongoDBModel

	ctx, spanFindOne := tracer.Start(ctx, "mongodb.find_by_entity.find_one")

	if err = coll.FindOne(ctx, bson.M{"entity_id": id}).Decode(&record); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}

		libOpentelemetry.HandleSpanError(&spanFindOne, "Failed to find metadata", err)

		logger.Errorf("Failed to find metadata: %v", err)

		return nil, err
	}

	spanFindOne.End()

	return record.ToEntity(), nil
}

// FindByEntityIDs retrieves metadata from the mongodb using a list of entity_ids.
func (mmr *MetadataMongoDBRepository) FindByEntityIDs(ctx context.Context, collection string, entityIDs []string) ([]*Metadata, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.find_by_entity_ids")
	defer span.End()

	if len(entityIDs) == 0 {
		return []*Metadata{}, nil
	}

	db, err := mmr.connection.GetDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database connection", err)

		logger.Errorf("Failed to get database connection: %v", err)

		return nil, err
	}

	coll := db.Database(strings.ToLower(mmr.Database)).Collection(strings.ToLower(collection))

	filter := bson.M{"entity_id": bson.M{"$in": entityIDs}}

	ctx, spanFind := tracer.Start(ctx, "mongodb.find_by_entity_ids.find")
	defer spanFind.End()

	cur, err := coll.Find(ctx, filter, options.Find())
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanFind, "Failed to find metadata", err)

		logger.Errorf("Failed to find metadata: %v", err)

		return nil, err
	}
	defer cur.Close(ctx)

	var meta []*MetadataMongoDBModel

	for cur.Next(ctx) {
		var record MetadataMongoDBModel
		if err := cur.Decode(&record); err != nil {
			libOpentelemetry.HandleSpanError(&spanFind, "Failed to decode metadata", err)

			logger.Errorf("Failed to decode metadata: %v", err)

			return nil, err
		}

		meta = append(meta, &record)
	}

	if err := cur.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&spanFind, "Failed to iterate metadata", err)

		logger.Errorf("Failed to iterate metadata: %v", err)

		return nil, err
	}

	if err := cur.Close(ctx); err != nil {
		libOpentelemetry.HandleSpanError(&spanFind, "Failed to close cursor", err)

		logger.Errorf("Failed to close cursor: %v", err)

		return nil, err
	}

	metadata := make([]*Metadata, 0, len(meta))
	for i := range meta {
		metadata = append(metadata, meta[i].ToEntity())
	}

	return metadata, nil
}

// Update an metadata entity into mongodb.
func (mmr *MetadataMongoDBRepository) Update(ctx context.Context, collection, id string, metadata map[string]any) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.update_metadata")
	defer span.End()

	db, err := mmr.connection.GetDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database", err)

		return err
	}

	coll := db.Database(strings.ToLower(mmr.Database)).Collection(strings.ToLower(collection))
	opts := options.Update().SetUpsert(true)
	filter := bson.M{"entity_id": id}
	update := bson.D{{Key: "$set", Value: bson.D{{Key: "metadata", Value: metadata}, {Key: "updated_at", Value: time.Now()}}}}

	ctx, spanUpdate := tracer.Start(ctx, "mongodb.update_metadata.update_one")

	updated, err := coll.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanUpdate, "Failed to update metadata", err)

		if errors.Is(err, mongo.ErrNoDocuments) {
			return pkg.ValidateBusinessError(constant.ErrEntityNotFound, collection)
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
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.delete_metadata")
	defer span.End()

	db, err := mmr.connection.GetDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database", err)

		return err
	}

	opts := options.Delete()

	coll := db.Database(strings.ToLower(mmr.Database)).Collection(strings.ToLower(collection))

	ctx, spanDelete := tracer.Start(ctx, "mongodb.delete_metadata.delete_one")

	deleted, err := coll.DeleteOne(ctx, bson.D{{Key: "entity_id", Value: id}}, opts)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanDelete, "Failed to delete metadata", err)

		return err
	}

	spanDelete.End()

	if deleted.DeletedCount > 0 {
		logger.Infoln("deleted a document with entity_id: ", id)
	}

	return nil
}

// CreateIndex creates an index on the mongodb.
func (mmr *MetadataMongoDBRepository) CreateIndex(ctx context.Context, collection string, input *mmodel.CreateMetadataIndexInput) (*mmodel.MetadataIndex, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.create_index")
	defer span.End()

	db, err := mmr.connection.GetDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database", err)

		return nil, err
	}

	coll := db.Database(strings.ToLower(mmr.Database)).Collection(strings.ToLower(collection))

	indexName := fmt.Sprintf("metadata.%s", input.MetadataKey)

	sparse := true
	if input.Sparse != nil {
		sparse = *input.Sparse
	}

	opts := options.Index().
		SetUnique(input.Unique).
		SetSparse(sparse)

	ctx, spanCreateIndex := tracer.Start(ctx, "mongodb.create_index.create_one")
	defer spanCreateIndex.End()

	_, err = coll.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: indexName, Value: 1}},
		Options: opts,
	})
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanCreateIndex, "Failed to create index", err)

		return nil, err
	}

	logger.Infof("Created index %s on collection %s", indexName, collection)

	return &mmodel.MetadataIndex{
		IndexName:   fmt.Sprintf("%s_1", indexName),
		EntityName:  collection,
		MetadataKey: input.MetadataKey,
		Unique:      input.Unique,
		Sparse:      sparse,
	}, nil
}

// MongoDBIndexStats represents the structure returned by $indexStats aggregation.
type MongoDBIndexStats struct {
	Name     string `bson:"name"`
	Accesses struct {
		Ops   int64     `bson:"ops"`
		Since time.Time `bson:"since"`
	} `bson:"accesses"`
}

// FindAllIndexes retrieves all indexes from the mongodb with usage statistics.
func (mmr *MetadataMongoDBRepository) FindAllIndexes(ctx context.Context, collection string) ([]*mmodel.MetadataIndex, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.find_all_indexes")
	defer span.End()

	db, err := mmr.connection.GetDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database", err)

		return nil, err
	}

	coll := db.Database(strings.ToLower(mmr.Database)).Collection(strings.ToLower(collection))

	// First, get index stats via aggregation
	ctx, spanStats := tracer.Start(ctx, "mongodb.find_all_indexes.stats")

	statsCur, err := coll.Aggregate(ctx, mongo.Pipeline{
		{{Key: "$indexStats", Value: bson.D{}}},
	})
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanStats, "Failed to get index stats", err)

		logger.Errorf("Failed to get index stats: %v", err)

		return nil, err
	}

	defer func() {
		if closeErr := statsCur.Close(ctx); closeErr != nil {
			libOpentelemetry.HandleSpanError(&spanStats, "Failed to close stats cursor", closeErr)
			logger.Errorf("Failed to close stats cursor: %v", closeErr)
		}
	}()

	// Build a map of index name -> stats
	indexStatsMap := make(map[string]*mmodel.IndexStats)

	for statsCur.Next(ctx) {
		var stats MongoDBIndexStats
		if err := statsCur.Decode(&stats); err != nil {
			libOpentelemetry.HandleSpanError(&spanStats, "Failed to decode index stats", err)

			logger.Errorf("Failed to decode index stats: %v", err)

			return nil, err
		}

		statsSince := stats.Accesses.Since
		indexStatsMap[stats.Name] = &mmodel.IndexStats{
			Accesses:   stats.Accesses.Ops,
			StatsSince: &statsSince,
		}
	}

	spanStats.End()

	// Now get index definitions
	opts := options.ListIndexes()

	ctx, spanFind := tracer.Start(ctx, "mongodb.find_all_indexes.list")
	defer spanFind.End()

	cur, err := coll.Indexes().List(ctx, opts)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanFind, "Failed to find indexes", err)

		return nil, err
	}

	defer func() {
		if closeErr := cur.Close(ctx); closeErr != nil {
			libOpentelemetry.HandleSpanError(&spanFind, "Failed to close cursor", closeErr)
			logger.Errorf("Failed to close cursor: %v", closeErr)
		}
	}()

	var metadataIndexes []*mmodel.MetadataIndex

	const metadataPrefix = "metadata."

	for cur.Next(ctx) {
		var record MongoDBIndexInfo

		if err := cur.Decode(&record); err != nil {
			libOpentelemetry.HandleSpanError(&spanFind, "Failed to decode metadata index", err)

			logger.Errorf("Failed to decode metadata index: %v", err)

			return nil, err
		}

		for _, elem := range record.Key {
			// Only include indexes on nested metadata fields
			if !strings.HasPrefix(elem.Key, metadataPrefix) {
				continue
			}

			// Strip the "metadata." prefix from the key for user-friendly display
			metadataKey := strings.TrimPrefix(elem.Key, metadataPrefix)

			metadataIndexes = append(metadataIndexes, &mmodel.MetadataIndex{
				IndexName:   record.Name,
				EntityName:  collection,
				MetadataKey: metadataKey,
				Unique:      record.Unique,
				Sparse:      record.Sparse,
				Stats:       indexStatsMap[record.Name],
			})
		}
	}

	if err := cur.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&spanFind, "Failed to iterate metadata indexes", err)

		logger.Errorf("Failed to iterate metadata indexes: %v", err)

		return nil, err
	}

	return metadataIndexes, nil
}

// DeleteIndex deletes an index from the mongodb.
func (mmr *MetadataMongoDBRepository) DeleteIndex(ctx context.Context, collection, indexName string) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.delete_index")
	defer span.End()

	db, err := mmr.connection.GetDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database", err)

		logger.Errorf("Failed to get database: %v", err)

		return err
	}

	coll := db.Database(strings.ToLower(mmr.Database)).Collection(strings.ToLower(collection))

	ctx, spanDelete := tracer.Start(ctx, "mongodb.delete_index.delete_one")
	defer spanDelete.End()

	_, err = coll.Indexes().DropOne(ctx, indexName)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanDelete, "Failed to delete index", err)

		var cmdErr mongo.CommandError
		if errors.As(err, &cmdErr) && cmdErr.Name == "IndexNotFound" {
			return pkg.ValidateBusinessError(constant.ErrMetadataIndexNotFound, "metadata_index")
		}

		return err
	}

	logger.Infof("Deleted index %s on collection %s", indexName, collection)

	return nil
}
