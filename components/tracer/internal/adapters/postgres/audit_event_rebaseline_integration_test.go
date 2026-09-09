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
// verify_audit_hash_chain now accepts a row under either formula. These tests
// lock the three properties that matter: the historical trail verifies, and
// tampering is still caught on both sides of the boundary.
// =============================================================================

// legacyChainFixture rebuilds an upgraded deployment's audit table inside tx:
// legacyRows rows carrying pre-000017 five-field hashes, followed by
// currentRows rows written by the live trigger, which chains onto the last
// historical hash exactly as it would in production.
//
// The append-only rules are disabled for the duration of tx (ALTER TABLE is
// transactional in PostgreSQL, so the rollback restores them) because forging a
// historical row is the only way to reproduce a pre-migration hash: the trigger
// always overwrites NEW.hash with the current formula.
func legacyChainFixture(t *testing.T, tx *sql.Tx, legacyRows, currentRows int) {
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
		SELECT 'LIMIT_CREATED','CREATE','SUCCESS','res-'||g,'limit','user','u'||g,'name'||g,'10.0.0.'||g
		FROM generate_series(1, $1) g`, legacyRows)
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

	_, err = tx.ExecContext(ctx, `
		INSERT INTO audit_events (event_type, action, result, resource_id, resource_type,
		                          actor_type, actor_id, actor_name, actor_ip_address)
		SELECT 'LIMIT_UPDATED','UPDATE','SUCCESS','post-'||g,'limit','api_key','k'||g,'kname'||g,'10.0.1.'||g
		FROM generate_series(1, $1) g`, currentRows)
	require.NoError(t, err, "insert the rows written by the current trigger")
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
	legacyChainFixture(t, tx, 3, 2)

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
	legacyChainFixture(t, tx, 3, 2)

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
	legacyChainFixture(t, tx, 3, 2)

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
	legacyChainFixture(t, tx, 3, 2)

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
	legacyChainFixture(t, tx, 3, 2)

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

// TestIntegration_AuditHashChain_LegacyFallbackIsRefusedForASeparatorBearingResourceID
// pins the accepted cost of the separator guard, so the behaviour is a decision
// on record rather than a surprise in the field.
//
// The guard withholds the legacy fallback from any row whose shared fields carry
// the '|' separator, because such an input is ambiguous with a longer formula.
// A pre-000017 row whose resource_id genuinely contains a separator therefore
// reports invalid. resource_id holds entity identifiers — uuids and the like —
// so this is not a shape the product writes; the guard fails closed rather than
// widening what the fallback accepts.
func TestIntegration_AuditHashChain_LegacyFallbackIsRefusedForASeparatorBearingResourceID(t *testing.T) {
	tx := beginRolledBackTx(t)
	legacyChainFixture(t, tx, 1, 0)

	ctx := context.Background()

	var target int64
	require.NoError(t, tx.QueryRowContext(ctx,
		"SELECT min(id) FROM audit_events",
	).Scan(&target), "pick the historical row")

	// Rewrite resource_id to carry a separator AND re-hash it under the legacy
	// formula, so the row is genuinely consistent with the legacy formula and the
	// only reason it can be refused is the separator guard.
	_, err := tx.ExecContext(ctx, `
		UPDATE audit_events SET resource_id = 'res|with|separators' WHERE id = $1`, target)
	require.NoError(t, err, "give the historical row a separator-bearing resource_id")

	_, err = tx.ExecContext(ctx, `
		UPDATE audit_events SET hash = encode(sha256((
		      COALESCE(previous_hash, 'GENESIS')
		   || '|' || event_id::text
		   || '|' || event_type
		   || '|' || to_char(created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US"Z"')
		   || '|' || resource_id)::bytea),'hex')
		WHERE id = $1`, target)
	require.NoError(t, err, "re-hash the row under the legacy formula")

	isValid, firstInvalidID, _ := verifyFromGenesis(t, tx)

	require.False(t, isValid,
		"the legacy fallback is deliberately withheld from a row whose shared fields carry the separator; this pins that accepted cost")
	require.True(t, firstInvalidID.Valid, "first_invalid_id MUST name the row")
	require.Equal(t, target, firstInvalidID.Int64, "the separator-bearing row MUST be the one flagged")
}
