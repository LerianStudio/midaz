-- ============================================
-- Migration: 000017_audit_actor_in_hash
-- Description: Harden the audit hash chain to cover actor identity fields.
--
-- Background — Taura security finding:
--   Audit events were being recorded with a hardcoded generic actor
--   (type=system, id=svc_tracer). The companion code change captures the
--   real authenticated identity (JWT sub or API-key label) via Principal in
--   request context. This migration closes the remaining gap: tampering of
--   actor_type / actor_id / actor_name / actor_ip on existing rows would not
--   be detected by verify_audit_hash_chain because the original hash formula
--   (migrations 000001 / 000002) covered only:
--       previous_hash | event_id | event_type | created_at | resource_id
--
-- What changes here:
--   The canonical hash input is extended (append-only — keeps existing field
--   positions for diff auditability) with the four actor fields:
--       previous_hash | event_id | event_type | created_at | resource_id
--       | actor_type | actor_id | COALESCE(actor_name,'') | COALESCE(actor_ip_address,'')
--
--   Both calculate_audit_event_hash() (trigger) and verify_audit_hash_chain()
--   are replaced in lockstep so the chain stays internally consistent.
--
-- Trade-off (RE-BASELINE):
--   Historical rows were inserted under the OLD trigger that hashed only the
--   first five fields. Their stored hash will NOT match the new formula. The
--   verifier will report "Hash mismatch" on those rows from this migration
--   onward — that is the expected, documented behavior. Operators with
--   production audit data should:
--     1. Snapshot verify_audit_hash_chain() before applying this migration.
--     2. Apply 000017.
--     3. Document the pre-migration boundary as the verification floor
--        (any audit-trail verification after upgrade should start from
--        id = N+1 where N is the highest pre-migration id).
--
-- actor_type_enum:
--   The enum is extended with 'api_key' so requests authenticated via the
--   API Key middleware can be attributed to a deployment label (e.g.
--   "tracer-default", "tracer-prod-eu") instead of falling back to "system".
--   ALTER TYPE ... ADD VALUE is one-way in PostgreSQL — the .down.sql does
--   NOT remove the value (would require DROP TYPE + recreate, destroying
--   the audit table). Down restores only the hash formulas.
-- ============================================

-- 1. Extend the actor_type_enum with the api_key variant.
ALTER TYPE actor_type_enum ADD VALUE IF NOT EXISTS 'api_key';

-- ============================================
-- 2. Replace calculate_audit_event_hash() with the actor-aware formula.
-- ============================================

CREATE OR REPLACE FUNCTION calculate_audit_event_hash()
RETURNS TRIGGER AS $$
DECLARE
    prev_hash VARCHAR(64);
    hash_input TEXT;
BEGIN
    -- Advisory lock (314159265 — pi digits, same key as migration 000001).
    -- Serializes concurrent inserts so the read of the previous_hash and the
    -- write of the new row's hash are atomic relative to the chain.
    PERFORM pg_advisory_xact_lock(314159265);

    -- Read the most recent row's hash to chain to.
    SELECT hash INTO prev_hash
    FROM audit_events
    ORDER BY id DESC
    LIMIT 1;

    NEW.previous_hash := prev_hash;

    -- Canonical field order (extended): the first five fields preserve their
    -- positions from migration 000001; the four actor fields are appended.
    -- COALESCE on the nullable actor_name maps NULL to '' so the hash input
    -- is deterministic regardless of whether the column is populated.
    hash_input := COALESCE(prev_hash, 'GENESIS')
        || '|' || NEW.event_id::text
        || '|' || NEW.event_type
        || '|' || to_char(NEW.created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')
        || '|' || NEW.resource_id
        || '|' || NEW.actor_type::text
        || '|' || NEW.actor_id
        || '|' || COALESCE(NEW.actor_name, '')
        || '|' || COALESCE(NEW.actor_ip_address, '');

    NEW.hash := encode(sha256(hash_input::bytea), 'hex');

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- ============================================
-- 3. Replace verify_audit_hash_chain() to match the new formula.
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
    hash_input TEXT;
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

        -- MUST stay byte-for-byte identical to calculate_audit_event_hash above.
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
