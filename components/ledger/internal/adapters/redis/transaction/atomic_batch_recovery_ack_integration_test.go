//go:build integration

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
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/internal/cachepolicy"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"
)

type atomicBatchRecoveryAckFixture struct {
	ctx            context.Context
	repository     *RedisConsumerRepository
	organizationID uuid.UUID
	ledgerID       uuid.UUID
	executionID    uuid.UUID
	record         AtomicTransactionBatchIdempotencyRecord
	recordKey      string
	indexKey       string
	receiptKey     string
	queueKey       string
	fields         []string
	payloads       []string
}

func TestIntegrationAtomicTransactionBatchFinalizationUsesReceiptRetention(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	fixture := newAtomicBatchRecoveryAckFixture(t, container.Client)
	result, err := fixture.repository.FinalizeAtomicTransactionBatch(
		fixture.ctx,
		fixture.organizationID,
		fixture.ledgerID,
		fixture.executionID,
		fixture.record.OwnerToken,
		map[uuid.UUID]json.RawMessage{
			fixture.record.TransactionIDs[1]: json.RawMessage(`{"id":"second"}`),
			fixture.record.TransactionIDs[0]: json.RawMessage(`{"id":"first"}`),
		},
		0,
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, AtomicTransactionBatchFinalized, result.Outcome)
	assert.JSONEq(
		t,
		`{"batchId":"00000000-0000-0000-0000-000000000010","transactions":[{"id":"first"},{"id":"second"}]}`,
		string(result.Response),
	)
	recordTTL := container.Client.TTL(fixture.ctx, fixture.recordKey).Val()
	indexTTL := container.Client.TTL(fixture.ctx, fixture.indexKey).Val()
	assert.Greater(t, recordTTL, 30*time.Second)
	assert.LessOrEqual(t, recordTTL, 37*time.Second)
	assert.Greater(t, indexTTL, 30*time.Second)
	assert.LessOrEqual(t, indexTTL, 37*time.Second)
}

