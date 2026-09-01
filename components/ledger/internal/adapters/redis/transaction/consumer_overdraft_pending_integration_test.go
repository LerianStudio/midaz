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

func pendingOverdraftBalanceOp(
	orgID, ledgerID uuid.UUID,
	alias, key, direction string,
	available, onHold, overdraftUsed decimal.Decimal,
	version int64,
	settings *mmodel.BalanceSettings,
	operation, transactionType string,
	amount, overdraftAmount decimal.Decimal,
	routeValidationEnabled bool,
) mmodel.BalanceOperation {
	return mmodel.BalanceOperation{
		Balance: &mmodel.Balance{
			ID:             uuid.NewString(),
			OrganizationID: orgID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      uuid.NewString(),
			Alias:          alias,
			Key:            key,
			AssetCode:      "USD",
			Available:      available,
			OnHold:         onHold,
			Version:        version,
			AccountType:    "deposit",
			AllowSending:   true,
			AllowReceiving: true,
			Direction:      direction,
			OverdraftUsed:  overdraftUsed,
			Settings:       settings,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		},
		Alias: alias + "#" + key,
		Amount: mtransaction.Amount{
			Asset:                  "USD",
			Value:                  amount,
			Operation:              operation,
			TransactionType:        transactionType,
			OverdraftAmount:        overdraftAmount,
			RouteValidationEnabled: routeValidationEnabled,
		},
		InternalKey: utils.BalanceInternalKey(orgID, ledgerID, alias+"#"+key),
	}
}

func findBalanceByKey(t *testing.T, balances []*mmodel.Balance, key string) *mmodel.Balance {
	t.Helper()

	for _, balance := range balances {
		if balance != nil && balance.Key == key {
			return balance
		}
	}

	require.Failf(t, "balance not found", "key %q not found", key)

	return nil
}

func findLatestBalanceByKey(t *testing.T, balances []*mmodel.Balance, key string) *mmodel.Balance {
	t.Helper()

	var latest *mmodel.Balance
	for _, balance := range balances {
		if balance == nil || balance.Key != key {
			continue
		}

		if latest == nil || balance.Version > latest.Version {
			latest = balance
		}
	}

	require.NotNil(t, latest, "expected balance key %q in result", key)

	return latest
}

