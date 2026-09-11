// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
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

	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

type createEngineReader struct {
	TransactionReader
	settings mmodel.LedgerSettings
	balances []*mmodel.Balance
	reads    int
}

func (reader *createEngineReader) GetParsedLedgerSettings(context.Context, uuid.UUID, uuid.UUID) (mmodel.LedgerSettings, error) {
	return reader.settings, nil
}

func (reader *createEngineReader) GetBalances(_ context.Context, _, _ uuid.UUID, aliases []string) ([]*mmodel.Balance, error) {
	reader.reads++
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

func (reader *createEngineReader) GetEngineBalances(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Balance, []*mmodel.Balance, error) {
	pool, err := LoadEngineSnapshotPool(ctx, organizationID, ledgerID, aliases, reader.GetBalances)
	return pool.ExplicitBalances, pool.Balances, err
}

func (reader *createEngineReader) ValidateAccountingRules(context.Context, uuid.UUID, uuid.UUID, []mmodel.BalanceOperation, *mtransaction.Responses, string) (*mmodel.TransactionRouteCache, error) {
	return nil, nil
}

type createAppliedTransactionCompleter struct {
	outcome   TransactionPersistenceOutcome
	err       error
	envelopes []*TransactionCompletionRecord
}

func (finalizer *createAppliedTransactionCompleter) Complete(_ context.Context, envelope *TransactionCompletionRecord) (TransactionCompletionResult, error) {
	finalizer.envelopes = append(finalizer.envelopes, envelope)
	if finalizer.err != nil {
		return TransactionCompletionResult{}, finalizer.err
	}

	payload, err := DecodeTransactionCompletionPlan([]byte(envelope.Payload))
	if err != nil {
		return TransactionCompletionResult{}, err
	}

	record, err := BuildTransactionWriteSet(*payload, envelope.Result)
	if err != nil {
		return TransactionCompletionResult{}, err
	}

	return TransactionCompletionResult{Record: record, Outcome: finalizer.outcome}, nil
}

type applyingCreateEngine struct {
	t                      *testing.T
	requests               []EngineExecution
	expectedSourceVersions []int64
	before                 func(EngineExecution) error
}

func (executor *applyingCreateEngine) Execute(_ context.Context, execution EngineExecution) (*accounting.ExecutionResult, error) {
	executor.requests = append(executor.requests, execution)
	require.NotNil(executor.t, executor.t)
	require.Len(executor.t, execution.Execution.Transactions, 1)
	call := len(executor.requests) - 1
	require.Less(executor.t, call, len(executor.expectedSourceVersions))
	expectedSourceVersion := executor.expectedSourceVersions[call]
	transaction := execution.Execution.Transactions[0]
	require.NotEmpty(executor.t, transaction.Postings)
	require.True(executor.t, transaction.Postings[0].Amount.Equal(decimal.NewFromInt(10)))
	source := createEngineSnapshot(executor.t, execution.Execution.Balances, transaction.Postings[0].BalanceRef)
	require.True(executor.t, source.Available.Equal(decimal.NewFromInt(100)))
	require.True(executor.t, source.OnHold.IsZero())
	require.Equal(executor.t, expectedSourceVersion, source.Version)

	if executor.before != nil {
		if err := executor.before(execution); err != nil {
			return nil, err
		}
	}

	afterSourceVersion := int64(2)
	if expectedSourceVersion == 2 {
		afterSourceVersion = 3
	}
	sourceBefore := accounting.BalanceState{Available: decimal.NewFromInt(100), OnHold: decimal.Zero, OverdraftUsed: decimal.Zero, Version: expectedSourceVersion}
	sourceAfter := accounting.BalanceState{Available: decimal.NewFromInt(90), OnHold: decimal.Zero, OverdraftUsed: decimal.Zero, Version: afterSourceVersion}
	if len(transaction.Postings) == 1 {
		require.Equal(executor.t, accounting.PostingHold, transaction.Postings[0].Type)
		sourceAfter.OnHold = decimal.NewFromInt(10)
		movement := createEngineMovement(transaction.ID, transaction.Postings[0], "movement-hold", sourceBefore, sourceAfter)
		source.Available, source.OnHold, source.Version = decimal.NewFromInt(90), decimal.NewFromInt(10), afterSourceVersion
		return &accounting.ExecutionResult{Movements: []accounting.Movement{movement}, Final: []accounting.BalanceSnapshot{source}}, nil
	}

	require.Len(executor.t, transaction.Postings, 2)
	require.Equal(executor.t, accounting.PostingDebit, transaction.Postings[0].Type)
	require.Equal(executor.t, accounting.PostingCredit, transaction.Postings[1].Type)
	require.True(executor.t, transaction.Postings[1].Amount.Equal(decimal.NewFromInt(10)))
	target := createEngineSnapshot(executor.t, execution.Execution.Balances, transaction.Postings[1].BalanceRef)
	require.True(executor.t, target.Available.Equal(decimal.NewFromInt(100)))
	require.True(executor.t, target.OnHold.IsZero())
	require.Equal(executor.t, int64(1), target.Version)
	targetBefore := accounting.BalanceState{Available: decimal.NewFromInt(100), OnHold: decimal.Zero, OverdraftUsed: decimal.Zero, Version: 1}
	targetAfter := accounting.BalanceState{Available: decimal.NewFromInt(110), OnHold: decimal.Zero, OverdraftUsed: decimal.Zero, Version: 2}
	movements := []accounting.Movement{
		createEngineMovement(transaction.ID, transaction.Postings[0], "movement-source", sourceBefore, sourceAfter),
		createEngineMovement(transaction.ID, transaction.Postings[1], "movement-target", targetBefore, targetAfter),
	}
	source.Available, source.OnHold, source.Version = decimal.NewFromInt(90), decimal.Zero, afterSourceVersion
	target.Available, target.OnHold, target.Version = decimal.NewFromInt(110), decimal.Zero, 2
	return &accounting.ExecutionResult{Movements: movements, Final: []accounting.BalanceSnapshot{source, target}}, nil
}

func createEngineSnapshot(t *testing.T, snapshots []accounting.BalanceSnapshot, balanceRef string) accounting.BalanceSnapshot {
	t.Helper()
	for _, snapshot := range snapshots {
		if snapshot.BalanceRef == balanceRef {
			return snapshot
		}
	}
	t.Fatalf("missing test snapshot %q", balanceRef)
	return accounting.BalanceSnapshot{}
}

func createEngineMovement(transactionID uuid.UUID, posting accounting.Posting, ref string, before, after accounting.BalanceState) accounting.Movement {
	return accounting.Movement{
		Ref: ref, TransactionID: transactionID, PostingRef: posting.Ref,
		Role: accounting.RolePrimary, BalanceRef: posting.BalanceRef, Type: posting.Type,
		Amount: decimal.NewFromInt(10), Before: before, After: after,
	}
}

type createEngineErrorExecutor struct {
	err      error
	requests []EngineExecution
}

func (executor *createEngineErrorExecutor) Execute(_ context.Context, execution EngineExecution) (*accounting.ExecutionResult, error) {
	executor.requests = append(executor.requests, execution)
	return nil, executor.err
}

func TestCreateTransactionV1UsesOptInEngineWithoutLegacyMutationPorts(t *testing.T) {
	t.Setenv("AUDIT_LOG_ENABLED", "false")
	ctrl := gomock.NewController(t)
	redisRepo := txRedis.NewMockRedisRepository(ctrl)
	idempotencySet := make(chan struct{})
	redisRepo.EXPECT().SetNX(gomock.Any(), gomock.Any(), "", time.Minute).Return(true, nil)
	redisRepo.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), time.Minute).DoAndReturn(
		func(context.Context, string, string, time.Duration) error {
			close(idempotencySet)
			return nil
		},
	)

	organizationID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	ledgerID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	source := translationBalance(organizationID, ledgerID, "33333333-3333-4333-8333-333333333333", "@source", constant.DefaultBalanceKey)
	target := translationBalance(organizationID, ledgerID, "44444444-4444-4444-8444-444444444444", "@target", constant.DefaultBalanceKey)
	reader := &createEngineReader{balances: []*mmodel.Balance{source, target}}
	executor := &applyingCreateEngine{t: t, expectedSourceVersions: []int64{1}}
	finalizer := &createAppliedTransactionCompleter{outcome: TransactionPersistenceOutcome{TransactionStatus: constant.APPROVED}}
	uc := &UseCase{
		TransactionRedisRepo:        redisRepo,
		TransactionReader:           reader,
		Engine:                      executor,
		AppliedTransactionCompleter: finalizer,
	}

	transactionDate := time.Date(2026, time.September, 8, 12, 30, 0, 0, time.UTC)
	input := createEngineTransaction(transactionDate)
	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-a")
	ctx = libObservability.ContextWithHeaderID(ctx, "request-a")

	got, replayed, err := uc.CreateTransactionV1(ctx, CreateTransactionV1Input{
		OrganizationID: organizationID, LedgerID: ledgerID, Transaction: input,
		TransactionStatus: constant.CREATED, IdempotencyTTL: time.Minute,
	})
	require.NoError(t, err)
	assert.False(t, replayed)
	require.NotNil(t, got)
	assert.Equal(t, constant.CREATED, got.Status.Code)
	assert.Equal(t, transactionDate, got.CreatedAt)
	assert.Len(t, got.Operations, 2)
	require.Len(t, executor.requests, 1)
	require.Len(t, finalizer.envelopes, 1)
	assert.Equal(t, "tenant-a", finalizer.envelopes[0].TenantID)
	payload := mustCreateEnginePayload(t, finalizer.envelopes[0])
	assert.Equal(t, "request-a", payload.HeaderID)
	assert.Equal(t, executor.requests[0].Execution.ExecutionID, finalizer.envelopes[0].ExecutionID)
	assert.GreaterOrEqual(t, reader.reads, 2)
	projected, err := BuildOperationRecordsFromMovements(*payload, finalizer.envelopes[0].Result)
	require.NoError(t, err)
	assert.Equal(t, projected, got.Operations)
	assert.Equal(t, []string{"10", "10"}, []string{got.Operations[0].Amount.Value.String(), got.Operations[1].Amount.Value.String()})
	assert.Equal(t, []int64{1, 1}, []int64{*got.Operations[0].Balance.Version, *got.Operations[1].Balance.Version})
	assert.Equal(t, []int64{2, 2}, []int64{*got.Operations[0].BalanceAfter.Version, *got.Operations[1].BalanceAfter.Version})

	select {
	case <-idempotencySet:
	case <-time.After(time.Second):
		t.Fatal("durable success did not populate the idempotency value")
	}
}

