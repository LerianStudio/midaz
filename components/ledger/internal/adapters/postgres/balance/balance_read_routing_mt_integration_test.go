//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package balance

import (
	"context"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	libPostgres "github.com/LerianStudio/lib-commons/v7/commons/postgres"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	pgtestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"
)

// TestTransactionalRead_MultiTenant is the multi-tenant (MT) non-regression proof for
// the read-routing seam. It mirrors TestTransactionalRead_UnderDivergence but forces
// resolution through the TENANT context handle instead of the static r.connection,
// proving that a marked transactional read lands on the TENANT's primary and a pure
// query stays on the TENANT's replica.
//
// MT wiring: the repo is constructed with requireTenant=true so the static-connection
// fallback in getDB is OFF (r.connection is nil). The A(primary)/B(replica) dbresolver
// handle is injected into context via tmcore.ContextWithPG(..., constant.ModuleTransaction),
// which is exactly the key getDB prioritizes (GetPGContext(ctx, ModuleTransaction)). A
// tenant id is set via tmcore.ContextWithTenantID to represent a resolved tenant. This
// makes the seam operate on the tenant's handle without ever touching r.connection.
//
// The deterministic divergence is identical to the single-tenant divergence test: two INDEPENDENT Postgres
// databases wired PrimaryDSN=A / ReplicaDSN=B behind one *libPostgres.Client. They do
// NOT replicate; A holds the seeded balance row, B does not (infinite lag). Redis is
// provisioned and the balance key is verified ABSENT to document the NX-seed (cache
// miss -> Postgres) precondition. Helpers requireRowCount / requireRedisKeyAbsent are
// reused from the sibling divergence file in this package; schema migration is handled
// by pgtestutil.SetupMigratedContainer.
func TestTransactionalRead_MultiTenant(t *testing.T) {
	// --- Two independent Postgres databases: A = tenant primary, B = tenant replica ---
	primary := pgtestutil.SetupMigratedContainer(t, "transaction") // A
	replica := pgtestutil.SetupMigratedContainer(t, "transaction") // B

	primaryDSN := pgtestutil.BuildConnectionString(primary.Host, primary.Port, primary.Config)
	replicaDSN := pgtestutil.BuildConnectionString(replica.Host, replica.Port, replica.Config)

	// Single lib-commons client wiring PrimaryDSN -> A and ReplicaDSN -> B, mirroring the
	// transactional client bootstrap builds. This is the TENANT handle: it is injected
	// into context, NOT set on the repo's static connection.
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

	// Resolve the dbresolver.DB (A/B-wired) that middleware would inject per tenant.
	tenantResolver, err := conn.Resolver(context.Background())
	require.NoError(t, err, "failed to resolve tenant dbresolver handle")

	// --- Redis container for the cache overlay (NX-seed precondition) ---
	redisResult := redistestutil.SetupReusableContainer(t)

	// --- Divergence: seed the balance ONLY in A (tenant primary) with a KNOWN fresh value ---
	orgID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	accountID := uuid.Must(libCommons.GenerateUUIDv7())
	tenantID := uuid.Must(libCommons.GenerateUUIDv7()).String()

	const (
		alias        = "@mt-divergence"
		balanceKey   = "default"
		aliasWithKey = alias + "#" + balanceKey
	)

	primaryFresh := decimal.NewFromInt(999)

	params := pgtestutil.DefaultBalanceParams()
	params.Alias = alias
	params.Key = balanceKey
	params.Available = primaryFresh
	pgtestutil.CreateTestBalance(t, primary.DB, orgID, ledgerID, accountID, params)

	// Guard the divergence deterministically: A HAS the row, B does NOT.
	requireRowCount(t, primary.DB, orgID, ledgerID, alias, 1, "tenant primary (A) must contain the seeded row")
	requireRowCount(t, replica.DB, orgID, ledgerID, alias, 0, "tenant replica (B) must NOT contain the seeded row")

	// --- NX-seed precondition: force the Redis balance key ABSENT ---
	internalKey := utils.BalanceInternalKey(orgID, ledgerID, aliasWithKey)
	requireRedisKeyAbsent(t, redisResult.Client, internalKey)

	// Build the tenant-carrying context: the module-specific PG handle + tenant id, as
	// the tenant middleware (lib-commons tenant managers) would inject in MT mode.
	tenantCtx := tmcore.ContextWithPG(context.Background(), tenantResolver, constant.ModuleTransaction)
	tenantCtx = tmcore.ContextWithTenantID(tenantCtx, tenantID)

	// Repo built with requireTenant=true so the static r.connection path is OFF: the ONLY
	// resolvable handle is the one carried in context. r.connection is deliberately nil.
	repo := NewBalancePostgreSQLRepository(nil, true, true)

	// Subtest 1: flag ON + intent MARKED + tenant handle -> reads the TENANT PRIMARY (A).
	t.Run("mt_flag_on_intent_marked_reads_tenant_primary_NX_seed", func(t *testing.T) {
		repo.routeTxReadsToPrimary = true

		markedCtx := readrouting.WithPrimaryRead(tenantCtx)

		// Defensive: the tenant PG handle MUST be present on the context, else resolution
		// silently fell to the static path and this test degenerates to single-tenant.
		require.NotNil(t, tmcore.GetPGContext(markedCtx, constant.ModuleTransaction),
			"tenant PG handle missing from context: resolution would fall to the static path (test degraded to ST)")

		balances, err := repo.ListByAliasesWithKeys(markedCtx, orgID, ledgerID, []string{aliasWithKey})
		require.NoError(t, err, "marked read routed to tenant primary should succeed")
		require.Len(t, balances, 1, "tenant primary (A) holds the fresh row, so exactly one balance is returned")

		assert.Equal(t, alias, balances[0].Alias, "alias should match the tenant primary row")
		assert.True(t, balances[0].Available.Equal(primaryFresh),
			"marked read on the tenant handle must return A's fresh value (%s), got %s", primaryFresh, balances[0].Available)
	})

	// Subtest 2: flag ON + intent ABSENT + tenant handle -> stays on the TENANT REPLICA (B).
	t.Run("mt_pure_query_unmarked_ctx_stays_on_tenant_replica", func(t *testing.T) {
		repo.routeTxReadsToPrimary = true

		// Defensive: the tenant PG handle MUST be present on the (unmarked) context too.
		require.NotNil(t, tmcore.GetPGContext(tenantCtx, constant.ModuleTransaction),
			"tenant PG handle missing from context: resolution would fall to the static path (test degraded to ST)")

		// No WithPrimaryRead: a pure MT query must be unaffected by the rollout and read
		// the tenant replica (B), which lacks the diverged row.
		balances, err := repo.ListByAliasesWithKeys(tenantCtx, orgID, ledgerID, []string{aliasWithKey})
		require.NoError(t, err, "pure query on tenant replica should succeed even when the row is absent")
		assert.Empty(t, balances,
			"unmarked ctx with flag ON must read the tenant replica (B) -> empty result, proving pure MT queries are unaffected")
	})
}
