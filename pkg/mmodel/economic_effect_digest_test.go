// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedisEconomicEffectDigest_NormalizesExactDecimalsAndPreservesMultisets(t *testing.T) {
	t.Parallel()

	operation := completeDigestOperation("operation-1", "9007199254740992", "1.0")
	balance := completeDigestBalance("balance-1", "0001.000", "-0.000")
	want, err := RedisEconomicEffectDigest([]OperationRedis{operation}, []BalanceRedis{balance})
	require.NoError(t, err)

	equivalentOperation := operation
	equivalentOperation.Snapshot.OverdraftUsedBefore = "01.0000"
	equivalentBalance := balance
	equivalentBalance.OverdraftUsed = "1e0"
	equivalentBalance.OverdraftLimit = "0"
	got, err := RedisEconomicEffectDigest([]OperationRedis{equivalentOperation}, []BalanceRedis{equivalentBalance})
	require.NoError(t, err)
	assert.Equal(t, want, got)

	for _, distinct := range []string{"9007199254740991", "9007199254740993", "9007199254740992000000000000000000000000"} {
		candidate := operation
		candidate.AmountValue = decimal.RequireFromString(distinct)
		digest, digestErr := RedisEconomicEffectDigest([]OperationRedis{candidate}, []BalanceRedis{balance})
		require.NoError(t, digestErr)
		assert.NotEqual(t, want, digest)
	}

	duplicateOperations := []OperationRedis{operation, operation, completeDigestOperation("operation-2", "3", "0")}
	duplicateBalances := []BalanceRedis{balance, balance, completeDigestBalance("balance-2", "4", "0")}
	ordered, err := RedisEconomicEffectDigest(duplicateOperations, duplicateBalances)
	require.NoError(t, err)
	reordered, err := RedisEconomicEffectDigest(
		[]OperationRedis{duplicateOperations[2], duplicateOperations[0], duplicateOperations[1]},
		[]BalanceRedis{duplicateBalances[2], duplicateBalances[1], duplicateBalances[0]},
	)
	require.NoError(t, err)
	assert.Equal(t, ordered, reordered)
	withoutDuplicate, err := RedisEconomicEffectDigest(duplicateOperations[1:], duplicateBalances[1:])
	require.NoError(t, err)
	assert.NotEqual(t, ordered, withoutDuplicate)
}

func TestCanonicalEconomicDecimal_MatchesLedgerDecimalGrammar(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		left  string
		right string
	}{
		{left: "1", right: "1.0"},
		{left: "01.000", right: "1e0"},
		{left: "-0", right: "0"},
		{left: "-123.4500", right: "-1.2345e2"},
		{left: "1e-1000", right: "0." + strings.Repeat("0", 999) + "1"},
		{left: "99999999999999999999999999999999999999999999999999.0", right: "9.9999999999999999999999999999999999999999999999999e49"},
	} {
		left, err := canonicalEconomicDecimal(test.left)
		require.NoError(t, err)
		right, err := canonicalEconomicDecimal(test.right)
		require.NoError(t, err)
		assert.Equal(t, left, right)
	}

	for _, invalid := range []string{"", ".", "1e", "NaN", "Inf", "1e999999999999999999999999999999999999"} {
		_, err := canonicalEconomicDecimal(invalid)
		require.Error(t, err)
	}
}

func TestRedisEconomicEffectDigest_RejectsMalformedDecimals(t *testing.T) {
	t.Parallel()

	operation := completeDigestOperation("operation", "1", "0")
	balance := completeDigestBalance("balance", "1", "0")

	operation.Snapshot.OverdraftUsedAfter = "not-a-decimal"
	_, err := RedisEconomicEffectDigest([]OperationRedis{operation}, []BalanceRedis{balance})
	require.Error(t, err)

	operation = completeDigestOperation("operation", "1", "0")
	balance.OverdraftLimit = "1e999999999999999999999999999999999999"
	_, err = RedisEconomicEffectDigest([]OperationRedis{operation}, []BalanceRedis{balance})
	require.Error(t, err)
}

func completeDigestOperation(id, amount, overdraft string) OperationRedis {
	return OperationRedis{
		ID: id, TransactionID: "transaction", BalanceID: "balance", BalanceKey: "default",
		AccountID: "account", OrganizationID: "organization", LedgerID: "ledger", Type: "DEBIT",
		Direction: "debit", AssetCode: "USD", AmountValue: decimal.RequireFromString(amount),
		BalanceAvailable: decimal.NewFromInt(10), BalanceOnHold: decimal.Zero, BalanceVersion: 1,
		BalanceAfterAvailable: decimal.NewFromInt(9), BalanceAfterOnHold: decimal.Zero,
		BalanceAfterVersion: 2, BalanceAffected: true,
		Snapshot: OperationSnapshot{OverdraftUsedBefore: overdraft, OverdraftUsedAfter: "0"},
	}
}

func completeDigestBalance(id, overdraftUsed, overdraftLimit string) BalanceRedis {
	return BalanceRedis{
		ID: id, Alias: "@account", Key: "default", AccountID: "account", AssetCode: "USD",
		Available: decimal.NewFromInt(9), OnHold: decimal.Zero, Version: 2, AccountType: "deposit",
		AllowSending: 1, AllowReceiving: 1, Direction: "debit", OverdraftUsed: overdraftUsed,
		AllowOverdraft: 1, OverdraftLimitEnabled: 1, OverdraftLimit: overdraftLimit,
		BalanceScope: BalanceScopeTransactional,
	}
}
