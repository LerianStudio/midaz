//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package transaction

import (
	"context"
	"database/sql"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	libPostgres "github.com/LerianStudio/lib-commons/v7/commons/postgres"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

// TestTransactionRead_UnderDivergence proves the read-routing seam end-to-end for the
// three transaction reads the revert eligibility gate depends on: Find (backs
// GetTransactionByID), FindByParentID (backs GetParentByTransactionID) and
// FindWithOperations (backs GetTransactionWithOperationsByID). A revert issued
// immediately after its create must read its own write; served by a lagging replica the
// gate finds nothing and answers 404 for a transaction that exists.
//
// Real streaming replication is non-deterministic, so — exactly as the balance seam's
// divergence test does — TWO INDEPENDENT Postgres databases are wired as "primary" (A)
// and "replica" (B) behind a single dbresolver-backed *libPostgres.Client. They do NOT
// replicate: writing the rows ONLY to A and leaving B empty simulates INFINITE
// replication lag deterministically.
//
// dbresolver routes non-transactional reads (QueryContext/QueryRowContext) to the
// replica pool (B) and pins read-only BeginTx to the primary pool (A).
// readseam.AcquireReadFrom opens a read-only tx exactly when the rollout flag is on AND
// the context carries the primary-read intent, which is what routes the read to A.
//
// No Redis container is provisioned: unlike the balance NX-seed path, these three reads
// have no cache overlay — they fall through to Postgres unconditionally, so the
// divergence is the sole variable.
func TestTransactionRead_UnderDivergence(t *testing.T) {
	// --- Two independent Postgres databases: A = primary, B = replica ---
	primary := pgtestutil.SetupMigratedContainer(t, "transaction") // A
	replica := pgtestutil.SetupMigratedContainer(t, "transaction") // B

	primaryDSN := pgtestutil.BuildConnectionString(primary.Host, primary.Port, primary.Config)
	replicaDSN := pgtestutil.BuildConnectionString(replica.Host, replica.Port, replica.Config)

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

	// --- Divergence: seed the revert-gate rows ONLY in A (primary) ---
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())
	balanceID := uuid.Must(libCommons.GenerateUUIDv7())

	originParams := pgtestutil.DefaultTransactionParams()
	originParams.Status = "APPROVED"
	originID := pgtestutil.CreateTestTransaction(t, primary.DB, orgID, ledgerID, originParams)

	// One operation so the FindWithOperations inner join has a row to return: without
	// it the join is empty and the gate cannot tell "no operations" from "no transaction".
	pgtestutil.CreateTestOperation(t, primary.DB, orgID, ledgerID, pgtestutil.OperationParams{
		TransactionID: originID,
		Description:   "divergence origin leg",
		Type:          "DEBIT",
		AccountID:     accountID,
		AccountAlias:  "@divergence",
		BalanceID:     balanceID,
		AssetCode:     "USD",
		Amount:        decimal.NewFromInt(100),
		Status:        "APPROVED",
	})

	// A child transaction pointing back at the origin, so FindByParentID has a row to
	// find — the "already reverted" arm of the gate.
	childParams := pgtestutil.DefaultTransactionParams()
	childParams.Status = "APPROVED"
	childParams.ParentTransactionID = &originID
	pgtestutil.CreateTestTransaction(t, primary.DB, orgID, ledgerID, childParams)

	// Guard the divergence deterministically: A HAS the rows, B does NOT.
	requireTransactionRowCount(t, primary.DB, orgID, ledgerID, 2, "primary (A) must contain the seeded transactions")
	requireTransactionRowCount(t, replica.DB, orgID, ledgerID, 0, "replica (B) must NOT contain the seeded transactions")

	ctx := context.Background()

	// Subtest 1: flag ON + intent marked -> all three gate reads see PRIMARY (A).
	t.Run("flag_on_intent_marked_reads_primary", func(t *testing.T) {
		repo := NewTransactionPostgreSQLRepository(conn, true)

		markedCtx := readrouting.WithPrimaryRead(ctx)

		found, err := repo.Find(markedCtx, orgID, ledgerID, originID)
		require.NoError(t, err, "Find routed to primary should succeed")
		assert.Equal(t, originID.String(), found.ID, "Find must return A's row")

		child, err := repo.FindByParentID(markedCtx, orgID, ledgerID, originID)
		require.NoError(t, err, "FindByParentID routed to primary should succeed")
		require.NotNil(t, child, "the child row lives in A, so the parent lookup must find it")
		require.NotNil(t, child.ParentTransactionID)
		assert.Equal(t, originID.String(), *child.ParentTransactionID)

		withOps, err := repo.FindWithOperations(markedCtx, orgID, ledgerID, originID)
		require.NoError(t, err, "FindWithOperations routed to primary should succeed")
		assert.Equal(t, originID.String(), withOps.ID, "FindWithOperations must return A's row")
		assert.Len(t, withOps.Operations, 1, "the seeded operation lives in A and must come back with the transaction")
	})

	// Subtest 2: flag OFF -> reads REPLICA (B), which lacks the rows. This is the bug
	// the pin exists to close: the gate sees nothing and reports not-found.
	t.Run("flag_off_reads_replica_missing_rows", func(t *testing.T) {
		repo := NewTransactionPostgreSQLRepository(conn, false)

		// Intent marker is irrelevant when the flag is off; mark it to prove the flag
		// alone gates routing.
		markedCtx := readrouting.WithPrimaryRead(ctx)

		requireReplicaMiss(markedCtx, t, repo, orgID, ledgerID, originID)
	})

	// Subtest 3: pure query (unmarked ctx) with flag ON -> stays on REPLICA (B).
	t.Run("pure_query_unmarked_ctx_stays_on_replica", func(t *testing.T) {
		repo := NewTransactionPostgreSQLRepository(conn, true)

		// No WithPrimaryRead: a read outside the revert gate must be unaffected by the
		// rollout, so replica routing stays byte-for-byte what it was.
		requireReplicaMiss(ctx, t, repo, orgID, ledgerID, originID)
	})
}

