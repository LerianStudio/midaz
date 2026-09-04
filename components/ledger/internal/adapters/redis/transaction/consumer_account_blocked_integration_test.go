//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"strings"
	"testing"
	"time"

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
// ACCOUNT-BLOCK GUARD INTEGRATION TESTS
// =============================================================================
// These tests cover the Lua pre-pass in balance_atomic_operation.lua that
// rejects a batch with ErrAccountBlocked (0502) before any mutation when any
// involved balance belongs to a blocked account, the CANCELED exemption
// (RF-4C), the blob-wins effective-state rule, and the in-place Blocked
// rewrite performed by scripts/update_balance_blocked.lua.
//
// At the Lua contract level a REVERT is indistinguishable from a direct
// create: both run with isPending=false and transactionStatus=APPROVED. The
// direct-shaped cases below therefore cover the revert surface too.

// blockedOp builds a BalanceOperation whose balance carries the account-block
// flag, mirroring the shape overdraftOp produces for the overdraft suite.
func blockedOp(
	orgID, ledgerID uuid.UUID,
	alias string,
	available, onHold decimal.Decimal,
	version int64,
	blocked bool,
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
			OnHold:         onHold,
			Version:        version,
			AccountType:    "deposit",
			AllowSending:   true,
			AllowReceiving: true,
			Blocked:        blocked,
			CreatedAt:      time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			UpdatedAt:      time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
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

func requireAccountBlockedErr(t *testing.T, err error) {
	t.Helper()

	require.Error(t, err, "blocked account must reject the batch")
	assert.True(t, strings.Contains(err.Error(), constant.ErrAccountBlocked.Error()),
		"error should contain 0502, got: %v", err)
}

// TestIntegration_AccountBlocked_DirectSourceRejects covers the direct create
// (and, by ARGV shape, the revert): a blocked source balance on a cold cache
// rejects with 0502 via the ARGV fallback, and the fill never happens.
func TestIntegration_AccountBlocked_DirectSourceRejects(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	op := blockedOp(orgID, ledgerID, "@blk-direct-src", decimal.NewFromInt(500), decimal.Zero, 1, true,
		constant.DEBIT, decimal.NewFromInt(100))

	_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{op})
	requireAccountBlockedErr(t, err)

	exists, err := infra.redisContainer.Client.Exists(ctx, op.InternalKey).Result()
	require.NoError(t, err)
	assert.Zero(t, exists, "the guard rejects before the fill, so the blob must not be created")
}

// TestIntegration_AccountBlocked_DestinationRejectsAndBatchIsAtomic covers the
// bidirectional rule and batch atomicity: only the DESTINATION is blocked, the
// whole batch rejects and the healthy source balance is left unmutated.
func TestIntegration_AccountBlocked_DestinationRejectsAndBatchIsAtomic(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	// Prime the source balance so it has a live blob to protect.
	primeSrc := blockedOp(orgID, ledgerID, "@blk-atomic-src", decimal.NewFromInt(500), decimal.Zero, 1, false,
		constant.DEBIT, decimal.NewFromInt(100))
	_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{primeSrc})
	require.NoError(t, err)

	before := readCachedBalance(t, infra, primeSrc.InternalKey)

	src := blockedOp(orgID, ledgerID, "@blk-atomic-src", decimal.NewFromInt(400), decimal.Zero, before.Version, false,
		constant.DEBIT, decimal.NewFromInt(50))
	dst := blockedOp(orgID, ledgerID, "@blk-atomic-dst", decimal.NewFromInt(0), decimal.Zero, 1, true,
		constant.CREDIT, decimal.NewFromInt(50))

	_, err = infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{src, dst})
	requireAccountBlockedErr(t, err)

	after := readCachedBalance(t, infra, primeSrc.InternalKey)
	assert.Equal(t, before.Available, after.Available, "no balance may be mutated in a rejected batch")
	assert.Equal(t, before.Version, after.Version, "version must not increment in a rejected batch")
}

