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

	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// DELETE TOMBSTONE GUARD INTEGRATION TESTS (honored-lock)
// =============================================================================
// These tests cover the Lua pre-pass in balance_atomic_operation.lua that
// rejects a batch with ErrAccountIneligibility (0019) when any balance in it
// carries a live "<balanceKey>:deleted" tombstone, before any mutation runs.

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

// TestIntegration_DeleteTombstone_RejectsAndDoesNotMutate exercises case (a):
// a live tombstone on the single balance in the batch makes the atomic op
// return 0019 and leaves the balance cache value/version untouched.
func TestIntegration_DeleteTombstone_RejectsAndDoesNotMutate(t *testing.T) {
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
		uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{primeOp})
	require.NoError(t, err)

	before := readCachedBalance(t, infra, primeOp.InternalKey)

	// Lay down the tombstone on the SEPARATE key; the balance key is untouched.
	tombstoneKey := primeOp.InternalKey + ":deleted"
	require.NoError(t, infra.redisContainer.Client.Set(ctx, tombstoneKey, "1", 0).Err())

	// A subsequent op on the tombstoned balance must be rejected with 0019.
	rejectOp := overdraftOp(orgID, ledgerID, "@ts-single", "deposit", "credit",
		decimal.NewFromInt(300), decimal.Zero, before.Version, nil,
		constant.DEBIT, decimal.NewFromInt(100))

	_, err = infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{rejectOp})

	require.Error(t, err, "tombstoned balance must be rejected")
	assert.True(t, strings.Contains(err.Error(), constant.ErrAccountIneligibility.Error()),
		"error should contain 0019, got: %v", err)

	// No mutation: value and version are exactly the pre-tombstone snapshot.
	after := readCachedBalance(t, infra, primeOp.InternalKey)
	assert.Equal(t, before.Available, after.Available,
		"Available must be unchanged when the batch is rejected")
	assert.Equal(t, before.Version, after.Version,
		"Version must not increment when the batch is rejected")
}

// TestIntegration_DeleteTombstone_NoTombstone_ProceedsNormally exercises case
// (b): with no tombstone present, the atomic op mutates the balance as before.
// This guards against the pre-pass rejecting healthy batches.
func TestIntegration_DeleteTombstone_NoTombstone_ProceedsNormally(t *testing.T) {
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
		uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{op})

	require.NoError(t, err, "no tombstone -> op must proceed")
	require.Len(t, result.After, 1)
	assert.True(t, result.After[0].Available.Equal(decimal.NewFromInt(300)),
		"Available should decrement normally, got %s", result.After[0].Available)
}

// TestIntegration_DeleteTombstone_BatchAtomicity exercises case (c): a
// two-balance batch where only one balance is tombstoned. The whole batch is
// rejected with 0019 and the NON-tombstoned balance is left unmutated, proving
// the pre-pass runs before any mutation in the batch.
func TestIntegration_DeleteTombstone_BatchAtomicity(t *testing.T) {
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
		uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{opA, opB})
	require.NoError(t, err)

	beforeA := readCachedBalance(t, infra, opA.InternalKey)

	// Tombstone ONLY balance B.
	require.NoError(t, infra.redisContainer.Client.Set(ctx, opB.InternalKey+":deleted", "1", 0).Err())

	// Re-run the batch (A first, then the tombstoned B). The pre-pass must
	// reject the whole batch before A is mutated.
	nextA := overdraftOp(orgID, ledgerID, "@ts-batch-a", "deposit", "credit",
		beforeA.availableDecimal(t), decimal.Zero, beforeA.Version, nil,
		constant.DEBIT, decimal.NewFromInt(50))
	nextB := overdraftOp(orgID, ledgerID, "@ts-batch-b", "deposit", "credit",
		decimal.NewFromInt(400), decimal.Zero, 2, nil,
		constant.DEBIT, decimal.NewFromInt(50))

	_, err = infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{nextA, nextB})

	require.Error(t, err, "batch with a tombstoned balance must be rejected")
	assert.True(t, strings.Contains(err.Error(), constant.ErrAccountIneligibility.Error()),
		"error should contain 0019, got: %v", err)

	// The non-tombstoned balance A must be untouched (pre-pass atomicity).
	afterA := readCachedBalance(t, infra, opA.InternalKey)
	assert.Equal(t, beforeA.Available, afterA.Available,
		"non-tombstoned balance Available must be unchanged")
	assert.Equal(t, beforeA.Version, afterA.Version,
		"non-tombstoned balance Version must not increment")
}

// availableDecimal parses the cached Available string into a decimal for reuse
// as the next op's read version input.
func (cb cachedBalance) availableDecimal(t *testing.T) decimal.Decimal {
	t.Helper()

	d, err := decimal.NewFromString(cb.Available)
	require.NoError(t, err)

	return d
}
