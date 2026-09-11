//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// =============================================================================
// ON-HOLD FLOOR (0174) INTEGRATION TESTS
// =============================================================================
// OnHold is held capacity: it can only be released by the transition that placed
// it. A subtraction that would take it below zero therefore means the batch is
// releasing a hold that is no longer there — the double-apply that left
// on_hold = -100 in the incident. The script refuses it with 0174 and rolls the
// whole batch back.
//
// All four subtraction sites are covered: RELEASE in both shapes, the
// route-validated ON_HOLD on APPROVED, and the APPROVED DEBIT.
//
// Scope is the adapter layer only — a real Valkey from testcontainers, no
// application environment.

// onHoldFloorOp builds a transition leg against a balance carrying a hold.
func onHoldFloorOp(
	orgID, ledgerID uuid.UUID,
	alias string,
	available, onHold decimal.Decimal,
	version int64,
	operation, transactionStatus string,
	amount decimal.Decimal,
	routeValidationEnabled bool,
) mmodel.BalanceOperation {
	return pendingOverdraftBalanceOp(orgID, ledgerID, alias, constant.DefaultBalanceKey, constant.DirectionCredit,
		available, onHold, decimal.Zero, version, nil,
		operation, transactionStatus, amount, decimal.Zero, routeValidationEnabled)
}

// seedHold runs a real pending create so the cached balance carries a genuine
// hold, and returns the balance key plus the state the hold left behind.
func seedHold(
	t *testing.T, infra *integrationTestInfra, ctx context.Context,
	orgID, ledgerID uuid.UUID, alias string,
	available, holdAmount decimal.Decimal,
) (string, crossGateBalanceSnapshot) {
	t.Helper()

	holdOp := onHoldFloorOp(orgID, ledgerID, alias, available, decimal.Zero, 1,
		constant.ONHOLD, constant.PENDING, holdAmount, false)

	_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.PENDING, true, []mmodel.BalanceOperation{holdOp}, nil)
	require.NoError(t, err, "the pending create that seeds the hold must succeed")

	key := utils.BalanceInternalKey(orgID, ledgerID, alias+"#"+constant.DefaultBalanceKey)

	return key, snapshotOf(t, infra, key)
}

// TestIntegration_OnHoldFloor_ReleaseBeyondHoldIsRejected covers cases (a) and
// (b): every site that subtracts from OnHold refuses to breach zero and rolls the
// batch back byte for byte.
//
// The corrupted input is the real one from the incident: a transition carrying an
// amount larger than the hold the balance actually holds, which is what a
// re-executed transition looks like once the first one already released it.
func TestIntegration_OnHoldFloor_ReleaseBeyondHoldIsRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	testCases := []struct {
		name                   string
		alias                  string
		operation              string
		transactionStatus      string
		routeValidationEnabled bool
	}{
		{
			name:              "release on cancel, legacy shape",
			alias:             "@onhold-floor-release-legacy",
			operation:         constant.RELEASE,
			transactionStatus: constant.CANCELED,
		},
		{
			name:                   "release on cancel, route-validated shape",
			alias:                  "@onhold-floor-release-rv",
			operation:              constant.RELEASE,
			transactionStatus:      constant.CANCELED,
			routeValidationEnabled: true,
		},
		{
			name:                   "on-hold on commit, route-validated shape",
			alias:                  "@onhold-floor-onhold-approved-rv",
			operation:              constant.ONHOLD,
			transactionStatus:      constant.APPROVED,
			routeValidationEnabled: true,
		},
		{
			name:              "debit on commit",
			alias:             "@onhold-floor-debit-approved",
			operation:         constant.DEBIT,
			transactionStatus: constant.APPROVED,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			infra := setupRedisIntegrationInfra(t)
			ctx := context.Background()
			orgID := uuid.New()
			ledgerID := uuid.New()

			// 50 held against 500 available.
			key, afterHold := seedHold(t, infra, ctx, orgID, ledgerID, tc.alias,
				decimal.NewFromInt(500), decimal.NewFromInt(50))
			require.Equal(t, "50", afterHold.onHold)

			// The transition claims to release 100 — twice what is held.
			breach := onHoldFloorOp(orgID, ledgerID, tc.alias,
				decimal.NewFromInt(450), decimal.NewFromInt(50), afterHold.version,
				tc.operation, tc.transactionStatus, decimal.NewFromInt(100), tc.routeValidationEnabled)

			_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
				uuid.New(), tc.transactionStatus, true, []mmodel.BalanceOperation{breach}, nil)

			require.Error(t, err, "a subtraction that would take OnHold below zero must be refused")
			assert.Contains(t, err.Error(), constant.ErrStaleBalanceVersion.Error())

			assert.Equal(t, afterHold, snapshotOf(t, infra, key),
				"the rollback must restore Available, OnHold and Version exactly")
			assert.Equal(t, "0", readCachedBalance(t, infra, key).OverdraftUsed,
				"a rejected batch must not accrue overdraft")
		})
	}
}

