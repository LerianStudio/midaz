// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package command

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
	"go.opentelemetry.io/otel"
	"go.uber.org/mock/gomock"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"

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
)

// Sentinel errors for test assertions.
var (
	errFailedToUpdateBalances      = errors.New("failed to update balances")
	errBatchInsertFailed           = errors.New("batch insert failed")
	errDatabaseConnectionRefused   = errors.New("database connection refused")
	errFailedToCreateOperationMeta = errors.New("failed to create operation metadata")
	errFailedToCreateMetadata      = errors.New("failed to create metadata")
	errRedisConnectionRefused      = errors.New("redis connection refused")
	errRedisWriteFailed            = errors.New("redis write failed")
)

// Int64Ptr returns a pointer to the given int64 value.
func Int64Ptr(v int64) *int64 {
	return &v
}

func mustMsgpackMarshal(t *testing.T, payload any) []byte {
	t.Helper()

	data, err := msgpack.Marshal(payload)
	require.NoError(t, err)

	return data
}

func setupLifecycleActionCapture(t *testing.T, mockBrokerRepo *redpanda.MockProducerRepository, expected int, topic string) <-chan string {
	t.Helper()

	actions := make(chan string, expected)

	mockBrokerRepo.EXPECT().
		ProducerDefault(gomock.Any(), topic, gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _ string, payload []byte) (*string, error) {
			event := mmodel.Event{}
			require.NoError(t, json.Unmarshal(payload, &event))

			actions <- event.Action

			return nil, nil
		}).
		Times(expected)

	return actions
}

func collectLifecycleActions(t *testing.T, actions <-chan string, expected int) []string {
	t.Helper()

	collected := make([]string, 0, expected)
	timeout := time.After(2 * time.Second)

	for len(collected) < expected {
		select {
		case action := <-actions:
			collected = append(collected, action)
		case <-timeout:
			t.Fatalf("timed out waiting for %d lifecycle events, got %d", expected, len(collected))
		}
	}

	return collected
}

// NOTE: A previous helper `expectOperationNotFoundLookup` mocked OperationRepo.Find
// with AnyTimes(), but CreateBalanceTransactionOperationsAsync never calls Find.
// Operation idempotency is handled at the PostgreSQL level via ON CONFLICT (id) DO
// NOTHING in CreateBatch. The dead mock was removed to avoid misleading future readers.

// MockLogger is a mock implementation of logger for testing.
type MockLogger struct{}

func (m *MockLogger) Debug(args ...any)                                        {}
func (m *MockLogger) Debugf(format string, args ...any)                        {}
func (m *MockLogger) Debugln(args ...any)                                      {}
func (m *MockLogger) Info(args ...any)                                         {}
func (m *MockLogger) Infof(format string, args ...any)                         {}
func (m *MockLogger) Infoln(args ...any)                                       {}
func (m *MockLogger) Warn(args ...any)                                         {}
func (m *MockLogger) Warnf(format string, args ...any)                         {}
func (m *MockLogger) Warnln(args ...any)                                       {}
func (m *MockLogger) Error(args ...any)                                        {}
func (m *MockLogger) Errorf(format string, args ...any)                        {}
func (m *MockLogger) Errorln(args ...any)                                      {}
func (m *MockLogger) Fatal(args ...any)                                        {}
func (m *MockLogger) Fatalf(format string, args ...any)                        {}
func (m *MockLogger) Fatalln(args ...any)                                      {}
func (m *MockLogger) Sync() error                                              { return nil }
func (m *MockLogger) WithDefaultMessageTemplate(template string) libLog.Logger { return m }
func (m *MockLogger) WithFields(args ...any) libLog.Logger                     { return m }

type testBatchTx struct{}

func (tx *testBatchTx) ExecContext(_ context.Context, _ string, _ ...any) (sql.Result, error) {
	return nil, nil //nolint:nilnil // test stub: caller never inspects the result
}

func (tx *testBatchTx) QueryContext(_ context.Context, _ string, _ ...any) (*sql.Rows, error) {
	return nil, nil
}

func (tx *testBatchTx) Commit() error {
	return nil
}

func (tx *testBatchTx) Rollback() error {
	return nil
}

type batchCapableTransactionRepo struct {
	*transaction.MockRepository
	beginTxFn           func(context.Context) (sqlBatchTx, error)
	createBatchWithTxFn func(context.Context, sqlExecQueryTx, []*transaction.Transaction) error
}

func (r *batchCapableTransactionRepo) BeginTx(ctx context.Context) (sqlBatchTx, error) {
	if r.beginTxFn != nil {
		return r.beginTxFn(ctx)
	}

	return &testBatchTx{}, nil
}

func (r *batchCapableTransactionRepo) CreateBatchWithTx(ctx context.Context, tx sqlExecQueryTx, transactions []*transaction.Transaction) error {
	if r.createBatchWithTxFn != nil {
		return r.createBatchWithTxFn(ctx, tx, transactions)
	}

	return nil
}

type batchCapableOperationRepo struct {
	*operation.MockRepository
	createBatchWithTxFn func(context.Context, interface {
		ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	}, []*operation.Operation) error
}

func (r *batchCapableOperationRepo) CreateBatchWithTx(
	ctx context.Context,
	tx interface {
		ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	},
	operations []*operation.Operation,
) error {
	if r.createBatchWithTxFn != nil {
		return r.createBatchWithTxFn(ctx, tx, operations)
	}

	return nil
}

type batchCapableBalanceRepo struct {
	*balance.MockRepository
	balancesUpdateWithTxFn func(context.Context, sqlExecQueryTx, uuid.UUID, uuid.UUID, []*mmodel.Balance) error
}

func (r *batchCapableBalanceRepo) BalancesUpdateWithTx(
	ctx context.Context,
	tx sqlExecQueryTx,
	organizationID, ledgerID uuid.UUID,
	balances []*mmodel.Balance,
) error {
	if r.balancesUpdateWithTxFn != nil {
		return r.balancesUpdateWithTxFn(ctx, tx, organizationID, ledgerID, balances)
	}

	return nil
}

