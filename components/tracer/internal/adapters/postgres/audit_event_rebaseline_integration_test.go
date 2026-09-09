// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package postgres

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
)

// =============================================================================
// Audit hash-chain verification across the 000017 actor re-baseline.
//
// Migration 000017 appended the four actor fields to the canonical hash input.
// Rows written by the pre-000017 trigger hashed only the first five fields, and
// audit_events is append-only (prevent_audit_event_update /
// prevent_audit_event_delete, migration 000004), so those hashes can never be
// rewritten to the new formula.
//
// GET /v1/audit-events/{id}/verify calls verify_audit_hash_chain(1, <id>) with
// the start id as a literal — no route, parameter or header can raise the floor
// past the boundary. So an upgraded deployment used to meet its first
// pre-000017 row, report "Hash mismatch", and EXIT there: the trail read as
// tampered with AND nothing past that row was ever checked.
//
// verify_audit_hash_chain now offers the legacy formula only to rows at or
// below the re-baseline boundary that migration 000024 records once, at upgrade
// time, in audit_hash_legacy_boundary. Being written before the re-baseline is a
// position in the chain, not a property of a row's contents, and audit_events.id
// is assigned inside the chain's advisory lock (migration 000023) so id order is
// chain order.
//
// These tests lock the properties that matter: the historical trail verifies
// whatever its resource_id holds, tampering is still caught on both sides of the
// boundary, and no row written after the re-baseline can be re-read under the
// shorter formula.
// =============================================================================

// legacyChainFixture rebuilds an upgraded deployment's audit table inside tx:
// legacyRows rows carrying pre-000017 five-field hashes, followed by
// currentRows rows written by the live trigger, which chains onto the last
// historical hash exactly as it would in production. resourceIDPrefix is
// interpolated into the historical rows' resource_id so a caller can put
// characters of its choosing — a '|' separator included — into the one field
// both hash formulas share.
//
// The boundary is then recorded exactly as migration 000024 records it, which is
// what "the upgrade ran while these historical rows were the whole table" means.
//
// The append-only rules are disabled for the duration of tx (ALTER TABLE is
// transactional in PostgreSQL, so the rollback restores them) because forging a
// historical row is the only way to reproduce a pre-migration hash: the trigger
// always overwrites NEW.hash with the current formula.
func legacyChainFixture(t *testing.T, tx *sql.Tx, legacyRows, currentRows int, resourceIDPrefix string) {
	t.Helper()

	ctx := context.Background()

	for _, stmt := range []string{
		"ALTER TABLE audit_events DISABLE RULE prevent_audit_event_update",
		"ALTER TABLE audit_events DISABLE RULE prevent_audit_event_delete",
		"DELETE FROM audit_events",
	} {
		_, err := tx.ExecContext(ctx, stmt)
		require.NoError(t, err, "prepare append-only table for the fixture: %s", stmt)
	}

	_, err := tx.ExecContext(ctx, `
		INSERT INTO audit_events (event_type, action, result, resource_id, resource_type,
		                          actor_type, actor_id, actor_name, actor_ip_address)
		SELECT 'LIMIT_CREATED','CREATE','SUCCESS',$2||'-'||g,'limit','user','u'||g,'name'||g,'10.0.0.'||g
		FROM generate_series(1, $1) g`, legacyRows, resourceIDPrefix)
	require.NoError(t, err, "insert the rows that will carry historical hashes")

	// Re-hash them under the legacy five-field formula, chaining as the old
	// trigger did. This must happen before the current-formula rows are
	// inserted, so the trigger reads the last historical hash as its predecessor.
	_, err = tx.ExecContext(ctx, `
		DO $$
		DECLARE rec RECORD; prev text := 'GENESIS'; h text;
		BEGIN
		  FOR rec IN SELECT * FROM audit_events ORDER BY id ASC LOOP
		    h := encode(sha256((prev
		       || '|' || rec.event_id::text
		       || '|' || rec.event_type
		       || '|' || to_char(rec.created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US"Z"')
		       || '|' || rec.resource_id)::bytea),'hex');
		    UPDATE audit_events SET previous_hash = NULLIF(prev,'GENESIS'), hash = h WHERE id = rec.id;
		    prev := h;
		  END LOOP;
		END $$`)
	require.NoError(t, err, "rewrite the historical rows under the legacy formula")

	recordRebaselineBoundary(t, tx)

	_, err = tx.ExecContext(ctx, `
		INSERT INTO audit_events (event_type, action, result, resource_id, resource_type,
		                          actor_type, actor_id, actor_name, actor_ip_address)
		SELECT 'LIMIT_UPDATED','UPDATE','SUCCESS','post-'||g,'limit','api_key','k'||g,'kname'||g,'10.0.1.'||g
		FROM generate_series(1, $1) g`, currentRows)
	require.NoError(t, err, "insert the rows written by the current trigger")
}

