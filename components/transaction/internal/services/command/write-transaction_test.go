// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

// testData holds common test data used across multiple tests
type testData struct {
	organizationID   uuid.UUID
	ledgerID         uuid.UUID
	transactionID    string
	transactionInput *pkgTransaction.Transaction
	validate         *pkgTransaction.Responses
	balances         []*mmodel.Balance
	tran             *transaction.Transaction
}

// createTestData creates common test data for transaction write tests
func createTestData(organizationID, ledgerID uuid.UUID) *testData {
	transactionID := uuid.New().String()

	transactionInput := &pkgTransaction.Transaction{}

	validate := &pkgTransaction.Responses{
		Aliases: []string{"alias1", "alias2"},
		From: map[string]pkgTransaction.Amount{
			"alias1": {
				Asset: "USD",
				Value: decimal.NewFromInt(50),
			},
		},
		To: map[string]pkgTransaction.Amount{
			"alias2": {
				Asset: "EUR",
				Value: decimal.NewFromInt(40),
			},
		},
	}

	balances := []*mmodel.Balance{
		{
			ID:             uuid.New().String(),
			AccountID:      uuid.New().String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Alias:          "alias1",
			Available:      decimal.NewFromInt(100),
			OnHold:         decimal.NewFromInt(0),
			Version:        1,
			AccountType:    "deposit",
			AllowSending:   true,
			AllowReceiving: true,
			AssetCode:      "USD",
		},
		{
			ID:             uuid.New().String(),
			AccountID:      uuid.New().String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Alias:          "alias2",
			Available:      decimal.NewFromInt(200),
			OnHold:         decimal.NewFromInt(0),
			Version:        1,
			AccountType:    "deposit",
			AllowSending:   true,
			AllowReceiving: true,
			AssetCode:      "EUR",
		},
	}

	tran := &transaction.Transaction{
		ID:             transactionID,
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		Operations:     []*operation.Operation{},
		Metadata:       map[string]any{},
	}

	return &testData{
		organizationID:   organizationID,
		ledgerID:         ledgerID,
		transactionID:    transactionID,
		transactionInput: transactionInput,
		validate:         validate,
		balances:         balances,
		tran:             tran,
	}
}

