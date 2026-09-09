// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v6/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/engine"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

var fixedPendingCreatedAt = time.Date(2026, time.September, 7, 10, 15, 0, 0, time.UTC)

type transitionEngineReader struct {
	TransactionReader
	writeBehind        *transaction.Transaction
	persisted          *transaction.Transaction
	settings           mmodel.LedgerSettings
	balances           []*mmodel.Balance
	persistedReads     int
	persistedOnPrimary bool
	balanceAliases     [][]string
}

func (reader *transitionEngineReader) GetWriteBehindTransaction(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*transaction.Transaction, error) {
	return reader.writeBehind, nil
}

func (reader *transitionEngineReader) GetTransactionWithOperationsByID(ctx context.Context, _, _, _ uuid.UUID) (*transaction.Transaction, error) {
	reader.persistedReads++
	reader.persistedOnPrimary = readrouting.IsPrimaryRead(ctx)
	return reader.persisted, nil
}

func (reader *transitionEngineReader) GetParsedLedgerSettings(context.Context, uuid.UUID, uuid.UUID) (mmodel.LedgerSettings, error) {
	return reader.settings, nil
}

func (reader *transitionEngineReader) GetBalances(_ context.Context, _, _ uuid.UUID, aliases []string) ([]*mmodel.Balance, error) {
	reader.balanceAliases = append(reader.balanceAliases, append([]string(nil), aliases...))
	out := make([]*mmodel.Balance, 0, len(aliases))
	for _, alias := range aliases {
		for _, balance := range reader.balances {
			if mtransaction.AliasKey(balance.Alias, balance.Key) == alias {
				out = append(out, balance)
			}
		}
	}
	return out, nil
}

func (reader *transitionEngineReader) GetBalanceEngineBalances(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Balance, []*mmodel.Balance, error) {
	pool, err := LoadBalanceEngineSnapshotPool(ctx, organizationID, ledgerID, aliases, reader.GetBalances)
	return pool.ExplicitBalances, pool.Balances, err
}

func (reader *transitionEngineReader) ValidateAccountingRules(context.Context, uuid.UUID, uuid.UUID, []mmodel.BalanceOperation, *mtransaction.Responses, string) (*mmodel.TransactionRouteCache, error) {
	return nil, nil
}

type transitionEngineExecutor struct {
	t          *testing.T
	requests   []EngineExecution
	guardCalls []ExecutionGuard
	before     func(EngineExecution) error
}

func (executor *transitionEngineExecutor) EnsureTransactionGuard(_ context.Context, _, _, transactionID uuid.UUID, token string) error {
	executor.guardCalls = append(executor.guardCalls, ExecutionGuard{TransactionID: transactionID, NextToken: token})
	return nil
}

