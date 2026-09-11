//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// requireCrossGateRejection asserts the adapter translated the script's rejection
// into the typed 409. The code is read off the struct rather than the message:
// EntityConflictError.Error() carries only the message, so a string match would
// pass on any conflict.
func requireCrossGateRejection(t *testing.T, err error) {
	t.Helper()

	require.Error(t, err)

	var conflict pkg.EntityConflictError

	require.ErrorAs(t, err, &conflict, "the cross gate must surface as a conflict")
	require.Equal(t, constant.ErrTransactionAlreadyTransitioned.Error(), conflict.Code)
}

// =============================================================================
// CROSS-TRANSITION GATE (0511) INTEGRATION TESTS
// =============================================================================
// These tests cover the gate in balance_atomic_operation.lua that refuses a
// terminal transition when the OPPOSITE terminal marker is already stamped: a
// commit arriving after a cancel landed, or the mirror. Without it the second
// transition posts a second movement on balances the first already settled —
// OnHold goes negative and money is created (the G7 incident).
//
// The gate sits AFTER the replay gate and BEFORE every other guard and every
// mutation, so a rejection leaves zero side effects and a resend of the SAME
// transition is still a replay rather than a conflict.
//
// Scope is the adapter layer only — a real Valkey from testcontainers, no
// application environment.

// heldBalanceOp builds the balance operation of a transition acting on a balance
// that already carries a hold: the state a pending create left behind.
func heldBalanceOp(
	orgID, ledgerID uuid.UUID,
	alias string,
	available, onHold decimal.Decimal,
	version int64,
	operation, transactionStatus string,
	amount decimal.Decimal,
) mmodel.BalanceOperation {
	return pendingOverdraftBalanceOp(orgID, ledgerID, alias, constant.DefaultBalanceKey, constant.DirectionCredit,
		available, onHold, decimal.Zero, version, nil,
		operation, transactionStatus, amount, decimal.Zero, false)
}

// crossGateBalanceSnapshot is the full state a duplicate transition would move.
type crossGateBalanceSnapshot struct {
	available string
	onHold    string
	version   int64
}

func snapshotOf(t *testing.T, infra *integrationTestInfra, key string) crossGateBalanceSnapshot {
	t.Helper()

	cached := readCachedBalance(t, infra, key)

	return crossGateBalanceSnapshot{
		available: cached.Available,
		onHold:    cached.OnHold,
		version:   cached.Version,
	}
}

// TestIntegration_CrossGate_CommitAfterCancelIsRejected covers case (a): the
// cancel landed and stamped its marker, so the commit that arrives afterwards is
// refused with 0511 and moves nothing.
func TestIntegration_CrossGate_CommitAfterCancelIsRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	txID := uuid.New()
	alias := "@cross-gate-commit-after-cancel"
	key := utils.BalanceInternalKey(orgID, ledgerID, alias+"#"+constant.DefaultBalanceKey)

	// The cancel lands: 300 available with 200 held becomes 500 available, nothing
	// held, and the :CANCELED marker is stamped.
	cancelOp := heldBalanceOp(orgID, ledgerID, alias,
		decimal.NewFromInt(300), decimal.NewFromInt(200), 2,
		constant.RELEASE, constant.CANCELED, decimal.NewFromInt(200))

	_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		txID, constant.CANCELED, true, []mmodel.BalanceOperation{cancelOp}, nil)
	require.NoError(t, err)

	afterCancel := snapshotOf(t, infra, key)
	require.Equal(t, "500", afterCancel.available)
	require.Equal(t, "0", afterCancel.onHold)

	cancelMarker := applyMarkerKeyFor(t, ctx, orgID, ledgerID, txID, constant.CANCELED)

	exists, err := infra.redisContainer.Client.Exists(ctx, cancelMarker).Result()
	require.NoError(t, err)
	require.Equal(t, int64(1), exists, "the cancel must stamp its marker")

	// The commit of the SAME transaction now arrives. Without the gate it would
	// subtract the already-released hold and credit the destination again.
	commitOp := heldBalanceOp(orgID, ledgerID, alias,
		decimal.NewFromInt(300), decimal.NewFromInt(200), 2,
		constant.DEBIT, constant.APPROVED, decimal.NewFromInt(200))

	_, err = infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		txID, constant.APPROVED, true, []mmodel.BalanceOperation{commitOp}, nil)

	requireCrossGateRejection(t, err)

	assert.Equal(t, afterCancel, snapshotOf(t, infra, key),
		"a rejected cross transition must not move Available, OnHold or Version")

	approvedMarker := applyMarkerKeyFor(t, ctx, orgID, ledgerID, txID, constant.APPROVED)

	exists, err = infra.redisContainer.Client.Exists(ctx, approvedMarker).Result()
	require.NoError(t, err)
	assert.Equal(t, int64(0), exists, "a rejected transition must leave no marker of its own")
}