// setupMocksForFallback sets up all mocks needed for CreateBalanceTransactionOperationsAsync
// which is called as a fallback when RabbitMQ fails
func setupMocksForFallback(
	mockBalanceRepo *balance.MockRepository,
	mockTransactionRepo *transaction.MockRepository,
	mockMetadataRepo *mongodb.MockRepository,
	mockRabbitMQRepo *rabbitmq.MockProducerRepository,
	mockRedisRepo *redis.MockRedisRepository,
	tran *transaction.Transaction,
	organizationID, ledgerID uuid.UUID,
) {
	// Mock RedisRepo.ListBalanceByKey for stale balance check
	mockRedisRepo.EXPECT().
		ListBalanceByKey(gomock.Any(), organizationID, ledgerID, "alias1").
		Return(nil, nil).
		AnyTimes()
	mockRedisRepo.EXPECT().
		ListBalanceByKey(gomock.Any(), organizationID, ledgerID, "alias2").
		Return(nil, nil).
		AnyTimes()

	// Mock BalanceRepo.BalancesUpdate
	mockBalanceRepo.EXPECT().
		BalancesUpdate(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	// Mock TransactionRepo.Create
	mockTransactionRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		Return(tran, nil).
		AnyTimes()

	// Mock MetadataRepo.Create for transaction metadata
	mockMetadataRepo.EXPECT().
		Create(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	// Mock RabbitMQRepo.ProducerDefault for transaction events (called by SendTransactionEvents)
	mockRabbitMQRepo.EXPECT().
		ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil).
		AnyTimes()

	// Mock RedisRepo.RemoveMessageFromQueue for removing transaction from queue
	mockRedisRepo.EXPECT().
		RemoveMessageFromQueue(gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()
}

// TestWriteTransaction tests the routing logic that decides between async and sync execution
func TestWriteTransaction(t *testing.T) {
	t.Run("routes_to_async_when_env_true", func(t *testing.T) {
		// Set env var to enable async mode
		t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "true")
		t.Setenv("RABBITMQ_TRANSACTION_BALANCE_OPERATION_EXCHANGE", "test-exchange")
		t.Setenv("RABBITMQ_TRANSACTION_BALANCE_OPERATION_KEY", "test-key")

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		uc := &UseCase{
			RabbitMQRepo: mockRabbitMQRepo,
			RedisRepo:    mockRedisRepo,
		}

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		td := createTestData(organizationID, ledgerID)

		// Expect RabbitMQ producer to be called (async path)
		mockRabbitMQRepo.EXPECT().
			ProducerDefault(gomock.Any(), "test-exchange", "test-key", gomock.Any()).
			Return(nil, nil).
			Times(1)

		err := uc.WriteTransaction(ctx, organizationID, ledgerID, td.transactionInput, td.validate, td.balances, td.tran)

		assert.NoError(t, err)
	})

	t.Run("routes_to_async_when_env_TRUE_uppercase", func(t *testing.T) {
		// Test case-insensitivity of env var
		t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "TRUE")
		t.Setenv("RABBITMQ_TRANSACTION_BALANCE_OPERATION_EXCHANGE", "test-exchange")
		t.Setenv("RABBITMQ_TRANSACTION_BALANCE_OPERATION_KEY", "test-key")

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		uc := &UseCase{
			RabbitMQRepo: mockRabbitMQRepo,
			RedisRepo:    mockRedisRepo,
		}

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		td := createTestData(organizationID, ledgerID)

		// Expect RabbitMQ producer to be called (async path)
		mockRabbitMQRepo.EXPECT().
			ProducerDefault(gomock.Any(), "test-exchange", "test-key", gomock.Any()).
			Return(nil, nil).
			Times(1)

		err := uc.WriteTransaction(ctx, organizationID, ledgerID, td.transactionInput, td.validate, td.balances, td.tran)

		assert.NoError(t, err)
	})

	t.Run("routes_to_sync_when_env_false", func(t *testing.T) {
		// Set env var to disable async mode
		t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockTransactionRepo := transaction.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		td := createTestData(organizationID, ledgerID)

		uc := &UseCase{
			BalanceRepo:     mockBalanceRepo,
			TransactionRepo: mockTransactionRepo,
			MetadataRepo:    mockMetadataRepo,
			RabbitMQRepo:    mockRabbitMQRepo,
			RedisRepo:       mockRedisRepo,
		}

		// Setup mocks for sync path (CreateBalanceTransactionOperationsAsync)
		setupMocksForFallback(mockBalanceRepo, mockTransactionRepo, mockMetadataRepo, mockRabbitMQRepo, mockRedisRepo, td.tran, organizationID, ledgerID)

		err := uc.WriteTransaction(ctx, organizationID, ledgerID, td.transactionInput, td.validate, td.balances, td.tran)

		assert.NoError(t, err)
	})

	t.Run("routes_to_sync_when_env_unset", func(t *testing.T) {
		// Do not set RABBITMQ_TRANSACTION_ASYNC - should default to sync

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockTransactionRepo := transaction.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		td := createTestData(organizationID, ledgerID)

		uc := &UseCase{
			BalanceRepo:     mockBalanceRepo,
			TransactionRepo: mockTransactionRepo,
			MetadataRepo:    mockMetadataRepo,
			RabbitMQRepo:    mockRabbitMQRepo,
			RedisRepo:       mockRedisRepo,
		}

		// Setup mocks for sync path (CreateBalanceTransactionOperationsAsync)
		setupMocksForFallback(mockBalanceRepo, mockTransactionRepo, mockMetadataRepo, mockRabbitMQRepo, mockRedisRepo, td.tran, organizationID, ledgerID)

		err := uc.WriteTransaction(ctx, organizationID, ledgerID, td.transactionInput, td.validate, td.balances, td.tran)

		assert.NoError(t, err)
	})

	t.Run("routes_to_sync_when_env_invalid_value", func(t *testing.T) {
		// Set env var to an invalid value - should default to sync
		t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "yes")

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockTransactionRepo := transaction.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		td := createTestData(organizationID, ledgerID)

		uc := &UseCase{
			BalanceRepo:     mockBalanceRepo,
			TransactionRepo: mockTransactionRepo,
			MetadataRepo:    mockMetadataRepo,
			RabbitMQRepo:    mockRabbitMQRepo,
			RedisRepo:       mockRedisRepo,
		}

		// Setup mocks for sync path (CreateBalanceTransactionOperationsAsync)
		setupMocksForFallback(mockBalanceRepo, mockTransactionRepo, mockMetadataRepo, mockRabbitMQRepo, mockRedisRepo, td.tran, organizationID, ledgerID)

		err := uc.WriteTransaction(ctx, organizationID, ledgerID, td.transactionInput, td.validate, td.balances, td.tran)

		assert.NoError(t, err)
	})
}

