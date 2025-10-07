# Transaction Adapters

## Overview

The `adapters` package implements the infrastructure layer of the transaction service, following hexagonal architecture principles. This layer provides concrete implementations of repository interfaces, abstracting external dependencies (PostgreSQL, MongoDB, RabbitMQ, Redis) from the business logic.

## Purpose

This package provides:

- **PostgreSQL repositories**: Transaction, operation, balance, asset rate, routing persistence
- **MongoDB repositories**: Flexible metadata storage
- **RabbitMQ adapters**: Message queue integration for async processing and events
- **Redis adapters**: Caching for performance and idempotency
- **Repository pattern**: Clean separation between business logic and data access
- **Database abstraction**: Decoupling from specific database implementations

## Package Structure

```
adapters/
├── postgres/                    # PostgreSQL repositories
│   ├── transaction/            # Transaction entity persistence
│   ├── operation/              # Operation entity persistence
│   ├── balance/                # Balance entity persistence
│   ├── assetrate/              # Asset rate entity persistence
│   ├── operationroute/         # Operation route persistence
│   └── transactionroute/       # Transaction route persistence
├── mongodb/                     # MongoDB repositories
│   ├── metadata.go             # Metadata model
│   └── metadata.mongodb.go     # Metadata repository implementation
├── rabbitmq/                    # RabbitMQ adapters
│   ├── producer.rabbitmq.go    # Message producer
│   └── consumer.rabbitmq.go    # Message consumer (worker)
├── redis/                       # Redis adapters
│   ├── redis.go                # Redis operations
│   └── redis.redis.go          # Redis repository implementation
└── README.md                    # This file
```

## Repository Pattern

Each entity follows the Repository pattern with:

- **Interface definition**: Defines data access contract
- **Model struct**: Database representation
- **Repository implementation**: Concrete PostgreSQL/MongoDB implementation
- **Conversion methods**: ToEntity, FromEntity for domain model mapping

### PostgreSQL Repositories

**Transaction Repository:**

- CRUD operations for transactions
- Status management
- Parent transaction support
- Soft delete

**Operation Repository:**

- CRUD operations for operations
- Transaction relationship
- Balance tracking
- Operation type filtering

**Balance Repository:**

- CRUD operations for balances
- Account relationship
- Optimistic locking (version)
- Batch updates
- Additional balance support

**AssetRate Repository:**

- CRUD operations for exchange rates
- Currency pair lookup
- TTL management
- Upsert semantics

**OperationRoute Repository:**

- CRUD operations for operation routes
- Account rule management
- Transaction route relationship checking
- Soft delete

**TransactionRoute Repository:**

- CRUD operations for transaction routes
- Operation route relationship management (many-to-many)
- Soft delete with cascade

### MongoDB Repositories

**Metadata Repository:**

- Flexible key-value storage
- Entity-based organization
- Batch retrieval
- Upsert semantics
- Metadata filtering

### RabbitMQ Adapters

**Producer:**

- Message publishing with retry
- Exponential backoff
- Channel recovery
- Trace context propagation

**Consumer:**

- Message consumption (worker)
- Async transaction processing
- Error handling and retry

### Redis Adapters

**Redis Repository:**

- Key-value operations
- Cache management
- Idempotency key storage
- Queue operations
- Binary data support (msgpack)

## Data Models

### PostgreSQL Models

All PostgreSQL models follow the same pattern:

```go
type EntityPostgreSQLModel struct {
    ID             string
    // ... entity fields ...
    CreatedAt      time.Time
    UpdatedAt      time.Time
    DeletedAt      sql.NullTime
}

func (m *EntityPostgreSQLModel) ToEntity() *mmodel.Entity
func (m *EntityPostgreSQLModel) FromEntity(e *mmodel.Entity)
```

**Conversion Logic:**

- ToEntity: Database → Domain model
- FromEntity: Domain model → Database
- Handles status decomposition
- Handles DeletedAt conversion
- Generates UUIDv7 for new entities

### MongoDB Models

