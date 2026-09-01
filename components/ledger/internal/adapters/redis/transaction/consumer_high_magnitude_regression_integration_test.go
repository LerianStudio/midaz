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
// HIGH-MAGNITUDE FAIL-CLOSED REGRESSION TESTS
//
// Finding G1: sub_decimal compared operands via tonumber (IEEE-754 double),
// which mis-orders the subtraction above ~15-17 significant digits and
// discarded the integer loop's final borrow, so a negative result could come
// back as a huge positive number. min_decimal inherited the bug. Consequence:
// the 0018 (insufficient funds) and 0167 (overdraft limit) gates, overdraft
// repayment, and OnHold arithmetic all failed OPEN above double precision.
//
// These tests exercise ProcessBalanceAtomicOperation end to end (real engine,
// not the pure arithmetic harness) at magnitudes/scales drawn from the audit
// repro table, proving the consumers are fail-closed post-fix.
// =============================================================================

// TestIntegration_HighMagnitude_InsufficientFunds_Returns0018 proves the 0018
// gate rejects a debit that exceeds Available only once precision goes past
// what an IEEE-754 double can represent exactly, and that rejection leaves
// the cached balance untouched.
func TestIntegration_HighMagnitude_InsufficientFunds_Returns0018(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	t.Run("decimal_scale_at_1e9_direct_debit", func(t *testing.T) {
		infra := setupRedisIntegrationInfra(t)
		ctx := context.Background()
		orgID := uuid.New()
		ledgerID := uuid.New()

		// Available and the debit differ by 1e-8, past the point where the
		// pre-fix double comparison in sub_decimal broke (repro table: 8
		// decimals at ~1e9).
		op := overdraftOp(orgID, ledgerID, "@0018-scale-1e9", "deposit", constant.DirectionCredit,
			decimal.RequireFromString("1000000000.00000001"), decimal.Zero, 1, nil,
			constant.DEBIT, decimal.RequireFromString("1000000000.00000002"))

		_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
			uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{op})

		require.Error(t, err, "a debit exceeding available by 1e-8 at 1e9 magnitude must be rejected")
		assert.Contains(t, err.Error(), constant.ErrInsufficientFunds.Error())

		cached := readCachedBalance(t, infra, op.InternalKey)
		assert.Equal(t, "1000000000.00000001", cached.Available, "rejected debit must not move available")
		assert.Equal(t, int64(1), cached.Version, "rejected debit must not consume a version")
	})

	t.Run("integer_19_digits_direct_debit", func(t *testing.T) {
		infra := setupRedisIntegrationInfra(t)
		ctx := context.Background()
		orgID := uuid.New()
		ledgerID := uuid.New()

		// 19-digit integers one unit apart: repro table entry where the
		// double comparison broke by exactly 1 unit.
		op := overdraftOp(orgID, ledgerID, "@0018-int19", "deposit", constant.DirectionCredit,
			decimal.RequireFromString("1000000000000000000"), decimal.Zero, 1, nil,
			constant.DEBIT, decimal.RequireFromString("1000000000000000001"))

		_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
			uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{op})

		require.Error(t, err, "a debit exceeding a 19-digit available by 1 must be rejected")
		assert.Contains(t, err.Error(), constant.ErrInsufficientFunds.Error())

		cached := readCachedBalance(t, infra, op.InternalKey)
		assert.Equal(t, "1000000000000000000", cached.Available, "rejected debit must not move available")
		assert.Equal(t, int64(1), cached.Version, "rejected debit must not consume a version")
	})

	t.Run("pending_hold_legacy_shape", func(t *testing.T) {
		infra := setupRedisIntegrationInfra(t)
		ctx := context.Background()
		orgID := uuid.New()
		ledgerID := uuid.New()
		alias := "@0018-pending-legacy"

		onHold := pendingOverdraftBalanceOp(orgID, ledgerID, alias, constant.DefaultBalanceKey, constant.DirectionCredit,
			decimal.RequireFromString("100000000000000.01"), decimal.Zero, decimal.Zero, 1, nil,
			constant.ONHOLD, constant.PENDING, decimal.RequireFromString("100000000000000.02"), decimal.Zero, false)

		_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
			uuid.New(), constant.PENDING, true, []mmodel.BalanceOperation{onHold})

		require.Error(t, err, "a hold exceeding available at high magnitude must be rejected")
		assert.Contains(t, err.Error(), constant.ErrInsufficientFunds.Error())

		cached := readCachedBalance(t, infra, utils.BalanceInternalKey(orgID, ledgerID, alias+"#"+constant.DefaultBalanceKey))
		assert.Equal(t, "100000000000000.01", cached.Available, "rejected hold must not move available")
		assert.Equal(t, "0", cached.OnHold, "rejected hold must not place a hold")
		assert.Equal(t, int64(1), cached.Version, "rejected hold must not consume a version")
	})

	t.Run("pending_hold_route_validated_shape", func(t *testing.T) {
		infra := setupRedisIntegrationInfra(t)
		ctx := context.Background()
		orgID := uuid.New()
		ledgerID := uuid.New()
		alias := "@0018-pending-rv"

		debit := pendingOverdraftBalanceOp(orgID, ledgerID, alias, constant.DefaultBalanceKey, constant.DirectionCredit,
			decimal.RequireFromString("100000000000000.01"), decimal.Zero, decimal.Zero, 1, nil,
			constant.DEBIT, constant.PENDING, decimal.RequireFromString("100000000000000.02"), decimal.Zero, true)
		onHold := pendingOverdraftBalanceOp(orgID, ledgerID, alias, constant.DefaultBalanceKey, constant.DirectionCredit,
			decimal.RequireFromString("100000000000000.01"), decimal.Zero, decimal.Zero, 1, nil,
			constant.ONHOLD, constant.PENDING, decimal.RequireFromString("100000000000000.02"), decimal.Zero, true)

		_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
			uuid.New(), constant.PENDING, true, []mmodel.BalanceOperation{debit, onHold})

		require.Error(t, err, "a hold exceeding available at high magnitude must be rejected")
		assert.Contains(t, err.Error(), constant.ErrInsufficientFunds.Error())

		cached := readCachedBalance(t, infra, utils.BalanceInternalKey(orgID, ledgerID, alias+"#"+constant.DefaultBalanceKey))
		assert.Equal(t, "100000000000000.01", cached.Available, "rejected hold must not move available")
		assert.Equal(t, "0", cached.OnHold, "rejected hold must not place a hold")
		assert.Equal(t, int64(1), cached.Version, "rejected hold must not consume a version")
	})
}

