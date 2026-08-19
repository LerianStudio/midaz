//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/pkg/readrouting"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/repository"
	postgrestestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
)

type commitBarrierTransactionRepository struct {
	transaction.Repository
	commitReady chan struct{}
	allowCommit chan struct{}
}

func (repo *commitBarrierTransactionRepository) BeginTx(ctx context.Context) (repository.DBTransaction, error) {
	dbTx, err := repo.Repository.BeginTx(ctx)
	if err != nil {
		return nil, err
	}

	return &commitBarrierDBTransaction{
		DBTransaction: dbTx,
		commitReady:   repo.commitReady,
		allowCommit:   repo.allowCommit,
	}, nil
}

type commitBarrierDBTransaction struct {
	repository.DBTransaction
	commitReady chan struct{}
	allowCommit chan struct{}
}

func (tx *commitBarrierDBTransaction) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	querier, ok := tx.DBTransaction.(repository.DBQuerier)
	if !ok {
		return nil, repository.ErrQueryContextNotSupported
	}

	return querier.QueryContext(ctx, query, args...)
}

func (tx *commitBarrierDBTransaction) Commit() error {
	close(tx.commitReady)
	<-tx.allowCommit

	return tx.DBTransaction.Commit()
}

type failingAtomicOperationRepository struct {
	operation.Repository
	err error
}

func (repo *failingAtomicOperationRepository) CreateBulkTx(
	context.Context,
	repository.DBExecutor,
	[]*operation.Operation,
) (*repository.BulkInsertResult, error) {
	return nil, repo.err
}

