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
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"
)

func TestIntegrationAtomicTransactionBatchIdempotencyClaim(t *testing.T) {
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

	t.Run("one concurrent caller wins the atomic first claim", func(t *testing.T) {
		effectiveKey := "concurrent-first-claim"
		redisKey := atomicBatchNamespacedIdempotencyKey(t, ctx, organizationID, ledgerID, effectiveKey)
		t.Cleanup(func() { require.NoError(t, container.Client.Del(context.Background(), redisKey).Err()) })

		const callers = 16
		type outcome struct {
			result *AtomicTransactionBatchClaimResult
			err    error
		}
		outcomes := make(chan outcome, callers)
		var ready sync.WaitGroup
		ready.Add(callers)
		start := make(chan struct{})

		for range callers {
			go func() {
				claim := atomicBatchIdempotencyClaim("a")
				claim.BatchID = uuid.New()
				claim.OwnerToken = uuid.NewString()
				ready.Done()
				<-start
				result, err := repository.ClaimAtomicTransactionBatch(
					ctx, organizationID, ledgerID, effectiveKey, claim,
				)
				outcomes <- outcome{result: result, err: err}
			}()
		}

		ready.Wait()
		close(start)

		claimed := 0
		inProgress := 0
		var winner AtomicTransactionBatchIdempotencyRecord
		for range callers {
			observed := <-outcomes
			require.NotNil(t, observed.result)
			switch observed.result.Outcome {
			case AtomicTransactionBatchClaimed:
				require.NoError(t, observed.err)
				claimed++
				winner = observed.result.Record
			case AtomicTransactionBatchInProgress:
				var conflict pkg.EntityConflictError
				require.ErrorAs(t, observed.err, &conflict)
				require.Equal(t, constant.ErrIdempotencyKey.Error(), conflict.Code)
				inProgress++
			default:
				t.Fatalf("unexpected outcome %q", observed.result.Outcome)
			}
		}

		require.Equal(t, 1, claimed)
		require.Equal(t, callers-1, inProgress)
		require.Equal(t, time.Duration(-1), container.Client.TTL(ctx, redisKey).Val(), "claimed record must not expire")

		storedJSON, err := container.Client.Get(ctx, redisKey).Bytes()
		require.NoError(t, err)
		var stored AtomicTransactionBatchIdempotencyRecord
		require.NoError(t, json.Unmarshal(storedJSON, &stored))
		require.Equal(t, winner, stored)
	})

	t.Run("complete replays and a changed fingerprint conflicts", func(t *testing.T) {
		effectiveKey := "completed-replay"
		redisKey := atomicBatchNamespacedIdempotencyKey(t, ctx, organizationID, ledgerID, effectiveKey)
		t.Cleanup(func() { require.NoError(t, container.Client.Del(context.Background(), redisKey).Err()) })

		complete := atomicBatchIdempotencyComplete("c")
		complete.Response = json.RawMessage(`{"batchId":"00000000-0000-0000-0000-000000000010","transactions":[{"id":"one"},{"id":"two"}]}`)
		payload, err := json.Marshal(complete)
		require.NoError(t, err)
		require.NoError(t, container.Client.Set(ctx, redisKey, payload, 0).Err())

		replayClaim := atomicBatchIdempotencyClaim("c")
		replay, err := repository.ClaimAtomicTransactionBatch(
			ctx, organizationID, ledgerID, effectiveKey, replayClaim,
		)
		require.NoError(t, err)
		require.Equal(t, AtomicTransactionBatchReplayed, replay.Outcome)
		require.Equal(t, complete.BatchID, replay.Record.BatchID)
		require.JSONEq(t, string(complete.Response), string(replay.Record.Response))

		changedClaim := atomicBatchIdempotencyClaim("d")
		conflictResult, err := repository.ClaimAtomicTransactionBatch(
			ctx, organizationID, ledgerID, effectiveKey, changedClaim,
		)
		var conflict pkg.EntityConflictError
		require.ErrorAs(t, err, &conflict)
		require.Equal(t, constant.ErrIdempotencyKey.Error(), conflict.Code)
		require.Equal(t, AtomicTransactionBatchFingerprintConflict, conflictResult.Outcome)

		unchanged, err := container.Client.Get(ctx, redisKey).Bytes()
		require.NoError(t, err)
		require.Equal(t, payload, unchanged)
	})

	t.Run("batch namespace cannot collide with singular idempotency", func(t *testing.T) {
		effectiveKey := "shared-client-key"
		batchKey := atomicBatchNamespacedIdempotencyKey(t, ctx, organizationID, ledgerID, effectiveKey)
		singularKey, err := tenantKeyFromContextOrError(ctx, utils.IdempotencyInternalKey(organizationID, ledgerID, effectiveKey))
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, container.Client.Del(context.Background(), batchKey, singularKey).Err()) })

		require.NotEqual(t, singularKey, batchKey)
		require.NoError(t, container.Client.Set(ctx, singularKey, `{"singular":true}`, 0).Err())

		claim := atomicBatchIdempotencyClaim("e")
		result, err := repository.ClaimAtomicTransactionBatch(
			ctx, organizationID, ledgerID, effectiveKey, claim,
		)
		require.NoError(t, err)
		require.Equal(t, AtomicTransactionBatchClaimed, result.Outcome)
		require.Equal(t, `{"singular":true}`, container.Client.Get(ctx, singularKey).Val())
		require.NotEmpty(t, container.Client.Get(ctx, batchKey).Val())
	})
}

func atomicBatchNamespacedIdempotencyKey(
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
