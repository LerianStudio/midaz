-- ============================================
-- Function: calculate_audit_event_hash
-- Trigger function to calculate hash chain for audit events
-- Each record's hash includes the previous record's hash for tamper detection
--
-- Hash includes (in canonical order with pipe delimiter):
--   1. previous_hash (or "GENESIS")
--   2. event_id
--   3. event_type (includes action, e.g., RULE_CREATED)
--   4. created_at
--   5. resource_id
-- ============================================

CREATE OR REPLACE FUNCTION calculate_audit_event_hash()
RETURNS TRIGGER AS $$
DECLARE
    prev_hash VARCHAR(64);
    hash_input TEXT;
BEGIN
    -- Acquire advisory lock to serialize access to the last audit event hash
    -- This prevents concurrent inserts from reading the same previous_hash
    -- Lock is held for the transaction and released automatically at commit/rollback
    PERFORM pg_advisory_xact_lock(314159265); -- Fixed key for audit hash chain serialization

    -- Get the hash of the previous record (if any)
    SELECT hash INTO prev_hash
    FROM audit_events
    ORDER BY id DESC
    LIMIT 1;

    NEW.previous_hash := prev_hash;

    -- Build hash input with pipe delimiter (canonical field order)
    -- Use to_char with UTC timezone for deterministic timestamp representation
    hash_input := COALESCE(prev_hash, 'GENESIS') || '|' ||
                  NEW.event_id::text || '|' ||
                  NEW.event_type || '|' ||
                  to_char(NEW.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') || '|' ||
                  NEW.resource_id;

    -- Calculate SHA-256 hash
    NEW.hash := encode(sha256(hash_input::bytea), 'hex');

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