// TestIntegration_CrossGate_CancelAfterCommitIsRejected covers case (b): the
// mirror of the case above, with the statuses swapped.
func TestIntegration_CrossGate_CancelAfterCommitIsRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	txID := uuid.New()
	alias := "@cross-gate-cancel-after-commit"
	key := utils.BalanceInternalKey(orgID, ledgerID, alias+"#"+constant.DefaultBalanceKey)

	// The commit lands: the hold is consumed, Available stays where the pending
	// create left it, and the :APPROVED marker is stamped.
	commitOp := heldBalanceOp(orgID, ledgerID, alias,
		decimal.NewFromInt(300), decimal.NewFromInt(200), 2,
		constant.DEBIT, constant.APPROVED, decimal.NewFromInt(200))

	_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		txID, constant.APPROVED, true, []mmodel.BalanceOperation{commitOp}, nil)
	require.NoError(t, err)

	afterCommit := snapshotOf(t, infra, key)
	require.Equal(t, "300", afterCommit.available)
	require.Equal(t, "0", afterCommit.onHold)

	// The cancel of the SAME transaction now arrives. Without the gate it would
	// release a hold that is no longer there and credit Available a second time.
	cancelOp := heldBalanceOp(orgID, ledgerID, alias,
		decimal.NewFromInt(300), decimal.NewFromInt(200), 2,
		constant.RELEASE, constant.CANCELED, decimal.NewFromInt(200))

	_, err = infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		txID, constant.CANCELED, true, []mmodel.BalanceOperation{cancelOp}, nil)

	requireCrossGateRejection(t, err)

	assert.Equal(t, afterCommit, snapshotOf(t, infra, key),
		"a rejected cross transition must not move Available, OnHold or Version")
}

// runCrossGateProbe executes the real script with an empty batch and BOTH marker
// slots filled, returning the raw response string and the script error. An empty
// batch is enough because both gates sit ahead of the operation loop, and the raw
// string is the only place the replay flag is observable — the adapter's decode
// does not surface it.
func runCrossGateProbe(t *testing.T, infra *integrationTestInfra, ctx context.Context,
	orgID, ledgerID, txID uuid.UUID, markerKey, oppositeMarkerKey string,
) (string, error) {
	t.Helper()

	transactionKey, err := tenantKeyFromContextOrError(ctx,
		utils.TransactionInternalKey(orgID, ledgerID, txID.String()))
	require.NoError(t, err)

	backupQueue, err := tenantKeyFromContextOrError(ctx, TransactionBackupQueue)
	require.NoError(t, err)

	scheduleKey, err := tenantKeyFromContextOrError(ctx, utils.BalanceSyncScheduleKey)
	require.NoError(t, err)

	raw, runErr := balanceAtomicScript.Run(ctx, infra.redisContainer.Client,
		[]string{backupQueue, transactionKey, scheduleKey},
		"", "", "0", markerKey, oppositeMarkerKey).Result()
	if runErr != nil {
		return "", runErr
	}

	response, ok := raw.(string)
	require.True(t, ok, "the script must answer with a string, got %T", raw)

	return response, nil
}

