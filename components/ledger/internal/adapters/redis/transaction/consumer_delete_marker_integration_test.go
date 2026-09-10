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

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/utils"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// DELETE MARKER GUARD INTEGRATION TESTS (honored-lock)
// =============================================================================
// These tests cover the Lua pre-pass in balance_atomic_operation.lua that
// rejects a batch with ErrAccountIneligibility (0019) when any balance in it
// carries a live marker in the dedicated delete-marker namespace, before any mutation runs.

const deleteMarkerNamespacePrefix = "balance_delete_marker:{transactions}:"

func deleteMarkerKeyForIntegration(balanceKey string) string {
	return deleteMarkerNamespacePrefix + strings.TrimPrefix(balanceKey, "balance:{transactions}:")
}

func legacyDeleteMarkerKeyForIntegration(balanceKey string) string {
	return balanceKey + ":deleted"
}

// readCachedBalance fetches and decodes the balance cache entry written by the
// Lua script for the given key. It fails the test if the key is missing.
func readCachedBalance(t *testing.T, infra *integrationTestInfra, key string) cachedBalance {
	t.Helper()

	raw, err := infra.redisContainer.Client.Get(context.Background(), key).Result()
	require.NoError(t, err, "balance cache key %q must exist", key)

	var cb cachedBalance
	require.NoError(t, json.Unmarshal([]byte(raw), &cb))

	return cb
}

// TestIntegration_DeleteMarker_RejectsAndDoesNotMutate exercises case (a):
// a live delete marker on the single balance in the batch makes the atomic op
// return 0019 and leaves the balance cache value/version untouched.
func TestIntegration_DeleteMarker_RejectsAndDoesNotMutate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	// Prime the balance cache with a successful op (version 1 -> 2, 500 -> 300).
	primeOp := overdraftOp(orgID, ledgerID, "@ts-single", "deposit", "credit",
		decimal.NewFromInt(500), decimal.Zero, 1, nil,
		constant.DEBIT, decimal.NewFromInt(200))

	_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{primeOp}, nil)
	require.NoError(t, err)

	before := readCachedBalance(t, infra, primeOp.InternalKey)

	// Lay down the delete marker on the SEPARATE key; the balance key is untouched.
	deleteMarkerKey := deleteMarkerKeyForIntegration(primeOp.InternalKey)
	require.NoError(t, infra.redisContainer.Client.Set(ctx, deleteMarkerKey, "1", 0).Err())
	markerExists, markerErr := infra.redisContainer.Client.Exists(ctx, deleteMarkerKey).Result()
	require.NoError(t, markerErr)
	require.Equal(t, int64(1), markerExists, "the dedicated marker must be present before the guarded operation")

	// A subsequent op on the balance carrying a delete marker must be rejected with 0019.
	rejectOp := overdraftOp(orgID, ledgerID, "@ts-single", "deposit", "credit",
		decimal.NewFromInt(300), decimal.Zero, before.Version, nil,
		constant.DEBIT, decimal.NewFromInt(100))

	_, err = infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{rejectOp}, nil)

	require.Error(t, err, "balance carrying a delete marker must be rejected")
	assert.True(t, strings.Contains(err.Error(), constant.ErrAccountIneligibility.Error()),
		"error should contain 0019, got: %v", err)

	// No mutation: value and version are exactly the pre-delete-marker snapshot.
	after := readCachedBalance(t, infra, primeOp.InternalKey)
	assert.Equal(t, before.Available, after.Available,
		"Available must be unchanged when the batch is rejected")
	assert.Equal(t, before.Version, after.Version,
		"Version must not increment when the batch is rejected")
}

func TestIntegration_DeleteMarker_MixedLegacyAndNamespacedMarkersReject(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	op := overdraftOp(orgID, ledgerID, "@ts-mixed-marker", "deposit", "credit",
		decimal.NewFromInt(500), decimal.Zero, 1, nil,
		constant.DEBIT, decimal.NewFromInt(100))

	for _, markerKey := range []string{
		legacyDeleteMarkerKeyForIntegration(op.InternalKey),
		deleteMarkerKeyForIntegration(op.InternalKey),
	} {
		_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
			uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{op}, nil)
		require.NoError(t, err)
		require.NoError(t, infra.redisContainer.Client.Set(ctx, markerKey, "owner", 0).Err())

		_, err = infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
			uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{op}, nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), constant.ErrAccountIneligibility.Error())
		require.NoError(t, infra.redisContainer.Client.Del(ctx, markerKey).Err())
	}
}

func TestIntegration_DeleteMarker_TenantNamespaceRejectsMatchingBalance(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	tenantID := "delete-marker-tenant-" + uuid.NewString()
	ctx := tmcore.ContextWithTenantID(context.Background(), tenantID)
	orgID := uuid.New()
	ledgerID := uuid.New()
	op := overdraftOp(orgID, ledgerID, "@ts-tenant-marker", "deposit", "credit",
		decimal.NewFromInt(500), decimal.Zero, 1, nil,
		constant.DEBIT, decimal.NewFromInt(100))

	// SetNX in the command layer would namespace this logical marker as below.
	// The Lua operation receives the tenant-prefixed balance key and must replace
	// its balance namespace in-place to find this physical marker.
	physicalMarkerKey := "tenant:" + tenantID + ":" + deleteMarkerKeyForIntegration(op.InternalKey)
	require.NoError(t, infra.redisContainer.Client.Set(ctx, physicalMarkerKey, "owner", 0).Err())
	markerExists, markerErr := infra.redisContainer.Client.Exists(context.Background(), physicalMarkerKey).Result()
	require.NoError(t, markerErr)
	require.Equal(t, int64(1), markerExists, "the tenant-prefixed marker must be present before the guarded operation")

	_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{op}, nil)

	require.Error(t, err)
	assert.Contains(t, err.Error(), constant.ErrAccountIneligibility.Error())
}

