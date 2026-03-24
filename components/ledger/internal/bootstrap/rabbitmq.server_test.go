// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
)

// =============================================================================
// UNIT TESTS - handlerBTOQueue
// =============================================================================

func TestHandlerBTOQueue(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("msgpack_unmarshal_error", func(t *testing.T) {
		t.Parallel()

		consumer := &MultiQueueConsumer{
			UseCase: &command.UseCase{},
		}

		// Invalid msgpack body (plain text that can't be unmarshaled)
		invalidBody := []byte{0xFF, 0xFE, 0xFD} // Invalid msgpack bytes

		err := consumer.handlerBTOQueue(ctx, invalidBody)

		require.Error(t, err)
	})

	t.Run("invalid_transaction_data_in_queue", func(t *testing.T) {
		t.Parallel()

		consumer := &MultiQueueConsumer{
			UseCase: &command.UseCase{},
		}

		// Create queue with invalid transaction data
		queueMessage := mmodel.Queue{
			OrganizationID: uuid.New(),
			LedgerID:       uuid.New(),
			QueueData: []mmodel.QueueData{
				{
					ID:    uuid.New(),
					Value: []byte{}, // Empty value will fail msgpack unmarshal
				},
			},
		}

		body, err := msgpack.Marshal(queueMessage)
		require.NoError(t, err)

		err = consumer.handlerBTOQueue(ctx, body)

		// The UseCase.CreateBalanceTransactionOperationsAsync tries to
		// unmarshal QueueData.Value into TransactionProcessingPayload which fails
		require.Error(t, err)
	})
}

// =============================================================================
// UNIT TESTS - MultiQueueConsumer struct
// =============================================================================

func TestMultiQueueConsumer_StructFields(t *testing.T) {
	t.Parallel()

	uc := &command.UseCase{}
	consumer := &MultiQueueConsumer{
		UseCase: uc,
	}

	assert.NotNil(t, consumer)
	assert.Equal(t, uc, consumer.UseCase)
}

// =============================================================================
// UNIT TESTS - aggregatePayloadsByOrgLedger
// =============================================================================

func TestAggregatePayloadsByOrgLedger(t *testing.T) {
	t.Parallel()

	orgID1 := uuid.New().String()
	orgID2 := uuid.New().String()
	ledgerID1 := uuid.New().String()
	ledgerID2 := uuid.New().String()

	t.Run("empty_payloads", func(t *testing.T) {
		t.Parallel()

		result := &command.BulkResult{
			TransactionsAttempted: 10,
			TransactionsInserted:  8,
			TransactionsIgnored:   2,
		}

		counts := aggregatePayloadsByOrgLedger(nil, result)

		assert.Empty(t, counts)
	})

	t.Run("single_org_ledger", func(t *testing.T) {
		t.Parallel()

		payloads := []transaction.TransactionProcessingPayload{
			{Transaction: &transaction.Transaction{OrganizationID: orgID1, LedgerID: ledgerID1}},
			{Transaction: &transaction.Transaction{OrganizationID: orgID1, LedgerID: ledgerID1}},
		}

		result := &command.BulkResult{
			TransactionsAttempted: 2,
			TransactionsInserted:  2,
			TransactionsIgnored:   0,
			OperationsAttempted:   0,
			OperationsInserted:    0,
			OperationsIgnored:     0,
		}

		counts := aggregatePayloadsByOrgLedger(payloads, result)

		assert.Len(t, counts, 1)

		key := bulkMetricKey{organizationID: orgID1, ledgerID: ledgerID1}
		assert.Contains(t, counts, key)
		assert.Equal(t, int64(2), counts[key].payloadCount)
		assert.Equal(t, int64(2), counts[key].transactionsAttempted)
		assert.Equal(t, int64(2), counts[key].transactionsInserted)
		assert.Equal(t, int64(0), counts[key].transactionsIgnored)
	})

	t.Run("multiple_org_ledgers", func(t *testing.T) {
		t.Parallel()

		payloads := []transaction.TransactionProcessingPayload{
			{Transaction: &transaction.Transaction{OrganizationID: orgID1, LedgerID: ledgerID1}},
			{Transaction: &transaction.Transaction{OrganizationID: orgID1, LedgerID: ledgerID1}},
			{Transaction: &transaction.Transaction{OrganizationID: orgID2, LedgerID: ledgerID2}},
		}

		result := &command.BulkResult{
			TransactionsAttempted: 3,
			TransactionsInserted:  3,
			TransactionsIgnored:   0,
		}

		counts := aggregatePayloadsByOrgLedger(payloads, result)

		assert.Len(t, counts, 2)

		key1 := bulkMetricKey{organizationID: orgID1, ledgerID: ledgerID1}
		key2 := bulkMetricKey{organizationID: orgID2, ledgerID: ledgerID2}

		assert.Contains(t, counts, key1)
		assert.Contains(t, counts, key2)

		// 2/3 of payloads belong to org1/ledger1
		assert.Equal(t, int64(2), counts[key1].payloadCount)
		assert.Equal(t, int64(2), counts[key1].transactionsAttempted) // 3 * 2/3 = 2

		// 1/3 of payloads belong to org2/ledger2
		assert.Equal(t, int64(1), counts[key2].payloadCount)
		assert.Equal(t, int64(1), counts[key2].transactionsAttempted) // 3 * 1/3 = 1
	})

	t.Run("nil_transaction_skipped", func(t *testing.T) {
		t.Parallel()

		payloads := []transaction.TransactionProcessingPayload{
			{Transaction: &transaction.Transaction{OrganizationID: orgID1, LedgerID: ledgerID1}},
			{Transaction: nil}, // Should be skipped
		}

		result := &command.BulkResult{
			TransactionsAttempted: 1,
			TransactionsInserted:  1,
		}

		counts := aggregatePayloadsByOrgLedger(payloads, result)

		assert.Len(t, counts, 1)
		key := bulkMetricKey{organizationID: orgID1, ledgerID: ledgerID1}
		assert.Equal(t, int64(1), counts[key].payloadCount)
	})
}

// =============================================================================
// UNIT TESTS - recordBulkOTelMetrics
// =============================================================================

func TestRecordBulkOTelMetrics_NilFactory(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	result := &command.BulkResult{
		TransactionsAttempted: 10,
	}
	payloads := []transaction.TransactionProcessingPayload{}

	// Should not panic when factory is nil
	assert.NotPanics(t, func() {
		recordBulkOTelMetrics(ctx, nil, result, payloads, 100)
	})
}

func TestRecordBulkCounter_NilFactory(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Should not panic when factory is nil
	assert.NotPanics(t, func() {
		recordBulkCounter(ctx, nil, utils.BulkRecorderTransactionsAttempted, 10, nil)
	})
}

func TestRecordBulkCounter_ZeroValue(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Should not panic when value is zero (short-circuits before calling factory)
	assert.NotPanics(t, func() {
		recordBulkCounter(ctx, nil, utils.BulkRecorderTransactionsAttempted, 0, nil)
	})
}

func TestRecordBulkHistogram_NilFactory(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Should not panic when factory is nil
	assert.NotPanics(t, func() {
		recordBulkHistogram(ctx, nil, utils.BulkRecorderBulkDuration, 100, nil)
	})
}
