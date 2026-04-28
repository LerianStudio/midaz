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

	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
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

func TestIntegration_Overdraft_PendingLegacyHoldMutatesCompanion(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	alias := "@t17-pending-hold"

	settings := &mmodel.BalanceSettings{
		BalanceScope:          mmodel.BalanceScopeTransactional,
		AllowOverdraft:        true,
		OverdraftLimitEnabled: true,
		OverdraftLimit:        ptrString("100"),
	}

	defaultOp := pendingOverdraftBalanceOp(orgID, ledgerID, alias, constant.DefaultBalanceKey, constant.DirectionCredit,
		decimal.NewFromInt(50), decimal.Zero, decimal.Zero, 1, settings,
		constant.ONHOLD, constant.PENDING, decimal.NewFromInt(100), decimal.Zero, false)
	companionOp := pendingOverdraftBalanceOp(orgID, ledgerID, alias, constant.OverdraftBalanceKey, constant.DirectionDebit,
		decimal.Zero, decimal.Zero, decimal.Zero, 1, nil,
		constant.DEBIT, constant.PENDING, decimal.NewFromInt(50), decimal.Zero, false)

	result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.PENDING, true, []mmodel.BalanceOperation{defaultOp, companionOp})

	require.NoError(t, err)
	require.Len(t, result.After, 2)

	defaultAfter := findBalanceByKey(t, result.After, constant.DefaultBalanceKey)
	companionAfter := findBalanceByKey(t, result.After, constant.OverdraftBalanceKey)

	assert.True(t, defaultAfter.Available.IsZero())
	assert.True(t, defaultAfter.OnHold.Equal(decimal.NewFromInt(100)))
	assert.True(t, defaultAfter.OverdraftUsed.Equal(decimal.NewFromInt(50)))
	assert.True(t, companionAfter.Available.Equal(decimal.NewFromInt(50)))
}

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

	pendingDefault := pendingOverdraftBalanceOp(orgID, ledgerID, alias, constant.DefaultBalanceKey, constant.DirectionCredit,
		decimal.NewFromInt(50), decimal.Zero, decimal.Zero, 1, settings,
		constant.ONHOLD, constant.PENDING, decimal.NewFromInt(100), decimal.Zero, false)
	pendingCompanion := pendingOverdraftBalanceOp(orgID, ledgerID, alias, constant.OverdraftBalanceKey, constant.DirectionDebit,
		decimal.Zero, decimal.Zero, decimal.Zero, 1, nil,
		constant.DEBIT, constant.PENDING, decimal.NewFromInt(50), decimal.Zero, false)

	pendingResult, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.PENDING, true, []mmodel.BalanceOperation{pendingDefault, pendingCompanion})
	require.NoError(t, err)

	defaultAfterPending := findLatestBalanceByKey(t, pendingResult.After, constant.DefaultBalanceKey)
	companionAfterPending := findBalanceByKey(t, pendingResult.After, constant.OverdraftBalanceKey)

	cancelDefault := pendingOverdraftBalanceOp(orgID, ledgerID, alias, constant.DefaultBalanceKey, constant.DirectionCredit,
		defaultAfterPending.Available, defaultAfterPending.OnHold, defaultAfterPending.OverdraftUsed, defaultAfterPending.Version, settings,
		constant.RELEASE, constant.CANCELED, decimal.NewFromInt(100), decimal.NewFromInt(50), false)
	cancelCompanion := pendingOverdraftBalanceOp(orgID, ledgerID, alias, constant.OverdraftBalanceKey, constant.DirectionDebit,
		companionAfterPending.Available, companionAfterPending.OnHold, companionAfterPending.OverdraftUsed, companionAfterPending.Version, nil,
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

	pendingDebit := pendingOverdraftBalanceOp(orgID, ledgerID, alias, constant.DefaultBalanceKey, constant.DirectionCredit,
		decimal.NewFromInt(50), decimal.Zero, decimal.Zero, 1, settings,
		constant.DEBIT, constant.PENDING, decimal.NewFromInt(100), decimal.Zero, true)
	pendingOnHold := pendingOverdraftBalanceOp(orgID, ledgerID, alias, constant.DefaultBalanceKey, constant.DirectionCredit,
		decimal.NewFromInt(50), decimal.Zero, decimal.Zero, 1, settings,
		constant.ONHOLD, constant.PENDING, decimal.NewFromInt(100), decimal.Zero, true)
	pendingCompanion := pendingOverdraftBalanceOp(orgID, ledgerID, alias, constant.OverdraftBalanceKey, constant.DirectionDebit,
		decimal.Zero, decimal.Zero, decimal.Zero, 1, nil,
		constant.DEBIT, constant.PENDING, decimal.NewFromInt(50), decimal.Zero, true)

	pendingResult, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.PENDING, true, []mmodel.BalanceOperation{pendingDebit, pendingOnHold, pendingCompanion})
	require.NoError(t, err)

	defaultAfterPending := findLatestBalanceByKey(t, pendingResult.After, constant.DefaultBalanceKey)
	companionAfterPending := findBalanceByKey(t, pendingResult.After, constant.OverdraftBalanceKey)

	cancelRelease := pendingOverdraftBalanceOp(orgID, ledgerID, alias, constant.DefaultBalanceKey, constant.DirectionCredit,
		defaultAfterPending.Available, defaultAfterPending.OnHold, defaultAfterPending.OverdraftUsed, defaultAfterPending.Version, settings,
		constant.RELEASE, constant.CANCELED, decimal.NewFromInt(100), decimal.Zero, true)
	cancelCredit := pendingOverdraftBalanceOp(orgID, ledgerID, alias, constant.DefaultBalanceKey, constant.DirectionCredit,
		defaultAfterPending.Available, defaultAfterPending.OnHold, defaultAfterPending.OverdraftUsed, defaultAfterPending.Version, settings,
		constant.CREDIT, constant.CANCELED, decimal.NewFromInt(100), decimal.NewFromInt(50), true)
	cancelCompanion := pendingOverdraftBalanceOp(orgID, ledgerID, alias, constant.OverdraftBalanceKey, constant.DirectionDebit,
		companionAfterPending.Available, companionAfterPending.OnHold, companionAfterPending.OverdraftUsed, companionAfterPending.Version, nil,
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
