-- Migration: Add RULE_DRAFTED and DRAFT to audit event enums
--
-- Supports the new POST /v1/rules/{id}/draft endpoint (INACTIVE → DRAFT transition).
-- Adds:
--   - 'RULE_DRAFTED' to audit_event_type_enum (after RULE_DEACTIVATED)
--   - 'DRAFT' to audit_action_enum

-- Add RULE_DRAFTED to audit event type enum
ALTER TYPE audit_event_type_enum ADD VALUE IF NOT EXISTS 'RULE_DRAFTED' AFTER 'RULE_DEACTIVATED';

-- Add DRAFT to audit action enum
ALTER TYPE audit_action_enum ADD VALUE IF NOT EXISTS 'DRAFT' AFTER 'DEACTIVATE';

