# Onboarding Service - Internal

## Overview

This directory contains the internal implementation of the Midaz onboarding service, organized following hexagonal architecture (ports and adapters) principles. The onboarding service manages the creation and lifecycle of organizations, ledgers, accounts, assets, and related entities.

## Architecture

The service follows a layered architecture with clear separation of concerns:

```
internal/
├── services/        # Business Logic Layer (Domain/Application)
├── adapters/        # Infrastructure Layer (Ports & Adapters)
└── bootstrap/       # Application Initialization
```

### Hexagonal Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    HTTP Handlers                         │
│                  (External Layer)                        │
└────────────────────────┬────────────────────────────────┘
                         │
┌────────────────────────▼────────────────────────────────┐
│                  Services Layer                          │
│            (Business Logic / Use Cases)                  │
│  ┌──────────────┐              ┌──────────────┐        │
│  │   Command    │              │    Query     │        │
│  │  (Writes)    │              │   (Reads)    │        │
│  └──────────────┘              └──────────────┘        │
└────────────────────────┬────────────────────────────────┘
                         │ depends on
┌────────────────────────▼────────────────────────────────┐
│              Repository Interfaces                       │
│           (Defined in adapters package)                  │
└────────────────────────┬────────────────────────────────┘
                         │ implemented by
┌────────────────────────▼────────────────────────────────┐
│                 Adapters Layer                           │
│          (Infrastructure Implementations)                │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐             │
│  │PostgreSQL│  │ MongoDB  │  │ RabbitMQ │             │
│  └──────────┘  └──────────┘  └──────────┘             │
└─────────────────────────────────────────────────────────┘
```

## Directory Structure

### services/ - Business Logic Layer

**Status**: ✅ **100% DOCUMENTED** (55 files, ~7,000 lines)

Contains the core business logic organized using CQRS pattern:

- **command/**: Write operations (create, update, delete)

  - All CRUD operations for entities
  - Business rule validation
  - Event publishing
  - Metadata management

- **query/**: Read operations (get, list, count)
  - Single entity retrieval
  - Paginated lists
  - Metadata-based queries
  - Count operations for pagination

**Key Features:**

- CQRS pattern separation
- Business rule enforcement
- Error handling and validation
- OpenTelemetry tracing
- RabbitMQ event publishing

**Documentation**: See [services/README.md](services/README.md)

### adapters/ - Infrastructure Layer

**Status**: 🔄 **MODELS DOCUMENTED** (13 files, ~3,000 lines)

Implements repository interfaces for data persistence:

- **postgres/**: PostgreSQL repositories for entities

  - Organization, Ledger, Account, Asset
  - Portfolio, Segment, AccountType
  - CRUD operations with soft deletes
  - Query builders for complex filters

- **mongodb/**: MongoDB repositories for metadata

  - Flexible schema-less storage
  - Metadata enrichment
  - Batch operations

- **rabbitmq/**: Message queue adapters
  - Event publishing
  - Async communication

**Key Features:**

- Repository pattern
- Database abstraction
- Model conversion (DB ↔ Domain)
- Error conversion
- Connection pooling

**Documentation**: See [adapters/README.md](adapters/README.md)

### bootstrap/ - Application Initialization

**Status**: ⏳ **PENDING DOCUMENTATION**

Handles application startup and dependency injection:

- Database connection setup
- Repository initialization
- Service layer wiring
- Configuration loading

## Service Responsibilities

The onboarding service manages:

### 1. Organizations

- Top-level entities representing companies/tenants
- Hierarchical structure (parent-child relationships)
- Address and legal information
- Status management

### 2. Ledgers

- Containers for financial data within organizations
- Unique naming within organization
- Status tracking

### 3. Assets

- Currencies, cryptocurrencies, commodities
- Type classification
- Code validation (ISO 4217 for currencies)
- Automatic external account creation

### 4. Accounts

- Fundamental units for balance tracking
- Hierarchical structure (parent-child)
- Asset-specific (one asset per account)
- Portfolio and segment grouping
- Alias-based identification

### 5. Portfolios

- Logical grouping of accounts
- Organizational structure
- Entity ID linking

### 6. Segments

- Logical divisions within ledgers
- Regional/departmental organization
- Unique naming within ledger

### 7. Account Types

- Account classification
- Accounting validation rules
- Optional enforcement

### 8. Metadata

- Flexible key-value storage
- Entity enrichment
- Custom fields
- MongoDB-based

## Data Flow

### Create Entity Flow

```
1. HTTP Request
   ↓
