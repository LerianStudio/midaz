// Package mongodb provides MongoDB adapter implementations for the transaction component.
//
// This package implements the infrastructure layer for metadata storage in MongoDB,
// following the hexagonal architecture pattern. Metadata is stored separately from
// the main entity data to allow flexible schema-less extensions.
//
// Architecture Overview:
//
// The metadata adapter provides:
//   - Flexible key-value storage for entity extensions
//   - Entity-agnostic metadata (works with transactions, operations, etc.)
//   - JSON document storage with BSON serialization
//   - Full CRUD operations with OpenTelemetry tracing
//
// Why MongoDB for Metadata:
//
// MongoDB is chosen for metadata storage because:
//   - Schema-less design allows arbitrary metadata fields
//   - Document model maps naturally to JSON metadata
//   - Horizontal scaling for high-volume transaction metadata
//   - Rich query capabilities for metadata filtering
//
// Data Flow:
//
//	Domain Entity → Metadata (domain) → MetadataMongoDBModel (BSON) → MongoDB
//	MongoDB → MetadataMongoDBModel (BSON) → Metadata (domain) → Domain Entity
//
// Related Packages:
//   - github.com/LerianStudio/lib-commons/v2/commons/mongo: MongoDB connection management
//   - github.com/LerianStudio/midaz/v3/pkg/net/http: Query filter definitions
package mongodb

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"time"
)

// MetadataMongoDBModel represents the metadata storage model for MongoDB.
//
// This model maps directly to MongoDB documents with BSON serialization.
// It serves as the persistence layer representation of metadata, separate
// from the domain model to maintain hexagonal architecture boundaries.
//
// Document Structure:
//
//	{
//	    "_id": ObjectId("..."),
//	    "entity_id": "uuid-of-related-entity",
//	    "entity_name": "transaction",
//	    "metadata": { "key1": "value1", "key2": 123 },
//	    "created_at": ISODate("..."),
//	    "updated_at": ISODate("...")
//	}
//
// Indexing Strategy:
//
// The following indexes should exist for optimal performance:
//   - entity_id: Unique index for fast lookups
//   - entity_name + entity_id: Compound index for collection-based queries
//   - created_at: For time-range queries
//
// Thread Safety:
//
// MetadataMongoDBModel is not thread-safe. Each goroutine should work with
// its own instance. The repository handles concurrent access at the database level.
type MetadataMongoDBModel struct {
	ID         primitive.ObjectID `bson:"_id,omitempty"`
	EntityID   string             `bson:"entity_id"`
	EntityName string             `bson:"entity_name"`
	Data       JSON               `bson:"metadata"`
	CreatedAt  time.Time          `bson:"created_at"`
	UpdatedAt  time.Time          `bson:"updated_at"`
}

// Metadata is the domain model for entity metadata.
//
// This struct represents metadata in the domain layer, decoupled from
// MongoDB-specific concerns. It acts as a Data Transfer Object between
// the repository and application layers.
//
// Purpose:
//
// Metadata allows entities to store arbitrary key-value data without
// schema changes. Common use cases include:
//   - Custom fields added by integrations
//   - Audit trail extensions
//   - Partner-specific data
//   - Feature flags per entity
//
// Fields:
//   - ID: MongoDB ObjectID (auto-generated on insert)
//   - EntityID: UUID of the parent entity (transaction, operation, etc.)
//   - EntityName: Type of entity for namespacing
//   - Data: Flexible JSON key-value pairs
//   - CreatedAt: Record creation timestamp
//   - UpdatedAt: Last modification timestamp
type Metadata struct {
	ID         primitive.ObjectID
	EntityID   string
	EntityName string
	Data       JSON
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// JSON represents a flexible key-value document for MongoDB storage.
//
// This type provides a schema-less container for metadata fields.
// It implements database/sql/driver interfaces for compatibility with
// database scanning and value conversion.
//
// Supported Value Types:
//   - string, int, float64, bool (primitives)
//   - []any (arrays)
//   - map[string]any (nested objects)
//   - nil (null values)
//
// Thread Safety:
//
// JSON maps are not thread-safe. Do not share instances across goroutines
// without synchronization.
type JSON map[string]any

// Value implements driver.Valuer for database serialization.
//
// This method marshals the JSON map to a byte slice for storage.
// It enables the JSON type to be used with database/sql interfaces.
//
// Returns:
//   - driver.Value: JSON-encoded byte slice
//   - error: Marshaling error if values cannot be serialized
func (mj JSON) Value() (driver.Value, error) {
	return json.Marshal(mj)
}

// Scan implements sql.Scanner for database deserialization.
//
// This method unmarshals a byte slice from the database into the JSON map.
// It enables the JSON type to be used with database/sql row scanning.
//
// Parameters:
//   - value: Raw database value (expected to be []byte)
//
// Returns:
//   - error: Type assertion error or JSON unmarshal error
//
// Error Scenarios:
//   - "type assertion to []byte failed": Database value is not a byte slice
//   - JSON unmarshal errors: Invalid JSON in database
func (mj *JSON) Scan(value any) error {
	b, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}

	return json.Unmarshal(b, &mj)
}

// ToEntity converts a MetadataMongoDBModel to the domain Metadata model.
//
// This method implements the outbound mapping in hexagonal architecture,
// transforming the persistence model back to the domain representation.
//
// Mapping Process:
//  1. Copy all fields directly (no transformation needed)
//  2. Preserve ObjectID for update/delete operations
//  3. Maintain timestamps for audit trail
//
// Returns:
//   - *Metadata: Domain model with all fields mapped
func (mmm *MetadataMongoDBModel) ToEntity() *Metadata {
	return &Metadata{
		ID:         mmm.ID,
		EntityID:   mmm.EntityID,
		EntityName: mmm.EntityName,
		Data:       mmm.Data,
		CreatedAt:  mmm.CreatedAt,
		UpdatedAt:  mmm.UpdatedAt,
	}
}

// FromEntity converts a domain Metadata model to MetadataMongoDBModel.
//
// This method implements the inbound mapping in hexagonal architecture,
// transforming the domain representation to the persistence model.
//
// Mapping Process:
//  1. Copy all fields directly (no transformation needed)
//  2. Preserve existing ObjectID if updating
//  3. Maintain timestamps from domain model
//
// Parameters:
//   - md: Domain Metadata model to convert
//
// Returns:
//   - error: Always nil (signature kept for interface consistency)
func (mmm *MetadataMongoDBModel) FromEntity(md *Metadata) error {
	mmm.ID = md.ID
	mmm.EntityID = md.EntityID
	mmm.EntityName = md.EntityName
	mmm.Data = md.Data
	mmm.CreatedAt = md.CreatedAt
	mmm.UpdatedAt = md.UpdatedAt

	return nil
}
