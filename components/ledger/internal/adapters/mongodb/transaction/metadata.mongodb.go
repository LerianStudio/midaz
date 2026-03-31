// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mongodb

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libMongo "github.com/LerianStudio/lib-commons/v4/commons/mongo"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	tmcore "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/core"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/LerianStudio/midaz/v3/pkg/repository"
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
	CreateBulk(ctx context.Context, collection string, metadata []*Metadata) (*repository.MongoDBBulkInsertResult, error)
	FindList(ctx context.Context, collection string, filter http.QueryHeader) ([]*Metadata, error)
	FindByEntity(ctx context.Context, collection, id string) (*Metadata, error)
	FindByEntityIDs(ctx context.Context, collection string, entityIDs []string) ([]*Metadata, error)
	Update(ctx context.Context, collection, id string, metadata map[string]any) error
	UpdateBulk(ctx context.Context, collection string, updates []MetadataBulkUpdate) (*repository.MongoDBBulkUpdateResult, error)
	Delete(ctx context.Context, collection, id string) error
	CreateIndex(ctx context.Context, collection string, input *mmodel.CreateMetadataIndexInput) (*mmodel.MetadataIndex, error)
	FindAllIndexes(ctx context.Context, collection string) ([]*mmodel.MetadataIndex, error)
	DeleteIndex(ctx context.Context, collection, indexName string) error
}

// MetadataMongoDBRepository is a MongoDD-specific implementation of the MetadataRepository.
type MetadataMongoDBRepository struct {
	connection    *libMongo.Client
	requireTenant bool
}

// NewMetadataMongoDBRepository returns a new instance of MetadataMongoDBLRepository using the given MongoDB connection.
func NewMetadataMongoDBRepository(mc *libMongo.Client, requireTenant ...bool) *MetadataMongoDBRepository {
	r := &MetadataMongoDBRepository{connection: mc}
	if len(requireTenant) > 0 {
		r.requireTenant = requireTenant[0]
	}

	// Connection is validated per-request via getDatabase(ctx).
	// In multi-tenant mode, static connection can be nil when only
	// context-injected tenant DB is expected.
	return r
}

// getDatabase resolves the MongoDB database for the current request.
// In multi-tenant mode, the middleware injects a tenant-specific *mongo.Database into context.
// In single-tenant mode (or when no tenant context exists), falls back to the static connection.
func (mmr *MetadataMongoDBRepository) getDatabase(ctx context.Context) (*mongo.Database, error) {
	// Module-specific database (from middleware WithModule)
	if db := tmcore.GetMBContext(ctx, constant.ModuleTransaction); db != nil {
		return db, nil
	}

	// Generic database fallback (single-module services)
	if db := tmcore.GetMBContext(ctx); db != nil {
		return db, nil
	}

	if mmr.requireTenant {
		return nil, fmt.Errorf("tenant mongo database missing from context")
	}

	if mmr.connection == nil {
		return nil, fmt.Errorf("mongo connection is nil")
	}

	return mmr.connection.Database(ctx)
}

// Create inserts a new metadata entity into mongodb using upsert for idempotency.
// If metadata for the same entity_id and entity_name already exists, the operation is a no-op.
// This ensures that duplicate calls (e.g., from retries or bulk processing) do not create duplicate documents.
func (mmr *MetadataMongoDBRepository) Create(ctx context.Context, collection string, metadata *Metadata) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.create_metadata")
	defer span.End()

	db, err := mmr.getDatabase(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return err
	}

	coll := db.Collection(strings.ToLower(collection))
	record := &MetadataMongoDBModel{}

	if err := record.FromEntity(metadata); err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to convert metadata to model", err)

		return err
	}

	ctx, spanUpsert := tracer.Start(ctx, "mongodb.create_metadata.upsert")

	// Use upsert with $setOnInsert to ensure idempotency:
	// - If no document exists for this entity_id, insert the full record
	// - If a document already exists, do nothing (no update)
	filter := bson.M{
		"entity_id":   metadata.EntityID,
		"entity_name": metadata.EntityName,
	}

	update := bson.M{
		"$setOnInsert": record,
	}

	opts := options.Update().SetUpsert(true)

	upsertResult, err := coll.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanUpsert, "Failed to upsert metadata", err)

		return err
	}

	spanUpsert.End()

	if upsertResult.UpsertedCount > 0 {
		logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Inserted metadata document for entity %s", metadata.EntityID))
	} else {
		logger.Log(ctx, libLog.LevelDebug, fmt.Sprintf("Metadata already exists for entity %s, skipped insert", metadata.EntityID))
	}

	return nil
}

