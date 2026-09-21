// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package query

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
	"go.uber.org/mock/gomock"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	postgres "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

type engineWriteBehindRepositoryFake struct {
	index             []byte
	indexErr          error
	materialized      *redis.EngineMaterializedTransaction
	materializedErr   error
	envelope          []byte
	receipt           []byte
	evidenceErr       error
	materializeResult bool
	materializeErr    error
	materializedCalls int
}

func (fake *engineWriteBehindRepositoryFake) GetEngineTransactionIndex(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) ([]byte, error) {
	return append([]byte(nil), fake.index...), fake.indexErr
}

func (fake *engineWriteBehindRepositoryFake) GetEngineTransactionEvidence(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID) ([]byte, []byte, error) {
	return append([]byte(nil), fake.envelope...), append([]byte(nil), fake.receipt...), fake.evidenceErr
}

func (fake *engineWriteBehindRepositoryFake) GetEngineMaterializedTransaction(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*redis.EngineMaterializedTransaction, error) {
	return fake.materialized, fake.materializedErr
}

func (fake *engineWriteBehindRepositoryFake) MaterializeEngineTransaction(_ context.Context, _, _, _, _ uuid.UUID, _ []byte, _ time.Duration) (bool, error) {
	fake.materializedCalls++
	return fake.materializeResult, fake.materializeErr
}

func TestResolveEngineWriteBehindTransactionReconstructsAndMaterializes(t *testing.T) {
	index, envelope, receipt, expected := queryEngineWriteBehindFixture(t)
	fake := &engineWriteBehindRepositoryFake{
		index: index, envelope: envelope, receipt: receipt, materializeResult: true,
		materialized: &redis.EngineMaterializedTransaction{FormatVersion: 1, ExecutionID: expectedExecutionID(t), Payload: []byte("corrupt")},
	}
	uc := &UseCase{EngineWriteBehindRepo: fake, EngineWriteBehindCodec: command.EngineWriteBehindEvidenceCodec{}}
	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-query")

	resolved, err := uc.ResolveEngineWriteBehindTransaction(ctx, uuid.MustParse(expected.OrganizationID), uuid.MustParse(expected.LedgerID), uuid.MustParse(expected.ID))
	require.NoError(t, err)
	require.Equal(t, EngineTransactionResolutionEvidence, resolved.Source)
	require.True(t, resolved.Pending)
	require.Equal(t, expected.ID, resolved.Transaction.ID)
	require.Equal(t, map[string]any{"channel": "api"}, resolved.Transaction.Metadata)
	require.Len(t, resolved.Transaction.Operations, 1)
	require.Equal(t, map[string]any{"purpose": "transfer"}, resolved.Transaction.Operations[0].Metadata)
	require.Equal(t, 1, fake.materializedCalls)

	fake.materialized = nil
	fake.materializedErr = redis.ErrEngineWriteBehindNotFound
	fake.materializedCalls = 0
	resolved, err = uc.ResolveEngineWriteBehindTransaction(ctx, uuid.MustParse(expected.OrganizationID), uuid.MustParse(expected.LedgerID), uuid.MustParse(expected.ID))
	require.NoError(t, err)
	require.Equal(t, EngineTransactionResolutionEvidence, resolved.Source)
	require.Equal(t, 1, fake.materializedCalls)
}

