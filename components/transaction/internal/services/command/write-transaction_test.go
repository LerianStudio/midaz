// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redpanda"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
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
	enabled          bool
	publishErr       error
	publishCalls     int
	lastTopic        string
	lastPartitionKey string
	lastPayload      []byte
}

func (s *stubAuthorizerPublisher) Enabled() bool {
	return s.enabled
}

func (s *stubAuthorizerPublisher) PublishBalanceOperations(_ context.Context, topic, partitionKey string, payload []byte, _ map[string]string) error {
	s.publishCalls++
	s.lastTopic = topic
	s.lastPartitionKey = partitionKey
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
// which is called as a fallback when broker publishing fails
func setupMocksForFallback(
	mockBalanceRepo *balance.MockRepository,
	mockTransactionRepo *transaction.MockRepository,
	mockMetadataRepo *mongodb.MockRepository,
	mockBrokerRepo *redpanda.MockProducerRepository,
	mockRedisRepo *redis.MockRedisRepository,
	tran *transaction.Transaction,
	organizationID, ledgerID uuid.UUID,
) {
	setupMocksForFallbackWithOperation(mockBalanceRepo, mockTransactionRepo, mockMetadataRepo, mockBrokerRepo, mockRedisRepo, nil, tran, organizationID, ledgerID)
}

func setupMocksForFallbackWithOperation(
	mockBalanceRepo *balance.MockRepository,
	mockTransactionRepo *transaction.MockRepository,
	mockMetadataRepo *mongodb.MockRepository,
	mockBrokerRepo *redpanda.MockProducerRepository,
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
		Times(1)

	// Mock TransactionRepo.Create
	mockTransactionRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		Return(tran, nil).
		Times(1)

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

	// Mock BrokerRepo.ProducerDefault for transaction events (called by SendTransactionEvents)
	mockBrokerRepo.EXPECT().
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

		mockBrokerRepo := redpanda.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		uc := &UseCase{
			BrokerRepo:             mockBrokerRepo,
			RedisRepo:              mockRedisRepo,
			BalanceOperationsTopic: "test-topic",
			TransactionAsync:       true,
		}

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		td := createTestData(organizationID, ledgerID)

		// Expect broker producer to be called (async path) with context-aware method
		mockBrokerRepo.EXPECT().
			ProducerDefaultWithContext(gomock.Any(), "test-topic", td.transactionID, gomock.Any()).
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
		mockBrokerRepo := redpanda.NewMockProducerRepository(ctrl)
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
			BrokerRepo:      mockBrokerRepo,
			RedisRepo:       mockRedisRepo,
			OperationRepo:   mockOperationRepo,
		}

		// Setup mocks for sync path (CreateBalanceTransactionOperationsAsync)
		setupMocksForFallbackWithOperation(mockBalanceRepo, mockTransactionRepo, mockMetadataRepo, mockBrokerRepo, mockRedisRepo, mockOperationRepo, td.tran, organizationID, ledgerID)

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
		mockBrokerRepo := redpanda.NewMockProducerRepository(ctrl)
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
			BrokerRepo:      mockBrokerRepo,
			RedisRepo:       mockRedisRepo,
			OperationRepo:   mockOperationRepo,
		}

		// Setup mocks for sync path (CreateBalanceTransactionOperationsAsync)
		setupMocksForFallbackWithOperation(mockBalanceRepo, mockTransactionRepo, mockMetadataRepo, mockBrokerRepo, mockRedisRepo, mockOperationRepo, td.tran, organizationID, ledgerID)

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
		mockBrokerRepo := redpanda.NewMockProducerRepository(ctrl)
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
			BrokerRepo:      mockBrokerRepo,
			RedisRepo:       mockRedisRepo,
			OperationRepo:   mockOperationRepo,
		}

		// Setup mocks for sync path (CreateBalanceTransactionOperationsAsync)
		setupMocksForFallbackWithOperation(mockBalanceRepo, mockTransactionRepo, mockMetadataRepo, mockBrokerRepo, mockRedisRepo, mockOperationRepo, td.tran, organizationID, ledgerID)

		err := uc.WriteTransaction(ctx, organizationID, ledgerID, td.transactionInput, td.validate, td.balances, td.tran)

		assert.NoError(t, err)
	})

	t.Run("returns_error_when_transaction_is_nil", func(t *testing.T) {
		uc := &UseCase{}

		err := uc.WriteTransaction(context.Background(), uuid.New(), uuid.New(), &pkgTransaction.Transaction{}, &pkgTransaction.Responses{}, nil, nil)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "transaction is nil")
	})
}

