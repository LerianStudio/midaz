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
	"github.com/LerianStudio/midaz/v4/pkg/utils"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// APPLY MARKER (SCRIPT IDEMPOTENCY) INTEGRATION TESTS
// =============================================================================
// These tests cover the idempotency marker in balance_atomic_operation.lua: the
// script writes `transaction_apply_marker:{transactions}:<org>:<ledger>:<tx>:<STATUS>`
// atomically with the balance mutations, and a later execution of the SAME
// identity is answered from that marker instead of re-applying the deltas.
//
// The identity is what makes a resend safe without making a legitimate second
// posting impossible: a pending create (PENDING) and its commit (APPROVED) share
// a transaction id but not a status, so both legs still execute.
//
// Scope is the adapter layer only — a real Valkey from testcontainers, no
// application environment.

const applyMarkerTTLSeconds = 86400

// applyMarkerKeyFor builds the tenant-namespaced marker key exactly as
// ProcessBalanceAtomicOperation does, so a test asserts against the key the
// script was actually handed.
func applyMarkerKeyFor(t *testing.T, ctx context.Context, orgID, ledgerID uuid.UUID, transactionID uuid.UUID, status string) string {
	t.Helper()

	key, err := tenantKeyFromContextOrError(ctx,
		utils.TransactionApplyMarkerKey(orgID, ledgerID, transactionID.String(), status))
	require.NoError(t, err)

	return key
}

// balanceStateOf reduces a cached balance to the three fields a duplicate
// application would move.
func balanceStateOf(t *testing.T, infra *integrationTestInfra, key string) (available string, overdraftUsed string, version int64) {
	t.Helper()

	cached := readCachedBalance(t, infra, key)

	return cached.Available, cached.OverdraftUsed, cached.Version
}

// TestIntegration_ApplyMarker_ReplayIsIdempotent is the core case: the same
// execution submitted twice moves the balance once, and the second call is
// answered with the first call's response.
//
// It also asserts the WIRE contract the Go decode depends on. The response the
// second execution returns is the stored payload with `"replayed":true` spliced
// onto its front as raw bytes — never a cjson round trip, which could reformat
// the decimal strings that carry financial values.
func TestIntegration_ApplyMarker_ReplayIsIdempotent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	txID := uuid.New()

	op := overdraftOp(orgID, ledgerID, "@apply-replay", "deposit", "credit",
		decimal.NewFromInt(500), decimal.Zero, 1, nil,
		constant.DEBIT, decimal.NewFromInt(200))

	first, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		txID, constant.APPROVED, false, []mmodel.BalanceOperation{op}, nil)
	require.NoError(t, err)
	require.Len(t, first.After, 1)

	availableAfterFirst, _, versionAfterFirst := balanceStateOf(t, infra, op.InternalKey)
	assert.Equal(t, "300", availableAfterFirst)
	assert.Equal(t, int64(2), versionAfterFirst)

	// Same identity, same plan — this is what a go-redis resend looks like to the
	// server, and what a client retry looks like to the adapter.
	second, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		txID, constant.APPROVED, false, []mmodel.BalanceOperation{op}, nil)
	require.NoError(t, err, "a replay must succeed, not fail")
	require.Len(t, second.After, 1)

	availableAfterSecond, _, versionAfterSecond := balanceStateOf(t, infra, op.InternalKey)
	assert.Equal(t, availableAfterFirst, availableAfterSecond,
		"a replay must not move Available")
	assert.Equal(t, versionAfterFirst, versionAfterSecond,
		"a replay must not increment Version")

	assert.Equal(t, first.Before[0].Available.String(), second.Before[0].Available.String())
	assert.Equal(t, first.After[0].Available.String(), second.After[0].Available.String())
	assert.Equal(t, first.After[0].Version, second.After[0].Version)

	// Wire contract: the replayed payload is the stored one, byte for byte, behind
	// the spliced flag.
	markerKey := applyMarkerKeyFor(t, ctx, orgID, ledgerID, txID, constant.APPROVED)

	stored, err := infra.redisContainer.Client.Get(ctx, markerKey).Result()
	require.NoError(t, err, "a successful execution must leave a marker")

	replayed := runApplyMarkerReplayProbe(t, infra, ctx, orgID, ledgerID, txID, markerKey)
	assert.Equal(t, `{"replayed":true,`+stored[1:], replayed,
		"the replay must splice the flag onto the stored payload without re-encoding it")

	var decoded balanceAtomicResponse
	require.NoError(t, json.Unmarshal([]byte(replayed), &decoded))
	assert.True(t, decoded.Replayed, "the adapter must decode the replay flag")
}

