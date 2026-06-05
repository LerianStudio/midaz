// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package integration

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	libPostgres "github.com/LerianStudio/lib-commons/v5/commons/postgres"
	"github.com/golang-migrate/migrate/v4"
	migratepostgres "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil"
	"github.com/LerianStudio/midaz/v4/components/tracer/internal/testutil_integration"
)

// legacyHeadVersion is the highest schema_migrations.version present on the
// pinned pre-refactor develop tip (legacyDevelopRef below): 12 schema
// migrations + 3 function migrations tracked in a separate table.
const legacyHeadVersion = 12

// headVersion is the expected final schema_migrations.version after applying
// the HEAD migrations (unified single-runner, 001..016).
const headVersion = 17

// legacyDevelopRef is the immutable commit representing the last state of
// origin/develop before the unify-sql-migrations feature branched. Pinned
// so this test keeps validating the upgrade path from the true dual-runner
// layout, even after this PR merges and develop advances.
//
// To update (only if a new renumbering feature lands): re-pin to the
// merge-base of the new branch and the pre-refactor develop tip.
const legacyDevelopRef = "0a77ac3e4945db1846626aab91f9899079877365"

// TestUpgradePath_FromDevelopToHead verifies that a production database at
// the migration sequence in the pinned legacyDevelopRef (schema_migrations.
// version = 12) can be upgraded in place to the HEAD sequence (unified
// single-runner) without corruption.
//
// This is the behavioral proof of the invariant codified in docs/PROJECT_RULES.md
// ("Migration Renumbering Invariant"). It simulates:
//
//  1. A fresh container boot with migrations from the pinned legacy commit
//     (dual-runner layout: `migrations/functions/` + numbered schema
//     migrations 001..012, tracked in `schema_migrations_functions` +
//     `schema_migrations`).
//  2. In-place upgrade to HEAD migrations (unified single-runner, 001..016)
//     using the exact same boot runner production will use (libPostgres.Migrator).
//  3. Assertions that the final state matches a fresh install: version=16,
//     legacy tracking table dropped, hash-chain functions installed, audit
//     trigger operational.
//
// See TestUpgradePath_FromMultipleLegacyVersions for the parametrized cousin
// that exercises the same upgrade from earlier legacy versions.
//
// The test spins up its own Postgres testcontainer (independent of the shared
// SetupTestSuite container) and shells out to `git archive <legacyDevelopRef>`
// to materialize the legacy migration set. It runs unconditionally in the
// integration suite so that any future regression to the Migration
// Renumbering Invariant (docs/PROJECT_RULES.md) is caught in CI.
//
// The test calls t.Skipf with an actionable message when legacyDevelopRef is
// not fetched locally (e.g. a shallow clone), rather than failing with a
// cryptic git archive error. CI that runs this test must configure
// actions/checkout with fetch-depth: 0 (or fetch the specific SHA).
func TestUpgradePath_FromDevelopToHead(t *testing.T) {
	runUpgradePathScenario(t, legacyHeadVersion)
}

