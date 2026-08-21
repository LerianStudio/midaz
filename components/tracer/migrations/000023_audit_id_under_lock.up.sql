-- ============================================
-- Migration: 000023_audit_id_under_lock
-- Description: Assign audit_events.id INSIDE the advisory-locked region so the
--   id order can never disagree with the hash-chain link order.
--
-- Background — audit hash chain forks under concurrency:
--   audit_events.id is BIGSERIAL (000004). Its nextval is evaluated at INSERT
--   default-expansion — BEFORE the BEFORE-INSERT trigger
--   calculate_audit_event_hash() acquires pg_advisory_xact_lock(314159265) and
--   reads the predecessor by max committed id (formula from 000017). A lower-id
--   transaction can therefore stall between "id materialized" and "critical
--   section entered" while a higher-id transaction commits first; the lower-id
--   row then links to the higher row's hash. verify_audit_hash_chain walks
--   id ASC and flags that lower id (field symptom: firstInvalidId 1553).
--   Atomicity was never the issue (the BEFORE trigger runs in-txn; rollback
--   discards) — ordering was.
--
-- What changes here:
--   1. Drop the BIGSERIAL DEFAULT on audit_events.id. The underlying sequence
--      audit_events_id_seq is KEPT (still OWNED BY the column); only the default
--      expression is removed.
--   2. CREATE OR REPLACE calculate_audit_event_hash() so that, AFTER acquiring
--      pg_advisory_xact_lock(314159265) and BEFORE reading the predecessor, it
--      assigns NEW.id := nextval('audit_events_id_seq'). The row that reads
--      predecessor P is then guaranteed the next id after P: id-order ==
--      hash-chain order. The actor-in-hash formula and previous_hash linkage
--      are byte-for-byte identical to 000017.
--
--   verify_audit_hash_chain() is intentionally NOT touched (the ascending-id
--   verifier is preserved as-is), and neither are the anti-tamper protections
--   (anti-TRUNCATE trigger, prevent_audit_event_update / _delete rules).
--
-- Sequence name:
--   audit_events_id_seq is the deterministic BIGSERIAL name for
--   audit_events.id and remains resolvable via
--   pg_get_serial_sequence('audit_events','id') after the DEFAULT is dropped
--   (the OWNED BY dependency is unaffected).
--
-- Safety: metadata-only + trigger replacement. Dropping a column DEFAULT and
--   CREATE OR REPLACE FUNCTION are catalog operations; there is NO table
--   rewrite and no blocking scan of audit_events.
-- ============================================

-- 1. Move id assignment out of the client-visible DEFAULT. Keep the sequence.
ALTER TABLE audit_events ALTER COLUMN id DROP DEFAULT;

-- 2. Assign the id inside the advisory-locked region, monotonic with the chain.
CREATE OR REPLACE FUNCTION calculate_audit_event_hash()
RETURNS TRIGGER AS $$
DECLARE
    prev_hash VARCHAR(64);
    hash_input TEXT;
BEGIN
    -- Advisory lock (314159265 — pi digits, same key as migration 000001).
    -- Serializes concurrent inserts so id assignment, the read of the
    -- previous_hash and the write of the new row's hash are atomic relative to
    -- the chain.
    PERFORM pg_advisory_xact_lock(314159265);

    -- Assign the id INSIDE the locked region. Because nextval now runs under
    -- the same lock that guards the predecessor read below, the row that reads
    -- predecessor P is guaranteed the next id after P: id-order == chain-order.
    NEW.id := nextval('audit_events_id_seq');

    -- Read the most recent row's hash to chain to.
    SELECT hash INTO prev_hash
    FROM audit_events
    ORDER BY id DESC
    LIMIT 1;

    NEW.previous_hash := prev_hash;

    -- Canonical field order — byte-for-byte identical to migration 000017:
    -- the first five fields preserve their positions from 000001; the four
    -- actor fields are appended. MUST stay in lockstep with
    -- verify_audit_hash_chain().
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
