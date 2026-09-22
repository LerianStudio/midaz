// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transactiongroup"
	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

type crossLedgerLifecycleReader struct {
	*atomicTransactionBatchSettingsReader
	members []*transaction.Transaction
}

func (reader *crossLedgerLifecycleReader) FindTransactionsByGroupID(context.Context, uuid.UUID) ([]*transaction.Transaction, error) {
	return reader.members, nil
}

func (reader *crossLedgerLifecycleReader) ResolveTransactionProjection(
	_ context.Context,
	organizationID, ledgerID, transactionID uuid.UUID,
) (*transaction.Transaction, uuid.UUID, bool, error) {
	for _, member := range reader.members {
		if member != nil && member.ID == transactionID.String() &&
			member.OrganizationID == organizationID.String() && member.LedgerID == ledgerID.String() {
			return member, uuid.Nil, false, nil
		}
	}

	return nil, uuid.Nil, false, nil
}

type applyingCrossLedgerLifecycleEngine struct {
	t          *testing.T
	executions []EngineExecution
	guards     []ExecutionGuard
}

func (engine *applyingCrossLedgerLifecycleEngine) EnsureTransactionGuard(
	_ context.Context,
	_, _ uuid.UUID,
	transactionID uuid.UUID,
	token string,
) error {
	engine.guards = append(engine.guards, ExecutionGuard{TransactionID: transactionID, NextToken: token})
	return nil
}

func (engine *applyingCrossLedgerLifecycleEngine) Execute(
	_ context.Context,
	execution EngineExecution,
) (*accounting.ExecutionResult, error) {
	engine.t.Helper()
	engine.executions = append(engine.executions, execution)

	states := make(map[string]accounting.BalanceState, len(execution.Execution.Balances))
	snapshots := make(map[string]accounting.BalanceSnapshot, len(execution.Execution.Balances))
	for _, snapshot := range execution.Execution.Balances {
		key := completionScopedBalanceRef(snapshot.OrganizationID, snapshot.LedgerID, snapshot.BalanceRef)
		states[key] = accounting.BalanceState{
			Available: snapshot.Available, OnHold: snapshot.OnHold,
			OverdraftUsed: snapshot.OverdraftUsed, Version: snapshot.Version,
		}
		snapshots[key] = snapshot
	}

	result := &accounting.ExecutionResult{}
	touched := make([]string, 0)
	seen := make(map[string]struct{})
	for transactionIndex, engineTransaction := range execution.Execution.Transactions {
		for postingIndex, posting := range engineTransaction.Postings {
			key := completionScopedBalanceRef(engineTransaction.OrganizationID, engineTransaction.LedgerID, posting.BalanceRef)
			before, exists := states[key]
			require.True(engine.t, exists, "missing snapshot for %s", key)
			after := before
			switch posting.Type {
			case accounting.PostingDebit:
				after.Available = after.Available.Sub(posting.Amount)
			case accounting.PostingCredit:
				after.Available = after.Available.Add(posting.Amount)
			case accounting.PostingUnreserve, accounting.PostingRelease:
				after.Available = after.Available.Add(posting.Amount)
				after.OnHold = after.OnHold.Sub(posting.Amount)
			default:
				require.FailNow(engine.t, "unexpected lifecycle posting", string(posting.Type))
			}
			after.Version++
			states[key] = after
			result.Movements = append(result.Movements, accounting.Movement{
				Ref:           fmt.Sprintf("movement:%d:%d", transactionIndex, postingIndex),
				TransactionID: engineTransaction.ID, PostingRef: posting.Ref,
				Role: accounting.RolePrimary, BalanceRef: posting.BalanceRef, Type: posting.Type,
				Amount: posting.Amount, Before: before, After: after,
			})
			if _, ok := seen[key]; !ok {
				seen[key] = struct{}{}
				touched = append(touched, key)
			}
		}
	}
	for _, key := range touched {
		snapshot := snapshots[key]
		state := states[key]
		snapshot.Available, snapshot.OnHold = state.Available, state.OnHold
		snapshot.OverdraftUsed, snapshot.Version = state.OverdraftUsed, state.Version
		result.Final = append(result.Final, snapshot)
	}

	return result, nil
}

