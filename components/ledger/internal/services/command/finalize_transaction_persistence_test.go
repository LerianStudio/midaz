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

type revertRolloutLeaseRecorder struct {
	tokens []string
	modes  []string
	err    error
}

func (r *revertRolloutLeaseRecorder) CompleteRevert(_ context.Context, mode, token string) error {
	return r.complete(mode, token)
}

func (r *revertRolloutLeaseRecorder) complete(mode, token string) error {
	r.modes = append(r.modes, mode)
	r.tokens = append(r.tokens, token)

	return r.err
}

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
	economicOperation, balanceAfter := completeOutcomeEvidence(organizationID, ledgerID, reverseID)
	originString := originID.String()
	created := constant.CREATED
	approved := constant.APPROVED
	replay := &transaction.Transaction{
		ID:                  reverseID.String(),
		OrganizationID:      organizationID.String(),
		LedgerID:            ledgerID.String(),
		ParentTransactionID: &originString,
		Status:              transaction.Status{Code: created},
		Operations:          []*operation.Operation{economicOperation},
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
		BalancesAfter:   []*mmodel.Balance{balanceAfter},
	}
	bindCompleteTransactionIdentity(&payload)
	persisted.Amount = payload.Transaction.Amount
	persisted.AssetCode = payload.Transaction.AssetCode
	legacyHash, err := utils.LegacyTransactionIdempotencyHash(*payload.Input)
	require.NoError(t, err)
	legacyKey = utils.IdempotencyInternalKey(organizationID, ledgerID, legacyHash)
	claim.LegacyFenceKey = &legacyKey
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
	redisRepo.EXPECT().EnrichTransactionBackup(gomock.Any(), organizationID, ledgerID, reverseID,
		gomock.Any(), constant.ActionRevert, gomock.Any()).
		Return([]mmodel.OperationRedis{economicOperation.ToRedis()}, []mmodel.BalanceRedis{balanceAfter.ToRedis()}, false, nil).
		Times(2)

	for range 2 {
		transactionRepo.EXPECT().FindWithOperations(gomock.Any(), organizationID, ledgerID, reverseID).
			DoAndReturn(func(ctx context.Context, _, _ uuid.UUID, _ uuid.UUID) (*transaction.Transaction, error) {
				require.True(t, readrouting.IsPrimaryRead(ctx))
				return &persisted, nil
			})
		claimRepo.EXPECT().Claim(gomock.Any(), organizationID, ledgerID, originID, reverseID,
			&legacyKey, nil, nil, nil, nil).
			Return(claim, false, nil)
	}

	firstCleanupErr := errors.New("lost cleanup response")
	gomock.InOrder(
		claimRepo.EXPECT().Transition(gomock.Any(), organizationID, ledgerID, originID, reverseID, revertclaim.StateCompleted, nil).Return(nil),
		redisRepo.EXPECT().CompleteOwnedKey(gomock.Any(), originKey, reverseID.String(), gomock.Any(), gomock.Any()).Return(true, nil),
		redisRepo.EXPECT().CompleteOwnedKey(gomock.Any(), legacyKey, reverseID.String(), gomock.Any(), gomock.Any()).Return(true, nil),
		redisRepo.EXPECT().FinalizeTransactionPersistence(gomock.Any(), organizationID, ledgerID, reverseID, attempt,
			gomock.Any(), gomock.Any()).Return(firstCleanupErr),
		claimRepo.EXPECT().Transition(gomock.Any(), organizationID, ledgerID, originID, reverseID, revertclaim.StateCompleted, nil).Return(nil),
		redisRepo.EXPECT().CompleteOwnedKey(gomock.Any(), originKey, reverseID.String(), gomock.Any(), gomock.Any()).Return(false, nil),
		redisRepo.EXPECT().MGet(gomock.Any(), []string{originKey, originKey + ":owner"}).Return(map[string]string{originKey: string(encoded)}, nil),
		redisRepo.EXPECT().CompleteOwnedKey(gomock.Any(), legacyKey, reverseID.String(), gomock.Any(), gomock.Any()).Return(false, nil),
		redisRepo.EXPECT().MGet(gomock.Any(), []string{legacyKey, legacyKey + ":owner"}).Return(map[string]string{legacyKey: string(encoded)}, nil),
		redisRepo.EXPECT().FinalizeTransactionPersistence(gomock.Any(), organizationID, ledgerID, reverseID, attempt,
			gomock.Any(), gomock.Any()).Return(nil),
	)

	uc := &UseCase{TransactionRepo: transactionRepo, RevertClaimRepo: claimRepo, TransactionRedisRepo: redisRepo}
	managed, err := uc.FinalizeDurableTransactionPersistence(context.Background(), organizationID, ledgerID, payload)
	require.True(t, managed)
	require.ErrorIs(t, err, firstCleanupErr)

	managed, err = uc.FinalizeDurableTransactionPersistence(context.Background(), organizationID, ledgerID, payload)
	require.True(t, managed)
	require.NoError(t, err)
}

