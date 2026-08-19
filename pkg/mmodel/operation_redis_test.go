// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedisOperationEconomicComplete_RequiresEveryReplayDiscriminator(t *testing.T) {
	t.Parallel()

	complete := OperationRedis{
		ID: "operation", TransactionID: "transaction", BalanceID: "balance", BalanceKey: "default",
		AccountID: "account", OrganizationID: "organization", LedgerID: "ledger", Type: "DEBIT",
		Direction: "debit", AssetCode: "USD", AmountValue: decimal.Zero,
		Snapshot: OperationSnapshot{OverdraftUsedBefore: "0", OverdraftUsedAfter: "0"},
	}
	require.True(t, RedisOperationEconomicComplete(complete))

	for _, mutate := range []func(*OperationRedis){
		func(value *OperationRedis) { value.ID = "" },
		func(value *OperationRedis) { value.TransactionID = "" },
		func(value *OperationRedis) { value.BalanceID = "" },
		func(value *OperationRedis) { value.BalanceKey = "" },
		func(value *OperationRedis) { value.AccountID = "" },
		func(value *OperationRedis) { value.OrganizationID = "" },
		func(value *OperationRedis) { value.LedgerID = "" },
		func(value *OperationRedis) { value.Type = "" },
		func(value *OperationRedis) { value.Direction = "" },
		func(value *OperationRedis) { value.AssetCode = "" },
		func(value *OperationRedis) { value.Snapshot.OverdraftUsedBefore = "" },
		func(value *OperationRedis) { value.Snapshot.OverdraftUsedAfter = "invalid" },
	} {
		candidate := complete
		mutate(&candidate)
		assert.False(t, RedisOperationEconomicComplete(candidate))
	}
}
