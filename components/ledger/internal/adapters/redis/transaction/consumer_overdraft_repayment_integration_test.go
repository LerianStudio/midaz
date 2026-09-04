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

	libConstants "github.com/LerianStudio/lib-commons/v7/commons/constants"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// findBalanceByAliasKey resolves a balance by its "<alias>#<key>" identity,
// which is what disambiguates the legs when a batch carries several balances
// sharing an alias or a balance key.
func findBalanceByAliasKey(t *testing.T, balances []*mmodel.Balance, aliasKey string) *mmodel.Balance {
	t.Helper()

	for _, balance := range balances {
		if balance != nil && balance.Alias == aliasKey {
			return balance
		}
	}

	require.Failf(t, "balance not found", "alias key %q not found", aliasKey)

	return nil
}

// findLatestBalanceByAliasKey returns the highest-version snapshot for an alias
// key, which is what a batch mutating the same balance twice requires (the
// route-validated cancel splits the source restore into RELEASE + CREDIT).
func findLatestBalanceByAliasKey(t *testing.T, balances []*mmodel.Balance, aliasKey string) *mmodel.Balance {
	t.Helper()

	var latest *mmodel.Balance

	for _, balance := range balances {
		if balance == nil || balance.Alias != aliasKey {
			continue
		}

		if latest == nil || balance.Version > latest.Version {
			latest = balance
		}
	}

	require.NotNilf(t, latest, "alias key %q not found", aliasKey)

	return latest
}

// TestIntegration_Overdraft_PendingDestinationCreditDefersRepayment locks the
// deferred-leg contract: a PENDING create carries the destination CREDIT into
// the atomic batch, but that credit only posts at commit. When the destination
// already carries outstanding overdraft the script must leave it completely
// alone — repaying against an Available the credit never reached drives the
// result negative, and the floor block then re-accrues the deficit on top of
// the outstanding OverdraftUsed, doubling it.
func TestIntegration_Overdraft_PendingDestinationCreditDefersRepayment(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	sourceAliasKey := "@pending-defer-src" + "#" + constant.DefaultBalanceKey
	destAliasKey := "@pending-defer-dst" + "#" + constant.DefaultBalanceKey

	destSettings := &mmodel.BalanceSettings{
		BalanceScope:          mmodel.BalanceScopeTransactional,
		AllowOverdraft:        true,
		OverdraftLimitEnabled: true,
		OverdraftLimit:        ptrString("100"),
	}

	sourceOp := pendingOverdraftBalanceOp(orgID, ledgerID, "@pending-defer-src", constant.DefaultBalanceKey, constant.DirectionCredit,
		decimal.NewFromInt(1000), decimal.Zero, decimal.Zero, 1, nil,
		constant.ONHOLD, constant.PENDING, decimal.NewFromInt(60), decimal.Zero, false)

	destOp := pendingOverdraftBalanceOp(orgID, ledgerID, "@pending-defer-dst", constant.DefaultBalanceKey, constant.DirectionCredit,
		decimal.Zero, decimal.Zero, decimal.NewFromInt(50), 1, destSettings,
		libConstants.CREDIT, constant.PENDING, decimal.NewFromInt(60), decimal.Zero, false)

	result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.PENDING, true, []mmodel.BalanceOperation{sourceOp, destOp})
	require.NoError(t, err)

	sourceAfter := findBalanceByAliasKey(t, result.After, sourceAliasKey)
	assert.True(t, sourceAfter.Available.Equal(decimal.NewFromInt(940)), "got %s", sourceAfter.Available.String())
	assert.True(t, sourceAfter.OnHold.Equal(decimal.NewFromInt(60)), "got %s", sourceAfter.OnHold.String())

	for _, balance := range result.After {
		if balance != nil {
			assert.NotEqual(t, destAliasKey, balance.Alias,
				"the deferred destination credit changed nothing, so it must not report a mutation")
		}
	}

	// An unchanged balance is deliberately absent from the atomic result, so the
	// cache entry is the only place its final values can be read back.
	destCached := readCachedBalance(t, infra, utils.BalanceInternalKey(orgID, ledgerID, destAliasKey))

	assert.Equal(t, "0", destCached.Available,
		"a pending create must not move the destination available")
	assert.Equal(t, "50", destCached.OverdraftUsed,
		"a pending create must neither repay nor re-accrue the destination overdraft")
	assert.Equal(t, int64(1), destCached.Version, "an untouched balance must not consume a version")
}