func (executor *transitionEngineExecutor) Execute(_ context.Context, execution EngineExecution) (*engine.Result, error) {
	executor.requests = append(executor.requests, execution)
	if executor.before != nil {
		if err := executor.before(execution); err != nil {
			return nil, err
		}
	}

	require.Len(executor.t, execution.Request.Transactions, 1)
	transactionIntent := execution.Request.Transactions[0]
	source := literalTransitionSnapshot(executor.t, execution.Request.Balances, "@source#default")

	require.NotEmpty(executor.t, transactionIntent.Postings)
	switch transactionIntent.Postings[0].Type {
	case engine.PostingUnreserve:
		require.Len(executor.t, transactionIntent.Postings, 2)
		target := literalTransitionSnapshot(executor.t, execution.Request.Balances, "@target#default")
		unreserve, credit := transactionIntent.Postings[0], transactionIntent.Postings[1]
		require.Equal(executor.t, "from:0:unreserve", unreserve.Ref)
		require.Equal(executor.t, "@source#default", unreserve.BalanceRef)
		require.Equal(executor.t, decimal.NewFromInt(10), unreserve.Amount)
		require.Equal(executor.t, "to:0:credit", credit.Ref)
		require.Equal(executor.t, "@target#default", credit.BalanceRef)
		require.Equal(executor.t, decimal.NewFromInt(10), credit.Amount)
		sourceAfterVersion, targetAfterVersion := int64(2), int64(2)
		if len(executor.requests) == 2 {
			require.Equal(executor.t, int64(2), source.Version)
			sourceAfterVersion = 3
		} else {
			require.Equal(executor.t, int64(1), source.Version)
		}
		return &engine.Result{
			Movements: []engine.Movement{{
				Ref: "commit-source", TransactionID: transactionIntent.ID, PostingRef: unreserve.Ref,
				Role: engine.RolePrimary, BalanceRef: unreserve.BalanceRef, Type: engine.PostingUnreserve,
				Amount: decimal.NewFromInt(10), Before: engine.BalanceState{Available: decimal.NewFromInt(10), OnHold: decimal.NewFromInt(10), Version: source.Version},
				After: engine.BalanceState{Available: decimal.NewFromInt(10), Version: sourceAfterVersion},
			}, {
				Ref: "commit-target", TransactionID: transactionIntent.ID, PostingRef: credit.Ref,
				Role: engine.RolePrimary, BalanceRef: credit.BalanceRef, Type: engine.PostingCredit,
				Amount: decimal.NewFromInt(10), Before: engine.BalanceState{Available: decimal.Zero, OnHold: decimal.NewFromInt(10), Version: target.Version},
				After: engine.BalanceState{Available: decimal.NewFromInt(10), OnHold: decimal.NewFromInt(10), Version: targetAfterVersion},
			}},
			Final: []engine.BalanceSnapshot{literalFinalSnapshot(source, decimal.NewFromInt(10), decimal.Zero, sourceAfterVersion), literalFinalSnapshot(target, decimal.NewFromInt(10), decimal.NewFromInt(10), targetAfterVersion)},
		}, nil
	case engine.PostingRelease:
		require.Len(executor.t, transactionIntent.Postings, 1)
		posting := transactionIntent.Postings[0]
		require.Equal(executor.t, "from:0:release", posting.Ref)
		require.Equal(executor.t, "@source#default", posting.BalanceRef)
		require.Equal(executor.t, decimal.NewFromInt(10), posting.Amount)
		require.Equal(executor.t, int64(1), source.Version)
		return &engine.Result{
			Movements: []engine.Movement{{
				Ref: "cancel-source", TransactionID: transactionIntent.ID, PostingRef: posting.Ref,
				Role: engine.RolePrimary, BalanceRef: posting.BalanceRef, Type: engine.PostingRelease,
				Amount: decimal.NewFromInt(10), Before: engine.BalanceState{Available: decimal.NewFromInt(10), OnHold: decimal.NewFromInt(10), Version: source.Version},
				After: engine.BalanceState{Available: decimal.NewFromInt(20), Version: 2},
			}},
			Final: []engine.BalanceSnapshot{literalFinalSnapshot(source, decimal.NewFromInt(20), decimal.Zero, 2)},
		}, nil
	default:
		require.FailNow(executor.t, "unexpected pending transition posting", "%s", transactionIntent.Postings[0].Type)
		return nil, errors.New("unexpected pending transition posting")
	}
}

func literalTransitionSnapshot(t *testing.T, snapshots []engine.BalanceSnapshot, balanceRef string) engine.BalanceSnapshot {
	t.Helper()
	for _, snapshot := range snapshots {
		if snapshot.BalanceRef == balanceRef {
			return snapshot
		}
	}
	require.FailNow(t, "missing pending transition balance snapshot", "%s", balanceRef)
	return engine.BalanceSnapshot{}
}

func literalFinalSnapshot(snapshot engine.BalanceSnapshot, available, onHold decimal.Decimal, version int64) engine.BalanceSnapshot {
	snapshot.Available, snapshot.OnHold, snapshot.Version = available, onHold, version
	return snapshot
}

