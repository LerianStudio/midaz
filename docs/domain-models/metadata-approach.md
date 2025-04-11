# Metadata Approach

**Navigation:** [Home](../) > [Domain Models](./) > Metadata Approach

This document describes the metadata approach implemented in the Midaz system, explaining how the system uses flexible metadata to extend core entities.

## Overview

Midaz employs a polyglot persistence approach that separates structured entity data from flexible metadata:

- **Core Entity Data**: Stored in PostgreSQL with well-defined schemas
- **Flexible Metadata**: Stored in MongoDB with a schema-less approach

This design provides a best-of-both-worlds solution that maintains strong relational integrity for core financial data while allowing for extensibility through customizable metadata attributes.

The key advantages of this approach include:

1. **Schema Flexibility**: Entities can be extended with arbitrary metadata without schema migrations
2. **Data Separation**: Core entity data remains in a strongly-typed relational database
3. **Extensibility**: New attributes can be added to entities as needed by clients
4. **Performance**: Each database technology is used for its strengths
5. **Evolution**: The system can evolve without breaking changes to existing structures

## Metadata Architecture

### Dual-Storage Model

The system separates data storage between two database technologies:

```
┌─────────────────────────┐     ┌─────────────────────────┐
│ PostgreSQL              │     │ MongoDB                 │
│                         │     │                         │
│ - Core entity data      │     │ - Flexible metadata     │
│ - Relational integrity  │     │ - Document-based        │
│ - Transactional data    │     │ - Schema-less           │
│ - Strong typing         │     │ - Key-value attributes  │
└──────────┬──────────────┘     └────────────┬────────────┘
           │                                  │
           │                                  │
           │ Common Entity ID (UUID)          │
           └──────────────┬──────────────────┘
                          │
                          ▼
                 ┌─────────────────┐
                 │ Combined Entity │
                 └─────────────────┘
```

### Metadata Structure

MongoDB stores metadata documents with the following structure:

```json
{
  "_id": "ObjectId('...')",
  "entity_id": "uuid-reference-to-postgres-entity",
  "entity_name": "EntityTypeName",
  "metadata": {
    "custom_key1": "value1",
    "custom_key2": "value2",
    "custom_key3": 123
  },
  "created_at": "2023-01-01T00:00:00.000Z",
  "updated_at": "2023-01-01T00:00:00.000Z"
}
```

Key components:
- **entity_id**: UUID reference to the entity in PostgreSQL
- **entity_name**: Type/name of the entity (e.g., "Account", "Transaction")
- **metadata**: Flexible key-value map for custom attributes

## Metadata Implementation

### Data Models

All entity models include a metadata field:

```go
type Account struct {
    ID             string         `json:"id"`
    Name           string         `json:"name"`
    // Other structured fields...
    
    // Flexible metadata field for custom attributes
    Metadata       map[string]any `json:"metadata,omitempty"`
}
```

### API Validation

Inputs with metadata undergo validation:

```go
type CreateAccountInput struct {
    Name     string         `json:"name" validate:"max=256"`
    // Other fields...
    
    // Metadata with validation rules
    Metadata map[string]any `json:"metadata" validate:"dive,keys,keymax=100,endkeys,nonested,valuemax=2000"`
}
```

The validation enforces:
- Key length maximum of 100 characters
- Value length maximum of 2000 characters
- Flat key-value structure (non-nested)

### MongoDB Adapter Implementation

The system uses a dedicated repository interface for metadata operations:

```go
// Repository provides an interface for operations related to metadata entities.
type Repository interface {
    Create(ctx context.Context, collection string, metadata *Metadata) error
    FindList(ctx context.Context, collection string, filter http.QueryHeader) ([]*Metadata, error)
    FindByEntity(ctx context.Context, collection, id string) (*Metadata, error)
    Update(ctx context.Context, collection, id string, metadata map[string]any) error
    Delete(ctx context.Context, collection, id string) error
}
```

This is implemented with MongoDB-specific code:

```go
// MetadataMongoDBRepository is a MongoDB-specific implementation.
type MetadataMongoDBRepository struct {
    connection *libMongo.MongoConnection
    Database   string
}
```

## Metadata Operations

### Creation Flow

When a new entity is created:

1. The core entity is first stored in PostgreSQL
2. A metadata document is created in MongoDB with the entity ID reference
3. The combined entity with metadata is returned to the client

```go
// Create entity in PostgreSQL
entityID, err := uc.EntityRepo.Create(ctx, entity)

// Create metadata in MongoDB
meta := mongodb.Metadata{
    EntityID:   entityID,
    EntityName: entityName,
    Data:       metadata,
    CreatedAt:  time.Now(),
    UpdatedAt:  time.Now(),
}
err = uc.MetadataRepo.Create(ctx, entityName, &meta)
```

