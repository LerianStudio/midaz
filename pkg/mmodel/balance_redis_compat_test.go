// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mmodel

import (
	"encoding/json"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dualShapeBalanceBlob is a cache entry in the dual shape newer releases write:
// every field as a lowerCamel key (booleans, string version) and as a CamelCase
// key (0/1 flags, numeric version), tagged SchemaVersion 2.
const dualShapeBalanceBlob = `{
	"SchemaVersion": 2,
	"id": "0199a3a0-0000-7000-8000-000000000001", "ID": "0199a3a0-0000-7000-8000-000000000001",
	"accountId": "0199a3a0-0000-7000-8000-000000000002", "AccountID": "0199a3a0-0000-7000-8000-000000000002",
	"accountType": "deposit", "AccountType": "deposit",
	"assetCode": "BRL", "AssetCode": "BRL",
	"alias": "@alice", "Alias": "@alice",
	"key": "default", "Key": "default",
	"direction": "credit", "Direction": "credit",
	"balanceScope": "transactional", "BalanceScope": "transactional",
	"available": "1000.50", "Available": "1000.50",
	"onHold": "25", "OnHold": "25",
	"overdraftUsed": "0", "OverdraftUsed": "0",
	"overdraftLimit": "300", "OverdraftLimit": "300",
	"version": "7", "Version": 7,
	"allowSending": true, "AllowSending": 1,
	"allowReceiving": false, "AllowReceiving": 0,
	"blocked": false, "Blocked": 0,
	"allowOverdraft": true, "AllowOverdraft": 1,
	"overdraftLimitEnabled": true, "OverdraftLimitEnabled": 1
}`

// Mixed blobs: an older writer updated only the CamelCase keys of a dual-shape
// entry, so the lowerCamel keys still carry the previous state. The two
// constants differ only in key order.
const (
	mixedBalanceBlobCamelFirst = `{"SchemaVersion":2,"ID":"bal-1","AccountID":"acc-1","AssetCode":"BRL","AccountType":"deposit","Key":"default",` +
		`"Alias":"0#@alice#default","Available":"999","OnHold":"0","Version":2,"AllowSending":1,"AllowReceiving":1,"OverdraftUsed":"0",` +
		`"id":"bal-1","alias":"@alice","available":"1000","onHold":"5","version":"1","allowSending":false,"allowReceiving":true,"overdraftUsed":"3"}`
	mixedBalanceBlobLowerFirst = `{"id":"bal-1","alias":"@alice","available":"1000","onHold":"5","version":"1","allowSending":false,"allowReceiving":true,"overdraftUsed":"3",` +
		`"SchemaVersion":2,"ID":"bal-1","AccountID":"acc-1","AssetCode":"BRL","AccountType":"deposit","Key":"default",` +
		`"Alias":"0#@alice#default","Available":"999","OnHold":"0","Version":2,"AllowSending":1,"AllowReceiving":1,"OverdraftUsed":"0"}`
)

func TestBalanceRedis_UnmarshalJSON_DualShapeBlob(t *testing.T) {
	t.Parallel()

	var b BalanceRedis
	require.NoError(t, json.Unmarshal([]byte(dualShapeBalanceBlob), &b))

	assert.Equal(t, "0199a3a0-0000-7000-8000-000000000001", b.ID)
	assert.Equal(t, "0199a3a0-0000-7000-8000-000000000002", b.AccountID)
	assert.Equal(t, "@alice", b.Alias)
	assert.Equal(t, "default", b.Key)
	assert.Equal(t, "BRL", b.AssetCode)
	assert.Equal(t, "deposit", b.AccountType)
	assert.Equal(t, "credit", b.Direction)
	assert.Equal(t, "transactional", b.BalanceScope)
	assert.True(t, decimal.RequireFromString("1000.50").Equal(b.Available), "available: got %s", b.Available)
	assert.True(t, decimal.RequireFromString("25").Equal(b.OnHold), "onHold: got %s", b.OnHold)
	assert.Equal(t, "0", b.OverdraftUsed)
	assert.Equal(t, "300", b.OverdraftLimit)
	assert.Equal(t, int64(7), b.Version)
	assert.Equal(t, 1, b.AllowSending)
	assert.Equal(t, 0, b.AllowReceiving)
	assert.Equal(t, 1, b.AllowOverdraft)
	assert.Equal(t, 1, b.OverdraftLimitEnabled)
}

func TestBalanceRedis_UnmarshalJSON_MixedBlobReadsCamelCaseState(t *testing.T) {
	t.Parallel()

	for name, blob := range map[string]string{
		"camel case keys first":  mixedBalanceBlobCamelFirst,
		"lower camel keys first": mixedBalanceBlobLowerFirst,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var b BalanceRedis
			require.NoError(t, json.Unmarshal([]byte(blob), &b))

			assert.True(t, decimal.NewFromInt(999).Equal(b.Available), "available must come from the CamelCase key, got %s", b.Available)
			assert.True(t, decimal.Zero.Equal(b.OnHold), "onHold must come from the CamelCase key, got %s", b.OnHold)
			assert.Equal(t, int64(2), b.Version)
			assert.Equal(t, "0#@alice#default", b.Alias)
			assert.Equal(t, 1, b.AllowSending)
			assert.Equal(t, 1, b.AllowReceiving)
			assert.Equal(t, "0", b.OverdraftUsed)
		})
	}
}

