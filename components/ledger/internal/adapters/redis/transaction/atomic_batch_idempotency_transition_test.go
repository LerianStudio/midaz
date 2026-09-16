// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

func TestTransitionAtomicTransactionBatch_WiresStateAndTerminalTTL(t *testing.T) {
	t.Parallel()

	organizationID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	ledgerID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	prepared := atomicBatchPreparedRecord(false)
	applied := atomicBatchAppliedRecord()
	complete := atomicBatchIdempotencyComplete("a")

	tests := []struct {
		name          string
		expectedState AtomicTransactionBatchIdempotencyState
		next          AtomicTransactionBatchIdempotencyRecord
		ttl           time.Duration
	}{
		{
			name:          "claimed to prepared has no TTL",
			expectedState: AtomicTransactionBatchStateClaimed,
			next:          prepared,
		},
		{
			name:          "prepared to applied has no TTL",
			expectedState: AtomicTransactionBatchStatePrepared,
			next:          applied,
		},
		{
			name:          "applied to complete starts replay TTL",
			expectedState: AtomicTransactionBatchStateApplied,
			next:          complete,
			ttl:           300,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			payload, err := json.Marshal(tt.next)
			require.NoError(t, err)
			client := &atomicBatchClaimEvalClient{result: []any{
				string(AtomicTransactionBatchTransitionUpdated), string(payload),
			}}
			repository := &RedisConsumerRepository{conn: &staticRedisProvider{client: client}}

			result, err := repository.TransitionAtomicTransactionBatch(
				context.Background(),
				organizationID,
				ledgerID,
				"effective-key",
				tt.next.OwnerToken,
				tt.expectedState,
				tt.next,
				tt.ttl,
			)
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, AtomicTransactionBatchTransitionUpdated, result.Outcome)
			assert.Equal(t, tt.next, result.Record)
			assert.Equal(t, transitionAtomicTransactionBatchLua, client.capturedLua)
			require.Len(t, client.capturedArgs, 4)
			assert.Equal(t, tt.next.OwnerToken, client.capturedArgs[0])
			assert.Equal(t, string(tt.expectedState), client.capturedArgs[1])
			assert.Equal(t, timeDurationSecondsString(tt.ttl), client.capturedArgs[3])
		})
	}
}

func TestTransitionAtomicTransactionBatch_RejectsInvalidTTLAndOwnerBeforeRedis(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	client := &atomicBatchClaimEvalClient{}
	repository := &RedisConsumerRepository{conn: &staticRedisProvider{client: client}}

	prepared := atomicBatchPreparedRecord(false)
	result, err := repository.TransitionAtomicTransactionBatch(
		context.Background(), organizationID, ledgerID, "key", prepared.OwnerToken,
		AtomicTransactionBatchStateClaimed, prepared, 1,
	)
	assert.Nil(t, result)
	assert.ErrorContains(t, err, "nonterminal")

	complete := atomicBatchIdempotencyComplete("a")
	result, err = repository.TransitionAtomicTransactionBatch(
		context.Background(), organizationID, ledgerID, "key", complete.OwnerToken,
		AtomicTransactionBatchStateApplied, complete, 0,
	)
	assert.Nil(t, result)
	assert.ErrorContains(t, err, "positive replay TTL")

	result, err = repository.TransitionAtomicTransactionBatch(
		context.Background(), organizationID, ledgerID, "key", "stale-owner",
		AtomicTransactionBatchStateClaimed, prepared, 0,
	)
	assert.Nil(t, result)
	assert.ErrorContains(t, err, "owner token is invalid")
	assert.Empty(t, client.capturedLua)
}

