//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v3/pkg/utils"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// DIRECTION, STALE-VERSION, AND SYNC-SCHEDULING TESTS (IS-6..IS-8, IS-10)
// =============================================================================
//
// Helpers (`overdraftOp`, `ptrString`, `cachedBalance`) are defined in the
// companion file `consumer_overdraft_split_integration_test.go`.

// TestIntegration_Overdraft_DebitDirection_DebitOperation exercises IS-6.
// For direction=debit balances, DEBIT increases Available (inverted polarity).
// This matches Go's applyDebitDirectionBalance and the PRD's overdraft model
// where debiting the overdraft balance records usage (increases the debt).
func TestIntegration_Overdraft_DebitDirection_DebitOperation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	// direction=debit: DEBIT adds → 100 + 50 = 150
	op := overdraftOp(orgID, ledgerID, "@is6-debit-dir", "deposit", "debit",
		decimal.NewFromInt(100), decimal.Zero, 1, nil,
		constant.DEBIT, decimal.NewFromInt(50))

	result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{op})

	require.NoError(t, err)
	require.Len(t, result.After, 1)
	assert.True(t, result.After[0].Available.Equal(decimal.NewFromInt(150)),
		"DEBIT on direction=debit should increase Available, got %s", result.After[0].Available)
}

// TestIntegration_Overdraft_DebitDirection_CreditOperation exercises IS-7.
// For direction=debit balances, CREDIT decreases Available (inverted polarity).
// This matches Go's applyDebitDirectionBalance and the PRD's repayment model
// where crediting the overdraft balance reduces the tracked debt.
func TestIntegration_Overdraft_DebitDirection_CreditOperation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	// direction=debit: CREDIT subtracts → 100 - 30 = 70
	op := overdraftOp(orgID, ledgerID, "@is7-debit-credit", "deposit", "debit",
		decimal.NewFromInt(100), decimal.Zero, 1, nil,
		constant.CREDIT, decimal.NewFromInt(30))

	result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{op})

	require.NoError(t, err)
	require.Len(t, result.After, 1)
	assert.True(t, result.After[0].Available.Equal(decimal.NewFromInt(70)),
		"CREDIT on direction=debit should decrease Available, got %s",
		result.After[0].Available)
}

// TestIntegration_Overdraft_StaleVersion_Returns0175 exercises IS-8: when the
// cache holds a newer Version than the caller supplied, the Lua overdraft
// branch refuses to apply the split and returns error code 0175.
//
// The setup pre-seeds the balance key with Version=6, then submits an
// operation carrying Version=5 in ARGV. The script's `SET NX` fails, the
// cached entry is loaded (Version=6), the deficit triggers the overdraft
// branch, and the stale-version guard fires.
func TestIntegration_Overdraft_StaleVersion_Returns0175(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	alias := "@is8-stale"
	balanceKey := alias + "#default"
	internalKey := utils.BalanceInternalKey(orgID, ledgerID, balanceKey)

	seeded := cachedBalance{
		ID:                    uuid.New().String(),
		Available:             "100",
		OnHold:                "0",
		Version:               6,
		AccountType:           "deposit",
		AccountID:             uuid.New().String(),
		AssetCode:             "USD",
		AllowSending:          1,
		AllowReceiving:        1,
		Key:                   balanceKey,
		Direction:             "credit",
		OverdraftUsed:         "0",
		AllowOverdraft:        1,
		OverdraftLimitEnabled: 0,
		OverdraftLimit:        "0",
		BalanceScope:          mmodel.BalanceScopeTransactional,
	}
	payload, marshalErr := json.Marshal(seeded)
	require.NoError(t, marshalErr)
	require.NoError(t, infra.redisContainer.Client.Set(
		context.Background(), internalKey, payload, time.Hour).Err())

	settings := &mmodel.BalanceSettings{
		BalanceScope:   mmodel.BalanceScopeTransactional,
		AllowOverdraft: true,
	}
	op := mmodel.BalanceOperation{
		Balance: &mmodel.Balance{
			ID:             seeded.ID,
			OrganizationID: orgID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      seeded.AccountID,
			Alias:          alias,
			Key:            balanceKey,
			AssetCode:      "USD",
			Available:      decimal.NewFromInt(100),
			OnHold:         decimal.Zero,
			Version:        5, // stale: cache has 6
			AccountType:    "deposit",
			AllowSending:   true,
			AllowReceiving: true,
			Direction:      "credit",
			OverdraftUsed:  decimal.Zero,
			Settings:       settings,
		},
		Alias: alias,
		Amount: mtransaction.Amount{
			Asset:     "USD",
			Value:     decimal.NewFromInt(200),
			Operation: constant.DEBIT,
		},
		InternalKey: internalKey,
	}

	_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{op})

	require.Error(t, err, "stale version must be rejected")
	assert.True(t, strings.Contains(err.Error(), constant.ErrStaleBalanceVersion.Error()),
		"error should contain 0175, got: %v", err)
}

// TestIntegration_Overdraft_VersionIncrementAndSyncScheduling exercises IS-10:
// when overdraft accrual changes OverdraftUsed, the Lua script must increment
// Version and enqueue the balance key in the sync-scheduling ZSET so the
// balance-sync worker picks it up for PostgreSQL persistence.
func TestIntegration_Overdraft_VersionIncrementAndSyncScheduling(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	settings := &mmodel.BalanceSettings{
		BalanceScope:          mmodel.BalanceScopeTransactional,
		AllowOverdraft:        true,
		OverdraftLimitEnabled: false,
	}
	op := overdraftOp(orgID, ledgerID, "@is10-sync", "deposit", "credit",
		decimal.NewFromInt(100), decimal.Zero, 1, settings,
		constant.DEBIT, decimal.NewFromInt(150))

	result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{op})

	require.NoError(t, err)
	require.Len(t, result.After, 1)

	assert.Equal(t, int64(2), result.After[0].Version,
		"Version must increment when OverdraftUsed changes, got %d",
		result.After[0].Version)

	// ZSET entry proves the balance was scheduled for async PostgreSQL sync.
	score, zerr := infra.redisContainer.Client.ZScore(
		context.Background(), utils.BalanceSyncScheduleKey, op.InternalKey).Result()
	require.NoError(t, zerr, "balance key should be present in the sync-schedule ZSET")
	assert.Greater(t, score, float64(0), "scheduled dueAt must be a positive timestamp")
}
