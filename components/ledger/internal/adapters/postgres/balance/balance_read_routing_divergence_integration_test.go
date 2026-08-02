//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package balance

import (
	"context"
	"database/sql"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	libPostgres "github.com/LerianStudio/lib-commons/v6/commons/postgres"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"
)

// TestTransactionalRead_UnderDivergence proves the read-routing feature end-to-end
// under a PRIMARY/REPLICA divergence, on the NX-seed money path (balance absent in
// Redis -> hydrated from Postgres).
//
// Real streaming replication is non-deterministic, so instead we provision TWO
// INDEPENDENT Postgres containers wired as "primary" (A) and "replica" (B) behind a
// single dbresolver-backed *libPostgres.Client. They do NOT replicate: their contents
// are controlled independently. Writing a balance row ONLY to A and leaving B without
// that row simulates INFINITE replication lag deterministically.
//
// dbresolver routes non-transactional reads (QueryContext) to the replica pool (B) and
// pins read-only BeginTx to the primary pool (A). readseam.AcquireReadFrom opens a
// read-only tx exactly when the rollout flag is on AND the context carries the
// primary-read intent, which is what routes the read to A. This is the seam under test.
//
// Placement rationale: this file lives at the balance repository layer because the
// most honest exercise of the NX-seed read is repo.ListByAliasesWithKeys -- the exact
// method query.UseCase.GetBalances falls through to on a Redis cache miss. Asserting
// here keeps the two-Postgres divergence the SOLE variable and avoids dragging the
// tenant-namespaced Redis key path and full UseCase wiring into the routing signal.
// A real Redis container is still provisioned and the balance key is verified ABSENT
// so the NX-seed (cache miss -> Postgres) precondition is documented and enforced.
func TestTransactionalRead_UnderDivergence(t *testing.T) {
	// --- Two independent Postgres containers: A = primary, B = replica ---
	primary := pgtestutil.SetupContainer(t) // A
	replica := pgtestutil.SetupContainer(t) // B

	migrationsPath := pgtestutil.FindMigrationsPath(t, "transaction")

	primaryDSN := pgtestutil.BuildConnectionString(primary.Host, primary.Port, primary.Config)
	replicaDSN := pgtestutil.BuildConnectionString(replica.Host, replica.Port, replica.Config)

	// Migrate BOTH databases so the schema (balance table) exists in each. The
	// divergence under test is ROW CONTENT, not schema: A has the row, B does not.
	migrateSchema(t, primaryDSN, primary.Config.DBName, migrationsPath)
	migrateSchema(t, replicaDSN, replica.Config.DBName, migrationsPath)

	// Single lib-commons client wiring PrimaryDSN -> A and ReplicaDSN -> B, mirroring
	// how bootstrap/config.postgres.transaction*.go builds the transactional client.
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
	redisResult := redistestutil.SetupContainer(t)

	// --- Divergence: seed the balance ONLY in A (primary) with a KNOWN fresh value ---
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())

	const (
		alias        = "@divergence"
		balanceKey   = "default"
		aliasWithKey = alias + "#" + balanceKey
	)

	primaryFresh := decimal.NewFromInt(777)

	params := pgtestutil.DefaultBalanceParams()
	params.Alias = alias
	params.Key = balanceKey
	params.Available = primaryFresh
	pgtestutil.CreateTestBalance(t, primary.DB, orgID, ledgerID, accountID, params)

	// Guard the divergence deterministically: A HAS the row, B does NOT.
	requireRowCount(t, primary.DB, orgID, ledgerID, alias, 1, "primary (A) must contain the seeded row")
	requireRowCount(t, replica.DB, orgID, ledgerID, alias, 0, "replica (B) must NOT contain the seeded row")

	// --- NX-seed precondition: force the Redis balance key ABSENT ---
	internalKey := utils.BalanceInternalKey(orgID, ledgerID, aliasWithKey)
	requireRedisKeyAbsent(t, redisResult.Client, internalKey)

	ctx := context.Background()

	// Subtest 1: flag ON + intent marked -> reads PRIMARY (A) fresh value.
	t.Run("flag_on_intent_marked_reads_primary_NX_seed", func(t *testing.T) {
		repo := NewBalancePostgreSQLRepository(conn, true)

		markedCtx := readrouting.WithPrimaryRead(ctx)

		balances, err := repo.ListByAliasesWithKeys(markedCtx, orgID, ledgerID, []string{aliasWithKey})
		require.NoError(t, err, "read routed to primary should succeed")
		require.Len(t, balances, 1, "primary (A) holds the fresh row, so exactly one balance is returned")

		assert.Equal(t, alias, balances[0].Alias, "alias should match the primary row")
		assert.True(t, balances[0].Available.Equal(primaryFresh),
			"routed read must return A's fresh value (%s), got %s", primaryFresh, balances[0].Available)
	})

	// Subtest 2: flag OFF -> reads REPLICA (B), which lacks the row (documents current behavior).
	t.Run("flag_off_reads_replica_missing_row", func(t *testing.T) {
		repo := NewBalancePostgreSQLRepository(conn, false)

		// Intent marker is irrelevant when the flag is off; mark it to prove the flag
		// alone gates routing.
		markedCtx := readrouting.WithPrimaryRead(ctx)

		balances, err := repo.ListByAliasesWithKeys(markedCtx, orgID, ledgerID, []string{aliasWithKey})
		require.NoError(t, err, "replica read should succeed even when the row is absent")
		assert.Empty(t, balances,
			"flag OFF reads replica (B), which does not have the diverged row -> empty result")
	})

	// Subtest 3: pure query (unmarked ctx) with flag ON -> stays on REPLICA (B).
	t.Run("pure_query_unmarked_ctx_stays_on_replica", func(t *testing.T) {
		repo := NewBalancePostgreSQLRepository(conn, true)

		// No WithPrimaryRead: a pure/read-only query must be unaffected by the rollout.
		balances, err := repo.ListByAliasesWithKeys(ctx, orgID, ledgerID, []string{aliasWithKey})
		require.NoError(t, err, "pure query on replica should succeed")
		assert.Empty(t, balances,
			"unmarked ctx with flag ON must read replica (B) -> empty result, proving pure queries are unaffected")
	})
}

