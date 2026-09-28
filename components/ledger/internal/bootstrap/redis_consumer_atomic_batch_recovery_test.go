// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
)

type atomicBatchRecoveryFinalizerStub struct {
	prepared *command.AtomicTransactionBatchRecoveryFinalization
	err      error
	calls    int
	order    *[]string
	record   *command.TransactionCompletionRecord
}

func (stub *atomicBatchRecoveryFinalizerStub) PrepareAtomicTransactionBatchRecoveryFinalization(
	_ context.Context,
	record *command.TransactionCompletionRecord,
	_ command.TransactionCompletionResult,
) (*command.AtomicTransactionBatchRecoveryFinalization, error) {
	stub.calls++
	stub.record = record
	*stub.order = append(*stub.order, "batch-finalization")

	return stub.prepared, stub.err
}

type atomicBatchRecoveryQueueStub struct {
	txRedis.RedisRepository

	status        int64
	err           error
	batchCalls    int
	ordinaryCalls int
	order         *[]string
	source        txRedis.RecoveryQueueSource
	organization  uuid.UUID
	ledger        uuid.UUID
	field         string
	payload       string
	terminal      bool
	completedAt   time.Time
	receiptToken  string
	transactions  map[uuid.UUID]json.RawMessage
}

func (stub *atomicBatchRecoveryQueueStub) CompareAndDeleteRecoveryFrom(
	_ context.Context,
	_ txRedis.RecoveryQueueSource,
	_, _ string,
) (int64, error) {
	stub.ordinaryCalls++

	return stub.status, stub.err
}

func (stub *atomicBatchRecoveryQueueStub) CompareAndDeleteAtomicTransactionBatchRecoveryWithProtectionFrom(
	_ context.Context,
	source txRedis.RecoveryQueueSource,
	organizationID, ledgerID uuid.UUID,
	field, expectedPayload string,
	terminal bool,
	completedAt time.Time,
	receiptToken string,
	transactions map[uuid.UUID]json.RawMessage,
) (int64, error) {
	stub.batchCalls++
	stub.source = source
	stub.organization = organizationID
	stub.ledger = ledgerID
	stub.field = field
	stub.payload = expectedPayload
	stub.terminal = terminal
	stub.completedAt = completedAt
	stub.receiptToken = receiptToken
	stub.transactions = transactions
	*stub.order = append(*stub.order, "batch-ack")

	return stub.status, stub.err
}

func TestCompleteRecoveryRecord_UsesPreparedBatchFinalizationInProtectedAck(t *testing.T) {
	organizationID := uuid.MustParse("01994f13-29b7-7000-8000-000000000121")
	ledgerID := uuid.MustParse("01994f13-29b7-7000-8000-000000000122")
	transactionID := uuid.MustParse("01994f13-29b7-7000-8000-000000000123")
	executionID := uuid.MustParse("01994f13-29b7-7000-8000-000000000124")
	completedAt := time.Date(2026, time.September, 16, 13, 0, 0, 0, time.UTC)
	field := transactionID.String() + ":" + executionID.String()
	const raw = `{"formatVersion":2}`
	transactions := map[uuid.UUID]json.RawMessage{
		transactionID: json.RawMessage(`{"id":"transaction"}`),
	}
	order := []string{}
	completer := &recoveryCompleterStub{order: &order}
	finalizer := &atomicBatchRecoveryFinalizerStub{
		prepared: &command.AtomicTransactionBatchRecoveryFinalization{
			ReceiptToken: "receipt-token",
			Transactions: transactions,
		},
		order: &order,
	}
	queue := &atomicBatchRecoveryQueueStub{
		status: txRedis.RecoveryAckDeleted,
		order:  &order,
	}
	record := &command.TransactionCompletionRecord{
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		ExecutionID:    executionID,
		TransactionID:  transactionID,
	}
	coordinator := &recoveryRecordCompleter{
		queue:          queue,
		completer:      completer,
		batchFinalizer: finalizer,
		clock:          func() time.Time { return completedAt },
	}

	err := coordinator.complete(
		context.Background(),
		txRedis.RecoveryQueueSourceEngineRecover,
		field,
		raw,
		record,
	)
	require.NoError(t, err)
	assert.Equal(t, []string{"durable-finalization", "batch-finalization", "batch-ack"}, order)
	assert.Equal(t, 1, finalizer.calls)
	assert.Same(t, record, finalizer.record)
	assert.Equal(t, 1, queue.batchCalls)
	assert.Zero(t, queue.ordinaryCalls)
	assert.Equal(t, txRedis.RecoveryQueueSourceEngineRecover, queue.source)
	assert.Equal(t, organizationID, queue.organization)
	assert.Equal(t, ledgerID, queue.ledger)
	assert.Equal(t, field, queue.field)
	assert.Equal(t, raw, queue.payload)
	assert.True(t, queue.terminal)
	assert.Equal(t, completedAt, queue.completedAt)
	assert.Equal(t, "receipt-token", queue.receiptToken)
	assert.Equal(t, transactions, queue.transactions)
}

