//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	libConstants "github.com/LerianStudio/lib-commons/v6/commons/constants"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// findBalanceByAliasKey resolves a balance by its "<alias>#<key>" identity,
// which is what disambiguates the legs when a batch carries several balances
// sharing an alias or a balance key.
func findBalanceByAliasKey(t *testing.T, balances []*mmodel.Balance, aliasKey string) *mmodel.Balance {
	t.Helper()

	for _, balance := range balances {
		if balance != nil && balance.Alias == aliasKey {
			return balance
		}
	}

	require.Failf(t, "balance not found", "alias key %q not found", aliasKey)

	return nil
}

// TestIntegration_Overdraft_PendingDestinationCreditDefersRepayment locks the
// deferred-leg contract: a PENDING create carries the destination CREDIT into
// the atomic batch, but that credit only posts at commit. When the destination
// already carries outstanding overdraft the script must leave it completely
// alone — repaying against an Available the credit never reached drives the
// result negative, and the floor block then re-accrues the deficit on top of
// the outstanding OverdraftUsed, doubling it.
func TestIntegration_Overdraft_PendingDestinationCreditDefersRepayment(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	sourceAliasKey := "@pending-defer-src" + "#" + constant.DefaultBalanceKey
	destAliasKey := "@pending-defer-dst" + "#" + constant.DefaultBalanceKey

	destSettings := &mmodel.BalanceSettings{
		BalanceScope:          mmodel.BalanceScopeTransactional,
		AllowOverdraft:        true,
		OverdraftLimitEnabled: true,
		OverdraftLimit:        ptrString("100"),
	}

	sourceOp := pendingOverdraftBalanceOp(orgID, ledgerID, "@pending-defer-src", constant.DefaultBalanceKey, constant.DirectionCredit,
		decimal.NewFromInt(1000), decimal.Zero, decimal.Zero, 1, nil,
		constant.ONHOLD, constant.PENDING, decimal.NewFromInt(60), decimal.Zero, false)

	destOp := pendingOverdraftBalanceOp(orgID, ledgerID, "@pending-defer-dst", constant.DefaultBalanceKey, constant.DirectionCredit,
		decimal.Zero, decimal.Zero, decimal.NewFromInt(50), 1, destSettings,
		libConstants.CREDIT, constant.PENDING, decimal.NewFromInt(60), decimal.Zero, false)

	result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.PENDING, true, []mmodel.BalanceOperation{sourceOp, destOp})
	require.NoError(t, err)

	sourceAfter := findBalanceByAliasKey(t, result.After, sourceAliasKey)
	assert.True(t, sourceAfter.Available.Equal(decimal.NewFromInt(940)), "got %s", sourceAfter.Available.String())
	assert.True(t, sourceAfter.OnHold.Equal(decimal.NewFromInt(60)), "got %s", sourceAfter.OnHold.String())

	for _, balance := range result.After {
		if balance != nil {
			assert.NotEqual(t, destAliasKey, balance.Alias,
				"the deferred destination credit changed nothing, so it must not report a mutation")
		}
	}

	// An unchanged balance is deliberately absent from the atomic result, so the
	// cache entry is the only place its final values can be read back.
	destCached := readCachedBalance(t, infra, utils.BalanceInternalKey(orgID, ledgerID, destAliasKey))

	assert.Equal(t, "0", destCached.Available,
		"a pending create must not move the destination available")
	assert.Equal(t, "50", destCached.OverdraftUsed,
		"a pending create must neither repay nor re-accrue the destination overdraft")
	assert.Equal(t, int64(1), destCached.Version, "an untouched balance must not consume a version")
}
