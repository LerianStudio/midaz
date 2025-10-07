# Transaction Service

## Overview

The **transaction service** is a core component of the Midaz ledger platform responsible for processing financial transactions, managing account balances, tracking operations, and enforcing double-entry accounting principles. It provides RESTful APIs and async processing for high-throughput transaction handling.

## Purpose

The transaction service:

- **Processes transactions**: Validates and executes financial transactions
- **Manages balances**: Tracks available and on-hold amounts for accounts
- **Records operations**: Creates debit/credit/hold/release operations
- **Enforces accounting rules**: Double-entry bookkeeping, balance validation
- **Provides routing**: Automated transaction routing based on rules
- **Publishes events**: Transaction events and audit logs
- **Supports async processing**: Queue-based for high throughput
- **Implements idempotency**: Prevents duplicate transactions

## Architecture

The service follows **hexagonal architecture** (ports and adapters):

```
transaction/
├── cmd/                    # Application entry point
├── internal/               # Internal implementation
│   ├── services/          # Business logic (CQRS)
│   ├── adapters/          # Infrastructure
│   └── bootstrap/         # Initialization
├── api/                    # OpenAPI specifications
├── migrations/             # Database migrations
└── README.md              # This file
```

## Key Features

### 1. Transaction Processing

**Input Formats:**

- JSON: Direct transaction specification
- DSL (Gold): Domain-specific language for complex transactions

**Processing Modes:**

- Sync: Immediate processing, returns completed transaction
- Async: Queue-based, returns pending transaction

**Validation:**

- Account existence
- Balance sufficiency
- Double-entry balance
- Asset code matching

### 2. Balance Management

**Balance Types:**

- Default balance (key: "default")
- Additional balances (custom keys)

**Balance Components:**

- Available: Funds available for use
- On-Hold: Temporarily held funds
- Version: Optimistic locking counter

**Operations:**

- Create: Initialize account balances
- Update: Modify balance amounts
- Delete: Remove empty balances

### 3. Double-Entry Accounting

**Principles:**

- Every transaction has debits and credits
- Total debits = Total credits
- Balance changes are atomic
- Audit trail for all operations

**Operation Types:**

- DEBIT: Money leaving account
- CREDIT: Money entering account
- ON_HOLD: Reserve funds
- RELEASE: Release held funds

### 4. Transaction Routing

**Components:**

- **Transaction Routes**: Define transaction flow patterns
- **Operation Routes**: Define account selection rules

**Use Cases:**

- Automated account selection
- Rule-based routing
- Complex transaction patterns
- Multi-account distributions

**Caching:**

- Routes cached in Redis
- Invalidated on updates
- Performance optimization

### 5. Asset Rates

**Purpose:**

- Currency conversion
- Exchange rate management
- Multi-currency transactions

**Features:**

- Upsert semantics
- TTL support
- Source tracking
- Scale factors

### 6. Idempotency

**Implementation:**

- Idempotency-Key header
- Redis storage
- Configurable TTL
- Duplicate detection

**Benefits:**

- Prevents duplicate transactions
- Safe retry logic
- Network failure resilience

### 7. Event-Driven Architecture

**Published Events:**

- Transaction status changes
- Operation audit logs
- Balance updates

**Event Format:**

```json
{
  "source": "midaz",
  "event_type": "transaction",
  "action": "APPROVED",
  "timestamp": "2025-10-07T12:00:00Z",
  "organization_id": "uuid",
  "ledger_id": "uuid",
  "payload": { ... }
}
```

## API Endpoints

### Transactions

- `POST /v1/organizations/:org_id/ledgers/:ledger_id/transactions` - Create transaction (JSON)
- `POST /v1/organizations/:org_id/ledgers/:ledger_id/transactions/dsl` - Create transaction (DSL)
- `GET /v1/organizations/:org_id/ledgers/:ledger_id/transactions` - List transactions
- `GET /v1/organizations/:org_id/ledgers/:ledger_id/transactions/:id` - Get transaction
- `PATCH /v1/organizations/:org_id/ledgers/:ledger_id/transactions/:id` - Update transaction

### Balances

- `GET /v1/organizations/:org_id/ledgers/:ledger_id/balances` - List balances
- `GET /v1/organizations/:org_id/ledgers/:ledger_id/balances/:id` - Get balance
- `POST /v1/organizations/:org_id/ledgers/:ledger_id/accounts/:account_id/balances` - Create additional balance
- `PATCH /v1/organizations/:org_id/ledgers/:ledger_id/balances/:id` - Update balance
- `DELETE /v1/organizations/:org_id/ledgers/:ledger_id/balances/:id` - Delete balance

### Operations

- `GET /v1/organizations/:org_id/ledgers/:ledger_id/operations` - List operations
- `GET /v1/organizations/:org_id/ledgers/:ledger_id/operations/:id` - Get operation
- `PATCH /v1/organizations/:org_id/ledgers/:ledger_id/operations/:id` - Update operation

### Asset Rates