// TestIntegration_OnHoldFloor_ExactZeroIsAllowed covers case (c): the floor is
// `< 0`, not `<= 0`. Releasing exactly what is held is the normal cancel, and it
// must land.
func TestIntegration_OnHoldFloor_ExactZeroIsAllowed(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	alias := "@onhold-floor-exact-zero"

	key, afterHold := seedHold(t, infra, ctx, orgID, ledgerID, alias,
		decimal.NewFromInt(500), decimal.NewFromInt(100))
	require.Equal(t, "100", afterHold.onHold)
	require.Equal(t, "400", afterHold.available)

	release := onHoldFloorOp(orgID, ledgerID, alias,
		decimal.NewFromInt(400), decimal.NewFromInt(100), afterHold.version,
		constant.RELEASE, constant.CANCELED, decimal.NewFromInt(100), false)

	_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.CANCELED, true, []mmodel.BalanceOperation{release}, nil)
	require.NoError(t, err, "releasing exactly what is held lands on the floor, it does not breach it")

	state := snapshotOf(t, infra, key)
	assert.Equal(t, "0", state.onHold, "a full release leaves OnHold at zero")
	assert.Equal(t, "500", state.available, "a full release restores Available")
	assert.Equal(t, afterHold.version+1, state.version)
}

// TestIntegration_OnHoldFloor_RollbackUnwindsTheWholeBatch covers case (d): the
// breach is detected on the SECOND group, after the first one already wrote. The
// rollback has to unwind both, versions included, or a rejected batch would leave
// half a transaction applied.
func TestIntegration_OnHoldFloor_RollbackUnwindsTheWholeBatch(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	destinationAlias := "@onhold-floor-batch-destination"
	sourceAlias := "@onhold-floor-batch-source"

	// The destination is materialized by a posting of its own, so its pre-batch
	// state is a real cached value rather than a first-touch seed.
	destinationSeed := onHoldFloorOp(orgID, ledgerID, destinationAlias,
		decimal.NewFromInt(100), decimal.Zero, 1,
		constant.CREDIT, constant.APPROVED, decimal.NewFromInt(100), false)

	_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{destinationSeed}, nil)
	require.NoError(t, err)

	destinationKey := utils.BalanceInternalKey(orgID, ledgerID, destinationAlias+"#"+constant.DefaultBalanceKey)
	destinationBefore := snapshotOf(t, infra, destinationKey)
	require.Equal(t, "200", destinationBefore.available)

	sourceKey, sourceBefore := seedHold(t, infra, ctx, orgID, ledgerID, sourceAlias,
		decimal.NewFromInt(500), decimal.NewFromInt(50))

	// Group 1 credits the destination and WILL be written; group 2 breaches the
	// floor on the source and aborts the batch.
	destinationLeg := onHoldFloorOp(orgID, ledgerID, destinationAlias,
		decimal.NewFromInt(200), decimal.Zero, destinationBefore.version,
		constant.CREDIT, constant.APPROVED, decimal.NewFromInt(100), false)
	sourceLeg := onHoldFloorOp(orgID, ledgerID, sourceAlias,
		decimal.NewFromInt(450), decimal.NewFromInt(50), sourceBefore.version,
		constant.DEBIT, constant.APPROVED, decimal.NewFromInt(100), false)

	_, err = infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, true, []mmodel.BalanceOperation{destinationLeg, sourceLeg}, nil)

	require.Error(t, err, "the batch must abort on the second group's floor breach")
	assert.Contains(t, err.Error(), constant.ErrStaleBalanceVersion.Error())

	assert.Equal(t, destinationBefore, snapshotOf(t, infra, destinationKey),
		"the group that already applied must be rolled back, version included")
	assert.Equal(t, sourceBefore, snapshotOf(t, infra, sourceKey),
		"the group that breached the floor must not have moved")

	// Control: the destination leg is a real mutation, so the assertion above is
	// proving a rollback rather than a leg that never moved anything.
	_, err = infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, true, []mmodel.BalanceOperation{destinationLeg}, nil)
	require.NoError(t, err)

	destinationAlone := snapshotOf(t, infra, destinationKey)
	assert.Equal(t, "300", destinationAlone.available,
		"the leg rolled back above does move the balance when its batch succeeds")
	assert.Equal(t, destinationBefore.version+1, destinationAlone.version)
}

