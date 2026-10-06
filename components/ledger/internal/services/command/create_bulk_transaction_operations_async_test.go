// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/rabbitmq"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/repository"
	pkgStreaming "github.com/LerianStudio/midaz/v4/pkg/streaming"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

// mockDBTransaction is a test double for repository.DBTransaction.
type mockDBTransaction struct {
	commitCalled   bool
	rollbackCalled bool
	commitErr      error
	rollbackErr    error
}

func (m *mockDBTransaction) ExecContext(_ context.Context, _ string, _ ...any) (sql.Result, error) {
	return nil, nil
}

func (m *mockDBTransaction) Commit() error {
	m.commitCalled = true
	return m.commitErr
}

func (m *mockDBTransaction) Rollback() error {
	m.rollbackCalled = true
	return m.rollbackErr
}

func TestCreateBulkTransactionOperationsAsync_EmptyPayloads(t *testing.T) {
	t.Parallel()

	uc := &UseCase{}

	result, err := uc.CreateBulkTransactionOperationsAsync(context.Background(), nil)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, int64(0), result.TransactionsAttempted)
	assert.Equal(t, int64(0), result.OperationsAttempted)
}

func TestCreateBulkTransactionOperationsAsync_EmptySlice(t *testing.T) {
	t.Parallel()

	uc := &UseCase{}

	result, err := uc.CreateBulkTransactionOperationsAsync(context.Background(), []transaction.TransactionProcessingPayload{})

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, int64(0), result.TransactionsAttempted)
}

func TestCreateBulkTransactionOperationsAsync_SingleTransaction_Success(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockOperationRepo := operation.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		TransactionRepo:         mockTransactionRepo,
		OperationRepo:           mockOperationRepo,
		TransactionMetadataRepo: mockMetadataRepo,
		BalanceRepo:             mockBalanceRepo,
		RabbitMQRepo:            mockRabbitMQRepo,
		TransactionRedisRepo:    mockRedisRepo,
	}

	orgID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New().String()

	tx := &transaction.Transaction{
		ID:             transactionID,
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		Status: transaction.Status{
			Code: constant.CREATED,
		},
		Operations: []*operation.Operation{
			{
				ID:            uuid.New().String(),
				TransactionID: transactionID,
			},
		},
	}

	payload := transaction.TransactionProcessingPayload{
		Transaction: tx,
		Validate: &mtransaction.Responses{
			Aliases: []string{"alias1"},
		},
		Balances: []*mmodel.Balance{
			{
				ID:        uuid.New().String(),
				Alias:     "alias1",
				Available: decimal.NewFromInt(100),
			},
		},
		BalancesAfter: []*mmodel.Balance{
			{
				ID:        uuid.New().String(),
				Alias:     "alias1",
				Available: decimal.NewFromInt(50),
			},
		},
		Version: "v2",
	}

	// Mock DB transaction for atomic inserts
	mockTx := &mockDBTransaction{}
	mockTransactionRepo.EXPECT().
		BeginTx(gomock.Any()).
		Return(mockTx, nil).
		Times(1)

	// Mock bulk insert transactions (using Tx variant)
	mockTransactionRepo.EXPECT().
		CreateBulkTx(gomock.Any(), mockTx, gomock.Any()).
		Return(&repository.BulkInsertResult{
			Attempted: 1,
			Inserted:  1,
			Ignored:   0,
		}, nil).
		Times(1)

	// Mock bulk insert operations (using Tx variant)
	mockOperationRepo.EXPECT().
		CreateBulkTx(gomock.Any(), mockTx, gomock.Any()).
		Return(&repository.BulkInsertResult{
			Attempted: 1,
			Inserted:  1,
			Ignored:   0,
		}, nil).
		Times(1)

	// Note: Balance updates are handled by BalanceSyncWorker, not in this flow

	// Mock metadata creation (may be called)
	mockMetadataRepo.EXPECT().
		Create(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	// Mock event sending (async - uses ProducerDefault)
	mockRabbitMQRepo.EXPECT().
		ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil).
		AnyTimes()

	// Mock Redis cleanup (async)
	mockRedisRepo.EXPECT().
		RemoveMessageFromQueue(gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	mockRedisRepo.EXPECT().
		Del(gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	result, err := uc.CreateBulkTransactionOperationsAsync(context.Background(), []transaction.TransactionProcessingPayload{payload})

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, int64(1), result.TransactionsAttempted)
	assert.Equal(t, int64(1), result.TransactionsInserted)
	assert.Equal(t, int64(1), result.OperationsAttempted)
	assert.Equal(t, int64(1), result.OperationsInserted)
	assert.False(t, result.FallbackUsed)
}

func TestCreateBulkTransactionOperationsAsync_MultipleTransactions_Success(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockOperationRepo := operation.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		TransactionRepo:         mockTransactionRepo,
		OperationRepo:           mockOperationRepo,
		TransactionMetadataRepo: mockMetadataRepo,
		BalanceRepo:             mockBalanceRepo,
		RabbitMQRepo:            mockRabbitMQRepo,
		TransactionRedisRepo:    mockRedisRepo,
	}

	payloads := make([]transaction.TransactionProcessingPayload, 3)
	for i := 0; i < 3; i++ {
		orgID := uuid.New()
		ledgerID := uuid.New()
		transactionID := uuid.New().String()

		payloads[i] = transaction.TransactionProcessingPayload{
			Transaction: &transaction.Transaction{
				ID:             transactionID,
				OrganizationID: orgID.String(),
				LedgerID:       ledgerID.String(),
				Status: transaction.Status{
					Code: constant.CREATED,
				},
				Operations: []*operation.Operation{
					{ID: uuid.New().String(), TransactionID: transactionID},
					{ID: uuid.New().String(), TransactionID: transactionID},
				},
			},
			Validate: &mtransaction.Responses{
				Aliases: []string{"alias1"},
			},
			Balances: []*mmodel.Balance{
				{ID: uuid.New().String(), Alias: "alias1", Available: decimal.NewFromInt(100)},
			},
			BalancesAfter: []*mmodel.Balance{
				{ID: uuid.New().String(), Alias: "alias1", Available: decimal.NewFromInt(50)},
			},
			Version: "v2",
		}
	}

	// Mock DB transaction for atomic inserts
	mockTx := &mockDBTransaction{}
	mockTransactionRepo.EXPECT().
		BeginTx(gomock.Any()).
		Return(mockTx, nil).
		Times(1)

	// Mock bulk insert transactions (3 transactions, using Tx variant)
	mockTransactionRepo.EXPECT().
		CreateBulkTx(gomock.Any(), mockTx, gomock.Any()).
		Return(&repository.BulkInsertResult{
			Attempted: 3,
			Inserted:  3,
			Ignored:   0,
		}, nil).
		Times(1)

	// Mock bulk insert operations (6 operations total, using Tx variant)
	mockOperationRepo.EXPECT().
		CreateBulkTx(gomock.Any(), mockTx, gomock.Any()).
		Return(&repository.BulkInsertResult{
			Attempted: 6,
			Inserted:  6,
			Ignored:   0,
		}, nil).
		Times(1)

	// Note: Balance updates are handled by BalanceSyncWorker, not in this flow

	// Mock metadata and events
	mockMetadataRepo.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockRabbitMQRepo.EXPECT().ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockRedisRepo.EXPECT().RemoveMessageFromQueue(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockRedisRepo.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	result, err := uc.CreateBulkTransactionOperationsAsync(context.Background(), payloads)

	require.NoError(t, err)
	assert.Equal(t, int64(3), result.TransactionsAttempted)
	assert.Equal(t, int64(3), result.TransactionsInserted)
	assert.Equal(t, int64(6), result.OperationsAttempted)
	assert.Equal(t, int64(6), result.OperationsInserted)
}

