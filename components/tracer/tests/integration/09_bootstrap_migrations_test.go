// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package integration

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	libPostgres "github.com/LerianStudio/lib-commons/v5/commons/postgres"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
)

// TestBootstrapAppliesAllMigrations verifies that bootstrap.InitServers()
// applies every migration in migrations/ via a single unified runner and that
// no legacy artifacts from the previous two-runner architecture remain. This
// test is the behavioral contract for the boot-time migration path.
//
// The global TestMain (see main_test.go) calls testutil_integration.SetupTestSuite,
// which invokes bootstrap.InitServers() against a fresh testcontainer — that is
// the boot path under test. We then connect directly to the container and assert
// the post-boot database state.
//
// Post-conditions enforced:
//  1. The three PostgreSQL functions that migrations 000001-000003 install
//     (calculate_audit_event_hash, verify_audit_hash_chain, prevent_truncate)
//     exist in pg_proc.
//  2. golang-migrate's schema_migrations table reports version = 17 and dirty = false.
//  3. The legacy tracking table schema_migrations_functions does NOT exist.
//     Either it was never created (single-runner layout) or migration 000016
//     dropped it.
//  4. The audit_events hash-chain BEFORE INSERT trigger is wired: inserting a
//     row without supplying a hash must yield a populated event_hash.
//  5. A second libPostgres.NewMigrator.Up on an already-migrated DB is a no-op
//     (idempotent replay invariant — second pillar of the Migration
//     Renumbering Invariant in docs/tracer/INVARIANTS.md).
func TestBootstrapAppliesAllMigrations(t *testing.T) {
	// Parent ctx bounds the non-migration work only (DB asserts, queries,
	// subtest cleanup). Each migrator.Up() call below derives its own
	// independent 5-minute budget from context.Background() so a slow Up
	// cannot eat into the time other sub-tests need for their assertions.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	db, err := sql.Open("pgx", testutil.GetTestDSN())
	require.NoError(t, err, "open db connection")

	t.Cleanup(func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("close db: %v", closeErr)
		}
	})

	require.NoError(t, db.PingContext(ctx), "ping db")

	// Sub-tests below intentionally do NOT call t.Parallel(): they share the
	// suite-wide testcontainer DB, and the Makefile already enforces -p=1 on
	// the integration suite. Parallelism here would only invite flakiness
	// without buying wall-clock wins.
	t.Run("required_functions_installed", func(t *testing.T) {
		requiredFunctions := []string{
			"calculate_audit_event_hash",
			"verify_audit_hash_chain",
			"prevent_truncate",
		}

		for _, fn := range requiredFunctions {
			var count int

			err := db.QueryRowContext(ctx,
				`SELECT COUNT(*)
				   FROM pg_proc p
				   JOIN pg_namespace n ON n.oid = p.pronamespace
				  WHERE n.nspname = 'public'
				    AND p.prokind = 'f'
				    AND p.proname = $1`, fn,
			).Scan(&count)

			require.NoError(t, err, "query pg_proc for %s", fn)
			require.GreaterOrEqual(t, count, 1,
				"function %s must be installed by migrations 000001-000003", fn)
		}
	})

	// Shared suite DB; no t.Parallel() — see file-level note above.
	t.Run("schema_migrations_reports_version_17", func(t *testing.T) {
		var (
			version int
			dirty   bool
		)

		// ORDER BY version DESC LIMIT 1 deterministically picks the highest
		// applied row; golang-migrate only ever has a single row at HEAD, but
		// ordering makes the query robust if a partial-history layout ever
		// appears (e.g. mid-migration failure leaving multiple rows).
		err := db.QueryRowContext(ctx,
			`SELECT version, dirty FROM schema_migrations ORDER BY version DESC LIMIT 1`,
		).Scan(&version, &dirty)

		require.NoError(t, err, "query schema_migrations")
		require.Equal(t, headVersion, version,
			"migration 017 must be the final applied version after unified boot")
		require.False(t, dirty, "schema_migrations must not be dirty after clean boot")
	})

	// Shared suite DB; no t.Parallel() — see file-level note above.
	t.Run("legacy_schema_migrations_functions_table_is_absent", func(t *testing.T) {
		var exists bool

		err := db.QueryRowContext(ctx,
			`SELECT EXISTS (
				SELECT 1 FROM information_schema.tables
				WHERE table_schema = 'public'
				  AND table_name = 'schema_migrations_functions'
			)`,
		).Scan(&exists)

		require.NoError(t, err, "query information_schema.tables")
		require.False(t, exists,
			"legacy tracking table schema_migrations_functions must not exist "+
				"(dropped by migration 000016 or never created under the unified runner)")
	})

	// Shared suite DB; no t.Parallel() — see file-level note above.
	t.Run("hash_chain_trigger_populates_event_hash_on_insert", func(t *testing.T) {
		// Insert a minimal audit_events row without supplying hash/previous_hash.
		// The BEFORE INSERT trigger must compute the SHA-256 hash.
		resourceID := testutil.MustDeterministicUUID(9001).String()

		_, err := db.ExecContext(ctx, `
			INSERT INTO audit_events (
				event_type, action, result,
				resource_id, resource_type,
				actor_type, actor_id, actor_name, actor_ip_address
			) VALUES (
				'TRANSACTION_VALIDATED', 'VALIDATE', 'ALLOW',
				$1, 'transaction',
				'system', 'bootstrap-migration-test', 'bootstrap-migration-test', '127.0.0.1'
			)
		`, resourceID)
		require.NoError(t, err, "insert audit_events row")

		t.Cleanup(func() {
			// audit_events has DELETE protection rule; use session_replication_role
			// to bypass it for this test-only cleanup.
			//
			// We pin the three statements to a single *sql.Conn so the replica
			// role flip applies to the DELETE that follows. Running them on the
			// pool would let the connection pool hand out up to three different
			// backend sessions, and session_replication_role is per-session —
			// SET on one session has no effect on DELETE running on another.
			conn, connErr := db.Conn(context.Background())
			if connErr != nil {
				t.Logf("acquire dedicated conn for cleanup: %v", connErr)
				return
			}

			defer func() {
				if closeErr := conn.Close(); closeErr != nil {
					t.Logf("close cleanup conn: %v", closeErr)
				}
			}()

			if _, execErr := conn.ExecContext(context.Background(),
				`SET session_replication_role = replica`,
			); execErr != nil {
				t.Logf("SET session_replication_role = replica: %v", execErr)
				return
			}

			if _, execErr := conn.ExecContext(context.Background(),
				`DELETE FROM audit_events WHERE resource_id = $1`, resourceID,
			); execErr != nil {
				t.Logf("DELETE audit_events resource_id=%s: %v", resourceID, execErr)
			}

			if _, execErr := conn.ExecContext(context.Background(),
				`SET session_replication_role = origin`,
			); execErr != nil {
				t.Logf("SET session_replication_role = origin: %v", execErr)
			}
		})

		var hash string

		err = db.QueryRowContext(ctx,
			`SELECT hash FROM audit_events WHERE resource_id = $1`, resourceID,
		).Scan(&hash)

		require.NoError(t, err, "select hash from audit_events")
		require.Regexp(t, `^[0-9a-f]{64}$`, hash,
			"calculate_audit_event_hash trigger must populate a SHA-256 hex hash")
	})

	// Shared suite DB; no t.Parallel() — see file-level note above.
	t.Run("idempotent_replay_on_already_migrated_db", func(t *testing.T) {
		// Second pillar of the Migration Renumbering Invariant (see
		// docs/tracer/INVARIANTS.md): a second Up() on a fully-migrated DB must be
		// a clean no-op. We build a brand-new libPostgres.NewMigrator against
		// the same suite DSN (simulating a process restart) and assert:
		//   - Up() returns nil
		//   - schema_migrations.version still = 17, dirty still = false
		//   - the count of functions created by migrations 000001..000003 is
		//     unchanged (proves no duplicate CREATE executed)
		migrator, migErr := libPostgres.NewMigrator(libPostgres.MigrationConfig{
			PrimaryDSN:           testutil.GetTestDSN(),
			DatabaseName:         "tracer_test",
			MigrationsPath:       resolveHeadMigrationsDir(ctx, t),
			AllowMultiStatements: false,
		})
		require.NoError(t, migErr, "build second migrator for idempotent replay")

		// Count the three migration-installed functions BEFORE re-running Up.
		const functionCountQuery = `
			SELECT COUNT(*)
			FROM pg_proc p
			JOIN pg_namespace n ON n.oid = p.pronamespace
			WHERE n.nspname = 'public'
			  AND p.prokind = 'f'
			  AND p.proname IN (
				'calculate_audit_event_hash',
				'verify_audit_hash_chain',
				'prevent_truncate'
			)
		`

		var functionCountBefore int

		err := db.QueryRowContext(ctx, functionCountQuery).Scan(&functionCountBefore)
		require.NoError(t, err, "count migration functions before replay")
		require.GreaterOrEqual(t, functionCountBefore, 3,
			"three hash-chain functions must be installed before replay")

		// Independent 5-minute budget derived from context.Background() so this
		// Up() gets the full production deadline regardless of how long prior
		// sub-tests consumed from the test-level ctx. Mirrors the production
		// bootstrap's migration budget (see internal/bootstrap/config.go).
		upCtx, upCancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer upCancel()

		require.NoError(t, migrator.Up(upCtx),
			"second Up() on already-migrated DB must succeed as a no-op")

		// Post-replay invariants: version/dirty untouched.
		var (
			postVersion int
			postDirty   bool
		)

		err = db.QueryRowContext(ctx,
			`SELECT version, dirty FROM schema_migrations ORDER BY version DESC LIMIT 1`,
		).Scan(&postVersion, &postDirty)
		require.NoError(t, err, "read schema_migrations after idempotent replay")
		require.Equal(t, headVersion, postVersion,
			"schema_migrations.version must remain 17 after idempotent replay")
		require.False(t, postDirty,
			"schema_migrations.dirty must remain false after idempotent replay")

		// No new functions created (nor old ones dropped-and-recreated into
		// duplicates — Postgres pg_proc would surface duplicates as distinct
		// rows).
		var functionCountAfter int

		err = db.QueryRowContext(ctx, functionCountQuery).Scan(&functionCountAfter)
		require.NoError(t, err, "count migration functions after replay")
		require.Equal(t, functionCountBefore, functionCountAfter,
			"idempotent replay must not change pg_proc function count "+
				"(before=%d after=%d)", functionCountBefore, functionCountAfter)
	})
}

