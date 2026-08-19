// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/repository"
)

type orderedAtomicTx struct {
	events      *[]string
	commitErr   error
	rollbackErr error
}

func (tx *orderedAtomicTx) ExecContext(context.Context, string, ...any) (sql.Result, error) {
	return nil, errors.New("unexpected direct execution")
}

func (tx *orderedAtomicTx) Commit() error {
	*tx.events = append(*tx.events, "commit")

	return tx.commitErr
}

func (tx *orderedAtomicTx) Rollback() error {
	*tx.events = append(*tx.events, "rollback")

	return tx.rollbackErr
}

func TestPersistTransactionAndOperationsAtomic_FreshFeeExpandedTerminalCommitsTogether(t *testing.T) {
	ctrl := gomock.NewController(t)
	txRepo := transaction.NewMockRepository(ctrl)
	opRepo := operation.NewMockRepository(ctrl)
	uc := &UseCase{TransactionRepo: txRepo, OperationRepo: opRepo}

	events := make([]string, 0, 4)
	dbTx := &orderedAtomicTx{events: &events}
	payload := atomicPersistencePayload(t, constant.CREATED, false, 4)

	txRepo.EXPECT().BeginTx(gomock.Any()).DoAndReturn(func(context.Context) (repository.DBTransaction, error) {
		events = append(events, "begin")

		return dbTx, nil
	})
	txRepo.EXPECT().CreateBulkTx(gomock.Any(), dbTx, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ repository.DBExecutor, transactions []*transaction.Transaction) (*repository.BulkInsertResult, error) {
			events = append(events, "transaction")
			require.Len(t, transactions, 1)
			require.Equal(t, constant.APPROVED, transactions[0].Status.Code)

			return &repository.BulkInsertResult{Attempted: 1, Inserted: 1, InsertedIDs: []string{transactions[0].ID}}, nil
		})
	opRepo.EXPECT().CreateBulkTx(gomock.Any(), dbTx, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ repository.DBExecutor, operations []*operation.Operation) (*repository.BulkInsertResult, error) {
			events = append(events, "operations")
			require.Len(t, operations, 4, "fee-expanded terminal transaction must persist every economic leg")

			return &repository.BulkInsertResult{Attempted: 4, Inserted: 4}, nil
		})

	persisted, phase, err := uc.persistTransactionAndOperationsAtomic(context.Background(), payload)
	require.NoError(t, err)
	require.Equal(t, constant.APPROVED, persisted.Status.Code)
	require.Equal(t, TransactionLifecyclePhaseCreated, phase)
	require.Equal(t, []string{"begin", "transaction", "operations", "commit"}, events)
}

func TestPersistTransactionAndOperationsAtomic_OperationFailureRollsBackTerminalTransaction(t *testing.T) {
	ctrl := gomock.NewController(t)
	txRepo := transaction.NewMockRepository(ctrl)
	opRepo := operation.NewMockRepository(ctrl)
	uc := &UseCase{TransactionRepo: txRepo, OperationRepo: opRepo}

	events := make([]string, 0, 4)
	dbTx := &orderedAtomicTx{events: &events}
	payload := atomicPersistencePayload(t, constant.CREATED, false, 2)
	opErr := errors.New("operation persistence failed")

	txRepo.EXPECT().BeginTx(gomock.Any()).Return(dbTx, nil)
	txRepo.EXPECT().CreateBulkTx(gomock.Any(), dbTx, gomock.Any()).Return(
		&repository.BulkInsertResult{Attempted: 1, Inserted: 1, InsertedIDs: []string{payload.Transaction.ID}}, nil)
	opRepo.EXPECT().CreateBulkTx(gomock.Any(), dbTx, gomock.Any()).Return(nil, opErr)

	_, _, err := uc.persistTransactionAndOperationsAtomic(context.Background(), payload)
	require.ErrorIs(t, err, opErr)
	require.Equal(t, []string{"rollback"}, events)
}