// TestIntegration_HighMagnitude_OverdraftLimit_Returns0167 proves the 0167
// gate rejects a projected OverdraftUsed that exceeds the configured limit
// only at high magnitude/scale, and that the at-limit boundary (deficit ==
// limit) is still inclusive and allowed.
func TestIntegration_HighMagnitude_OverdraftLimit_Returns0167(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	settings := func() *mmodel.BalanceSettings {
		return &mmodel.BalanceSettings{
			BalanceScope:          mmodel.BalanceScopeTransactional,
			AllowOverdraft:        true,
			OverdraftLimitEnabled: true,
			OverdraftLimit:        ptrString("100000000000000.01"),
		}
	}

	t.Run("exceeded_by_a_cent_at_1e14_rejected", func(t *testing.T) {
		infra := setupRedisIntegrationInfra(t)
		ctx := context.Background()
		orgID := uuid.New()
		ledgerID := uuid.New()

		op := overdraftOp(orgID, ledgerID, "@0167-rejected", "deposit", constant.DirectionCredit,
			decimal.Zero, decimal.Zero, 1, settings(),
			constant.DEBIT, decimal.RequireFromString("100000000000000.02"))

		_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
			uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{op})

		require.Error(t, err, "deficit=100000000000000.02 with limit=100000000000000.01 must be rejected")
		assert.Contains(t, err.Error(), constant.ErrOverdraftLimitExceeded.Error())

		cached := readCachedBalance(t, infra, op.InternalKey)
		assert.Equal(t, "0", cached.Available, "rollback must restore available")
		assert.Equal(t, "0", cached.OverdraftUsed, "rollback must restore overdraft used")
		assert.Equal(t, int64(1), cached.Version, "rejected overdraft must not consume a version")
	})

	t.Run("at_limit_at_1e14_allowed", func(t *testing.T) {
		infra := setupRedisIntegrationInfra(t)
		ctx := context.Background()
		orgID := uuid.New()
		ledgerID := uuid.New()

		op := overdraftOp(orgID, ledgerID, "@0167-boundary", "deposit", constant.DirectionCredit,
			decimal.Zero, decimal.Zero, 1, settings(),
			constant.DEBIT, decimal.RequireFromString("100000000000000.01"))

		result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
			uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{op})

		require.NoError(t, err, "deficit==limit is inclusive and must be allowed")
		require.Len(t, result.After, 1)
		assert.True(t, result.After[0].Available.IsZero(), "available should floor at zero")
		assert.True(t, result.After[0].OverdraftUsed.Equal(decimal.RequireFromString("100000000000000.01")),
			"overdraft used should equal the limit, got %s", result.After[0].OverdraftUsed)
	})

	t.Run("exceeded_at_19_digit_integers_rejected", func(t *testing.T) {
		infra := setupRedisIntegrationInfra(t)
		ctx := context.Background()
		orgID := uuid.New()
		ledgerID := uuid.New()

		intSettings := &mmodel.BalanceSettings{
			BalanceScope:          mmodel.BalanceScopeTransactional,
			AllowOverdraft:        true,
			OverdraftLimitEnabled: true,
			OverdraftLimit:        ptrString("1000000000000000000"),
		}

		op := overdraftOp(orgID, ledgerID, "@0167-int19", "deposit", constant.DirectionCredit,
			decimal.Zero, decimal.Zero, 1, intSettings,
			constant.DEBIT, decimal.RequireFromString("1000000000000000001"))

		_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
			uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{op})

		require.Error(t, err, "deficit exceeding a 19-digit limit by 1 must be rejected")
		assert.Contains(t, err.Error(), constant.ErrOverdraftLimitExceeded.Error())
	})
}