// TestIntegration_AccountBlocked_HoldRejects covers the pending create.
func TestIntegration_AccountBlocked_HoldRejects(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	op := blockedOp(orgID, ledgerID, "@blk-hold", decimal.NewFromInt(500), decimal.Zero, 1, true,
		constant.ONHOLD, decimal.NewFromInt(100))

	_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.PENDING, true, []mmodel.BalanceOperation{op})
	requireAccountBlockedErr(t, err)
}

// TestIntegration_AccountBlocked_CommitOfPreBlockPendingRejects covers RF-04:
// a pending created while the account was open does not grandfather its
// commit. The block lands via the in-place rewrite (blob wins over the stale
// ARGV the commit still carries), and the commit is rejected.
func TestIntegration_AccountBlocked_CommitOfPreBlockPendingRejects(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	// Create the pending hold while the account is open.
	hold := blockedOp(orgID, ledgerID, "@blk-commit", decimal.NewFromInt(500), decimal.Zero, 1, false,
		constant.ONHOLD, decimal.NewFromInt(100))
	_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.PENDING, true, []mmodel.BalanceOperation{hold})
	require.NoError(t, err, "hold on an open account must succeed")

	// Block the account: in-place rewrite of the live blob.
	require.NoError(t, infra.repo.UpdateBalanceCacheBlocked(ctx, orgID, ledgerID,
		[]string{"@blk-commit#default"}, true))

	// Commit the pending: ARGV still says not blocked (stale Go read), the
	// blob says blocked — the blob wins and the commit rejects.
	held := readCachedBalance(t, infra, hold.InternalKey)
	commit := blockedOp(orgID, ledgerID, "@blk-commit", decimal.NewFromInt(400), decimal.NewFromInt(100), held.Version, false,
		constant.DEBIT, decimal.NewFromInt(100))

	_, err = infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, true, []mmodel.BalanceOperation{commit})
	requireAccountBlockedErr(t, err)

	after := readCachedBalance(t, infra, hold.InternalKey)
	assert.Equal(t, held.OnHold, after.OnHold, "rejected commit must leave the hold untouched")
}

// TestIntegration_AccountBlocked_CancelPasses covers RF-4C: a CANCELED batch
// skips the guard entirely, releasing the hold on a blocked account.
func TestIntegration_AccountBlocked_CancelPasses(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	hold := blockedOp(orgID, ledgerID, "@blk-cancel", decimal.NewFromInt(500), decimal.Zero, 1, false,
		constant.ONHOLD, decimal.NewFromInt(100))
	_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.PENDING, true, []mmodel.BalanceOperation{hold})
	require.NoError(t, err)

	require.NoError(t, infra.repo.UpdateBalanceCacheBlocked(ctx, orgID, ledgerID,
		[]string{"@blk-cancel#default"}, true))

	held := readCachedBalance(t, infra, hold.InternalKey)
	cancel := blockedOp(orgID, ledgerID, "@blk-cancel", decimal.NewFromInt(400), decimal.NewFromInt(100), held.Version, true,
		constant.RELEASE, decimal.NewFromInt(100))

	result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.CANCELED, true, []mmodel.BalanceOperation{cancel})
	require.NoError(t, err, "cancel must always pass, even on a blocked account (RF-4C)")
	require.Len(t, result.After, 1)

	after := readCachedBalance(t, infra, hold.InternalKey)
	assert.Equal(t, "0", after.OnHold, "cancel must release the hold")
	assert.Equal(t, "500", after.Available, "cancel must restore Available")
	assert.Equal(t, 1, after.Blocked, "the cancel rewrite must preserve the Blocked flag on the blob")
}