func TestCreateTransactionV2ExecutesPreparedBalancesOnce(t *testing.T) {
	t.Setenv("AUDIT_LOG_ENABLED", "false")
	ctrl := gomock.NewController(t)
	redisRepo := txRedis.NewMockRedisRepository(ctrl)
	idempotencySet := make(chan struct{})
	redisRepo.EXPECT().SetNX(gomock.Any(), gomock.Any(), "", time.Minute).Return(true, nil).Times(1)
	redisRepo.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), time.Minute).DoAndReturn(
		func(context.Context, string, string, time.Duration) error {
			close(idempotencySet)
			return nil
		},
	).Times(1)

	organizationID := uuid.MustParse("aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
	ledgerID := uuid.MustParse("bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb")
	source := translationBalance(organizationID, ledgerID, "cccccccc-cccc-4ccc-8ccc-cccccccccccc", "@source", constant.DefaultBalanceKey)
	target := translationBalance(organizationID, ledgerID, "dddddddd-dddd-4ddd-8ddd-dddddddddddd", "@target", constant.DefaultBalanceKey)
	settings := mmodel.LedgerSettings{}
	settings.Tracer.Mode = mmodel.TracerModeEnforce
	reader := &createEngineReader{settings: settings, balances: []*mmodel.Balance{source, target}}
	executor := &applyingCreateEngine{t: t, expectedSourceVersions: []int64{1}}
	finalizer := &createAppliedTransactionCompleter{outcome: TransactionPersistenceOutcome{TransactionStatus: constant.APPROVED}}
	feeApplier := &fakeFeeApplier{}
	reservationID := uuid.MustParse("eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee")
	reserver := &stubReserver{result: &tracer.ReserveResult{ReservationIDs: []uuid.UUID{reservationID}}}
	uc := &UseCase{
		TransactionRedisRepo: redisRepo, TransactionReader: reader,
		Engine: executor, AppliedTransactionCompleter: finalizer,
		FeeApplier: feeApplier, TracerReserver: reserver,
	}

	transactionDate := time.Date(2026, time.September, 8, 13, 45, 0, 0, time.UTC)
	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-v2")
	ctx = libObservability.ContextWithHeaderID(ctx, "request-v2")
	got, replayed, err := uc.CreateTransactionV2(ctx, CreateTransactionV2Input{
		OrganizationID: organizationID, LedgerID: ledgerID,
		Transaction: createEngineTransaction(transactionDate), TransactionStatus: constant.CREATED,
		IdempotencyTTL: time.Minute,
	})
	require.NoError(t, err)
	assert.False(t, replayed)
	require.NotNil(t, got)
	assert.Equal(t, 1, feeApplier.calls)
	assert.Equal(t, 1, reserver.reserveCalls)
	assert.Equal(t, []uuid.UUID{reservationID}, reserver.confirmedIDs)
	assert.Empty(t, reserver.releasedIDs)
	require.Len(t, executor.requests, 1)
	assert.Equal(t, int64(1), executor.requests[0].Execution.Balances[0].Version)
	payload := mustCreateEngineRecovery(t, executor.requests[0])
	assert.Equal(t, transactionDate, payload.TransactionDate)
	assert.Equal(t, transactionDate, payload.TransactionCreatedAt)

	select {
	case <-idempotencySet:
	case <-time.After(time.Second):
		t.Fatal("durable v2 success did not populate the idempotency value")
	}
}

