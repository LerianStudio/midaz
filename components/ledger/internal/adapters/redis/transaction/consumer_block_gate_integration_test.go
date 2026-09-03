//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// =============================================================================
// BLOCK GATE — LUA SCRIPT BEHAVIOUR (REDIS INTEGRATION)
// =============================================================================
// These tests drive the embedded script DIRECTLY, so they read the raw verdict
// the gate returns ("BLOCKED:<id>", "NEEDS_HYDRATION" or the balance JSON)
// before any Go mapping can smooth it over. The Go handling of those verdicts
// is covered separately against ProcessBalanceAtomicOperation.
//
// Every denial case asserts on the balance KEY not existing, not merely on the
// returned value: the script's first act on a balance is `SET ... NX`, so a key
// that was never created is proof that pass 1 ran ahead of pass 2.

// gateOp builds a balance operation for the gate tests, with the account ID and
// the exception grant under the test's control.
func gateOp(
	orgID, ledgerID, accountID uuid.UUID,
	alias string,
	granted bool,
	available, onHold decimal.Decimal,
	operation string,
	amount decimal.Decimal,
) mmodel.BalanceOperation {
	balanceKey := alias + "#" + constant.DefaultBalanceKey

	grantedExceptionID := ""
	if granted {
		grantedExceptionID = uuid.NewString()
	}

	return mmodel.BalanceOperation{
		Balance: &mmodel.Balance{
			ID:             uuid.New().String(),
			OrganizationID: orgID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      accountID.String(),
			Alias:          alias,
			Key:            balanceKey,
			AssetCode:      "USD",
			Available:      available,
			OnHold:         onHold,
			Version:        1,
			AccountType:    "deposit",
			AllowSending:   true,
			AllowReceiving: true,
			Direction:      constant.DirectionCredit,
			OverdraftUsed:  decimal.Zero,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		},
		Alias: alias,
		Amount: mtransaction.Amount{
			Asset:              "USD",
			Value:              amount,
			Operation:          operation,
			GrantedExceptionID: grantedExceptionID,
		},
		InternalKey: utils.BalanceInternalKey(orgID, ledgerID, balanceKey),
	}
}

// runGateScript invokes the embedded atomic script through the production plan
// builder and key set, returning the raw reply.
func runGateScript(
	t *testing.T,
	infra *integrationTestInfra,
	ctx context.Context,
	orgID, ledgerID uuid.UUID,
	status string,
	pending bool,
	ops []mmodel.BalanceOperation,
) (any, error) {
	t.Helper()

	plan, err := infra.repo.buildBalanceAtomicOperationPlan(ctx, status, pending, ops)
	require.NoError(t, err)

	keys, err := tenantKeysFromContext(ctx, []string{
		TransactionBackupQueue,
		utils.TransactionInternalKey(orgID, ledgerID, uuid.New().String()),
		utils.BalanceSyncScheduleKey,
		utils.BlockedAccountsInternalKey(orgID, ledgerID),
	})
	require.NoError(t, err)

	return balanceAtomicScript.Run(ctx, infra.redisContainer.Client, keys, plan.args...).Result()
}

// requireNoBalanceKeys fails unless every balance in ops is absent from Redis —
// the observable form of "nothing was mutated".
func requireNoBalanceKeys(t *testing.T, infra *integrationTestInfra, ctx context.Context, ops []mmodel.BalanceOperation) {
	t.Helper()

	for _, op := range ops {
		exists, err := infra.redisContainer.Client.Exists(ctx, op.InternalKey).Result()
		require.NoError(t, err)
		assert.Equal(t, int64(0), exists,
			"balance %q must not have been touched by a denied batch", op.InternalKey)
	}
}

// hydrateBlockedSet marks the ledger's index as fully hydrated and blocks the
// given accounts, mirroring what the block command leaves behind.
func hydrateBlockedSet(t *testing.T, infra *integrationTestInfra, ctx context.Context, orgID, ledgerID uuid.UUID, blocked ...uuid.UUID) {
	t.Helper()

	require.NoError(t, infra.repo.HydrateBlockedAccounts(ctx, orgID, ledgerID, blocked))
}

