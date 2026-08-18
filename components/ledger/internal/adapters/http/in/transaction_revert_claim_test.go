// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	"github.com/google/uuid"
	redislib "github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/revertclaim"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

func completeRevertEconomicOperation(
	organizationID, ledgerID, transactionID uuid.UUID,
	operationID string,
) (*operation.Operation, *mmodel.Balance) {
	amount := decimal.NewFromInt(100)
	before := decimal.NewFromInt(900)
	after := decimal.NewFromInt(1000)
	onHold := decimal.Zero
	beforeVersion := int64(1)
	afterVersion := int64(2)
	balanceID := uuid.NewString()
	accountID := uuid.NewString()
	economicOperation := &operation.Operation{
		ID: operationID, TransactionID: transactionID.String(), Type: constant.CREDIT,
		AssetCode: "USD", Amount: operation.Amount{Value: &amount}, BalanceID: balanceID,
		BalanceKey: constant.DefaultBalanceKey, AccountID: accountID,
		OrganizationID: organizationID.String(), LedgerID: ledgerID.String(),
		BalanceAffected: true, Direction: constant.DirectionCredit,
		Balance:      operation.Balance{Available: &before, OnHold: &onHold, Version: &beforeVersion},
		BalanceAfter: operation.Balance{Available: &after, OnHold: &onHold, Version: &afterVersion},
		Snapshot:     mmodel.OperationSnapshot{OverdraftUsedBefore: "0", OverdraftUsedAfter: "0"},
	}
	balanceAfter := &mmodel.Balance{
		ID: balanceID, Alias: "@revert", Key: constant.DefaultBalanceKey, AccountID: accountID,
		AssetCode: "USD", Available: after, OnHold: onHold, Version: afterVersion,
		AccountType: "asset", AllowSending: true, AllowReceiving: true,
		Direction: constant.DirectionCredit, OverdraftUsed: decimal.Zero,
	}

	return economicOperation, balanceAfter
}

var testRedisGeneration = "test-financial-dataset-generation"

func TestRevertRolloutHandoffPending_RequiresAtomicRedisAbsenceAndCompatibleClaim(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	originID := uuid.New()
	reverseID := uuid.New()
	otherReverseID := uuid.New()
	readErr := errors.New("redis unavailable")

	tests := []struct {
		name        string
		evidence    bool
		redisErr    error
		claim       *revertclaim.Claim
		claimErr    error
		wantPending bool
	}{
		{name: "surviving Redis evidence", evidence: true, wantPending: true},
		{name: "unreadable Redis evidence", redisErr: readErr, wantPending: true},
		{name: "released claim and no Redis evidence", claim: nil},
		{name: "matching terminal claim and no Redis evidence", claim: &revertclaim.Claim{
			ReverseTransactionID: reverseID, State: revertclaim.StateCompleted,
		}},
		{name: "nonterminal claim remains pending", claim: &revertclaim.Claim{
			ReverseTransactionID: reverseID, State: revertclaim.StateClaimed,
		}, wantPending: true},
		{name: "different reserved reverse remains pending", claim: &revertclaim.Claim{
			ReverseTransactionID: otherReverseID, State: revertclaim.StateCompleted,
		}, wantPending: true},
		{name: "unreadable claim remains pending", claimErr: errors.New("postgres unavailable"), wantPending: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			redisRepo := redis.NewMockRedisRepository(ctrl)
			claimRepo := revertclaim.NewMockRepository(ctrl)
			claimRepo.EXPECT().Get(gomock.Any(), organizationID, ledgerID, originID).Return(tc.claim, tc.claimErr)
			if tc.claimErr == nil {
				redisRepo.EXPECT().TransactionEconomicEvidenceExists(gomock.Any(), organizationID, ledgerID,
					reverseID, "").Return(tc.evidence, true, tc.redisErr)
			}

			handler := &TransactionHandler{
				RevertUpdateFreeze: &revertUpdateFreezeStub{},
				Command: &command.UseCase{
					TransactionRedisRepo: redisRepo,
					RevertClaimRepo:      claimRepo,
				},
			}
			assert.Equal(t, tc.wantPending, handler.revertRolloutHandoffPending(context.Background(),
				organizationID, ledgerID, originID, reverseID))
		})
	}
}

func TestRevertRolloutHandoffPending_RequiresExactGenerationTerminalSeal(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	originID := uuid.New()
	reverseID := uuid.New()
	rolloutMode := "bridge"
	rolloutToken := "origin-generation"
	redisGeneration := "financial-dataset-generation"

	for _, tc := range []struct {
		name             string
		terminalComplete bool
		terminalErr      error
		wantPending      bool
	}{
		{name: "generation is not sealed", wantPending: true},
		{name: "generation proof is unreadable", terminalErr: errors.New("redis unavailable"), wantPending: true},
		{name: "generation is atomically sealed", terminalComplete: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			redisRepo := redis.NewMockRedisRepository(ctrl)
			claimRepo := revertclaim.NewMockRepository(ctrl)
			claimRepo.EXPECT().Get(gomock.Any(), organizationID, ledgerID, originID).Return(&revertclaim.Claim{
				ReverseTransactionID: reverseID, State: revertclaim.StateCompleted,
				RolloutMode: &rolloutMode, RolloutToken: &rolloutToken, RedisGeneration: &redisGeneration,
			}, nil)
			redisRepo.EXPECT().TransactionEconomicEvidenceExists(gomock.Any(), organizationID, ledgerID,
				reverseID, redisGeneration).Return(false, true, nil)
			freeze := &revertUpdateFreezeStub{terminalComplete: tc.terminalComplete, terminalErr: tc.terminalErr}
			handler := &TransactionHandler{
				RevertUpdateFreeze: freeze,
				Command:            &command.UseCase{TransactionRedisRepo: redisRepo, RevertClaimRepo: claimRepo},
			}
			assert.Equal(t, tc.wantPending, handler.revertRolloutHandoffPending(context.Background(),
				organizationID, ledgerID, originID, reverseID))
		})
	}
}

func TestFailRevertClaim_OnlyProvenPreMutationFailureReleases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		execution     revertExecutionState
		cause         error
		expectRelease bool
	}{
		{
			name:          "failure before balance attempt releases",
			execution:     revertExecutionState{},
			cause:         errors.New("route validation unavailable"),
			expectRelease: true,
		},
		{
			name:      "lost seed response remains fenced for reconciliation",
			execution: revertExecutionState{SeedWriteAmbiguous: true},
			cause:     errors.New("i/o timeout after seed write"),
		},
		{
			name:          "Lua business rejection proves rollback and releases",
			execution:     revertExecutionState{SeedWritten: true, BalanceAttempted: true},
			cause:         pkg.ValidateBusinessError(constant.ErrInsufficientFunds, constant.EntityBalance),
			expectRelease: true,
		},
		{
			name:      "lost Lua response remains fenced for reconciliation",
			execution: revertExecutionState{BalanceAttempted: true},
			cause:     errors.New("i/o timeout after command dispatch"),
		},
		{
			name:      "failure after committed balances remains fenced for reconciliation",
			execution: revertExecutionState{BalanceAttempted: true, BalanceCommitted: true},
			cause:     errors.New("postgres persistence unavailable"),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			claimRepo := revertclaim.NewMockRepository(ctrl)
			redisRepo := redis.NewMockRedisRepository(ctrl)
			claim := &revertclaim.Claim{
				OrganizationID:       uuid.New(),
				LedgerID:             uuid.New(),
				OriginTransactionID:  uuid.New(),
				ReverseTransactionID: uuid.New(),
				State:                revertclaim.StateClaimed,
			}
			legacyKey := "legacy-fence"
			legacyOwner := claim.ReverseTransactionID.String()
			claim.LegacyFenceKey = &legacyKey
			claim.LegacyFenceOwner = &legacyOwner

			if tc.expectRelease {
				if tc.execution.SeedWritten {
					redisRepo.EXPECT().RemoveMessageFromQueueIfStatus(gomock.Any(), utils.TransactionInternalKey(
						claim.OrganizationID, claim.LedgerID, claim.ReverseTransactionID.String()), constant.CREATED,
						claim.ReverseTransactionID.String(), mmodel.TransactionOutcomeCommitted, true).Return(true, nil)
				}
				redisRepo.EXPECT().ReleaseOwnedKey(gomock.Any(), originRevertIdempotencyKey(claim),
					claim.ReverseTransactionID.String()).Return(true, nil)
				redisRepo.EXPECT().ReleaseOwnedKey(gomock.Any(), revertExecutionFenceKey(claim),
					claim.ReverseTransactionID.String()).Return(true, nil)
				redisRepo.EXPECT().ReleaseOwnedKey(gomock.Any(), "legacy-fence", claim.ReverseTransactionID.String()).Return(true, nil)
				claimRepo.EXPECT().Release(gomock.Any(), claim.OrganizationID, claim.LedgerID,
					claim.OriginTransactionID, claim.ReverseTransactionID).Return(true, nil)
			} else {
				claimRepo.EXPECT().Transition(gomock.Any(), claim.OrganizationID, claim.LedgerID,
					claim.OriginTransactionID, claim.ReverseTransactionID,
					revertclaim.StateReconciliationRequired, gomock.Not(gomock.Nil())).Return(nil)
			}

			handler := &TransactionHandler{Command: &command.UseCase{
				RevertClaimRepo:      claimRepo,
				TransactionRedisRepo: redisRepo,
			}}
			err := handler.failRevertClaim(context.Background(), claim, &tc.execution, "legacy-fence", tc.cause)

			if tc.expectRelease {
				assert.ErrorIs(t, err, tc.cause)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "being reconciled")
			}
		})
	}
}

func TestAcquireLegacyRevertBarrier_AmbiguousAcquireReturnsOwnedKeyForCleanup(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	redisRepo := redis.NewMockRedisRepository(ctrl)
	claim := &revertclaim.Claim{ReverseTransactionID: uuid.New()}
	organizationID := uuid.New()
	ledgerID := uuid.New()
	legacyHash := "legacy-hash"
	wantKey := utils.IdempotencyInternalKey(organizationID, ledgerID, legacyHash)
	claim.LegacyFenceKey = &wantKey
	legacyOwner := claim.ReverseTransactionID.String()
	claim.LegacyFenceOwner = &legacyOwner
	redisRepo.EXPECT().AcquireOwnedKey(gomock.Any(), wantKey, claim.ReverseTransactionID.String(), time.Duration(0)).
		Return(false, errors.New("lost script response"))

	handler := &TransactionHandler{Command: &command.UseCase{TransactionRedisRepo: redisRepo}}
	key, replay, err := handler.acquireLegacyRevertBarrier(context.Background(), claim)

	require.Error(t, err)
	assert.Equal(t, wantKey, key)
	assert.Nil(t, replay)
}

func TestAcquireOriginRevertBarrier_AtomicallyClaimsMainAndOwner(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	redisRepo := redis.NewMockRedisRepository(ctrl)
	organizationID := uuid.New()
	ledgerID := uuid.New()
	reverseID := uuid.New()
	hash := "origin-hash"
	wantKey := utils.IdempotencyInternalKey(organizationID, ledgerID, hash)
	redisRepo.EXPECT().AcquireOwnedKey(gomock.Any(), wantKey, reverseID.String(), time.Duration(0)).Return(true, nil)

	handler := &TransactionHandler{Command: &command.UseCase{TransactionRedisRepo: redisRepo}}
	key, replay, err := handler.acquireOriginRevertBarrier(context.Background(), organizationID, ledgerID,
		reverseID, "", hash)

	require.NoError(t, err)
	require.NotNil(t, key)
	assert.Equal(t, wantKey, *key)
	assert.Nil(t, replay)
}

