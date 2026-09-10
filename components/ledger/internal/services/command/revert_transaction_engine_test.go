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

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	txRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/tracer"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

type revertEngineReader struct {
	*revertReader
	balances []*mmodel.Balance
	reads    int
}

func (reader *revertEngineReader) GetBalances(_ context.Context, _, _ uuid.UUID, aliases []string) ([]*mmodel.Balance, error) {
	reader.reads++
	balances := make([]*mmodel.Balance, 0, len(aliases))
	for _, alias := range aliases {
		for _, balance := range reader.balances {
			if mtransaction.AliasKey(balance.Alias, balance.Key) == alias {
				balances = append(balances, balance)
			}
		}
	}

	return balances, nil
}

func (reader *revertEngineReader) GetBalanceEngineBalances(ctx context.Context, organizationID, ledgerID uuid.UUID, aliases []string) ([]*mmodel.Balance, []*mmodel.Balance, error) {
	pool, err := LoadBalanceEngineSnapshotPool(ctx, organizationID, ledgerID, aliases, reader.GetBalances)
	return pool.ExplicitBalances, pool.Balances, err
}

type revertLiteralEngine struct {
	t        *testing.T
	requests []EngineExecution
}

func (executor *revertLiteralEngine) Execute(_ context.Context, execution EngineExecution) (*accounting.ExecutionResult, error) {
	executor.requests = append(executor.requests, execution)
	require.Len(executor.t, execution.Execution.Transactions, 1)
	transactionIntent := execution.Execution.Transactions[0]
	require.Len(executor.t, transactionIntent.Postings, 2)
	require.Equal(executor.t, accounting.PostingDebit, transactionIntent.Postings[0].Type)
	require.Equal(executor.t, accounting.PostingCredit, transactionIntent.Postings[1].Type)
	require.Equal(executor.t, "@payee#default", transactionIntent.Postings[0].BalanceRef)
	require.Equal(executor.t, "@payer#default", transactionIntent.Postings[1].BalanceRef)
	require.True(executor.t, transactionIntent.Postings[0].Amount.Equal(decimal.NewFromInt(10)))
	require.True(executor.t, transactionIntent.Postings[1].Amount.Equal(decimal.NewFromInt(10)))

	source := createEngineSnapshot(executor.t, execution.Execution.Balances, "@payee#default")
	target := createEngineSnapshot(executor.t, execution.Execution.Balances, "@payer#default")
	require.Equal(executor.t, int64(7), source.Version)
	require.Equal(executor.t, int64(3), target.Version)
	// The atomic engine may start from a newer live cache value than the
	// cache-aside seed carried by Go.
	sourceBefore := accounting.BalanceState{Available: decimal.NewFromInt(50), Version: 8}
	sourceAfter := accounting.BalanceState{Available: decimal.NewFromInt(40), Version: 9}
	targetBefore := accounting.BalanceState{Available: decimal.NewFromInt(20), Version: 3}
	targetAfter := accounting.BalanceState{Available: decimal.NewFromInt(30), Version: 4}
	source.Available, source.Version = decimal.NewFromInt(40), 9
	target.Available, target.Version = decimal.NewFromInt(30), 4

	return &accounting.ExecutionResult{
		Movements: []accounting.Movement{
			{
				Ref: "revert-source", TransactionID: transactionIntent.ID, PostingRef: transactionIntent.Postings[0].Ref,
				Role: accounting.RolePrimary, BalanceRef: source.BalanceRef, Type: accounting.PostingDebit,
				Amount: decimal.NewFromInt(10), Before: sourceBefore, After: sourceAfter,
			},
			{
				Ref: "revert-target", TransactionID: transactionIntent.ID, PostingRef: transactionIntent.Postings[1].Ref,
				Role: accounting.RolePrimary, BalanceRef: target.BalanceRef, Type: accounting.PostingCredit,
				Amount: decimal.NewFromInt(10), Before: targetBefore, After: targetAfter,
			},
		},
		Final: []accounting.BalanceSnapshot{source, target},
	}, nil
}

