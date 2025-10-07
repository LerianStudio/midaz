// Package mongodb provides MongoDB repository implementations for metadata persistence.
//
// This package implements the Repository pattern for flexible metadata storage in MongoDB.
// Metadata is stored separately from primary entity data (in PostgreSQL) to support:
//   - Schema-less data (no predefined fields)
//   - Large metadata values (up to 2000 characters per value)
//   - Flexible querying by metadata fields
//   - Schema evolution without migrations
//
// The package uses MongoDB's document model to store entity metadata with the structure:
//   - entity_id: UUID of the entity (Organization, Account, etc.)
//   - entity_name: Type of entity (e.g., "Organization", "Account")
//   - metadata: Flexible key-value map
//   - created_at, updated_at: Timestamps
package mongodb

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// MetadataMongoDBModel represents the MongoDB document structure for metadata.
//
// This model maps directly to MongoDB's BSON format and provides the database
// representation of entity metadata. Each document stores metadata for a single entity.
//
// Fields:
//   - ID: MongoDB ObjectID (auto-generated)
//   - EntityID: UUID of the entity this metadata belongs to
//   - EntityName: Type of entity (Organization, Ledger, Account, etc.)
//   - Data: Flexible key-value metadata map
//   - CreatedAt: Timestamp when metadata was created
//   - UpdatedAt: Timestamp when metadata was last modified
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
// This struct represents metadata in the business logic layer, decoupled from
// MongoDB-specific types. It's used for passing metadata between layers.
type Metadata struct {
	ID         primitive.ObjectID // MongoDB document ID
	EntityID   string             // UUID of the entity
	EntityName string             // Entity type name
	Data       JSON               // Flexible metadata map
	CreatedAt  time.Time          // Creation timestamp
	UpdatedAt  time.Time          // Last update timestamp
}

// JSON is a flexible map type for storing metadata key-value pairs.
//
// This type implements database/sql/driver.Valuer and sql.Scanner interfaces
// to support JSON serialization/deserialization for SQL databases (though
// primarily used with MongoDB in this package).
//
// Metadata constraints (enforced at HTTP layer):
//   - Keys: Max 100 characters, alphanumeric + underscore
//   - Values: Max 2000 characters
//   - No nested objects (flat structure only)
type JSON map[string]any

// Value marshals the JSON map to a database value (implements driver.Valuer).
//
// This method allows JSON to be used with database/sql drivers that support
// JSON columns. It serializes the map to JSON bytes.
//
// Returns:
//   - driver.Value: JSON bytes
//   - error: Marshaling error if serialization fails
func (mj JSON) Value() (driver.Value, error) {
	return json.Marshal(mj)
}

// Scan unmarshals a database value into the JSON map (implements sql.Scanner).
//
// This method allows JSON to be populated from database/sql query results.
// It expects the value to be a byte slice containing JSON data.
//
// Parameters:
//   - value: Database value (expected to be []byte)
//
// Returns:
//   - error: Type assertion or unmarshal error
func (mj *JSON) Scan(value any) error {
	b, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}

	return json.Unmarshal(b, &mj)
}

// ToEntity converts a MongoDB model to a domain Metadata entity.
//
// This method transforms the database representation into the business logic
// representation, decoupling the domain layer from MongoDB-specific types.
//
// Returns:
//   - *Metadata: Domain model with all fields copied
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

// FromEntity converts a domain Metadata entity to a MongoDB model.
//
// This method transforms the business logic representation into the database
// representation for persistence in MongoDB.
//
// Parameters:
//   - md: Domain metadata entity to convert
//
// Returns:
//   - error: Always returns nil (kept for interface consistency)
//
// Side Effects:
//   - Modifies the receiver (*mmm) in place with values from md
func (mmm *MetadataMongoDBModel) FromEntity(md *Metadata) error {
	mmm.ID = md.ID
	mmm.EntityID = md.EntityID
	mmm.EntityName = md.EntityName
	mmm.Data = md.Data
	mmm.CreatedAt = md.CreatedAt
	mmm.UpdatedAt = md.UpdatedAt

	return nil
}
