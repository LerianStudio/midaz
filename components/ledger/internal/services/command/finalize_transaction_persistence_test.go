// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/revertclaim"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	transactionredis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

func TestFinalizeDurableTransactionPersistence_RetryAfterLostCleanupUsesOneTerminalHandoff(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	transactionRepo := transaction.NewMockRepository(ctrl)
	claimRepo := revertclaim.NewMockRepository(ctrl)
	redisRepo := transactionredis.NewMockRedisRepository(ctrl)
	organizationID := uuid.New()
	ledgerID := uuid.New()
	originID := uuid.New()
	reverseID := uuid.New()
	operationID := uuid.NewString()
	originString := originID.String()
	created := constant.CREATED
	approved := constant.APPROVED
	replay := &transaction.Transaction{
		ID:                  reverseID.String(),
		OrganizationID:      organizationID.String(),
		LedgerID:            ledgerID.String(),
		ParentTransactionID: &originString,
		Status:              transaction.Status{Code: created},
		Operations:          []*operation.Operation{{ID: operationID}},
	}
	persisted := *replay
	persisted.Status = transaction.Status{Code: approved}
	legacyKey := utils.IdempotencyInternalKey(organizationID, ledgerID, "legacy")
	legacyOwner := reverseID.String()
	claim := &revertclaim.Claim{
		OrganizationID:       organizationID,
		LedgerID:             ledgerID,
		OriginTransactionID:  originID,
		ReverseTransactionID: reverseID,
		LegacyFenceKey:       &legacyKey,
		LegacyFenceOwner:     &legacyOwner,
		State:                revertclaim.StateMutated,
	}
	payload := transaction.TransactionProcessingPayload{
		Transaction:     replay,
		Validate:        &mtransaction.Responses{Pending: false},
		AttemptOwner:    reverseID.String(),
		ExpectedOutcome: mmodel.TransactionOutcomeCommitted,
	}
	originKey := utils.IdempotencyInternalKey(organizationID, ledgerID,
		libCommons.HashSHA256(utils.RevertIdempotencyHashSource(originID)))
	attempt := mmodel.BalanceExecutionAttempt{
		ExecutionKey: utils.TransactionBalanceExecutionKey(organizationID, ledgerID, reverseID),
		OutcomeKey:   utils.TransactionBalanceOutcomeKey(organizationID, ledgerID, reverseID),
		Owner:        reverseID.String(),
		Outcome:      mmodel.TransactionOutcomeCommitted,
		Identity:     reverseID,
	}
	encoded, err := json.Marshal(replay)
	require.NoError(t, err)

	for range 2 {
		transactionRepo.EXPECT().FindWithOperations(gomock.Any(), organizationID, ledgerID, reverseID).
			DoAndReturn(func(ctx context.Context, _, _ uuid.UUID, _ uuid.UUID) (*transaction.Transaction, error) {
				require.True(t, readrouting.IsPrimaryRead(ctx))
				return &persisted, nil
			})
		claimRepo.EXPECT().Claim(gomock.Any(), organizationID, ledgerID, originID, reverseID, nil, nil).
			Return(claim, false, nil)
	}

	firstCleanupErr := errors.New("lost cleanup response")
	gomock.InOrder(
		claimRepo.EXPECT().Transition(gomock.Any(), organizationID, ledgerID, originID, reverseID, revertclaim.StateCompleted, nil).Return(nil),
		redisRepo.EXPECT().CompleteOwnedKey(gomock.Any(), originKey, reverseID.String(), gomock.Any(), gomock.Any()).Return(true, nil),
		redisRepo.EXPECT().CompleteOwnedKey(gomock.Any(), legacyKey, reverseID.String(), gomock.Any(), gomock.Any()).Return(true, nil),
		redisRepo.EXPECT().FinalizeTransactionPersistence(gomock.Any(), organizationID, ledgerID, reverseID, attempt,
			[]string{operationID}).Return(firstCleanupErr),
		claimRepo.EXPECT().Transition(gomock.Any(), organizationID, ledgerID, originID, reverseID, revertclaim.StateCompleted, nil).Return(nil),
		redisRepo.EXPECT().CompleteOwnedKey(gomock.Any(), originKey, reverseID.String(), gomock.Any(), gomock.Any()).Return(false, nil),
		redisRepo.EXPECT().Get(gomock.Any(), originKey).Return(string(encoded), nil),
		redisRepo.EXPECT().CompleteOwnedKey(gomock.Any(), legacyKey, reverseID.String(), gomock.Any(), gomock.Any()).Return(false, nil),
		redisRepo.EXPECT().Get(gomock.Any(), legacyKey).Return(string(encoded), nil),
		redisRepo.EXPECT().FinalizeTransactionPersistence(gomock.Any(), organizationID, ledgerID, reverseID, attempt,
			[]string{operationID}).Return(nil),
	)

	uc := &UseCase{TransactionRepo: transactionRepo, RevertClaimRepo: claimRepo, TransactionRedisRepo: redisRepo}
	managed, err := uc.FinalizeDurableTransactionPersistence(context.Background(), organizationID, ledgerID, payload)
	require.True(t, managed)
	require.ErrorIs(t, err, firstCleanupErr)

	managed, err = uc.FinalizeDurableTransactionPersistence(context.Background(), organizationID, ledgerID, payload)
	require.True(t, managed)
	require.NoError(t, err)
}