// recordRebaselineBoundary re-runs migration 000024's own boundary computation
// against the table as it stands right now, which is how a test says "the
// upgrade ran at this point in the table's life". The statement is copied from
// the migration deliberately: if the two ever diverge, these tests stop
// describing the shipped rule.
//
// The boundary is append-only in production, so the fixture disables its rules
// the same way it disables audit_events'. Both are restored by the rollback.
func recordRebaselineBoundary(t *testing.T, tx *sql.Tx) {
	t.Helper()

	ctx := context.Background()

	for _, stmt := range []string{
		"ALTER TABLE audit_hash_legacy_boundary DISABLE RULE prevent_audit_hash_legacy_boundary_update",
		"ALTER TABLE audit_hash_legacy_boundary DISABLE RULE prevent_audit_hash_legacy_boundary_delete",
		"DELETE FROM audit_hash_legacy_boundary",
		`INSERT INTO audit_hash_legacy_boundary (singleton, max_legacy_id)
		 SELECT TRUE, COALESCE(MAX(id), 0)
		 FROM audit_events
		 WHERE hash = encode(sha256((COALESCE(previous_hash, 'GENESIS')
		         || '|' || event_id::text
		         || '|' || event_type
		         || '|' || to_char(created_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS.US"Z"')
		         || '|' || resource_id)::bytea), 'hex')`,
	} {
		_, err := tx.ExecContext(ctx, stmt)
		require.NoError(t, err, "record the re-baseline boundary: %s", stmt)
	}
}

// readBoundary returns the recorded re-baseline floor.
func readBoundary(t *testing.T, tx *sql.Tx) int64 {
	t.Helper()

	var boundary int64
	require.NoError(t, tx.QueryRowContext(context.Background(),
		"SELECT max_legacy_id FROM audit_hash_legacy_boundary",
	).Scan(&boundary), "the boundary must be recorded by the migration")

	return boundary
}

// convertToLegacyDigest writes ONLY the hash column of one row, setting it to
// that row's own legacy five-field digest and touching no other column — the
// row's fields stay exactly as the service wrote them.
//
// This is the "sleeper" shape. It is the one conversion a content check cannot
// see, because there is nothing anomalous to see: the row is byte-identical to
// what the trigger produced apart from its digest. Once accepted, the service
// keeps appending and the row becomes an ordinary middle row whose actor columns
// can then be rewritten any number of times with no write to any hash column.
func convertToLegacyDigest(t *testing.T, tx *sql.Tx, id int64) {
	t.Helper()

	res, err := tx.ExecContext(context.Background(), `
		UPDATE audit_events SET hash = encode(sha256((
		      COALESCE(previous_hash, 'GENESIS')
		   || '|' || event_id::text
		   || '|' || event_type
		   || '|' || to_char(created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US"Z"')
		   || '|' || resource_id)::bytea),'hex')
		WHERE id = $1`, id)
	require.NoError(t, err, "re-base the row onto its own legacy digest")

	affected, err := res.RowsAffected()
	require.NoError(t, err, "read affected rows")
	require.Equal(t, int64(1), affected, "the conversion must rewrite exactly one row")
}

// verifyFromGenesis runs the verification the product runs: start id 1, the
// literal the repository hard-codes at audit_event_repository.go, over the
// whole chain.
func verifyFromGenesis(t *testing.T, tx *sql.Tx) (isValid bool, firstInvalidID sql.NullInt64, totalChecked int64) {
	t.Helper()

	var detail sql.NullString

	err := tx.QueryRowContext(context.Background(),
		"SELECT is_valid, first_invalid_id, total_checked, error_detail FROM verify_audit_hash_chain(1, NULL)",
	).Scan(&isValid, &firstInvalidID, &totalChecked, &detail)
	require.NoError(t, err, "verify_audit_hash_chain must be callable")

	return isValid, firstInvalidID, totalChecked
}

