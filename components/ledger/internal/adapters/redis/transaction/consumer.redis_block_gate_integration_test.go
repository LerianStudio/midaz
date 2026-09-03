//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"errors"
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
// BLOCK GATE — END-TO-END THROUGH ProcessBalanceAtomicOperation (INTEGRATION)
// =============================================================================
// The sibling file drives the script directly to pin the raw verdicts. These
// drive the public repository method, which is where the verdicts become a Go
// error and where an unhydrated index is repaired from the source of truth and
// retried exactly once.

// TestIntegration_BlockGateRepo_DenialSurfacesTheAccount proves a denial leaves
// the method as a typed error naming the account, so the command layer can map
// it to 0502 without parsing strings.
func TestIntegration_BlockGateRepo_DenialSurfacesTheAccount(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID, ledgerID := uuid.New(), uuid.New()
	accountID := uuid.New()

	hydrateBlockedSet(t, infra, ctx, orgID, ledgerID, accountID)

	ops := []mmodel.BalanceOperation{
		gateOp(orgID, ledgerID, accountID, "@repo-denied", false,
			decimal.NewFromInt(500), decimal.Zero, constant.DEBIT, decimal.NewFromInt(100)),
	}

	_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, ops)

	var blockedErr AccountBlockedError
	require.ErrorAs(t, err, &blockedErr)
	assert.Equal(t, accountID.String(), blockedErr.AccountID)

	requireNoBalanceKeys(t, infra, ctx, ops)
}

// TestIntegration_BlockGateRepo_RepairsAnUnhydratedIndexAndRetries is the
// cold-start path: nothing has ever written this ledger's index, so the first
// run cannot answer. The repository rebuilds from the source of truth and runs
// the script again — once.
func TestIntegration_BlockGateRepo_RepairsAnUnhydratedIndexAndRetries(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID, ledgerID := uuid.New(), uuid.New()

	source := &stubBlockedAccountsSource{}
	infra.repo.blockedAccountsSource = source

	ops := []mmodel.BalanceOperation{
		gateOp(orgID, ledgerID, uuid.New(), "@repo-cold", false,
			decimal.NewFromInt(500), decimal.Zero, constant.DEBIT, decimal.NewFromInt(100)),
	}

	result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, ops)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, 1, source.calls, "the index is repaired once, not on every run")
	assert.Equal(t, "400", readCachedBalance(t, infra, ops[0].InternalKey).Available,
		"the retry after the repair must apply the operation")

	// The repair left the index usable, so the ledger's next transaction pays
	// nothing for it.
	hydrated, _, err := infra.repo.IsHydratedAndBlocked(ctx, orgID, ledgerID, nil)
	require.NoError(t, err)
	assert.True(t, hydrated)
}

// TestIntegration_BlockGateRepo_RepairThenDeny covers the same cold start when
// the source of truth DOES carry a blocked account: the rebuilt index must deny
// on the retry rather than let the transaction through.
func TestIntegration_BlockGateRepo_RepairThenDeny(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID, ledgerID := uuid.New(), uuid.New()
	accountID := uuid.New()

	infra.repo.blockedAccountsSource = &stubBlockedAccountsSource{accountIDs: []uuid.UUID{accountID}}

	ops := []mmodel.BalanceOperation{
		gateOp(orgID, ledgerID, accountID, "@repo-repair-deny", false,
			decimal.NewFromInt(500), decimal.Zero, constant.DEBIT, decimal.NewFromInt(100)),
	}

	_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, ops)

	var blockedErr AccountBlockedError
	require.ErrorAs(t, err, &blockedErr,
		"a block recorded only in Postgres must still deny once the index is rebuilt")
	assert.Equal(t, accountID.String(), blockedErr.AccountID)

	requireNoBalanceKeys(t, infra, ctx, ops)
}