func TestAcquireOriginRevertBarrier_AmbiguousAcquireReturnsOwnedKeyForCleanup(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	redisRepo := redis.NewMockRedisRepository(ctrl)
	organizationID := uuid.New()
	ledgerID := uuid.New()
	reverseID := uuid.New()
	wantKey := utils.IdempotencyInternalKey(organizationID, ledgerID, "origin-hash")
	redisRepo.EXPECT().AcquireOwnedKey(gomock.Any(), wantKey, reverseID.String(), time.Duration(0)).
		Return(false, errors.New("lost script response"))

	handler := &TransactionHandler{Command: &command.UseCase{TransactionRedisRepo: redisRepo}}
	key, replay, err := handler.acquireOriginRevertBarrier(context.Background(), organizationID, ledgerID,
		reverseID, "origin-hash", "ignored")

	require.Error(t, err)
	require.NotNil(t, key)
	assert.Equal(t, wantKey, *key)
	assert.Nil(t, replay)
}

func TestCompleteOriginRevertBarrier_RematerializesPersistedReplayWhenFenceIsAbsent(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	redisRepo := redis.NewMockRedisRepository(ctrl)
	reverseID := uuid.New()
	originID := uuid.NewString()
	reverse := &transaction.Transaction{ID: reverseID.String(), ParentTransactionID: &originID}
	originKey := "idempotency:{origin}"
	ttl := time.Minute

	redisRepo.EXPECT().CompleteOwnedKey(gomock.Any(), originKey, reverseID.String(), gomock.Any(), ttl).Return(false, nil)
	redisRepo.EXPECT().Get(gomock.Any(), originKey).Return("", redislib.Nil)
	redisRepo.EXPECT().AcquireOwnedKey(gomock.Any(), originKey, reverseID.String(), time.Duration(0)).Return(true, nil)
	redisRepo.EXPECT().CompleteOwnedKey(gomock.Any(), originKey, reverseID.String(), gomock.Any(), ttl).Return(true, nil)

	handler := &TransactionHandler{Command: &command.UseCase{TransactionRedisRepo: redisRepo}}
	require.NoError(t, handler.completeOriginRevertBarrier(context.Background(), &originKey, reverseID, reverse, ttl))
}

func TestCompleteOriginRevertBarrier_LostCompletionResponseVerifiesExactReplay(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	redisRepo := redis.NewMockRedisRepository(ctrl)
	reverseID := uuid.New()
	originID := uuid.NewString()
	reverse := &transaction.Transaction{ID: reverseID.String(), ParentTransactionID: &originID}
	originKey := "idempotency:{origin}"
	encoded, err := json.Marshal(reverse)
	require.NoError(t, err)

	redisRepo.EXPECT().CompleteOwnedKey(gomock.Any(), originKey, reverseID.String(), gomock.Any(), time.Minute).
		Return(false, errors.New("lost completion response"))
	redisRepo.EXPECT().Get(gomock.Any(), originKey).Return(string(encoded), nil)

	handler := &TransactionHandler{Command: &command.UseCase{TransactionRedisRepo: redisRepo}}
	require.NoError(t, handler.completeOriginRevertBarrier(context.Background(), &originKey, reverseID, reverse, time.Minute))
}

func TestCompleteOriginRevertBarrier_RejectsSameIDsWithDifferentBalanceSnapshots(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	redisRepo := redis.NewMockRedisRepository(ctrl)
	reverseID := uuid.New()
	originID := uuid.NewString()
	operationID := uuid.NewString()
	before := decimal.NewFromInt(100)
	after := decimal.Zero
	versionBefore := int64(1)
	versionAfter := int64(2)
	reverse := &transaction.Transaction{
		ID: reverseID.String(), ParentTransactionID: &originID,
		Operations: []*operation.Operation{{
			ID:           operationID,
			Balance:      operation.Balance{Available: &before, Version: &versionBefore},
			BalanceAfter: operation.Balance{Available: &after, Version: &versionAfter},
		}},
	}
	cached := *reverse
	wrongAfter := decimal.NewFromInt(1)
	cached.Operations = []*operation.Operation{{
		ID:           operationID,
		Balance:      operation.Balance{Available: &before, Version: &versionBefore},
		BalanceAfter: operation.Balance{Available: &wrongAfter, Version: &versionAfter},
	}}
	originKey := "idempotency:{origin}"
	encoded, err := json.Marshal(&cached)
	require.NoError(t, err)

	redisRepo.EXPECT().CompleteOwnedKey(gomock.Any(), originKey, reverseID.String(), gomock.Any(), time.Minute).
		Return(false, errors.New("lost completion response"))
	redisRepo.EXPECT().Get(gomock.Any(), originKey).Return(string(encoded), nil)

	handler := &TransactionHandler{Command: &command.UseCase{TransactionRedisRepo: redisRepo}}
	require.Error(t, handler.completeOriginRevertBarrier(context.Background(), &originKey, reverseID, reverse, time.Minute))
}

func TestCompleteOriginRevertBarrier_ConcurrentRetryAcceptsExactWinner(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	redisRepo := redis.NewMockRedisRepository(ctrl)
	reverseID := uuid.New()
	originID := uuid.NewString()
	reverse := &transaction.Transaction{ID: reverseID.String(), ParentTransactionID: &originID}
	originKey := "idempotency:{origin}"
	encoded, err := json.Marshal(reverse)
	require.NoError(t, err)

	redisRepo.EXPECT().CompleteOwnedKey(gomock.Any(), originKey, reverseID.String(), gomock.Any(), time.Minute).Return(false, nil)
	redisRepo.EXPECT().Get(gomock.Any(), originKey).Return("", redislib.Nil)
	redisRepo.EXPECT().AcquireOwnedKey(gomock.Any(), originKey, reverseID.String(), time.Duration(0)).Return(false, nil)
	redisRepo.EXPECT().CompleteOwnedKey(gomock.Any(), originKey, reverseID.String(), gomock.Any(), time.Minute).Return(false, nil)
	redisRepo.EXPECT().Get(gomock.Any(), originKey).Return(string(encoded), nil)

	handler := &TransactionHandler{Command: &command.UseCase{TransactionRedisRepo: redisRepo}}
	require.NoError(t, handler.completeOriginRevertBarrier(context.Background(), &originKey, reverseID, reverse, time.Minute))
}

func TestFailRevertClaim_LegacyCleanupFailureKeepsDurableClaim(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	claimRepo := revertclaim.NewMockRepository(ctrl)
	redisRepo := redis.NewMockRedisRepository(ctrl)
	legacyKey := "legacy-fence"
	claim := &revertclaim.Claim{
		OrganizationID:       uuid.New(),
		LedgerID:             uuid.New(),
		OriginTransactionID:  uuid.New(),
		ReverseTransactionID: uuid.New(),
		LegacyFenceKey:       &legacyKey,
		RedisGeneration:      &testRedisGeneration,
		State:                revertclaim.StateClaimed,
	}
	legacyOwner := claim.ReverseTransactionID.String()
	claim.LegacyFenceOwner = &legacyOwner
	redisRepo.EXPECT().ReleaseOwnedKey(gomock.Any(), originRevertIdempotencyKey(claim),
		claim.ReverseTransactionID.String()).Return(true, nil)
	redisRepo.EXPECT().ReleaseOwnedKey(gomock.Any(), revertExecutionFenceKey(claim),
		claim.ReverseTransactionID.String()).Return(true, nil)
	redisRepo.EXPECT().ReleaseOwnedKey(gomock.Any(), "legacy-fence", claim.ReverseTransactionID.String()).
		Return(false, errors.New("lost cleanup response"))
	reason := "pre_movement_fence_cleanup_failed"
	claimRepo.EXPECT().Transition(gomock.Any(), claim.OrganizationID, claim.LedgerID,
		claim.OriginTransactionID, claim.ReverseTransactionID,
		revertclaim.StateReconciliationRequired, &reason).Return(nil)

	handler := &TransactionHandler{Command: &command.UseCase{
		RevertClaimRepo:      claimRepo,
		TransactionRedisRepo: redisRepo,
	}}
	err := handler.failRevertClaim(context.Background(), claim, &revertExecutionState{}, "legacy-fence", errors.New("validation failed"))

	var unavailable pkg.ServiceUnavailableError
	require.ErrorAs(t, err, &unavailable)
	assert.Equal(t, constant.ErrRevertReconciliationRequired.Error(), unavailable.Code)
	// No PostgreSQL Release expectation exists: a failed Redis cleanup must
	// retain the durable claim rather than orphan a persistent legacy fence.
}

func TestFailRevertClaim_BackupCleanupFailureKeepsEveryFence(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	claimRepo := revertclaim.NewMockRepository(ctrl)
	redisRepo := redis.NewMockRedisRepository(ctrl)
	claim := &revertclaim.Claim{
		OrganizationID:       uuid.New(),
		LedgerID:             uuid.New(),
		OriginTransactionID:  uuid.New(),
		ReverseTransactionID: uuid.New(),
		State:                revertclaim.StateClaimed,
	}
	redisRepo.EXPECT().RemoveMessageFromQueueIfStatus(gomock.Any(), utils.TransactionInternalKey(
		claim.OrganizationID, claim.LedgerID, claim.ReverseTransactionID.String()), constant.CREATED,
		claim.ReverseTransactionID.String(), mmodel.TransactionOutcomeCommitted, true).
		Return(false, errors.New("lost queue cleanup response"))
	reason := "pre_movement_backup_cleanup_failed"
	claimRepo.EXPECT().Transition(gomock.Any(), claim.OrganizationID, claim.LedgerID,
		claim.OriginTransactionID, claim.ReverseTransactionID,
		revertclaim.StateReconciliationRequired, &reason).Return(nil)

	handler := &TransactionHandler{Command: &command.UseCase{
		RevertClaimRepo:      claimRepo,
		TransactionRedisRepo: redisRepo,
	}}
	err := handler.failRevertClaim(context.Background(), claim, &revertExecutionState{SeedWritten: true}, "legacy-fence", errors.New("validation failed"))

	var unavailable pkg.ServiceUnavailableError
	require.ErrorAs(t, err, &unavailable)
	assert.Equal(t, constant.ErrRevertReconciliationRequired.Error(), unavailable.Code)
	// No origin, execution-lease, legacy, or PostgreSQL release expectation:
	// an uncertain queue cleanup preserves the complete recovery set.
}

func TestFailRevertClaim_ExpiredExecutionLeaseNeverDeletesSuccessorBarriers(t *testing.T) {
	t.Parallel()

	claim := &revertclaim.Claim{
		OrganizationID:       uuid.New(),
		LedgerID:             uuid.New(),
		OriginTransactionID:  uuid.New(),
		ReverseTransactionID: uuid.New(),
		State:                revertclaim.StateClaimed,
	}
	cause := pkg.ValidateBusinessError(constant.ErrIdempotencyKey, "RevertExecutionLease", "expired")
	handler := &TransactionHandler{}

	err := handler.failRevertClaim(context.Background(), claim,
		&revertExecutionState{BalanceAttempted: true}, "legacy-fence", cause)

	assert.ErrorIs(t, err, cause)
}

