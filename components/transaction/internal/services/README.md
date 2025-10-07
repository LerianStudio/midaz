# Transaction Services

## Overview

The `services` package implements the business logic layer for the Midaz transaction service using the CQRS (Command Query Responsibility Segregation) pattern. This layer orchestrates transaction processing, balance management, and double-entry accounting operations.

## Purpose

This package provides:

- **Transaction processing**: Create, validate, and execute financial transactions
- **Balance management**: Create, update, and delete account balances
- **Operation tracking**: Record debits, credits, holds, and releases
- **Asset rate management**: Currency conversion and exchange rates
- **Transaction routing**: Automated transaction routing based on rules
- **Event publishing**: Audit logs and transaction events
- **Cache management**: Performance optimization for routing
- **Idempotency**: Duplicate request prevention

## Package Structure

```
services/
├── command/              # Command side (write operations)
│   ├── command.go       # UseCase struct and repository aggregation
│   ├── create-transaction.go
│   ├── create-balance.go
│   ├── create-operation.go
│   ├── create-assetrate.go
│   ├── create-transaction-route.go
│   ├── create-operation-route.go
│   ├── create-balance-additional.go
│   ├── update-transaction.go
│   ├── update-balance.go
│   ├── update-operation.go
│   ├── update-metadata.go
│   ├── update-transaction-route.go
│   ├── update-operation-route.go
│   ├── delete_balance.go
│   ├── delete-transaction-route.go
│   ├── delete-operation-route.go
│   ├── create-idempotency-key.go
│   ├── send-bto-execute-async.go
│   ├── create-balance-transaction-operations-async.go
│   ├── send-transaction-events.go
│   ├── send-log-transaction-audit-queue.go
│   ├── create-transaction-route-cache.go
│   ├── delete-transaction-route-cache.go
│   └── reload-operation-route-cache.go
├── query/                # Query side (read operations)
│   ├── query.go         # UseCase struct and repository aggregation
│   └── [query operations]
└── errors.go            # Error handling utilities
```

## CQRS Pattern

### Command Side (Write Operations)

The command side handles all write operations with business logic enforcement:

**Transaction Operations:**

- `CreateTransaction`: Create transaction record
- `UpdateTransaction`: Update description and metadata
- `UpdateTransactionStatus`: Change transaction status

**Balance Operations:**

- `CreateBalance`: Initialize account balances (from queue)
- `CreateAdditionalBalance`: Create additional balance entries
- `UpdateBalances`: Batch update balances after transaction
- `Update`: Update balance flags
- `DeleteBalance`: Soft-delete empty balances

**Operation Operations:**

- `CreateOperation`: Create operations from DSL specifications
- `UpdateOperation`: Update description and metadata

**Asset Rate Operations:**

- `CreateOrUpdateAssetRate`: Upsert exchange rates

**Routing Operations:**

- `CreateTransactionRoute`: Create transaction routing rules
- `UpdateTransactionRoute`: Update routing rules
- `DeleteTransactionRouteByID`: Delete routing rules
- `CreateOperationRoute`: Create account selection rules
- `UpdateOperationRoute`: Update account selection rules
- `DeleteOperationRouteByID`: Delete account selection rules

**Cache Operations:**

- `CreateAccountingRouteCache`: Cache transaction routes
- `DeleteTransactionRouteCache`: Invalidate route cache
- `ReloadOperationRouteCache`: Refresh caches after route updates

**Async Processing:**

- `TransactionExecute`: Route to sync/async processing
- `SendBTOExecuteAsync`: Publish to queue for async processing
- `CreateBTOExecuteSync`: Process synchronously
- `CreateBalanceTransactionOperationsAsync`: Worker function
- `SendTransactionEvents`: Publish transaction events
- `SendLogTransactionAuditQueue`: Publish audit logs

**Idempotency:**

- `CreateOrCheckIdempotencyKey`: Prevent duplicate transactions
- `SetValueOnExistingIdempotencyKey`: Store transaction result

### Query Side (Read Operations)

The query side handles all read operations with metadata enrichment:

**Transaction Queries:**

