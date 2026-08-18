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

func TestCreateBalanceTransactionOperationsAsync_AnnotationRejectsFinancialEvidenceBeforeWrites(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name   string
		mutate func(*transaction.TransactionProcessingPayload)
	}{
		{name: "balance mutation", mutate: func(payload *transaction.TransactionProcessingPayload) {
			payload.Transaction.Operations[0].BalanceAffected = true
		}},
		{name: "nonzero operation amount", mutate: func(payload *transaction.TransactionProcessingPayload) {
			value := decimal.NewFromInt(1000)
			payload.Transaction.Operations[0].Amount.Value = &value
		}},
		{name: "balance evidence", mutate: func(payload *transaction.TransactionProcessingPayload) {
			payload.Balances = []*mmodel.Balance{{ID: uuid.NewString()}}
		}},
		{name: "outcome owner", mutate: func(payload *transaction.TransactionProcessingPayload) {
			payload.AttemptOwner = uuid.NewString()
			payload.ExpectedOutcome = mmodel.TransactionOutcomeCommitted
		}},
		{name: "partial discriminator", mutate: func(payload *transaction.TransactionProcessingPayload) {
			payload.EffectModeVersion = 0
		}},
		{name: "unknown discriminator", mutate: func(payload *transaction.TransactionProcessingPayload) {
			payload.EffectMode = "UNKNOWN"
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			organizationID := uuid.New()
			ledgerID := uuid.New()
			transactionID := uuid.New()
			zero := decimal.Zero
			informationalAmount := decimal.NewFromInt(1000)
			zeroVersion := int64(0)
			operationValue := &operation.Operation{
				ID: uuid.NewString(), TransactionID: transactionID.String(), Type: constant.DEBIT,
				AssetCode: "USD", Amount: operation.Amount{Value: &zero}, BalanceID: uuid.NewString(),
				BalanceKey: constant.DefaultBalanceKey, AccountID: uuid.NewString(), AccountAlias: "@annotation",
				OrganizationID: organizationID.String(), LedgerID: ledgerID.String(), BalanceAffected: false,
				Direction:    constant.DirectionDebit,
				Balance:      operation.Balance{Available: &zero, OnHold: &zero, Version: &zeroVersion},
				BalanceAfter: operation.Balance{Available: &zero, OnHold: &zero, Version: &zeroVersion},
				Snapshot:     mmodel.OperationSnapshot{OverdraftUsedBefore: "0", OverdraftUsedAfter: "0"},
			}
			payload := transaction.TransactionProcessingPayload{
				Transaction: &transaction.Transaction{
					ID: transactionID.String(), OrganizationID: organizationID.String(), LedgerID: ledgerID.String(),
					Amount: &informationalAmount, Status: transaction.Status{Code: constant.NOTED},
					Operations: []*operation.Operation{operationValue},
				},
				Input: &mtransaction.Transaction{Send: mtransaction.Send{Asset: "USD", Value: informationalAmount}},
				Validate: &mtransaction.Responses{Asset: "USD", From: map[string]mtransaction.Amount{
					"0#@annotation#default": {
						Asset: "USD", Value: decimal.NewFromInt(1000), Operation: constant.DEBIT,
						Direction: constant.DirectionDebit, TransactionType: constant.NOTED,
					},
				}},
				Version:           "v2",
				EffectModeVersion: mmodel.TransactionEffectModeVersion,
				EffectMode:        mmodel.TransactionEffectAnnotationOnly,
			}
			test.mutate(&payload)
			raw, err := msgpack.Marshal(payload)
			require.NoError(t, err)

			err = (&UseCase{}).CreateBalanceTransactionOperationsAsync(context.Background(), mmodel.Queue{
				OrganizationID: organizationID,
				LedgerID:       ledgerID,
				QueueData:      []mmodel.QueueData{{ID: transactionID, Value: raw}},
			})
			require.Error(t, err, "annotation evidence must fail before any repository can be called")
		})
	}
}

func TestValidateProcessingPayloadEffectMode_AnnotationPreservesInformationalAmountWithZeroBalanceEffect(t *testing.T) {
	t.Parallel()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()
	informationalAmount := decimal.NewFromInt(1000)
	zero := decimal.Zero
	zeroVersion := int64(0)
	payload := transaction.TransactionProcessingPayload{
		Transaction: &transaction.Transaction{
			ID: transactionID.String(), OrganizationID: organizationID.String(), LedgerID: ledgerID.String(),
			Amount: &informationalAmount, Status: transaction.Status{Code: constant.NOTED},
			Operations: []*operation.Operation{{
				ID: uuid.NewString(), TransactionID: transactionID.String(), Type: constant.DEBIT,
				AssetCode: "USD", Amount: operation.Amount{Value: &zero}, BalanceID: uuid.NewString(),
				BalanceKey: constant.DefaultBalanceKey, AccountID: uuid.NewString(), AccountAlias: "@annotation",
				OrganizationID: organizationID.String(), LedgerID: ledgerID.String(), Direction: constant.DirectionDebit,
				Balance:      operation.Balance{Available: &zero, OnHold: &zero, Version: &zeroVersion},
				BalanceAfter: operation.Balance{Available: &zero, OnHold: &zero, Version: &zeroVersion},
				Snapshot:     mmodel.OperationSnapshot{OverdraftUsedBefore: "0", OverdraftUsedAfter: "0"},
			}},
		},
		Input: &mtransaction.Transaction{Send: mtransaction.Send{Asset: "USD", Value: informationalAmount}},
		Validate: &mtransaction.Responses{Asset: "USD", From: map[string]mtransaction.Amount{
			"0#@annotation#default": {
				Asset: "USD", Value: informationalAmount, Operation: constant.DEBIT,
				Direction: constant.DirectionDebit, TransactionType: constant.NOTED,
			},
		}},
		EffectModeVersion: mmodel.TransactionEffectModeVersion,
		EffectMode:        mmodel.TransactionEffectAnnotationOnly,
	}

	mode, err := validateProcessingPayloadEffectMode(organizationID, ledgerID, &payload)
	require.NoError(t, err)
	require.Equal(t, mmodel.TransactionEffectAnnotationOnly, mode)
	require.True(t, payload.Transaction.Amount.Equal(informationalAmount))
	require.True(t, payload.Transaction.Operations[0].Amount.Value.IsZero())

	divergentAmount := decimal.NewFromInt(999)
	payload.Transaction.Amount = &divergentAmount
	_, err = validateProcessingPayloadEffectMode(organizationID, ledgerID, &payload)
	require.ErrorContains(t, err,
		"informational amount differs")
}

