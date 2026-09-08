//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/balancecache"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"

	"github.com/google/uuid"
	redisclient "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

type settingsCASAfterGetHook struct {
	reads    atomic.Int32
	afterGet func(context.Context, int32) error
}

func (h *settingsCASAfterGetHook) DialHook(next redisclient.DialHook) redisclient.DialHook {
	return next
}

func (h *settingsCASAfterGetHook) ProcessHook(next redisclient.ProcessHook) redisclient.ProcessHook {
	return func(ctx context.Context, cmd redisclient.Cmder) error {
		if err := next(ctx, cmd); err != nil {
			return err
		}
		if cmd.Name() != "get" {
			return nil
		}

		return h.afterGet(ctx, h.reads.Add(1))
	}
}

func (h *settingsCASAfterGetHook) ProcessPipelineHook(next redisclient.ProcessPipelineHook) redisclient.ProcessPipelineHook {
	return next
}

func newSettingsCASHookedRepository(
	t *testing.T,
	infra *integrationTestInfra,
	hook *settingsCASAfterGetHook,
) *RedisConsumerRepository {
	t.Helper()
	conn := redistestutil.CreateConnectionWithDB(t, infra.redisContainer.Addr, infra.redisContainer.DB)
	client, err := conn.GetClient(t.Context())
	require.NoError(t, err)
	client.AddHook(hook)

	return &RedisConsumerRepository{conn: conn}
}

func TestIntegration_UpdateBalanceCacheSettings_UpgradesLegacyBalanceToCoherentDualSchema(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	ledgerID := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	cacheKey := "@settings-dual#default"
	internalKey := utils.BalanceInternalKey(orgID, ledgerID, cacheKey)
	raw := `{"ID":"33333333-3333-3333-3333-333333333333","AccountID":"44444444-4444-4444-4444-444444444444","Alias":"@settings-dual","Available":"12345678901234567890.123456789","OnHold":"0.000000001","OverdraftUsed":"98765432109876543210.987654321","Version":9007199254740993,"AccountType":"liability","AssetCode":"USD","Key":"@settings-dual#default","AllowSending":1,"AllowReceiving":0,"Direction":"credit","AllowOverdraft":1,"OverdraftLimitEnabled":0,"OverdraftLimit":"10000000000000000000.000000001","BalanceScope":"transactional","CreatedAt":"2026-01-02T03:04:05.000000006Z","UpdatedAt":"2026-02-03T04:05:06.000000007Z"}`
	require.NoError(t, infra.redisContainer.Client.Set(ctx, internalKey, raw, time.Hour).Err())

	limit := "99999999999999999999.123456789"
	settings := &mmodel.BalanceSettings{
		AllowOverdraft:        false,
		OverdraftLimitEnabled: true,
		OverdraftLimit:        &limit,
		BalanceScope:          mmodel.BalanceScopeTransactional,
	}
	require.NoError(t, infra.repo.UpdateBalanceCacheSettings(ctx, orgID, ledgerID, cacheKey, settings))

	cached := readLimitNormalizationCache(t, infra, internalKey)
	require.Equal(t, `9007199254740993`, string(cached["Version"]))
	require.JSONEq(t, `2`, string(cached["SchemaVersion"]))
	require.JSONEq(t, `"12345678901234567890.123456789"`, string(cached["Available"]))
	require.JSONEq(t, `"12345678901234567890.123456789"`, string(cached["available"]))
	require.JSONEq(t, `"0.000000001"`, string(cached["OnHold"]))
	require.JSONEq(t, `"0.000000001"`, string(cached["onHold"]))
	require.JSONEq(t, `"98765432109876543210.987654321"`, string(cached["OverdraftUsed"]))
	require.JSONEq(t, `"98765432109876543210.987654321"`, string(cached["overdraftUsed"]))
	require.JSONEq(t, `"9007199254740993"`, string(cached["version"]))
	require.JSONEq(t, `0`, string(cached["AllowOverdraft"]))
	require.JSONEq(t, `false`, string(cached["allowOverdraft"]))
	require.JSONEq(t, `1`, string(cached["OverdraftLimitEnabled"]))
	require.JSONEq(t, `true`, string(cached["overdraftLimitEnabled"]))
	require.JSONEq(t, `"99999999999999999999.123456789"`, string(cached["OverdraftLimit"]))
	require.JSONEq(t, `"99999999999999999999.123456789"`, string(cached["overdraftLimit"]))
}

