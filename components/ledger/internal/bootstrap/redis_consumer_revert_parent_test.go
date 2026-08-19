// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/revertclaim"
	transactionredis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

type backupRevertRolloutPhaseStub struct {
	phase string
}

func (s backupRevertRolloutPhaseStub) ApprovedUpdatePolicy(context.Context, string) (bool, bool, error) {
	return s.phase == transactionredis.RevertUpdateFreezeActive || s.phase == transactionredis.RevertUpdateFreezeDrained, true, nil
}

func (s backupRevertRolloutPhaseStub) ReadyForMode(context.Context, string) (bool, error) {
	return true, nil
}

func (s backupRevertRolloutPhaseStub) AcquireApprovedUpdate(context.Context, string, string) (bool, bool, bool, error) {
	return true, false, true, nil
}

func (s backupRevertRolloutPhaseStub) ReleaseApprovedUpdate(context.Context, string) error {
	return nil
}

func (s backupRevertRolloutPhaseStub) AcquireRevert(context.Context, string, string, string) (bool, bool, string, error) {
	return true, true, s.phase, nil
}

func (s backupRevertRolloutPhaseStub) ReleaseRevert(context.Context, string, string, string) error {
	return nil
}

func (s backupRevertRolloutPhaseStub) CompleteRevert(context.Context, string, string) error {
	return nil
}

func (s backupRevertRolloutPhaseStub) RevertTerminalHandoffComplete(context.Context, string, string) (bool, error) {
	return true, nil
}

func (s backupRevertRolloutPhaseStub) FinancialDurability(context.Context) error {
	return nil
}

func (s backupRevertRolloutPhaseStub) FinancialDatasetGeneration(context.Context) (string, error) {
	return "test-generation", nil
}

func (s backupRevertRolloutPhaseStub) Phase(context.Context) (string, error) {
	return s.phase, nil
}

