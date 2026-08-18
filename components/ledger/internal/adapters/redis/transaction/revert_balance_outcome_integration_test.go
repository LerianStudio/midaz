//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"
)

// TestIntegration_RevertBalanceOutcomeIsAtomic proves the recovery signal used
// after a lost Lua response is committed with the balance movement itself. A
// successful mutation always leaves its transaction backup marker; a
// script-declared rejection leaves neither marker nor partial balance write.
func TestIntegration_RevertBalanceOutcomeIsAtomic(t *testing.T) {
	t.Setenv("ALLOW_INSECURE_TLS", "true")
	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	successID := uuid.New()
	successOps := []mmodel.BalanceOperation{redistestutil.CreateBalanceOperationWithAvailable(
		organizationID, ledgerID, "@revert-outcome-success", "USD", constant.DEBIT,
		decimal.NewFromInt(100), decimal.NewFromInt(1000), "deposit",
	)}
	result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, organizationID, ledgerID, successID, constant.CREATED, false, successOps)
	require.NoError(t, err)
	require.Len(t, result.After, 1)
	assert.True(t, decimal.NewFromInt(900).Equal(result.After[0].Available))

	outcome, err := infra.repo.ReadMessageFromQueue(ctx, utils.TransactionInternalKey(organizationID, ledgerID, successID.String()))
	require.NoError(t, err)
	assert.NotEmpty(t, outcome, "a committed balance movement must have its atomic recovery marker")

	rejectedID := uuid.New()
	rejectedOps := []mmodel.BalanceOperation{redistestutil.CreateBalanceOperationWithAvailable(
		organizationID, ledgerID, "@revert-outcome-rejected", "USD", constant.DEBIT,
		decimal.NewFromInt(100), decimal.NewFromInt(50), "deposit",
	)}
	_, err = infra.repo.ProcessBalanceAtomicOperation(ctx, organizationID, ledgerID, rejectedID, constant.CREATED, false, rejectedOps)
	require.Error(t, err)
	assert.Contains(t, err.Error(), constant.ErrInsufficientFunds.Error())

	_, err = infra.repo.ReadMessageFromQueue(ctx, utils.TransactionInternalKey(organizationID, ledgerID, rejectedID.String()))
	assert.ErrorIs(t, err, redis.Nil, "a rolled-back Lua rejection must not publish a recovery marker")
	rejectedBalance, err := infra.repo.Get(ctx, rejectedOps[0].InternalKey)
	require.NoError(t, err)
	assert.Contains(t, rejectedBalance, `"Available":"50"`,
		"a rolled-back Lua rejection may seed the input balance but must preserve its pre-mutation amount")
}
