// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"encoding/json"
	"testing"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransactionRedisQueue_UnmarshalJSON_DualBalancesUseLegacyUppercaseFields(t *testing.T) {
	t.Parallel()

	input := `{
		"balances": [{
			"ID":"before-upper","Alias":"@before-upper","Key":"upper-key","AccountID":"account-upper","AssetCode":"USD",
			"Available":"9007199254740993.123456789","OnHold":17.25,"Version":9007199254740993,"AccountType":"deposit",
			"AllowSending":1,"AllowReceiving":0,"Direction":"debit","OverdraftUsed":"4.50","AllowOverdraft":1,
			"OverdraftLimitEnabled":1,"OverdraftLimit":"100.25","BalanceScope":"transactional",
			"id":"before-lower","alias":"@before-lower","key":"lower-key","accountId":"account-lower","assetCode":"BRL",
			"available":"1.01","onHold":"2.02","version":"7","accountType":"stale","allowSending":false,
			"allowReceiving":true,"direction":"credit","overdraftUsed":"0","allowOverdraft":false,
			"overdraftLimitEnabled":false,"overdraftLimit":"0","balanceScope":"internal","SchemaVersion":2
		}],
		"balancesAfter": [{
			"ID":"after-upper","Alias":"@after-upper","Key":"after-key","AccountID":"after-account","AssetCode":"EUR",
			"Available":99,"OnHold":"3.75","Version":42,"AccountType":"checking","AllowSending":0,"AllowReceiving":1,
			"Direction":"credit","OverdraftUsed":"0","AllowOverdraft":0,"OverdraftLimitEnabled":0,"OverdraftLimit":"0",
			"BalanceScope":"internal","id":"after-lower","version":"8","allowSending":true,"allowReceiving":false,
			"SchemaVersion":2
		}]
	}`

	var queue TransactionRedisQueue
	err := json.Unmarshal([]byte(input), &queue)
	require.NoError(t, err)
	require.Len(t, queue.Balances, 1)
	require.Len(t, queue.BalancesAfter, 1)

	before := queue.Balances[0]
	assert.Equal(t, "before-upper", before.ID)
	assert.Equal(t, "@before-upper", before.Alias)
	assert.Equal(t, "upper-key", before.Key)
	assert.Equal(t, "account-upper", before.AccountID)
	assert.Equal(t, "USD", before.AssetCode)
	assert.True(t, decimal.RequireFromString("9007199254740993.123456789").Equal(before.Available))
	assert.True(t, decimal.RequireFromString("17.25").Equal(before.OnHold))
	assert.Equal(t, int64(9007199254740993), before.Version)
	assert.Equal(t, "deposit", before.AccountType)
	assert.Equal(t, 1, before.AllowSending)
	assert.Equal(t, 0, before.AllowReceiving)
	assert.Equal(t, "debit", before.Direction)
	assert.Equal(t, "4.50", before.OverdraftUsed)
	assert.Equal(t, 1, before.AllowOverdraft)
	assert.Equal(t, 1, before.OverdraftLimitEnabled)
	assert.Equal(t, "100.25", before.OverdraftLimit)
	assert.Equal(t, "transactional", before.BalanceScope)

	after := queue.BalancesAfter[0]
	assert.Equal(t, "after-upper", after.ID)
	assert.Equal(t, "@after-upper", after.Alias)
	assert.Equal(t, "after-key", after.Key)
	assert.Equal(t, "after-account", after.AccountID)
	assert.Equal(t, int64(42), after.Version)
	assert.Equal(t, 0, after.AllowSending)
	assert.Equal(t, 1, after.AllowReceiving)
}

func TestBalanceRedis_UnmarshalJSON_InvalidUppercaseDoesNotFallBackToLowercase(t *testing.T) {
	t.Parallel()

	input := `{
		"Available":10,"available":"1.5","OnHold":0,"onHold":"2.5",
		"Version":"invalid","version":"7","AllowSending":1,"allowSending":true,
		"AllowReceiving":1,"allowReceiving":true,"SchemaVersion":2
	}`

	var balance BalanceRedis
	err := json.Unmarshal([]byte(input), &balance)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "version")
}

func TestBalanceRedis_UnmarshalJSON_SchemaTwoLowercaseRepresentation(t *testing.T) {
	t.Parallel()

	input := `{
		"id":"new-id","alias":"@new-alias","key":"new-key","accountId":"new-account","assetCode":"USD",
		"available":"9007199254740993.000000001","onHold":2.5,"version":"9007199254740993","accountType":"deposit",
		"allowSending":true,"allowReceiving":false,"direction":"debit","overdraftUsed":"3.25","allowOverdraft":true,
		"overdraftLimitEnabled":false,"overdraftLimit":"250.75","balanceScope":"transactional","SchemaVersion":2
	}`

	var balance BalanceRedis
	err := json.Unmarshal([]byte(input), &balance)
	require.NoError(t, err)

	assert.Equal(t, "new-id", balance.ID)
	assert.Equal(t, "@new-alias", balance.Alias)
	assert.Equal(t, "new-key", balance.Key)
	assert.Equal(t, "new-account", balance.AccountID)
	assert.True(t, decimal.RequireFromString("9007199254740993.000000001").Equal(balance.Available))
	assert.True(t, decimal.RequireFromString("2.5").Equal(balance.OnHold))
	assert.Equal(t, int64(9007199254740993), balance.Version)
	assert.Equal(t, 1, balance.AllowSending)
	assert.Equal(t, 0, balance.AllowReceiving)
	assert.Equal(t, "3.25", balance.OverdraftUsed)
	assert.Equal(t, 1, balance.AllowOverdraft)
	assert.Equal(t, 0, balance.OverdraftLimitEnabled)
}

