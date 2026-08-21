// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package integration

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
)

// TestIntegration_AuditHashChain is the behavioral contract for migration
// 000023_audit_id_under_lock.
//
// THE DEFECT (pre-000023):
//
//	audit_events.id is BIGSERIAL. Its nextval is evaluated at INSERT
//	default-expansion — BEFORE the BEFORE-INSERT trigger
//	calculate_audit_event_hash() acquires pg_advisory_xact_lock(314159265)
//	and reads the predecessor by max committed id. A lower-id transaction can
//	therefore stall between "id materialized" and "critical section entered"
//	while a higher-id transaction commits first; the lower-id row then links
//	to the higher row's hash. The ascending-id verifier
//	(verify_audit_hash_chain) walks id ASC and flags the lower id — the field
//	symptom firstInvalidId 1553.
//
// THE FIX (000023):
//
//	Drop the BIGSERIAL DEFAULT (keep the sequence) and assign
//	NEW.id := nextval('audit_events_id_seq') INSIDE the advisory-locked region,
//	before reading the predecessor. The row that reads predecessor P is then
//	guaranteed the next id after P: id-order == hash-chain order, so the
//	ascending-id verifier can never observe a fork.
//
// Each sub-test provisions its OWN throwaway Postgres container (via
// startUpgradePathContainer from 10_upgrade_path_test.go) so schema state never
// leaks between cases. newHeadReservationMigrate (21_reservation_amounts_decimal
// _test.go) supplies the HEAD-bound golang-migrate instance + dedicated *sql.DB.
func TestIntegration_AuditHashChain(t *testing.T) {
	// Sub-tests intentionally do NOT call t.Parallel(): the integration Makefile
	// enforces -p=1 and each case drives its own container to completion.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	tests := []struct {
		name string
		run  func(t *testing.T, mig *migrate.Migrate, db *sql.DB, dsn string)
	}{
		{
			// GREEN: the core contract. Many concurrent inserters against the
			// migrated (000023) schema produce a chain the ascending-id verifier
			// accepts — no fork.
			name: "concurrent_inserts_do_not_fork_the_chain",
			run: func(t *testing.T, mig *migrate.Migrate, db *sql.DB, dsn string) {
				require.NoError(t, migrateAuditChainUp(mig), "apply migrations up to 000023")

				const workers = 64

				pool, err := sql.Open("pgx", dsn)
				require.NoError(t, err, "open workload pool")
				t.Cleanup(func() { _ = pool.Close() })
				pool.SetMaxOpenConns(24)

				grp, gctx := errgroup.WithContext(ctx)
				for i := 0; i < workers; i++ {
					resourceID := fmt.Sprintf("rule-concurrent-%03d", i)
					grp.Go(func() error {
						return auditChainInsertRule(gctx, pool, resourceID)
					})
				}
				require.NoError(t, grp.Wait(), "all concurrent inserts must succeed")

				require.Equal(t, int64(workers), auditChainRowCount(ctx, t, db),
					"every concurrent insert must have persisted exactly one row")

				valid, firstInvalid, total, detail := auditChainVerify(ctx, t, db)
				require.True(t, valid,
					"chain must verify after %d concurrent inserts (firstInvalidId=%v, detail=%q)",
					workers, firstInvalid, detail.String)
				require.False(t, firstInvalid.Valid, "no row may be flagged invalid")
				require.Equal(t, int64(workers), total, "verifier must walk every row")
			},
		},
		{
			// GREEN discriminator: with id assigned INSIDE the lock, forcing the
			// pre-fix interleave is impossible. A controller holds the real audit
			// lock; two inserts block BEFORE they can assign an id; whichever the
			// lock queue serves first also takes the lower id. id-order therefore
			// tracks commit-order regardless of which goroutine started first.
			name: "id_assigned_inside_lock_tracks_commit_order",
			run: func(t *testing.T, mig *migrate.Migrate, db *sql.DB, dsn string) {
				require.NoError(t, migrateAuditChainUp(mig), "apply migrations up to 000023")

				// Genesis row so the pre-lock predecessor read is non-empty.
				require.NoError(t, auditChainInsertRule(ctx, db, "rule-genesis"), "seed genesis row")

				control, err := db.Conn(ctx)
				require.NoError(t, err, "grab controller connection")
				defer control.Close()

				// Hold the real audit lock so both inserts park before the trigger
				// can reach nextval (000023 assigns id after this lock).
				_, err = control.ExecContext(ctx, "SELECT pg_advisory_lock(314159265)")
				require.NoError(t, err, "controller takes audit lock")

				pool, err := sql.Open("pgx", dsn)
				require.NoError(t, err, "open ordering pool")
				t.Cleanup(func() { _ = pool.Close() })
				pool.SetMaxOpenConns(4)

				var wg sync.WaitGroup
				wg.Add(2)
				errs := make([]error, 2)
				for i, rid := range []string{"rule-order-first", "rule-order-second"} {
					go func() {
						defer wg.Done()
						errs[i] = auditChainInsertRule(ctx, pool, rid)
					}()
				}

				// Both inserters must be parked on the audit lock before release.
				waitForBlockedAdvisoryLocks(ctx, t, db, 314159265, 2)

				_, err = control.ExecContext(ctx, "SELECT pg_advisory_unlock(314159265)")
				require.NoError(t, err, "controller releases audit lock")
				wg.Wait()
				require.NoError(t, errs[0], "first ordered insert")
				require.NoError(t, errs[1], "second ordered insert")

				valid, firstInvalid, _, detail := auditChainVerify(ctx, t, db)
				require.True(t, valid,
					"serialized inserts must verify (firstInvalidId=%v, detail=%q)", firstInvalid, detail.String)

				// Discriminating link assertion for the ordering property this case
				// names: with id assigned INSIDE the lock the three rows form one
				// contiguous chain — genesis <- lower <- higher — regardless of which
				// goroutine started first. The verifier walking id ASC already accepts
				// the chain, but assert the two racing rows' links directly so the case
				// fails loudly if id-order ever diverged from chain-order.
				genesisHash := auditChainSingleHash(ctx, t, db, "rule-genesis")

				rows, err := db.QueryContext(ctx,
					`SELECT hash, previous_hash FROM audit_events
					 WHERE resource_id IN ('rule-order-first', 'rule-order-second')
					 ORDER BY id ASC`)
				require.NoError(t, err, "read the two racing rows in id order")
				defer rows.Close()

				type racingRow struct {
					hash     string
					prevHash sql.NullString
				}

				var racing []racingRow
				for rows.Next() {
					var r racingRow
					require.NoError(t, rows.Scan(&r.hash, &r.prevHash), "scan racing row")
					racing = append(racing, r)
				}
				require.NoError(t, rows.Err(), "iterate racing rows")
				require.Len(t, racing, 2, "both racing rows must have committed")

				lower, higher := racing[0], racing[1]
				require.True(t, lower.prevHash.Valid, "lower-id racing row must carry a previous_hash")
				require.Equal(t, genesisHash, lower.prevHash.String,
					"the lower-id racing row must link to the genesis row's hash")
				require.True(t, higher.prevHash.Valid, "higher-id racing row must carry a previous_hash")
				require.Equal(t, lower.hash, higher.prevHash.String,
					"the higher-id racing row must link to the lower-id racing row's hash")
			},
		},
		{
			name: "migration_up_down_up_is_idempotent_and_preserves_id_type",
			run: func(t *testing.T, mig *migrate.Migrate, db *sql.DB, _ string) {
				require.NoError(t, migrateAuditChainUp(mig), "apply up to 000023")

				// id stays bigint after the DEFAULT is dropped; the default is gone.
				require.Equal(t, "bigint", auditColumnType(ctx, t, db, "audit_events", "id"),
					"000023 must preserve the bigint type on audit_events.id")
				require.False(t, auditColumnHasDefault(ctx, t, db, "audit_events", "id"),
					"000023 must drop the BIGSERIAL DEFAULT on audit_events.id")

				require.NoError(t, mig.Steps(-1), "step down 000023")
				require.Equal(t, "bigint", auditColumnType(ctx, t, db, "audit_events", "id"),
					"down must keep bigint on audit_events.id")
				require.True(t, auditColumnHasDefault(ctx, t, db, "audit_events", "id"),
					"down must restore the BIGSERIAL DEFAULT on audit_events.id")

				require.NoError(t, migrateAuditChainUp(mig), "re-apply up to 000023 (idempotent cycle)")
				require.False(t, auditColumnHasDefault(ctx, t, db, "audit_events", "id"),
					"re-applied 000023 must drop the DEFAULT again")

				// Inserts still work and verify after the full cycle.
				require.NoError(t, auditChainInsertRule(ctx, db, "rule-after-cycle-1"), "insert after cycle")
				require.NoError(t, auditChainInsertRule(ctx, db, "rule-after-cycle-2"), "insert after cycle")
				valid, _, _, detail := auditChainVerify(ctx, t, db)
				require.True(t, valid, "chain must verify after up/down/up (detail=%q)", detail.String)
			},
		},
		{
			name: "anti_tamper_rules_still_reject_update_delete_truncate",
			run: func(t *testing.T, mig *migrate.Migrate, db *sql.DB, _ string) {
				require.NoError(t, migrateAuditChainUp(mig), "apply up to 000023")
				require.NoError(t, auditChainInsertRule(ctx, db, "rule-immutable"), "seed immutable row")

				origHash := auditChainSingleHash(ctx, t, db, "rule-immutable")

				// UPDATE is silently swallowed by the DO INSTEAD NOTHING rule.
				res, err := db.ExecContext(ctx,
					"UPDATE audit_events SET actor_name = 'tampered' WHERE resource_id = $1", "rule-immutable")
				require.NoError(t, err, "UPDATE must not error (rule turns it into a no-op)")
				affected, _ := res.RowsAffected()
				require.Equal(t, int64(0), affected, "UPDATE must affect zero rows")
				require.Equal(t, origHash, auditChainSingleHash(ctx, t, db, "rule-immutable"),
					"row must be byte-identical after attempted UPDATE")

				// DELETE is likewise a no-op.
				res, err = db.ExecContext(ctx,
					"DELETE FROM audit_events WHERE resource_id = $1", "rule-immutable")
				require.NoError(t, err, "DELETE must not error (rule turns it into a no-op)")
				affected, _ = res.RowsAffected()
				require.Equal(t, int64(0), affected, "DELETE must affect zero rows")
				require.Equal(t, int64(1), auditChainRowCount(ctx, t, db), "row must survive attempted DELETE")

				// TRUNCATE is blocked hard by the BEFORE TRUNCATE trigger.
				_, err = db.ExecContext(ctx, "TRUNCATE audit_events")
				require.Error(t, err, "TRUNCATE must be rejected by the SOX/GLBA guard")
			},
		},
		{
			name: "sequential_baseline_still_verifies",
			run: func(t *testing.T, mig *migrate.Migrate, db *sql.DB, _ string) {
				require.NoError(t, migrateAuditChainUp(mig), "apply up to 000023")
				for i := 0; i < 10; i++ {
					require.NoError(t, auditChainInsertRule(ctx, db, fmt.Sprintf("rule-seq-%02d", i)),
						"sequential insert %d", i)
				}
				valid, firstInvalid, total, detail := auditChainVerify(ctx, t, db)
				require.True(t, valid, "sequential chain must verify (firstInvalidId=%v, detail=%q)",
					firstInvalid, detail.String)
				require.Equal(t, int64(10), total, "verifier must walk all sequential rows")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dsn := startUpgradePathContainer(ctx, t)
			mig, db := newHeadReservationMigrate(ctx, t, dsn)
			tt.run(t, mig, db, dsn)
		})
	}
}