func beginRolledBackTx(t *testing.T) *sql.Tx {
	t.Helper()

	tx, err := testutil.SetupIntegrationDB(t).BeginTx(context.Background(), nil)
	require.NoError(t, err, "begin fixture transaction")

	t.Cleanup(func() {
		// Rollback restores the append-only rules and discards every forged row.
		if err := tx.Rollback(); err != nil && err != sql.ErrTxDone {
			t.Logf("rollback fixture transaction: %v", err)
		}
	})

	return tx
}

// TestIntegration_AuditHashChain_VerifiesAcrossTheActorRebaseline is the
// customer-facing assertion: a deployment upgraded past migration 000017 with
// pre-existing audit data reports its trail intact, and the verifier walks the
// WHOLE range instead of stopping at the boundary.
//
// Against the single-formula verifier this returns is_valid=false with
// total_checked=1 of 5 rows.
func TestIntegration_AuditHashChain_VerifiesAcrossTheActorRebaseline(t *testing.T) {
	tx := beginRolledBackTx(t)
	legacyChainFixture(t, tx, 3, 2, "res")

	isValid, firstInvalidID, totalChecked := verifyFromGenesis(t, tx)

	require.True(t, isValid,
		"an upgraded deployment MUST NOT report its audit trail as tampered with just because rows predate the 000017 re-baseline")
	require.False(t, firstInvalidID.Valid,
		"first_invalid_id MUST be NULL on a clean chain; got %d", firstInvalidID.Int64)
	require.Equal(t, int64(5), totalChecked,
		"the verifier MUST walk the whole chain; stopping early leaves later rows unchecked")
}

// TestIntegration_AuditHashChain_StillCatchesTamperedHistoricalRow proves the
// legacy fallback is not a blanket pass. A pre-000017 row's hash covers the
// first five fields, so altering one of them must still be detected — that is
// exactly the tamper evidence those rows always carried.
func TestIntegration_AuditHashChain_StillCatchesTamperedHistoricalRow(t *testing.T) {
	tx := beginRolledBackTx(t)
	legacyChainFixture(t, tx, 3, 2, "res")

	var target int64
	require.NoError(t, tx.QueryRowContext(context.Background(),
		"UPDATE audit_events SET resource_id = 'TAMPERED' WHERE id = (SELECT min(id) FROM audit_events) RETURNING id",
	).Scan(&target), "tamper the first historical row")

	isValid, firstInvalidID, _ := verifyFromGenesis(t, tx)

	require.False(t, isValid, "a tampered historical row MUST be reported invalid")
	require.True(t, firstInvalidID.Valid, "first_invalid_id MUST name the tampered row")
	require.Equal(t, target, firstInvalidID.Int64, "the tampered historical row MUST be the one flagged")
}

// TestIntegration_AuditHashChain_StillCatchesTamperedActorOnCurrentRow is the
// first security lock on the fallback. A row written under the current formula
// stores the nine-field digest of its own field values, so simply rewriting an
// actor column leaves the stored hash matching neither formula. actor_id is
// covered by the current formula only, so it is the field that would slip
// through a naive fallback.
//
// Rewriting an actor column is not the whole threat: the legacy input is a
// PREFIX of the current one, so a row can be reshaped so that its legacy digest
// equals its original current digest. That is
// TestIntegration_AuditHashChain_RefusesResourceIDAbsorptionForgery below.
func TestIntegration_AuditHashChain_StillCatchesTamperedActorOnCurrentRow(t *testing.T) {
	tx := beginRolledBackTx(t)
	legacyChainFixture(t, tx, 3, 2, "res")

	var target int64
	require.NoError(t, tx.QueryRowContext(context.Background(),
		"UPDATE audit_events SET actor_id = 'TAMPERED' WHERE id = (SELECT max(id) FROM audit_events) RETURNING id",
	).Scan(&target), "tamper the actor identity on the newest row")

	isValid, firstInvalidID, _ := verifyFromGenesis(t, tx)

	require.False(t, isValid,
		"rewriting actor identity on a post-000017 row MUST still be detected — the legacy fallback must not launder it")
	require.True(t, firstInvalidID.Valid, "first_invalid_id MUST name the tampered row")
	require.Equal(t, target, firstInvalidID.Int64, "the tampered row MUST be the one flagged")
}

