// Package mongodb provides MongoDB adapter implementations for the onboarding service.
// This file contains the repository implementation for metadata storage.
package mongodb

import (
	"context"
	"errors"
	"strings"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libMongo "github.com/LerianStudio/lib-commons/v2/commons/mongo"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Repository provides an interface for metadata entity persistence operations in MongoDB.
//
// This interface defines the contract for metadata data access, following the Repository
// pattern. It abstracts MongoDB operations from the business logic layer.
//
// Collection Names:
//   - Each entity type has its own collection (e.g., "organization", "account", "ledger")
//   - Collection names are passed as parameters for flexibility
//
// All methods:
//   - Accept context for tracing, logging, and cancellation
//   - Use entity_id (UUID) as the primary lookup key
//   - Support flexible metadata queries
//
//go:generate mockgen --destination=metadata.mongodb_mock.go --package=mongodb . Repository
type Repository interface {
	// Create inserts new metadata for an entity.
	// Creates a new document with entity_id, entity_name, and metadata fields.
	Create(ctx context.Context, collection string, metadata *Metadata) error

	// FindList retrieves metadata documents with optional filtering.
	// Supports metadata-based queries and pagination when UseMetadata is true.
	// Returns empty array if no documents found (not an error).
	FindList(ctx context.Context, collection string, filter http.QueryHeader) ([]*Metadata, error)

	// FindByEntity retrieves metadata for a single entity by entity_id.
	// Returns nil (not error) if metadata doesn't exist.
	// Used to check if metadata exists before operations.
	FindByEntity(ctx context.Context, collection, id string) (*Metadata, error)

	// FindByEntityIDs retrieves metadata for multiple entities by their entity_ids.
	// Used for batch metadata enrichment of entity lists.
	// Returns only metadata that exists (may be fewer than requested).
	FindByEntityIDs(ctx context.Context, collection string, entityIDs []string) ([]*Metadata, error)

	// Update modifies metadata for an entity using upsert semantics.
	// Creates the document if it doesn't exist (upsert: true).
	// Replaces the entire metadata map for an entity. Any merging logic must be handled by the caller.
	Update(ctx context.Context, collection, id string, metadata map[string]any) error

	// Delete removes metadata for an entity.
	// Does not return error if metadata doesn't exist (idempotent).
	Delete(ctx context.Context, collection, id string) error
}

// MetadataMongoDBRepository is a MongoDB implementation of the Repository interface.
//
// This struct provides concrete MongoDB-based persistence for entity metadata.
// It uses MongoDB's flexible document model to store schema-less metadata.
//
// Features:
//   - Upsert semantics for updates (creates if doesn't exist)
//   - Flexible querying by metadata fields
//   - Batch operations for performance
//   - OpenTelemetry tracing for all operations
//   - Structured logging with context
type MetadataMongoDBRepository struct {
	connection *libMongo.MongoConnection // MongoDB connection pool
	Database   string                    // Database name
}

// NewMetadataMongoDBRepository creates a new MongoDB metadata repository instance.
//
// This constructor initializes the repository with a MongoDB connection and verifies
// connectivity by attempting to get the database handle. If the connection fails,
// it panics to prevent the application from starting with a broken database connection.
//
// Parameters:
//   - mc: MongoDB connection from lib-commons
//
// Returns:
//   - *MetadataMongoDBRepository: Initialized repository
//
// Panics:
//   - If MongoDB connection cannot be established
//
// Note: Panicking in constructors is acceptable for critical dependencies.
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

// Create inserts new metadata for an entity into MongoDB.
//
// This method creates a new metadata document with the entity's UUID as the lookup key.
// The collection name determines which entity type the metadata belongs to.
//
// Collection names are lowercased automatically for consistency.
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - collection: Collection name (entity type, e.g., "Organization", "Account")
//   - metadata: Metadata to insert
//
// Returns:
//   - error: Database error if insertion fails
//
// OpenTelemetry: Creates span "mongodb.create_metadata"
func (mmr *MetadataMongoDBRepository) Create(ctx context.Context, collection string, metadata *Metadata) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.create_metadata")
	defer span.End()

	db, err := mmr.connection.GetDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database", err)

		logger.Errorf("Failed to get database: %v", err)

		return err
	}

	coll := db.Database(strings.ToLower(mmr.Database)).Collection(strings.ToLower(collection))
	record := &MetadataMongoDBModel{}

	if err := record.FromEntity(metadata); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to convert metadata to model", err)

		logger.Errorf("Failed to convert metadata to model: %v", err)

		return err
	}

	ctx, spanInsert := tracer.Start(ctx, "mongodb.create_metadata.insert")

	_, err = coll.InsertOne(ctx, record)
	if err != nil {
		libOpentelemetry.HandleSpanError(&spanInsert, "Failed to insert metadata", err)

		logger.Errorf("Failed to insert metadata: %v", err)

		return err
	}

	spanInsert.End()

	return nil
}