// TestBootstrapMigrations_RefusesDirtyReapply guarantees that
// libPostgres.NewMigrator.Up refuses to run when schema_migrations is in a
// dirty state (dirty=true), rather than silently re-applying migrations.
//
// Why this is a SOX/GLBA audit guardrail:
// A migration that fails mid-apply leaves schema_migrations.dirty=true. If the
// next boot silently retried and "succeeded" without operator intervention,
// the audit trail would have a gap — an unacknowledged failure + recovery
// would be invisible in the compliance record. lib-commons/golang-migrate
// already refuse in this scenario; this test is a regression fence to keep
// that behaviour locked in.
//
// Test isolation:
// This test provisions its own Postgres testcontainer (reusing
// startUpgradePathContainer / resolveHeadMigrationsDir from
// 10_upgrade_path_test.go, same package). It deliberately does NOT reuse the
// shared SetupTestSuite container — toggling schema_migrations.dirty on that
// shared DB would contaminate every other test in the suite.
//
// Flow:
//  1. Boot a throwaway Postgres container.
//  2. Apply HEAD migrations cleanly via libPostgres.NewMigrator (version=17,
//     dirty=false).
//  3. Force dirty state: UPDATE schema_migrations SET dirty = true.
//  4. Build a brand-new libPostgres.NewMigrator (same DSN) to simulate a
//     process restart, and call Up.
//  5. Assert: Up returns a non-nil error that wraps
//     libPostgres.ErrMigrationDirty AND whose rendered message contains
//     "dirty" (case-insensitive). The `errors.Is` check is the strong
//     invariant (tied to lib-commons' classifyMigrationError); the substring
//     check is the weak, human-readable fence.
func TestBootstrapMigrations_RefusesDirtyReapply(t *testing.T) {
	// Parent ctx bounds non-migration work (container boot, DB asserts,
	// teardown). The two migrator.Up() calls below each derive their own
	// independent 5-minute budget from context.Background() — modelling two
	// distinct boot attempts, each entitled to the full production migration
	// SLA — so the clean-apply phase cannot shrink the retry phase's window.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// --- 1. Dedicated throwaway container (no contamination of shared DB) ----
	dsn := startUpgradePathContainer(ctx, t)

	// --- 2. Apply HEAD migrations cleanly -----------------------------------
	headMigrationsDir := resolveHeadMigrationsDir(ctx, t)

	migrator, err := libPostgres.NewMigrator(libPostgres.MigrationConfig{
		PrimaryDSN:           dsn,
		DatabaseName:         "tracer_test",
		MigrationsPath:       headMigrationsDir,
		AllowMultiStatements: false,
	})
	require.NoError(t, err, "build HEAD migrator")

	// Clean-apply boot: independent 5-minute budget from context.Background().
	// Not derived from the outer test ctx so this Up() cannot eat into the
	// retry Up()'s budget later. Matches the production bootstrap contract
	// (see internal/bootstrap/config.go).
	cleanUpCtx, cleanUpCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cleanUpCancel()

	require.NoError(t, migrator.Up(cleanUpCtx),
		"HEAD migrations must apply cleanly before we force the dirty flag")

	// Sanity: schema_migrations is clean at headVersion (constant from
	// 10_upgrade_path_test.go) before we poison it.
	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err, "open db for dirty-state poisoning")

	t.Cleanup(func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("close db: %v", closeErr)
		}
	})

	var (
		preVersion int
		preDirty   bool
	)

	err = db.QueryRowContext(ctx,
		`SELECT version, dirty FROM schema_migrations ORDER BY version DESC LIMIT 1`,
	).Scan(&preVersion, &preDirty)
	require.NoError(t, err, "read schema_migrations after clean apply")
	require.Equal(t, headVersion, preVersion,
		"clean HEAD apply must land at version %d", headVersion)
	require.False(t, preDirty, "clean HEAD apply must not be dirty")

	// --- 3. Force dirty state ------------------------------------------------
	//
	// A real dirty row in production happens when a migration statement fails
	// mid-apply. Rather than contriving a failing migration (fragile; couples
	// the test to SQL implementation details), we directly flip the tracking
	// flag — which is exactly the state golang-migrate would have recorded.
	result, err := db.ExecContext(ctx,
		`UPDATE schema_migrations SET dirty = true WHERE version = $1`, headVersion,
	)
	require.NoError(t, err, "force dirty flag on schema_migrations")

	rowsAffected, err := result.RowsAffected()
	require.NoError(t, err, "RowsAffected after poisoning dirty flag")
	require.Equal(t, int64(1), rowsAffected,
		"expected exactly one schema_migrations row to flip to dirty")

	// --- 4. Fresh migrator (simulates process restart) must REFUSE ----------
	retryMigrator, err := libPostgres.NewMigrator(libPostgres.MigrationConfig{
		PrimaryDSN:           dsn,
		DatabaseName:         "tracer_test",
		MigrationsPath:       headMigrationsDir,
		AllowMultiStatements: false,
	})
	require.NoError(t, err, "build retry migrator")

	// Retry boot: independent 5-minute budget from context.Background() —
	// models a fresh process restart after the clean apply and is entitled
	// to the full production migration SLA regardless of what the clean-apply
	// phase consumed. Expected to return ErrMigrationDirty well under the
	// deadline; the symmetric budget is the point.
	retryUpCtx, retryUpCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer retryUpCancel()

	upErr := retryMigrator.Up(retryUpCtx)

	// --- 5. Assertions -------------------------------------------------------
	require.Error(t, upErr,
		"Up must refuse to re-apply over a dirty schema_migrations row — "+
			"silent recovery would leave a gap in the SOX/GLBA audit trail")

	// Strong invariant: the error MUST wrap lib-commons' ErrMigrationDirty
	// sentinel. classifyMigrationError (see lib-commons/commons/postgres)
	// performs `errors.As(err, &migrate.ErrDirty{})` and re-wraps with
	// ErrMigrationDirty, so callers can reliably switch on it.
	require.True(t, errors.Is(upErr, libPostgres.ErrMigrationDirty),
		"Up error must wrap libPostgres.ErrMigrationDirty; got: %v", upErr)

	// Weak, human-readable fence: the rendered error text contains "dirty".
	// Protects against a future lib-commons refactor that silently drops the
	// word from the error message.
	require.Contains(t, strings.ToLower(upErr.Error()), "dirty",
		"Up error message must mention 'dirty' so operators can triage; got: %s",
		upErr.Error())

	// Post-condition: the dirty flag must still be set (Up did not "fix" it
	// by silently re-applying). Operator must explicitly resolve.
	var postDirty bool

	err = db.QueryRowContext(ctx,
		`SELECT dirty FROM schema_migrations WHERE version = $1`, headVersion,
	).Scan(&postDirty)
	require.NoError(t, err, "re-read schema_migrations.dirty after refused Up")
	require.True(t, postDirty,
		"dirty flag must remain set after refused Up — requires manual operator intervention")
}