// TestIntegration_DeleteMarker_NoMarker_ProceedsNormally exercises case
// (b): with no delete marker present, the atomic op mutates the balance as before.
// This guards against the pre-pass rejecting healthy batches.
func TestIntegration_DeleteMarker_NoMarker_ProceedsNormally(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	op := overdraftOp(orgID, ledgerID, "@ts-none", "deposit", "credit",
		decimal.NewFromInt(500), decimal.Zero, 1, nil,
		constant.DEBIT, decimal.NewFromInt(200))

	result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{op}, nil)

	require.NoError(t, err, "no delete marker -> op must proceed")
	require.Len(t, result.After, 1)
	assert.True(t, result.After[0].Available.Equal(decimal.NewFromInt(300)),
		"Available should decrement normally, got %s", result.After[0].Available)
}

// TestIntegration_DeleteMarker_BatchAtomicity exercises case (c): a
// two-balance batch where only one balance carries a delete marker. The whole batch is
// rejected with 0019 and the balance without a delete marker is left unmutated, proving
// the pre-pass runs before any mutation in the batch.
func TestIntegration_DeleteMarker_BatchAtomicity(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	opA := overdraftOp(orgID, ledgerID, "@ts-batch-a", "deposit", "credit",
		decimal.NewFromInt(500), decimal.Zero, 1, nil,
		constant.DEBIT, decimal.NewFromInt(100))
	opB := overdraftOp(orgID, ledgerID, "@ts-batch-b", "deposit", "credit",
		decimal.NewFromInt(500), decimal.Zero, 1, nil,
		constant.DEBIT, decimal.NewFromInt(100))

	// Prime both balances in the cache with a healthy batch.
	_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{opA, opB}, nil)
	require.NoError(t, err)

	beforeA := readCachedBalance(t, infra, opA.InternalKey)

	// Delete marker ONLY balance B.
	require.NoError(t, infra.redisContainer.Client.Set(ctx, deleteMarkerKeyForIntegration(opB.InternalKey), "1", 0).Err())

	// Re-run the batch (A first, then B carrying a delete marker). The pre-pass must
	// reject the whole batch before A is mutated.
	nextA := overdraftOp(orgID, ledgerID, "@ts-batch-a", "deposit", "credit",
		beforeA.availableDecimal(t), decimal.Zero, beforeA.Version, nil,
		constant.DEBIT, decimal.NewFromInt(50))
	nextB := overdraftOp(orgID, ledgerID, "@ts-batch-b", "deposit", "credit",
		decimal.NewFromInt(400), decimal.Zero, 2, nil,
		constant.DEBIT, decimal.NewFromInt(50))

	_, err = infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{nextA, nextB}, nil)

	require.Error(t, err, "batch with a balance carrying a delete marker must be rejected")
	assert.True(t, strings.Contains(err.Error(), constant.ErrAccountIneligibility.Error()),
		"error should contain 0019, got: %v", err)

	// The balance without a delete marker A must be untouched (pre-pass atomicity).
	afterA := readCachedBalance(t, infra, opA.InternalKey)
	assert.Equal(t, beforeA.Available, afterA.Available,
		"balance without a delete marker Available must be unchanged")
	assert.Equal(t, beforeA.Version, afterA.Version,
		"balance without a delete marker Version must not increment")
}

func TestIntegration_DeleteMarkerNamespaceCannotCollideWithSiblingBalanceKey(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	alias := "@ts-collision"
	baseInternalKey := utils.BalanceInternalKey(orgID, ledgerID, alias+"#usd")
	siblingInternalKey := utils.BalanceInternalKey(orgID, ledgerID, alias+"#usd:deleted")
	markerKey := deleteMarkerKeyForIntegration(baseInternalKey)

	// This is the marker for balance "usd". A valid sibling balance is allowed to
	// use "usd:deleted" as its key and must remain independently mutable.
	assert.NotEqual(t, markerKey, siblingInternalKey)
	require.NoError(t, infra.redisContainer.Client.Set(ctx, markerKey, "owner", 0).Err())

	sibling := mmodel.BalanceOperation{
		Balance: &mmodel.Balance{
			ID:             uuid.NewString(),
			OrganizationID: orgID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      uuid.NewString(),
			Alias:          alias,
			Key:            "usd:deleted",
			AssetCode:      "USD",
			Available:      decimal.NewFromInt(500),
			OnHold:         decimal.Zero,
			Version:        1,
			AccountType:    "deposit",
			AllowSending:   true,
			AllowReceiving: true,
			Direction:      "credit",
			OverdraftUsed:  decimal.Zero,
		},
		Alias:       alias + "#usd:deleted",
		Amount:      mtransaction.Amount{Asset: "USD", Value: decimal.NewFromInt(100), Operation: constant.DEBIT},
		InternalKey: siblingInternalKey,
	}

	result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{sibling}, nil)

	require.NoError(t, err, "a marker for usd must not block the valid sibling balance usd:deleted")
	require.Len(t, result.After, 1)
	assert.True(t, result.After[0].Available.Equal(decimal.NewFromInt(400)))
}

// availableDecimal parses the cached Available string into a decimal for reuse
// as the next op's read version input.
func (cb cachedBalance) availableDecimal(t *testing.T) decimal.Decimal {
	t.Helper()

	d, err := decimal.NewFromString(cb.Available)
	require.NoError(t, err)

	return d
}
