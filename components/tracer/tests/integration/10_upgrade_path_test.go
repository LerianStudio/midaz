// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

//go:build integration

package integration

import (
	"bufio"
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	libPostgres "github.com/LerianStudio/lib-commons/v7/commons/postgres"
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
	tracermigrations "github.com/LerianStudio/midaz/v4/components/tracer/migrations"
)

//go:embed testdata/legacy_dual_runner
var legacyMigrations embed.FS

// legacyHeadVersion is the highest schema_migrations.version present in the
// pre-refactor dual-runner fixture: 12 schema migrations + 3 function
// migrations tracked in a separate table.
const legacyHeadVersion = 12

const legacyFixtureRoot = "testdata/legacy_dual_runner"

// headVersion is the expected final schema_migrations.version after applying
// the HEAD migrations (unified single-runner, 000001..000024).
const headVersion = 24

// legacyFixtureManifestSHA256 pins the manifest for the immutable historical
// migration fixture under testdata/legacy_dual_runner. The SQL files were
// recovered byte-for-byte from the last published dual-runner image; see the
// fixture SOURCE.md for its immutable OCI digest and source revision.
const legacyFixtureManifestSHA256 = "a5882440d2fa835e86c2d1dd1a5dcf3afcd0eb58d435d27b1f00add9cd0dd379"

// TestUpgradePath_FromDevelopToHead verifies that a production database at
// the migration sequence in the embedded legacy fixture (schema_migrations.
// version = 12) can be upgraded in place to the HEAD sequence (unified
// single-runner) without corruption.
//
// This is the behavioral proof of the invariant codified in docs/tracer/INVARIANTS.md
// ("Migration Renumbering Invariant"). It simulates:
//
//  1. A fresh container boot with migrations from the immutable legacy fixture
//     (dual-runner layout: `migrations/functions/` + numbered schema
//     migrations 001..012, tracked in `schema_migrations_functions` +
//     `schema_migrations`).
//  2. In-place upgrade to HEAD migrations (unified single-runner, 000001..000021)
//     using the exact same boot runner production will use (libPostgres.Migrator).
//  3. Assertions that the final state matches a fresh install: version=headVersion,
//     legacy tracking table dropped, hash-chain functions installed, audit
//     trigger operational.
//
// See TestUpgradePath_FromMultipleLegacyVersions for the parametrized cousin
// that exercises the same upgrade from earlier legacy versions.
//
// The test spins up its own Postgres testcontainer (independent of the shared
// SetupTestSuite container) and reads the historical migration set from
// testdata. It has no network or git-history dependency, so a shallow checkout
// executes the same upgrade proof as a full local clone.
func TestUpgradePath_FromDevelopToHead(t *testing.T) {
	runUpgradePathScenario(t, legacyHeadVersion)
}

