// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	mongodb "github.com/LerianStudio/midaz/v3/components/ledger/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v3/components/ledger/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/ledger/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/ledger/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/ledger/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/v3/components/ledger/adapters/redis/transaction"
	redis "github.com/LerianStudio/midaz/v3/components/ledger/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
	"go.uber.org/mock/gomock"
)

// Int64Ptr returns a pointer to the given int64 value
func Int64Ptr(v int64) *int64 {
	return &v
}

// MockLogger is a mock implementation of logger for testing
type MockLogger struct{}

func (m *MockLogger) Log(_ context.Context, _ libLog.Level, _ string, _ ...libLog.Field) {}
func (m *MockLogger) With(_ ...libLog.Field) libLog.Logger                               { return m }
func (m *MockLogger) WithGroup(_ string) libLog.Logger                                   { return m }
func (m *MockLogger) Enabled(_ libLog.Level) bool                                        { return true }
func (m *MockLogger) Sync(_ context.Context) error                                       { return nil }

func TestCreateBalanceTransactionOperationsAsync(t *testing.T) {
	t.Run("success_append_only_transaction_and_operations", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockTransactionRepo := transaction.NewMockRepository(ctrl)
		mockOperationRepo := operation.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		// Create a UseCase with all required dependencies
		uc := &UseCase{
			TransactionRepo: mockTransactionRepo,
			OperationRepo:   mockOperationRepo,
			MetadataRepo:    mockMetadataRepo,
			BalanceRepo:     mockBalanceRepo,
			RabbitMQRepo:    mockRabbitMQRepo,
			RedisRepo:       mockRedisRepo,
		}

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		transactionID := uuid.New().String()

		// Mock transaction data with correct types
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
			Metadata:       map[string]interface{}{},
		}

		transactionInput := &pkgTransaction.Transaction{}

		// Create a transaction queue with the necessary fields
		transactionQueue := transaction.TransactionProcessingPayload{
			Transaction: tran,
			Validate:    validate,
			Balances:    balances,
			Input:       transactionInput,
		}

		transactionBytes, marshalErr := msgpack.Marshal(transactionQueue)
		require.NoError(t, marshalErr, "failed to marshal transaction queue")
		queueData := []mmodel.QueueData{
			{
				ID:    uuid.New(),
				Value: transactionBytes,
			},
		}

		queue := mmodel.Queue{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			QueueData:      queueData,
		}

		// Mock BalanceRepo.BalancesUpdate (called by UpdateBalances before transaction create)
		mockBalanceRepo.EXPECT().
			BalancesUpdate(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil).
			Times(1)

		// Mock TransactionRepo.Create
		mockTransactionRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			Return(tran, nil).
			Times(1)

		// Mock MetadataRepo.Create for transaction metadata
		mockMetadataRepo.EXPECT().
			Create(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil).
			AnyTimes()

		// Mock RabbitMQRepo.ProducerDefault for transaction events
		mockRabbitMQRepo.EXPECT().
			ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, nil).
			AnyTimes()

		// Mock RedisRepo.RemoveMessageFromQueue for removing transaction from queue
		mockRedisRepo.EXPECT().
			RemoveMessageFromQueue(gomock.Any(), gomock.Any()).
			Return(nil).
			AnyTimes()

		// Mock RedisRepo.Del for removing transaction from write-behind cache
		mockRedisRepo.EXPECT().
			Del(gomock.Any(), gomock.Any()).
			Return(nil).
			AnyTimes()

		// Call the method
		err := uc.CreateBalanceTransactionOperationsAsync(ctx, queue)

		assert.NoError(t, err)
	})

	t.Run("error_duplicate_transaction", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockTransactionRepo := transaction.NewMockRepository(ctrl)
		mockOperationRepo := operation.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		// Create a UseCase with all required dependencies
		uc := &UseCase{
			TransactionRepo: mockTransactionRepo,
			OperationRepo:   mockOperationRepo,
			MetadataRepo:    mockMetadataRepo,
			BalanceRepo:     mockBalanceRepo,
			RabbitMQRepo:    mockRabbitMQRepo,
			RedisRepo:       mockRedisRepo,
		}

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		transactionID := uuid.New().String()

		// Mock transaction data with correct types
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
			Metadata:       map[string]interface{}{},
		}

		transactionInput := &pkgTransaction.Transaction{}

		transactionQueue := transaction.TransactionProcessingPayload{
			Transaction: tran,
			Validate:    validate,
			Balances:    balances,
			Input:       transactionInput,
		}

		transactionBytes, marshalErr := msgpack.Marshal(transactionQueue)
		require.NoError(t, marshalErr, "failed to marshal transaction queue")
		queueData := []mmodel.QueueData{
			{
				ID:    uuid.New(),
				Value: transactionBytes,
			},
		}

		queue := mmodel.Queue{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			QueueData:      queueData,
		}

		// Mock BalanceRepo.BalancesUpdate (called by UpdateBalances before transaction create)
		mockBalanceRepo.EXPECT().
			BalancesUpdate(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil).
			Times(1)

		// Mock TransactionRepo.Create with duplicate key error
		pgErr := &pgconn.PgError{Code: "23505"}
		mockTransactionRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			Return(nil, pgErr).
			Times(1)

		// Mock MetadataRepo.Create for transaction metadata (should be called even with duplicate error)
		mockMetadataRepo.EXPECT().
			Create(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil).
			Times(1)

		// Mock RabbitMQRepo.ProducerDefault for transaction events (goroutine will still be called)
		mockRabbitMQRepo.EXPECT().
			ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, nil).
			AnyTimes()

		// Mock RedisRepo.RemoveMessageFromQueue for removing transaction from queue
		mockRedisRepo.EXPECT().
			RemoveMessageFromQueue(gomock.Any(), gomock.Any()).
			Return(nil).
			AnyTimes()

		// Mock RedisRepo.Del for removing transaction from write-behind cache
		mockRedisRepo.EXPECT().
			Del(gomock.Any(), gomock.Any()).
			Return(nil).
			AnyTimes()

		err := uc.CreateBalanceTransactionOperationsAsync(ctx, queue)

		assert.NoError(t, err) // Duplicate key errors are handled gracefully
	})

	t.Run("success_with_multiple_operations", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockTransactionRepo := transaction.NewMockRepository(ctrl)
		mockOperationRepo := operation.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		// Create a UseCase with all required dependencies
		uc := &UseCase{
			TransactionRepo: mockTransactionRepo,
			OperationRepo:   mockOperationRepo,
			MetadataRepo:    mockMetadataRepo,
			BalanceRepo:     mockBalanceRepo,
			RabbitMQRepo:    mockRabbitMQRepo,
			RedisRepo:       mockRedisRepo,
		}

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		transactionID := uuid.New().String()

		// Mock transaction data with correct types
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

		// Create operations for the transaction
		Amount := decimal.NewFromInt(50)
		operation1 := &operation.Operation{
			ID:             uuid.New().String(),
			TransactionID:  transactionID,
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      uuid.New().String(),
			Type:           "debit",
			AssetCode:      "USD",
			Amount: operation.Amount{
				Value: &Amount,
			},
			Balance: operation.Balance{ // ensure version before is present
				Version: Int64Ptr(1),
			},
			BalanceAfter: operation.Balance{ // ensure version after is present
				Version: Int64Ptr(2),
			},
			Metadata: map[string]interface{}{"key1": "value1"},
		}

		Amount = decimal.NewFromInt(40)
		operation2 := &operation.Operation{
			ID:             uuid.New().String(),
			TransactionID:  transactionID,
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      uuid.New().String(),
			Type:           "credit",
			AssetCode:      "EUR",
			Amount: operation.Amount{
				Value: &Amount,
			},
			Balance: operation.Balance{ // ensure version before is present
				Version: Int64Ptr(1),
			},
			BalanceAfter: operation.Balance{ // ensure version after is present
				Version: Int64Ptr(2),
			},
			Metadata: map[string]interface{}{"key2": "value2"},
		}

		tran := &transaction.Transaction{
			ID:             transactionID,
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Operations:     []*operation.Operation{operation1, operation2},
			Metadata:       map[string]interface{}{"transaction_key": "transaction_value"},
		}

		transactionInput := &pkgTransaction.Transaction{}

		// Create a transaction queue with the necessary fields
		transactionQueue := transaction.TransactionProcessingPayload{
			Transaction: tran,
			Validate:    validate,
			Balances:    balances,
			Input:       transactionInput,
		}

		transactionBytes, marshalErr := msgpack.Marshal(transactionQueue)
		require.NoError(t, marshalErr, "failed to marshal transaction queue")
		queueData := []mmodel.QueueData{
			{
				ID:    uuid.New(),
				Value: transactionBytes,
			},
		}

		queue := mmodel.Queue{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			QueueData:      queueData,
		}

		// Mock BalanceRepo.BalancesUpdate (called by UpdateBalances before transaction create)
		mockBalanceRepo.EXPECT().
			BalancesUpdate(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil).
			Times(1)

		// Mock TransactionRepo.Create
		mockTransactionRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			Return(tran, nil).
			Times(1)

		// Mock MetadataRepo.Create for transaction metadata
		mockMetadataRepo.EXPECT().
			Create(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil).
			Times(1)

		// Mock OperationRepo.Create for both operations and assert versions exist
		mockOperationRepo.EXPECT().
			Create(gomock.Any(), operation1).
			DoAndReturn(func(_ context.Context, op *operation.Operation) (*operation.Operation, error) {
				assert.NotNil(t, op.Balance.Version)
				assert.NotNil(t, op.BalanceAfter.Version)
				return op, nil
			})

		mockOperationRepo.EXPECT().
			Create(gomock.Any(), operation2).
			DoAndReturn(func(_ context.Context, op *operation.Operation) (*operation.Operation, error) {
				assert.NotNil(t, op.Balance.Version)
				assert.NotNil(t, op.BalanceAfter.Version)
				return op, nil
			})

		// Mock MetadataRepo.Create for operation metadata
		mockMetadataRepo.EXPECT().
			Create(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil).
			Times(2)

		// Mock RabbitMQRepo.ProducerDefault for transaction events
		mockRabbitMQRepo.EXPECT().
			ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, nil).
			AnyTimes()

		// Mock RedisRepo.RemoveMessageFromQueue for removing transaction from queue
		mockRedisRepo.EXPECT().
			RemoveMessageFromQueue(gomock.Any(), gomock.Any()).
			Return(nil).
			AnyTimes()

		// Mock RedisRepo.Del for removing transaction from write-behind cache
		mockRedisRepo.EXPECT().
			Del(gomock.Any(), gomock.Any()).
			Return(nil).
			AnyTimes()

		// Call the method
		err := uc.CreateBalanceTransactionOperationsAsync(ctx, queue)

		assert.NoError(t, err)
	})

	t.Run("error_creating_operation", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockTransactionRepo := transaction.NewMockRepository(ctrl)
		mockOperationRepo := operation.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		// Create a UseCase with all required dependencies
		uc := &UseCase{
			TransactionRepo: mockTransactionRepo,
			OperationRepo:   mockOperationRepo,
			MetadataRepo:    mockMetadataRepo,
			BalanceRepo:     mockBalanceRepo,
			RabbitMQRepo:    mockRabbitMQRepo,
			RedisRepo:       mockRedisRepo,
		}

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		transactionID := uuid.New().String()

		// Mock transaction data with correct types
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
		}

		// Create operations for the transaction
		Amount := decimal.NewFromInt(50)
		operation1 := &operation.Operation{
			ID:             uuid.New().String(),
			TransactionID:  transactionID,
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      uuid.New().String(),
			Type:           "debit",
			AssetCode:      "USD",
			Amount: operation.Amount{
				Value: &Amount,
			},
			Metadata: map[string]interface{}{"key1": "value1"},
		}

		Amount = decimal.NewFromInt(40)
		operation2 := &operation.Operation{
			ID:             uuid.New().String(),
			TransactionID:  transactionID,
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      uuid.New().String(),
			Type:           "credit",
			AssetCode:      "EUR",
			Amount: operation.Amount{
				Value: &Amount,
			},
			Metadata: map[string]interface{}{"key2": "value2"},
		}

		tran := &transaction.Transaction{
			ID:             transactionID,
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Operations:     []*operation.Operation{operation1, operation2},
			Metadata:       map[string]interface{}{"transaction_key": "transaction_value"},
		}

		transactionInput := &pkgTransaction.Transaction{}

		// Create a transaction queue with the necessary fields
		transactionQueue := transaction.TransactionProcessingPayload{
			Transaction: tran,
			Validate:    validate,
			Balances:    balances,
			Input:       transactionInput,
		}

		transactionBytes, marshalErr := msgpack.Marshal(transactionQueue)
		require.NoError(t, marshalErr, "failed to marshal transaction queue")
		queueData := []mmodel.QueueData{
			{
				ID:    uuid.New(),
				Value: transactionBytes,
			},
		}

		queue := mmodel.Queue{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			QueueData:      queueData,
		}

		// Mock BalanceRepo.BalancesUpdate (called by UpdateBalances before transaction create)
		mockBalanceRepo.EXPECT().
			BalancesUpdate(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil).
			Times(1)

		// Mock TransactionRepo.Create
		mockTransactionRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			Return(tran, nil).
			Times(1)

		// Mock MetadataRepo.Create for transaction metadata
		mockMetadataRepo.EXPECT().
			Create(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil).
			Times(1)

		// Mock OperationRepo.Create to return an error for the first operation
		operationError := errors.New("failed to create operation")
		mockOperationRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			Return(nil, operationError).
			Times(1)

		// Call the method
		err := uc.CreateBalanceTransactionOperationsAsync(ctx, queue)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create operation")
	})

	t.Run("error_duplicate_operation", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockTransactionRepo := transaction.NewMockRepository(ctrl)
		mockOperationRepo := operation.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		// Create a UseCase with all required dependencies
		uc := &UseCase{
			TransactionRepo: mockTransactionRepo,
			OperationRepo:   mockOperationRepo,
			MetadataRepo:    mockMetadataRepo,
			BalanceRepo:     mockBalanceRepo,
			RabbitMQRepo:    mockRabbitMQRepo,
			RedisRepo:       mockRedisRepo,
		}

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		transactionID := uuid.New().String()

		// Mock transaction data with correct types
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
		}

		// Create operations for the transaction
		Amount := decimal.NewFromInt(50)
		operation1 := &operation.Operation{
			ID:             uuid.New().String(),
			TransactionID:  transactionID,
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      uuid.New().String(),
			Type:           "debit",
			AssetCode:      "USD",
			Amount: operation.Amount{
				Value: &Amount,
			},
			Metadata: map[string]interface{}{"key1": "value1"},
		}

		Amount = decimal.NewFromInt(50)
		operation2 := &operation.Operation{
			ID:             uuid.New().String(),
			TransactionID:  transactionID,
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      uuid.New().String(),
			Type:           "credit",
			AssetCode:      "EUR",
			Amount: operation.Amount{
				Value: &Amount,
			},
			Metadata: map[string]interface{}{"key2": "value2"},
		}

		tran := &transaction.Transaction{
			ID:             transactionID,
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Operations:     []*operation.Operation{operation1, operation2},
			Metadata:       map[string]interface{}{"transaction_key": "transaction_value"},
		}

		transactionInput := &pkgTransaction.Transaction{}

		// Create a transaction queue with the necessary fields
		transactionQueue := transaction.TransactionProcessingPayload{
			Transaction: tran,
			Validate:    validate,
			Balances:    balances,
			Input:       transactionInput,
		}

		transactionBytes, marshalErr := msgpack.Marshal(transactionQueue)
		require.NoError(t, marshalErr, "failed to marshal transaction queue")
		queueData := []mmodel.QueueData{
			{
				ID:    uuid.New(),
				Value: transactionBytes,
			},
		}

		queue := mmodel.Queue{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			QueueData:      queueData,
		}

		// Mock BalanceRepo.BalancesUpdate (called by UpdateBalances before transaction create)
		mockBalanceRepo.EXPECT().
			BalancesUpdate(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil).
			Times(1)

		// Mock TransactionRepo.Create
		mockTransactionRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			Return(tran, nil).
			Times(1)

		// Mock MetadataRepo.Create for transaction metadata
		mockMetadataRepo.EXPECT().
			Create(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil).
			Times(1)

		// Mock OperationRepo.Create to return a duplicate key error for the first operation
		pgErr := &pgconn.PgError{Code: "23505"}
		mockOperationRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			Return(nil, pgErr).
			Times(1)

		// Mock OperationRepo.Create for the second operation
		mockOperationRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			Return(operation2, nil).
			Times(1)

		// Mock MetadataRepo.Create for operation metadata (only for second operation)
		mockMetadataRepo.EXPECT().
			Create(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil).
			Times(1)

		// Mock RabbitMQRepo.ProducerDefault for transaction events (goroutine will still be called)
		mockRabbitMQRepo.EXPECT().
			ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, nil).
			AnyTimes()

		// Mock RedisRepo.RemoveMessageFromQueue for removing transaction from queue
		mockRedisRepo.EXPECT().
			RemoveMessageFromQueue(gomock.Any(), gomock.Any()).
			Return(nil).
			AnyTimes()

		// Mock RedisRepo.Del for removing transaction from write-behind cache
		mockRedisRepo.EXPECT().
			Del(gomock.Any(), gomock.Any()).
			Return(nil).
			AnyTimes()

		// Call the method
		err := uc.CreateBalanceTransactionOperationsAsync(ctx, queue)

		assert.NoError(t, err) // Duplicate key errors are handled gracefully
	})

	t.Run("error_creating_operation_metadata", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockTransactionRepo := transaction.NewMockRepository(ctrl)
		mockOperationRepo := operation.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		// Create a UseCase with all required dependencies
		uc := &UseCase{
			TransactionRepo: mockTransactionRepo,
			OperationRepo:   mockOperationRepo,
			MetadataRepo:    mockMetadataRepo,
			BalanceRepo:     mockBalanceRepo,
			RabbitMQRepo:    mockRabbitMQRepo,
			RedisRepo:       mockRedisRepo,
		}

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		transactionID := uuid.New().String()

		// Mock transaction data with correct types
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

		// Create operations for the transaction
		Amount := decimal.NewFromInt(50)
		operation1 := &operation.Operation{
			ID:             uuid.New().String(),
			TransactionID:  transactionID,
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			AccountID:      uuid.New().String(),
			Type:           "debit",
			AssetCode:      "USD",
			Amount: operation.Amount{
				Value: &Amount,
			},
			Metadata: map[string]interface{}{"key1": "value1"},
		}

		tran := &transaction.Transaction{
			ID:             transactionID,
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Operations:     []*operation.Operation{operation1},
			Metadata:       map[string]interface{}{"transaction_key": "transaction_value"},
		}

		transactionInput := &pkgTransaction.Transaction{}

		// Create a transaction queue with the necessary fields
		transactionQueue := transaction.TransactionProcessingPayload{
			Transaction: tran,
			Validate:    validate,
			Balances:    balances,
			Input:       transactionInput,
		}

		transactionBytes, marshalErr := msgpack.Marshal(transactionQueue)
		require.NoError(t, marshalErr, "failed to marshal transaction queue")
		queueData := []mmodel.QueueData{
			{
				ID:    uuid.New(),
				Value: transactionBytes,
			},
		}

		queue := mmodel.Queue{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			QueueData:      queueData,
		}

		// Mock BalanceRepo.BalancesUpdate (called by UpdateBalances before transaction create)
		mockBalanceRepo.EXPECT().
			BalancesUpdate(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil).
			Times(1)

		// Mock TransactionRepo.Create
		mockTransactionRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			Return(tran, nil).
			Times(1)

		// Mock MetadataRepo.Create for transaction metadata
		mockMetadataRepo.EXPECT().
			Create(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil).
			Times(1)

		// Mock OperationRepo.Create for the operation
		mockOperationRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			Return(operation1, nil).
			Times(1)

		// Mock MetadataRepo.Create for operation metadata to return an error
		metadataError := errors.New("failed to create operation metadata")
		mockMetadataRepo.EXPECT().
			Create(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(metadataError).
			Times(1)

		// Call the method
		err := uc.CreateBalanceTransactionOperationsAsync(ctx, queue)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create operation metadata")
	})
}

