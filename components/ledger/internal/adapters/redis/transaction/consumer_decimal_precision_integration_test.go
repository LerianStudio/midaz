//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"
)

func TestIntegration_BalanceAtomicOperation_ExactDecimalMoneyPath(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupFinancialRedisIntegrationInfra(t)
	ctx := context.Background()

	t.Run("fractional underflow fails closed", func(t *testing.T) {
		organizationID := uuid.New()
		ledgerID := uuid.New()
		op := redistestutil.CreateBalanceOperationWithAvailable(
			organizationID, ledgerID, "@fractional-underflow", "USD", constant.DEBIT,
			decimal.RequireFromString("1.000000000000000001"),
			decimal.RequireFromString("1.000000000000000000"), "deposit",
		)

		_, err := infra.repo.ProcessBalanceAtomicOperation(
			ctx, organizationID, ledgerID, uuid.New(), constant.APPROVED, false,
			[]mmodel.BalanceOperation{op},
		)

		require.Error(t, err)
		assert.Contains(t, err.Error(), constant.ErrInsufficientFunds.Error())
		assertCachedExactBalance(t, infra, op.InternalKey, "1", "0", 1)
	})

	t.Run("integer underflow beyond binary float precision fails closed", func(t *testing.T) {
		organizationID := uuid.New()
		ledgerID := uuid.New()
		op := redistestutil.CreateBalanceOperationWithAvailable(
			organizationID, ledgerID, "@integer-underflow", "USD", constant.DEBIT,
			decimal.RequireFromString("9007199254740993"),
			decimal.RequireFromString("9007199254740992"), "deposit",
		)

		_, err := infra.repo.ProcessBalanceAtomicOperation(
			ctx, organizationID, ledgerID, uuid.New(), constant.APPROVED, false,
			[]mmodel.BalanceOperation{op},
		)

		require.Error(t, err)
		assert.Contains(t, err.Error(), constant.ErrInsufficientFunds.Error())
		assertCachedExactBalance(t, infra, op.InternalKey, "9007199254740992", "0", 1)
	})

	t.Run("external negative result stays exact", func(t *testing.T) {
		organizationID := uuid.New()
		ledgerID := uuid.New()
		op := redistestutil.CreateBalanceOperationWithAvailable(
			organizationID, ledgerID, "@external-negative", "USD", constant.DEBIT,
			decimal.RequireFromString("1.000000000000000001"),
			decimal.RequireFromString("1.000000000000000000"), constant.ExternalAccountType,
		)

		result, err := infra.repo.ProcessBalanceAtomicOperation(
			ctx, organizationID, ledgerID, uuid.New(), constant.APPROVED, false,
			[]mmodel.BalanceOperation{op},
		)

		require.NoError(t, err)
		require.Len(t, result.After, 1)
		assert.True(t, result.After[0].Available.Equal(decimal.RequireFromString("-0.000000000000000001")))
		assertCachedExactBalance(t, infra, op.InternalKey, "-0.000000000000000001", "0", 2)
	})

	t.Run("overdraft limit rejects one fractional quantum above boundary", func(t *testing.T) {
		organizationID := uuid.New()
		ledgerID := uuid.New()
		limit := "1.000000000000000000"
		settings := &mmodel.BalanceSettings{
			BalanceScope:          mmodel.BalanceScopeTransactional,
			AllowOverdraft:        true,
			OverdraftLimitEnabled: true,
			OverdraftLimit:        &limit,
		}
		op := overdraftOp(
			organizationID, ledgerID, "@exact-overdraft-limit", "deposit", "credit",
			decimal.Zero, decimal.RequireFromString("1.000000000000000000"), 1, settings,
			constant.DEBIT, decimal.RequireFromString("0.000000000000000001"),
		)

		_, err := infra.repo.ProcessBalanceAtomicOperation(
			ctx, organizationID, ledgerID, uuid.New(), constant.APPROVED, false,
			[]mmodel.BalanceOperation{op},
		)

		require.Error(t, err)
		assert.Contains(t, err.Error(), constant.ErrOverdraftLimitExceeded.Error())
		assertCachedExactBalance(t, infra, op.InternalKey, "0", "1", 1)
	})
}

func TestIntegration_BalanceAtomicOperation_ConcurrentExactDebitsCannotCreateMoney(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupFinancialRedisIntegrationInfra(t)
	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	initial := decimal.RequireFromString("1.000000000000000000")
	amount := decimal.RequireFromString("0.500000000000000001")
	expectedFinal := decimal.RequireFromString("0.499999999999999999")
	base := redistestutil.CreateBalanceOperationWithAvailable(
		organizationID, ledgerID, "@concurrent-exact", "USD", constant.DEBIT,
		amount, initial, "deposit",
	)

	operations := make([]mmodel.BalanceOperation, 2)
	for index := range operations {
		operations[index] = base
		balance := *base.Balance
		operations[index].Balance = &balance
	}

	start := make(chan struct{})
	results := make(chan error, len(operations))
	var workers sync.WaitGroup
	workers.Add(len(operations))

	for index := range operations {
		go func(operation mmodel.BalanceOperation) {
			defer workers.Done()
			<-start
			_, err := infra.repo.ProcessBalanceAtomicOperation(
				ctx, organizationID, ledgerID, uuid.New(), constant.APPROVED, false,
				[]mmodel.BalanceOperation{operation},
			)
			results <- err
		}(operations[index])
	}

	close(start)
	workers.Wait()
	close(results)

	succeeded := 0
	rejected := 0
	for err := range results {
		if err == nil {
			succeeded++
			continue
		}
		assert.Contains(t, err.Error(), constant.ErrInsufficientFunds.Error())
		rejected++
	}

	assert.Equal(t, 1, succeeded, "only one exact debit can consume the available funds")
	assert.Equal(t, 1, rejected, "the competing exact debit must fail closed")
	assertCachedExactBalance(t, infra, base.InternalKey, expectedFinal.String(), "0", 2)
	assert.True(t, initial.Sub(expectedFinal).Equal(amount),
		"the committed balance delta must equal exactly one debit")
}

func assertCachedExactBalance(
	t *testing.T,
	infra *integrationTestInfra,
	key, available, overdraftUsed string,
	version int64,
) {
	t.Helper()

	raw, err := infra.redisContainer.Client.Get(context.Background(), key).Result()
	require.NoError(t, err)

	var balance cachedBalance
	require.NoError(t, json.Unmarshal([]byte(raw), &balance))
	assert.Equal(t, available, balance.Available)
	assert.Equal(t, overdraftUsed, balance.OverdraftUsed)
	assert.Equal(t, version, balance.Version)
}