func TestResolveEngineWriteBehindTransactionUsesOnlyMatchingMaterializedExecution(t *testing.T) {
	index, envelope, receipt, expected := queryEngineWriteBehindFixture(t)
	payload, err := msgpack.Marshal(expected)
	require.NoError(t, err)
	fake := &engineWriteBehindRepositoryFake{
		index: index, envelope: envelope, receipt: receipt, materializeResult: true,
		materialized: &redis.EngineMaterializedTransaction{FormatVersion: 1, ExecutionID: expectedExecutionID(t), Payload: payload},
	}
	uc := &UseCase{EngineWriteBehindRepo: fake, EngineWriteBehindCodec: command.EngineWriteBehindEvidenceCodec{}}
	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-query")

	resolved, err := uc.ResolveEngineWriteBehindTransaction(ctx, uuid.MustParse(expected.OrganizationID), uuid.MustParse(expected.LedgerID), uuid.MustParse(expected.ID))
	require.NoError(t, err)
	require.Equal(t, EngineTransactionResolutionMaterialized, resolved.Source)
	require.Equal(t, expected.ID, resolved.Transaction.ID)
	require.Zero(t, fake.materializedCalls)

	fake.materialized.ExecutionID = uuid.New()
	resolved, err = uc.ResolveEngineWriteBehindTransaction(ctx, uuid.MustParse(expected.OrganizationID), uuid.MustParse(expected.LedgerID), uuid.MustParse(expected.ID))
	require.NoError(t, err)
	require.Equal(t, EngineTransactionResolutionEvidence, resolved.Source)
	require.Equal(t, 1, fake.materializedCalls)
}

func TestResolveEngineWriteBehindTransactionFailsClosedOnMissingEvidence(t *testing.T) {
	index, _, _, expected := queryEngineWriteBehindFixture(t)
	fake := &engineWriteBehindRepositoryFake{index: index, materializedErr: redis.ErrEngineWriteBehindNotFound, evidenceErr: redis.ErrEngineWriteBehindNotFound}
	uc := &UseCase{EngineWriteBehindRepo: fake, EngineWriteBehindCodec: command.EngineWriteBehindEvidenceCodec{}}
	ctx := tmcore.ContextWithTenantID(context.Background(), "tenant-query")

	resolved, err := uc.ResolveEngineWriteBehindTransaction(ctx, uuid.MustParse(expected.OrganizationID), uuid.MustParse(expected.LedgerID), uuid.MustParse(expected.ID))
	require.Nil(t, resolved)
	require.Error(t, err)
	require.ErrorIs(t, err, redis.ErrEngineWriteBehindNotFound)
}

func TestResolveEngineWriteBehindTransactionFallsBackToPrimaryWithoutIndex(t *testing.T) {
	ctrl := gomock.NewController(t)
	transactionRepo := postgres.NewMockRepository(ctrl)
	metadataRepo := mongodb.NewMockRepository(ctrl)
	organizationID, ledgerID, transactionID := uuid.New(), uuid.New(), uuid.New()
	expected := &postgres.Transaction{ID: transactionID.String(), OrganizationID: organizationID.String(), LedgerID: ledgerID.String()}
	fake := &engineWriteBehindRepositoryFake{indexErr: redis.ErrEngineWriteBehindNotFound}

	transactionRepo.EXPECT().
		FindWithOperations(gomock.Cond(func(ctx context.Context) bool { return readrouting.IsPrimaryRead(ctx) }), organizationID, ledgerID, transactionID).
		Return(expected, nil)
	metadataRepo.EXPECT().FindByEntity(gomock.Any(), constant.EntityTransaction, transactionID.String()).
		Return(&mongodb.Metadata{Data: map[string]any{"durable": true}}, nil)
	uc := &UseCase{EngineWriteBehindRepo: fake, EngineWriteBehindCodec: command.EngineWriteBehindEvidenceCodec{}, TransactionRepo: transactionRepo, TransactionMetadataRepo: metadataRepo}

	resolved, err := uc.ResolveEngineWriteBehindTransaction(context.Background(), organizationID, ledgerID, transactionID)
	require.NoError(t, err)
	require.Equal(t, EngineTransactionResolutionPrimary, resolved.Source)
	require.False(t, resolved.Pending)
	require.Equal(t, map[string]any{"durable": true}, resolved.Transaction.Metadata)
}