// TestIntegration_AuditHashChain_PreFixForkReproduction is the RED-equivalent.
//
// It pins the schema at version 000022 (pre-fix, id from BIGSERIAL DEFAULT) and
// replaces calculate_audit_event_hash() with a variant that is byte-identical to
// the production 000017 formula EXCEPT it blocks on a per-id TEST gate
// (pg_advisory_xact_lock(NEW.id)) at the top — i.e. AFTER the id has been
// materialized by the DEFAULT but BEFORE the real critical section. That gate
// makes the pre-000023 nextval->lock window (normally a few instructions,
// impossible to hit deterministically) fully controllable, so the fork the
// field saw at scale is reproduced on demand.
//
// It drives the exact interleave: genesis id=1 commits; id=2 (txn A) parks; id=3
// (txn B) parks; B is released first and links to id=1; A is released and links
// to the now-committed id=3. Row 2 (lower id) ends up chained to row 3 (higher
// id). The unmodified ascending-id verifier then rejects the chain at id=2.
//
// This is the state 000023 makes unreachable. It runs against the pre-fix
// trigger only; the GREEN cases above prove 000023 prevents it.
func TestIntegration_AuditHashChain_PreFixForkReproduction(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	dsn := startUpgradePathContainer(ctx, t)
	mig, db := newHeadReservationMigrate(ctx, t, dsn)

	// Pin at 000022: pre-fix schema where id comes from the BIGSERIAL DEFAULT.
	require.NoError(t, mig.Migrate(22), "migrate up to pre-fix version 000022")
	installGatedPreFixTrigger(ctx, t, db)

	// Genesis (id=1). Key 1 is never gated, so this proceeds immediately.
	require.NoError(t, auditChainInsertRule(ctx, db, "fork-genesis"), "seed genesis row (id=1)")

	// Controller session takes the per-id gates for the two racing rows.
	control, err := db.Conn(ctx)
	require.NoError(t, err, "grab controller connection")
	defer control.Close()
	_, err = control.ExecContext(ctx, "SELECT pg_advisory_lock(2), pg_advisory_lock(3)")
	require.NoError(t, err, "controller takes gates for ids 2 and 3")

	poolA, err := sql.Open("pgx", dsn)
	require.NoError(t, err, "open A pool")
	defer poolA.Close()
	poolB, err := sql.Open("pgx", dsn)
	require.NoError(t, err, "open B pool")
	defer poolB.Close()

	// Txn A -> id=2, parks on gate(2). Txn B -> id=3, parks on gate(3).
	aErr := make(chan error, 1)
	bErr := make(chan error, 1)
	go func() { aErr <- auditChainInsertRule(ctx, poolA, "fork-a") }()
	waitForBlockedAdvisoryLocks(ctx, t, db, 2, 1) // A holds id=2, blocked on gate(2)
	go func() { bErr <- auditChainInsertRule(ctx, poolB, "fork-b") }()
	waitForBlockedAdvisoryLocks(ctx, t, db, 3, 1) // B holds id=3, blocked on gate(3)

	// Release B (id=3) first: it links to the max committed id (1).
	_, err = control.ExecContext(ctx, "SELECT pg_advisory_unlock(3)")
	require.NoError(t, err, "release gate(3)")
	require.NoError(t, <-bErr, "txn B (id=3) must commit")

	// Release A (id=2): it now links to the freshly committed id=3.
	_, err = control.ExecContext(ctx, "SELECT pg_advisory_unlock(2)")
	require.NoError(t, err, "release gate(2)")
	require.NoError(t, <-aErr, "txn A (id=2) must commit")

	// The verifier rejects the fork: id 2 links to id 3's hash.
	valid, firstInvalid, _, detail := auditChainVerify(ctx, t, db)
	require.False(t, valid, "pre-fix trigger must produce a chain the verifier rejects")
	require.True(t, firstInvalid.Valid, "verifier must name the forked id")
	require.Equal(t, int64(2), firstInvalid.Int64,
		"the lower id (2) must be flagged for linking to the higher id (3); detail=%q", detail.String)
}

