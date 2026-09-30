//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// TestIntegration_GetBalancesByKeys_LegacyIndexedAliasDecodesInBatch covers the
// balance-sync fetch over blobs cached by 4.0.x and 3.8.x, whose Alias carries
// the transaction entry key ("<index>#<alias>#<key>"). One undecodable entry
// fails the whole batch, so a single legacy blob would keep every balance in
// the batch on the schedule.
func TestIntegration_GetBalancesByKeys_LegacyIndexedAliasDecodesInBatch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := t.Context()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	type cachedBalance struct {
		alias         string
		blob          string
		wantID        string
		wantAvailable decimal.Decimal
		wantVersion   int64
	}

	legacyID, mixedID, modernID := uuid.NewString(), uuid.NewString(), uuid.NewString()
	legacyAlias := "@legacy-indexed-" + uuid.NewString()
	mixedAlias := "@mixed-indexed-" + uuid.NewString()
	modernAlias := "@modern-" + uuid.NewString()

	balances := []cachedBalance{
		{
			alias: legacyAlias,
			blob: fmt.Sprintf(`{"ID":%q,"AccountID":%q,"AccountType":"deposit","AssetCode":"USD",`+
				`"Alias":%q,"Key":"default","Available":"10","OnHold":"0","Version":7,`+
				`"AllowSending":1,"AllowReceiving":1,"Direction":"credit","OverdraftUsed":"0",`+
				`"AllowOverdraft":0,"OverdraftLimitEnabled":0,"OverdraftLimit":"0","BalanceScope":"transactional"}`,
				legacyID, uuid.NewString(), "0#"+legacyAlias+"#default"),
			wantID:        legacyID,
			wantAvailable: decimal.NewFromInt(10),
			wantVersion:   7,
		},
		{
			alias: mixedAlias,
			blob: fmt.Sprintf(`{"SchemaVersion":2,`+
				`"ID":%[1]q,"AccountID":%[2]q,"AccountType":"deposit","AssetCode":"USD","Alias":%[3]q,"Key":"default",`+
				`"Available":"999","OnHold":"0","Version":7,"AllowSending":1,"AllowReceiving":1,`+
				`"id":%[1]q,"accountId":%[2]q,"accountType":"deposit","assetCode":"USD","alias":%[4]q,"key":"default",`+
				`"available":"1000","onHold":"0","version":"1","allowSending":true,"allowReceiving":true}`,
				mixedID, uuid.NewString(), "12#"+mixedAlias+"#default", mixedAlias),
			wantID:        mixedID,
			wantAvailable: decimal.NewFromInt(999),
			wantVersion:   7,
		},
		{
			alias: modernAlias,
			blob: fmt.Sprintf(`{"SchemaVersion":2,"id":%q,"accountId":%q,"accountType":"deposit","assetCode":"USD",`+
				`"alias":%q,"key":"default","direction":"credit","balanceScope":"transactional","available":"25",`+
				`"onHold":"0","overdraftUsed":"0","version":"3","allowSending":true,"allowReceiving":true,`+
				`"allowOverdraft":false,"overdraftLimitEnabled":false,"overdraftLimit":"0"}`,
				modernID, uuid.NewString(), modernAlias),
			wantID:        modernID,
			wantAvailable: decimal.NewFromInt(25),
			wantVersion:   3,
		},
	}

	keys := make([]string, 0, len(balances))

	for _, balance := range balances {
		key := utils.BalanceInternalKey(organizationID, ledgerID, balance.alias+"#default")
		require.NoError(t, infra.redisContainer.Client.Set(ctx, key, balance.blob, 0).Err())

		keys = append(keys, key)
	}

	fetched, err := infra.repo.GetBalancesByKeys(ctx, keys)
	require.NoError(t, err)
	require.Len(t, fetched, len(balances))

	for i, balance := range balances {
		got := fetched[keys[i]]
		require.NotNil(t, got, "balance %s must be read", balance.alias)
		require.Equal(t, balance.wantID, got.ID)
		require.Equal(t, "default", got.Key)
		require.True(t, balance.wantAvailable.Equal(got.Available),
			"balance %s available = %s, want %s", balance.alias, got.Available, balance.wantAvailable)
		require.Equal(t, balance.wantVersion, got.Version, "balance %s version", balance.alias)
	}
}