// TestIntegration_AccountBlocked_CancelFillMaterializesBlocked proves the one
// transactional path that can fill a blob for a blocked account — a CANCELED
// batch on a cold cache — materializes Blocked=1 in the Lua CamelCase casing.
func TestIntegration_AccountBlocked_CancelFillMaterializesBlocked(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	cancel := blockedOp(orgID, ledgerID, "@blk-fill", decimal.NewFromInt(400), decimal.NewFromInt(100), 3, true,
		constant.RELEASE, decimal.NewFromInt(100))

	_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.CANCELED, true, []mmodel.BalanceOperation{cancel})
	require.NoError(t, err)

	raw, err := infra.redisContainer.Client.Get(ctx, cancel.InternalKey).Result()
	require.NoError(t, err)
	assert.Contains(t, raw, `"Blocked":1`, "the fill must persist the flag in CamelCase for the Lua reader")
}

// TestIntegration_AccountBlocked_BlobWinsOverArgv locks the effective-state
// rule in both directions: a live blob saying open admits a batch whose stale
// ARGV says blocked, and a live blob saying blocked rejects a batch whose
// stale ARGV says open.
func TestIntegration_AccountBlocked_BlobWinsOverArgv(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	// Prime an OPEN blob.
	prime := blockedOp(orgID, ledgerID, "@blk-blobwins", decimal.NewFromInt(500), decimal.Zero, 1, false,
		constant.DEBIT, decimal.NewFromInt(100))
	_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{prime})
	require.NoError(t, err)

	// Stale ARGV claims blocked; the open blob wins and the batch passes.
	cur := readCachedBalance(t, infra, prime.InternalKey)
	staleBlocked := blockedOp(orgID, ledgerID, "@blk-blobwins", decimal.NewFromInt(400), decimal.Zero, cur.Version, true,
		constant.DEBIT, decimal.NewFromInt(50))

	_, err = infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{staleBlocked})
	require.NoError(t, err, "an open live blob must win over a stale blocked ARGV")

	// Flip the blob to blocked; a stale open ARGV must now reject.
	require.NoError(t, infra.repo.UpdateBalanceCacheBlocked(ctx, orgID, ledgerID,
		[]string{"@blk-blobwins#default"}, true))

	cur = readCachedBalance(t, infra, prime.InternalKey)
	staleOpen := blockedOp(orgID, ledgerID, "@blk-blobwins", decimal.NewFromInt(350), decimal.Zero, cur.Version, false,
		constant.DEBIT, decimal.NewFromInt(50))

	_, err = infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{staleOpen})
	requireAccountBlockedErr(t, err)
}

// TestIntegration_AccountBlocked_LegacyBlobIsNotBlocked covers rollout
// compatibility: a live blob written before the feature (no Blocked field)
// admits transactions, and the post-mutation rewrite materializes Blocked=0.
func TestIntegration_AccountBlocked_LegacyBlobIsNotBlocked(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	key := utils.BalanceInternalKey(orgID, ledgerID, "@blk-legacy#default")
	balanceID := uuid.New().String()
	accountID := uuid.New().String()

	legacy := `{"ID":"` + balanceID + `","Available":"500","OnHold":"0","Version":1,` +
		`"AccountType":"deposit","AccountID":"` + accountID + `","AssetCode":"USD",` +
		`"AllowSending":1,"AllowReceiving":1,"Key":"default"}`
	require.NoError(t, infra.redisContainer.Client.Set(ctx, key, legacy, time.Hour).Err())

	op := blockedOp(orgID, ledgerID, "@blk-legacy", decimal.NewFromInt(500), decimal.Zero, 1, false,
		constant.DEBIT, decimal.NewFromInt(100))

	result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{op})
	require.NoError(t, err, "a legacy blob without the Blocked field must not block")
	require.Len(t, result.After, 1)

	raw, err := infra.redisContainer.Client.Get(ctx, key).Result()
	require.NoError(t, err)
	assert.Contains(t, raw, `"Blocked":0`,
		"the post-mutation rewrite must materialize Blocked=0 on a legacy blob")
}

// =============================================================================
// update_balance_blocked.lua CONTRACT TESTS
// =============================================================================