func TestReleaseFreshRevertClaim_AmbiguousAcquireMayReleaseOnlyWhenBothLegacyKeysAreAbsent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		values      map[string]string
		wantRelease bool
	}{
		{name: "script provably never wrote either key", values: map[string]string{}, wantRelease: true},
		{name: "persistent fence survived lost response", values: map[string]string{"legacy-fence": ""}},
		{name: "owner token survived lost response", values: map[string]string{"legacy-fence:owner": uuid.NewString()}},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			claimRepo := revertclaim.NewMockRepository(ctrl)
			redisRepo := redis.NewMockRedisRepository(ctrl)
			claim := &revertclaim.Claim{
				OrganizationID:       uuid.New(),
				LedgerID:             uuid.New(),
				OriginTransactionID:  uuid.New(),
				ReverseTransactionID: uuid.New(),
			}
			legacyKey := "legacy-fence"
			legacyOwner := claim.ReverseTransactionID.String()
			claim.LegacyFenceKey = &legacyKey
			claim.LegacyFenceOwner = &legacyOwner
			redisRepo.EXPECT().ReleaseOwnedKey(gomock.Any(), "legacy-fence", claim.ReverseTransactionID.String()).Return(false, nil)
			redisRepo.EXPECT().MGet(gomock.Any(), []string{"legacy-fence", "legacy-fence:owner"}).Return(tc.values, nil)
			if tc.wantRelease {
				claimRepo.EXPECT().Release(gomock.Any(), claim.OrganizationID, claim.LedgerID,
					claim.OriginTransactionID, claim.ReverseTransactionID).Return(true, nil)
			}

			handler := &TransactionHandler{Command: &command.UseCase{
				RevertClaimRepo:      claimRepo,
				TransactionRedisRepo: redisRepo,
			}}
			err := handler.releaseFreshRevertClaim(context.Background(), claim, "legacy-fence", true)
			if tc.wantRelease {
				require.NoError(t, err)

				return
			}
			require.ErrorContains(t, err, "ownership unresolved")
		})
	}
}

func TestReleaseFreshRevertClaim_RequiresPostgresClaimRelease(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	claimRepo := revertclaim.NewMockRepository(ctrl)
	redisRepo := redis.NewMockRedisRepository(ctrl)
	claim := &revertclaim.Claim{
		OrganizationID:       uuid.New(),
		LedgerID:             uuid.New(),
		OriginTransactionID:  uuid.New(),
		ReverseTransactionID: uuid.New(),
	}
	legacyKey := "legacy-fence"
	legacyOwner := claim.ReverseTransactionID.String()
	claim.LegacyFenceKey = &legacyKey
	claim.LegacyFenceOwner = &legacyOwner

	redisRepo.EXPECT().ReleaseOwnedKey(gomock.Any(), "legacy-fence", claim.ReverseTransactionID.String()).Return(true, nil)
	claimRepo.EXPECT().Release(gomock.Any(), claim.OrganizationID, claim.LedgerID,
		claim.OriginTransactionID, claim.ReverseTransactionID).Return(false, nil)

	handler := &TransactionHandler{Command: &command.UseCase{
		RevertClaimRepo:      claimRepo,
		TransactionRedisRepo: redisRepo,
	}}
	err := handler.releaseFreshRevertClaim(context.Background(), claim, "legacy-fence", false)
	require.ErrorContains(t, err, "claim was not released")
}

func TestRecoverProvenPreMovementRevert_CleansRedisBeforeReleasingClaim(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	claimRepo := revertclaim.NewMockRepository(ctrl)
	redisRepo := redis.NewMockRedisRepository(ctrl)
	legacyKey := "legacy-fence"
	claim := &revertclaim.Claim{
		OrganizationID:       uuid.New(),
		LedgerID:             uuid.New(),
		OriginTransactionID:  uuid.New(),
		ReverseTransactionID: uuid.New(),
		LegacyFenceKey:       &legacyKey,
		RedisGeneration:      &testRedisGeneration,
		State:                revertclaim.StateClaimed,
	}
	legacyOwner := claim.ReverseTransactionID.String()
	claim.LegacyFenceOwner = &legacyOwner
	backupKey := utils.TransactionInternalKey(claim.OrganizationID, claim.LedgerID, claim.ReverseTransactionID.String())
	backup, err := json.Marshal(mmodel.TransactionRedisQueue{
		TransactionID:       claim.ReverseTransactionID,
		ParentTransactionID: &claim.OriginTransactionID,
		TransactionStatus:   constant.CREATED,
		AttemptOwner:        claim.ReverseTransactionID.String(),
		ExpectedOutcome:     mmodel.TransactionOutcomeCommitted,
	})
	require.NoError(t, err)

	redisRepo.EXPECT().TransactionEconomicEvidenceExists(gomock.Any(), claim.OrganizationID, claim.LedgerID,
		claim.ReverseTransactionID, testRedisGeneration).Return(true, true, nil)
	redisRepo.EXPECT().ReadMessageFromQueue(gomock.Any(), backupKey).Return(backup, nil)
	redisRepo.EXPECT().Get(gomock.Any(), utils.TransactionBalanceOutcomeKey(claim.OrganizationID,
		claim.LedgerID, claim.ReverseTransactionID)).Return("", nil)
	redisRepo.EXPECT().MGet(gomock.Any(), []string{revertExecutionFenceKey(claim),
		revertExecutionFenceKey(claim) + ":owner"}).Return(map[string]string{}, nil)
	claimRepo.EXPECT().BeginPreMutationRecovery(gomock.Any(), claim.OrganizationID, claim.LedgerID,
		claim.OriginTransactionID, claim.ReverseTransactionID).Return(true, nil)
	redisRepo.EXPECT().ReleaseOwnedKey(gomock.Any(), originRevertIdempotencyKey(claim),
		claim.ReverseTransactionID.String()).Return(true, nil)
	redisRepo.EXPECT().ReleaseProvenPreMovementRevert(gomock.Any(), claim.OrganizationID, claim.LedgerID,
		claim.OriginTransactionID, claim.ReverseTransactionID, constant.CREATED, gomock.Any()).Return(true, true, nil)
	redisRepo.EXPECT().ReleaseOwnedKey(gomock.Any(), legacyKey, claim.ReverseTransactionID.String()).Return(true, nil)
	claimRepo.EXPECT().Release(gomock.Any(), claim.OrganizationID, claim.LedgerID,
		claim.OriginTransactionID, claim.ReverseTransactionID).Return(true, nil)

	handler := &TransactionHandler{Command: &command.UseCase{
		RevertClaimRepo:      claimRepo,
		TransactionRedisRepo: redisRepo,
	}, RevertUpdateFreeze: &revertUpdateFreezeStub{}}
	recovered, err := handler.recoverProvenPreMovementRevert(context.Background(), claim)

	require.NoError(t, err)
	assert.True(t, recovered)
}

func TestRecoverProvenPreMovementRevert_GenerationMismatchNeverReleasesClaim(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	claimRepo := revertclaim.NewMockRepository(ctrl)
	redisRepo := redis.NewMockRedisRepository(ctrl)
	claim := &revertclaim.Claim{
		OrganizationID:       uuid.New(),
		LedgerID:             uuid.New(),
		OriginTransactionID:  uuid.New(),
		ReverseTransactionID: uuid.New(),
		RedisGeneration:      &testRedisGeneration,
		State:                revertclaim.StateClaimed,
	}
	redisRepo.EXPECT().TransactionEconomicEvidenceExists(gomock.Any(), claim.OrganizationID, claim.LedgerID,
		claim.ReverseTransactionID, testRedisGeneration).Return(false, false, nil)
	claimRepo.EXPECT().Transition(gomock.Any(), claim.OrganizationID, claim.LedgerID,
		claim.OriginTransactionID, claim.ReverseTransactionID, revertclaim.StateReconciliationRequired, gomock.Any()).
		Return(nil)

	handler := &TransactionHandler{Command: &command.UseCase{
		RevertClaimRepo:      claimRepo,
		TransactionRedisRepo: redisRepo,
	}, RevertUpdateFreeze: &revertUpdateFreezeStub{}}
	recovered, err := handler.recoverProvenPreMovementRevert(context.Background(), claim)

	assert.False(t, recovered)
	var unavailable pkg.ServiceUnavailableError
	require.ErrorAs(t, err, &unavailable)
	assert.Equal(t, constant.ErrRevertReconciliationRequired.Error(), unavailable.Code)
}

func TestRecoverProvenPreMovementRevert_TerminalBackupWinningCleanupRaceIsNeverReleased(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	claimRepo := revertclaim.NewMockRepository(ctrl)
	redisRepo := redis.NewMockRedisRepository(ctrl)
	claim := &revertclaim.Claim{
		OrganizationID:       uuid.New(),
		LedgerID:             uuid.New(),
		OriginTransactionID:  uuid.New(),
		ReverseTransactionID: uuid.New(),
		RedisGeneration:      &testRedisGeneration,
		State:                revertclaim.StateClaimed,
	}
	backupKey := utils.TransactionInternalKey(claim.OrganizationID, claim.LedgerID,
		claim.ReverseTransactionID.String())
	backup, err := json.Marshal(mmodel.TransactionRedisQueue{
		TransactionID:       claim.ReverseTransactionID,
		ParentTransactionID: &claim.OriginTransactionID,
		TransactionStatus:   constant.CREATED,
		AttemptOwner:        claim.ReverseTransactionID.String(),
		ExpectedOutcome:     mmodel.TransactionOutcomeCommitted,
	})
	require.NoError(t, err)

	redisRepo.EXPECT().TransactionEconomicEvidenceExists(gomock.Any(), claim.OrganizationID, claim.LedgerID,
		claim.ReverseTransactionID, testRedisGeneration).Return(true, true, nil)
	redisRepo.EXPECT().Get(gomock.Any(), utils.TransactionBalanceOutcomeKey(claim.OrganizationID,
		claim.LedgerID, claim.ReverseTransactionID)).Return("", nil)
	redisRepo.EXPECT().MGet(gomock.Any(), []string{revertExecutionFenceKey(claim),
		revertExecutionFenceKey(claim) + ":owner"}).Return(map[string]string{}, nil)
	redisRepo.EXPECT().ReadMessageFromQueue(gomock.Any(), backupKey).Return(backup, nil)
	claimRepo.EXPECT().BeginPreMutationRecovery(gomock.Any(), claim.OrganizationID, claim.LedgerID,
		claim.OriginTransactionID, claim.ReverseTransactionID).Return(true, nil)
	redisRepo.EXPECT().ReleaseProvenPreMovementRevert(gomock.Any(), claim.OrganizationID, claim.LedgerID,
		claim.OriginTransactionID, claim.ReverseTransactionID, constant.CREATED, gomock.Any()).Return(false, true, nil)
	reason := "pre_movement_backup_cleanup_failed"
	claimRepo.EXPECT().Transition(gomock.Any(), claim.OrganizationID, claim.LedgerID,
		claim.OriginTransactionID, claim.ReverseTransactionID,
		revertclaim.StateReconciliationRequired, &reason).Return(nil)

	handler := &TransactionHandler{Command: &command.UseCase{
		RevertClaimRepo:      claimRepo,
		TransactionRedisRepo: redisRepo,
	}, RevertUpdateFreeze: &revertUpdateFreezeStub{}}
	recovered, err := handler.recoverProvenPreMovementRevert(context.Background(), claim)

	assert.False(t, recovered)
	var unavailable pkg.ServiceUnavailableError
	require.ErrorAs(t, err, &unavailable)
	assert.Equal(t, constant.ErrRevertReconciliationRequired.Error(), unavailable.Code)
	// No Redis barrier or PostgreSQL claim release is expected after the exact
	// pre-movement envelope was replaced by a terminal state.
}