// TestUpgradePath_FromMultipleLegacyVersions parametrizes the develop→HEAD
// upgrade across several schema_migrations.version starting points on the
// pinned pre-refactor legacy layout (legacyDevelopRef).
//
// Rationale: the Migration Renumbering Invariant must hold not only from the
// latest develop version (12) but also from any strictly earlier version, so
// databases that upgraded to develop later in its lifecycle are covered too.
// Each case picks a legacy version that stress-tests a different subset of
// the renumbered HEAD migrations:
//
//   - 3  — only legacy 1-3 applied (initial_schema, convert_cents,
//     draft_audit_enums). HEAD replays files 4..16, which re-executes
//     the entire renumbered initial_schema + convert_cents path. Proves
//     the CREATE TYPE and ALTER ... TYPE DECIMAL guards added to
//     HEAD 000004 and 000005 hold.
//   - 7  — legacy 1-7 applied; HEAD replays 8..16. Exercises the ADD
//     CONSTRAINT idempotency guards in HEAD 000010 (renumbered from
//     legacy 000007_add_limit_period_columns).
//   - 11 — legacy 1-11 applied; HEAD replays 12..16. Close to the head of
//     develop, catches issues isolated to the very last renumbers.
//   - 12 — full develop → HEAD. Covered by TestUpgradePath_FromDevelopToHead
//     and redundantly here for a single authoritative matrix.
//
// Each case provisions its own Postgres testcontainer so cross-scenario
// contamination is impossible. Combined runtime on a warm Docker engine is
// ~8s (4 × ~2s).
func TestUpgradePath_FromMultipleLegacyVersions(t *testing.T) {
	cases := []struct {
		name          string
		legacyVersion int
	}{
		{"from_dual_runner_v03_to_unified", 3},
		{"from_dual_runner_v07_to_unified", 7},
		{"from_dual_runner_v11_to_unified", 11},
		{"from_dual_runner_v12_to_unified", legacyHeadVersion},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			runUpgradePathScenario(t, tc.legacyVersion)
		})
	}
}

// runUpgradePathScenario executes the full develop→HEAD upgrade flow for a
// given legacy target version:
//
//  1. Extract the pinned legacy migrations (legacyDevelopRef), or skip if the ref is missing.
//  2. Boot a dedicated Postgres container.
//  3. Apply legacy function migrations (always all 3) and legacy schema
//     migrations up to the target version.
//  4. Assert the pre-upgrade state matches a real develop DB at that version.
//  5. Apply HEAD migrations via libPostgres.NewMigrator.
//  6. Assert the final state matches a fresh HEAD install and that
//     version-specific post-conditions hold.
func runUpgradePathScenario(t *testing.T, legacyVersion int) {
	t.Helper()

	// 10-minute scenario budget absorbs git archive + container startup +
	// legacy replay BEFORE the HEAD migrator.Up gets to run. Production
	// grants migrations a full 5-minute deadline (see
	// internal/bootstrap/config.go); if the scenario ctx were 5 minutes,
	// the HEAD Up would inherit only the leftover budget after everything
	// that came before — shorter than what prod promises. Bumping the outer
	// ctx to 10 minutes leaves a real 5 minutes for the Up() child below.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// --- 1. Extract legacyDevelopRef migrations via `git archive` ----------
	oldMigrationsDir := extractDevelopMigrations(ctx, t)

	// --- 2. Boot a throwaway Postgres container -----------------------------
	dsn := startUpgradePathContainer(ctx, t)

	// --- 3. Apply OLD dual-runner sequence ----------------------------------
	applyLegacyFunctionMigrations(ctx, t, dsn, filepath.Join(oldMigrationsDir, "functions"))
	applyLegacySchemaMigrationsUpTo(ctx, t, dsn, oldMigrationsDir, legacyVersion)

	// Sanity-check the pre-upgrade state: we must look like an old production DB
	// at the requested legacy version.
	assertLegacyState(ctx, t, dsn, legacyVersion)

	// --- 4. Apply HEAD unified runner ---------------------------------------
	headMigrationsDir := resolveHeadMigrationsDir(ctx, t)

	migrator, err := libPostgres.NewMigrator(libPostgres.MigrationConfig{
		PrimaryDSN:           dsn,
		DatabaseName:         "tracer_test",
		MigrationsPath:       headMigrationsDir,
		AllowMultiStatements: false,
	})
	require.NoError(t, err, "build HEAD migrator")

	// Mirror the production bootstrap's migration budget (see
	// internal/bootstrap/config.go): the HEAD Up() gets a dedicated
	// 5-minute child derived from context.Background() so that legacy
	// replay or container setup time cannot shrink the migration budget
	// below what production guarantees.
	upCtx, upCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer upCancel()

	require.NoError(t, migrator.Up(upCtx), "apply HEAD migrations over legacy state")

	// --- 5. Assert the upgraded state matches a fresh install ---------------
	assertUpgradedState(ctx, t, dsn)
	assertUpgradedStateForLegacyVersion(ctx, t, dsn, legacyVersion)
}