func TestCreateBulkTransactionOperationsAsync_WithDuplicates(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockOperationRepo := operation.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		TransactionRepo:         mockTransactionRepo,
		OperationRepo:           mockOperationRepo,
		TransactionMetadataRepo: mockMetadataRepo,
		BalanceRepo:             mockBalanceRepo,
		RabbitMQRepo:            mockRabbitMQRepo,
		TransactionRedisRepo:    mockRedisRepo,
	}

	orgID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New().String()

	payload := transaction.TransactionProcessingPayload{
		Transaction: &transaction.Transaction{
			ID:             transactionID,
			OrganizationID: orgID.String(),
			LedgerID:       ledgerID.String(),
			Status:         transaction.Status{Code: constant.CREATED},
			Operations: []*operation.Operation{
				{ID: uuid.New().String(), TransactionID: transactionID},
			},
		},
		Validate:      &mtransaction.Responses{Aliases: []string{"alias1"}},
		Balances:      []*mmodel.Balance{{ID: uuid.New().String(), Alias: "alias1", Available: decimal.NewFromInt(100)}},
		BalancesAfter: []*mmodel.Balance{{ID: uuid.New().String(), Alias: "alias1", Available: decimal.NewFromInt(50)}},
		Version:       "v2",
	}

	// Mock DB transaction for atomic inserts
	mockTx := &mockDBTransaction{}
	mockTransactionRepo.EXPECT().
		BeginTx(gomock.Any()).
		Return(mockTx, nil).
		Times(1)

	// Mock bulk insert with 1 ignored (duplicate)
	mockTransactionRepo.EXPECT().
		CreateBulkTx(gomock.Any(), mockTx, gomock.Any()).
		Return(&repository.BulkInsertResult{
			Attempted: 1,
			Inserted:  0,
			Ignored:   1,
		}, nil).
		Times(1)

	mockOperationRepo.EXPECT().
		CreateBulkTx(gomock.Any(), mockTx, gomock.Any()).
		Return(&repository.BulkInsertResult{
			Attempted: 1,
			Inserted:  0,
			Ignored:   1,
		}, nil).
		Times(1)

	// Note: Balance updates are handled by BalanceSyncWorker, not in this flow

	mockMetadataRepo.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockRabbitMQRepo.EXPECT().ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockRedisRepo.EXPECT().RemoveMessageFromQueue(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockRedisRepo.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	result, err := uc.CreateBulkTransactionOperationsAsync(context.Background(), []transaction.TransactionProcessingPayload{payload})

	require.NoError(t, err)
	assert.Equal(t, int64(1), result.TransactionsAttempted)
	assert.Equal(t, int64(0), result.TransactionsInserted)
	assert.Equal(t, int64(1), result.TransactionsIgnored)
}

func TestCreateBulkTransactionOperationsAsync_StatusTransition_BelowThreshold(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockOperationRepo := operation.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		TransactionRepo:         mockTransactionRepo,
		OperationRepo:           mockOperationRepo,
		TransactionMetadataRepo: mockMetadataRepo,
		BalanceRepo:             mockBalanceRepo,
		RabbitMQRepo:            mockRabbitMQRepo,
		TransactionRedisRepo:    mockRedisRepo,
	}

	orgID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New().String()

	// Status transition: PENDING -> APPROVED
	payload := transaction.TransactionProcessingPayload{
		Transaction: &transaction.Transaction{
			ID:             transactionID,
			OrganizationID: orgID.String(),
			LedgerID:       ledgerID.String(),
			Status:         transaction.Status{Code: constant.APPROVED},
			Operations:     []*operation.Operation{},
		},
		Validate: &mtransaction.Responses{
			Aliases: []string{"alias1"},
			Pending: true, // This indicates a pending transaction being updated
		},
		Balances:      []*mmodel.Balance{{ID: uuid.New().String(), Alias: "alias1", Available: decimal.NewFromInt(100)}},
		BalancesAfter: []*mmodel.Balance{{ID: uuid.New().String(), Alias: "alias1", Available: decimal.NewFromInt(50)}},
		Version:       "v2",
	}

	// Note: Balance updates are handled by BalanceSyncWorker, not in this flow

	// Below threshold (< 10), uses individual update via UpdateTransactionStatus
	// which internally calls TransactionRepo.Update
	mockTransactionRepo.EXPECT().
		Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(&transaction.Transaction{ID: transactionID}, nil).
		Times(1)

	mockMetadataRepo.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockRabbitMQRepo.EXPECT().ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockRedisRepo.EXPECT().RemoveMessageFromQueue(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockRedisRepo.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	result, err := uc.CreateBulkTransactionOperationsAsync(context.Background(), []transaction.TransactionProcessingPayload{payload})

	require.NoError(t, err)
	assert.Equal(t, int64(0), result.TransactionsAttempted) // No inserts
	assert.Equal(t, int64(1), result.TransactionsUpdateAttempted)
	assert.Equal(t, int64(1), result.TransactionsUpdated)
}