func TestRecoverProvenPreMovementRevert_MissingSeedAfterExpiredLeaseProvesCrashBeforeSeed(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	claimRepo := revertclaim.NewMockRepository(ctrl)
	redisRepo := redis.NewMockRedisRepository(ctrl)
	claim := &revertclaim.Claim{
		OrganizationID:       uuid.New(),
		LedgerID:             uuid.New(),
		OriginTransactionID:  uuid.New(),
		ReverseTransactionID: uuid.New(),
		RedisGeneration:      &testRedisGeneration,
		State:                revertclaim.StateClaimed,
	}
	backupKey := utils.TransactionInternalKey(claim.OrganizationID, claim.LedgerID, claim.ReverseTransactionID.String())

	redisRepo.EXPECT().TransactionEconomicEvidenceExists(gomock.Any(), claim.OrganizationID, claim.LedgerID,
		claim.ReverseTransactionID, testRedisGeneration).Return(false, true, nil)
	redisRepo.EXPECT().Get(gomock.Any(), utils.TransactionBalanceOutcomeKey(claim.OrganizationID,
		claim.LedgerID, claim.ReverseTransactionID)).Return("", nil)
	redisRepo.EXPECT().MGet(gomock.Any(), []string{revertExecutionFenceKey(claim),
		revertExecutionFenceKey(claim) + ":owner"}).Return(map[string]string{}, nil)
	redisRepo.EXPECT().ReadMessageFromQueue(gomock.Any(), backupKey).Return(nil, redislib.Nil)
	claimRepo.EXPECT().BeginPreMutationRecovery(gomock.Any(), claim.OrganizationID, claim.LedgerID,
		claim.OriginTransactionID, claim.ReverseTransactionID).Return(true, nil)
	redisRepo.EXPECT().ReleaseProvenPreMovementRevert(gomock.Any(), claim.OrganizationID, claim.LedgerID,
		claim.OriginTransactionID, claim.ReverseTransactionID, constant.CREATED, gomock.Any()).Return(true, true, nil)
	redisRepo.EXPECT().ReleaseOwnedKey(gomock.Any(), originRevertIdempotencyKey(claim),
		claim.ReverseTransactionID.String()).Return(true, nil)
	claimRepo.EXPECT().Release(gomock.Any(), claim.OrganizationID, claim.LedgerID,
		claim.OriginTransactionID, claim.ReverseTransactionID).Return(true, nil)

	handler := &TransactionHandler{Command: &command.UseCase{
		RevertClaimRepo:      claimRepo,
		TransactionRedisRepo: redisRepo,
	}, RevertUpdateFreeze: &revertUpdateFreezeStub{}}
	recovered, err := handler.recoverProvenPreMovementRevert(context.Background(), claim)

	require.NoError(t, err)
	assert.True(t, recovered)
}

func TestRecoverProvenPreMovementRevert_RequiresExactOriginInSeed(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	redisRepo := redis.NewMockRedisRepository(ctrl)
	claim := &revertclaim.Claim{
		OrganizationID:       uuid.New(),
		LedgerID:             uuid.New(),
		OriginTransactionID:  uuid.New(),
		ReverseTransactionID: uuid.New(),
		RedisGeneration:      &testRedisGeneration,
		State:                revertclaim.StateClaimed,
	}
	backup, err := json.Marshal(mmodel.TransactionRedisQueue{TransactionID: claim.ReverseTransactionID})
	require.NoError(t, err)
	redisRepo.EXPECT().TransactionEconomicEvidenceExists(gomock.Any(), claim.OrganizationID, claim.LedgerID,
		claim.ReverseTransactionID, testRedisGeneration).Return(true, true, nil)
	redisRepo.EXPECT().ReadMessageFromQueue(gomock.Any(),
		utils.TransactionInternalKey(claim.OrganizationID, claim.LedgerID, claim.ReverseTransactionID.String())).Return(backup, nil)
	redisRepo.EXPECT().Get(gomock.Any(), utils.TransactionBalanceOutcomeKey(claim.OrganizationID,
		claim.LedgerID, claim.ReverseTransactionID)).Return("", nil)
	redisRepo.EXPECT().MGet(gomock.Any(), []string{revertExecutionFenceKey(claim),
		revertExecutionFenceKey(claim) + ":owner"}).Return(map[string]string{}, nil)

	handler := &TransactionHandler{Command: &command.UseCase{TransactionRedisRepo: redisRepo},
		RevertUpdateFreeze: &revertUpdateFreezeStub{}}
	recovered, err := handler.recoverProvenPreMovementRevert(context.Background(), claim)

	require.NoError(t, err)
	assert.False(t, recovered)
}

func TestRecoverProvenPreMovementRevert_ActiveExecutionLeaseCannotBeRecovered(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	redisRepo := redis.NewMockRedisRepository(ctrl)
	claim := &revertclaim.Claim{
		OrganizationID:       uuid.New(),
		LedgerID:             uuid.New(),
		OriginTransactionID:  uuid.New(),
		ReverseTransactionID: uuid.New(),
		RedisGeneration:      &testRedisGeneration,
		State:                revertclaim.StateClaimed,
	}
	redisRepo.EXPECT().TransactionEconomicEvidenceExists(gomock.Any(), claim.OrganizationID, claim.LedgerID,
		claim.ReverseTransactionID, testRedisGeneration).Return(true, true, nil)
	redisRepo.EXPECT().Get(gomock.Any(), utils.TransactionBalanceOutcomeKey(claim.OrganizationID,
		claim.LedgerID, claim.ReverseTransactionID)).Return("", nil)
	redisRepo.EXPECT().MGet(gomock.Any(), []string{revertExecutionFenceKey(claim),
		revertExecutionFenceKey(claim) + ":owner"}).Return(map[string]string{
		revertExecutionFenceKey(claim):            "",
		revertExecutionFenceKey(claim) + ":owner": claim.ReverseTransactionID.String(),
	}, nil)

	handler := &TransactionHandler{Command: &command.UseCase{TransactionRedisRepo: redisRepo},
		RevertUpdateFreeze: &revertUpdateFreezeStub{}}
	recovered, err := handler.recoverProvenPreMovementRevert(context.Background(), claim)

	require.NoError(t, err)
	assert.False(t, recovered)
}

func TestRecoverProvenPreMovementRevert_ResumesIdempotentRecoveringCleanup(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	claimRepo := revertclaim.NewMockRepository(ctrl)
	redisRepo := redis.NewMockRedisRepository(ctrl)
	legacyKey := "legacy-fence"
	claim := &revertclaim.Claim{
		OrganizationID:       uuid.New(),
		LedgerID:             uuid.New(),
		OriginTransactionID:  uuid.New(),
		ReverseTransactionID: uuid.New(),
		LegacyFenceKey:       &legacyKey,
		RedisGeneration:      &testRedisGeneration,
		State:                revertclaim.StateRecovering,
	}
	legacyOwner := claim.ReverseTransactionID.String()
	claim.LegacyFenceOwner = &legacyOwner
	backupKey := utils.TransactionInternalKey(claim.OrganizationID, claim.LedgerID, claim.ReverseTransactionID.String())
	redisRepo.EXPECT().TransactionEconomicEvidenceExists(gomock.Any(), claim.OrganizationID, claim.LedgerID,
		claim.ReverseTransactionID, testRedisGeneration).Return(false, true, nil)
	redisRepo.EXPECT().Get(gomock.Any(), utils.TransactionBalanceOutcomeKey(claim.OrganizationID,
		claim.LedgerID, claim.ReverseTransactionID)).Return("", nil)
	redisRepo.EXPECT().MGet(gomock.Any(), []string{revertExecutionFenceKey(claim),
		revertExecutionFenceKey(claim) + ":owner"}).Return(map[string]string{}, nil)
	redisRepo.EXPECT().ReadMessageFromQueue(gomock.Any(), backupKey).Return(nil, redislib.Nil)

	claimRepo.EXPECT().BeginPreMutationRecovery(gomock.Any(), claim.OrganizationID, claim.LedgerID,
		claim.OriginTransactionID, claim.ReverseTransactionID).Return(true, nil)
	redisRepo.EXPECT().ReleaseProvenPreMovementRevert(gomock.Any(), claim.OrganizationID, claim.LedgerID,
		claim.OriginTransactionID, claim.ReverseTransactionID, constant.CREATED, gomock.Any()).Return(true, true, nil)
	redisRepo.EXPECT().ReleaseOwnedKey(gomock.Any(), originRevertIdempotencyKey(claim),
		claim.ReverseTransactionID.String()).Return(true, nil)
	redisRepo.EXPECT().ReleaseOwnedKey(gomock.Any(), legacyKey, claim.ReverseTransactionID.String()).Return(false, nil)
	redisRepo.EXPECT().MGet(gomock.Any(), []string{legacyKey, legacyKey + ":owner"}).Return(map[string]string{}, nil)
	claimRepo.EXPECT().Release(gomock.Any(), claim.OrganizationID, claim.LedgerID,
		claim.OriginTransactionID, claim.ReverseTransactionID).Return(true, nil)

	handler := &TransactionHandler{Command: &command.UseCase{
		RevertClaimRepo:      claimRepo,
		TransactionRedisRepo: redisRepo,
	}, RevertUpdateFreeze: &revertUpdateFreezeStub{}}
	recovered, err := handler.recoverProvenPreMovementRevert(context.Background(), claim)

	require.NoError(t, err)
	assert.True(t, recovered)
}

func TestRecoverProvenPreMovementRevert_FinalLeavesForeignLegacyFenceUntouched(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	claimRepo := revertclaim.NewMockRepository(ctrl)
	redisRepo := redis.NewMockRedisRepository(ctrl)
	legacyKey := "legacy-fence"
	claim := &revertclaim.Claim{
		OrganizationID:       uuid.New(),
		LedgerID:             uuid.New(),
		OriginTransactionID:  uuid.New(),
		ReverseTransactionID: uuid.New(),
		LegacyFenceKey:       &legacyKey,
		RedisGeneration:      &testRedisGeneration,
		State:                revertclaim.StateRecovering,
	}
	legacyOwner := claim.ReverseTransactionID.String()
	claim.LegacyFenceOwner = &legacyOwner
	foreignOwner := uuid.NewString()
	backupKey := utils.TransactionInternalKey(claim.OrganizationID, claim.LedgerID, claim.ReverseTransactionID.String())
	redisRepo.EXPECT().TransactionEconomicEvidenceExists(gomock.Any(), claim.OrganizationID, claim.LedgerID,
		claim.ReverseTransactionID, testRedisGeneration).Return(false, true, nil)
	redisRepo.EXPECT().Get(gomock.Any(), utils.TransactionBalanceOutcomeKey(claim.OrganizationID,
		claim.LedgerID, claim.ReverseTransactionID)).Return("", nil)
	redisRepo.EXPECT().MGet(gomock.Any(), []string{revertExecutionFenceKey(claim),
		revertExecutionFenceKey(claim) + ":owner"}).Return(map[string]string{}, nil)
	redisRepo.EXPECT().ReadMessageFromQueue(gomock.Any(), backupKey).Return(nil, redislib.Nil)

	claimRepo.EXPECT().BeginPreMutationRecovery(gomock.Any(), claim.OrganizationID, claim.LedgerID,
		claim.OriginTransactionID, claim.ReverseTransactionID).Return(true, nil)
	redisRepo.EXPECT().ReleaseProvenPreMovementRevert(gomock.Any(), claim.OrganizationID, claim.LedgerID,
		claim.OriginTransactionID, claim.ReverseTransactionID, constant.CREATED, gomock.Any()).Return(true, true, nil)
	redisRepo.EXPECT().ReleaseOwnedKey(gomock.Any(), originRevertIdempotencyKey(claim),
		claim.ReverseTransactionID.String()).Return(true, nil)
	redisRepo.EXPECT().ReleaseOwnedKey(gomock.Any(), legacyKey, claim.ReverseTransactionID.String()).Return(false, nil)
	redisRepo.EXPECT().MGet(gomock.Any(), []string{legacyKey, legacyKey + ":owner"}).Return(map[string]string{
		legacyKey:            "",
		legacyKey + ":owner": foreignOwner,
	}, nil)
	claimRepo.EXPECT().Release(gomock.Any(), claim.OrganizationID, claim.LedgerID,
		claim.OriginTransactionID, claim.ReverseTransactionID).Return(true, nil)

	handler := &TransactionHandler{
		RevertIdempotencyMode: revertIdempotencyModeFinal,
		Command: &command.UseCase{
			RevertClaimRepo:      claimRepo,
			TransactionRedisRepo: redisRepo,
		},
		RevertUpdateFreeze: &revertUpdateFreezeStub{},
	}
	recovered, err := handler.recoverProvenPreMovementRevert(context.Background(), claim)

	require.NoError(t, err)
	assert.True(t, recovered)
}

