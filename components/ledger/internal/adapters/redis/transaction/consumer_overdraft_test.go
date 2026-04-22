// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"strconv"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// ARGV Builder — overdraft field propagation
// =============================================================================

func TestBuildPlan_IncludesOverdraftFields(t *testing.T) {
	t.Parallel()

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	overdraftLimit := "500.00"
	balanceOps := []mmodel.BalanceOperation{
		{
			Balance: &mmodel.Balance{
				ID:             uuid.Must(libCommons.GenerateUUIDv7()).String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				AccountID:      uuid.Must(libCommons.GenerateUUIDv7()).String(),
				Alias:          "@sender",
				Key:            "default",
				AssetCode:      "USD",
				Available:      decimal.NewFromInt(1000),
				OnHold:         decimal.Zero,
				Version:        3,
				AccountType:    "deposit",
				AllowSending:   true,
				AllowReceiving: true,
				Direction:      "debit",
				OverdraftUsed:  decimal.NewFromInt(50),
				Settings: &mmodel.BalanceSettings{
					AllowOverdraft:        true,
					OverdraftLimitEnabled: true,
					OverdraftLimit:        &overdraftLimit,
					BalanceScope:          mmodel.BalanceScopeTransactional,
				},
			},
			Alias: "@sender",
			Amount: mtransaction.Amount{
				Asset:     "USD",
				Value:     decimal.NewFromInt(200),
				Operation: constant.DEBIT,
			},
			InternalKey: utils.BalanceInternalKey(organizationID, ledgerID, "default"),
		},
	}

	repo := &RedisConsumerRepository{conn: newFailOnCallConnection(t)}

	plan, err := repo.buildBalanceAtomicOperationPlan(
		t.Context(), constant.APPROVED, false, balanceOps,
	)
	require.NoError(t, err)
	require.NotNil(t, plan)
	require.Len(t, plan.args, 23,
		"ARGV must contain 23 entries per balance (groupSize=23)")

	assert.Equal(t, "debit", plan.args[17], "ARGV[i+17] balance.Direction")
	assert.Equal(t, "50", plan.args[18], "ARGV[i+18] balance.OverdraftUsed")
	assert.Equal(t, 1, plan.args[19], "ARGV[i+19] AllowOverdraft (1=true)")
	assert.Equal(t, 1, plan.args[20], "ARGV[i+20] OverdraftLimitEnabled (1=true)")
	assert.Equal(t, "500.00", plan.args[21], "ARGV[i+21] OverdraftLimit")
	assert.Equal(t, mmodel.BalanceScopeTransactional, plan.args[22], "ARGV[i+22] BalanceScope")
}

func TestBuildPlan_DefaultOverdraftFields(t *testing.T) {
	t.Parallel()

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())

	balanceOps := []mmodel.BalanceOperation{
		{
			Balance: &mmodel.Balance{
				ID:             uuid.Must(libCommons.GenerateUUIDv7()).String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				AccountID:      uuid.Must(libCommons.GenerateUUIDv7()).String(),
				Alias:          "@receiver",
				Key:            "default",
				AssetCode:      "USD",
				Available:      decimal.NewFromInt(500),
				OnHold:         decimal.Zero,
				Version:        1,
				AccountType:    "deposit",
				AllowSending:   true,
				AllowReceiving: true,
			},
			Alias: "@receiver",
			Amount: mtransaction.Amount{
				Asset:     "USD",
				Value:     decimal.NewFromInt(100),
				Operation: constant.CREDIT,
			},
			InternalKey: utils.BalanceInternalKey(organizationID, ledgerID, "default"),
		},
	}

	repo := &RedisConsumerRepository{conn: newFailOnCallConnection(t)}

	plan, err := repo.buildBalanceAtomicOperationPlan(
		t.Context(), constant.APPROVED, false, balanceOps,
	)
	require.NoError(t, err)
	require.NotNil(t, plan)
	require.Len(t, plan.args, 23,
		"ARGV must contain 23 entries even when overdraft fields are defaults")

	dirVal, ok := plan.args[17].(string)
	require.True(t, ok, "ARGV[i+17] (Direction) must be a string")
	assert.Contains(t, []string{"", "credit"}, dirVal,
		"ARGV[i+17] default Direction")

	assert.Equal(t, "0", plan.args[18], "ARGV[i+18] zero OverdraftUsed")
	assert.Equal(t, 0, plan.args[19], "ARGV[i+19] AllowOverdraft=false")
	assert.Equal(t, 0, plan.args[20], "ARGV[i+20] OverdraftLimitEnabled=false")
	assert.Equal(t, "0", plan.args[21], "ARGV[i+21] zero OverdraftLimit")
	assert.Equal(t, mmodel.BalanceScopeTransactional, plan.args[22],
		"ARGV[i+22] default BalanceScope")
}

func TestBuildPlan_GroupSizeMatchesLua(t *testing.T) {
	t.Parallel()

	assert.Equal(t, 23, luaArgsPerOperation,
		"luaArgsPerOperation must be 23 to include the 6 new overdraft ARGV fields")
}