func TestCreateMetadataAsync(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		MetadataRepo: mockMetadataRepo,
	}

	ctx := context.Background()

	logger := &MockLogger{}
	metadata := map[string]any{"key": "value"}
	ID := uuid.New().String()
	collection := "Transaction"

	t.Run("success", func(t *testing.T) {
		mockMetadataRepo.EXPECT().
			Create(gomock.Any(), collection, gomock.Any()).
			Return(nil).
			Times(1)

		err := uc.CreateMetadataAsync(ctx, logger, metadata, ID, collection)
		assert.NoError(t, err)
	})

	t.Run("error", func(t *testing.T) {
		mockMetadataRepo.EXPECT().
			Create(gomock.Any(), collection, gomock.Any()).
			Return(errors.New("failed to create metadata")).
			Times(1)

		err := uc.CreateMetadataAsync(ctx, logger, metadata, ID, collection)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create metadata")
	})
}

func TestCreateBTOAsync(t *testing.T) {
	// This test simply verifies that CreateBTOAsync doesn't panic
	// Since it's just a wrapper around CreateBalanceTransactionOperationsAsync
	// which is tested separately, we don't need to test it extensively

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mocks for the repositories
	mockOperationRepo := operation.NewMockRepository(ctrl)
	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	// Create a real UseCase with mock repositories
	uc := &UseCase{
		OperationRepo:   mockOperationRepo,
		TransactionRepo: mockTransactionRepo,
		MetadataRepo:    mockMetadataRepo,
		BalanceRepo:     mockBalanceRepo,
		RabbitMQRepo:    mockRabbitMQRepo,
		RedisRepo:       mockRedisRepo,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	// Create a transaction queue with valid data
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
		ID:             uuid.New().String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		Operations:     []*operation.Operation{},
		Metadata:       map[string]interface{}{},
	}

	transactionInput := &pkgTransaction.Transaction{}

	transactionQueue := transaction.TransactionProcessingPayload{
		Transaction: tran,
		Validate:    validate,
		Balances:    balances,
		Input:       transactionInput,
	}

	transactionBytes, marshalErr := msgpack.Marshal(transactionQueue)
	require.NoError(t, marshalErr, "failed to marshal transaction queue")
	queueData := []mmodel.QueueData{
		{
			ID:    uuid.New(),
			Value: transactionBytes,
		},
	}

	queue := mmodel.Queue{
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		QueueData:      queueData,
	}

	// Mock BalanceRepo.BalancesUpdate (called by UpdateBalances before transaction create)
	mockBalanceRepo.EXPECT().
		BalancesUpdate(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	mockTransactionRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		Return(tran, nil).
		AnyTimes()

	mockMetadataRepo.EXPECT().
		Create(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	// Mock RabbitMQRepo.ProducerDefault for transaction events
	mockRabbitMQRepo.EXPECT().
		ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil).
		AnyTimes()

	// Mock RedisRepo.RemoveMessageFromQueue for removing transaction from queue
	mockRedisRepo.EXPECT().
		RemoveMessageFromQueue(gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	// Mock RedisRepo.Del for removing transaction from write-behind cache
	mockRedisRepo.EXPECT().
		Del(gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	// Call the method - this should not panic
	uc.CreateBTOSync(ctx, queue)
}

func TestUpdateTransactionBackupOperations(t *testing.T) {
	t.Run("success_updates_backup_with_operations", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		uc := &UseCase{
			RedisRepo: mockRedisRepo,
		}

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		transactionID := uuid.New().String()

		amount := decimal.NewFromFloat(100.00)
		avail := decimal.NewFromFloat(500.00)
		onHold := decimal.NewFromFloat(0)
		version := int64(1)

		operations := []*operation.Operation{
			{
				ID:            "op-1",
				TransactionID: transactionID,
				Type:          "DEBIT",
				AssetCode:     "BRL",
				Amount:        operation.Amount{Value: &amount},
				Balance: operation.Balance{
					Available: &avail,
					OnHold:    &onHold,
					Version:   &version,
				},
				BalanceAfter: operation.Balance{
					Available: &avail,
					OnHold:    &onHold,
					Version:   &version,
				},
				AccountID:      "acc-1",
				BalanceID:      "bal-1",
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
			},
		}

		backupJSON := `{"header_id":"req-1","transaction_id":"` + transactionID + `","organization_id":"` + organizationID.String() + `","ledger_id":"` + ledgerID.String() + `","ttl":"2026-01-01T00:00:00Z","transaction_status":"CREATED","transaction_date":"2026-01-01T00:00:00Z"}`

		mockRedisRepo.EXPECT().
			ReadMessageFromQueue(gomock.Any(), gomock.Any()).
			Return([]byte(backupJSON), nil).
			Times(1)

		mockRedisRepo.EXPECT().
			AddMessageToQueue(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, _ string, raw []byte) error {
				var queue mmodel.TransactionRedisQueue
				err := json.Unmarshal(raw, &queue)
				require.NoError(t, err)
				assert.Len(t, queue.Operations, 1)
				assert.Equal(t, "op-1", queue.Operations[0].ID)
				assert.Equal(t, "DEBIT", queue.Operations[0].Type)
				return nil
			}).
			Times(1)

		uc.UpdateTransactionBackupOperations(ctx, organizationID, ledgerID, transactionID, operations)
	})

	t.Run("read_failure_does_not_panic", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		uc := &UseCase{
			RedisRepo: mockRedisRepo,
		}

		ctx := context.Background()

		mockRedisRepo.EXPECT().
			ReadMessageFromQueue(gomock.Any(), gomock.Any()).
			Return(nil, errors.New("redis connection refused")).
			Times(1)

		// Should not panic, just log and return
		uc.UpdateTransactionBackupOperations(ctx, uuid.New(), uuid.New(), "tx-1", nil)
	})

	t.Run("unmarshal_failure_does_not_panic", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		uc := &UseCase{
			RedisRepo: mockRedisRepo,
		}

		ctx := context.Background()

		mockRedisRepo.EXPECT().
			ReadMessageFromQueue(gomock.Any(), gomock.Any()).
			Return([]byte("invalid json{{{"), nil).
			Times(1)

		// Should not panic, just log and return
		uc.UpdateTransactionBackupOperations(ctx, uuid.New(), uuid.New(), "tx-2", nil)
	})

	t.Run("write_failure_does_not_panic", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		uc := &UseCase{
			RedisRepo: mockRedisRepo,
		}

		ctx := context.Background()

		backupJSON := `{"header_id":"req-1","transaction_id":"` + uuid.New().String() + `","organization_id":"` + uuid.New().String() + `","ledger_id":"` + uuid.New().String() + `","ttl":"2026-01-01T00:00:00Z","transaction_status":"CREATED","transaction_date":"2026-01-01T00:00:00Z"}`

		mockRedisRepo.EXPECT().
			ReadMessageFromQueue(gomock.Any(), gomock.Any()).
			Return([]byte(backupJSON), nil).
			Times(1)

		mockRedisRepo.EXPECT().
			AddMessageToQueue(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(errors.New("redis write failed")).
			Times(1)

		// Should not panic, just log and return
		uc.UpdateTransactionBackupOperations(ctx, uuid.New(), uuid.New(), "tx-3", []*operation.Operation{})
	})
}

