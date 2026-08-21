-- ============================================
-- Migration: 000023_audit_id_under_lock (DOWN)
-- Description: Restore the pre-000023 layout — id assigned by the BIGSERIAL
--   DEFAULT (outside the lock) and the 000017 trigger body that does NOT touch
--   NEW.id. Lossless with the up: no data is rewritten either way.
--
-- WARNING — re-introduces the concurrency fork:
--   Rolling back returns id assignment to INSERT default-expansion, so the
--   lower-id-links-to-higher-id fork this migration closes becomes possible
--   again under concurrency. Roll back only with that understanding.
-- ============================================

-- 1. Restore the BIGSERIAL DEFAULT on audit_events.id from the kept sequence.
ALTER TABLE audit_events ALTER COLUMN id SET DEFAULT nextval('audit_events_id_seq'::regclass);

-- 2. Restore the pre-000023 calculate_audit_event_hash() — identical to the
--    000017 body (id is NOT assigned in the trigger).
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