func TestIntegration_UpdateBalanceCacheSettings_ResultRemainsConsumableByAtomicOperation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.MustParse("55555555-5555-5555-5555-555555555555")
	ledgerID := uuid.MustParse("66666666-6666-6666-6666-666666666666")
	transactionID := uuid.MustParse("77777777-7777-7777-7777-777777777777")
	alias := "@settings-process"
	cacheKey := alias + "#default"
	op := newLimitNormalizationOperation(orgID, ledgerID, alias, 1)
	op.InternalKey = utils.BalanceInternalKey(orgID, ledgerID, cacheKey)
	seedLimitNormalizationCache(t, infra, op, "1000")

	require.NoError(t, infra.repo.UpdateBalanceCacheSettings(ctx, orgID, ledgerID, cacheKey, op.Balance.Settings))

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

	persisted, err := infra.redisContainer.Client.Get(ctx, op.InternalKey).Bytes()
	require.NoError(t, err)
	var persistedFields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(persisted, &persistedFields))
	t.Logf("persisted identity tokens: Key=%s Alias=%s key=%s alias=%s",
		persistedFields["Key"], persistedFields["Alias"], persistedFields["key"], persistedFields["alias"])
	snapshot, err := balancecache.DecodeForRead(persisted)
	require.NoError(t, err, "persisted identity tokens: Key=%s Alias=%s key=%s alias=%s",
		persistedFields["Key"], persistedFields["Alias"], persistedFields["key"], persistedFields["alias"])
	require.Equal(t, "119", snapshot.Available.String())
	require.Equal(t, int64(8), snapshot.Version)

	updatedLimit := "2000"
	op.Balance.Settings.OverdraftLimit = &updatedLimit
	require.NoError(t, infra.repo.UpdateBalanceCacheSettings(ctx, orgID, ledgerID, cacheKey, op.Balance.Settings))
	persistedAfterSettings, err := infra.redisContainer.Client.Get(ctx, op.InternalKey).Bytes()
	require.NoError(t, err)
	snapshotAfterSettings, err := balancecache.DecodeForRead(persistedAfterSettings)
	require.NoError(t, err)
	require.Equal(t, "119", snapshotAfterSettings.Available.String())
	require.Equal(t, int64(8), snapshotAfterSettings.Version)
}

func TestIntegration_UpdateBalanceCacheSettings_UpgradesNewOnlyBalanceToCoherentDualSchema(t *testing.T) {
	infra := setupRedisIntegrationInfra(t)
	ctx := t.Context()
	orgID := uuid.MustParse("81111111-1111-1111-1111-111111111111")
	ledgerID := uuid.MustParse("82222222-2222-2222-2222-222222222222")
	cacheKey := "@settings-new-only#default"
	internalKey := utils.BalanceInternalKey(orgID, ledgerID, cacheKey)
	raw := `{"SchemaVersion":2,"id":"83333333-3333-3333-3333-333333333333","accountId":"84444444-4444-4444-4444-444444444444","alias":"@settings-new-only","available":"321.000000001","onHold":"2.5","overdraftUsed":"0.000000009","version":"17","accountType":"deposit","assetCode":"USD","key":"@settings-new-only#default","allowSending":true,"allowReceiving":false,"direction":"credit","allowOverdraft":true,"overdraftLimitEnabled":false,"overdraftLimit":"25","balanceScope":"transactional"}`
	require.NoError(t, infra.redisContainer.Client.Set(ctx, internalKey, raw, time.Hour).Err())

	limit := "750.125"
	require.NoError(t, infra.repo.UpdateBalanceCacheSettings(ctx, orgID, ledgerID, cacheKey, &mmodel.BalanceSettings{
		AllowOverdraft: true, OverdraftLimitEnabled: true, OverdraftLimit: &limit,
		BalanceScope: mmodel.BalanceScopeTransactional,
	}))

	cached := readLimitNormalizationCache(t, infra, internalKey)
	require.Equal(t, "321.000000001", decodeSettingsUpdateField[string](t, cached, "Available"))
	require.Equal(t, "321.000000001", decodeSettingsUpdateField[string](t, cached, "available"))
	require.Equal(t, int64(17), decodeSettingsUpdateField[int64](t, cached, "Version"))
	require.Equal(t, "17", decodeSettingsUpdateField[string](t, cached, "version"))
	require.Equal(t, 1, decodeSettingsUpdateField[int](t, cached, "AllowOverdraft"))
	require.True(t, decodeSettingsUpdateField[bool](t, cached, "allowOverdraft"))
	require.Equal(t, "750.125", decodeSettingsUpdateField[string](t, cached, "OverdraftLimit"))
	require.Equal(t, "750.125", decodeSettingsUpdateField[string](t, cached, "overdraftLimit"))
}

