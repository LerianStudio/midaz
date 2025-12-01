// Package mongodb provides MongoDB data models and adapters for flexible metadata storage.
//
// This package implements the infrastructure layer for storing arbitrary key-value metadata
// associated with domain entities. MongoDB is chosen for metadata storage because:
//   - Schema flexibility: Metadata structure varies per entity type and tenant
//   - JSON-native: Natural mapping from API request bodies
//   - Document queries: Efficient filtering by metadata fields
//
// Architecture Context:
// While PostgreSQL stores structured entity data (accounts, transactions), MongoDB
// stores extensible metadata that can vary without schema migrations. This polyglot
// persistence pattern allows:
//   - PostgreSQL: ACID transactions for financial data
//   - MongoDB: Flexible schema for custom entity attributes
//
// Metadata Concept:
// Metadata enables tenants to attach custom key-value data to entities without
// modifying the core schema. Common use cases:
//   - External system IDs (ERP codes, CRM references)
//   - Business categorization (cost centers, project codes)
//   - Regulatory tags (compliance flags, audit markers)
//
// Data Model:
//
//	┌─────────────────────────────────────────────┐
//	│              Metadata Document              │
//	├─────────────────────────────────────────────┤
//	│  _id: ObjectID      (MongoDB auto-generated)│
//	│  entity_id: string  (UUID of parent entity) │
//	│  entity_name: string (Type: Account, etc.)  │
//	│  metadata: object   (Arbitrary JSON data)   │
//	│  created_at: date                           │
//	│  updated_at: date                           │
//	└─────────────────────────────────────────────┘
//
// Collection Strategy:
// Each entity type has its own collection (lowercase), enabling:
//   - Independent scaling per entity type
//   - Simpler index management
//   - Cleaner backup/restore operations
//
// Related Packages:
//   - mmodel: Domain entities that own metadata
//   - command: Services that create/update metadata
//   - query: Services that retrieve metadata
package mongodb

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"time"
)

// MetadataMongoDBModel represents metadata as stored in MongoDB.
//
// This model maps directly to MongoDB documents with BSON tags defining field names.
// The _id field uses MongoDB's ObjectID for efficient indexing and sharding.
//
// Document Example:
//
//	{
//	    "_id": ObjectId("507f1f77bcf86cd799439011"),
//	    "entity_id": "01957e87-1234-7def-8000-abcdef123456",
//	    "entity_name": "Account",
//	    "metadata": {
//	        "erp_code": "ACC-001",
//	        "cost_center": "CC-SALES",
//	        "tags": ["premium", "verified"]
//	    },
//	    "created_at": ISODate("2024-01-15T10:30:00Z"),
//	    "updated_at": ISODate("2024-01-15T10:30:00Z")
//	}
//
// Indexes:
//   - Unique index on (entity_id) for fast entity lookups
//   - Optional indexes on metadata fields for query optimization
type MetadataMongoDBModel struct {
	ID         primitive.ObjectID `bson:"_id,omitempty"` // MongoDB ObjectID, auto-generated
	EntityID   string             `bson:"entity_id"`     // UUID of the parent entity
	EntityName string             `bson:"entity_name"`   // Entity type (Account, Ledger, etc.)
	Data       JSON               `bson:"metadata"`      // Arbitrary key-value metadata
	CreatedAt  time.Time          `bson:"created_at"`    // Document creation timestamp
	UpdatedAt  time.Time          `bson:"updated_at"`    // Last modification timestamp
}

// Metadata is the domain representation of entity metadata.
//
// This struct serves as the internal domain model, decoupled from MongoDB-specific
// types like BSON tags. It's used throughout the application layer while
// MetadataMongoDBModel handles persistence concerns.
//
// Separation of Concerns:
//   - Metadata: Used in business logic, services, handlers
//   - MetadataMongoDBModel: Used only in the MongoDB adapter layer
//
// This separation allows changing the storage implementation without affecting
// business logic.
type Metadata struct {
	ID         primitive.ObjectID // MongoDB document ID
	EntityID   string             // UUID of the owning entity
	EntityName string             // Type name of the owning entity
	Data       JSON               // Arbitrary metadata as JSON object
	CreatedAt  time.Time          // Creation timestamp
	UpdatedAt  time.Time          // Last update timestamp
}

// JSON represents a flexible map of string keys to arbitrary values.
//
// This type is used for storing schemaless metadata in MongoDB. It implements
// the database/sql/driver interfaces for compatibility with SQL scanners,
// enabling consistent handling across both PostgreSQL (for queries) and MongoDB.
//
// Supported Value Types:
//   - string, int, float64, bool: Primitive values
//   - []any: Arrays (e.g., tags, categories)
//   - map[string]any: Nested objects
//   - nil: Null values
//
// Usage:
//
//	metadata := mongodb.JSON{
//	    "external_id": "EXT-12345",
//	    "priority": 1,
//	    "tags": []string{"urgent", "vip"},
//	    "config": map[string]any{
//	        "notify": true,
//	        "channel": "email",
//	    },
//	}
type JSON map[string]any

// Value implements driver.Valuer for SQL compatibility.
//
// This method marshals the JSON map to a byte slice for storage in SQL databases
// that support JSON columns. While metadata is primarily stored in MongoDB,
// this enables hybrid queries that join metadata with PostgreSQL entities.
//
// Returns:
//   - driver.Value: JSON byte slice
//   - error: JSON marshaling errors (rare for valid Go maps)
func (mj JSON) Value() (driver.Value, error) {
	return json.Marshal(mj)
}

// Scan implements sql.Scanner for SQL compatibility.
//
// This method unmarshals JSON bytes from SQL query results into the JSON map.
// Used when metadata is retrieved via SQL joins or when temporarily stored
// in PostgreSQL.
//
// Parameters:
//   - value: Byte slice from database driver
//
// Returns:
//   - error: Type assertion failure or JSON unmarshal errors
func (mj *JSON) Scan(value any) error {
	b, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}

	return json.Unmarshal(b, &mj)
}

// ToEntity converts the MongoDB model to the domain Metadata struct.
//
// This method maps MongoDB document fields to the domain model for use in
// business logic. The conversion is straightforward as both structures
// share the same fields.
//
// Returns:
//   - *Metadata: Domain model ready for application layer processing
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

// FromEntity populates the MongoDB model from a domain Metadata struct.
//
// This method maps domain model fields to MongoDB document fields for
// persistence. Used when creating or updating metadata documents.
//
// Parameters:
//   - md: Domain metadata to convert
//
// Returns:
//   - error: Always nil (signature kept for interface consistency)
//
// Note: The error return is maintained for potential future validation
// or conversion logic that might fail.
func (mmm *MetadataMongoDBModel) FromEntity(md *Metadata) error {
	mmm.ID = md.ID
	mmm.EntityID = md.EntityID
	mmm.EntityName = md.EntityName
	mmm.Data = md.Data
	mmm.CreatedAt = md.CreatedAt
	mmm.UpdatedAt = md.UpdatedAt

	return nil
}