func TestIntegrationAtomicTransactionBatchRecoveryAckFinalizesBeforeLastDelete(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	fixture := newAtomicBatchRecoveryAckFixture(t, container.Client)
	completedAt := time.Date(2026, time.September, 16, 12, 0, 0, 0, time.UTC)

	status, err := fixture.repository.CompareAndDeleteAtomicTransactionBatchRecoveryWithProtectionFrom(
		fixture.ctx,
		RecoveryQueueSourceEngineRecover,
		fixture.organizationID,
		fixture.ledgerID,
		fixture.fields[0],
		fixture.payloads[0],
		true,
		completedAt,
		"",
		nil,
	)
	require.NoError(t, err)
	assert.Equal(t, RecoveryAckDeleted, status)
	assert.False(t, container.Client.HExists(fixture.ctx, fixture.queueKey, fixture.fields[0]).Val())
	assert.True(t, container.Client.HExists(fixture.ctx, fixture.queueKey, fixture.fields[1]).Val())

	candidate, err := fixture.repository.GetAtomicTransactionBatchFinalizationCandidate(
		fixture.ctx,
		fixture.organizationID,
		fixture.ledgerID,
		fixture.executionID,
		fixture.record.TransactionIDs[1],
	)
	require.NoError(t, err)
	require.NotNil(t, candidate)
	assert.True(t, candidate.Candidate)
	assert.NotEmpty(t, candidate.ReceiptToken)

	status, err = fixture.repository.CompareAndDeleteAtomicTransactionBatchRecoveryWithProtectionFrom(
		fixture.ctx,
		RecoveryQueueSourceEngineRecover,
		fixture.organizationID,
		fixture.ledgerID,
		fixture.fields[1],
		fixture.payloads[1],
		true,
		completedAt.Add(time.Second),
		"",
		nil,
	)
	require.NoError(t, err)
	assert.Equal(t, RecoveryAckFinalizationRequired, status)
	assert.True(t, container.Client.HExists(fixture.ctx, fixture.queueKey, fixture.fields[1]).Val(),
		"the last retry trigger must survive until a terminal response is ready")
	assert.Equal(t, time.Duration(-1), container.Client.TTL(fixture.ctx, fixture.recordKey).Val())

	_, err = fixture.repository.CompareAndDeleteAtomicTransactionBatchRecoveryWithProtectionFrom(
		fixture.ctx,
		RecoveryQueueSourceEngineRecover,
		fixture.organizationID,
		fixture.ledgerID,
		fixture.fields[1],
		fixture.payloads[1],
		true,
		completedAt.Add(time.Second),
		candidate.ReceiptToken,
		map[uuid.UUID]json.RawMessage{
			fixture.record.TransactionIDs[0]: json.RawMessage(`{"id":"first"}`),
		},
	)
	require.ErrorContains(t, err, "requires 2 transaction responses")
	assert.True(t, container.Client.HExists(fixture.ctx, fixture.queueKey, fixture.fields[1]).Val(),
		"response reconstruction failure must preserve the last retry trigger")

	responses := map[uuid.UUID]json.RawMessage{
		fixture.record.TransactionIDs[1]: json.RawMessage(`{"id":"second"}`),
		fixture.record.TransactionIDs[0]: json.RawMessage(`{"id":"first"}`),
	}
	status, err = fixture.repository.CompareAndDeleteAtomicTransactionBatchRecoveryWithProtectionFrom(
		fixture.ctx,
		RecoveryQueueSourceEngineRecover,
		fixture.organizationID,
		fixture.ledgerID,
		fixture.fields[1],
		fixture.payloads[1],
		true,
		completedAt.Add(time.Second),
		candidate.ReceiptToken,
		responses,
	)
	require.NoError(t, err)
	assert.Equal(t, RecoveryAckDeleted, status)
	assert.False(t, container.Client.HExists(fixture.ctx, fixture.queueKey, fixture.fields[1]).Val())

	var complete AtomicTransactionBatchIdempotencyRecord
	require.NoError(t, json.Unmarshal([]byte(container.Client.Get(fixture.ctx, fixture.recordKey).Val()), &complete))
	assert.Equal(t, AtomicTransactionBatchStateComplete, complete.State)
	assert.JSONEq(
		t,
		`{"batchId":"00000000-0000-0000-0000-000000000010","transactions":[{"id":"first"},{"id":"second"}]}`,
		string(complete.Response),
	)
	recordTTL := container.Client.TTL(fixture.ctx, fixture.recordKey).Val()
	indexTTL := container.Client.TTL(fixture.ctx, fixture.indexKey).Val()
	assert.Greater(t, recordTTL, 30*time.Second)
	assert.LessOrEqual(t, recordTTL, 37*time.Second)
	assert.Greater(t, indexTTL, 30*time.Second)
	assert.LessOrEqual(t, indexTTL, 37*time.Second)

	status, err = fixture.repository.CompareAndDeleteAtomicTransactionBatchRecoveryWithProtectionFrom(
		fixture.ctx,
		RecoveryQueueSourceEngineRecover,
		fixture.organizationID,
		fixture.ledgerID,
		fixture.fields[1],
		fixture.payloads[1],
		true,
		completedAt.Add(time.Second),
		candidate.ReceiptToken,
		responses,
	)
	require.NoError(t, err)
	assert.Equal(t, RecoveryAckMissing, status)
	assert.LessOrEqual(t, container.Client.TTL(fixture.ctx, fixture.recordKey).Val(), recordTTL,
		"duplicate delivery must not extend terminal retention")
}