func TestCreateTransactionEnginePendingRetainsBodyAndDefersTracerConfirm(t *testing.T) {
	t.Setenv("AUDIT_LOG_ENABLED", "false")
	ctrl := gomock.NewController(t)
	redisRepo := txRedis.NewMockRedisRepository(ctrl)
	idempotencySet := make(chan struct{})
	redisRepo.EXPECT().SetNX(gomock.Any(), gomock.Any(), "", time.Minute).Return(true, nil)
	redisRepo.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), time.Minute).DoAndReturn(
		func(context.Context, string, string, time.Duration) error {
			close(idempotencySet)
			return nil
		},
	)

	organizationID := uuid.MustParse("12121212-1212-4212-8212-121212121212")
	ledgerID := uuid.MustParse("34343434-3434-4434-8434-343434343434")
	settings := mmodel.LedgerSettings{}
	settings.Tracer.Mode = mmodel.TracerModeEnforce
	reader := &createEngineReader{settings: settings, balances: []*mmodel.Balance{
		translationBalance(organizationID, ledgerID, "56565656-5656-4656-8656-565656565656", "@source", constant.DefaultBalanceKey),
		translationBalance(organizationID, ledgerID, "78787878-7878-4878-8878-787878787878", "@target", constant.DefaultBalanceKey),
	}}
	executor := &applyingCreateEngine{t: t, expectedSourceVersions: []int64{1}}
	finalizer := &createAppliedTransactionCompleter{outcome: TransactionPersistenceOutcome{TransactionStatus: constant.PENDING}}
	reservationID := uuid.MustParse("90909090-9090-4090-8090-909090909090")
	reserver := &stubReserver{result: &tracer.ReserveResult{ReservationIDs: []uuid.UUID{reservationID}}}
	uc := &UseCase{
		TransactionRedisRepo: redisRepo, TransactionReader: reader,
		Engine: executor, AppliedTransactionCompleter: finalizer, TracerReserver: reserver,
	}

	transactionDate := time.Date(2026, time.September, 8, 15, 0, 0, 0, time.UTC)
	input := createEngineTransaction(transactionDate)
	input.Pending = true
	input.TransactionDate = nil
	got, _, err := uc.CreateTransactionV2(
		tmcore.ContextWithTenantID(context.Background(), "tenant-pending"),
		CreateTransactionV2Input{
			OrganizationID: organizationID, LedgerID: ledgerID, Transaction: input,
			TransactionStatus: constant.PENDING, IdempotencyTTL: time.Minute,
		},
	)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, constant.PENDING, got.Status.Code)
	assert.Equal(t, input.Send.Value, got.Body.Send.Value)
	assert.Equal(t, 1, reserver.reserveCalls)
	assert.Empty(t, reserver.confirmedIDs)
	assert.Empty(t, reserver.releasedIDs)
	require.Len(t, executor.requests, 1)
	assert.Equal(t, ExecutionGuard{
		TransactionID: executor.requests[0].Execution.Transactions[0].ID,
		ExpectedToken: "", NextToken: constant.PENDING,
	}, executor.requests[0].Guards[0])

	select {
	case <-idempotencySet:
	case <-time.After(time.Second):
		t.Fatal("durable pending success did not populate the idempotency value")
	}
}

