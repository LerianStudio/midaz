// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/rabbitmq"
	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
)

// =============================================================================
// TRANSITION LIFECYCLE EVENT — EXACTLY ONCE ACROSS BOTH WRITE MODES
// =============================================================================
// transaction.committed and transaction.canceled describe the status
// transition, so the writer that WINS the status compare-and-set owns the
// emission. In the async write mode that winner is the request path; in the sync
// mode it is the backup consumer. The loser reports a no-op and stays silent,
// which is what keeps the fact on the wire exactly once instead of twice or —
// the regression these tests pin — not at all.

// committingUseCaseWithEmitter wires a transition that runs past the balance
// commit with a streaming emitter attached and a compare-and-set that lands.
func committingUseCaseWithEmitter(t *testing.T, tran *transaction.Transaction, emitter *pkgStreaming.MockEmitter) *UseCase {
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
	redisRepo.EXPECT().Del(gomock.Any(), gomock.Any()).AnyTimes()

	transactionRepo := transaction.NewMockRepository(ctrl)
	transactionRepo.EXPECT().
		UpdateStatusFromPending(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(tran, true, nil).AnyTimes()
	transactionRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil, errWriteUnavailable).AnyTimes()

	// The async write mode publishes the transaction instead of inserting it.
	// It fails here on purpose: the emission decision is made before the write,
	// and the run must not depend on the write landing to have emitted.
	producer := rabbitmq.NewMockProducerRepository(ctrl)
	producer.EXPECT().ProducerDefaultWithContext(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, errWriteUnavailable).AnyTimes()

	reader := &pendingReader{pending: tran}

	return &UseCase{
		TransactionRedisRepo: redisRepo,
		TransactionReader:    reader,
		TransactionRepo:      transactionRepo,
		RabbitMQRepo:         producer,
		Streaming:            emitter,
	}
}

// TestTransitionLifecycleEvent_AsyncWinnerEmits proves the request path emits
// when its compare-and-set wins in async write mode. Before this, the async mode
// emitted nothing at all: the consumer is the only other emitter and it finds
// the row already settled.
func TestTransitionLifecycleEvent_AsyncWinnerEmits(t *testing.T) {
	testCases := []struct {
		name      string
		eventType string
		commit    func(*UseCase, PendingTransitionInput) (*transaction.Transaction, error)
	}{
		{
			name:      "commit emits transaction.committed",
			eventType: "committed",
			commit: func(uc *UseCase, in PendingTransitionInput) (*transaction.Transaction, error) {
				return uc.CommitTransactionV1(context.Background(), in)
			},
		},
		{
			name:      "cancel emits transaction.canceled",
			eventType: "canceled",
			commit: func(uc *UseCase, in PendingTransitionInput) (*transaction.Transaction, error) {
				return uc.CancelTransactionV1(context.Background(), in)
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "true")

			tran := pendingTransaction(false)
			emitter := pkgStreaming.NewMockEmitter()

			uc := committingUseCaseWithEmitter(t, tran, emitter)

			_, _ = tc.commit(uc, pendingTransitionInputFor(tran))

			pkgStreaming.AssertEventEmitted(t, emitter, "transaction", tc.eventType)
		})
	}
}

// TestTransitionLifecycleEvent_SyncModeLeavesTheEmissionToTheConsumer is the
// counterpart: with the async write mode off, the request path never writes the
// status and therefore never emits. The consumer is the writer, and emitting
// here as well would put the same fact on the wire twice.
func TestTransitionLifecycleEvent_SyncModeLeavesTheEmissionToTheConsumer(t *testing.T) {
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	tran := pendingTransaction(false)
	emitter := pkgStreaming.NewMockEmitter()

	uc := committingUseCaseWithEmitter(t, tran, emitter)

	_, _ = uc.CommitTransactionV1(context.Background(), pendingTransitionInputFor(tran))

	assert.Empty(t, emitter.Events(),
		"in sync write mode the consumer owns the status write and the emission")
}

// TestCreateOrUpdateTransaction_LostCASEmitsNothing pins the consumer half of
// the contract: a compare-and-set that matched no PENDING row means another
// writer already settled the transaction and already emitted, so this one
// reports a no-op — the phase SendTransactionEvents answers with silence.
func TestCreateOrUpdateTransaction_LostCASEmitsNothing(t *testing.T) {
	tran := pendingTransaction(false)
	tran.Status = transaction.Status{Code: constant.APPROVED}

	ctrl := gomock.NewController(t)
	transactionRepo := transaction.NewMockRepository(ctrl)

	transactionRepo.EXPECT().Create(gomock.Any(), gomock.Any()).
		Return(nil, &pgconn.PgError{Code: constant.UniqueViolationCode}).Times(1)
	transactionRepo.EXPECT().
		UpdateStatusFromPending(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, false, nil).Times(1)

	emitter := pkgStreaming.NewMockEmitter()
	uc := &UseCase{TransactionRepo: transactionRepo, Streaming: emitter}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(context.Background())

	got, phase, err := uc.CreateOrUpdateTransaction(context.Background(), logger, tracer,
		transaction.TransactionProcessingPayload{
			Transaction: tran,
			Input:       &mtransaction.Transaction{},
			Validate:    &mtransaction.Responses{Pending: true},
		})

	require.NoError(t, err)
	require.Equal(t, TransactionLifecyclePhaseNoop, phase)

	uc.SendTransactionEvents(context.Background(), got, phase)

	assert.Empty(t, emitter.Events(),
		"the writer that lost the compare-and-set must not re-emit a fact the winner already published")
}

// TestCreateOrUpdateTransaction_StatusCASErrorPropagates proves the consumer
// treats a technical failure of the status write as retryable. Swallowing it
// would acknowledge a message whose transition never landed; the phase stays
// no-op so no event is published for a state change that did not happen.
func TestCreateOrUpdateTransaction_StatusCASErrorPropagates(t *testing.T) {
	tran := pendingTransaction(false)
	tran.Status = transaction.Status{Code: constant.CANCELED}
	dbErr := errors.New("status update unavailable")

	ctrl := gomock.NewController(t)
	transactionRepo := transaction.NewMockRepository(ctrl)

	transactionRepo.EXPECT().Create(gomock.Any(), gomock.Any()).
		Return(nil, &pgconn.PgError{Code: constant.UniqueViolationCode}).Times(1)
	transactionRepo.EXPECT().
		UpdateStatusFromPending(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, false, dbErr).Times(1)

	uc := &UseCase{TransactionRepo: transactionRepo}

	logger, tracer, _, _ := libObservability.NewTrackingFromContext(context.Background())

	got, phase, err := uc.CreateOrUpdateTransaction(context.Background(), logger, tracer,
		transaction.TransactionProcessingPayload{
			Transaction: tran,
			Input:       &mtransaction.Transaction{},
			Validate:    &mtransaction.Responses{Pending: true},
		})

	require.ErrorIs(t, err, dbErr, "a technical failure must reach the broker as a retryable message")
	assert.Equal(t, TransactionLifecyclePhaseNoop, phase)
	assert.Nil(t, got)
}
