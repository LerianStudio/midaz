// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	operationPostgres "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	transactionPostgres "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/rabbitmq"
	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

type orderedAtomicTransactionBatchCompleter struct {
	delegate *createAppliedTransactionCompleter
	order    *[]string
	failAt   int
	err      error
}

func (completer *orderedAtomicTransactionBatchCompleter) Complete(
	ctx context.Context,
	record *TransactionCompletionRecord,
) (TransactionCompletionResult, error) {
	*completer.order = append(*completer.order, "complete:"+record.TransactionID.String())
	if len(completer.delegate.envelopes) == completer.failAt {
		completer.delegate.envelopes = append(completer.delegate.envelopes, record)

		return TransactionCompletionResult{}, completer.err
	}

	return completer.delegate.Complete(ctx, record)
}

type orderedAtomicTransactionBatchAcknowledger struct {
	delegate *recordingEngineRecoveryAcknowledger
	order    *[]string
}

func (acknowledger *orderedAtomicTransactionBatchAcknowledger) AcknowledgeEngineRecovery(
	ctx context.Context,
	record *TransactionCompletionRecord,
	completion TransactionCompletionResult,
) error {
	*acknowledger.order = append(*acknowledger.order, "ack:"+record.TransactionID.String())

	return acknowledger.delegate.AcknowledgeEngineRecovery(ctx, record, completion)
}

func TestCreateAtomicTransactionBatchV2_CompletesAndAcknowledgesPartitionsInOrder(t *testing.T) {
	repository := &atomicTransactionBatchClaimRepositoryFake{}
	engine := &applyingAtomicTransactionBatchEngine{t: t}
	reserver := atomicTransactionBatchExecutionReserver()
	uc, input, transactionIDs, _ := atomicTransactionBatchExecutionFixture(
		t,
		repository,
		engine,
		reserver,
	)

	order := make([]string, 0, len(transactionIDs)*2)
	repository.captureOrder = &order
	completionDelegate := &createAppliedTransactionCompleter{
		outcome: TransactionPersistenceOutcome{TransactionStatus: constant.APPROVED},
	}
	acknowledgmentDelegate := &recordingEngineRecoveryAcknowledger{}
	uc.AppliedTransactionCompleter = &orderedAtomicTransactionBatchCompleter{
		delegate: completionDelegate,
		order:    &order,
		failAt:   -1,
	}
	uc.EngineRecoveryAcknowledger = &orderedAtomicTransactionBatchAcknowledger{
		delegate: acknowledgmentDelegate,
		order:    &order,
	}

	// Strict mocks with no expectations prove the batch coordinator never
	// reaches direct transaction/operation writes, legacy RabbitMQ write-behind,
	// or the Redis backup-queue API. Durable projection is owned only by the
	// injected idempotent completer above.
	controller := gomock.NewController(t)
	uc.TransactionRepo = transactionPostgres.NewMockRepository(controller)
	uc.OperationRepo = operationPostgres.NewMockRepository(controller)
	uc.TransactionRedisRepo = txRedis.NewMockRedisRepository(controller)
	uc.RabbitMQRepo = rabbitmq.NewMockProducerRepository(controller)

	result, err := uc.CreateAtomicTransactionBatchV2(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Transactions, len(transactionIDs))
	require.Len(t, completionDelegate.envelopes, len(transactionIDs))
	require.Len(t, acknowledgmentDelegate.records, len(transactionIDs))

	assert.Equal(t, []string{
		"capture:" + transactionIDs[0].String(),
		"capture:" + transactionIDs[1].String(),
		"complete:" + transactionIDs[0].String(),
		"complete:" + transactionIDs[1].String(),
		"ack:" + transactionIDs[0].String(),
		"ack:" + transactionIDs[1].String(),
	}, order)
	assert.Equal(t, len(transactionIDs), repository.captures)
	for index, transactionID := range transactionIDs {
		record := completionDelegate.envelopes[index]
		assert.Equal(t, transactionID, record.TransactionID)
		assert.Equal(t, transactionID, acknowledgmentDelegate.records[index].TransactionID)
		assert.Equal(t, transactionID.String(), result.Transactions[index].ID)
		assert.Equal(t, constant.CREATED, result.Transactions[index].Status.Code)
		require.NotEmpty(t, record.Result.Movements)
		for _, movement := range record.Result.Movements {
			assert.Equal(t, transactionID, movement.TransactionID)
		}
	}
	assert.Equal(t, 1, repository.handoffs)
	assert.Equal(t, 1, repository.finalizations)
	require.Len(t, engine.executions, 1)
}