func TestResolveEngineWriteBehindTransactionFailsClosedOnIndexTransportError(t *testing.T) {
	ctrl := gomock.NewController(t)
	transactionRepo := postgres.NewMockRepository(ctrl)
	transportErr := errors.New("valkey unavailable")
	fake := &engineWriteBehindRepositoryFake{indexErr: transportErr}
	uc := &UseCase{
		EngineWriteBehindRepo:  fake,
		EngineWriteBehindCodec: command.EngineWriteBehindEvidenceCodec{},
		TransactionRepo:        transactionRepo,
	}

	resolved, err := uc.ResolveEngineWriteBehindTransaction(context.Background(), uuid.New(), uuid.New(), uuid.New())
	require.Nil(t, resolved)
	require.ErrorIs(t, err, transportErr)
}

func queryEngineWriteBehindFixture(t testing.TB) ([]byte, []byte, []byte, *postgres.Transaction) {
	t.Helper()
	organizationID := uuid.MustParse("11111111-1111-4111-8111-111111111111")
	ledgerID := uuid.MustParse("22222222-2222-4222-8222-222222222222")
	transactionID := uuid.MustParse("33333333-3333-4333-8333-333333333333")
	executionID := expectedExecutionID(t)
	balanceID := uuid.MustParse("55555555-5555-4555-8555-555555555555")
	accountID := uuid.MustParse("66666666-6666-4666-8666-666666666666")
	date := time.Date(2026, time.September, 17, 12, 0, 0, 0, time.UTC)
	routeID := "77777777-7777-4777-8777-777777777777"
	operation := command.OperationRecordSpec{
		TransactionID: transactionID, PostingRef: "source:0", BalanceRef: "@source#default", Role: accounting.RolePrimary,
		Side: command.OperationSpecSideFrom, RowType: constant.DEBIT, Direction: constant.DirectionDebit,
		RouteID: &routeID, Description: "frozen", ChartOfAccounts: "customer", Metadata: map[string]any{"purpose": "transfer"},
		Balance: command.OperationBalanceContext{
			ID: balanceID.String(), AccountID: accountID.String(), OrganizationID: organizationID.String(), LedgerID: ledgerID.String(),
			Alias: "@source", Key: "default", AssetCode: "USD", AccountType: "deposit", Direction: "credit",
			Available: decimal.NewFromInt(100), Version: 0,
		},
		RequestedAmount: decimal.NewFromInt(30), CompatibilityPath: command.OperationRecordStandard,
	}
	plan := command.TransactionCompletionPlan{
		FormatVersion: command.TransactionCompletionFormatVersion, TenantID: "tenant-query", HeaderID: "header-query",
		TransactionID: transactionID, OrganizationID: organizationID, LedgerID: ledgerID, ExecutionID: executionID,
		TTL: date.Add(time.Hour), TransactionStatus: constant.APPROVED, Action: constant.ActionDirect,
		TransactionDate: date, TransactionCreatedAt: date, TransactionUpdatedAt: date, OperationUpdatedAt: date,
		TransactionInput: mtransaction.Transaction{Description: "transfer", Metadata: map[string]any{"channel": "api"}, Send: mtransaction.Send{Asset: "USD", Value: decimal.NewFromInt(30)}},
		Validate:         &mtransaction.Responses{From: map[string]mtransaction.Amount{"@source": {Value: decimal.NewFromInt(30)}}},
		OperationSpecs:   []command.OperationRecordSpec{operation},
	}
	intent := command.EngineIntent{
		TenantID: plan.TenantID, OrganizationID: organizationID, LedgerID: ledgerID, ExecutionID: executionID,
		Transactions: []command.EngineTransactionIntent{{
			TransactionID: transactionID, Action: plan.Action, TransactionStatus: plan.TransactionStatus,
			TransactionDate: date, TransactionCreatedAt: date, TransactionUpdatedAt: date, OperationUpdatedAt: date,
			Input: plan.TransactionInput, PostingRefs: []string{"source:0"}, OperationSpecs: []command.OperationRecordIntent{operation.Intent()},
		}},
	}
	fingerprint, err := command.ComputeEngineIntentFingerprint(intent)
	require.NoError(t, err)
	plan.IntentFingerprint = fingerprint
	payload, err := command.EncodeTransactionCompletionPlan(plan)
	require.NoError(t, err)
	result := accounting.ExecutionResult{
		Movements: []accounting.Movement{{
			Ref: "movement:0", TransactionID: transactionID, PostingRef: "source:0", Role: accounting.RolePrimary,
			BalanceRef: "@source#default", Type: accounting.PostingDebit, Amount: decimal.NewFromInt(30),
			Before: accounting.BalanceState{Available: decimal.NewFromInt(100)}, After: accounting.BalanceState{Available: decimal.NewFromInt(70), Version: 1},
		}},
		Final: []accounting.BalanceSnapshot{{
			BalanceRef: "@source#default", ID: balanceID, AccountID: accountID, AccountType: "deposit", AssetCode: "USD",
			Alias: "@source", Key: "default", Direction: "credit", Available: decimal.NewFromInt(70), Version: 1,
		}},
	}
	record := command.TransactionCompletionRecord{
		FormatVersion: command.TransactionCompletionFormatVersion, TenantID: plan.TenantID,
		OrganizationID: organizationID, LedgerID: ledgerID, ExecutionID: executionID,
		IntentFingerprint: fingerprint, TransactionID: transactionID, Payload: string(payload), Result: result,
	}
	envelope, err := command.EncodeTransactionWriteBehindEnvelope(command.TransactionWriteBehindEnvelope{
		FormatVersion: command.TransactionWriteBehindFormatVersion, ApplicationState: command.TransactionApplicationConfirmed,
		ReplayState: command.TransactionReplayReconstructible, DurabilityState: command.TransactionDurabilityPending,
		Record: record, Dependencies: []command.TransactionEvidenceReference{},
	})
	require.NoError(t, err)
	index, err := command.EncodeTransactionEvidenceIndex(command.TransactionEvidenceIndex{
		FormatVersion: command.TransactionEvidenceIndexFormatVersion, TenantID: plan.TenantID,
		OrganizationID: organizationID, LedgerID: ledgerID, TransactionID: transactionID, ExecutionID: executionID,
		Action: plan.Action, ApplicationState: command.TransactionApplicationConfirmed,
		ReplayState: command.TransactionReplayReconstructible, DurabilityState: command.TransactionDurabilityPending,
		RecoveryField: transactionID.String() + ":" + executionID.String(), ReceiptField: executionID.String(),
		Dependencies: []command.TransactionEvidenceReference{},
	})
	require.NoError(t, err)
	receipt, err := json.Marshal(map[string]any{
		"formatVersion": 1, "tenantId": plan.TenantID, "organizationId": organizationID, "ledgerId": ledgerID,
		"executionId": executionID, "intentFingerprint": fingerprint, "response": `{"protocolVersion":1,"movements":[{}],"final":[{}]}`,
		"protection": map[string]any{
			"formatVersion": 2, "transactions": []uuid.UUID{transactionID},
			"recoveryFields": []string{transactionID.String() + ":" + executionID.String()}, "indexFields": []uuid.UUID{transactionID},
			"acknowledged": map[string]bool{}, "terminalCompletedAtMs": map[string]int64{},
		},
	})
	require.NoError(t, err)
	views, err := command.BuildTransactionEvidenceViews(record)
	require.NoError(t, err)

	return index, envelope, receipt, views.Lookup
}

func expectedExecutionID(t testing.TB) uuid.UUID {
	t.Helper()
	return uuid.MustParse("44444444-4444-4444-8444-444444444444")
}

var _ redis.EngineWriteBehindRepository = (*engineWriteBehindRepositoryFake)(nil)