// TestIntegration_Overdraft_PendingHoldRejectsOverdraftDraw locks the product
// rule: a HOLD never draws overdraft. Debt is created only by conclusive
// operations, so a pending create whose hold exceeds Available is rejected with
// the classic insufficient-funds error even on an allowOverdraft balance, and
// nothing is persisted. Both pending shapes are covered — the legacy single
// ON_HOLD and the route-validated DEBIT + ON_HOLD double entry, where the DEBIT
// leg is the one that would go negative.
func TestIntegration_Overdraft_PendingHoldRejectsOverdraftDraw(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	settings := func() *mmodel.BalanceSettings {
		return &mmodel.BalanceSettings{
			BalanceScope:          mmodel.BalanceScopeTransactional,
			AllowOverdraft:        true,
			OverdraftLimitEnabled: true,
			OverdraftLimit:        ptrString("100"),
		}
	}

	// assertUntouched proves the rejection moved nothing: the rollback restores
	// every balance the batch touched to the state the caller read.
	assertUntouched := func(t *testing.T, infra *integrationTestInfra, orgID, ledgerID uuid.UUID, alias string) {
		t.Helper()

		def := readCachedBalance(t, infra, utils.BalanceInternalKey(orgID, ledgerID, alias+"#"+constant.DefaultBalanceKey))
		assert.Equal(t, "50", def.Available, "a rejected hold must not move available")
		assert.Equal(t, "0", def.OnHold, "a rejected hold must not place a hold")
		assert.Equal(t, "0", def.OverdraftUsed, "a rejected hold must not accrue overdraft")
		assert.Equal(t, int64(1), def.Version, "a rejected hold must not consume a version")
	}

	t.Run("legacy_shape", func(t *testing.T) {
		infra := setupRedisIntegrationInfra(t)
		ctx := context.Background()
		orgID := uuid.New()
		ledgerID := uuid.New()
		alias := "@pending-hold-reject-legacy"

		onHold := pendingOverdraftBalanceOp(orgID, ledgerID, alias, constant.DefaultBalanceKey, constant.DirectionCredit,
			decimal.NewFromInt(50), decimal.Zero, decimal.Zero, 1, settings(),
			constant.ONHOLD, constant.PENDING, decimal.NewFromInt(100), decimal.Zero, false)

		_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
			uuid.New(), constant.PENDING, true, []mmodel.BalanceOperation{onHold})

		require.Error(t, err, "a hold exceeding available must be rejected, not floored into overdraft")
		assert.Contains(t, err.Error(), "0018")

		assertUntouched(t, infra, orgID, ledgerID, alias)
	})

	t.Run("route_validated_shape", func(t *testing.T) {
		infra := setupRedisIntegrationInfra(t)
		ctx := context.Background()
		orgID := uuid.New()
		ledgerID := uuid.New()
		alias := "@pending-hold-reject-rv"

		// Route validation splits the hold: the DEBIT drops Available (and is
		// what would overdraw), the ON_HOLD raises OnHold.
		debit := pendingOverdraftBalanceOp(orgID, ledgerID, alias, constant.DefaultBalanceKey, constant.DirectionCredit,
			decimal.NewFromInt(50), decimal.Zero, decimal.Zero, 1, settings(),
			constant.DEBIT, constant.PENDING, decimal.NewFromInt(100), decimal.Zero, true)
		onHold := pendingOverdraftBalanceOp(orgID, ledgerID, alias, constant.DefaultBalanceKey, constant.DirectionCredit,
			decimal.NewFromInt(50), decimal.Zero, decimal.Zero, 1, settings(),
			constant.ONHOLD, constant.PENDING, decimal.NewFromInt(100), decimal.Zero, true)

		_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
			uuid.New(), constant.PENDING, true, []mmodel.BalanceOperation{debit, onHold})

		require.Error(t, err, "a hold exceeding available must be rejected, not floored into overdraft")
		assert.Contains(t, err.Error(), "0018")

		assertUntouched(t, infra, orgID, ledgerID, alias)
	})
}

// TestIntegration_Overdraft_PendingLegacyCancelRestoresCompanion locks the
// UNWIND path, which must keep working unchanged: holds no longer draw overdraft,
// but a pending created under an earlier build did, and those pendings still have
// to be cancelable after deploy.
//
// The drawn state is therefore SEEDED directly rather than produced by running a
// hold — a hold that draws is now rejected, so the old setup could not reach this
// path at all. The seeded values are exactly what an earlier build left behind: a
// 100 hold against 50 available floored Available at 0, put 100 in OnHold, accrued
// 50 of overdraft, and moved the mirrored 50 onto the companion. The Lua NX-seed
// materializes the cache from these incoming values on first touch, so the cancel
// batch runs against the same state it would have read in production.
func TestIntegration_Overdraft_PendingLegacyCancelRestoresCompanion(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	alias := "@t17-pending-cancel"

	settings := &mmodel.BalanceSettings{
		BalanceScope:          mmodel.BalanceScopeTransactional,
		AllowOverdraft:        true,
		OverdraftLimitEnabled: true,
		OverdraftLimit:        ptrString("100"),
	}

	// Drawn state left by a hold under an earlier build (version already bumped
	// once by that hold).
	const drawnVersion = 2

	drawnAvailable, drawnOnHold, drawnOverdraftUsed := decimal.Zero, decimal.NewFromInt(100), decimal.NewFromInt(50)
	companionAvailable := decimal.NewFromInt(50)

	cancelDefault := pendingOverdraftBalanceOp(orgID, ledgerID, alias, constant.DefaultBalanceKey, constant.DirectionCredit,
		drawnAvailable, drawnOnHold, drawnOverdraftUsed, drawnVersion, settings,
		constant.RELEASE, constant.CANCELED, decimal.NewFromInt(100), decimal.NewFromInt(50), false)
	cancelCompanion := pendingOverdraftBalanceOp(orgID, ledgerID, alias, constant.OverdraftBalanceKey, constant.DirectionDebit,
		companionAvailable, decimal.Zero, decimal.Zero, drawnVersion, nil,
		constant.CREDIT, constant.CANCELED, decimal.NewFromInt(50), decimal.Zero, false)

	cancelResult, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.CANCELED, true, []mmodel.BalanceOperation{cancelDefault, cancelCompanion})
	require.NoError(t, err)
	require.Len(t, cancelResult.After, 2)

	defaultAfterCancel := findLatestBalanceByKey(t, cancelResult.After, constant.DefaultBalanceKey)
	companionAfterCancel := findBalanceByKey(t, cancelResult.After, constant.OverdraftBalanceKey)

	assert.True(t, defaultAfterCancel.Available.Equal(decimal.NewFromInt(50)))
	assert.True(t, defaultAfterCancel.OnHold.IsZero())
	assert.True(t, defaultAfterCancel.OverdraftUsed.IsZero())
	assert.True(t, companionAfterCancel.Available.IsZero())
}

