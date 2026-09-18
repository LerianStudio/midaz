// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/rabbitmq"
	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
)

func TestPendingTransition_AsyncWriteBehindPreservesPriorOperations(t *testing.T) {
	tests := []struct {
		name       string
		status     string
		transition func(*UseCase, PendingTransitionInput) (*transaction.Transaction, error)
	}{
		{
			name:   "commit",
			status: constant.APPROVED,
			transition: func(uc *UseCase, in PendingTransitionInput) (*transaction.Transaction, error) {
				return uc.CommitTransactionV1(context.Background(), in)
			},
		},
		{
			name:   "cancel",
			status: constant.CANCELED,
			transition: func(uc *UseCase, in PendingTransitionInput) (*transaction.Transaction, error) {
				return uc.CancelTransactionV1(context.Background(), in)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("AUDIT_LOG_ENABLED", "false")
			t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "true")

			tran := pendingTransaction(false)
			tran.Operations = []*operation.Operation{
				{ID: uuid.New().String(), Type: constant.ONHOLD},
				{ID: uuid.New().String(), Type: constant.ONHOLD},
			}
			priorIDs := operationIDs(tran.Operations)
			before, after := pendingTransitionBalanceSnapshots(tran, tt.status)

			ctrl := gomock.NewController(t)
			redisRepo := txRedis.NewMockRedisRepository(ctrl)
			cachedBytes := make(chan []byte, 1)

			redisRepo.EXPECT().SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil).Times(1)
			redisRepo.EXPECT().AddMessageToQueue(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
			redisRepo.EXPECT().ReadMessageFromQueue(gomock.Any(), gomock.Any()).Return(nil, errors.New("no backup entry")).Times(1)
			redisRepo.EXPECT().ProcessBalanceAtomicOperation(
				gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
			).Return(&mmodel.BalanceAtomicResult{Before: before, After: after}, nil).Times(1)
			redisRepo.EXPECT().SetBytes(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, _ string, data []byte, _ time.Duration) error {
					cachedBytes <- append([]byte(nil), data...)

					return nil
				}).Times(1)
			redisRepo.EXPECT().Del(gomock.Any(), gomock.Any()).Times(0)

			transactionRepo := transaction.NewMockRepository(ctrl)
			transactionRepo.EXPECT().
				UpdateStatusFromPending(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return(tran, true, nil).Times(1)

			publishedBytes := make(chan []byte, 1)
			producer := rabbitmq.NewMockProducerRepository(ctrl)
			producer.EXPECT().ProducerDefaultWithContext(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				DoAndReturn(func(_ context.Context, _, _ string, data []byte) (*string, error) {
					publishedBytes <- append([]byte(nil), data...)

					return nil, nil
				}).Times(1)

			uc := &UseCase{
				TransactionRedisRepo: redisRepo,
				TransactionReader:    &pendingReader{pending: tran, balances: before},
				TransactionRepo:      transactionRepo,
				RabbitMQRepo:         producer,
			}

			got, err := tt.transition(uc, pendingTransitionInputFor(tran))
			require.NoError(t, err)
			require.Equal(t, tt.status, got.Status.Code)
			require.NotEmpty(t, got.Operations, "the transition must publish new operation legs")

			var queued mmodel.Queue
			require.NoError(t, msgpack.Unmarshal(<-publishedBytes, &queued))
			require.Len(t, queued.QueueData, 1)

			var payload transaction.TransactionProcessingPayload
			require.NoError(t, msgpack.Unmarshal(queued.QueueData[0].Value, &payload))
			require.NotNil(t, payload.Transaction)
			assert.Equal(t, operationIDs(got.Operations), operationIDs(payload.Transaction.Operations),
				"the RabbitMQ message must contain only the new transition operations")
			assert.NotContains(t, operationIDs(payload.Transaction.Operations), priorIDs[0])
			assert.NotContains(t, operationIDs(payload.Transaction.Operations), priorIDs[1])

			var cached transaction.Transaction
			select {
			case data := <-cachedBytes:
				require.NoError(t, msgpack.Unmarshal(data, &cached))
			case <-time.After(2 * time.Second):
				t.Fatal("timed out waiting for the asynchronous write-behind refresh")
			}

			expectedCachedIDs := append(append([]string(nil), priorIDs...), operationIDs(got.Operations)...)
			assert.Equal(t, tt.status, cached.Status.Code)
			assert.Equal(t, expectedCachedIDs, operationIDs(cached.Operations),
				"the cache must retain the original hold before the new transition operations")
		})
	}
}

func pendingTransitionBalanceSnapshots(tran *transaction.Transaction, status string) ([]*mmodel.Balance, []*mmodel.Balance) {
	amount := decimal.NewFromInt(100)
	zero := decimal.Zero

	before := []*mmodel.Balance{
		{
			ID:             uuid.New().String(),
			OrganizationID: tran.OrganizationID,
			LedgerID:       tran.LedgerID,
			AccountID:      uuid.New().String(),
			Alias:          "@payer",
			Key:            constant.DefaultBalanceKey,
			AssetCode:      "BRL",
			Available:      zero,
			OnHold:         amount,
			Version:        2,
			AllowSending:   true,
			AllowReceiving: true,
		},
		{
			ID:             uuid.New().String(),
			OrganizationID: tran.OrganizationID,
			LedgerID:       tran.LedgerID,
			AccountID:      uuid.New().String(),
			Alias:          "@payee",
			Key:            constant.DefaultBalanceKey,
			AssetCode:      "BRL",
			Available:      zero,
			OnHold:         zero,
			Version:        1,
			AllowSending:   true,
			AllowReceiving: true,
		},
	}

	after := []*mmodel.Balance{
		clonePendingBalance(before[0]),
		clonePendingBalance(before[1]),
	}
	after[0].OnHold = zero
	after[0].Version++

	if status == constant.CANCELED {
		after[0].Available = amount
	} else {
		after[1].Available = amount
		after[1].Version++
	}

	return before, after
}

func clonePendingBalance(balance *mmodel.Balance) *mmodel.Balance {
	cloned := *balance

	return &cloned
}

func operationIDs(operations []*operation.Operation) []string {
	ids := make([]string, 0, len(operations))

	for _, op := range operations {
		if op != nil {
			ids = append(ids, op.ID)
		}
	}

	return ids
}