// auditChainHeadVersion is the migration this file is the contract for
// (000023_audit_id_under_lock). Migrating to exactly this version keeps the
// assertions stable if later migrations layer on top.
const auditChainHeadVersion = 23

// migrateAuditChainUp migrates up to auditChainHeadVersion, treating
// migrate.ErrNoChange as success so a re-apply is a clean no-op.
func migrateAuditChainUp(mig *migrate.Migrate) error {
	if err := mig.Migrate(auditChainHeadVersion); err != nil && err != migrate.ErrNoChange {
		return err
	}

	return nil
}

// auditChainInsertRule inserts one RULE_CREATED audit event through the real
// BEFORE-INSERT trigger, letting id/created_at/event_id/context/metadata take
// their schema defaults. Exercised concurrently and sequentially.
func auditChainInsertRule(ctx context.Context, db interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}, resourceID string,
) error {
	_, err := db.ExecContext(ctx,
		`INSERT INTO audit_events
			(event_type, action, result, resource_id, resource_type,
			 actor_type, actor_id, actor_name, actor_ip_address)
		 VALUES ('RULE_CREATED', 'CREATE', 'SUCCESS', $1, 'rule',
			 'system', 'svc_tracer', 'Tracer Service', '127.0.0.1')`,
		resourceID)
	if err != nil {
		return fmt.Errorf("insert audit event %q: %w", resourceID, err)
	}

	return nil
}

