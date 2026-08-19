// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package in

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

type revertUpdateFreezeStub struct {
	active           bool
	ready            bool
	err              error
	releaseErr       error
	policyRead       int
	readyRead        int
	approvedReleases int
	revertReleases   int
	revertTokens     []string
	revertAttempts   []string
	completeErr      error
	completedModes   []string
	completedTokens  []string
	terminalComplete bool
	terminalErr      error
	redisGeneration  string
	generationReads  int
	durabilityErr    error
	releasedLegacy   bool
	readySequence    []bool
}

func TestActiveRevertIdempotencyMode_ZeroValuePreservesReleasedAlgorithm(t *testing.T) {
	t.Parallel()

	handler := &TransactionHandler{}
	assert.Equal(t, revertIdempotencyModeLegacy, handler.activeRevertIdempotencyMode())
}

func (s *revertUpdateFreezeStub) ApprovedUpdatePolicy(context.Context, string) (bool, bool, error) {
	s.policyRead++

	return s.active, s.ready, s.err
}

func (s *revertUpdateFreezeStub) ReadyForMode(context.Context, string) (bool, error) {
	s.readyRead++
	if len(s.readySequence) > 0 {
		ready := s.readySequence[0]
		s.readySequence = s.readySequence[1:]

		return ready, s.err
	}

	return s.ready, s.err
}

func (s *revertUpdateFreezeStub) FinancialDurability(context.Context) error {
	return s.durabilityErr
}

func (s *revertUpdateFreezeStub) FinancialDatasetGeneration(context.Context) (string, error) {
	s.generationReads++
	if s.err != nil {
		return "", s.err
	}
	if s.redisGeneration == "" {
		return "test-generation", nil
	}

	return s.redisGeneration, nil
}

func (s *revertUpdateFreezeStub) AcquireApprovedUpdate(context.Context, string, string) (bool, bool, bool, error) {
	if s.err != nil {
		return false, false, false, s.err
	}
	if s.active {
		return false, true, false, nil
	}

	return s.ready, false, s.ready, nil
}

func (s *revertUpdateFreezeStub) ReleaseApprovedUpdate(context.Context, string) error {
	s.approvedReleases++

	return s.releaseErr
}

func (s *revertUpdateFreezeStub) AcquireRevert(_ context.Context, mode, token, attemptID string) (bool, bool, string, error) {
	s.revertTokens = append(s.revertTokens, token)
	s.revertAttempts = append(s.revertAttempts, attemptID)
	if mode == revertIdempotencyModeFinal && s.ready {
		return true, false, "finalized", s.err
	}
	if s.releasedLegacy {
		return true, true, "uninitialized", s.err
	}
	phase := ""
	if s.active {
		phase = "active"
	}

	return s.ready, s.ready, phase, s.err
}

func TestAcquireRevertRolloutRequest_ReleasedLegacyTracksInitializationDrainWithoutDatasetWitness(t *testing.T) {
	t.Parallel()

	freeze := &revertUpdateFreezeStub{ready: true, releasedLegacy: true}
	handler := &TransactionHandler{RevertIdempotencyMode: revertIdempotencyModeLegacy, RevertUpdateFreeze: freeze}
	phase, token, generation, release, err := handler.acquireRevertRolloutRequest(context.Background(),
		uuid.New(), uuid.New(), uuid.New())

	require.NoError(t, err)
	assert.Equal(t, "uninitialized", phase)
	assert.Empty(t, token)
	assert.Empty(t, generation)
	require.NotNil(t, release)
	assert.Zero(t, freeze.generationReads)
	require.NoError(t, release())
	assert.Equal(t, 1, freeze.revertReleases)
}

