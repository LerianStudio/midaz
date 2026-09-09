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
--   1. audit_hash_legacy_boundary records, once, the highest audit_events.id
--      written under the pre-000017 formula (0 when the deployment holds no
--      such row). This is the floor 000017 told operators to write down by
--      hand and nothing ever recorded, computed from the data itself.
--      The floor is recorded once and kept: the down migration does not drop it
--      and a re-apply does not re-measure it, so the first honest measurement
--      survives every rollback. An append-only floor that a rollback resets is
--      not append-only.
--   2. verify_audit_hash_chain accepts a row when its stored hash matches the
--      CURRENT nine-field formula (migrations 000017 / 000023), or — only for
--      a row at or below that boundary — the LEGACY five-field formula
--      (migrations 000001 / 000002 / 000004). Those are the only two formulas
--      this schema has ever written.
--
--   The chain-break check is unchanged: it compares stored previous_hash
--   against the running hash, is formula independent, and stays continuous
--   across the boundary because the trigger always chains to the
--   predecessor's stored hash whatever formula produced it.
--
-- Why the fallback is POSITIONAL and not a property of the row:
--   The legacy input is a strict PREFIX of the current one, and the last field
--   the two share (resource_id) is free-form VARCHAR(255) with no CHECK. So a
--   fallback offered on the row's CONTENT is forgeable with no hash weakness
--   at all, in two distinct shapes:
--
--     * ABSORPTION — rewrite resource_id to
--       '<resource_id>|<actor_type>|<actor_id>|<actor_name>|<actor_ip>' and put
--       anything in the actor columns. The row's legacy digest then equals its
--       original current digest, so stored hash and previous_hash never change
--       and an off-site digest export still matches.
--
--     * SLEEPER — write ONLY the hash column of the newest row, setting it to
--       that row's own legacy digest and touching nothing else. The row now
--       reads as legacy; the service keeps appending and it becomes an ordinary
--       middle row. From then on its actor columns can be rewritten any number
--       of times with NO write to any hash column anywhere.
--
--   Neither shape can be told from genuine legacy data by inspecting the row:
--   a sleeper's fields are, by construction, exactly what the service wrote.
--   Counting the '|' separators in the shared input catches absorption and
--   misses the sleeper entirely, while refusing the fallback to any genuine
--   pre-000017 row whose resource_id happens to contain a separator — which
--   nothing forbids, and which reproduces the very defect this migration
--   exists to fix.
--
--   Being written before the re-baseline is a POSITION in the chain, not a
--   property of a row's contents, and audit_events.id is assigned inside the
--   hash-chain advisory lock (migration 000023) so id order is chain order.
--   Recording that position once, at upgrade time, is therefore the only rule
--   that admits every genuine historical row and no rewritten one.
--
-- How the boundary is computed:
--   One pass over audit_events, one sha256 per row: the highest id whose stored
--   hash equals its own legacy five-field digest. A row cannot match both
--   formulas (the legacy input is a prefix of the current one, so agreement
--   would be a sha256 collision), so one digest per row decides it. The digest
--   uses the row's OWN stored previous_hash — the exact value the trigger fed
--   into it — so the classification of one row never depends on another and a
--   chain break cannot move the boundary.
--
--   The pass is a plain SELECT: it takes ACCESS SHARE on audit_events, which
--   does not block the INSERT path, and it does not take the chain's advisory
--   lock. A row committed after its snapshot is current-format by definition
--   and correctly lands above the boundary.
--
-- What each row keeps:
--   * A post-boundary row keeps full nine-field coverage, actor fields
--     included, and can never be re-read as a legacy input — neither by
--     absorption nor by a sleeper conversion.
--   * A pre-boundary row regains exactly the coverage it always had — the
--     first five fields, resource_id included, whatever characters it holds.
--     Its stored hash contains no information about the actor columns, so no
--     verifier can give it actor coverage retroactively.
--     ACCEPTED LIMIT, stated plainly: rewriting an actor column on a
--     pre-000017 row is not detectable. Before this migration that row was
--     reported invalid unconditionally, along with every other historical row,
--     which is an alarm that fires on a clean trail rather than tamper
--     evidence.
--   * RESIDUAL 1 — rolling deploys. Two replica generations writing at once can
--     commit a handful of current-format rows below the last legacy id, and
--     those rows sit under the boundary. They verify under the current formula,
--     so nothing about them changes unless they are rewritten — but if one is,
--     the absorption shape above is available on it. The window is the overlap
--     of the two generations and is bounded by the boundary row's id.
--     Conversely, an old replica that commits a legacy-format row AFTER this
--     migration takes its snapshot lands above the boundary and is reported
--     invalid. Draining the old generation before upgrading avoids both.
--   * RESIDUAL 2 — a conversion made BEFORE this migration runs. The boundary
--     is computed from the data, and a sleeper planted before it runs is, by
--     construction, indistinguishable from genuine pre-000017 data: a row whose
--     stored hash is its own five-field digest, chained correctly, with every
--     field exactly as the trigger wrote it. Nothing in the database separates
--     the two, so such a row raises the recorded floor and everything below it
--     stays legacy-eligible. Two things bound this. It needs the privilege to
--     disable the append-only rules at upgrade time — the same privilege that
--     could rewrite the floor afterwards, so it is not a new class of access.
--     And recorded_at gives an auditor the out-of-band check: a floor recorded
--     outside the deployment's upgrade window, or one that names an id newer
--     than the operator's own pre-upgrade high-water mark, is the signal.
--     The verifier this migration replaces refuses such a row, so on this one
--     point the change trades detection for the ability to verify at all. It
--     narrows the previous repair's equivalent hole, which stood permanently,
--     to the interval before the upgrade - and to that interval ONCE: the down
--     migration keeps the floor and this INSERT never re-measures it, so a
--     migration cycle cannot re-open the window.
--
--   Only the verifier changes. calculate_audit_event_hash() still writes the
--   nine-field formula, so every newly inserted row is covered in full.
--
-- Runtime, and the one way this migration can fail:
--   This is the first tracer migration whose duration scales with the audit
--   trail. The pass is CPU-bound on sha256 and essentially linear in row count:
--   ~2.3 s per million rows, so ~22 s at 10M, ~114 s at 50M, ~4 min at 100M on
--   a fast host, and a multiple of that on a small managed instance. Row width
--   barely matters.
--
--   Nothing in the tenant-manager migration path sets a statement timeout, but
--   an inherited one - an RDS/Aurora parameter group, a DBA default, a BYOC
--   standard - would abort the pass mid-transaction. The data rolls back
--   cleanly, but golang-migrate leaves schema_migrations at version 24 with
--   dirty = true and refuses every later apply. SET LOCAL below removes the
--   timeout for this migration's transaction only, so that cannot happen.
--
--   If an operator still meets a wedged tenant, from an earlier apply or a
--   cancelled session:
--     migrate -path <migrations> -database <url> force 23
--     migrate -path <migrations> -database <url> up
--   force 23 only clears the dirty flag; the boundary table and the new
--   function were rolled back with the aborted transaction, so the re-run
--   recomputes them from the same rows.
-- ============================================