// forgeAbsorption rewrites one row so that its LEGACY five-field digest equals
// the digest already stored for it under the CURRENT nine-field formula, and
// then replaces the actor identity with attackerActorID.
//
// It works by absorbing the four actor fields into resource_id, the last field
// the two formulas share and the only free-form one. Nothing recomputes a hash:
// the stored hash and previous_hash come out byte-identical, which is what makes
// this the dangerous shape — an off-site digest export or a write-once copy
// still matches after the rewrite.
//
// Returns the id of the rewritten row.
func forgeAbsorption(t *testing.T, tx *sql.Tx, id int64, attackerActorID string) {
	t.Helper()

	res, err := tx.ExecContext(context.Background(), `
		UPDATE audit_events
		SET resource_id = resource_id
		      || '|' || actor_type::text
		      || '|' || actor_id
		      || '|' || COALESCE(actor_name, '')
		      || '|' || COALESCE(actor_ip_address, ''),
		    actor_id = $2,
		    actor_name = '',
		    actor_ip_address = ''
		WHERE id = $1`, id, attackerActorID)
	require.NoError(t, err, "absorb the actor fields into resource_id")

	affected, err := res.RowsAffected()
	require.NoError(t, err, "read affected rows")
	require.Equal(t, int64(1), affected, "the forgery must rewrite exactly one row")
}

// storedDigests reads the two columns a tamper must leave untouched to stay
// invisible to an off-site digest export.
func storedDigests(t *testing.T, tx *sql.Tx, id int64) (hash string, previousHash sql.NullString) {
	t.Helper()

	require.NoError(t, tx.QueryRowContext(context.Background(),
		"SELECT hash, previous_hash FROM audit_events WHERE id = $1", id,
	).Scan(&hash, &previousHash), "read the stored digests")

	return hash, previousHash
}

// TestIntegration_AuditHashChain_RefusesResourceIDAbsorptionForgery is the
// security lock that matters most, because it is the one an unguarded two-formula
// verifier fails.
//
// The legacy five-field input is a strict prefix of the current nine-field input
// and resource_id is the last field they share, so appending the four actor
// values to resource_id makes the row's legacy digest equal the digest already
// stored for it. A verifier that computes the legacy digest unconditionally
// accepts that row, and actor identity has been rewritten on an append-only
// audit record with the stored hash unchanged — invisible to the verifier, to a
// published digest and to a write-once copy at the same time.
//
// The rewritten row must be reported invalid, and it must be the row named.
func TestIntegration_AuditHashChain_RefusesResourceIDAbsorptionForgery(t *testing.T) {
	tx := beginRolledBackTx(t)
	legacyChainFixture(t, tx, 3, 2, "res")

	// Establish the trail reads clean before the forgery, so a failure below
	// cannot be a pre-existing broken fixture.
	isValid, _, totalChecked := verifyFromGenesis(t, tx)
	require.True(t, isValid, "the fixture trail must verify before the forgery")
	require.Equal(t, int64(5), totalChecked, "the fixture trail must be walked in full before the forgery")

	var target int64
	require.NoError(t, tx.QueryRowContext(context.Background(),
		"SELECT max(id) FROM audit_events",
	).Scan(&target), "pick the newest current-formula row")

	hashBefore, prevBefore := storedDigests(t, tx, target)

	forgeAbsorption(t, tx, target, "forged-actor")

	hashAfter, prevAfter := storedDigests(t, tx, target)
	require.Equal(t, hashBefore, hashAfter,
		"the forgery must leave the stored hash untouched, otherwise it is not the shape under test")
	require.Equal(t, prevBefore, prevAfter,
		"the forgery must leave previous_hash untouched, otherwise it is not the shape under test")

	isValid, firstInvalidID, _ := verifyFromGenesis(t, tx)

	require.False(t, isValid,
		"absorbing the actor fields into resource_id MUST NOT verify: it rewrites actor identity on an append-only record while the stored hash stays byte-identical")
	require.True(t, firstInvalidID.Valid, "first_invalid_id MUST name the forged row")
	require.Equal(t, target, firstInvalidID.Int64, "the forged row MUST be the one flagged")
}