// requireReplicaMiss asserts the three revert-gate reads all land on the replica (B),
// which holds none of the seeded rows: Find reports not-found, FindByParentID answers
// "no parent", and FindWithOperations comes back with the empty value its join produces.
func requireReplicaMiss(ctx context.Context, t *testing.T, repo *TransactionPostgreSQLRepository, orgID, ledgerID, originID uuid.UUID) {
	t.Helper()

	found, err := repo.Find(ctx, orgID, ledgerID, originID)
	require.Error(t, err, "replica (B) does not have the row, so Find must report not-found")
	assert.Nil(t, found)

	child, err := repo.FindByParentID(ctx, orgID, ledgerID, originID)
	require.NoError(t, err, "a missing parent link is not an error")
	assert.Nil(t, child, "replica (B) has no child row to find")

	withOps, err := repo.FindWithOperations(ctx, orgID, ledgerID, originID)
	require.NoError(t, err, "the join simply returns no rows on the replica")
	assert.Empty(t, withOps.ID, "an empty ID is exactly the fallback condition the revert gate keys on")
}

// requireTransactionRowCount asserts the number of non-deleted transaction rows in a raw
// database handle, making the primary/replica divergence explicit and deterministic.
func requireTransactionRowCount(t *testing.T, db *sql.DB, orgID, ledgerID uuid.UUID, want int, msg string) {
	t.Helper()

	var got int

	err := db.QueryRow(
		`SELECT COUNT(*) FROM transaction WHERE organization_id = $1 AND ledger_id = $2 AND deleted_at IS NULL`,
		orgID, ledgerID,
	).Scan(&got)
	require.NoError(t, err, "failed to count transaction rows: %s", msg)
	require.Equal(t, want, got, msg)
}