// TestWriteTransactionAsync tests the async queue publishing with fallback behavior
func TestWriteTransactionAsync(t *testing.T) {
	t.Run("authorizer_enabled_publishes_without_local_broker", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBrokerRepo := redpanda.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		authorizer := &stubAuthorizerPublisher{enabled: true}

		uc := &UseCase{
			BrokerRepo:             mockBrokerRepo,
			RedisRepo:              mockRedisRepo,
			Authorizer:             authorizer,
			BalanceOperationsTopic: "test-topic",
		}

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		td := createTestData(organizationID, ledgerID)

		err := uc.WriteTransactionAsync(ctx, organizationID, ledgerID, td.transactionInput, td.validate, td.balances, td.tran)

		assert.NoError(t, err)
		assert.Equal(t, 1, authorizer.publishCalls)
		assert.Equal(t, "test-topic", authorizer.lastTopic)
		assert.Equal(t, td.transactionID, authorizer.lastPartitionKey)
		assert.NotEmpty(t, authorizer.lastPayload)
	})

	t.Run("uses_shard_router_resolve_balance_for_partition_key", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBrokerRepo := redpanda.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		authorizer := &stubAuthorizerPublisher{enabled: true}
		router := shard.NewRouter(8)

		uc := &UseCase{
			BrokerRepo:             mockBrokerRepo,
			RedisRepo:              mockRedisRepo,
			Authorizer:             authorizer,
			ShardRouter:            router,
			BalanceOperationsTopic: "test-topic",
		}

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		td := createTestData(organizationID, ledgerID)
		td.tran.Operations = []*operation.Operation{
			{
				AccountAlias: "@external/USD",
				BalanceKey:   shard.ExternalBalanceKey(3),
				Type:         constant.DEBIT,
			},
		}

		err := uc.WriteTransactionAsync(ctx, organizationID, ledgerID, td.transactionInput, td.validate, td.balances, td.tran)

		assert.NoError(t, err)
		assert.Equal(t, strconv.Itoa(3), authorizer.lastPartitionKey)
	})

	t.Run("authorizer_publish_fails_fallback_to_local_broker", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBrokerRepo := redpanda.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		authorizer := &stubAuthorizerPublisher{enabled: true, publishErr: errors.New("authorizer unavailable")}

		uc := &UseCase{
			BrokerRepo:             mockBrokerRepo,
			RedisRepo:              mockRedisRepo,
			Authorizer:             authorizer,
			BalanceOperationsTopic: "test-topic",
		}

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		td := createTestData(organizationID, ledgerID)

		mockBrokerRepo.EXPECT().
			ProducerDefaultWithContext(gomock.Any(), "test-topic", td.transactionID, gomock.Any()).
			Return(nil, nil).
			Times(1)

		err := uc.WriteTransactionAsync(ctx, organizationID, ledgerID, td.transactionInput, td.validate, td.balances, td.tran)

		assert.NoError(t, err)
		assert.Equal(t, 1, authorizer.publishCalls)
	})

	t.Run("success_publishes_to_queue", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBrokerRepo := redpanda.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		uc := &UseCase{
			BrokerRepo:             mockBrokerRepo,
			RedisRepo:              mockRedisRepo,
			BalanceOperationsTopic: "test-topic",
		}

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		td := createTestData(organizationID, ledgerID)

		// Expect broker producer to be called with correct topic and key
		mockBrokerRepo.EXPECT().
			ProducerDefaultWithContext(gomock.Any(), "test-topic", td.transactionID, gomock.Any()).
			Return(nil, nil).
			Times(1)

		err := uc.WriteTransactionAsync(ctx, organizationID, ledgerID, td.transactionInput, td.validate, td.balances, td.tran)

		assert.NoError(t, err)
	})

	t.Run("broker_fails_fallback_to_db_succeeds", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockTransactionRepo := transaction.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		mockBrokerRepo := redpanda.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)
		mockOperationRepo := operation.NewMockRepository(ctrl)

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		td := createTestData(organizationID, ledgerID)

		uc := &UseCase{
			BalanceRepo:            mockBalanceRepo,
			TransactionRepo:        mockTransactionRepo,
			MetadataRepo:           mockMetadataRepo,
			BrokerRepo:             mockBrokerRepo,
			RedisRepo:              mockRedisRepo,
			OperationRepo:          mockOperationRepo,
			BalanceOperationsTopic: "test-topic",
		}

		// Broker producer fails - triggers fallback
		mockBrokerRepo.EXPECT().
			ProducerDefaultWithContext(gomock.Any(), "test-topic", td.transactionID, gomock.Any()).
			Return(nil, errors.New("broker connection failed")).
			Times(1)

		// Setup mocks for fallback path (CreateBalanceTransactionOperationsAsync)
		setupMocksForFallbackWithOperation(mockBalanceRepo, mockTransactionRepo, mockMetadataRepo, mockBrokerRepo, mockRedisRepo, mockOperationRepo, td.tran, organizationID, ledgerID)

		err := uc.WriteTransactionAsync(ctx, organizationID, ledgerID, td.transactionInput, td.validate, td.balances, td.tran)

		// Should succeed via fallback
		assert.NoError(t, err)
	})

	t.Run("broker_fails_fallback_to_db_fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockTransactionRepo := transaction.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		mockBrokerRepo := redpanda.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		td := createTestData(organizationID, ledgerID)

		uc := &UseCase{
			BalanceRepo:            mockBalanceRepo,
			TransactionRepo:        mockTransactionRepo,
			MetadataRepo:           mockMetadataRepo,
			BrokerRepo:             mockBrokerRepo,
			RedisRepo:              mockRedisRepo,
			BalanceOperationsTopic: "test-topic",
		}

		// Broker producer fails - triggers fallback
		mockBrokerRepo.EXPECT().
			ProducerDefaultWithContext(gomock.Any(), "test-topic", td.transactionID, gomock.Any()).
			Return(nil, errors.New("broker connection failed")).
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
		// Test behavior when topic and partition key config values are empty
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBrokerRepo := redpanda.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		uc := &UseCase{
			BrokerRepo:             mockBrokerRepo,
			RedisRepo:              mockRedisRepo,
			BalanceOperationsTopic: "",
		}

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		td := createTestData(organizationID, ledgerID)

		// Expect broker producer to be called with empty topic and transaction key
		mockBrokerRepo.EXPECT().
			ProducerDefaultWithContext(gomock.Any(), "", td.transactionID, gomock.Any()).
			Return(nil, nil).
			Times(1)

		err := uc.WriteTransactionAsync(ctx, organizationID, ledgerID, td.transactionInput, td.validate, td.balances, td.tran)

		assert.NoError(t, err)
	})

	t.Run("returns_error_when_msgpack_marshal_fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBrokerRepo := redpanda.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		uc := &UseCase{
			BrokerRepo:             mockBrokerRepo,
			RedisRepo:              mockRedisRepo,
			BalanceOperationsTopic: "test-topic",
		}

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		td := createTestData(organizationID, ledgerID)
		td.transactionInput.Metadata = map[string]any{"unsupported": make(chan int)}

		err := uc.WriteTransactionAsync(ctx, organizationID, ledgerID, td.transactionInput, td.validate, td.balances, td.tran)

		assert.Error(t, err)
	})

	t.Run("returns_error_when_transaction_is_nil", func(t *testing.T) {
		uc := &UseCase{}

		err := uc.WriteTransactionAsync(context.Background(), uuid.New(), uuid.New(), &pkgTransaction.Transaction{}, &pkgTransaction.Responses{}, nil, nil)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "transaction is nil")
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
		mockBrokerRepo := redpanda.NewMockProducerRepository(ctrl)
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
			BrokerRepo:      mockBrokerRepo,
			RedisRepo:       mockRedisRepo,
			OperationRepo:   mockOperationRepo,
		}

		// Setup mocks for CreateBalanceTransactionOperationsAsync
		setupMocksForFallbackWithOperation(mockBalanceRepo, mockTransactionRepo, mockMetadataRepo, mockBrokerRepo, mockRedisRepo, mockOperationRepo, td.tran, organizationID, ledgerID)

		err := uc.WriteTransactionSync(ctx, organizationID, ledgerID, td.transactionInput, td.validate, td.balances, td.tran)

		assert.NoError(t, err)
	})

	t.Run("error_propagates_from_balance_update", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockTransactionRepo := transaction.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		mockBrokerRepo := redpanda.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		td := createTestData(organizationID, ledgerID)

		uc := &UseCase{
			BalanceRepo:     mockBalanceRepo,
			TransactionRepo: mockTransactionRepo,
			MetadataRepo:    mockMetadataRepo,
			BrokerRepo:      mockBrokerRepo,
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
		mockBrokerRepo := redpanda.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		td := createTestData(organizationID, ledgerID)

		uc := &UseCase{
			BalanceRepo:     mockBalanceRepo,
			TransactionRepo: mockTransactionRepo,
			MetadataRepo:    mockMetadataRepo,
			BrokerRepo:      mockBrokerRepo,
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
		mockBrokerRepo := redpanda.NewMockProducerRepository(ctrl)
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
			BrokerRepo:      mockBrokerRepo,
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

		// Mock BrokerRepo.ProducerDefault for transaction events
		mockBrokerRepo.EXPECT().
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

func TestExtractPrimaryDebitRoute(t *testing.T) {
	t.Run("returns empty when transaction is nil", func(t *testing.T) {
		alias, balanceKey := extractPrimaryDebitRoute(nil)
		assert.Equal(t, "", alias)
		assert.Equal(t, "", balanceKey)
	})

	t.Run("returns empty when operations are empty", func(t *testing.T) {
		tran := &transaction.Transaction{}
		alias, balanceKey := extractPrimaryDebitRoute(tran)
		assert.Equal(t, "", alias)
		assert.Equal(t, "", balanceKey)
	})

	t.Run("prefers internal debit alias", func(t *testing.T) {
		tran := &transaction.Transaction{
			Operations: []*operation.Operation{
				{AccountAlias: "@external/USD", BalanceKey: shard.ExternalBalanceKey(3), Type: constant.DEBIT},
				{AccountAlias: "@internal-src", BalanceKey: "default", Type: constant.DEBIT},
				{AccountAlias: "@internal-dst", Type: constant.CREDIT},
			},
		}

		alias, balanceKey := extractPrimaryDebitRoute(tran)
		assert.Equal(t, "@internal-src", alias)
		assert.Equal(t, "default", balanceKey)
	})

	t.Run("falls back to first non-empty alias for external-only operations", func(t *testing.T) {
		tran := &transaction.Transaction{
			Operations: []*operation.Operation{
				{AccountAlias: "@external/USD", BalanceKey: shard.ExternalBalanceKey(5), Type: constant.DEBIT},
				{AccountAlias: "@external/EUR", Type: constant.CREDIT},
			},
		}

		alias, balanceKey := extractPrimaryDebitRoute(tran)
		assert.Equal(t, "@external/USD", alias)
		assert.Equal(t, shard.ExternalBalanceKey(5), balanceKey)
	})

	t.Run("ignores nil operations and blank aliases", func(t *testing.T) {
		tran := &transaction.Transaction{
			Operations: []*operation.Operation{
				nil,
				{AccountAlias: "", Type: constant.DEBIT},
				{AccountAlias: "@valid", Type: constant.CREDIT},
			},
		}

		alias, balanceKey := extractPrimaryDebitRoute(tran)
		assert.Equal(t, "@valid", alias)
		assert.Equal(t, constant.DefaultBalanceKey, balanceKey)
	})
}
