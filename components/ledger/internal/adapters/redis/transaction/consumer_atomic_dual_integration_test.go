//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestIntegration_ProcessBalanceAtomicOperation_UpgradesLegacyBalanceToCoherentDualSchema(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := t.Context()
	orgID := uuid.MustParse("e1111111-1111-1111-1111-111111111111")
	ledgerID := uuid.MustParse("e2222222-2222-2222-2222-222222222222")
	transactionID := uuid.MustParse("e7777777-7777-7777-7777-777777777777")
	alias := "@atomic-dual"
	op := newLimitNormalizationOperation(orgID, ledgerID, alias, 1)
	op.InternalKey = utils.BalanceInternalKey(orgID, ledgerID, alias+"#default")
	seedLimitNormalizationCache(t, infra, op, "1000")

	result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		transactionID, "ACTIVE", false, []mmodel.BalanceOperation{op})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Before, 1, "a successful financial commit must return its before row")
	require.Len(t, result.After, 1, "a successful financial commit must return its after row")
	require.Equal(t, int64(7), result.Before[0].Version)
	require.Equal(t, "120", result.Before[0].Available.String())
	require.Equal(t, "11", result.Before[0].OnHold.String())
	require.Equal(t, "0", result.Before[0].OverdraftUsed.String())
	require.Equal(t, int64(8), result.After[0].Version)
	require.Equal(t, "119", result.After[0].Available.String())
	require.Equal(t, "11", result.After[0].OnHold.String())
	require.Equal(t, "0", result.After[0].OverdraftUsed.String())

	cached := readLimitNormalizationCache(t, infra, op.InternalKey)
	assertAtomicDualCache(t, cached, alias)
}

func assertAtomicDualCache(t *testing.T, cached map[string]json.RawMessage, alias string) {
	t.Helper()

	require.Equal(t, 2, decodeSettingsUpdateField[int](t, cached, "SchemaVersion"))
	require.Equal(t, "119", decodeSettingsUpdateField[string](t, cached, "Available"))
	require.Equal(t, "119", decodeSettingsUpdateField[string](t, cached, "available"))
	require.Equal(t, "11", decodeSettingsUpdateField[string](t, cached, "OnHold"))
	require.Equal(t, "11", decodeSettingsUpdateField[string](t, cached, "onHold"))
	require.Equal(t, "0", decodeSettingsUpdateField[string](t, cached, "OverdraftUsed"))
	require.Equal(t, "0", decodeSettingsUpdateField[string](t, cached, "overdraftUsed"))
	require.Equal(t, int64(8), decodeSettingsUpdateField[int64](t, cached, "Version"))
	require.Equal(t, "8", decodeSettingsUpdateField[string](t, cached, "version"))

	stringPairs := [][2]string{
		{"ID", "id"},
		{"AccountID", "accountId"},
		{"Available", "available"},
		{"OnHold", "onHold"},
		{"OverdraftUsed", "overdraftUsed"},
		{"AccountType", "accountType"},
		{"AssetCode", "assetCode"},
		{"Direction", "direction"},
		{"OverdraftLimit", "overdraftLimit"},
		{"BalanceScope", "balanceScope"},
	}
	for _, pair := range stringPairs {
		require.Equal(t,
			decodeSettingsUpdateField[string](t, cached, pair[0]),
			decodeSettingsUpdateField[string](t, cached, pair[1]),
			"%s and %s must be coherent", pair[0], pair[1])
	}
	require.Equal(t, alias, decodeSettingsUpdateField[string](t, cached, "alias"))
	require.Equal(t, "default", decodeSettingsUpdateField[string](t, cached, "key"))
	require.Equal(t, alias, decodeSettingsUpdateField[string](t, cached, "Alias"))
	require.Equal(t, alias+"#default", decodeSettingsUpdateField[string](t, cached, "Key"))

	booleanPairs := [][2]string{
		{"AllowSending", "allowSending"},
		{"AllowReceiving", "allowReceiving"},
		{"AllowOverdraft", "allowOverdraft"},
		{"OverdraftLimitEnabled", "overdraftLimitEnabled"},
	}
	for _, pair := range booleanPairs {
		require.Equal(t,
			decodeSettingsUpdateField[int](t, cached, pair[0]) == 1,
			decodeSettingsUpdateField[bool](t, cached, pair[1]),
			"%s and %s must be coherent", pair[0], pair[1])
	}
	require.Equal(t,
		strconv.FormatInt(decodeSettingsUpdateField[int64](t, cached, "Version"), 10),
		decodeSettingsUpdateField[string](t, cached, "version"),
		"Version and version must be coherent")
}