-- The pass over audit_events below is proportional to the trail, so an
-- inherited database- or role-level statement_timeout would abort it and leave
-- the tenant's migration state dirty. LOCAL scopes this to the migration's own
-- transaction and nothing else.
SET LOCAL statement_timeout = 0;

-- ============================================
-- 1. Record the re-baseline boundary.
-- ============================================

CREATE TABLE IF NOT EXISTS audit_hash_legacy_boundary (
    -- One row, forever: the CHECK plus the primary key make a second one
    -- impossible.
    singleton     BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK (singleton),

    -- Highest audit_events.id written under the pre-000017 five-field formula.
    -- 0 means the deployment has no pre-000017 data and no row is ever offered
    -- the legacy formula.
    max_legacy_id BIGINT  NOT NULL,

    -- When the floor was recorded. An auditor comparing this against the
    -- deployment's upgrade window is the one check that catches a boundary
    -- computed over a trail that had already been altered.
    recorded_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

COMMENT ON TABLE audit_hash_legacy_boundary IS
    'Audit hash-chain re-baseline floor (migration 000024). max_legacy_id is the highest audit_events.id written under the pre-000017 five-field hash formula; verify_audit_hash_chain offers the legacy formula only at or below it. Append-only, like audit_events itself: raising this value re-opens the legacy formula on rows written after the re-baseline.';

-- Measured ONCE, and never re-measured. The HAVING makes a re-apply a no-op
-- instead of a second measurement, and the down migration keeps the table for
-- the same reason: a floor re-derived after the trail has been altered is a
-- floor the alteration chose. Without this pair, anyone able to run a migration
-- cycle re-opens the pre-upgrade window at will, and the honest recording -
-- recorded_at included - is overwritten by the new one.
INSERT INTO audit_hash_legacy_boundary (singleton, max_legacy_id)
SELECT TRUE, COALESCE(MAX(id), 0)
FROM audit_events
WHERE hash = encode(sha256((COALESCE(previous_hash, 'GENESIS')
        || '|' || event_id::text
        || '|' || event_type
        || '|' || to_char(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')
        || '|' || resource_id)::bytea), 'hex')
HAVING NOT EXISTS (SELECT 1 FROM audit_hash_legacy_boundary);

-- Same append-only protection audit_events carries (migration 000004). The
-- boundary decides which rows may be read under the weaker formula, so it must
-- be no easier to move than the trail it guards.
CREATE OR REPLACE RULE prevent_audit_hash_legacy_boundary_update AS
    ON UPDATE TO audit_hash_legacy_boundary DO INSTEAD NOTHING;

CREATE OR REPLACE RULE prevent_audit_hash_legacy_boundary_delete AS
    ON DELETE TO audit_hash_legacy_boundary DO INSTEAD NOTHING;

-- TRUNCATE bypasses RULEs, which is why migration 000004 pairs its rules with a
-- prevent_truncate() trigger on every immutable table. Without the trigger here
-- the floor is strictly easier to move than the trail it guards: TRUNCATE plus
-- one INSERT raises it above every row, with no DDL and no rule dropped, and
-- forges recorded_at along with it. DROP TABLE takes this trigger with it, so
-- the down migration needs nothing.
CREATE OR REPLACE TRIGGER prevent_audit_hash_legacy_boundary_truncate_trigger
    BEFORE TRUNCATE ON audit_hash_legacy_boundary
    FOR EACH STATEMENT
    EXECUTE FUNCTION prevent_truncate();

-- The verifier reads the floor on every call, so a caller that can already read
-- audit_events must be able to read it too, or GET /v1/audit-events/{id}/verify
-- raises "permission denied" instead of answering - and migration 000004's own
-- production guidance provisions exactly such a role. The table holds one
-- bigint and one timestamp; SELECT on it discloses nothing audit_events does
-- not, and PUBLIC inside a tenant database is bounded by CONNECT.
GRANT SELECT ON audit_hash_legacy_boundary TO PUBLIC;

-- ============================================
-- 2. Offer the legacy formula only below the boundary.
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
    legacy_boundary BIGINT;
    checked_count BIGINT := 0;
    invalid_id BIGINT := NULL;
    chain_valid BOOLEAN := TRUE;
    err_detail TEXT := NULL;
BEGIN
    -- Read the re-baseline floor once. An absent row means no legacy formula is
    -- offered to anything, which is the fail-closed direction.
    SELECT max_legacy_id INTO legacy_boundary FROM audit_hash_legacy_boundary;
    legacy_boundary := COALESCE(legacy_boundary, 0);

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
            -- Offered ONLY to a row that predates the re-baseline, and computed
            -- only on a miss, so a clean deployment pays no extra digest.
            --
            -- Above the boundary the row was written by the nine-field trigger,
            -- so a stored hash that matches only the five-field formula means
            -- the hash column was rewritten. Refusing it there is what keeps a
            -- post-re-baseline row from being converted into a legacy row and
            -- then quietly re-attributed.
            IF rec.id <= legacy_boundary THEN
                legacy_hash := encode(sha256(shared_input::bytea), 'hex');
            ELSE
                legacy_hash := NULL;
            END IF;

            IF legacy_hash IS NULL OR rec.hash != legacy_hash THEN
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

-- The floor decides which rows may be read under the weaker formula, so the
-- verifier must not resolve its name through the CALLER's search_path. A role
-- with CREATE on the database - which is what GRANT ALL PRIVILEGES ON DATABASE
-- carries - can otherwise create its own audit_hash_legacy_boundary in a schema
-- it owns, put that schema first, and from then on read a floor of its choosing
-- on every verification it runs.
--
-- The pin has to name the RESOLVED schema. `SET search_path FROM CURRENT`
-- captures the literal text of the migration session's path, which is
-- '"$user", public' on any deployment that migrates without setting one, and
-- $user is re-resolved to the CALLING role at execution time - so a schema
-- named after that role still shadows the floor. current_schema() is the schema
-- the CREATE TABLE above just used, differs per tenant, and holds no element a
-- caller can influence.
DO $pin$
BEGIN
    EXECUTE format(
        'ALTER FUNCTION verify_audit_hash_chain(BIGINT, BIGINT) SET search_path = %I',
        current_schema());
END
$pin$;
