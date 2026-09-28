//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"errors"
	"testing"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/internal/cachepolicy"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"
)

func TestIntegrationAbortAtomicTransactionBatchConfirmedRefusal(t *testing.T) {
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
	executionID := *atomicBatchAppliedRecord().ExecutionID
	receiptKey, err := tenantKeyFromContextOrError(
		ctx,
		atomicTransactionBatchEngineReceiptInternalKey(organizationID, ledgerID),
	)
	require.NoError(t, err)
	recoveryKey, err := tenantKeyFromContextOrError(ctx, cachepolicy.EngineRecoverQueue)
	require.NoError(t, err)

	prepareApplied := func(t *testing.T, suffix string) (string, string, AtomicTransactionBatchIdempotencyRecord) {
		t.Helper()

		effectiveKey := "confirmed-refusal-" + suffix
		recordKey := transitionNamespacedBatchKey(t, ctx, organizationID, ledgerID, effectiveKey)
		indexKey := transitionNamespacedBatchExecutionIndexKey(t, ctx, organizationID, ledgerID, executionID)
		require.NoError(t, container.Client.Del(ctx, recordKey, indexKey, receiptKey, recoveryKey).Err())

		claim := atomicBatchIdempotencyClaim("a")
		_, err := repository.ClaimAtomicTransactionBatch(ctx, organizationID, ledgerID, effectiveKey, claim)
		require.NoError(t, err)
		prepared := atomicBatchPreparedRecord(false)
		_, err = repository.TransitionAtomicTransactionBatch(
			ctx, organizationID, ledgerID, effectiveKey, claim.OwnerToken,
			AtomicTransactionBatchStateClaimed, prepared, 0,
		)
		require.NoError(t, err)
		applied := atomicBatchAppliedRecord()
		_, err = repository.HandoffAtomicTransactionBatchExecution(
			ctx, organizationID, ledgerID, effectiveKey, claim.OwnerToken, applied,
		)
		require.NoError(t, err)

		t.Cleanup(func() {
			require.NoError(t, container.Client.Del(context.Background(), recordKey, indexKey, receiptKey, recoveryKey).Err())
		})

		return recordKey, indexKey, applied
	}

	t.Run("deletes matching record and index only without evidence", func(t *testing.T) {
		recordKey, indexKey, applied := prepareApplied(t, "clean")

		result, err := repository.AbortAtomicTransactionBatchConfirmedRefusal(
			ctx, organizationID, ledgerID, "confirmed-refusal-clean", applied.OwnerToken,
			executionID, applied.TransactionIDs,
		)
		require.NoError(t, err)
		require.Equal(t, AtomicTransactionBatchRefusalDeleted, result.Outcome)
		require.Equal(t, int64(0), container.Client.Exists(ctx, recordKey, indexKey).Val())

		replayed, err := repository.AbortAtomicTransactionBatchConfirmedRefusal(
			ctx, organizationID, ledgerID, "confirmed-refusal-clean", applied.OwnerToken,
			executionID, applied.TransactionIDs,
		)
		require.NoError(t, err)
		require.Equal(t, AtomicTransactionBatchRefusalAlreadyDeleted, replayed.Outcome)
	})

	t.Run("receipt evidence retains state", func(t *testing.T) {
		recordKey, indexKey, applied := prepareApplied(t, "receipt")
		require.NoError(t, container.Client.HSet(ctx, receiptKey, executionID.String(), `{"receipt":true}`).Err())

		result, err := repository.AbortAtomicTransactionBatchConfirmedRefusal(
			ctx, organizationID, ledgerID, "confirmed-refusal-receipt", applied.OwnerToken,
			executionID, applied.TransactionIDs,
		)
		require.ErrorIs(t, err, ErrAtomicTransactionBatchRefusalProtected)
		require.Equal(t, AtomicTransactionBatchRefusalEngineEvidence, result.Outcome)
		require.Equal(t, int64(2), container.Client.Exists(ctx, recordKey, indexKey).Val())
	})

	t.Run("recovery evidence retains state", func(t *testing.T) {
		recordKey, indexKey, applied := prepareApplied(t, "recovery")
		field := applied.TransactionIDs[0].String() + ":" + executionID.String()
		require.NoError(t, container.Client.HSet(ctx, recoveryKey, field, `{"recovery":true}`).Err())

		result, err := repository.AbortAtomicTransactionBatchConfirmedRefusal(
			ctx, organizationID, ledgerID, "confirmed-refusal-recovery", applied.OwnerToken,
			executionID, applied.TransactionIDs,
		)
		require.ErrorIs(t, err, ErrAtomicTransactionBatchRefusalProtected)
		require.Equal(t, AtomicTransactionBatchRefusalEngineEvidence, result.Outcome)
		require.Equal(t, int64(2), container.Client.Exists(ctx, recordKey, indexKey).Val())
	})

	t.Run("stale owner and ambiguous key type retain state", func(t *testing.T) {
		recordKey, indexKey, applied := prepareApplied(t, "ambiguous")

		result, err := repository.AbortAtomicTransactionBatchConfirmedRefusal(
			ctx, organizationID, ledgerID, "confirmed-refusal-ambiguous", "stale-owner",
			executionID, applied.TransactionIDs,
		)
		require.ErrorIs(t, err, ErrAtomicTransactionBatchRefusalProtected)
		require.Equal(t, AtomicTransactionBatchRefusalStaleOwner, result.Outcome)

		require.NoError(t, container.Client.Set(ctx, receiptKey, "wrong-type", 0).Err())
		result, err = repository.AbortAtomicTransactionBatchConfirmedRefusal(
			ctx, organizationID, ledgerID, "confirmed-refusal-ambiguous", applied.OwnerToken,
			executionID, applied.TransactionIDs,
		)
		require.Nil(t, result)
		require.Error(t, err)
		require.False(t, errors.Is(err, ErrAtomicTransactionBatchRefusalProtected))
		require.Equal(t, int64(2), container.Client.Exists(ctx, recordKey, indexKey).Val())
	})
}
