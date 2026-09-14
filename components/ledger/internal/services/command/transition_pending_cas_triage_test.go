// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/rabbitmq"
	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
)

// =============================================================================
// LOST STATUS COMPARE-AND-SET — TRIAGE OF WHAT ZERO ROWS MEANS
// =============================================================================
// In the async write mode a commit/cancel is served from the write-behind cache,
// so it can run before the create has inserted its row. The status
// compare-and-set then matches zero rows for a reason opposite to the one it
// matches zero rows for when a rival transition already settled the transaction:
// the first is a transition the async pipeline still has to settle, the second is
// a conflict. These tests pin which answer each outcome gets.

// errStatusReadUnavailable stands in for a technical failure of the triage read.
var errStatusReadUnavailable = errors.New("transaction read unavailable")

// primaryReadCtx matches only a context carrying the primary-read intent, so a
// triage read routed at a replica fails the expectation instead of passing
// silently: the row it looks for was written moments ago and a replica may not
// carry it yet.
type primaryReadCtx struct {
	seen *bool
}

func (m primaryReadCtx) Matches(x any) bool {
	ctx, ok := x.(context.Context)
	if !ok {
		return false
	}

	if !readrouting.IsPrimaryRead(ctx) {
		return false
	}

	*m.seen = true

	return true
}

func (m primaryReadCtx) String() string {
	return "a context carrying primary-read intent"
}

// triageCase describes one outcome of the triage: how the repository answers the
// status write and the read behind it.
type triageCase struct {
	// firstCAS is what the inline compare-and-set reports.
	firstCAS bool
	// found is the row the triage read answers with, when findErr is nil.
	found *transaction.Transaction
	// findErr is the error the triage read answers with.
	findErr error
	// retryCAS is what the retried compare-and-set reports, for the outcome that
	// retries.
	retryCAS bool
	// retryCASErr is the error the retried compare-and-set answers with.
	retryCASErr error
	// expectFind is whether the triage read must happen at all.
	expectFind bool
	// expectRetry is whether the compare-and-set must be retried.
	expectRetry bool
	// expectWrite is whether the run must reach the transaction write.
	expectWrite bool
}

// triageUseCase wires a transition that runs past the balance commit and lands on
// the status compare-and-set the case describes. It returns the use case, the
// emitter it publishes through and the flag the primary-read matcher sets.
func triageUseCase(t *testing.T, tran *transaction.Transaction, tc triageCase) (*UseCase, *pkgStreaming.MockEmitter, *bool) {
	t.Helper()

	ctrl := gomock.NewController(t)
	redisRepo := txRedis.NewMockRedisRepository(ctrl)

	redisRepo.EXPECT().GetBytes(gomock.Any(), gomock.Any()).Return(nil, errors.New("cache miss")).AnyTimes()
	redisRepo.EXPECT().SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil).AnyTimes()
	redisRepo.EXPECT().AddMessageToQueue(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	redisRepo.EXPECT().ReadMessageFromQueue(gomock.Any(), gomock.Any()).Return(nil, errors.New("no backup entry")).AnyTimes()
	redisRepo.EXPECT().ProcessBalanceAtomicOperation(
		gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
	).Return(&mmodel.BalanceAtomicResult{}, nil).AnyTimes()

	// The balances moved, so no branch past the atomic mutation releases the lock.
	redisRepo.EXPECT().Del(gomock.Any(), gomock.Any()).Times(0)

	transactionRepo := transaction.NewMockRepository(ctrl)

	casCall := transactionRepo.EXPECT().
		UpdateStatusFromPending(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(tran, tc.firstCAS, nil).Times(1)

	seenPrimaryRead := false

	if tc.expectFind {
		findCall := transactionRepo.EXPECT().
			Find(primaryReadCtx{seen: &seenPrimaryRead}, gomock.Any(), gomock.Any(), gomock.Any()).
			Return(tc.found, tc.findErr).Times(1).After(casCall)

		if tc.expectRetry {
			transactionRepo.EXPECT().
				UpdateStatusFromPending(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
				Return(tran, tc.retryCAS, tc.retryCASErr).Times(1).After(findCall)
		}
	} else {
		transactionRepo.EXPECT().Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)
	}

	transactionRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil, errWriteUnavailable).AnyTimes()

	// The async write mode publishes the transaction instead of inserting it. It
	// fails on purpose: reaching it is the proof the run carried on, and the error
	// it raises is the one the caller must see instead of a false conflict.
	writeTimes := 1
	if !tc.expectWrite {
		writeTimes = 0
	}

	producer := rabbitmq.NewMockProducerRepository(ctrl)
	producer.EXPECT().ProducerDefaultWithContext(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, errWriteUnavailable).Times(writeTimes)

	emitter := pkgStreaming.NewMockEmitter()

	uc := &UseCase{
		TransactionRedisRepo: redisRepo,
		TransactionReader:    &pendingReader{pending: tran},
		TransactionRepo:      transactionRepo,
		RabbitMQRepo:         producer,
		Streaming:            emitter,
	}

	return uc, emitter, &seenPrimaryRead
}

