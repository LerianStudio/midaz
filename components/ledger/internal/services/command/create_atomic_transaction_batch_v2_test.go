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

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libObservability "github.com/LerianStudio/lib-observability/v4"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	feemodel "github.com/LerianStudio/midaz/v4/components/ledger/pkg/feeshared/model"
	"github.com/LerianStudio/midaz/v4/pkg"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

type capturingAtomicBatchEquivalenceEngine struct {
	executions []EngineExecution
}

func (engine *capturingAtomicBatchEquivalenceEngine) Execute(
	_ context.Context,
	execution EngineExecution,
) (*accounting.ExecutionResult, error) {
	engine.executions = append(engine.executions, execution)
	transaction := execution.Execution.Transactions[0]

	return nil, &accounting.Failure{
		Code:             accounting.FailureInsufficientFunds,
		TransactionIndex: 0,
		PostingIndex:     0,
		BalanceRef:       transaction.Postings[0].BalanceRef,
	}
}

type atomicTransactionBatchSettingsReader struct {
	TransactionReader
	settings       mmodel.LedgerSettings
	balances       []*mmodel.Balance
	err            error
	calls          int
	engineReads    int
	organizationID uuid.UUID
	ledgerID       uuid.UUID
}

func (reader *atomicTransactionBatchSettingsReader) GetParsedLedgerSettings(
	_ context.Context,
	organizationID, ledgerID uuid.UUID,
) (mmodel.LedgerSettings, error) {
	reader.calls++
	reader.organizationID = organizationID
	reader.ledgerID = ledgerID

	return reader.settings, reader.err
}

func (reader *atomicTransactionBatchSettingsReader) GetEngineBalances(
	ctx context.Context,
	organizationID, ledgerID uuid.UUID,
	aliases []string,
) ([]*mmodel.Balance, []*mmodel.Balance, error) {
	reader.engineReads++
	pool, err := LoadEngineSnapshotPool(ctx, organizationID, ledgerID, aliases,
		func(_ context.Context, _, _ uuid.UUID, requested []string) ([]*mmodel.Balance, error) {
			selected := make([]*mmodel.Balance, 0, len(requested))
			for _, alias := range requested {
				for _, balance := range reader.balances {
					if mtransaction.AliasKey(balance.Alias, balance.Key) == alias {
						selected = append(selected, balance)
					}
				}
			}

			return selected, nil
		})
	if err != nil {
		return nil, nil, err
	}

	return pool.ExplicitBalances, pool.Balances, nil
}

func (reader *atomicTransactionBatchSettingsReader) ValidateAccountingRules(
	context.Context,
	uuid.UUID,
	uuid.UUID,
	[]mmodel.BalanceOperation,
	*mtransaction.Responses,
	string,
) (*mmodel.TransactionRouteCache, error) {
	return nil, nil
}