- `POST /v1/organizations/:org_id/ledgers/:ledger_id/asset-rates` - Create/update asset rate
- `GET /v1/organizations/:org_id/ledgers/:ledger_id/asset-rates` - List asset rates
- `GET /v1/organizations/:org_id/ledgers/:ledger_id/asset-rates/:id` - Get asset rate

### Transaction Routes

- `POST /v1/organizations/:org_id/ledgers/:ledger_id/transaction-routes` - Create transaction route
- `GET /v1/organizations/:org_id/ledgers/:ledger_id/transaction-routes` - List transaction routes
- `GET /v1/organizations/:org_id/ledgers/:ledger_id/transaction-routes/:id` - Get transaction route
- `PATCH /v1/organizations/:org_id/ledgers/:ledger_id/transaction-routes/:id` - Update transaction route
- `DELETE /v1/organizations/:org_id/ledgers/:ledger_id/transaction-routes/:id` - Delete transaction route

### Operation Routes

- Similar REST patterns

## Database Schema

### PostgreSQL Tables

**Core tables:**

- `transaction` - Financial transactions
- `operation` - Debit/credit operations
- `balance` - Account balances
- `asset_rate` - Exchange rates
- `operation_route` - Account selection rules
- `transaction_route` - Transaction routing rules
- `transaction_route_operation_route` - Many-to-many junction table

**Schema Features:**

- UUIDv7 primary keys
- Foreign key constraints
- Unique constraints
- Soft delete support
- Optimistic locking (balance.version)

### MongoDB Collections

**Metadata collections:**

- `Transaction` - Transaction metadata
- `Operation` - Operation metadata
- `AssetRate` - Asset rate metadata
- `TransactionRoute` - Transaction route metadata
- `OperationRoute` - Operation route metadata

## Message Queue

### Published Messages

**BTO Queue (Balance-Transaction-Operation):**

- Exchange: Configurable
- Format: Msgpack
- Purpose: Async transaction processing

**Transaction Events:**

- Exchange: Configurable
- Format: JSON
- Routing key: `midaz.transaction.{STATUS}`
- Purpose: External event consumers

**Audit Logs:**

- Exchange: Configurable
- Format: JSON
- Purpose: Compliance and forensics

### Consumed Messages

**Account Creation:**

- From: Onboarding service
- Purpose: Initialize balances

**BTO Processing:**

- From: Transaction service (self)
- Purpose: Async transaction completion

## Processing Modes

### Synchronous Mode

**Flow:**

1. Receive request
2. Validate transaction
3. Create transaction (PENDING)
4. Update balances
5. Create operations
6. Update status (APPROVED)
7. Return completed transaction

**Characteristics:**

- Immediate consistency
- Lower throughput
- Simpler debugging
- No queue dependency

### Asynchronous Mode

**Flow:**

1. Receive request
2. Validate transaction
3. Create transaction (PENDING)
4. Publish to queue
5. Return pending transaction

Worker: 6. Consume from queue 7. Update balances 8. Create operations 9. Update status (APPROVED) 10. Publish events

**Characteristics:**

- Eventual consistency
- Higher throughput
- Horizontal scaling
- Queue dependency

## Caching

### Transaction Route Cache

**Purpose:** Fast route lookup during transaction processing

**Storage:** Redis with msgpack
**Key:** `accounting_routes:{org_id}:{ledger_id}:{route_id}`
**TTL:** No expiration (manual invalidation)

### Idempotency Cache

**Purpose:** Prevent duplicate transactions

**Storage:** Redis with JSON
**Key:** `idempotency:{org_id}:{ledger_id}:{key}`
**TTL:** Configurable (default: 24 hours)

## Security

- JWT authentication (optional)
- Parameterized SQL queries
- Input validation
- Audit logging
- TLS support

## Performance

### Optimizations

- Connection pooling
- Batch balance updates
- Route caching
- Async processing
- Cursor pagination

### Scalability

- Horizontal scaling (async mode)
- Read replicas (queries)
- Queue-based processing
- Stateless design

## Observability

### Logging

Structured logging with:

- Request IDs
- Transaction IDs
- Log levels
- JSON format

### Tracing

OpenTelemetry spans for:

- HTTP requests
- Database queries
- Queue operations
- Business operations

### Metrics

- Transaction throughput
- Processing duration
- Queue depth
- Error rates

## Known Issues

**Critical:**

- logger.Fatalf() crashes server (see BUGS.md)

**Recommendations:**

- Replace Fatal logs with error returns
- Add circuit breakers
- Implement dead letter queues
- Add retry policies

## Related Services

- **Onboarding Service**: Creates accounts, publishes to queue
- **Console**: Web UI
- **MDZ CLI**: Command-line interface

## Documentation

- `internal/services/README.md`: Business logic details
- `internal/adapters/README.md`: Infrastructure details
- `api/`: OpenAPI specifications
- `migrations/`: Database schema

## Support

- GitHub Issues: https://github.com/LerianStudio/midaz/issues
- Documentation: https://docs.midaz.io
- Community: https://discord.gg/midaz