func TestBalanceRedis_UnmarshalJSON_LowerCamelOnlyDualShapeValues(t *testing.T) {
	t.Parallel()

	blob := `{"SchemaVersion":2,"id":"bal-1","alias":"@bob","key":"default","available":"10","onHold":"0","version":"12",` +
		`"allowSending":true,"allowReceiving":false,"allowOverdraft":false,"overdraftLimitEnabled":false}`

	var b BalanceRedis
	require.NoError(t, json.Unmarshal([]byte(blob), &b))

	assert.Equal(t, int64(12), b.Version)
	assert.Equal(t, 1, b.AllowSending)
	assert.Equal(t, 0, b.AllowReceiving)
	assert.Equal(t, "@bob", b.Alias)
}

// TestBalanceRedis_UnmarshalJSON_SingleShapeBlobs pins the values produced for
// the single-shape entries the Lua script and Go writers already emit.
func TestBalanceRedis_UnmarshalJSON_SingleShapeBlobs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  BalanceRedis
	}{
		{
			name: "lua camel case entry",
			input: `{"ID":"bal-1","Available":"150.25","OnHold":"10","Version":4,"AccountType":"deposit","AccountID":"acc-1",` +
				`"AssetCode":"USD","AllowSending":1,"AllowReceiving":0,"Key":"savings","Direction":"debit","OverdraftUsed":"5",` +
				`"AllowOverdraft":1,"OverdraftLimitEnabled":1,"OverdraftLimit":"100","BalanceScope":"internal","Alias":"0#@a#savings"}`,
			want: BalanceRedis{
				ID: "bal-1", Alias: "0#@a#savings", Key: "savings", AccountID: "acc-1", AssetCode: "USD",
				Available: decimal.RequireFromString("150.25"), OnHold: decimal.NewFromInt(10), Version: 4,
				AccountType: "deposit", AllowSending: 1, AllowReceiving: 0, Direction: "debit", OverdraftUsed: "5",
				AllowOverdraft: 1, OverdraftLimitEnabled: 1, OverdraftLimit: "100", BalanceScope: "internal",
			},
		},
		{
			name:  "lower camel go entry without optional fields",
			input: `{"id":"bal-2","alias":"@b","key":"","accountId":"acc-2","assetCode":"BRL","available":"1","onHold":"0","version":1,"accountType":"deposit","allowSending":1,"allowReceiving":1}`,
			want: BalanceRedis{
				ID: "bal-2", Alias: "@b", Key: "default", AccountID: "acc-2", AssetCode: "BRL",
				Available: decimal.NewFromInt(1), OnHold: decimal.Zero, Version: 1, AccountType: "deposit",
				AllowSending: 1, AllowReceiving: 1, OverdraftUsed: "0", OverdraftLimit: "0",
			},
		},
		{
			name:  "numeric money and overdraft used",
			input: `{"Available":1500.5,"OnHold":0,"Version":1,"OverdraftUsed":2.5}`,
			want: BalanceRedis{
				Key: "default", Available: decimal.NewFromFloat(1500.5), OnHold: decimal.Zero, Version: 1,
				OverdraftUsed: "2.5", OverdraftLimit: "0",
			},
		},
		{
			name:  "overdraft used string is kept verbatim",
			input: `{"Available":"1","OnHold":"0","OverdraftUsed":"not-a-number"}`,
			want: BalanceRedis{
				Key: "default", Available: decimal.NewFromInt(1), OnHold: decimal.Zero,
				OverdraftUsed: "not-a-number", OverdraftLimit: "0",
			},
		},
		{
			name:  "keys matched regardless of case",
			input: `{"AVAILABLE":"3","onhold":"1","VERSION":9,"accountid":"acc-9"}`,
			want: BalanceRedis{
				Key: "default", AccountID: "acc-9", Available: decimal.NewFromInt(3), OnHold: decimal.NewFromInt(1),
				Version: 9, OverdraftUsed: "0", OverdraftLimit: "0",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var got BalanceRedis
			require.NoError(t, json.Unmarshal([]byte(tt.input), &got))

			assert.True(t, tt.want.Available.Equal(got.Available), "available: want %s, got %s", tt.want.Available, got.Available)
			assert.True(t, tt.want.OnHold.Equal(got.OnHold), "onHold: want %s, got %s", tt.want.OnHold, got.OnHold)

			got.Available, got.OnHold = tt.want.Available, tt.want.OnHold
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestBalanceRedis_UnmarshalJSON_RejectsUnreadableEntries(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"missing available":        `{"OnHold":"0","Version":1}`,
		"boolean available":        `{"Available":true,"OnHold":"0"}`,
		"non numeric version text": `{"SchemaVersion":2,"available":"1","onHold":"0","version":"x"}`,
		"unknown schema version":   `{"SchemaVersion":3,"Available":"1","OnHold":"0","Version":1}`,
		"non boolean flag":         `{"SchemaVersion":2,"available":"1","onHold":"0","allowSending":"yes"}`,
		"json null":                `null`,
	}

	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var b BalanceRedis
			assert.Error(t, json.Unmarshal([]byte(input), &b))
		})
	}
}

func TestTransactionRedisQueue_UnmarshalJSON_MixedBalanceSnapshot(t *testing.T) {
	t.Parallel()

	record := `{"transaction_id":"0199a3a0-0000-7000-8000-0000000000aa","transaction_status":"APPROVED",` +
		`"balances":[` + mixedBalanceBlobLowerFirst + `],"balancesAfter":[` + mixedBalanceBlobCamelFirst + `]}`

	var queue TransactionRedisQueue
	require.NoError(t, json.Unmarshal([]byte(record), &queue))

	require.Len(t, queue.Balances, 1)
	require.Len(t, queue.BalancesAfter, 1)
	assert.True(t, decimal.NewFromInt(999).Equal(queue.Balances[0].Available), "got %s", queue.Balances[0].Available)
	assert.Equal(t, int64(2), queue.BalancesAfter[0].Version)
	assert.Equal(t, "0#@alice#default", queue.BalancesAfter[0].Alias)
}
