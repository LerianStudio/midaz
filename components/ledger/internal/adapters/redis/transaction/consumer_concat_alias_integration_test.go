//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
)

func TestIntegration_ProcessBalanceAtomicOperation_ConcatAliasColdCache(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	infra := setupRedisIntegrationInfra(t)
	orgID := uuid.New()
	ledgerID := uuid.New()
	source := redistestutil.CreateBalanceOperationWithIdentity(
		orgID, ledgerID, 0, "@concat-cold-source", "default", "USD", constant.DEBIT,
		decimal.NewFromInt(10), decimal.NewFromInt(100), decimal.Zero,
	)
	destination := redistestutil.CreateBalanceOperationWithIdentity(
		orgID, ledgerID, 0, "@concat-cold-destination", "default", "USD", constant.CREDIT,
		decimal.NewFromInt(10), decimal.NewFromInt(50), decimal.Zero,
	)

	result, err := infra.processBalanceAtomicOperationWithoutBlockException(
		t.Context(), orgID, ledgerID, uuid.New(), constant.APPROVED, false,
		[]mmodel.BalanceOperation{source, destination},
	)
	require.NoError(t, err)
	require.Len(t, result.After, 2)
	require.Equal(t, source.Alias, result.After[0].Alias)
	require.Equal(t, "90", result.After[0].Available.String())
	require.Equal(t, destination.Alias, result.After[1].Alias)
	require.Equal(t, "60", result.After[1].Available.String())
}

func TestIntegration_ProcessBalanceAtomicOperation_ConcatAliasProjectsNonDefaultKey(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	infra := setupRedisIntegrationInfra(t)
	orgID := uuid.New()
	ledgerID := uuid.New()
	op := redistestutil.CreateBalanceOperationWithIdentity(
		orgID, ledgerID, 0, "@concat-non-default", "food", "USD", constant.DEBIT,
		decimal.NewFromInt(10), decimal.NewFromInt(100), decimal.Zero,
	)

	result, err := infra.processBalanceAtomicOperationWithoutBlockException(
		t.Context(), orgID, ledgerID, uuid.New(), constant.APPROVED, false,
		[]mmodel.BalanceOperation{op},
	)
	require.NoError(t, err)
	require.Len(t, result.After, 1)
	require.Equal(t, op.Alias, result.After[0].Alias)

	cached := readConcatAliasCache(t, infra, op.InternalKey)
	require.Equal(t, op.Alias, decodeSettingsUpdateField[string](t, cached, "Alias"))
	require.Equal(t, "@concat-non-default", decodeSettingsUpdateField[string](t, cached, "alias"))
	require.Equal(t, "food", decodeSettingsUpdateField[string](t, cached, "key"))
}

func TestIntegration_ProcessBalanceAtomicOperation_ConcatAliasKeepsDuplicateLegsDistinct(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	infra := setupRedisIntegrationInfra(t)
	orgID := uuid.New()
	ledgerID := uuid.New()
	first := redistestutil.CreateBalanceOperationWithIdentity(
		orgID, ledgerID, 0, "@concat-duplicate", "default", "USD", constant.DEBIT,
		decimal.NewFromInt(10), decimal.NewFromInt(100), decimal.Zero,
	)
	second := first
	second.Alias = "1#@concat-duplicate#default"
	second.Amount.Value = decimal.NewFromInt(20)

	result, err := infra.processBalanceAtomicOperationWithoutBlockException(
		t.Context(), orgID, ledgerID, uuid.New(), constant.APPROVED, false,
		[]mmodel.BalanceOperation{first, second},
	)
	require.NoError(t, err)
	require.Len(t, result.After, 2)
	require.Equal(t, first.Alias, result.After[0].Alias)
	require.Equal(t, "90", result.After[0].Available.String())
	require.Equal(t, int64(2), result.After[0].Version)
	require.Equal(t, second.Alias, result.After[1].Alias)
	require.Equal(t, "70", result.After[1].Available.String())
	require.Equal(t, int64(3), result.After[1].Version)
}