// TestIntegration_HighMagnitude_OverdraftRepayment_ExactSplit proves a credit
// repaying outstanding overdraft at high magnitude/scale splits exactly: the
// repayable portion decrements OverdraftUsed and only the remainder reaches
// Available.
func TestIntegration_HighMagnitude_OverdraftRepayment_ExactSplit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	overdraftUsed := decimal.RequireFromString("100000000000000.02")
	credit := decimal.RequireFromString("100000000000000.03")

	op := overdraftOp(orgID, ledgerID, "@repay-high-magnitude", "deposit", constant.DirectionCredit,
		decimal.Zero, overdraftUsed, 1, nil,
		constant.CREDIT, credit)

	result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{op})

	require.NoError(t, err)
	require.Len(t, result.After, 1)

	assert.True(t, result.After[0].OverdraftUsed.IsZero(),
		"the credit exceeds outstanding overdraft, so it must fully repay it; got %s",
		result.After[0].OverdraftUsed)
	assert.True(t, result.After[0].Available.Equal(decimal.RequireFromString("0.01")),
		"the remainder after repayment (100000000000000.03-100000000000000.02) must land in available; got %s",
		result.After[0].Available)

	// Conservation: the credit is fully accounted for between the repayment
	// (OverdraftUsed decrement) and the remainder (Available increment).
	overdraftDelta := overdraftUsed.Sub(result.After[0].OverdraftUsed)
	availableDelta := result.After[0].Available.Sub(decimal.Zero)
	assert.True(t, overdraftDelta.Add(availableDelta).Equal(credit),
		"overdraft repaid (%s) + available credited (%s) must equal the credit (%s)",
		overdraftDelta, availableDelta, credit)
}