func TestCreateBulkTransactionOperationsAsync_MixedInsertedAndDuplicate_CleansDuplicateBackup(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockOperationRepo := operation.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		TransactionRepo:         mockTransactionRepo,
		OperationRepo:           mockOperationRepo,
		TransactionMetadataRepo: mockMetadataRepo,
		BalanceRepo:             mockBalanceRepo,
		RabbitMQRepo:            mockRabbitMQRepo,
		TransactionRedisRepo:    mockRedisRepo,
	}

	orgID := uuid.New()
	ledgerID := uuid.New()

	insertedTxID := uuid.New().String()
	duplicateTxID := uuid.New().String()

	payloads := []transaction.TransactionProcessingPayload{
		{
			Transaction: &transaction.Transaction{
				ID:             insertedTxID,
				OrganizationID: orgID.String(),
				LedgerID:       ledgerID.String(),
				Status:         transaction.Status{Code: constant.APPROVED},
				Operations: []*operation.Operation{
					{ID: uuid.New().String(), TransactionID: insertedTxID},
				},
			},
			Validate:      &mtransaction.Responses{Aliases: []string{"alias1"}},
			Balances:      []*mmodel.Balance{{ID: uuid.New().String(), Alias: "alias1", Available: decimal.NewFromInt(100)}},
			BalancesAfter: []*mmodel.Balance{{ID: uuid.New().String(), Alias: "alias1", Available: decimal.NewFromInt(50)}},
			Version:       "v2",
		},
		{
			Transaction: &transaction.Transaction{
				ID:             duplicateTxID,
				OrganizationID: orgID.String(),
				LedgerID:       ledgerID.String(),
				Status:         transaction.Status{Code: constant.APPROVED},
				Operations: []*operation.Operation{
					{ID: uuid.New().String(), TransactionID: duplicateTxID},
				},
			},
			Validate:      &mtransaction.Responses{Aliases: []string{"alias2"}},
			Balances:      []*mmodel.Balance{{ID: uuid.New().String(), Alias: "alias2", Available: decimal.NewFromInt(100)}},
			BalancesAfter: []*mmodel.Balance{{ID: uuid.New().String(), Alias: "alias2", Available: decimal.NewFromInt(50)}},
			Version:       "v2",
		},
	}

	mockTx := &mockDBTransaction{}
	mockTransactionRepo.EXPECT().
		BeginTx(gomock.Any()).
		Return(mockTx, nil).
		Times(1)

	mockTransactionRepo.EXPECT().
		CreateBulkTx(gomock.Any(), mockTx, gomock.Any()).
		Return(&repository.BulkInsertResult{
			Attempted:   2,
			Inserted:    1,
			Ignored:     1,
			InsertedIDs: []string{insertedTxID},
		}, nil).
		Times(1)

	mockOperationRepo.EXPECT().
		CreateBulkTx(gomock.Any(), mockTx, gomock.Any()).
		Return(&repository.BulkInsertResult{
			Attempted: 2,
			Inserted:  1,
			Ignored:   1,
		}, nil).
		Times(1)

	mockMetadataRepo.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockRabbitMQRepo.EXPECT().ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()

	rawQueue, err := json.Marshal(mmodel.TransactionRedisQueue{TransactionStatus: constant.CREATED})
	require.NoError(t, err)

	var removeWG sync.WaitGroup
	removeWG.Add(2)

	var delWG sync.WaitGroup
	delWG.Add(2)

	mockRedisRepo.EXPECT().
		ReadMessageFromQueue(gomock.Any(), gomock.Any()).
		Return(rawQueue, nil).
		AnyTimes()

	mockRedisRepo.EXPECT().
		RemoveMessageFromQueue(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string) error {
			removeWG.Done()
			return nil
		}).
		Times(2)

	mockRedisRepo.EXPECT().
		Del(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string) error {
			delWG.Done()
			return nil
		}).
		Times(2)

	result, err := uc.CreateBulkTransactionOperationsAsync(context.Background(), payloads)

	require.NoError(t, err)
	assert.Equal(t, int64(2), result.TransactionsAttempted)
	assert.Equal(t, int64(1), result.TransactionsInserted)
	assert.Equal(t, int64(1), result.TransactionsIgnored)

	waitWithTimeout := func(wg *sync.WaitGroup, what string) {
		t.Helper()

		done := make(chan struct{})
		go func() {
			wg.Wait()
			close(done)
		}()

		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("timed out waiting for %s", what)
		}
	}

	waitWithTimeout(&removeWG, "backup queue cleanup")
	waitWithTimeout(&delWG, "write-behind cleanup")
}

func TestCreateBulkTransactionOperationsAsync_NilTransaction_Skipped(t *testing.T) {
	t.Parallel()

	uc := &UseCase{}

	payloads := []transaction.TransactionProcessingPayload{
		{Transaction: nil}, // Nil transaction should be skipped
	}

	result, err := uc.CreateBulkTransactionOperationsAsync(context.Background(), payloads)

	require.NoError(t, err)
	assert.Equal(t, int64(0), result.TransactionsAttempted)
	assert.Equal(t, int64(0), result.OperationsAttempted)
}

func TestCreateBulkTransactionOperationsAsync_BulkInsertFails_UsesFallback(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockOperationRepo := operation.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		TransactionRepo:         mockTransactionRepo,
		OperationRepo:           mockOperationRepo,
		TransactionMetadataRepo: mockMetadataRepo,
		BalanceRepo:             mockBalanceRepo,
		RabbitMQRepo:            mockRabbitMQRepo,
		TransactionRedisRepo:    mockRedisRepo,
	}

	orgID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New().String()
	operationID := uuid.New().String()

	payload := transaction.TransactionProcessingPayload{
		Transaction: &transaction.Transaction{
			ID:             transactionID,
			OrganizationID: orgID.String(),
			LedgerID:       ledgerID.String(),
			Status:         transaction.Status{Code: constant.CREATED},
			Operations: []*operation.Operation{
				{ID: operationID, TransactionID: transactionID},
			},
		},
		Validate:      &mtransaction.Responses{Aliases: []string{"alias1"}},
		Balances:      []*mmodel.Balance{{ID: uuid.New().String(), Alias: "alias1", Available: decimal.NewFromInt(100)}},
		BalancesAfter: []*mmodel.Balance{{ID: uuid.New().String(), Alias: "alias1", Available: decimal.NewFromInt(50)}},
		Version:       "v2",
	}

	// Mock DB transaction for atomic inserts - will fail and rollback
	mockTx := &mockDBTransaction{}
	mockTransactionRepo.EXPECT().
		BeginTx(gomock.Any()).
		Return(mockTx, nil).
		Times(1)

	// Bulk insert fails - should trigger rollback and fallback
	mockTransactionRepo.EXPECT().
		CreateBulkTx(gomock.Any(), mockTx, gomock.Any()).
		Return(nil, errors.New("bulk insert failed")).
		Times(1)

	// Fallback processing calls CreateBalanceTransactionOperationsAsync
	// which creates or updates transaction individually
	mockTransactionRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		Return(&transaction.Transaction{ID: transactionID}, nil).
		Times(1)

	// Fallback also creates operations
	mockOperationRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		Return(&operation.Operation{ID: operationID}, nil).
		Times(1)

	// Note: Balance updates are handled by BalanceSyncWorker, not in fallback flow

	// Metadata creation and events
	mockMetadataRepo.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockRabbitMQRepo.EXPECT().ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockRedisRepo.EXPECT().RemoveMessageFromQueue(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockRedisRepo.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	result, err := uc.CreateBulkTransactionOperationsAsync(context.Background(), []transaction.TransactionProcessingPayload{payload})

	require.NoError(t, err)
	assert.True(t, result.FallbackUsed)
	assert.Equal(t, int64(1), result.FallbackCount)
	require.NotNil(t, result.MetadataFailedTransactionIDs)
	assert.Empty(t, result.MetadataFailedTransactionIDs)
}