// TestUpgradePath_FromMultipleLegacyVersions parametrizes the develop→HEAD
// upgrade across several schema_migrations.version starting points on the
// pinned pre-refactor legacy layout.
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
//  1. Verify and load the immutable legacy migration fixture.
//  2. Boot a dedicated Postgres container.
//  3. Apply legacy function migrations (always all 3) and legacy schema
//     migrations up to the target version.
//  4. Assert the pre-upgrade state matches a real develop DB at that version.
//  5. Apply HEAD migrations via libPostgres.NewMigrator.
//  6. Assert the final state matches a fresh HEAD install and that
//     version-specific post-conditions hold.
func runUpgradePathScenario(t *testing.T, legacyVersion int) {
	t.Helper()

	// 10-minute scenario budget absorbs fixture validation + container startup +
	// legacy replay BEFORE the HEAD migrator.Up gets to run. Production
	// grants migrations a full 5-minute deadline (see
	// internal/bootstrap/config.go); if the scenario ctx were 5 minutes,
	// the HEAD Up would inherit only the leftover budget after everything
	// that came before — shorter than what prod promises. Bumping the outer
	// ctx to 10 minutes leaves a real 5 minutes for the Up() child below.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	// --- 1. Load the immutable legacy dual-runner fixture -------------------
	oldMigrationsDir := resolveLegacyMigrationsDir(t)

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

// resolveLegacyMigrationsDir returns the repository-owned historical fixture
// after verifying both the pinned manifest and every SQL file in it. A fixture
// change therefore fails closed before a database container is provisioned.
func resolveLegacyMigrationsDir(t *testing.T) string {
	t.Helper()

	verifyLegacyMigrationFixture(t, legacyMigrations)

	return materializeMigrationFS(t, legacyMigrations, legacyFixtureRoot)
}

// resolveHeadMigrationsDir materializes the production migration set embedded
// at compile time. The temporary filesystem tree preserves the file:// input
// contract of lib-commons' golang-migrate wrapper without requiring the source
// checkout to exist when the test binary runs.
func resolveHeadMigrationsDir(_ context.Context, t *testing.T) string {
	t.Helper()

	destinationRoot := t.TempDir()
	require.NoError(t, tracermigrations.WriteTo(destinationRoot), "materialize embedded HEAD migrations")

	return destinationRoot
}

func materializeMigrationFS(t *testing.T, source fs.FS, sourceRoot string) string {
	t.Helper()

	destinationRoot := t.TempDir()

	err := fs.WalkDir(source, sourceRoot, func(sourcePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		relativePath, relErr := embeddedRelativePath(sourceRoot, sourcePath)
		if relErr != nil {
			return relErr
		}
		if relativePath == "." {
			return nil
		}

		destinationPath := filepath.Join(destinationRoot, filepath.FromSlash(relativePath))
		if entry.IsDir() {
			return os.MkdirAll(destinationPath, 0o755)
		}
		if !strings.HasSuffix(entry.Name(), ".sql") {
			return nil
		}

		body, readErr := fs.ReadFile(source, sourcePath)
		if readErr != nil {
			return readErr
		}

		return os.WriteFile(destinationPath, body, 0o600)
	})
	require.NoError(t, err, "materialize embedded migrations")

	return destinationRoot
}

func verifyLegacyMigrationFixture(t *testing.T, fixture fs.FS) {
	t.Helper()

	manifest, err := fs.ReadFile(fixture, path.Join(legacyFixtureRoot, "SHA256SUMS"))
	require.NoError(t, err, "read immutable legacy migration manifest")

	manifestDigest := sha256.Sum256(manifest)
	require.Equal(t, legacyFixtureManifestSHA256, fmt.Sprintf("%x", manifestDigest),
		"legacy migration manifest changed; restore the historical fixture from SOURCE.md")

	manifestFiles := make([]string, 0, 30)
	scanner := bufio.NewScanner(strings.NewReader(string(manifest)))

	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, "  ", 2)
		require.Len(t, parts, 2, "invalid legacy migration manifest line %q", line)

		expectedDigest, decodeErr := hex.DecodeString(parts[0])
		require.NoError(t, decodeErr, "decode digest for %s", parts[1])
		require.Len(t, expectedDigest, sha256.Size, "digest for %s must be SHA-256", parts[1])

		body, readErr := fs.ReadFile(fixture, path.Join(legacyFixtureRoot, parts[1]))
		require.NoError(t, readErr, "read legacy migration %s", parts[1])

		actualDigest := sha256.Sum256(body)
		require.Equal(t, expectedDigest, actualDigest[:],
			"legacy migration %s changed; restore it from SOURCE.md", parts[1])

		manifestFiles = append(manifestFiles, filepath.ToSlash(parts[1]))
	}

	require.NoError(t, scanner.Err(), "scan immutable legacy migration manifest")
	require.Len(t, manifestFiles, 30, "legacy fixture must contain 12 schema and 3 function migration pairs")

	actualFiles := make([]string, 0, len(manifestFiles))
	err = fs.WalkDir(fixture, legacyFixtureRoot, func(fixturePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			return nil
		}

		relativePath, relErr := embeddedRelativePath(legacyFixtureRoot, fixturePath)
		if relErr != nil {
			return relErr
		}

		actualFiles = append(actualFiles, relativePath)

		return nil
	})
	require.NoError(t, err, "enumerate legacy migration fixture")

	sort.Strings(manifestFiles)
	sort.Strings(actualFiles)
	require.Equal(t, manifestFiles, actualFiles, "legacy migration manifest must cover every SQL file exactly once")
}

