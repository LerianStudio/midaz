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
// SHARED OVERDRAFT TEST HELPERS (used by both overdraft integration files)
// =============================================================================
// overdraftOp builds a BalanceOperation with Direction/Settings populated,
// since the shared fixtures helper does not expose those fields.
func overdraftOp(
	orgID, ledgerID uuid.UUID,
	alias, accountType, direction string,
	available, overdraftUsed decimal.Decimal,
	version int64, settings *mmodel.BalanceSettings,
	operation string, amount decimal.Decimal,
) mmodel.BalanceOperation {
	balanceKey := alias + "#default"

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
			AccountType:    accountType,
			AllowSending:   true,
			AllowReceiving: true,
			Direction:      direction,
			OverdraftUsed:  overdraftUsed,
			Settings:       settings,
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

func ptrString(s string) *string { return &s }

// cachedBalance mirrors the JSON shape the Lua script writes to Redis.
type cachedBalance struct {
	ID                    string `json:"ID"`
	Available             string `json:"Available"`
	OnHold                string `json:"OnHold"`
	Version               int64  `json:"Version"`
	AccountType           string `json:"AccountType"`
	AccountID             string `json:"AccountID"`
	AssetCode             string `json:"AssetCode"`
	AllowSending          int    `json:"AllowSending"`
	AllowReceiving        int    `json:"AllowReceiving"`
	Key                   string `json:"Key"`
	Direction             string `json:"Direction"`
	OverdraftUsed         string `json:"OverdraftUsed"`
	AllowOverdraft        int    `json:"AllowOverdraft"`
	OverdraftLimitEnabled int    `json:"OverdraftLimitEnabled"`
	OverdraftLimit        string `json:"OverdraftLimit"`
	BalanceScope          string `json:"BalanceScope"`
}

// =============================================================================
// OVERDRAFT SPLIT & LIMIT INTEGRATION TESTS (IS-1..IS-5, IS-9)
// =============================================================================
// TestIntegration_Overdraft_DirectionAwareDebit_NoOverdraft exercises IS-1:
// a debit on a credit-direction balance with overdraft disabled simply
// decreases Available and never touches OverdraftUsed.
func TestIntegration_Overdraft_DirectionAwareDebit_NoOverdraft(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	op := overdraftOp(orgID, ledgerID, "@is1-credit", "deposit", "credit",
		decimal.NewFromInt(500), decimal.Zero, 1, nil,
		constant.DEBIT, decimal.NewFromInt(200))

	result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{op})

	require.NoError(t, err)
	require.Len(t, result.After, 1)

	assert.True(t, result.After[0].Available.Equal(decimal.NewFromInt(300)),
		"Available should decrement normally without overdraft, got %s",
		result.After[0].Available)
	assert.True(t, result.After[0].OverdraftUsed.IsZero(),
		"OverdraftUsed must remain zero, got %s", result.After[0].OverdraftUsed)
}

// TestIntegration_Overdraft_UnlimitedSplit_FloorsAtZero exercises IS-2:
// unlimited overdraft floors Available at zero and accrues the deficit
// in OverdraftUsed.
func TestIntegration_Overdraft_UnlimitedSplit_FloorsAtZero(t *testing.T) {
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
	op := overdraftOp(orgID, ledgerID, "@is2-split", "deposit", "credit",
		decimal.NewFromInt(100), decimal.Zero, 1, settings,
		constant.DEBIT, decimal.NewFromInt(250))

	result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{op})

	require.NoError(t, err)
	require.Len(t, result.After, 1)

	assert.True(t, result.After[0].Available.IsZero(),
		"Available should floor at zero, got %s", result.After[0].Available)
	assert.True(t, result.After[0].OverdraftUsed.Equal(decimal.NewFromInt(150)),
		"OverdraftUsed should equal the deficit (150), got %s",
		result.After[0].OverdraftUsed)
}

