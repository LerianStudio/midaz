// Package mongodb provides the MongoDB repository implementation for metadata storage.
//
// This file contains the Repository interface and MetadataMongoDBRepository implementation,
// providing CRUD operations for entity metadata in MongoDB.
//
// Key Features:
//   - Dynamic collection routing based on entity type
//   - Flexible querying with metadata field filters
//   - Pagination support for large result sets
//   - Upsert semantics for updates (create if not exists)
//   - Atomic document operations
//
// Collection Naming:
// Collections are named after entity types in lowercase:
//   - "account" for Account metadata
//   - "ledger" for Ledger metadata
//   - "organization" for Organization metadata
//
// Query Patterns:
//
//	┌─────────────────────────────────────────────────────────────┐
//	│                   Metadata Queries                          │
//	├─────────────────────────────────────────────────────────────┤
//	│  FindByEntity(id)       → Single metadata by entity ID      │
//	│  FindByEntityIDs(ids)   → Batch lookup by entity IDs        │
//	│  FindList(filter)       → Filtered list with pagination     │
//	│  Create(metadata)       → Insert new metadata document      │
//	│  Update(id, data)       → Upsert metadata fields            │
//	│  Delete(id)             → Remove metadata document          │
//	└─────────────────────────────────────────────────────────────┘
//
// Observability:
// All methods create OpenTelemetry spans for distributed tracing.
// Span names follow the pattern "mongodb.{operation}" (e.g., "mongodb.find_by_entity").
//
// Related Files:
//   - metadata.go: Contains MetadataMongoDBModel and domain Metadata types
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

// Repository defines the interface for metadata persistence operations.
//
// This interface follows the repository pattern, abstracting MongoDB operations
// behind a clean interface. The collection parameter enables dynamic routing
// to entity-specific collections.
//
// Collection Parameter:
// All methods accept a collection name (entity type) which routes operations
// to the correct MongoDB collection. The collection name is converted to
// lowercase internally.
//
// Thread Safety:
// Implementations must be safe for concurrent use. The MongoDB implementation
// uses the official Go driver which handles connection pooling internally.
//
// Mock Generation:
//
//go:generate mockgen --destination=metadata.mongodb_mock.go --package=mongodb . Repository
type Repository interface {
	// Create inserts a new metadata document into the specified collection.
	// The metadata.EntityID must be unique within the collection.
	Create(ctx context.Context, collection string, metadata *Metadata) error

	// FindList retrieves metadata documents matching the filter criteria.
	// Supports pagination via filter.Page and filter.Limit when UseMetadata is true.
	// The filter.Metadata field contains MongoDB query operators.
	FindList(ctx context.Context, collection string, filter http.QueryHeader) ([]*Metadata, error)

	// FindByEntity retrieves a single metadata document by entity ID.
	// Returns nil, nil if no document exists (not an error).
	FindByEntity(ctx context.Context, collection, id string) (*Metadata, error)

	// FindByEntityIDs retrieves metadata for multiple entities in a single query.
	// Returns empty slice if no documents found. Uses $in operator for efficiency.
	FindByEntityIDs(ctx context.Context, collection string, entityIDs []string) ([]*Metadata, error)

	// Update performs an upsert on metadata fields for the specified entity.
	// Creates the document if it doesn't exist (upsert: true).
	// Only updates the metadata field and updated_at timestamp.
	Update(ctx context.Context, collection, id string, metadata map[string]any) error

	// Delete removes the metadata document for the specified entity.
	// No error is returned if the document doesn't exist.
	Delete(ctx context.Context, collection, id string) error
}

// MetadataMongoDBRepository is the MongoDB implementation of the Repository interface.
//
// This repository manages metadata documents in MongoDB, using a separate collection
// per entity type. It uses the official MongoDB Go driver for all operations.
//
// Database Structure:
//
//	database: {Database}
//	├── account/          # Account metadata
//	├── ledger/           # Ledger metadata
//	├── organization/     # Organization metadata
//	└── ...               # Other entity types
//
// Connection Management:
// The repository holds a MongoConnection reference which manages the connection
// pool. Database connections are obtained per-operation via GetDB(ctx).
//
// Thread Safety:
// The repository is safe for concurrent use. The MongoDB driver handles
// connection pooling and concurrent access internally.
type MetadataMongoDBRepository struct {
	connection *libMongo.MongoConnection // MongoDB connection pool manager
	Database   string                    // Database name for all collections
}

// NewMetadataMongoDBRepository creates a new metadata repository with the given connection.
//
// This constructor validates the MongoDB connection by calling GetDB(). If the
// connection cannot be established, it panics to prevent the application from
// starting with a broken database connection.
//
// Panic Behavior:
// This is one of the few places where panic is acceptable because:
//  1. It runs during application startup (not request handling)
//  2. A failed database connection is unrecoverable
//  3. Failing fast prevents silent failures
//
// Parameters:
//   - mc: MongoDB connection pool manager from lib-commons
//
// Returns:
//   - *MetadataMongoDBRepository: Ready-to-use repository instance
//
// Panics:
//   - If MongoDB connection cannot be established
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

