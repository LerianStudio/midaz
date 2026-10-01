//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// Balance cache entries written by a newer release carry every field twice
// (CamelCase and lowerCamel). Once the Lua script of this release rewrites such
// an entry it updates only the CamelCase keys, so the lowerCamel keys keep a
// stale state. The script's answer and its backup record carry the whole entry.

// dualShapeEntry renders a balance cache entry with CamelCase keys holding the
// live state and lowerCamel keys holding staleAvailable/staleVersion.
func dualShapeEntry(op mmodel.BalanceOperation, available string, version int64, staleAvailable, staleVersion string, lowerCamelFirst bool) string {
	b := op.Balance

	camel := fmt.Sprintf(`"ID":%q,"AccountID":%q,"AssetCode":%q,"AccountType":%q,"Key":%q,"Alias":%q,`+
		`"Available":%q,"OnHold":"0","Version":%d,"AllowSending":1,"AllowReceiving":1,"Direction":"credit",`+
		`"OverdraftUsed":"0","AllowOverdraft":0,"OverdraftLimitEnabled":0,"OverdraftLimit":"0","BalanceScope":"transactional","Blocked":0`,
		b.ID, b.AccountID, b.AssetCode, b.AccountType, b.Key, b.Alias, available, version)
	lowerCamel := fmt.Sprintf(`"id":%q,"accountId":%q,"assetCode":%q,"accountType":%q,"key":%q,"alias":"stale-alias",`+
		`"available":%q,"onHold":"7","version":%q,"allowSending":false,"allowReceiving":false,"direction":"credit",`+
		`"overdraftUsed":"0","allowOverdraft":false,"overdraftLimitEnabled":false,"overdraftLimit":"0","balanceScope":"transactional","blocked":false`,
		b.ID, b.AccountID, b.AssetCode, b.AccountType, b.Key, staleAvailable, staleVersion)

	if lowerCamelFirst {
		return `{"SchemaVersion":2,` + lowerCamel + `,` + camel + `}`
	}

	return `{"SchemaVersion":2,` + camel + `,` + lowerCamel + `}`
}

func TestIntegration_ProcessBalanceAtomicOperation_DualShapeCachedBalances(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	for name, lowerCamelFirst := range map[string]bool{
		"camel case keys first":  false,
		"lower camel keys first": true,
	} {
		t.Run(name, func(t *testing.T) {
			infra := setupRedisIntegrationInfra(t)
			ctx := context.Background()
			orgID := uuid.New()
			ledgerID := uuid.New()
			transactionID := uuid.New()

			source := overdraftOp(orgID, ledgerID, "@dual-src", "deposit", "credit",
				decimal.NewFromInt(1000), decimal.Zero, 3, nil, constant.DEBIT, decimal.NewFromInt(100))
			destination := overdraftOp(orgID, ledgerID, "@dual-dst", "deposit", "credit",
				decimal.Zero, decimal.Zero, 5, nil, constant.CREDIT, decimal.NewFromInt(100))

			require.NoError(t, infra.repo.Set(ctx, source.InternalKey, dualShapeEntry(source, "1000", 3, "5000", "1", lowerCamelFirst), 3600))
			require.NoError(t, infra.repo.Set(ctx, destination.InternalKey, dualShapeEntry(destination, "0", 5, "777", "2", lowerCamelFirst), 3600))

			result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID, transactionID,
				constant.APPROVED, false, []mmodel.BalanceOperation{source, destination})
			require.NoError(t, err)

			require.Len(t, result.Before, 2, "every moved balance must be reported")
			require.Len(t, result.After, 2, "every moved balance must be reported")

			before := balancesByAlias(result.Before)
			after := balancesByAlias(result.After)

			assert.True(t, before["@dual-src"].Available.Equal(decimal.NewFromInt(1000)), "source before must be the live CamelCase value, got %s", before["@dual-src"].Available)
			assert.Equal(t, int64(3), before["@dual-src"].Version)
			assert.True(t, after["@dual-src"].Available.Equal(decimal.NewFromInt(900)), "got %s", after["@dual-src"].Available)
			assert.Equal(t, int64(4), after["@dual-src"].Version)
			assert.True(t, after["@dual-dst"].Available.Equal(decimal.NewFromInt(100)), "got %s", after["@dual-dst"].Available)
			assert.Equal(t, int64(6), after["@dual-dst"].Version)

			cached := decodeCachedBalance(t, ctx, infra, source.InternalKey)
			assert.True(t, cached.Available.Equal(decimal.NewFromInt(900)), "the cache must read back the live value, got %s", cached.Available)
			assert.Equal(t, int64(4), cached.Version)

			raw, err := infra.repo.ReadMessageFromQueue(ctx, utils.TransactionInternalKey(orgID, ledgerID, transactionID.String()))
			require.NoError(t, err)

			var backup mmodel.TransactionRedisQueue
			require.NoError(t, json.Unmarshal(raw, &backup), "the backup record written by the script must decode")
			require.Len(t, backup.Balances, 2)
			require.Len(t, backup.BalancesAfter, 2)

			for _, snapshot := range backup.BalancesAfter {
				if snapshot.Alias == "@dual-src" {
					assert.True(t, snapshot.Available.Equal(decimal.NewFromInt(900)), "got %s", snapshot.Available)
				}
			}
		})
	}
}

// An entry the script can move but this release cannot read is only detectable
// after the movement, which is why the answer is reported as unusable instead
// of as a rejection.
func TestIntegration_ProcessBalanceAtomicOperation_UnreadableEntryReportsUnusableResult(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	source := overdraftOp(orgID, ledgerID, "@unreadable-src", "deposit", "credit",
		decimal.NewFromInt(1000), decimal.Zero, 1, nil, constant.DEBIT, decimal.NewFromInt(100))

	entry := `{"ID":12345,"AccountID":"` + source.Balance.AccountID + `","AssetCode":"USD","AccountType":"deposit","Key":"default",` +
		`"Available":"1000","OnHold":"0","Version":1,"AllowSending":1,"AllowReceiving":1,"Direction":"credit",` +
		`"OverdraftUsed":"0","AllowOverdraft":0,"OverdraftLimitEnabled":0,"OverdraftLimit":"0","BalanceScope":"transactional"}`
	require.NoError(t, infra.repo.Set(ctx, source.InternalKey, entry, 3600))

	_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID, uuid.New(),
		constant.APPROVED, false, []mmodel.BalanceOperation{source})

	var unusable *UnusableBalanceResultError
	require.ErrorAs(t, err, &unusable)

	var moved map[string]any
	value, getErr := infra.repo.Get(ctx, source.InternalKey)
	require.NoError(t, getErr)
	require.NoError(t, json.Unmarshal([]byte(value), &moved))
	assert.Equal(t, "900", moved["Available"], "the script moved the balance before its answer was found unusable")
}

func balancesByAlias(balances []*mmodel.Balance) map[string]*mmodel.Balance {
	byAlias := make(map[string]*mmodel.Balance, len(balances))
	for _, b := range balances {
		byAlias[b.Alias] = b
	}

	return byAlias
}

func decodeCachedBalance(t *testing.T, ctx context.Context, infra *integrationTestInfra, key string) mmodel.BalanceRedis {
	t.Helper()

	value, err := infra.repo.Get(ctx, key)
	require.NoError(t, err)

	var cached mmodel.BalanceRedis
	require.NoError(t, json.Unmarshal([]byte(value), &cached))

	return cached
}