func TestClassifyAndExtractEntities_SortsTransactions(t *testing.T) {
	t.Parallel()

	uc := &UseCase{}

	// Create transactions with IDs that would sort differently
	payloads := []transaction.TransactionProcessingPayload{
		{
			Transaction: &transaction.Transaction{
				ID:     "zzz00000-0000-0000-0000-000000000003",
				Status: transaction.Status{Code: constant.CREATED},
			},
		},
		{
			Transaction: &transaction.Transaction{
				ID:     "aaa00000-0000-0000-0000-000000000001",
				Status: transaction.Status{Code: constant.CREATED},
			},
		},
		{
			Transaction: &transaction.Transaction{
				ID:     "mmm00000-0000-0000-0000-000000000002",
				Status: transaction.Status{Code: constant.CREATED},
			},
		},
	}

	toInsert, _ := uc.classifyAndExtractEntities(payloads)

	// Verify transactions are sorted by ID
	assert.Equal(t, "aaa00000-0000-0000-0000-000000000001", toInsert.transactions[0].ID)
	assert.Equal(t, "mmm00000-0000-0000-0000-000000000002", toInsert.transactions[1].ID)
	assert.Equal(t, "zzz00000-0000-0000-0000-000000000003", toInsert.transactions[2].ID)
}

func TestClassifyAndExtractEntities_SeparatesInsertsAndUpdates(t *testing.T) {
	t.Parallel()

	uc := &UseCase{}

	payloads := []transaction.TransactionProcessingPayload{
		// Insert: new transaction
		{
			Transaction: &transaction.Transaction{
				ID:     uuid.New().String(),
				Status: transaction.Status{Code: constant.CREATED},
			},
		},
		// Update: pending -> approved
		{
			Transaction: &transaction.Transaction{
				ID:     uuid.New().String(),
				Status: transaction.Status{Code: constant.APPROVED},
			},
			Validate: &mtransaction.Responses{Pending: true},
			Version:  "v2",
		},
		// Update: pending -> canceled
		{
			Transaction: &transaction.Transaction{
				ID:     uuid.New().String(),
				Status: transaction.Status{Code: constant.CANCELED},
			},
			Validate: &mtransaction.Responses{Pending: true},
			Version:  "v2",
		},
	}

	toInsert, toUpdate := uc.classifyAndExtractEntities(payloads)

	assert.Len(t, toInsert.transactions, 1)
	assert.Len(t, toUpdate.transactions, 2)
}