func TestIntegration_AsyncTerminalVisibilityIsAtomic(t *testing.T) {
	pgContainer := postgrestestutil.SetupMigratedContainer(t, "transaction")
	connStr := postgrestestutil.BuildConnectionString(pgContainer.Host, pgContainer.Port, pgContainer.Config)
	pgConn := postgrestestutil.ConnectPostgresClient(t, connStr, connStr)
	baseTransactionRepo := transaction.NewTransactionPostgreSQLRepository(pgConn)
	baseOperationRepo := operation.NewOperationPostgreSQLRepository(pgConn)

	t.Run("individual fee-expanded terminal and redelivery", func(t *testing.T) {
		payload := integrationAtomicPayload(constant.CREATED, false)
		barrierRepo := &commitBarrierTransactionRepository{
			Repository:  baseTransactionRepo,
			commitReady: make(chan struct{}),
			allowCommit: make(chan struct{}),
		}
		uc := &UseCase{TransactionRepo: barrierRepo, OperationRepo: baseOperationRepo}

		type result struct {
			phase string
			err   error
		}
		done := make(chan result, 1)
		go func() {
			_, phase, err := uc.persistTransactionAndOperationsAtomic(context.Background(), payload)
			done <- result{phase: phase, err: err}
		}()

		select {
		case <-barrierRepo.commitReady:
		case early := <-done:
			t.Fatalf("individual persistence returned before commit barrier: phase=%s err=%v", early.phase, early.err)
		case <-time.After(10 * time.Second):
			t.Fatal("individual persistence did not reach its commit boundary")
		}
		requireTransactionSnapshot(t, pgContainer.DB, payload.Transaction.ID, "", 0)
		requireRoutedTransactionSnapshots(t, baseTransactionRepo, payload, "", 0)
		close(barrierRepo.allowCommit)

		persisted := <-done
		require.NoError(t, persisted.err)
		require.Equal(t, TransactionLifecyclePhaseCreated, persisted.phase)
		requireTransactionSnapshot(t, pgContainer.DB, payload.Transaction.ID, constant.APPROVED, 4)
		requireRoutedTransactionSnapshots(t, baseTransactionRepo, payload, constant.APPROVED, 4)

		// A broker lost-ACK redelivery is an exact no-op: no operation is
		// duplicated and the terminal multiset stays complete.
		replayUC := &UseCase{TransactionRepo: baseTransactionRepo, OperationRepo: baseOperationRepo}
		_, phase, err := replayUC.persistTransactionAndOperationsAtomic(context.Background(), payload)
		require.NoError(t, err)
		require.Equal(t, TransactionLifecyclePhaseNoop, phase)
		requireTransactionSnapshot(t, pgContainer.DB, payload.Transaction.ID, constant.APPROVED, 4)
	})

	t.Run("bulk pending terminal and redelivery", func(t *testing.T) {
		payload := integrationAtomicPayload(constant.APPROVED, true)
		pending := *payload.Transaction
		pending.Status = transaction.Status{Code: constant.PENDING}
		pending.Operations = nil
		_, err := baseTransactionRepo.Create(context.Background(), &pending)
		require.NoError(t, err)

		barrierRepo := &commitBarrierTransactionRepository{
			Repository:  baseTransactionRepo,
			commitReady: make(chan struct{}),
			allowCommit: make(chan struct{}),
		}
		uc := &UseCase{TransactionRepo: barrierRepo, OperationRepo: baseOperationRepo}
		toInsert, toUpdate := uc.classifyAndExtractEntities([]transaction.TransactionProcessingPayload{payload})
		bulkResult := &BulkResult{InsertedTransactionIDs: make(map[string]struct{})}
		done := make(chan error, 1)
		go func() {
			done <- uc.performBulkInsertAndUpdate(context.Background(), &MockLogger{}, toInsert, toUpdate, bulkResult)
		}()

		select {
		case <-barrierRepo.commitReady:
		case earlyErr := <-done:
			t.Fatalf("bulk persistence returned before commit barrier: %v", earlyErr)
		case <-time.After(10 * time.Second):
			t.Fatal("bulk persistence did not reach its commit boundary")
		}
		requireTransactionSnapshot(t, pgContainer.DB, payload.Transaction.ID, constant.PENDING, 0)
		requireRoutedTransactionSnapshots(t, baseTransactionRepo, payload, constant.PENDING, 0)
		close(barrierRepo.allowCommit)
		require.NoError(t, <-done)
		requireTransactionSnapshot(t, pgContainer.DB, payload.Transaction.ID, constant.APPROVED, 4)
		requireRoutedTransactionSnapshots(t, baseTransactionRepo, payload, constant.APPROVED, 4)

		// Bulk redelivery also stays exact and cannot append a second set of
		// fee-expanded legs.
		replayUC := &UseCase{TransactionRepo: baseTransactionRepo, OperationRepo: baseOperationRepo}
		toInsert, toUpdate = replayUC.classifyAndExtractEntities([]transaction.TransactionProcessingPayload{payload})
		bulkResult = &BulkResult{InsertedTransactionIDs: make(map[string]struct{})}
		require.NoError(t, replayUC.performBulkInsertAndUpdate(context.Background(), &MockLogger{}, toInsert, toUpdate, bulkResult))
		requireTransactionSnapshot(t, pgContainer.DB, payload.Transaction.ID, constant.APPROVED, 4)
	})

	t.Run("operation failure rolls back every PostgreSQL write", func(t *testing.T) {
		payload := integrationAtomicPayload(constant.CREATED, false)
		operationErr := errors.New("injected operation failure")
		uc := &UseCase{
			TransactionRepo: baseTransactionRepo,
			OperationRepo: &failingAtomicOperationRepository{
				Repository: baseOperationRepo,
				err:        operationErr,
			},
		}

		_, _, err := uc.persistTransactionAndOperationsAtomic(context.Background(), payload)
		require.ErrorIs(t, err, operationErr)
		requireTransactionSnapshot(t, pgContainer.DB, payload.Transaction.ID, "", 0)
	})
}

func requireTransactionSnapshot(t *testing.T, db *sql.DB, transactionID, wantStatus string, wantOperations int) {
	t.Helper()

	var status string
	var operationCount int
	err := db.QueryRow(`
		SELECT t.status, COUNT(o.id)
		  FROM transaction t
		  LEFT JOIN operation o ON o.transaction_id = t.id
		 WHERE t.id = $1
		 GROUP BY t.status`, transactionID).Scan(&status, &operationCount)
	if wantStatus == "" {
		require.ErrorIs(t, err, sql.ErrNoRows)

		return
	}

	require.NoError(t, err)
	require.Equal(t, wantStatus, status)
	require.Equal(t, wantOperations, operationCount)
}

