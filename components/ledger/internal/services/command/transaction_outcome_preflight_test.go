// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	transactionredis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
)

func TestCreateBalanceTransactionOperationsAsync_OutcomeWithoutGenerationPreflightsBeforeWrites(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		mutate func([]mmodel.OperationRedis, []mmodel.BalanceRedis) ([]mmodel.OperationRedis, []mmodel.BalanceRedis)
	}{
		{
			name: "different operation amount",
			mutate: func(operations []mmodel.OperationRedis, balances []mmodel.BalanceRedis) ([]mmodel.OperationRedis, []mmodel.BalanceRedis) {
				operations[0].AmountValue = operations[0].AmountValue.Add(decimal.NewFromInt(1))

				return operations, balances
			},
		},
		{
			name: "different operation tenant",
			mutate: func(operations []mmodel.OperationRedis, balances []mmodel.BalanceRedis) ([]mmodel.OperationRedis, []mmodel.BalanceRedis) {
				operations[0].OrganizationID = uuid.NewString()

				return operations, balances
			},
		},
		{
			name: "different balance",
			mutate: func(operations []mmodel.OperationRedis, balances []mmodel.BalanceRedis) ([]mmodel.OperationRedis, []mmodel.BalanceRedis) {
				balances[0].Version++

				return operations, balances
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			transactionRepo := transaction.NewMockRepository(ctrl)
			operationRepo := operation.NewMockRepository(ctrl)
			balanceRepo := balance.NewMockRepository(ctrl)
			redisRepo := transactionredis.NewMockRedisRepository(ctrl)
			organizationID := uuid.New()
			ledgerID := uuid.New()
			transactionID := uuid.New()
			operationValue, balanceAfter := completeOutcomeEvidence(organizationID, ledgerID, transactionID)
			canonicalOperations, canonicalBalances := test.mutate(
				[]mmodel.OperationRedis{operationValue.ToRedis()},
				[]mmodel.BalanceRedis{balanceAfter.ToRedis()},
			)
			redisRepo.EXPECT().EnrichTransactionBackup(gomock.Any(), organizationID, ledgerID, transactionID,
				gomock.Any(), constant.ActionCommit, gomock.Any()).
				Return(canonicalOperations, canonicalBalances, false, nil)

			payload := transaction.TransactionProcessingPayload{
				Transaction: &transaction.Transaction{
					ID: transactionID.String(), OrganizationID: organizationID.String(), LedgerID: ledgerID.String(),
					Status: transaction.Status{Code: constant.APPROVED}, Operations: []*operation.Operation{operationValue},
				},
				Validate: &mtransaction.Responses{}, Version: "v2", BalancesAfter: []*mmodel.Balance{balanceAfter},
				AttemptOwner: uuid.NewString(), ExpectedOutcome: mmodel.TransactionOutcomeCommitted,
			}
			raw, err := msgpack.Marshal(payload)
			require.NoError(t, err)
			uc := &UseCase{
				TransactionRepo: transactionRepo, OperationRepo: operationRepo,
				BalanceRepo: balanceRepo, TransactionRedisRepo: redisRepo,
			}

			err = uc.CreateBalanceTransactionOperationsAsync(context.Background(), mmodel.Queue{
				OrganizationID: organizationID, LedgerID: ledgerID,
				QueueData: []mmodel.QueueData{{ID: uuid.New(), Value: raw}},
			})
			require.ErrorContains(t, err, "authoritative Redis envelope")
		})
	}
}