func TestIsStatusTransition(t *testing.T) {
	t.Parallel()

	uc := &UseCase{}

	tests := []struct {
		name     string
		payload  transaction.TransactionProcessingPayload
		expected bool
	}{
		{
			name: "pending to approved is transition",
			payload: transaction.TransactionProcessingPayload{
				Transaction: &transaction.Transaction{
					Status: transaction.Status{Code: constant.APPROVED},
				},
				Validate: &mtransaction.Responses{Pending: true},
				Version:  "v2",
			},
			expected: true,
		},
		{
			name: "pending to canceled is transition",
			payload: transaction.TransactionProcessingPayload{
				Transaction: &transaction.Transaction{
					Status: transaction.Status{Code: constant.CANCELED},
				},
				Validate: &mtransaction.Responses{Pending: true},
				Version:  "v2",
			},
			expected: true,
		},
		{
			name: "new created is not transition",
			payload: transaction.TransactionProcessingPayload{
				Transaction: &transaction.Transaction{
					Status: transaction.Status{Code: constant.CREATED},
				},
			},
			expected: false,
		},
		{
			name: "nil validate is not transition",
			payload: transaction.TransactionProcessingPayload{
				Transaction: &transaction.Transaction{
					Status: transaction.Status{Code: constant.APPROVED},
				},
				Validate: nil,
			},
			expected: false,
		},
		{
			name: "pending false is not transition",
			payload: transaction.TransactionProcessingPayload{
				Transaction: &transaction.Transaction{
					Status: transaction.Status{Code: constant.APPROVED},
				},
				Validate: &mtransaction.Responses{Pending: false},
				Version:  "v2",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := uc.isStatusTransition(tt.payload)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBulkResult_InitialValues(t *testing.T) {
	t.Parallel()

	result := &BulkResult{}

	assert.Equal(t, int64(0), result.TransactionsAttempted)
	assert.Equal(t, int64(0), result.TransactionsInserted)
	assert.Equal(t, int64(0), result.TransactionsIgnored)
	assert.Equal(t, int64(0), result.TransactionsUpdateAttempted)
	assert.Equal(t, int64(0), result.TransactionsUpdated)
	assert.Equal(t, int64(0), result.OperationsAttempted)
	assert.Equal(t, int64(0), result.OperationsInserted)
	assert.Equal(t, int64(0), result.OperationsIgnored)
	assert.False(t, result.FallbackUsed)
	assert.Equal(t, int64(0), result.FallbackCount)
}

func TestSortTransactionsByID(t *testing.T) {
	t.Parallel()

	transactions := []*transaction.Transaction{
		{ID: "ccc"},
		{ID: "aaa"},
		{ID: "bbb"},
	}

	sortTransactionsByID(transactions)

	assert.Equal(t, "aaa", transactions[0].ID)
	assert.Equal(t, "bbb", transactions[1].ID)
	assert.Equal(t, "ccc", transactions[2].ID)
}

func TestSortOperationsByID(t *testing.T) {
	t.Parallel()

	operations := []*operation.Operation{
		{ID: "ccc"},
		{ID: "aaa"},
		{ID: "bbb"},
	}

	sortOperationsByID(operations)

	assert.Equal(t, "aaa", operations[0].ID)
	assert.Equal(t, "bbb", operations[1].ID)
	assert.Equal(t, "ccc", operations[2].ID)
}

func TestExtractOrgLedgerIDs_Valid(t *testing.T) {
	t.Parallel()

	uc := &UseCase{}

	orgID := uuid.New()
	ledgerID := uuid.New()

	payload := transaction.TransactionProcessingPayload{
		Transaction: &transaction.Transaction{
			OrganizationID: orgID.String(),
			LedgerID:       ledgerID.String(),
		},
	}

	extractedOrg, extractedLedger, err := uc.extractOrgLedgerIDs(payload)

	require.NoError(t, err)
	assert.Equal(t, orgID, extractedOrg)
	assert.Equal(t, ledgerID, extractedLedger)
}

func TestExtractOrgLedgerIDs_NilTransaction(t *testing.T) {
	t.Parallel()

	uc := &UseCase{}

	payload := transaction.TransactionProcessingPayload{
		Transaction: nil,
	}

	_, _, err := uc.extractOrgLedgerIDs(payload)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil transaction")
}

func TestExtractOrgLedgerIDs_InvalidOrgID(t *testing.T) {
	t.Parallel()

	uc := &UseCase{}

	payload := transaction.TransactionProcessingPayload{
		Transaction: &transaction.Transaction{
			OrganizationID: "not-a-uuid",
			LedgerID:       uuid.New().String(),
		},
	}

	_, _, err := uc.extractOrgLedgerIDs(payload)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid organization ID")
}

func TestExtractOrgLedgerIDs_InvalidLedgerID(t *testing.T) {
	t.Parallel()

	uc := &UseCase{}

	payload := transaction.TransactionProcessingPayload{
		Transaction: &transaction.Transaction{
			OrganizationID: uuid.New().String(),
			LedgerID:       "not-a-uuid",
		},
	}

	_, _, err := uc.extractOrgLedgerIDs(payload)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid ledger ID")
}

func TestCreateBulkTransactionOperationsAsync_BalanceUpdateFails_UsesFallback(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockOperationRepo := operation.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		TransactionRepo:         mockTransactionRepo,
		OperationRepo:           mockOperationRepo,
		TransactionMetadataRepo: mockMetadataRepo,
		BalanceRepo:             mockBalanceRepo,
		RabbitMQRepo:            mockRabbitMQRepo,
		TransactionRedisRepo:    mockRedisRepo,
	}

	orgID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New().String()
	operationID := uuid.New().String()

	payload := transaction.TransactionProcessingPayload{
		Transaction: &transaction.Transaction{
			ID:             transactionID,
			OrganizationID: orgID.String(),
			LedgerID:       ledgerID.String(),
			Status:         transaction.Status{Code: constant.CREATED},
			Operations: []*operation.Operation{
				{ID: operationID, TransactionID: transactionID},
			},
		},
		Validate:      &mtransaction.Responses{Aliases: []string{"alias1"}},
		Balances:      []*mmodel.Balance{{ID: uuid.New().String(), Alias: "alias1", Available: decimal.NewFromInt(100)}},
		BalancesAfter: []*mmodel.Balance{{ID: uuid.New().String(), Alias: "alias1", Available: decimal.NewFromInt(50)}},
		Version:       "v2",
	}

	// Mock DB transaction for atomic inserts
	mockTx := &mockDBTransaction{}
	mockTransactionRepo.EXPECT().
		BeginTx(gomock.Any()).
		Return(mockTx, nil).
		Times(1)

	// Bulk insert succeeds (using Tx variant)
	mockTransactionRepo.EXPECT().
		CreateBulkTx(gomock.Any(), mockTx, gomock.Any()).
		Return(&repository.BulkInsertResult{
			Attempted: 1,
			Inserted:  1,
			Ignored:   0,
		}, nil).
		Times(1)

	mockOperationRepo.EXPECT().
		CreateBulkTx(gomock.Any(), mockTx, gomock.Any()).
		Return(&repository.BulkInsertResult{
			Attempted: 1,
			Inserted:  1,
			Ignored:   0,
		}, nil).
		Times(1)

	// Note: Balance updates are handled by BalanceSyncWorker, not in this flow
	// This test verifies bulk insert succeeds without inline balance updates

	// Metadata creation and events proceed
	mockMetadataRepo.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockRabbitMQRepo.EXPECT().ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockRedisRepo.EXPECT().RemoveMessageFromQueue(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockRedisRepo.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	result, err := uc.CreateBulkTransactionOperationsAsync(context.Background(), []transaction.TransactionProcessingPayload{payload})

	// Balance update failure does NOT trigger fallback since transaction is already persisted
	// The balance sync worker will eventually reconcile
	require.NoError(t, err)
	assert.Equal(t, int64(1), result.TransactionsAttempted)
	assert.Equal(t, int64(1), result.TransactionsInserted)
	assert.False(t, result.FallbackUsed) // Fallback is NOT used for balance update failures
}

func TestCreateBulkTransactionOperationsAsync_StatusTransition_AboveThreshold_UsesBulkUpdate(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockOperationRepo := operation.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		TransactionRepo:         mockTransactionRepo,
		OperationRepo:           mockOperationRepo,
		TransactionMetadataRepo: mockMetadataRepo,
		BalanceRepo:             mockBalanceRepo,
		RabbitMQRepo:            mockRabbitMQRepo,
		TransactionRedisRepo:    mockRedisRepo,
	}

	// Create 12 status transition payloads (above threshold of 10)
	payloads := make([]transaction.TransactionProcessingPayload, 12)
	for i := 0; i < 12; i++ {
		orgID := uuid.New()
		ledgerID := uuid.New()
		transactionID := uuid.New().String()

		payloads[i] = transaction.TransactionProcessingPayload{
			Transaction: &transaction.Transaction{
				ID:             transactionID,
				OrganizationID: orgID.String(),
				LedgerID:       ledgerID.String(),
				Status:         transaction.Status{Code: constant.APPROVED},
				Operations:     []*operation.Operation{}, // No operations for status transitions
			},
			Validate: &mtransaction.Responses{
				Aliases: []string{"alias1"},
				Pending: true, // This indicates a pending transaction being updated
			},
			Balances:      []*mmodel.Balance{{ID: uuid.New().String(), Alias: "alias1", Available: decimal.NewFromInt(100)}},
			BalancesAfter: []*mmodel.Balance{{ID: uuid.New().String(), Alias: "alias1", Available: decimal.NewFromInt(50)}},
			Version:       "v2",
		}
	}

	// Note: Balance updates are handled by BalanceSyncWorker, not in this flow

	// Mock bulk update (above threshold uses UpdateBulk instead of individual Update)
	mockTransactionRepo.EXPECT().
		UpdateBulk(gomock.Any(), gomock.Any()).
		Return(&repository.BulkUpdateResult{
			Attempted: 12,
			Updated:   12,
			Unchanged: 0,
		}, nil).
		Times(1)

	// Mock metadata and events
	mockMetadataRepo.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockRabbitMQRepo.EXPECT().ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockRedisRepo.EXPECT().RemoveMessageFromQueue(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
	mockRedisRepo.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

	result, err := uc.CreateBulkTransactionOperationsAsync(context.Background(), payloads)

	require.NoError(t, err)
	assert.Equal(t, int64(0), result.TransactionsAttempted) // No inserts
	assert.Equal(t, int64(12), result.TransactionsUpdateAttempted)
	assert.Equal(t, int64(12), result.TransactionsUpdated)
	assert.False(t, result.FallbackUsed)
}

func TestClassifyAndExtractEntities_DoesNotMutateOriginal(t *testing.T) {
	t.Parallel()

	// This test verifies that classifyAndExtractEntities creates a shallow copy
	// of transactions, preserving the original payload for fallback processing.

	uc := &UseCase{}

	// Create payload with a populated Body field
	originalBody := mtransaction.Transaction{
		ChartOfAccountsGroupName: "test-chart-group",
		Description:              "Original description that should be preserved",
	}

	originalPayload := transaction.TransactionProcessingPayload{
		Transaction: &transaction.Transaction{
			ID:             uuid.New().String(),
			OrganizationID: uuid.New().String(),
			LedgerID:       uuid.New().String(),
			Status:         transaction.Status{Code: constant.CREATED},
			Body:           originalBody,
			Description:    "Test transaction",
		},
	}

	// Store original values for comparison
	originalBodyChartGroup := originalPayload.Transaction.Body.ChartOfAccountsGroupName
	originalBodyDesc := originalPayload.Transaction.Body.Description

	// Call classifyAndExtractEntities which should create copies
	toInsert, _ := uc.classifyAndExtractEntities([]transaction.TransactionProcessingPayload{originalPayload})

	// CRITICAL: Verify original payload Body is NOT mutated
	assert.Equal(t, originalBodyChartGroup, originalPayload.Transaction.Body.ChartOfAccountsGroupName,
		"Original payload Body.ChartOfAccountsGroupName should NOT be modified")
	assert.Equal(t, originalBodyDesc, originalPayload.Transaction.Body.Description,
		"Original payload Body.Description should NOT be modified")

	// Verify extracted transaction has empty body (as expected for bulk insert)
	require.Len(t, toInsert.transactions, 1)
	assert.Empty(t, toInsert.transactions[0].Body.ChartOfAccountsGroupName,
		"Extracted transaction should have empty Body")
	assert.Empty(t, toInsert.transactions[0].Body.Description,
		"Extracted transaction should have empty Body.Description")

	// Verify original transaction ID is preserved (shallow copy of value fields)
	assert.Equal(t, originalPayload.Transaction.ID, toInsert.transactions[0].ID,
		"Transaction ID should be preserved in copy")
}

func TestIndividualUpdateTransactionStatus_PartialFailure(t *testing.T) {
	t.Parallel()

	// This test verifies that individualUpdateTransactionStatus returns an
	// aggregated error when some updates fail, instead of swallowing errors.

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTransactionRepo := transaction.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionRepo: mockTransactionRepo,
	}

	// Create 5 transactions for update
	transactions := make([]*transaction.Transaction, 5)
	for i := range 5 {
		transactions[i] = &transaction.Transaction{
			ID:             uuid.New().String(),
			OrganizationID: uuid.New().String(),
			LedgerID:       uuid.New().String(),
			Status:         transaction.Status{Code: constant.APPROVED},
		}
	}

	// Mock: first 3 succeed, last 2 fail
	// Update is called for each transaction
	gomock.InOrder(
		mockTransactionRepo.EXPECT().
			Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(&transaction.Transaction{}, nil),
		mockTransactionRepo.EXPECT().
			Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(&transaction.Transaction{}, nil),
		mockTransactionRepo.EXPECT().
			Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(&transaction.Transaction{}, nil),
		mockTransactionRepo.EXPECT().
			Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, errors.New("database connection lost")),
		mockTransactionRepo.EXPECT().
			Update(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, errors.New("database timeout")),
	)

	result := &BulkResult{
		TransactionsUpdateAttempted: int64(len(transactions)),
	}

	logger := libLog.NewMockLogger(ctrl)
	// Expect warning logs for 2 failed updates (msg + transaction_id field + err field)
	// and a debug summary log (msg + 3 count fields).
	logger.EXPECT().Log(gomock.Any(), libLog.LevelWarn, gomock.Any(), gomock.Any(), gomock.Any()).Times(2)
	logger.EXPECT().Log(gomock.Any(), libLog.LevelDebug, gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(1)

	err := uc.individualUpdateTransactionStatus(context.Background(), logger, transactions, result)

	// CRITICAL: Should return error indicating partial failure
	require.Error(t, err, "Should return error when some updates fail")
	assert.Contains(t, err.Error(), "failed to update 2 of 5 transactions",
		"Error should contain accurate failure count")

	// Verify result reflects only successful updates
	assert.Equal(t, int64(3), result.TransactionsUpdated,
		"TransactionsUpdated should reflect only successful updates")
}

// TestClassifyAndExtractEntities_CollectsOperationsForStatusTransitions verifies that
// operations from status transitions (commit/cancel flows) are correctly collected in toInsert.operations.
// This is a regression test for the bug where operations from commit flows were silently discarded.
func TestClassifyAndExtractEntities_CollectsOperationsForStatusTransitions(t *testing.T) {
	t.Parallel()

	uc := &UseCase{}

	transactionID := uuid.New().String()
	op1ID := uuid.New().String()
	op2ID := uuid.New().String()

	// Status transition: PENDING -> APPROVED with operations (commit flow)
	payloads := []transaction.TransactionProcessingPayload{
		{
			Transaction: &transaction.Transaction{
				ID:     transactionID,
				Status: transaction.Status{Code: constant.APPROVED},
				Operations: []*operation.Operation{
					{ID: op1ID, TransactionID: transactionID},
					{ID: op2ID, TransactionID: transactionID},
				},
			},
			Validate: &mtransaction.Responses{Pending: true},
			Version:  "v2",
		},
	}

	toInsert, toUpdate := uc.classifyAndExtractEntities(payloads)

	// Transaction should be in toUpdate (status transition)
	assert.Len(t, toUpdate.transactions, 1, "Status transition should go to toUpdate")
	assert.Len(t, toInsert.transactions, 0, "No new transactions to insert")

	// CRITICAL: Operations MUST be collected in toInsert.operations
	assert.Len(t, toInsert.operations, 2, "Operations from status transition MUST be collected")

	// Verify operation IDs are correct
	operationIDs := make(map[string]bool)
	for _, op := range toInsert.operations {
		operationIDs[op.ID] = true
	}
	assert.True(t, operationIDs[op1ID], "Operation 1 should be in toInsert.operations")
	assert.True(t, operationIDs[op2ID], "Operation 2 should be in toInsert.operations")
}

// TestClassifyAndExtractEntities_MixedBatch_CollectsAllOperations verifies that
// operations from BOTH new inserts AND status transitions are collected in a mixed batch.
// This ensures the fix handles the common case of batches containing both types.
func TestClassifyAndExtractEntities_MixedBatch_CollectsAllOperations(t *testing.T) {
	t.Parallel()

	uc := &UseCase{}

	// Transaction 1: New insert
	tx1ID := uuid.New().String()
	op1ID := uuid.New().String()

	// Transaction 2: Status transition with operations
	tx2ID := uuid.New().String()
	op2ID := uuid.New().String()
	op3ID := uuid.New().String()

	payloads := []transaction.TransactionProcessingPayload{
		// Insert: new transaction
		{
			Transaction: &transaction.Transaction{
				ID:     tx1ID,
				Status: transaction.Status{Code: constant.CREATED},
				Operations: []*operation.Operation{
					{ID: op1ID, TransactionID: tx1ID},
				},
			},
		},
		// Update: pending -> approved with operations
		{
			Transaction: &transaction.Transaction{
				ID:     tx2ID,
				Status: transaction.Status{Code: constant.APPROVED},
				Operations: []*operation.Operation{
					{ID: op2ID, TransactionID: tx2ID},
					{ID: op3ID, TransactionID: tx2ID},
				},
			},
			Validate: &mtransaction.Responses{Pending: true},
			Version:  "v2",
		},
	}

	toInsert, toUpdate := uc.classifyAndExtractEntities(payloads)

	// Verify transaction classification
	assert.Len(t, toInsert.transactions, 1, "One transaction should be insert")
	assert.Len(t, toUpdate.transactions, 1, "One transaction should be update")

	// CRITICAL: ALL operations should be in toInsert.operations (from both insert AND update)
	assert.Len(t, toInsert.operations, 3, "All 3 operations should be collected")

	// Verify all operation IDs
	operationIDs := make(map[string]bool)
	for _, op := range toInsert.operations {
		operationIDs[op.ID] = true
	}
	assert.True(t, operationIDs[op1ID], "Operation from insert should be collected")
	assert.True(t, operationIDs[op2ID], "Operation 1 from status transition should be collected")
	assert.True(t, operationIDs[op3ID], "Operation 2 from status transition should be collected")
}

// bulkMetadataFixture wires CreateBulkTransactionOperationsAsync with gomock repositories
// and records the backup and write-behind keys the cleanup goroutines remove, plus the
// streaming events they emit. It must be built inside a synctest bubble so the test can
// wait deterministically for every goroutine the use case starts.
// Proving that a goroutine-triggered cleanup did NOT happen needs that wait for every
// goroutine, which a wait on expected calls (as in requireFanOutDrains) cannot provide.
type bulkMetadataFixture struct {
	uc      *UseCase
	emitter *pkgStreaming.MockEmitter
	orgID   uuid.UUID
	ledger  uuid.UUID

	mu          sync.Mutex
	removedKeys []string
	deletedKeys []string
}

// newBulkMetadataFixture inserts every transaction in insertedTxIDs, treats the others as
// duplicates, and fails the transaction metadata write of every ID in metadataFailedTxIDs.
func newBulkMetadataFixture(
	t *testing.T,
	allTxIDs []string,
	insertedTxIDs []string,
	metadataFailedTxIDs map[string]struct{},
) *bulkMetadataFixture {
	t.Helper()

	ctrl := gomock.NewController(t)

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockOperationRepo := operation.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	f := &bulkMetadataFixture{
		emitter: pkgStreaming.NewMockEmitter(),
		orgID:   uuid.New(),
		ledger:  uuid.New(),
	}

	f.uc = &UseCase{
		TransactionRepo:         mockTransactionRepo,
		OperationRepo:           mockOperationRepo,
		TransactionMetadataRepo: mockMetadataRepo,
		TransactionRedisRepo:    mockRedisRepo,
		Streaming:               f.emitter,
	}

	mockTx := &mockDBTransaction{}
	mockTransactionRepo.EXPECT().BeginTx(gomock.Any()).Return(mockTx, nil).Times(1)

	mockTransactionRepo.EXPECT().
		CreateBulkTx(gomock.Any(), mockTx, gomock.Any()).
		Return(&repository.BulkInsertResult{
			Attempted:   int64(len(allTxIDs)),
			Inserted:    int64(len(insertedTxIDs)),
			Ignored:     int64(len(allTxIDs) - len(insertedTxIDs)),
			InsertedIDs: insertedTxIDs,
		}, nil).
		Times(1)

	mockOperationRepo.EXPECT().
		CreateBulkTx(gomock.Any(), mockTx, gomock.Any()).
		Return(&repository.BulkInsertResult{Attempted: int64(len(allTxIDs)), Inserted: int64(len(insertedTxIDs))}, nil).
		Times(1)

	// A document-level bulk error routes every entry through the individual fallback,
	// which fails exactly the transactions listed in metadataFailedTxIDs.
	mockMetadataRepo.EXPECT().
		CreateBulk(gomock.Any(), constant.EntityTransaction, gomock.Any()).
		Return(nil, errors.New("bulk insert failed")).
		AnyTimes()

	mockMetadataRepo.EXPECT().
		Create(gomock.Any(), constant.EntityTransaction, gomock.Any()).
		DoAndReturn(func(_ context.Context, _ string, meta *mongodb.Metadata) error {
			if _, fail := metadataFailedTxIDs[meta.EntityID]; fail {
				return errors.New("metadata create failed")
			}

			return nil
		}).
		AnyTimes()

	rawQueue, err := json.Marshal(mmodel.TransactionRedisQueue{TransactionStatus: constant.CREATED})
	require.NoError(t, err)

	mockRedisRepo.EXPECT().ReadMessageFromQueue(gomock.Any(), gomock.Any()).Return(rawQueue, nil).AnyTimes()

	mockRedisRepo.EXPECT().
		RemoveMessageFromQueue(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, key string) error {
			f.mu.Lock()
			defer f.mu.Unlock()

			f.removedKeys = append(f.removedKeys, key)

			return nil
		}).
		AnyTimes()

	mockRedisRepo.EXPECT().
		Del(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, key string) error {
			f.mu.Lock()
			defer f.mu.Unlock()

			f.deletedKeys = append(f.deletedKeys, key)

			return nil
		}).
		AnyTimes()

	return f
}

// payload builds an APPROVED transaction payload with transaction metadata.
func (f *bulkMetadataFixture) payload(txID string) transaction.TransactionProcessingPayload {
	return transaction.TransactionProcessingPayload{
		Transaction: &transaction.Transaction{
			ID:             txID,
			OrganizationID: f.orgID.String(),
			LedgerID:       f.ledger.String(),
			Status:         transaction.Status{Code: constant.APPROVED},
			Metadata:       map[string]any{"reference": "bulk"},
			Operations: []*operation.Operation{
				{ID: uuid.New().String(), TransactionID: txID},
			},
		},
		Validate: &mtransaction.Responses{Aliases: []string{"alias"}},
		Version:  "v2",
	}
}

// cleanedTxIDs returns the transactions whose backup and write-behind entries were removed.
func (f *bulkMetadataFixture) cleanedTxIDs(t *testing.T, txIDs []string) (backup, writeBehind []string) {
	t.Helper()

	f.mu.Lock()
	defer f.mu.Unlock()

	removed := make(map[string]struct{}, len(f.removedKeys))
	for _, k := range f.removedKeys {
		removed[k] = struct{}{}
	}

	deleted := make(map[string]struct{}, len(f.deletedKeys))
	for _, k := range f.deletedKeys {
		deleted[k] = struct{}{}
	}

	for _, id := range txIDs {
		if _, ok := removed[utils.TransactionInternalKey(f.orgID, f.ledger, id)]; ok {
			backup = append(backup, id)
		}

		if _, ok := deleted[utils.WriteBehindTransactionKey(f.orgID, f.ledger, id)]; ok {
			writeBehind = append(writeBehind, id)
		}
	}

	require.Len(t, f.removedKeys, len(backup), "no backup key outside the batch may be removed")
	require.Len(t, f.deletedKeys, len(writeBehind), "no write-behind key outside the batch may be removed")

	return backup, writeBehind
}

// postedSubjects returns the transaction IDs that received a transaction.posted event.
func (f *bulkMetadataFixture) postedSubjects() []string {
	var subjects []string

	for _, evt := range f.emitter.Events() {
		if evt.DefinitionKey == "transaction.posted" {
			subjects = append(subjects, evt.Subject)
		}
	}

	return subjects
}

func TestCreateBulkTransactionOperationsAsync_MetadataFailure_ReportedWithoutChangingError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		okTxID := uuid.New().String()
		failedTxID := uuid.New().String()
		all := []string{okTxID, failedTxID}

		f := newBulkMetadataFixture(t, all, all, map[string]struct{}{failedTxID: {}})

		result, err := f.uc.CreateBulkTransactionOperationsAsync(context.Background(),
			[]transaction.TransactionProcessingPayload{f.payload(okTxID), f.payload(failedTxID)})

		synctest.Wait()

		require.NoError(t, err)
		assert.False(t, result.FallbackUsed)
		assert.Equal(t, int64(0), result.FallbackCount)
		assert.Equal(t, map[string]struct{}{failedTxID: {}}, result.MetadataFailedTransactionIDs)
	})
}