// CreateBulk inserts multiple metadata entities into mongodb using BulkWrite with upsert semantics.
// Documents are sorted by EntityID before insert to prevent deadlock-like contention in concurrent scenarios.
// Large batches are chunked (1000 docs/chunk) to stay within MongoDB's BSON document size limits.
// Returns MongoDBBulkInsertResult with counts of attempted, inserted, and matched (duplicate) documents.
//
// NOTE: Uses $setOnInsert with upsert to ensure idempotency - if a document exists, it's not modified.
// NOTE: The input slice is sorted in-place by EntityID. Callers should not rely on original order.
func (mmr *MetadataMongoDBRepository) CreateBulk(ctx context.Context, collection string, metadata []*Metadata) (*repository.MongoDBBulkInsertResult, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.create_bulk_metadata")
	defer span.End()

	// Early return for empty input
	if len(metadata) == 0 {
		return &repository.MongoDBBulkInsertResult{}, nil
	}

	// Validate no nil elements
	for i, m := range metadata {
		if m == nil {
			err := fmt.Errorf("nil metadata at index %d", i)
			libOpentelemetry.HandleSpanError(span, "Invalid input: nil metadata", err)

			return nil, err
		}
	}

	// Sort by EntityID to prevent deadlock-like contention
	sort.Slice(metadata, func(i, j int) bool {
		return metadata[i].EntityID < metadata[j].EntityID
	})

	db, err := mmr.getDatabase(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return nil, err
	}

	coll := db.Collection(strings.ToLower(collection))

	result := &repository.MongoDBBulkInsertResult{
		Attempted:   0,
		InsertedIDs: make([]string, 0, len(metadata)),
	}

	// Chunk into batches of 1000 to stay within MongoDB limits
	const chunkSize = 1000

	for i := 0; i < len(metadata); i += chunkSize {
		// Check for context cancellation between chunks
		select {
		case <-ctx.Done():
			libOpentelemetry.HandleSpanError(span, "Context cancelled during bulk insert", ctx.Err())
			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Context cancelled during bulk insert: %v", ctx.Err()))

			return result, ctx.Err()
		default:
		}

		end := min(i+chunkSize, len(metadata))
		result.Attempted += int64(end - i)

		chunkResult, err := mmr.insertMetadataChunk(ctx, coll, metadata[i:end])
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to insert metadata chunk", err)
			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to insert metadata chunk: %v", err))

			return result, err
		}

		result.Inserted += chunkResult.inserted
		result.Matched += chunkResult.matched
		result.InsertedIDs = append(result.InsertedIDs, chunkResult.insertedIDs...)
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf(
		"Bulk insert metadata: attempted=%d, inserted=%d, matched=%d",
		result.Attempted, result.Inserted, result.Matched,
	))

	return result, nil
}

// metadataChunkResult holds the result of inserting a chunk of metadata documents.
type metadataChunkResult struct {
	inserted    int64
	matched     int64
	insertedIDs []string
}

// insertMetadataChunk inserts a chunk of metadata using MongoDB BulkWrite.
// Uses UpdateOne with $setOnInsert and upsert:true for idempotency.
func (mmr *MetadataMongoDBRepository) insertMetadataChunk(ctx context.Context, coll *mongo.Collection, metadata []*Metadata) (*metadataChunkResult, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)
	_ = logger // silence unused warning - parent function handles logging

	ctx, span := tracer.Start(ctx, "mongodb.insert_metadata_chunk")
	defer span.End()

	if len(metadata) == 0 {
		return &metadataChunkResult{}, nil
	}

	// Build BulkWrite models
	models := make([]mongo.WriteModel, 0, len(metadata))

	for _, m := range metadata {
		record := &MetadataMongoDBModel{}
		if err := record.FromEntity(m); err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to convert metadata to model", err)

			return nil, err
		}

		// Use UpdateOne with upsert and $setOnInsert for idempotency
		filter := bson.M{
			"entity_id":   m.EntityID,
			"entity_name": m.EntityName,
		}

		update := bson.M{
			"$setOnInsert": record,
		}

		model := mongo.NewUpdateOneModel().
			SetFilter(filter).
			SetUpdate(update).
			SetUpsert(true)

		models = append(models, model)
	}

	// Execute BulkWrite with ordered:false for parallel execution
	opts := options.BulkWrite().SetOrdered(false)

	bulkResult, err := coll.BulkWrite(ctx, models, opts)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "BulkWrite failed", err)

		return nil, err
	}

	result := &metadataChunkResult{
		inserted:    bulkResult.UpsertedCount,
		matched:     bulkResult.MatchedCount,
		insertedIDs: make([]string, 0, bulkResult.UpsertedCount),
	}

	// Track which documents were actually inserted by checking UpsertedIDs indices
	for idx := range bulkResult.UpsertedIDs {
		if idx >= 0 && idx < int64(len(metadata)) {
			result.insertedIDs = append(result.insertedIDs, metadata[idx].EntityID)
		}
	}

	return result, nil
}