// requireNotAlreadyTransitioned asserts the error, whatever it is, is not the
// already-transitioned conflict: reporting one here would tell the caller another
// transition settled the transaction when nothing of the sort is known.
func requireNotAlreadyTransitioned(t *testing.T, err error) {
	t.Helper()

	var conflict pkg.EntityConflictError

	if errors.As(err, &conflict) {
		assert.NotEqual(t, constant.ErrTransactionAlreadyTransitioned.Error(), conflict.Code,
			"a transition whose row is missing or unreadable must not be reported as already transitioned")
	}
}

// settledRow is the row the triage read answers with for a given status.
func settledRow(tran *transaction.Transaction, status string) *transaction.Transaction {
	return &transaction.Transaction{
		ID:             tran.ID,
		OrganizationID: tran.OrganizationID,
		LedgerID:       tran.LedgerID,
		Status:         transaction.Status{Code: status},
	}
}

// TestPendingTransitionTriage_MissingRowDefersToAsyncPipeline covers outcome (a):
// the create has not inserted the row yet, so the compare-and-set had nothing to
// match. The transition is not a conflict — it carries on to the transaction
// write, where the queued message lets the consumer settle the status and own the
// lifecycle event.
func TestPendingTransitionTriage_MissingRowDefersToAsyncPipeline(t *testing.T) {
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "true")

	tran := pendingTransaction(false)

	uc, emitter, seenPrimaryRead := triageUseCase(t, tran, triageCase{
		firstCAS:    false,
		findErr:     pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityTransaction),
		expectFind:  true,
		expectWrite: true,
	})

	_, err := uc.CommitTransactionV1(context.Background(), pendingTransitionInputFor(tran))

	requireNotAlreadyTransitioned(t, err)

	assert.True(t, *seenPrimaryRead,
		"the triage read must be routed at the primary: the row it looks for was written moments ago")
	assert.Empty(t, emitter.Events(),
		"the consumer that settles the deferred status owns the lifecycle event")
}

// TestPendingTransitionTriage_RowLandedRetryWinsEmitsOnce covers outcome (b): the
// row landed between the compare-and-set and the read, the retry wins, and this
// call is therefore the writer that owns the lifecycle event.
func TestPendingTransitionTriage_RowLandedRetryWinsEmitsOnce(t *testing.T) {
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "true")

	tran := pendingTransaction(false)

	uc, emitter, _ := triageUseCase(t, tran, triageCase{
		firstCAS:    false,
		found:       settledRow(tran, constant.PENDING),
		retryCAS:    true,
		expectFind:  true,
		expectRetry: true,
		expectWrite: true,
	})

	_, err := uc.CommitTransactionV1(context.Background(), pendingTransitionInputFor(tran))

	requireNotAlreadyTransitioned(t, err)

	pkgStreaming.AssertEventEmitted(t, emitter, "transaction", "committed")
	assert.Len(t, emitter.Events(), 1,
		"the retried compare-and-set won once, so the fact goes on the wire once")
}

// TestPendingTransitionTriage_RowLandedRetryLosesStaysSilent covers outcome (c):
// the retry still matches no PENDING row. No rival can settle the row while this
// transition holds the lock, so the status is unknown rather than known-lost —
// the run carries on without emitting and without answering a conflict.
func TestPendingTransitionTriage_RowLandedRetryLosesStaysSilent(t *testing.T) {
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "true")

	tran := pendingTransaction(false)

	uc, emitter, _ := triageUseCase(t, tran, triageCase{
		firstCAS:    false,
		found:       settledRow(tran, constant.PENDING),
		retryCAS:    false,
		expectFind:  true,
		expectRetry: true,
		expectWrite: true,
	})

	_, err := uc.CommitTransactionV1(context.Background(), pendingTransitionInputFor(tran))

	requireNotAlreadyTransitioned(t, err)

	assert.Empty(t, emitter.Events(),
		"a transition that never won the status write must not publish the fact")
}

// TestPendingTransitionTriage_RetryErrorStaysSilent is the technical-failure twin
// of the outcome above: the retried write itself fails, which says nothing about
// who settled the row.
func TestPendingTransitionTriage_RetryErrorStaysSilent(t *testing.T) {
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "true")

	tran := pendingTransaction(false)

	uc, emitter, _ := triageUseCase(t, tran, triageCase{
		firstCAS:    false,
		found:       settledRow(tran, constant.PENDING),
		retryCASErr: errors.New("status update unavailable"),
		expectFind:  true,
		expectRetry: true,
		expectWrite: true,
	})

	_, err := uc.CommitTransactionV1(context.Background(), pendingTransitionInputFor(tran))

	requireNotAlreadyTransitioned(t, err)

	assert.Empty(t, emitter.Events(),
		"a status write that failed technically published nothing to claim")
}