func TestBalanceRedis_UnmarshalJSON_HistoricalLowercaseRepresentation(t *testing.T) {
	t.Parallel()

	input := `{
		"id":"old-id","alias":"@old-alias","key":"old-key","accountId":"old-account","assetCode":"BRL",
		"available":125.75,"onHold":"4.25","version":17,"accountType":"checking",
		"allowSending":1,"allowReceiving":0,"allowOverdraft":0,"overdraftLimitEnabled":1
	}`

	var balance BalanceRedis
	err := json.Unmarshal([]byte(input), &balance)
	require.NoError(t, err)

	assert.Equal(t, "old-id", balance.ID)
	assert.Equal(t, "@old-alias", balance.Alias)
	assert.Equal(t, "old-key", balance.Key)
	assert.Equal(t, "old-account", balance.AccountID)
	assert.True(t, decimal.RequireFromString("125.75").Equal(balance.Available))
	assert.True(t, decimal.RequireFromString("4.25").Equal(balance.OnHold))
	assert.Equal(t, int64(17), balance.Version)
	assert.Equal(t, 1, balance.AllowSending)
	assert.Equal(t, 0, balance.AllowReceiving)
	assert.Equal(t, 0, balance.AllowOverdraft)
	assert.Equal(t, 1, balance.OverdraftLimitEnabled)
}

func TestBalanceRedis_UnmarshalJSON_HistoricalMarshaledZeroOptionals(t *testing.T) {
	t.Parallel()

	wire, err := json.Marshal(BalanceRedis{
		Available:    decimal.RequireFromString("999"),
		OnHold:       decimal.RequireFromString("777"),
		Version:      5,
		BalanceScope: "internal",
	})
	require.NoError(t, err)

	var balance BalanceRedis
	require.NoError(t, json.Unmarshal(wire, &balance))

	assert.True(t, decimal.RequireFromString("999").Equal(balance.Available))
	assert.True(t, decimal.RequireFromString("777").Equal(balance.OnHold))
	assert.Equal(t, int64(5), balance.Version)
	assert.Equal(t, "0", balance.OverdraftUsed)
	assert.Equal(t, "0", balance.OverdraftLimit)
	assert.Equal(t, constant.DefaultBalanceKey, balance.Key)
	assert.Equal(t, "internal", balance.BalanceScope)
}

func TestBalanceRedis_UnmarshalJSON_ReceiverAndNullCompatibility(t *testing.T) {
	t.Parallel()

	balance := BalanceRedis{
		ID:             "existing-id",
		Key:            "existing-key",
		Version:        19,
		AllowSending:   1,
		OverdraftUsed:  "8",
		OverdraftLimit: "12",
	}

	err := json.Unmarshal([]byte(`{
		"Available":"3","OnHold":"2","Version":null,"AllowSending":null,
		"OverdraftUsed":null,"OverdraftLimit":null
	}`), &balance)
	require.NoError(t, err)

	assert.Equal(t, "existing-id", balance.ID, "an absent field must preserve receiver overlay state")
	assert.Equal(t, "existing-key", balance.Key)
	assert.Equal(t, int64(19), balance.Version, "legacy numeric null keeps encoding/json no-op semantics")
	assert.Equal(t, 1, balance.AllowSending)
	assert.Equal(t, "0", balance.OverdraftUsed)
	assert.Equal(t, "12", balance.OverdraftLimit, "legacy text null keeps encoding/json no-op semantics")
}

func TestBalanceRedis_UnmarshalJSON_StrictSchemaTwoLowerTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		field string
	}{
		{name: "numeric version", field: `"version":7`},
		{name: "numeric flag", field: `"version":"7","allowSending":1`},
		{name: "null flag", field: `"version":"7","allowSending":null`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			input := `{"available":"1","onHold":"0","SchemaVersion":2,` + tt.field + `}`
			var balance BalanceRedis
			err := json.Unmarshal([]byte(input), &balance)

			require.Error(t, err)
		})
	}
}

func TestBalanceRedis_UnmarshalJSON_TopLevelNullAndLegacyExponentVersion(t *testing.T) {
	t.Parallel()

	var balance BalanceRedis
	require.Error(t, json.Unmarshal([]byte(`null`), &balance))

	err := json.Unmarshal([]byte(`{"Available":"1","OnHold":"0","Version":1e3}`), &balance)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "version")
}