// auditChainVerify runs the production ascending-id verifier over the whole
// chain and returns its four result columns.
func auditChainVerify(ctx context.Context, t *testing.T, db *sql.DB) (bool, sql.NullInt64, int64, sql.NullString) {
	t.Helper()

	var (
		valid        bool
		firstInvalid sql.NullInt64
		total        int64
		detail       sql.NullString
	)

	err := db.QueryRowContext(ctx,
		"SELECT is_valid, first_invalid_id, total_checked, error_detail FROM verify_audit_hash_chain(1, NULL)").
		Scan(&valid, &firstInvalid, &total, &detail)
	require.NoError(t, err, "call verify_audit_hash_chain")

	return valid, firstInvalid, total, detail
}

// auditChainRowCount returns the number of rows in audit_events.
func auditChainRowCount(ctx context.Context, t *testing.T, db *sql.DB) int64 {
	t.Helper()

	var n int64

	require.NoError(t, db.QueryRowContext(ctx, "SELECT count(*) FROM audit_events").Scan(&n),
		"count audit_events")

	return n
}

// auditChainSingleHash returns the stored hash of the single row with the given
// resource_id.
func auditChainSingleHash(ctx context.Context, t *testing.T, db *sql.DB, resourceID string) string {
	t.Helper()

	var hash string

	require.NoError(t, db.QueryRowContext(ctx,
		"SELECT hash FROM audit_events WHERE resource_id = $1", resourceID).Scan(&hash),
		"read hash for %s", resourceID)

	return hash
}