// TestIntegration_HighMagnitude_OnHoldLifecycle_CommitConservesAmount proves
// a route-validated commit at high magnitude/scale moves the held amount
// off the source's OnHold and onto the destination's Available with no
// leakage: the two legs' deltas sum to zero.
func TestIntegration_HighMagnitude_OnHoldLifecycle_CommitConservesAmount(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	amount := decimal.RequireFromString("1000000000.00000001")
	sourceAvailable := decimal.RequireFromString("500")
	destAvailableBefore := decimal.RequireFromString("1000000000.00000002")

	sourceOp := pendingOverdraftBalanceOp(orgID, ledgerID, "@commit-hold-src", constant.DefaultBalanceKey, constant.DirectionCredit,
		sourceAvailable, amount, decimal.Zero, 1, nil,
		constant.ONHOLD, constant.APPROVED, amount, decimal.Zero, true)

	destOp := pendingOverdraftBalanceOp(orgID, ledgerID, "@commit-hold-dst", constant.DefaultBalanceKey, constant.DirectionCredit,
		destAvailableBefore, decimal.Zero, decimal.Zero, 1, nil,
		constant.CREDIT, constant.APPROVED, amount, decimal.Zero, true)

	result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, true, []mmodel.BalanceOperation{sourceOp, destOp})
	require.NoError(t, err)

	sourceAfter := findBalanceByAliasKey(t, result.After, "@commit-hold-src#"+constant.DefaultBalanceKey)
	destAfter := findBalanceByAliasKey(t, result.After, "@commit-hold-dst#"+constant.DefaultBalanceKey)

	assert.True(t, sourceAfter.OnHold.IsZero(), "the commit must fully drop the hold; got %s", sourceAfter.OnHold)
	assert.True(t, sourceAfter.Available.Equal(sourceAvailable),
		"a commit consumes the hold, not available; got %s", sourceAfter.Available)
	assert.True(t, destAfter.Available.Equal(destAvailableBefore.Add(amount)),
		"destination available must receive exactly the committed amount; got %s", destAfter.Available)

	sourceOnHoldDelta := sourceAfter.OnHold.Sub(amount)
	destAvailableDelta := destAfter.Available.Sub(destAvailableBefore)
	assert.True(t, sourceOnHoldDelta.Add(destAvailableDelta).IsZero(),
		"conservation: source OnHold delta (%s) + destination Available delta (%s) must sum to zero",
		sourceOnHoldDelta, destAvailableDelta)
}

// TestIntegration_HighMagnitude_OnHoldLifecycle_CancelConservesAmount proves
// a legacy-shape cancel at high magnitude/scale releases the hold back into
// Available with no leakage on the same balance: the Available and OnHold
// deltas sum to zero.
func TestIntegration_HighMagnitude_OnHoldLifecycle_CancelConservesAmount(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	availableBefore := decimal.RequireFromString("300000000000000.02")
	onHoldBefore := decimal.RequireFromString("100000000000000.01")
	releaseAmount := onHoldBefore

	release := pendingOverdraftBalanceOp(orgID, ledgerID, "@cancel-hold", constant.DefaultBalanceKey, constant.DirectionCredit,
		availableBefore, onHoldBefore, decimal.Zero, 1, nil,
		constant.RELEASE, constant.CANCELED, releaseAmount, decimal.Zero, false)

	result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.CANCELED, true, []mmodel.BalanceOperation{release})
	require.NoError(t, err)
	require.Len(t, result.After, 1)

	after := result.After[0]
	assert.True(t, after.OnHold.IsZero(), "the cancel must fully release the hold; got %s", after.OnHold)
	assert.True(t, after.Available.Equal(availableBefore.Add(releaseAmount)),
		"the cancel must restore the full released amount to available; got %s", after.Available)

	availableDelta := after.Available.Sub(availableBefore)
	onHoldDelta := after.OnHold.Sub(onHoldBefore)
	assert.True(t, availableDelta.Add(onHoldDelta).IsZero(),
		"conservation: available delta (%s) + onHold delta (%s) must sum to zero", availableDelta, onHoldDelta)
}
