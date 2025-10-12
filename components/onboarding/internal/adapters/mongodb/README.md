# MongoDB Adapter - Metadata Storage

This package provides MongoDB repository implementation for flexible metadata storage across all onboarding entities.

## Purpose

While core entity data is stored in PostgreSQL for ACID compliance, metadata fields are stored in MongoDB to provide:

- Schema-less flexibility for custom attributes
- Efficient querying of nested JSON structures
- Scalable storage for varying metadata sizes

## Implementation

- `metadata.go` - Core types and interfaces
- `metadata.mongodb.go` - MongoDB repository implementation
- `metadata.mongodb_mock.go` - Generated mock for testing

## Operations

The repository supports:

- Create - Store metadata for an entity
- FindByEntity - Retrieve metadata by entity ID
- FindByEntityIDs - Batch retrieval for multiple entities
- Update - Merge updates into existing metadata
- Delete - Remove metadata when entity is deleted

## Data Model

```go
type MetadataMongoDBModel struct {
    ID         ObjectID  // MongoDB document ID
    EntityID   string    // Reference to PostgreSQL entity
    EntityName string    // Entity type (Account, Ledger, etc.)
    Data       JSON      // Flexible metadata object
    CreatedAt  time.Time
    UpdatedAt  time.Time
}
```

## Usage

The metadata repository is injected into command/query use cases and coordinates with PostgreSQL repositories to provide complete entity data with custom attributes.