func TestRevertTransaction_TargetEmptyRequestAbortsWhenInitializationCommitsBeforeBalance(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	transactionRepo := transaction.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)
	organizationID := uuid.New()
	ledgerID := uuid.New()
	originID := uuid.New()
	amount := decimal.NewFromInt(10)
	makeOperation := func(operationType, direction, alias string) *operation.Operation {
		return &operation.Operation{
			ID: uuid.NewString(), TransactionID: originID.String(), Type: operationType, Direction: direction,
			AccountAlias: alias, BalanceKey: constant.DefaultBalanceKey, AssetCode: "USD",
			Amount: operation.Amount{Value: &amount},
		}
	}
	origin := &transaction.Transaction{
		ID: originID.String(), OrganizationID: organizationID.String(), LedgerID: ledgerID.String(),
		AssetCode: "USD", Amount: &amount, Status: transaction.Status{Code: constant.APPROVED},
		Body: mtransaction.Transaction{},
		Operations: []*operation.Operation{
			makeOperation(constant.CREDIT, constant.DirectionCredit, "@source"),
			makeOperation(constant.DEBIT, constant.DirectionDebit, "@destination"),
		},
	}
	transactionRepo.EXPECT().FindByParentID(gomock.Any(), organizationID, ledgerID, originID).
		DoAndReturn(func(ctx context.Context, _, _, _ uuid.UUID) (*transaction.Transaction, error) {
			require.True(t, readrouting.IsPrimaryRead(ctx))

			return nil, nil
		})
	transactionRepo.EXPECT().FindWithOperations(gomock.Any(), organizationID, ledgerID, originID).
		DoAndReturn(func(ctx context.Context, _, _, _ uuid.UUID) (*transaction.Transaction, error) {
			require.True(t, readrouting.IsPrimaryRead(ctx))

			return origin, nil
		})
	metadataRepo.EXPECT().FindByEntity(gomock.Any(), constant.EntityTransaction, originID.String()).Return(nil, nil)
	freeze := &revertUpdateFreezeStub{ready: true, releasedLegacy: true, readySequence: []bool{false}}
	handler := &TransactionHandler{
		Query:                 &query.UseCase{TransactionRepo: transactionRepo, TransactionMetadataRepo: metadataRepo},
		RevertIdempotencyMode: revertIdempotencyModeLegacy, RevertUpdateFreeze: freeze,
	}

	reverse, replayed, err := handler.revertTransaction(context.Background(), organizationID, ledgerID, originID)
	require.Error(t, err)
	assert.Nil(t, reverse)
	assert.False(t, replayed)
	assert.Equal(t, 1, freeze.readyRead, "the primary certificate must be rechecked immediately before the legacy money path")
	assert.Equal(t, 1, freeze.revertReleases, "the exact pre-initialization drain attempt must be released on abort")
}

func TestAcquireRevertRolloutRequest_ReusesOriginTokenAfterCrash(t *testing.T) {
	t.Parallel()

	freeze := &revertUpdateFreezeStub{ready: true}
	handler := &TransactionHandler{RevertIdempotencyMode: revertIdempotencyModeLegacy, RevertUpdateFreeze: freeze}
	organizationID := uuid.New()
	ledgerID := uuid.New()
	originID := uuid.New()

	_, firstToken, _, _, err := handler.acquireRevertRolloutRequest(context.Background(), organizationID, ledgerID, originID)
	require.NoError(t, err)
	_, retryToken, _, _, err := handler.acquireRevertRolloutRequest(context.Background(), organizationID, ledgerID, originID)
	require.NoError(t, err)
	_, otherToken, _, _, err := handler.acquireRevertRolloutRequest(context.Background(), organizationID, ledgerID, uuid.New())
	require.NoError(t, err)

	assert.Equal(t, firstToken, retryToken,
		"a same-origin retry must own and release the exact admission stranded by a crashed request")
	assert.NotEqual(t, firstToken, otherToken, "distinct economic origins must remain independent rollout blockers")
	assert.Equal(t, []string{firstToken, retryToken, otherToken}, freeze.revertTokens)
	assert.NotEqual(t, freeze.revertAttempts[0], freeze.revertAttempts[1],
		"distinct HTTP attempts for one origin must not share release ownership")
}