func TestPendingTransitionUsesOptInBalanceEngineAfterSQLConfirmation(t *testing.T) {
	t.Setenv("AUDIT_LOG_ENABLED", "false")
	for _, test := range []struct {
		name     string
		status   string
		version2 bool
		call     func(*UseCase, context.Context, PendingTransitionInput) (*transaction.Transaction, error)
	}{
		{"commit v1", constant.APPROVED, false, (*UseCase).CommitTransactionV1},
		{"cancel v1", constant.CANCELED, false, (*UseCase).CancelTransactionV1},
		{"commit v2", constant.APPROVED, true, (*UseCase).CommitTransactionV2},
		{"cancel v2", constant.CANCELED, true, (*UseCase).CancelTransactionV2},
	} {
		t.Run(test.name, func(t *testing.T) {
			uc, reader, executor, finalizer, in := newTransitionEngineUseCase(t, test.status)
			reader.settings.Tracer.Mode = mmodel.TracerModeEnforce
			reserver := &stubReserver{}
			uc.TracerReserver = reserver
			ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-transition")
			ctx = libObservability.ContextWithHeaderID(ctx, "request-transition")

			got, err := test.call(uc, ctx, in)
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, test.status, got.Status.Code)
			assert.Equal(t, fixedPendingCreatedAt, got.CreatedAt)
			assert.Equal(t, 1, reader.persistedReads)
			assert.True(t, reader.persistedOnPrimary)
			require.Len(t, executor.guardCalls, 1)
			assert.Equal(t, constant.PENDING, executor.guardCalls[0].NextToken)
			require.Len(t, executor.requests, 1)
			assert.Equal(t, ExecutionGuard{TransactionID: in.TransactionID, ExpectedToken: constant.PENDING, NextToken: test.status}, executor.requests[0].Guards[0])
			require.Len(t, finalizer.envelopes, 1)
			payload := mustCreateEnginePayload(t, finalizer.envelopes[0])
			assert.Equal(t, fixedPendingCreatedAt, payload.TransactionCreatedAt)
			assert.Equal(t, "tenant-transition", payload.TenantID)
			assert.Equal(t, "request-transition", payload.HeaderID)
			assert.Equal(t, reader.persisted.FeesSkipped, payload.FeesSkipped)
			assert.Equal(t, reader.persisted.TracerSkipped, payload.TracerSkipped)
			assert.Equal(t, "pending transition", payload.TransactionInput.Description)
			assert.Equal(t, "@source", reader.persisted.Body.Send.Source.From[0].AccountAlias)
			for _, row := range got.Operations {
				assert.Equal(t, uuid.Version(5), uuid.MustParse(row.ID).Version())
			}
			if !test.version2 {
				assert.Empty(t, reserver.confirmedTxns)
				assert.Empty(t, reserver.releasedTxns)
			} else if test.status == constant.APPROVED {
				assert.Equal(t, []uuid.UUID{in.TransactionID}, reserver.confirmedTxns)
				assert.Empty(t, reserver.releasedTxns)
			} else {
				assert.Equal(t, []uuid.UUID{in.TransactionID}, reserver.releasedTxns)
				assert.Empty(t, reserver.confirmedTxns)
			}
		})
	}
}

func TestPendingTransitionV2RetriesSnapshotsButRunsPhaseTwoOnce(t *testing.T) {
	t.Setenv("AUDIT_LOG_ENABLED", "false")
	uc, reader, executor, finalizer, in := newTransitionEngineUseCase(t, constant.APPROVED)
	reader.settings.Tracer.Mode = mmodel.TracerModeEnforce
	reserver := &stubReserver{}
	uc.TracerReserver = reserver
	staleCalls := 0
	executor.before = func(execution EngineExecution) error {
		if staleCalls != 0 {
			return nil
		}
		staleCalls++
		reader.balances[0].Version++
		return &engine.Failure{
			Code: engine.FailureStaleVersion, TransactionIndex: 0, PostingIndex: 0,
			BalanceRef: execution.Request.Transactions[0].Postings[0].BalanceRef,
		}
	}

	got, err := uc.CommitTransactionV2(tmcore.ContextWithTenantID(context.Background(), "tenant-retry"), in)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Len(t, executor.requests, 2)
	assert.Equal(t, int64(1), executor.requests[0].Request.Balances[0].Version)
	assert.Equal(t, int64(2), executor.requests[1].Request.Balances[0].Version)
	assert.Equal(t, executor.requests[0].Request.ExecutionID, executor.requests[1].Request.ExecutionID)
	assert.Equal(t, executor.requests[0].Request.Transactions, executor.requests[1].Request.Transactions)
	first := mustCreateEngineRecovery(t, executor.requests[0])
	second := mustCreateEngineRecovery(t, executor.requests[1])
	assert.Equal(t, first.TTL, second.TTL)
	assert.Equal(t, first.TransactionDate, second.TransactionDate)
	assert.Equal(t, first.TransactionUpdatedAt, second.TransactionUpdatedAt)
	assert.Equal(t, first.OperationUpdatedAt, second.OperationUpdatedAt)
	assert.Equal(t, fixedPendingCreatedAt, second.TransactionCreatedAt)
	assert.Equal(t, []uuid.UUID{in.TransactionID}, reserver.confirmedTxns)
	assert.Empty(t, reserver.releasedTxns)
	assert.Len(t, finalizer.envelopes, 1)
	assert.Equal(t, 1, staleCalls)
}

