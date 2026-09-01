-- ============================================
-- Migration Rollback: 000004_initial_schema
-- Description: Drop all MVP tables
-- ============================================

-- Drop audit events
DROP TRIGGER IF EXISTS audit_events_hash_chain ON audit_events;
DROP TRIGGER IF EXISTS prevent_audit_event_truncate_trigger ON audit_events;
DROP RULE IF EXISTS prevent_audit_event_delete ON audit_events;
DROP RULE IF EXISTS prevent_audit_event_update ON audit_events;
DROP TABLE IF EXISTS audit_events;
-- Note: Functions and Event Triggers are dropped by the down migrations of 000001-000003, which own them.
-- Note: actor_type uses ENUM actor_type_enum; DROP TYPE statement is below (see line 29).

-- Drop immutability rules and triggers (replaced triggers due to golang-migrate bug #590)
DROP TRIGGER IF EXISTS prevent_transaction_validation_truncate_trigger ON transaction_validations;
DROP RULE IF EXISTS prevent_transaction_validation_delete ON transaction_validations;
DROP RULE IF EXISTS prevent_transaction_validation_update ON transaction_validations;

-- Drop tables in reverse order (respecting FK constraints)
DROP TABLE IF EXISTS transaction_validations CASCADE;
DROP TABLE IF EXISTS usage_counters CASCADE;
DROP TABLE IF EXISTS limits CASCADE;
DROP TABLE IF EXISTS rules CASCADE;

-- Drop enums
DROP TYPE IF EXISTS transaction_type_enum;
DROP TYPE IF EXISTS decision_enum;
DROP TYPE IF EXISTS actor_type_enum;

-- Drop new enum types (rule, limit, audit)
DROP TYPE IF EXISTS rule_status_enum;
DROP TYPE IF EXISTS limit_type_enum;
DROP TYPE IF EXISTS limit_status_enum;
DROP TYPE IF EXISTS audit_event_type_enum;
DROP TYPE IF EXISTS audit_action_enum;
DROP TYPE IF EXISTS audit_result_enum;
DROP TYPE IF EXISTS resource_type_enum;