func TestRecoverProvenPreMovementRevert_StaleOwnerCannotDeleteSuccessorOriginFence(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	claimRepo := revertclaim.NewMockRepository(ctrl)
	redisRepo := redis.NewMockRedisRepository(ctrl)
	claim := &revertclaim.Claim{
		OrganizationID:       uuid.New(),
		LedgerID:             uuid.New(),
		OriginTransactionID:  uuid.New(),
		ReverseTransactionID: uuid.New(),
		RedisGeneration:      &testRedisGeneration,
		State:                revertclaim.StateRecovering,
	}
	originKey := originRevertIdempotencyKey(claim)
	backupKey := utils.TransactionInternalKey(claim.OrganizationID, claim.LedgerID,
		claim.ReverseTransactionID.String())
	redisRepo.EXPECT().TransactionEconomicEvidenceExists(gomock.Any(), claim.OrganizationID, claim.LedgerID,
		claim.ReverseTransactionID, testRedisGeneration).Return(false, true, nil)
	redisRepo.EXPECT().Get(gomock.Any(), utils.TransactionBalanceOutcomeKey(claim.OrganizationID,
		claim.LedgerID, claim.ReverseTransactionID)).Return("", nil)
	redisRepo.EXPECT().MGet(gomock.Any(), []string{revertExecutionFenceKey(claim),
		revertExecutionFenceKey(claim) + ":owner"}).Return(map[string]string{}, nil)
	redisRepo.EXPECT().ReadMessageFromQueue(gomock.Any(), backupKey).Return(nil, redislib.Nil)
	claimRepo.EXPECT().BeginPreMutationRecovery(gomock.Any(), claim.OrganizationID, claim.LedgerID,
		claim.OriginTransactionID, claim.ReverseTransactionID).Return(true, nil)
	redisRepo.EXPECT().ReleaseProvenPreMovementRevert(gomock.Any(), claim.OrganizationID, claim.LedgerID,
		claim.OriginTransactionID, claim.ReverseTransactionID, constant.CREATED, gomock.Any()).Return(true, true, nil)
	redisRepo.EXPECT().ReleaseOwnedKey(gomock.Any(), originKey, claim.ReverseTransactionID.String()).Return(false, nil)
	redisRepo.EXPECT().MGet(gomock.Any(), []string{originKey, originKey + ":owner"}).Return(map[string]string{
		originKey:            "",
		originKey + ":owner": uuid.NewString(),
	}, nil)
	reason := "pre_movement_origin_fence_cleanup_failed"
	claimRepo.EXPECT().Transition(gomock.Any(), claim.OrganizationID, claim.LedgerID,
		claim.OriginTransactionID, claim.ReverseTransactionID,
		revertclaim.StateReconciliationRequired, &reason).Return(nil)

	handler := &TransactionHandler{Command: &command.UseCase{
		RevertClaimRepo:      claimRepo,
		TransactionRedisRepo: redisRepo,
	}, RevertUpdateFreeze: &revertUpdateFreezeStub{}}
	recovered, err := handler.recoverProvenPreMovementRevert(context.Background(), claim)

	assert.False(t, recovered)
	var unavailable pkg.ServiceUnavailableError
	require.ErrorAs(t, err, &unavailable)
	assert.Equal(t, constant.ErrRevertReconciliationRequired.Error(), unavailable.Code)
	// No queue, legacy, or PostgreSQL release expectation exists: ownership
	// mismatch stops stale cleanup at the successor's origin fence.
}

func TestAssignDeterministicRevertOperationIDs_ReplayReusesExactSet(t *testing.T) {
	t.Parallel()

	reverseID := uuid.New()
	first := []*operation.Operation{{}, {}, {}}
	second := []*operation.Operation{{}, {}, {}}
	require.NoError(t, assignDeterministicRevertOperationIDs(reverseID.String(), first))
	require.NoError(t, assignDeterministicRevertOperationIDs(reverseID.String(), second))

	seen := make(map[string]struct{}, len(first))
	for index := range first {
		assert.Equal(t, first[index].ID, second[index].ID)
		_, duplicate := seen[first[index].ID]
		assert.False(t, duplicate)
		seen[first[index].ID] = struct{}{}
		_, err := uuid.Parse(first[index].ID)
		require.NoError(t, err)
		assert.Equal(t, uuid.Version(7), uuid.MustParse(first[index].ID).Version())
	}

	other := []*operation.Operation{{}}
	require.NoError(t, assignDeterministicRevertOperationIDs(uuid.NewString(), other))
	assert.NotEqual(t, first[0].ID, other[0].ID)
}

func TestMayReleaseRevertFences_AmbiguousAndPostMutationFailuresStayFenced(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		execution *revertExecutionState
		cause     error
		want      bool
	}{
		{
			name: "ordinary transaction cleanup remains unchanged",
			want: true,
		},
		{
			name:      "pre movement failure releases all fences",
			execution: &revertExecutionState{},
			cause:     errors.New("route validation unavailable"),
			want:      true,
		},
		{
			name:      "lost seed response preserves postgres legacy and origin fences",
			execution: &revertExecutionState{SeedWriteAmbiguous: true},
			cause:     errors.New("i/o timeout after seed write"),
		},
		{
			name:      "Lua rejection proves rollback and releases all fences",
			execution: &revertExecutionState{BalanceAttempted: true},
			cause:     pkg.ValidateBusinessError(constant.ErrInsufficientFunds, constant.EntityBalance),
			want:      true,
		},
		{
			name:      "lost Lua response preserves postgres legacy and origin fences",
			execution: &revertExecutionState{BalanceAttempted: true},
			cause:     errors.New("i/o timeout after command dispatch"),
		},
		{
			name:      "post movement failure preserves postgres legacy and origin fences",
			execution: &revertExecutionState{BalanceAttempted: true, BalanceCommitted: true},
			cause:     errors.New("postgres persistence unavailable"),
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, mayReleaseRevertFences(tc.execution, tc.cause))
		})
	}
}

func TestCompleteLegacyRevertBarrier_FailureAfterMovementNeverReleasesFence(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	claimRepo := revertclaim.NewMockRepository(ctrl)
	redisRepo := redis.NewMockRedisRepository(ctrl)
	claim := &revertclaim.Claim{
		OrganizationID:       uuid.New(),
		LedgerID:             uuid.New(),
		OriginTransactionID:  uuid.New(),
		ReverseTransactionID: uuid.New(),
		State:                revertclaim.StateCompleted,
	}
	legacyFenceKey := "legacy-fence"
	claim.LegacyFenceKey = &legacyFenceKey
	legacyFenceOwner := claim.ReverseTransactionID.String()
	claim.LegacyFenceOwner = &legacyFenceOwner
	originID := claim.OriginTransactionID.String()
	reverse := &transaction.Transaction{
		ID:                  claim.ReverseTransactionID.String(),
		ParentTransactionID: &originID,
	}

	redisRepo.EXPECT().CompleteOwnedKey(gomock.Any(), "legacy-fence", claim.ReverseTransactionID.String(),
		gomock.Any(), gomock.Any()).Return(false, errors.New("connection lost during completion"))
	reason := "legacy_revert_fence_completion_failed"
	claimRepo.EXPECT().Transition(gomock.Any(), claim.OrganizationID, claim.LedgerID,
		claim.OriginTransactionID, claim.ReverseTransactionID,
		revertclaim.StateReconciliationRequired, &reason).Return(nil)

	handler := &TransactionHandler{Command: &command.UseCase{
		RevertClaimRepo:      claimRepo,
		TransactionRedisRepo: redisRepo,
	}}
	err := handler.completeLegacyRevertBarrier(context.Background(), claim, "legacy-fence", reverse)

	var unavailable pkg.ServiceUnavailableError
	require.ErrorAs(t, err, &unavailable)
	assert.Equal(t, constant.ErrRevertReconciliationRequired.Error(), unavailable.Code)
	// No ReleaseOwnedKey, Del, or claim Release expectation exists: any cleanup
	// in this post-movement failure path makes the test fail through gomock.
}

func TestSettleFinalLegacyRevertBarrier_LostCompletionWithOwnerStillPresentRequiresReconciliation(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	claimRepo := revertclaim.NewMockRepository(ctrl)
	redisRepo := redis.NewMockRedisRepository(ctrl)
	legacyKey := "legacy-fence"
	claim := &revertclaim.Claim{
		OrganizationID:       uuid.New(),
		LedgerID:             uuid.New(),
		OriginTransactionID:  uuid.New(),
		ReverseTransactionID: uuid.New(),
		LegacyFenceKey:       &legacyKey,
	}
	legacyOwner := claim.ReverseTransactionID.String()
	claim.LegacyFenceOwner = &legacyOwner
	originID := claim.OriginTransactionID.String()
	reverse := &transaction.Transaction{
		ID:                  claim.ReverseTransactionID.String(),
		ParentTransactionID: &originID,
	}
	replay, err := json.Marshal(reverse)
	require.NoError(t, err)

	redisRepo.EXPECT().MGet(gomock.Any(), []string{legacyKey, legacyKey + ":owner"}).Return(map[string]string{
		legacyKey:            "",
		legacyKey + ":owner": legacyOwner,
	}, nil)
	redisRepo.EXPECT().CompleteOwnedKey(gomock.Any(), legacyKey, legacyOwner, gomock.Any(), gomock.Any()).
		Return(false, errors.New("lost completion response"))
	redisRepo.EXPECT().MGet(gomock.Any(), []string{legacyKey, legacyKey + ":owner"}).Return(map[string]string{
		legacyKey:            string(replay),
		legacyKey + ":owner": legacyOwner,
	}, nil)
	reason := "legacy_revert_fence_verification_failed"
	claimRepo.EXPECT().Transition(gomock.Any(), claim.OrganizationID, claim.LedgerID,
		claim.OriginTransactionID, claim.ReverseTransactionID,
		revertclaim.StateReconciliationRequired, &reason).Return(nil)

	handler := &TransactionHandler{Command: &command.UseCase{
		RevertClaimRepo:      claimRepo,
		TransactionRedisRepo: redisRepo,
	}}
	err = handler.settleFinalLegacyRevertBarrier(context.Background(), claim, legacyKey, reverse)

	var unavailable pkg.ServiceUnavailableError
	require.ErrorAs(t, err, &unavailable)
	assert.Equal(t, constant.ErrRevertReconciliationRequired.Error(), unavailable.Code)
}

func TestResolveDurableRevertClaim_CrashAfterBalanceNeverStartsAnotherMutation(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	transactionRepo := transaction.NewMockRepository(ctrl)
	claim := &revertclaim.Claim{
		OrganizationID:       uuid.New(),
		LedgerID:             uuid.New(),
		OriginTransactionID:  uuid.New(),
		ReverseTransactionID: uuid.New(),
		State:                revertclaim.StateMutated,
	}
	transactionRepo.EXPECT().FindByParentID(gomock.Any(), claim.OrganizationID, claim.LedgerID,
		claim.OriginTransactionID).Return(nil, nil)

	handler := &TransactionHandler{Query: &query.UseCase{TransactionRepo: transactionRepo}}
	replay, replayed, err := handler.resolveDurableRevertClaim(context.Background(), claim)

	assert.Nil(t, replay)
	assert.False(t, replayed)
	require.Error(t, err)
	var unavailable pkg.ServiceUnavailableError
	require.ErrorAs(t, err, &unavailable)
	assert.Equal(t, constant.ErrRevertReconciliationRequired.Error(), unavailable.Code)
}

