// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

// requireAlreadyTransitioned asserts the error is the 409 a transition answers
// when the transaction is already terminal.
func requireAlreadyTransitioned(t *testing.T, err error) {
	t.Helper()

	require.Error(t, err)

	var conflict pkg.EntityConflictError

	require.ErrorAs(t, err, &conflict)
	assert.Equal(t, constant.ErrTransactionAlreadyTransitioned.Error(), conflict.Code)
}

// TestPendingTransition_EmptyBodyRejectsBeforeAnyEffect proves the body guard
// runs ahead of every read and every evaluation: a row still marked PENDING whose
// body was already nulled by a terminal transition answers 0511, reads no
// balance, evaluates no script — and releases the lock, because nothing moved.
func TestPendingTransition_EmptyBodyRejectsBeforeAnyEffect(t *testing.T) {
	tran := pendingTransaction(false)
	tran.Body = mtransaction.Transaction{}

	ctrl := gomock.NewController(t)
	redisRepo := txRedis.NewMockRedisRepository(ctrl)

	redisRepo.EXPECT().GetBytes(gomock.Any(), gomock.Any()).Return(nil, errors.New("cache miss")).AnyTimes()
	redisRepo.EXPECT().SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil).Times(1)
	redisRepo.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).Times(1)

	// Nothing past the guard may happen.
	redisRepo.EXPECT().AddMessageToQueue(gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	redisRepo.EXPECT().ProcessBalanceAtomicOperation(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).Times(0)

	reader := &pendingReader{pending: tran}
	uc := &UseCase{TransactionRedisRepo: redisRepo, TransactionReader: reader}

	_, err := uc.CommitTransactionV1(context.Background(), pendingTransitionInputFor(tran))

	requireAlreadyTransitioned(t, err)
	assert.Zero(t, reader.getBalancesCalls, "an already-transitioned body must reject before any balance read")
}

// TestPendingTransition_NonPendingStillAnswers0099 pins the order of the two
// guards from the other side: a direct transaction carries no body either, so
// the status guard has to run FIRST or every commit of a non-PENDING row would
// change code from 0099 to 0511 — a contract break for callers that branch on it.
func TestPendingTransition_NonPendingStillAnswers0099(t *testing.T) {
	tran := pendingTransaction(false)
	tran.Body = mtransaction.Transaction{}
	tran.Status = transaction.Status{Code: constant.APPROVED}

	ctrl := gomock.NewController(t)
	redisRepo := txRedis.NewMockRedisRepository(ctrl)

	redisRepo.EXPECT().GetBytes(gomock.Any(), gomock.Any()).Return(nil, errors.New("cache miss")).AnyTimes()
	redisRepo.EXPECT().SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil).Times(1)
	redisRepo.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).Times(1)

	reader := &pendingReader{pending: tran}
	uc := &UseCase{TransactionRedisRepo: redisRepo, TransactionReader: reader}

	_, err := uc.CancelTransactionV1(context.Background(), pendingTransitionInputFor(tran))

	require.Error(t, err)

	var conflict pkg.EntityConflictError

	require.ErrorAs(t, err, &conflict)
	assert.Equal(t, constant.ErrCommitTransactionNotPending.Error(), conflict.Code)
}

// TestPendingTransition_PopulatedBodyPassesTheGuard is the negative control: a
// legitimate PENDING always carries the body the create persisted, so the guard
// is transparent and the transition runs on to the balance read.
func TestPendingTransition_PopulatedBodyPassesTheGuard(t *testing.T) {
	tran := pendingTransaction(false)

	ctrl := gomock.NewController(t)
	redisRepo := txRedis.NewMockRedisRepository(ctrl)

	redisRepo.EXPECT().GetBytes(gomock.Any(), gomock.Any()).Return(nil, errors.New("cache miss")).AnyTimes()
	redisRepo.EXPECT().SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil).Times(1)
	redisRepo.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).Times(1)

	reader := &pendingReader{pending: tran, balancesErr: errBalancesUnavailable}
	uc := &UseCase{TransactionRedisRepo: redisRepo, TransactionReader: reader}

	_, err := uc.CommitTransactionV1(context.Background(), pendingTransitionInputFor(tran))

	require.ErrorIs(t, err, errBalancesUnavailable)
	assert.Equal(t, 1, reader.getBalancesCalls, "a populated body must reach the balance read")
}

// corruptedResultBalance is a balance the operation builder cannot map: its
// OverdraftLimit is not a decimal, so ToTransactionBalance refuses the row. It is
// the cheapest way to fail BuildOperations AFTER the balances have already moved.
func corruptedResultBalance(alias string) *mmodel.Balance {
	limit := "not-a-decimal"

	return &mmodel.Balance{
		ID:        uuid.New().String(),
		Alias:     alias,
		Key:       constant.DefaultBalanceKey,
		Available: decimal.NewFromInt(100),
		OnHold:    decimal.Zero,
		Settings:  &mmodel.BalanceSettings{OverdraftLimit: &limit},
	}
}