// auditColumnType returns the information_schema data_type of table.column.
func auditColumnType(ctx context.Context, t *testing.T, db *sql.DB, table, column string) string {
	t.Helper()

	var dataType string

	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT data_type FROM information_schema.columns
		 WHERE table_schema = 'public' AND table_name = $1 AND column_name = $2`,
		table, column).Scan(&dataType),
		"lookup %s.%s data_type", table, column)

	return dataType
}

// auditColumnHasDefault reports whether table.column has a non-null column
// default in information_schema (true while the BIGSERIAL DEFAULT is present).
func auditColumnHasDefault(ctx context.Context, t *testing.T, db *sql.DB, table, column string) bool {
	t.Helper()

	var def sql.NullString

	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT column_default FROM information_schema.columns
		 WHERE table_schema = 'public' AND table_name = $1 AND column_name = $2`,
		table, column).Scan(&def),
		"lookup %s.%s column_default", table, column)

	return def.Valid
}

// waitForBlockedAdvisoryLocks blocks until at least want backends are waiting
// (granted=false) on the single-argument advisory lock keyed by key. For a
// single-arg bigint key k, pg_locks records classid = k>>32, objid = k&0xFFFFFFFF,
// objsubid = 1.
func waitForBlockedAdvisoryLocks(ctx context.Context, t *testing.T, db *sql.DB, key int64, want int) {
	t.Helper()

	classID := int64(uint64(key) >> 32)
	objID := int64(uint64(key) & 0xFFFFFFFF)

	waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	for {
		var blocked int

		err := db.QueryRowContext(waitCtx,
			`SELECT count(*) FROM pg_locks
			 WHERE locktype = 'advisory' AND NOT granted
			   AND classid = $1 AND objid = $2 AND objsubid = 1`,
			classID, objID).Scan(&blocked)
		if err == nil && blocked >= want {
			return
		}

		// Check the deadline before the query error so a timeout reports the
		// waiting-for message (with the last count seen) rather than a bare
		// context-cancelled scan error.
		require.NoError(t, waitCtx.Err(),
			"timed out waiting for %d backend(s) blocked on advisory key %d (saw %d)", want, key, blocked)
		require.NoError(t, err, "poll pg_locks for advisory key %d", key)

		time.Sleep(25 * time.Millisecond)
	}
}

// installGatedPreFixTrigger replaces calculate_audit_event_hash() with a
// TEST-ONLY variant that reproduces the pre-000023 ordering (id from the
// BIGSERIAL DEFAULT, already assigned when this BEFORE-INSERT trigger runs) and
// adds a per-id advisory gate at the top so the nextval->critical-section window
// can be driven deterministically. The hash formula is byte-identical to
// production 000017.
func installGatedPreFixTrigger(ctx context.Context, t *testing.T, db *sql.DB) {
	t.Helper()

	_, err := db.ExecContext(ctx, `
CREATE OR REPLACE FUNCTION calculate_audit_event_hash()
RETURNS TRIGGER AS $$
DECLARE
    prev_hash VARCHAR(64);
    hash_input TEXT;
BEGIN
    -- TEST-ONLY per-id gate. NEW.id is ALREADY set here (pre-000023 fills id
    -- from the BIGSERIAL DEFAULT before this trigger fires), so blocking here
    -- reproduces the pre-fix window: id materialized, critical section not yet
    -- entered.
    PERFORM pg_advisory_xact_lock(NEW.id);

    -- Real audit lock (unchanged from 000017).
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
`)
	require.NoError(t, err, "install gated pre-fix trigger")
}
