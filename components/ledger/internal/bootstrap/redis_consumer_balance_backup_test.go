// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

func TestBalanceFromBackup_PreservesOverdraftAuditState(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	limit := "500"
	balance, err := balanceFromBackup(mmodel.BalanceRedis{
		ID:                    uuid.NewString(),
		AccountID:             uuid.NewString(),
		Alias:                 "0#@source#default",
		AssetCode:             "USD",
		Available:             decimal.NewFromInt(-125),
		Version:               9,
		Direction:             constant.DirectionDebit,
		OverdraftUsed:         "125",
		AllowOverdraft:        1,
		OverdraftLimitEnabled: 1,
		OverdraftLimit:        limit,
		BalanceScope:          mmodel.BalanceScopeInternal,
	}, organizationID, ledgerID)

	require.NoError(t, err)
	assert.Equal(t, constant.DefaultBalanceKey, balance.Key)
	assert.Equal(t, constant.DirectionDebit, balance.Direction)
	assert.True(t, decimal.NewFromInt(125).Equal(balance.OverdraftUsed))
	require.NotNil(t, balance.Settings)
	assert.True(t, balance.Settings.AllowOverdraft)
	assert.True(t, balance.Settings.OverdraftLimitEnabled)
	require.NotNil(t, balance.Settings.OverdraftLimit)
	assert.Equal(t, limit, *balance.Settings.OverdraftLimit)
	assert.Equal(t, mmodel.BalanceScopeInternal, balance.Settings.BalanceScope)
	assert.Equal(t, organizationID.String(), balance.OrganizationID)
	assert.Equal(t, ledgerID.String(), balance.LedgerID)
}
