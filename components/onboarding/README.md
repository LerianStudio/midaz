# Onboarding Service

## Overview

The **onboarding service** is a core component of the Midaz ledger platform responsible for managing the lifecycle of organizations, ledgers, accounts, assets, portfolios, segments, and account types. It provides RESTful APIs for creating, reading, updating, and deleting these entities.

## Purpose

The onboarding service:

- **Manages entity lifecycle**: CRUD operations for all core entities
- **Enforces business rules**: Validation, uniqueness constraints, accounting rules
- **Coordinates with transaction service**: Sends account creation events via RabbitMQ
- **Provides metadata storage**: Flexible key-value metadata for all entities
- **Supports hierarchical structures**: Parent-child relationships (organizations, accounts)
- **Implements CQRS pattern**: Separate command and query responsibilities

## Architecture

The service follows **hexagonal architecture** (ports and adapters) with clear separation of concerns:

```
onboarding/
├── cmd/                    # Application entry point
├── internal/               # Internal implementation (not importable)
│   ├── services/          # Business logic layer (domain/application)
│   ├── adapters/          # Infrastructure layer (ports & adapters)
│   └── bootstrap/         # Application initialization
├── api/                    # OpenAPI specifications
├── migrations/             # Database migration scripts
└── README.md              # This file
```

### Layer Responsibilities

**Services Layer (Business Logic):**

- Implements use cases
- Enforces business rules
- Orchestrates repository operations
- Handles metadata enrichment
- Publishes events to RabbitMQ

**Adapters Layer (Infrastructure):**

- PostgreSQL repositories (entity persistence)
- MongoDB repositories (metadata storage)
- RabbitMQ producers (event publishing)
- Redis consumers (caching and idempotency)
- HTTP handlers (REST API)

**Bootstrap Layer (Application):**

- Configuration loading
- Dependency injection
- Server initialization
- Graceful shutdown

## Entity Hierarchy

The onboarding service manages entities in a hierarchical structure:

```
Organization (top level)
  └── Ledger
      ├── Asset (currencies, cryptocurrencies, commodities)
      │   └── External Account (auto-created for each asset)
      ├── Portfolio (grouping of accounts)
      ├── Segment (logical divisions)
      ├── Account (financial buckets)
      │   ├── Parent Account (optional hierarchy)
      │   └── Balance (managed by transaction service)
      └── AccountType (optional account classification)
```

## Key Features

### 1. CQRS Pattern

The service separates read and write operations:

**Command Side (Write):**

- Create, update, delete operations
- Business rule enforcement
- Event publishing
- Located in `internal/services/command/`

**Query Side (Read):**

- Get, list, count operations
- Metadata enrichment
- Pagination support
- Located in `internal/services/query/`

### 2. Metadata Support

All entities support flexible metadata:

- Stored in MongoDB for schema flexibility
- Key-value pairs (max 2000 chars per value)
- Supports RFC 7396 JSON Merge Patch
- Queryable via metadata filters

### 3. Soft Delete

All entities use soft delete:

- `deleted_at` timestamp instead of physical deletion
- Preserved for audit purposes
- Excluded from normal queries
- Cannot be undeleted

### 4. Account Creation Flow

When an account is created:

1. Onboarding service validates and persists account
2. Account event is published to RabbitMQ
3. Transaction service consumes event
4. Transaction service initializes balances
5. Account becomes available for transactions

### 5. Asset and External Account

When an asset is created:

1. Asset is persisted in onboarding service
2. External account is auto-created with alias `@external/{CODE}`
3. External account event is published to RabbitMQ
4. Transaction service initializes external account balances
5. External account is used for transactions with external parties

### 6. Validation

The service implements multiple validation layers:

- **HTTP layer**: Request body validation (required fields, formats)
- **Business layer**: Business rule validation (uniqueness, relationships)
- **Database layer**: Constraints (foreign keys, unique indexes)

## API Endpoints

### Organizations

- `POST /v1/organizations` - Create organization
- `GET /v1/organizations` - List organizations
- `GET /v1/organizations/:id` - Get organization by ID
- `PATCH /v1/organizations/:id` - Update organization
- `DELETE /v1/organizations/:id` - Delete organization

### Ledgers

- `POST /v1/organizations/:organization_id/ledgers` - Create ledger
- `GET /v1/organizations/:organization_id/ledgers` - List ledgers
- `GET /v1/organizations/:organization_id/ledgers/:id` - Get ledger
- `PATCH /v1/organizations/:organization_id/ledgers/:id` - Update ledger
- `DELETE /v1/organizations/:organization_id/ledgers/:id` - Delete ledger

### Assets

- `POST /v1/organizations/:organization_id/ledgers/:ledger_id/assets` - Create asset
- `GET /v1/organizations/:organization_id/ledgers/:ledger_id/assets` - List assets
- `GET /v1/organizations/:organization_id/ledgers/:ledger_id/assets/:id` - Get asset
- `PATCH /v1/organizations/:organization_id/ledgers/:ledger_id/assets/:id` - Update asset
- `DELETE /v1/organizations/:organization_id/ledgers/:ledger_id/assets/:id` - Delete asset

### Accounts

