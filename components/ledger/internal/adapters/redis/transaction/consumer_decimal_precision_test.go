// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	redislib "github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

func newExactDecimalUnitRepository(t *testing.T) (*RedisConsumerRepository, *redislib.Client) {
	t.Helper()

	server := miniredis.RunT(t)
	client := redislib.NewClient(&redislib.Options{Addr: server.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })

	return &RedisConsumerRepository{conn: &staticRedisProvider{client: client}}, client
}

func TestBalanceAtomicOperation_ExactDecimalUnderflowFailsClosed(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name      string
		available string
		amount    string
	}{
		{
			name:      "fraction beyond binary float precision",
			available: "1.000000000000000000",
			amount:    "1.000000000000000001",
		},
		{
			name:      "integer beyond binary float precision",
			available: "9007199254740992",
			amount:    "9007199254740993",
		},
		{
			name:      "eighty digit integer",
			available: strings.Repeat("9", 80),
			amount:    "1" + strings.Repeat("0", 80),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			repo, client := newExactDecimalUnitRepository(t)
			organizationID := uuid.New()
			ledgerID := uuid.New()
			op := createBalanceOperation(
				organizationID,
				ledgerID,
				"@exact-underflow",
				"USD",
				constant.DEBIT,
				decimal.RequireFromString(testCase.amount),
				decimal.RequireFromString(testCase.available),
			)

			_, err := repo.ProcessBalanceAtomicOperation(
				context.Background(), organizationID, ledgerID, uuid.New(),
				constant.APPROVED, false, []mmodel.BalanceOperation{op},
			)

			require.Error(t, err)
			assert.Contains(t, err.Error(), constant.ErrInsufficientFunds.Error())

			raw, getErr := client.Get(context.Background(), op.InternalKey).Result()
			require.NoError(t, getErr)
			assert.Contains(t, raw, `"Available":"`+decimal.RequireFromString(testCase.available).String()+`"`)
			assert.Contains(t, raw, `"Version":1`)
		})
	}
}

func TestBalanceAtomicOperation_ExactDecimalNormalizesCachedScale(t *testing.T) {
	t.Parallel()

	repo, client := newExactDecimalUnitRepository(t)
	organizationID := uuid.New()
	ledgerID := uuid.New()
	op := createBalanceOperation(
		organizationID,
		ledgerID,
		"@normalized-scale",
		"USD",
		constant.DEBIT,
		decimal.RequireFromString("0.2300"),
		decimal.RequireFromString("1.2300"),
	)

	cached := map[string]any{
		"ID": op.Balance.ID, "Available": "0001.2300", "OnHold": "-0.0000", "Version": 1,
		"AccountType": "deposit", "AccountID": op.Balance.AccountID, "AssetCode": "USD",
		"AllowSending": 1, "AllowReceiving": 1, "Key": op.Balance.Key, "Direction": "credit",
		"OverdraftUsed": "000.0000", "AllowOverdraft": 0, "OverdraftLimitEnabled": 0,
		"OverdraftLimit": "000.0000", "BalanceScope": mmodel.BalanceScopeTransactional,
	}
	raw, err := json.Marshal(cached)
	require.NoError(t, err)
	require.NoError(t, client.Set(context.Background(), op.InternalKey, raw, 0).Err())

	result, err := repo.ProcessBalanceAtomicOperation(
		context.Background(), organizationID, ledgerID, uuid.New(),
		constant.APPROVED, false, []mmodel.BalanceOperation{op},
	)

	require.NoError(t, err)
	require.Len(t, result.After, 1)
	assert.True(t, result.After[0].Available.Equal(decimal.NewFromInt(1)))
	assert.True(t, result.After[0].OnHold.IsZero())
	assert.True(t, result.After[0].OverdraftUsed.IsZero())

	stored, getErr := client.Get(context.Background(), op.InternalKey).Result()
	require.NoError(t, getErr)
	assert.Contains(t, stored, `"Available":"1"`)
	assert.Contains(t, stored, `"OnHold":"-0.0000"`, "untouched fields retain their stored representation")
}

func TestBalanceAtomicOperation_ExactDecimalArithmeticPreservesSignAndScale(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		accountType string
		operation   string
		available   string
		amount      string
		expected    string
	}{
		{
			name:        "one fractional quantum remains",
			accountType: "deposit",
			operation:   constant.DEBIT,
			available:   "1.000000000000000001",
			amount:      "1.000000000000000000",
			expected:    "0.000000000000000001",
		},
		{
			name:        "external balance carries exact negative quantum",
			accountType: constant.ExternalAccountType,
			operation:   constant.DEBIT,
			available:   "1.000000000000000000",
			amount:      "1.000000000000000001",
			expected:    "-0.000000000000000001",
		},
		{
			name:        "credit reduces an external negative balance exactly",
			accountType: constant.ExternalAccountType,
			operation:   constant.CREDIT,
			available:   "-1.000000000000000001",
			amount:      "1.000000000000000000",
			expected:    "-0.000000000000000001",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			repo, _ := newExactDecimalUnitRepository(t)
			organizationID := uuid.New()
			ledgerID := uuid.New()
			op := createBalanceOperation(
				organizationID,
				ledgerID,
				"@exact-result",
				"USD",
				testCase.operation,
				decimal.RequireFromString(testCase.amount),
				decimal.RequireFromString(testCase.available),
			)
			op.Balance.AccountType = testCase.accountType

			result, err := repo.ProcessBalanceAtomicOperation(
				context.Background(), organizationID, ledgerID, uuid.New(),
				constant.APPROVED, false, []mmodel.BalanceOperation{op},
			)

			require.NoError(t, err)
			require.Len(t, result.After, 1)
			assert.True(t, result.After[0].Available.Equal(decimal.RequireFromString(testCase.expected)),
				"expected exact result %s, got %s", testCase.expected, result.After[0].Available)
		})
	}
}
