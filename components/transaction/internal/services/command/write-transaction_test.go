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
	"github.com/LerianStudio/midaz/v3/pkg/shard"
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

type stubAuthorizerPublisher struct {
	enabled        bool
	publishErr     error
	publishCalls   int
	lastExchange   string
	lastRoutingKey string
	lastPayload    []byte
}

func (s *stubAuthorizerPublisher) Enabled() bool {
	return s.enabled
}

func (s *stubAuthorizerPublisher) PublishBalanceOperations(_ context.Context, exchange, routingKey string, payload []byte, _ map[string]string) error {
	s.publishCalls++
	s.lastExchange = exchange
	s.lastRoutingKey = routingKey
	s.lastPayload = payload

	return s.publishErr
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
	setupMocksForFallbackWithOperation(mockBalanceRepo, mockTransactionRepo, mockMetadataRepo, mockRabbitMQRepo, mockRedisRepo, nil, tran, organizationID, ledgerID)
}

func setupMocksForFallbackWithOperation(
	mockBalanceRepo *balance.MockRepository,
	mockTransactionRepo *transaction.MockRepository,
	mockMetadataRepo *mongodb.MockRepository,
	mockRabbitMQRepo *rabbitmq.MockProducerRepository,
	mockRedisRepo *redis.MockRedisRepository,
	mockOperationRepo *operation.MockRepository,
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

	// Mock OperationRepo.CreateBatch
	if mockOperationRepo != nil {
		mockOperationRepo.EXPECT().
			CreateBatch(gomock.Any(), gomock.Any()).
			Return(nil).
			AnyTimes()
	}

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
	t.Run("routes_to_async_when_flag_true", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		uc := &UseCase{
			RabbitMQRepo:                    mockRabbitMQRepo,
			RedisRepo:                       mockRedisRepo,
			RabbitMQBalanceOperationExchange: "test-exchange",
			RabbitMQBalanceOperationKey:      "test-key",
			TransactionAsync:                true,
		}

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		td := createTestData(organizationID, ledgerID)

		// Expect RabbitMQ producer to be called (async path) with context-aware method
		mockRabbitMQRepo.EXPECT().
			ProducerDefaultWithContext(gomock.Any(), "test-exchange", "test-key", gomock.Any()).
			Return(nil, nil).
			Times(1)

		err := uc.WriteTransaction(ctx, organizationID, ledgerID, td.transactionInput, td.validate, td.balances, td.tran)

		assert.NoError(t, err)
	})

	t.Run("routes_to_async_when_flag_set", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		uc := &UseCase{
			RabbitMQRepo:                    mockRabbitMQRepo,
			RedisRepo:                       mockRedisRepo,
			RabbitMQBalanceOperationExchange: "test-exchange",
			RabbitMQBalanceOperationKey:      "test-key",
			TransactionAsync:                true,
		}

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		td := createTestData(organizationID, ledgerID)

		// Expect RabbitMQ producer to be called (async path) with context-aware method
		mockRabbitMQRepo.EXPECT().
			ProducerDefaultWithContext(gomock.Any(), "test-exchange", "test-key", gomock.Any()).
			Return(nil, nil).
			Times(1)

		err := uc.WriteTransaction(ctx, organizationID, ledgerID, td.transactionInput, td.validate, td.balances, td.tran)

		assert.NoError(t, err)
	})

	t.Run("routes_to_sync_when_flag_false", func(t *testing.T) {

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockTransactionRepo := transaction.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)
		mockOperationRepo := operation.NewMockRepository(ctrl)

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
			OperationRepo:   mockOperationRepo,
		}

		// Setup mocks for sync path (CreateBalanceTransactionOperationsAsync)
		setupMocksForFallbackWithOperation(mockBalanceRepo, mockTransactionRepo, mockMetadataRepo, mockRabbitMQRepo, mockRedisRepo, mockOperationRepo, td.tran, organizationID, ledgerID)

		err := uc.WriteTransaction(ctx, organizationID, ledgerID, td.transactionInput, td.validate, td.balances, td.tran)

		assert.NoError(t, err)
	})

	t.Run("routes_to_sync_when_flag_default", func(t *testing.T) {
		// TransactionAsync defaults to false - should route to sync

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockTransactionRepo := transaction.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)
		mockOperationRepo := operation.NewMockRepository(ctrl)

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
			OperationRepo:   mockOperationRepo,
		}

		// Setup mocks for sync path (CreateBalanceTransactionOperationsAsync)
		setupMocksForFallbackWithOperation(mockBalanceRepo, mockTransactionRepo, mockMetadataRepo, mockRabbitMQRepo, mockRedisRepo, mockOperationRepo, td.tran, organizationID, ledgerID)

		err := uc.WriteTransaction(ctx, organizationID, ledgerID, td.transactionInput, td.validate, td.balances, td.tran)

		assert.NoError(t, err)
	})

	t.Run("routes_to_sync_when_flag_false_explicitly", func(t *testing.T) {
		// TransactionAsync is explicitly false - should route to sync

		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockTransactionRepo := transaction.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)
		mockOperationRepo := operation.NewMockRepository(ctrl)

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
			OperationRepo:   mockOperationRepo,
		}

		// Setup mocks for sync path (CreateBalanceTransactionOperationsAsync)
		setupMocksForFallbackWithOperation(mockBalanceRepo, mockTransactionRepo, mockMetadataRepo, mockRabbitMQRepo, mockRedisRepo, mockOperationRepo, td.tran, organizationID, ledgerID)

		err := uc.WriteTransaction(ctx, organizationID, ledgerID, td.transactionInput, td.validate, td.balances, td.tran)

		assert.NoError(t, err)
	})
}