func TestPreflightOutcomeBackedTransaction_AnnotationAdoptsOnlyCanonicalOperationIdentity(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name      string
		mutate    func(*mmodel.OperationRedis)
		wantErr   string
		wantAdopt bool
	}{
		{
			name: "lost acknowledgement redelivery adopts the single-assignment id",
			mutate: func(operation *mmodel.OperationRedis) {
				operation.ID = uuid.NewString()
			},
			wantAdopt: true,
		},
		{
			name: "canonical row with a different direction is rejected",
			mutate: func(operation *mmodel.OperationRedis) {
				operation.ID = uuid.NewString()
				operation.Direction = constant.DirectionCredit
			},
			wantErr: "operation effect differs",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			redisRepo := transactionredis.NewMockRedisRepository(ctrl)
			organizationID := uuid.New()
			ledgerID := uuid.New()
			transactionID := uuid.New()
			payload := completeAnnotationPersistencePayload(organizationID, ledgerID, transactionID)
			candidateID := payload.Transaction.Operations[0].ID
			canonical := payload.Transaction.Operations[0].ToRedis()
			test.mutate(&canonical)

			enrichment := redisRepo.EXPECT().EnrichTransactionBackup(gomock.Any(), organizationID, ledgerID, transactionID,
				gomock.Any(), constant.ActionDirect, nil).
				Return([]mmodel.OperationRedis{canonical}, nil, false, nil)
			if test.wantAdopt {
				enrichment.Times(2)
			}
			uc := &UseCase{TransactionRedisRepo: redisRepo}

			outcomeBacked, terminal, err := uc.preflightOutcomeBackedTransaction(
				context.Background(), organizationID, ledgerID, &payload,
			)
			require.False(t, outcomeBacked)
			require.False(t, terminal)
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
				require.Equal(t, candidateID, payload.Transaction.Operations[0].ID)

				return
			}
			require.NoError(t, err)
			if test.wantAdopt {
				require.NotEqual(t, candidateID, canonical.ID)
				require.Equal(t, canonical.ID, payload.Transaction.Operations[0].ID)

				redeliveredOperation := *payload.Transaction.Operations[0]
				redeliveredOperation.ID = uuid.NewString()
				redeliveredTransaction := *payload.Transaction
				redeliveredTransaction.Operations = []*operation.Operation{&redeliveredOperation}
				redelivered := payload
				redelivered.Transaction = &redeliveredTransaction
				outcomeBacked, terminal, err = uc.preflightOutcomeBackedTransaction(
					context.Background(), organizationID, ledgerID, &redelivered,
				)
				require.NoError(t, err)
				require.False(t, outcomeBacked)
				require.False(t, terminal)
				require.Equal(t, canonical.ID, redelivered.Transaction.Operations[0].ID)
			}
		})
	}
}

func completeAnnotationPersistencePayload(
	organizationID, ledgerID, transactionID uuid.UUID,
) transaction.TransactionProcessingPayload {
	informationalAmount := decimal.NewFromInt(1000)
	zero := decimal.Zero
	zeroVersion := int64(0)

	return transaction.TransactionProcessingPayload{
		Transaction: &transaction.Transaction{
			ID: transactionID.String(), OrganizationID: organizationID.String(), LedgerID: ledgerID.String(),
			Amount: &informationalAmount, Status: transaction.Status{Code: constant.NOTED},
			Operations: []*operation.Operation{{
				ID: uuid.NewString(), TransactionID: transactionID.String(), Type: constant.DEBIT,
				AssetCode: "USD", Amount: operation.Amount{Value: &zero}, BalanceID: uuid.NewString(),
				BalanceKey: constant.DefaultBalanceKey, AccountID: uuid.NewString(), AccountAlias: "@annotation",
				OrganizationID: organizationID.String(), LedgerID: ledgerID.String(), Direction: constant.DirectionDebit,
				Balance:      operation.Balance{Available: &zero, OnHold: &zero, Version: &zeroVersion},
				BalanceAfter: operation.Balance{Available: &zero, OnHold: &zero, Version: &zeroVersion},
				Snapshot:     mmodel.OperationSnapshot{OverdraftUsedBefore: "0", OverdraftUsedAfter: "0"},
			}},
		},
		Input: &mtransaction.Transaction{Send: mtransaction.Send{Asset: "USD", Value: informationalAmount}},
		Validate: &mtransaction.Responses{Asset: "USD", From: map[string]mtransaction.Amount{
			"0#@annotation#default": {
				Asset: "USD", Value: informationalAmount, Operation: constant.DEBIT,
				Direction: constant.DirectionDebit, TransactionType: constant.NOTED,
			},
		}},
		Version:           "v2",
		EffectModeVersion: mmodel.TransactionEffectModeVersion,
		EffectMode:        mmodel.TransactionEffectAnnotationOnly,
	}
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
