# PostgreSQL Configuration

**Navigation:** [Home](../../) > [Components](../) > [Infrastructure](./) > PostgreSQL

This document describes the PostgreSQL configuration and usage in the Midaz system.

## Overview

Midaz uses PostgreSQL as its primary relational database for storing structured data across multiple services. Each microservice has its own dedicated database within the PostgreSQL instance. The system follows best practices for connection management, schema design, and database operations.

## Database Structure

The system uses two primary databases:

- **onboarding**: Stores organization, entity, and metadata information
- **transaction**: Stores financial transactions, operations, and balances

Each database follows a consistent pattern with:
- UUID primary keys
- Soft deletion (using `deleted_at` timestamp)
- Timestamps for record lifecycle (`created_at`, `updated_at`)
- Foreign key constraints for referential integrity
- JSON/JSONB columns for flexible data

## Database Configuration

### Initial Setup

PostgreSQL is initialized with a setup script (`init.sql`) that:

1. Creates a replication user for database replication
2. Sets up physical and logical replication slots
3. Creates the required databases

```sql
CREATE USER replicator WITH REPLICATION LOGIN ENCRYPTED PASSWORD 'replicator_password';

SELECT pg_create_physical_replication_slot('replication_slot');
SELECT * FROM pg_create_logical_replication_slot('logical_slot', 'pgoutput');

CREATE DATABASE onboarding;
CREATE DATABASE transaction;
```

### Connection Configuration

Connection strings are built dynamically using environment variables:

```
postgresql://${DB_USER}:${DB_PASSWORD}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=disable
```

Key environment variables:

| Variable | Description | Default |
|----------|-------------|---------|
| DB_HOST | Primary database host | localhost |
| DB_PORT | Database port | 5432 |
| DB_USER | Database username | postgres |
| DB_PASSWORD | Database password | postgres |
| DB_NAME | Database name | varies by service |
| DB_MAX_OPEN_CONNS | Maximum open connections | 100 |
| DB_MAX_IDLE_CONNS | Maximum idle connections | 10 |

For read replicas, similar variables with the `DB_REPLICA_` prefix are used.

## Connection Management

### Connection Pooling

The system implements connection pooling with configurable limits:

- `MaxOpenConnections`: Controls maximum concurrent connections (default: 100)
- `MaxIdleConnections`: Controls connections kept in pool when idle (default: 10)

### Read/Write Split

The infrastructure supports primary/replica configuration:
- Write operations go to the primary database
- Read operations can be directed to replicas
- Configured through separate connection parameters for primary and replica databases

## Schema Management

### Migrations

Database schema is managed through migration files in each service:

- Migration files follow a naming convention: `{sequence}_{description}.{up|down}.sql`
- Migrations are versioned and run in sequence
- Both up (apply) and down (rollback) migrations are provided

Example migration to create a table:

```sql
CREATE TABLE IF NOT EXISTS organization
(
    id                                   UUID PRIMARY KEY NOT NULL,
    parent_organization_id               UUID,
    legal_name                           TEXT NOT NULL,
    doing_business_as                    TEXT,
    legal_document                       TEXT NOT NULL,
    address                              JSONB NOT NULL,
    status                               TEXT NOT NULL,
    status_description                   TEXT,
    created_at                           TIMESTAMP WITH TIME ZONE NOT NULL,
    updated_at                           TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    deleted_at                           TIMESTAMP WITH TIME ZONE,
    FOREIGN KEY (parent_organization_id) REFERENCES organization (id)
);
```

### Common Schema Patterns

All tables follow these patterns:

1. **UUID Primary Keys**: Instead of auto-incrementing integers
2. **Timestamps**:
   - `created_at`: When the record was created
   - `updated_at`: When the record was last updated
   - `deleted_at`: When the record was marked as deleted (null if active)
3. **JSONB for Complex Data**: Used for fields like addresses or other complex structures
4. **Status Fields**: Text fields with descriptive status values
5. **Indexes**: Created for frequently queried fields

## Database Access Patterns

### Repository Pattern

All database access follows the repository pattern:
- Interface defines the repository contract
- PostgreSQL-specific implementation provides database access
- Clear separation between domain models and database operations

### Soft Deletion

Records are never physically deleted, only marked as deleted:
- `deleted_at` column is set with the current timestamp when deleted
- Queries include `WHERE deleted_at IS NULL` to filter active records
- Simplifies auditing and potential recovery

### Error Handling

Database operations follow consistent error handling:
- SQL errors are mapped to domain-specific errors
- Connection errors are properly handled and reported
- Retries are implemented for transient failures

### Monitoring & Telemetry

Database operations are monitored through:
- OpenTelemetry instrumentation with spans for queries
- Performance metrics for query execution time
- Structured logging for database operations

## Running Locally

To run PostgreSQL locally using Docker:

```bash
cd components/infra
docker-compose up -d postgres
```

## Best Practices

1. **Always use parameterized queries** to prevent SQL injection
2. **Include indexes** for frequently queried columns
3. **Use transactions** when multiple related operations must succeed or fail together
4. **Implement connection pooling** to efficiently manage database connections
5. **Set up monitoring** to track query performance
6. **Follow the soft deletion pattern** for audit trails
7. **Use UUIDs** for primary keys to avoid sequential ID issues

## Common Issues and Troubleshooting

1. **Connection Failures**:
   - Check network connectivity to the database
   - Verify username/password are correct
   - Ensure database name exists and is spelled correctly

2. **Performance Issues**:
   - Check for missing indexes on frequently queried columns
   - Review query execution plans with EXPLAIN
   - Monitor connection pool utilization

3. **Schema Migration Errors**:
   - Check that migration files are properly sequenced
   - Ensure all dependencies exist before creating foreign keys
   - Use transactions for complex migrations

## Related Documentation

- [Entity Lifecycle](../../architecture/data-flow/entity-lifecycle.md)
- [Transaction Lifecycle](../../architecture/data-flow/transaction-lifecycle.md)
- [Financial Model](../../domain-models/financial-model.md)