//nolint:funlen
func TestCreateBalanceTransactionOperationsAsync(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockTransactionRepo := transaction.NewMockRepository(ctrl)
		mockOperationRepo := operation.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockBrokerRepo := redpanda.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		// Create a UseCase with all required dependencies
		uc := &UseCase{
			TransactionRepo: mockTransactionRepo,
			OperationRepo:   mockOperationRepo,
			MetadataRepo:    mockMetadataRepo,
			BalanceRepo:     mockBalanceRepo,
			BrokerRepo:      mockBrokerRepo,
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
			Metadata:       map[string]any{},
		}

		transactionInput := &pkgTransaction.Transaction{}

		// Create a transaction queue with the necessary fields
		transactionQueue := transaction.TransactionProcessingPayload{
			Transaction: tran,
			Validate:    validate,
			Balances:    balances,
			Input:       transactionInput,
		}

		transactionBytes := mustMsgpackMarshal(t, transactionQueue)
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

		// Mock RedisRepo.ListBalanceByKey for stale balance check (return nil to proceed with update)
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

		// Mock MetadataRepo.Create for transaction metadata
		mockMetadataRepo.EXPECT().
			Create(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil).
			AnyTimes()

		// Mock BrokerRepo.ProducerDefault for transaction events
		mockBrokerRepo.EXPECT().
			ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, nil).
			AnyTimes()

		// Mock RedisRepo.RemoveMessageFromQueue for removing transaction from queue
		mockRedisRepo.EXPECT().
			RemoveMessageFromQueue(gomock.Any(), gomock.Any()).
			Return(nil).
			AnyTimes()

		// Call the method
		err := uc.CreateBalanceTransactionOperationsAsync(ctx, queue)

		require.NoError(t, err)
	})

	t.Run("error_update_balances", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockTransactionRepo := transaction.NewMockRepository(ctrl)
		mockOperationRepo := operation.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockBrokerRepo := redpanda.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		// Create a UseCase with mock repositories
		uc := &UseCase{
			TransactionRepo: mockTransactionRepo,
			OperationRepo:   mockOperationRepo,
			MetadataRepo:    mockMetadataRepo,
			BalanceRepo:     mockBalanceRepo,
			BrokerRepo:      mockBrokerRepo,
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

		tran := &transaction.Transaction{
			ID:             transactionID,
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Operations:     []*operation.Operation{},
			Metadata:       map[string]any{},
		}

		transactionInput := &pkgTransaction.Transaction{}

		transactionQueue := transaction.TransactionProcessingPayload{
			Transaction: tran,
			Validate:    validate,
			Balances:    balances,
			Input:       transactionInput,
		}

		transactionBytes := mustMsgpackMarshal(t, transactionQueue)
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

		// Mock RedisRepo.ListBalanceByKey for stale balance check (return nil to proceed with update)
		mockRedisRepo.EXPECT().
			ListBalanceByKey(gomock.Any(), organizationID, ledgerID, "alias1").
			Return(nil, nil).
			AnyTimes()

		mockTransactionRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			Return(tran, nil).
			Times(1)

		// Mock BalanceRepo.BalancesUpdate to return an error
		mockBalanceRepo.EXPECT().
			BalancesUpdate(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(errFailedToUpdateBalances).
			Times(1)

		// Call the method
		err := uc.CreateBalanceTransactionOperationsAsync(ctx, queue)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to update balances")
	})

	t.Run("error_duplicate_transaction", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockTransactionRepo := transaction.NewMockRepository(ctrl)
		mockOperationRepo := operation.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockBrokerRepo := redpanda.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		// Create a UseCase with all required dependencies
		uc := &UseCase{
			TransactionRepo: mockTransactionRepo,
			OperationRepo:   mockOperationRepo,
			MetadataRepo:    mockMetadataRepo,
			BalanceRepo:     mockBalanceRepo,
			BrokerRepo:      mockBrokerRepo,
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
			Metadata:       map[string]any{},
		}

		transactionInput := &pkgTransaction.Transaction{}

		transactionQueue := transaction.TransactionProcessingPayload{
			Transaction: tran,
			Validate:    validate,
			Balances:    balances,
			Input:       transactionInput,
		}

		transactionBytes := mustMsgpackMarshal(t, transactionQueue)
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

		// Mock RedisRepo.ListBalanceByKey for stale balance check (return nil to proceed with update)
		mockRedisRepo.EXPECT().
			ListBalanceByKey(gomock.Any(), organizationID, ledgerID, "alias1").
			Return(nil, nil).
			AnyTimes()

		// Mock TransactionRepo.Create idempotent duplicate behavior (ON CONFLICT DO NOTHING)
		mockTransactionRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			Return(nil, nil).
			Times(1)

		mockBalanceRepo.EXPECT().
			BalancesUpdate(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil).
			Times(1)

		mockMetadataRepo.EXPECT().
			Create(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil).
			AnyTimes()

		// Mock RedisRepo.RemoveMessageFromQueue for removing transaction from queue
		mockRedisRepo.EXPECT().
			RemoveMessageFromQueue(gomock.Any(), gomock.Any()).
			Return(nil).
			AnyTimes()

		err := uc.CreateBalanceTransactionOperationsAsync(ctx, queue)

		require.NoError(t, err) // Duplicate key errors are handled gracefully
	})

	t.Run("success_with_multiple_operations", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockTransactionRepo := transaction.NewMockRepository(ctrl)
		mockOperationRepo := operation.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockBrokerRepo := redpanda.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		// Create a UseCase with all required dependencies
		uc := &UseCase{
			TransactionRepo: mockTransactionRepo,
			OperationRepo:   mockOperationRepo,
			MetadataRepo:    mockMetadataRepo,
			BalanceRepo:     mockBalanceRepo,
			BrokerRepo:      mockBrokerRepo,
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
			Metadata: map[string]any{"key1": "value1"},
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
			Metadata: map[string]any{"key2": "value2"},
		}

		tran := &transaction.Transaction{
			ID:             transactionID,
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Operations:     []*operation.Operation{operation1, operation2},
			Metadata:       map[string]any{"transaction_key": "transaction_value"},
		}

		transactionInput := &pkgTransaction.Transaction{}

		// Create a transaction queue with the necessary fields
		transactionQueue := transaction.TransactionProcessingPayload{
			Transaction: tran,
			Validate:    validate,
			Balances:    balances,
			Input:       transactionInput,
		}

		transactionBytes := mustMsgpackMarshal(t, transactionQueue)
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

		// Mock RedisRepo.ListBalanceByKey for stale balance check (return nil to proceed with update)
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

		// Mock MetadataRepo.Create for transaction metadata
		mockMetadataRepo.EXPECT().
			Create(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil).
			Times(1)

		// Mock OperationRepo.CreateBatch for batch insert of all operations
		mockOperationRepo.EXPECT().
			CreateBatch(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, ops []*operation.Operation) error {
				require.Len(t, ops, 2)
				assert.Equal(t, operation1.ID, ops[0].ID)
				assert.Equal(t, operation2.ID, ops[1].ID)
				require.NotNil(t, ops[0].Balance.Version)
				require.NotNil(t, ops[0].BalanceAfter.Version)
				require.NotNil(t, ops[1].Balance.Version)
				require.NotNil(t, ops[1].BalanceAfter.Version)

				return nil
			}).
			Times(1)

		// Mock MetadataRepo.Create for operation metadata
		mockMetadataRepo.EXPECT().
			Create(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil).
			Times(2)

		// Mock BrokerRepo.ProducerDefault for transaction events
		mockBrokerRepo.EXPECT().
			ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, nil).
			AnyTimes()

		// Mock RedisRepo.RemoveMessageFromQueue for removing transaction from queue
		mockRedisRepo.EXPECT().
			RemoveMessageFromQueue(gomock.Any(), gomock.Any()).
			Return(nil).
			AnyTimes()

		// Call the method
		err := uc.CreateBalanceTransactionOperationsAsync(ctx, queue)

		require.NoError(t, err)
	})

	t.Run("error_creating_operation", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockTransactionRepo := transaction.NewMockRepository(ctrl)
		mockOperationRepo := operation.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockBrokerRepo := redpanda.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		// Create a UseCase with all required dependencies
		uc := &UseCase{
			TransactionRepo: mockTransactionRepo,
			OperationRepo:   mockOperationRepo,
			MetadataRepo:    mockMetadataRepo,
			BalanceRepo:     mockBalanceRepo,
			BrokerRepo:      mockBrokerRepo,
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
			Metadata: map[string]any{"key1": "value1"},
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
			Metadata: map[string]any{"key2": "value2"},
		}

		tran := &transaction.Transaction{
			ID:             transactionID,
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Operations:     []*operation.Operation{operation1, operation2},
			Metadata:       map[string]any{"transaction_key": "transaction_value"},
		}

		transactionInput := &pkgTransaction.Transaction{}

		// Create a transaction queue with the necessary fields
		transactionQueue := transaction.TransactionProcessingPayload{
			Transaction: tran,
			Validate:    validate,
			Balances:    balances,
			Input:       transactionInput,
		}

		transactionBytes := mustMsgpackMarshal(t, transactionQueue)
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

		// Mock RedisRepo.ListBalanceByKey for stale balance check (return nil to proceed with update)
		mockRedisRepo.EXPECT().
			ListBalanceByKey(gomock.Any(), organizationID, ledgerID, "alias1").
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

		// Mock MetadataRepo.Create for transaction metadata
		mockMetadataRepo.EXPECT().
			Create(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil).
			Times(1)

		// Mock OperationRepo.CreateBatch to return an error
		operationError := errBatchInsertFailed
		mockOperationRepo.EXPECT().
			CreateBatch(gomock.Any(), gomock.Any()).
			Return(operationError).
			Times(1)

		// Call the method
		err := uc.CreateBalanceTransactionOperationsAsync(ctx, queue)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "batch insert failed")
	})

	t.Run("error_batch_insert_database_failure", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockTransactionRepo := transaction.NewMockRepository(ctrl)
		mockOperationRepo := operation.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockBrokerRepo := redpanda.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		// Create a UseCase with all required dependencies
		uc := &UseCase{
			TransactionRepo: mockTransactionRepo,
			OperationRepo:   mockOperationRepo,
			MetadataRepo:    mockMetadataRepo,
			BalanceRepo:     mockBalanceRepo,
			BrokerRepo:      mockBrokerRepo,
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
			Metadata: map[string]any{"key1": "value1"},
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
			Metadata: map[string]any{"key2": "value2"},
		}

		tran := &transaction.Transaction{
			ID:             transactionID,
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Operations:     []*operation.Operation{operation1, operation2},
			Metadata:       map[string]any{"transaction_key": "transaction_value"},
		}

		transactionInput := &pkgTransaction.Transaction{}

		// Create a transaction queue with the necessary fields
		transactionQueue := transaction.TransactionProcessingPayload{
			Transaction: tran,
			Validate:    validate,
			Balances:    balances,
			Input:       transactionInput,
		}

		transactionBytes := mustMsgpackMarshal(t, transactionQueue)
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

		// Mock RedisRepo.ListBalanceByKey for stale balance check (return nil to proceed with update)
		mockRedisRepo.EXPECT().
			ListBalanceByKey(gomock.Any(), organizationID, ledgerID, "alias1").
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

		// Mock MetadataRepo.Create for transaction metadata
		mockMetadataRepo.EXPECT().
			Create(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil).
			Times(1)

		// Mock OperationRepo.CreateBatch to return a database connection error
		mockOperationRepo.EXPECT().
			CreateBatch(gomock.Any(), gomock.Any()).
			Return(errDatabaseConnectionRefused).
			Times(1)

		// Call the method
		err := uc.CreateBalanceTransactionOperationsAsync(ctx, queue)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "database connection refused")
	})

	t.Run("error_creating_operation_metadata", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockTransactionRepo := transaction.NewMockRepository(ctrl)
		mockOperationRepo := operation.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockBrokerRepo := redpanda.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		// Create a UseCase with all required dependencies
		uc := &UseCase{
			TransactionRepo: mockTransactionRepo,
			OperationRepo:   mockOperationRepo,
			MetadataRepo:    mockMetadataRepo,
			BalanceRepo:     mockBalanceRepo,
			BrokerRepo:      mockBrokerRepo,
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
			Metadata: map[string]any{"key1": "value1"},
		}

		tran := &transaction.Transaction{
			ID:             transactionID,
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Operations:     []*operation.Operation{operation1},
			Metadata:       map[string]any{"transaction_key": "transaction_value"},
		}

		transactionInput := &pkgTransaction.Transaction{}

		// Create a transaction queue with the necessary fields
		transactionQueue := transaction.TransactionProcessingPayload{
			Transaction: tran,
			Validate:    validate,
			Balances:    balances,
			Input:       transactionInput,
		}

		transactionBytes := mustMsgpackMarshal(t, transactionQueue)
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

		// Mock RedisRepo.ListBalanceByKey for stale balance check (return nil to proceed with update)
		mockRedisRepo.EXPECT().
			ListBalanceByKey(gomock.Any(), organizationID, ledgerID, "alias1").
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

		// Mock MetadataRepo.Create for transaction metadata
		mockMetadataRepo.EXPECT().
			Create(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil).
			Times(1)

		// Mock OperationRepo.CreateBatch for batch insert of the operation
		mockOperationRepo.EXPECT().
			CreateBatch(gomock.Any(), gomock.Any()).
			Return(nil).
			Times(1)

		// Mock MetadataRepo.Create for operation metadata to return an error
		metadataError := errFailedToCreateOperationMeta
		mockMetadataRepo.EXPECT().
			Create(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(metadataError).
			Times(1)

		// Call the method
		err := uc.CreateBalanceTransactionOperationsAsync(ctx, queue)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create operation metadata")
	})
}