func TestIntegration_UpdateBalanceCacheSettings_DivergentDualBalanceUsesLegacyAuthority(t *testing.T) {
	infra := setupRedisIntegrationInfra(t)
	ctx := t.Context()
	orgID := uuid.MustParse("91111111-1111-1111-1111-111111111111")
	ledgerID := uuid.MustParse("92222222-2222-2222-2222-222222222222")
	cacheKey := "@settings-authority#default"
	internalKey := utils.BalanceInternalKey(orgID, ledgerID, cacheKey)
	raw := `{"SchemaVersion":2,"ID":"93333333-3333-3333-3333-333333333333","id":"aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa","AccountID":"94444444-4444-4444-4444-444444444444","accountId":"bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb","Alias":"@settings-authority","alias":"@wrong","Available":"700.000000001","available":"1","OnHold":"12.50","onHold":"99","OverdraftUsed":"3.25","overdraftUsed":"88","Version":7,"version":"99","AccountType":"deposit","accountType":"liability","AssetCode":"USD","assetCode":"EUR","Key":"@settings-authority#default","key":"@wrong#default","AllowSending":1,"allowSending":false,"AllowReceiving":0,"allowReceiving":true,"Direction":"credit","direction":"debit","AllowOverdraft":0,"allowOverdraft":true,"OverdraftLimitEnabled":0,"overdraftLimitEnabled":true,"OverdraftLimit":"0","overdraftLimit":"999","BalanceScope":"transactional","balanceScope":"wrong"}`
	require.NoError(t, infra.redisContainer.Client.Set(ctx, internalKey, raw, time.Hour).Err())

	limit := "40"
	require.NoError(t, infra.repo.UpdateBalanceCacheSettings(ctx, orgID, ledgerID, cacheKey, &mmodel.BalanceSettings{
		AllowOverdraft: false, OverdraftLimitEnabled: true, OverdraftLimit: &limit,
		BalanceScope: mmodel.BalanceScopeTransactional,
	}))

	cached := readLimitNormalizationCache(t, infra, internalKey)
	require.Equal(t, "93333333-3333-3333-3333-333333333333", decodeSettingsUpdateField[string](t, cached, "id"))
	require.Equal(t, "700.000000001", decodeSettingsUpdateField[string](t, cached, "Available"))
	require.Equal(t, "700.000000001", decodeSettingsUpdateField[string](t, cached, "available"))
	require.Equal(t, "12.5", decodeSettingsUpdateField[string](t, cached, "onHold"))
	require.Equal(t, "3.25", decodeSettingsUpdateField[string](t, cached, "overdraftUsed"))
	require.Equal(t, int64(7), decodeSettingsUpdateField[int64](t, cached, "Version"))
	require.Equal(t, "7", decodeSettingsUpdateField[string](t, cached, "version"))
	require.Equal(t, 0, decodeSettingsUpdateField[int](t, cached, "AllowOverdraft"))
	require.False(t, decodeSettingsUpdateField[bool](t, cached, "allowOverdraft"))
}