func TestResolveDurableRevertClaim_HardCrashAfterLuaUsesAtomicOutcome(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	transactionRepo := transaction.NewMockRepository(ctrl)
	claimRepo := revertclaim.NewMockRepository(ctrl)
	redisRepo := redis.NewMockRedisRepository(ctrl)
	claim := &revertclaim.Claim{
		OrganizationID:       uuid.New(),
		LedgerID:             uuid.New(),
		OriginTransactionID:  uuid.New(),
		ReverseTransactionID: uuid.New(),
		State:                revertclaim.StateClaimed,
	}
	backup, err := json.Marshal(mmodel.TransactionRedisQueue{
		TransactionID:       claim.ReverseTransactionID,
		ParentTransactionID: &claim.OriginTransactionID,
		BalancesAfter:       []mmodel.BalanceRedis{{ID: uuid.NewString()}},
	})
	require.NoError(t, err)

	transactionRepo.EXPECT().FindByParentID(gomock.Any(), claim.OrganizationID, claim.LedgerID,
		claim.OriginTransactionID).Return(nil, nil)
	redisRepo.EXPECT().ReadMessageFromQueue(gomock.Any(),
		utils.TransactionInternalKey(claim.OrganizationID, claim.LedgerID, claim.ReverseTransactionID.String())).Return(backup, nil)
	reason := "reverse_balance_committed_before_persistence"
	claimRepo.EXPECT().Transition(gomock.Any(), claim.OrganizationID, claim.LedgerID,
		claim.OriginTransactionID, claim.ReverseTransactionID,
		revertclaim.StateReconciliationRequired, &reason).Return(nil)

	handler := &TransactionHandler{
		Query: &query.UseCase{TransactionRepo: transactionRepo},
		Command: &command.UseCase{
			RevertClaimRepo:      claimRepo,
			TransactionRedisRepo: redisRepo,
		},
	}
	replay, replayed, err := handler.resolveDurableRevertClaim(context.Background(), claim)

	assert.Nil(t, replay)
	assert.False(t, replayed)
	var unavailable pkg.ServiceUnavailableError
	require.ErrorAs(t, err, &unavailable)
	assert.Equal(t, constant.ErrRevertReconciliationRequired.Error(), unavailable.Code)
}

func TestReverseMatchesClaim_RequiresReservedIDAndExactOrigin(t *testing.T) {
	t.Parallel()

	claim := &revertclaim.Claim{
		OriginTransactionID:  uuid.New(),
		ReverseTransactionID: uuid.New(),
	}
	originID := claim.OriginTransactionID.String()
	matching := &transaction.Transaction{ID: claim.ReverseTransactionID.String(), ParentTransactionID: &originID}
	assert.True(t, reverseMatchesClaim(matching, claim))

	wrongOrigin := uuid.NewString()
	assert.False(t, reverseMatchesClaim(&transaction.Transaction{
		ID: claim.ReverseTransactionID.String(), ParentTransactionID: &wrongOrigin,
	}, claim))
	assert.False(t, reverseMatchesClaim(&transaction.Transaction{
		ID: uuid.NewString(), ParentTransactionID: &originID,
	}, claim))
}

func TestAdoptPersistedReverse_MissingOperationsRequiresReconciliation(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	transactionRepo := transaction.NewMockRepository(ctrl)
	claimRepo := revertclaim.NewMockRepository(ctrl)
	organizationID := uuid.New()
	ledgerID := uuid.New()
	originID := uuid.New()
	reverseID := uuid.New()
	originIDString := originID.String()
	persisted := &transaction.Transaction{ID: reverseID.String(), ParentTransactionID: &originIDString}
	claim := &revertclaim.Claim{
		OrganizationID:       organizationID,
		LedgerID:             ledgerID,
		OriginTransactionID:  originID,
		ReverseTransactionID: reverseID,
		State:                revertclaim.StateClaimed,
	}

	claimRepo.EXPECT().Claim(gomock.Any(), organizationID, ledgerID, originID, reverseID,
		nil, nil, nil, nil, nil).Return(claim, true, nil)
	transactionRepo.EXPECT().FindWithOperations(gomock.Any(), organizationID, ledgerID, reverseID).Return(nil, nil)
	reason := "reverse_transaction_missing_operations"
	claimRepo.EXPECT().Transition(gomock.Any(), organizationID, ledgerID, originID, reverseID,
		revertclaim.StateReconciliationRequired, &reason).Return(nil)

	handler := &TransactionHandler{
		Query:   &query.UseCase{TransactionRepo: transactionRepo},
		Command: &command.UseCase{RevertClaimRepo: claimRepo},
	}
	result, replayed, err := handler.adoptPersistedReverse(context.Background(), organizationID, ledgerID,
		originID, persisted, nil, nil, nil)
	assert.Nil(t, result)
	assert.False(t, replayed)
	var unavailable pkg.ServiceUnavailableError
	require.ErrorAs(t, err, &unavailable)
	assert.Equal(t, constant.ErrRevertReconciliationRequired.Error(), unavailable.Code)
}

func TestAdoptPersistedReverse_FinalCompletesPersistedBridgeFenceAndExactOutcomeCleanup(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	transactionRepo := transaction.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)
	claimRepo := revertclaim.NewMockRepository(ctrl)
	redisRepo := redis.NewMockRedisRepository(ctrl)
	organizationID := uuid.New()
	ledgerID := uuid.New()
	originID := uuid.New()
	reverseID := uuid.New()
	legacyKey := "idempotency:{persisted-bridge-h1}:original-payload"
	legacyOwner := reverseID.String()
	rolloutMode := "bridge"
	rolloutToken := "persisted-bridge-generation"
	originIDString := originID.String()
	operationID := uuid.NewString()
	economicOperation, balanceAfter := completeRevertEconomicOperation(organizationID, ledgerID, reverseID, operationID)
	persisted := &transaction.Transaction{
		ID:                  reverseID.String(),
		OrganizationID:      organizationID.String(),
		LedgerID:            ledgerID.String(),
		ParentTransactionID: &originIDString,
		Status:              transaction.Status{Code: constant.APPROVED},
		Operations:          []*operation.Operation{economicOperation},
	}
	claim := &revertclaim.Claim{
		OrganizationID:       organizationID,
		LedgerID:             ledgerID,
		OriginTransactionID:  originID,
		ReverseTransactionID: reverseID,
		LegacyFenceKey:       &legacyKey,
		LegacyFenceOwner:     &legacyOwner,
		RolloutMode:          &rolloutMode,
		RolloutToken:         &rolloutToken,
		State:                revertclaim.StateMutated,
	}
	backup, err := json.Marshal(mmodel.TransactionRedisQueue{
		TransactionID:       reverseID,
		ParentTransactionID: &originID,
		AttemptOwner:        reverseID.String(),
		ExpectedOutcome:     mmodel.TransactionOutcomeCommitted,
		BalancesAfter:       []mmodel.BalanceRedis{balanceAfter.ToRedis()},
		Operations:          []mmodel.OperationRedis{economicOperation.ToRedis()},
	})
	require.NoError(t, err)

	claimRepo.EXPECT().Claim(gomock.Any(), organizationID, ledgerID, originID, reverseID,
		nil, nil, nil, nil, nil).
		Return(claim, false, nil)
	transactionRepo.EXPECT().FindWithOperations(gomock.Any(), organizationID, ledgerID, reverseID).
		Return(persisted, nil)
	metadataRepo.EXPECT().FindByEntity(gomock.Any(), constant.EntityTransaction, reverseID.String()).
		Return(nil, nil)
	redisRepo.EXPECT().ReadMessageFromQueue(gomock.Any(),
		utils.TransactionInternalKey(organizationID, ledgerID, reverseID.String())).Return(backup, nil).Times(2)
	redisRepo.EXPECT().CompleteOwnedKey(gomock.Any(), originRevertIdempotencyKey(claim), reverseID.String(),
		gomock.Any(), gomock.Any()).Return(true, nil)
	redisRepo.EXPECT().MGet(gomock.Any(), []string{legacyKey, legacyKey + ":owner"}).Return(map[string]string{
		legacyKey:            "",
		legacyKey + ":owner": reverseID.String(),
	}, nil)
	redisRepo.EXPECT().CompleteOwnedKey(gomock.Any(), legacyKey, reverseID.String(),
		gomock.Any(), gomock.Any()).Return(true, nil)
	claimRepo.EXPECT().Transition(gomock.Any(), organizationID, ledgerID, originID, reverseID,
		revertclaim.StateCompleted, nil).Return(nil)
	redisRepo.EXPECT().EnrichTransactionBackup(gomock.Any(), organizationID, ledgerID, reverseID,
		gomock.Any(), constant.ActionRevert, gomock.Any()).
		Return([]mmodel.OperationRedis{economicOperation.ToRedis()}, []mmodel.BalanceRedis{balanceAfter.ToRedis()}, false, nil)
	redisRepo.EXPECT().FinalizeTransactionPersistence(gomock.Any(), organizationID, ledgerID, reverseID,
		mmodel.BalanceExecutionAttempt{
			ExecutionKey: utils.TransactionBalanceExecutionKey(organizationID, ledgerID, reverseID),
			OutcomeKey:   utils.TransactionBalanceOutcomeKey(organizationID, ledgerID, reverseID),
			Owner:        reverseID.String(),
			Outcome:      mmodel.TransactionOutcomeCommitted,
			Identity:     reverseID,
		}, gomock.Any(), gomock.Any()).Return(nil)

	freeze := &revertUpdateFreezeStub{ready: true}
	handler := &TransactionHandler{
		RevertIdempotencyMode: revertIdempotencyModeFinal,
		RevertUpdateFreeze:    freeze,
		Query: &query.UseCase{
			TransactionRepo:         transactionRepo,
			TransactionMetadataRepo: metadataRepo,
		},
		Command: &command.UseCase{
			RevertClaimRepo:      claimRepo,
			TransactionRedisRepo: redisRepo,
		},
	}
	result, replayed, err := handler.adoptPersistedReverse(context.Background(), organizationID, ledgerID,
		originID, persisted, nil, nil, nil)
	require.NoError(t, err)
	assert.True(t, replayed)
	assert.Same(t, persisted, result)
	assert.Equal(t, []string{rolloutMode}, freeze.completedModes)
	assert.Equal(t, []string{rolloutToken}, freeze.completedTokens,
		"HTTP adoption after lost async completion must seal the exact persisted generation")
	// No expectation exists for a recalculated or foreign key: final adoption is permitted
	// to complete only the immutable H1 key stored in the durable bridge claim.
}