func TestCreateBalanceTransactionOperationsAsync_EmitsPostingFailedOnBalanceError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockOperationRepo := operation.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockBrokerRepo := redpanda.NewMockProducerRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	actions := setupLifecycleActionCapture(t, mockBrokerRepo, 1, "test-decision-events")

	uc := &UseCase{
		TransactionRepo: mockTransactionRepo,
		OperationRepo:   mockOperationRepo,
		MetadataRepo:    mockMetadataRepo,
		BalanceRepo:     mockBalanceRepo,
		BrokerRepo:      mockBrokerRepo,
		RedisRepo:       mockRedisRepo,
		EventsTopic:     "test-decision-events",
		EventsEnabled:   true,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New().String()

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
			OnHold:         decimal.Zero,
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
		Status:         transaction.Status{Code: constant.CREATED},
		Operations:     []*operation.Operation{},
	}

	payload := transaction.TransactionProcessingPayload{
		Transaction: tran,
		Validate:    validate,
		Balances:    balances,
		Input:       &pkgTransaction.Transaction{},
	}

	queue := mmodel.Queue{
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		QueueData: []mmodel.QueueData{
			{ID: uuid.New(), Value: mustMsgpackMarshal(t, payload)},
		},
	}

	mockRedisRepo.EXPECT().
		ListBalanceByKey(gomock.Any(), organizationID, ledgerID, "alias1").
		Return(nil, nil).
		AnyTimes()

	mockTransactionRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		Return(tran, nil).
		Times(1)

	mockBalanceRepo.EXPECT().
		BalancesUpdate(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(errFailedToUpdateBalances).
		Times(1)

	err := uc.CreateBalanceTransactionOperationsAsync(ctx, queue)

	require.Error(t, err)
	observed := collectLifecycleActions(t, actions, 1)
	assert.Equal(t, []string{string(pkgTransaction.DecisionLifecycleActionPostingFailed)}, observed)
}

func TestCreateBalanceTransactionOperationsAsync_DuplicateReplaySuppressesPostingFailed(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockOperationRepo := operation.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockBrokerRepo := redpanda.NewMockProducerRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		TransactionRepo: mockTransactionRepo,
		OperationRepo:   mockOperationRepo,
		MetadataRepo:    mockMetadataRepo,
		BalanceRepo:     mockBalanceRepo,
		BrokerRepo:      mockBrokerRepo,
		RedisRepo:       mockRedisRepo,
		EventsTopic:     "test-decision-events",
		EventsEnabled:   true,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New().String()

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
			OnHold:         decimal.Zero,
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
		Status:         transaction.Status{Code: constant.CREATED},
		Operations:     []*operation.Operation{},
	}

	payload := transaction.TransactionProcessingPayload{
		Transaction: tran,
		Validate:    validate,
		Balances:    balances,
		Input:       &pkgTransaction.Transaction{},
	}

	queue := mmodel.Queue{
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		QueueData: []mmodel.QueueData{
			{ID: uuid.New(), Value: mustMsgpackMarshal(t, payload)},
		},
	}

	mockRedisRepo.EXPECT().
		ListBalanceByKey(gomock.Any(), organizationID, ledgerID, "alias1").
		Return(nil, nil).
		AnyTimes()

	mockTransactionRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		Return(nil, nil).
		Times(1)

	mockBalanceRepo.EXPECT().
		BalancesUpdate(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(errFailedToUpdateBalances).
		Times(1)

	err := uc.CreateBalanceTransactionOperationsAsync(ctx, queue)

	require.Error(t, err)
}

func TestEmitBatchPostingFailed_EmitsOnlyForNonNilTransactions(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockBrokerRepo := redpanda.NewMockProducerRepository(ctrl)
	actions := setupLifecycleActionCapture(t, mockBrokerRepo, 2, "test-decision-events")

	uc := &UseCase{
		BrokerRepo:    mockBrokerRepo,
		EventsTopic:   "test-decision-events",
		EventsEnabled: true,
	}

	processed := []processedTransactionBatchItem{
		{
			decisionContract: pkgTransaction.DefaultDecisionContract(),
			transaction: &transaction.Transaction{
				ID:             uuid.New().String(),
				OrganizationID: uuid.New().String(),
				LedgerID:       uuid.New().String(),
			},
		},
		{
			decisionContract: pkgTransaction.DefaultDecisionContract(),
			transaction:      nil,
		},
		{
			decisionContract: pkgTransaction.DefaultDecisionContract(),
			transaction: &transaction.Transaction{
				ID:             uuid.New().String(),
				OrganizationID: uuid.New().String(),
				LedgerID:       uuid.New().String(),
			},
		},
	}

	uc.emitBatchPostingFailed(context.Background(), &MockLogger{}, processed)

	observed := collectLifecycleActions(t, actions, 2)
	assert.ElementsMatch(t, []string{
		string(pkgTransaction.DecisionLifecycleActionPostingFailed),
		string(pkgTransaction.DecisionLifecycleActionPostingFailed),
	}, observed)
}