func TestBuildPlan_MultipleBalancesOverdraftPositions(t *testing.T) {
	t.Parallel()

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	limit := "1000.00"

	balanceOps := []mmodel.BalanceOperation{
		{
			Balance: &mmodel.Balance{
				ID:             uuid.Must(libCommons.GenerateUUIDv7()).String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				AccountID:      uuid.Must(libCommons.GenerateUUIDv7()).String(),
				Alias:          "@first",
				Key:            "default",
				AssetCode:      "BRL",
				Available:      decimal.NewFromInt(100),
				OnHold:         decimal.Zero,
				Version:        1,
				AccountType:    "deposit",
				AllowSending:   true,
				AllowReceiving: true,
				Direction:      "credit",
				OverdraftUsed:  decimal.Zero,
			},
			Alias: "@first",
			Amount: mtransaction.Amount{
				Asset:     "BRL",
				Value:     decimal.NewFromInt(10),
				Operation: constant.DEBIT,
			},
			InternalKey: utils.BalanceInternalKey(organizationID, ledgerID, "default"),
		},
		{
			Balance: &mmodel.Balance{
				ID:             uuid.Must(libCommons.GenerateUUIDv7()).String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				AccountID:      uuid.Must(libCommons.GenerateUUIDv7()).String(),
				Alias:          "@second",
				Key:            "default",
				AssetCode:      "BRL",
				Available:      decimal.NewFromInt(200),
				OnHold:         decimal.Zero,
				Version:        2,
				AccountType:    "deposit",
				AllowSending:   true,
				AllowReceiving: true,
				Direction:      "debit",
				OverdraftUsed:  decimal.NewFromInt(75),
				Settings: &mmodel.BalanceSettings{
					AllowOverdraft:        true,
					OverdraftLimitEnabled: true,
					OverdraftLimit:        &limit,
					BalanceScope:          mmodel.BalanceScopeInternal,
				},
			},
			Alias: "@second",
			Amount: mtransaction.Amount{
				Asset:     "BRL",
				Value:     decimal.NewFromInt(10),
				Operation: constant.CREDIT,
			},
			InternalKey: utils.BalanceInternalKey(organizationID, ledgerID, "default"),
		},
	}

	repo := &RedisConsumerRepository{conn: newFailOnCallConnection(t)}

	plan, err := repo.buildBalanceAtomicOperationPlan(
		t.Context(), constant.APPROVED, false, balanceOps,
	)
	require.NoError(t, err)
	require.Len(t, plan.args, 46, "Two operations × 23 fields = 46 ARGV entries")

	secondBase := 23
	assert.Equal(t, "debit", plan.args[secondBase+17], "2nd balance Direction")
	assert.Equal(t, "75", plan.args[secondBase+18], "2nd balance OverdraftUsed")
	assert.Equal(t, 1, plan.args[secondBase+19], "2nd balance AllowOverdraft")
	assert.Equal(t, 1, plan.args[secondBase+20], "2nd balance OverdraftLimitEnabled")
	assert.Equal(t, "1000.00", plan.args[secondBase+21], "2nd balance OverdraftLimit")
	assert.Equal(t, mmodel.BalanceScopeInternal, plan.args[secondBase+22], "2nd balance BalanceScope")
}

func TestBuildPlan_ExistingFieldPositionsUnchanged(t *testing.T) {
	t.Parallel()

	organizationID := uuid.Must(libCommons.GenerateUUIDv7())
	ledgerID := uuid.Must(libCommons.GenerateUUIDv7())
	balanceID := uuid.Must(libCommons.GenerateUUIDv7()).String()
	accountID := uuid.Must(libCommons.GenerateUUIDv7()).String()

	balanceOps := []mmodel.BalanceOperation{
		{
			Balance: &mmodel.Balance{
				ID:             balanceID,
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				AccountID:      accountID,
				Alias:          "@test",
				Key:            "default",
				AssetCode:      "USD",
				Available:      decimal.NewFromInt(750),
				OnHold:         decimal.NewFromInt(25),
				Version:        5,
				AccountType:    "deposit",
				AllowSending:   true,
				AllowReceiving: false,
			},
			Alias: "@test",
			Amount: mtransaction.Amount{
				Asset:                  "USD",
				Value:                  decimal.NewFromInt(100),
				Operation:              constant.DEBIT,
				RouteValidationEnabled: true,
			},
			InternalKey: utils.BalanceInternalKey(organizationID, ledgerID, "default"),
		},
	}

	repo := &RedisConsumerRepository{conn: newFailOnCallConnection(t)}

	plan, err := repo.buildBalanceAtomicOperationPlan(
		t.Context(), constant.APPROVED, true, balanceOps,
	)
	require.NoError(t, err)
	require.NotNil(t, plan)

	// Positions 0-16 must remain unchanged from the original layout.
	assert.Equal(t, 1, plan.args[1], "ARGV[i+1] isPending")
	assert.Equal(t, constant.APPROVED, plan.args[2], "ARGV[i+2] transactionStatus")
	assert.Equal(t, constant.DEBIT, plan.args[3], "ARGV[i+3] operation")
	assert.Equal(t, "100", plan.args[4], "ARGV[i+4] amount")
	assert.Equal(t, "@test", plan.args[5], "ARGV[i+5] alias")
	assert.Equal(t, 1, plan.args[6], "ARGV[i+6] routeValidationEnabled")
	assert.Equal(t, balanceID, plan.args[7], "ARGV[i+7] balance.ID")
	assert.Equal(t, "750", plan.args[8], "ARGV[i+8] balance.Available")
	assert.Equal(t, "25", plan.args[9], "ARGV[i+9] balance.OnHold")
	assert.Equal(t, strconv.FormatInt(5, 10), plan.args[10], "ARGV[i+10] balance.Version")
	assert.Equal(t, "deposit", plan.args[11], "ARGV[i+11] balance.AccountType")
	assert.Equal(t, accountID, plan.args[12], "ARGV[i+12] balance.AccountID")
	assert.Equal(t, "USD", plan.args[13], "ARGV[i+13] balance.AssetCode")
	assert.Equal(t, 1, plan.args[14], "ARGV[i+14] balance.AllowSending")
	assert.Equal(t, 0, plan.args[15], "ARGV[i+15] balance.AllowReceiving")
	assert.Equal(t, "default", plan.args[16], "ARGV[i+16] balance.Key")
}