// TestIntegration_Overdraft_CommitDestinationCreditRepaysOnce locks the commit
// half of the same lifecycle: the destination credit posts on the APPROVED
// transition, so that is where the overdraft repayment happens. The enrichment
// layer queues a sibling CREDIT on the direction=debit companion, and the two
// legs must settle exactly one repayment between them — the primary keeps the
// remainder, the companion sheds the repaid liability.
func TestIntegration_Overdraft_CommitDestinationCreditRepaysOnce(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	sourceAliasKey := "@commit-repay-src" + "#" + constant.DefaultBalanceKey
	destAliasKey := "@commit-repay-dst" + "#" + constant.DefaultBalanceKey
	companionAliasKey := "@commit-repay-dst" + "#" + constant.OverdraftBalanceKey

	destSettings := &mmodel.BalanceSettings{
		BalanceScope:          mmodel.BalanceScopeTransactional,
		AllowOverdraft:        true,
		OverdraftLimitEnabled: true,
		OverdraftLimit:        ptrString("100"),
	}

	// Commit consumes the hold the pending create reserved: the source leg is an
	// ON_HOLD under route validation and never touches Available.
	sourceOp := pendingOverdraftBalanceOp(orgID, ledgerID, "@commit-repay-src", constant.DefaultBalanceKey, constant.DirectionCredit,
		decimal.NewFromInt(940), decimal.NewFromInt(60), decimal.Zero, 1, nil,
		constant.ONHOLD, constant.APPROVED, decimal.NewFromInt(60), decimal.Zero, true)

	destOp := pendingOverdraftBalanceOp(orgID, ledgerID, "@commit-repay-dst", constant.DefaultBalanceKey, constant.DirectionCredit,
		decimal.Zero, decimal.Zero, decimal.NewFromInt(50), 1, destSettings,
		libConstants.CREDIT, constant.APPROVED, decimal.NewFromInt(60), decimal.Zero, false)

	companionOp := pendingOverdraftBalanceOp(orgID, ledgerID, "@commit-repay-dst", constant.OverdraftBalanceKey, constant.DirectionDebit,
		decimal.NewFromInt(50), decimal.Zero, decimal.Zero, 1, nil,
		libConstants.CREDIT, constant.APPROVED, decimal.NewFromInt(50), decimal.Zero, false)

	result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, true, []mmodel.BalanceOperation{sourceOp, destOp, companionOp})
	require.NoError(t, err)

	sourceAfter := findBalanceByAliasKey(t, result.After, sourceAliasKey)
	assert.True(t, sourceAfter.Available.Equal(decimal.NewFromInt(940)),
		"a commit consumes the hold, not available; got %s", sourceAfter.Available.String())
	assert.True(t, sourceAfter.OnHold.IsZero(), "got %s", sourceAfter.OnHold.String())

	destAfter := findBalanceByAliasKey(t, result.After, destAliasKey)
	assert.True(t, destAfter.Available.Equal(decimal.NewFromInt(10)),
		"available must receive credit minus repayment; got %s", destAfter.Available.String())
	assert.True(t, destAfter.OverdraftUsed.IsZero(),
		"the credit must repay the outstanding overdraft exactly once; got %s", destAfter.OverdraftUsed.String())

	companionAfter := findBalanceByAliasKey(t, result.After, companionAliasKey)
	assert.True(t, companionAfter.Available.IsZero(),
		"the companion must shed the repaid liability in lock-step; got %s", companionAfter.Available.String())
}

