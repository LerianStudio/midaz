// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
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

// errLuaUnavailable stands in for a failure of the atomic balance mutation.
var errLuaUnavailable = errors.New("atomic balance operation unavailable")

// errWriteUnavailable stands in for a persistence failure after the balances moved.
var errWriteUnavailable = errors.New("transaction write unavailable")

// pendingReader is a TransactionReader that answers the transition's three reads from
// fixed values, so a commit/cancel runs without the persistence stack behind it.
type pendingReader struct {
	versionReader

	pending          *transaction.Transaction
	getBalancesCalls int
	balancesErr      error
}

func (r *pendingReader) GetWriteBehindTransaction(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*transaction.Transaction, error) {
	return r.pending, nil
}

func (r *pendingReader) GetBalances(context.Context, uuid.UUID, uuid.UUID, []string) ([]*mmodel.Balance, error) {
	r.getBalancesCalls++

	return nil, r.balancesErr
}

func (r *pendingReader) ValidateAccountingRules(context.Context, uuid.UUID, uuid.UUID, []mmodel.BalanceOperation, *mtransaction.Responses, string) (*mmodel.TransactionRouteCache, error) {
	return nil, nil
}

// pendingTransaction builds the minimal PENDING transaction the transition accepts: a
// balanced single-leg body and the PENDING status the not-pending guard checks.
func pendingTransaction(skipTracer bool) *transaction.Transaction {
	amount := decimal.NewFromInt(100)

	body := mtransaction.Transaction{
		Send: mtransaction.Send{
			Asset: "BRL",
			Value: amount,
			Source: mtransaction.Source{
				From: []mtransaction.FromTo{{
					AccountAlias: "@payer",
					IsFrom:       true,
					Amount:       &mtransaction.Amount{Asset: "BRL", Value: amount},
				}},
			},
			Distribute: mtransaction.Distribute{
				To: []mtransaction.FromTo{{
					AccountAlias: "@payee",
					Amount:       &mtransaction.Amount{Asset: "BRL", Value: amount},
				}},
			},
		},
	}

	if skipTracer {
		body.Skip = &mtransaction.TransactionSkip{Tracer: true}
	}

	return &transaction.Transaction{
		ID:             uuid.New().String(),
		OrganizationID: uuid.New().String(),
		LedgerID:       uuid.New().String(),
		AssetCode:      "BRL",
		Amount:         &amount,
		Status:         transaction.Status{Code: constant.PENDING},
		Body:           body,
	}
}

// pendingTransitionInputFor names the transaction the fake reader will answer with.
func pendingTransitionInputFor(tran *transaction.Transaction) PendingTransitionInput {
	return PendingTransitionInput{
		OrganizationID: uuid.MustParse(tran.OrganizationID),
		LedgerID:       uuid.MustParse(tran.LedgerID),
		TransactionID:  tran.IDtoUUID(),
	}
}

// TestPendingTransition_LockHeldRejects proves a transition that loses the Redis lock
// answers 422 ErrPendingTransactionLocked and reads nothing further.
func TestPendingTransition_LockHeldRejects(t *testing.T) {
	tran := pendingTransaction(false)

	ctrl := gomock.NewController(t)
	redisRepo := txRedis.NewMockRedisRepository(ctrl)

	redisRepo.EXPECT().GetBytes(gomock.Any(), gomock.Any()).Return(nil, errors.New("cache miss")).AnyTimes()
	redisRepo.EXPECT().SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(false, nil).Times(1)
	redisRepo.EXPECT().Del(gomock.Any(), gomock.Any()).Times(0)

	reader := &pendingReader{pending: tran}
	uc := &UseCase{TransactionRedisRepo: redisRepo, TransactionReader: reader}

	_, err := uc.CommitTransactionV1(context.Background(), pendingTransitionInputFor(tran))

	require.Error(t, err)

	var conflict pkg.EntityConflictError

	require.ErrorAs(t, err, &conflict)
	assert.Equal(t, constant.ErrPendingTransactionLocked.Error(), conflict.Code)
	assert.Zero(t, reader.getBalancesCalls, "a lost lock must reject before any balance read")
}