func TestRevertTransactionV2UsesOptInBalanceEngineWithStableChildIdentity(t *testing.T) {
	t.Setenv("AUDIT_LOG_ENABLED", "false")
	ctrl := gomock.NewController(t)
	redisRepo := txRedis.NewMockRedisRepository(ctrl)
	idempotencySet := make(chan struct{})
	redisRepo.EXPECT().SetNX(gomock.Any(), gomock.Any(), "", time.Duration(300)).Return(true, nil).Times(1)
	redisRepo.EXPECT().Set(gomock.Any(), gomock.Any(), gomock.Any(), time.Duration(300)).DoAndReturn(
		func(context.Context, string, string, time.Duration) error {
			close(idempotencySet)
			return nil
		},
	).Times(1)

	organizationID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	ledgerID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	originID := uuid.MustParse("33333333-3333-4333-8333-333333333333")
	origin := revertEngineOrigin(organizationID, ledgerID, originID)
	settings := mmodel.LedgerSettings{}
	settings.Tracer.Mode = mmodel.TracerModeEnforce
	reader := &revertEngineReader{
		revertReader: &revertReader{origin: origin, versionReader: versionReader{settings: settings}},
		balances: []*mmodel.Balance{
			revertEngineBalance(organizationID, ledgerID, "44444444-4444-4444-8444-444444444444", "@payee", 50, 7),
			revertEngineBalance(organizationID, ledgerID, "55555555-5555-4555-8555-555555555555", "@payer", 20, 3),
		},
	}
	executor := &revertLiteralEngine{t: t}
	finalizer := &createEngineFinalizer{outcome: TransactionPersistenceOutcome{TransactionStatus: constant.APPROVED}}
	reservationID := uuid.MustParse("66666666-6666-4666-8666-666666666666")
	reserver := &stubReserver{result: &tracer.ReserveResult{ReservationIDs: []uuid.UUID{reservationID}}}
	uc := &UseCase{
		TransactionRedisRepo: redisRepo, TransactionReader: reader,
		BalanceEngine: executor, TransactionCompleter: finalizer, TracerReserver: reserver,
	}
	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-revert")
	ctx = libObservability.ContextWithHeaderID(ctx, "revert-request")

	got, replayed, err := uc.RevertTransactionV2(ctx, RevertTransactionInput{
		OrganizationID: organizationID, LedgerID: ledgerID, TransactionID: originID,
	})
	require.NoError(t, err)
	assert.False(t, replayed)
	require.NotNil(t, got)
	require.NotNil(t, got.ParentTransactionID)
	assert.Equal(t, originID.String(), *got.ParentTransactionID)
	assert.NotEqual(t, originID.String(), got.ID)
	assert.Equal(t, constant.CREATED, got.Status.Code)
	require.Len(t, got.Operations, 2)
	assert.Equal(t, []string{"10", "10"}, []string{got.Operations[0].Amount.Value.String(), got.Operations[1].Amount.Value.String()})

	require.Len(t, executor.requests, 1)
	firstExecution := executor.requests[0]
	assert.NotEqual(t, originID, firstExecution.Execution.Transactions[0].ID)
	assert.Equal(t, got.ID, firstExecution.Execution.Transactions[0].ID.String())
	assert.GreaterOrEqual(t, reader.reads, 2)

	require.Len(t, finalizer.envelopes, 1)
	payload := mustCreateEnginePayload(t, finalizer.envelopes[0])
	require.NotNil(t, payload.ParentTransactionID)
	assert.Equal(t, originID, *payload.ParentTransactionID)
	assert.Equal(t, constant.ActionRevert, payload.Action)
	assert.Equal(t, constant.CREATED, payload.TransactionStatus)
	assert.False(t, payload.FeesSkipped)
	assert.False(t, payload.TracerSkipped)
	require.Len(t, payload.TransactionInput.Send.Source.From, 1)
	require.NotNil(t, payload.TransactionInput.Send.Source.From[0].Amount)
	assert.Equal(t, "10", payload.TransactionInput.Send.Source.From[0].Amount.Value.String())
	assert.Equal(t, "revert-request", payload.HeaderID)
	assert.Equal(t, "tenant-revert", payload.TenantID)

	assert.Equal(t, 1, reserver.reserveCalls)
	assert.Equal(t, []uuid.UUID{reservationID}, reserver.confirmedIDs)
	assert.Empty(t, reserver.releasedIDs)
	select {
	case <-idempotencySet:
	case <-time.After(time.Second):
		t.Fatal("durable revert did not populate the idempotency value")
	}
}

