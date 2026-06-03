-- Rollback: Remove RULE_DRAFTED and DRAFT from audit event enums
--
-- PostgreSQL does not support ALTER TYPE ... REMOVE VALUE directly.
-- The rollback recreates the enum types without the new values.
-- This requires temporarily dropping and recreating constraints that use these types.
--
-- WARNING: Step 3 (ALTER COLUMN ... USING cast) will fail if any rows contain
-- 'RULE_DRAFTED' or 'DRAFT' values. Delete or update those rows before running
-- this rollback.

-- Step 1: Rename current enum types
ALTER TYPE audit_event_type_enum RENAME TO audit_event_type_enum_old;
ALTER TYPE audit_action_enum RENAME TO audit_action_enum_old;

-- Step 2: Create new enum types without the draft values
CREATE TYPE audit_event_type_enum AS ENUM (
    'TRANSACTION_VALIDATED',
    'RULE_CREATED', 'RULE_UPDATED', 'RULE_ACTIVATED', 'RULE_DEACTIVATED', 'RULE_DELETED',
    'LIMIT_CREATED', 'LIMIT_UPDATED', 'LIMIT_ACTIVATED', 'LIMIT_DEACTIVATED', 'LIMIT_DELETED'
);
CREATE TYPE audit_action_enum AS ENUM ('VALIDATE', 'CREATE', 'UPDATE', 'DELETE', 'ACTIVATE', 'DEACTIVATE');

-- Step 3: Alter column types on audit_events table
ALTER TABLE audit_events
    ALTER COLUMN event_type TYPE audit_event_type_enum USING event_type::text::audit_event_type_enum;
ALTER TABLE audit_events
    ALTER COLUMN action TYPE audit_action_enum USING action::text::audit_action_enum;

-- Step 4: Drop old enum types
DROP TYPE audit_event_type_enum_old;
DROP TYPE audit_action_enum_old;