func embeddedRelativePath(root, name string) (string, error) {
	if name == root {
		return ".", nil
	}
	if root == "." {
		return name, nil
	}

	relativePath, ok := strings.CutPrefix(name, root+"/")
	if !ok {
		return "", fmt.Errorf("embedded path %q is outside root %q", name, root)
	}

	return relativePath, nil
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

	container, err := postgres.Run(
		ctx,
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
// behaviour captured by the legacy fixture, in a minimal way sufficient to
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
			"legacy fixture must expose exactly 3 function migrations "+
				"(hash chain + truncate protection); drift here would signal "+
				"the fixture no longer represents the true dual-runner layout")

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

			_, err = db.ExecContext(
				ctx,
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
		"legacy maxVersion must be ≤ %d (the head of the legacy fixture)", legacyHeadVersion)

	// Discover the list of up-files on disk so we can range-check maxVersion
	// against reality (and give a useful error if the legacy fixture ever diverges).
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

	err = db.QueryRowContext(
		ctx,
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

		err := db.QueryRowContext(
			ctx,
			`SELECT version, dirty FROM schema_migrations ORDER BY version DESC LIMIT 1`,
		).Scan(&version, &dirty)
		require.NoError(t, err, "read legacy schema_migrations.version")
		require.Equal(t, legacyVersion, version,
			"legacy snapshot must land at schema_migrations.version = %d", legacyVersion)
		require.False(t, dirty, "legacy schema_migrations must not be dirty")

		var functionRows int

		err = db.QueryRowContext(
			ctx,
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

		err := db.QueryRowContext(
			ctx,
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

			err = db.QueryRowContext(
				ctx,
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

		err = db.QueryRowContext(
			ctx,
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

		err := db.QueryRowContext(
			ctx,
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

// preRebaselineVersion is the last schema version whose audit trigger still
// wrote the pre-000017 five-field hash. Stopping the migrator here is how a
// test manufactures genuine historical audit records.
const preRebaselineVersion = 16

// headMigrator builds a raw golang-migrate instance over the HEAD migration
// set. libPostgres.NewMigrator only exposes Up, and the rollback cycle below
// needs Migrate and Steps as well.
func headMigrator(t *testing.T, dsn, migrationsDir string) *migrate.Migrate {
	t.Helper()

	db, err := sql.Open("pgx", dsn)
	require.NoError(t, err, "open db for the HEAD migrator")

	driver, err := migratepostgres.WithInstance(db, &migratepostgres.Config{
		DatabaseName:          "tracer_test",
		SchemaName:            "public",
		MultiStatementEnabled: false,
	})
	require.NoError(t, err, "build HEAD migrate postgres driver")

	mig, err := migrate.NewWithDatabaseInstance("file://"+migrationsDir, "tracer_test", driver)
	require.NoError(t, err, "build HEAD migrate instance")

	t.Cleanup(func() {
		if srcErr, dbErr := mig.Close(); srcErr != nil || dbErr != nil {
			t.Logf("close HEAD migrator: source=%v database=%v", srcErr, dbErr)
		}
	})

	return mig
}

// TestUpgradePath_ReBaselineFloorSurvivesARollbackCycle pins the one property
// that makes the audit re-baseline floor worth recording: it is measured once.
//
// The floor is the highest audit id written under the pre-000017 hash formula,
// and records at or below it may be judged by that shorter formula. If a
// rollback dropped the floor, the next upgrade would re-measure it against
// whatever the trail said at that moment — so anyone able to run a migration
// cycle could convert a record in between, have the floor raised to cover it,
// and get a fresh recording timestamp that hides when the real measurement
// happened. That is the difference between an append-only floor and a floor.
//
// So: the rollback keeps the floor, the re-apply does not re-measure it, and a
// conversion planted between the two does not move it and is still reported.
func TestUpgradePath_ReBaselineFloorSurvivesARollbackCycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	dsn := startUpgradePathContainer(ctx, t)
	migrationsDir := resolveHeadMigrationsDir(ctx, t)
	mig := headMigrator(t, dsn, migrationsDir)

	// Genuine historical records: written while the pre-000017 trigger is the
	// live one, so their stored hashes really are five-field digests.
	require.NoError(t, mig.Migrate(preRebaselineVersion),
		"migrate to the last version whose audit trigger wrote the five-field hash")

	withTestDB(t, dsn, "open db to seed historical audit records", func(db *sql.DB) {
		_, err := db.ExecContext(ctx, `
			INSERT INTO audit_events (event_type, action, result, resource_id, resource_type,
			    actor_type, actor_id, actor_name, actor_ip_address)
			SELECT 'RULE_CREATED','CREATE','SUCCESS','res-'||g,'rule','user','u'||g,'n'||g,'10.0.0.'||g
			FROM generate_series(1,5) g`)
		require.NoError(t, err, "seed five historical audit records")
	})

	require.NoError(t, mig.Up(), "apply the remaining migrations, which records the floor")

	// Two records written by the live nine-field trigger, so the trail has rows
	// on both sides of the floor. The conversion below targets one of these:
	// above the floor it must be reported, and re-measuring the floor after the
	// conversion is planted is exactly what would stop it being reported.
	withTestDB(t, dsn, "open db to seed current audit records", func(db *sql.DB) {
		_, err := db.ExecContext(ctx, `
			INSERT INTO audit_events (event_type, action, result, resource_id, resource_type,
			    actor_type, actor_id, actor_name, actor_ip_address)
			SELECT 'RULE_CREATED','CREATE','SUCCESS','cur-'||g,'rule','user','u'||g,'n'||g,'10.0.1.'||g
			FROM generate_series(1,2) g`)
		require.NoError(t, err, "seed two current audit records")
	})

	// readFloor returns the recorded floor and its recording timestamp, the
	// pair an auditor compares against the deployment's upgrade window.
	readFloor := func(t *testing.T) (id int64, recordedAt time.Time) {
		t.Helper()

		withTestDB(t, dsn, "open db to read the floor", func(db *sql.DB) {
			require.NoError(t, db.QueryRowContext(ctx,
				"SELECT max_legacy_id, recorded_at FROM audit_hash_legacy_boundary",
			).Scan(&id, &recordedAt), "the floor must be recorded")
		})

		return id, recordedAt
	}

	firstID, firstRecordedAt := readFloor(t)
	require.Equal(t, int64(5), firstID, "the floor must be the highest historical audit id")

	// --- the rollback must not discard the measurement -----------------------
	require.NoError(t, mig.Steps(-1), "roll the re-baseline migration back")

	afterDownID, afterDownRecordedAt := readFloor(t)
	require.Equal(t, firstID, afterDownID,
		"the rollback MUST keep the floor; re-measuring it later is what re-opens the pre-upgrade window")
	require.True(t, firstRecordedAt.Equal(afterDownRecordedAt),
		"the rollback MUST keep the recording timestamp, which is the auditor's out-of-band check")

	// --- a conversion planted between down and up must not move it -----------
	withTestDB(t, dsn, "open db to plant a conversion", func(db *sql.DB) {
		for _, stmt := range []string{
			"ALTER TABLE audit_events DISABLE RULE prevent_audit_event_update",
			`UPDATE audit_events SET hash = encode(sha256((
			      COALESCE(previous_hash, 'GENESIS')
			   || '|' || event_id::text
			   || '|' || event_type
			   || '|' || to_char(created_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.US"Z"')
			   || '|' || resource_id)::bytea),'hex')
			 WHERE id = (SELECT max(id) FROM audit_events)`,
			"ALTER TABLE audit_events ENABLE RULE prevent_audit_event_update",
		} {
			_, err := db.ExecContext(ctx, stmt)
			require.NoError(t, err, "plant the conversion: %s", stmt)
		}
	})

	require.NoError(t, mig.Up(), "re-apply the re-baseline migration")

	secondID, secondRecordedAt := readFloor(t)
	require.Equal(t, firstID, secondID,
		"the re-apply MUST NOT re-measure the floor; a conversion planted in between would otherwise raise it")
	require.True(t, firstRecordedAt.Equal(secondRecordedAt),
		"the re-apply MUST leave the recording timestamp byte-identical to the first measurement")

	withTestDB(t, dsn, "open db to verify the trail", func(db *sql.DB) {
		var (
			isValid        bool
			firstInvalidID sql.NullInt64
			totalChecked   int64
			detail         sql.NullString
		)

		require.NoError(t, db.QueryRowContext(ctx,
			"SELECT is_valid, first_invalid_id, total_checked, error_detail FROM verify_audit_hash_chain(1, NULL)",
		).Scan(&isValid, &firstInvalidID, &totalChecked, &detail), "verify the trail")

		require.False(t, isValid,
			"the record converted between the rollback and the re-apply MUST still be reported")
		require.Equal(t, int64(7), firstInvalidID.Int64,
			"the converted record, which sits above the floor, MUST be the one flagged")
	})
}

// TestUpgradePath_ReBaselineFloorLandsBesideTheAuditTrail pins WHERE the
// re-baseline migration installs what it installs.
//
// The floor, the verifier and the verifier's pinned path all have to sit in the
// schema that holds audit_events. An unqualified name in a migration lands
// wherever the MIGRATING SESSION's path points, and PostgreSQL's default path
// leads with "$user" — so one `CREATE SCHEMA <migrating role>` between two
// releases, its own documented per-user-schema pattern, is enough to make that
// a schema holding no audit trail at all.
//
// The migration then reports success, because its own boundary pass still finds
// the trail through the session path. What breaks is everything afterwards:
// GET /v1/audit-events/{id}/verify raises `relation "audit_events" does not
// exist` from inside the pinned function, permanently, for every caller —
// where the previous verifier answered.
func TestUpgradePath_ReBaselineFloorLandsBesideTheAuditTrail(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	dsn := startUpgradePathContainer(ctx, t)
	migrationsDir := resolveHeadMigrationsDir(ctx, t)
	mig := headMigrator(t, dsn, migrationsDir)

	// The version immediately before the re-baseline migration, so the one
	// step that follows is the migration under test.
	const (
		lastVersionBeforeTheReBaseline = 23
		seededRecords                  = 3
	)

	require.NoError(t, mig.Migrate(lastVersionBeforeTheReBaseline),
		"apply every migration up to the one before the re-baseline")

	withTestDB(t, dsn, "open db to seed audit records", func(db *sql.DB) {
		_, err := db.ExecContext(ctx, `
			INSERT INTO audit_events (event_type, action, result, resource_id, resource_type,
			    actor_type, actor_id, actor_name, actor_ip_address)
			SELECT 'RULE_CREATED','CREATE','SUCCESS','res-'||g,'rule','user','u'||g,'n'||g,'10.0.0.'||g
			FROM generate_series(1,3) g`)
		require.NoError(t, err, "seed audit records")
	})

	// The divergence: a schema named after the migrating role, which the
	// default '"$user", public' puts ahead of the schema holding the trail.
	var trailSchema string

	withTestDB(t, dsn, "open db to create the role-named schema", func(db *sql.DB) {
		// Omitting the schema name is PostgreSQL's own shorthand for "name it
		// after the role", which is the shape "$user" resolves to.
		_, err := db.ExecContext(ctx, "CREATE SCHEMA AUTHORIZATION CURRENT_USER")
		require.NoError(t, err, "create a schema named after the migrating role")

		require.NoError(t, db.QueryRowContext(ctx, `
			SELECT n.nspname
			FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE c.oid = 'audit_events'::regclass`).Scan(&trailSchema), "locate the audit trail")

		var sessionSchema string
		require.NoError(t, db.QueryRowContext(ctx, "SELECT current_schema()").Scan(&sessionSchema),
			"read the migrating session's schema")
		require.NotEqual(t, trailSchema, sessionSchema,
			"the fixture only means something while the two differ")
	})

	require.NoError(t, mig.Up(), "apply the re-baseline migration")

	withTestDB(t, dsn, "open db to inspect the re-baseline install", func(db *sql.DB) {
		var floorSchema, functionSchema, pin string
		require.NoError(t, db.QueryRowContext(ctx, `
			SELECT (SELECT n.nspname FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
			        WHERE c.relname = 'audit_hash_legacy_boundary'),
			       (SELECT n.nspname FROM pg_proc p JOIN pg_namespace n ON n.oid = p.pronamespace
			        WHERE p.proname = 'verify_audit_hash_chain'),
			       (SELECT p.proconfig[1] FROM pg_proc p WHERE p.proname = 'verify_audit_hash_chain')`,
		).Scan(&floorSchema, &functionSchema, &pin), "inspect what the migration installed")

		require.Equal(t, trailSchema, floorSchema,
			"the floor MUST be created beside the trail it measures, not in the migrating session's schema")
		require.Equal(t, trailSchema, functionSchema,
			"the verifier MUST replace the one callers already resolve, not appear in a schema of its own")
		require.Equal(t, "search_path="+trailSchema+", pg_temp", pin,
			"the pin MUST name the schema that holds the trail, with the temporary schema searched last")

		var (
			isValid        bool
			firstInvalidID sql.NullInt64
			totalChecked   int64
			detail         sql.NullString
		)
		require.NoError(t, db.QueryRowContext(ctx,
			"SELECT is_valid, first_invalid_id, total_checked, error_detail FROM verify_audit_hash_chain(1, NULL)",
		).Scan(&isValid, &firstInvalidID, &totalChecked, &detail),
			"the verify endpoint MUST still answer after the upgrade")
		require.True(t, isValid, "the untouched trail MUST report intact: %v", detail.String)
		require.Equal(t, int64(seededRecords), totalChecked, "every seeded record MUST be walked")
	})
}