func TestIntegrationAtomicTransactionBatchRecoveryAckFinalizesFromCapturedResponses(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	fixture := newAtomicBatchRecoveryAckFixture(t, container.Client)
	completedAt := time.Date(2026, time.September, 16, 12, 0, 0, 0, time.UTC)
	responses := []json.RawMessage{
		json.RawMessage(`{"id":"first","status":"CREATED"}`),
		json.RawMessage(`{"id":"second","status":"PENDING"}`),
	}

	for index, transactionID := range fixture.record.TransactionIDs {
		result, err := fixture.repository.CaptureAtomicTransactionBatchInitialResponse(
			fixture.ctx,
			fixture.organizationID,
			fixture.ledgerID,
			fixture.executionID,
			fixture.record.OwnerToken,
			transactionID,
			responses[index],
		)
		require.NoError(t, err)
		require.Equal(t, AtomicTransactionBatchInitialResponseCaptured, result.Outcome)
	}

	status, err := fixture.repository.CompareAndDeleteAtomicTransactionBatchRecoveryWithProtectionFrom(
		fixture.ctx,
		RecoveryQueueSourceEngineRecover,
		fixture.organizationID,
		fixture.ledgerID,
		fixture.fields[0],
		fixture.payloads[0],
		true,
		completedAt,
		"",
		nil,
	)
	require.NoError(t, err)
	require.Equal(t, RecoveryAckDeleted, status)

	candidate, err := fixture.repository.GetAtomicTransactionBatchFinalizationCandidate(
		fixture.ctx,
		fixture.organizationID,
		fixture.ledgerID,
		fixture.executionID,
		fixture.record.TransactionIDs[1],
	)
	require.NoError(t, err)
	require.True(t, candidate.Candidate)

	status, err = fixture.repository.CompareAndDeleteAtomicTransactionBatchRecoveryWithProtectionFrom(
		fixture.ctx,
		RecoveryQueueSourceEngineRecover,
		fixture.organizationID,
		fixture.ledgerID,
		fixture.fields[1],
		fixture.payloads[1],
		true,
		completedAt.Add(time.Second),
		candidate.ReceiptToken,
		nil,
	)
	require.NoError(t, err)
	require.Equal(t, RecoveryAckDeleted, status)

	var complete AtomicTransactionBatchIdempotencyRecord
	require.NoError(t, json.Unmarshal([]byte(container.Client.Get(fixture.ctx, fixture.recordKey).Val()), &complete))
	require.Equal(t, AtomicTransactionBatchStateComplete, complete.State)
	require.JSONEq(
		t,
		`{"batchId":"00000000-0000-0000-0000-000000000010","transactions":[{"id":"first","status":"CREATED"},{"id":"second","status":"PENDING"}]}`,
		string(complete.Response),
	)
}

func TestIntegrationAtomicTransactionBatchRecoveryAckRetainsMemberWhenReceiptChanges(t *testing.T) {
	if testing.Short() {
		t.Skip("requires Valkey")
	}

	container := redistestutil.SetupReusableContainer(t)
	fixture := newAtomicBatchRecoveryAckFixture(t, container.Client)
	completedAt := time.Date(2026, time.September, 16, 12, 0, 0, 0, time.UTC)

	status, err := fixture.repository.CompareAndDeleteAtomicTransactionBatchRecoveryWithProtectionFrom(
		fixture.ctx,
		RecoveryQueueSourceEngineRecover,
		fixture.organizationID,
		fixture.ledgerID,
		fixture.fields[0],
		fixture.payloads[0],
		true,
		completedAt,
		"",
		nil,
	)
	require.NoError(t, err)
	assert.Equal(t, RecoveryAckDeleted, status)

	candidate, err := fixture.repository.GetAtomicTransactionBatchFinalizationCandidate(
		fixture.ctx,
		fixture.organizationID,
		fixture.ledgerID,
		fixture.executionID,
		fixture.record.TransactionIDs[1],
	)
	require.NoError(t, err)
	require.True(t, candidate.Candidate)

	var changed map[string]any
	require.NoError(t, json.Unmarshal([]byte(candidate.ReceiptToken), &changed))
	changed["extension"] = "concurrent-worker"
	changedPayload, err := json.Marshal(changed)
	require.NoError(t, err)
	require.NoError(t, container.Client.HSet(
		fixture.ctx,
		fixture.receiptKey,
		fixture.executionID.String(),
		changedPayload,
	).Err())

	status, err = fixture.repository.CompareAndDeleteAtomicTransactionBatchRecoveryWithProtectionFrom(
		fixture.ctx,
		RecoveryQueueSourceEngineRecover,
		fixture.organizationID,
		fixture.ledgerID,
		fixture.fields[1],
		fixture.payloads[1],
		true,
		completedAt.Add(time.Second),
		candidate.ReceiptToken,
		map[uuid.UUID]json.RawMessage{
			fixture.record.TransactionIDs[0]: json.RawMessage(`{"id":"first"}`),
			fixture.record.TransactionIDs[1]: json.RawMessage(`{"id":"second"}`),
		},
	)
	require.NoError(t, err)
	assert.Equal(t, RecoveryAckReceiptChanged, status)
	assert.True(t, container.Client.HExists(fixture.ctx, fixture.queueKey, fixture.fields[1]).Val())
	assert.Equal(t, time.Duration(-1), container.Client.TTL(fixture.ctx, fixture.recordKey).Val())
}