// TestIntegration_Overdraft_LimitExceeded_Returns0167 exercises IS-3: a
// projected OverdraftUsed strictly above the limit is rejected with 0167
// and the cached balance is rolled back to its pre-op state.
func TestIntegration_Overdraft_LimitExceeded_Returns0167(t *testing.T) {
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
		OverdraftLimitEnabled: true,
		OverdraftLimit:        ptrString("50"),
	}
	op := overdraftOp(orgID, ledgerID, "@is3-rejected", "deposit", "credit",
		decimal.NewFromInt(100), decimal.Zero, 1, settings,
		constant.DEBIT, decimal.NewFromInt(200))

	_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{op})

	require.Error(t, err, "deficit=100 with limit=50 must be rejected")
	assert.True(t, strings.Contains(err.Error(), constant.ErrOverdraftLimitExceeded.Error()),
		"error should contain 0167, got: %v", err)

	// Rollback invariant: the balance cache retains the pre-operation snapshot.
	raw, getErr := infra.redisContainer.Client.Get(context.Background(), op.InternalKey).Result()
	require.NoError(t, getErr, "balance key should still exist after rollback")

	var cb cachedBalance

	require.NoError(t, json.Unmarshal([]byte(raw), &cb))
	assert.Equal(t, "100", cb.Available, "Available must be unchanged after rollback")
	assert.Equal(t, "0", cb.OverdraftUsed, "OverdraftUsed must be unchanged after rollback")
	assert.Equal(t, int64(1), cb.Version, "Version must not increment on rollback")
}

// TestIntegration_Overdraft_LimitBoundary_Allowed exercises IS-4: a deficit
// exactly equal to the limit is allowed (inclusive boundary).
func TestIntegration_Overdraft_LimitBoundary_Allowed(t *testing.T) {
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
		OverdraftLimitEnabled: true,
		OverdraftLimit:        ptrString("100"),
	}
	op := overdraftOp(orgID, ledgerID, "@is4-boundary", "deposit", "credit",
		decimal.NewFromInt(100), decimal.Zero, 1, settings,
		constant.DEBIT, decimal.NewFromInt(200))

	result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{op})

	require.NoError(t, err, "deficit=limit=100 is inclusive and must be allowed")
	require.Len(t, result.After, 1)

	assert.True(t, result.After[0].Available.IsZero(), "Available should be zero")
	assert.True(t, result.After[0].OverdraftUsed.Equal(decimal.NewFromInt(100)),
		"OverdraftUsed should equal limit (100), got %s",
		result.After[0].OverdraftUsed)
}

// TestIntegration_Overdraft_CumulativeAccrual exercises IS-5: an existing
// OverdraftUsed accumulates with the new deficit under the limit.
func TestIntegration_Overdraft_CumulativeAccrual(t *testing.T) {
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
		OverdraftLimitEnabled: true,
		OverdraftLimit:        ptrString("200"),
	}
	op := overdraftOp(orgID, ledgerID, "@is5-cumulative", "deposit", "credit",
		decimal.Zero, decimal.NewFromInt(80), 1, settings,
		constant.DEBIT, decimal.NewFromInt(50))

	result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{op})

	require.NoError(t, err)
	require.Len(t, result.After, 1)

	assert.True(t, result.After[0].Available.IsZero(), "Available stays floored at zero")
	assert.True(t, result.After[0].OverdraftUsed.Equal(decimal.NewFromInt(130)),
		"OverdraftUsed should be 80+50=130, got %s",
		result.After[0].OverdraftUsed)
}

// TestIntegration_Overdraft_ExternalAccount_BypassesOverdraft exercises IS-9:
// external accounts bypass the overdraft branch and still accept negative
// Available on DEBIT.
func TestIntegration_Overdraft_ExternalAccount_BypassesOverdraft(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	op := overdraftOp(orgID, ledgerID, "@is9-external", "external", "credit",
		decimal.NewFromInt(100), decimal.Zero, 1, nil,
		constant.DEBIT, decimal.NewFromInt(200))

	result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{op})

	require.NoError(t, err, "external DEBIT must succeed even when going negative")
	require.Len(t, result.After, 1)

	assert.True(t, result.After[0].Available.Equal(decimal.NewFromInt(-100)),
		"external account Available should be -100, got %s",
		result.After[0].Available)
	assert.True(t, result.After[0].OverdraftUsed.IsZero(),
		"overdraft logic must not trigger for external accounts")
}