func TestTransitionCrossLedgerGroupV2_RejectsTerminalGroupBeforeLocksOrEngine(t *testing.T) {
	groupID := uuid.MustParse("0199a500-0000-7000-8000-000000000001")
	target := pendingTransaction(false)
	groupText := groupID.String()
	target.GroupID = &groupText
	in := pendingTransitionInputFor(target)

	ctrl := gomock.NewController(t)
	repo := transactiongroup.NewMockRepository(ctrl)
	repo.EXPECT().FindByID(gomock.Any(), groupID).Return(&transactiongroup.TransactionGroup{
		ID:     groupID,
		Status: constant.APPROVED,
	}, nil)

	uc := &UseCase{TransactionGroupRepo: repo}
	result, err := uc.transitionCrossLedgerGroupV2(context.Background(), in, target, constant.APPROVED)
	require.Error(t, err)
	assert.Nil(t, result)

	var business pkg.UnprocessableOperationError
	require.True(t, errors.As(err, &business))
	assert.Equal(t, constant.ErrCrossLedgerGroupNotPending.Error(), business.Code)
	assert.Contains(t, business.Message, constant.APPROVED)
}

func TestTransitionCrossLedgerGroupV2_CommitAndCancelUseOneAtomicExecution(t *testing.T) {
	for _, test := range []struct {
		name             string
		status           string
		wantTransactions int
	}{
		{name: "commit", status: constant.APPROVED, wantTransactions: 2},
		{name: "cancel", status: constant.CANCELED, wantTransactions: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			uc, repo, engine, target, in, group := newCrossLedgerLifecycleFixture(t, test.status)

			repo.EXPECT().FindByID(gomock.Any(), group.ID).Return(group, nil)
			repo.EXPECT().UpdateStatus(gomock.Any(), group.ID, constant.PENDING, test.status).Return(true, nil)

			result, err := uc.transitionCrossLedgerGroupV2(context.Background(), in, target, test.status)
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, group.ID, result.BatchID)
			require.Len(t, result.Transactions, test.wantTransactions)
			for _, tran := range result.Transactions {
				assert.Equal(t, test.status, tran.Status.Code)
				assert.Equal(t, group.ID.String(), *tran.GroupID)
			}

			require.Len(t, engine.executions, 1)
			assert.Len(t, engine.executions[0].Execution.Transactions, test.wantTransactions)
			assert.Equal(t, ExecutionGuard{
				TransactionID: uuid.MustParse(target.ID),
				ExpectedToken: constant.PENDING,
				NextToken:     test.status,
			}, engine.executions[0].Guards[0])
			if test.status == constant.APPROVED {
				assert.Empty(t, engine.executions[0].Guards[1].ExpectedToken)
			}
		})
	}
}