func TestInitializeAtomicTransactionBatchV2_FreezesOrderedIDsAndNondecreasingTimestamps(t *testing.T) {
	organizationID := uuid.MustParse("01994f13-29b7-7000-8000-000000000001")
	ledgerID := uuid.MustParse("01994f13-29b7-7000-8000-000000000002")
	batchID := uuid.MustParse("01994f13-29b7-7000-8000-000000000003")
	firstTransactionID := uuid.MustParse("01994f13-29b7-7000-8000-000000000004")
	secondTransactionID := uuid.MustParse("01994f13-29b7-7000-8000-000000000005")

	base := time.Date(2026, time.September, 16, 12, 0, 0, 0, time.UTC)
	reader := &atomicTransactionBatchSettingsReader{settings: mmodel.LedgerSettings{}}
	uc := &UseCase{
		TransactionReader: reader,
		UUIDv7Generator:   orderedAtomicTransactionBatchUUIDs(t, batchID, firstTransactionID, secondTransactionID),
		Clock: orderedAtomicTransactionBatchTimes(
			t,
			base,
			base.Add(-time.Second),
			base.Add(2*time.Second),
			base.Add(time.Second),
			base.Add(4*time.Second),
			base.Add(3*time.Second),
		),
	}

	input := CreateAtomicTransactionBatchV2Input{Transactions: []CreateAtomicTransactionBatchV2ItemInput{
		atomicTransactionBatchItemInput(organizationID, ledgerID, "@source-0", "@destination-0"),
		atomicTransactionBatchItemInput(organizationID, ledgerID, "@source-1", "@destination-1"),
	}}

	run, err := uc.initializeAtomicTransactionBatchV2(context.Background(), input)
	require.NoError(t, err)
	require.Len(t, run.items, 2)

	assert.Equal(t, batchID, run.batchID)
	assert.Equal(t, []int{0, 1}, []int{run.items[0].index, run.items[1].index})
	assert.Equal(t, []uuid.UUID{firstTransactionID, secondTransactionID}, []uuid.UUID{
		run.items[0].transactionID,
		run.items[1].transactionID,
	})
	assert.Equal(t, base, run.items[0].transactionCreatedAt)
	assert.Equal(t, base, run.items[0].transactionUpdatedAt)
	assert.Equal(t, base.Add(2*time.Second), run.items[0].operationUpdatedAt)
	assert.Equal(t, base.Add(2*time.Second), run.items[1].transactionCreatedAt)
	assert.Equal(t, base.Add(4*time.Second), run.items[1].transactionUpdatedAt)
	assert.Equal(t, base.Add(4*time.Second), run.items[1].operationUpdatedAt)
	assert.Equal(t, run.items[0].transactionCreatedAt, run.items[0].transactionDate)
	assert.Equal(t, run.items[1].transactionCreatedAt, run.items[1].transactionDate)

	assert.Equal(t, 1, reader.calls)
	assert.Equal(t, organizationID, reader.organizationID)
	assert.Equal(t, ledgerID, reader.ledgerID)

	// Item state is a deep clone: later fee/default/normalization mutation cannot
	// rewrite the caller's ordered request slice.
	run.items[0].input.Send.Source.From[0].AccountAlias = "@mutated"
	assert.Equal(t, "@source-0", input.Transactions[0].Transaction.Send.Source.From[0].AccountAlias)
}