func TestIntegration_ProcessBalanceAtomicOperation_ConcatAliasPendingCommit(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	infra := setupRedisIntegrationInfra(t)
	orgID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()
	hold := redistestutil.CreateBalanceOperationWithIdentity(
		orgID, ledgerID, 0, "@concat-pending", "default", "USD", constant.ONHOLD,
		decimal.NewFromInt(10), decimal.NewFromInt(100), decimal.Zero,
	)

	pending, err := infra.processBalanceAtomicOperationWithoutBlockException(
		t.Context(), orgID, ledgerID, transactionID, constant.PENDING, true,
		[]mmodel.BalanceOperation{hold},
	)
	require.NoError(t, err)
	require.Len(t, pending.After, 1)
	require.Equal(t, hold.Alias, pending.After[0].Alias)
	require.Equal(t, "90", pending.After[0].Available.String())
	require.Equal(t, "10", pending.After[0].OnHold.String())

	commit := hold
	committedBalance := *hold.Balance
	commit.Balance = &committedBalance
	commit.Amount.Operation = constant.DEBIT
	commit.Balance.Available = pending.After[0].Available
	commit.Balance.OnHold = pending.After[0].OnHold
	commit.Balance.Version = pending.After[0].Version

	approved, err := infra.processBalanceAtomicOperationWithoutBlockException(
		t.Context(), orgID, ledgerID, transactionID, constant.APPROVED, true,
		[]mmodel.BalanceOperation{commit},
	)
	require.NoError(t, err)
	require.Len(t, approved.After, 1)
	require.Equal(t, commit.Alias, approved.After[0].Alias)
	require.Equal(t, "90", approved.After[0].Available.String())
	require.Equal(t, "0", approved.After[0].OnHold.String())
	require.Equal(t, int64(3), approved.After[0].Version)
}

func TestIntegration_ProcessBalanceAtomicOperation_ConcatAliasAcceptsExistingCacheShapes(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	infra := setupRedisIntegrationInfra(t)
	orgID := uuid.New()
	ledgerID := uuid.New()

	t.Run("legacy uppercase cache", func(t *testing.T) {
		op := redistestutil.CreateBalanceOperationWithIdentity(
			orgID, ledgerID, 0, "@concat-legacy", "default", "USD", constant.DEBIT,
			decimal.NewFromInt(1), decimal.NewFromInt(100), decimal.Zero,
		)
		seedConcatAliasCache(t, infra, op.InternalKey, legacyConcatAliasCache(op))

		result, err := infra.processBalanceAtomicOperationWithoutBlockException(
			t.Context(), orgID, ledgerID, uuid.New(), constant.APPROVED, false,
			[]mmodel.BalanceOperation{op},
		)
		require.NoError(t, err)
		require.Len(t, result.After, 1)
		require.Equal(t, op.Alias, result.After[0].Alias)
		require.Equal(t, "99", result.After[0].Available.String())
	})

	t.Run("modern lower-camel cache", func(t *testing.T) {
		op := redistestutil.CreateBalanceOperationWithIdentity(
			orgID, ledgerID, 0, "@concat-modern", "default", "USD", constant.DEBIT,
			decimal.NewFromInt(1), decimal.NewFromInt(100), decimal.Zero,
		)
		seedConcatAliasCache(t, infra, op.InternalKey, modernConcatAliasCache(op))

		result, err := infra.processBalanceAtomicOperationWithoutBlockException(
			t.Context(), orgID, ledgerID, uuid.New(), constant.APPROVED, false,
			[]mmodel.BalanceOperation{op},
		)
		require.NoError(t, err)
		require.Len(t, result.After, 1)
		require.Equal(t, op.Alias, result.After[0].Alias)
		require.Equal(t, "99", result.After[0].Available.String())
	})
}

func legacyConcatAliasCache(op mmodel.BalanceOperation) map[string]any {
	return map[string]any{
		"ID": op.Balance.ID, "AccountID": op.Balance.AccountID, "Alias": op.Alias,
		"Available": "100", "OnHold": "0", "Version": 1,
		"AccountType": op.Balance.AccountType, "AssetCode": op.Balance.AssetCode,
		"AllowSending": 1, "AllowReceiving": 1, "Key": op.Balance.Key,
	}
}

func modernConcatAliasCache(op mmodel.BalanceOperation) map[string]any {
	return map[string]any{
		"SchemaVersion": 2,
		"id":            op.Balance.ID, "accountId": op.Balance.AccountID,
		"available": "100", "onHold": "0", "overdraftUsed": "0", "version": "1",
		"accountType": op.Balance.AccountType, "assetCode": op.Balance.AssetCode,
		"allowSending": true, "allowReceiving": true, "blocked": false,
		"alias": op.Balance.Alias, "key": op.Balance.Key, "direction": "",
		"allowOverdraft": false, "overdraftLimitEnabled": false, "overdraftLimit": "0",
		"balanceScope": mmodel.BalanceScopeTransactional,
	}
}

func seedConcatAliasCache(t *testing.T, infra *integrationTestInfra, key string, cache map[string]any) {
	t.Helper()
	raw, err := json.Marshal(cache)
	require.NoError(t, err)
	require.NoError(t, infra.redisContainer.Client.Set(t.Context(), key, raw, time.Hour).Err())
}

func readConcatAliasCache(t *testing.T, infra *integrationTestInfra, key string) map[string]json.RawMessage {
	t.Helper()
	raw, err := infra.redisContainer.Client.Get(t.Context(), key).Bytes()
	require.NoError(t, err)

	var cache map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &cache))

	return cache
}
