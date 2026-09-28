//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestIntegration_ProcessBalanceAtomicOperation_NormalizesExistingDualLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := t.Context()
	orgID := uuid.MustParse("e1111111-1111-1111-1111-111111111111")
	ledgerID := uuid.MustParse("e2222222-2222-2222-2222-222222222222")
	transactionID := uuid.MustParse("e7777777-7777-7777-7777-777777777777")
	alias := "@public-dual-limit"

	op := newLimitNormalizationOperation(orgID, ledgerID, alias, 0)
	op.InternalKey = utils.BalanceInternalKey(orgID, ledgerID, alias+"#default")
	op.Balance.Available = decimal.NewFromInt(120)
	op.Balance.OnHold = decimal.NewFromInt(11)
	op.Balance.Version = 9007199254740993
	*op.Balance.Settings.OverdraftLimit = decimal.NewFromInt(10).String()
	seedLimitNormalizationCache(t, infra, op, "1000")

	client := infra.redisContainer.Client
	raw, err := client.Get(ctx, op.InternalKey).Result()
	require.NoError(t, err)
	raw = strings.Replace(raw, `"OverdraftLimit":"1000"`, `"OverdraftLimit":"010.00"`, 1)
	raw = strings.TrimSuffix(raw, "}") + `,"overdraftLimit":"99","UnknownExtension":{"owner":"observed","number":9007199254740993}}`
	require.NoError(t, client.Set(ctx, op.InternalKey, raw, time.Minute).Err())
	expires, err := client.Do(ctx, "PEXPIRETIME", op.InternalKey).Int64()
	require.NoError(t, err)

	hook := &limitNormalizationCASHook{
		key: op.InternalKey,
		onAttempt: func(ctx context.Context, observed string, attempt int) error {
			if attempt != 1 {
				return nil
			}

			concurrent := strings.Replace(
				observed,
				`"UnknownExtension":{"owner":"observed","number":9007199254740993}`,
				`"UnknownExtension":{"owner":"concurrent","number":9007199254740993}`,
				1,
			)
			_, err := client.Eval(ctx, `return redis.call('SET', KEYS[1], ARGV[1], 'KEEPTTL')`, []string{op.InternalKey}, concurrent).Result()

			return err
		},
	}
	newLimitNormalizationHookedClient(t, infra, hook)

	result, err := infra.processBalanceAtomicOperationWithoutBlockException(
		ctx, orgID, ledgerID, transactionID, "ACTIVE", false,
		[]mmodel.BalanceOperation{op},
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Empty(t, result.Before)
	require.Empty(t, result.After)
	require.Equal(t, 2, hook.attempts, "the rejected CAS must trigger a fresh read and repair")

	stored := readLimitNormalizationCache(t, infra, op.InternalKey)
	require.Equal(t, `{"owner":"concurrent","number":9007199254740993}`, string(stored["UnknownExtension"]))
	require.Equal(t, `9007199254740993`, string(stored["Version"]))
	require.Equal(t, `"9007199254740993"`, string(stored["version"]))
	require.JSONEq(t, `"10"`, string(stored["OverdraftLimit"]))
	require.JSONEq(t, `"10"`, string(stored["overdraftLimit"]))
	require.JSONEq(t, `2`, string(stored["SchemaVersion"]))

	for _, pair := range [][2]string{
		{"ID", "id"},
		{"AccountID", "accountId"},
		{"Alias", "alias"},
		{"Available", "available"},
		{"OnHold", "onHold"},
		{"OverdraftUsed", "overdraftUsed"},
		{"Version", "version"},
		{"AccountType", "accountType"},
		{"AssetCode", "assetCode"},
		{"Key", "key"},
		{"AllowSending", "allowSending"},
		{"AllowReceiving", "allowReceiving"},
		{"Direction", "direction"},
		{"AllowOverdraft", "allowOverdraft"},
		{"OverdraftLimitEnabled", "overdraftLimitEnabled"},
		{"OverdraftLimit", "overdraftLimit"},
		{"BalanceScope", "balanceScope"},
	} {
		require.Contains(t, stored, pair[0])
		require.Contains(t, stored, pair[1])
	}
	actualExpires, err := client.Do(ctx, "PEXPIRETIME", op.InternalKey).Int64()
	require.NoError(t, err)
	require.Equal(t, expires, actualExpires)
}