// TestWriteTransactionAsync tests the async queue publishing with fallback behavior
func TestWriteTransactionAsync(t *testing.T) {
	t.Run("success_publishes_to_queue", func(t *testing.T) {
		t.Setenv("RABBITMQ_TRANSACTION_BALANCE_OPERATION_EXCHANGE", "test-exchange")
		t.Setenv("RABBITMQ_TRANSACTION_BALANCE_OPERATION_KEY", "test-key")

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		uc := &UseCase{
			RabbitMQRepo: mockRabbitMQRepo,
			RedisRepo:    mockRedisRepo,
		}

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		td := createTestData(organizationID, ledgerID)

		// Expect RabbitMQ producer to be called with correct exchange and key
		mockRabbitMQRepo.EXPECT().
			ProducerDefault(gomock.Any(), "test-exchange", "test-key", gomock.Any()).
			Return(nil, nil).
			Times(1)

		err := uc.WriteTransactionAsync(ctx, organizationID, ledgerID, td.transactionInput, td.validate, td.balances, td.tran)

		assert.NoError(t, err)
	})

	t.Run("rabbitmq_fails_fallback_to_db_succeeds", func(t *testing.T) {
		t.Setenv("RABBITMQ_TRANSACTION_BALANCE_OPERATION_EXCHANGE", "test-exchange")
		t.Setenv("RABBITMQ_TRANSACTION_BALANCE_OPERATION_KEY", "test-key")

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockTransactionRepo := transaction.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		td := createTestData(organizationID, ledgerID)

		uc := &UseCase{
			BalanceRepo:     mockBalanceRepo,
			TransactionRepo: mockTransactionRepo,
			MetadataRepo:    mockMetadataRepo,
			RabbitMQRepo:    mockRabbitMQRepo,
			RedisRepo:       mockRedisRepo,
		}

		// RabbitMQ producer fails - triggers fallback
		mockRabbitMQRepo.EXPECT().
			ProducerDefault(gomock.Any(), "test-exchange", "test-key", gomock.Any()).
			Return(nil, errors.New("rabbitmq connection failed")).
			Times(1)

		// Setup mocks for fallback path (CreateBalanceTransactionOperationsAsync)
		setupMocksForFallback(mockBalanceRepo, mockTransactionRepo, mockMetadataRepo, mockRabbitMQRepo, mockRedisRepo, td.tran, organizationID, ledgerID)

		err := uc.WriteTransactionAsync(ctx, organizationID, ledgerID, td.transactionInput, td.validate, td.balances, td.tran)

		// Should succeed via fallback
		assert.NoError(t, err)
	})

	t.Run("rabbitmq_fails_fallback_to_db_fails", func(t *testing.T) {
		t.Setenv("RABBITMQ_TRANSACTION_BALANCE_OPERATION_EXCHANGE", "test-exchange")
		t.Setenv("RABBITMQ_TRANSACTION_BALANCE_OPERATION_KEY", "test-key")

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockTransactionRepo := transaction.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		td := createTestData(organizationID, ledgerID)

		uc := &UseCase{
			BalanceRepo:     mockBalanceRepo,
			TransactionRepo: mockTransactionRepo,
			MetadataRepo:    mockMetadataRepo,
			RabbitMQRepo:    mockRabbitMQRepo,
			RedisRepo:       mockRedisRepo,
		}

		// RabbitMQ producer fails - triggers fallback
		mockRabbitMQRepo.EXPECT().
			ProducerDefault(gomock.Any(), "test-exchange", "test-key", gomock.Any()).
			Return(nil, errors.New("rabbitmq connection failed")).
			Times(1)

		// Mock RedisRepo.ListBalanceByKey for stale balance check
		mockRedisRepo.EXPECT().
			ListBalanceByKey(gomock.Any(), organizationID, ledgerID, "alias1").
			Return(nil, nil).
			AnyTimes()
		mockRedisRepo.EXPECT().
			ListBalanceByKey(gomock.Any(), organizationID, ledgerID, "alias2").
			Return(nil, nil).
			AnyTimes()

		// Fallback also fails - BalancesUpdate returns error
		mockBalanceRepo.EXPECT().
			BalancesUpdate(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(errors.New("database connection failed")).
			Times(1)

		err := uc.WriteTransactionAsync(ctx, organizationID, ledgerID, td.transactionInput, td.validate, td.balances, td.tran)

		// Should return error from fallback
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "database connection failed")
	})

	t.Run("success_with_empty_env_vars", func(t *testing.T) {
		// Test behavior when exchange and key env vars are empty
		t.Setenv("RABBITMQ_TRANSACTION_BALANCE_OPERATION_EXCHANGE", "")
		t.Setenv("RABBITMQ_TRANSACTION_BALANCE_OPERATION_KEY", "")

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		uc := &UseCase{
			RabbitMQRepo: mockRabbitMQRepo,
			RedisRepo:    mockRedisRepo,
		}

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		td := createTestData(organizationID, ledgerID)

		// Expect RabbitMQ producer to be called with empty exchange and key
		mockRabbitMQRepo.EXPECT().
			ProducerDefault(gomock.Any(), "", "", gomock.Any()).
			Return(nil, nil).
			Times(1)

		err := uc.WriteTransactionAsync(ctx, organizationID, ledgerID, td.transactionInput, td.validate, td.balances, td.tran)

		assert.NoError(t, err)
	})
}