func requireRoutedTransactionSnapshots(
	t *testing.T,
	repo transaction.Repository,
	payload transaction.TransactionProcessingPayload,
	wantStatus string,
	wantOperations int,
) {
	t.Helper()

	organizationID := uuid.MustParse(payload.Transaction.OrganizationID)
	ledgerID := uuid.MustParse(payload.Transaction.LedgerID)
	transactionID := uuid.MustParse(payload.Transaction.ID)
	contexts := []struct {
		name string
		ctx  context.Context
	}{
		{name: "primary", ctx: readrouting.WithPrimaryRead(context.Background())},
		{name: "replica", ctx: context.Background()},
	}
	for _, route := range contexts {
		if wantStatus == "" {
			_, err := repo.Find(route.ctx, organizationID, ledgerID, transactionID)
			require.Error(t, err, "%s route must not observe an uncommitted terminal transaction", route.name)

			continue
		}
		if wantOperations == 0 {
			persisted, err := repo.Find(route.ctx, organizationID, ledgerID, transactionID)
			require.NoError(t, err, "%s route must read the pre-terminal transaction", route.name)
			require.Equal(t, wantStatus, persisted.Status.Code, route.name)

			continue
		}
		persisted, err := repo.FindWithOperations(route.ctx, organizationID, ledgerID, transactionID)
		require.NoError(t, err, "%s route must read the committed transaction", route.name)
		require.Equal(t, wantStatus, persisted.Status.Code, route.name)
		require.Len(t, persisted.Operations, wantOperations, route.name)
	}
}

func integrationAtomicPayload(status string, pending bool) transaction.TransactionProcessingPayload {
	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()
	transactionAmount := decimal.NewFromInt(1000)
	now := time.Now().UTC().Truncate(time.Microsecond)
	legAmounts := []int64{1000, 10, 1000, 10}
	directions := []string{"debit", "debit", "credit", "credit"}
	operations := make([]*operation.Operation, 0, len(legAmounts))
	for index, legAmount := range legAmounts {
		amount := decimal.NewFromInt(legAmount)
		availableBefore := decimal.NewFromInt(10000)
		availableAfter := availableBefore.Sub(amount)
		onHoldBefore := decimal.Zero
		onHoldAfter := decimal.Zero
		versionBefore := int64(1)
		versionAfter := int64(2)
		operations = append(operations, &operation.Operation{
			ID:             uuid.NewString(),
			TransactionID:  transactionID.String(),
			Description:    "atomic async economic leg",
			Type:           directions[index],
			Direction:      directions[index],
			AssetCode:      "USD",
			Amount:         operation.Amount{Value: &amount},
			Balance:        operation.Balance{Available: &availableBefore, OnHold: &onHoldBefore, Version: &versionBefore},
			BalanceAfter:   operation.Balance{Available: &availableAfter, OnHold: &onHoldAfter, Version: &versionAfter},
			Status:         operation.Status{Code: constant.APPROVED},
			AccountID:      uuid.NewString(),
			AccountAlias:   "@atomic-account",
			BalanceID:      uuid.NewString(),
			BalanceKey:     "default",
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			CreatedAt:      now,
			UpdatedAt:      now,
		})
	}

	return transaction.TransactionProcessingPayload{
		Transaction: &transaction.Transaction{
			ID:             transactionID.String(),
			Description:    "atomic async fee-expanded transaction",
			Status:         transaction.Status{Code: status},
			Amount:         &transactionAmount,
			AssetCode:      "USD",
			LedgerID:       ledgerID.String(),
			OrganizationID: organizationID.String(),
			Operations:     operations,
			CreatedAt:      now,
			UpdatedAt:      now,
		},
		Validate: &mtransaction.Responses{Pending: pending},
		Input:    &mtransaction.Transaction{},
		Version:  "v2",
	}
}