func TestCompleteOwnedReplay_LostResponseCannotAcceptReplayWhileOwnerSurvives(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	redisRepo := transactionredis.NewMockRedisRepository(ctrl)
	key := "idempotency:{same-slot}"
	owner := uuid.NewString()
	originID := uuid.NewString()
	replay := &transaction.Transaction{
		ID:                  owner,
		ParentTransactionID: &originID,
		Operations:          []*operation.Operation{{ID: uuid.NewString()}},
	}
	encoded, err := json.Marshal(replay)
	require.NoError(t, err)

	redisRepo.EXPECT().CompleteOwnedKey(gomock.Any(), key, owner, string(encoded), gomock.Any()).Return(false, nil)
	redisRepo.EXPECT().MGet(gomock.Any(), []string{key, key + ":owner"}).Return(map[string]string{
		key:            string(encoded),
		key + ":owner": owner,
	}, nil)

	uc := &UseCase{TransactionRedisRepo: redisRepo}
	err = uc.completeOwnedReplay(context.Background(), key, owner, string(encoded), replay)
	require.ErrorContains(t, err, "owner remains")
}

func TestCompleteDurableRevertReplay_FinalPreservesForeignLegacyFence(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	redisRepo := transactionredis.NewMockRedisRepository(ctrl)
	organizationID := uuid.New()
	ledgerID := uuid.New()
	originID := uuid.New()
	reverseID := uuid.New()
	foreignOriginID := uuid.NewString()
	foreignReverseID := uuid.NewString()
	originString := originID.String()
	legacyKey := utils.IdempotencyInternalKey(organizationID, ledgerID, "payload-hash-collision")
	legacyOwner := reverseID.String()
	claim := &revertclaim.Claim{
		OrganizationID: organizationID, LedgerID: ledgerID, OriginTransactionID: originID,
		ReverseTransactionID: reverseID, LegacyFenceKey: &legacyKey, LegacyFenceOwner: &legacyOwner,
	}
	replay := &transaction.Transaction{
		ID: reverseID.String(), ParentTransactionID: &originString,
		Operations: []*operation.Operation{{ID: uuid.NewString()}},
	}
	foreignReplay := &transaction.Transaction{
		ID: foreignReverseID, ParentTransactionID: &foreignOriginID,
		Operations: []*operation.Operation{{ID: uuid.NewString()}},
	}
	foreignRaw, err := json.Marshal(foreignReplay)
	require.NoError(t, err)
	originKey := utils.IdempotencyInternalKey(organizationID, ledgerID,
		libCommons.HashSHA256(utils.RevertIdempotencyHashSource(originID)))

	redisRepo.EXPECT().CompleteOwnedKey(gomock.Any(), originKey, reverseID.String(), gomock.Any(), gomock.Any()).
		Return(true, nil)
	redisRepo.EXPECT().CompleteOwnedKey(gomock.Any(), legacyKey, reverseID.String(), gomock.Any(), gomock.Any()).
		Return(false, nil)
	redisRepo.EXPECT().MGet(gomock.Any(), []string{legacyKey, legacyKey + ":owner"}).
		Return(map[string]string{legacyKey: string(foreignRaw), legacyKey + ":owner": foreignReverseID}, nil).
		Times(2)

	uc := &UseCase{
		TransactionRedisRepo:  redisRepo,
		RevertIdempotencyMode: "final",
	}
	require.NoError(t, uc.completeDurableRevertReplay(context.Background(), originID, claim, replay))
}