func TestTransitionAtomicTransactionBatch_ClassifiesCASConflicts(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	prepared := atomicBatchPreparedRecord(false)
	payload, err := json.Marshal(prepared)
	require.NoError(t, err)

	for _, tt := range []struct {
		outcome AtomicTransactionBatchTransitionOutcome
		payload string
		wantErr bool
	}{
		{outcome: AtomicTransactionBatchTransitionUpdated, payload: string(payload)},
		{outcome: AtomicTransactionBatchAlreadyTransitioned, payload: string(payload)},
		{outcome: AtomicTransactionBatchTransitionMissing, wantErr: true},
		{outcome: AtomicTransactionBatchTransitionStaleOwner, payload: string(payload), wantErr: true},
		{outcome: AtomicTransactionBatchTransitionStateConflict, payload: string(payload), wantErr: true},
	} {
		client := &atomicBatchClaimEvalClient{result: []any{string(tt.outcome), tt.payload}}
		repository := &RedisConsumerRepository{conn: &staticRedisProvider{client: client}}

		result, err := repository.TransitionAtomicTransactionBatch(
			context.Background(), organizationID, ledgerID, "key", prepared.OwnerToken,
			AtomicTransactionBatchStateClaimed, prepared, 0,
		)
		require.NotNil(t, result)
		assert.Equal(t, tt.outcome, result.Outcome)
		if tt.wantErr {
			var conflict pkg.EntityConflictError
			require.ErrorAs(t, err, &conflict)
			assert.Equal(t, constant.ErrIdempotencyKey.Error(), conflict.Code)
		} else {
			require.NoError(t, err)
		}
	}
}

func TestDeleteAtomicTransactionBatch_ClassifiesOwnershipAndProtection(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	claim := atomicBatchIdempotencyClaim("a")
	payload, err := json.Marshal(claim)
	require.NoError(t, err)

	for _, tt := range []struct {
		name           string
		outcome        AtomicTransactionBatchDeleteOutcome
		payload        string
		engineEvidence bool
		wantErr        bool
		wantEvidence   string
	}{
		{name: "deleted", outcome: AtomicTransactionBatchDeleted, payload: string(payload), wantEvidence: "0"},
		{name: "missing", outcome: AtomicTransactionBatchDeleteMissing, wantEvidence: "0"},
		{name: "stale owner", outcome: AtomicTransactionBatchDeleteStaleOwner, payload: string(payload), wantErr: true, wantEvidence: "0"},
		{name: "execution protected", outcome: AtomicTransactionBatchDeleteProtected, payload: string(payload), wantErr: true, wantEvidence: "0"},
		{name: "engine evidence protected", outcome: AtomicTransactionBatchDeleteProtected, payload: string(payload), engineEvidence: true, wantErr: true, wantEvidence: "1"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := &atomicBatchClaimEvalClient{result: []any{string(tt.outcome), tt.payload}}
			repository := &RedisConsumerRepository{conn: &staticRedisProvider{client: client}}
			result, err := repository.CleanupAbandonedAtomicTransactionBatch(
				context.Background(), organizationID, ledgerID, "key", claim.OwnerToken, tt.engineEvidence,
			)
			require.NotNil(t, result)
			assert.Equal(t, tt.outcome, result.Outcome)
			if tt.wantErr {
				var conflict pkg.EntityConflictError
				require.ErrorAs(t, err, &conflict)
			} else {
				require.NoError(t, err)
			}
			assert.Equal(t, deleteAtomicTransactionBatchLua, client.capturedLua)
			require.Len(t, client.capturedArgs, 2)
			assert.Equal(t, tt.wantEvidence, client.capturedArgs[1])
		})
	}
}

func atomicBatchPreparedRecord(withExecutionID bool) AtomicTransactionBatchIdempotencyRecord {
	record := atomicBatchIdempotencyClaim("a")
	record.State = AtomicTransactionBatchStatePrepared
	record.TransactionIDs = atomicBatchTransactionIDs()
	if withExecutionID {
		record.ExecutionID = uuidPointer(uuid.MustParse("00000000-0000-0000-0000-000000000020"))
	}

	return record
}

func atomicBatchAppliedRecord() AtomicTransactionBatchIdempotencyRecord {
	record := atomicBatchPreparedRecord(true)
	record.State = AtomicTransactionBatchStateApplied

	return record
}

func timeDurationSecondsString(value time.Duration) string {
	return strconv.FormatInt(int64(value), 10)
}