func TestFinalizeDurableTransactionPersistence_LegacyStatusComesFromPrimary(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	transactionRepo := transaction.NewMockRepository(ctrl)
	claimRepo := revertclaim.NewMockRepository(ctrl)
	redisRepo := transactionredis.NewMockRedisRepository(ctrl)
	organizationID := uuid.New()
	ledgerID := uuid.New()
	originID := uuid.New()
	reverseID := uuid.New()
	originString := originID.String()
	operationID := uuid.NewString()
	replay := &transaction.Transaction{
		ID:                  reverseID.String(),
		OrganizationID:      organizationID.String(),
		LedgerID:            ledgerID.String(),
		ParentTransactionID: &originString,
		Status:              transaction.Status{Code: constant.CREATED},
		Operations:          []*operation.Operation{{ID: operationID}},
	}
	persisted := *replay
	persisted.Status = transaction.Status{Code: constant.APPROVED}
	claim := &revertclaim.Claim{
		OrganizationID:       organizationID,
		LedgerID:             ledgerID,
		OriginTransactionID:  originID,
		ReverseTransactionID: reverseID,
	}
	payload := transaction.TransactionProcessingPayload{
		Transaction: replay,
		Validate:    &mtransaction.Responses{Pending: false},
	}
	originKey := utils.IdempotencyInternalKey(organizationID, ledgerID,
		libCommons.HashSHA256(utils.RevertIdempotencyHashSource(originID)))

	transactionRepo.EXPECT().FindWithOperations(gomock.Any(), organizationID, ledgerID, reverseID).Return(&persisted, nil)
	claimRepo.EXPECT().Claim(gomock.Any(), organizationID, ledgerID, originID, reverseID, nil, nil).Return(claim, true, nil)
	claimRepo.EXPECT().Transition(gomock.Any(), organizationID, ledgerID, originID, reverseID, revertclaim.StateCompleted, nil).Return(nil)
	redisRepo.EXPECT().CompleteOwnedKey(gomock.Any(), originKey, reverseID.String(), gomock.Any(), gomock.Any()).Return(true, nil)
	redisRepo.EXPECT().FinalizeLegacyTransactionPersistence(gomock.Any(), organizationID, ledgerID,
		reverseID, originID, constant.CREATED, []string{operationID}).Return(nil)

	uc := &UseCase{TransactionRepo: transactionRepo, RevertClaimRepo: claimRepo, TransactionRedisRepo: redisRepo}
	managed, err := uc.FinalizeDurableTransactionPersistence(context.Background(), organizationID, ledgerID, payload)
	require.True(t, managed)
	require.NoError(t, err)
}

func TestFinalizeDurableTransactionPersistence_RetryAfterClaimBeforeReplay(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	transactionRepo := transaction.NewMockRepository(ctrl)
	claimRepo := revertclaim.NewMockRepository(ctrl)
	redisRepo := transactionredis.NewMockRedisRepository(ctrl)
	organizationID := uuid.New()
	ledgerID := uuid.New()
	originID := uuid.New()
	reverseID := uuid.New()
	originString := originID.String()
	operationID := uuid.NewString()
	replay := &transaction.Transaction{
		ID:                  reverseID.String(),
		OrganizationID:      organizationID.String(),
		LedgerID:            ledgerID.String(),
		ParentTransactionID: &originString,
		Status:              transaction.Status{Code: constant.CREATED},
		Operations:          []*operation.Operation{{ID: operationID}},
	}
	persisted := *replay
	persisted.Status = transaction.Status{Code: constant.APPROVED}
	claim := &revertclaim.Claim{
		OrganizationID:       organizationID,
		LedgerID:             ledgerID,
		OriginTransactionID:  originID,
		ReverseTransactionID: reverseID,
		LegacyFenceKey:       ptrString(utils.IdempotencyInternalKey(organizationID, ledgerID, "phase-zero-h1")),
	}
	payload := transaction.TransactionProcessingPayload{Transaction: replay, Validate: &mtransaction.Responses{}}
	originKey := utils.IdempotencyInternalKey(organizationID, ledgerID,
		libCommons.HashSHA256(utils.RevertIdempotencyHashSource(originID)))

	for range 2 {
		transactionRepo.EXPECT().FindWithOperations(gomock.Any(), organizationID, ledgerID, reverseID).Return(&persisted, nil)
		claimRepo.EXPECT().Claim(gomock.Any(), organizationID, ledgerID, originID, reverseID, nil, nil).
			Return(claim, false, nil)
		claimRepo.EXPECT().Transition(gomock.Any(), organizationID, ledgerID, originID, reverseID,
			revertclaim.StateCompleted, nil).Return(nil)
	}

	replayErr := errors.New("lost replay publication response")
	gomock.InOrder(
		redisRepo.EXPECT().CompleteOwnedKey(gomock.Any(), originKey, reverseID.String(), gomock.Any(), gomock.Any()).Return(false, replayErr),
		redisRepo.EXPECT().Get(gomock.Any(), originKey).Return("", replayErr),
		redisRepo.EXPECT().CompleteOwnedKey(gomock.Any(), originKey, reverseID.String(), gomock.Any(), gomock.Any()).Return(true, nil),
		redisRepo.EXPECT().CompleteUnownedKey(gomock.Any(), *claim.LegacyFenceKey, gomock.Any(), gomock.Any()).Return(true, nil),
		redisRepo.EXPECT().FinalizeLegacyTransactionPersistence(gomock.Any(), organizationID, ledgerID,
			reverseID, originID, constant.CREATED, []string{operationID}).Return(nil),
	)

	uc := &UseCase{TransactionRepo: transactionRepo, RevertClaimRepo: claimRepo, TransactionRedisRepo: redisRepo}
	managed, err := uc.FinalizeDurableTransactionPersistence(context.Background(), organizationID, ledgerID, payload)
	require.True(t, managed)
	require.ErrorIs(t, err, replayErr)

	managed, err = uc.FinalizeDurableTransactionPersistence(context.Background(), organizationID, ledgerID, payload)
	require.True(t, managed)
	require.NoError(t, err)
}