func (s *revertUpdateFreezeStub) ReleaseRevert(context.Context, string, string, string) error {
	s.revertReleases++

	return s.releaseErr
}

func (s *revertUpdateFreezeStub) CompleteRevert(_ context.Context, mode, token string) error {
	s.completedModes = append(s.completedModes, mode)
	s.completedTokens = append(s.completedTokens, token)

	return s.completeErr
}

func (s *revertUpdateFreezeStub) RevertTerminalHandoffComplete(context.Context, string, string) (bool, error) {
	return s.terminalComplete, s.terminalErr
}

func TestAcquireRolloutRequest_AmbiguousAdmissionPreservesSharedRevertOrigin(t *testing.T) {
	t.Parallel()

	acquireErr := errors.New("redis response lost")

	tests := []struct {
		name         string
		invoke       func(*TransactionHandler) error
		wantApproved int
		wantRevert   int
	}{
		{
			name: "approved update",
			invoke: func(handler *TransactionHandler) error {
				_, err := handler.acquireApprovedUpdateRolloutRequest(context.Background(), constant.APPROVED)

				return err
			},
			wantApproved: 1,
		},
		{
			name: "revert admission remains durable when its response is ambiguous",
			invoke: func(handler *TransactionHandler) error {
				_, _, _, _, err := handler.acquireRevertRolloutRequest(context.Background(), uuid.New(), uuid.New(), uuid.New())

				return err
			},
			wantRevert: 0,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			freeze := &revertUpdateFreezeStub{err: acquireErr}
			handler := &TransactionHandler{RevertIdempotencyMode: revertIdempotencyModeBridge, RevertUpdateFreeze: freeze}

			err := tc.invoke(handler)
			require.ErrorIs(t, err, acquireErr)
			assert.Equal(t, tc.wantApproved, freeze.approvedReleases)
			assert.Equal(t, tc.wantRevert, freeze.revertReleases)
		})
	}
}

func TestRequireRevertRolloutBarrier_RejectsMissingOrUnreadableMarker(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		mode     string
		freeze   *revertUpdateFreezeStub
		wantCode string
	}{
		{name: "phase zero permits old algorithm while marker is absent", mode: revertIdempotencyModeLegacy, freeze: &revertUpdateFreezeStub{ready: true}},
		{name: "finalized marker fences a surviving phase zero revert", mode: revertIdempotencyModeLegacy, freeze: &revertUpdateFreezeStub{}, wantCode: constant.ErrRevertRolloutFreezeRequired.Error()},
		{name: "bridge requires active marker", mode: revertIdempotencyModeBridge, freeze: &revertUpdateFreezeStub{}, wantCode: constant.ErrRevertRolloutFreezeRequired.Error()},
		{name: "final requires active or finalized marker", mode: revertIdempotencyModeFinal, freeze: &revertUpdateFreezeStub{}, wantCode: constant.ErrRevertRolloutFreezeRequired.Error()},
		{name: "finalized marker permits final requests", mode: revertIdempotencyModeFinal, freeze: &revertUpdateFreezeStub{ready: true}},
		{name: "marker read failure is technical", mode: revertIdempotencyModeBridge, freeze: &revertUpdateFreezeStub{err: errors.New("redis unavailable")}},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			handler := &TransactionHandler{RevertIdempotencyMode: tc.mode, RevertUpdateFreeze: tc.freeze}
			err := handler.requireRevertRolloutBarrier(context.Background())
			if tc.wantCode == "" && (tc.freeze == nil || tc.freeze.err == nil) {
				require.NoError(t, err)

				return
			}

			require.Error(t, err)
			if tc.wantCode != "" {
				var businessErr pkg.ServiceUnavailableError
				require.ErrorAs(t, err, &businessErr)
				assert.Equal(t, tc.wantCode, businessErr.Code)
			}
		})
	}
}