func newAtomicBatchRecoveryAckFixture(
	t *testing.T,
	client goredis.UniversalClient,
) atomicBatchRecoveryAckFixture {
	t.Helper()

	tenantID := uuid.NewSHA1(
		uuid.MustParse("b6a41d73-ef6c-50c6-a97a-7828077996e7"),
		[]byte(t.Name()),
	).String()
	ctx := tmcore.ContextWithTenantID(t.Context(), tenantID)
	repository, err := NewConsumerRedis(&staticRedisProvider{client: client})
	require.NoError(t, err)
	organizationID := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	ledgerID := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	record := atomicBatchAppliedRecord()
	record.FormatVersion = AtomicTransactionBatchLegacyFormatVersion
	executionID := *record.ExecutionID
	recordKey, err := tenantKeyFromContextOrError(
		ctx,
		utils.AtomicTransactionBatchIdempotencyInternalKey(organizationID, ledgerID, "recovery-ack"),
	)
	require.NoError(t, err)
	indexKey, err := tenantKeyFromContextOrError(
		ctx,
		utils.AtomicTransactionBatchExecutionIndexInternalKey(organizationID, ledgerID, executionID),
	)
	require.NoError(t, err)
	receiptKey, err := tenantKeyFromContextOrError(
		ctx,
		atomicTransactionBatchEngineReceiptInternalKey(organizationID, ledgerID),
	)
	require.NoError(t, err)
	queueKey, err := tenantKeyFromContextOrError(ctx, cachepolicy.EngineRecoverQueue)
	require.NoError(t, err)
	guardKey, err := tenantKeyFromContextOrError(
		ctx,
		"engine:"+cachepolicy.HashTag+":guards:"+organizationID.String()+":"+ledgerID.String(),
	)
	require.NoError(t, err)
	protectionKey, err := tenantKeyFromContextOrError(
		ctx,
		"engine:"+cachepolicy.HashTag+":protection:"+organizationID.String()+":"+ledgerID.String(),
	)
	require.NoError(t, err)
	cleanupKey, err := tenantKeyFromContextOrError(ctx, EngineRecoveryCleanupSchedule)
	require.NoError(t, err)

	fields := make([]string, len(record.TransactionIDs))
	payloads := make([]string, len(record.TransactionIDs))
	acknowledged := make(map[string]bool, len(record.TransactionIDs))
	terminal := make(map[string]int64, len(record.TransactionIDs))
	for index, transactionID := range record.TransactionIDs {
		fields[index] = transactionID.String() + ":" + executionID.String()
		payloads[index] = `{"formatVersion":2,"index":` + string(rune('0'+index)) + `}`
		acknowledged[transactionID.String()] = false
		terminal[transactionID.String()] = 0
	}
	receipt := atomicTransactionBatchReceipt{
		FormatVersion:     1,
		OrganizationID:    organizationID,
		LedgerID:          ledgerID,
		ExecutionID:       executionID,
		IntentFingerprint: "engine-intent-fingerprint",
		Protection: atomicTransactionBatchReceiptProtection{
			FormatVersion:         1,
			RetentionSeconds:      37,
			Transactions:          append([]uuid.UUID(nil), record.TransactionIDs...),
			RecoveryFields:        append([]string(nil), fields...),
			Acknowledged:          acknowledged,
			TerminalCompletedAtMS: terminal,
		},
	}
	recordPayload, err := json.Marshal(record)
	require.NoError(t, err)
	receiptPayload, err := json.Marshal(receipt)
	require.NoError(t, err)
	require.NoError(t, client.Set(ctx, recordKey, recordPayload, 0).Err())
	require.NoError(t, client.Set(ctx, indexKey, recordKey, 0).Err())
	require.NoError(t, client.HSet(
		ctx,
		receiptKey,
		executionID.String(),
		receiptPayload,
	).Err())
	require.NoError(t, client.HSet(
		ctx,
		queueKey,
		fields[0],
		payloads[0],
		fields[1],
		payloads[1],
	).Err())
	cleanupKeys := []string{recordKey, indexKey, receiptKey, queueKey, guardKey, protectionKey, cleanupKey}
	t.Cleanup(func() { require.NoError(t, client.Del(context.Background(), cleanupKeys...).Err()) })

	return atomicBatchRecoveryAckFixture{
		ctx:            ctx,
		repository:     repository,
		organizationID: organizationID,
		ledgerID:       ledgerID,
		executionID:    executionID,
		record:         record,
		recordKey:      recordKey,
		indexKey:       indexKey,
		receiptKey:     receiptKey,
		queueKey:       queueKey,
		fields:         fields,
		payloads:       payloads,
	}
}