// TestPendingTransition_BuildOperationsFailureKeepsLock proves the lock is NOT
// released once the balances have moved. BuildOperations sits past the atomic
// mutation, so releasing there let a retry re-enter a transaction whose money
// already moved and apply it a second time — the G7 window. The lock is held
// instead and its TTL is the cooldown.
func TestPendingTransition_BuildOperationsFailureKeepsLock(t *testing.T) {
	tran := pendingTransaction(false)

	ctrl := gomock.NewController(t)
	redisRepo := txRedis.NewMockRedisRepository(ctrl)

	corrupted := []*mmodel.Balance{corruptedResultBalance("@payer")}

	redisRepo.EXPECT().GetBytes(gomock.Any(), gomock.Any()).Return(nil, errors.New("cache miss")).AnyTimes()
	redisRepo.EXPECT().SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil).Times(1)
	redisRepo.EXPECT().AddMessageToQueue(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	redisRepo.EXPECT().ProcessBalanceAtomicOperation(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(&mmodel.BalanceAtomicResult{Before: corrupted, After: corrupted}, nil).Times(1)

	// The guard: no branch past the atomic mutation may delete the lock.
	redisRepo.EXPECT().Del(gomock.Any(), gomock.Any()).Times(0)

	reader := &pendingReader{pending: tran}
	uc := &UseCase{TransactionRedisRepo: redisRepo, TransactionReader: reader}

	_, err := uc.CommitTransactionV1(context.Background(), pendingTransitionInputFor(tran))

	require.Error(t, err, "a corrupted balance must fail the operation build")
	assert.Contains(t, err.Error(), "OverdraftLimit",
		"the run must stop inside BuildOperations, not somewhere later")
}

// TestPendingTransition_StatusCASConflictRejects proves the durable backstop: on
// the inline status flip the compare-and-set matches no PENDING row — another
// transition already settled the transaction — and the request is answered 0511
// instead of writing the second transition through.
func TestPendingTransition_StatusCASConflictRejects(t *testing.T) {
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "true")

	tran := pendingTransaction(false)

	ctrl := gomock.NewController(t)
	redisRepo := txRedis.NewMockRedisRepository(ctrl)

	redisRepo.EXPECT().GetBytes(gomock.Any(), gomock.Any()).Return(nil, errors.New("cache miss")).AnyTimes()
	redisRepo.EXPECT().SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil).AnyTimes()
	redisRepo.EXPECT().AddMessageToQueue(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	redisRepo.EXPECT().ReadMessageFromQueue(gomock.Any(), gomock.Any()).Return(nil, errors.New("no backup entry")).AnyTimes()
	redisRepo.EXPECT().ProcessBalanceAtomicOperation(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(&mmodel.BalanceAtomicResult{}, nil).AnyTimes()
	redisRepo.EXPECT().Del(gomock.Any(), gomock.Any()).Times(0)

	transactionRepo := transaction.NewMockRepository(ctrl)
	transactionRepo.EXPECT().
		UpdateStatusFromPending(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, false, nil).Times(1)

	// The generic Update keeps its PATCH semantics and is never the transition's
	// path: a call here would mean the compare-and-set was bypassed.
	transactionRepo.EXPECT().Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	reader := &pendingReader{pending: tran}
	uc := &UseCase{TransactionRedisRepo: redisRepo, TransactionReader: reader, TransactionRepo: transactionRepo}

	_, err := uc.CommitTransactionV1(context.Background(), pendingTransitionInputFor(tran))

	requireAlreadyTransitioned(t, err)
}

// TestCreateOrUpdateTransaction_TransitionCASIsIdempotent proves the backup
// consumer's opposite reading of the same zero-row result: the transition it
// carries was already applied (by the request path, or by a replay of this very
// message), so it reports a no-op and succeeds. Failing would send an
// already-settled transition to retry and then to the DLQ, and reporting an
// update would put a duplicate lifecycle event on the wire.
func TestCreateOrUpdateTransaction_TransitionCASIsIdempotent(t *testing.T) {
	tran := pendingTransaction(false)
	tran.Status = transaction.Status{Code: constant.APPROVED}

	ctrl := gomock.NewController(t)
	transactionRepo := transaction.NewMockRepository(ctrl)

	transactionRepo.EXPECT().Create(gomock.Any(), gomock.Any()).
		Return(nil, &pgconn.PgError{Code: constant.UniqueViolationCode}).Times(1)
	transactionRepo.EXPECT().
		UpdateStatusFromPending(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, false, nil).Times(1)

	uc := &UseCase{TransactionRepo: transactionRepo}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(context.Background())

	got, phase, err := uc.CreateOrUpdateTransaction(context.Background(), logger, tracer,
		transaction.TransactionProcessingPayload{
			Transaction: tran,
			Input:       &mtransaction.Transaction{},
			Validate:    &mtransaction.Responses{Pending: true},
		})

	require.NoError(t, err, "an already-applied transition must not fail the message")
	assert.Equal(t, TransactionLifecyclePhaseNoop, phase, "no state change means no lifecycle event")
	assert.NotNil(t, got)
}

// TestCreateOrUpdateTransaction_TransitionCASLands is the counterpart: the row is
// still PENDING, the compare-and-set flips it, and the consumer reports the
// update phase that drives the lifecycle event.
func TestCreateOrUpdateTransaction_TransitionCASLands(t *testing.T) {
	tran := pendingTransaction(false)
	tran.Status = transaction.Status{Code: constant.CANCELED}

	ctrl := gomock.NewController(t)
	transactionRepo := transaction.NewMockRepository(ctrl)

	transactionRepo.EXPECT().Create(gomock.Any(), gomock.Any()).
		Return(nil, &pgconn.PgError{Code: constant.UniqueViolationCode}).Times(1)
	transactionRepo.EXPECT().
		UpdateStatusFromPending(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(tran, true, nil).Times(1)

	uc := &UseCase{TransactionRepo: transactionRepo}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(context.Background())

	_, phase, err := uc.CreateOrUpdateTransaction(context.Background(), logger, tracer,
		transaction.TransactionProcessingPayload{
			Transaction: tran,
			Input:       &mtransaction.Transaction{},
			Validate:    &mtransaction.Responses{Pending: true},
		})

	require.NoError(t, err)
	assert.Equal(t, TransactionLifecyclePhaseUpdated, phase)
}