// extractDevelopMigrations materializes the migrations/ tree from the pinned
// legacyDevelopRef commit into a unique temp directory using `git archive |
// tar -x`. The ref is an immutable SHA (not a branch name), so the legacy
// fixture never drifts even when origin/develop advances past this PR.
//
// Availability check is two-step:
//  1. Best-effort `git fetch origin <SHA>` pulls the commit + tree into the
//     local object store on shallow clones where GitHub allows fetch-by-SHA
//     (uploadpack.allowReachableSHA1InWant=true, enabled by default for
//     org repos). Errors are swallowed — the next step decides.
//  2. `git cat-file -e <SHA>^{tree}` confirms the TREE is actually present.
//     This is stricter than `git rev-parse --verify`, which returns success
//     even on shallow clones when the commit is known as an ancestor but
//     its tree has not been downloaded — the exact failure mode that
//     produces "fatal: not a tree object" from the later `git archive`.
//
// If the tree is still missing after the best-effort fetch, the test skips
// with an actionable message rather than failing with a cryptic git archive
// error.
//
// ctx is threaded into every subprocess via exec.CommandContext so a cancelled
// or timed-out test context also kills the underlying git/tar processes,
// preventing orphaned children in edge cases (LFS hang, broken pipe, etc.).
func extractDevelopMigrations(ctx context.Context, t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	rootBytes, err := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel").Output()
	require.NoError(t, err, "resolve git toplevel")

	repoRoot := strings.TrimSpace(string(rootBytes))

	// Best-effort fetch: pulls the SHA (and its tree) into the local object
	// store on shallow clones. Ignored on failure — the cat-file check below
	// is the authoritative gate. This covers the common CI case where
	// actions/checkout uses fetch-depth: 1 by default but the GitHub host
	// allows fetch-by-SHA for reachable commits.
	fetch := exec.CommandContext(ctx, "git", "fetch", "--no-tags", "--quiet",
		"origin", legacyDevelopRef)
	fetch.Dir = repoRoot
	_ = fetch.Run() // silent: cat-file below is the real gate

	// Real availability check: the TREE of the SHA must be present locally.
	// `git rev-parse --verify` would pass on a shallow clone whenever the
	// commit is a known ancestor, even if the underlying tree was never
	// downloaded — exactly the case that later produces
	// "fatal: not a tree object" from `git archive`. `cat-file -e ^{tree}`
	// resolves the tree and fails loudly if it is missing.
	verifyTree := exec.CommandContext(ctx, "git", "cat-file", "-e",
		legacyDevelopRef+"^{tree}")
	verifyTree.Dir = repoRoot

	if verifyErr := verifyTree.Run(); verifyErr != nil {
		t.Skipf("legacy develop tree for %s not available locally (got %v); "+
			"the test runner appears to have used a shallow clone and the "+
			"best-effort fetch of the SHA also failed. Fix by setting "+
			"actions/checkout fetch-depth: 0 in CI, or ensure the host "+
			"supports fetch-by-SHA (uploadpack.allowReachableSHA1InWant)",
			legacyDevelopRef, verifyErr)
	}

	archive := exec.CommandContext(ctx, "git", "archive", legacyDevelopRef, "migrations/")
	archive.Dir = repoRoot

	tarCmd := exec.CommandContext(ctx, "tar", "-x", "-C", dir)

	pipe, pipeErr := archive.StdoutPipe()
	require.NoError(t, pipeErr, "git archive stdout pipe")

	tarCmd.Stdin = pipe

	var archiveErr, tarErr bytes.Buffer
	archive.Stderr = &archiveErr
	tarCmd.Stderr = &tarErr

	// Start git archive BEFORE tar so a Start() failure on archive fails fast
	// without leaving tar blocked on a pipe that will never produce bytes.
	require.NoError(t, archive.Start(), "start git archive")
	require.NoError(t, tarCmd.Start(), "start tar")
	require.NoError(t, archive.Wait(), "git archive: %s", archiveErr.String())
	require.NoError(t, tarCmd.Wait(), "tar extract: %s", tarErr.String())

	extracted := filepath.Join(dir, "migrations")

	info, statErr := os.Stat(extracted)
	require.NoError(t, statErr, "expected %s to exist after extract", extracted)
	require.True(t, info.IsDir(), "%s must be a directory", extracted)

	return extracted
}