// UpdateBulk updates multiple metadata entities in mongodb using BulkWrite.
// Documents are sorted by EntityID before update to prevent deadlock-like contention.
// Large batches are chunked (1000 docs/chunk) to stay within MongoDB's BSON limits.
// Returns MongoDBBulkUpdateResult with counts of attempted, modified, and matched documents.
//
// NOTE: Uses upsert semantics - will insert new documents if no match is found, consistent with single Update method.
// NOTE: The input slice is sorted in-place by EntityID. Callers should not rely on original order.
func (mmr *MetadataMongoDBRepository) UpdateBulk(ctx context.Context, collection string, updates []MetadataBulkUpdate) (*repository.MongoDBBulkUpdateResult, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.update_bulk_metadata")
	defer span.End()

	// Early return for empty input
	if len(updates) == 0 {
		return &repository.MongoDBBulkUpdateResult{}, nil
	}

	// Sort by EntityID to prevent deadlock-like contention
	sort.Slice(updates, func(i, j int) bool {
		return updates[i].EntityID < updates[j].EntityID
	})

	db, err := mmr.getDatabase(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		return nil, err
	}

	coll := db.Collection(strings.ToLower(collection))

	result := &repository.MongoDBBulkUpdateResult{
		Attempted: 0,
	}

	// Chunk into batches of 1000
	const chunkSize = 1000

	for i := 0; i < len(updates); i += chunkSize {
		// Check for context cancellation between chunks
		select {
		case <-ctx.Done():
			libOpentelemetry.HandleSpanError(span, "Context cancelled during bulk update", ctx.Err())
			logger.Log(ctx, libLog.LevelWarn, fmt.Sprintf("Context cancelled during bulk update: %v", ctx.Err()))

			return result, ctx.Err()
		default:
		}

		end := min(i+chunkSize, len(updates))
		result.Attempted += int64(end - i)

		chunkModified, chunkMatched, err := mmr.updateMetadataChunk(ctx, coll, updates[i:end])
		if err != nil {
			libOpentelemetry.HandleSpanError(span, "Failed to update metadata chunk", err)
			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to update metadata chunk: %v", err))

			return result, err
		}

		result.Modified += chunkModified
		result.Matched += chunkMatched
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf(
		"Bulk update metadata: attempted=%d, modified=%d, matched=%d",
		result.Attempted, result.Modified, result.Matched,
	))

	return result, nil
}

// updateMetadataChunk updates a chunk of metadata using MongoDB BulkWrite.
func (mmr *MetadataMongoDBRepository) updateMetadataChunk(ctx context.Context, coll *mongo.Collection, updates []MetadataBulkUpdate) (int64, int64, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)
	_ = logger // silence unused warning - parent function handles logging

	ctx, span := tracer.Start(ctx, "mongodb.update_metadata_chunk")
	defer span.End()

	if len(updates) == 0 {
		return 0, 0, nil
	}

	// Build BulkWrite models
	models := make([]mongo.WriteModel, 0, len(updates))
	now := time.Now()

	for _, u := range updates {
		filter := bson.M{"entity_id": u.EntityID}

		update := bson.D{
			{Key: "$set", Value: bson.D{
				{Key: "metadata", Value: u.Data},
				{Key: "updated_at", Value: now},
			}},
		}

		model := mongo.NewUpdateOneModel().
			SetFilter(filter).
			SetUpdate(update).
			SetUpsert(true)

		models = append(models, model)
	}

	// Execute BulkWrite with ordered:false for parallel execution
	opts := options.BulkWrite().SetOrdered(false)

	bulkResult, err := coll.BulkWrite(ctx, models, opts)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "BulkWrite update failed", err)

		return 0, 0, err
	}

	return bulkResult.ModifiedCount, bulkResult.MatchedCount, nil
}