// TestIntegration_BlockGate_DeniesBlockedAccountWithoutGrant is the base case:
// the account sits in the SET, the operation carries no grant, so the batch is
// refused before it can write anything.
func TestIntegration_BlockGate_DeniesBlockedAccountWithoutGrant(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID, ledgerID := uuid.New(), uuid.New()
	accountID := uuid.New()

	hydrateBlockedSet(t, infra, ctx, orgID, ledgerID, accountID)

	ops := []mmodel.BalanceOperation{
		gateOp(orgID, ledgerID, accountID, "@gate-denied", false,
			decimal.NewFromInt(500), decimal.Zero, constant.DEBIT, decimal.NewFromInt(100)),
	}

	got, err := runGateScript(t, infra, ctx, orgID, ledgerID, constant.APPROVED, false, ops)
	require.NoError(t, err, "a denial is a structured verdict, not a script error")
	assert.Equal(t, "BLOCKED:"+accountID.String(), got,
		"the verdict must name the account that caused it")

	requireNoBalanceKeys(t, infra, ctx, ops)
}

// TestIntegration_BlockGate_DeniesBeforeAnyMutationInMultiOperationBatch is the
// two-pass proof. Only the LAST operation's account is blocked, so a script
// that checked inline would already have written the first two balances by the
// time it found the denial.
func TestIntegration_BlockGate_DeniesBeforeAnyMutationInMultiOperationBatch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID, ledgerID := uuid.New(), uuid.New()
	blockedID := uuid.New()

	hydrateBlockedSet(t, infra, ctx, orgID, ledgerID, blockedID)

	ops := []mmodel.BalanceOperation{
		gateOp(orgID, ledgerID, uuid.New(), "@gate-first", false,
			decimal.NewFromInt(500), decimal.Zero, constant.DEBIT, decimal.NewFromInt(100)),
		gateOp(orgID, ledgerID, uuid.New(), "@gate-second", false,
			decimal.NewFromInt(500), decimal.Zero, constant.CREDIT, decimal.NewFromInt(50)),
		gateOp(orgID, ledgerID, blockedID, "@gate-last", false,
			decimal.NewFromInt(500), decimal.Zero, constant.CREDIT, decimal.NewFromInt(50)),
	}

	got, err := runGateScript(t, infra, ctx, orgID, ledgerID, constant.APPROVED, false, ops)
	require.NoError(t, err)
	assert.Equal(t, "BLOCKED:"+blockedID.String(), got)

	requireNoBalanceKeys(t, infra, ctx, ops)
}

// TestIntegration_BlockGate_GrantTranspassesTheBlock covers the exception path:
// the very same blocked account transacts normally once the operation carries a
// grant.
func TestIntegration_BlockGate_GrantTranspassesTheBlock(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID, ledgerID := uuid.New(), uuid.New()
	accountID := uuid.New()

	hydrateBlockedSet(t, infra, ctx, orgID, ledgerID, accountID)

	ops := []mmodel.BalanceOperation{
		gateOp(orgID, ledgerID, accountID, "@gate-granted", true,
			decimal.NewFromInt(500), decimal.Zero, constant.DEBIT, decimal.NewFromInt(100)),
	}

	got, err := runGateScript(t, infra, ctx, orgID, ledgerID, constant.APPROVED, false, ops)
	require.NoError(t, err)
	assert.NotContains(t, got, "BLOCKED:", "a granted operation must not be denied")

	assert.Equal(t, "400", readCachedBalance(t, infra, ops[0].InternalKey).Available,
		"the granted operation must apply in full")
}