// runApplyMarkerReplayProbe executes the real script against the marker with an
// empty batch, returning the raw response string. The replay gate is the first
// thing the script does, so an empty batch is enough to read the stored payload
// back off the wire — and it is the only way to observe the raw string, which
// the adapter's decode does not surface.
func runApplyMarkerReplayProbe(t *testing.T, infra *integrationTestInfra, ctx context.Context, orgID, ledgerID, txID uuid.UUID, markerKey string) string {
	t.Helper()

	transactionKey, err := tenantKeyFromContextOrError(ctx,
		utils.TransactionInternalKey(orgID, ledgerID, txID.String()))
	require.NoError(t, err)

	backupQueue, err := tenantKeyFromContextOrError(ctx, TransactionBackupQueue)
	require.NoError(t, err)

	scheduleKey, err := tenantKeyFromContextOrError(ctx, utils.BalanceSyncScheduleKey)
	require.NoError(t, err)

	raw, err := balanceAtomicScript.Run(ctx, infra.redisContainer.Client,
		[]string{backupQueue, transactionKey, scheduleKey},
		"", "", "0", markerKey).Result()
	require.NoError(t, err)

	response, ok := raw.(string)
	require.True(t, ok, "the script must answer with a string, got %T", raw)

	return response
}

// TestIntegration_ApplyMarker_MarkerCarriesResponseAndTTL locks the marker's
// storage contract: it holds the response the caller received, and it expires on
// the balance snapshot's horizon rather than living forever.
func TestIntegration_ApplyMarker_MarkerCarriesResponseAndTTL(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	txID := uuid.New()

	op := overdraftOp(orgID, ledgerID, "@apply-ttl", "deposit", "credit",
		decimal.NewFromInt(500), decimal.Zero, 1, nil,
		constant.DEBIT, decimal.NewFromInt(100))

	_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		txID, constant.APPROVED, false, []mmodel.BalanceOperation{op}, nil)
	require.NoError(t, err)

	markerKey := applyMarkerKeyFor(t, ctx, orgID, ledgerID, txID, constant.APPROVED)

	stored, err := infra.redisContainer.Client.Get(ctx, markerKey).Result()
	require.NoError(t, err)

	var decoded balanceAtomicResponse
	require.NoError(t, json.Unmarshal([]byte(stored), &decoded))
	assert.False(t, decoded.Replayed, "the stored payload is the ORIGINAL response, unflagged")
	require.Len(t, decoded.After, 1)
	assert.Equal(t, "400", decoded.After[0].Available.String())

	ttl, err := infra.redisContainer.Client.TTL(ctx, markerKey).Result()
	require.NoError(t, err)
	assert.Greater(t, ttl.Seconds(), float64(applyMarkerTTLSeconds-100),
		"the marker must carry the full TTL")
	assert.LessOrEqual(t, ttl.Seconds(), float64(applyMarkerTTLSeconds))
}

