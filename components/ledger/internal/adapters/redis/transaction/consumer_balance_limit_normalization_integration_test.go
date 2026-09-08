//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	_ "embed"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/balancecache"

	"github.com/stretchr/testify/require"
)

//go:embed scripts/normalize_balance_limit.lua
var balanceLimitNormalizationTestLua string

func TestIntegration_BalanceLimitNormalization_WholeBlobCAS(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	client := infra.redisContainer.Client
	ctx := context.Background()
	key := "{transactions}:normalization:whole-blob-cas"
	observed := `{"ID":"00000000-0000-0000-0000-000000000001","AccountID":"00000000-0000-0000-0000-000000000002","Alias":"@whole-blob","Key":"default","Available":"120","OnHold":"37.00","OverdraftUsed":"0.3","Version":9007199254740993,"AccountType":"liability","AssetCode":"USD","AllowSending":1,"AllowReceiving":1,"Direction":"debit","AllowOverdraft":1,"OverdraftLimitEnabled":1,"OverdraftLimit":"1e+2","BalanceScope":"transactional","Unknown":{"numbers":[-0,1e9999,1.00]}}`
	replacementBytes, err := balancecache.NormalizeLimitDual([]byte(observed), "@whole-blob")
	require.NoError(t, err)
	replacement := string(replacementBytes)

	t.Run("matching observation stores validated replacement and preserves absolute expiry", func(t *testing.T) {
		require.NoError(t, client.Set(ctx, key, observed, time.Minute).Err())
		before, err := client.Do(ctx, "PEXPIRETIME", key).Int64()
		require.NoError(t, err)

		status, err := client.Eval(ctx, balanceLimitNormalizationTestLua, []string{key}, observed, replacement).Int64()
		require.NoError(t, err)
		require.EqualValues(t, 1, status)

		stored, err := client.Get(ctx, key).Result()
		require.NoError(t, err)
		require.Equal(t, replacement, stored)
		after, err := client.Do(ctx, "PEXPIRETIME", key).Int64()
		require.NoError(t, err)
		require.Equal(t, before, after, "the original absolute expiry must not change")
	})

	t.Run("exact no-op preserves canonical bytes and expiry", func(t *testing.T) {
		canonical := replacement
		require.NoError(t, client.Set(ctx, key, canonical, time.Minute).Err())
		before, err := client.Do(ctx, "PEXPIRETIME", key).Int64()
		require.NoError(t, err)

		status, err := client.Eval(ctx, balanceLimitNormalizationTestLua, []string{key}, canonical, canonical).Int64()
		require.NoError(t, err)
		require.EqualValues(t, 1, status)

		stored, err := client.Get(ctx, key).Result()
		require.NoError(t, err)
		require.Equal(t, canonical, stored)
		after, err := client.Do(ctx, "PEXPIRETIME", key).Int64()
		require.NoError(t, err)
		require.Equal(t, before, after)
	})
}

func TestIntegration_BalanceLimitNormalization_ConditionalUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	client := infra.redisContainer.Client
	ctx := context.Background()
	key := "{transactions}:normalization:conditional"
	observed := `{"Available":"120","Version":42,"OverdraftLimit":"1e+2","Unknown":"observed"}`
	replacement := `{"Available":"120","Version":42,"OverdraftLimit":"100","Unknown":"observed"}`

	t.Run("missing cache does not create a balance", func(t *testing.T) {
		require.NoError(t, client.Del(ctx, key).Err())
		status, err := client.Eval(ctx, balanceLimitNormalizationTestLua, []string{key}, observed, replacement).Int64()
		require.NoError(t, err)
		require.Zero(t, status)
		exists, err := client.Exists(ctx, key).Result()
		require.NoError(t, err)
		require.Zero(t, exists)
	})

	t.Run("any concurrent blob change wins", func(t *testing.T) {
		concurrent := `{"Available":"120","Version":9007199254740993,"OverdraftLimit":"1e+2","Unknown":"concurrent"}`
		require.NoError(t, client.Set(ctx, key, concurrent, time.Minute).Err())
		expires, err := client.Do(ctx, "PEXPIRETIME", key).Int64()
		require.NoError(t, err)

		status, err := client.Eval(ctx, balanceLimitNormalizationTestLua, []string{key}, observed, replacement).Int64()
		require.NoError(t, err)
		require.EqualValues(t, 2, status)
		stored, err := client.Get(ctx, key).Result()
		require.NoError(t, err)
		require.Equal(t, concurrent, stored)
		actualExpires, err := client.Do(ctx, "PEXPIRETIME", key).Int64()
		require.NoError(t, err)
		require.Equal(t, expires, actualExpires)
	})
}