func TestFinalizeDurableTransactionPersistence_RolloutStatusComesFromPrimary(t *testing.T) {
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
	economicOperation, balanceAfter := completeOutcomeEvidence(organizationID, ledgerID, reverseID)
	replay := &transaction.Transaction{
		ID:                  reverseID.String(),
		OrganizationID:      organizationID.String(),
		LedgerID:            ledgerID.String(),
		ParentTransactionID: &originString,
		Status:              transaction.Status{Code: constant.CREATED},
		Operations:          []*operation.Operation{economicOperation},
	}
	persisted := *replay
	persisted.Status = transaction.Status{Code: constant.APPROVED}
	input := mtransaction.Transaction{
		Description: "immutable phase-zero reverse",
		Send:        mtransaction.Send{Asset: "USD", Value: decimal.NewFromInt(10)},
	}
	legacyHash, err := utils.LegacyTransactionIdempotencyHash(input)
	require.NoError(t, err)
	legacyKey := utils.IdempotencyInternalKey(organizationID, ledgerID, legacyHash)
	legacyOwner := reverseID.String()
	rolloutMode := "bridge"
	rolloutToken := "phase-zero-request"
	redisGeneration := "financial-dataset-generation"
	claim := &revertclaim.Claim{
		OrganizationID:       organizationID,
		LedgerID:             ledgerID,
		OriginTransactionID:  originID,
		ReverseTransactionID: reverseID,
		LegacyFenceKey:       &legacyKey,
		LegacyFenceOwner:     &legacyOwner,
		RolloutMode:          &rolloutMode,
		RolloutToken:         &rolloutToken,
		RedisGeneration:      &redisGeneration,
	}
	payload := transaction.TransactionProcessingPayload{
		Transaction:        replay,
		Validate:           &mtransaction.Responses{Pending: false},
		Input:              &input,
		AttemptOwner:       reverseID.String(),
		ExpectedOutcome:    mmodel.TransactionOutcomeCommitted,
		RevertRolloutMode:  rolloutMode,
		RevertRolloutToken: rolloutToken,
		RedisGeneration:    redisGeneration,
		BalancesAfter:      []*mmodel.Balance{balanceAfter},
	}
	bindCompleteTransactionIdentity(&payload)
	persisted.Amount = payload.Transaction.Amount
	persisted.AssetCode = payload.Transaction.AssetCode
	originKey := utils.IdempotencyInternalKey(organizationID, ledgerID,
		libCommons.HashSHA256(utils.RevertIdempotencyHashSource(originID)))

	redisRepo.EXPECT().EnrichTransactionBackup(gomock.Any(), organizationID, ledgerID, reverseID,
		gomock.Any(), constant.ActionRevert, gomock.Any()).
		Return([]mmodel.OperationRedis{economicOperation.ToRedis()}, []mmodel.BalanceRedis{balanceAfter.ToRedis()}, false, nil)
	transactionRepo.EXPECT().FindWithOperations(gomock.Any(), organizationID, ledgerID, reverseID).Return(&persisted, nil)
	claimRepo.EXPECT().Claim(gomock.Any(), organizationID, ledgerID, originID, reverseID,
		&legacyKey, &legacyOwner, &rolloutMode, &rolloutToken, &redisGeneration).Return(claim, true, nil)
	claimRepo.EXPECT().Transition(gomock.Any(), organizationID, ledgerID, originID, reverseID, revertclaim.StateCompleted, nil).Return(nil)
	redisRepo.EXPECT().CompleteOwnedKey(gomock.Any(), originKey, reverseID.String(), gomock.Any(), gomock.Any()).Return(true, nil)
	redisRepo.EXPECT().CompleteOwnedKey(gomock.Any(), legacyKey, reverseID.String(), gomock.Any(), gomock.Any()).Return(true, nil)
	redisRepo.EXPECT().FinalizeTransactionPersistence(gomock.Any(), organizationID, ledgerID, reverseID,
		gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	rollout := &revertRolloutLeaseRecorder{}
	uc := &UseCase{
		TransactionRepo: transactionRepo, RevertClaimRepo: claimRepo, TransactionRedisRepo: redisRepo,
		RevertRolloutLease: rollout,
	}
	managed, err := uc.FinalizeDurableTransactionPersistence(context.Background(), organizationID, ledgerID, payload)
	require.True(t, managed)
	require.NoError(t, err)
	require.Equal(t, []string{"phase-zero-request"}, rollout.tokens,
		"the pre-activation admission remains held until the exact reverse and operations are durable and Redis cleanup completes")
	require.Equal(t, []string{"bridge"}, rollout.modes,
		"terminal handoff must release the generation-scoped rollout barrier that admitted this reverse")
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
	economicOperation, balanceAfter := completeOutcomeEvidence(organizationID, ledgerID, reverseID)
	operationID := economicOperation.ID
	replay := &transaction.Transaction{
		ID:                  reverseID.String(),
		OrganizationID:      organizationID.String(),
		LedgerID:            ledgerID.String(),
		ParentTransactionID: &originString,
		Status:              transaction.Status{Code: constant.CREATED},
		Operations:          []*operation.Operation{economicOperation},
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
	payload := transaction.TransactionProcessingPayload{
		Transaction: replay, Validate: &mtransaction.Responses{}, BalancesAfter: []*mmodel.Balance{balanceAfter},
	}
	bindCompleteTransactionIdentity(&payload)
	persisted.Amount = payload.Transaction.Amount
	persisted.AssetCode = payload.Transaction.AssetCode
	legacyHash, err := utils.LegacyTransactionIdempotencyHash(*payload.Input)
	require.NoError(t, err)
	legacyKey := utils.IdempotencyInternalKey(organizationID, ledgerID, legacyHash)
	claim.LegacyFenceKey = &legacyKey
	originKey := utils.IdempotencyInternalKey(organizationID, ledgerID,
		libCommons.HashSHA256(utils.RevertIdempotencyHashSource(originID)))

	for range 2 {
		redisRepo.EXPECT().EnrichTransactionBackup(gomock.Any(), organizationID, ledgerID, reverseID,
			gomock.Any(), constant.ActionRevert, nil).
			Return([]mmodel.OperationRedis{economicOperation.ToRedis()}, []mmodel.BalanceRedis{balanceAfter.ToRedis()}, false, nil)
		transactionRepo.EXPECT().FindWithOperations(gomock.Any(), organizationID, ledgerID, reverseID).Return(&persisted, nil)
		claimRepo.EXPECT().Claim(gomock.Any(), organizationID, ledgerID, originID, reverseID,
			&legacyKey, nil, nil, nil, nil).
			Return(claim, false, nil)
		claimRepo.EXPECT().Transition(gomock.Any(), organizationID, ledgerID, originID, reverseID,
			revertclaim.StateCompleted, nil).Return(nil)
	}

	replayErr := errors.New("lost replay publication response")
	gomock.InOrder(
		redisRepo.EXPECT().CompleteOwnedKey(gomock.Any(), originKey, reverseID.String(), gomock.Any(), gomock.Any()).Return(false, replayErr),
		redisRepo.EXPECT().MGet(gomock.Any(), []string{originKey, originKey + ":owner"}).Return(nil, replayErr),
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

func TestFinalizeDurableTransactionPersistence_RejectsDifferentPersistedRolloutGeneration(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	transactionRepo := transaction.NewMockRepository(ctrl)
	claimRepo := revertclaim.NewMockRepository(ctrl)
	redisRepo := transactionredis.NewMockRedisRepository(ctrl)
	organizationID := uuid.New()
	ledgerID := uuid.New()
	originID := uuid.New()
	reverseID := uuid.New()
	economicOperation, balanceAfter := completeOutcomeEvidence(organizationID, ledgerID, reverseID)
	originString := originID.String()
	expected := &transaction.Transaction{
		ID: reverseID.String(), OrganizationID: organizationID.String(), LedgerID: ledgerID.String(),
		ParentTransactionID: &originString, Status: transaction.Status{Code: constant.CREATED},
		Operations: []*operation.Operation{economicOperation},
	}
	persisted := *expected
	persisted.Status = transaction.Status{Code: constant.APPROVED}
	input := mtransaction.Transaction{
		Description: "immutable reverse", Send: mtransaction.Send{Asset: "USD", Value: decimal.NewFromInt(10)},
	}
	legacyHash, err := utils.LegacyTransactionIdempotencyHash(input)
	require.NoError(t, err)
	legacyKey := utils.IdempotencyInternalKey(organizationID, ledgerID, legacyHash)
	legacyOwner := reverseID.String()
	persistedMode := "legacy"
	persistedToken := "different-generation"
	redisGeneration := "financial-dataset-generation"
	claim := &revertclaim.Claim{
		OrganizationID: organizationID, LedgerID: ledgerID, OriginTransactionID: originID,
		ReverseTransactionID: reverseID, LegacyFenceKey: &legacyKey, LegacyFenceOwner: &legacyOwner,
		RolloutMode: &persistedMode, RolloutToken: &persistedToken, RedisGeneration: &redisGeneration,
	}
	payload := transaction.TransactionProcessingPayload{
		Transaction: expected, Validate: &mtransaction.Responses{}, Input: &input,
		AttemptOwner: reverseID.String(), ExpectedOutcome: mmodel.TransactionOutcomeCommitted,
		RevertRolloutMode: "bridge", RevertRolloutToken: "incoming-generation",
		RedisGeneration: redisGeneration,
		BalancesAfter:   []*mmodel.Balance{balanceAfter},
	}
	bindCompleteTransactionIdentity(&payload)
	persisted.Amount = payload.Transaction.Amount
	persisted.AssetCode = payload.Transaction.AssetCode

	redisRepo.EXPECT().EnrichTransactionBackup(gomock.Any(), organizationID, ledgerID, reverseID,
		gomock.Any(), constant.ActionRevert, gomock.Any()).
		Return([]mmodel.OperationRedis{economicOperation.ToRedis()}, []mmodel.BalanceRedis{balanceAfter.ToRedis()}, false, nil)
	transactionRepo.EXPECT().FindWithOperations(gomock.Any(), organizationID, ledgerID, reverseID).
		Return(&persisted, nil)
	claimRepo.EXPECT().Claim(gomock.Any(), organizationID, ledgerID, originID, reverseID,
		&legacyKey, &legacyOwner, ptrString("bridge"), ptrString("incoming-generation"), &redisGeneration).
		Return(claim, false, nil)

	uc := &UseCase{TransactionRepo: transactionRepo, RevertClaimRepo: claimRepo, TransactionRedisRepo: redisRepo}
	managed, err := uc.FinalizeDurableTransactionPersistence(context.Background(), organizationID, ledgerID, payload)
	require.True(t, managed)
	require.ErrorContains(t, err, "rollout generation mismatch")
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
	firstOperation, firstBalance := completeOutcomeEvidence(organizationID, ledgerID, transactionID)
	secondOperation, secondBalance := completeOutcomeEvidence(organizationID, ledgerID, transactionID)
	approved := constant.APPROVED
	expected := &transaction.Transaction{
		ID:             transactionID.String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		Status:         transaction.Status{Code: approved},
		Operations: []*operation.Operation{
			firstOperation,
			secondOperation,
		},
	}
	persisted := *expected
	persisted.Operations = []*operation.Operation{
		{ID: holdOperationID},
		firstOperation,
		secondOperation,
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
		BalancesAfter:   []*mmodel.Balance{firstBalance, secondBalance},
	}
	bindCompleteTransactionIdentity(&payload)
	persisted.Amount = payload.Transaction.Amount
	persisted.AssetCode = payload.Transaction.AssetCode

	redisRepo.EXPECT().EnrichTransactionBackup(gomock.Any(), organizationID, ledgerID, transactionID,
		gomock.Any(), constant.ActionCommit, gomock.Any()).
		Return([]mmodel.OperationRedis{firstOperation.ToRedis(), secondOperation.ToRedis()},
			[]mmodel.BalanceRedis{firstBalance.ToRedis(), secondBalance.ToRedis()}, false, nil)
	transactionRepo.EXPECT().FindWithOperations(gomock.Any(), organizationID, ledgerID, transactionID).
		Return(&persisted, nil)
	redisRepo.EXPECT().FinalizeTransactionPersistence(gomock.Any(), organizationID, ledgerID, transactionID,
		attempt, gomock.Any(), gomock.Any()).Return(nil)

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
	transactionAmount := decimal.NewFromInt(10)
	expected := &transaction.Transaction{
		ID:             transactionID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Status:         transaction.Status{Code: constant.APPROVED},
		Amount:         &transactionAmount,
		AssetCode:      "USD",
		Operations:     []*operation.Operation{expectedOperation},
	}
	durable := &transaction.Transaction{
		ID:             transactionID,
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Status:         transaction.Status{Code: constant.APPROVED},
		Amount:         &transactionAmount,
		AssetCode:      "USD",
		Operations:     []*operation.Operation{durableOperation},
	}

	require.ErrorContains(t, proveDurableTransactionPayload(durable, expected),
		"durable transaction operation set mismatch")
	require.False(t, replayIdentityMatches(durable, expected),
		"terminal replay cannot accept matching IDs with a different balance fact")
}

func TestProveDurableTransactionPayload_RejectsTransactionAmountOrAssetDivergence(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()
	economicOperation, _ := completeOutcomeEvidence(organizationID, ledgerID, transactionID)
	amount := decimal.NewFromInt(100)
	expected := &transaction.Transaction{
		ID: transactionID.String(), OrganizationID: organizationID.String(), LedgerID: ledgerID.String(),
		Amount: &amount, AssetCode: "USD", Status: transaction.Status{Code: constant.APPROVED},
		Operations: []*operation.Operation{economicOperation},
	}

	for _, test := range []struct {
		name    string
		mutate  func(*transaction.Transaction)
		wantErr string
	}{
		{
			name: "amount",
			mutate: func(persisted *transaction.Transaction) {
				divergent := decimal.NewFromInt(999)
				persisted.Amount = &divergent
			},
			wantErr: "amount mismatch",
		},
		{
			name:    "asset",
			mutate:  func(persisted *transaction.Transaction) { persisted.AssetCode = "EUR" },
			wantErr: "asset mismatch",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			persisted := *expected
			test.mutate(&persisted)
			require.ErrorContains(t, proveDurableTransactionPayload(&persisted, expected), test.wantErr)
		})
	}
}

func TestProveCompletedDurableReplay_RejectsReverseWithoutCompletedPrimaryClaim(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	transactionRepo := transaction.NewMockRepository(ctrl)
	claimRepo := revertclaim.NewMockRepository(ctrl)
	organizationID := uuid.New()
	ledgerID := uuid.New()
	originID := uuid.New()
	reverseID := uuid.New()
	operationID := uuid.NewString()
	originString := originID.String()
	generation := uuid.NewString()
	rolloutMode := "bridge"
	rolloutToken := uuid.NewString()
	economicOperation := &operation.Operation{ID: operationID, TransactionID: reverseID.String()}
	expected := &transaction.Transaction{
		ID: reverseID.String(), OrganizationID: organizationID.String(), LedgerID: ledgerID.String(),
		ParentTransactionID: &originString, Status: transaction.Status{Code: constant.CREATED},
		Operations: []*operation.Operation{economicOperation},
	}
	persisted := *expected
	persisted.Status = transaction.Status{Code: constant.APPROVED}
	payload := transaction.TransactionProcessingPayload{
		Transaction: expected, RedisGeneration: generation,
		RevertRolloutMode: rolloutMode, RevertRolloutToken: rolloutToken,
	}
	bindCompleteTransactionIdentity(&payload)
	persisted.Amount = payload.Transaction.Amount
	persisted.AssetCode = payload.Transaction.AssetCode

	transactionRepo.EXPECT().FindWithOperations(gomock.Any(), organizationID, ledgerID, reverseID).
		DoAndReturn(func(ctx context.Context, _, _ uuid.UUID, _ uuid.UUID) (*transaction.Transaction, error) {
			require.True(t, readrouting.IsPrimaryRead(ctx))

			return &persisted, nil
		})
	claimRepo.EXPECT().Get(gomock.Any(), organizationID, ledgerID, originID).
		DoAndReturn(func(ctx context.Context, _, _, _ uuid.UUID) (*revertclaim.Claim, error) {
			require.True(t, readrouting.IsPrimaryRead(ctx))

			return &revertclaim.Claim{
				OrganizationID: organizationID, LedgerID: ledgerID, OriginTransactionID: originID,
				ReverseTransactionID: reverseID, State: revertclaim.StateMutated,
				RedisGeneration: &generation, RolloutMode: &rolloutMode, RolloutToken: &rolloutToken,
			}, nil
		})

	uc := &UseCase{TransactionRepo: transactionRepo, RevertClaimRepo: claimRepo}
	_, err := uc.ProveCompletedDurableReplay(context.Background(), organizationID, ledgerID, payload)
	require.ErrorContains(t, err, "completed reverse replay claim differs from terminal tombstone")
}