func TestIntegration_UpdateBalanceBlockedScript_AbsentKeyIsSkipped(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	key := "blocked-script-test:" + uuid.NewString() // never seeded

	result, err := updateBalanceBlockedScript.Run(ctx, infra.redisContainer.Client,
		[]string{key}, 1, "86400").Result()
	require.NoError(t, err)

	values, ok := result.([]any)
	require.True(t, ok, "script must return the {written, corrupt} pair")
	assert.EqualValues(t, 0, values[0], "an absent key must not count as written")
	assert.EqualValues(t, 0, values[1], "an absent key is not corrupt")

	exists, err := infra.redisContainer.Client.Exists(ctx, key).Result()
	require.NoError(t, err)
	assert.Zero(t, exists, "the script must not create absent keys")
}

func TestIntegration_UpdateBalanceBlockedScript_CorruptBlobSkippedAndReported(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()

	key := "blocked-script-test:" + uuid.NewString()
	corrupt := "not-valid-json{{{"
	require.NoError(t, infra.redisContainer.Client.Set(ctx, key, corrupt, time.Hour).Err())

	result, err := updateBalanceBlockedScript.Run(ctx, infra.redisContainer.Client,
		[]string{key}, 1, "86400").Result()
	require.NoError(t, err)

	values, ok := result.([]any)
	require.True(t, ok)
	assert.EqualValues(t, 0, values[0])
	assert.EqualValues(t, 1, values[1], "a corrupt blob must be reported")

	unchanged, err := infra.redisContainer.Client.Get(ctx, key).Result()
	require.NoError(t, err)
	assert.Equal(t, corrupt, unchanged, "a corrupt blob must not be mutated")
}

// TestIntegration_UpdateBalanceBlocked_RewritePreservesLiveState locks the
// no-DEL contract end-to-end through the repository method: a blob produced by
// the real atomic script gets ONLY its Blocked flag rewritten — Available,
// OnHold, Version, OverdraftUsed and casing are byte-level intact — and the
// flag flips back off on unblock (idempotent no-op semantics, RF-02).
func TestIntegration_UpdateBalanceBlocked_RewritePreservesLiveState(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	// Materialize a real blob with live state via the atomic script.
	prime := blockedOp(orgID, ledgerID, "@blk-rewrite", decimal.NewFromInt(500), decimal.Zero, 1, false,
		constant.DEBIT, decimal.NewFromInt(200))
	_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{prime})
	require.NoError(t, err)

	before := readCachedBalance(t, infra, prime.InternalKey)

	// Block: only the flag may change.
	require.NoError(t, infra.repo.UpdateBalanceCacheBlocked(ctx, orgID, ledgerID,
		[]string{"@blk-rewrite#default"}, true))

	blocked := readCachedBalance(t, infra, prime.InternalKey)
	assert.Equal(t, 1, blocked.Blocked)
	assert.Equal(t, before.Available, blocked.Available, "rewrite must preserve Available")
	assert.Equal(t, before.OnHold, blocked.OnHold, "rewrite must preserve OnHold")
	assert.Equal(t, before.Version, blocked.Version, "rewrite must preserve Version")
	assert.Equal(t, before.OverdraftUsed, blocked.OverdraftUsed, "rewrite must preserve OverdraftUsed")

	ttl, err := infra.redisContainer.Client.TTL(ctx, prime.InternalKey).Result()
	require.NoError(t, err)
	assert.Greater(t, ttl, time.Duration(0), "the rewrite must keep the key expiring")

	// Unblock twice: the second pass is a natural no-op (RF-02).
	for range 2 {
		require.NoError(t, infra.repo.UpdateBalanceCacheBlocked(ctx, orgID, ledgerID,
			[]string{"@blk-rewrite#default"}, false))
	}

	open := readCachedBalance(t, infra, prime.InternalKey)
	assert.Equal(t, 0, open.Blocked)
	assert.Equal(t, before.Available, open.Available)
	assert.Equal(t, before.Version, open.Version)
}