### Update Flow

When updating metadata:

1. The existing metadata is retrieved from MongoDB
2. New metadata is merged with existing metadata
3. The updated metadata is saved back to MongoDB

```go
// Get existing metadata
existingMetadata, err := uc.MetadataRepo.FindByEntity(ctx, entityName, entityID)

// Merge metadata
metadataToUpdate := libCommons.MergeMaps(metadata, existingMetadata.Data)

// Update metadata
err = uc.MetadataRepo.Update(ctx, entityName, entityID, metadataToUpdate)
```

### Query Flow

When querying entities with metadata:

1. Metadata is queried from MongoDB based on search criteria
2. Entity IDs are extracted from the metadata results
3. Core entities are fetched from PostgreSQL using the IDs
4. Entities and metadata are combined in the results

```go
// Get metadata matching criteria
metadata, err := uc.MetadataRepo.FindList(ctx, entityName, filter)

// Extract entity IDs and create metadata map
uuids := make([]uuid.UUID, len(metadata))
metadataMap := make(map[string]map[string]any, len(metadata))
for i, meta := range metadata {
    uuids[i] = uuid.MustParse(meta.EntityID)
    metadataMap[meta.EntityID] = meta.Data
}

// Get entities from PostgreSQL
entities, err := uc.EntityRepo.ListByIDs(ctx, uuids)

// Combine entities with their metadata
for i := range entities {
    if data, ok := metadataMap[entities[i].ID]; ok {
        entities[i].Metadata = data
    }
}
```

## Metadata Collections

MongoDB collections are organized by entity type, including:

- **organization**: Organization metadata
- **ledger**: Ledger metadata
- **asset**: Asset metadata
- **segment**: Segment metadata
- **portfolio**: Portfolio metadata
- **account**: Account metadata
- **transaction**: Transaction metadata
- **operation**: Operation metadata

Each collection stores metadata documents related to its specific entity type.

## Metadata Querying

The system supports querying entities by metadata attributes:

```
GET /v1/organizations?metadata.custom_key=value
```

These queries use MongoDB's flexible querying capabilities:

1. Metadata is first searched in MongoDB
2. Matching entity IDs are used to retrieve entities from PostgreSQL
3. Results are combined and returned

## Common Metadata Use Cases

### Tagging and Categorization

Metadata can be used for tagging and categorization:

```json
{
  "metadata": {
    "tags": ["important", "high-priority"],
    "category": "personal",
    "department": "finance"
  }
}
```

### Custom Attributes

Entities can be extended with custom domain-specific attributes:

```json
{
  "metadata": {
    "risk_level": "high",
    "credit_score": 720,
    "customer_type": "premium",
    "notes": "VIP customer"
  }
}
```

### Integration Data

Metadata can store external system references:

```json
{
  "metadata": {
    "external_id": "SAP-12345",
    "source_system": "ERP",
    "integration_timestamp": "2023-01-01T00:00:00Z"
  }
}
```

### Audit Information

Additional audit information beyond standard timestamps:

```json
{
  "metadata": {
    "created_by": "user123",
    "approved_by": "manager456",
    "approval_date": "2023-01-02T00:00:00Z",
    "revision": 3
  }
}
```

## Best Practices

### Metadata Design

1. **Keep Metadata Flat**: Avoid deep nesting in metadata structures
2. **Use Consistent Keys**: Establish conventions for metadata keys
3. **Size Considerations**: Keep metadata values concise (under 2000 characters)
4. **Type Consistency**: Maintain consistent data types for metadata values

### API Usage

1. **Metadata Filtering**: Use metadata for filtering only when necessary
2. **Partial Updates**: Update only the required metadata fields
3. **Documentation**: Document metadata fields used by your application

### Performance Considerations

1. **Batch Operations**: Combine metadata operations when possible
2. **Cached Access**: Consider caching frequently accessed metadata
3. **Query Optimization**: Limit metadata queries to necessary fields

## Limitations and Constraints

1. **Validation**: Metadata keys are limited to 100 characters maximum
2. **Value Size**: Metadata values are limited to 2000 characters maximum
3. **Nesting**: Nested structures are not supported (flat key-value pairs only)
4. **Transactions**: Cross-database transactions are not supported

## Extending the System

New metadata fields can be added without code changes:

1. Client applications can add new metadata fields as needed
2. No schema migrations are required
3. Existing code continues to work without modification

## Related Documentation

- [MongoDB Configuration](../components/infrastructure/mongodb.md)
- [PostgreSQL Configuration](../components/infrastructure/postgresql.md)
- [Entity Hierarchy](entity-hierarchy.md)
