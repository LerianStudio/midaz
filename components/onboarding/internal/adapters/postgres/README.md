# PostgreSQL Adapters - Onboarding

This directory contains PostgreSQL repository implementations for the onboarding service's domain entities.

## Structure

Each subdirectory contains a repository implementation for a specific entity:

- `account/` - Account persistence with hierarchical support
- `accounttype/` - Account type configuration management
- `asset/` - Asset and currency management
- `ledger/` - Ledger lifecycle and configuration
- `organization/` - Organization master data
- `portfolio/` - Portfolio grouping of accounts
- `segment/` - Logical segmentation within ledgers

## Implementation Pattern

Each repository follows a consistent pattern:

1. **Interface Definition** (`*.go`) - Repository contract defining available operations
2. **PostgreSQL Model** (`*.go`) - SQL-specific struct with conversion methods
3. **Implementation** (`*.postgresql.go`) - Concrete PostgreSQL repository
4. **Mock** (`*.postgresql_mock.go`) - Generated mock for testing

## Database Operations

All repositories provide standard CRUD operations:

- Create with UUID generation
- FindByID with soft-delete awareness
- Update with optimistic locking
- Delete (soft delete with timestamp)
- FindAll with pagination support

## Transaction Support

Repositories accept `*sql.Tx` for transaction participation, enabling atomic multi-entity operations coordinated by the service layer.
