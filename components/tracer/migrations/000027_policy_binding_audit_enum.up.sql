-- Commit the enum addition before any activation records its audit event.
ALTER TYPE audit_event_type_enum ADD VALUE IF NOT EXISTS 'POLICY_BOUND';