func TestEngineWriteBehindAtomicTransactionBatchFinalizesReplayBeforeAsyncProjection(t *testing.T) {
	repository := &atomicTransactionBatchClaimRepositoryFake{}
	engine := &applyingAtomicTransactionBatchEngine{t: t}
	uc, input, transactionIDs, _ := atomicTransactionBatchExecutionFixture(
		t, repository, engine, atomicTransactionBatchExecutionReserver(),
	)
	completion := &createAppliedTransactionCompleter{
		outcome: TransactionPersistenceOutcome{TransactionStatus: constant.APPROVED},
	}
	dispatcher := &createWriteBehindDispatcherStub{}
	uc.AppliedTransactionCompleter = completion
	uc.TransactionWriteBehindAsync = true
	uc.TransactionWriteBehindDispatcher = dispatcher

	result, err := uc.CreateAtomicTransactionBatchV2(context.Background(), input)

	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Transactions, len(transactionIDs))
	require.Equal(t, len(transactionIDs), repository.captures)
	require.Equal(t, 1, repository.finalizations)
	require.Equal(t, len(transactionIDs), dispatcher.calls)
	require.Empty(t, completion.envelopes, "confirmed async publication must not block on SQL/Mongo")
	require.Len(t, engine.executions, 1, "projection transport must never reapply accounting")
}

func TestCreateAtomicTransactionBatchV2_CompletionFailureReturnsFrozenReplayAndLeavesAllRecordsForRecovery(t *testing.T) {
	cause := errors.New("durable projection unavailable")
	repository := &atomicTransactionBatchClaimRepositoryFake{}
	engine := &applyingAtomicTransactionBatchEngine{t: t}
	reserver := atomicTransactionBatchExecutionReserver()
	uc, input, transactionIDs, _ := atomicTransactionBatchExecutionFixture(
		t,
		repository,
		engine,
		reserver,
	)

	order := make([]string, 0, 3)
	completionDelegate := &createAppliedTransactionCompleter{
		outcome: TransactionPersistenceOutcome{TransactionStatus: constant.APPROVED},
	}
	acknowledgmentDelegate := &recordingEngineRecoveryAcknowledger{}
	uc.AppliedTransactionCompleter = &orderedAtomicTransactionBatchCompleter{
		delegate: completionDelegate,
		order:    &order,
		failAt:   1,
		err:      cause,
	}
	uc.EngineRecoveryAcknowledger = &orderedAtomicTransactionBatchAcknowledger{
		delegate: acknowledgmentDelegate,
		order:    &order,
	}

	result, err := uc.CreateAtomicTransactionBatchV2(context.Background(), input)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Transactions, len(transactionIDs))
	assert.Equal(t, []string{
		"complete:" + transactionIDs[0].String(),
		"complete:" + transactionIDs[1].String(),
	}, order)
	require.Len(t, completionDelegate.envelopes, 2)
	require.Empty(t, acknowledgmentDelegate.records)
	assert.Equal(t, 1, repository.handoffs)
	assert.Equal(t, 1, repository.finalizations)
	assert.Zero(t, repository.aborts)
	assert.Zero(t, repository.deletes)
	require.Len(t, engine.executions, 1, "completion failure must never retry accounting")
}

func TestCreateAtomicTransactionBatchV2_FinalizationFailureRetainsLastRecoveryRecord(t *testing.T) {
	cause := errors.New("terminal idempotency unavailable")
	repository := &atomicTransactionBatchClaimRepositoryFake{finalizationErr: cause}
	engine := &applyingAtomicTransactionBatchEngine{t: t}
	reserver := atomicTransactionBatchExecutionReserver()
	uc, input, transactionIDs, _ := atomicTransactionBatchExecutionFixture(
		t,
		repository,
		engine,
		reserver,
	)

	order := make([]string, 0, len(transactionIDs)*2)
	completionDelegate := &createAppliedTransactionCompleter{
		outcome: TransactionPersistenceOutcome{TransactionStatus: constant.APPROVED},
	}
	acknowledgmentDelegate := &recordingEngineRecoveryAcknowledger{}
	uc.AppliedTransactionCompleter = &orderedAtomicTransactionBatchCompleter{
		delegate: completionDelegate,
		order:    &order,
		failAt:   -1,
	}
	uc.EngineRecoveryAcknowledger = &orderedAtomicTransactionBatchAcknowledger{
		delegate: acknowledgmentDelegate,
		order:    &order,
	}

	result, err := uc.CreateAtomicTransactionBatchV2(context.Background(), input)
	require.ErrorIs(t, err, cause)
	assert.Nil(t, result)
	assert.Empty(t, order)
	assert.Equal(t, 1, repository.finalizations)
	require.Empty(t, completionDelegate.envelopes)
	require.Empty(t, acknowledgmentDelegate.records)
	require.Len(t, engine.executions, 1, "terminal finalization failure must never retry accounting")
}