// TestPendingTransition_RevokedTracerSkipIsNotHonored proves the commit/cancel
// re-resolution of the persisted skip never turns a revoked opt-in into a rejection:
// the body asks to skip the tracer, the ledger no longer permits it, and the transition
// runs on to the balance read instead of answering 422. Authorization was enforced at
// create; re-enforcing it here would fail a transaction whose balances are already held.
func TestPendingTransition_RevokedTracerSkipIsNotHonored(t *testing.T) {
	tran := pendingTransaction(true)

	ctrl := gomock.NewController(t)
	redisRepo := txRedis.NewMockRedisRepository(ctrl)

	redisRepo.EXPECT().GetBytes(gomock.Any(), gomock.Any()).Return(nil, errors.New("cache miss")).AnyTimes()
	redisRepo.EXPECT().SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil).Times(1)
	redisRepo.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).Times(1)

	reader := &pendingReader{pending: tran, balancesErr: errBalancesUnavailable}
	// The ledger opts into no skip, so the persisted opt-in is no longer permitted.
	reader.settings = mmodel.LedgerSettings{}

	uc := &UseCase{TransactionRedisRepo: redisRepo, TransactionReader: reader}

	_, err := uc.CommitTransactionV2(context.Background(), pendingTransitionInputFor(tran))

	require.ErrorIs(t, err, errBalancesUnavailable, "a revoked skip must not reject the transition")
	assert.Equal(t, 1, reader.getBalancesCalls, "the transition must reach the balance read")
}

// TestPendingTransition_BalanceFailureRemovesSeedAndReleasesLock proves a failed atomic
// mutation unwinds both compensations: the pre-seeded backup entry is removed and the
// lock is released, so a retry can run.
func TestPendingTransition_BalanceFailureRemovesSeedAndReleasesLock(t *testing.T) {
	tran := pendingTransaction(false)

	ctrl := gomock.NewController(t)
	redisRepo := txRedis.NewMockRedisRepository(ctrl)

	redisRepo.EXPECT().GetBytes(gomock.Any(), gomock.Any()).Return(nil, errors.New("cache miss")).AnyTimes()
	redisRepo.EXPECT().SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil).Times(1)
	redisRepo.EXPECT().AddMessageToQueue(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).Times(1)
	redisRepo.EXPECT().ProcessBalanceAtomicOperation(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, errLuaUnavailable).Times(1)
	redisRepo.EXPECT().RemoveMessageFromQueue(gomock.Any(), gomock.Any()).Return(nil).Times(1)
	redisRepo.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).Times(1)

	reader := &pendingReader{pending: tran}
	uc := &UseCase{TransactionRedisRepo: redisRepo, TransactionReader: reader}

	_, err := uc.CommitTransactionV1(context.Background(), pendingTransitionInputFor(tran))

	require.ErrorIs(t, err, errLuaUnavailable)
}

// TestPendingTransition_WriteFailureKeepsLock proves the one branch that deliberately
// does NOT release the lock: past the atomic mutation the balances have already moved,
// so a persistence failure is reconciled by the backup queue rather than by a retry.
func TestPendingTransition_WriteFailureKeepsLock(t *testing.T) {
	tran := pendingTransaction(false)

	uc, redisRepo, _ := newCommittingUseCase(t, tran)
	redisRepo.EXPECT().Del(gomock.Any(), gomock.Any()).Times(0)

	_, err := uc.CommitTransactionV1(context.Background(), pendingTransitionInputFor(tran))

	require.Error(t, err)

	var internal pkg.InternalServerError

	require.ErrorAs(t, err, &internal)
	assert.Equal(t, constant.ErrMessageBrokerUnavailable.Error(), internal.Code)
}

