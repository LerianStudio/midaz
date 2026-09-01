//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// =============================================================================
// ACCOUNT BLOCK FLAG — CACHE SURVIVAL INTEGRATION TESTS
// =============================================================================
// The atomic script REPLACES the Go-provided balance table with the decoded
// cache entry on every cache hit (`balance = cjson.decode(currentBalance)`).
// A field that is neither in the cached document nor re-injected by the
// backfill block is therefore DROPPED on the very next write. These tests run
// the script against a real Redis and read the raw key back, because that drop
// is invisible to any assertion made on the Go return value alone.

// blockedBalanceOp builds a balance operation carrying an explicit account
// block state, mirroring the shape the enrichment layer hands to the atomic
// script.
func blockedBalanceOp(orgID, ledgerID uuid.UUID, alias string, blocked bool, available decimal.Decimal, version int64, operation string, amount decimal.Decimal) mmodel.BalanceOperation {
	balanceKey := alias + "#" + constant.DefaultBalanceKey

	return mmodel.BalanceOperation{
		Balance: &mmodel.Balance{
			ID:             uuid.New().String(),
			OrganizationID: orgID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      uuid.New().String(),
			Alias:          alias,
			Key:            balanceKey,
			AssetCode:      "USD",
			Available:      available,
			OnHold:         decimal.Zero,
			Version:        version,
			AccountType:    "deposit",
			AllowSending:   true,
			AllowReceiving: true,
			AccountBlocked: blocked,
			Direction:      constant.DirectionCredit,
			OverdraftUsed:  decimal.Zero,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		},
		Alias: alias,
		Amount: mtransaction.Amount{
			Asset:     "USD",
			Value:     amount,
			Operation: operation,
		},
		InternalKey: utils.BalanceInternalKey(orgID, ledgerID, balanceKey),
	}
}

// TestIntegration_AccountBlocked_SurvivesFirstSeedAndCacheHitRewrite is the
// field-drop guard. A blocked balance is seeded through the script (first-seed
// encode), then mutated again through the SAME key so the second run takes the
// cache-hit path: decode-replace, backfill, rewrite. The raw cache entry must
// still report the account as blocked after the rewrite.
func TestIntegration_AccountBlocked_SurvivesFirstSeedAndCacheHitRewrite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	seedOp := blockedBalanceOp(orgID, ledgerID, "@blocked-survives", true,
		decimal.NewFromInt(500), 1, constant.DEBIT, decimal.NewFromInt(100))

	_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{seedOp})
	require.NoError(t, err)

	seeded := readCachedBalance(t, infra, seedOp.InternalKey)
	require.Equal(t, 1, seeded.AccountBlocked,
		"the first-seed encode must persist the account block flag")
	require.Equal(t, "400", seeded.Available)

	// Second operation on the same key: the SET NX fails, so the script drops
	// the Go-provided table and continues from the decoded cache entry.
	hitOp := blockedBalanceOp(orgID, ledgerID, "@blocked-survives", true,
		decimal.NewFromInt(400), 2, constant.DEBIT, decimal.NewFromInt(50))

	_, err = infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{hitOp})
	require.NoError(t, err)

	after := readCachedBalance(t, infra, hitOp.InternalKey)

	assert.Equal(t, "350", after.Available, "the cache-hit path must still apply the operation")
	assert.Equal(t, 1, after.AccountBlocked,
		"AccountBlocked must survive the decode-replace + rewrite of a cache hit")
}

// TestIntegration_AccountBlocked_ListBalanceByKeyProjectsTheFlag covers the
// direct single-key cache read, which converts the 1/0 mirror back to a bool on
// its own path — separate from the atomic script's return value.
func TestIntegration_AccountBlocked_ListBalanceByKeyProjectsTheFlag(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	op := blockedBalanceOp(orgID, ledgerID, "@blocked-bykey", true,
		decimal.NewFromInt(500), 1, constant.DEBIT, decimal.NewFromInt(100))

	_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{op})
	require.NoError(t, err)

	got, err := infra.repo.ListBalanceByKey(ctx, orgID, ledgerID, op.Balance.Key)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.True(t, got.AccountBlocked,
		"the single-key cache read must convert the 1/0 mirror back to a bool")
}

// TestIntegration_AccountBlocked_LegacyCacheEntryReadsAsUnblocked covers the
// backward-compatibility half: an entry written before the field existed must
// keep working, and must materialize as NOT blocked rather than erroring or
// staying nil inside the Lua table.
func TestIntegration_AccountBlocked_LegacyCacheEntryReadsAsUnblocked(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	op := blockedBalanceOp(orgID, ledgerID, "@blocked-legacy", false,
		decimal.NewFromInt(500), 1, constant.DEBIT, decimal.NewFromInt(100))

	// Hand-write a cache entry in the pre-field shape: no AccountBlocked key at
	// all. This is exactly what an entry seeded by the previous release looks
	// like when the new binary rolls over it.
	legacy := map[string]any{
		"ID":                    op.Balance.ID,
		"Available":             "500",
		"OnHold":                "0",
		"Version":               1,
		"AccountType":           "deposit",
		"AccountID":             op.Balance.AccountID,
		"AssetCode":             "USD",
		"AllowSending":          1,
		"AllowReceiving":        1,
		"Key":                   op.Balance.Key,
		"Direction":             constant.DirectionCredit,
		"OverdraftUsed":         "0",
		"AllowOverdraft":        0,
		"OverdraftLimitEnabled": 0,
		"OverdraftLimit":        "0",
		"BalanceScope":          mmodel.BalanceScopeTransactional,
	}

	raw, err := json.Marshal(legacy)
	require.NoError(t, err)

	require.NoError(t, infra.redisContainer.Client.Set(ctx, op.InternalKey, string(raw), time.Hour).Err())

	_, err = infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{op})
	require.NoError(t, err, "a legacy cache entry without the field must not fail the atomic operation")

	after := readCachedBalance(t, infra, op.InternalKey)

	assert.Equal(t, "400", after.Available, "the operation must still apply to a legacy entry")
	assert.Equal(t, 0, after.AccountBlocked,
		"a legacy entry must materialize as not blocked, never nil and never an error")

	// Asserted on the RAW document, not the decoded struct: a missing key and an
	// explicit 0 both decode to the Go zero value, so only the raw bytes can
	// show that the cache-hit backfill re-materialized the field instead of
	// leaving it nil for the next writer to drop again.
	rewritten, err := infra.redisContainer.Client.Get(ctx, op.InternalKey).Result()
	require.NoError(t, err)
	assert.Contains(t, rewritten, `"AccountBlocked"`,
		"the backfill must write the field back into the cached document")
}
