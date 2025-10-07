# Onboarding Adapters

## Overview

The `adapters` package implements the infrastructure layer of the onboarding service, following hexagonal architecture principles. This layer provides concrete implementations of repository interfaces, abstracting external dependencies (PostgreSQL, MongoDB, RabbitMQ) from the business logic.

## Purpose

This package provides:

- **PostgreSQL repositories**: Entity persistence with CRUD operations
- **MongoDB repositories**: Flexible metadata storage
- **RabbitMQ adapters**: Message queue integration for async processing
- **Repository pattern**: Clean separation between business logic and data access
- **Database abstraction**: Decoupling from specific database implementations

## Package Structure

```
adapters/
├── postgres/                    # PostgreSQL repositories
│   ├── organization/           # Organization entity persistence
│   │   ├── organization.go                    # Model and conversions
│   │   └── organization.postgresql.go         # Repository implementation
│   ├── account/                # Account entity persistence
│   │   ├── account.go                         # Model and conversions
│   │   └── account.postgresql.go              # Repository implementation
│   ├── ledger/                 # Ledger entity persistence
│   │   ├── ledger.go                          # Model and conversions
│   │   └── ledger.postgresql.go               # Repository implementation
│   ├── asset/                  # Asset entity persistence
│   │   ├── asset.go                           # Model and conversions
│   │   └── asset.postgresql.go                # Repository implementation
│   ├── portfolio/              # Portfolio entity persistence
│   │   ├── portfolio.go                       # Model and conversions
│   │   └── portfolio.postgresql.go            # Repository implementation
│   ├── segment/                # Segment entity persistence
│   │   ├── segment.go                         # Model and conversions
│   │   └── segment.postgresql.go              # Repository implementation
│   └── accounttype/            # Account type entity persistence
│       ├── accounttype.go                     # Model and conversions
│       └── accounttype.postgresql.go          # Repository implementation
├── mongodb/                    # MongoDB repositories
│   ├── metadata.go                            # Metadata model
│   └── metadata.mongodb.go                    # Metadata repository
└── rabbitmq/                   # RabbitMQ adapters
    └── (message queue adapters)
```

## Architecture Pattern

### Repository Pattern

All adapters follow the Repository pattern:

```
┌─────────────────────┐
│  Business Logic     │
│  (Services Layer)   │
└──────────┬──────────┘
           │ depends on
           ↓
┌─────────────────────┐
│ Repository Interface│  ← Defined in adapter package
└──────────┬──────────┘
           │ implemented by
           ↓
┌─────────────────────┐
│ Concrete Repository │  ← PostgreSQL/MongoDB implementation
│ (This Package)      │
└─────────────────────┘
```

### Model Conversion

Each repository package contains:

1. **Database Model**: Represents the database schema

   - Maps to database tables/collections
   - Uses database-specific types (sql.NullTime, primitive.ObjectID)
   - Includes struct tags for ORM/driver mapping

2. **Conversion Methods**:

   - `ToEntity()`: Database model → Domain model
   - `FromEntity()`: Domain model → Database model

3. **Repository Implementation**: Concrete data access methods
   - CRUD operations
   - Query methods
   - Batch operations

## PostgreSQL Repositories

### Common Features

All PostgreSQL repositories implement:

- **Soft deletes**: Records marked as deleted (deleted_at timestamp) but not physically removed
- **UUID primary keys**: UUIDv7 for time-ordered identifiers
- **Status tracking**: Status code + description fields
- **Metadata support**: Flexible key-value metadata stored in MongoDB
- **OpenTelemetry tracing**: All operations traced for observability
- **Error conversion**: PostgreSQL errors converted to business errors

### Repository Interface Pattern

```go
type Repository interface {
    Create(ctx context.Context, entity *mmodel.Entity) (*mmodel.Entity, error)
    Update(ctx context.Context, id uuid.UUID, entity *mmodel.Entity) (*mmodel.Entity, error)
    Find(ctx context.Context, id uuid.UUID) (*mmodel.Entity, error)
    FindAll(ctx context.Context, filter http.Pagination) ([]*mmodel.Entity, error)
    Delete(ctx context.Context, id uuid.UUID) error
    Count(ctx context.Context) (int64, error)
}
```

### Model Conversion Pattern

```go
// Database Model
type EntityPostgreSQLModel struct {
    ID                string
    Name              string
    Status            string
    StatusDescription *string
    CreatedAt         time.Time
    UpdatedAt         time.Time
    DeletedAt         sql.NullTime
    Metadata          map[string]any
}

// To Domain Model
func (m *EntityPostgreSQLModel) ToEntity() *mmodel.Entity {
    status := mmodel.Status{
        Code:        m.Status,
        Description: m.StatusDescription,
    }

    entity := &mmodel.Entity{
        ID:        m.ID,
        Name:      m.Name,
        Status:    status,
        CreatedAt: m.CreatedAt,
        UpdatedAt: m.UpdatedAt,
    }

    if !m.DeletedAt.Time.IsZero() {
        deletedAtCopy := m.DeletedAt.Time
        entity.DeletedAt = &deletedAtCopy
    }

    return entity
}

// From Domain Model
func (m *EntityPostgreSQLModel) FromEntity(entity *mmodel.Entity) {
    *m = EntityPostgreSQLModel{
        ID:                libCommons.GenerateUUIDv7().String(),
        Name:              entity.Name,
        Status:            entity.Status.Code,
        StatusDescription: entity.Status.Description,
        CreatedAt:         entity.CreatedAt,
        UpdatedAt:         entity.UpdatedAt,
    }

    if entity.DeletedAt != nil {
        m.DeletedAt = sql.NullTime{
            Time:  *entity.DeletedAt,
            Valid: true,
        }
    }
}
```

## MongoDB Repositories