// resolveHeadMigrationsDir returns the absolute path of the migrations/ tree
// in the current working copy (HEAD of the feature branch).
//
// ctx is threaded into the git subprocess via exec.CommandContext so a
// cancelled or timed-out test context kills the child process, matching the
// rest of the upgrade-path flow and avoiding orphaned children on edge cases.
func resolveHeadMigrationsDir(ctx context.Context, t *testing.T) string {
	t.Helper()

	out, err := exec.CommandContext(ctx, "git", "rev-parse", "--show-toplevel").Output()
	require.NoError(t, err, "git rev-parse --show-toplevel")

	root := strings.TrimSpace(string(out))

	return filepath.Join(root, "migrations")
}

// withTestDB opens a *sql.DB against dsn, invokes fn, and guarantees Close()
// via t.Cleanup (runs after fn even on require.FailNow). Centralizes the
// open/close dance so helpers don't each have to carry a defer db.Close().
//
// The context-free openMsg is passed through to require.NoError so per-caller
// context (e.g. "open db for legacy assertions") stays attached to the
// failure message.
func withTestDB(t *testing.T, dsn, openMsg string, fn func(db *sql.DB)) {
	t.Helper()

	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err, openMsg)

	t.Cleanup(func() {
		if closeErr := db.Close(); closeErr != nil {
			t.Logf("close db (%s): %v", openMsg, closeErr)
		}
	})

	fn(db)
}

// startUpgradePathContainer provisions a dedicated Postgres testcontainer so
// the upgrade-path scenario does not contaminate the shared integration-test
// database.
func startUpgradePathContainer(ctx context.Context, t *testing.T) string {
	t.Helper()

	container, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("tracer_test"),
		postgres.WithUsername("tracer"),
		postgres.WithPassword("tracer"),
		// Matched to the shared integration suite (internal/testutil_integration
		// /testcontainer.go) — each upgrade-path scenario is isolated in its own
		// container, but aligning the ceiling avoids surprise if a future test
		// extension starts sharing this helper. The bump from Postgres' default
		// 100 exists because the shared integration suite is saturated by
		// background workers that outlive Service.Shutdown() plus the net-new
		// migration tests this feature adds; raising the ceiling on the test
		// container is a per-test-environment mitigation with zero production
		// impact. The numeric value is single-sourced via
		// testutil_integration.TestPostgresMaxConnections.
		testcontainers.CustomizeRequest(testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				Cmd: []string{"-c", fmt.Sprintf("max_connections=%d", testutil_integration.TestPostgresMaxConnections)},
			},
		}),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	require.NoError(t, err, "start upgrade-path container")

	// Register termination IMMEDIATELY after a successful start, before any
	// other require that could fail and otherwise leak the container to Ryuk.
	// t.Cleanup is the sole termination path: it runs in LIFO order relative
	// to other cleanups registered by the caller, which is sufficient for
	// per-scenario container isolation.
	t.Cleanup(func() {
		terminateCtx, terminateCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer terminateCancel()

		if termErr := container.Terminate(terminateCtx); termErr != nil {
			t.Logf("terminate upgrade-path container: %v", termErr)
		}
	})

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err, "upgrade-path container ConnectionString")

	return connStr
}