func TestResolveBackupParentTransactionID_RequiresClaimOrExplicitPhaseZeroOutcome(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	originID := uuid.New()
	reverseID := uuid.New()

	tests := []struct {
		name         string
		message      mmodel.TransactionRedisQueue
		claim        *revertclaim.Claim
		wantParent   *uuid.UUID
		wantPoison   string
		expectLookup bool
		rolloutPhase string
	}{
		{
			name:    "ordinary transaction has no parent",
			message: mmodel.TransactionRedisQueue{OrganizationID: organizationID, LedgerID: ledgerID, TransactionID: reverseID},
		},
		{
			name: "new revert envelope matches its claim",
			message: mmodel.TransactionRedisQueue{
				OrganizationID: organizationID, LedgerID: ledgerID, TransactionID: reverseID,
				ParentTransactionID: &originID, Action: constant.ActionRevert,
				BalancesAfter: []mmodel.BalanceRedis{{ID: uuid.NewString()}},
			},
			claim: &revertclaim.Claim{
				OrganizationID: organizationID, LedgerID: ledgerID,
				OriginTransactionID: originID, ReverseTransactionID: reverseID,
			},
			wantParent:   &originID,
			expectLookup: true,
		},
		{
			name: "bridge legacy envelope recovers origin from reserved reverse id",
			message: mmodel.TransactionRedisQueue{
				OrganizationID: organizationID, LedgerID: ledgerID, TransactionID: reverseID,
				Action: constant.ActionRevert, BalancesAfter: []mmodel.BalanceRedis{{ID: uuid.NewString()}},
			},
			claim: &revertclaim.Claim{
				OrganizationID: organizationID, LedgerID: ledgerID,
				OriginTransactionID: originID, ReverseTransactionID: reverseID,
			},
			wantParent:   &originID,
			expectLookup: true,
		},
		{
			name: "phase zero envelope adopts explicit origin with atomic outcome",
			message: mmodel.TransactionRedisQueue{
				OrganizationID: organizationID, LedgerID: ledgerID, TransactionID: reverseID,
				ParentTransactionID: &originID, Action: constant.ActionRevert,
				BalancesAfter: []mmodel.BalanceRedis{{ID: uuid.NewString()}},
			},
			wantParent:   &originID,
			expectLookup: true,
		},
		{
			name: "phase zero seed without atomic outcome waits for recovery",
			message: mmodel.TransactionRedisQueue{
				OrganizationID: organizationID, LedgerID: ledgerID, TransactionID: reverseID,
				ParentTransactionID: &originID, Action: constant.ActionRevert,
			},
			wantPoison:   "revert_balance_outcome_missing",
			expectLookup: true,
		},
		{
			name: "drained phase zero seed without atomic outcome is proven abandoned",
			message: mmodel.TransactionRedisQueue{
				OrganizationID: organizationID, LedgerID: ledgerID, TransactionID: reverseID,
				ParentTransactionID: &originID, Action: constant.ActionRevert,
			},
			wantPoison:   "phase_zero_pre_movement_seed_drained",
			expectLookup: true,
			rolloutPhase: transactionredis.RevertUpdateFreezeDrained,
		},
		{
			name: "old revert without a durable claim is quarantined",
			message: mmodel.TransactionRedisQueue{
				OrganizationID: organizationID, LedgerID: ledgerID, TransactionID: reverseID,
				Action: constant.ActionRevert,
			},
			wantPoison:   "revert_claim_missing",
			expectLookup: true,
		},
		{
			name: "claimed revert seed without atomic balance outcome is quarantined",
			message: mmodel.TransactionRedisQueue{
				OrganizationID: organizationID, LedgerID: ledgerID, TransactionID: reverseID,
				ParentTransactionID: &originID, Action: constant.ActionRevert,
			},
			claim: &revertclaim.Claim{
				OrganizationID: organizationID, LedgerID: ledgerID,
				OriginTransactionID: originID, ReverseTransactionID: reverseID,
			},
			wantPoison:   "revert_balance_outcome_missing",
			expectLookup: true,
		},
		{
			name: "envelope origin that disagrees with claim is quarantined",
			message: mmodel.TransactionRedisQueue{
				OrganizationID: organizationID, LedgerID: ledgerID, TransactionID: reverseID,
				ParentTransactionID: func() *uuid.UUID { id := uuid.New(); return &id }(),
				Action:              constant.ActionRevert,
				BalancesAfter:       []mmodel.BalanceRedis{{ID: uuid.NewString()}},
			},
			claim: &revertclaim.Claim{
				OrganizationID: organizationID, LedgerID: ledgerID,
				OriginTransactionID: originID, ReverseTransactionID: reverseID,
			},
			wantPoison:   "revert_origin_claim_mismatch",
			expectLookup: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			claimRepo := revertclaim.NewMockRepository(ctrl)
			if tc.expectLookup {
				claimRepo.EXPECT().GetByReverseID(gomock.Any(), organizationID, ledgerID, reverseID).Return(tc.claim, nil)
			}

			consumer := &RedisQueueConsumer{TransactionHandler: in.TransactionHandler{
				Command:            &command.UseCase{RevertClaimRepo: claimRepo},
				RevertUpdateFreeze: backupRevertRolloutPhaseStub{phase: tc.rolloutPhase},
			}}
			parent, poison, err := consumer.resolveBackupParentTransactionID(context.Background(), tc.message)
			require.NoError(t, err)
			assert.Equal(t, tc.wantPoison, poison)
			if tc.wantParent == nil {
				assert.Nil(t, parent)
			} else {
				require.NotNil(t, parent)
				assert.Equal(t, tc.wantParent.String(), *parent)
			}
		})
	}
}

func TestPersistedDrainedLegacyFenceKey_RequiresExactImmutableBackupWitness(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	originID := uuid.New()
	input := mtransaction.Transaction{Description: "drained old reverse", Send: mtransaction.Send{
		Asset: "USD", Value: decimal.NewFromInt(100),
	}}
	legacyHash, err := utils.LegacyTransactionIdempotencyHash(input)
	require.NoError(t, err)
	exactKey := utils.IdempotencyInternalKey(organizationID, ledgerID, legacyHash)
	queue := mmodel.TransactionRedisQueue{
		OrganizationID: organizationID, LedgerID: ledgerID, ParentTransactionID: &originID,
		TransactionInput: input, RevertLegacyFenceKey: exactKey,
	}

	got, err := persistedDrainedLegacyFenceKey(queue)
	require.NoError(t, err)
	assert.Equal(t, exactKey, got)

	missing := queue
	missing.RevertLegacyFenceKey = ""
	_, err = persistedDrainedLegacyFenceKey(missing)
	require.Error(t, err)

	foreign := queue
	foreign.RevertLegacyFenceKey = utils.IdempotencyInternalKey(organizationID, ledgerID, uuid.NewString())
	_, err = persistedDrainedLegacyFenceKey(foreign)
	require.Error(t, err)

	changedInput := queue
	changedInput.TransactionInput.Description = "candidate changed the H1 hash"
	_, err = persistedDrainedLegacyFenceKey(changedInput)
	require.Error(t, err, "a persisted key cannot be retargeted by changing the backup input")
}