func TestPreflightDurableBulkPayloads_OutcomeWithoutGenerationRejectsDivergenceBeforeWrites(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	transactionRepo := transaction.NewMockRepository(ctrl)
	operationRepo := operation.NewMockRepository(ctrl)
	balanceRepo := balance.NewMockRepository(ctrl)
	redisRepo := transactionredis.NewMockRedisRepository(ctrl)
	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()
	operationValue, balanceAfter := completeOutcomeEvidence(organizationID, ledgerID, transactionID)
	canonicalBalance := balanceAfter.ToRedis()
	canonicalBalance.Direction = constant.DirectionCredit
	redisRepo.EXPECT().EnrichTransactionBackup(gomock.Any(), organizationID, ledgerID, transactionID,
		gomock.Any(), constant.ActionCommit, gomock.Any()).
		Return([]mmodel.OperationRedis{operationValue.ToRedis()}, []mmodel.BalanceRedis{canonicalBalance}, false, nil)

	payload := transaction.TransactionProcessingPayload{
		Transaction: &transaction.Transaction{
			ID: transactionID.String(), OrganizationID: organizationID.String(), LedgerID: ledgerID.String(),
			Status: transaction.Status{Code: constant.APPROVED}, Operations: []*operation.Operation{operationValue},
		},
		Validate: &mtransaction.Responses{}, Version: "v2", BalancesAfter: []*mmodel.Balance{balanceAfter},
		AttemptOwner: uuid.NewString(), ExpectedOutcome: mmodel.TransactionOutcomeCommitted,
	}
	uc := &UseCase{
		TransactionRepo: transactionRepo, OperationRepo: operationRepo,
		BalanceRepo: balanceRepo, TransactionRedisRepo: redisRepo,
	}

	result, err := uc.CreateBulkTransactionOperationsAsync(context.Background(), []transaction.TransactionProcessingPayload{payload})
	require.ErrorContains(t, err, "authoritative Redis envelope")
	require.NotNil(t, result)
	require.Zero(t, result.TransactionsAttempted)
	require.Zero(t, result.OperationsAttempted)
}

func TestCreateBalanceTransactionOperationsAsync_OutcomeWithoutGenerationLostAckIsExactReadOnlyReplay(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	transactionRepo := transaction.NewMockRepository(ctrl)
	operationRepo := operation.NewMockRepository(ctrl)
	balanceRepo := balance.NewMockRepository(ctrl)
	redisRepo := transactionredis.NewMockRedisRepository(ctrl)
	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()
	economicOperation, balanceAfter := completeOutcomeEvidence(organizationID, ledgerID, transactionID)
	canonicalOperations := []mmodel.OperationRedis{economicOperation.ToRedis()}
	canonicalBalances := []mmodel.BalanceRedis{balanceAfter.ToRedis()}
	redisRepo.EXPECT().EnrichTransactionBackup(gomock.Any(), organizationID, ledgerID, transactionID,
		gomock.Any(), constant.ActionCommit, gomock.Any()).
		Return(canonicalOperations, canonicalBalances, true, nil).
		Times(2)

	persisted := &transaction.Transaction{
		ID: transactionID.String(), OrganizationID: organizationID.String(), LedgerID: ledgerID.String(),
		Status: transaction.Status{Code: constant.APPROVED}, Operations: []*operation.Operation{economicOperation},
	}
	transactionRepo.EXPECT().FindWithOperations(gomock.Any(), organizationID, ledgerID, transactionID).
		Return(persisted, nil)
	redisRepo.EXPECT().FinalizeTransactionPersistence(gomock.Any(), organizationID, ledgerID, transactionID,
		gomock.Any(), gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _, id uuid.UUID, _ mmodel.BalanceExecutionAttempt,
			operations []mmodel.OperationRedis, balances []mmodel.BalanceRedis) error {
			require.True(t, sameRedisEconomicOperationMultiset(
				organizationID, ledgerID, id, canonicalOperations, operations,
			))
			require.True(t, mmodel.RedisBalanceSetEconomicEqual(canonicalBalances, balances))

			return nil
		})
	payload := transaction.TransactionProcessingPayload{
		Transaction: persisted, Validate: &mtransaction.Responses{}, Version: "v2",
		BalancesAfter: []*mmodel.Balance{balanceAfter}, AttemptOwner: uuid.NewString(),
		ExpectedOutcome: mmodel.TransactionOutcomeCommitted,
	}
	raw, err := msgpack.Marshal(payload)
	require.NoError(t, err)
	uc := &UseCase{
		TransactionRepo: transactionRepo, OperationRepo: operationRepo,
		BalanceRepo: balanceRepo, TransactionRedisRepo: redisRepo,
	}

	err = uc.CreateBalanceTransactionOperationsAsync(context.Background(), mmodel.Queue{
		OrganizationID: organizationID, LedgerID: ledgerID,
		QueueData: []mmodel.QueueData{{ID: uuid.New(), Value: raw}},
	})
	require.NoError(t, err)
}

