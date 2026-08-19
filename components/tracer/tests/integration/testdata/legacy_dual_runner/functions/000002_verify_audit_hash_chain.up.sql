-- ============================================
-- Function: verify_audit_hash_chain
-- Verifies the integrity of the hash chain
-- Returns true if chain is valid, false if tampered
--
-- IMPORTANT: Must use the same field order and delimiter as calculate_audit_event_hash()
-- Order: previous_hash | event_id | event_type | created_at | resource_id
-- ============================================

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

        -- Calculate expected hash (MUST match calculate_audit_event_hash exactly)
        -- Use to_char with UTC timezone for deterministic timestamp representation
        hash_input := prev_hash || '|' ||
                      rec.event_id::text || '|' ||
                      rec.event_type || '|' ||
                      to_char(rec.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"') || '|' ||
                      rec.resource_id;
        expected_hash := encode(sha256(hash_input::bytea), 'hex');

        -- Check if stored hash matches expected
        IF rec.hash != expected_hash THEN
            chain_valid := FALSE;
            invalid_id := rec.id;
            err_detail := 'Hash mismatch: expected ' || expected_hash || ', got ' || rec.hash;
            EXIT;
        END IF;

        -- Check if previous_hash chain is intact
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
