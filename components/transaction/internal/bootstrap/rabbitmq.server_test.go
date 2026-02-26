// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
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