func TestPendingCancelUsesPersistedOverdraftCapAndOnlyLoadsSources(t *testing.T) {
	uc, reader, executor, _, in := newTransitionEngineUseCase(t, constant.CANCELED)
	historicalUsage := decimal.NewFromInt(4)
	reader.persisted.Operations = []*operation.Operation{{
		ID: uuid.New().String(), AccountAlias: "@source", BalanceKey: constant.OverdraftBalanceKey,
		Type: constant.OVERDRAFT, Direction: constant.DirectionDebit,
		Amount: operation.Amount{Value: &historicalUsage},
	}}
	reader.balances[0].OverdraftUsed = decimal.NewFromInt(9)
	companion := translationBalance(in.OrganizationID, in.LedgerID, "66666666-6666-4666-8666-666666666666", "@source", constant.OverdraftBalanceKey)
	companion.AccountID = reader.balances[0].AccountID
	companion.Direction = constant.DirectionDebit
	reader.balances = append(reader.balances, companion)
	executor.before = func(EngineExecution) error { return errors.New("stop after observing request") }

	_, err := uc.CancelTransactionV1(tmcore.ContextWithTenantID(context.Background(), "tenant-cancel-cap"), in)
	require.Error(t, err)
	require.Len(t, executor.requests, 1)
	require.Len(t, executor.requests[0].Request.Transactions, 1)
	require.Len(t, executor.requests[0].Request.Transactions[0].Postings, 1)
	assert.Equal(t, historicalUsage, executor.requests[0].Request.Transactions[0].Postings[0].OverdraftAmount)
	assert.Equal(t, decimal.NewFromInt(9), executor.requests[0].Request.Balances[0].OverdraftUsed)
	for _, aliases := range reader.balanceAliases {
		assert.NotContains(t, aliases, "@target#default")
	}
}

func TestPendingTransitionEngineFailureBoundaries(t *testing.T) {
	finalizationErr := errors.New("finalization unavailable")
	indeterminate := testBalanceEngineTechnicalError{code: "transport", indeterminate: true, cause: errors.New("outcome unknown")}

	for _, test := range []struct {
		name        string
		executorErr error
		finalizeErr error
		wantUnlock  bool
	}{
		{
			name: "confirmed refusal unlocks",
			executorErr: &engine.Failure{
				Code: engine.FailureOnHoldUnderflow, TransactionIndex: 0, PostingIndex: 0, BalanceRef: "@source#default",
			},
			wantUnlock: true,
		},
		{name: "indeterminate execution retains lock", executorErr: indeterminate},
		{name: "finalization failure retains lock", finalizeErr: finalizationErr},
	} {
		t.Run(test.name, func(t *testing.T) {
			uc, reader, executor, finalizer, in := newTransitionEngineUseCase(t, constant.APPROVED)
			reader.settings.Tracer.Mode = mmodel.TracerModeEnforce
			reserver := &stubReserver{}
			uc.TracerReserver = reserver
			if test.executorErr != nil {
				executor.before = func(EngineExecution) error { return test.executorErr }
			}
			finalizer.err = test.finalizeErr
			if test.wantUnlock {
				uc.TransactionRedisRepo.(*txRedis.MockRedisRepository).EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			}

			_, err := uc.CommitTransactionV2(tmcore.ContextWithTenantID(context.Background(), "tenant-failure"), in)
			require.Error(t, err)
			if test.finalizeErr != nil {
				assert.ErrorIs(t, err, finalizationErr)
				assert.Len(t, finalizer.envelopes, 1)
				assert.Equal(t, []uuid.UUID{in.TransactionID}, reserver.confirmedTxns)
			} else {
				assert.Empty(t, finalizer.envelopes)
				assert.Empty(t, reserver.confirmedTxns)
			}
		})
	}
}