// FindList retrieves metadata from the mongodb all metadata or a list by specify metadata.
func (mmr *MetadataMongoDBRepository) FindList(ctx context.Context, collection string, filter http.QueryHeader) ([]*Metadata, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.find_list")
	defer span.End()

	db, err := mmr.getDatabase(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database connection: %v", err))

		return nil, err
	}

	coll := db.Collection(strings.ToLower(collection))

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
		libOpentelemetry.HandleSpanError(spanFind, "Failed to find metadata", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to find metadata: %v", err))

		return nil, err
	}

	spanFind.End()

	var meta []*MetadataMongoDBModel

	for cur.Next(ctx) {
		var record MetadataMongoDBModel
		if err := cur.Decode(&record); err != nil {
			libOpentelemetry.HandleSpanError(spanFind, "Failed to decode metadata", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to decode metadata: %v", err))

			return nil, err
		}

		meta = append(meta, &record)
	}

	if err := cur.Err(); err != nil {
		libOpentelemetry.HandleSpanError(spanFind, "Failed to iterate metadata", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to iterate metadata: %v", err))

		return nil, err
	}

	if err := cur.Close(ctx); err != nil {
		libOpentelemetry.HandleSpanError(spanFind, "Failed to close cursor", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to close cursor: %v", err))

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

	db, err := mmr.getDatabase(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database: %v", err))

		return nil, err
	}

	coll := db.Collection(strings.ToLower(collection))

	var record MetadataMongoDBModel

	ctx, spanFindOne := tracer.Start(ctx, "mongodb.find_by_entity.find_one")

	if err = coll.FindOne(ctx, bson.M{"entity_id": id}).Decode(&record); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}

		libOpentelemetry.HandleSpanError(spanFindOne, "Failed to find metadata", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to find metadata: %v", err))

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

	db, err := mmr.getDatabase(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database connection", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database connection: %v", err))

		return nil, err
	}

	coll := db.Collection(strings.ToLower(collection))

	filter := bson.M{"entity_id": bson.M{"$in": entityIDs}}

	ctx, spanFind := tracer.Start(ctx, "mongodb.find_by_entity_ids.find")
	defer spanFind.End()

	cur, err := coll.Find(ctx, filter, options.Find())
	if err != nil {
		libOpentelemetry.HandleSpanError(spanFind, "Failed to find metadata", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to find metadata: %v", err))

		return nil, err
	}
	defer cur.Close(ctx)

	var meta []*MetadataMongoDBModel

	for cur.Next(ctx) {
		var record MetadataMongoDBModel
		if err := cur.Decode(&record); err != nil {
			libOpentelemetry.HandleSpanError(spanFind, "Failed to decode metadata", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to decode metadata: %v", err))

			return nil, err
		}

		meta = append(meta, &record)
	}

	if err := cur.Err(); err != nil {
		libOpentelemetry.HandleSpanError(spanFind, "Failed to iterate metadata", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to iterate metadata: %v", err))

		return nil, err
	}

	if err := cur.Close(ctx); err != nil {
		libOpentelemetry.HandleSpanError(spanFind, "Failed to close cursor", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to close cursor: %v", err))

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

	db, err := mmr.getDatabase(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)

		return err
	}

	coll := db.Collection(strings.ToLower(collection))
	opts := options.Update().SetUpsert(true)
	filter := bson.M{"entity_id": id}
	update := bson.D{{Key: "$set", Value: bson.D{{Key: "metadata", Value: metadata}, {Key: "updated_at", Value: time.Now()}}}}

	ctx, spanUpdate := tracer.Start(ctx, "mongodb.update_metadata.update_one")

	updated, err := coll.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanUpdate, "Failed to update metadata", err)

		if errors.Is(err, mongo.ErrNoDocuments) {
			return pkg.ValidateBusinessError(constant.ErrEntityNotFound, collection)
		}

		return err
	}

	spanUpdate.End()

	if updated.ModifiedCount > 0 {
		logger.Log(ctx, libLog.LevelInfo, fmt.Sprintln("updated a document with entity_id: ", id))
	}

	return nil
}

// Delete an metadata entity into mongodb.
func (mmr *MetadataMongoDBRepository) Delete(ctx context.Context, collection, id string) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.delete_metadata")
	defer span.End()

	db, err := mmr.getDatabase(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)

		return err
	}

	opts := options.Delete()

	coll := db.Collection(strings.ToLower(collection))

	ctx, spanDelete := tracer.Start(ctx, "mongodb.delete_metadata.delete_one")

	deleted, err := coll.DeleteOne(ctx, bson.D{{Key: "entity_id", Value: id}}, opts)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanDelete, "Failed to delete metadata", err)

		return err
	}

	spanDelete.End()

	if deleted.DeletedCount > 0 {
		logger.Log(ctx, libLog.LevelInfo, fmt.Sprintln("deleted a document with entity_id: ", id))
	}

	return nil
}