```go
type MetadataMongoDBModel struct {
    ID         primitive.ObjectID
    EntityID   string
    EntityName string
    Data       JSON
    CreatedAt  time.Time
    UpdatedAt  time.Time
}
```

## Database Operations

### Soft Delete

All entities use soft delete:

- Sets `deleted_at` timestamp
- Excluded from queries via `WHERE deleted_at IS NULL`
- Preserved for audit purposes
- Cannot be undeleted

### Optimistic Locking

Balances use version numbers:

- Incremented on each update
- UPDATE includes `WHERE version = $n`
- Prevents concurrent modification conflicts
- Returns error if version mismatch

### Batch Operations

**Balance Updates:**

```sql
UPDATE balance SET
    available = CASE
        WHEN id = $1 THEN $2
        WHEN id = $3 THEN $4
        ...
    END,
    version = version + 1
WHERE id = ANY($array)
```

### Transaction Management

- PostgreSQL transactions for consistency
- Rollback on errors
- Isolation level: Read Committed
- Connection pooling

## Caching Strategy

### Transaction Route Cache

**Key Format:**

- `accounting_routes:{org_id}:{ledger_id}:{route_id}`

**Value:**

- Msgpack-serialized TransactionRouteCache
- Pre-categorized by source/destination
- No TTL (manual invalidation)

**Invalidation:**

- On transaction route update/delete
- On operation route update (reload all affected)

### Idempotency Cache

**Key Format:**

- `idempotency:{org_id}:{ledger_id}:{key}`

**Value:**

- JSON-serialized transaction
- Stored after successful processing

**TTL:**

- Configurable (typically 24 hours)
- Automatic expiration

## Message Queue

### Published Messages

**BTO Queue (Balance-Transaction-Operation):**

- Exchange: Configurable
- Format: Msgpack
- Contains: Balances, transaction, DSL, validation
- Consumed by: Transaction worker

**Transaction Events:**

- Exchange: Configurable
- Format: JSON
- Routing key: `midaz.transaction.{STATUS}`
- Consumed by: External systems

**Audit Logs:**

- Exchange: Configurable
- Format: JSON
- Contains: All operations
- Consumed by: Audit service

### Consumed Messages

**Account Creation:**

- From: Onboarding service
- Creates: Initial balances

**BTO Processing:**

- From: Transaction service (self)
- Updates: Balances, creates operations

## Error Handling

### PostgreSQL Errors

Mapped to business errors:

- `23505` (unique violation) → Duplicate error
- `23503` (foreign key violation) → Not found error
- `sql.ErrNoRows` → Entity not found

### MongoDB Errors

- Connection errors → Internal server error
- Not found → Returns nil (not an error)
- Duplicate key → Handled gracefully

### RabbitMQ Errors

- Connection failures → Retry with backoff
- Publish failures → Fallback to sync
- **CRITICAL:** Some code uses logger.Fatalf (crashes server)

### Redis Errors

- Connection failures → Logged, operation continues
- Cache miss → Fetch from database
- Set failures → Logged, operation continues

## Performance Considerations

### Connection Pooling

**PostgreSQL:**

- Primary for writes
- Replica for reads (if configured)
- Max connections: Configurable

**MongoDB:**

- Max pool size: Configurable
- Automatic connection management

**Redis:**

- Pool size: Configurable
- Min idle connections: Configurable

### Query Optimization

- Indexed columns: id, organization_id, ledger_id, account_id
- Batch operations for multiple records
- Cursor pagination for large result sets
- Squirrel query builder for complex queries

### Caching

- Transaction routes cached for fast lookups
- Idempotency keys prevent duplicate processing
- Metadata cached at service layer

## Security

- Parameterized queries (no SQL injection)
- Prepared statements
- Connection pooling limits
- TLS support for all connections
- Audit logging for compliance

## Related Packages

- `internal/services`: Business logic layer
- `internal/bootstrap`: Application initialization
- `pkg/mmodel`: Domain models

## Notes

- All repositories panic on connection failures (intentional fail-fast)
- Soft delete is used throughout for audit purposes
- Version numbers prevent race conditions in balance updates
- Msgpack used for efficient binary serialization in queues and cache
