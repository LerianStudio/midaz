// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package redis

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
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

	client.getValues[indexKey] = "idempotency_atomic_batch:{transactions}:other:scope:" + string(make([]byte, 64))
	result, err = repository.GetAtomicTransactionBatchByExecutionID(
		context.Background(), organizationID, ledgerID, executionID,
	)
	assert.Nil(t, result)
	assert.ErrorContains(t, err, "invalid record pointer")
}

func TestBuildAtomicTransactionBatchResponse_UsesStoredOrder(t *testing.T) {
	t.Parallel()

	record := atomicBatchAppliedRecord()
	record.FormatVersion = AtomicTransactionBatchLegacyFormatVersion
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
		`{"transactions":[{"id":"first"},{"id":"second"}]}`,
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
	applied.FormatVersion = AtomicTransactionBatchLegacyFormatVersion
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
	assert.Equal(t, []string{
		recordKey,
		indexKey,
		atomicTransactionBatchEngineReceiptInternalKey(organizationID, ledgerID),
	}, client.capturedKeys)
	require.Len(t, client.capturedArgs, 6)
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

func TestGetAtomicTransactionBatchFinalizationCandidate_RequiresReceiptTenantOfTheRecoveryScope(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct {
		name                  string
		contextTenantID       string
		receiptTenantID       string
		protectionFormat      int
		wantInvalidReceiptErr bool
	}{
		{name: "single-tenant receipt without tenant", protectionFormat: 2},
		{name: "tenant receipt in its own tenant", contextTenantID: "tenant-a", receiptTenantID: "tenant-a", protectionFormat: 2},
		{name: "receipt of another tenant", contextTenantID: "tenant-a", receiptTenantID: "tenant-b", protectionFormat: 2, wantInvalidReceiptErr: true},
		{name: "receipt without tenant in a tenant scope", contextTenantID: "tenant-a", protectionFormat: 2, wantInvalidReceiptErr: true},
		{name: "tenant receipt in the single-tenant scope", receiptTenantID: "tenant-a", protectionFormat: 2, wantInvalidReceiptErr: true},
		{name: "format-one receipt carries no tenant", contextTenantID: "tenant-a", protectionFormat: 1},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctx := context.Background()
			if testCase.contextTenantID != "" {
				ctx = tmcore.ContextWithTenantID(ctx, testCase.contextTenantID)
			}

			organizationID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
			ledgerID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
			record := atomicBatchAppliedRecord()
			executionID := *record.ExecutionID
			currentID, siblingID := record.TransactionIDs[0], record.TransactionIDs[1]

			protection := atomicTransactionBatchReceiptProtection{
				FormatVersion:    testCase.protectionFormat,
				RetentionSeconds: 37,
				Transactions:     append([]uuid.UUID(nil), record.TransactionIDs...),
				RecoveryFields: []string{
					currentID.String() + ":" + executionID.String(),
					siblingID.String() + ":" + executionID.String(),
				},
				Acknowledged:          map[string]bool{currentID.String(): false, siblingID.String(): true},
				TerminalCompletedAtMS: map[string]int64{},
			}
			if testCase.protectionFormat == 2 {
				protection.IndexFields = append([]uuid.UUID(nil), record.TransactionIDs...)
			}

			receiptPayload, err := json.Marshal(atomicTransactionBatchReceipt{
				FormatVersion:     1,
				TenantID:          testCase.receiptTenantID,
				OrganizationID:    organizationID,
				LedgerID:          ledgerID,
				ExecutionID:       executionID,
				IntentFingerprint: "engine-intent-fingerprint",
				Protection:        protection,
			})
			require.NoError(t, err)
			recordPayload, err := json.Marshal(record)
			require.NoError(t, err)

			indexKey, err := tenantKeyFromContextOrError(ctx,
				utils.AtomicTransactionBatchExecutionIndexInternalKey(organizationID, ledgerID, executionID))
			require.NoError(t, err)
			recordKey, err := tenantKeyFromContextOrError(ctx,
				utils.AtomicTransactionBatchIdempotencyInternalKey(organizationID, ledgerID, "effective-key"))
			require.NoError(t, err)
			receiptKey, err := tenantKeyFromContextOrError(ctx,
				atomicTransactionBatchEngineReceiptInternalKey(organizationID, ledgerID))
			require.NoError(t, err)

			client := &atomicBatchClaimEvalClient{
				getValues:  map[string]string{indexKey: recordKey, recordKey: string(recordPayload)},
				hashValues: map[string]map[string]string{receiptKey: {executionID.String(): string(receiptPayload)}},
			}
			repository := &RedisConsumerRepository{conn: &staticRedisProvider{client: client}}

			result, err := repository.GetAtomicTransactionBatchFinalizationCandidate(
				ctx, organizationID, ledgerID, executionID, currentID,
			)

			if testCase.wantInvalidReceiptErr {
				assert.Nil(t, result)
				assert.EqualError(t, err, "atomic transaction batch execution receipt is invalid")

				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.True(t, result.Candidate, "the last unacknowledged member finalizes the batch")
			assert.Equal(t, string(receiptPayload), result.ReceiptToken)
		})
	}
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