// TestIntegration_CrossGate_ReplayWinsOverTheCrossConflict covers case (c), the
// ORDER of the two gates. With BOTH markers present, a resend of the transition
// that already ran is a replay to be re-reported — never a 0511. Reversing the
// order would turn every go-redis retry of a legitimate commit into a conflict.
func TestIntegration_CrossGate_ReplayWinsOverTheCrossConflict(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	txID := uuid.New()
	alias := "@cross-gate-replay-order"
	key := utils.BalanceInternalKey(orgID, ledgerID, alias+"#"+constant.DefaultBalanceKey)

	commitOp := heldBalanceOp(orgID, ledgerID, alias,
		decimal.NewFromInt(300), decimal.NewFromInt(200), 2,
		constant.DEBIT, constant.APPROVED, decimal.NewFromInt(200))

	_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		txID, constant.APPROVED, true, []mmodel.BalanceOperation{commitOp}, nil)
	require.NoError(t, err)

	afterCommit := snapshotOf(t, infra, key)

	approvedMarker := applyMarkerKeyFor(t, ctx, orgID, ledgerID, txID, constant.APPROVED)
	canceledMarker := applyMarkerKeyFor(t, ctx, orgID, ledgerID, txID, constant.CANCELED)

	stored, err := infra.redisContainer.Client.Get(ctx, approvedMarker).Result()
	require.NoError(t, err)

	// Plant the opposite marker by hand, so BOTH are present when the resend
	// arrives. This is the only state in which the two gates disagree.
	require.NoError(t, infra.redisContainer.Client.Set(ctx, canceledMarker, stored, 0).Err())

	// The real adapter path: a resend of the same commit must succeed.
	replayResult, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		txID, constant.APPROVED, true, []mmodel.BalanceOperation{commitOp}, nil)
	require.NoError(t, err,
		"a resend of the transition that already ran must replay, never hit the cross gate")
	require.NotNil(t, replayResult)

	assert.Equal(t, afterCommit, snapshotOf(t, infra, key),
		"a replay must not move the balance a second time")

	// The wire proof: the response carries the replay flag, and the script never
	// reached the cross gate even with the opposite marker sitting right there.
	response, probeErr := runCrossGateProbe(t, infra, ctx, orgID, ledgerID, txID, approvedMarker, canceledMarker)
	require.NoError(t, probeErr, "the replay gate must answer before the cross gate is consulted")
	assert.Equal(t, `{"replayed":true,`+stored[1:], response)

	var decoded balanceAtomicResponse
	require.NoError(t, json.Unmarshal([]byte(response), &decoded))
	assert.True(t, decoded.Replayed)

	// And the control: with the OWN marker gone, the same opposite marker now
	// rejects — proving the probe above was answered by the replay gate and not by
	// a disarmed cross gate.
	require.NoError(t, infra.redisContainer.Client.Del(ctx, approvedMarker).Err())

	_, probeErr = runCrossGateProbe(t, infra, ctx, orgID, ledgerID, txID, approvedMarker, canceledMarker)
	require.Error(t, probeErr)
	assert.Contains(t, probeErr.Error(), constant.ErrTransactionAlreadyTransitioned.Error())
}

// TestIntegration_CrossGate_ExpiredOppositeMarkerIsTransparent covers case (d):
// the marker carries a TTL, so past it the gate evaporates and the transition is
// no longer barred here. That is the window the Postgres status CAS covers
// durably — the Lua gate is the fast path, not the only one.
func TestIntegration_CrossGate_ExpiredOppositeMarkerIsTransparent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	txID := uuid.New()
	alias := "@cross-gate-expired"
	key := utils.BalanceInternalKey(orgID, ledgerID, alias+"#"+constant.DefaultBalanceKey)

	cancelOp := heldBalanceOp(orgID, ledgerID, alias,
		decimal.NewFromInt(300), decimal.NewFromInt(200), 2,
		constant.RELEASE, constant.CANCELED, decimal.NewFromInt(200))

	_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		txID, constant.CANCELED, true, []mmodel.BalanceOperation{cancelOp}, nil)
	require.NoError(t, err)

	versionAfterCancel := snapshotOf(t, infra, key).version

	// Simulate the TTL elapsing: the marker is simply gone.
	canceledMarker := applyMarkerKeyFor(t, ctx, orgID, ledgerID, txID, constant.CANCELED)
	require.NoError(t, infra.redisContainer.Client.Del(ctx, canceledMarker).Err())

	// A hold is placed again so the commit has something to consume; it carries a
	// transaction id of its own, so no marker of this transaction is involved.
	holdOp := heldBalanceOp(orgID, ledgerID, alias,
		decimal.NewFromInt(500), decimal.Zero, versionAfterCancel,
		constant.ONHOLD, constant.PENDING, decimal.NewFromInt(200))

	_, err = infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.PENDING, true, []mmodel.BalanceOperation{holdOp}, nil)
	require.NoError(t, err)

	commitOp := heldBalanceOp(orgID, ledgerID, alias,
		decimal.NewFromInt(300), decimal.NewFromInt(200), versionAfterCancel+1,
		constant.DEBIT, constant.APPROVED, decimal.NewFromInt(200))

	_, err = infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		txID, constant.APPROVED, true, []mmodel.BalanceOperation{commitOp}, nil)
	require.NoError(t, err,
		"an expired opposite marker must leave the gate transparent")

	assert.Equal(t, "0", snapshotOf(t, infra, key).onHold)
}