### Metadata Repository

The metadata repository provides flexible, schema-less storage for entity metadata:

**Features:**

- **Collection per entity type**: Separate collections for Organization, Account, etc.
- **Flexible schema**: No predefined fields, supports any key-value pairs
- **Upsert semantics**: Creates document if it doesn't exist
- **Batch operations**: Efficient retrieval for multiple entities
- **Metadata queries**: Filter entities by metadata fields

**Document Structure:**

```json
{
  "_id": ObjectId("..."),
  "entity_id": "uuid-of-entity",
  "entity_name": "Organization",
  "metadata": {
    "department": "Engineering",
    "cost_center": "CC-1234",
    "custom_field": "value"
  },
  "created_at": ISODate("..."),
  "updated_at": ISODate("...")
}
```

**Repository Interface:**

```go
type Repository interface {
    Create(ctx context.Context, collection string, metadata *Metadata) error
    FindList(ctx context.Context, collection string, filter http.QueryHeader) ([]*Metadata, error)
    FindByEntity(ctx context.Context, collection, id string) (*Metadata, error)
    FindByEntityIDs(ctx context.Context, collection string, entityIDs []string) ([]*Metadata, error)
    Update(ctx context.Context, collection, id string, metadata map[string]any) error
    Delete(ctx context.Context, collection, id string) error
}
```

## RabbitMQ Adapters

RabbitMQ adapters handle asynchronous message publishing for:

- **Account creation events**: Notify transaction service of new accounts
- **Balance initialization**: Trigger balance setup for new accounts
- **Event-driven architecture**: Decouple services via message queues

## Error Handling

### PostgreSQL Error Conversion

PostgreSQL errors are converted to business errors:

```go
var pgErr *pgconn.PgError
if errors.As(err, &pgErr) {
    return services.ValidatePGError(pgErr, entityType)
}
```

**Common conversions:**

- `23505` (unique violation) → `ErrDuplicateLedger` or similar
- `23503` (foreign key violation) → `ErrEntityNotFound`
- `23514` (check constraint) → `ErrInvalidParameter`

### MongoDB Error Handling

MongoDB errors are handled gracefully:

- `ErrNoDocuments` → Returns nil (not an error for optional metadata)
- Connection errors → Logged and returned as-is
- Validation errors → Converted to business errors

## Usage Examples

### Creating an Entity

```go
// Service layer
org := &mmodel.Organization{
    LegalName: "Acme Corp",
    Status: mmodel.Status{Code: "ACTIVE"},
}

// Repository creates in PostgreSQL
createdOrg, err := orgRepo.Create(ctx, org)

// Metadata created in MongoDB
metadata := map[string]any{"industry": "Technology"}
err = metadataRepo.Create(ctx, "organization", &mongodb.Metadata{
    EntityID:   createdOrg.ID,
    EntityName: "Organization",
    Data:       metadata,
})
```

### Querying with Metadata

```go
// Find entities by metadata
filter := http.QueryHeader{
    UseMetadata: true,
    Metadata: bson.M{
        "metadata.department": "Engineering",
    },
    Limit: 10,
    Page:  1,
}

// Get entity IDs from MongoDB
metadataList, err := metadataRepo.FindList(ctx, "organization", filter)

// Fetch entities from PostgreSQL
ids := extractEntityIDs(metadataList)
orgs, err := orgRepo.ListByIDs(ctx, ids)
```

## Best Practices

### 1. Always Use Context

All repository methods accept `context.Context` for:

- Request cancellation
- Timeout propagation
- Tracing information
- Logging context

### 2. Handle Soft Deletes

Queries must exclude soft-deleted records:

```sql
WHERE deleted_at IS NULL
```

### 3. Use Transactions When Needed

For operations spanning multiple tables, use database transactions:

```go
tx, err := db.BeginTx(ctx, nil)
defer tx.Rollback()

// Multiple operations...

tx.Commit()
```

### 4. Metadata Separation

Store structured data in PostgreSQL, flexible data in MongoDB:

- **PostgreSQL**: Queryable, indexed fields (name, status, IDs)
- **MongoDB**: Flexible, schema-less metadata

### 5. Error Conversion

Always convert database errors to business errors before returning to service layer.

## Testing

### Unit Tests

Each repository should have unit tests covering:

- CRUD operations
- Error conditions
- Edge cases (nil values, empty strings)
- Soft delete behavior

### Integration Tests

Integration tests verify:

- Actual database operations
- Transaction behavior
- Constraint enforcement
- Performance characteristics

## Performance Considerations

### PostgreSQL

- **Indexes**: Ensure proper indexes on frequently queried fields
- **Connection pooling**: Use lib-commons connection pool
- **Batch operations**: Use `ListByIDs` for multiple entities
- **Query optimization**: Use EXPLAIN ANALYZE for slow queries

### MongoDB

- **Indexes**: Index entity_id for fast lookups
- **Batch retrieval**: Use $in operator for multiple entities
- **Projection**: Only fetch needed fields
- **Connection pooling**: Reuse connections via lib-commons

## Dependencies

- **lib-commons**: Common utilities, database connections, tracing
- **squirrel**: SQL query builder for complex queries
- **pgx**: PostgreSQL driver
- **mongo-driver**: MongoDB driver
- **pkg/mmodel**: Domain models
- **pkg/constant**: Error codes and constants

## Migration Strategy

When adding new fields:

1. **Add to PostgreSQL model** (if structured data)
2. **Update ToEntity/FromEntity** methods
3. **Run database migration**
4. **Update repository methods** as needed

For flexible fields, use MongoDB metadata instead.

---

**Note**: This package follows hexagonal architecture principles, keeping infrastructure concerns separate from business logic. All database-specific code is contained within this package, allowing the service layer to remain database-agnostic.