func ptrString(value string) *string {
	return &value
}

func TestFinalizeDurableTransactionPersistence_LifecycleProvesNewOperationsAndPreservesHoldHistory(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	transactionRepo := transaction.NewMockRepository(ctrl)
	redisRepo := transactionredis.NewMockRedisRepository(ctrl)
	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()
	holdOperationID := uuid.NewString()
	terminalOperationIDs := []string{uuid.NewString(), uuid.NewString()}
	approved := constant.APPROVED
	expected := &transaction.Transaction{
		ID:             transactionID.String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		Status:         transaction.Status{Code: approved},
		Operations: []*operation.Operation{
			{ID: terminalOperationIDs[0]},
			{ID: terminalOperationIDs[1]},
		},
	}
	persisted := *expected
	persisted.Operations = []*operation.Operation{
		{ID: holdOperationID},
		{ID: terminalOperationIDs[0]},
		{ID: terminalOperationIDs[1]},
	}
	attempt := mmodel.BalanceExecutionAttempt{
		ExecutionKey: utils.TransactionBalanceExecutionKey(organizationID, ledgerID, transactionID),
		OutcomeKey:   utils.TransactionBalanceOutcomeKey(organizationID, ledgerID, transactionID),
		Owner:        uuid.NewString(),
		Outcome:      mmodel.TransactionOutcomeCommitted,
		Identity:     transactionID,
	}
	payload := transaction.TransactionProcessingPayload{
		Transaction:     expected,
		AttemptOwner:    attempt.Owner,
		ExpectedOutcome: attempt.Outcome,
	}

	transactionRepo.EXPECT().FindWithOperations(gomock.Any(), organizationID, ledgerID, transactionID).
		Return(&persisted, nil)
	redisRepo.EXPECT().FinalizeTransactionPersistence(gomock.Any(), organizationID, ledgerID, transactionID,
		attempt, terminalOperationIDs).Return(nil)

	uc := &UseCase{TransactionRepo: transactionRepo, TransactionRedisRepo: redisRepo}
	managed, err := uc.FinalizeDurableTransactionPersistence(context.Background(), organizationID, ledgerID, payload)
	require.True(t, managed)
	require.NoError(t, err)
}

func TestProveDurableTransactionPayload_RejectsSameOperationIDsWithDifferentBalanceSnapshots(t *testing.T) {
	t.Parallel()

	transactionID := uuid.NewString()
	organizationID := uuid.NewString()
	ledgerID := uuid.NewString()
	operationID := uuid.NewString()
	before := decimal.NewFromInt(100)
	expectedAfter := decimal.NewFromInt(90)
	durableAfter := decimal.NewFromInt(89)
	versionBefore := int64(1)
	versionAfter := int64(2)
	newOperation := func(after *decimal.Decimal) *operation.Operation {
		return &operation.Operation{
			ID:        operationID,
			BalanceID: uuid.NewString(),
			Balance: operation.Balance{
				Available: &before,
				Version:   &versionBefore,
			},
			BalanceAfter: operation.Balance{
				Available: after,
				Version:   &versionAfter,
			},
		}
	}
	expectedOperation := newOperation(&expectedAfter)
	durableOperation := newOperation(&durableAfter)
	durableOperation.BalanceID = expectedOperation.BalanceID
	expected := &transaction.Transaction{
		ID:             transactionID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Status:         transaction.Status{Code: constant.APPROVED},
		Operations:     []*operation.Operation{expectedOperation},
	}
	durable := &transaction.Transaction{
		ID:             transactionID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Status:         transaction.Status{Code: constant.APPROVED},
		Operations:     []*operation.Operation{durableOperation},
	}

	require.ErrorContains(t, proveDurableTransactionPayload(durable, expected),
		"durable transaction operation set mismatch")
	require.False(t, replayIdentityMatches(durable, expected),
		"terminal replay cannot accept matching IDs with a different balance fact")
}