func TestCreateAtomicTransactionBatchV2_PreservesOrderedResult(t *testing.T) {
	organizationID := uuid.MustParse("01994f13-29b7-7000-8000-000000000031")
	ledgerID := uuid.MustParse("01994f13-29b7-7000-8000-000000000032")
	batchID := uuid.MustParse("01994f13-29b7-7000-8000-000000000033")
	firstTransactionID := uuid.MustParse("01994f13-29b7-7000-8000-000000000034")
	secondTransactionID := uuid.MustParse("01994f13-29b7-7000-8000-000000000035")
	now := time.Date(2026, time.September, 16, 12, 30, 0, 0, time.UTC)
	reader := &atomicTransactionBatchSettingsReader{balances: []*mmodel.Balance{
		atomicTransactionBatchTestBalance(organizationID, ledgerID, "01994f13-29b7-7000-8000-000000000036", "@source-0", "BRL"),
		atomicTransactionBatchTestBalance(organizationID, ledgerID, "01994f13-29b7-7000-8000-000000000037", "@destination-0", "BRL"),
		atomicTransactionBatchTestBalance(organizationID, ledgerID, "01994f13-29b7-7000-8000-000000000038", "@source-1", "BRL"),
		atomicTransactionBatchTestBalance(organizationID, ledgerID, "01994f13-29b7-7000-8000-000000000039", "@destination-1", "BRL"),
	}}
	uc := &UseCase{
		TransactionReader: reader,
		UUIDv7Generator: orderedAtomicTransactionBatchUUIDs(
			t,
			batchID,
			firstTransactionID,
			secondTransactionID,
		),
		Clock: func() time.Time { return now },
	}

	result, err := uc.CreateAtomicTransactionBatchV2(context.Background(), CreateAtomicTransactionBatchV2Input{
		Transactions: []CreateAtomicTransactionBatchV2ItemInput{
			atomicTransactionBatchItemInput(organizationID, ledgerID, "@source-0", "@destination-0"),
			atomicTransactionBatchItemInput(organizationID, ledgerID, "@source-1", "@destination-1"),
		},
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Len(t, result.Transactions, 2)

	assert.Equal(t, batchID, result.BatchID)
	assert.False(t, result.Replayed)
	assert.Equal(t, firstTransactionID.String(), result.Transactions[0].ID)
	assert.Equal(t, secondTransactionID.String(), result.Transactions[1].ID)
	assert.Equal(t, "@source-0", result.Transactions[0].Source[0])
	assert.Equal(t, "@source-1", result.Transactions[1].Source[0])
	assert.Equal(t, now, result.Transactions[0].CreatedAt)
	assert.Equal(t, now, result.Transactions[1].CreatedAt)
	assert.Equal(t, 1, reader.calls)
}

func TestCreateAtomicTransactionBatchV2_RejectsFirstCommonScopeMismatchBeforeExternalWork(t *testing.T) {
	organizationID := uuid.MustParse("01994f13-29b7-7000-8000-000000000011")
	ledgerID := uuid.MustParse("01994f13-29b7-7000-8000-000000000012")
	otherLedgerID := uuid.MustParse("01994f13-29b7-7000-8000-000000000013")
	reader := &atomicTransactionBatchSettingsReader{}
	uc := &UseCase{TransactionReader: reader}

	result, err := uc.CreateAtomicTransactionBatchV2(context.Background(), CreateAtomicTransactionBatchV2Input{
		Transactions: []CreateAtomicTransactionBatchV2ItemInput{
			atomicTransactionBatchItemInput(organizationID, ledgerID, "@source-0", "@destination-0"),
			atomicTransactionBatchItemInput(organizationID, otherLedgerID, "@source-1", "@destination-1"),
			atomicTransactionBatchItemInput(uuid.New(), ledgerID, "@source-2", "@destination-2"),
		},
	})
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, 0, reader.calls)
	var scopeError pkg.UnprocessableOperationError
	require.True(t, errors.As(err, &scopeError))
	assert.Equal(t, constant.ErrTransactionScopeMismatch.Error(), scopeError.Code)

	var carrier *pkg.FieldErrorCarrier
	require.True(t, errors.As(err, &carrier))
	assert.Equal(t, []pkg.FieldError{{
		Location: "body.transactions[1]",
		Message:  "transaction scope must match the first batch item",
	}}, carrier.FieldErrors())
}

func TestCreateAtomicTransactionBatchV2_ReturnsOnlyFirstStateDependentFailure(t *testing.T) {
	organizationID := uuid.MustParse("01994f13-29b7-7000-8000-000000000021")
	ledgerID := uuid.MustParse("01994f13-29b7-7000-8000-000000000022")
	batchID := uuid.MustParse("01994f13-29b7-7000-8000-000000000023")
	transactionIDs := []uuid.UUID{
		uuid.MustParse("01994f13-29b7-7000-8000-000000000024"),
		uuid.MustParse("01994f13-29b7-7000-8000-000000000025"),
		uuid.MustParse("01994f13-29b7-7000-8000-000000000026"),
	}
	now := time.Date(2026, time.September, 16, 13, 0, 0, 0, time.UTC)
	reader := &atomicTransactionBatchSettingsReader{}
	uc := &UseCase{
		TransactionReader: reader,
		UUIDv7Generator: orderedAtomicTransactionBatchUUIDs(
			t,
			batchID,
			transactionIDs[0],
			transactionIDs[1],
			transactionIDs[2],
		),
		Clock: func() time.Time { return now },
	}

	items := []CreateAtomicTransactionBatchV2ItemInput{
		atomicTransactionBatchItemInput(organizationID, ledgerID, "@source-0", "@destination-0"),
		atomicTransactionBatchItemInput(organizationID, ledgerID, "@source-1", "@destination-1"),
		atomicTransactionBatchItemInput(organizationID, ledgerID, "@source-2", "@destination-2"),
	}
	firstFuture := mtransaction.TransactionDate(now.Add(time.Minute))
	secondFuture := mtransaction.TransactionDate(now.Add(2 * time.Minute))
	items[1].Transaction.TransactionDate = &firstFuture
	items[2].Transaction.TransactionDate = &secondFuture

	result, err := uc.CreateAtomicTransactionBatchV2(context.Background(), CreateAtomicTransactionBatchV2Input{Transactions: items})
	require.Error(t, err)
	assert.Nil(t, result)
	assert.Equal(t, 1, reader.calls)
	assertAtomicTransactionBatchValidationCode(t, err, constant.ErrInvalidFutureTransactionDate.Error())

	var carrier *pkg.FieldErrorCarrier
	require.True(t, errors.As(err, &carrier))
	assert.Equal(t, []pkg.FieldError{{
		Location: "body.transactions[1]",
		Message:  "transaction date validation failed",
	}}, carrier.FieldErrors())
}

func TestCreateAtomicTransactionBatchV2_HonorsPerItemControlSkips(t *testing.T) {
	organizationID := uuid.MustParse("01994f13-29b7-7000-8000-000000000061")
	ledgerID := uuid.MustParse("01994f13-29b7-7000-8000-000000000062")
	batchID := uuid.MustParse("01994f13-29b7-7000-8000-000000000063")
	transactionID := uuid.MustParse("01994f13-29b7-7000-8000-000000000064")
	now := time.Date(2026, time.September, 16, 14, 0, 0, 0, time.UTC)
	settings := mmodel.LedgerSettings{}
	settings.Overrides.AllowFeeSkip = true
	settings.Overrides.AllowTracerSkip = true
	reader := &atomicTransactionBatchSettingsReader{
		settings: settings,
		balances: []*mmodel.Balance{
			atomicTransactionBatchTestBalance(organizationID, ledgerID, "01994f13-29b7-7000-8000-000000000065", "@source", "BRL"),
			atomicTransactionBatchTestBalance(organizationID, ledgerID, "01994f13-29b7-7000-8000-000000000066", "@destination", "BRL"),
		},
	}
	feeApplier := &fakeFeeApplier{}
	uc := &UseCase{
		TransactionReader: reader,
		FeeApplier:        feeApplier,
		UUIDv7Generator:   orderedAtomicTransactionBatchUUIDs(t, batchID, transactionID),
		Clock:             func() time.Time { return now },
	}
	item := atomicTransactionBatchItemInput(organizationID, ledgerID, "@source", "@destination")
	item.Transaction.Skip = &mtransaction.TransactionSkip{Fees: true, Tracer: true}

	result, err := uc.CreateAtomicTransactionBatchV2(context.Background(), CreateAtomicTransactionBatchV2Input{
		Transactions: []CreateAtomicTransactionBatchV2ItemInput{item},
	})
	require.NoError(t, err)
	require.Len(t, result.Transactions, 1)

	assert.Equal(t, 0, feeApplier.calls, "an honored fee skip must add no downstream fee work")
	assert.Equal(t, 1, reader.engineReads)
	assert.True(t, result.Transactions[0].FeesSkipped)
	assert.True(t, result.Transactions[0].TracerSkipped)
}

func TestCreateAtomicTransactionBatchV2_OneItemMatchesSingularAccountingAndCompletionIntent(t *testing.T) {
	t.Setenv("AUDIT_LOG_ENABLED", "false")

	organizationID := uuid.MustParse("01994f13-29b7-7000-8000-000000000041")
	ledgerID := uuid.MustParse("01994f13-29b7-7000-8000-000000000042")
	exceptionID := uuid.MustParse("01994f13-29b7-7000-8000-000000000043")
	transactionDate := time.Date(2026, time.September, 15, 12, 0, 0, 0, time.UTC)

	source := atomicTransactionBatchTestBalance(organizationID, ledgerID, "01994f13-29b7-7000-8000-000000000044", "@source", "USD")
	source.Available = decimal.NewFromInt(5)
	source.Direction = constant.DirectionCredit
	source.Settings = &mmodel.BalanceSettings{
		BalanceScope:   mmodel.BalanceScopeTransactional,
		AllowOverdraft: true,
	}
	target := atomicTransactionBatchTestBalance(organizationID, ledgerID, "01994f13-29b7-7000-8000-000000000045", "@target", "USD")
	feeCollector := atomicTransactionBatchTestBalance(organizationID, ledgerID, "01994f13-29b7-7000-8000-000000000046", "@fee", "USD")
	companion := atomicTransactionBatchTestBalance(organizationID, ledgerID, "01994f13-29b7-7000-8000-000000000047", "@source", "USD")
	companion.Key = constant.OverdraftBalanceKey
	companion.AccountID = source.AccountID
	companion.Direction = constant.DirectionDebit
	companion.Settings = &mmodel.BalanceSettings{BalanceScope: mmodel.BalanceScopeInternal}
	balances := []*mmodel.Balance{source, target, feeCollector, companion}

	raw := createEngineTransaction(transactionDate)
	feeMutation := func(calculate *feemodel.FeeCalculate) {
		calculate.Transaction.Send.Value = decimal.NewFromInt(11)
		calculate.Transaction.Send.Source.From[0].Amount.Value = decimal.NewFromInt(11)
		calculate.Transaction.Send.Distribute.To = append(
			calculate.Transaction.Send.Distribute.To,
			mtransaction.FromTo{
				AccountAlias: "@fee",
				Amount: &mtransaction.Amount{
					Asset: "USD",
					Value: decimal.NewFromInt(1),
				},
			},
		)
	}

	ctrl := gomock.NewController(t)
	singularRedis := txRedis.NewMockRedisRepository(ctrl)
	singularRedis.EXPECT().SetNX(gomock.Any(), gomock.Any(), "", time.Minute).Return(true, nil)
	singularRedis.EXPECT().GetAccountBlockException(gomock.Any(), organizationID, ledgerID, exceptionID).
		Return(&mmodel.AccountBlockExceptionRedis{Alias: "@source", Amount: "11"}, nil)
	singularRedis.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil)

	singularReader := &createEngineReader{balances: balances}
	singularFee := &fakeFeeApplier{mutate: feeMutation}
	singularEngine := &capturingAtomicBatchEquivalenceEngine{}
	singular := &UseCase{
		TransactionRedisRepo:        singularRedis,
		TransactionReader:           singularReader,
		Engine:                      singularEngine,
		AppliedTransactionCompleter: &createAppliedTransactionCompleter{},
		FeeApplier:                  singularFee,
	}
	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-batch-equivalence")
	ctx = libObservability.ContextWithHeaderID(ctx, "request-batch-equivalence")

	singularInput, err := clonePendingTransactionInput(raw)
	require.NoError(t, err)
	_, _, err = singular.CreateTransactionV2(ctx, CreateTransactionV2Input{
		OrganizationID:          organizationID,
		LedgerID:                ledgerID,
		Transaction:             singularInput,
		TransactionStatus:       constant.CREATED,
		IdempotencyTTL:          time.Minute,
		AccountBlockExceptionID: &exceptionID,
	})
	require.Error(t, err)
	require.Len(t, singularEngine.executions, 1)
	require.Equal(t, 1, singularFee.calls)
	singularPrepared := singularEngine.executions[0]
	singularTransactionID := singularPrepared.Execution.Transactions[0].ID
	singularPlan := mustCreateEngineRecovery(t, singularPrepared)

	batchRedis := txRedis.NewMockRedisRepository(ctrl)
	batchRedis.EXPECT().GetAccountBlockException(gomock.Any(), organizationID, ledgerID, exceptionID).
		Return(&mmodel.AccountBlockExceptionRedis{Alias: "@source", Amount: "11"}, nil)
	batchReader := &createEngineReader{balances: balances}
	batchFee := &fakeFeeApplier{mutate: feeMutation}
	batchID := uuid.MustParse("01994f13-29b7-7000-8000-000000000048")
	batch := &UseCase{
		TransactionRedisRepo: batchRedis,
		TransactionReader:    batchReader,
		FeeApplier:           batchFee,
		UUIDv7Generator:      orderedAtomicTransactionBatchUUIDs(t, batchID, singularTransactionID),
		Clock:                func() time.Time { return transactionDate.Add(time.Hour) },
	}
	batchInput, err := clonePendingTransactionInput(raw)
	require.NoError(t, err)
	run, err := batch.initializeAtomicTransactionBatchV2(ctx, CreateAtomicTransactionBatchV2Input{
		Transactions: []CreateAtomicTransactionBatchV2ItemInput{{
			OrganizationID:          organizationID,
			LedgerID:                ledgerID,
			Transaction:             batchInput,
			AccountBlockExceptionID: &exceptionID,
		}},
		IdempotencyTTL: time.Minute,
	})
	require.NoError(t, err)
	logger, tracer, _, _ := libObservability.NewTrackingFromContext(ctx)
	prepareCtx, span := tracer.Start(ctx, "test.prepare_atomic_transaction_batch")
	t.Cleanup(func() { span.End() })
	require.NoError(t, batch.prepareAtomicTransactionBatchItems(prepareCtx, span, logger, run))
	require.Equal(t, 1, batchFee.calls)
	require.Len(t, run.items, 1)

	batchItem := &run.items[0]
	assert.Equal(t, singularPrepared.Execution.Transactions[0], batchItem.prepared.transaction)
	assert.Equal(t, singularPrepared.Execution.Balances, batchItem.prepared.pool.Snapshots)
	singularProjection, err := json.Marshal(singularPlan.OperationSpecs)
	require.NoError(t, err)
	batchProjection, err := json.Marshal(batchItem.prepared.projection)
	require.NoError(t, err)
	assert.JSONEq(t, string(singularProjection), string(batchProjection))
	require.NotNil(t, batchItem.prepared.transaction.AccountBlockException)
	assert.Equal(t, exceptionID, batchItem.prepared.transaction.AccountBlockException.ExceptionID)
	assert.Equal(t, accounting.DrawAllowed, batchItem.prepared.transaction.Postings[0].DrawPolicy)
	companionFound := false
	for _, spec := range batchItem.prepared.projection {
		if spec.Role != accounting.RoleOverdraftCompanion {
			continue
		}
		companionFound = true
		assert.Equal(t, singularTransactionID, spec.TransactionID)
		assert.Equal(t, "from:0:debit", spec.PostingRef)
		assert.Equal(t, "@source#overdraft", spec.BalanceRef)
		assert.Equal(t, constant.OVERDRAFT, spec.RowType)
		assert.True(t, spec.RequestedAmount.Equal(decimal.NewFromInt(11)))
	}
	assert.True(t, companionFound, "overdraft companion projection must be prepared")

	batchCreateRun := run.createTransactionRun(batchItem)
	batchPrepared, err := batch.buildCreateEngineExecution(batchCreateRun, createBalanceExecutionContext{
		executionID:        singularPrepared.Execution.ExecutionID,
		tenantID:           singularPlan.TenantID,
		headerID:           singularPlan.HeaderID,
		enqueuedAt:         singularPlan.TTL,
		transactionUpdated: singularPlan.TransactionUpdatedAt,
		operationUpdated:   singularPlan.OperationUpdatedAt,
		guard:              singularPrepared.Guards[0],
	}, batchItem.prepared)
	require.NoError(t, err)
	assert.Equal(t, singularPrepared, batchPrepared.Execution)
	require.Len(t, batchPrepared.CompletionPlans, 1)
	batchPlan, err := EncodeTransactionCompletionPlan(batchPrepared.CompletionPlans[0])
	require.NoError(t, err)
	assert.Equal(t, singularPrepared.CompletionPlans[0].Payload, batchPlan)
}