func TestOperationMsgpackRoundtrip(t *testing.T) {
	t.Run("direction_and_route_id_survive_roundtrip", func(t *testing.T) {
		routeID := uuid.New().String()
		amount := decimal.NewFromInt(100)
		version := int64(1)

		original := operation.Operation{
			ID:            uuid.New().String(),
			TransactionID: uuid.New().String(),
			Description:   "test operation",
			Type:          "DEBIT",
			AssetCode:     "BRL",
			Amount:        operation.Amount{Value: &amount},
			Balance: operation.Balance{
				Available: &amount,
				OnHold:    &amount,
				Version:   &version,
			},
			BalanceAfter: operation.Balance{
				Available: &amount,
				OnHold:    &amount,
				Version:   &version,
			},
			Status: operation.Status{
				Code: "ACTIVE",
			},
			AccountID:      uuid.New().String(),
			AccountAlias:   "@person1",
			BalanceKey:     "default",
			BalanceID:      uuid.New().String(),
			OrganizationID: uuid.New().String(),
			LedgerID:       uuid.New().String(),
			Direction:      "debit",
			RouteID:        &routeID,
		}

		data, err := msgpack.Marshal(original)
		require.NoError(t, err, "marshal should not fail")

		var decoded operation.Operation
		err = msgpack.Unmarshal(data, &decoded)
		require.NoError(t, err, "unmarshal should not fail")

		assert.Equal(t, original.Direction, decoded.Direction, "Direction must survive roundtrip")
		assert.NotNil(t, decoded.RouteID, "RouteID must not be nil after roundtrip")
		assert.Equal(t, *original.RouteID, *decoded.RouteID, "RouteID value must survive roundtrip")
		assert.Equal(t, original.ID, decoded.ID, "ID must survive roundtrip")
		assert.Equal(t, original.Type, decoded.Type, "Type must survive roundtrip")
		assert.Equal(t, original.AssetCode, decoded.AssetCode, "AssetCode must survive roundtrip")
	})
}