func TestCreateTransactionEngineFailureCleanupBoundary(t *testing.T) {
	t.Setenv("AUDIT_LOG_ENABLED", "false")
	technicalFailure := errors.New("transport outcome unknown")
	finalizationFailure := errors.New("sql unavailable")

	for _, test := range []struct {
		name        string
		executor    func() Engine
		finalizeErr error
		wantDelete  bool
	}{
		{
			name: "confirmed financial refusal releases claim",
			executor: func() Engine {
				return &applyingCreateEngine{t: t, expectedSourceVersions: []int64{1}, before: func(execution EngineExecution) error {
					return &accounting.Failure{
						Code: accounting.FailureInsufficientFunds, TransactionIndex: 0, PostingIndex: 0,
						BalanceRef: execution.Execution.Transactions[0].Postings[0].BalanceRef,
					}
				}}
			},
			wantDelete: true,
		},
		{name: "unknown transport retains claim", executor: func() Engine {
			return &createEngineErrorExecutor{err: technicalFailure}
		}},
		{name: "post-success finalization failure retains claim", executor: func() Engine {
			return &applyingCreateEngine{t: t, expectedSourceVersions: []int64{1}}
		}, finalizeErr: finalizationFailure},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			redisRepo := txRedis.NewMockRedisRepository(ctrl)
			redisRepo.EXPECT().SetNX(gomock.Any(), gomock.Any(), "", time.Minute).Return(true, nil).Times(1)
			if test.wantDelete {
				redisRepo.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).Times(1)
			}

			organizationID := uuid.MustParse("11111111-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
			ledgerID := uuid.MustParse("22222222-bbbb-4bbb-8bbb-bbbbbbbbbbbb")
			reader := &createEngineReader{balances: []*mmodel.Balance{
				translationBalance(organizationID, ledgerID, "33333333-cccc-4ccc-8ccc-cccccccccccc", "@source", constant.DefaultBalanceKey),
				translationBalance(organizationID, ledgerID, "44444444-dddd-4ddd-8ddd-dddddddddddd", "@target", constant.DefaultBalanceKey),
			}}
			finalizer := &createAppliedTransactionCompleter{
				outcome: TransactionPersistenceOutcome{TransactionStatus: constant.APPROVED}, err: test.finalizeErr,
			}
			uc := &UseCase{
				TransactionRedisRepo: redisRepo, TransactionReader: reader,
				Engine: test.executor(), AppliedTransactionCompleter: finalizer,
			}
			transactionDate := time.Date(2026, time.September, 8, 14, 15, 0, 0, time.UTC)

			_, replayed, err := uc.CreateTransactionV1(
				tmcore.ContextWithTenantID(context.Background(), "tenant-failure"),
				CreateTransactionV1Input{
					OrganizationID: organizationID, LedgerID: ledgerID,
					Transaction: createEngineTransaction(transactionDate), TransactionStatus: constant.CREATED,
					IdempotencyTTL: time.Minute,
				},
			)
			require.Error(t, err)
			assert.False(t, replayed)
			if test.finalizeErr != nil {
				assert.ErrorIs(t, err, test.finalizeErr)
				assert.Len(t, finalizer.envelopes, 1)
			} else {
				assert.Empty(t, finalizer.envelopes)
			}
		})
	}
}