// TestIntegration_BlockGateRepo_SourceFailureIsInfrastructureNotDenial is the
// contract that keeps an outage from being reported as a business rejection: a
// block that cannot be CONFIRMED is unavailability, so it must never surface as
// the 0502-shaped denial, and it must never let the transaction through.
func TestIntegration_BlockGateRepo_SourceFailureIsInfrastructureNotDenial(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID, ledgerID := uuid.New(), uuid.New()

	infra.repo.blockedAccountsSource = &stubBlockedAccountsSource{err: errors.New("postgres is down")}

	ops := []mmodel.BalanceOperation{
		gateOp(orgID, ledgerID, uuid.New(), "@repo-source-down", false,
			decimal.NewFromInt(500), decimal.Zero, constant.DEBIT, decimal.NewFromInt(100)),
	}

	_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, ops)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrBlockedAccountsIndexUnavailable)

	var blockedErr AccountBlockedError
	assert.False(t, errors.As(err, &blockedErr), "an outage is not a denial")

	requireNoBalanceKeys(t, infra, ctx, ops)
}

// TestIntegration_BlockGateRepo_NilSourceFailsClosed pins the misconfiguration
// case end to end: a repository wired without a source cannot repair the index
// and therefore cannot let anything through.
func TestIntegration_BlockGateRepo_NilSourceFailsClosed(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID, ledgerID := uuid.New(), uuid.New()

	infra.repo.blockedAccountsSource = nil

	ops := []mmodel.BalanceOperation{
		gateOp(orgID, ledgerID, uuid.New(), "@repo-no-source", false,
			decimal.NewFromInt(500), decimal.Zero, constant.DEBIT, decimal.NewFromInt(100)),
	}

	_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, ops)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrBlockedAccountsIndexUnavailable)

	requireNoBalanceKeys(t, infra, ctx, ops)
}

// TestIntegration_BlockGateRepo_HydratedIndexCostsNoRepair is the common path:
// once the index carries its sentinel, the source of truth is never consulted
// again, so a normal transaction pays exactly one in-script SMISMEMBER.
func TestIntegration_BlockGateRepo_HydratedIndexCostsNoRepair(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID, ledgerID := uuid.New(), uuid.New()

	source := &stubBlockedAccountsSource{}
	infra.repo.blockedAccountsSource = source

	require.NoError(t, infra.repo.HydrateBlockedAccounts(ctx, orgID, ledgerID, nil))

	ops := []mmodel.BalanceOperation{
		gateOp(orgID, ledgerID, uuid.New(), "@repo-warm", false,
			decimal.NewFromInt(500), decimal.Zero, constant.DEBIT, decimal.NewFromInt(100)),
	}

	_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, ops)
	require.NoError(t, err)

	assert.Equal(t, 0, source.calls,
		"a hydrated index must not reach back to the source of truth")
	assert.Equal(t, "400", readCachedBalance(t, infra, ops[0].InternalKey).Available)
}

// TestIntegration_BlockGateRepo_BlockedSetKeyIsTenantNamespaced guards the key
// the script reads: the gate and the block command must agree on it, or a block
// would be written to one key and enforced from another.
func TestIntegration_BlockGateRepo_BlockedSetKeyIsTenantNamespaced(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID, ledgerID := uuid.New(), uuid.New()
	accountID := uuid.New()

	// Written through the block command's path...
	require.NoError(t, infra.repo.HydrateBlockedAccounts(ctx, orgID, ledgerID, nil))
	require.NoError(t, infra.repo.AddBlockedAccount(ctx, orgID, ledgerID, accountID))

	expectedKey, err := tenantKeysFromContext(ctx, []string{utils.BlockedAccountsInternalKey(orgID, ledgerID)})
	require.NoError(t, err)

	members, err := infra.redisContainer.Client.SMembers(ctx, expectedKey[0]).Result()
	require.NoError(t, err)
	assert.Contains(t, members, accountID.String())

	// ...and enforced through the script's KEYS[4].
	ops := []mmodel.BalanceOperation{
		gateOp(orgID, ledgerID, accountID, "@repo-key", false,
			decimal.NewFromInt(500), decimal.Zero, constant.DEBIT, decimal.NewFromInt(100)),
	}

	_, err = infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, ops)

	var blockedErr AccountBlockedError
	require.ErrorAs(t, err, &blockedErr,
		"the gate must read the same key the block command writes")
}