func TestAdoptPersistedReverse_FinalPreservesForeignLegacyCollision(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	transactionRepo := transaction.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)
	claimRepo := revertclaim.NewMockRepository(ctrl)
	redisRepo := redis.NewMockRedisRepository(ctrl)
	organizationID := uuid.New()
	ledgerID := uuid.New()
	originID := uuid.New()
	reverseID := uuid.New()
	legacyKey := "idempotency:{foreign-final-h1}:payload"
	legacyOwner := reverseID.String()
	originIDString := originID.String()
	operationID := uuid.NewString()
	economicOperation, balanceAfter := completeRevertEconomicOperation(organizationID, ledgerID, reverseID, operationID)
	persisted := &transaction.Transaction{
		ID:                  reverseID.String(),
		OrganizationID:      organizationID.String(),
		LedgerID:            ledgerID.String(),
		ParentTransactionID: &originIDString,
		Status:              transaction.Status{Code: constant.APPROVED},
		Operations:          []*operation.Operation{economicOperation},
	}
	claim := &revertclaim.Claim{
		OrganizationID:       organizationID,
		LedgerID:             ledgerID,
		OriginTransactionID:  originID,
		ReverseTransactionID: reverseID,
		LegacyFenceKey:       &legacyKey,
		LegacyFenceOwner:     &legacyOwner,
		State:                revertclaim.StateMutated,
	}
	backup, err := json.Marshal(mmodel.TransactionRedisQueue{
		TransactionID:       reverseID,
		ParentTransactionID: &originID,
		AttemptOwner:        reverseID.String(),
		ExpectedOutcome:     mmodel.TransactionOutcomeCommitted,
		BalancesAfter:       []mmodel.BalanceRedis{balanceAfter.ToRedis()},
		Operations:          []mmodel.OperationRedis{economicOperation.ToRedis()},
	})
	require.NoError(t, err)

	claimRepo.EXPECT().Claim(gomock.Any(), organizationID, ledgerID, originID, reverseID,
		nil, nil, nil, nil, nil).
		Return(claim, false, nil)
	transactionRepo.EXPECT().FindWithOperations(gomock.Any(), organizationID, ledgerID, reverseID).
		Return(persisted, nil)
	metadataRepo.EXPECT().FindByEntity(gomock.Any(), constant.EntityTransaction, reverseID.String()).
		Return(nil, nil)
	redisRepo.EXPECT().ReadMessageFromQueue(gomock.Any(),
		utils.TransactionInternalKey(organizationID, ledgerID, reverseID.String())).Return(backup, nil).Times(2)
	redisRepo.EXPECT().CompleteOwnedKey(gomock.Any(), originRevertIdempotencyKey(claim), reverseID.String(),
		gomock.Any(), gomock.Any()).Return(true, nil)
	redisRepo.EXPECT().MGet(gomock.Any(), []string{legacyKey, legacyKey + ":owner"}).Return(map[string]string{
		legacyKey:            `{"id":"foreign-reverse"}`,
		legacyKey + ":owner": reverseID.String(),
	}, nil)
	claimRepo.EXPECT().Transition(gomock.Any(), organizationID, ledgerID, originID, reverseID,
		revertclaim.StateCompleted, nil).Return(nil)
	redisRepo.EXPECT().EnrichTransactionBackup(gomock.Any(), organizationID, ledgerID, reverseID,
		gomock.Any(), constant.ActionRevert, gomock.Any()).
		Return([]mmodel.OperationRedis{economicOperation.ToRedis()}, []mmodel.BalanceRedis{balanceAfter.ToRedis()}, false, nil)
	redisRepo.EXPECT().FinalizeTransactionPersistence(gomock.Any(), organizationID, ledgerID, reverseID,
		gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)

	handler := &TransactionHandler{
		RevertIdempotencyMode: revertIdempotencyModeFinal,
		Query: &query.UseCase{
			TransactionRepo:         transactionRepo,
			TransactionMetadataRepo: metadataRepo,
		},
		Command: &command.UseCase{
			RevertClaimRepo:      claimRepo,
			TransactionRedisRepo: redisRepo,
		},
	}
	result, replayed, err := handler.adoptPersistedReverse(context.Background(), organizationID, ledgerID,
		originID, persisted, nil, nil, nil)
	require.NoError(t, err)
	assert.True(t, replayed)
	assert.Same(t, persisted, result)
}

func TestAdoptPersistedReverse_FinalCleansExactPhaseZeroBackupAfterPrimaryProof(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	transactionRepo := transaction.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)
	claimRepo := revertclaim.NewMockRepository(ctrl)
	redisRepo := redis.NewMockRedisRepository(ctrl)
	organizationID := uuid.New()
	ledgerID := uuid.New()
	originID := uuid.New()
	reverseID := uuid.New()
	originIDString := originID.String()
	operationID := uuid.NewString()
	persisted := &transaction.Transaction{
		ID:                  reverseID.String(),
		ParentTransactionID: &originIDString,
		Status:              transaction.Status{Code: constant.APPROVED},
		Operations:          []*operation.Operation{{ID: operationID}},
	}
	claim := &revertclaim.Claim{
		OrganizationID:       organizationID,
		LedgerID:             ledgerID,
		OriginTransactionID:  originID,
		ReverseTransactionID: reverseID,
		State:                revertclaim.StateMutated,
	}
	backup, err := json.Marshal(mmodel.TransactionRedisQueue{
		TransactionID:       reverseID,
		ParentTransactionID: &originID,
		TransactionStatus:   constant.CREATED,
		Validate:            &mtransaction.Responses{Pending: false},
		BalancesAfter:       []mmodel.BalanceRedis{{ID: uuid.NewString()}},
		Operations:          []mmodel.OperationRedis{{ID: operationID}},
	})
	require.NoError(t, err)

	claimRepo.EXPECT().Claim(gomock.Any(), organizationID, ledgerID, originID, reverseID,
		nil, nil, nil, nil, nil).
		Return(claim, false, nil)
	transactionRepo.EXPECT().FindWithOperations(gomock.Any(), organizationID, ledgerID, reverseID).
		Return(persisted, nil)
	metadataRepo.EXPECT().FindByEntity(gomock.Any(), constant.EntityTransaction, reverseID.String()).
		Return(nil, nil)
	redisRepo.EXPECT().ReadMessageFromQueue(gomock.Any(),
		utils.TransactionInternalKey(organizationID, ledgerID, reverseID.String())).Return(backup, nil).Times(2)
	redisRepo.EXPECT().CompleteOwnedKey(gomock.Any(), originRevertIdempotencyKey(claim), reverseID.String(),
		gomock.Any(), gomock.Any()).Return(true, nil)
	claimRepo.EXPECT().Transition(gomock.Any(), organizationID, ledgerID, originID, reverseID,
		revertclaim.StateCompleted, nil).Return(nil)
	redisRepo.EXPECT().FinalizeLegacyTransactionPersistence(gomock.Any(), organizationID, ledgerID,
		reverseID, originID, constant.CREATED, []string{operationID}).Return(nil)

	handler := &TransactionHandler{
		RevertIdempotencyMode: revertIdempotencyModeFinal,
		Query: &query.UseCase{
			TransactionRepo:         transactionRepo,
			TransactionMetadataRepo: metadataRepo,
		},
		Command: &command.UseCase{
			RevertClaimRepo:      claimRepo,
			TransactionRedisRepo: redisRepo,
		},
	}
	result, replayed, err := handler.adoptPersistedReverse(context.Background(), organizationID, ledgerID,
		originID, persisted, nil, nil, nil)
	require.NoError(t, err)
	assert.True(t, replayed)
	assert.Same(t, persisted, result)
}

func TestLoadCompleteReverse_PartialOperationSetIsNotReplayable(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	transactionRepo := transaction.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)
	redisRepo := redis.NewMockRedisRepository(ctrl)
	claim := &revertclaim.Claim{
		OrganizationID:       uuid.New(),
		LedgerID:             uuid.New(),
		OriginTransactionID:  uuid.New(),
		ReverseTransactionID: uuid.New(),
		State:                revertclaim.StateMutated,
	}
	originID := claim.OriginTransactionID.String()
	operationA := uuid.NewString()
	operationB := uuid.NewString()
	operationUnexpected := uuid.NewString()
	persisted := &transaction.Transaction{
		ID:                  claim.ReverseTransactionID.String(),
		ParentTransactionID: &originID,
		Operations: []*operation.Operation{
			{ID: operationA},
			{ID: operationUnexpected},
		},
	}
	backup, err := json.Marshal(mmodel.TransactionRedisQueue{
		TransactionID:       claim.ReverseTransactionID,
		ParentTransactionID: &claim.OriginTransactionID,
		Operations: []mmodel.OperationRedis{
			{ID: operationA},
			{ID: operationB},
		},
	})
	require.NoError(t, err)

	transactionRepo.EXPECT().FindWithOperations(gomock.Any(), claim.OrganizationID, claim.LedgerID,
		claim.ReverseTransactionID).Return(persisted, nil)
	metadataRepo.EXPECT().FindByEntity(gomock.Any(), constant.EntityTransaction,
		claim.ReverseTransactionID.String()).Return(nil, nil)
	redisRepo.EXPECT().ReadMessageFromQueue(gomock.Any(),
		utils.TransactionInternalKey(claim.OrganizationID, claim.LedgerID, claim.ReverseTransactionID.String())).Return(backup, nil)

	handler := &TransactionHandler{
		Query: &query.UseCase{
			TransactionRepo:         transactionRepo,
			TransactionMetadataRepo: metadataRepo,
		},
		Command: &command.UseCase{TransactionRedisRepo: redisRepo},
	}
	result, complete, err := handler.loadCompleteReverse(context.Background(), claim)
	require.NoError(t, err)
	assert.Same(t, persisted, result)
	assert.False(t, complete, "equal operation counts with a missing expected ID must still require reconciliation")
}

func TestLoadCompleteReverse_SameOperationIDsWithDifferentBalanceSnapshotsRequireReconciliation(t *testing.T) {
	t.Parallel()

	for _, outcomeBacked := range []bool{false, true} {
		t.Run(map[bool]string{false: "legacy phase zero", true: "outcome backed"}[outcomeBacked], func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			transactionRepo := transaction.NewMockRepository(ctrl)
			metadataRepo := mongodb.NewMockRepository(ctrl)
			redisRepo := redis.NewMockRedisRepository(ctrl)
			claim := &revertclaim.Claim{
				OrganizationID:       uuid.New(),
				LedgerID:             uuid.New(),
				OriginTransactionID:  uuid.New(),
				ReverseTransactionID: uuid.New(),
				State:                revertclaim.StateCompleted,
			}
			originID := claim.OriginTransactionID.String()
			availableBefore := decimal.NewFromInt(100)
			availableAfter := decimal.NewFromInt(90)
			onHold := decimal.Zero
			versionBefore := int64(1)
			versionAfter := int64(2)
			persistedOperation := &operation.Operation{
				ID:        uuid.NewString(),
				BalanceID: uuid.NewString(),
				Balance: operation.Balance{
					Available: &availableBefore,
					OnHold:    &onHold,
					Version:   &versionBefore,
				},
				BalanceAfter: operation.Balance{
					Available: &availableAfter,
					OnHold:    &onHold,
					Version:   &versionAfter,
				},
				Snapshot: mmodel.OperationSnapshot{
					OverdraftUsedBefore: "0",
					OverdraftUsedAfter:  "0",
				},
			}
			persisted := &transaction.Transaction{
				ID:                  claim.ReverseTransactionID.String(),
				ParentTransactionID: &originID,
				Operations:          []*operation.Operation{persistedOperation},
			}
			queuedOperation := persistedOperation.ToRedis()
			queuedOperation.BalanceAfterAvailable = decimal.NewFromInt(89)
			queued := mmodel.TransactionRedisQueue{
				TransactionID:       claim.ReverseTransactionID,
				ParentTransactionID: &claim.OriginTransactionID,
				Operations:          []mmodel.OperationRedis{queuedOperation},
			}
			if outcomeBacked {
				queued.AttemptOwner = claim.ReverseTransactionID.String()
				queued.ExpectedOutcome = mmodel.TransactionOutcomeCommitted
				queued.BalancesAfter = []mmodel.BalanceRedis{{ID: persistedOperation.BalanceID}}
			}
			backup, err := json.Marshal(queued)
			require.NoError(t, err)

			transactionRepo.EXPECT().FindWithOperations(gomock.Any(), claim.OrganizationID, claim.LedgerID,
				claim.ReverseTransactionID).Return(persisted, nil)
			metadataRepo.EXPECT().FindByEntity(gomock.Any(), constant.EntityTransaction,
				claim.ReverseTransactionID.String()).Return(nil, nil)
			redisRepo.EXPECT().ReadMessageFromQueue(gomock.Any(),
				utils.TransactionInternalKey(claim.OrganizationID, claim.LedgerID,
					claim.ReverseTransactionID.String())).Return(backup, nil)

			handler := &TransactionHandler{
				Query: &query.UseCase{
					TransactionRepo:         transactionRepo,
					TransactionMetadataRepo: metadataRepo,
				},
				Command: &command.UseCase{TransactionRedisRepo: redisRepo},
			}
			result, complete, err := handler.loadCompleteReverse(context.Background(), claim)
			require.NoError(t, err)
			assert.Same(t, persisted, result)
			assert.False(t, complete, "a completed claim cannot authorize cleanup of divergent economic evidence")
		})
	}
}

