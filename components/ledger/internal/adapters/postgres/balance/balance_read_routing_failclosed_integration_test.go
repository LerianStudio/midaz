//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package balance

import (
	"context"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	libPostgres "github.com/LerianStudio/lib-commons/v7/commons/postgres"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"
)

// TestTransactionalRead_PrimaryUnavailable_FailsClosed is the characterization proof
// that the Phase-1 read-routing seam is FAIL-CLOSED under REAL primary unavailability.
//
// The seam (readseam.AcquireReadFrom) routes a marked transactional read to the primary
// by opening a read-only tx over the dbresolver handle; BeginTx always targets the
// primary pool. When the routing flag is ON and the primary-read intent is present but
// the primary is DOWN, the read-only tx cannot be opened. The contract under test:
// AcquireReadFrom returns the wrapped "open read-only primary read transaction" error
// and NO replica reader — for a ledger, reading wrong state is worse than failing
// (readseam.go:92-97). A silent replica fallback is a correctness bug.
//
// Determinism mirrors the sibling divergence test: two INDEPENDENT Postgres containers
// wired PrimaryDSN=A / ReplicaDSN=B behind one *libPostgres.Client. They do NOT
// replicate. The balance row is seeded ONLY in A and left ABSENT in B, so a silent
// fallback to B would be OBSERVABLE as an empty, error-free result — the exact
// anti-behavior this test forbids. Redis is provisioned and the balance key verified
// ABSENT to document the NX-seed (cache miss -> Postgres) precondition. Helpers
// migrateSchema / requireRowCount / requireRedisKeyAbsent are reused from the sibling
// divergence file in this package.
//
// The primary (A) is brought down mid-test via Container.Terminate and we wait until it
// is effectively unavailable (IsRunning false + a raw ping to A errors) BEFORE the
// assert. No time.Now() is used in assertions: waiting uses assert.Eventually with a
// fixed budget, and the seam contract itself is asserted, not any wall-clock value.
func TestTransactionalRead_PrimaryUnavailable_FailsClosed(t *testing.T) {
	// --- Two independent Postgres containers: A = primary, B = replica ---
	primary := pgtestutil.SetupContainer(t) // A
	replica := pgtestutil.SetupContainer(t) // B

	migrationsPath := pgtestutil.FindMigrationsPath(t, "transaction")

	primaryDSN := pgtestutil.BuildConnectionString(primary.Host, primary.Port, primary.Config)
	replicaDSN := pgtestutil.BuildConnectionString(replica.Host, replica.Port, replica.Config)

	// Migrate BOTH databases so the schema exists in each; the divergence under test is
	// row content (A has the row, B does not), not schema.
	migrateExclusiveSchema(t, primaryDSN, primary.Config.DBName, migrationsPath)
	migrateExclusiveSchema(t, replicaDSN, replica.Config.DBName, migrationsPath)

	// Single lib-commons client wiring PrimaryDSN -> A and ReplicaDSN -> B, mirroring the
	// transactional client bootstrap builds.
	conn, err := libPostgres.New(libPostgres.Config{
		PrimaryDSN: primaryDSN,
		ReplicaDSN: replicaDSN,
	})
	require.NoError(t, err, "failed to build postgres client over A(primary)/B(replica)")
	require.NoError(t, conn.Connect(context.Background()), "failed to connect postgres client")

	t.Cleanup(func() {
		if closeErr := conn.Close(); closeErr != nil {
			t.Logf("failed to close postgres client: %v", closeErr)
		}
	})

	// --- Redis container for the cache overlay (NX-seed precondition) ---
	redisResult := redistestutil.SetupReusableContainer(t)

	// --- Divergence: seed the balance ONLY in A (primary) with a KNOWN fresh value ---
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())

	const (
		alias        = "@failclosed"
		balanceKey   = "default"
		aliasWithKey = alias + "#" + balanceKey
	)

	primaryFresh := decimal.NewFromInt(555)

	params := pgtestutil.DefaultBalanceParams()
	params.Alias = alias
	params.Key = balanceKey
	params.Available = primaryFresh
	pgtestutil.CreateTestBalance(t, primary.DB, orgID, ledgerID, accountID, params)

	// Guard the divergence deterministically: A HAS the row, B does NOT. B holding 0 rows
	// is what makes a silent fallback DETECTABLE (empty, error-free) rather than masked.
	requireRowCount(t, primary.DB, orgID, ledgerID, alias, 1, "primary (A) must contain the seeded row")
	requireRowCount(t, replica.DB, orgID, ledgerID, alias, 0, "replica (B) must NOT contain the seeded row")

	// --- NX-seed precondition: force the Redis balance key ABSENT ---
	internalKey := utils.BalanceInternalKey(orgID, ledgerID, aliasWithKey)
	requireRedisKeyAbsent(t, redisResult.Client, internalKey)

	ctx := context.Background()

	// --- Bring the PRIMARY (A) down and wait until it is effectively unavailable ---
	// Terminate stops and removes A. The cleanup registered by SetupContainer will call
	// Terminate again; a second Terminate on an already-removed container is tolerated
	// (it logs, does not fail the test).
	require.NoError(t, primary.Container.Terminate(context.Background()), "failed to terminate primary (A)")

	requirePrimaryUnavailable(t, primary)

	// Subtest 1 (contract under test): flag ON + intent marked + primary DOWN -> the
	// routed read FAILS CLOSED: wrapped tx-open error, and NO silent replica fallback.
	t.Run("flag_on_intent_marked_primary_down_fails_closed", func(t *testing.T) {
		repo := NewBalancePostgreSQLRepository(conn, true)

		markedCtx := readrouting.WithPrimaryRead(ctx)

		balances, err := repo.ListByAliasesWithKeys(markedCtx, orgID, ledgerID, []string{aliasWithKey})

		require.Error(t, err, "primary down + marked read MUST fail closed, not fall back to replica")
		assert.ErrorContains(t, err, "open read-only primary read transaction",
			"error must wrap the readseam tx-open failure, proving the failure came from the primary-routing seam")
		assert.Empty(t, balances,
			"fail-closed contract is an ERROR with NO data; any non-empty result here would be a silent replica fallback")
	})

	// Subtest 2 (contrast — makes the assertion bite): SAME primary-A unavailability, but
	// flag OFF. The pure replica read is UNAFFECTED: it reads B (which lacks the row),
	// returns empty and NO error. This proves fail-closed is specific to the
	// routed-to-primary path and that Subtest 1's error is not a generic infra failure.
	t.Run("flag_off_primary_down_replica_read_unaffected", func(t *testing.T) {
		repo := NewBalancePostgreSQLRepository(conn, false)

		// Intent marker is irrelevant when the flag is off; mark it to prove the flag
		// alone gates routing even while the primary is down.
		markedCtx := readrouting.WithPrimaryRead(ctx)

		balances, err := repo.ListByAliasesWithKeys(markedCtx, orgID, ledgerID, []string{aliasWithKey})

		require.NoError(t, err, "flag OFF reads replica (B), which is UP: primary-A being down must not affect it")
		assert.Empty(t, balances,
			"replica (B) lacks the diverged row -> empty result, proving the replica path is untouched by primary loss")
	})
}

func migrateExclusiveSchema(t *testing.T, dsn, dbName, migrationsPath string) {
	t.Helper()

	migrator, err := libPostgres.NewMigrator(libPostgres.MigrationConfig{
		PrimaryDSN:     dsn,
		DatabaseName:   dbName,
		MigrationsPath: migrationsPath,
	})
	require.NoError(t, err, "failed to create migrator for %s", dbName)
	require.NoError(t, migrator.Up(context.Background()), "failed to run migrations for %s", dbName)
}

// requirePrimaryUnavailable blocks until the primary container is effectively
// unavailable: it is no longer running AND a raw connection ping to it errors. This
// gates the fail-closed assert on a real down primary without using time.Now() in the
// assertion — waiting is bounded by assert.Eventually's fixed budget.
func requirePrimaryUnavailable(t *testing.T, primary *pgtestutil.ContainerResult) {
	t.Helper()

	require.Eventually(t, func() bool {
		if primary.Container.IsRunning() {
			return false
		}

		return primary.DB.PingContext(context.Background()) != nil
	}, 60*time.Second, 500*time.Millisecond, "primary (A) did not become unavailable in time")
}