// FindList retrieves metadata documents with optional filtering and pagination.
//
// This method supports two query modes:
// 1. Metadata-based filtering: When UseMetadata is true, queries by metadata fields with pagination
// 2. All documents: When UseMetadata is false, returns all documents (no pagination)
//
// The filter.Metadata field contains BSON query conditions for metadata-based searches.
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - collection: Collection name (entity type)
//   - filter: Query parameters including metadata filters and pagination
//
// Returns:
//   - []*Metadata: Array of metadata documents (empty if none found)
//   - error: Database error if query fails
//
// OpenTelemetry: Creates span "mongodb.find_list"
func (mmr *MetadataMongoDBRepository) FindList(ctx context.Context, collection string, filter http.QueryHeader) ([]*Metadata, error) {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.find_list")
	defer span.End()

	db, err := mmr.connection.GetDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database", err)

		logger.Errorf("Failed to get database: %v", err)

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
		libOpentelemetry.HandleSpanError(&spanFind, "Failed to find metadata", err)

		logger.Errorf("Failed to find metadata: %v", err)

		return nil, err
	}

	spanFind.End()

	var meta []*MetadataMongoDBModel

	for cur.Next(ctx) {
		var record MetadataMongoDBModel
		if err := cur.Decode(&record); err != nil {
			libOpentelemetry.HandleSpanError(&span, "Failed to decode metadata", err)

			logger.Errorf("Failed to decode metadata: %v", err)

			return nil, err
		}

		meta = append(meta, &record)
	}

	if err := cur.Err(); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to iterate metadata", err)

		logger.Errorf("Failed to iterate metadata: %v", err)

		return nil, err
	}

	if err := cur.Close(ctx); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to close cursor", err)

		logger.Errorf("Failed to close cursor: %v", err)

		return nil, err
	}

	metadata := make([]*Metadata, 0, len(meta))
	for i := range meta {
		metadata = append(metadata, meta[i].ToEntity())
	}

	return metadata, nil
}

// FindByEntity retrieves metadata for a single entity by entity_id.
//
// This method returns nil (not an error) if the metadata document doesn't exist.
// This behavior allows callers to distinguish between "no metadata" and "query failed".
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - collection: Collection name (entity type)
//   - id: Entity UUID to look up
//
// Returns:
//   - *Metadata: Metadata document, or nil if not found
//   - error: Database error if query fails (not returned for "not found")
//
// OpenTelemetry: Creates span "mongodb.find_by_entity"
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

		libOpentelemetry.HandleSpanError(&spanFindOne, "Failed to find metadata by entity", err)

		logger.Errorf("Failed to find metadata by entity: %v", err)

		return nil, err
	}

	spanFindOne.End()

	return record.ToEntity(), nil
}

// FindByEntityIDs retrieves metadata for multiple entities by their entity_ids.
//
// This method uses MongoDB's $in operator for efficient batch retrieval.
// It returns only metadata that exists (may be fewer documents than requested IDs).
// Returns empty array if entityIDs is empty (optimization to avoid query).
//
// Use Cases:
//   - Batch metadata enrichment for list queries
//   - Fetching metadata for multiple entities in one query
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - collection: Collection name (entity type)
//   - entityIDs: Array of entity UUIDs to retrieve metadata for
//
// Returns:
//   - []*Metadata: Array of found metadata (may be fewer than requested)
//   - error: Database error if query fails
//
// OpenTelemetry: Creates span "mongodb.find_by_entity_ids"
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

// Update modifies metadata for an entity using upsert semantics.
//
// This method uses MongoDB's upsert option to create the document if it doesn't exist.
// The update operation:
// 1. Sets the metadata field to the provided map (replaces existing metadata)
// 2. Updates the updated_at timestamp
// 3. Creates the document if entity_id doesn't exist (upsert: true)
//
// Note: This replaces the entire metadata map, not merging. Merging is done at the
// service layer before calling this method.
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - collection: Collection name (entity type)
//   - id: Entity UUID
//   - metadata: Complete metadata map to set
//
// Returns:
//   - error: Database error if update fails
//
// OpenTelemetry: Creates span "mongodb.update_metadata"
func (mmr *MetadataMongoDBRepository) Update(ctx context.Context, collection, id string, metadata map[string]any) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "mongodb.update_metadata")
	defer span.End()

	db, err := mmr.connection.GetDB(ctx)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to get database", err)

		logger.Errorf("Failed to get database: %v", err)

		return err
	}

	coll := db.Database(strings.ToLower(mmr.Database)).Collection(strings.ToLower(collection))
	opts := options.Update().SetUpsert(true)
	filter := bson.M{"entity_id": id}
	update := bson.D{{Key: "$set", Value: bson.D{{Key: "metadata", Value: metadata}, {Key: "updated_at", Value: time.Now()}}}}

	ctx, spanUpdate := tracer.Start(ctx, "mongodb.update_metadata.update_one")

	updated, err := coll.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			err := pkg.ValidateBusinessError(constant.ErrEntityNotFound, collection)

			libOpentelemetry.HandleSpanBusinessErrorEvent(&spanUpdate, "Failed to update metadata", err)

			logger.Warnf("Failed to update metadata: %v", err)

			return err
		}

		libOpentelemetry.HandleSpanError(&spanUpdate, "Failed to update metadata", err)

		logger.Errorf("Failed to update metadata: %v", err)

		return err
	}

	spanUpdate.End()

	if updated.ModifiedCount > 0 {
		logger.Infoln("updated a document with entity_id: ", id)
	}

	return nil
}

// Delete removes metadata for an entity from MongoDB.
//
// This method performs a hard delete (physically removes the document).
// It's idempotent - doesn't return an error if the document doesn't exist.
//
// Parameters:
//   - ctx: Context for tracing, logging, and cancellation
//   - collection: Collection name (entity type)
//   - id: Entity UUID
//
// Returns:
//   - error: Database error if deletion fails (not returned for "not found")
//
// OpenTelemetry: Creates span "mongodb.delete_metadata"
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