// TestWriteTransactionSync tests the synchronous direct DB write path
func TestWriteTransactionSync(t *testing.T) {
	t.Run("success_writes_directly_to_db", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockTransactionRepo := transaction.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		td := createTestData(organizationID, ledgerID)

		uc := &UseCase{
			BalanceRepo:     mockBalanceRepo,
			TransactionRepo: mockTransactionRepo,
			MetadataRepo:    mockMetadataRepo,
			RabbitMQRepo:    mockRabbitMQRepo,
			RedisRepo:       mockRedisRepo,
		}

		// Setup mocks for CreateBalanceTransactionOperationsAsync
		setupMocksForFallback(mockBalanceRepo, mockTransactionRepo, mockMetadataRepo, mockRabbitMQRepo, mockRedisRepo, td.tran, organizationID, ledgerID)

		err := uc.WriteTransactionSync(ctx, organizationID, ledgerID, td.transactionInput, td.validate, td.balances, td.tran)

		assert.NoError(t, err)
	})

	t.Run("error_propagates_from_balance_update", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockTransactionRepo := transaction.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		td := createTestData(organizationID, ledgerID)

		uc := &UseCase{
			BalanceRepo:     mockBalanceRepo,
			TransactionRepo: mockTransactionRepo,
			MetadataRepo:    mockMetadataRepo,
			RabbitMQRepo:    mockRabbitMQRepo,
			RedisRepo:       mockRedisRepo,
		}

		// Mock RedisRepo.ListBalanceByKey for stale balance check
		mockRedisRepo.EXPECT().
			ListBalanceByKey(gomock.Any(), organizationID, ledgerID, "alias1").
			Return(nil, nil).
			AnyTimes()
		mockRedisRepo.EXPECT().
			ListBalanceByKey(gomock.Any(), organizationID, ledgerID, "alias2").
			Return(nil, nil).
			AnyTimes()

		// BalancesUpdate fails
		mockBalanceRepo.EXPECT().
			BalancesUpdate(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(errors.New("failed to update balances")).
			Times(1)

		err := uc.WriteTransactionSync(ctx, organizationID, ledgerID, td.transactionInput, td.validate, td.balances, td.tran)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update balances")
	})

	t.Run("error_propagates_from_transaction_create", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockTransactionRepo := transaction.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		td := createTestData(organizationID, ledgerID)

		uc := &UseCase{
			BalanceRepo:     mockBalanceRepo,
			TransactionRepo: mockTransactionRepo,
			MetadataRepo:    mockMetadataRepo,
			RabbitMQRepo:    mockRabbitMQRepo,
			RedisRepo:       mockRedisRepo,
		}

		// Mock RedisRepo.ListBalanceByKey for stale balance check
		mockRedisRepo.EXPECT().
			ListBalanceByKey(gomock.Any(), organizationID, ledgerID, "alias1").
			Return(nil, nil).
			AnyTimes()
		mockRedisRepo.EXPECT().
			ListBalanceByKey(gomock.Any(), organizationID, ledgerID, "alias2").
			Return(nil, nil).
			AnyTimes()

		// BalancesUpdate succeeds
		mockBalanceRepo.EXPECT().
			BalancesUpdate(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil).
			Times(1)

		// TransactionRepo.Create fails (not a duplicate key error)
		mockTransactionRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			Return(nil, errors.New("failed to create transaction")).
			Times(1)

		err := uc.WriteTransactionSync(ctx, organizationID, ledgerID, td.transactionInput, td.validate, td.balances, td.tran)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create transaction")
	})

	t.Run("success_with_single_balance", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockTransactionRepo := transaction.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		transactionID := uuid.New().String()

		// Create minimal test data with single balance
		transactionInput := &pkgTransaction.Transaction{}
		validate := &pkgTransaction.Responses{
			Aliases: []string{"alias1"},
			From: map[string]pkgTransaction.Amount{
				"alias1": {
					Asset: "USD",
					Value: decimal.NewFromInt(50),
				},
			},
		}
		balances := []*mmodel.Balance{
			{
				ID:             uuid.New().String(),
				AccountID:      uuid.New().String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Alias:          "alias1",
				Available:      decimal.NewFromInt(100),
				OnHold:         decimal.NewFromInt(0),
				Version:        1,
				AccountType:    "deposit",
				AllowSending:   true,
				AllowReceiving: true,
				AssetCode:      "USD",
			},
		}
		tran := &transaction.Transaction{
			ID:             transactionID,
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Operations:     []*operation.Operation{},
			Metadata:       map[string]any{},
		}

		uc := &UseCase{
			BalanceRepo:     mockBalanceRepo,
			TransactionRepo: mockTransactionRepo,
			MetadataRepo:    mockMetadataRepo,
			RabbitMQRepo:    mockRabbitMQRepo,
			RedisRepo:       mockRedisRepo,
		}

		// Mock RedisRepo.ListBalanceByKey for stale balance check
		mockRedisRepo.EXPECT().
			ListBalanceByKey(gomock.Any(), organizationID, ledgerID, "alias1").
			Return(nil, nil).
			AnyTimes()

		// Mock BalanceRepo.BalancesUpdate
		mockBalanceRepo.EXPECT().
			BalancesUpdate(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil).
			AnyTimes()

		// Mock TransactionRepo.Create
		mockTransactionRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			Return(tran, nil).
			AnyTimes()

		// Mock MetadataRepo.Create
		mockMetadataRepo.EXPECT().
			Create(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil).
			AnyTimes()

		// Mock RabbitMQRepo.ProducerDefault for transaction events
		mockRabbitMQRepo.EXPECT().
			ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, nil).
			AnyTimes()

		// Mock RedisRepo.RemoveMessageFromQueue
		mockRedisRepo.EXPECT().
			RemoveMessageFromQueue(gomock.Any(), gomock.Any()).
			Return(nil).
			AnyTimes()

		err := uc.WriteTransactionSync(ctx, organizationID, ledgerID, transactionInput, validate, balances, tran)

		assert.NoError(t, err)
	})
}