// TestWriteTransactionAsync tests the async queue publishing with fallback behavior
func TestWriteTransactionAsync(t *testing.T) {
	t.Run("authorizer_enabled_publishes_without_local_rabbitmq", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		authorizer := &stubAuthorizerPublisher{enabled: true}

		uc := &UseCase{
			RabbitMQRepo:                    mockRabbitMQRepo,
			RedisRepo:                       mockRedisRepo,
			Authorizer:                      authorizer,
			RabbitMQBalanceOperationExchange: "test-exchange",
			RabbitMQBalanceOperationKey:      "test-key",
		}

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		td := createTestData(organizationID, ledgerID)

		err := uc.WriteTransactionAsync(ctx, organizationID, ledgerID, td.transactionInput, td.validate, td.balances, td.tran)

		assert.NoError(t, err)
		assert.Equal(t, 1, authorizer.publishCalls)
		assert.Equal(t, "test-exchange", authorizer.lastExchange)
		assert.Equal(t, "test-key", authorizer.lastRoutingKey)
		assert.NotEmpty(t, authorizer.lastPayload)
	})

	t.Run("authorizer_publish_fails_fallback_to_local_rabbitmq", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		authorizer := &stubAuthorizerPublisher{enabled: true, publishErr: errors.New("authorizer unavailable")}

		uc := &UseCase{
			RabbitMQRepo:                    mockRabbitMQRepo,
			RedisRepo:                       mockRedisRepo,
			Authorizer:                      authorizer,
			RabbitMQBalanceOperationExchange: "test-exchange",
			RabbitMQBalanceOperationKey:      "test-key",
		}

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		td := createTestData(organizationID, ledgerID)

		mockRabbitMQRepo.EXPECT().
			ProducerDefaultWithContext(gomock.Any(), "test-exchange", "test-key", gomock.Any()).
			Return(nil, nil).
			Times(1)

		err := uc.WriteTransactionAsync(ctx, organizationID, ledgerID, td.transactionInput, td.validate, td.balances, td.tran)

		assert.NoError(t, err)
		assert.Equal(t, 1, authorizer.publishCalls)
	})

	t.Run("success_publishes_to_queue", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		uc := &UseCase{
			RabbitMQRepo:                    mockRabbitMQRepo,
			RedisRepo:                       mockRedisRepo,
			RabbitMQBalanceOperationExchange: "test-exchange",
			RabbitMQBalanceOperationKey:      "test-key",
		}

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		td := createTestData(organizationID, ledgerID)

		// Expect RabbitMQ producer to be called with correct exchange and key
		mockRabbitMQRepo.EXPECT().
			ProducerDefaultWithContext(gomock.Any(), "test-exchange", "test-key", gomock.Any()).
			Return(nil, nil).
			Times(1)

		err := uc.WriteTransactionAsync(ctx, organizationID, ledgerID, td.transactionInput, td.validate, td.balances, td.tran)

		assert.NoError(t, err)
	})

	t.Run("rabbitmq_fails_fallback_to_db_succeeds", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockTransactionRepo := transaction.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)
		mockOperationRepo := operation.NewMockRepository(ctrl)

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		td := createTestData(organizationID, ledgerID)

		uc := &UseCase{
			BalanceRepo:                     mockBalanceRepo,
			TransactionRepo:                 mockTransactionRepo,
			MetadataRepo:                    mockMetadataRepo,
			RabbitMQRepo:                    mockRabbitMQRepo,
			RedisRepo:                       mockRedisRepo,
			OperationRepo:                   mockOperationRepo,
			RabbitMQBalanceOperationExchange: "test-exchange",
			RabbitMQBalanceOperationKey:      "test-key",
		}

		// RabbitMQ producer fails - triggers fallback
		mockRabbitMQRepo.EXPECT().
			ProducerDefaultWithContext(gomock.Any(), "test-exchange", "test-key", gomock.Any()).
			Return(nil, errors.New("rabbitmq connection failed")).
			Times(1)

		// Setup mocks for fallback path (CreateBalanceTransactionOperationsAsync)
		setupMocksForFallbackWithOperation(mockBalanceRepo, mockTransactionRepo, mockMetadataRepo, mockRabbitMQRepo, mockRedisRepo, mockOperationRepo, td.tran, organizationID, ledgerID)

		err := uc.WriteTransactionAsync(ctx, organizationID, ledgerID, td.transactionInput, td.validate, td.balances, td.tran)

		// Should succeed via fallback
		assert.NoError(t, err)
	})

	t.Run("rabbitmq_fails_fallback_to_db_fails", func(t *testing.T) {
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
			BalanceRepo:                     mockBalanceRepo,
			TransactionRepo:                 mockTransactionRepo,
			MetadataRepo:                    mockMetadataRepo,
			RabbitMQRepo:                    mockRabbitMQRepo,
			RedisRepo:                       mockRedisRepo,
			RabbitMQBalanceOperationExchange: "test-exchange",
			RabbitMQBalanceOperationKey:      "test-key",
		}

		// RabbitMQ producer fails - triggers fallback
		mockRabbitMQRepo.EXPECT().
			ProducerDefaultWithContext(gomock.Any(), "test-exchange", "test-key", gomock.Any()).
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

	t.Run("success_with_empty_config_values", func(t *testing.T) {
		// Test behavior when exchange and key config values are empty
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		uc := &UseCase{
			RabbitMQRepo:                    mockRabbitMQRepo,
			RedisRepo:                       mockRedisRepo,
			RabbitMQBalanceOperationExchange: "",
			RabbitMQBalanceOperationKey:      "",
		}

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		td := createTestData(organizationID, ledgerID)

		// Expect RabbitMQ producer to be called with empty exchange and key
		mockRabbitMQRepo.EXPECT().
			ProducerDefaultWithContext(gomock.Any(), "", "", gomock.Any()).
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
		mockOperationRepo := operation.NewMockRepository(ctrl)

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
			OperationRepo:   mockOperationRepo,
		}

		// Setup mocks for CreateBalanceTransactionOperationsAsync
		setupMocksForFallbackWithOperation(mockBalanceRepo, mockTransactionRepo, mockMetadataRepo, mockRabbitMQRepo, mockRedisRepo, mockOperationRepo, td.tran, organizationID, ledgerID)

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
		mockOperationRepo := operation.NewMockRepository(ctrl)

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
			OperationRepo:   mockOperationRepo,
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

		// Mock OperationRepo.CreateBatch
		mockOperationRepo.EXPECT().
			CreateBatch(gomock.Any(), gomock.Any()).
			Return(nil).
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

// TestResolveBTORoutingKey_Distribution verifies that multiple transaction IDs
// distribute across different shards (not all to the same one), and that
// edge cases like a zero-value router are handled correctly.
func TestResolveBTORoutingKey_Distribution(t *testing.T) {
	t.Parallel()

	baseKey := "balance.transaction.operation"

	t.Run("different_txids_distribute_across_shards", func(t *testing.T) {
		t.Parallel()

		router := shard.NewRouter(8)

		// Generate enough UUIDs to statistically guarantee at least 2 different shards.
		// With 8 shards and 100 random UUIDs, the probability of all mapping to the
		// same shard is (1/8)^99 -- essentially zero.
		routingKeys := make(map[string]struct{})

		for range 100 {
			txID := uuid.New()
			key := resolveBTORoutingKey(baseKey, txID, router, true)
			routingKeys[key] = struct{}{}
		}

		assert.Greater(t, len(routingKeys), 1, "100 random UUIDs must produce at least 2 distinct routing keys across 8 shards")
	})

	t.Run("zero_value_router_returns_base_key", func(t *testing.T) {
		t.Parallel()

		// A zero-value Router has ShardCount()==0, which makes
		// IsShardedBTOQueueEnabled return false.
		router := &shard.Router{}
		txID := uuid.New()
		result := resolveBTORoutingKey(baseKey, txID, router, true)
		assert.Equal(t, baseKey, result, "zero-value router (ShardCount=0) must return base key")
	})

	t.Run("shard_key_format_is_correct", func(t *testing.T) {
		t.Parallel()

		router := shard.NewRouter(8)
		txID := uuid.New()
		result := resolveBTORoutingKey(baseKey, txID, router, true)

		assert.NotEqual(t, baseKey, result, "sharded routing key must differ from base key")
		assert.Contains(t, result, baseKey+".", "sharded key must start with base key plus dot separator")
		assert.Contains(t, result, "shard_", "sharded key must contain shard_ prefix")
	})
}

// TestIsShardedBTOQueueEnabled tests the guard function that determines
// whether sharded BTO queues should be used.
func TestIsShardedBTOQueueEnabled(t *testing.T) {
	t.Parallel()

	t.Run("nil_router_disabled", func(t *testing.T) {
		t.Parallel()

		assert.False(t, IsShardedBTOQueueEnabled(nil, true))
	})

	t.Run("zero_shard_count_disabled", func(t *testing.T) {
		t.Parallel()

		assert.False(t, IsShardedBTOQueueEnabled(&shard.Router{}, true))
	})

	t.Run("valid_router_flag_false_disabled", func(t *testing.T) {
		t.Parallel()

		assert.False(t, IsShardedBTOQueueEnabled(shard.NewRouter(8), false))
	})

	t.Run("valid_router_flag_true_enabled", func(t *testing.T) {
		t.Parallel()

		assert.True(t, IsShardedBTOQueueEnabled(shard.NewRouter(8), true))
	})
}