func TestPendingTransitionRejectsTerminalSQLAndResolvesGuardConflict(t *testing.T) {
	t.Run("terminal SQL wins over write-behind pending", func(t *testing.T) {
		uc, reader, executor, _, in := newTransitionEngineUseCase(t, constant.APPROVED)
		reader.persisted.Status.Code = constant.APPROVED
		reader.persisted.Body = mtransaction.Transaction{}
		uc.TransactionRedisRepo.(*txRedis.MockRedisRepository).EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).Times(1)

		_, err := uc.CommitTransactionV1(context.Background(), in)
		assertBusinessCode(t, err, constant.ErrCommitTransactionNotPending.Error())
		assert.Empty(t, executor.guardCalls)
		assert.Empty(t, executor.requests)
	})

	t.Run("guard conflict with pending SQL is locked", func(t *testing.T) {
		uc, reader, executor, _, in := newTransitionEngineUseCase(t, constant.APPROVED)
		executor.before = func(EngineExecution) error {
			return testBalanceEngineTechnicalError{code: "execution_guard_conflict", cause: errors.New("guard changed")}
		}
		uc.TransactionRedisRepo.(*txRedis.MockRedisRepository).EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).Times(1)

		_, err := uc.CommitTransactionV1(context.Background(), in)
		assertBusinessCode(t, err, constant.ErrPendingTransactionLocked.Error())
		assert.Equal(t, 2, reader.persistedReads)
		assert.Len(t, executor.requests, 1)
	})

	t.Run("guard conflict with terminal SQL is not pending", func(t *testing.T) {
		uc, reader, executor, _, in := newTransitionEngineUseCase(t, constant.APPROVED)
		executor.before = func(EngineExecution) error {
			reader.persisted.Status.Code = constant.CANCELED
			return testBalanceEngineTechnicalError{code: "execution_guard_conflict", cause: errors.New("guard changed")}
		}
		uc.TransactionRedisRepo.(*txRedis.MockRedisRepository).EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).Times(1)

		_, err := uc.CommitTransactionV1(context.Background(), in)
		assertBusinessCode(t, err, constant.ErrCommitTransactionNotPending.Error())
		assert.Equal(t, 2, reader.persistedReads)
	})
}

func TestPendingTransitionPreservesLargeMetadataNumbers(t *testing.T) {
	t.Setenv("AUDIT_LOG_ENABLED", "false")
	uc, reader, _, finalizer, in := newTransitionEngineUseCase(t, constant.APPROVED)
	large := json.Number("9007199254740993")
	reader.persisted.Body.Metadata = map[string]any{"sequence": large}
	reader.persisted.Body.Send.Source.From[0].Metadata = map[string]any{"legSequence": large}

	_, err := uc.CommitTransactionV1(tmcore.ContextWithTenantID(context.Background(), "tenant-number"), in)
	require.NoError(t, err)
	require.Len(t, finalizer.envelopes, 1)
	payload := mustCreateEnginePayload(t, finalizer.envelopes[0])
	assert.Equal(t, large, payload.TransactionInput.Metadata["sequence"])
	assert.Equal(t, large, payload.TransactionInput.Send.Source.From[0].Metadata["legSequence"])
}

func TestPendingCancelFailsClosedWithoutPersistedOperations(t *testing.T) {
	uc, reader, executor, _, in := newTransitionEngineUseCase(t, constant.CANCELED)
	reader.persisted.Operations = nil
	uc.TransactionRedisRepo.(*txRedis.MockRedisRepository).EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).Times(1)

	_, err := uc.CancelTransactionV2(context.Background(), in)
	require.ErrorIs(t, err, ErrInvalidBalanceEngineRecovery)
	assert.Empty(t, executor.guardCalls)
	assert.Empty(t, executor.requests)
}