func TestCompleteRecoveryRecord_RetainsBatchMemberForFinalizationRetry(t *testing.T) {
	tests := []struct {
		name       string
		status     int64
		want       string
		finalError error
	}{
		{
			name:   "response required after out-of-order acknowledgment",
			status: txRedis.RecoveryAckFinalizationRequired,
			want:   "finalization response is not ready",
		},
		{
			name:   "receipt changed by concurrent worker",
			status: txRedis.RecoveryAckReceiptChanged,
			want:   "receipt changed during finalization",
		},
		{
			name:       "finalization preparation failed",
			status:     txRedis.RecoveryAckDeleted,
			want:       "prepare atomic transaction batch recovery finalization",
			finalError: errors.New("primary projection unavailable"),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			order := []string{}
			completer := &recoveryCompleterStub{order: &order}
			finalizer := &atomicBatchRecoveryFinalizerStub{
				prepared: &command.AtomicTransactionBatchRecoveryFinalization{},
				err:      test.finalError,
				order:    &order,
			}
			queue := &atomicBatchRecoveryQueueStub{status: test.status, order: &order}
			coordinator := &recoveryRecordCompleter{
				queue:          queue,
				completer:      completer,
				batchFinalizer: finalizer,
				clock: func() time.Time {
					return time.Date(2026, time.September, 16, 13, 0, 0, 0, time.UTC)
				},
			}
			record := &command.TransactionCompletionRecord{
				OrganizationID: uuid.New(),
				LedgerID:       uuid.New(),
				ExecutionID:    uuid.New(),
				TransactionID:  uuid.New(),
			}

			err := coordinator.complete(
				context.Background(),
				txRedis.RecoveryQueueSourceEngineRecover,
				"field",
				"raw",
				record,
			)
			require.ErrorContains(t, err, test.want)
			assert.Equal(t, 1, finalizer.calls)
			if test.finalError != nil {
				assert.Zero(t, queue.batchCalls)
				assert.Equal(t, []string{"durable-finalization", "batch-finalization"}, order)
				return
			}
			assert.Equal(t, 1, queue.batchCalls)
			assert.Equal(t, []string{"durable-finalization", "batch-finalization", "batch-ack"}, order)
		})
	}
}

func TestCompleteRecoveryRecord_BatchFinalizerDoesNotAffectLegacySource(t *testing.T) {
	order := []string{}
	completer := &recoveryCompleterStub{order: &order}
	finalizer := &atomicBatchRecoveryFinalizerStub{
		prepared: &command.AtomicTransactionBatchRecoveryFinalization{},
		order:    &order,
	}
	queue := &atomicBatchRecoveryQueueStub{status: txRedis.RecoveryAckDeleted, order: &order}
	coordinator := &recoveryRecordCompleter{
		queue:          queue,
		completer:      completer,
		batchFinalizer: finalizer,
		clock: func() time.Time {
			return time.Date(2026, time.September, 16, 13, 0, 0, 0, time.UTC)
		},
	}

	err := coordinator.complete(
		context.Background(),
		txRedis.RecoveryQueueSourceLegacyBackup,
		"field",
		"raw",
		&command.TransactionCompletionRecord{},
	)
	require.NoError(t, err)
	assert.Zero(t, finalizer.calls)
	assert.Zero(t, queue.batchCalls)
	assert.Equal(t, 1, queue.ordinaryCalls)
	assert.Equal(t, []string{"durable-finalization"}, order)
}