// TestIntegration_ApplyMarker_PendingLifecycleAppliesBothLegs is the case the
// identity exists for: a pending create and its commit share a transaction id.
// Both must post, and only a resend of either must replay.
func TestIntegration_ApplyMarker_PendingLifecycleAppliesBothLegs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	txID := uuid.New()

	holdOp := overdraftOp(orgID, ledgerID, "@apply-lifecycle", "deposit", "credit",
		decimal.NewFromInt(500), decimal.Zero, 1, nil,
		constant.ONHOLD, decimal.NewFromInt(200))

	_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		txID, constant.PENDING, true, []mmodel.BalanceOperation{holdOp}, nil)
	require.NoError(t, err)

	available, _, version := balanceStateOf(t, infra, holdOp.InternalKey)
	assert.Equal(t, "300", available)
	assert.Equal(t, int64(2), version)

	onHold := readCachedBalance(t, infra, holdOp.InternalKey).OnHold
	assert.Equal(t, "200", onHold)

	// The commit carries the SAME transaction id with a different status, so it is
	// a different identity and must execute.
	commitOp := overdraftOp(orgID, ledgerID, "@apply-lifecycle", "deposit", "credit",
		decimal.NewFromInt(300), decimal.Zero, 2, nil,
		constant.DEBIT, decimal.NewFromInt(200))

	_, err = infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		txID, constant.APPROVED, true, []mmodel.BalanceOperation{commitOp}, nil)
	require.NoError(t, err, "the commit must not be mistaken for a replay of the create")

	availableAfterCommit, _, versionAfterCommit := balanceStateOf(t, infra, holdOp.InternalKey)
	assert.Equal(t, "300", availableAfterCommit)
	assert.Equal(t, int64(3), versionAfterCommit)
	assert.Equal(t, "0", readCachedBalance(t, infra, holdOp.InternalKey).OnHold)

	// Both identities are marked, independently.
	pendingMarker := applyMarkerKeyFor(t, ctx, orgID, ledgerID, txID, constant.PENDING)
	approvedMarker := applyMarkerKeyFor(t, ctx, orgID, ledgerID, txID, constant.APPROVED)
	assert.NotEqual(t, pendingMarker, approvedMarker)

	for _, key := range []string{pendingMarker, approvedMarker} {
		exists, existsErr := infra.redisContainer.Client.Exists(ctx, key).Result()
		require.NoError(t, existsErr)
		assert.Equal(t, int64(1), exists, "marker %q must exist", key)
	}

	// Resending the commit replays.
	_, err = infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		txID, constant.APPROVED, true, []mmodel.BalanceOperation{commitOp}, nil)
	require.NoError(t, err)

	availableAfterReplay, _, versionAfterReplay := balanceStateOf(t, infra, holdOp.InternalKey)
	assert.Equal(t, availableAfterCommit, availableAfterReplay)
	assert.Equal(t, versionAfterCommit, versionAfterReplay,
		"a resent commit must not increment Version a second time")
}

// TestIntegration_ApplyMarker_BusinessErrorLeavesNoMarker proves a rejected batch
// stays retryable. The script rolls its writes back on a business error, so the
// state a retry re-reads is the pre-execution state and the retry must be free to
// reach the same rejection — a marker here would answer a later, legitimately
// different attempt with a stale failure.
func TestIntegration_ApplyMarker_BusinessErrorLeavesNoMarker(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	txID := uuid.New()

	op := overdraftOp(orgID, ledgerID, "@apply-insufficient", "deposit", "credit",
		decimal.NewFromInt(500), decimal.Zero, 1, nil,
		constant.DEBIT, decimal.NewFromInt(900))

	_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		txID, constant.APPROVED, false, []mmodel.BalanceOperation{op}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), constant.ErrInsufficientFunds.Error())

	markerKey := applyMarkerKeyFor(t, ctx, orgID, ledgerID, txID, constant.APPROVED)

	exists, err := infra.redisContainer.Client.Exists(ctx, markerKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), exists, "a rejected batch must leave no marker")

	// The retry reproduces the rejection rather than replaying anything.
	_, err = infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		txID, constant.APPROVED, false, []mmodel.BalanceOperation{op}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), constant.ErrInsufficientFunds.Error())

	exists, err = infra.redisContainer.Client.Exists(ctx, markerKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), exists)
}