// TestIntegration_OnHoldFloor_HappyPendingLifecycleIsUnchanged covers case (e):
// the floor must be invisible to every legitimate flow. A pending create followed
// by its commit, and a pending create followed by its cancel, both run end to end
// with the same before/after/version arithmetic they had before the guard existed.
func TestIntegration_OnHoldFloor_HappyPendingLifecycleIsUnchanged(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	t.Run("commit consumes the hold", func(t *testing.T) {
		infra := setupRedisIntegrationInfra(t)
		ctx := context.Background()
		orgID := uuid.New()
		ledgerID := uuid.New()
		alias := "@onhold-floor-happy-commit"

		key, afterHold := seedHold(t, infra, ctx, orgID, ledgerID, alias,
			decimal.NewFromInt(500), decimal.NewFromInt(200))
		require.Equal(t, "300", afterHold.available)
		require.Equal(t, "200", afterHold.onHold)
		require.Equal(t, int64(2), afterHold.version)

		commit := onHoldFloorOp(orgID, ledgerID, alias,
			decimal.NewFromInt(300), decimal.NewFromInt(200), afterHold.version,
			constant.DEBIT, constant.APPROVED, decimal.NewFromInt(200), false)

		result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
			uuid.New(), constant.APPROVED, true, []mmodel.BalanceOperation{commit}, nil)
		require.NoError(t, err)
		require.Len(t, result.Before, 1)
		require.Len(t, result.After, 1)

		assert.True(t, result.Before[0].OnHold.Equal(decimal.NewFromInt(200)))
		assert.True(t, result.After[0].OnHold.IsZero())
		assert.True(t, result.After[0].Available.Equal(decimal.NewFromInt(300)),
			"a commit consumes the hold; Available stays where the create left it")
		assert.Equal(t, afterHold.version+1, result.After[0].Version)

		state := snapshotOf(t, infra, key)
		assert.Equal(t, "300", state.available)
		assert.Equal(t, "0", state.onHold)
		assert.Equal(t, int64(3), state.version)
	})

	t.Run("cancel returns the hold", func(t *testing.T) {
		infra := setupRedisIntegrationInfra(t)
		ctx := context.Background()
		orgID := uuid.New()
		ledgerID := uuid.New()
		alias := "@onhold-floor-happy-cancel"

		key, afterHold := seedHold(t, infra, ctx, orgID, ledgerID, alias,
			decimal.NewFromInt(500), decimal.NewFromInt(200))
		require.Equal(t, "300", afterHold.available)
		require.Equal(t, "200", afterHold.onHold)

		cancel := onHoldFloorOp(orgID, ledgerID, alias,
			decimal.NewFromInt(300), decimal.NewFromInt(200), afterHold.version,
			constant.RELEASE, constant.CANCELED, decimal.NewFromInt(200), false)

		result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
			uuid.New(), constant.CANCELED, true, []mmodel.BalanceOperation{cancel}, nil)
		require.NoError(t, err)
		require.Len(t, result.After, 1)

		assert.True(t, result.After[0].OnHold.IsZero())
		assert.True(t, result.After[0].Available.Equal(decimal.NewFromInt(500)),
			"a cancel returns the held funds in full")
		assert.Equal(t, afterHold.version+1, result.After[0].Version)

		state := snapshotOf(t, infra, key)
		assert.Equal(t, "500", state.available)
		assert.Equal(t, "0", state.onHold)
		assert.Equal(t, int64(3), state.version)
	})
}