func TestPersistTransactionAndOperationsAtomic_LostACKRedeliveryIsOneAtomicNoop(t *testing.T) {
	ctrl := gomock.NewController(t)
	txRepo := transaction.NewMockRepository(ctrl)
	opRepo := operation.NewMockRepository(ctrl)
	uc := &UseCase{TransactionRepo: txRepo, OperationRepo: opRepo}

	events := make([]string, 0, 2)
	dbTx := &orderedAtomicTx{events: &events}
	payload := atomicPersistencePayload(t, constant.APPROVED, false, 4)

	txRepo.EXPECT().BeginTx(gomock.Any()).Return(dbTx, nil)
	txRepo.EXPECT().CreateBulkTx(gomock.Any(), dbTx, gomock.Any()).Return(
		&repository.BulkInsertResult{Attempted: 1, Ignored: 1}, nil)
	opRepo.EXPECT().CreateBulkTx(gomock.Any(), dbTx, gomock.Any()).Return(
		&repository.BulkInsertResult{Attempted: 4, Ignored: 4}, nil)

	_, phase, err := uc.persistTransactionAndOperationsAtomic(context.Background(), payload)
	require.NoError(t, err)
	require.Equal(t, TransactionLifecyclePhaseNoop, phase)
	require.Equal(t, []string{"commit"}, events)
}

func TestPersistTransactionAndOperationsAtomic_PendingTerminalTransitionAndOperationsCommitTogether(t *testing.T) {
	ctrl := gomock.NewController(t)
	txRepo := transaction.NewMockRepository(ctrl)
	opRepo := operation.NewMockRepository(ctrl)
	uc := &UseCase{TransactionRepo: txRepo, OperationRepo: opRepo}

	events := make([]string, 0, 4)
	dbTx := &orderedAtomicTx{events: &events}
	payload := atomicPersistencePayload(t, constant.APPROVED, true, 2)

	txRepo.EXPECT().BeginTx(gomock.Any()).Return(dbTx, nil)
	txRepo.EXPECT().CreateBulkTx(gomock.Any(), dbTx, gomock.Any()).Return(
		&repository.BulkInsertResult{Attempted: 1, Ignored: 1}, nil)
	opRepo.EXPECT().CreateBulkTx(gomock.Any(), dbTx, gomock.Any()).DoAndReturn(
		func(_ context.Context, _ repository.DBExecutor, _ []*operation.Operation) (*repository.BulkInsertResult, error) {
			events = append(events, "operations")

			return &repository.BulkInsertResult{Attempted: 2, Inserted: 2}, nil
		})
	txRepo.EXPECT().UpdateStatusFromPendingTx(gomock.Any(), dbTx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ repository.DBExecutor, _, _, _ uuid.UUID, tx *transaction.Transaction) (*transaction.Transaction, error) {
			events = append(events, "terminal")
			require.Equal(t, constant.APPROVED, tx.Status.Code)

			return tx, nil
		})

	_, phase, err := uc.persistTransactionAndOperationsAtomic(context.Background(), payload)
	require.NoError(t, err)
	require.Equal(t, TransactionLifecyclePhaseUpdated, phase)
	require.Equal(t, []string{"operations", "terminal", "commit"}, events)
}

