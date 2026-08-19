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

	for _, mutation := range []struct {
		name   string
		mutate func(*OperationRedis)
	}{
		{name: "missing id", mutate: func(value *OperationRedis) { value.ID = "" }},
		{name: "missing transaction id", mutate: func(value *OperationRedis) { value.TransactionID = "" }},
		{name: "missing balance id", mutate: func(value *OperationRedis) { value.BalanceID = "" }},
		{name: "missing balance key", mutate: func(value *OperationRedis) { value.BalanceKey = "" }},
		{name: "missing account id", mutate: func(value *OperationRedis) { value.AccountID = "" }},
		{name: "missing organization id", mutate: func(value *OperationRedis) { value.OrganizationID = "" }},
		{name: "missing ledger id", mutate: func(value *OperationRedis) { value.LedgerID = "" }},
		{name: "missing type", mutate: func(value *OperationRedis) { value.Type = "" }},
		{name: "missing direction", mutate: func(value *OperationRedis) { value.Direction = "" }},
		{name: "missing asset code", mutate: func(value *OperationRedis) { value.AssetCode = "" }},
		{name: "missing overdraft used before", mutate: func(value *OperationRedis) { value.Snapshot.OverdraftUsedBefore = "" }},
		{name: "malformed overdraft used after", mutate: func(value *OperationRedis) { value.Snapshot.OverdraftUsedAfter = "invalid" }},
	} {
		candidate := complete
		mutation.mutate(&candidate)
		assert.False(t, RedisOperationEconomicComplete(candidate), "mutation %q must make the operation incomplete", mutation.name)
	}
}