func newCrossLedgerLifecycleFixture(
	t *testing.T,
	status string,
) (*UseCase, *transactiongroup.MockRepository, *applyingCrossLedgerLifecycleEngine, *transaction.Transaction, PendingTransitionInput, *transactiongroup.TransactionGroup) {
	t.Helper()

	organizationID := uuid.MustParse("0199a500-0000-7000-8000-000000000011")
	ledgerA := uuid.MustParse("0199a500-0000-7000-8000-000000000012")
	ledgerB := uuid.MustParse("0199a500-0000-7000-8000-000000000013")
	groupID := uuid.MustParse("0199a500-0000-7000-8000-000000000014")
	targetID := uuid.MustParse("0199a500-0000-7000-8000-000000000015")
	destinationID := uuid.MustParse("0199a500-0000-7000-8000-000000000016")
	executionID := uuid.MustParse("0199a500-0000-7000-8000-000000000017")
	now := time.Date(2026, time.September, 22, 18, 0, 0, 0, time.UTC)

	original := crossLedgerTestTransaction(
		"100",
		[]mtransaction.FromTo{crossLedgerAmountLeg("@debit", "100", true)},
		[]mtransaction.FromTo{crossLedgerAmountLeg("@credit", "100", false)},
	)
	parts, err := decomposeCrossLedgerTransaction(original, crossLedgerTransactionScopes{
		from: []atomicTransactionBatchLedgerRef{{organizationID: organizationID, ledgerID: ledgerA}},
		to:   []atomicTransactionBatchLedgerRef{{organizationID: organizationID, ledgerID: ledgerB}},
	})
	require.NoError(t, err)
	intent, err := buildCrossLedgerGroupIntent("BRL", parts)
	require.NoError(t, err)
	rawIntent, err := encodeCrossLedgerGroupIntent(intent)
	require.NoError(t, err)

	groupText := groupID.String()
	originBody := intent.Parts[0].Transaction
	originBody.Pending = true
	amount := decimal.NewFromInt(100)
	target := &transaction.Transaction{
		ID: targetID.String(), OrganizationID: organizationID.String(), LedgerID: ledgerA.String(),
		GroupID: &groupText, Status: transaction.Status{Code: constant.PENDING}, Body: originBody,
		AssetCode: "BRL", Amount: &amount, CreatedAt: now,
		Operations: []*operation.Operation{{
			ID: uuid.NewString(), AccountAlias: "@debit", BalanceKey: constant.DefaultBalanceKey,
			Type: constant.ONHOLD, Amount: operation.Amount{Value: &amount},
		}},
	}

	settings := mmodel.LedgerSettings{CrossLedger: mmodel.CrossLedgerSettings{Enabled: true}}
	balances := []*mmodel.Balance{
		atomicTransactionBatchTestBalance(organizationID, ledgerA, "0199a500-0000-7000-8000-000000000021", "@debit", "BRL"),
		atomicTransactionBatchTestBalance(organizationID, ledgerA, "0199a500-0000-7000-8000-000000000022", "@external/BRL", "BRL"),
		atomicTransactionBatchTestBalance(organizationID, ledgerB, "0199a500-0000-7000-8000-000000000023", "@external/BRL", "BRL"),
		atomicTransactionBatchTestBalance(organizationID, ledgerB, "0199a500-0000-7000-8000-000000000024", "@credit", "BRL"),
	}
	balances[0].Available = decimal.Zero
	balances[0].OnHold = amount
	reader := &crossLedgerLifecycleReader{
		atomicTransactionBatchSettingsReader: &atomicTransactionBatchSettingsReader{
			settingsByRef: map[atomicTransactionBatchLedgerRef]mmodel.LedgerSettings{
				{organizationID: organizationID, ledgerID: ledgerA}: settings,
				{organizationID: organizationID, ledgerID: ledgerB}: settings,
			},
			balances: balances,
		},
		members: []*transaction.Transaction{target},
	}

	ctrl := gomock.NewController(t)
	repo := transactiongroup.NewMockRepository(ctrl)
	redisRepo := txRedis.NewMockRedisRepository(ctrl)
	redisRepo.EXPECT().SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil)
	engine := &applyingCrossLedgerLifecycleEngine{t: t}
	ids := []uuid.UUID{executionID}
	if status == constant.APPROVED {
		ids = []uuid.UUID{destinationID, executionID}
	}
	uc := &UseCase{
		TransactionGroupRepo: repo,
		TransactionReader:    reader,
		TransactionRedisRepo: redisRepo,
		Engine:               engine,
		AppliedTransactionCompleter: &createAppliedTransactionCompleter{
			outcome: TransactionPersistenceOutcome{TransactionStatus: status},
		},
		EngineRecoveryAcknowledger: &recordingEngineRecoveryAcknowledger{},
		UUIDv7Generator:            orderedAtomicTransactionBatchUUIDs(t, ids...),
		Clock:                      func() time.Time { return now },
	}
	group := &transactiongroup.TransactionGroup{
		ID: groupID, OrganizationID: organizationID, LedgerID: ledgerA,
		Status: constant.PENDING, AssetCode: "BRL", Intent: rawIntent,
	}

	return uc, repo, engine, target, pendingTransitionInputFor(target), group
}