func TestCreateBalanceTransactionOperationsBatch_OptimizedPath_EmitsPostingFailedAfterRetryExhaustion(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	baseTxRepo := transaction.NewMockRepository(ctrl)
	baseOpRepo := operation.NewMockRepository(ctrl)
	baseBalanceRepo := balance.NewMockRepository(ctrl)
	mockBrokerRepo := redpanda.NewMockProducerRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New().String()

	retryAttempts := 0

	txRepo := &batchCapableTransactionRepo{
		MockRepository: baseTxRepo,
		beginTxFn: func(context.Context) (sqlBatchTx, error) {
			return &testBatchTx{}, nil
		},
		createBatchWithTxFn: func(context.Context, sqlExecQueryTx, []*transaction.Transaction) error {
			retryAttempts++
			return &pgconn.PgError{Code: "40P01", Message: "deadlock detected"}
		},
	}

	opRepo := &batchCapableOperationRepo{MockRepository: baseOpRepo}
	balanceRepo := &batchCapableBalanceRepo{
		MockRepository: baseBalanceRepo,
		balancesUpdateWithTxFn: func(context.Context, sqlExecQueryTx, uuid.UUID, uuid.UUID, []*mmodel.Balance) error {
			return nil
		},
	}

	actions := setupLifecycleActionCapture(t, mockBrokerRepo, 1, "test-decision-events")

	uc := &UseCase{
		TransactionRepo:               txRepo,
		OperationRepo:                 opRepo,
		BalanceRepo:                   balanceRepo,
		BrokerRepo:                    mockBrokerRepo,
		RedisRepo:                     mockRedisRepo,
		EventsTopic:                   "test-decision-events",
		EventsEnabled:                 true,
		DecisionLifecycleSyncForTests: true,
	}

	baseTxRepo.EXPECT().
		ListByIDs(gomock.Any(), organizationID, ledgerID, gomock.Any()).
		Return(nil, nil).
		Times(1)

	mockRedisRepo.EXPECT().
		ListBalanceByKey(gomock.Any(), organizationID, ledgerID, "alias1").
		Return(nil, nil).
		AnyTimes()

	validate := &pkgTransaction.Responses{
		Aliases: []string{"alias1"},
		From: map[string]pkgTransaction.Amount{
			"alias1": {
				Asset:           "USD",
				Value:           decimal.NewFromInt(10),
				Operation:       constant.DEBIT,
				TransactionType: constant.CREATED,
			},
		},
	}

	balanceSnapshot := &mmodel.Balance{
		ID:             uuid.New().String(),
		AccountID:      uuid.New().String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		Alias:          "alias1",
		Available:      decimal.NewFromInt(100),
		OnHold:         decimal.Zero,
		Version:        1,
		AccountType:    "deposit",
		AllowSending:   true,
		AllowReceiving: true,
		AssetCode:      "USD",
	}

	tran := &transaction.Transaction{
		ID:             transactionID,
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		Status:         transaction.Status{Code: constant.CREATED},
		Operations:     []*operation.Operation{},
		Metadata:       map[string]any{},
	}

	payload := transaction.TransactionProcessingPayload{
		DecisionContract: pkgTransaction.DefaultDecisionContract(),
		Transaction:      tran,
		Validate:         validate,
		Balances:         []*mmodel.Balance{balanceSnapshot},
		Input:            &pkgTransaction.Transaction{},
	}

	queue := mmodel.Queue{
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		QueueData: []mmodel.QueueData{
			{ID: uuid.New(), Value: mustMsgpackMarshal(t, payload)},
		},
	}

	err := uc.CreateBalanceTransactionOperationsBatch(context.Background(), []mmodel.Queue{queue})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "deadlock detected")
	assert.Equal(t, 3, retryAttempts)

	observed := collectLifecycleActions(t, actions, 1)
	assert.Equal(t, []string{string(pkgTransaction.DecisionLifecycleActionPostingFailed)}, observed)
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
		require.NoError(t, err)
	})

	t.Run("error", func(t *testing.T) {
		mockMetadataRepo.EXPECT().
			Create(gomock.Any(), collection, gomock.Any()).
			Return(errFailedToCreateMetadata).
			Times(1)

		err := uc.CreateMetadataAsync(ctx, logger, metadata, ID, collection)
		require.Error(t, err)
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
	mockBrokerRepo := redpanda.NewMockProducerRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	// Create a real UseCase with mock repositories
	uc := &UseCase{
		OperationRepo:   mockOperationRepo,
		TransactionRepo: mockTransactionRepo,
		MetadataRepo:    mockMetadataRepo,
		BalanceRepo:     mockBalanceRepo,
		BrokerRepo:      mockBrokerRepo,
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
		Metadata:       map[string]any{},
	}

	transactionInput := &pkgTransaction.Transaction{}

	transactionQueue := transaction.TransactionProcessingPayload{
		Transaction: tran,
		Validate:    validate,
		Balances:    balances,
		Input:       transactionInput,
	}

	transactionBytes := mustMsgpackMarshal(t, transactionQueue)
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

	// Mock RedisRepo.ListBalanceByKey for stale balance check (return nil to proceed with update)
	mockRedisRepo.EXPECT().
		ListBalanceByKey(gomock.Any(), organizationID, ledgerID, "alias1").
		Return(nil, nil).
		AnyTimes()

	// Mock all the necessary calls to avoid nil pointer dereference
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

	// Mock BrokerRepo.ProducerDefault for transaction events
	mockBrokerRepo.EXPECT().
		ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil).
		AnyTimes()

	// Mock RedisRepo.RemoveMessageFromQueue for removing transaction from queue
	mockRedisRepo.EXPECT().
		RemoveMessageFromQueue(gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	// Call the method - this should not panic and should succeed
	err := uc.CreateBTOSync(ctx, queue)
	require.NoError(t, err)
}

//nolint:funlen
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

		balances := []*mmodel.Balance{{
			ID:             "bal-1",
			Alias:          "@alice",
			Key:            "default",
			AccountID:      "acc-1",
			AssetCode:      "BRL",
			Available:      avail,
			OnHold:         onHold,
			Version:        version,
			AllowSending:   true,
			AllowReceiving: true,
		}}

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
				assert.Len(t, queue.Balances, 1)
				assert.Equal(t, "@alice", queue.Balances[0].Alias)
				assert.Equal(t, "default", queue.Balances[0].Key)

				return nil
			}).
			Times(1)

		uc.UpdateTransactionBackupOperations(ctx, organizationID, ledgerID, transactionID, balances, operations)
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
			Return(nil, errRedisConnectionRefused).
			Times(1)

		// Should not panic, just log and return
		uc.UpdateTransactionBackupOperations(ctx, uuid.New(), uuid.New(), "tx-1", nil, nil)
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
		uc.UpdateTransactionBackupOperations(ctx, uuid.New(), uuid.New(), "tx-2", nil, nil)
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
			Return(errRedisWriteFailed).
			Times(1)

		// Should not panic, just log and return
		uc.UpdateTransactionBackupOperations(ctx, uuid.New(), uuid.New(), "tx-3", nil, []*operation.Operation{})
	})

	t.Run("updates_preflight_shard_and_legacy_backup", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		uc := &UseCase{
			RedisRepo:   mockRedisRepo,
			ShardRouter: shard.NewRouter(2),
		}

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		transactionID := uuid.New().String()

		backupJSON := `{"header_id":"req-1","transaction_id":"` + transactionID + `","organization_id":"` + organizationID.String() + `","ledger_id":"` + ledgerID.String() + `","ttl":"2026-01-01T00:00:00Z","transaction_status":"CREATED","transaction_date":"2026-01-01T00:00:00Z"}`

		// Only 2 reads: pre-flight shard (deterministic FNV hash) + legacy key
		mockRedisRepo.EXPECT().
			ReadMessageFromQueue(gomock.Any(), gomock.Any()).
			Return([]byte(backupJSON), nil).
			Times(2)

		mockRedisRepo.EXPECT().
			AddMessageToQueue(gomock.Any(), gomock.Any(), gomock.Any()).
			Times(2)

		uc.UpdateTransactionBackupOperations(ctx, organizationID, ledgerID, transactionID, nil, []*operation.Operation{{ID: "op-1"}})
	})
}

