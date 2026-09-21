// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/internal/cachepolicy"
)

func TestAbortAtomicTransactionBatchConfirmedRefusal_UsesOneEvidenceCAS(t *testing.T) {
	t.Parallel()

	organizationID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	ledgerID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	applied := atomicBatchAppliedRecord()
	payload, err := json.Marshal(applied)
	require.NoError(t, err)
	client := &atomicBatchClaimEvalClient{result: []any{
		string(AtomicTransactionBatchRefusalDeleted), string(payload),
	}}
	repository := &RedisConsumerRepository{conn: &staticRedisProvider{client: client}}

	result, err := repository.AbortAtomicTransactionBatchConfirmedRefusal(
		context.Background(),
		organizationID,
		ledgerID,
		"effective-key",
		applied.OwnerToken,
		*applied.ExecutionID,
		applied.TransactionIDs,
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, AtomicTransactionBatchRefusalDeleted, result.Outcome)
	assert.Equal(t, applied, result.Record)
	assert.Equal(t, abortAtomicTransactionBatchRefusalLua, client.capturedLua)
	require.Len(t, client.capturedKeys, 4)
	for _, key := range client.capturedKeys {
		assert.Equal(t, "transactions", redisHashTag(key), "all CAS keys must share the engine slot")
	}
	assert.Equal(t, atomicTransactionBatchEngineReceiptInternalKey(organizationID, ledgerID), client.capturedKeys[2])
	assert.Equal(t, cachepolicy.EngineRecoverQueue, client.capturedKeys[3])
	require.Len(t, client.capturedArgs, 3)
	assert.Equal(t, applied.OwnerToken, client.capturedArgs[0])
	assert.Equal(t, applied.ExecutionID.String(), client.capturedArgs[1])
	assert.JSONEq(t, string(mustJSONMarshal(t, applied.TransactionIDs)), client.capturedArgs[2].(string))
}

func TestAbortAtomicTransactionBatchConfirmedRefusal_ClassifiesProtectedOutcomes(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	applied := atomicBatchAppliedRecord()
	payload, err := json.Marshal(applied)
	require.NoError(t, err)

	for _, outcome := range []AtomicTransactionBatchRefusalAbortOutcome{
		AtomicTransactionBatchRefusalStaleOwner,
		AtomicTransactionBatchRefusalStateConflict,
		AtomicTransactionBatchRefusalExecutionConflict,
		AtomicTransactionBatchRefusalTransactionConflict,
		AtomicTransactionBatchRefusalIndexConflict,
		AtomicTransactionBatchRefusalEngineEvidence,
	} {
		t.Run(string(outcome), func(t *testing.T) {
			t.Parallel()

			client := &atomicBatchClaimEvalClient{result: []any{string(outcome), string(payload)}}
			repository := &RedisConsumerRepository{conn: &staticRedisProvider{client: client}}
			result, err := repository.AbortAtomicTransactionBatchConfirmedRefusal(
				context.Background(), organizationID, ledgerID, "key", applied.OwnerToken,
				*applied.ExecutionID, applied.TransactionIDs,
			)
			require.ErrorIs(t, err, ErrAtomicTransactionBatchRefusalProtected)
			require.NotNil(t, result)
			assert.Equal(t, outcome, result.Outcome)
		})
	}
}

func TestAbortAtomicTransactionBatchConfirmedRefusal_IsIdempotentAfterLostReply(t *testing.T) {
	t.Parallel()

	applied := atomicBatchAppliedRecord()
	client := &atomicBatchClaimEvalClient{result: []any{
		string(AtomicTransactionBatchRefusalAlreadyDeleted), "",
	}}
	repository := &RedisConsumerRepository{conn: &staticRedisProvider{client: client}}

	result, err := repository.AbortAtomicTransactionBatchConfirmedRefusal(
		context.Background(), uuid.New(), uuid.New(), "key", applied.OwnerToken,
		*applied.ExecutionID, applied.TransactionIDs,
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, AtomicTransactionBatchRefusalAlreadyDeleted, result.Outcome)
}

func TestAbortAtomicTransactionBatchConfirmedRefusal_ValidatesBeforeRedis(t *testing.T) {
	t.Parallel()

	client := &atomicBatchClaimEvalClient{}
	repository := &RedisConsumerRepository{conn: &staticRedisProvider{client: client}}
	result, err := repository.AbortAtomicTransactionBatchConfirmedRefusal(
		context.Background(), uuid.Nil, uuid.New(), "", "", uuid.Nil, nil,
	)
	assert.Nil(t, result)
	assert.Error(t, err)
	assert.Empty(t, client.capturedLua)
}

func mustJSONMarshal(t *testing.T, value any) []byte {
	t.Helper()

	payload, err := json.Marshal(value)
	require.NoError(t, err)

	return payload
}
