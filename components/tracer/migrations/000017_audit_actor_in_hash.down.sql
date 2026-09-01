-- ============================================
-- Migration: 000017_audit_actor_in_hash (DOWN)
-- Description: Restore the pre-000017 hash formula (5 canonical fields only).
--
-- IMPORTANT — partial rollback:
--   The enum value 'api_key' added to actor_type_enum CANNOT be removed by a
--   .down.sql in PostgreSQL without DROP TYPE + recreate, which would destroy
--   the audit_events table (the column references the enum). The value is
--   left in place; rows inserted with actor_type='api_key' between 000017
--   apply and rollback will remain readable but the new verifier (after
--   rollback) will report them as a Hash mismatch because the down-restored
--   formula does not include actor fields.
--
--   Operators rolling back MUST accept that the chain is no longer
--   verifiable across the 000017 window. This is symmetric to the
--   re-baseline declared in the .up.sql header.
-- ============================================

-- 1. Restore the original (pre-000017) calculate_audit_event_hash() — formula
--    from migration 000001, hashing only the five canonical fields.
CREATE OR REPLACE FUNCTION calculate_audit_event_hash()
RETURNS TRIGGER AS $$
DECLARE
    prev_hash VARCHAR(64);
    hash_input TEXT;
BEGIN
    PERFORM pg_advisory_xact_lock(314159265);

    SELECT hash INTO prev_hash
    FROM audit_events
    ORDER BY id DESC
    LIMIT 1;

    NEW.previous_hash := prev_hash;

    hash_input := COALESCE(prev_hash, 'GENESIS') || '|' ||
                  NEW.event_id::text || '|' ||
                  NEW.event_type || '|' ||
                  to_char(NEW.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') || '|' ||
                  NEW.resource_id;

    NEW.hash := encode(sha256(hash_input::bytea), 'hex');

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- 2. Restore the original verify_audit_hash_chain() — formula from migration 000002.
DROP FUNCTION IF EXISTS verify_audit_hash_chain(BIGINT, BIGINT);

CREATE FUNCTION verify_audit_hash_chain(
    start_id BIGINT DEFAULT 1,
    end_id BIGINT DEFAULT NULL
)
RETURNS TABLE (
    is_valid BOOLEAN,
    first_invalid_id BIGINT,
    total_checked BIGINT,
    error_detail TEXT
) AS $$
DECLARE
    rec RECORD;
    prev_hash VARCHAR(64) := 'GENESIS';
    expected_hash VARCHAR(64);
    hash_input TEXT;
    checked_count BIGINT := 0;
    invalid_id BIGINT := NULL;
    chain_valid BOOLEAN := TRUE;
    err_detail TEXT := NULL;
BEGIN
    FOR rec IN
        SELECT * FROM audit_events
        WHERE id >= start_id
        AND (end_id IS NULL OR id <= end_id)
        ORDER BY id ASC
    LOOP
        checked_count := checked_count + 1;

        hash_input := prev_hash || '|' ||
                      rec.event_id::text || '|' ||
                      rec.event_type || '|' ||
                      to_char(rec.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') || '|' ||
                      rec.resource_id;
        expected_hash := encode(sha256(hash_input::bytea), 'hex');

        IF rec.hash != expected_hash THEN
            chain_valid := FALSE;
            invalid_id := rec.id;
            err_detail := 'Hash mismatch: expected ' || expected_hash || ', got ' || rec.hash;
            EXIT;
        END IF;

        IF COALESCE(rec.previous_hash, 'GENESIS') != prev_hash THEN
            chain_valid := FALSE;
            invalid_id := rec.id;
            err_detail := 'Chain break: expected previous_hash ' || prev_hash || ', got ' || COALESCE(rec.previous_hash, 'NULL');
            EXIT;
        END IF;

        prev_hash := rec.hash;
    END LOOP;

    RETURN QUERY SELECT chain_valid, invalid_id, checked_count, err_detail;
END;
$$ LANGUAGE plpgsql;