func TestPerformBulkInsertAndUpdate_MixedFreshAndPendingTransactionsShareOneCommit(t *testing.T) {
	ctrl := gomock.NewController(t)
	txRepo := transaction.NewMockRepository(ctrl)
	opRepo := operation.NewMockRepository(ctrl)
	uc := &UseCase{TransactionRepo: txRepo, OperationRepo: opRepo}

	events := make([]string, 0, 5)
	dbTx := &orderedAtomicTx{events: &events}
	fresh := atomicPersistencePayload(t, constant.APPROVED, false, 2)
	terminal := atomicPersistencePayload(t, constant.APPROVED, true, 2)
	entities := &bulkInsertEntities{
		transactions: []*transaction.Transaction{fresh.Transaction},
		operations: append(append([]*operation.Operation{}, fresh.Transaction.Operations...),
			terminal.Transaction.Operations...),
	}
	updates := &bulkUpdateEntities{transactions: []*transaction.Transaction{terminal.Transaction}}
	result := &BulkResult{InsertedTransactionIDs: make(map[string]struct{})}

	txRepo.EXPECT().BeginTx(gomock.Any()).DoAndReturn(func(context.Context) (repository.DBTransaction, error) {
		events = append(events, "begin")

		return dbTx, nil
	})
	txRepo.EXPECT().CreateBulkTx(gomock.Any(), dbTx, entities.transactions).DoAndReturn(
		func(_ context.Context, _ repository.DBExecutor, transactions []*transaction.Transaction) (*repository.BulkInsertResult, error) {
			events = append(events, "fresh")

			return &repository.BulkInsertResult{Attempted: 1, Inserted: 1, InsertedIDs: []string{transactions[0].ID}}, nil
		})
	opRepo.EXPECT().CreateBulkTx(gomock.Any(), dbTx, entities.operations).DoAndReturn(
		func(_ context.Context, _ repository.DBExecutor, operations []*operation.Operation) (*repository.BulkInsertResult, error) {
			events = append(events, "operations")

			return &repository.BulkInsertResult{Attempted: int64(len(operations)), Inserted: int64(len(operations))}, nil
		})
	txRepo.EXPECT().UpdateStatusFromPendingTx(gomock.Any(), dbTx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, _ repository.DBExecutor, _, _, _ uuid.UUID, tx *transaction.Transaction) (*transaction.Transaction, error) {
			events = append(events, "terminal")

			return tx, nil
		})

	err := uc.performBulkInsertAndUpdate(context.Background(), &MockLogger{}, entities, updates, result)
	require.NoError(t, err)
	require.Equal(t, int64(1), result.TransactionsInserted)
	require.Equal(t, int64(1), result.TransactionsUpdated)
	require.Equal(t, int64(4), result.OperationsInserted)
	require.Equal(t, []string{"begin", "fresh", "operations", "terminal", "commit"}, events)
}

func TestPerformBulkInsertAndUpdate_TerminalFailureRollsBackOperations(t *testing.T) {
	ctrl := gomock.NewController(t)
	txRepo := transaction.NewMockRepository(ctrl)
	opRepo := operation.NewMockRepository(ctrl)
	uc := &UseCase{TransactionRepo: txRepo, OperationRepo: opRepo}

	events := make([]string, 0, 4)
	dbTx := &orderedAtomicTx{events: &events}
	terminal := atomicPersistencePayload(t, constant.APPROVED, true, 4)
	entities := &bulkInsertEntities{operations: terminal.Transaction.Operations}
	updates := &bulkUpdateEntities{transactions: []*transaction.Transaction{terminal.Transaction}}
	result := &BulkResult{InsertedTransactionIDs: make(map[string]struct{})}
	statusErr := errors.New("terminal compare-and-set failed")

	txRepo.EXPECT().BeginTx(gomock.Any()).Return(dbTx, nil)
	opRepo.EXPECT().CreateBulkTx(gomock.Any(), dbTx, entities.operations).Return(
		&repository.BulkInsertResult{Attempted: 4, Inserted: 4}, nil)
	txRepo.EXPECT().UpdateStatusFromPendingTx(gomock.Any(), dbTx, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, statusErr)

	err := uc.performBulkInsertAndUpdate(context.Background(), &MockLogger{}, entities, updates, result)
	require.ErrorIs(t, err, statusErr)
	require.Equal(t, []string{"rollback"}, events)
}

func atomicPersistencePayload(t *testing.T, status string, pending bool, operationCount int) transaction.TransactionProcessingPayload {
	t.Helper()

	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()
	amount := decimal.NewFromInt(10)
	operations := make([]*operation.Operation, 0, operationCount)
	for range operationCount {
		operations = append(operations, &operation.Operation{
			ID:             uuid.NewString(),
			TransactionID:  transactionID.String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      uuid.NewString(),
			AssetCode:      "USD",
			Amount:         operation.Amount{Value: &amount},
			Direction:      "DEBIT",
		})
	}

	return transaction.TransactionProcessingPayload{
		Transaction: &transaction.Transaction{
			ID:             transactionID.String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Status:         transaction.Status{Code: status},
			Operations:     operations,
		},
		Validate: &mtransaction.Responses{Pending: pending},
		Input:    &mtransaction.Transaction{},
		Version:  "v2",
	}
}