func TestValidateOperationsNotNil(t *testing.T) {
	t.Run("returns error when operation is nil", func(t *testing.T) {
		ops := []*operation.Operation{{ID: "op-1"}, nil}

		err := validateOperationsNotNil(ops)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "nil operation")
	})

	t.Run("returns nil when all entries are valid", func(t *testing.T) {
		ops := []*operation.Operation{{ID: "op-1"}, {ID: "op-2"}}

		err := validateOperationsNotNil(ops)

		require.NoError(t, err)
	})

	t.Run("returns nil for empty input", func(t *testing.T) {
		err := validateOperationsNotNil([]*operation.Operation{})

		require.NoError(t, err)
	})
}

func TestValidateBalancesNotNil(t *testing.T) {
	t.Run("returns error when balances is nil", func(t *testing.T) {
		err := validateBalancesNotNil(nil)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "balances is nil")
	})

	t.Run("returns error when balance element is nil", func(t *testing.T) {
		err := validateBalancesNotNil([]*mmodel.Balance{{Alias: "@acc#default"}, nil})

		require.Error(t, err)
		require.ErrorIs(t, err, errInvalidPayloadNilBalance)
		assert.Contains(t, err.Error(), "at index 1")
	})

	t.Run("returns nil when all entries are valid", func(t *testing.T) {
		err := validateBalancesNotNil([]*mmodel.Balance{{Alias: "@acc#default"}})

		require.NoError(t, err)
	})
}