// Create inserts a new metadata document into MongoDB.
//
// Process:
//  1. Get database connection from pool
//  2. Select collection based on entity type (lowercase)
//  3. Convert domain Metadata to MetadataMongoDBModel
//  4. Insert document into MongoDB
//
// Parameters:
//   - ctx: Request context with tracing and cancellation
//   - collection: Entity type name (e.g., "Account", "Ledger")
//   - metadata: Domain metadata to persist
//
// Returns:
//   - error: Database connection or insert failures
//
// Note: MongoDB auto-generates the _id field if not set.
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

// FindList retrieves metadata documents matching filter criteria with pagination.
//
// This method supports two modes:
//   - Full scan: When filter.UseMetadata is false, returns all documents
//   - Filtered: When filter.UseMetadata is true, applies MongoDB query with pagination
//
// Pagination:
// When filter.UseMetadata is true:
//   - Limit: filter.Limit documents per page
//   - Skip: (filter.Page - 1) * filter.Limit documents
//
// Query Format:
// The filter.Metadata field should contain MongoDB query operators:
//
//	{"metadata.category": "premium"}           // Exact match
//	{"metadata.score": {"$gt": 100}}           // Comparison
//	{"metadata.tags": {"$in": ["vip", "new"]}} // Array contains
//
// Parameters:
//   - ctx: Request context with tracing and cancellation
//   - collection: Entity type name for collection routing
//   - filter: Query parameters including metadata filter and pagination
//
// Returns:
//   - []*Metadata: Matching metadata documents
//   - error: Database connection or query failures
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

// FindByEntity retrieves a single metadata document by entity ID.
//
// This is the primary lookup method for entity metadata. It searches by
// entity_id field which should be indexed for performance.
//
// Parameters:
//   - ctx: Request context with tracing and cancellation
//   - collection: Entity type name for collection routing
//   - id: UUID of the entity whose metadata is being retrieved
//
// Returns:
//   - *Metadata: The metadata document if found
//   - nil, nil: If no document exists (not considered an error)
//   - nil, error: On database connection or query failures
//
// Note: Returns nil, nil for missing documents to allow callers to distinguish
// between "no metadata" and "error fetching metadata".
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

// FindByEntityIDs retrieves metadata for multiple entities in a single query.
//
// This batch operation uses MongoDB's $in operator to efficiently fetch
// metadata for multiple entities. It's more efficient than multiple
// FindByEntity calls when loading metadata for a list of entities.
//
// Query:
//
//	{ "entity_id": { "$in": ["id1", "id2", "id3", ...] } }
//
// Parameters:
//   - ctx: Request context with tracing and cancellation
//   - collection: Entity type name for collection routing
//   - entityIDs: List of entity UUIDs to fetch metadata for
//
// Returns:
//   - []*Metadata: Metadata documents for entities that have metadata
//   - Empty slice if entityIDs is empty or no documents found
//   - error: On database connection or query failures
//
// Note: The returned slice may have fewer elements than entityIDs if some
// entities don't have metadata. The caller must handle this case.
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

// Update performs an upsert on metadata fields for the specified entity.
//
// This method uses MongoDB's upsert functionality to create the document if
// it doesn't exist, or update it if it does. Only the metadata field and
// updated_at timestamp are modified; other fields remain unchanged.
//
// Update Operation:
//
//	filter: { "entity_id": id }
//	update: { "$set": { "metadata": {...}, "updated_at": now } }
//	options: { upsert: true }
//
// Parameters:
//   - ctx: Request context with tracing and cancellation
//   - collection: Entity type name for collection routing
//   - id: UUID of the entity whose metadata is being updated
//   - metadata: New metadata key-value pairs (replaces existing metadata)
//
// Returns:
//   - error: Database connection, query failures, or ErrEntityNotFound
//
// Note: The entire metadata object is replaced, not merged. To merge,
// the caller must first fetch existing metadata and combine values.
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

// Delete removes the metadata document for the specified entity.
//
// This method performs a hard delete (document removal) rather than soft delete.
// The document is permanently removed from the collection.
//
// Delete Operation:
//
//	filter: { "entity_id": id }
//	operation: DeleteOne
//
// Parameters:
//   - ctx: Request context with tracing and cancellation
//   - collection: Entity type name for collection routing
//   - id: UUID of the entity whose metadata is being deleted
//
// Returns:
//   - error: Database connection or delete failures
//
// Note: No error is returned if the document doesn't exist. This is intentional
// to make delete operations idempotent.
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
