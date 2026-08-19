// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

func TestBuildExpectedEconomicPlan_PreservesResolvedMultiset(t *testing.T) {
	t.Parallel()

	balance := &Balance{ID: "balance-1", Key: "default", AccountID: "account-1", AssetCode: "USD", Direction: constant.DirectionCredit}
	duplicate := BalanceOperation{
		Balance: balance, Alias: "0#@payer#default", InternalKey: "balance-key",
		EconomicSide: EconomicSideSource, EconomicRole: EconomicRoleFee,
		Amount: mtransaction.Amount{Asset: "USD", Value: decimal.RequireFromString("01.000"), Operation: constant.DEBIT, Direction: constant.DirectionDebit},
	}
	operations := []BalanceOperation{duplicate, duplicate}
	plan, err := BuildExpectedEconomicPlan(operations, constant.CREATED, false, "")
	require.NoError(t, err)
	require.Len(t, plan.Legs, 2)
	assert.NotEqual(t, plan.Legs[0].Identity, plan.Legs[1].Identity)
	assert.Equal(t, "1", plan.Legs[0].Amount)
	assert.Equal(t, EconomicRoleFee, plan.Legs[0].Role)
	require.NoError(t, ValidateExpectedEconomicPlan(plan))

	reordered, err := BuildExpectedEconomicPlan([]BalanceOperation{operations[1], operations[0]}, constant.CREATED, false, "")
	require.NoError(t, err)
	assert.Equal(t, plan.Digest, reordered.Digest)
}

func TestBuildExpectedEconomicPlan_DistinguishesBalanceKeyRoleAndMultiplicity(t *testing.T) {
	t.Parallel()

	build := func(key, role string) BalanceOperation {
		return BalanceOperation{
			Balance: &Balance{ID: "balance-" + key, Key: key, AccountID: "account", AssetCode: "USD", Direction: constant.DirectionCredit},
			Alias:   "0#@same#" + key, InternalKey: "redis-" + key, EconomicSide: EconomicSideSource, EconomicRole: role,
			Amount: mtransaction.Amount{Asset: "USD", Value: decimal.NewFromInt(2), Operation: constant.DEBIT, Direction: constant.DirectionDebit},
		}
	}
	base, err := BuildExpectedEconomicPlan([]BalanceOperation{build("default", EconomicRolePrimary)}, constant.CREATED, false, "")
	require.NoError(t, err)
	differentKey, err := BuildExpectedEconomicPlan([]BalanceOperation{build("fees", EconomicRolePrimary)}, constant.CREATED, false, "")
	require.NoError(t, err)
	differentRole, err := BuildExpectedEconomicPlan([]BalanceOperation{build("default", EconomicRoleFee)}, constant.CREATED, false, "")
	require.NoError(t, err)
	duplicate, err := BuildExpectedEconomicPlan([]BalanceOperation{build("default", EconomicRolePrimary), build("default", EconomicRolePrimary)}, constant.CREATED, false, "")
	require.NoError(t, err)

	assert.NotEqual(t, base.Digest, differentKey.Digest)
	assert.NotEqual(t, base.Digest, differentRole.Digest)
	assert.NotEqual(t, base.Digest, duplicate.Digest)
}

func TestBuildExpectedEconomicPlan_ExactDecimalAndInvalidity(t *testing.T) {
	t.Parallel()

	build := func(value string) BalanceOperation {
		return BalanceOperation{
			Balance: &Balance{ID: "balance", Key: "default", AccountID: "account", AssetCode: "USD", Direction: constant.DirectionCredit},
			Alias:   "0#@payer#default", InternalKey: "redis-balance", EconomicSide: EconomicSideSource,
			Amount: mtransaction.Amount{Asset: "USD", Value: decimal.RequireFromString(value), Operation: constant.DEBIT, Direction: constant.DirectionDebit},
		}
	}

	for _, equivalent := range []string{"1", "1.0", "01.000", "1e0"} {
		plan, err := BuildExpectedEconomicPlan([]BalanceOperation{build(equivalent)}, constant.CREATED, false, "")
		require.NoError(t, err)
		canonical, err := BuildExpectedEconomicPlan([]BalanceOperation{build("1")}, constant.CREATED, false, "")
		require.NoError(t, err)
		assert.Equal(t, canonical.Digest, plan.Digest, equivalent)
	}
	for _, distinct := range []string{"9007199254740991", "9007199254740992", "9007199254740993", "0.000000000000000000000000000000000001"} {
		plan, err := BuildExpectedEconomicPlan([]BalanceOperation{build(distinct)}, constant.CREATED, false, "")
		require.NoError(t, err)
		assert.Equal(t, decimal.RequireFromString(distinct).String(), plan.Legs[0].Amount)
	}
	for _, invalid := range []string{"0", "-0", "-1"} {
		_, err := BuildExpectedEconomicPlan([]BalanceOperation{build(invalid)}, constant.CREATED, false, "")
		assert.Error(t, err, invalid)
	}
}

func TestValidateExpectedEconomicPlan_RejectsMutationAndDuplicateIdentity(t *testing.T) {
	t.Parallel()

	operation := BalanceOperation{
		Balance: &Balance{ID: "balance", Key: "default", AccountID: "account", AssetCode: "USD", Direction: constant.DirectionCredit},
		Alias:   "0#@payer#default", InternalKey: "redis-balance", EconomicSide: EconomicSideSource,
		Amount: mtransaction.Amount{Asset: "USD", Value: decimal.NewFromInt(10), Operation: constant.DEBIT, Direction: constant.DirectionDebit},
	}
	plan, err := BuildExpectedEconomicPlan([]BalanceOperation{operation, operation}, constant.CREATED, false, "")
	require.NoError(t, err)

	malicious := *plan
	malicious.Legs = append([]ExpectedEconomicLeg(nil), plan.Legs...)
	malicious.Legs[0].Amount = "11"
	assert.Error(t, ValidateExpectedEconomicPlan(&malicious))

	duplicate := *plan
	duplicate.Legs = append([]ExpectedEconomicLeg(nil), plan.Legs...)
	duplicate.Legs[1].Identity = duplicate.Legs[0].Identity
	duplicate.Digest, err = expectedEconomicPlanDigest(duplicate.Version, duplicate.Legs)
	require.NoError(t, err)
	assert.Error(t, ValidateExpectedEconomicPlan(&duplicate))
}
