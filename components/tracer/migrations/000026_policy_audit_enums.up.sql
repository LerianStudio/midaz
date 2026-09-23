-- Enum additions must commit before a subsequent transaction uses the values.
ALTER TYPE audit_event_type_enum ADD VALUE IF NOT EXISTS 'POLICY_PUBLISHED';
ALTER TYPE resource_type_enum ADD VALUE IF NOT EXISTS 'policy';