func TestIntegration_ProcessBalanceAtomicOperation_DualCacheCompatibility(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := t.Context()
	orgID := uuid.MustParse("e1111111-1111-1111-1111-111111111111")
	ledgerID := uuid.MustParse("e2222222-2222-2222-2222-222222222222")
	transactionID := uuid.MustParse("e7777777-7777-7777-7777-777777777777")

	t.Run("cold successful mutation writes dual cache and decodable recovery", func(t *testing.T) {
		alias := "@dual-cold-success"
		op := newLimitNormalizationOperation(orgID, ledgerID, alias, 1)
		op.InternalKey = utils.BalanceInternalKey(orgID, ledgerID, alias+"#default")
		op.Balance.Available = decimal.NewFromInt(120)
		op.Balance.OnHold = decimal.NewFromInt(11)
		op.Balance.OverdraftUsed = decimal.Zero
		op.Balance.Version = 7
		exists, err := infra.redisContainer.Client.Exists(ctx, op.InternalKey).Result()
		require.NoError(t, err)
		require.Zero(t, exists)
		result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
			transactionID, "ACTIVE", false, []mmodel.BalanceOperation{op})
		require.NoError(t, err)
		require.Len(t, result.Before, 1)
		require.Len(t, result.After, 1)
		require.Equal(t, "120", result.Before[0].Available.String())
		require.Equal(t, "119", result.After[0].Available.String())
		assertAtomicDualCache(t, readLimitNormalizationCache(t, infra, op.InternalKey), alias)
		messages, err := infra.repo.ReadAllMessagesFromQueue(ctx)
		require.NoError(t, err)
		message, found := messages[utils.TransactionInternalKey(orgID, ledgerID, transactionID.String())]
		require.True(t, found)
		var recovery mmodel.TransactionRedisQueue
		require.NoError(t, json.Unmarshal([]byte(message), &recovery))
		require.Len(t, recovery.Balances, 1)
		require.Len(t, recovery.BalancesAfter, 1)
		require.Equal(t, "120", recovery.Balances[0].Available.String())
		require.Equal(t, "119", recovery.BalancesAfter[0].Available.String())
		require.Equal(t, int64(7), recovery.Balances[0].Version)
		require.Equal(t, int64(8), recovery.BalancesAfter[0].Version)
		require.Equal(t, "11", recovery.Balances[0].OnHold.String())
		require.Equal(t, "11", recovery.BalancesAfter[0].OnHold.String())
		require.Equal(t, "0", recovery.Balances[0].OverdraftUsed)
		require.Equal(t, "0", recovery.BalancesAfter[0].OverdraftUsed)
	})

	t.Run("later invalid legacy flag rejects before cold seed or mutation", func(t *testing.T) {
		cold := newLimitNormalizationOperation(orgID, ledgerID, "@dual-flag-cold", 1)
		cold.InternalKey = utils.BalanceInternalKey(orgID, ledgerID, "@dual-flag-cold#default")
		later := newLimitNormalizationOperation(orgID, ledgerID, "@dual-flag-invalid", 1)
		later.InternalKey = utils.BalanceInternalKey(orgID, ledgerID, "@dual-flag-invalid#default")
		seedLimitNormalizationCache(t, infra, later, "1000")
		cached := readLimitNormalizationCache(t, infra, later.InternalKey)
		cached["AllowSending"] = json.RawMessage(`2`)
		encoded, err := json.Marshal(cached)
		require.NoError(t, err)
		require.NoError(t, infra.redisContainer.Client.Set(ctx, later.InternalKey, encoded, time.Hour).Err())
		before := captureLimitNormalizationRedisState(t, infra)
		result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
			transactionID, "ACTIVE", false, []mmodel.BalanceOperation{cold, later})
		require.Error(t, err)
		require.Nil(t, result)
		require.Equal(t, before, captureLimitNormalizationRedisState(t, infra))
	})

	t.Run("warm stale lower shadows are refreshed from legacy authority", func(t *testing.T) {
		alias := "@dual-stale"
		op := newLimitNormalizationOperation(orgID, ledgerID, alias, 1)
		op.InternalKey = utils.BalanceInternalKey(orgID, ledgerID, alias+"#default")
		seedLimitNormalizationCache(t, infra, op, "1000")
		cached := readLimitNormalizationCache(t, infra, op.InternalKey)
		for key, value := range map[string]any{
			"SchemaVersion": 2, "id": "stale", "accountId": "stale", "alias": "@stale", "key": "stale",
			"available": "999", "onHold": "999", "overdraftUsed": "999", "version": "999",
			"accountType": "stale", "assetCode": "EUR", "direction": "debit", "overdraftLimit": "999",
			"balanceScope": "stale", "allowSending": false, "allowReceiving": false,
			"allowOverdraft": true, "overdraftLimitEnabled": false,
		} {
			encoded, err := json.Marshal(value)
			require.NoError(t, err)
			cached[key] = encoded
		}
		encoded, err := json.Marshal(cached)
		require.NoError(t, err)
		require.NoError(t, infra.redisContainer.Client.Set(ctx, op.InternalKey, encoded, time.Hour).Err())
		result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
			transactionID, "ACTIVE", false, []mmodel.BalanceOperation{op})
		require.NoError(t, err)
		require.Equal(t, "119", result.After[0].Available.String())
		assertAtomicDualCache(t, readLimitNormalizationCache(t, infra, op.InternalKey), alias)
	})

	t.Run("later new-only balance rejects before cold seed or mutation", func(t *testing.T) {
		cold := newLimitNormalizationOperation(orgID, ledgerID, "@dual-cold", 1)
		cold.InternalKey = utils.BalanceInternalKey(orgID, ledgerID, "@dual-cold#default")
		later := newLimitNormalizationOperation(orgID, ledgerID, "@dual-new-only", 1)
		later.InternalKey = utils.BalanceInternalKey(orgID, ledgerID, "@dual-new-only#default")
		require.NoError(t, infra.redisContainer.Client.Set(ctx, later.InternalKey,
			`{"SchemaVersion":2,"id":"e3333333-3333-3333-3333-333333333333","available":"120","version":"7"}`, time.Hour).Err())
		before := captureLimitNormalizationRedisState(t, infra)
		result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
			transactionID, "ACTIVE", false, []mmodel.BalanceOperation{cold, later})
		require.ErrorContains(t, err, "BALANCE_CACHE_SHAPE_UNSUPPORTED")
		require.Nil(t, result)
		require.Equal(t, before, captureLimitNormalizationRedisState(t, infra))
	})

	t.Run("warm zero amount preserves cache bytes and expiry", func(t *testing.T) {
		alias := "@dual-noop"
		op := newLimitNormalizationOperation(orgID, ledgerID, alias, 0)
		op.InternalKey = utils.BalanceInternalKey(orgID, ledgerID, alias+"#default")
		seedLimitNormalizationCache(t, infra, op, "1000")
		before := captureLimitNormalizationRedisState(t, infra)
		result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
			transactionID, "ACTIVE", false, []mmodel.BalanceOperation{op})
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, before[op.InternalKey], captureLimitNormalizationRedisState(t, infra)[op.InternalKey])
		cached := readLimitNormalizationCache(t, infra, op.InternalKey)
		require.NotContains(t, cached, "SchemaVersion")
		require.NotContains(t, cached, "available")
	})

	t.Run("warm pending credit preserves stale dual shadows", func(t *testing.T) {
		alias := "@dual-pending-noop"
		op := newLimitNormalizationOperation(orgID, ledgerID, alias, 1)
		op.Amount.Operation = "CREDIT"
		op.InternalKey = utils.BalanceInternalKey(orgID, ledgerID, alias+"#default")
		seedLimitNormalizationCache(t, infra, op, "1000")
		cached := readLimitNormalizationCache(t, infra, op.InternalKey)
		cached["SchemaVersion"] = json.RawMessage(`2`)
		cached["available"] = json.RawMessage(`"999"`)
		encoded, err := json.Marshal(cached)
		require.NoError(t, err)
		require.NoError(t, infra.redisContainer.Client.Set(ctx, op.InternalKey, encoded, time.Hour).Err())
		before := captureLimitNormalizationRedisState(t, infra)
		result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
			transactionID, "PENDING", true, []mmodel.BalanceOperation{op})
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, before[op.InternalKey], captureLimitNormalizationRedisState(t, infra)[op.InternalKey])
		require.Equal(t, "999", decodeSettingsUpdateField[string](t, readLimitNormalizationCache(t, infra, op.InternalKey), "available"))
	})

	for _, shape := range []string{"legacy", "dual", "cold"} {
		t.Run("financial refusal restores "+shape+" preprojection image", func(t *testing.T) {
			alias := "@dual-rollback-" + shape
			first := newLimitNormalizationOperation(orgID, ledgerID, alias, 1)
			first.InternalKey = utils.BalanceInternalKey(orgID, ledgerID, alias+"#default")
			first.Balance.Available = decimal.NewFromInt(120)
			first.Balance.OnHold = decimal.NewFromInt(11)
			first.Balance.Version = 7
			var before []byte
			if shape != "cold" {
				seedLimitNormalizationCache(t, infra, first, "1000")
				if shape == "dual" {
					cached := readLimitNormalizationCache(t, infra, first.InternalKey)
					cached["SchemaVersion"] = json.RawMessage(`2`)
					cached["available"] = json.RawMessage(`"999"`)
					encoded, err := json.Marshal(cached)
					require.NoError(t, err)
					require.NoError(t, infra.redisContainer.Client.Set(ctx, first.InternalKey, encoded, time.Hour).Err())
				}
				var err error
				before, err = infra.redisContainer.Client.Get(ctx, first.InternalKey).Bytes()
				require.NoError(t, err)
			}
			refused := newLimitNormalizationOperation(orgID, ledgerID, alias+"-refused", 121)
			refused.InternalKey = utils.BalanceInternalKey(orgID, ledgerID, alias+"-refused#default")
			seedLimitNormalizationCache(t, infra, refused, "1000")
			refusedCache := readLimitNormalizationCache(t, infra, refused.InternalKey)
			refusedCache["AllowOverdraft"] = json.RawMessage(`0`)
			encodedRefused, err := json.Marshal(refusedCache)
			require.NoError(t, err)
			require.NoError(t, infra.redisContainer.Client.Set(ctx, refused.InternalKey, encodedRefused, time.Hour).Err())
			refusedBefore, err := infra.redisContainer.Client.Get(ctx, refused.InternalKey).Bytes()
			require.NoError(t, err)
			result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
				transactionID, "ACTIVE", false, []mmodel.BalanceOperation{first, refused})
			require.Error(t, err)
			require.Nil(t, result)
			encodedError, marshalErr := json.Marshal(err)
			require.NoError(t, marshalErr)
			var businessError struct {
				Code string `json:"code"`
			}
			require.NoError(t, json.Unmarshal(encodedError, &businessError))
			require.Equal(t, "0018", businessError.Code)
			after, err := infra.redisContainer.Client.Get(ctx, first.InternalKey).Bytes()
			require.NoError(t, err)
			if shape == "cold" {
				cached := readLimitNormalizationCache(t, infra, first.InternalKey)
				require.NotContains(t, cached, "SchemaVersion")
				require.NotContains(t, cached, "available")
				require.Equal(t, "120", decodeSettingsUpdateField[string](t, cached, "Available"))
				require.Equal(t, "11", decodeSettingsUpdateField[string](t, cached, "OnHold"))
				require.Equal(t, int64(7), decodeSettingsUpdateField[int64](t, cached, "Version"))
			} else {
				require.JSONEq(t, string(before), string(after))
			}
			refusedAfter, err := infra.redisContainer.Client.Get(ctx, refused.InternalKey).Bytes()
			require.NoError(t, err)
			require.JSONEq(t, string(refusedBefore), string(refusedAfter))
		})
	}

	t.Run("exponent version mirrors the serialized legacy numeric value", func(t *testing.T) {
		alias := "@dual-exponent"
		op := newLimitNormalizationOperation(orgID, ledgerID, alias, 1)
		op.InternalKey = utils.BalanceInternalKey(orgID, ledgerID, alias+"#default")
		seedLimitNormalizationCache(t, infra, op, "1000")
		cached := readLimitNormalizationCache(t, infra, op.InternalKey)
		cached["Version"] = json.RawMessage(`9007199254740992`)
		encoded, err := json.Marshal(cached)
		require.NoError(t, err)
		require.NoError(t, infra.redisContainer.Client.Set(ctx, op.InternalKey, encoded, time.Hour).Err())
		_, err = infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
			transactionID, "ACTIVE", false, []mmodel.BalanceOperation{op})
		require.Error(t, err, "the legacy public int64 decoder cannot consume the cjson exponent token")
		cached = readLimitNormalizationCache(t, infra, op.InternalKey)
		require.Contains(t, string(cached["Version"]), "e+")
		serializedVersion, err := decimal.NewFromString(string(cached["Version"]))
		require.NoError(t, err)
		require.Equal(t, "9007199254741000", serializedVersion.String())
		require.Equal(t, serializedVersion.String(), decodeSettingsUpdateField[string](t, cached, "version"))
		require.Equal(t, "119", decodeSettingsUpdateField[string](t, cached, "Available"))
	})
}