func atomicTransactionBatchItemInput(
	organizationID, ledgerID uuid.UUID,
	from, to string,
) CreateAtomicTransactionBatchV2ItemInput {
	return CreateAtomicTransactionBatchV2ItemInput{
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		Transaction: mtransaction.Transaction{
			Description: "ordered batch transaction",
			Send: mtransaction.Send{
				Asset: "BRL",
				Value: decimal.NewFromInt(10),
				Source: mtransaction.Source{From: []mtransaction.FromTo{{
					AccountAlias: from,
					Amount:       &mtransaction.Amount{Value: decimal.NewFromInt(10)},
					IsFrom:       true,
				}}},
				Distribute: mtransaction.Distribute{To: []mtransaction.FromTo{{
					AccountAlias: to,
					Amount:       &mtransaction.Amount{Value: decimal.NewFromInt(10)},
				}}},
			},
		},
	}
}

func orderedAtomicTransactionBatchUUIDs(t *testing.T, values ...uuid.UUID) UUIDv7Generator {
	t.Helper()
	index := 0

	return func() (uuid.UUID, error) {
		t.Helper()
		require.Less(t, index, len(values), "UUIDv7 generator called more often than expected")
		value := values[index]
		index++

		return value, nil
	}
}

func orderedAtomicTransactionBatchTimes(t *testing.T, values ...time.Time) Clock {
	t.Helper()
	index := 0

	return func() time.Time {
		t.Helper()
		require.Less(t, index, len(values), "clock called more often than expected")
		value := values[index]
		index++

		return value
	}
}

func atomicTransactionBatchTestBalance(
	organizationID, ledgerID uuid.UUID,
	id, alias, asset string,
) *mmodel.Balance {
	balance := translationBalance(organizationID, ledgerID, id, alias, constant.DefaultBalanceKey)
	balance.AssetCode = asset

	return balance
}

func assertAtomicTransactionBatchValidationCode(t *testing.T, err error, code string) {
	t.Helper()

	var validation pkg.ValidationError
	require.True(t, errors.As(err, &validation))
	assert.Equal(t, code, validation.Code)
}