func TestProcessMessage_DrainedPhaseZeroSeedIsRemovedWithoutPersistence(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	claimRepo := revertclaim.NewMockRepository(ctrl)
	redisRepo := transactionredis.NewMockRedisRepository(ctrl)
	organizationID := uuid.New()
	ledgerID := uuid.New()
	originID := uuid.New()
	reverseID := uuid.New()
	key := "transaction:" + reverseID.String()
	message := mmodel.TransactionRedisQueue{
		OrganizationID:      organizationID,
		LedgerID:            ledgerID,
		TransactionID:       reverseID,
		ParentTransactionID: &originID,
		Action:              constant.ActionRevert,
		Validate:            &mtransaction.Responses{},
		TransactionInput: mtransaction.Transaction{
			Description: "drained phase-zero reverse",
			Send: mtransaction.Send{
				Value: decimal.NewFromInt(100),
				Asset: "USD",
			},
		},
	}
	legacyHash, err := utils.LegacyTransactionIdempotencyHash(message.TransactionInput)
	require.NoError(t, err)
	legacyKey := utils.IdempotencyInternalKey(organizationID, ledgerID, legacyHash)
	message.RevertLegacyFenceKey = legacyKey
	raw, err := json.Marshal(message)
	require.NoError(t, err)

	claimRepo.EXPECT().GetByReverseID(gomock.Any(), organizationID, ledgerID, reverseID).Return(nil, nil)
	claimRepo.EXPECT().Get(gomock.Any(), organizationID, ledgerID, originID).Return(nil, nil)
	gomock.InOrder(
		redisRepo.EXPECT().ReleaseUnownedEmptyKey(gomock.Any(), legacyKey).Return(true, nil),
		redisRepo.EXPECT().RemoveMessageFromQueueIfStatus(gomock.Any(), key, "", "", "", true).Return(true, nil),
		redisRepo.EXPECT().ClearBackupAttempt(gomock.Any(), key).Return(nil),
	)

	consumer := &RedisQueueConsumer{
		Logger: newTestLogger(),
		TransactionHandler: in.TransactionHandler{
			Command: &command.UseCase{RevertClaimRepo: claimRepo, TransactionRedisRepo: redisRepo},
			RevertUpdateFreeze: backupRevertRolloutPhaseStub{
				phase: transactionredis.RevertUpdateFreezeDrained,
			},
		},
	}
	consumer.processMessage(context.Background(), key, string(raw), message)
}

func TestRequiresAtomicOutcomeBackup_LegacyLifecycleCanReachRecoveryFallback(t *testing.T) {
	t.Parallel()

	assert.False(t, requiresAtomicOutcomeBackup(mmodel.TransactionRedisQueue{
		Action: constant.ActionCommit,
	}), "an old commit backup has no Lua outcome envelope and must reach the legacy rebuild path")
	assert.False(t, requiresAtomicOutcomeBackup(mmodel.TransactionRedisQueue{
		Action: constant.ActionCancel,
	}), "an old cancel backup has no Lua outcome envelope and must reach the legacy rebuild path")
	assert.True(t, requiresAtomicOutcomeBackup(mmodel.TransactionRedisQueue{
		Action:          constant.ActionCommit,
		AttemptOwner:    uuid.NewString(),
		ExpectedOutcome: mmodel.TransactionOutcomeCommitted,
	}), "a modern lifecycle backup must never bypass its atomic Redis outcome")
	assert.True(t, requiresAtomicOutcomeBackup(mmodel.TransactionRedisQueue{
		Action:          constant.ActionRevert,
		AttemptOwner:    uuid.NewString(),
		ExpectedOutcome: mmodel.TransactionOutcomeCommitted,
	}), "an outcome-backed reverse must never bypass its atomic Redis outcome")
	assert.True(t, requiresAtomicOutcomeBackup(mmodel.TransactionRedisQueue{
		AttemptOwner:    uuid.NewString(),
		ExpectedOutcome: mmodel.TransactionOutcomeCommitted,
	}), "a corrupted or rolling payload cannot erase its action to bypass an existing economic outcome identity")
}