// TestIntegration_ApplyMarker_EmptyBatchIsMarked covers the batch that changes
// nothing — a deferred destination credit on a pending create. It is still a
// completed execution, so it is marked: a resend replays instead of walking the
// ladder again.
func TestIntegration_ApplyMarker_EmptyBatchIsMarked(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	txID := uuid.New()

	// CREDIT on a PENDING batch matches no branch of the ladder: the destination
	// leg only posts on the commit, so nothing changes here.
	op := overdraftOp(orgID, ledgerID, "@apply-empty", "deposit", "credit",
		decimal.NewFromInt(500), decimal.Zero, 1, nil,
		constant.CREDIT, decimal.NewFromInt(100))

	result, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		txID, constant.PENDING, true, []mmodel.BalanceOperation{op}, nil)
	require.NoError(t, err)
	assert.Empty(t, result.Before)
	assert.Empty(t, result.After)

	markerKey := applyMarkerKeyFor(t, ctx, orgID, ledgerID, txID, constant.PENDING)

	stored, err := infra.redisContainer.Client.Get(ctx, markerKey).Result()
	require.NoError(t, err, "a zero-change batch is still an execution and must be marked")

	replayed := runApplyMarkerReplayProbe(t, infra, ctx, orgID, ledgerID, txID, markerKey)
	assert.Equal(t, `{"replayed":true,`+stored[1:], replayed)

	var decoded balanceAtomicResponse
	require.NoError(t, json.Unmarshal([]byte(replayed), &decoded))
	assert.True(t, decoded.Replayed)
	assert.Empty(t, decoded.Before)
	assert.Empty(t, decoded.After)
}

// TestIntegration_ApplyMarker_TenantNamespacedKey proves the marker travels
// through the SAME namespacing path as every other key the script is handed, so
// two tenants cannot answer each other's replays.
func TestIntegration_ApplyMarker_TenantNamespacedKey(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	tenantID := "apply-marker-tenant-" + uuid.NewString()
	ctx := tmcore.ContextWithTenantID(context.Background(), tenantID)
	orgID := uuid.New()
	ledgerID := uuid.New()
	txID := uuid.New()

	op := overdraftOp(orgID, ledgerID, "@apply-tenant", "deposit", "credit",
		decimal.NewFromInt(500), decimal.Zero, 1, nil,
		constant.DEBIT, decimal.NewFromInt(100))

	_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		txID, constant.APPROVED, false, []mmodel.BalanceOperation{op}, nil)
	require.NoError(t, err)

	logicalKey := utils.TransactionApplyMarkerKey(orgID, ledgerID, txID.String(), constant.APPROVED)
	physicalKey := applyMarkerKeyFor(t, ctx, orgID, ledgerID, txID, constant.APPROVED)

	require.True(t, strings.HasSuffix(physicalKey, logicalKey))
	assert.Equal(t, "tenant:"+tenantID+":"+logicalKey, physicalKey,
		"the marker must carry the tenant prefix the other keys carry")

	exists, err := infra.redisContainer.Client.Exists(context.Background(), physicalKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(1), exists)

	unprefixed, err := infra.redisContainer.Client.Exists(context.Background(), logicalKey).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), unprefixed, "no marker may be written outside the tenant namespace")
}