// TestIntegration_Overdraft_CancelDefersDestinationCredit locks the third batch
// of the two-phase lifecycle. A cancel batch still carries the destination
// CREDIT (only the operation-record slice drops the destination legs, not the
// validate maps that drive the balance operations), and that credit is as
// deferred on a cancel as it is on the create — the destination never received
// anything, so a cancel must leave it completely alone.
//
// The destination credit is the one leg route validation never marks
// ("destinations always use plain CREDIT"), which is exactly what separates it
// from the source restore: the source comes through as a RELEASE in the legacy
// shape and as a route-validated CREDIT in the other, and both must keep
// repaying. Both shapes are covered here.
func TestIntegration_Overdraft_CancelDefersDestinationCredit(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	destSettings := func() *mmodel.BalanceSettings {
		return &mmodel.BalanceSettings{
			BalanceScope:          mmodel.BalanceScopeTransactional,
			AllowOverdraft:        true,
			OverdraftLimitEnabled: true,
			OverdraftLimit:        ptrString("100"),
		}
	}

	// assertDestinationUntouched is the shared verdict: the deferred leg reports
	// no mutation and its cached state is byte-for-byte what it was.
	assertDestinationUntouched := func(ctx context.Context, t *testing.T, infra *integrationTestInfra,
		result *mmodel.BalanceAtomicResult, orgID, ledgerID uuid.UUID, destAliasKey string,
	) {
		t.Helper()

		for _, balance := range result.After {
			if balance != nil {
				assert.NotEqual(t, destAliasKey, balance.Alias,
					"the deferred destination credit changed nothing, so it must not report a mutation")
			}
		}

		cached := readCachedBalance(t, infra, utils.BalanceInternalKey(orgID, ledgerID, destAliasKey))

		assert.Equal(t, "0", cached.Available,
			"a cancel must not move the destination available")
		assert.Equal(t, "50", cached.OverdraftUsed,
			"a cancel must neither repay nor re-accrue the destination overdraft")
		assert.Equal(t, int64(1), cached.Version, "an untouched balance must not consume a version")
	}

	t.Run("route_validated_shape", func(t *testing.T) {
		infra := setupRedisIntegrationInfra(t)
		ctx := context.Background()
		orgID := uuid.New()
		ledgerID := uuid.New()
		payerAliasKey := "@cancel-defer-rv-payer" + "#" + constant.DefaultBalanceKey
		destAliasKey := "@cancel-defer-rv-dst" + "#" + constant.DefaultBalanceKey

		// Route-validated cancel splits the source restore in two: RELEASE drops
		// the hold, a sibling CREDIT gives Available back.
		release := pendingOverdraftBalanceOp(orgID, ledgerID, "@cancel-defer-rv-payer", constant.DefaultBalanceKey, constant.DirectionCredit,
			decimal.NewFromInt(940), decimal.NewFromInt(60), decimal.Zero, 1, nil,
			constant.RELEASE, constant.CANCELED, decimal.NewFromInt(60), decimal.Zero, true)

		restore := pendingOverdraftBalanceOp(orgID, ledgerID, "@cancel-defer-rv-payer", constant.DefaultBalanceKey, constant.DirectionCredit,
			decimal.NewFromInt(940), decimal.NewFromInt(60), decimal.Zero, 1, nil,
			libConstants.CREDIT, constant.CANCELED, decimal.NewFromInt(60), decimal.Zero, true)

		// The destination carries outstanding overdraft and never received the
		// pending credit. Route validation does not mark destination legs.
		destOp := pendingOverdraftBalanceOp(orgID, ledgerID, "@cancel-defer-rv-dst", constant.DefaultBalanceKey, constant.DirectionCredit,
			decimal.Zero, decimal.Zero, decimal.NewFromInt(50), 1, destSettings(),
			libConstants.CREDIT, constant.CANCELED, decimal.NewFromInt(60), decimal.Zero, false)

		result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
			uuid.New(), constant.CANCELED, true, []mmodel.BalanceOperation{release, restore, destOp})
		require.NoError(t, err)

		payerAfter := findLatestBalanceByAliasKey(t, result.After, payerAliasKey)
		assert.True(t, payerAfter.Available.Equal(decimal.NewFromInt(1000)),
			"the cancel must restore the payer in full; got %s", payerAfter.Available.String())
		assert.True(t, payerAfter.OnHold.IsZero(), "got %s", payerAfter.OnHold.String())

		assertDestinationUntouched(ctx, t, infra, result, orgID, ledgerID, destAliasKey)
	})

	t.Run("legacy_shape", func(t *testing.T) {
		infra := setupRedisIntegrationInfra(t)
		ctx := context.Background()
		orgID := uuid.New()
		ledgerID := uuid.New()
		payerAliasKey := "@cancel-defer-lg-payer" + "#" + constant.DefaultBalanceKey
		destAliasKey := "@cancel-defer-lg-dst" + "#" + constant.DefaultBalanceKey

		// Without route validation the RELEASE both drops the hold and restores
		// Available in one operation.
		release := pendingOverdraftBalanceOp(orgID, ledgerID, "@cancel-defer-lg-payer", constant.DefaultBalanceKey, constant.DirectionCredit,
			decimal.NewFromInt(940), decimal.NewFromInt(60), decimal.Zero, 1, nil,
			constant.RELEASE, constant.CANCELED, decimal.NewFromInt(60), decimal.Zero, false)

		destOp := pendingOverdraftBalanceOp(orgID, ledgerID, "@cancel-defer-lg-dst", constant.DefaultBalanceKey, constant.DirectionCredit,
			decimal.Zero, decimal.Zero, decimal.NewFromInt(50), 1, destSettings(),
			libConstants.CREDIT, constant.CANCELED, decimal.NewFromInt(60), decimal.Zero, false)

		result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
			uuid.New(), constant.CANCELED, true, []mmodel.BalanceOperation{release, destOp})
		require.NoError(t, err)

		payerAfter := findLatestBalanceByAliasKey(t, result.After, payerAliasKey)
		assert.True(t, payerAfter.Available.Equal(decimal.NewFromInt(1000)),
			"the cancel must restore the payer in full; got %s", payerAfter.Available.String())
		assert.True(t, payerAfter.OnHold.IsZero(), "got %s", payerAfter.OnHold.String())

		assertDestinationUntouched(ctx, t, infra, result, orgID, ledgerID, destAliasKey)
	})
}