func TestFinalizeDurableTransactionPersistence_OutcomeWithoutGenerationRejectsDivergenceBeforePrimaryWrite(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	transactionRepo := transaction.NewMockRepository(ctrl)
	redisRepo := transactionredis.NewMockRedisRepository(ctrl)
	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()
	economicOperation, balanceAfter := completeOutcomeEvidence(organizationID, ledgerID, transactionID)
	divergent := economicOperation.ToRedis()
	divergent.BalanceAfterVersion++
	redisRepo.EXPECT().EnrichTransactionBackup(gomock.Any(), organizationID, ledgerID, transactionID,
		gomock.Any(), constant.ActionCancel, gomock.Any()).
		Return([]mmodel.OperationRedis{divergent}, []mmodel.BalanceRedis{balanceAfter.ToRedis()}, false, nil)
	payload := transaction.TransactionProcessingPayload{
		Transaction: &transaction.Transaction{
			ID: transactionID.String(), OrganizationID: organizationID.String(), LedgerID: ledgerID.String(),
			Status: transaction.Status{Code: constant.CANCELED}, Operations: []*operation.Operation{economicOperation},
		},
		BalancesAfter: []*mmodel.Balance{balanceAfter}, AttemptOwner: uuid.NewString(),
		ExpectedOutcome: mmodel.TransactionOutcomeAborted,
	}
	uc := &UseCase{TransactionRepo: transactionRepo, TransactionRedisRepo: redisRepo}

	managed, err := uc.FinalizeDurableTransactionPersistence(context.Background(), organizationID, ledgerID, payload)
	require.True(t, managed)
	require.ErrorContains(t, err, "authoritative Redis envelope")
}

func TestFinalizeDurableTransactionPersistence_OutcomeWithoutGenerationRejectsNonterminalOutcomeBeforeReads(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()
	economicOperation, balanceAfter := completeOutcomeEvidence(organizationID, ledgerID, transactionID)
	payload := transaction.TransactionProcessingPayload{
		Transaction: &transaction.Transaction{
			ID: transactionID.String(), OrganizationID: organizationID.String(), LedgerID: ledgerID.String(),
			Status: transaction.Status{Code: constant.APPROVED}, Operations: []*operation.Operation{economicOperation},
		},
		BalancesAfter: []*mmodel.Balance{balanceAfter}, AttemptOwner: uuid.NewString(),
		ExpectedOutcome: constant.PENDING,
	}
	uc := &UseCase{}

	managed, err := uc.FinalizeDurableTransactionPersistence(context.Background(), organizationID, ledgerID, payload)
	require.True(t, managed)
	require.ErrorContains(t, err, "outcome is not terminal")
}

func completeOutcomeEvidence(
	organizationID, ledgerID, transactionID uuid.UUID,
) (*operation.Operation, *mmodel.Balance) {
	amount := decimal.NewFromInt(10)
	beforeAvailable := decimal.NewFromInt(100)
	afterAvailable := decimal.NewFromInt(90)
	onHold := decimal.Zero
	beforeVersion := int64(3)
	afterVersion := int64(4)
	balanceID := uuid.NewString()
	accountID := uuid.NewString()
	balanceAfter := &mmodel.Balance{
		ID: balanceID, Alias: "@source", Key: constant.DefaultBalanceKey, AccountID: accountID,
		AssetCode: "USD", Available: afterAvailable, OnHold: onHold, Version: afterVersion,
		AccountType: "deposit", AllowSending: true, AllowReceiving: true,
		Direction: constant.DirectionDebit, OverdraftUsed: decimal.Zero,
	}
	operationValue := &operation.Operation{
		ID: uuid.NewString(), TransactionID: transactionID.String(), Type: constant.DEBIT,
		AssetCode: "USD", Amount: operation.Amount{Value: &amount}, BalanceID: balanceID,
		BalanceKey: constant.DefaultBalanceKey, AccountID: accountID,
		OrganizationID: organizationID.String(), LedgerID: ledgerID.String(),
		BalanceAffected: true, Direction: constant.DirectionDebit,
		Balance: operation.Balance{
			Available: &beforeAvailable, OnHold: &onHold, Version: &beforeVersion,
		},
		BalanceAfter: operation.Balance{
			Available: &afterAvailable, OnHold: &onHold, Version: &afterVersion,
		},
		Snapshot: mmodel.OperationSnapshot{OverdraftUsedBefore: "0", OverdraftUsedAfter: "0"},
	}

	return operationValue, balanceAfter
}