func TestCreateBulkTransactionOperationsAsync_MetadataConfirmed_EmptyFailedSet(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		txID := uuid.New().String()

		f := newBulkMetadataFixture(t, []string{txID}, []string{txID}, nil)

		result, err := f.uc.CreateBulkTransactionOperationsAsync(context.Background(),
			[]transaction.TransactionProcessingPayload{f.payload(txID)})

		synctest.Wait()

		require.NoError(t, err)
		require.NotNil(t, result.MetadataFailedTransactionIDs)
		assert.Empty(t, result.MetadataFailedTransactionIDs)
	})
}

func TestCreateBulkTransactionOperationsAsync_EmptyPayloads_FailedSetInitialised(t *testing.T) {
	t.Parallel()

	uc := &UseCase{}

	result, err := uc.CreateBulkTransactionOperationsAsync(context.Background(), nil)

	require.NoError(t, err)
	require.NotNil(t, result.MetadataFailedTransactionIDs)
	assert.Empty(t, result.MetadataFailedTransactionIDs)
}

func TestCreateBulkTransactionOperationsAsync_MetadataFailure_SkipsCleanupOnlyForFailed(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		insertedOK := uuid.New().String()
		insertedFailed := uuid.New().String()
		duplicateOK := uuid.New().String()
		duplicateFailed := uuid.New().String()
		all := []string{insertedOK, insertedFailed, duplicateOK, duplicateFailed}

		f := newBulkMetadataFixture(t, all, []string{insertedOK, insertedFailed},
			map[string]struct{}{insertedFailed: {}, duplicateFailed: {}})

		payloads := make([]transaction.TransactionProcessingPayload, 0, len(all))
		for _, id := range all {
			payloads = append(payloads, f.payload(id))
		}

		result, err := f.uc.CreateBulkTransactionOperationsAsync(context.Background(), payloads)

		// Every cleanup and event goroutine the call started has finished.
		synctest.Wait()

		require.NoError(t, err)
		assert.Equal(t, map[string]struct{}{insertedFailed: {}, duplicateFailed: {}}, result.MetadataFailedTransactionIDs)

		backup, writeBehind := f.cleanedTxIDs(t, all)
		assert.ElementsMatch(t, []string{insertedOK, duplicateOK}, backup,
			"backup is removed for confirmed payloads, duplicates included, and retained for failed ones")
		assert.ElementsMatch(t, []string{insertedOK, duplicateOK}, writeBehind,
			"write-behind is removed for confirmed payloads, duplicates included, and retained for failed ones")
	})
}

func TestCreateBulkTransactionOperationsAsync_MetadataFailure_StillEmitsEventsForInserted(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		insertedFailed := uuid.New().String()
		insertedOK := uuid.New().String()
		duplicateFailed := uuid.New().String()
		all := []string{insertedFailed, insertedOK, duplicateFailed}

		f := newBulkMetadataFixture(t, all, []string{insertedFailed, insertedOK},
			map[string]struct{}{insertedFailed: {}, duplicateFailed: {}})

		payloads := make([]transaction.TransactionProcessingPayload, 0, len(all))
		for _, id := range all {
			payloads = append(payloads, f.payload(id))
		}

		_, err := f.uc.CreateBulkTransactionOperationsAsync(context.Background(), payloads)

		synctest.Wait()

		require.NoError(t, err)
		assert.ElementsMatch(t, []string{insertedFailed, insertedOK}, f.postedSubjects(),
			"inserted transactions emit regardless of metadata; duplicates do not emit")
	})
}