// migrateSchema applies the transaction-component migrations to the target DSN.
// Used to bring BOTH the primary (A) and replica (B) databases to the same schema so
// the divergence under test is purely row content.
func migrateSchema(t *testing.T, dsn, dbName, migrationsPath string) {
	t.Helper()

	migrator, err := libPostgres.NewMigrator(libPostgres.MigrationConfig{
		PrimaryDSN:     dsn,
		DatabaseName:   dbName,
		MigrationsPath: migrationsPath,
	})
	require.NoError(t, err, "failed to create migrator for %s", dbName)
	require.NoError(t, migrator.Up(context.Background()), "failed to run migrations for %s", dbName)
}

// requireRowCount asserts the number of non-deleted balance rows for an alias in a raw
// database handle, making the primary/replica divergence explicit and deterministic.
func requireRowCount(t *testing.T, db *sql.DB, orgID, ledgerID uuid.UUID, alias string, want int, msg string) {
	t.Helper()

	var got int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM balance WHERE organization_id = $1 AND ledger_id = $2 AND alias = $3 AND deleted_at IS NULL`,
		orgID, ledgerID, alias,
	).Scan(&got)
	require.NoError(t, err, "failed to count balance rows: %s", msg)
	require.Equal(t, want, got, msg)
}

// requireRedisKeyAbsent asserts the given key is not present in Redis, enforcing the
// NX-seed precondition (cache miss -> read falls through to Postgres).
func requireRedisKeyAbsent(t *testing.T, client *redis.Client, key string) {
	t.Helper()

	_, err := client.Get(context.Background(), key).Result()
	require.ErrorIs(t, err, redis.Nil, "Redis balance key must be ABSENT for the NX-seed path")
}