// CreateIndex creates an index on the mongodb.
func (mmr *MetadataMongoDBRepository) CreateIndex(ctx context.Context, collection string, input *mmodel.CreateMetadataIndexInput) (*mmodel.MetadataIndex, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.create_index")
	defer span.End()

	db, err := mmr.getDatabase(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)

		return nil, err
	}

	coll := db.Collection(strings.ToLower(collection))

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
		libOpentelemetry.HandleSpanError(spanCreateIndex, "Failed to create index", err)

		return nil, err
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Created index %s on collection %s", indexName, collection))

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

	db, err := mmr.getDatabase(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)

		return nil, err
	}

	coll := db.Collection(strings.ToLower(collection))

	// First, get index stats via aggregation
	ctx, spanStats := tracer.Start(ctx, "mongodb.find_all_indexes.stats")

	statsCur, err := coll.Aggregate(ctx, mongo.Pipeline{
		{{Key: "$indexStats", Value: bson.D{}}},
	})
	if err != nil {
		libOpentelemetry.HandleSpanError(spanStats, "Failed to get index stats", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get index stats: %v", err))

		return nil, err
	}

	defer func() {
		if closeErr := statsCur.Close(ctx); closeErr != nil {
			libOpentelemetry.HandleSpanError(spanStats, "Failed to close stats cursor", closeErr)
			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to close stats cursor: %v", closeErr))
		}
	}()

	// Build a map of index name -> stats
	indexStatsMap := make(map[string]*mmodel.IndexStats)

	for statsCur.Next(ctx) {
		var stats MongoDBIndexStats
		if err := statsCur.Decode(&stats); err != nil {
			libOpentelemetry.HandleSpanError(spanStats, "Failed to decode index stats", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to decode index stats: %v", err))

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
		libOpentelemetry.HandleSpanError(spanFind, "Failed to find indexes", err)

		return nil, err
	}

	defer func() {
		if closeErr := cur.Close(ctx); closeErr != nil {
			libOpentelemetry.HandleSpanError(spanFind, "Failed to close cursor", closeErr)
			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to close cursor: %v", closeErr))
		}
	}()

	var metadataIndexes []*mmodel.MetadataIndex

	const metadataPrefix = "metadata."

	for cur.Next(ctx) {
		var record MongoDBIndexInfo

		if err := cur.Decode(&record); err != nil {
			libOpentelemetry.HandleSpanError(spanFind, "Failed to decode metadata index", err)

			logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to decode metadata index: %v", err))

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
		libOpentelemetry.HandleSpanError(spanFind, "Failed to iterate metadata indexes", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to iterate metadata indexes: %v", err))

		return nil, err
	}

	return metadataIndexes, nil
}

// DeleteIndex deletes an index from the mongodb.
func (mmr *MetadataMongoDBRepository) DeleteIndex(ctx context.Context, collection, indexName string) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.delete_index")
	defer span.End()

	db, err := mmr.getDatabase(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(span, "Failed to get database", err)

		logger.Log(ctx, libLog.LevelError, fmt.Sprintf("Failed to get database: %v", err))

		return err
	}

	coll := db.Collection(strings.ToLower(collection))

	ctx, spanDelete := tracer.Start(ctx, "mongodb.delete_index.delete_one")
	defer spanDelete.End()

	_, err = coll.Indexes().DropOne(ctx, indexName)
	if err != nil {
		libOpentelemetry.HandleSpanError(spanDelete, "Failed to delete index", err)

		var cmdErr mongo.CommandError
		if errors.As(err, &cmdErr) && cmdErr.Name == "IndexNotFound" {
			return pkg.ValidateBusinessError(constant.ErrMetadataIndexNotFound, "metadata_index")
		}

		return err
	}

	logger.Log(ctx, libLog.LevelInfo, fmt.Sprintf("Deleted index %s on collection %s", indexName, collection))

	return nil
}
