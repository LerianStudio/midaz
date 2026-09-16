// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

func TestHandoffAtomicTransactionBatchExecution_WritesRecordAndIndexTogether(t *testing.T) {
	t.Parallel()

	organizationID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	ledgerID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	applied := atomicBatchAppliedRecord()
	payload, err := json.Marshal(applied)
	require.NoError(t, err)
	client := &atomicBatchClaimEvalClient{result: []any{
		string(AtomicTransactionBatchTransitionUpdated), string(payload),
	}}
	repository := &RedisConsumerRepository{conn: &staticRedisProvider{client: client}}

	result, err := repository.HandoffAtomicTransactionBatchExecution(
		context.Background(), organizationID, ledgerID, "effective-key", applied.OwnerToken, applied,
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, AtomicTransactionBatchTransitionUpdated, result.Outcome)
	assert.Equal(t, handoffAtomicTransactionBatchLua, client.capturedLua)
	require.Len(t, client.capturedKeys, 2)
	assert.Equal(
		t,
		utils.AtomicTransactionBatchIdempotencyInternalKey(organizationID, ledgerID, "effective-key"),
		client.capturedKeys[0],
	)
	assert.Equal(
		t,
		utils.AtomicTransactionBatchExecutionIndexInternalKey(organizationID, ledgerID, *applied.ExecutionID),
		client.capturedKeys[1],
	)
	assert.Equal(t, redisHashTag(client.capturedKeys[0]), redisHashTag(client.capturedKeys[1]))
	require.Len(t, client.capturedArgs, 3)
	assert.Equal(t, applied.ExecutionID.String(), client.capturedArgs[2])
}

func TestGetAtomicTransactionBatchByExecutionID_ValidatesScopedPointer(t *testing.T) {
	t.Parallel()

	organizationID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	ledgerID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	applied := atomicBatchAppliedRecord()
	executionID := *applied.ExecutionID
	indexKey := utils.AtomicTransactionBatchExecutionIndexInternalKey(organizationID, ledgerID, executionID)
	recordKey := utils.AtomicTransactionBatchIdempotencyInternalKey(organizationID, ledgerID, "effective-key")
	payload, err := json.Marshal(applied)
	require.NoError(t, err)

	client := &atomicBatchClaimEvalClient{getValues: map[string]string{
		indexKey:  recordKey,
		recordKey: string(payload),
	}}
	repository := &RedisConsumerRepository{conn: &staticRedisProvider{client: client}}

	result, err := repository.GetAtomicTransactionBatchByExecutionID(
		context.Background(), organizationID, ledgerID, executionID,
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, applied, result.Record)

	client.getValues[indexKey] = "idempotency_atomic_batch:{other:scope}:" + string(make([]byte, 64))
	result, err = repository.GetAtomicTransactionBatchByExecutionID(
		context.Background(), organizationID, ledgerID, executionID,
	)
	assert.Nil(t, result)
	assert.ErrorContains(t, err, "invalid record pointer")
}

func TestBuildAtomicTransactionBatchResponse_UsesStoredOrder(t *testing.T) {
	t.Parallel()

	record := atomicBatchAppliedRecord()
	firstID := record.TransactionIDs[0]
	secondID := record.TransactionIDs[1]
	responses := map[uuid.UUID]json.RawMessage{
		secondID: json.RawMessage(`{"id":"second"}`),
		firstID:  json.RawMessage(`{"id":"first"}`),
	}

	response, err := buildAtomicTransactionBatchResponse(record, responses)
	require.NoError(t, err)
	assert.Equal(
		t,
		`{"batchId":"00000000-0000-0000-0000-000000000010","transactions":[{"id":"first"},{"id":"second"}]}`,
		string(response),
	)
	assert.NotContains(t, string(responses[firstID]), "batchId")
	assert.NotContains(t, string(responses[secondID]), "batchId")
}