// TestIntegration_ApplyMarker_ReplayPreservesBackupHash covers the side effect a
// resend used to have even when the balances were the thing at stake: the replay
// returns BEFORE updateTransactionHash, so the backup queue still holds the
// snapshots of the execution that actually posted.
func TestIntegration_ApplyMarker_ReplayPreservesBackupHash(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	txID := uuid.New()

	op := overdraftOp(orgID, ledgerID, "@apply-backup", "deposit", "credit",
		decimal.NewFromInt(500), decimal.Zero, 1, nil,
		constant.DEBIT, decimal.NewFromInt(200))

	_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		txID, constant.APPROVED, false, []mmodel.BalanceOperation{op}, nil)
	require.NoError(t, err)

	transactionKey := utils.TransactionInternalKey(orgID, ledgerID, txID.String())

	backupAfterFirst, err := infra.redisContainer.Client.HGet(ctx, TransactionBackupQueue, transactionKey).Result()
	require.NoError(t, err)
	require.Contains(t, backupAfterFirst, "balancesAfter")

	// A resend carrying DIFFERENT amounts is still the same identity: it must not
	// rewrite the recorded snapshots.
	divergentOp := overdraftOp(orgID, ledgerID, "@apply-backup", "deposit", "credit",
		decimal.NewFromInt(300), decimal.Zero, 2, nil,
		constant.DEBIT, decimal.NewFromInt(50))

	_, err = infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		txID, constant.APPROVED, false, []mmodel.BalanceOperation{divergentOp}, nil)
	require.NoError(t, err)

	backupAfterReplay, err := infra.redisContainer.Client.HGet(ctx, TransactionBackupQueue, transactionKey).Result()
	require.NoError(t, err)
	assert.Equal(t, backupAfterFirst, backupAfterReplay,
		"a replay must not overwrite the backup snapshots of the execution that posted")

	available, _, version := balanceStateOf(t, infra, op.InternalKey)
	assert.Equal(t, "300", available)
	assert.Equal(t, int64(2), version)
}

// TestIntegration_ApplyMarker_TotalOverdraftRepaymentAppliesOnce closes the edge
// the card found. A credit that repays an overdraft IN FULL leaves
// OverdraftUsed at zero, so a resend of it no longer enters the repayment block
// and never reaches its stale-version guard: it simply credits Available a
// second time, silently. The marker is what stops it.
func TestIntegration_ApplyMarker_TotalOverdraftRepaymentAppliesOnce(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	settings := &mmodel.BalanceSettings{AllowOverdraft: true}

	// Draw the overdraft: 100 available, 300 debited -> Available 0, used 200.
	drawOp := overdraftOp(orgID, ledgerID, "@apply-overdraft", "deposit", "credit",
		decimal.NewFromInt(100), decimal.Zero, 1, settings,
		constant.DEBIT, decimal.NewFromInt(300))

	_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, false, []mmodel.BalanceOperation{drawOp}, nil)
	require.NoError(t, err)

	available, overdraftUsed, version := balanceStateOf(t, infra, drawOp.InternalKey)
	require.Equal(t, "0", available)
	require.Equal(t, "200", overdraftUsed)
	require.Equal(t, int64(2), version)

	// Repay it in FULL.
	repayTxID := uuid.New()
	repayOp := overdraftOp(orgID, ledgerID, "@apply-overdraft", "deposit", "credit",
		decimal.Zero, decimal.NewFromInt(200), 2, settings,
		constant.CREDIT, decimal.NewFromInt(200))

	_, err = infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		repayTxID, constant.APPROVED, false, []mmodel.BalanceOperation{repayOp}, nil)
	require.NoError(t, err)

	availableAfterRepay, overdraftAfterRepay, versionAfterRepay := balanceStateOf(t, infra, drawOp.InternalKey)
	require.Equal(t, "0", availableAfterRepay)
	require.Equal(t, "0", overdraftAfterRepay)
	require.Equal(t, int64(3), versionAfterRepay)

	// Resend it. Without the marker this credits Available a second time.
	_, err = infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		repayTxID, constant.APPROVED, false, []mmodel.BalanceOperation{repayOp}, nil)
	require.NoError(t, err)

	availableAfterResend, overdraftAfterResend, versionAfterResend := balanceStateOf(t, infra, drawOp.InternalKey)
	assert.Equal(t, "0", availableAfterResend,
		"a resent total repayment must not credit Available a second time")
	assert.Equal(t, "0", overdraftAfterResend)
	assert.Equal(t, versionAfterRepay, versionAfterResend,
		"a resent total repayment must not increment Version a second time")
}
