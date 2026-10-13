// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	libLog "github.com/LerianStudio/lib-observability/v4/log"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
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
)

// Int64Ptr returns a pointer to the given int64 value
func Int64Ptr(v int64) *int64 {
	return &v
}

// MockLogger is a mock implementation of logger for testing
type MockLogger struct{}

func (m *MockLogger) Log(_ context.Context, _ int, _ string, _ ...any) {}
func (m *MockLogger) With(_ ...any) libLog.Logger                      { return m }

func (m *MockLogger) WithGroup(_ string) libLog.Logger { return m }

func (m *MockLogger) Enabled(_ int) bool { return true }

func (m *MockLogger) Sync(_ context.Context) error { return nil }

// expectLegacyTransactionWrite expects the database transaction of one legacy
// write and its transaction insert, reporting the row as inserted or as already
// present. The returned double records whether the write committed.
func expectLegacyTransactionWrite(repo *transaction.MockRepository, rowInserted bool) *mockDBTransaction {
	dbTx := &mockDBTransaction{}
	result := &repository.BulkInsertResult{Attempted: 1, Ignored: 1}

	if rowInserted {
		result = &repository.BulkInsertResult{Attempted: 1, Inserted: 1, InsertedIDs: []string{"inserted"}}
	}

	repo.EXPECT().BeginTx(gomock.Any()).Return(dbTx, nil).Times(1)
	repo.EXPECT().CreateBulkTx(gomock.Any(), dbTx, gomock.Any()).Return(result, nil).Times(1)

	return dbTx
}

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
			TransactionRepo:         mockTransactionRepo,
			OperationRepo:           mockOperationRepo,
			TransactionMetadataRepo: mockMetadataRepo,
			BalanceRepo:             mockBalanceRepo,
			RabbitMQRepo:            mockRabbitMQRepo,
			TransactionRedisRepo:    mockRedisRepo,
		}

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		transactionID := uuid.New().String()

		// Mock transaction data with correct types
		validate := &mtransaction.Responses{
			Aliases: []string{"alias1", "alias2"},
			From: map[string]mtransaction.Amount{
				"alias1": {
					Asset: "USD",
					Value: decimal.NewFromInt(50),
				},
			},
			To: map[string]mtransaction.Amount{
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

		transactionInput := &mtransaction.Transaction{}

		// Create a transaction queue with the necessary fields
		transactionQueue := transaction.TransactionProcessingPayload{
			Transaction: tran,
			Validate:    validate,
			Balances:    balances,
			Input:       transactionInput,
			Version:     "v2",
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

		// Note: Balance updates are handled by BalanceSyncWorker, not in this flow

		dbTx := expectLegacyTransactionWrite(mockTransactionRepo, true)

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
		assert.True(t, dbTx.commitCalled, "the write must commit")
		assert.False(t, dbTx.rollbackCalled)
	})

	t.Run("existing_transaction_row_is_an_idempotent_noop", func(t *testing.T) {
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
			TransactionRepo:         mockTransactionRepo,
			OperationRepo:           mockOperationRepo,
			TransactionMetadataRepo: mockMetadataRepo,
			BalanceRepo:             mockBalanceRepo,
			RabbitMQRepo:            mockRabbitMQRepo,
			TransactionRedisRepo:    mockRedisRepo,
		}

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		transactionID := uuid.New().String()

		// Mock transaction data with correct types
		validate := &mtransaction.Responses{
			Aliases: []string{"alias1"},
			From: map[string]mtransaction.Amount{
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

		transactionInput := &mtransaction.Transaction{}

		transactionQueue := transaction.TransactionProcessingPayload{
			Transaction: tran,
			Validate:    validate,
			Balances:    balances,
			Input:       transactionInput,
			Version:     "v2",
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

		// Note: Balance updates are handled by BalanceSyncWorker, not in this flow

		// The insert reports the row as already present.
		dbTx := expectLegacyTransactionWrite(mockTransactionRepo, false)

		// Mock MetadataRepo.Create for transaction metadata (written again on a retry)
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

		assert.NoError(t, err)
		assert.True(t, dbTx.commitCalled)
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
			TransactionRepo:         mockTransactionRepo,
			OperationRepo:           mockOperationRepo,
			TransactionMetadataRepo: mockMetadataRepo,
			BalanceRepo:             mockBalanceRepo,
			RabbitMQRepo:            mockRabbitMQRepo,
			TransactionRedisRepo:    mockRedisRepo,
		}

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		transactionID := uuid.New().String()

		// Mock transaction data with correct types
		validate := &mtransaction.Responses{
			Aliases: []string{"alias1", "alias2"},
			From: map[string]mtransaction.Amount{
				"alias1": {
					Asset: "USD",
					Value: decimal.NewFromInt(50),
				},
			},
			To: map[string]mtransaction.Amount{
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

		// Create operations for the transaction.
		// operation1 carries non-zero overdraft snapshot values so that the
		// msgpack round-trip assertion in the OperationRepo.Create mock is
		// non-trivial — a regression that drops snapshot during serialization
		// will fail with a clear message.
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
			Balance: operation.Balance{
				Version:       Int64Ptr(1),
				OverdraftUsed: decimal.Zero,
			},
			BalanceAfter: operation.Balance{
				Version:       Int64Ptr(2),
				OverdraftUsed: decimal.NewFromInt(50),
			},
			Snapshot: mmodel.OperationSnapshot{
				OverdraftUsedBefore: "0",
				OverdraftUsedAfter:  "50",
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
			Balance: operation.Balance{
				Version:       Int64Ptr(1),
				OverdraftUsed: decimal.Zero,
			},
			BalanceAfter: operation.Balance{
				Version:       Int64Ptr(2),
				OverdraftUsed: decimal.Zero,
			},
			Snapshot: mmodel.OperationSnapshot{
				OverdraftUsedBefore: "0",
				OverdraftUsedAfter:  "0",
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

		transactionInput := &mtransaction.Transaction{}

		// Create a transaction queue with the necessary fields
		transactionQueue := transaction.TransactionProcessingPayload{
			Transaction: tran,
			Validate:    validate,
			Balances:    balances,
			Input:       transactionInput,
			Version:     "v2",
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

		// Note: Balance updates are handled by BalanceSyncWorker, not in this flow

		dbTx := expectLegacyTransactionWrite(mockTransactionRepo, true)

		// Mock MetadataRepo.Create for transaction metadata
		mockMetadataRepo.EXPECT().
			Create(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil).
			Times(1)

		// Mock OperationRepo.CreateBulkTx for both operations and assert versions exist.
		// Identity is asserted by ID inside DoAndReturn — direct struct equality
		// against operation1 / operation2 isn't usable here because the
		// transaction payload survives a msgpack round-trip via the queue, and
		// msgpack normalizes the internal big.Int of zero-valued
		// decimal.Decimal fields (Balance.OverdraftUsed under the always-
		// populated snapshot contract). The decimals still compare equal
		// semantically (`decimal.Equal`) but reflect.DeepEqual returns false,
		// which is the implicit matcher gomock uses when given a concrete
		// argument. Match by gomock.Any() and assert identity + snapshot
		// preservation explicitly.
		expectedOps := map[string]*operation.Operation{
			operation1.ID: operation1,
			operation2.ID: operation2,
		}
		mockOperationRepo.EXPECT().
			CreateBulkTx(gomock.Any(), dbTx, gomock.Any()).
			DoAndReturn(func(_ context.Context, _ repository.DBExecutor, ops []*operation.Operation) (*repository.BulkInsertResult, error) {
				require.Len(t, ops, 2)

				for _, op := range ops {
					exp, ok := expectedOps[op.ID]
					require.True(t, ok, "unexpected operation ID: %s", op.ID)
					delete(expectedOps, op.ID)

					assert.NotNil(t, op.Balance.Version)
					assert.NotNil(t, op.BalanceAfter.Version)

					// Always-populated snapshot contract: msgpack must preserve snapshot fields.
					assert.Equal(t, exp.Snapshot.OverdraftUsedBefore, op.Snapshot.OverdraftUsedBefore,
						"msgpack must preserve Snapshot.OverdraftUsedBefore for op %s", op.ID)
					assert.Equal(t, exp.Snapshot.OverdraftUsedAfter, op.Snapshot.OverdraftUsedAfter,
						"msgpack must preserve Snapshot.OverdraftUsedAfter for op %s", op.ID)

					// Decimal-aware equality survives msgpack big.Int normalization
					// (decimal.Zero{} vs decimal.NewFromInt(0) are .Equal() but not
					// reflect.DeepEqual).
					assert.True(t, op.Balance.OverdraftUsed.Equal(exp.Balance.OverdraftUsed),
						"msgpack must preserve Balance.OverdraftUsed for op %s: got %s want %s",
						op.ID, op.Balance.OverdraftUsed.String(), exp.Balance.OverdraftUsed.String())
					assert.True(t, op.BalanceAfter.OverdraftUsed.Equal(exp.BalanceAfter.OverdraftUsed),
						"msgpack must preserve BalanceAfter.OverdraftUsed for op %s: got %s want %s",
						op.ID, op.BalanceAfter.OverdraftUsed.String(), exp.BalanceAfter.OverdraftUsed.String())
				}

				return &repository.BulkInsertResult{Attempted: 2, Inserted: 2}, nil
			}).
			Times(1)

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
		assert.True(t, dbTx.commitCalled)
	})

	t.Run("error_creating_operation_rolls_back_and_writes_no_metadata", func(t *testing.T) {
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
			TransactionRepo:         mockTransactionRepo,
			OperationRepo:           mockOperationRepo,
			TransactionMetadataRepo: mockMetadataRepo,
			BalanceRepo:             mockBalanceRepo,
			RabbitMQRepo:            mockRabbitMQRepo,
			TransactionRedisRepo:    mockRedisRepo,
		}

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		transactionID := uuid.New().String()

		// Mock transaction data with correct types
		validate := &mtransaction.Responses{
			Aliases: []string{"alias1", "alias2"},
			From: map[string]mtransaction.Amount{
				"alias1": {
					Asset: "USD",
					Value: decimal.NewFromInt(50),
				},
			},
			To: map[string]mtransaction.Amount{
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

		transactionInput := &mtransaction.Transaction{}

		// Create a transaction queue with the necessary fields
		transactionQueue := transaction.TransactionProcessingPayload{
			Transaction: tran,
			Validate:    validate,
			Balances:    balances,
			Input:       transactionInput,
			Version:     "v2",
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

		// Note: Balance updates are handled by BalanceSyncWorker, not in this flow

		dbTx := expectLegacyTransactionWrite(mockTransactionRepo, true)

		// No metadata expectation: nothing is written to MongoDB before the
		// database transaction commits.
		operationError := errors.New("failed to create operation")
		mockOperationRepo.EXPECT().
			CreateBulkTx(gomock.Any(), dbTx, gomock.Any()).
			Return(nil, operationError).
			Times(1)

		// Call the method
		err := uc.CreateBalanceTransactionOperationsAsync(ctx, queue)

		require.ErrorIs(t, err, operationError)
		assert.False(t, dbTx.commitCalled, "a failed operation insert must not commit the transaction row")
		assert.True(t, dbTx.rollbackCalled)
	})

	t.Run("existing_operation_still_gets_its_metadata", func(t *testing.T) {
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
			TransactionRepo:         mockTransactionRepo,
			OperationRepo:           mockOperationRepo,
			TransactionMetadataRepo: mockMetadataRepo,
			BalanceRepo:             mockBalanceRepo,
			RabbitMQRepo:            mockRabbitMQRepo,
			TransactionRedisRepo:    mockRedisRepo,
		}

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		transactionID := uuid.New().String()

		// Mock transaction data with correct types
		validate := &mtransaction.Responses{
			Aliases: []string{"alias1", "alias2"},
			From: map[string]mtransaction.Amount{
				"alias1": {
					Asset: "USD",
					Value: decimal.NewFromInt(50),
				},
			},
			To: map[string]mtransaction.Amount{
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

		transactionInput := &mtransaction.Transaction{}

		// Create a transaction queue with the necessary fields
		transactionQueue := transaction.TransactionProcessingPayload{
			Transaction: tran,
			Validate:    validate,
			Balances:    balances,
			Input:       transactionInput,
			Version:     "v2",
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

		// Note: Balance updates are handled by BalanceSyncWorker, not in this flow

		dbTx := expectLegacyTransactionWrite(mockTransactionRepo, false)

		// One of the two operations is already persisted.
		mockOperationRepo.EXPECT().
			CreateBulkTx(gomock.Any(), dbTx, gomock.Any()).
			Return(&repository.BulkInsertResult{Attempted: 2, Inserted: 1, Ignored: 1}, nil).
			Times(1)

		metadataWritten := map[string]string{}
		mockMetadataRepo.EXPECT().
			Create(gomock.Any(), gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, collection string, meta *mongodb.Metadata) error {
				metadataWritten[meta.EntityID] = collection

				return nil
			}).
			Times(3)

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

		require.NoError(t, err)
		assert.True(t, dbTx.commitCalled)
		assert.Equal(t, map[string]string{
			transactionID: constant.EntityTransaction,
			operation1.ID: constant.EntityOperation,
			operation2.ID: constant.EntityOperation,
		}, metadataWritten, "every operation gets its metadata, including one already persisted")
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
			TransactionRepo:         mockTransactionRepo,
			OperationRepo:           mockOperationRepo,
			TransactionMetadataRepo: mockMetadataRepo,
			BalanceRepo:             mockBalanceRepo,
			RabbitMQRepo:            mockRabbitMQRepo,
			TransactionRedisRepo:    mockRedisRepo,
		}

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		transactionID := uuid.New().String()

		// Mock transaction data with correct types
		validate := &mtransaction.Responses{
			Aliases: []string{"alias1"},
			From: map[string]mtransaction.Amount{
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

		transactionInput := &mtransaction.Transaction{}

		// Create a transaction queue with the necessary fields
		transactionQueue := transaction.TransactionProcessingPayload{
			Transaction: tran,
			Validate:    validate,
			Balances:    balances,
			Input:       transactionInput,
			Version:     "v2",
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

		// Note: Balance updates are handled by BalanceSyncWorker, not in this flow

		dbTx := expectLegacyTransactionWrite(mockTransactionRepo, true)

		// Mock MetadataRepo.Create for transaction metadata
		mockMetadataRepo.EXPECT().
			Create(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil).
			Times(1)

		mockOperationRepo.EXPECT().
			CreateBulkTx(gomock.Any(), dbTx, gomock.Any()).
			Return(&repository.BulkInsertResult{Attempted: 1, Inserted: 1}, nil).
			Times(1)

		// Mock MetadataRepo.Create for operation metadata to return an error
		metadataError := errors.New("failed to create operation metadata")
		mockMetadataRepo.EXPECT().
			Create(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(metadataError).
			Times(1)

		// Call the method
		err := uc.CreateBalanceTransactionOperationsAsync(ctx, queue)

		require.ErrorIs(t, err, metadataError)
		assert.True(t, dbTx.commitCalled, "metadata is written after the rows commit")
	})
}

func TestCreateBalanceTransactionOperationsAsync_BackupCleanupSurvivesCancelledContext(t *testing.T) {
	const cleanupWait = 5 * time.Second

	newCleanupFixture := func(t *testing.T) (*UseCase, *redis.MockRedisRepository, mmodel.Queue) {
		t.Helper()

		ctrl := gomock.NewController(t)

		mockTransactionRepo := transaction.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		uc := &UseCase{
			TransactionRepo:         mockTransactionRepo,
			OperationRepo:           operation.NewMockRepository(ctrl),
			TransactionMetadataRepo: mockMetadataRepo,
			BalanceRepo:             balance.NewMockRepository(ctrl),
			RabbitMQRepo:            mockRabbitMQRepo,
			TransactionRedisRepo:    mockRedisRepo,
		}

		organizationID := uuid.New()
		ledgerID := uuid.New()

		tran := &transaction.Transaction{
			ID:             uuid.New().String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Status:         transaction.Status{Code: constant.CREATED},
			Operations:     []*operation.Operation{},
			Metadata:       map[string]any{},
		}

		payload := transaction.TransactionProcessingPayload{
			Transaction: tran,
			Validate:    &mtransaction.Responses{Aliases: []string{"alias1"}},
			Input:       &mtransaction.Transaction{},
			Version:     "v2",
		}

		payloadBytes, err := msgpack.Marshal(payload)
		require.NoError(t, err)

		expectLegacyTransactionWrite(mockTransactionRepo, true)
		mockMetadataRepo.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
		mockRabbitMQRepo.EXPECT().ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
		mockRedisRepo.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

		queue := mmodel.Queue{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			QueueData:      []mmodel.QueueData{{ID: uuid.New(), Value: payloadBytes}},
		}

		return uc, mockRedisRepo, queue
	}

	cancelledContext := func() context.Context {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		return ctx
	}

	// The cleanup goroutine cancels its own ctx once it returns, so the ctx
	// state is captured inside the mock hook, while the call is in flight.
	type cleanupCtxState struct {
		err         error
		hasDeadline bool
	}

	captureState := func(ctx context.Context) cleanupCtxState {
		_, hasDeadline := ctx.Deadline()

		return cleanupCtxState{err: ctx.Err(), hasDeadline: hasDeadline}
	}

	assertDetached := func(t *testing.T, state cleanupCtxState) {
		t.Helper()

		assert.NoError(t, state.err, "cleanup ctx must not inherit the caller's cancellation")
		assert.True(t, state.hasDeadline, "cleanup ctx must be bounded by a timeout")
	}

	t.Run("unconditional_cleanup_when_async_disabled", func(t *testing.T) {
		t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

		uc, mockRedisRepo, queue := newCleanupFixture(t)

		removed := make(chan cleanupCtxState, 1)

		mockRedisRepo.EXPECT().
			RemoveMessageFromQueue(gomock.Any(), gomock.Any()).
			DoAndReturn(func(ctx context.Context, _ string) error {
				removed <- captureState(ctx)
				return nil
			}).
			Times(1)

		err := uc.CreateBalanceTransactionOperationsAsync(cancelledContext(), queue)
		require.NoError(t, err)

		select {
		case state := <-removed:
			assertDetached(t, state)
		case <-time.After(cleanupWait):
			t.Fatal("backup cleanup did not reach RemoveMessageFromQueue")
		}
	})

	t.Run("conditional_cleanup_when_async_enabled", func(t *testing.T) {
		t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "true")

		uc, mockRedisRepo, queue := newCleanupFixture(t)

		rawBackup, err := json.Marshal(mmodel.TransactionRedisQueue{TransactionStatus: constant.CREATED})
		require.NoError(t, err)

		read := make(chan cleanupCtxState, 1)
		removed := make(chan cleanupCtxState, 1)

		gomock.InOrder(
			mockRedisRepo.EXPECT().
				ReadMessageFromQueue(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, _ string) ([]byte, error) {
					read <- captureState(ctx)
					return rawBackup, nil
				}).
				Times(1),
			mockRedisRepo.EXPECT().
				RemoveMessageFromQueue(gomock.Any(), gomock.Any()).
				DoAndReturn(func(ctx context.Context, _ string) error {
					removed <- captureState(ctx)
					return nil
				}).
				Times(1),
		)

		err = uc.CreateBalanceTransactionOperationsAsync(cancelledContext(), queue)
		require.NoError(t, err)

		for name, ch := range map[string]chan cleanupCtxState{"ReadMessageFromQueue": read, "RemoveMessageFromQueue": removed} {
			select {
			case state := <-ch:
				assertDetached(t, state)
			case <-time.After(cleanupWait):
				t.Fatalf("backup cleanup did not reach %s", name)
			}
		}
	})
}

func TestCreateMetadataAsync(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionMetadataRepo: mockMetadataRepo,
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
		OperationRepo:           mockOperationRepo,
		TransactionRepo:         mockTransactionRepo,
		TransactionMetadataRepo: mockMetadataRepo,
		BalanceRepo:             mockBalanceRepo,
		RabbitMQRepo:            mockRabbitMQRepo,
		TransactionRedisRepo:    mockRedisRepo,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	// Create a transaction queue with valid data
	validate := &mtransaction.Responses{
		Aliases: []string{"alias1"},
		From: map[string]mtransaction.Amount{
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

	transactionInput := &mtransaction.Transaction{}

	transactionQueue := transaction.TransactionProcessingPayload{
		Transaction: tran,
		Validate:    validate,
		Balances:    balances,
		Input:       transactionInput,
		Version:     "v2",
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

	expectLegacyTransactionWrite(mockTransactionRepo, true)

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

	require.NoError(t, uc.CreateBTOSync(ctx, queue))
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

		validate := &mtransaction.Responses{
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
