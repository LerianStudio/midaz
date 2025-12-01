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

// Repository provides an interface for metadata persistence operations in MongoDB.
//
// This interface defines the contract for metadata CRUD operations, following
// the repository pattern from Domain-Driven Design. It abstracts MongoDB-specific
// implementation details from the application layer.
//
// Design Decisions:
//
//   - Collection parameter: Allows reusing the same repository for different entity types
//   - Entity ID based: Lookups use entity_id (business ID) rather than MongoDB ObjectID
//   - Upsert on update: Creates metadata if it doesn't exist
//   - Batch operations: FindByEntityIDs supports efficient bulk lookups
//
// Usage:
//
//	repo := mongodb.NewMetadataMongoDBRepository(connection)
//	err := repo.Create(ctx, "transactions", &metadata)
//	meta, err := repo.FindByEntity(ctx, "transactions", transactionID)
//
// Thread Safety:
//
// All methods are thread-safe. The underlying MongoDB driver handles connection pooling
// and concurrent access.
//
// Observability:
//
// All methods create OpenTelemetry spans for distributed tracing.
// Span names follow the pattern: mongodb.<operation>
//
//go:generate mockgen --destination=metadata.mongodb_mock.go --package=mongodb . Repository
type Repository interface {
	Create(ctx context.Context, collection string, metadata *Metadata) error
	FindList(ctx context.Context, collection string, filter http.QueryHeader) ([]*Metadata, error)
	FindByEntity(ctx context.Context, collection, id string) (*Metadata, error)
	FindByEntityIDs(ctx context.Context, collection string, entityIDs []string) ([]*Metadata, error)
	Update(ctx context.Context, collection, id string, metadata map[string]any) error
	Delete(ctx context.Context, collection, id string) error
}

// MetadataMongoDBRepository is the MongoDB implementation of the Repository interface.
//
// This repository provides metadata persistence using MongoDB as the backing store.
// It implements the hexagonal architecture pattern by adapting the domain Repository
// interface to MongoDB-specific operations.
//
// Connection Management:
//
// The repository uses a shared MongoConnection from lib-commons which provides:
//   - Connection pooling
//   - Automatic reconnection
//   - Health checks
//
// Lifecycle:
//
//	conn := libMongo.NewMongoConnection(cfg)
//	repo := mongodb.NewMetadataMongoDBRepository(conn)
//	// Use repository...
//	// Connection cleanup handled by MongoConnection
//
// Thread Safety:
//
// MetadataMongoDBRepository is thread-safe after initialization. The underlying
// MongoDB driver handles concurrent operations.
//
// Fields:
//   - connection: Shared MongoDB connection (manages pool and lifecycle)
//   - Database: Database name for metadata storage (case-normalized to lowercase)
type MetadataMongoDBRepository struct {
	connection *libMongo.MongoConnection
	Database   string
}

// NewMetadataMongoDBRepository creates a new MetadataMongoDBRepository instance.
//
// This constructor initializes the repository with a MongoDB connection and
// validates connectivity before returning. It panics on connection failure
// to fail fast during application startup.
//
// Initialization Process:
//  1. Store connection reference
//  2. Extract database name from connection config
//  3. Verify connectivity by calling GetDB
//  4. Panic if connection fails (fail-fast startup)
//
// Parameters:
//   - mc: Configured MongoDB connection from lib-commons
//
// Returns:
//   - *MetadataMongoDBRepository: Initialized repository ready for use
//
// Panics:
//   - "Failed to connect mongodb": Connection verification failed
//
// Why Panic on Failure:
//
// This is intentional fail-fast behavior. If MongoDB is unavailable at startup,
// the application cannot function correctly. Panicking here ensures:
//   - Clear failure mode during deployment
//   - No silent degradation
//   - Immediate alerting in orchestration systems
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

// Create inserts a new metadata entity into MongoDB.
//
// This method persists metadata for an entity, allowing flexible schema-less
// extensions to core domain objects.
//
// Operation Process:
//  1. Start OpenTelemetry span for tracing
//  2. Get database connection from pool
//  3. Convert domain model to MongoDB model
//  4. Insert document into collection
//  5. Log successful insertion
//
// Parameters:
//   - ctx: Context with tracing and logging (must not be nil)
//   - collection: Target collection name (e.g., "transactions", "operations")
//   - metadata: Domain metadata model to persist
//
// Returns:
//   - error: Database connection or insertion error
//
// Tracing:
//
// Creates spans:
//   - mongodb.create_metadata: Overall operation
//   - mongodb.create_metadata.insert: Actual insert operation
//
// Error Scenarios:
//   - Connection pool exhausted
//   - MongoDB unavailable
//   - Document validation failure
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