// TestIntegration_AuditHashChain_RefusesAbsorptionOnAHistoricalRow closes the
// same forgery against a pre-000017 row, where the legacy formula is the one
// that legitimately applies. Absorbing into resource_id changes a field the
// legacy digest covers, so the row stops matching either formula.
//
// This is a NEGATIVE CONTROL and passes with the separator guard removed — it
// pins nothing about the guard. It is kept because it does discriminate against
// a different wrong fix: one that gave pre-000017 rows a blanket pass, or that
// dropped resource_id from the legacy input.
func TestIntegration_AuditHashChain_RefusesAbsorptionOnAHistoricalRow(t *testing.T) {
	tx := beginRolledBackTx(t)
	legacyChainFixture(t, tx, 3, 2, "res")

	var target int64
	require.NoError(t, tx.QueryRowContext(context.Background(),
		"SELECT min(id) FROM audit_events",
	).Scan(&target), "pick the oldest historical row")

	forgeAbsorption(t, tx, target, "forged-actor")

	isValid, firstInvalidID, _ := verifyFromGenesis(t, tx)

	require.False(t, isValid, "absorbing into resource_id on a historical row MUST be reported invalid")
	require.True(t, firstInvalidID.Valid, "first_invalid_id MUST name the forged row")
	require.Equal(t, target, firstInvalidID.Int64, "the forged historical row MUST be the one flagged")
}

// TestIntegration_AuditHashChain_RefusesASleeperConversionOnACurrentRow is the
// security lock a content-based fallback cannot hold.
//
// The attack writes ONLY the newest row's hash column, setting it to that row's
// own legacy five-field digest. No other column moves — the actor identity is
// still exactly what the service recorded, so there is nothing anomalous in the
// row to detect. If the verifier accepts it, the row is now a legacy row: the
// service keeps appending, it becomes an ordinary middle row, and from then on
// its actor columns can be rewritten any number of times with NO write to any
// hash column anywhere. One digest change, made while the row's content was
// untouched, buys unlimited undetectable re-attribution afterwards.
//
// The row is above the recorded boundary, so the legacy formula is not on offer
// and the conversion is refused. Counting separators in the shared input does
// NOT refuse it: the input is a genuine five-field input with exactly four
// separators.
func TestIntegration_AuditHashChain_RefusesASleeperConversionOnACurrentRow(t *testing.T) {
	tx := beginRolledBackTx(t)
	legacyChainFixture(t, tx, 3, 2, "res")

	isValid, _, totalChecked := verifyFromGenesis(t, tx)
	require.True(t, isValid, "the fixture trail must verify before the conversion")
	require.Equal(t, int64(5), totalChecked, "the fixture trail must be walked in full before the conversion")

	var target int64
	require.NoError(t, tx.QueryRowContext(context.Background(),
		"SELECT max(id) FROM audit_events",
	).Scan(&target), "pick the newest current-formula row")
	require.Greater(t, target, readBoundary(t, tx),
		"the row under attack must sit ABOVE the recorded boundary, otherwise this is not the shape under test")

	convertToLegacyDigest(t, tx, target)

	isValid, firstInvalidID, _ := verifyFromGenesis(t, tx)

	require.False(t, isValid,
		"re-basing a post-re-baseline row onto its own legacy digest MUST be reported invalid; accepting it converts the row into one whose actor identity can be rewritten forever with no digest change")
	require.True(t, firstInvalidID.Valid, "first_invalid_id MUST name the converted row")
	require.Equal(t, target, firstInvalidID.Int64, "the converted row MUST be the one flagged")
}