- `POST /v1/organizations/:organization_id/ledgers/:ledger_id/accounts` - Create account
- `GET /v1/organizations/:organization_id/ledgers/:ledger_id/accounts` - List accounts
- `GET /v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:id` - Get account
- `PATCH /v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:id` - Update account
- `DELETE /v1/organizations/:organization_id/ledgers/:ledger_id/accounts/:id` - Delete account

### Portfolios, Segments, Account Types

- Similar REST patterns for each entity type

## Database Schema

### PostgreSQL Tables

**Primary entity tables:**

- `organization` - Organizations
- `ledger` - Ledgers
- `asset` - Assets
- `account` - Accounts
- `portfolio` - Portfolios
- `segment` - Segments
- `account_type` - Account types

**Schema Features:**

- UUIDv7 primary keys (time-ordered)
- Foreign key constraints
- Unique constraints (names, aliases, codes)
- Soft delete support (deleted_at)
- Timestamps (created_at, updated_at)

### MongoDB Collections

**Metadata collections:**

- `organization` - Organization metadata
- `ledger` - Ledger metadata
- `asset` - Asset metadata
- `account` - Account metadata
- `portfolio` - Portfolio metadata
- `segment` - Segment metadata
- `account_type` - Account type metadata

**Document Structure:**

```json
{
  "_id": ObjectId,
  "entity_id": "uuid",
  "entity_name": "Account",
  "metadata": {
    "custom_field_1": "value1",
    "custom_field_2": "value2"
  },
  "created_at": ISODate,
  "updated_at": ISODate
}
```

## Message Queue

### RabbitMQ Integration

**Published Events:**

- Account creation events (to transaction service)
- External account creation events (for assets)

**Message Format:**

```json
{
  "entity_id": "uuid",
  "entity_name": "Account",
  "operation": "CREATE",
  "data": { ... account data ... }
}
```

**Retry Logic:**

- Max retries: 5
- Initial backoff: 500ms
- Max backoff: 10s
- Full jitter to prevent thundering herd

## Caching

### Redis Integration

**Cached Data:**

- Idempotency keys (for duplicate request prevention)
- Session data (if authentication enabled)

**Cache Configuration:**

- TTL: Configurable per operation
- Eviction: LRU (Least Recently Used)
- Persistence: Optional (depends on Redis configuration)

## Observability

### Logging

The service uses structured logging (Zap) with:

- Request IDs for correlation
- Log levels (debug, info, warn, error)
- JSON format for log aggregation

### Tracing

OpenTelemetry tracing for:

- HTTP requests (automatic via middleware)
- Database queries (automatic via lib-commons)
- RabbitMQ messages (automatic via lib-commons)
- Business operations (manual spans in use cases)

### Metrics

Automatic metrics collection for:

- HTTP request duration and status codes
- Database query duration
- RabbitMQ message publish success/failure
- Connection pool statistics

## Security

### Authentication

Optional JWT-based authentication via Casdoor:

- Validates JWT tokens
- Extracts user claims
- Enforces authorization rules

### Input Validation

Multiple validation layers:

- JSON schema validation
- Field format validation (UUIDs, email, etc.)
- Business rule validation
- SQL injection prevention
- Null byte detection

### Data Protection

- SQL parameterized queries (no string concatenation)
- Prepared statements
- Connection pooling with limits
- TLS support for all connections

## Testing

### Test Structure

```
tests/
├── integration/        # Integration tests
├── fixtures/          # Test data
└── helpers/           # Test utilities
```

### Running Tests

```bash
# Unit tests
make test

# Integration tests
make test-integration

# Coverage report
make coverage
```

## Deployment

### Docker

```bash
# Build image
docker build -t midaz-onboarding:latest .

# Run container
docker run -p 3000:3000 \
  -e DB_HOST=postgres \
  -e MONGO_HOST=mongodb \
  -e RABBITMQ_HOST=rabbitmq \
  midaz-onboarding:latest
```

### Kubernetes

The service is designed for Kubernetes deployment:

- Health check endpoints
- Graceful shutdown
- 12-factor app principles
- Environment-based configuration
- Stateless design

## Performance

### Optimization Strategies

**Database:**

- Connection pooling (primary + replica)
- Read replica for queries
- Indexed columns (id, organization_id, ledger_id, alias)
- Batch operations for metadata

**Caching:**

- Redis for idempotency keys
- Metadata caching (optional)
- Query result caching (optional)

**Message Queue:**

- Async processing for account creation
- Decouples onboarding from transaction service
- Retry logic for reliability

## Monitoring

### Health Checks

- `GET /health` - Service health status
- Checks: PostgreSQL, MongoDB, RabbitMQ, Redis

### Metrics Endpoints

- Prometheus metrics (if enabled)
- OpenTelemetry metrics export

## Related Services

- **Transaction Service**: Consumes account creation events, manages balances
- **Console**: Web UI for managing entities
- **MDZ CLI**: Command-line interface for API operations

## Documentation

- **API Documentation**: See `api/` directory for OpenAPI specs
- **Internal Documentation**: See `internal/README.md` for architecture details
- **Migration Guide**: See `migrations/` for database schema evolution

## Support

For issues, questions, or contributions:

- GitHub Issues: https://github.com/LerianStudio/midaz/issues
- Documentation: https://docs.midaz.io
- Community: https://discord.gg/midaz