func TestRevertTransactionBalanceEngineIndeterminateFailureRetainsClaim(t *testing.T) {
	t.Setenv("AUDIT_LOG_ENABLED", "false")
	ctrl := gomock.NewController(t)
	redisRepo := txRedis.NewMockRedisRepository(ctrl)
	redisRepo.EXPECT().SetNX(gomock.Any(), gomock.Any(), "", time.Duration(300)).Return(true, nil).Times(1)
	organizationID := uuid.MustParse("71111111-1111-4111-8111-111111111111")
	ledgerID := uuid.MustParse("72222222-2222-4222-8222-222222222222")
	originID := uuid.MustParse("73333333-3333-4333-8333-333333333333")
	reader := &revertEngineReader{
		revertReader: &revertReader{origin: revertEngineOrigin(organizationID, ledgerID, originID)},
		balances: []*mmodel.Balance{
			revertEngineBalance(organizationID, ledgerID, "74444444-4444-4444-8444-444444444444", "@payee", 50, 7),
			revertEngineBalance(organizationID, ledgerID, "75555555-5555-4555-8555-555555555555", "@payer", 20, 3),
		},
	}
	transportFailure := errors.New("balance engine outcome unknown")
	executor := &createEngineErrorExecutor{err: transportFailure}
	finalizer := &createEngineFinalizer{outcome: TransactionPersistenceOutcome{TransactionStatus: constant.APPROVED}}
	uc := &UseCase{
		TransactionRedisRepo: redisRepo, TransactionReader: reader,
		BalanceEngine: executor, TransactionCompleter: finalizer,
	}

	got, replayed, err := uc.RevertTransactionV1(context.Background(), RevertTransactionInput{
		OrganizationID: organizationID, LedgerID: ledgerID, TransactionID: originID,
	})
	require.ErrorIs(t, err, transportFailure)
	assert.Nil(t, got)
	assert.False(t, replayed)
	assert.Len(t, executor.requests, 1)
	assert.Empty(t, finalizer.envelopes)
}

func revertEngineOrigin(organizationID, ledgerID, originID uuid.UUID) *transaction.Transaction {
	amount := decimal.NewFromInt(10)
	companionAmount := decimal.NewFromInt(50)
	return &transaction.Transaction{
		ID: originID.String(), OrganizationID: organizationID.String(), LedgerID: ledgerID.String(),
		AssetCode: "USD", Amount: &amount, Status: transaction.Status{Code: constant.APPROVED},
		TracerSkipped: true,
		Operations: []*operation.Operation{
			{Type: constant.DEBIT, AccountAlias: "@payer", Amount: operation.Amount{Value: &amount}, AssetCode: "USD", BalanceKey: constant.DefaultBalanceKey},
			{Type: constant.CREDIT, AccountAlias: "@payee", Amount: operation.Amount{Value: &amount}, AssetCode: "USD", BalanceKey: constant.DefaultBalanceKey},
			{
				Type: constant.OVERDRAFT, Direction: constant.DirectionCredit, AccountAlias: "@payee",
				Amount: operation.Amount{Value: &companionAmount}, AssetCode: "USD", BalanceKey: constant.OverdraftBalanceKey,
			},
		},
	}
}

func revertEngineBalance(organizationID, ledgerID uuid.UUID, id, alias string, available, version int64) *mmodel.Balance {
	return &mmodel.Balance{
		ID: id, OrganizationID: organizationID.String(), LedgerID: ledgerID.String(),
		AccountID: uuid.NewSHA1(uuid.NameSpaceOID, []byte(alias)).String(),
		Alias:     alias, Key: constant.DefaultBalanceKey, AssetCode: "USD", AccountType: "deposit",
		Available: decimal.NewFromInt(available), Version: version, Direction: constant.DirectionCredit,
		AllowSending: true, AllowReceiving: true,
	}
}

var _ BalanceEngine = (*revertLiteralEngine)(nil)