// TestIntegration_AuditHashChain_RefusesASleeperThatBecameAMiddleRow closes the
// durable form of the same attack: convert the row while it is the chain head,
// let the service append normally, then rewrite the actor with no hash write at
// all. Every step must stay refused, including the appends, so an operator sees
// the alarm from the moment of conversion and it never clears itself.
func TestIntegration_AuditHashChain_RefusesASleeperThatBecameAMiddleRow(t *testing.T) {
	tx := beginRolledBackTx(t)
	legacyChainFixture(t, tx, 3, 2, "res")

	ctx := context.Background()

	var target int64
	require.NoError(t, tx.QueryRowContext(ctx,
		"SELECT max(id) FROM audit_events",
	).Scan(&target), "pick the newest current-formula row")

	convertToLegacyDigest(t, tx, target)

	isValid, _, _ := verifyFromGenesis(t, tx)
	require.False(t, isValid, "the conversion MUST be refused while the row is still the chain head")

	// Let the live trigger append two rows, which is what makes the converted row
	// an ordinary middle row in production.
	_, err := tx.ExecContext(ctx, `
		INSERT INTO audit_events (event_type, action, result, resource_id, resource_type,
		                          actor_type, actor_id, actor_name, actor_ip_address)
		SELECT 'LIMIT_UPDATED','UPDATE','SUCCESS','later-'||g,'limit','user','u'||g,'name'||g,'10.0.2.'||g
		FROM generate_series(1, 2) g`)
	require.NoError(t, err, "append two rows through the live trigger")

	isValid, firstInvalidID, _ := verifyFromGenesis(t, tx)
	require.False(t, isValid, "the conversion MUST stay refused once the row is a middle row")
	require.Equal(t, target, firstInvalidID.Int64, "the converted row MUST be the one flagged")

	// Rewrite the actor identity with no hash write. This is the payload the
	// conversion was for; it must not launder it.
	_, err = tx.ExecContext(ctx, `
		UPDATE audit_events
		   SET actor_type = 'system', actor_id = 'svc_tracer', actor_name = '', actor_ip_address = ''
		 WHERE id = $1`, target)
	require.NoError(t, err, "rewrite the actor identity on the converted row")

	isValid, firstInvalidID, _ = verifyFromGenesis(t, tx)
	require.False(t, isValid,
		"actor identity rewritten on a converted row MUST be reported invalid, with no hash write anywhere")
	require.Equal(t, target, firstInvalidID.Int64, "the rewritten row MUST be the one flagged")
}

// TestIntegration_AuditHashChain_RecordsTheReBaselineBoundaryAtUpgrade pins the
// rule the whole change rests on, in both directions.
//
// A deployment with historical rows records the highest historical id as its
// floor. A deployment with none records 0, so no row is ever offered the shorter
// formula and the trail is judged by the nine-field formula alone.
func TestIntegration_AuditHashChain_RecordsTheReBaselineBoundaryAtUpgrade(t *testing.T) {
	t.Run("historical rows present", func(t *testing.T) {
		tx := beginRolledBackTx(t)
		legacyChainFixture(t, tx, 3, 2, "res")

		var highestLegacy int64
		require.NoError(t, tx.QueryRowContext(context.Background(),
			"SELECT id FROM audit_events ORDER BY id ASC OFFSET 2 LIMIT 1",
		).Scan(&highestLegacy), "read the third (last historical) row")

		require.Equal(t, highestLegacy, readBoundary(t, tx),
			"the floor MUST be the highest id written under the pre-000017 formula")
	})

	t.Run("no historical rows", func(t *testing.T) {
		tx := beginRolledBackTx(t)
		legacyChainFixture(t, tx, 0, 4, "res")

		require.Equal(t, int64(0), readBoundary(t, tx),
			"a deployment with no pre-000017 data MUST record a floor of 0")

		isValid, _, totalChecked := verifyFromGenesis(t, tx)
		require.True(t, isValid, "an all-current trail MUST verify")
		require.Equal(t, int64(4), totalChecked, "an all-current trail MUST be walked in full")
	})
}

