-- Restore the single-formula verifier from migration 000017 and discard the
-- re-baseline boundary.
--
-- After this runs, an upgraded deployment holding pre-000017 audit rows reports
-- is_valid = false on every verification again, and the scan stops at the first
-- historical row.
--
-- The restored function body below MUST stay byte-identical to the one
-- migration 000017 defines: nothing here may depend on the boundary table,
-- because the boundary table is dropped at the end of this file.

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
    prev_hash VARCHAR(64);
    expected_hash VARCHAR(64);
    hash_input TEXT;
    checked_count BIGINT := 0;
    invalid_id BIGINT := NULL;
    chain_valid BOOLEAN := TRUE;
    err_detail TEXT := NULL;
BEGIN
    SELECT hash INTO prev_hash FROM audit_events WHERE id < start_id ORDER BY id DESC LIMIT 1;
    IF prev_hash IS NULL THEN
        prev_hash := 'GENESIS';
    END IF;

    FOR rec IN
        SELECT * FROM audit_events
        WHERE id >= start_id
        AND (end_id IS NULL OR id <= end_id)
        ORDER BY id ASC
    LOOP
        checked_count := checked_count + 1;

        hash_input := prev_hash
            || '|' || rec.event_id::text
            || '|' || rec.event_type
            || '|' || to_char(rec.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')
            || '|' || rec.resource_id
            || '|' || rec.actor_type::text
            || '|' || rec.actor_id
            || '|' || COALESCE(rec.actor_name, '')
            || '|' || COALESCE(rec.actor_ip_address, '');
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

-- Discard the boundary. The append-only rules go with the table, and a later
-- re-apply of 000024 recomputes the same value from the same rows.
DROP TABLE IF EXISTS audit_hash_legacy_boundary;
