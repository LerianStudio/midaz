# Migration Template

Use this template when creating new migrations. Copy and fill out for each significant migration.

---

## Migration XXXXXX: [Brief Description]

### Overview

[Describe in 1-2 sentences what this migration does]

### Migration Type

- [ ] **Additive** - Only adds new objects (tables, columns, indexes)
- [ ] **Expand** - First phase of expand-and-contract (adds without removing)
- [ ] **Contract** - Second phase of expand-and-contract (removes deprecated structures)

### Backward Compatibility Checklist

- [ ] This migration is backward compatible with the N-1 application version
- [ ] Old versions can continue working during rolling update
- [ ] The `.down.sql` file completely reverts the changes
- [ ] Rollback does not cause data loss

### Changes

#### Added
- [List columns/tables/indexes added]

#### Modified
- [List changes to existing structures]

#### Removed
- [List removals - should be empty for safe migrations]

### Dependencies

- [ ] No specific application version dependency
- [ ] Requires application version >= X.Y.Z (if applicable)

### UP File

```sql
-- 000XXX_description.up.sql

[Paste the .up.sql file content here]
```

### DOWN File

```sql
-- 000XXX_description.down.sql

[Paste the .down.sql file content here]
```

### Deployment Sequence

1. [ ] Apply migration via `make up` or `make up-backend`
2. [ ] Deploy application
3. [ ] Monitor errors for 24h
4. [ ] (If expand) Schedule CONTRACT migration for [date]

### Rollback Plan

In case of problems:

1. Revert application deploy (if already started)
2. Roll back the migration manually or via infrastructure
3. Verify previous version functionality
4. Document incident

### Linter Validation

Run the migration linter before submitting:

```bash
migration-lint ./components/<component>/migrations
```

- [ ] Linter passed with no errors
- [ ] Linter passed with no warnings (or warnings are justified)

### Tests Performed

- [ ] Tested in local environment
- [ ] Tested in staging environment
- [ ] Tested rollback scenario
- [ ] Tested with both application versions simultaneously

### Notes

[Additional notes, known risks, or special instructions]

---

## Filled Example

### Migration 000017: Add user preferences column

### Overview

Adds a JSONB column to store custom user preferences.

### Migration Type

- [x] **Additive** - Only adds new objects
- [ ] **Expand** - First phase of expand-and-contract
- [ ] **Contract** - Second phase of expand-and-contract

### Backward Compatibility Checklist

- [x] This migration is backward compatible with the N-1 application version
- [x] Old versions can continue working during rolling update
- [x] The `.down.sql` file completely reverts the changes
- [x] Rollback does not cause data loss

### Changes

#### Added
- Column `preferences` (JSONB, nullable) on `account` table
- GIN index on `preferences` for JSONB queries

#### Modified
- None

#### Removed
- None

### UP File

```sql
-- 000017_add_account_preferences.up.sql

ALTER TABLE account ADD COLUMN IF NOT EXISTS preferences JSONB;

CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_account_preferences
ON account USING GIN (preferences);
```

### DOWN File

```sql
-- 000017_add_account_preferences.down.sql

DROP INDEX CONCURRENTLY IF EXISTS idx_account_preferences;

ALTER TABLE account DROP COLUMN IF EXISTS preferences;
```

### Deployment Sequence

1. [x] Apply migration via `make up` or `make up-backend`
2. [ ] Deploy application v2.5.0
3. [ ] Monitor errors for 24h

### Linter Validation

```bash
migration-lint ./components/onboarding/migrations
```

- [x] Linter passed with no errors
- [x] Linter passed with no warnings

### Tests Performed

- [x] Tested in local environment
- [x] Tested in staging environment
- [x] Tested rollback scenario
- [x] Tested with both application versions simultaneously