// TestIntegration_AuditHashChain_BoundaryIsAppendOnly pins that the floor is no
// easier to move than the trail it guards. Raising it would put rows written
// after the re-baseline back within reach of the shorter formula, so UPDATE and
// DELETE are refused the same way they are on audit_events.
func TestIntegration_AuditHashChain_BoundaryIsAppendOnly(t *testing.T) {
	tx := beginRolledBackTx(t)

	ctx := context.Background()
	before := readBoundary(t, tx)

	_, err := tx.ExecContext(ctx, "UPDATE audit_hash_legacy_boundary SET max_legacy_id = 999999")
	require.NoError(t, err, "the UPDATE is silently discarded by the rule, not an error")
	require.Equal(t, before, readBoundary(t, tx), "raising the floor MUST have no effect")

	_, err = tx.ExecContext(ctx, "DELETE FROM audit_hash_legacy_boundary")
	require.NoError(t, err, "the DELETE is silently discarded by the rule, not an error")
	require.Equal(t, before, readBoundary(t, tx), "removing the floor MUST have no effect")

	var rows int64
	require.NoError(t, tx.QueryRowContext(ctx,
		"SELECT count(*) FROM audit_hash_legacy_boundary").Scan(&rows))
	require.Equal(t, int64(1), rows, "the floor MUST remain a single row")
}

// TestIntegration_AuditHashChain_VerifiesALegacyRowWhoseResourceIDHoldsASeparator
// is the customer-visible half of the boundary rule.
//
// resource_id is VARCHAR(255) with no CHECK constraint, and NewAuditEvent only
// trims whitespace, so nothing stops a pre-000017 record from holding a '|'.
// Nothing ever did: RecordReservationExpiryBatch writes an RFC3339Nano
// timestamp there today, not an identifier, so "resource_id holds uuids" was
// never a property of the product.
//
// Under a separator-counting fallback such a record reports "tampered with" and
// the scan STOPS on it, which is the defect this migration set out to fix,
// reproduced for any deployment holding one such row. Under the boundary rule
// the row is below the floor, the five-field formula applies to it, and the
// whole trail is walked.
func TestIntegration_AuditHashChain_VerifiesALegacyRowWhoseResourceIDHoldsASeparator(t *testing.T) {
	tx := beginRolledBackTx(t)
	legacyChainFixture(t, tx, 3, 2, "res|with|separators")

	var resourceID string
	require.NoError(t, tx.QueryRowContext(context.Background(),
		"SELECT resource_id FROM audit_events ORDER BY id ASC LIMIT 1",
	).Scan(&resourceID), "read the oldest historical row")
	require.Contains(t, resourceID, "|",
		"the fixture must actually put a separator in the field both formulas share")

	isValid, firstInvalidID, totalChecked := verifyFromGenesis(t, tx)

	require.True(t, isValid,
		"a genuine pre-000017 record MUST verify whatever characters its resource_id holds; nothing constrains that column")
	require.False(t, firstInvalidID.Valid,
		"first_invalid_id MUST be NULL on a clean chain; got %d", firstInvalidID.Int64)
	require.Equal(t, int64(5), totalChecked,
		"the verifier MUST walk the whole chain; stopping on such a row leaves every later row unchecked")
}

// TestIntegration_AuditHashChain_RefusesTheLegacyFormulaAboveTheBoundary pins
// that the floor is a floor and not a blanket pass, on a trail that has rows on
// both sides of it.
//
// The row under attack carries a separator-bearing resource_id, so a
// separator-counting guard would refuse it for the wrong reason. It must be
// refused because of where it sits, not because of what it contains.
func TestIntegration_AuditHashChain_RefusesTheLegacyFormulaAboveTheBoundary(t *testing.T) {
	tx := beginRolledBackTx(t)
	legacyChainFixture(t, tx, 3, 2, "res|with|separators")

	ctx := context.Background()
	boundary := readBoundary(t, tx)
	require.Positive(t, boundary, "the fixture must record a non-zero floor")

	var target int64
	require.NoError(t, tx.QueryRowContext(ctx,
		"SELECT max(id) FROM audit_events").Scan(&target), "pick the newest row")
	require.Greater(t, target, boundary, "the row under attack must sit above the floor")

	_, err := tx.ExecContext(ctx,
		"UPDATE audit_events SET resource_id = 'post|with|separators' WHERE id = $1", target)
	require.NoError(t, err, "give the row above the floor a separator-bearing resource_id")

	convertToLegacyDigest(t, tx, target)

	isValid, firstInvalidID, _ := verifyFromGenesis(t, tx)

	require.False(t, isValid,
		"a row above the floor MUST be judged by the nine-field formula alone, whatever its resource_id holds")
	require.Equal(t, target, firstInvalidID.Int64, "the row above the floor MUST be the one flagged")
}
