# PostgreSQL Adapters - Transaction

This directory contains PostgreSQL repository implementations for the transaction service's domain entities.

## Structure

Each subdirectory contains a repository implementation for a specific entity:

- `assetrate/` - Asset exchange rate management
- `balance/` - Account balance tracking with versioning
- `operation/` - Individual ledger operations within transactions
- `operationroute/` - Routing rules for operations (debit/credit patterns)
- `transaction/` - Transaction lifecycle and state management
- `transactionroute/` - Transaction routing configuration linking operation routes

## Implementation Pattern

Each repository follows a consistent pattern:

1. **Interface Definition** (`*.go`) - Repository contract defining available operations
2. **PostgreSQL Model** (`*.go`) - SQL-specific struct with conversion methods
3. **Implementation** (`*.postgresql.go`) - Concrete PostgreSQL repository
4. **Mock** (`*.postgresql_mock.go`) - Generated mock for testing

## Special Features

### Balance Repository

- Optimistic locking via version field
- Atomic balance updates coordinated with Redis
- Support for available/on-hold balance states

### Transaction Repository

- Complex filtering (date range, status, metadata)
- Parent-child transaction relationships
- Idempotency key support

### Operation Route Repository

- Cache-aware updates triggering Redis invalidation
- Account validation rules (alias or account type matching)

## Database Operations

All repositories provide:

- CRUD operations with UUID identifiers
- Soft delete support
- Pagination with cursor support
- Metadata filtering capabilities