// TestPendingTransitionTriage_TerminalRowRejects covers outcome (d): the row
// exists and is already terminal, which is the conflict 0511 describes. It is
// answered before the transaction write, so the losing transition never queues a
// status the winner already settled.
func TestPendingTransitionTriage_TerminalRowRejects(t *testing.T) {
	testCases := []struct {
		name   string
		status string
	}{
		{name: "already committed", status: constant.APPROVED},
		{name: "already canceled", status: constant.CANCELED},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "true")

			tran := pendingTransaction(false)

			uc, emitter, _ := triageUseCase(t, tran, triageCase{
				firstCAS:    false,
				found:       settledRow(tran, tc.status),
				expectFind:  true,
				expectWrite: false,
			})

			_, err := uc.CommitTransactionV1(context.Background(), pendingTransitionInputFor(tran))

			requireAlreadyTransitioned(t, err)

			assert.Empty(t, emitter.Events(),
				"the transition that lost the race must not publish the winner's fact again")
		})
	}
}

// TestPendingTransitionTriage_ReadErrorDoesNotMasqueradeAsConflict covers outcome
// (e): the triage read failed, so the row's status is unknown. Answering a
// conflict on an unanswered read would report a transition that may not have
// happened.
func TestPendingTransitionTriage_ReadErrorDoesNotMasqueradeAsConflict(t *testing.T) {
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "true")

	tran := pendingTransaction(false)

	uc, emitter, _ := triageUseCase(t, tran, triageCase{
		firstCAS:    false,
		findErr:     errStatusReadUnavailable,
		expectFind:  true,
		expectWrite: true,
	})

	_, err := uc.CommitTransactionV1(context.Background(), pendingTransitionInputFor(tran))

	requireNotAlreadyTransitioned(t, err)

	assert.Empty(t, emitter.Events(),
		"an unreadable status is not a status this call settled")
}

// TestPendingTransitionTriage_WinningCASSkipsTheRead covers outcome (f): the
// inline compare-and-set won, so there is nothing to triage. The extra primary
// read must not be paid on the path every healthy transition takes.
func TestPendingTransitionTriage_WinningCASSkipsTheRead(t *testing.T) {
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "true")

	tran := pendingTransaction(false)

	uc, emitter, _ := triageUseCase(t, tran, triageCase{
		firstCAS:    true,
		expectFind:  false,
		expectWrite: true,
	})

	_, err := uc.CommitTransactionV1(context.Background(), pendingTransitionInputFor(tran))

	requireNotAlreadyTransitioned(t, err)

	pkgStreaming.AssertEventEmitted(t, emitter, "transaction", "committed")
	assert.Len(t, emitter.Events(), 1,
		"the winner of the inline compare-and-set publishes the fact exactly once")
}

// TestPendingTransitionTriage_CancelMissingRowDefersToAsyncPipeline mirrors
// outcome (a) on the cancel half of the transition: both share the triage, and
// the cancel is the one that races hardest with the create in practice.
func TestPendingTransitionTriage_CancelMissingRowDefersToAsyncPipeline(t *testing.T) {
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "true")

	tran := pendingTransaction(false)

	uc, emitter, seenPrimaryRead := triageUseCase(t, tran, triageCase{
		firstCAS:    false,
		findErr:     pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityTransaction),
		expectFind:  true,
		expectWrite: true,
	})

	_, err := uc.CancelTransactionV1(context.Background(), pendingTransitionInputFor(tran))

	requireNotAlreadyTransitioned(t, err)

	assert.True(t, *seenPrimaryRead, "the cancel triage reads the primary as the commit one does")
	assert.Empty(t, emitter.Events(), "the consumer settles the deferred cancel and emits it")
}

// TestPendingTransitionTriage_ReadAddressesTheTransactionRow pins what the
// triage read is addressed with: the scope of the request and the transaction's
// own id, so the outcome it decides belongs to this transaction.
func TestPendingTransitionTriage_ReadAddressesTheTransactionRow(t *testing.T) {
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

	var (
		gotOrganizationID uuid.UUID
		gotLedgerID       uuid.UUID
		gotTransactionID  uuid.UUID
	)

	transactionRepo := transaction.NewMockRepository(ctrl)
	transactionRepo.EXPECT().
		UpdateStatusFromPending(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(tran, false, nil).Times(1)
	transactionRepo.EXPECT().
		Find(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, organizationID, ledgerID, id uuid.UUID) (*transaction.Transaction, error) {
			gotOrganizationID, gotLedgerID, gotTransactionID = organizationID, ledgerID, id

			return nil, pkg.ValidateBusinessError(constant.ErrEntityNotFound, constant.EntityTransaction)
		}).Times(1)
	transactionRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil, errWriteUnavailable).AnyTimes()

	producer := rabbitmq.NewMockProducerRepository(ctrl)
	producer.EXPECT().ProducerDefaultWithContext(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, errWriteUnavailable).Times(1)

	uc := &UseCase{
		TransactionRedisRepo: redisRepo,
		TransactionReader:    &pendingReader{pending: tran},
		TransactionRepo:      transactionRepo,
		RabbitMQRepo:         producer,
	}

	in := pendingTransitionInputFor(tran)

	_, _ = uc.CommitTransactionV1(context.Background(), in)

	require.Equal(t, in.OrganizationID, gotOrganizationID)
	require.Equal(t, in.LedgerID, gotLedgerID)
	require.Equal(t, in.TransactionID, gotTransactionID)
}
