//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"
)

func TestIntegrationAtomicTransactionBatchIdempotencyTransitionsAndCleanup(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	repository, err := NewConsumerRedis(&staticRedisProvider{client: container.Client})
	require.NoError(t, err)
	organizationID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	ledgerID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	tenantID := uuid.NewSHA1(uuid.MustParse("b6a41d73-ef6c-50c6-a97a-7828077996e7"), []byte(t.Name())).String()
	ctx := tmcore.ContextWithTenantID(t.Context(), tenantID)

	t.Run("state progression starts TTL only at complete", func(t *testing.T) {
		effectiveKey := "state-progression"
		redisKey := transitionNamespacedBatchKey(t, ctx, organizationID, ledgerID, effectiveKey)
		executionID := uuid.MustParse("00000000-0000-0000-0000-000000000020")
		indexKey := transitionNamespacedBatchExecutionIndexKey(t, ctx, organizationID, ledgerID, executionID)
		t.Cleanup(func() { require.NoError(t, container.Client.Del(context.Background(), redisKey, indexKey).Err()) })

		claim := atomicBatchIdempotencyClaim("a")
		claimed, err := repository.ClaimAtomicTransactionBatch(ctx, organizationID, ledgerID, effectiveKey, claim)
		require.NoError(t, err)
		require.Equal(t, AtomicTransactionBatchClaimed, claimed.Outcome)
		require.Equal(t, time.Duration(-1), container.Client.TTL(ctx, redisKey).Val())

		prepared := atomicBatchPreparedRecord(false)
		transitioned, err := repository.TransitionAtomicTransactionBatch(
			ctx, organizationID, ledgerID, effectiveKey, claim.OwnerToken,
			AtomicTransactionBatchStateClaimed, prepared, 0,
		)
		require.NoError(t, err)
		require.Equal(t, AtomicTransactionBatchTransitionUpdated, transitioned.Outcome)
		require.Equal(t, time.Duration(-1), container.Client.TTL(ctx, redisKey).Val())

		changedIDs := atomicBatchAppliedRecord()
		changedIDs.TransactionIDs = append([]uuid.UUID(nil), changedIDs.TransactionIDs...)
		changedIDs.TransactionIDs[0] = uuid.New()
		_, err = repository.HandoffAtomicTransactionBatchExecution(
			ctx, organizationID, ledgerID, effectiveKey, claim.OwnerToken, changedIDs,
		)
		require.ErrorContains(t, err, "ATOMIC_BATCH_IDEMPOTENCY_TRANSACTIONS_CHANGED")

		applied := atomicBatchAppliedRecord()
		transitioned, err = repository.HandoffAtomicTransactionBatchExecution(
			ctx, organizationID, ledgerID, effectiveKey, claim.OwnerToken, applied,
		)
		require.NoError(t, err)
		require.Equal(t, AtomicTransactionBatchTransitionUpdated, transitioned.Outcome)
		require.Equal(t, time.Duration(-1), container.Client.TTL(ctx, redisKey).Val())
		require.Equal(t, time.Duration(-1), container.Client.TTL(ctx, indexKey).Val())

		lookup, err := repository.GetAtomicTransactionBatchByExecutionID(
			ctx, organizationID, ledgerID, executionID,
		)
		require.NoError(t, err)
		require.NotNil(t, lookup)
		require.Equal(t, applied.TransactionIDs, lookup.Record.TransactionIDs)

		responses := map[uuid.UUID]json.RawMessage{
			applied.TransactionIDs[1]: json.RawMessage(`{"id":"second"}`),
			applied.TransactionIDs[0]: json.RawMessage(`{"id":"first"}`),
		}
		staleResult, err := repository.FinalizeAtomicTransactionBatch(
			ctx, organizationID, ledgerID, executionID, "stale-owner", responses, 60,
		)
		requireAtomicBatchFinalizationConflict(t, staleResult, err, AtomicTransactionBatchFinalizeStale)
		require.Equal(t, time.Duration(-1), container.Client.TTL(ctx, redisKey).Val())
		require.Equal(t, time.Duration(-1), container.Client.TTL(ctx, indexKey).Val())

		finalized, err := repository.FinalizeAtomicTransactionBatch(
			ctx, organizationID, ledgerID, executionID, claim.OwnerToken, responses, 60,
		)
		require.NoError(t, err)
		require.Equal(t, AtomicTransactionBatchFinalized, finalized.Outcome)
		require.Equal(
			t,
			`{"batchId":"00000000-0000-0000-0000-000000000010","transactions":[{"id":"first"},{"id":"second"}]}`,
			string(finalized.Response),
		)
		beforeRetry := container.Client.PTTL(ctx, redisKey).Val()
		require.Positive(t, beforeRetry)
		require.LessOrEqual(t, beforeRetry, 60*time.Second)
		indexBeforeRetry := container.Client.PTTL(ctx, indexKey).Val()
		require.Positive(t, indexBeforeRetry)

		finalized, err = repository.FinalizeAtomicTransactionBatch(
			ctx, organizationID, ledgerID, executionID, claim.OwnerToken, nil, 60,
		)
		require.NoError(t, err)
		require.Equal(t, AtomicTransactionBatchAlreadyComplete, finalized.Outcome)
		afterRetry := container.Client.PTTL(ctx, redisKey).Val()
		require.Positive(t, afterRetry)
		require.LessOrEqual(t, afterRetry, beforeRetry, "idempotent retry must not extend replay retention")
		indexAfterRetry := container.Client.PTTL(ctx, indexKey).Val()
		require.Positive(t, indexAfterRetry)
		require.LessOrEqual(t, indexAfterRetry, indexBeforeRetry, "idempotent retry must not extend index retention")

		replay, err := repository.ClaimAtomicTransactionBatch(ctx, organizationID, ledgerID, effectiveKey, claim)
		require.NoError(t, err)
		require.Equal(t, AtomicTransactionBatchReplayed, replay.Outcome)
		require.Equal(t, string(finalized.Response), string(replay.Record.Response), "replay bytes must be identical")
	})

	t.Run("cleanup checks owner state handoff and engine evidence", func(t *testing.T) {
		claimAndKey := func(t *testing.T, suffix string) (AtomicTransactionBatchIdempotencyRecord, string, string) {
			t.Helper()
			effectiveKey := "cleanup-" + suffix
			redisKey := transitionNamespacedBatchKey(t, ctx, organizationID, ledgerID, effectiveKey)
			t.Cleanup(func() { require.NoError(t, container.Client.Del(context.Background(), redisKey).Err()) })
			claim := atomicBatchIdempotencyClaim("b")
			claim.BatchID = uuid.New()
			claim.OwnerToken = uuid.NewString()
			_, err := repository.ClaimAtomicTransactionBatch(ctx, organizationID, ledgerID, effectiveKey, claim)
			require.NoError(t, err)

			return claim, effectiveKey, redisKey
		}

		claim, effectiveKey, redisKey := claimAndKey(t, "guards")
		result, err := repository.CleanupAbandonedAtomicTransactionBatch(
			ctx, organizationID, ledgerID, effectiveKey, "stale-owner", false,
		)
		requireAtomicBatchCleanupConflict(t, result, err, AtomicTransactionBatchDeleteStaleOwner)
		require.Equal(t, int64(1), container.Client.Exists(ctx, redisKey).Val())

		result, err = repository.CleanupAbandonedAtomicTransactionBatch(
			ctx, organizationID, ledgerID, effectiveKey, claim.OwnerToken, true,
		)
		requireAtomicBatchCleanupConflict(t, result, err, AtomicTransactionBatchDeleteProtected)
		require.Equal(t, int64(1), container.Client.Exists(ctx, redisKey).Val())

		result, err = repository.DeleteAtomicTransactionBatchPrePublication(
			ctx, organizationID, ledgerID, effectiveKey, claim.OwnerToken,
		)
		require.NoError(t, err)
		require.Equal(t, AtomicTransactionBatchDeleted, result.Outcome)
		require.Equal(t, int64(0), container.Client.Exists(ctx, redisKey).Val())

		claim, effectiveKey, redisKey = claimAndKey(t, "prepared-no-handoff")
		preparedWithoutHandoff := atomicBatchPreparedRecord(false)
		preparedWithoutHandoff.BatchID = claim.BatchID
		preparedWithoutHandoff.OwnerToken = claim.OwnerToken
		preparedWithoutHandoff.RequestFingerprint = claim.RequestFingerprint
		_, err = repository.TransitionAtomicTransactionBatch(
			ctx, organizationID, ledgerID, effectiveKey, claim.OwnerToken,
			AtomicTransactionBatchStateClaimed, preparedWithoutHandoff, 0,
		)
		require.NoError(t, err)
		result, err = repository.DeleteAtomicTransactionBatchPrePublication(
			ctx, organizationID, ledgerID, effectiveKey, claim.OwnerToken,
		)
		require.NoError(t, err)
		require.Equal(t, AtomicTransactionBatchDeleted, result.Outcome)
		require.Equal(t, int64(0), container.Client.Exists(ctx, redisKey).Val())

		claim, effectiveKey, redisKey = claimAndKey(t, "handoff")
		preparedForHandoff := atomicBatchPreparedRecord(false)
		preparedForHandoff.BatchID = claim.BatchID
		preparedForHandoff.OwnerToken = claim.OwnerToken
		preparedForHandoff.RequestFingerprint = claim.RequestFingerprint
		_, err = repository.TransitionAtomicTransactionBatch(
			ctx, organizationID, ledgerID, effectiveKey, claim.OwnerToken,
			AtomicTransactionBatchStateClaimed, preparedForHandoff, 0,
		)
		require.NoError(t, err)
		applied := preparedForHandoff
		applied.State = AtomicTransactionBatchStateApplied
		applied.ExecutionID = uuidPointer(uuid.New())
		indexKey := transitionNamespacedBatchExecutionIndexKey(
			t, ctx, organizationID, ledgerID, *applied.ExecutionID,
		)
		t.Cleanup(func() { require.NoError(t, container.Client.Del(context.Background(), indexKey).Err()) })
		_, err = repository.HandoffAtomicTransactionBatchExecution(
			ctx, organizationID, ledgerID, effectiveKey, claim.OwnerToken, applied,
		)
		require.NoError(t, err)

		result, err = repository.DeleteAtomicTransactionBatchPrePublication(
			ctx, organizationID, ledgerID, effectiveKey, claim.OwnerToken,
		)
		requireAtomicBatchCleanupConflict(t, result, err, AtomicTransactionBatchDeleteProtected)
		require.Equal(t, int64(1), container.Client.Exists(ctx, redisKey).Val())
	})

	t.Run("cleanup cannot delete after a racing execution handoff", func(t *testing.T) {
		for iteration := range 20 {
			effectiveKey := "handoff-race-" + uuid.NewString()
			redisKey := transitionNamespacedBatchKey(t, ctx, organizationID, ledgerID, effectiveKey)
			claim := atomicBatchIdempotencyClaim("c")
			claim.BatchID = uuid.New()
			claim.OwnerToken = uuid.NewString()
			_, err := repository.ClaimAtomicTransactionBatch(ctx, organizationID, ledgerID, effectiveKey, claim)
			require.NoError(t, err)

			prepared := atomicBatchPreparedRecord(false)
			prepared.BatchID = claim.BatchID
			prepared.OwnerToken = claim.OwnerToken
			prepared.RequestFingerprint = claim.RequestFingerprint
			_, err = repository.TransitionAtomicTransactionBatch(
				ctx, organizationID, ledgerID, effectiveKey, claim.OwnerToken,
				AtomicTransactionBatchStateClaimed, prepared, 0,
			)
			require.NoError(t, err)
			applied := prepared
			applied.State = AtomicTransactionBatchStateApplied
			applied.ExecutionID = uuidPointer(uuid.New())
			indexKey := transitionNamespacedBatchExecutionIndexKey(
				t, ctx, organizationID, ledgerID, *applied.ExecutionID,
			)

			start := make(chan struct{})
			var wait sync.WaitGroup
			wait.Add(2)
			var transitionResult *AtomicTransactionBatchTransitionResult
			var transitionErr error
			var deleteResult *AtomicTransactionBatchDeleteResult
			var deleteErr error
			go func() {
				defer wait.Done()
				<-start
				transitionResult, transitionErr = repository.HandoffAtomicTransactionBatchExecution(
					ctx, organizationID, ledgerID, effectiveKey, claim.OwnerToken, applied,
				)
			}()
			go func() {
				defer wait.Done()
				<-start
				deleteResult, deleteErr = repository.CleanupAbandonedAtomicTransactionBatch(
					ctx, organizationID, ledgerID, effectiveKey, claim.OwnerToken, false,
				)
			}()
			close(start)
			wait.Wait()

			transitionWon := transitionErr == nil && transitionResult.Outcome == AtomicTransactionBatchTransitionUpdated
			cleanupWon := deleteErr == nil && deleteResult.Outcome == AtomicTransactionBatchDeleted
			require.NotEqual(t, transitionWon, cleanupWon, "iteration %d must have exactly one winner", iteration)
			if transitionWon {
				requireAtomicBatchCleanupConflict(t, deleteResult, deleteErr, AtomicTransactionBatchDeleteProtected)
				require.Equal(t, int64(1), container.Client.Exists(ctx, redisKey).Val())
				require.Equal(t, int64(1), container.Client.Exists(ctx, indexKey).Val())
			} else {
				var conflict pkg.EntityConflictError
				require.ErrorAs(t, transitionErr, &conflict)
				require.Equal(t, AtomicTransactionBatchTransitionMissing, transitionResult.Outcome)
				require.Equal(t, int64(0), container.Client.Exists(ctx, redisKey).Val())
				require.Equal(t, int64(0), container.Client.Exists(ctx, indexKey).Val())
			}
			require.NoError(t, container.Client.Del(ctx, redisKey, indexKey).Err())
		}
	})
}

