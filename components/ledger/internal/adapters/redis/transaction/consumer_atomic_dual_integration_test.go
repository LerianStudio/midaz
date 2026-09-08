//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
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

	t.Run("complete new-only balance mutates and writes coherent dual cache", func(t *testing.T) {
		alias := "@dual-new-only-complete"
		op := newLimitNormalizationOperation(orgID, ledgerID, alias, 1)
		op.InternalKey = utils.BalanceInternalKey(orgID, ledgerID, alias+"#default")
		op.Balance.Version = 9007199254740992
		newOnly := map[string]any{
			"SchemaVersion":         2,
			"id":                    strings.ToUpper(op.Balance.ID),
			"accountId":             strings.ToUpper(op.Balance.AccountID),
			"available":             "120",
			"onHold":                "11",
			"overdraftUsed":         "0",
			"version":               "9007199254740992",
			"accountType":           op.Balance.AccountType,
			"assetCode":             op.Balance.AssetCode,
			"allowSending":          false,
			"allowReceiving":        false,
			"alias":                 alias,
			"key":                   "default",
			"direction":             "credit",
			"allowOverdraft":        false,
			"overdraftLimitEnabled": true,
			"overdraftLimit":        "1000",
			"balanceScope":          "transactional",
		}
		encoded, err := json.Marshal(newOnly)
		require.NoError(t, err)
		require.NoError(t, infra.redisContainer.Client.Set(ctx, op.InternalKey, encoded, time.Hour).Err())

		result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
			uuid.MustParse("e7777777-7777-7777-7777-777777777789"), "ACTIVE", false, []mmodel.BalanceOperation{op})
		require.NoError(t, err)
		require.Len(t, result.Before, 1)
		require.Len(t, result.After, 1)
		require.Equal(t, "120", result.Before[0].Available.String())
		require.Equal(t, "11", result.Before[0].OnHold.String())
		require.Equal(t, "0", result.Before[0].OverdraftUsed.String())
		require.Equal(t, int64(9007199254740992), result.Before[0].Version)
		require.Equal(t, "119", result.After[0].Available.String())
		require.Equal(t, "11", result.After[0].OnHold.String())
		require.Equal(t, "0", result.After[0].OverdraftUsed.String())
		require.Equal(t, int64(9007199254740993), result.After[0].Version)

		cached := readLimitNormalizationCache(t, infra, op.InternalKey)
		require.Equal(t, 2, decodeSettingsUpdateField[int](t, cached, "SchemaVersion"))
		for _, pair := range [][2]string{
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
		} {
			require.Equal(t, decodeSettingsUpdateField[string](t, cached, pair[0]),
				decodeSettingsUpdateField[string](t, cached, pair[1]), "%s and %s must be coherent", pair[0], pair[1])
		}
		require.Equal(t, alias, decodeSettingsUpdateField[string](t, cached, "Alias"))
		require.Equal(t, alias, decodeSettingsUpdateField[string](t, cached, "alias"))
		require.Equal(t, alias+"#default", decodeSettingsUpdateField[string](t, cached, "Key"))
		require.Equal(t, "default", decodeSettingsUpdateField[string](t, cached, "key"))
		for _, pair := range [][2]string{
			{"AllowSending", "allowSending"},
			{"AllowReceiving", "allowReceiving"},
			{"AllowOverdraft", "allowOverdraft"},
			{"OverdraftLimitEnabled", "overdraftLimitEnabled"},
		} {
			require.Equal(t, decodeSettingsUpdateField[int](t, cached, pair[0]) == 1,
				decodeSettingsUpdateField[bool](t, cached, pair[1]), "%s and %s must be coherent", pair[0], pair[1])
		}
		require.JSONEq(t, `9007199254740993`, string(cached["Version"]))
		require.Equal(t, "9007199254740993", decodeSettingsUpdateField[string](t, cached, "version"))
	})

	t.Run("new-only no-op preserves its exact cache image and expiry", func(t *testing.T) {
		alias := "@dual-new-only-noop"
		op := newLimitNormalizationOperation(orgID, ledgerID, alias, 0)
		op.InternalKey = utils.BalanceInternalKey(orgID, ledgerID, alias+"#default")
		newOnly := map[string]any{
			"SchemaVersion": 2, "id": op.Balance.ID, "accountId": op.Balance.AccountID,
			"available": "120", "onHold": "11", "version": "7",
			"accountType": op.Balance.AccountType, "assetCode": op.Balance.AssetCode,
			"allowSending": true, "allowReceiving": true, "alias": alias,
		}
		encoded, err := json.Marshal(newOnly)
		require.NoError(t, err)
		require.NoError(t, infra.redisContainer.Client.Set(ctx, op.InternalKey, encoded, time.Hour).Err())
		before := captureLimitNormalizationRedisState(t, infra)

		result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
			uuid.MustParse("e7777777-7777-7777-7777-777777777790"), "ACTIVE", false, []mmodel.BalanceOperation{op})
		require.NoError(t, err)
		require.Empty(t, result.Before)
		require.Empty(t, result.After)
		require.Equal(t, before[op.InternalKey], captureLimitNormalizationRedisState(t, infra)[op.InternalKey])
	})

	t.Run("mixed cache never falls back from invalid legacy authority to valid lower fields", func(t *testing.T) {
		cold := newLimitNormalizationOperation(orgID, ledgerID, "@dual-mixed-cold", 1)
		cold.InternalKey = utils.BalanceInternalKey(orgID, ledgerID, "@dual-mixed-cold#default")
		alias := "@dual-mixed-invalid"
		later := newLimitNormalizationOperation(orgID, ledgerID, alias, 1)
		later.InternalKey = utils.BalanceInternalKey(orgID, ledgerID, alias+"#default")
		mixed := map[string]any{
			"SchemaVersion": 2, "id": later.Balance.ID, "accountId": later.Balance.AccountID,
			"available": "120", "onHold": "11", "overdraftUsed": "0", "version": "7",
			"accountType": later.Balance.AccountType, "assetCode": later.Balance.AssetCode,
			"allowSending": true, "allowReceiving": true, "alias": alias, "key": "default",
			"direction": "credit", "allowOverdraft": false, "overdraftLimitEnabled": true,
			"overdraftLimit": "1000", "balanceScope": "transactional",
			"Available": false,
		}
		encoded, err := json.Marshal(mixed)
		require.NoError(t, err)
		require.NoError(t, infra.redisContainer.Client.Set(ctx, later.InternalKey, encoded, time.Hour).Err())
		before := captureLimitNormalizationRedisState(t, infra)

		result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
			uuid.MustParse("e7777777-7777-7777-7777-777777777791"), "ACTIVE", false,
			[]mmodel.BalanceOperation{cold, later})
		require.Error(t, err)
		require.Nil(t, result)
		require.Equal(t, before, captureLimitNormalizationRedisState(t, infra))
	})

	t.Run("financial refusal restores the raw new-only pre-mutation image", func(t *testing.T) {
		alias := "@dual-new-only-rollback"
		first := newLimitNormalizationOperation(orgID, ledgerID, alias, 1)
		first.InternalKey = utils.BalanceInternalKey(orgID, ledgerID, alias+"#default")
		newOnly := map[string]any{
			"SchemaVersion": 2, "id": first.Balance.ID, "accountId": first.Balance.AccountID,
			"available": "120", "onHold": "11", "overdraftUsed": "0", "version": "7",
			"accountType": first.Balance.AccountType, "assetCode": first.Balance.AssetCode,
			"allowSending": true, "allowReceiving": true, "alias": alias, "key": "default",
			"direction": "credit", "allowOverdraft": false, "overdraftLimitEnabled": true,
			"overdraftLimit": "1000", "balanceScope": "transactional",
		}
		encoded, err := json.Marshal(newOnly)
		require.NoError(t, err)
		require.NoError(t, infra.redisContainer.Client.Set(ctx, first.InternalKey, encoded, time.Hour).Err())
		before, err := infra.redisContainer.Client.Get(ctx, first.InternalKey).Bytes()
		require.NoError(t, err)

		refused := newLimitNormalizationOperation(orgID, ledgerID, alias+"-refused", 121)
		refused.InternalKey = utils.BalanceInternalKey(orgID, ledgerID, alias+"-refused#default")
		seedLimitNormalizationCache(t, infra, refused, "1000")
		refusedCache := readLimitNormalizationCache(t, infra, refused.InternalKey)
		refusedCache["AllowOverdraft"] = json.RawMessage(`0`)
		encodedRefused, err := json.Marshal(refusedCache)
		require.NoError(t, err)
		require.NoError(t, infra.redisContainer.Client.Set(ctx, refused.InternalKey, encodedRefused, time.Hour).Err())

		result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
			uuid.MustParse("e7777777-7777-7777-7777-777777777792"), "ACTIVE", false,
			[]mmodel.BalanceOperation{first, refused})
		require.Error(t, err)
		require.Nil(t, result)
		after, getErr := infra.redisContainer.Client.Get(ctx, first.InternalKey).Bytes()
		require.NoError(t, getErr)
		require.Equal(t, before, after)
	})

	t.Run("new-only live overdraft setting overrides stale operation settings and absent optionals default", func(t *testing.T) {
		alias := "@dual-new-only-live-settings"
		op := newLimitNormalizationOperation(orgID, ledgerID, alias, 121)
		op.InternalKey = utils.BalanceInternalKey(orgID, ledgerID, alias+"#default")
		newOnly := map[string]any{
			"SchemaVersion": 2, "id": op.Balance.ID, "accountId": op.Balance.AccountID,
			"available": "120", "onHold": "11", "version": "7",
			"accountType": op.Balance.AccountType, "assetCode": op.Balance.AssetCode,
			"allowSending": true, "allowReceiving": true, "alias": alias,
			"direction": "credit", "allowOverdraft": true,
		}
		encoded, err := json.Marshal(newOnly)
		require.NoError(t, err)
		require.NoError(t, infra.redisContainer.Client.Set(ctx, op.InternalKey, encoded, time.Hour).Err())

		result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
			uuid.MustParse("e7777777-7777-7777-7777-777777777793"), "ACTIVE", false, []mmodel.BalanceOperation{op})
		require.NoError(t, err)
		require.Equal(t, "0", result.After[0].Available.String())
		require.Equal(t, "1", result.After[0].OverdraftUsed.String())
		cached := readLimitNormalizationCache(t, infra, op.InternalKey)
		require.Equal(t, 1, decodeSettingsUpdateField[int](t, cached, "AllowOverdraft"))
		require.True(t, decodeSettingsUpdateField[bool](t, cached, "allowOverdraft"))
		require.Equal(t, "0", decodeSettingsUpdateField[string](t, cached, "OverdraftLimit"))
		require.False(t, decodeSettingsUpdateField[bool](t, cached, "overdraftLimitEnabled"))
		require.Equal(t, "transactional", decodeSettingsUpdateField[string](t, cached, "balanceScope"))
		require.Equal(t, "default", decodeSettingsUpdateField[string](t, cached, "key"))
	})

	for _, tc := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{name: "schema token is not exact integer two", mutate: func(cache map[string]any) { cache["SchemaVersion"] = json.RawMessage(`2.0`) }},
		{name: "flag is numeric", mutate: func(cache map[string]any) { cache["allowSending"] = 1 }},
		{name: "version is numeric", mutate: func(cache map[string]any) { cache["version"] = 7 }},
		{name: "available is numeric", mutate: func(cache map[string]any) { cache["available"] = 120 }},
		{name: "required on hold is missing", mutate: func(cache map[string]any) { delete(cache, "onHold") }},
		{name: "optional money is explicit null", mutate: func(cache map[string]any) { cache["overdraftUsed"] = json.RawMessage(`null`) }},
		{name: "direction enum is invalid", mutate: func(cache map[string]any) { cache["direction"] = "sideways" }},
		{name: "scope enum is invalid", mutate: func(cache map[string]any) { cache["balanceScope"] = "available" }},
		{name: "identity differs from operation", mutate: func(cache map[string]any) { cache["accountId"] = "e4444444-4444-4444-4444-444444444444" }},
		{name: "domain key contains qualifier", mutate: func(cache map[string]any) { cache["key"] = "alias#default" }},
		{name: "money is noncanonical", mutate: func(cache map[string]any) { cache["overdraftLimit"] = "1000.0" }},
	} {
		t.Run("invalid new-only "+tc.name+" rejects before batch writes", func(t *testing.T) {
			coldAlias := "@dual-invalid-new-cold-" + strings.ReplaceAll(tc.name, " ", "-")
			cold := newLimitNormalizationOperation(orgID, ledgerID, coldAlias, 1)
			cold.InternalKey = utils.BalanceInternalKey(orgID, ledgerID, coldAlias+"#default")
			alias := "@dual-invalid-new-" + strings.ReplaceAll(tc.name, " ", "-")
			later := newLimitNormalizationOperation(orgID, ledgerID, alias, 1)
			later.InternalKey = utils.BalanceInternalKey(orgID, ledgerID, alias+"#default")
			newOnly := map[string]any{
				"SchemaVersion": 2, "id": later.Balance.ID, "accountId": later.Balance.AccountID,
				"available": "120", "onHold": "11", "overdraftUsed": "0", "version": "7",
				"accountType": later.Balance.AccountType, "assetCode": later.Balance.AssetCode,
				"allowSending": true, "allowReceiving": true, "alias": alias, "key": "default",
				"direction": "credit", "allowOverdraft": false, "overdraftLimitEnabled": true,
				"overdraftLimit": "1000", "balanceScope": "transactional",
			}
			tc.mutate(newOnly)
			encoded, err := json.Marshal(newOnly)
			require.NoError(t, err)
			require.NoError(t, infra.redisContainer.Client.Set(ctx, later.InternalKey, encoded, time.Hour).Err())
			before := captureLimitNormalizationRedisState(t, infra)

			result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
				uuid.New(), "ACTIVE", false, []mmodel.BalanceOperation{cold, later})
			require.ErrorContains(t, err, "BALANCE_CACHE_SHAPE_UNSUPPORTED")
			require.Nil(t, result)
			require.Equal(t, before, captureLimitNormalizationRedisState(t, infra))
		})
	}

	t.Run("later incomplete new-only balance rejects before cold seed or mutation", func(t *testing.T) {
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

	t.Run("high version remains an exact integer across the public result and dual cache", func(t *testing.T) {
		alias := "@dual-exponent"
		op := newLimitNormalizationOperation(orgID, ledgerID, alias, 1)
		op.InternalKey = utils.BalanceInternalKey(orgID, ledgerID, alias+"#default")
		seedLimitNormalizationCache(t, infra, op, "1000")
		cached := readLimitNormalizationCache(t, infra, op.InternalKey)
		cached["Version"] = json.RawMessage(`9007199254740992`)
		encoded, err := json.Marshal(cached)
		require.NoError(t, err)
		require.NoError(t, infra.redisContainer.Client.Set(ctx, op.InternalKey, encoded, time.Hour).Err())
		result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
			transactionID, "ACTIVE", false, []mmodel.BalanceOperation{op})
		require.NoError(t, err)
		require.Equal(t, int64(9007199254740992), result.Before[0].Version)
		require.Equal(t, int64(9007199254740993), result.After[0].Version)
		cached = readLimitNormalizationCache(t, infra, op.InternalKey)
		require.JSONEq(t, `9007199254740993`, string(cached["Version"]))
		require.Equal(t, "9007199254740993", decodeSettingsUpdateField[string](t, cached, "version"))
		require.Equal(t, "119", decodeSettingsUpdateField[string](t, cached, "Available"))
		messages, err := infra.repo.ReadAllMessagesFromQueue(ctx)
		require.NoError(t, err)
		var recovery mmodel.TransactionRedisQueue
		require.NoError(t, json.Unmarshal([]byte(messages[utils.TransactionInternalKey(orgID, ledgerID, transactionID.String())]), &recovery))
		require.Equal(t, int64(9007199254740992), recovery.Balances[0].Version)
		require.Equal(t, int64(9007199254740993), recovery.BalancesAfter[0].Version)
	})

	t.Run("adjacent high versions do not collide during stale detection", func(t *testing.T) {
		alias := "@dual-high-stale"
		op := newLimitNormalizationOperation(orgID, ledgerID, alias, 121)
		op.InternalKey = utils.BalanceInternalKey(orgID, ledgerID, alias+"#default")
		op.Balance.Version = 9007199254740993
		seedLimitNormalizationCache(t, infra, op, "1000")
		cached := readLimitNormalizationCache(t, infra, op.InternalKey)
		cached["Version"] = json.RawMessage(`9007199254740992`)
		encoded, err := json.Marshal(cached)
		require.NoError(t, err)
		require.NoError(t, infra.redisContainer.Client.Set(ctx, op.InternalKey, encoded, time.Hour).Err())
		before, err := infra.redisContainer.Client.Get(ctx, op.InternalKey).Bytes()
		require.NoError(t, err)

		result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
			uuid.MustParse("e7777777-7777-7777-7777-777777777778"), "ACTIVE", false, []mmodel.BalanceOperation{op})
		require.ErrorContains(t, err, "0174")
		require.Nil(t, result)
		after, getErr := infra.redisContainer.Client.Get(ctx, op.InternalKey).Bytes()
		require.NoError(t, getErr)
		require.JSONEq(t, string(before), string(after))
	})

	t.Run("cold seed preserves a high version in cache result and recovery", func(t *testing.T) {
		alias := "@dual-high-cold"
		txID := uuid.MustParse("e7777777-7777-7777-7777-777777777779")
		op := newLimitNormalizationOperation(orgID, ledgerID, alias, 1)
		op.InternalKey = utils.BalanceInternalKey(orgID, ledgerID, alias+"#default")
		op.Balance.Version = 9007199254740992

		result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
			txID, "ACTIVE", false, []mmodel.BalanceOperation{op})
		require.NoError(t, err)
		require.Equal(t, int64(9007199254740992), result.Before[0].Version)
		require.Equal(t, int64(9007199254740993), result.After[0].Version)
		cached := readLimitNormalizationCache(t, infra, op.InternalKey)
		require.JSONEq(t, `9007199254740993`, string(cached["Version"]))
		require.Equal(t, "9007199254740993", decodeSettingsUpdateField[string](t, cached, "version"))

		messages, err := infra.repo.ReadAllMessagesFromQueue(ctx)
		require.NoError(t, err)
		var recovery mmodel.TransactionRedisQueue
		require.NoError(t, json.Unmarshal([]byte(messages[utils.TransactionInternalKey(orgID, ledgerID, txID.String())]), &recovery))
		require.Equal(t, int64(9007199254740992), recovery.Balances[0].Version)
		require.Equal(t, int64(9007199254740993), recovery.BalancesAfter[0].Version)
	})

	t.Run("repeated same-key operations advance the exact current version", func(t *testing.T) {
		alias := "@dual-high-repeated"
		first := newLimitNormalizationOperation(orgID, ledgerID, alias, 1)
		first.InternalKey = utils.BalanceInternalKey(orgID, ledgerID, alias+"#default")
		seedLimitNormalizationCache(t, infra, first, "1000")
		cached := readLimitNormalizationCache(t, infra, first.InternalKey)
		cached["Version"] = json.RawMessage(`9007199254740992`)
		encoded, err := json.Marshal(cached)
		require.NoError(t, err)
		require.NoError(t, infra.redisContainer.Client.Set(ctx, first.InternalKey, encoded, time.Hour).Err())
		second := first

		_, err = infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
			uuid.MustParse("e7777777-7777-7777-7777-777777777784"), "ACTIVE", false,
			[]mmodel.BalanceOperation{first, second})
		require.NoError(t, err)
		cached = readLimitNormalizationCache(t, infra, first.InternalKey)
		require.Equal(t, "118", decodeSettingsUpdateField[string](t, cached, "Available"))
		require.JSONEq(t, `9007199254740994`, string(cached["Version"]))
		require.Equal(t, "9007199254740994", decodeSettingsUpdateField[string](t, cached, "version"))
	})

	t.Run("repeated canceled same-key release and credit accept the current version", func(t *testing.T) {
		alias := "@dual-canceled-repeated"
		release := newLimitNormalizationOperation(orgID, ledgerID, alias, 1)
		release.InternalKey = utils.BalanceInternalKey(orgID, ledgerID, alias+"#default")
		seedLimitNormalizationCache(t, infra, release, "1000")
		cached := readLimitNormalizationCache(t, infra, release.InternalKey)
		cached["Available"] = json.RawMessage(`"0"`)
		cached["OnHold"] = json.RawMessage(`"1"`)
		cached["OverdraftUsed"] = json.RawMessage(`"10"`)
		cached["Version"] = json.RawMessage(`9007199254740992`)
		encoded, err := json.Marshal(cached)
		require.NoError(t, err)
		require.NoError(t, infra.redisContainer.Client.Set(ctx, release.InternalKey, encoded, time.Hour).Err())

		release.Amount.Operation = "RELEASE"
		release.Amount.RouteValidationEnabled = true
		release.Balance.Available = decimal.Zero
		release.Balance.OnHold = decimal.NewFromInt(1)
		release.Balance.OverdraftUsed = decimal.NewFromInt(10)
		release.Balance.Version = 9007199254740992
		credit := release
		credit.Amount.Operation = "CREDIT"
		credit.Balance.Version = 9007199254740992
		credit.Balance.Available = decimal.Zero
		credit.Balance.OnHold = decimal.Zero
		credit.Balance.OverdraftUsed = decimal.NewFromInt(10)
		txID := uuid.MustParse("e7777777-7777-7777-7777-777777777785")
		result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
			txID, constant.CANCELED, true, []mmodel.BalanceOperation{release, credit})
		require.NoError(t, err)
		require.Len(t, result.Before, 2)
		require.Len(t, result.After, 2)
		require.Equal(t, int64(9007199254740992), result.Before[0].Version)
		require.Equal(t, int64(9007199254740993), result.Before[1].Version)
		require.Equal(t, int64(9007199254740993), result.After[0].Version)
		require.Equal(t, int64(9007199254740994), result.After[1].Version)
		require.Equal(t, "0", result.After[0].Available.String())
		require.Equal(t, "0", result.After[0].OnHold.String())
		require.Equal(t, "10", result.After[0].OverdraftUsed.String())
		require.Equal(t, "0", result.After[1].Available.String())
		require.Equal(t, "0", result.After[1].OnHold.String())
		require.Equal(t, "9", result.After[1].OverdraftUsed.String())
		cached = readLimitNormalizationCache(t, infra, release.InternalKey)
		require.JSONEq(t, `9007199254740994`, string(cached["Version"]))
		require.Equal(t, "9007199254740994", decodeSettingsUpdateField[string](t, cached, "version"))
		messages, err := infra.repo.ReadAllMessagesFromQueue(ctx)
		require.NoError(t, err)
		var recovery mmodel.TransactionRedisQueue
		require.NoError(t, json.Unmarshal([]byte(messages[utils.TransactionInternalKey(orgID, ledgerID, txID.String())]), &recovery))
		require.Len(t, recovery.Balances, 2)
		require.Len(t, recovery.BalancesAfter, 2)
		require.Equal(t, int64(9007199254740992), recovery.Balances[0].Version)
		require.Equal(t, int64(9007199254740993), recovery.Balances[1].Version)
		require.Equal(t, int64(9007199254740993), recovery.BalancesAfter[0].Version)
		require.Equal(t, int64(9007199254740994), recovery.BalancesAfter[1].Version)
	})

	t.Run("raw version scanner accepts escaped key and ignores nested decoy", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			raw  func(string) string
		}{
			{name: "escaped top-level key", raw: func(raw string) string {
				return strings.Replace(raw, `"Version":7`, `"\u0056ersion":9007199254740992`, 1)
			}},
			{name: "nested key", raw: func(raw string) string {
				return strings.Replace(raw, `"Version":7`, `"Version":9007199254740992,"Nested":{"Version":7}`, 1)
			}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				alias := "@dual-raw-" + strings.ReplaceAll(tc.name, " ", "-")
				op := newLimitNormalizationOperation(orgID, ledgerID, alias, 1)
				op.InternalKey = utils.BalanceInternalKey(orgID, ledgerID, alias+"#default")
				seed := seedLimitNormalizationCache(t, infra, op, "1000")
				require.NoError(t, infra.redisContainer.Client.Set(ctx, op.InternalKey, tc.raw(seed), time.Hour).Err())
				op.Balance.Version = 9007199254740992
				result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
					uuid.MustParse("e7777777-7777-7777-7777-777777777787"), "ACTIVE", false, []mmodel.BalanceOperation{op})
				require.NoError(t, err)
				require.Equal(t, int64(9007199254740992), result.Before[0].Version)
				require.Equal(t, int64(9007199254740993), result.After[0].Version)
				cached := readLimitNormalizationCache(t, infra, op.InternalKey)
				require.JSONEq(t, `9007199254740993`, string(cached["Version"]))
			})
		}

		t.Run("plain and escaped duplicate rejects before writes", func(t *testing.T) {
			alias := "@dual-raw-duplicate"
			op := newLimitNormalizationOperation(orgID, ledgerID, alias, 1)
			op.InternalKey = utils.BalanceInternalKey(orgID, ledgerID, alias+"#default")
			seed := seedLimitNormalizationCache(t, infra, op, "1000")
			raw := strings.Replace(seed, `"Version":7`, `"Version":9007199254740992,"\u0056ersion":9007199254740992`, 1)
			require.NoError(t, infra.redisContainer.Client.Set(ctx, op.InternalKey, raw, time.Hour).Err())
			before := captureLimitNormalizationRedisState(t, infra)
			op.Balance.Version = 9007199254740992
			result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
				uuid.MustParse("e7777777-7777-7777-7777-777777777788"), "ACTIVE", false, []mmodel.BalanceOperation{op})
			require.Error(t, err)
			require.Nil(t, result)
			require.Equal(t, before, captureLimitNormalizationRedisState(t, infra))
		})
	})

	t.Run("max int64 minus one increments exactly to max int64", func(t *testing.T) {
		alias := "@dual-version-max"
		op := newLimitNormalizationOperation(orgID, ledgerID, alias, 1)
		op.InternalKey = utils.BalanceInternalKey(orgID, ledgerID, alias+"#default")
		seedLimitNormalizationCache(t, infra, op, "1000")
		cached := readLimitNormalizationCache(t, infra, op.InternalKey)
		cached["Version"] = json.RawMessage(`9223372036854775806`)
		encoded, err := json.Marshal(cached)
		require.NoError(t, err)
		require.NoError(t, infra.redisContainer.Client.Set(ctx, op.InternalKey, encoded, time.Hour).Err())

		result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
			uuid.MustParse("e7777777-7777-7777-7777-777777777780"), "ACTIVE", false, []mmodel.BalanceOperation{op})
		require.NoError(t, err)
		require.Equal(t, int64(9223372036854775806), result.Before[0].Version)
		require.Equal(t, int64(9223372036854775807), result.After[0].Version)
		cached = readLimitNormalizationCache(t, infra, op.InternalKey)
		require.JSONEq(t, `9223372036854775807`, string(cached["Version"]))
		require.Equal(t, "9223372036854775807", decodeSettingsUpdateField[string](t, cached, "version"))
	})

	t.Run("max int64 no-op remains valid and byte preserving", func(t *testing.T) {
		alias := "@dual-version-max-noop"
		op := newLimitNormalizationOperation(orgID, ledgerID, alias, 0)
		op.InternalKey = utils.BalanceInternalKey(orgID, ledgerID, alias+"#default")
		seedLimitNormalizationCache(t, infra, op, "1000")
		cached := readLimitNormalizationCache(t, infra, op.InternalKey)
		cached["Version"] = json.RawMessage(`9223372036854775807`)
		encoded, err := json.Marshal(cached)
		require.NoError(t, err)
		require.NoError(t, infra.redisContainer.Client.Set(ctx, op.InternalKey, encoded, time.Hour).Err())
		before, err := infra.redisContainer.Client.Get(ctx, op.InternalKey).Bytes()
		require.NoError(t, err)

		result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
			uuid.MustParse("e7777777-7777-7777-7777-777777777781"), "ACTIVE", false, []mmodel.BalanceOperation{op})
		require.NoError(t, err)
		require.Empty(t, result.Before)
		require.Empty(t, result.After)
		after, err := infra.redisContainer.Client.Get(ctx, op.InternalKey).Bytes()
		require.NoError(t, err)
		require.Equal(t, before, after)
	})

	t.Run("late version overflow restores exact balances and writes no recovery", func(t *testing.T) {
		first := newLimitNormalizationOperation(orgID, ledgerID, "@dual-overflow-first", 1)
		first.InternalKey = utils.BalanceInternalKey(orgID, ledgerID, "@dual-overflow-first#default")
		seedLimitNormalizationCache(t, infra, first, "1000")
		firstCache := readLimitNormalizationCache(t, infra, first.InternalKey)
		firstCache["Version"] = json.RawMessage(`9007199254740992`)
		encodedFirst, err := json.Marshal(firstCache)
		require.NoError(t, err)
		require.NoError(t, infra.redisContainer.Client.Set(ctx, first.InternalKey, encodedFirst, time.Hour).Err())

		last := newLimitNormalizationOperation(orgID, ledgerID, "@dual-overflow-last", 1)
		last.InternalKey = utils.BalanceInternalKey(orgID, ledgerID, "@dual-overflow-last#default")
		seedLimitNormalizationCache(t, infra, last, "1000")
		lastCache := readLimitNormalizationCache(t, infra, last.InternalKey)
		lastCache["Version"] = json.RawMessage(`9223372036854775807`)
		encodedLast, err := json.Marshal(lastCache)
		require.NoError(t, err)
		require.NoError(t, infra.redisContainer.Client.Set(ctx, last.InternalKey, encodedLast, time.Hour).Err())

		firstBefore, err := infra.redisContainer.Client.Get(ctx, first.InternalKey).Bytes()
		require.NoError(t, err)
		lastBefore, err := infra.redisContainer.Client.Get(ctx, last.InternalKey).Bytes()
		require.NoError(t, err)
		txID := uuid.MustParse("e7777777-7777-7777-7777-777777777782")
		result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
			txID, "ACTIVE", false, []mmodel.BalanceOperation{first, last})
		require.ErrorContains(t, err, "BALANCE_VERSION_OVERFLOW")
		require.Nil(t, result)
		firstAfter, getErr := infra.redisContainer.Client.Get(ctx, first.InternalKey).Bytes()
		require.NoError(t, getErr)
		lastAfter, getErr := infra.redisContainer.Client.Get(ctx, last.InternalKey).Bytes()
		require.NoError(t, getErr)
		require.JSONEq(t, string(firstBefore), string(firstAfter))
		require.JSONEq(t, string(lastBefore), string(lastAfter))
		messages, readErr := infra.repo.ReadAllMessagesFromQueue(ctx)
		require.NoError(t, readErr)
		_, found := messages[utils.TransactionInternalKey(orgID, ledgerID, txID.String())]
		require.False(t, found)
	})

	t.Run("later invalid incoming version rejects before any batch write", func(t *testing.T) {
		first := newLimitNormalizationOperation(orgID, ledgerID, "@dual-invalid-first", 1)
		first.InternalKey = utils.BalanceInternalKey(orgID, ledgerID, "@dual-invalid-first#default")
		seedLimitNormalizationCache(t, infra, first, "1000")
		last := newLimitNormalizationOperation(orgID, ledgerID, "@dual-invalid-last", 1)
		last.InternalKey = utils.BalanceInternalKey(orgID, ledgerID, "@dual-invalid-last#default")
		seedLimitNormalizationCache(t, infra, last, "1000")
		last.Balance.Version = -1
		firstBefore, err := infra.redisContainer.Client.Get(ctx, first.InternalKey).Bytes()
		require.NoError(t, err)
		lastBefore, err := infra.redisContainer.Client.Get(ctx, last.InternalKey).Bytes()
		require.NoError(t, err)

		result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
			uuid.MustParse("e7777777-7777-7777-7777-777777777783"), "ACTIVE", false,
			[]mmodel.BalanceOperation{first, last})
		require.ErrorContains(t, err, "BALANCE_DUAL_PROJECTION_INVALID")
		require.Nil(t, result)
		firstAfter, getErr := infra.redisContainer.Client.Get(ctx, first.InternalKey).Bytes()
		require.NoError(t, getErr)
		lastAfter, getErr := infra.redisContainer.Client.Get(ctx, last.InternalKey).Bytes()
		require.NoError(t, getErr)
		require.Equal(t, firstBefore, firstAfter)
		require.Equal(t, lastBefore, lastAfter)
	})
}