func TestIntegration_UpdateBalanceCacheSettings_OverwritesMalformedLegacySettingsWithNilLimitDefault(t *testing.T) {
	infra := setupRedisIntegrationInfra(t)
	ctx := t.Context()
	orgID := uuid.MustParse("a1111111-1111-1111-1111-111111111111")
	ledgerID := uuid.MustParse("a2222222-2222-2222-2222-222222222222")
	cacheKey := "@settings-malformed#default"
	internalKey := utils.BalanceInternalKey(orgID, ledgerID, cacheKey)
	raw := `{"ID":"a3333333-3333-3333-3333-333333333333","AccountID":"a4444444-4444-4444-4444-444444444444","Alias":"@settings-malformed","Available":"80","OnHold":"4","OverdraftUsed":"2","Version":5,"AccountType":"deposit","AssetCode":"USD","Key":"@settings-malformed#default","AllowSending":1,"AllowReceiving":1,"Direction":"credit","AllowOverdraft":{"bad":true},"OverdraftLimitEnabled":[1],"OverdraftLimit":false,"BalanceScope":17}`
	require.NoError(t, infra.redisContainer.Client.Set(ctx, internalKey, raw, time.Hour).Err())

	require.NoError(t, infra.repo.UpdateBalanceCacheSettings(ctx, orgID, ledgerID, cacheKey, &mmodel.BalanceSettings{
		BalanceScope: mmodel.BalanceScopeTransactional,
	}))

	cached := readLimitNormalizationCache(t, infra, internalKey)
	require.Equal(t, 0, decodeSettingsUpdateField[int](t, cached, "AllowOverdraft"))
	require.False(t, decodeSettingsUpdateField[bool](t, cached, "allowOverdraft"))
	require.Equal(t, 0, decodeSettingsUpdateField[int](t, cached, "OverdraftLimitEnabled"))
	require.False(t, decodeSettingsUpdateField[bool](t, cached, "overdraftLimitEnabled"))
	require.Equal(t, "0", decodeSettingsUpdateField[string](t, cached, "OverdraftLimit"))
	require.Equal(t, "0", decodeSettingsUpdateField[string](t, cached, "overdraftLimit"))
	require.Equal(t, "80", decodeSettingsUpdateField[string](t, cached, "Available"))
	require.Equal(t, int64(5), decodeSettingsUpdateField[int64](t, cached, "Version"))
}