// TestPendingTransition_V1NeverDrivesTheReservationLifecycle proves the /v1 contract
// carries no tracer: neither transition addresses the tracer by transaction id, even
// with an enforcing ledger and an injected reserver.
func TestPendingTransition_V1NeverDrivesTheReservationLifecycle(t *testing.T) {
	t.Run("commit", func(t *testing.T) {
		tran := pendingTransaction(false)

		uc, _, reserver := newCommittingUseCase(t, tran)

		_, _ = uc.CommitTransactionV1(context.Background(), pendingTransitionInputFor(tran))

		assert.Empty(t, reserver.confirmedTxns, "a /v1 commit must not confirm reservations")
		assert.Empty(t, reserver.releasedTxns)
	})

	t.Run("cancel", func(t *testing.T) {
		tran := pendingTransaction(false)

		uc, _, reserver := newCommittingUseCase(t, tran)

		_, _ = uc.CancelTransactionV1(context.Background(), pendingTransitionInputFor(tran))

		assert.Empty(t, reserver.releasedTxns, "a /v1 cancel must not release reservations")
		assert.Empty(t, reserver.confirmedTxns)
	})
}

// TestPendingTransition_V2DrivesTheReservationLifecycle is the counterpart: the /v2
// commit confirms every reservation the transaction holds and the /v2 cancel releases
// them, both addressed by transaction id and both after the balances moved.
func TestPendingTransition_V2DrivesTheReservationLifecycle(t *testing.T) {
	t.Run("commit confirms", func(t *testing.T) {
		tran := pendingTransaction(false)

		uc, _, reserver := newCommittingUseCase(t, tran)

		_, _ = uc.CommitTransactionV2(context.Background(), pendingTransitionInputFor(tran))

		assert.Equal(t, []uuid.UUID{tran.IDtoUUID()}, reserver.confirmedTxns)
		assert.Empty(t, reserver.releasedTxns)
	})

	t.Run("cancel releases", func(t *testing.T) {
		tran := pendingTransaction(false)

		uc, _, reserver := newCommittingUseCase(t, tran)

		_, _ = uc.CancelTransactionV2(context.Background(), pendingTransitionInputFor(tran))

		assert.Equal(t, []uuid.UUID{tran.IDtoUUID()}, reserver.releasedTxns)
		assert.Empty(t, reserver.confirmedTxns)
	})
}

// newCommittingUseCase wires a UseCase whose lock is free, whose backup queue accepts
// the seed and whose atomic mutation succeeds, so the transition runs past the balance
// commit and stops at the transaction write. The ledger enforces the tracer and a
// reserver is injected, so a pipeline that names the by-transaction seams reaches them.
func newCommittingUseCase(t *testing.T, tran *transaction.Transaction) (*UseCase, *txRedis.MockRedisRepository, *stubReserver) {
	t.Helper()

	ctrl := gomock.NewController(t)
	redisRepo := txRedis.NewMockRedisRepository(ctrl)

	redisRepo.EXPECT().GetBytes(gomock.Any(), gomock.Any()).Return(nil, errors.New("cache miss")).AnyTimes()
	redisRepo.EXPECT().SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil).AnyTimes()
	redisRepo.EXPECT().AddMessageToQueue(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	redisRepo.EXPECT().ReadMessageFromQueue(gomock.Any(), gomock.Any()).Return(nil, errors.New("no backup entry")).AnyTimes()
	redisRepo.EXPECT().ProcessBalanceAtomicOperation(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&mmodel.BalanceAtomicResult{}, nil).AnyTimes()

	reserver := &stubReserver{}

	settings := mmodel.LedgerSettings{}
	settings.Tracer = mmodel.TracerSettings{Mode: mmodel.TracerModeEnforce}

	reader := &pendingReader{pending: tran}
	reader.settings = settings

	transactionRepo := transaction.NewMockRepository(ctrl)
	transactionRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(nil, errWriteUnavailable).AnyTimes()

	uc := &UseCase{
		TransactionRedisRepo: redisRepo,
		TransactionReader:    reader,
		TransactionRepo:      transactionRepo,
		TracerReserver:       reserver,
	}

	return uc, redisRepo, reserver
}
