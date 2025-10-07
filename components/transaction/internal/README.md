# Transaction Service - Internal

## Overview

This directory contains the internal implementation of the Midaz transaction service, organized following hexagonal architecture (ports and adapters) principles. The transaction service manages financial transactions, account balances, operations, and double-entry accounting.

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
                         ↓
┌─────────────────────────────────────────────────────────┐
│                   Services Layer                         │
│              (Business Logic / Domain)                   │
│                                                          │
│  ┌──────────────────┐      ┌──────────────────┐        │
│  │  Command Side    │      │   Query Side     │        │
│  │  (Write Ops)     │      │   (Read Ops)     │        │
│  └──────────────────┘      └──────────────────┘        │
│                                                          │
│  • Transaction processing    • Transaction queries      │
│  • Balance management        • Balance queries          │
│  • Operation creation        • Operation queries        │
│  • Routing management        • Routing queries          │
│  • Event publishing          • Metadata enrichment      │
└────────────────────────┬────────────────────────────────┘
                         │
                         ↓
┌─────────────────────────────────────────────────────────┐
│                   Adapters Layer                         │
│                  (Infrastructure)                        │
│                                                          │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐ │
│  │  PostgreSQL  │  │   MongoDB    │  │   RabbitMQ   │ │
│  │ Repositories │  │ Repositories │  │   Producer   │ │
│  └──────────────┘  └──────────────┘  └──────────────┘ │
│                                                          │
│  ┌──────────────┐                                       │
│  │    Redis     │                                       │
│  │  Repository  │                                       │
│  └──────────────┘                                       │
└─────────────────────────────────────────────────────────┘
```

## Layer Responsibilities

### Services Layer

**Purpose:** Business logic and use case orchestration

**Responsibilities:**

- Transaction validation and processing
- Double-entry accounting enforcement
- Balance calculation and updates
- Operation creation from DSL specifications
- Routing rule management
- Event publishing
- Cache management
- Idempotency enforcement

**Pattern:** CQRS (Command Query Responsibility Segregation)

- Command side: Write operations
- Query side: Read operations

### Adapters Layer

**Purpose:** Infrastructure and external system integration

**Responsibilities:**

- Database persistence (PostgreSQL, MongoDB)
- Message queue operations (RabbitMQ)
- Caching operations (Redis)
- External API calls
- Data format conversion

**Pattern:** Repository pattern

- Interface definitions
- Concrete implementations
- Database abstraction

### Bootstrap Layer

**Purpose:** Application initialization and dependency injection

**Responsibilities:**

- Configuration loading
- Database connection setup
- Repository instantiation
- Use case wiring
- HTTP server configuration
- Middleware setup

## Data Flow

### Transaction Creation Flow

```
1. HTTP Request
   ↓
2. HTTP Handler (adapters/http/in)
   ↓
3. Command Use Case (services/command)
   ├→ Check idempotency (Redis) ⚠️ MUST BE FIRST
   ├→ Parse DSL or validate JSON
   ├→ Fetch balances (PostgreSQL)
   ├→ Validate transaction (lib-commons)
   ├→ Create transaction (PostgreSQL)
   ├→ Create metadata (MongoDB)
   ├→ Execute transaction (sync or async)
   │  ├→ Update balances (PostgreSQL)
   │  ├→ Create operations (PostgreSQL)
   │  └→ Publish events (RabbitMQ)
   └→ Return transaction
```

### Query Flow

```
1. HTTP Request
   ↓
2. HTTP Handler (adapters/http/in)
   ↓
3. Query Use Case (services/query)
   ├→ Fetch entity (PostgreSQL)
   ├→ Fetch metadata (MongoDB)
   ├→ Merge metadata
   └→ Return enriched entity
```

## Key Features

### 1. Double-Entry Accounting

Every transaction maintains the accounting equation:

- Assets = Liabilities + Equity
- Debits = Credits
- Balance changes are atomic

### 2. Transaction Processing Modes

**Synchronous:**

- Immediate processing
- Returns completed transaction
- Lower throughput
- Immediate consistency

**Asynchronous:**

- Queue-based processing
- Returns pending transaction
- Higher throughput
- Eventual consistency

### 3. Balance Management

**Balance Structure:**

- Available: Funds available for use
- On-Hold: Temporarily held funds
- Version: Optimistic locking

**Balance Operations:**

- DEBIT: Decrease available
- CREDIT: Increase available
- ON_HOLD: Move to on-hold
- RELEASE: Release from on-hold

### 4. Transaction Routing

**Automated Routing:**

- Define routing rules (transaction routes)
- Specify account selection criteria (operation routes)
- Match accounts by alias or type
- Enable complex routing logic

**Caching:**

- Routes cached in Redis
- Msgpack serialization
- Manual invalidation

### 5. Idempotency

Prevents duplicate transactions:

- Idempotency-Key header
- Stored in Redis
- Configurable TTL
- Returns cached result for duplicates

### 6. Event-Driven Architecture

**Published Events:**

- Transaction status changes
- Audit logs for operations
- Account balance updates

**Consumers:**

- External webhooks
- Analytics systems
- Audit storage
- Notification services

## Technologies

### Databases

**PostgreSQL:**

- Primary entity storage
- ACID transactions
- Soft delete support
- Optimistic locking

**MongoDB:**

- Metadata storage
- Flexible schema
- Document model

### Message Queue

**RabbitMQ:**

- Async transaction processing
- Event publishing
- Audit logging
- Retry logic with backoff

### Cache

**Redis:**

- Transaction route caching
- Idempotency keys
- Queue tracking
- Binary data support

### Libraries

- **lib-commons**: Shared utilities and transaction validation
- **Fiber**: HTTP web framework
- **Squirrel**: SQL query builder
- **msgpack**: Binary serialization
- **OpenTelemetry**: Distributed tracing

## Configuration

Key environment variables:

- `RABBITMQ_TRANSACTION_ASYNC`: Enable async processing
- `RABBITMQ_TRANSACTION_EVENTS_ENABLED`: Enable event publishing
- `AUDIT_LOG_ENABLED`: Enable audit logging
- See bootstrap/README.md for complete list

## Testing

- Unit tests for use cases
- Integration tests for repositories
- Mock repositories for isolated testing
- Test fixtures for common scenarios

## Monitoring

- OpenTelemetry tracing
- Structured logging (Zap)
- Prometheus metrics
- Health check endpoints

## Known Issues

**Critical Bugs:**

- logger.Fatalf() in send-log-transaction-audit-queue.go (crashes server)
- See BUGS.md for complete list

## Related Documentation

- `services/README.md`: Business logic documentation
- `adapters/README.md`: Infrastructure documentation
- `bootstrap/README.md`: Initialization documentation
- `../../README.md`: Service overview