// TestIntegration_Overdraft_PendingRouteValidationCancelAllowsSameBatchVersionChain
// is the route-validated sibling of the unwind test above, and locks the
// same-batch version chain: the RELEASE bumps the version, so the CREDIT that
// follows it in the same batch reads a version one ahead of the one it carried
// and must not be treated as stale.
//
// The drawn state is SEEDED for the same reason given above — a hold that draws
// is now rejected, so it cannot be produced by running one. In the
// route-validated shape the earlier build's hold mutated the default balance
// twice (DEBIT then ON_HOLD), which is why its seeded version is one higher than
// the companion's.
func TestIntegration_Overdraft_PendingRouteValidationCancelAllowsSameBatchVersionChain(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	alias := "@t17-route-cancel"

	settings := &mmodel.BalanceSettings{
		BalanceScope:          mmodel.BalanceScopeTransactional,
		AllowOverdraft:        true,
		OverdraftLimitEnabled: true,
		OverdraftLimit:        ptrString("100"),
	}

	// Drawn state left by a route-validated hold under an earlier build: the
	// default balance was mutated twice (DEBIT then ON_HOLD), the companion once.
	const (
		drawnDefaultVersion   = 3
		drawnCompanionVersion = 2
	)

	drawnAvailable, drawnOnHold, drawnOverdraftUsed := decimal.Zero, decimal.NewFromInt(100), decimal.NewFromInt(50)
	companionAvailable := decimal.NewFromInt(50)

	cancelRelease := pendingOverdraftBalanceOp(orgID, ledgerID, alias, constant.DefaultBalanceKey, constant.DirectionCredit,
		drawnAvailable, drawnOnHold, drawnOverdraftUsed, drawnDefaultVersion, settings,
		constant.RELEASE, constant.CANCELED, decimal.NewFromInt(100), decimal.Zero, true)
	cancelCredit := pendingOverdraftBalanceOp(orgID, ledgerID, alias, constant.DefaultBalanceKey, constant.DirectionCredit,
		drawnAvailable, drawnOnHold, drawnOverdraftUsed, drawnDefaultVersion, settings,
		constant.CREDIT, constant.CANCELED, decimal.NewFromInt(100), decimal.NewFromInt(50), true)
	cancelCompanion := pendingOverdraftBalanceOp(orgID, ledgerID, alias, constant.OverdraftBalanceKey, constant.DirectionDebit,
		companionAvailable, decimal.Zero, decimal.Zero, drawnCompanionVersion, nil,
		constant.CREDIT, constant.CANCELED, decimal.NewFromInt(50), decimal.Zero, true)

	cancelResult, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.CANCELED, true, []mmodel.BalanceOperation{cancelRelease, cancelCredit, cancelCompanion})
	require.NoError(t, err, "same-batch RELEASE must not make the following overdraft CREDIT look stale")
	require.Len(t, cancelResult.After, 3)

	defaultAfterCancel := findLatestBalanceByKey(t, cancelResult.After, constant.DefaultBalanceKey)
	companionAfterCancel := findBalanceByKey(t, cancelResult.After, constant.OverdraftBalanceKey)

	assert.True(t, defaultAfterCancel.Available.Equal(decimal.NewFromInt(50)))
	assert.True(t, defaultAfterCancel.OnHold.IsZero())
	assert.True(t, defaultAfterCancel.OverdraftUsed.IsZero())
	assert.True(t, companionAfterCancel.Available.IsZero())
}
