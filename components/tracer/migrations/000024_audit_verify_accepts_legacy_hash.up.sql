-- ============================================
-- Migration: 000024_audit_verify_accepts_legacy_hash
-- Description: Make verify_audit_hash_chain() accept a row written under
--              EITHER canonical hash formula, so an upgraded deployment stops
--              reporting its whole audit trail as tampered with.
--
-- The problem this closes:
--   Migration 000017 re-baselined the canonical hash input by appending the
--   four actor fields. Its own header records the consequence: rows inserted
--   under the pre-000017 trigger hashed only the first five fields, so "their
--   stored hash will NOT match the new formula", and it instructed operators to
--   treat the pre-migration boundary as a verification floor and verify from
--   id = N+1.
--
--   No such floor exists. The single production caller of this function is
--   GET /v1/audit-events/{id}/verify, which calls
--   verify_audit_hash_chain(1, <internal id>) — the start id is a literal 1 and
--   no route, query parameter or header can move it. An upgraded deployment
--   therefore verifies from the first row, meets the first pre-000017 row,
--   reports is_valid = false with "Hash mismatch", and EXITs there. Because the
--   loop leaves on the first mismatch, total_checked stops at that row too: not
--   only is the answer wrong, every row after the boundary goes unchecked, so
--   REAL tampering anywhere in the trail became undetectable.
--
-- Why the boundary cannot be repaired in data:
--   audit_events carries prevent_audit_event_update and
--   prevent_audit_event_delete (ON UPDATE / ON DELETE DO INSTEAD NOTHING, from
--   migration 000004). The table is append-only by construction, so no
--   migration can re-hash the historical rows to the new formula. Doing it
--   would mean dropping the append-only rules and rewriting the audit trail —
--   destroying the immutability the feature exists to provide.
--
-- What changes here:
--   A row is accepted when its stored hash matches the CURRENT nine-field
--   formula (migrations 000017 / 000023) or the LEGACY five-field formula
--   (migrations 000001 / 000002 / 000004). Those are the only two formulas
--   this schema has ever written. The chain-break check is unchanged: it
--   compares stored previous_hash against the running hash and is formula
--   independent, and the chain stays continuous across the boundary because
--   the trigger always chains to the predecessor's stored hash whatever
--   formula produced it.
--
-- Why this does not weaken tamper evidence:
--   * A post-000017 row can never match the legacy formula. Its stored hash is
--     the sha256 of the nine-field input; the five-field hash of the same row
--     is a different digest. So those rows keep full nine-field coverage,
--     actor fields included.
--   * A pre-000017 row regains exactly the coverage it always had — the first
--     five fields. Its stored hash contains no information about the actor
--     columns, so no verifier can give it actor coverage retroactively.
--   * Detection strictly increases. Today the scan stops at the first
--     historical row and checks nothing beyond it; after this change the scan
--     runs the whole requested range and flags any row that matches neither
--     formula.
--
--   Only the verifier changes. calculate_audit_event_hash() still writes the
--   nine-field formula, so every newly inserted row is covered in full.
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
    prev_hash VARCHAR(64);
    expected_hash VARCHAR(64);
    legacy_hash VARCHAR(64);
    shared_input TEXT;
    checked_count BIGINT := 0;
    invalid_id BIGINT := NULL;
    chain_valid BOOLEAN := TRUE;
    err_detail TEXT := NULL;
BEGIN
    -- Seed prev_hash from the row immediately before start_id (or GENESIS if
    -- start_id covers the first row of the chain).
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

        -- The five fields both formulas share, in their original positions.
        -- MUST stay byte-for-byte identical to the leading fields of
        -- calculate_audit_event_hash().
        shared_input := prev_hash
            || '|' || rec.event_id::text
            || '|' || rec.event_type
            || '|' || to_char(rec.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')
            || '|' || rec.resource_id;

        -- Current formula (000017 / 000023): the shared five plus the four
        -- actor fields.
        expected_hash := encode(sha256((shared_input
            || '|' || rec.actor_type::text
            || '|' || rec.actor_id
            || '|' || COALESCE(rec.actor_name, '')
            || '|' || COALESCE(rec.actor_ip_address, ''))::bytea), 'hex');

        IF rec.hash != expected_hash THEN
            -- Legacy formula (000001 / 000002 / 000004): the shared five alone.
            -- Computed only on a miss, so an unmigrated-data deployment pays one
            -- extra digest per historical row and a clean one pays none.
            legacy_hash := encode(sha256(shared_input::bytea), 'hex');

            IF rec.hash != legacy_hash THEN
                chain_valid := FALSE;
                invalid_id := rec.id;
                err_detail := 'Hash mismatch: expected ' || expected_hash || ', got ' || rec.hash;
                EXIT;
            END IF;
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