func TestOperationMsgpackBackwardCompatibility(t *testing.T) {
	t.Run("zero_value_direction_and_nil_route_id_are_preserved", func(t *testing.T) {
		amount := decimal.NewFromInt(50)
		version := int64(1)

		// Simulate an old-format message without Direction or RouteID
		original := operation.Operation{
			ID:            uuid.New().String(),
			TransactionID: uuid.New().String(),
			Type:          "CREDIT",
			AssetCode:     "USD",
			Amount:        operation.Amount{Value: &amount},
			Balance: operation.Balance{
				Available: &amount,
				OnHold:    &amount,
				Version:   &version,
			},
			BalanceAfter: operation.Balance{
				Available: &amount,
				OnHold:    &amount,
				Version:   &version,
			},
			Status: operation.Status{
				Code: "ACTIVE",
			},
			AccountID:      uuid.New().String(),
			BalanceID:      uuid.New().String(),
			OrganizationID: uuid.New().String(),
			LedgerID:       uuid.New().String(),
			// Direction intentionally left as zero value ("")
			// RouteID intentionally left as nil
		}

		data, err := msgpack.Marshal(original)
		require.NoError(t, err, "marshal should not fail")

		var decoded operation.Operation
		err = msgpack.Unmarshal(data, &decoded)
		require.NoError(t, err, "unmarshal should not fail for old-format message")

		assert.Equal(t, "", decoded.Direction, "Direction must be empty string for old-format messages")
		assert.Nil(t, decoded.RouteID, "RouteID must be nil for old-format messages")
		assert.Equal(t, original.ID, decoded.ID, "ID must survive roundtrip")
		assert.Equal(t, original.Type, decoded.Type, "Type must survive roundtrip")
	})
}