// TestIntegration_CrossGate_IndependentTransitionsAreUnaffected is the other half
// of case (d): a commit and a cancel of DIFFERENT transactions never see each
// other's markers, so the gate never fires on a legitimate flow.
func TestIntegration_CrossGate_IndependentTransitionsAreUnaffected(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()

	commitAlias := "@cross-gate-independent-commit"
	cancelAlias := "@cross-gate-independent-cancel"

	commitOp := heldBalanceOp(orgID, ledgerID, commitAlias,
		decimal.NewFromInt(300), decimal.NewFromInt(200), 2,
		constant.DEBIT, constant.APPROVED, decimal.NewFromInt(200))

	_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.APPROVED, true, []mmodel.BalanceOperation{commitOp}, nil)
	require.NoError(t, err, "an independent commit must not be barred")

	cancelOp := heldBalanceOp(orgID, ledgerID, cancelAlias,
		decimal.NewFromInt(300), decimal.NewFromInt(200), 2,
		constant.RELEASE, constant.CANCELED, decimal.NewFromInt(200))

	_, err = infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		uuid.New(), constant.CANCELED, true, []mmodel.BalanceOperation{cancelOp}, nil)
	require.NoError(t, err, "an independent cancel must not be barred")

	commitState := snapshotOf(t, infra, utils.BalanceInternalKey(orgID, ledgerID, commitAlias+"#"+constant.DefaultBalanceKey))
	cancelState := snapshotOf(t, infra, utils.BalanceInternalKey(orgID, ledgerID, cancelAlias+"#"+constant.DefaultBalanceKey))

	assert.Equal(t, "300", commitState.available)
	assert.Equal(t, "0", commitState.onHold)
	assert.Equal(t, "500", cancelState.available)
	assert.Equal(t, "0", cancelState.onHold)
}

// TestIntegration_CrossGate_StatusWithoutOppositeIsDisarmed covers case (e): only
// the two terminal transitions of a pending can contradict each other, so a
// create resolves no opposite key at all. Markers of BOTH terminal statuses are
// planted for the very same transaction id, and the pending create still runs.
func TestIntegration_CrossGate_StatusWithoutOppositeIsDisarmed(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	infra := setupRedisIntegrationInfra(t)
	ctx := context.Background()
	orgID := uuid.New()
	ledgerID := uuid.New()
	txID := uuid.New()
	alias := "@cross-gate-disarmed"
	key := utils.BalanceInternalKey(orgID, ledgerID, alias+"#"+constant.DefaultBalanceKey)

	for _, status := range []string{constant.APPROVED, constant.CANCELED} {
		marker := applyMarkerKeyFor(t, ctx, orgID, ledgerID, txID, status)
		require.NoError(t, infra.redisContainer.Client.Set(ctx, marker, `{"before":[],"after":[]}`, 0).Err())
	}

	holdOp := heldBalanceOp(orgID, ledgerID, alias,
		decimal.NewFromInt(500), decimal.Zero, 1,
		constant.ONHOLD, constant.PENDING, decimal.NewFromInt(200))

	_, err := infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		txID, constant.PENDING, true, []mmodel.BalanceOperation{holdOp}, nil)
	require.NoError(t, err, "a pending create has no opposite terminal status; the gate must stay disarmed")

	held := snapshotOf(t, infra, key)
	assert.Equal(t, "300", held.available)
	assert.Equal(t, "200", held.onHold)
	assert.Equal(t, int64(2), held.version)

	// A direct (non-pending) posting is the same case: CREATED has no opposite.
	directAlias := "@cross-gate-disarmed-direct"
	directOp := overdraftOp(orgID, ledgerID, directAlias, "deposit", constant.DirectionCredit,
		decimal.NewFromInt(500), decimal.Zero, 1, nil,
		constant.DEBIT, decimal.NewFromInt(100))

	directTxID := uuid.New()

	for _, status := range []string{constant.APPROVED, constant.CANCELED} {
		marker := applyMarkerKeyFor(t, ctx, orgID, ledgerID, directTxID, status)
		require.NoError(t, infra.redisContainer.Client.Set(ctx, marker, `{"before":[],"after":[]}`, 0).Err())
	}

	_, err = infra.repo.ProcessBalanceAtomicOperation(ctx, orgID, ledgerID,
		directTxID, constant.CREATED, false, []mmodel.BalanceOperation{directOp}, nil)
	require.NoError(t, err, "CREATED has no opposite terminal status; the gate must stay disarmed")
}