// TestIntegration_BlockGate_PartialHydrationNeedsHydration covers the
// fail-closed half. Members exist but the sentinel does not, so the index knows
// nothing and MUST say so instead of reading as "nothing is blocked".
func TestIntegration_BlockGate_PartialHydrationNeedsHydration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID, ledgerID := uuid.New(), uuid.New()
	accountID := uuid.New()

	// AddBlockedAccount writes a member without the sentinel — exactly the shape
	// an interrupted hydration, or a Redis that lost the key, leaves behind.
	require.NoError(t, infra.repo.AddBlockedAccount(ctx, orgID, ledgerID, accountID))

	ops := []mmodel.BalanceOperation{
		gateOp(orgID, ledgerID, accountID, "@gate-unhydrated", false,
			decimal.NewFromInt(500), decimal.Zero, constant.DEBIT, decimal.NewFromInt(100)),
	}

	got, err := runGateScript(t, infra, ctx, orgID, ledgerID, constant.APPROVED, false, ops)
	require.NoError(t, err)
	assert.Equal(t, "NEEDS_HYDRATION", got)

	requireNoBalanceKeys(t, infra, ctx, ops)
}

// TestIntegration_BlockGate_MissingSetNeedsHydration is the same invariant for a
// SET that does not exist at all — the Redis-restart case.
func TestIntegration_BlockGate_MissingSetNeedsHydration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID, ledgerID := uuid.New(), uuid.New()

	ops := []mmodel.BalanceOperation{
		gateOp(orgID, ledgerID, uuid.New(), "@gate-missing-set", false,
			decimal.NewFromInt(500), decimal.Zero, constant.DEBIT, decimal.NewFromInt(100)),
	}

	got, err := runGateScript(t, infra, ctx, orgID, ledgerID, constant.APPROVED, false, ops)
	require.NoError(t, err)
	assert.Equal(t, "NEEDS_HYDRATION", got,
		"an absent index is unknown, never empty")

	requireNoBalanceKeys(t, infra, ctx, ops)
}

// TestIntegration_BlockGate_CanceledIsExempt pins the contract carve-out: a
// cancel returns a hold to the very account it came from, so no money leaves a
// blocked account and the gate must stand aside.
func TestIntegration_BlockGate_CanceledIsExempt(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID, ledgerID := uuid.New(), uuid.New()
	accountID := uuid.New()

	hydrateBlockedSet(t, infra, ctx, orgID, ledgerID, accountID)

	ops := []mmodel.BalanceOperation{
		gateOp(orgID, ledgerID, accountID, "@gate-cancel", false,
			decimal.NewFromInt(400), decimal.NewFromInt(100), constant.RELEASE, decimal.NewFromInt(100)),
	}

	got, err := runGateScript(t, infra, ctx, orgID, ledgerID, constant.CANCELED, true, ops)
	require.NoError(t, err)
	assert.NotContains(t, got, "BLOCKED:", "cancel on a blocked account must not be denied")

	after := readCachedBalance(t, infra, ops[0].InternalKey)
	assert.Equal(t, "500", after.Available, "the hold must return to available")
	assert.Equal(t, "0", after.OnHold)
}

// TestIntegration_BlockGate_UnblockedAccountIsUnaffected is the common path: a
// hydrated index that does not carry the account changes nothing about how the
// batch behaves.
func TestIntegration_BlockGate_UnblockedAccountIsUnaffected(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID, ledgerID := uuid.New(), uuid.New()

	hydrateBlockedSet(t, infra, ctx, orgID, ledgerID, uuid.New())

	ops := []mmodel.BalanceOperation{
		gateOp(orgID, ledgerID, uuid.New(), "@gate-free", false,
			decimal.NewFromInt(500), decimal.Zero, constant.DEBIT, decimal.NewFromInt(100)),
	}

	got, err := runGateScript(t, infra, ctx, orgID, ledgerID, constant.APPROVED, false, ops)
	require.NoError(t, err)
	assert.NotContains(t, got, "BLOCKED:")

	assert.Equal(t, "400", readCachedBalance(t, infra, ops[0].InternalKey).Available)
}