func TestTransactionProcessingPayloadMsgpackRoundtrip(t *testing.T) {
	t.Run("nested_operations_with_direction_and_route_id_survive", func(t *testing.T) {
		routeID := uuid.New().String()
		amount := decimal.NewFromInt(200)
		version := int64(3)

		op1 := &operation.Operation{
			ID:            uuid.New().String(),
			TransactionID: uuid.New().String(),
			Type:          "DEBIT",
			AssetCode:     "BRL",
			Amount:        operation.Amount{Value: &amount},
			Balance: operation.Balance{
				Available: &amount,
				OnHold:    &amount,
				Version:   &version,
			},
			BalanceAfter: operation.Balance{
				Available: &amount,
				OnHold:    &amount,
				Version:   &version,
			},
			Status: operation.Status{
				Code: "ACTIVE",
			},
			AccountID:      uuid.New().String(),
			BalanceID:      uuid.New().String(),
			OrganizationID: uuid.New().String(),
			LedgerID:       uuid.New().String(),
			Direction:      "source",
			RouteID:        &routeID,
		}

		op2 := &operation.Operation{
			ID:            uuid.New().String(),
			TransactionID: op1.TransactionID,
			Type:          "CREDIT",
			AssetCode:     "BRL",
			Amount:        operation.Amount{Value: &amount},
			Balance: operation.Balance{
				Available: &amount,
				OnHold:    &amount,
				Version:   &version,
			},
			BalanceAfter: operation.Balance{
				Available: &amount,
				OnHold:    &amount,
				Version:   &version,
			},
			Status: operation.Status{
				Code: "ACTIVE",
			},
			AccountID:      uuid.New().String(),
			BalanceID:      uuid.New().String(),
			OrganizationID: uuid.New().String(),
			LedgerID:       uuid.New().String(),
			Direction:      "destination",
			RouteID:        &routeID,
		}

		tran := &transaction.Transaction{
			ID:             op1.TransactionID,
			OrganizationID: op1.OrganizationID,
			LedgerID:       op1.LedgerID,
			Operations:     []*operation.Operation{op1, op2},
		}

		validate := &pkgTransaction.Responses{
			Aliases: []string{"@src", "@dst"},
		}

		original := transaction.TransactionProcessingPayload{
			Transaction: tran,
			Validate:    validate,
		}

		data, err := msgpack.Marshal(original)
		require.NoError(t, err, "marshal should not fail")

		var decoded transaction.TransactionProcessingPayload
		err = msgpack.Unmarshal(data, &decoded)
		require.NoError(t, err, "unmarshal should not fail")

		require.NotNil(t, decoded.Transaction, "Transaction must not be nil")
		require.Len(t, decoded.Transaction.Operations, 2, "must have 2 operations")

		decodedOp1 := decoded.Transaction.Operations[0]
		assert.Equal(t, "source", decodedOp1.Direction, "first operation Direction must be 'source'")
		require.NotNil(t, decodedOp1.RouteID, "first operation RouteID must not be nil")
		assert.Equal(t, routeID, *decodedOp1.RouteID, "first operation RouteID value must match")

		decodedOp2 := decoded.Transaction.Operations[1]
		assert.Equal(t, "destination", decodedOp2.Direction, "second operation Direction must be 'destination'")
		require.NotNil(t, decodedOp2.RouteID, "second operation RouteID must not be nil")
		assert.Equal(t, routeID, *decodedOp2.RouteID, "second operation RouteID value must match")
	})
}