// applyLegacyFunctionMigrations reproduces the dual-runner function-migrator
// behaviour that existed on legacyDevelopRef, in a minimal way sufficient to
// seed the legacy state: it creates `schema_migrations_functions`, applies
// the three function SQL files in order, and records them as applied.
//
// INTENTIONALLY MINIMAL REPLAY — NOT A FAITHFUL LEGACY RUNNER.
// This helper does not reproduce the full behaviour of the deleted
// pkg/migration.FunctionMigrator. Specifically it does NOT:
//   - track or reconcile the `dirty` column on schema_migrations_functions
//     (mid-apply failure simulation),
//   - acquire a pg_advisory_lock to serialize concurrent migrators,
//   - wrap each file in its own transaction boundary.
//
// Those behaviours are irrelevant to this test's purpose, which is to verify
// post-upgrade invariants — that after the HEAD runner replays on top of a
// seeded legacy state, the final DB matches a greenfield install. The
// advisory-lock / transaction semantics of the original runner are now
// covered by lib-commons/v5/commons/postgres.Migrator and are asserted
// behaviourally by TestBootstrapMigrations_RefusesDirtyReapply.
//
// Function migrations are orthogonal to the schema_migrations.version target:
// develop always applied all three before any schema migration ran, regardless
// of how many schema migrations were subsequently applied. So we always replay
// the full set here, for every parametrized legacy version.
//
// We intentionally inline this logic (instead of importing the now-deleted
// pkg/migration package) so the test does not depend on legacy production
// code that no longer exists in the module.
func applyLegacyFunctionMigrations(ctx context.Context, t *testing.T, dsn, functionsPath string) {
	t.Helper()

	withTestDB(t, dsn, "open db for legacy function migrations", func(db *sql.DB) {
		_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations_functions (
			version BIGINT PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			dirty BOOLEAN NOT NULL DEFAULT FALSE
		)`)
		require.NoError(t, err, "create schema_migrations_functions")

		entries, err := os.ReadDir(functionsPath)
		require.NoError(t, err, "read %s", functionsPath)

		var upFiles []string

		for _, e := range entries {
			name := e.Name()
			if strings.HasSuffix(name, ".up.sql") {
				upFiles = append(upFiles, name)
			}
		}

		sort.Strings(upFiles)
		require.Len(t, upFiles, 3,
			"legacyDevelopRef must expose exactly 3 function migrations "+
				"(hash chain + truncate protection); drift here would signal "+
				"the pinned SHA no longer represents the true dual-runner layout")

		for _, fname := range upFiles {
			// Parse "000001_name.up.sql" → version=1, name="name"
			trimmed := strings.TrimSuffix(fname, ".up.sql")

			parts := strings.SplitN(trimmed, "_", 2)
			require.Len(t, parts, 2, "malformed function migration filename: %s", fname)

			// schema_migrations_functions.version is BIGINT; bind an int so
			// pgx doesn't rely on implicit string→bigint coercion (which can
			// silently paper over malformed filenames like "garbage_foo.up.sql").
			versionNum, convErr := strconv.Atoi(parts[0])
			require.NoError(t, convErr,
				"parse version prefix of legacy function migration %q", fname)

			body, err := os.ReadFile(filepath.Join(functionsPath, fname))
			require.NoError(t, err, "read %s", fname)

			_, err = db.ExecContext(ctx, string(body))
			require.NoError(t, err, "apply legacy function migration %s", fname)

			_, err = db.ExecContext(ctx,
				`INSERT INTO schema_migrations_functions (version, name) VALUES ($1, $2)`,
				versionNum, parts[1],
			)
			require.NoError(t, err, "record legacy function migration %s", fname)
		}
	})
}

// applyLegacySchemaMigrationsUpTo applies legacy schema migration files in
// order up to (and including) maxVersion, leaving schema_migrations in the
// state a golang-migrate-managed database would be in at that version. The
// subsequent HEAD libPostgres.Migrator picks up exactly where this call left
// off.
//
// We drive golang-migrate directly (rather than libPostgres.Migrator, which
// only exposes Up()) because partial targets require the Migrate(version)
// entry point. The postgres driver is used via WithInstance so we share the
// same pgx-stdlib *sql.DB the rest of the test uses.
func applyLegacySchemaMigrationsUpTo(ctx context.Context, t *testing.T, dsn, migrationsDir string, maxVersion int) {
	t.Helper()

	require.GreaterOrEqual(t, maxVersion, 1, "legacy maxVersion must be ≥ 1")
	require.LessOrEqual(t, maxVersion, legacyHeadVersion,
		"legacy maxVersion must be ≤ %d (the head of legacyDevelopRef)", legacyHeadVersion)

	// Discover the list of up-files on disk so we can range-check maxVersion
	// against reality (and give a useful error if legacyDevelopRef ever diverges).
	entries, err := os.ReadDir(migrationsDir)
	require.NoError(t, err, "read %s", migrationsDir)

	highestOnDisk := 0

	for _, e := range entries {
		n := e.Name()
		if !strings.HasSuffix(n, ".up.sql") || e.IsDir() {
			continue
		}

		trimmed := strings.TrimSuffix(n, ".up.sql")

		parts := strings.SplitN(trimmed, "_", 2)
		if len(parts) != 2 {
			continue
		}

		v, convErr := strconv.Atoi(parts[0])
		if convErr != nil {
			continue
		}

		if v > highestOnDisk {
			highestOnDisk = v
		}
	}

	require.GreaterOrEqual(t, highestOnDisk, maxVersion,
		"legacy migrationsDir %s must contain a file for version ≥ %d (highest found: %d)",
		migrationsDir, maxVersion, highestOnDisk)

	// Not using withTestDB here (even though other helpers do): migratepostgres
	// .WithInstance(db, ...) takes ownership of *sql.DB — its Close() closes the
	// underlying pool. Keeping db with a simple defer db.Close() + registering
	// t.Cleanup(mig.Close) afterwards leaves the second Close() as a harmless
	// double-close, but the ownership chain stays readable. Wrapping in
	// withTestDB would produce identical behavior at the cost of implicit
	// lifetime coupling between the callback return and the migrator teardown.
	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err, "open db for legacy schema migrations")
	defer db.Close()

	driver, err := migratepostgres.WithInstance(db, &migratepostgres.Config{
		DatabaseName:          "tracer_test",
		SchemaName:            "public",
		MultiStatementEnabled: false,
	})
	require.NoError(t, err, "build legacy migrate postgres driver")

	sourceURL := "file://" + migrationsDir

	mig, err := migrate.NewWithDatabaseInstance(sourceURL, "tracer_test", driver)
	require.NoError(t, err, "build legacy migrate instance")

	t.Cleanup(func() {
		// Close the migration source/database wrapper. We intentionally do
		// NOT close the underlying *sql.DB here; the pgx driver is shared
		// with the defer db.Close() above.
		srcErr, dbErr := mig.Close()
		if srcErr != nil {
			t.Logf("close legacy migrate source: %v", srcErr)
		}
		// mig.Close() returns a non-nil dbErr by design when the migrator
		// was created via WithInstance: the library intentionally skips
		// closing the caller-owned *sql.DB (it only closes the internal
		// source). We suppress dbErr here because the test suite's
		// teardown is responsible for closing the database connection.
		_ = dbErr
	})

	require.NoError(t, mig.Migrate(uint(maxVersion)),
		"apply legacy schema migrations up to version %d", maxVersion)

	// Sanity-check the recorded version matches the request.
	var (
		version int
		dirty   bool
	)

	err = db.QueryRowContext(ctx,
		`SELECT version, dirty FROM schema_migrations ORDER BY version DESC LIMIT 1`,
	).Scan(&version, &dirty)
	require.NoError(t, err, "read schema_migrations after legacy apply")
	require.Equal(t, maxVersion, version,
		"legacy schema_migrations.version must equal target %d, got %d", maxVersion, version)
	require.False(t, dirty, "legacy schema_migrations must not be dirty")
}

// assertLegacyState confirms the pre-upgrade DB looks like a production
// develop database at the requested legacy version: schema_migrations.version
// equals that value, dirty is false, and the 3 function rows are tracked.
func assertLegacyState(ctx context.Context, t *testing.T, dsn string, legacyVersion int) {
	t.Helper()

	withTestDB(t, dsn, "open db for legacy assertions", func(db *sql.DB) {
		var (
			version int
			dirty   bool
		)

		err := db.QueryRowContext(ctx,
			`SELECT version, dirty FROM schema_migrations ORDER BY version DESC LIMIT 1`,
		).Scan(&version, &dirty)
		require.NoError(t, err, "read legacy schema_migrations.version")
		require.Equal(t, legacyVersion, version,
			"legacy snapshot must land at schema_migrations.version = %d", legacyVersion)
		require.False(t, dirty, "legacy schema_migrations must not be dirty")

		var functionRows int

		err = db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM schema_migrations_functions`,
		).Scan(&functionRows)
		require.NoError(t, err, "count schema_migrations_functions")
		require.Equal(t, 3, functionRows,
			"legacy snapshot must record 3 function migrations applied")
	})
}

