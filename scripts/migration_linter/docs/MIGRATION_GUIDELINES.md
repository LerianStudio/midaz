# Database Migration Guidelines

This document establishes standards for writing SQL migrations that ensure:
- Zero-downtime during deployments
- Backward compatibility with previous application versions
- Safe rollbacks without data loss

---

## Fundamental Principles

### 1. Backward Compatibility

Every migration MUST be compatible with the N-1 version of the application. This means:
- The previous application version must continue working after the migration
- During rolling updates, both versions must function simultaneously

### 2. Reversibility

Every migration MUST have a corresponding `.down.sql` file that:
- Completely reverts the changes
- Does not cause data loss (when possible)
- Allows rollback to the previous application version

### 3. Atomicity

Each migration should make **a single logical change**. Avoid migrations that:
- Alter multiple unrelated tables
- Combine schema changes with data changes

---

## Expand-and-Contract Pattern

For changes that are not purely additive, use the **expand-and-contract** pattern:

### Phase 1: EXPAND (Migration N)

Adds new structures **without removing** the old ones.

```sql
-- 000017_expand_add_amount_decimal.up.sql

-- Add new column alongside the old one
ALTER TABLE transaction
ADD COLUMN IF NOT EXISTS amount_decimal DECIMAL;

-- Populate new column with converted data
UPDATE transaction
SET amount_decimal = (amount / POWER(10, COALESCE(amount_scale, 0)))::DECIMAL
WHERE amount_decimal IS NULL;

-- Create trigger to sync during transition
CREATE OR REPLACE FUNCTION sync_transaction_amount()
RETURNS TRIGGER AS $$
BEGIN
    IF NEW.amount_decimal IS NULL AND NEW.amount IS NOT NULL THEN
        NEW.amount_decimal := (NEW.amount / POWER(10, COALESCE(NEW.amount_scale, 0)))::DECIMAL;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER transaction_amount_sync
BEFORE INSERT OR UPDATE ON transaction
FOR EACH ROW EXECUTE FUNCTION sync_transaction_amount();
```

### Phase 2: MIGRATE (Application Release)

Deploy the new version that:
- Writes to BOTH columns (old and new)
- Reads from the NEW column
- Allows coexistence with version N-1

### Phase 3: CONTRACT (Migration N+M)

**Only after** all instances are on the new version:

```sql
-- 000020_contract_remove_old_amount.up.sql
-- WARNING: Execute only after deprecation period

DROP TRIGGER IF EXISTS transaction_amount_sync ON transaction;
DROP FUNCTION IF EXISTS sync_transaction_amount();

ALTER TABLE transaction DROP COLUMN IF EXISTS amount;
ALTER TABLE transaction DROP COLUMN IF EXISTS amount_scale;
```

---

## Safe Operations

### Add Nullable Column

```sql
ALTER TABLE users ADD COLUMN IF NOT EXISTS preferences JSONB;
```

### Add NOT NULL Column with Default

```sql
ALTER TABLE balance ADD COLUMN IF NOT EXISTS key TEXT NOT NULL DEFAULT 'default';
```

### Create Index Concurrently

```sql
-- IMPORTANT: Cannot be inside a transaction
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_account_alias
ON account (organization_id, ledger_id, alias);
```

### Create New Table

```sql
CREATE TABLE IF NOT EXISTS audit_log (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    entity_type TEXT NOT NULL,
    entity_id UUID NOT NULL,
    action TEXT NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);
```

### Add Constraint with NOT VALID

```sql
-- Add constraint without validating existing data (non-blocking)
ALTER TABLE orders
ADD CONSTRAINT fk_customer
FOREIGN KEY (customer_id) REFERENCES customers(id)
NOT VALID;

-- Validate in separate migration (can be done in background)
ALTER TABLE orders VALIDATE CONSTRAINT fk_customer;
```

---

## Prohibited Operations

### Direct DROP COLUMN

```sql
-- NEVER do this
ALTER TABLE transaction DROP COLUMN template;

-- Use expand-and-contract instead (see section above)
```

### Direct ALTER COLUMN TYPE

```sql
-- NEVER do this
ALTER TABLE transaction ALTER COLUMN amount TYPE DECIMAL;

-- Add new column, migrate data, remove old one
ALTER TABLE transaction ADD COLUMN amount_decimal DECIMAL;
UPDATE transaction SET amount_decimal = amount::DECIMAL;
-- (in later release) ALTER TABLE transaction DROP COLUMN amount;
```

### RENAME COLUMN

```sql
-- NEVER do this
ALTER TABLE operation RENAME COLUMN version_balance TO balance_version_before;

-- Use expand-and-contract
ALTER TABLE operation ADD COLUMN balance_version_before BIGINT;
UPDATE operation SET balance_version_before = version_balance;
-- (in later release) ALTER TABLE operation DROP COLUMN version_balance;
```

### VACUUM FULL

```sql
-- NEVER do this (requires exclusive lock)
VACUUM FULL transaction;

-- Use regular VACUUM or VACUUM ANALYZE
VACUUM ANALYZE transaction;
```

### TRUNCATE

```sql
-- NEVER do this
TRUNCATE TABLE audit_log;

-- Use soft delete or archiving
UPDATE audit_log SET deleted_at = NOW() WHERE created_at < '2024-01-01';
```

### CREATE INDEX without CONCURRENTLY

```sql
-- Blocks writes on the table
CREATE INDEX idx_name ON table (column);

-- Non-blocking
CREATE INDEX CONCURRENTLY idx_name ON table (column);
```

