// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

func TestCaptureAtomicTransactionBatchInitialResponse_UsesExecutionScopedCAS(t *testing.T) {
	t.Parallel()

	organizationID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	ledgerID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	record := atomicBatchAppliedRecord()
	record.FormatVersion = AtomicTransactionBatchLegacyFormatVersion
	executionID := *record.ExecutionID
	transactionID := record.TransactionIDs[1]
	recordKey := utils.AtomicTransactionBatchIdempotencyInternalKey(organizationID, ledgerID, "effective-key")
	indexKey := utils.AtomicTransactionBatchExecutionIndexInternalKey(organizationID, ledgerID, executionID)
	stored, err := json.Marshal(record)
	require.NoError(t, err)

	captured := record
	captured.FormatVersion = AtomicTransactionBatchIdempotencyFormatVersion
	captured.InitialResponses = map[string]string{
		transactionID.String(): base64.StdEncoding.EncodeToString([]byte(`{"id":"second","status":"CREATED"}`)),
	}
	capturedPayload, err := json.Marshal(captured)
	require.NoError(t, err)

	client := &atomicBatchClaimEvalClient{
		getValues: map[string]string{indexKey: recordKey, recordKey: string(stored)},
		result:    []any{string(AtomicTransactionBatchInitialResponseCaptured), string(capturedPayload)},
	}
	repository := &RedisConsumerRepository{conn: &staticRedisProvider{client: client}}
	response := json.RawMessage(`{"id":"second","status":"CREATED"}`)

	result, err := repository.CaptureAtomicTransactionBatchInitialResponse(
		context.Background(), organizationID, ledgerID, executionID, record.OwnerToken, transactionID, response,
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, AtomicTransactionBatchInitialResponseCaptured, result.Outcome)
	assert.Equal(t, captured.InitialResponses, result.Record.InitialResponses)
	assert.Equal(t, captureAtomicTransactionBatchInitialResponseLua, client.capturedLua)
	assert.Equal(t, []string{recordKey, indexKey}, client.capturedKeys)
	require.Len(t, client.capturedArgs, 6)
	assert.Equal(t, record.OwnerToken, client.capturedArgs[0])
	assert.Equal(t, executionID.String(), client.capturedArgs[1])
	assert.Equal(t, transactionID.String(), client.capturedArgs[2])
	assert.Equal(t, base64.StdEncoding.EncodeToString(response), client.capturedArgs[3])
	assert.NotContains(t, string(capturedPayload), `"order"`)
}

func TestAtomicTransactionBatchCapturedResponses_RequiresEveryMemberAndPreservesBytes(t *testing.T) {
	t.Parallel()

	record := atomicBatchAppliedRecord()
	record.FormatVersion = AtomicTransactionBatchIdempotencyFormatVersion
	record.InitialResponses = atomicBatchInitialResponses(record.TransactionIDs)

	responses, err := atomicTransactionBatchCapturedResponses(record)
	require.NoError(t, err)
	assert.Equal(t, []byte(`{"id":"initial-0"}`), []byte(responses[record.TransactionIDs[0]]))
	assert.Equal(t, []byte(`{"id":"initial-1"}`), []byte(responses[record.TransactionIDs[1]]))

	delete(record.InitialResponses, record.TransactionIDs[1].String())
	_, err = atomicTransactionBatchCapturedResponses(record)
	assert.ErrorContains(t, err, "requires 2 captured")
}