func TestPendingTransitionRejectsMissingSQLConfirmation(t *testing.T) {
	uc, reader, executor, _, in := newTransitionEngineUseCase(t, constant.APPROVED)
	reader.persisted = nil
	uc.TransactionRedisRepo.(*txRedis.MockRedisRepository).EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).Times(1)

	_, err := uc.CommitTransactionV1(context.Background(), in)
	var notFound pkg.EntityNotFoundError
	require.ErrorAs(t, err, &notFound)
	assert.Equal(t, constant.ErrTransactionIDNotFound.Error(), notFound.Code)
	assert.Empty(t, executor.guardCalls)
	assert.Empty(t, executor.requests)
}

func assertBusinessCode(t *testing.T, err error, code string) {
	t.Helper()
	require.Error(t, err)
	var conflict pkg.EntityConflictError
	require.ErrorAs(t, err, &conflict)
	assert.Equal(t, code, conflict.Code)
}

func newTransitionEngineUseCase(t *testing.T, terminalStatus string) (*UseCase, *transitionEngineReader, *transitionEngineExecutor, *createEngineFinalizer, PendingTransitionInput) {
	t.Helper()
	organizationID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	ledgerID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	transactionID := uuid.MustParse("33333333-3333-4333-8333-333333333333")
	amount := decimal.NewFromInt(10)
	body := mtransaction.Transaction{
		Description: "pending transition",
		Send: mtransaction.Send{
			Asset: "USD", Value: amount,
			Source:     mtransaction.Source{From: []mtransaction.FromTo{{AccountAlias: "@source", Amount: &mtransaction.Amount{Asset: "USD", Value: amount}}}},
			Distribute: mtransaction.Distribute{To: []mtransaction.FromTo{{AccountAlias: "@target", Amount: &mtransaction.Amount{Asset: "USD", Value: amount}}}},
		},
	}
	persisted := &transaction.Transaction{
		ID: transactionID.String(), OrganizationID: organizationID.String(), LedgerID: ledgerID.String(),
		Status: transaction.Status{Code: constant.PENDING}, Body: body, CreatedAt: fixedPendingCreatedAt,
		FeesSkipped: true, TracerSkipped: true,
		Operations: []*operation.Operation{{ID: uuid.New().String(), AccountAlias: "@source", BalanceKey: constant.DefaultBalanceKey, Type: constant.ONHOLD}},
	}
	writeBehind := *persisted
	writeBehind.Body.Description = "unconfirmed write-behind body"
	reader := &transitionEngineReader{
		writeBehind: &writeBehind, persisted: persisted,
		balances: []*mmodel.Balance{
			transitionBalance(organizationID, ledgerID, "44444444-4444-4444-8444-444444444444", "@source", amount),
			transitionBalance(organizationID, ledgerID, "55555555-5555-4555-8555-555555555555", "@target", decimal.Zero),
		},
	}
	executor := &transitionEngineExecutor{t: t}
	finalizer := &createEngineFinalizer{outcome: BalanceEngineRecoveryOutcome{TransactionStatus: terminalStatus}}
	ctrl := gomock.NewController(t)
	redisRepo := txRedis.NewMockRedisRepository(ctrl)
	redisRepo.EXPECT().SetNX(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(true, nil).Times(1)
	uc := &UseCase{
		TransactionRedisRepo: redisRepo, TransactionReader: reader,
		BalanceEngine: executor, BalanceEngineFinalizer: finalizer,
	}
	return uc, reader, executor, finalizer, PendingTransitionInput{OrganizationID: organizationID, LedgerID: ledgerID, TransactionID: transactionID}
}

func transitionBalance(organizationID, ledgerID uuid.UUID, id, alias string, available decimal.Decimal) *mmodel.Balance {
	balance := translationBalance(organizationID, ledgerID, id, alias, constant.DefaultBalanceKey)
	balance.Available = available
	balance.OnHold = decimal.NewFromInt(10)
	return balance
}

var _ BalanceEngineGuardBootstrapper = (*transitionEngineExecutor)(nil)