---

## Migration Linter

The migration linter is a tool that automatically detects dangerous patterns in SQL migration files. It helps enforce the guidelines described in this document.

### Usage

```bash
# Basic usage
migration-lint <path-to-migrations>

# With strict mode (treats warnings as errors)
migration-lint <path-to-migrations> --strict
```

### Detected Patterns

| Pattern | Severity | Description |
|---------|----------|-------------|
| `DROP COLUMN` | ERROR | Requires expand-and-contract pattern |
| `ALTER COLUMN TYPE` | ERROR | Requires new column + data migration |
| `VACUUM FULL` | ERROR | Requires exclusive lock, use `VACUUM ANALYZE` |
| `TRUNCATE` | ERROR | Destroys data, use soft delete or archiving |
| `RENAME COLUMN` | ERROR | Breaks existing queries |
| `RENAME TABLE` | ERROR | Breaks existing queries |
| `ADD COLUMN NOT NULL` (without DEFAULT) | ERROR | Requires table rewrite |
| `DROP TABLE` (without IF EXISTS) | WARNING | May fail if table doesn't exist |
| `REINDEX` (without CONCURRENTLY) | WARNING | Blocks table during reindex |
| `CREATE INDEX` (without CONCURRENTLY) | WARNING | Blocks writes during creation |
| `DROP INDEX` (without CONCURRENTLY) | WARNING | Blocks table during drop |
| `SET NOT NULL` | WARNING | May fail if NULLs exist, blocks for validation |

### Output Format

The linter outputs issues grouped by file with numbered issues and grouped suggestions:

```
Analyzing 3 migrations in ./components/onboarding/migrations

---> 000006_create_account_type_table.up.sql
  1. [WARNING] Line 19: CREATE INDEX without CONCURRENTLY blocks table. Use CREATE INDEX CONCURRENTLY.
  2. [WARNING] Line 25: CREATE INDEX without CONCURRENTLY blocks table. Use CREATE INDEX CONCURRENTLY.

  Suggestions:
    [#1, #2]
      Use CONCURRENTLY option:
          CREATE INDEX CONCURRENTLY idx_name ON table (column);
          CREATE UNIQUE INDEX CONCURRENTLY idx_name ON table (column);
          Note: Cannot be used inside a transaction block

---> 000010_add_user_column.up.sql
  1. [ERROR] Line 5: Adding NOT NULL column without DEFAULT requires table rewrite. Add DEFAULT value.

  Suggestions:
    [#1]
      Add DEFAULT value to avoid table rewrite:
          ALTER TABLE table_name ADD COLUMN column_name TYPE NOT NULL DEFAULT value;

          Or use multi-step approach:
          1. ADD COLUMN column_name TYPE DEFAULT value;
          2. UPDATE table SET column_name = value WHERE column_name IS NULL;
          3. ALTER COLUMN column_name SET NOT NULL;

Summary: 1 error(s), 2 warning(s)

Validation failed!
```

### Exit Codes

| Code | Description |
|------|-------------|
| 0 | All migrations passed or only warnings (non-strict mode) |
| 1 | Errors found, or warnings found in strict mode |

### CI Integration

The linter is integrated into CI via the Makefile:

```bash
# Lint all migration directories
make migrate-lint

# Lint specific component
make migrate-lint-onboarding
make migrate-lint-transaction
```

---

## Code Review Checklist

### Before Approving a Migration

- [ ] Does the migration have a corresponding `.down.sql` file?
- [ ] Does the `.down.sql` completely revert the changes?
- [ ] Is the migration backward compatible with the N-1 application version?
- [ ] Are there no `DROP COLUMN` or `ALTER COLUMN TYPE` statements?
- [ ] Do indexes use `CONCURRENTLY`?
- [ ] Do `NOT NULL` columns have `DEFAULT`?
- [ ] Was the migration tested in a staging environment?
- [ ] Did the linter pass (`make migrate-lint`)?

---

## Deployment Sequence

```
┌─────────────────────────────────────────────────────────────┐
│              DEPLOY WITH SCHEMA CHANGE                       │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  T+0: PRE-DEPLOY                                            │
│       ├── Code review of migration                          │
│       ├── make migrate-lint (CI)                            │
│       └── Merge PR                                          │
│                                                              │
│  T+1: MIGRATION                                              │
│       ├── make migrate-up COMPONENT=xxx                     │
│       ├── Verify migration applied                          │
│       └── DB has BOTH structures (old + new)                │
│                                                              │
│  T+2: APPLICATION DEPLOY                                     │
│       ├── Rolling update starts                             │
│       ├── Pods v1 (old) coexist with pods v2 (new)         │
│       └── Both work with current schema                     │
│                                                              │
│  T+3: VERIFICATION                                           │
│       ├── Monitor errors                                    │
│       ├── Check metrics                                     │
│       └── Minimum 24h observation                           │
│                                                              │
│  T+7 to T+30: CONTRACT (if necessary)                       │
│       ├── All instances on new version                      │
│       ├── Deprecation period completed                      │
│       └── Remove old structures                             │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

---

## References

- [PostgreSQL ALTER TABLE](https://www.postgresql.org/docs/current/sql-altertable.html)
- [PostgreSQL CREATE INDEX](https://www.postgresql.org/docs/current/sql-createindex.html)
- [Expand and Contract Pattern](https://www.prisma.io/dataguide/types/relational/expand-and-contract-pattern)
- [Zero-Downtime Migrations](https://blog.pragmaticengineer.com/zero-downtime-database-migrations/)