func TestConfirmedPrecommitEngineFailureIsConservative(t *testing.T) {
	request := accounting.Execution{
		Transactions: []accounting.Transaction{{
			BalanceRequirements: []accounting.BalanceRequirement{{BalanceRef: "@source#default", AssetCode: "USD", Permission: accounting.BalancePermissionSend}},
			Postings:            []accounting.Posting{{Ref: "source", BalanceRef: "@source#default"}},
		}},
		Balances: []accounting.BalanceSnapshot{{BalanceRef: "@source#default"}},
	}
	financial := &accounting.Failure{Code: accounting.FailureInsufficientFunds, TransactionIndex: 0, PostingIndex: 0, BalanceRef: "@source#default"}
	technical := testEngineTechnicalError{code: "execute_indeterminate", indeterminate: true, cause: financial}
	assert.False(t, confirmedPrecommitEngineFailure(request, technical))
	assert.False(t, confirmedPrecommitEngineFailure(request, &accounting.Failure{
		Code: "unknown", TransactionIndex: 0, PostingIndex: 0, BalanceRef: financial.BalanceRef,
	}))
	assert.True(t, confirmedPrecommitEngineFailure(request, financial))
	assert.True(t, confirmedPrecommitEngineFailure(request, &accounting.Failure{
		Code: accounting.FailureSendingNotAllowed, TransactionIndex: 0, PostingIndex: -1, BalanceRef: "@source#default",
	}))
}