func TestLoadCompleteReverse_NonTerminalClaimWithoutBackupRequiresReconciliation(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	transactionRepo := transaction.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)
	redisRepo := redis.NewMockRedisRepository(ctrl)
	claim := &revertclaim.Claim{
		OrganizationID:       uuid.New(),
		LedgerID:             uuid.New(),
		OriginTransactionID:  uuid.New(),
		ReverseTransactionID: uuid.New(),
		State:                revertclaim.StateMutated,
	}
	originID := claim.OriginTransactionID.String()
	persisted := &transaction.Transaction{
		ID:                  claim.ReverseTransactionID.String(),
		ParentTransactionID: &originID,
		Operations:          []*operation.Operation{{ID: uuid.NewString()}},
	}
	transactionRepo.EXPECT().FindWithOperations(gomock.Any(), claim.OrganizationID, claim.LedgerID,
		claim.ReverseTransactionID).Return(persisted, nil)
	metadataRepo.EXPECT().FindByEntity(gomock.Any(), constant.EntityTransaction,
		claim.ReverseTransactionID.String()).Return(nil, nil)
	redisRepo.EXPECT().ReadMessageFromQueue(gomock.Any(),
		utils.TransactionInternalKey(claim.OrganizationID, claim.LedgerID,
			claim.ReverseTransactionID.String())).Return(nil, redislib.Nil)

	handler := &TransactionHandler{
		Query: &query.UseCase{
			TransactionRepo:         transactionRepo,
			TransactionMetadataRepo: metadataRepo,
		},
		Command: &command.UseCase{TransactionRedisRepo: redisRepo},
	}
	result, complete, err := handler.loadCompleteReverse(context.Background(), claim)
	require.NoError(t, err)
	assert.Same(t, persisted, result)
	assert.False(t, complete)
}

func TestLoadCompleteReverse_TerminalReceiptMustMatchPrimaryEconomicOperation(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		legacy     bool
		mutate     func(*mmodel.OperationRedis)
		wantReplay bool
	}{
		{name: "exact terminal receipt", wantReplay: true},
		{name: "exact pre-generation compatibility receipt", legacy: true, wantReplay: true},
		{name: "divergent terminal receipt", mutate: func(op *mmodel.OperationRedis) {
			op.BalanceAfterVersion++
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			transactionRepo := transaction.NewMockRepository(ctrl)
			metadataRepo := mongodb.NewMockRepository(ctrl)
			redisRepo := redis.NewMockRedisRepository(ctrl)
			claim := &revertclaim.Claim{
				OrganizationID:       uuid.New(),
				LedgerID:             uuid.New(),
				OriginTransactionID:  uuid.New(),
				ReverseTransactionID: uuid.New(),
				State:                revertclaim.StateCompleted,
			}
			if !tc.legacy {
				claim.RedisGeneration = &testRedisGeneration
			}
			originID := claim.OriginTransactionID.String()
			persistedOperation, balanceAfter := completeRevertEconomicOperation(
				claim.OrganizationID, claim.LedgerID, claim.ReverseTransactionID,
				claim.ReverseTransactionID.String()+"-operation")
			persisted := &transaction.Transaction{
				ID: claim.ReverseTransactionID.String(), ParentTransactionID: &originID,
				Status: transaction.Status{Code: constant.APPROVED}, Operations: []*operation.Operation{persistedOperation},
			}
			transactionRepo.EXPECT().FindWithOperations(gomock.Any(), claim.OrganizationID, claim.LedgerID,
				claim.ReverseTransactionID).Return(persisted, nil)
			metadataRepo.EXPECT().FindByEntity(gomock.Any(), constant.EntityTransaction,
				claim.ReverseTransactionID.String()).Return(nil, nil)
			redisRepo.EXPECT().ReadMessageFromQueue(gomock.Any(), utils.TransactionInternalKey(
				claim.OrganizationID, claim.LedgerID, claim.ReverseTransactionID.String())).Return(nil, redislib.Nil)
			canonical := persistedOperation.ToRedis()
			if tc.mutate != nil {
				tc.mutate(&canonical)
			}
			redisRepo.EXPECT().EnrichTransactionBackup(gomock.Any(), claim.OrganizationID, claim.LedgerID,
				claim.ReverseTransactionID, gomock.Any(), constant.ActionRevert, gomock.Any()).Return(
				[]mmodel.OperationRedis{canonical}, []mmodel.BalanceRedis{balanceAfter.ToRedis()}, true, nil)
			receipt := mmodel.TransactionPersistenceTombstone{
				Identity: claim.ReverseTransactionID, ParentTransactionID: claim.OriginTransactionID.String(),
				TransactionStatus: constant.CREATED, Action: constant.ActionRevert,
			}
			if !tc.legacy {
				receipt.Owner = claim.ReverseTransactionID.String()
				receipt.Outcome = mmodel.TransactionOutcomeCommitted
				receipt.RedisGeneration = testRedisGeneration
			}
			receiptRaw, marshalErr := json.Marshal(receipt)
			require.NoError(t, marshalErr)
			redisRepo.EXPECT().Get(gomock.Any(), utils.TransactionPersistenceTombstoneKey(
				claim.OrganizationID, claim.LedgerID, claim.ReverseTransactionID)).Return(string(receiptRaw), nil)

			handler := &TransactionHandler{
				Query: &query.UseCase{
					TransactionRepo:         transactionRepo,
					TransactionMetadataRepo: metadataRepo,
				},
				Command: &command.UseCase{TransactionRedisRepo: redisRepo},
			}
			result, complete, err := handler.loadCompleteReverse(context.Background(), claim)
			require.NoError(t, err)
			assert.Same(t, persisted, result)
			assert.Equal(t, tc.wantReplay, complete)
		})
	}
}

func TestLegacyRevertBarrierKeyFromBackup_UsesImmutableOriginSnapshot(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	redisRepo := redis.NewMockRedisRepository(ctrl)
	organizationID := uuid.New()
	ledgerID := uuid.New()
	originID := uuid.New()
	reverseID := uuid.New()
	debit := mtransaction.Amount{Asset: "USD", Value: decimal.NewFromInt(10), Operation: constant.DEBIT}
	credit := mtransaction.Amount{Asset: "USD", Value: decimal.NewFromInt(10), Operation: constant.CREDIT}
	immutableInput := mtransaction.Transaction{
		Description: "description at old reverse admission",
		Send: mtransaction.Send{
			Asset:      "USD",
			Value:      decimal.NewFromInt(10),
			Source:     mtransaction.Source{From: []mtransaction.FromTo{{AccountAlias: "@source", Amount: &debit}}},
			Distribute: mtransaction.Distribute{To: []mtransaction.FromTo{{AccountAlias: "@destination", Amount: &credit}}},
		},
	}
	backup, err := json.Marshal(mmodel.TransactionRedisQueue{
		TransactionID:       reverseID,
		ParentTransactionID: &originID,
		OrganizationID:      organizationID,
		LedgerID:            ledgerID,
		TransactionInput:    immutableInput,
	})
	require.NoError(t, err)
	redisRepo.EXPECT().ReadMessageFromQueue(gomock.Any(),
		utils.TransactionInternalKey(organizationID, ledgerID, reverseID.String())).Return(backup, nil)

	wantHash, err := legacyRevertIdempotencyHash(immutableInput)
	require.NoError(t, err)
	handler := &TransactionHandler{Command: &command.UseCase{TransactionRedisRepo: redisRepo}}
	got, err := handler.legacyRevertBarrierKeyFromBackup(context.Background(), organizationID, ledgerID, originID, reverseID)
	require.NoError(t, err)
	assert.Equal(t, utils.IdempotencyInternalKey(organizationID, ledgerID, wantHash), got)
}

func TestLegacyRevertBarrierKeyFromBackup_MissingImmutableInputFailsClosed(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	redisRepo := redis.NewMockRedisRepository(ctrl)
	organizationID := uuid.New()
	ledgerID := uuid.New()
	originID := uuid.New()
	reverseID := uuid.New()
	backup, err := json.Marshal(mmodel.TransactionRedisQueue{
		TransactionID:       reverseID,
		ParentTransactionID: &originID,
		OrganizationID:      organizationID,
		LedgerID:            ledgerID,
	})
	require.NoError(t, err)
	redisRepo.EXPECT().ReadMessageFromQueue(gomock.Any(),
		utils.TransactionInternalKey(organizationID, ledgerID, reverseID.String())).Return(backup, nil)

	handler := &TransactionHandler{Command: &command.UseCase{TransactionRedisRepo: redisRepo}}
	_, err = handler.legacyRevertBarrierKeyFromBackup(context.Background(), organizationID, ledgerID, originID, reverseID)
	var unavailable pkg.ServiceUnavailableError
	require.ErrorAs(t, err, &unavailable)
	assert.Equal(t, constant.ErrRevertReconciliationRequired.Error(), unavailable.Code)
}

func TestRevertIdempotencyModesAndKeys_DoNotAssumeOneRedisClusterSlot(t *testing.T) {
	t.Parallel()

	organizationID := uuid.MustParse("550e8400-e29b-41d4-a716-446655440000")
	ledgerID := uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8")
	originID := uuid.MustParse("019fae1d-8423-75b8-91c2-0b37952eb503")

	legacyKey := utils.IdempotencyInternalKey(organizationID, ledgerID, "payload-hash")
	originKey := utils.IdempotencyInternalKey(organizationID, ledgerID,
		libCommons.HashSHA256(utils.RevertIdempotencyHashSource(originID)))
	balanceOutcomeKey := utils.TransactionInternalKey(organizationID, ledgerID, uuid.NewString())

	assert.Contains(t, legacyKey, "{550e8400-e29b-41d4-a716-446655440000:6ba7b810-9dad-11d1-80b4-00c04fd430c8:payload-hash}")
	assert.NotEqual(t, legacyKey, originKey)
	assert.NotEqual(t, redisClusterSlot(legacyKey), redisClusterSlot(originKey),
		"combining bridge barriers in one Lua script would fail with CROSSSLOT")
	assert.Equal(t, redisClusterSlot(legacyKey), redisClusterSlot(legacyKey+":owner"),
		"owner-checked cleanup is safe because companion metadata shares the legacy fence slot")
	assert.Equal(t, redisClusterSlot(originKey), redisClusterSlot(originKey+":owner"),
		"origin cleanup uses the same owner-checked same-slot contract")
	assert.Contains(t, balanceOutcomeKey, "{transactions}")
	assert.NotContains(t, originKey, "{transactions}")
	assert.Equal(t, revertIdempotencyModeLegacy, (&TransactionHandler{}).activeRevertIdempotencyMode())
	assert.Equal(t, revertIdempotencyModeBridge, (&TransactionHandler{RevertIdempotencyMode: "BRIDGE"}).activeRevertIdempotencyMode())
	assert.Equal(t, revertIdempotencyModeFinal, (&TransactionHandler{RevertIdempotencyMode: "FINAL"}).activeRevertIdempotencyMode())
}

func redisClusterSlot(key string) uint16 {
	hashInput := key
	if open := strings.IndexByte(key, '{'); open >= 0 {
		if closeOffset := strings.IndexByte(key[open+1:], '}'); closeOffset > 0 {
			hashInput = key[open+1 : open+1+closeOffset]
		}
	}

	var crc uint16
	for i := range len(hashInput) {
		crc ^= uint16(hashInput[i]) << 8
		for range 8 {
			if crc&0x8000 != 0 {
				crc = crc<<1 ^ 0x1021
			} else {
				crc <<= 1
			}
		}
	}

	return crc % 16384
}