func transitionNamespacedBatchExecutionIndexKey(
	t *testing.T,
	ctx context.Context,
	organizationID, ledgerID, executionID uuid.UUID,
) string {
	t.Helper()

	key, err := tenantKeyFromContextOrError(
		ctx,
		utils.AtomicTransactionBatchExecutionIndexInternalKey(organizationID, ledgerID, executionID),
	)
	require.NoError(t, err)

	return key
}

func transitionNamespacedBatchKey(
	t *testing.T,
	ctx context.Context,
	organizationID, ledgerID uuid.UUID,
	effectiveKey string,
) string {
	t.Helper()

	key, err := tenantKeyFromContextOrError(
		ctx,
		utils.AtomicTransactionBatchIdempotencyInternalKey(organizationID, ledgerID, effectiveKey),
	)
	require.NoError(t, err)

	return key
}

func requireAtomicBatchCleanupConflict(
	t *testing.T,
	result *AtomicTransactionBatchDeleteResult,
	err error,
	want AtomicTransactionBatchDeleteOutcome,
) {
	t.Helper()

	var conflict pkg.EntityConflictError
	require.ErrorAs(t, err, &conflict)
	require.NotNil(t, result)
	require.Equal(t, want, result.Outcome)
}

func requireAtomicBatchFinalizationConflict(
	t *testing.T,
	result *AtomicTransactionBatchFinalizationResult,
	err error,
	want AtomicTransactionBatchFinalizationOutcome,
) {
	t.Helper()

	var conflict pkg.EntityConflictError
	require.ErrorAs(t, err, &conflict)
	require.NotNil(t, result)
	require.Equal(t, want, result.Outcome)
}