- Get transaction by ID
- List transactions with pagination
- Get transaction by DSL

**Balance Queries:**

- Get balance by ID
- List balances by account
- List all balances

**Operation Queries:**

- Get operation by ID
- List operations by transaction
- List all operations

**Asset Rate Queries:**

- Get asset rate by ID
- Get asset rate by currency pair
- List asset rates

**Routing Queries:**

- Get transaction route by ID
- List transaction routes
- Get operation route by ID
- List operation routes

## Key Concepts

### Double-Entry Accounting

The transaction service enforces double-entry accounting principles:

- Every transaction has equal debits and credits
- Debits decrease liability/equity/revenue or increase asset/expense
- Credits increase liability/equity/revenue or decrease asset/expense
- Total debits must equal total credits

### Transaction Processing Flow

**Synchronous Mode:**

```
1. Receive transaction request
2. Parse DSL or validate JSON
3. Fetch account balances (with SELECT FOR UPDATE)
4. Validate transaction (lib-commons)
5. Create transaction record (PENDING)
6. Update balances
7. Create operations
8. Update transaction status (APPROVED)
9. Publish events (async)
10. Return transaction to caller
```

**Asynchronous Mode:**

```
1. Receive transaction request
2. Parse DSL or validate JSON
3. Fetch account balances (with SELECT FOR UPDATE)
4. Validate transaction (lib-commons)
5. Create transaction record (PENDING)
6. Publish BTO data to queue
7. Return transaction to caller (PENDING status)

Worker Process:
8. Consume BTO message from queue
9. Update balances
10. Create operations
11. Update transaction status (APPROVED)
12. Publish events
13. Remove from Redis queue
```

### Balance Management

**Balance Structure:**

- Available: Funds available for transactions
- On-Hold: Funds temporarily held (pending transactions)
- Version: Optimistic locking counter

**Balance Operations:**

- DEBIT: Decreases available (money leaving)
- CREDIT: Increases available (money entering)
- ON_HOLD: Moves from available to on-hold
- RELEASE: Moves from on-hold to available

### Transaction Routing

**Transaction Routes:**

- Define how transactions flow through the system
- Specify source and destination account selection rules
- Enable automated transaction processing
- Cached in Redis for performance

**Operation Routes:**

- Define account selection criteria
- Support matching by alias or account_type
- Specify operation type (source or destination)
- Reusable across multiple transaction routes

### Idempotency

The service implements idempotency to prevent duplicate transactions:

**Flow:**

1. Client sends request with Idempotency-Key header
2. Service checks Redis for existing key
3. If key exists: Return cached transaction (duplicate request)
4. If key doesn't exist: Create key, process transaction
5. After processing: Store transaction in idempotency key
6. Subsequent requests return stored transaction

**Key Format:**

- `idempotency:{org_id}:{ledger_id}:{key}`
- TTL: Configurable (typically 24 hours)

### Event Publishing

**Transaction Events:**

- Published when transaction status changes
- Routing key: `midaz.transaction.{STATUS}`
- Consumers: Webhooks, analytics, notifications
- Fire-and-forget (errors logged but not returned)

**Audit Logs:**

- Published for all operations
- Contains full operation details
- Used for compliance and forensics
- **CRITICAL BUG:** Uses logger.Fatalf (crashes server)

## Business Rules

### Transaction Validation

- All accounts must exist
- All accounts must have balances
- Total debits must equal total credits
- Source accounts must have sufficient funds
- Accounts must allow sending/receiving
- Asset codes must match

### Balance Constraints

- Available amount cannot go negative
- On-hold amount cannot go negative
- Version number prevents concurrent modifications
- Balances can only be deleted if zero

### Routing Rules

- Transaction routes must have source and destination
- Operation routes must specify account matching rules
- Operation routes cannot be deleted if referenced
- Cache must be invalidated on route changes

## Error Handling

The service uses structured error handling:

- Database errors → Business errors (via ValidateBusinessError)
- PostgreSQL errors → Specific error codes
- Validation errors → User-friendly messages
- Not found errors → EntityNotFoundError

