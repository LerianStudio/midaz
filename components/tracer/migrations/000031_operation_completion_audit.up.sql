-- Commit enum additions before a transaction records completion. The events
-- describe known producer outcomes, never a fabricated validation decision.
ALTER TYPE audit_event_type_enum ADD VALUE IF NOT EXISTS 'RESERVE_OPERATION_CONFIRMED';
ALTER TYPE audit_event_type_enum ADD VALUE IF NOT EXISTS 'RESERVE_OPERATION_RELEASED';
ALTER TYPE resource_type_enum ADD VALUE IF NOT EXISTS 'reserve_operation';