func TestIntegration_UpdateBalanceCacheSettings_CASPreservesConcurrentFinancialChange(t *testing.T) {
	infra := setupRedisIntegrationInfra(t)
	ctx := t.Context()
	orgID := uuid.MustParse("b1111111-1111-1111-1111-111111111111")
	ledgerID := uuid.MustParse("b2222222-2222-2222-2222-222222222222")
	cacheKey := "@settings-cas-change#default"
	internalKey := utils.BalanceInternalKey(orgID, ledgerID, cacheKey)
	initial := `{"ID":"b3333333-3333-3333-3333-333333333333","AccountID":"b4444444-4444-4444-4444-444444444444","Alias":"@settings-cas-change","Available":"120","OnHold":"11","OverdraftUsed":"0","Version":7,"AccountType":"deposit","AssetCode":"USD","Key":"@settings-cas-change#default","AllowSending":1,"AllowReceiving":1,"Direction":"credit","AllowOverdraft":0,"OverdraftLimitEnabled":0,"OverdraftLimit":"0","BalanceScope":"transactional"}`
	newer := `{"ID":"b3333333-3333-3333-3333-333333333333","AccountID":"b4444444-4444-4444-4444-444444444444","Alias":"@settings-cas-change","Available":"119","OnHold":"11","OverdraftUsed":"0","Version":8,"AccountType":"deposit","AssetCode":"USD","Key":"@settings-cas-change#default","AllowSending":1,"AllowReceiving":1,"Direction":"credit","AllowOverdraft":0,"OverdraftLimitEnabled":0,"OverdraftLimit":"0","BalanceScope":"transactional"}`
	require.NoError(t, infra.redisContainer.Client.Set(ctx, internalKey, initial, time.Hour).Err())

	hook := &settingsCASAfterGetHook{}
	hook.afterGet = func(ctx context.Context, read int32) error {
		if read != 1 {
			return nil
		}

		return infra.redisContainer.Client.SetArgs(ctx, internalKey, newer, redisclient.SetArgs{KeepTTL: true}).Err()
	}
	repo := newSettingsCASHookedRepository(t, infra, hook)
	limit := "500"
	require.NoError(t, repo.UpdateBalanceCacheSettings(ctx, orgID, ledgerID, cacheKey, &mmodel.BalanceSettings{
		AllowOverdraft: true, OverdraftLimitEnabled: true, OverdraftLimit: &limit,
		BalanceScope: mmodel.BalanceScopeTransactional,
	}))

	require.Equal(t, int32(2), hook.reads.Load(), "the first conflict must force one fresh read")
	cached := readLimitNormalizationCache(t, infra, internalKey)
	require.Equal(t, "119", decodeSettingsUpdateField[string](t, cached, "Available"))
	require.Equal(t, "119", decodeSettingsUpdateField[string](t, cached, "available"))
	require.Equal(t, int64(8), decodeSettingsUpdateField[int64](t, cached, "Version"))
	require.Equal(t, "8", decodeSettingsUpdateField[string](t, cached, "version"))
	require.Equal(t, 1, decodeSettingsUpdateField[int](t, cached, "AllowOverdraft"))
	require.True(t, decodeSettingsUpdateField[bool](t, cached, "allowOverdraft"))
}

func TestIntegration_UpdateBalanceCacheSettings_CASStopsAfterThreeConflictsWithoutMutation(t *testing.T) {
	infra := setupRedisIntegrationInfra(t)
	ctx := t.Context()
	orgID := uuid.MustParse("c1111111-1111-1111-1111-111111111111")
	ledgerID := uuid.MustParse("c2222222-2222-2222-2222-222222222222")
	cacheKey := "@settings-cas-exhausted#default"
	internalKey := utils.BalanceInternalKey(orgID, ledgerID, cacheKey)
	initial := `{"ID":"c3333333-3333-3333-3333-333333333333","AccountID":"c4444444-4444-4444-4444-444444444444","Alias":"@settings-cas-exhausted","Available":"120","OnHold":"11","OverdraftUsed":"0","Version":7,"AccountType":"deposit","AssetCode":"USD","Key":"@settings-cas-exhausted#default","AllowSending":1,"AllowReceiving":1,"Direction":"credit","AllowOverdraft":0,"OverdraftLimitEnabled":0,"OverdraftLimit":"0","BalanceScope":"transactional"}`
	conflicts := []string{
		`{"ID":"c3333333-3333-3333-3333-333333333333","AccountID":"c4444444-4444-4444-4444-444444444444","Alias":"@settings-cas-exhausted","Available":"119","OnHold":"11","OverdraftUsed":"0","Version":8,"AccountType":"deposit","AssetCode":"USD","Key":"@settings-cas-exhausted#default","AllowSending":1,"AllowReceiving":1,"Direction":"credit","AllowOverdraft":0,"OverdraftLimitEnabled":0,"OverdraftLimit":"0","BalanceScope":"transactional"}`,
		`{"ID":"c3333333-3333-3333-3333-333333333333","AccountID":"c4444444-4444-4444-4444-444444444444","Alias":"@settings-cas-exhausted","Available":"118","OnHold":"11","OverdraftUsed":"0","Version":9,"AccountType":"deposit","AssetCode":"USD","Key":"@settings-cas-exhausted#default","AllowSending":1,"AllowReceiving":1,"Direction":"credit","AllowOverdraft":0,"OverdraftLimitEnabled":0,"OverdraftLimit":"0","BalanceScope":"transactional"}`,
		`{"ID":"c3333333-3333-3333-3333-333333333333","AccountID":"c4444444-4444-4444-4444-444444444444","Alias":"@settings-cas-exhausted","Available":"117","OnHold":"11","OverdraftUsed":"0","Version":10,"AccountType":"deposit","AssetCode":"USD","Key":"@settings-cas-exhausted#default","AllowSending":1,"AllowReceiving":1,"Direction":"credit","AllowOverdraft":0,"OverdraftLimitEnabled":0,"OverdraftLimit":"0","BalanceScope":"transactional"}`,
	}
	require.NoError(t, infra.redisContainer.Client.Set(ctx, internalKey, initial, time.Hour).Err())
	expiresBefore, err := infra.redisContainer.Client.Do(ctx, "PEXPIRETIME", internalKey).Int64()
	require.NoError(t, err)

	hook := &settingsCASAfterGetHook{}
	hook.afterGet = func(ctx context.Context, read int32) error {
		return infra.redisContainer.Client.SetArgs(ctx, internalKey, conflicts[read-1], redisclient.SetArgs{KeepTTL: true}).Err()
	}
	repo := newSettingsCASHookedRepository(t, infra, hook)
	limit := "500"
	err = repo.UpdateBalanceCacheSettings(ctx, orgID, ledgerID, cacheKey, &mmodel.BalanceSettings{
		AllowOverdraft: true, OverdraftLimitEnabled: true, OverdraftLimit: &limit,
		BalanceScope: mmodel.BalanceScopeTransactional,
	})
	require.Error(t, err)
	require.Equal(t, int32(3), hook.reads.Load(), "the CAS attempt budget must be exactly three reads")

	stored, getErr := infra.redisContainer.Client.Get(ctx, internalKey).Result()
	require.NoError(t, getErr)
	require.Equal(t, conflicts[2], stored, "the failed settings PATCH must not alter the latest financial cache")
	expiresAfter, expiryErr := infra.redisContainer.Client.Do(ctx, "PEXPIRETIME", internalKey).Int64()
	require.NoError(t, expiryErr)
	require.Equal(t, expiresBefore, expiresAfter, "the failed settings PATCH must preserve the latest cache expiry")
}