// assertUpgradedState mirrors the fresh-install contract from
// TestBootstrapAppliesAllMigrations: after the HEAD runner replays on top of
// the legacy state, the database must be structurally equivalent to a
// greenfield boot. These assertions hold regardless of the starting legacy
// version.
func assertUpgradedState(ctx context.Context, t *testing.T, dsn string) {
	t.Helper()

	withTestDB(t, dsn, "open db for upgraded assertions", func(db *sql.DB) {
		// 1. schema_migrations now tracks headVersion.
		var (
			version int
			dirty   bool
		)

		err := db.QueryRowContext(ctx,
			`SELECT version, dirty FROM schema_migrations ORDER BY version DESC LIMIT 1`,
		).Scan(&version, &dirty)
		require.NoError(t, err, "read upgraded schema_migrations.version")
		require.Equal(t, headVersion, version, "HEAD runner must land at version %d after upgrade", headVersion)
		require.False(t, dirty, "upgraded schema_migrations must not be dirty")

		// 2. Legacy tracking table has been dropped by migration 000016.
		var legacyExists bool

		err = db.QueryRowContext(ctx, `SELECT EXISTS (
			SELECT 1 FROM information_schema.tables
			WHERE table_schema = 'public' AND table_name = 'schema_migrations_functions'
		)`).Scan(&legacyExists)
		require.NoError(t, err, "check legacy table after upgrade")
		require.False(t, legacyExists,
			"migration 000016 must drop schema_migrations_functions during upgrade")

		// 3. Required hash-chain functions are installed.
		for _, fn := range []string{
			"calculate_audit_event_hash",
			"verify_audit_hash_chain",
			"prevent_truncate",
		} {
			var count int

			err = db.QueryRowContext(ctx,
				`SELECT COUNT(*) FROM pg_proc WHERE proname = $1`, fn,
			).Scan(&count)
			require.NoError(t, err, "pg_proc lookup for %s", fn)
			require.GreaterOrEqual(t, count, 1,
				"function %s must survive upgrade (legacy created it; HEAD 001-003 is idempotent)", fn)
		}

		// 4. Hash-chain trigger on audit_events is still operational.
		// Deterministic resource_id via testutil.MustDeterministicUUID keeps
		// the post-upgrade insert reproducible across runs (no time.Now()
		// sources of nondeterminism); the seed 10001 is unique to this test
		// site to avoid collisions with parallel fixtures.
		_, err = db.ExecContext(ctx, `
			INSERT INTO audit_events (
				event_type, action, result,
				resource_id, resource_type,
				actor_type, actor_id, actor_name, actor_ip_address
			) VALUES (
				'TRANSACTION_VALIDATED', 'VALIDATE', 'ALLOW',
				$1, 'transaction',
				'system', 'upgrade-path-test', 'upgrade-path-test', '127.0.0.1'
			)
		`, testutil.MustDeterministicUUID(10001).String())
		require.NoError(t, err, "insert audit_events through post-upgrade trigger")

		// 5. limits.max_amount must be DECIMAL (not BIGINT) — proves that either
		//    (a) the legacy convert-cents migration already ran, or (b) the HEAD
		//    renumbered convert-cents re-ran and its idempotency guard correctly
		//    executed the conversion exactly once (not zero, not twice).
		var maxAmountType string

		err = db.QueryRowContext(ctx,
			`SELECT data_type FROM information_schema.columns
			 WHERE table_schema = 'public'
			   AND table_name   = 'limits'
			   AND column_name  = 'max_amount'`,
		).Scan(&maxAmountType)
		require.NoError(t, err, "lookup limits.max_amount data_type")
		require.Equal(t, "numeric", maxAmountType,
			"limits.max_amount must be DECIMAL/numeric after upgrade (was the convert-cents guard applied correctly?)")
	})
}