2. Handler validates and decodes
   ↓
3. Service layer (Command)
   - Validates business rules
   - Checks dependencies
   - Generates UUID
   ↓
4. PostgreSQL Repository
   - Persists entity
   - Returns created entity
   ↓
5. MongoDB Repository
   - Stores metadata
   ↓
6. RabbitMQ (if applicable)
   - Publishes event
   ↓
7. HTTP Response
```

### Query Entity Flow

```
1. HTTP Request with filters
   ↓
2. Handler validates parameters
   ↓
3. Service layer (Query)
   - Parses filters
   - Applies pagination
   ↓
4. PostgreSQL Repository
   - Executes query
   - Returns entities
   ↓
5. MongoDB Repository
   - Fetches metadata (batch)
   - Enriches entities
   ↓
6. HTTP Response with enriched data
```

## Error Handling

### Error Flow

```
Database Error
   ↓
Repository converts to business error
   ↓
Service layer validates and enriches
   ↓
HTTP handler converts to HTTP response
```

### Error Types

- **EntityNotFoundError**: Entity doesn't exist
- **ValidationError**: Business rule violation
- **EntityConflictError**: Duplicate or constraint violation
- **InternalServerError**: Unexpected errors

## Testing Strategy

### Unit Tests

- Service layer business logic
- Model conversions
- Validation rules

### Integration Tests

- Repository operations
- Database constraints
- Transaction behavior

### End-to-End Tests

- Full request/response cycle
- Multi-service interactions
- Event publishing

## Performance Considerations

### Caching

- Metadata cached in MongoDB
- Repository-level caching for frequent queries

### Batch Operations

- Bulk metadata retrieval
- Batch entity fetching by IDs

### Pagination

- Limit/offset pagination
- Cursor-based pagination for large datasets
- Count queries optimized

### Indexing

- Primary keys (UUID)
- Foreign keys
- Frequently queried fields (alias, code, name)
- Metadata fields in MongoDB

## Observability

### OpenTelemetry Tracing

All operations traced with spans:

- Service layer: `command.*` and `query.*`
- Repository layer: `postgres.*` and `mongodb.*`
- Span attributes include entity IDs, types, and operations

### Logging

Structured logging with context:

- Request IDs
- Entity IDs
- Operation types
- Error details

### Metrics

- Operation latency
- Error rates
- Entity counts
- Database connection pool stats

## Configuration

### Environment Variables

- Database connection strings
- MongoDB connection
- RabbitMQ configuration
- Feature flags (e.g., ACCOUNT_TYPE_VALIDATION)

### Feature Flags

- `ACCOUNT_TYPE_VALIDATION`: Enable/disable account type validation per org:ledger

## Dependencies

### External Libraries

- **lib-commons**: Common utilities, database connections, tracing
- **squirrel**: SQL query builder
- **pgx**: PostgreSQL driver
- **mongo-driver**: MongoDB driver
- **fiber**: HTTP framework

### Internal Packages

- **pkg/mmodel**: Domain models
- **pkg/constant**: Error codes and constants
- **pkg/errors**: Error handling utilities
- **pkg/net/http**: HTTP utilities

## Migration and Deployment

### Database Migrations

Located in `migrations/` directory at service root:

- Sequential numbered migrations
- Up and down migrations
- Applied automatically on startup

### Deployment Considerations

- Database connection pooling
- Graceful shutdown
- Health checks
- Readiness probes

## Future Enhancements

### Planned Features

- Event sourcing for audit trail
- CQRS read models for performance
- GraphQL API support
- Advanced querying capabilities

### Technical Debt

- See [BUGS.md](../../../BUGS.md) for identified issues
- Repository implementation completion
- Additional test coverage

---

## Documentation Status

**Overall Progress**: ~30% complete

- ✅ **pkg/ layer**: 100% documented
- ✅ **services/ layer**: 100% documented
- 🔄 **adapters/ layer**: Models documented, repositories in progress
- ⏳ **bootstrap/ layer**: Pending

**Total Documentation**: ~20,000 lines across 98 files

---

**Note**: This service follows clean architecture principles with clear separation between business logic (services) and infrastructure (adapters). The service layer is database-agnostic and can be tested independently of infrastructure concerns.