// FindList retrieves metadata from MongoDB with optional filtering.
//
// This method supports flexible querying for metadata records, including
// metadata field matching and date range filtering.
//
// Operation Process:
//  1. Start OpenTelemetry span for tracing
//  2. Get database connection from pool
//  3. Build MongoDB filter from QueryHeader
//  4. Execute find query
//  5. Decode results and convert to domain models
//
// Parameters:
//   - ctx: Context with tracing and logging (must not be nil)
//   - collection: Target collection name
//   - filter: Query parameters including:
//   - Metadata: Key-value pairs to match
//   - StartDate/EndDate: Date range for created_at
//
// Returns:
//   - []*Metadata: Matching metadata records (empty slice if none found)
//   - error: Database connection or query error
//
// Tracing:
//
// Creates spans:
//   - mongodb.find_list: Overall operation
//   - mongodb.find_list.find: Actual query execution
//
// Query Building:
//
// The filter is built dynamically:
//   - Metadata fields are matched exactly
//   - Date range uses $gte/$lte operators on created_at
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

// FindByEntity retrieves metadata by entity ID.
//
// This method looks up metadata for a specific entity, returning nil if
// no metadata exists (not an error condition).
//
// Operation Process:
//  1. Start OpenTelemetry span for tracing
//  2. Get database connection from pool
//  3. Execute FindOne with entity_id filter
//  4. Convert to domain model if found
//
// Parameters:
//   - ctx: Context with tracing and logging (must not be nil)
//   - collection: Target collection name
//   - id: Entity ID (UUID) to look up
//
// Returns:
//   - *Metadata: Found metadata or nil if not found
//   - error: Database connection or query error (not returned for "not found")
//
// Tracing:
//
// Creates spans:
//   - mongodb.find_by_entity: Overall operation
//   - mongodb.find_by_entity.find_one: Actual query execution
//
// Not Found Handling:
//
// Returns (nil, nil) when entity has no metadata. This is intentional:
//   - Metadata is optional for entities
//   - Callers should check for nil result
//   - Distinguishes "no metadata" from "error occurred"
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

// FindByEntityIDs retrieves metadata for multiple entities in a single query.
//
// This method provides efficient bulk lookup using MongoDB's $in operator,
// avoiding N+1 query patterns when fetching metadata for multiple entities.
//
// Operation Process:
//  1. Return empty slice if no IDs provided (optimization)
//  2. Start OpenTelemetry span for tracing
//  3. Get database connection from pool
//  4. Execute find with $in filter on entity_id
//  5. Decode results and convert to domain models
//
// Parameters:
//   - ctx: Context with tracing and logging (must not be nil)
//   - collection: Target collection name
//   - entityIDs: List of entity IDs to look up
//
// Returns:
//   - []*Metadata: Found metadata (may be fewer than requested if some entities have no metadata)
//   - error: Database connection or query error
//
// Tracing:
//
// Creates spans:
//   - mongodb.find_by_entity_ids: Overall operation
//   - mongodb.find_by_entity_ids.find: Actual query execution
//
// Performance:
//
// Uses $in operator for efficient batch lookup. For very large ID lists (>1000),
// consider chunking to avoid MongoDB query size limits.
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

// Update modifies or creates metadata for an entity (upsert).
//
// This method uses upsert semantics: if metadata doesn't exist for the entity,
// it will be created. This simplifies client code by eliminating the need to
// check existence before writing.
//
// Operation Process:
//  1. Start OpenTelemetry span for tracing
//  2. Get database connection from pool
//  3. Build update document with $set operator
//  4. Execute UpdateOne with upsert option
//  5. Log modification result
//
// Parameters:
//   - ctx: Context with tracing and logging (must not be nil)
//   - collection: Target collection name
//   - id: Entity ID (UUID) to update
//   - metadata: New metadata key-value pairs (replaces existing metadata field)
//
// Returns:
//   - error: Database connection or update error
//
// Tracing:
//
// Creates spans:
//   - mongodb.update_metadata: Overall operation
//   - mongodb.update_metadata.update_one: Actual update execution
//
// Update Semantics:
//
// Uses $set to replace the entire metadata field. Individual field updates
// require fetching, modifying, and writing back. The updated_at timestamp
// is automatically set to current time.
//
// Error Scenarios:
//   - ErrEntityNotFound: Entity not found (only if upsert fails)
//   - Connection errors
//   - Document validation errors
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

// Delete removes metadata for an entity.
//
// This method deletes metadata by entity ID. It is idempotent: deleting
// non-existent metadata succeeds silently (DeletedCount = 0).
//
// Operation Process:
//  1. Start OpenTelemetry span for tracing
//  2. Get database connection from pool
//  3. Execute DeleteOne with entity_id filter
//  4. Log deletion result
//
// Parameters:
//   - ctx: Context with tracing and logging (must not be nil)
//   - collection: Target collection name
//   - id: Entity ID (UUID) to delete metadata for
//
// Returns:
//   - error: Database connection or deletion error
//
// Tracing:
//
// Creates spans:
//   - mongodb.delete_metadata: Overall operation
//   - mongodb.delete_metadata.delete_one: Actual deletion execution
//
// Idempotency:
//
// Delete is safe to call multiple times. If metadata doesn't exist,
// the operation succeeds with DeletedCount = 0. This supports:
//   - Retry logic in distributed systems
//   - Cleanup operations without existence checks
//   - Event-driven deletion handlers
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