// assertUpgradedStateForLegacyVersion layers version-specific post-conditions
// on top of assertUpgradedState, proving that the renumbered HEAD migrations
// corresponding to each legacy range actually produced the expected DDL when
// replayed. Some assertions are always valid (the constraint/index exists
// regardless of path); others only apply when HEAD had to newly create the
// artifact from scratch.
func assertUpgradedStateForLegacyVersion(ctx context.Context, t *testing.T, dsn string, legacyVersion int) {
	t.Helper()

	withTestDB(t, dsn, "open db for version-specific post-upgrade assertions", func(db *sql.DB) {
		// chk_limits_custom_dates_required must exist exactly once regardless of
		// entry point.
		//   - From legacy v <  7: HEAD 000010 takes the "not exists" guard branch
		//     and creates the constraint freshly.
		//   - From legacy v >= 7: legacy 000007 already created it; HEAD 000010
		//     takes the "exists" guard branch and skips.
		// An un-guarded ADD CONSTRAINT would have errored at migrator.Up for
		// v >= 7, so the fact that Up() succeeded AND count == 1 is the
		// behavioral proof the guard logic is correct.
		var constraintCount int

		err := db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM pg_constraint
			 WHERE conname = 'chk_limits_custom_dates_required'
			   AND conrelid = 'public.limits'::regclass`,
		).Scan(&constraintCount)
		require.NoError(t, err, "count chk_limits_custom_dates_required")
		require.Equal(t, 1, constraintCount,
			"chk_limits_custom_dates_required must exist exactly once after upgrade from v=%d", legacyVersion)

		// The partial unique dedup index on audit_events(resource_id, event_type)
		// must exist after upgrade, regardless of path:
		//   - From legacy v <  11: HEAD 000014 creates it fresh.
		//   - From legacy v >= 11: legacy 000011 already created the
		//     equivalently-named index; HEAD 000014 uses IF NOT EXISTS, no-op.
		var dedupIndexExists bool

		err = db.QueryRowContext(ctx, `SELECT EXISTS (
			SELECT 1 FROM pg_indexes
			WHERE schemaname = 'public'
			  AND tablename  = 'audit_events'
			  AND indexname  = 'idx_audit_events_validation_dedup'
		)`).Scan(&dedupIndexExists)
		require.NoError(t, err, "lookup idx_audit_events_validation_dedup")
		require.True(t, dedupIndexExists,
			"idx_audit_events_validation_dedup must exist after upgrade from v=%d", legacyVersion)
	})
}