func TestCreateBalanceTransactionOperationsAsync_RejectsEmptyQueueData(t *testing.T) {
	uc := &UseCase{}

	err := uc.CreateBalanceTransactionOperationsAsync(context.Background(), mmodel.Queue{
		OrganizationID: uuid.New(),
		LedgerID:       uuid.New(),
		QueueData:      []mmodel.QueueData{},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty queue data")
}

func TestCreateBalanceTransactionOperationsAsync_RejectsMultipleQueueDataItems(t *testing.T) {
	uc := &UseCase{}

	err := uc.CreateBalanceTransactionOperationsAsync(context.Background(), mmodel.Queue{
		OrganizationID: uuid.New(),
		LedgerID:       uuid.New(),
		QueueData: []mmodel.QueueData{
			{ID: uuid.New(), Value: []byte{0x01}},
			{ID: uuid.New(), Value: []byte{0x02}},
		},
	})

	require.Error(t, err)
	assert.ErrorIs(t, err, errInvalidQueuePayloadCount)
}

func TestCreateBalanceTransactionOperationsAsync_RejectsNilTransaction(t *testing.T) {
	uc := &UseCase{}

	payload := transaction.TransactionProcessingPayload{
		Transaction: nil,
		Validate:    &pkgTransaction.Responses{},
		Balances:    []*mmodel.Balance{},
		Input:       &pkgTransaction.Transaction{},
	}

	msg := mustMsgpackMarshal(t, payload)

	err := uc.CreateBalanceTransactionOperationsAsync(context.Background(), mmodel.Queue{
		OrganizationID: uuid.New(),
		LedgerID:       uuid.New(),
		QueueData: []mmodel.QueueData{
			{ID: uuid.New(), Value: msg},
		},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "transaction is nil")
}

func TestCreateBalanceTransactionOperationsAsync_RejectsNilValidate(t *testing.T) {
	uc := &UseCase{}

	payload := transaction.TransactionProcessingPayload{
		Transaction: &transaction.Transaction{Status: transaction.Status{Code: constant.CREATED}},
		Validate:    nil,
		Balances:    []*mmodel.Balance{},
		Input:       &pkgTransaction.Transaction{},
	}

	msg := mustMsgpackMarshal(t, payload)

	err := uc.CreateBalanceTransactionOperationsAsync(context.Background(), mmodel.Queue{
		OrganizationID: uuid.New(),
		LedgerID:       uuid.New(),
		QueueData: []mmodel.QueueData{
			{ID: uuid.New(), Value: msg},
		},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "validate is nil")
}

func TestValidateTransactionProcessingPayload_NormalizesDecisionContract(t *testing.T) {
	payload := &transaction.TransactionProcessingPayload{
		Transaction: &transaction.Transaction{Status: transaction.Status{Code: constant.CREATED}},
		Validate:    &pkgTransaction.Responses{},
		Balances:    []*mmodel.Balance{{Alias: "@alice#default"}},
	}

	err := validateTransactionProcessingPayload(payload)
	require.NoError(t, err)
	assert.Equal(t, pkgTransaction.DecisionEndpointModeSync, payload.DecisionContract.EndpointMode)
	assert.Equal(t, pkgTransaction.GuaranteeModelSourceDurable, payload.DecisionContract.GuaranteeModel)
	assert.Equal(t, pkgTransaction.DestinationBlockedPolicyCreditAnyway, payload.DecisionContract.DestinationBlockedPolicy)
	assert.Equal(t, int64(150), payload.DecisionContract.DecisionLatencySLOMs)
}

func TestCreateBalanceTransactionOperationsBatch_FallbackWrapsErrors(t *testing.T) {
	uc := &UseCase{}

	err := uc.CreateBalanceTransactionOperationsBatch(context.Background(), []mmodel.Queue{
		{
			OrganizationID: uuid.New(),
			LedgerID:       uuid.New(),
			QueueData:      []mmodel.QueueData{},
		},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "batch item 0 failed")
	assert.Contains(t, err.Error(), "empty queue data")
}

func TestCreateBalanceTransactionOperationsBatch_FallbackPropagatesUnmarshalError(t *testing.T) {
	uc := &UseCase{}

	err := uc.CreateBalanceTransactionOperationsBatch(context.Background(), []mmodel.Queue{
		{
			OrganizationID: uuid.New(),
			LedgerID:       uuid.New(),
			QueueData: []mmodel.QueueData{
				{ID: uuid.New(), Value: []byte{0x01, 0x02, 0x03}},
			},
		},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "batch item 0 failed")
	assert.Contains(t, err.Error(), "msgpack")
}

func TestBuildBatchBalanceUpdates_SameBalanceAcrossBatch(t *testing.T) {
	uc := &UseCase{}
	orgID := uuid.New()
	ledgerID := uuid.New()
	balanceID := uuid.New().String()

	updates := []balanceUpdateItem{
		{
			organizationID: orgID,
			ledgerID:       ledgerID,
			validate: pkgTransaction.Responses{
				From: map[string]pkgTransaction.Amount{
					"0#@alice#default": {
						Asset:           "USD",
						Value:           decimal.NewFromInt(100),
						Operation:       constant.DEBIT,
						TransactionType: constant.CREATED,
					},
				},
				To: map[string]pkgTransaction.Amount{},
			},
			balances: []*mmodel.Balance{
				{
					ID:        balanceID,
					Alias:     "0#@alice#default",
					Available: decimal.NewFromInt(1000),
					OnHold:    decimal.Zero,
					Version:   10,
				},
			},
		},
		{
			organizationID: orgID,
			ledgerID:       ledgerID,
			validate: pkgTransaction.Responses{
				From: map[string]pkgTransaction.Amount{
					"0#@alice#default": {
						Asset:           "USD",
						Value:           decimal.NewFromInt(50),
						Operation:       constant.DEBIT,
						TransactionType: constant.CREATED,
					},
				},
				To: map[string]pkgTransaction.Amount{},
			},
			balances: []*mmodel.Balance{
				{
					ID:        balanceID,
					Alias:     "0#@alice#default",
					Available: decimal.NewFromInt(900),
					OnHold:    decimal.Zero,
					Version:   11,
				},
			},
		},
	}

	groups, err := uc.buildBatchBalanceUpdates(updates)
	require.NoError(t, err)
	require.Len(t, groups, 1)
	require.Len(t, groups[0].balances, 1)

	updated := groups[0].balances[0]
	assert.True(t, updated.Available.Equal(decimal.NewFromInt(850)))
	assert.Equal(t, int64(12), updated.Version)
}

func TestBuildBatchBalanceUpdates_AccumulatesOutOfOrderSnapshots(t *testing.T) {
	// When two transactions in the same batch touch the same balance with different
	// snapshot versions, BOTH operations must be applied cumulatively. Previously,
	// the stale snapshot (version 10) was silently skipped because its version was
	// less than the accumulated version (12). This caused financial data loss.
	uc := &UseCase{}
	orgID := uuid.New()
	ledgerID := uuid.New()
	balanceID := uuid.New().String()

	updates := []balanceUpdateItem{
		{
			organizationID: orgID,
			ledgerID:       ledgerID,
			validate: pkgTransaction.Responses{
				From: map[string]pkgTransaction.Amount{
					"0#@alice#default": {
						Asset:           "USD",
						Value:           decimal.NewFromInt(50),
						Operation:       constant.DEBIT,
						TransactionType: constant.CREATED,
					},
				},
				To: map[string]pkgTransaction.Amount{},
			},
			balances: []*mmodel.Balance{{
				ID:        balanceID,
				Alias:     "0#@alice#default",
				Available: decimal.NewFromInt(900),
				OnHold:    decimal.Zero,
				Version:   11,
			}},
		},
		{
			organizationID: orgID,
			ledgerID:       ledgerID,
			validate: pkgTransaction.Responses{
				From: map[string]pkgTransaction.Amount{
					"0#@alice#default": {
						Asset:           "USD",
						Value:           decimal.NewFromInt(100),
						Operation:       constant.DEBIT,
						TransactionType: constant.CREATED,
					},
				},
				To: map[string]pkgTransaction.Amount{},
			},
			balances: []*mmodel.Balance{{
				ID:        balanceID,
				Alias:     "0#@alice#default",
				Available: decimal.NewFromInt(1000),
				OnHold:    decimal.Zero,
				Version:   10,
			}},
		},
	}

	groups, err := uc.buildBatchBalanceUpdates(updates)
	require.NoError(t, err)
	require.Len(t, groups, 1)
	require.Len(t, groups[0].balances, 1)

	// T1: debit 50 from 900 -> 850 (v11->v12)
	// T2: debit 100 from accumulated 850 -> 750 (v12->v13)
	// Both operations applied cumulatively.
	updated := groups[0].balances[0]
	assert.True(t, updated.Available.Equal(decimal.NewFromInt(750)), "expected 750 but got %s", updated.Available)
	assert.Equal(t, int64(13), updated.Version)
}

// ---------------------------------------------------------------------------
// Tests for CreateBalanceTransactionOperationsBatch
// ---------------------------------------------------------------------------.

// TestCreateBalanceTransactionOperationsBatch_FallbackToPerItem verifies that
// when the underlying repositories do NOT implement the batch-optimized
// interfaces (batchTransactionRepository, batchOperationRepository,
// batchBalanceRepository), the function gracefully falls back to calling
// CreateBalanceTransactionOperationsAsync for each queue item individually.
//
// The standard gomock-generated mocks (transaction.MockRepository, etc.) only
// satisfy the base Repository interfaces, so the type assertions in
// CreateBalanceTransactionOperationsBatch will fail → triggering fallback.
func TestCreateBalanceTransactionOperationsBatch_FallbackToPerItem(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTransactionRepo := transaction.NewMockRepository(ctrl)
	mockOperationRepo := operation.NewMockRepository(ctrl)
	mockMetadataRepo := mongodb.NewMockRepository(ctrl)
	mockBalanceRepo := balance.NewMockRepository(ctrl)
	mockBrokerRepo := redpanda.NewMockProducerRepository(ctrl)
	mockRedisRepo := redis.NewMockRedisRepository(ctrl)

	uc := &UseCase{
		TransactionRepo: mockTransactionRepo,
		OperationRepo:   mockOperationRepo,
		MetadataRepo:    mockMetadataRepo,
		BalanceRepo:     mockBalanceRepo,
		BrokerRepo:      mockBrokerRepo,
		RedisRepo:       mockRedisRepo,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	// Build two independent queue items that each succeed through the
	// per-item CreateBalanceTransactionOperationsAsync path.
	buildQueue := func(alias string) mmodel.Queue {
		validate := &pkgTransaction.Responses{
			Aliases: []string{alias},
			From: map[string]pkgTransaction.Amount{
				alias: {
					Asset:           "USD",
					Value:           decimal.NewFromInt(10),
					Operation:       constant.DEBIT,
					TransactionType: constant.CREATED,
				},
			},
			To: map[string]pkgTransaction.Amount{},
		}
		bal := &mmodel.Balance{
			ID:             uuid.New().String(),
			AccountID:      uuid.New().String(),
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Alias:          alias,
			Available:      decimal.NewFromInt(1000),
			OnHold:         decimal.Zero,
			Version:        1,
			AccountType:    "deposit",
			AllowSending:   true,
			AllowReceiving: true,
			AssetCode:      "USD",
		}
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
			Validate:    validate,
			Balances:    []*mmodel.Balance{bal},
			Input:       &pkgTransaction.Transaction{},
		}

		return mmodel.Queue{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			QueueData: []mmodel.QueueData{
				{ID: uuid.New(), Value: mustMsgpackMarshal(t, payload)},
			},
		}
	}

	queues := []mmodel.Queue{
		buildQueue("alias_a"),
		buildQueue("alias_b"),
	}

	// Each per-item call will go through the full
	// CreateBalanceTransactionOperationsAsync flow, so we mock the full
	// sequence twice (once per queue item).
	mockRedisRepo.EXPECT().
		ListBalanceByKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil).
		AnyTimes()
	mockBalanceRepo.EXPECT().
		BalancesUpdate(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		Times(2)
	mockTransactionRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, tx *transaction.Transaction) (*transaction.Transaction, error) {
			return tx, nil
		}).
		Times(2)
	mockMetadataRepo.EXPECT().
		Create(gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()
	mockBrokerRepo.EXPECT().
		ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
		Return(nil, nil).
		AnyTimes()
	mockRedisRepo.EXPECT().
		RemoveMessageFromQueue(gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	err := uc.CreateBalanceTransactionOperationsBatch(ctx, queues)
	require.NoError(t, err)
}

// TestCreateBalanceTransactionOperationsBatch_EmptyInput verifies that an empty
// slice of queues returns nil immediately — no repo calls, no side effects.
func TestCreateBalanceTransactionOperationsBatch_EmptyInput(t *testing.T) {
	uc := &UseCase{}

	err := uc.CreateBalanceTransactionOperationsBatch(context.Background(), []mmodel.Queue{})
	require.NoError(t, err)

	err = uc.CreateBalanceTransactionOperationsBatch(context.Background(), nil)
	require.NoError(t, err)
}

// TestCreateBalanceTransactionOperationsBatch_ValidationErrors tests that
// validation errors from malformed queue items are surfaced through the
// fallback path (standard mocks → fallback → per-item
// CreateBalanceTransactionOperationsAsync → validation failure).
func TestCreateBalanceTransactionOperationsBatch_ValidationErrors(t *testing.T) {
	t.Run("empty_queue_data_in_item", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Standard mocks do NOT implement batch interfaces → fallback path.
		uc := &UseCase{
			TransactionRepo: transaction.NewMockRepository(ctrl),
			OperationRepo:   operation.NewMockRepository(ctrl),
			MetadataRepo:    mongodb.NewMockRepository(ctrl),
			BalanceRepo:     balance.NewMockRepository(ctrl),
			BrokerRepo:      redpanda.NewMockProducerRepository(ctrl),
			RedisRepo:       redis.NewMockRedisRepository(ctrl),
		}

		err := uc.CreateBalanceTransactionOperationsBatch(context.Background(), []mmodel.Queue{
			{
				OrganizationID: uuid.New(),
				LedgerID:       uuid.New(),
				QueueData:      []mmodel.QueueData{},
			},
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "batch item 0 failed")
		assert.Contains(t, err.Error(), "empty queue data")
	})

	t.Run("nil_transaction_in_payload", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		uc := &UseCase{
			TransactionRepo: transaction.NewMockRepository(ctrl),
			OperationRepo:   operation.NewMockRepository(ctrl),
			MetadataRepo:    mongodb.NewMockRepository(ctrl),
			BalanceRepo:     balance.NewMockRepository(ctrl),
			BrokerRepo:      redpanda.NewMockProducerRepository(ctrl),
			RedisRepo:       redis.NewMockRedisRepository(ctrl),
		}

		payload := transaction.TransactionProcessingPayload{
			Transaction: nil,
			Validate:    &pkgTransaction.Responses{},
			Balances:    []*mmodel.Balance{},
			Input:       &pkgTransaction.Transaction{},
		}

		err := uc.CreateBalanceTransactionOperationsBatch(context.Background(), []mmodel.Queue{
			{
				OrganizationID: uuid.New(),
				LedgerID:       uuid.New(),
				QueueData: []mmodel.QueueData{
					{ID: uuid.New(), Value: mustMsgpackMarshal(t, payload)},
				},
			},
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "transaction is nil")
	})

	t.Run("nil_validate_in_payload", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		uc := &UseCase{
			TransactionRepo: transaction.NewMockRepository(ctrl),
			OperationRepo:   operation.NewMockRepository(ctrl),
			MetadataRepo:    mongodb.NewMockRepository(ctrl),
			BalanceRepo:     balance.NewMockRepository(ctrl),
			BrokerRepo:      redpanda.NewMockProducerRepository(ctrl),
			RedisRepo:       redis.NewMockRedisRepository(ctrl),
		}

		payload := transaction.TransactionProcessingPayload{
			Transaction: &transaction.Transaction{
				Status: transaction.Status{Code: constant.CREATED},
			},
			Validate: nil,
			Balances: []*mmodel.Balance{},
			Input:    &pkgTransaction.Transaction{},
		}

		err := uc.CreateBalanceTransactionOperationsBatch(context.Background(), []mmodel.Queue{
			{
				OrganizationID: uuid.New(),
				LedgerID:       uuid.New(),
				QueueData: []mmodel.QueueData{
					{ID: uuid.New(), Value: mustMsgpackMarshal(t, payload)},
				},
			},
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "validate is nil")
	})
}

// TestCreateBalanceTransactionOperationsBatch_BatchGrouping verifies that
// buildBatchBalanceUpdates correctly groups balance updates by
// organizationID|ledgerID scope. Two items targeting different org/ledger pairs
// must produce two separate balanceBatchUpdateGroup entries.
func TestCreateBalanceTransactionOperationsBatch_BatchGrouping(t *testing.T) {
	uc := &UseCase{}

	orgA := uuid.New()
	ledgerA := uuid.New()
	orgB := uuid.New()
	ledgerB := uuid.New()

	balanceA := &mmodel.Balance{
		ID:        uuid.New().String(),
		Alias:     "0#@alice#default",
		Available: decimal.NewFromInt(500),
		OnHold:    decimal.Zero,
		Version:   1,
	}

	balanceB := &mmodel.Balance{
		ID:        uuid.New().String(),
		Alias:     "0#@bob#default",
		Available: decimal.NewFromInt(300),
		OnHold:    decimal.Zero,
		Version:   1,
	}

	updates := []balanceUpdateItem{
		{
			organizationID: orgA,
			ledgerID:       ledgerA,
			validate: pkgTransaction.Responses{
				From: map[string]pkgTransaction.Amount{
					"0#@alice#default": {
						Asset:           "USD",
						Value:           decimal.NewFromInt(10),
						Operation:       constant.DEBIT,
						TransactionType: constant.CREATED,
					},
				},
				To: map[string]pkgTransaction.Amount{},
			},
			balances: []*mmodel.Balance{balanceA},
		},
		{
			organizationID: orgB,
			ledgerID:       ledgerB,
			validate: pkgTransaction.Responses{
				From: map[string]pkgTransaction.Amount{
					"0#@bob#default": {
						Asset:           "BRL",
						Value:           decimal.NewFromInt(20),
						Operation:       constant.DEBIT,
						TransactionType: constant.CREATED,
					},
				},
				To: map[string]pkgTransaction.Amount{},
			},
			balances: []*mmodel.Balance{balanceB},
		},
	}

	groups, err := uc.buildBatchBalanceUpdates(updates)
	require.NoError(t, err)
	require.Len(t, groups, 2, "expected 2 groups for 2 different org|ledger scopes")

	// Build a lookup so we don't depend on map iteration order.
	scopeMap := make(map[string]balanceBatchUpdateGroup, 2)

	for _, g := range groups {
		key := g.organizationID.String() + "|" + g.ledgerID.String()
		scopeMap[key] = g
	}

	keyA := orgA.String() + "|" + ledgerA.String()
	keyB := orgB.String() + "|" + ledgerB.String()

	require.Contains(t, scopeMap, keyA, "expected group for org A | ledger A")
	require.Contains(t, scopeMap, keyB, "expected group for org B | ledger B")

	// Verify each group has exactly one balance.
	assert.Len(t, scopeMap[keyA].balances, 1)
	assert.Len(t, scopeMap[keyB].balances, 1)

	// Verify the balance amounts were correctly debited.
	// Alice: 500 - 10 = 490, version 1 → 2
	assert.True(t, scopeMap[keyA].balances[0].Available.Equal(decimal.NewFromInt(490)),
		"Alice: expected 490 but got %s", scopeMap[keyA].balances[0].Available)
	assert.Equal(t, int64(2), scopeMap[keyA].balances[0].Version)

	// Bob: 300 - 20 = 280, version 1 → 2
	assert.True(t, scopeMap[keyB].balances[0].Available.Equal(decimal.NewFromInt(280)),
		"Bob: expected 280 but got %s", scopeMap[keyB].balances[0].Available)
	assert.Equal(t, int64(2), scopeMap[keyB].balances[0].Version)
}

// TestCreateBalanceTransactionOperationsBatch_CumulativeBalanceApplication is
// the critical C-1 regression test. It verifies that when multiple
// transactions in the same batch all touch the SAME balance — each carrying
// the SAME Redis snapshot version — ALL mutations are applied cumulatively,
// not dropped.
//
// Scenario:
//
//	Balance "alice" starts at Available=1000, Version=10 (the Redis snapshot).
//	Three transactions in the batch each DEBIT 100 from "alice".
//	All three carry version=10 because they were all read from the same
//	Redis snapshot before the batch was assembled.
//
// Expected result after buildBatchBalanceUpdates:
//
//	Available = 1000 - 100 - 100 - 100 = 700
//	Version   = 10 + 3 = 13 (one increment per operation)
//
// If the C-1 bug were present (stale snapshot skipping), only the first
// transaction would be applied and the other two would be silently dropped,
// resulting in Available=900, Version=11 — a catastrophic financial data loss.
func TestCreateBalanceTransactionOperationsBatch_CumulativeBalanceApplication(t *testing.T) {
	uc := &UseCase{}

	orgID := uuid.New()
	ledgerID := uuid.New()
	balanceID := uuid.New().String()

	const (
		initialAvailable = 1000
		debitPerTx       = 100
		snapshotVersion  = 10
		txCount          = 3
	)

	// Build 3 update items, all referencing the same balance at the same
	// snapshot version — exactly what happens when the Redis Lua script
	// returns the same balance state to multiple concurrent transactions
	// that get batched together.
	updates := make([]balanceUpdateItem, 0, txCount)
	for i := 0; i < txCount; i++ {
		updates = append(updates, balanceUpdateItem{
			organizationID: orgID,
			ledgerID:       ledgerID,
			validate: pkgTransaction.Responses{
				From: map[string]pkgTransaction.Amount{
					"0#@alice#default": {
						Asset:           "USD",
						Value:           decimal.NewFromInt(debitPerTx),
						Operation:       constant.DEBIT,
						TransactionType: constant.CREATED,
					},
				},
				To: map[string]pkgTransaction.Amount{},
			},
			balances: []*mmodel.Balance{
				{
					ID:        balanceID,
					Alias:     "0#@alice#default",
					Available: decimal.NewFromInt(initialAvailable),
					OnHold:    decimal.Zero,
					Version:   snapshotVersion,
				},
			},
		})
	}

	groups, err := uc.buildBatchBalanceUpdates(updates)
	require.NoError(t, err)
	require.Len(t, groups, 1, "all updates target same org|ledger → 1 group")
	require.Len(t, groups[0].balances, 1, "all updates target same balance ID → 1 balance")

	updated := groups[0].balances[0]

	expectedAvailable := decimal.NewFromInt(initialAvailable - (debitPerTx * txCount)) // expected: 700
	expectedVersion := int64(snapshotVersion + txCount)                                // 10 + 3 = 13

	assert.True(t, updated.Available.Equal(expectedAvailable),
		"C-1 regression: expected Available=%s but got %s (all %d debits must be applied cumulatively)",
		expectedAvailable, updated.Available, txCount)

	assert.Equal(t, expectedVersion, updated.Version,
		"C-1 regression: expected Version=%d but got %d (one version increment per debit)",
		expectedVersion, updated.Version)

	// Verify balance ID and alias are preserved from the original.
	assert.Equal(t, balanceID, updated.ID)
	assert.Equal(t, "0#@alice#default", updated.Alias)
	assert.True(t, updated.OnHold.Equal(decimal.Zero), "OnHold should remain zero for DEBIT+CREATED")
}

// TestCreateBalanceTransactionOperationsBatch_CumulativeBalanceApplication_MixedDirections
// extends the C-1 test to verify cumulative application with both debits and
// credits hitting the same balance in a single batch.
//
// Scenario:
//
//	Balance "alice" starts at Available=1000, Version=5.
//	T1: DEBIT  200 from alice  → Available=800,  Version=6
//	T2: CREDIT 50  to   alice  → Available=850,  Version=7
//	T3: DEBIT  100 from alice  → Available=750,  Version=8
//
// All three carry snapshot version=5 from Redis.
func TestCreateBalanceTransactionOperationsBatch_CumulativeBalanceApplication_MixedDirections(t *testing.T) {
	uc := &UseCase{}

	orgID := uuid.New()
	ledgerID := uuid.New()
	balanceID := uuid.New().String()

	const snapshotVersion = 5

	updates := []balanceUpdateItem{
		{
			organizationID: orgID,
			ledgerID:       ledgerID,
			validate: pkgTransaction.Responses{
				From: map[string]pkgTransaction.Amount{
					"0#@alice#default": {
						Asset:           "USD",
						Value:           decimal.NewFromInt(200),
						Operation:       constant.DEBIT,
						TransactionType: constant.CREATED,
					},
				},
				To: map[string]pkgTransaction.Amount{},
			},
			balances: []*mmodel.Balance{{
				ID:        balanceID,
				Alias:     "0#@alice#default",
				Available: decimal.NewFromInt(1000),
				OnHold:    decimal.Zero,
				Version:   snapshotVersion,
			}},
		},
		{
			organizationID: orgID,
			ledgerID:       ledgerID,
			validate: pkgTransaction.Responses{
				From: map[string]pkgTransaction.Amount{},
				To: map[string]pkgTransaction.Amount{
					"0#@alice#default": {
						Asset:           "USD",
						Value:           decimal.NewFromInt(50),
						Operation:       constant.CREDIT,
						TransactionType: constant.CREATED,
					},
				},
			},
			balances: []*mmodel.Balance{{
				ID:        balanceID,
				Alias:     "0#@alice#default",
				Available: decimal.NewFromInt(1000),
				OnHold:    decimal.Zero,
				Version:   snapshotVersion,
			}},
		},
		{
			organizationID: orgID,
			ledgerID:       ledgerID,
			validate: pkgTransaction.Responses{
				From: map[string]pkgTransaction.Amount{
					"0#@alice#default": {
						Asset:           "USD",
						Value:           decimal.NewFromInt(100),
						Operation:       constant.DEBIT,
						TransactionType: constant.CREATED,
					},
				},
				To: map[string]pkgTransaction.Amount{},
			},
			balances: []*mmodel.Balance{{
				ID:        balanceID,
				Alias:     "0#@alice#default",
				Available: decimal.NewFromInt(1000),
				OnHold:    decimal.Zero,
				Version:   snapshotVersion,
			}},
		},
	}

	groups, err := uc.buildBatchBalanceUpdates(updates)
	require.NoError(t, err)
	require.Len(t, groups, 1)
	require.Len(t, groups[0].balances, 1)

	updated := groups[0].balances[0]

	// Expected result: 750 (initial 1000 with debit 200, credit 50, debit 100).
	assert.True(t, updated.Available.Equal(decimal.NewFromInt(750)),
		"expected 750 but got %s", updated.Available)

	// 5 + 3 operations = 8
	assert.Equal(t, int64(8), updated.Version,
		"expected version 8 but got %d", updated.Version)
}

// TestCreateBalanceTransactionOperationsBatch_FallbackCollectsAllErrors verifies
// that when multiple items fail in the fallback path, ALL errors are collected
// and joined — not just the first one.
func TestCreateBalanceTransactionOperationsBatch_FallbackCollectsAllErrors(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Standard mocks → fallback path.
	uc := &UseCase{
		TransactionRepo: transaction.NewMockRepository(ctrl),
		OperationRepo:   operation.NewMockRepository(ctrl),
		MetadataRepo:    mongodb.NewMockRepository(ctrl),
		BalanceRepo:     balance.NewMockRepository(ctrl),
		BrokerRepo:      redpanda.NewMockProducerRepository(ctrl),
		RedisRepo:       redis.NewMockRedisRepository(ctrl),
	}

	// Two items, both with empty QueueData → both will fail validation.
	err := uc.CreateBalanceTransactionOperationsBatch(context.Background(), []mmodel.Queue{
		{OrganizationID: uuid.New(), LedgerID: uuid.New(), QueueData: []mmodel.QueueData{}},
		{OrganizationID: uuid.New(), LedgerID: uuid.New(), QueueData: []mmodel.QueueData{}},
	})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "batch item 0 failed")
	assert.Contains(t, err.Error(), "batch item 1 failed")
}

func TestCreateOrUpdateTransaction_DuplicatePendingUpdatesStatus(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTransactionRepo := transaction.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionRepo: mockTransactionRepo,
	}

	orgID := uuid.New()
	ledgerID := uuid.New()
	txID := uuid.New()

	tran := &transaction.Transaction{
		ID:             txID.String(),
		OrganizationID: orgID.String(),
		LedgerID:       ledgerID.String(),
		Status: transaction.Status{
			Code: constant.CREATED,
		},
	}

	payload := transaction.TransactionProcessingPayload{
		Transaction: tran,
		Validate: &pkgTransaction.Responses{
			Pending: true,
		},
	}

	mockTransactionRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		Return(nil, nil).
		Times(1)

	mockTransactionRepo.EXPECT().
		Update(gomock.Any(), orgID, ledgerID, txID, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _, _ uuid.UUID, updated *transaction.Transaction) (*transaction.Transaction, error) {
			require.Equal(t, constant.APPROVED, updated.Status.Code)
			return updated, nil
		}).
		Times(1)

	result, duplicate, err := uc.CreateOrUpdateTransaction(context.Background(), &MockLogger{}, otel.Tracer("test"), payload)
	require.NoError(t, err)
	require.True(t, duplicate)
	require.NotNil(t, result)
	require.Equal(t, constant.APPROVED, result.Status.Code)
}