func createEngineTransaction(transactionDate time.Time) mtransaction.Transaction {
	return mtransaction.Transaction{
		Description:     "engine create",
		TransactionDate: (*mtransaction.TransactionDate)(&transactionDate),
		Send: mtransaction.Send{
			Asset: "USD", Value: decimal.NewFromInt(10),
			Source: mtransaction.Source{From: []mtransaction.FromTo{{
				AccountAlias: "@source", Amount: &mtransaction.Amount{Asset: "USD", Value: decimal.NewFromInt(10)},
			}}},
			Distribute: mtransaction.Distribute{To: []mtransaction.FromTo{{
				AccountAlias: "@target", Amount: &mtransaction.Amount{Asset: "USD", Value: decimal.NewFromInt(10)},
			}}},
		},
	}
}

func mustCreateEnginePayload(t *testing.T, envelope *TransactionCompletionRecord) *TransactionCompletionPlan {
	t.Helper()
	payload, err := DecodeTransactionCompletionPlan([]byte(envelope.Payload))
	require.NoError(t, err)
	return payload
}

func mustCreateEngineRecovery(t *testing.T, execution EngineExecution) *TransactionCompletionPlan {
	t.Helper()
	require.Len(t, execution.CompletionPlans, 1)
	payload, err := DecodeTransactionCompletionPlan(execution.CompletionPlans[0].Payload)
	require.NoError(t, err)
	return payload
}

func TestIdempotencyRetentionSecondsSupportsBothDurationConventions(t *testing.T) {
	t.Parallel()

	require.Equal(t, int64(300), idempotencyRetentionSeconds(time.Duration(300)))
	require.Equal(t, int64(60), idempotencyRetentionSeconds(time.Minute))
	require.Equal(t, int64(604800), idempotencyRetentionSeconds(7*24*time.Hour))
}

var (
	_ AppliedTransactionCompleter = (*createAppliedTransactionCompleter)(nil)
	_ Engine                      = (*applyingCreateEngine)(nil)
	_ Engine                      = (*createEngineErrorExecutor)(nil)
)
