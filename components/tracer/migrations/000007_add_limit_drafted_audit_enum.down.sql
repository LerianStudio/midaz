-- Rollback: Remove LIMIT_DRAFTED from audit event type enum
--
-- PostgreSQL does not support ALTER TYPE ... REMOVE VALUE directly.
-- The rollback recreates the audit_event_type_enum without LIMIT_DRAFTED.
--
-- WARNING: Step 3 (ALTER COLUMN ... USING cast) will fail if any rows contain
-- 'LIMIT_DRAFTED' values. Delete or update those rows before running this rollback.
--
-- Note: This rollback preserves RULE_DRAFTED and DRAFT (added in migration 000003).

-- Step 1: Rename current enum type
ALTER TYPE audit_event_type_enum RENAME TO audit_event_type_enum_old;

-- Step 2: Create new enum type without LIMIT_DRAFTED
-- Includes all values from migrations 000001 + 000003, minus LIMIT_DRAFTED
CREATE TYPE audit_event_type_enum AS ENUM (
    'TRANSACTION_VALIDATED',
    'RULE_CREATED', 'RULE_UPDATED', 'RULE_ACTIVATED', 'RULE_DEACTIVATED', 'RULE_DRAFTED', 'RULE_DELETED',
    'LIMIT_CREATED', 'LIMIT_UPDATED', 'LIMIT_ACTIVATED', 'LIMIT_DEACTIVATED', 'LIMIT_DELETED'
);

-- Step 3: Alter column type on audit_events table
ALTER TABLE audit_events
    ALTER COLUMN event_type TYPE audit_event_type_enum USING event_type::text::audit_event_type_enum;

-- Step 4: Drop old enum type
DROP TYPE audit_event_type_enum_old;