func TestFinalizeAtomicTransactionBatch_StoresOrderedResponseAndClassifiesStaleOwner(t *testing.T) {
	t.Parallel()

	organizationID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	ledgerID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	applied := atomicBatchAppliedRecord()
	executionID := *applied.ExecutionID
	indexKey := utils.AtomicTransactionBatchExecutionIndexInternalKey(organizationID, ledgerID, executionID)
	recordKey := utils.AtomicTransactionBatchIdempotencyInternalKey(organizationID, ledgerID, "effective-key")
	appliedPayload, err := json.Marshal(applied)
	require.NoError(t, err)
	responses := map[uuid.UUID]json.RawMessage{
		applied.TransactionIDs[1]: json.RawMessage(`{"id":"second"}`),
		applied.TransactionIDs[0]: json.RawMessage(`{"id":"first"}`),
	}
	expectedResponse, err := buildAtomicTransactionBatchResponse(applied, responses)
	require.NoError(t, err)
	complete := applied
	complete.State = AtomicTransactionBatchStateComplete
	complete.Response = expectedResponse
	completePayload, err := json.Marshal(complete)
	require.NoError(t, err)

	client := &atomicBatchClaimEvalClient{
		getValues: map[string]string{indexKey: recordKey, recordKey: string(appliedPayload)},
		result:    []any{string(AtomicTransactionBatchFinalized), string(completePayload)},
	}
	repository := &RedisConsumerRepository{conn: &staticRedisProvider{client: client}}

	result, err := repository.FinalizeAtomicTransactionBatch(
		context.Background(), organizationID, ledgerID, executionID,
		applied.OwnerToken, responses, 300,
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, AtomicTransactionBatchFinalized, result.Outcome)
	assert.Equal(t, string(expectedResponse), string(result.Response))
	assert.Equal(t, finalizeAtomicTransactionBatchLua, client.capturedLua)
	assert.Equal(t, []string{recordKey, indexKey}, client.capturedKeys)
	require.Len(t, client.capturedArgs, 4)
	assert.Equal(t, "300", client.capturedArgs[3])

	client.result = []any{string(AtomicTransactionBatchFinalizeStale), string(appliedPayload)}
	result, err = repository.FinalizeAtomicTransactionBatch(
		context.Background(), organizationID, ledgerID, executionID,
		"stale-owner", responses, 300,
	)
	require.NotNil(t, result)
	assert.Equal(t, AtomicTransactionBatchFinalizeStale, result.Outcome)
	var conflict pkg.EntityConflictError
	require.ErrorAs(t, err, &conflict)
}

func TestFinalizeAtomicTransactionBatch_CompleteRetryReturnsStoredBytes(t *testing.T) {
	t.Parallel()

	organizationID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	ledgerID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	complete := atomicBatchIdempotencyComplete("a")
	complete.Response = json.RawMessage(`{"batchId":"00000000-0000-0000-0000-000000000010","transactions":[{"id":"first"},{"id":"second"}]}`)
	executionID := *complete.ExecutionID
	indexKey := utils.AtomicTransactionBatchExecutionIndexInternalKey(organizationID, ledgerID, executionID)
	recordKey := utils.AtomicTransactionBatchIdempotencyInternalKey(organizationID, ledgerID, "effective-key")
	payload, err := json.Marshal(complete)
	require.NoError(t, err)
	client := &atomicBatchClaimEvalClient{
		getValues: map[string]string{indexKey: recordKey, recordKey: string(payload)},
		result:    []any{string(AtomicTransactionBatchAlreadyComplete), string(payload)},
	}
	repository := &RedisConsumerRepository{conn: &staticRedisProvider{client: client}}

	result, err := repository.FinalizeAtomicTransactionBatch(
		context.Background(), organizationID, ledgerID, executionID,
		complete.OwnerToken, nil, time.Duration(300),
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, AtomicTransactionBatchAlreadyComplete, result.Outcome)
	assert.Equal(t, string(complete.Response), string(result.Response))
}

func redisHashTag(key string) string {
	start := -1
	for index, character := range key {
		if character == '{' {
			start = index + 1
		}
		if character == '}' && start >= 0 {
			return key[start:index]
		}
	}

	return ""
}