## Performance Optimizations

### Caching

**Transaction Route Cache:**

- Stored in Redis with msgpack serialization
- No TTL (persists until invalidated)
- Reduces database queries during transaction processing

**Idempotency Cache:**

- Stored in Redis with configurable TTL
- Prevents duplicate transaction processing
- Stores full transaction result

### Batch Operations

**Balance Updates:**

- Batch update multiple balances in single query
- Reduces database round trips
- Uses optimistic locking (version numbers)

**Operation Creation:**

- Creates multiple operations efficiently
- Uses channels for async communication

### Async Processing

**Queue-Based Processing:**

- Offloads balance/operation creation to workers
- Improves API response time
- Enables horizontal scaling
- Fallback to sync if queue unavailable

## Configuration

### Environment Variables

**Processing Mode:**

- `RABBITMQ_TRANSACTION_ASYNC`: Enable async processing ("true"/"false")

**Event Publishing:**

- `RABBITMQ_TRANSACTION_EVENTS_ENABLED`: Enable transaction events ("true"/"false")
- `RABBITMQ_TRANSACTION_EVENTS_EXCHANGE`: Exchange for transaction events
- `AUDIT_LOG_ENABLED`: Enable audit logging ("true"/"false")
- `RABBITMQ_AUDIT_EXCHANGE`: Exchange for audit logs
- `RABBITMQ_AUDIT_KEY`: Routing key for audit logs

**Queue Configuration:**

- `RABBITMQ_TRANSACTION_BALANCE_OPERATION_EXCHANGE`: BTO exchange
- `RABBITMQ_TRANSACTION_BALANCE_OPERATION_KEY`: BTO routing key

## Dependencies

### Repositories

**PostgreSQL:**

- TransactionRepo: Transaction persistence
- OperationRepo: Operation persistence
- BalanceRepo: Balance persistence
- AssetRateRepo: Exchange rate persistence
- OperationRouteRepo: Operation route persistence
- TransactionRouteRepo: Transaction route persistence

**MongoDB:**

- MetadataRepo: Flexible metadata storage

**RabbitMQ:**

- RabbitMQRepo: Message publishing

**Redis:**

- RedisRepo: Caching and idempotency

### External Libraries

- **lib-commons/transaction**: Transaction validation and processing
- **msgpack**: Efficient binary serialization
- **OpenTelemetry**: Distributed tracing

## Known Issues

### Critical Bugs

1. **logger.Fatalf() in send-log-transaction-audit-queue.go**
   - Lines 40, 69
   - Crashes entire application on audit log failures
   - Should return errors instead
   - See BUGS.md for details

### Recommendations

- Replace logger.Fatalf with error returns
- Add circuit breaker for RabbitMQ operations
- Implement retry logic for metadata operations
- Add monitoring for queue depth
- Consider dead letter queue for failed messages

## Testing

The service includes comprehensive tests:

- Unit tests for each use case
- Integration tests for database operations
- Mock repositories for isolated testing
- Test fixtures for common scenarios

## Related Packages

- `internal/adapters`: Infrastructure implementations
- `internal/bootstrap`: Application initialization
- `pkg/mmodel`: Domain models
- `pkg/gold`: DSL parser

## Monitoring

### OpenTelemetry Spans

All operations create spans for tracing:

- `command.create_transaction`
- `command.update_balances_new`
- `command.create_operation`
- `command.send_bto_execute_async`
- And many more...

### Logging

Structured logging with levels:

- Info: Normal operations
- Warn: Recoverable issues
- Error: Operation failures
- Fatal: Critical failures (BUG - should not be used)

## Architecture

The service follows hexagonal architecture:

```
HTTP Handler → Command Use Case → Repository → Database
                    ↓
              Event Publisher → RabbitMQ
                    ↓
              Cache Manager → Redis
```

## Notes

- The service name "CreateBalanceTransactionOperationsAsync" is misleading - it runs sync when called directly
- Async/sync mode is determined by environment variable
- All soft deletes preserve data for audit purposes
- Metadata is stored separately in MongoDB for flexibility