func TestIntegration_UpdateBalanceCacheSettings_CASDoesNotRecreateBalanceDeletedAfterRead(t *testing.T) {
	infra := setupRedisIntegrationInfra(t)
	ctx := t.Context()
	orgID := uuid.MustParse("d1111111-1111-1111-1111-111111111111")
	ledgerID := uuid.MustParse("d2222222-2222-2222-2222-222222222222")
	cacheKey := "@settings-cas-delete#default"
	internalKey := utils.BalanceInternalKey(orgID, ledgerID, cacheKey)
	raw := `{"ID":"d3333333-3333-3333-3333-333333333333","AccountID":"d4444444-4444-4444-4444-444444444444","Alias":"@settings-cas-delete","Available":"120","OnHold":"11","OverdraftUsed":"0","Version":7,"AccountType":"deposit","AssetCode":"USD","Key":"@settings-cas-delete#default","AllowSending":1,"AllowReceiving":1,"Direction":"credit","AllowOverdraft":0,"OverdraftLimitEnabled":0,"OverdraftLimit":"0","BalanceScope":"transactional"}`
	require.NoError(t, infra.redisContainer.Client.Set(ctx, internalKey, raw, time.Hour).Err())

	hook := &settingsCASAfterGetHook{}
	hook.afterGet = func(ctx context.Context, read int32) error {
		if read == 1 {
			return infra.redisContainer.Client.Del(ctx, internalKey).Err()
		}

		return nil
	}
	repo := newSettingsCASHookedRepository(t, infra, hook)
	require.NoError(t, repo.UpdateBalanceCacheSettings(ctx, orgID, ledgerID, cacheKey, &mmodel.BalanceSettings{
		AllowOverdraft: true, BalanceScope: mmodel.BalanceScopeTransactional,
	}))

	exists, err := infra.redisContainer.Client.Exists(ctx, internalKey).Result()
	require.NoError(t, err)
	require.Zero(t, exists, "a balance deleted after the read must not be recreated by settings PATCH")
}
