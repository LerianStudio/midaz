package command

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	constant "github.com/LerianStudio/lib-commons/v2/commons/constants"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/outbox"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/testsupport"
	pkg "github.com/LerianStudio/midaz/v3/pkg"
	midazconstant "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/dbtx"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/vmihailenco/msgpack/v5"
	"go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/mock/gomock"
)

// Int64Ptr returns a pointer to the given int64 value
func Int64Ptr(v int64) *int64 {
	return &v
}

// mockDBProvider implements dbtx.TxBeginner for testing
type mockDBProvider struct {
	db *sql.DB
}

func (m *mockDBProvider) BeginTx(ctx context.Context, opts *sql.TxOptions) (dbtx.Tx, error) {
	tx, err := m.db.BeginTx(ctx, opts)
	if err != nil {
		return nil, err
	}
	return &mockTxAdapter{tx}, nil
}

// mockTxAdapter wraps *sql.Tx to implement dbtx.Tx
type mockTxAdapter struct {
	*sql.Tx
}

func TestCreateBalanceTransactionOperationsAsync(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockTransactionRepo := transaction.NewMockRepository(ctrl)
		mockOperationRepo := operation.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)
		mockOutboxRepo := outbox.NewMockRepository(ctrl)

		// Create mock DB and DBProvider
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		mock.ExpectBegin()
		mock.ExpectCommit()

		dbProvider := &mockDBProvider{db: db}

		// Create a UseCase with all required dependencies
		uc := &UseCase{
			TransactionRepo: mockTransactionRepo,
			OperationRepo:   mockOperationRepo,
			MetadataRepo:    mockMetadataRepo,
			BalanceRepo:     mockBalanceRepo,
			RabbitMQRepo:    mockRabbitMQRepo,
			RedisRepo:       mockRedisRepo,
			OutboxRepo:      mockOutboxRepo,
			DBProvider:      dbProvider,
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
					Asset:           "USD",
					Value:           decimal.NewFromInt(50),
					Operation:       constant.DEBIT,
					TransactionType: constant.CREATED,
				},
			},
			To: map[string]pkgTransaction.Amount{
				"alias2": {
					Asset:           "EUR",
					Value:           decimal.NewFromInt(40),
					Operation:       constant.CREDIT,
					TransactionType: constant.CREATED,
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
			Status: transaction.Status{
				Code: midazconstant.CREATED,
			},
		}

		parseDSL := &pkgTransaction.Transaction{}

		// Create a transaction queue with the necessary fields
		transactionQueue := transaction.TransactionQueue{
			Transaction: tran,
			Validate:    validate,
			Balances:    balances,
			ParseDSL:    parseDSL,
		}

		transactionBytes, _ := msgpack.Marshal(transactionQueue)
		queueData := []mmodel.QueueData{
			{
				ID:    uuid.MustParse(tran.ID),
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

		// Mock OutboxRepo.Create for metadata outbox entries
		mockOutboxRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			Return(nil).
			AnyTimes()

		// Mock TransactionRepo.UpdateBalanceStatus for async balance status update
		mockTransactionRepo.EXPECT().
			UpdateBalanceStatus(gomock.Any(), organizationID, ledgerID, uuid.MustParse(transactionID), midazconstant.BalanceStatusConfirmed).
			Return(nil).
			Times(1)

		// Call the method
		callErr := uc.CreateBalanceTransactionOperationsAsync(ctx, queue)

		assert.NoError(t, callErr)
	})

	t.Run("error_update_balances", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockTransactionRepo := transaction.NewMockRepository(ctrl)
		mockOperationRepo := operation.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)
		mockOutboxRepo := outbox.NewMockRepository(ctrl)

		// Create mock DB and DBProvider with rollback expectation
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		mock.ExpectBegin()
		mock.ExpectRollback() // Expect rollback due to balance update failure

		dbProvider := &mockDBProvider{db: db}

		// Create a UseCase with mock repositories
		uc := &UseCase{
			TransactionRepo: mockTransactionRepo,
			OperationRepo:   mockOperationRepo,
			MetadataRepo:    mockMetadataRepo,
			BalanceRepo:     mockBalanceRepo,
			RabbitMQRepo:    mockRabbitMQRepo,
			RedisRepo:       mockRedisRepo,
			OutboxRepo:      mockOutboxRepo,
			DBProvider:      dbProvider,
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
					Asset:           "USD",
					Value:           decimal.NewFromInt(50),
					Operation:       constant.DEBIT,
					TransactionType: constant.CREATED,
				},
			},
			To: map[string]pkgTransaction.Amount{
				"alias2": {
					Asset:           "EUR",
					Value:           decimal.NewFromInt(40),
					Operation:       constant.CREDIT,
					TransactionType: constant.CREATED,
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
			Status: transaction.Status{
				Code: midazconstant.CREATED,
			},
		}

		parseDSL := &pkgTransaction.Transaction{}

		transactionQueue := transaction.TransactionQueue{
			Transaction: tran,
			Validate:    validate,
			Balances:    balances,
			ParseDSL:    parseDSL,
		}

		transactionBytes, _ := msgpack.Marshal(transactionQueue)
		queueData := []mmodel.QueueData{
			{
				ID:    uuid.MustParse(tran.ID),
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

		// Mock BalanceRepo.BalancesUpdate to return an error
		mockBalanceRepo.EXPECT().
			BalancesUpdate(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(errors.New("failed to update balances")).
			Times(1)

		// Call the method
		callErr := uc.CreateBalanceTransactionOperationsAsync(ctx, queue)

		assert.Error(t, callErr)
		var internalErr pkg.InternalServerError
		if errors.As(callErr, &internalErr) {
			assert.Contains(t, internalErr.Err.Error(), "failed to update balances")
		} else {
			assert.Contains(t, callErr.Error(), "failed to update balances")
		}
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
		mockOutboxRepo := outbox.NewMockRepository(ctrl)

		// Create mock DB and DBProvider
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		mock.ExpectBegin()
		mock.ExpectCommit()

		dbProvider := &mockDBProvider{db: db}

		// Create a UseCase with all required dependencies
		uc := &UseCase{
			TransactionRepo: mockTransactionRepo,
			OperationRepo:   mockOperationRepo,
			MetadataRepo:    mockMetadataRepo,
			BalanceRepo:     mockBalanceRepo,
			RabbitMQRepo:    mockRabbitMQRepo,
			RedisRepo:       mockRedisRepo,
			OutboxRepo:      mockOutboxRepo,
			DBProvider:      dbProvider,
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
					Asset:           "USD",
					Value:           decimal.NewFromInt(50),
					Operation:       constant.DEBIT,
					TransactionType: constant.CREATED,
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
			Status: transaction.Status{
				Code: midazconstant.CREATED,
			},
		}

		parseDSL := &pkgTransaction.Transaction{}

		transactionQueue := transaction.TransactionQueue{
			Transaction: tran,
			Validate:    validate,
			Balances:    balances,
			ParseDSL:    parseDSL,
		}

		transactionBytes, _ := msgpack.Marshal(transactionQueue)
		queueData := []mmodel.QueueData{
			{
				ID:    uuid.MustParse(tran.ID),
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

		// Mock TransactionRepo.Create with duplicate key error
		pgErr := &pgconn.PgError{Code: "23505"}
		mockTransactionRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			Return(nil, pgErr).
			Times(1)

		// Note: With the outbox pattern, MetadataRepo.Create is no longer called directly.
		// Metadata is queued to the outbox and processed asynchronously by a worker.

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

		// Mock OutboxRepo.Create for metadata outbox entries
		mockOutboxRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			Return(nil).
			AnyTimes()

		// Mock TransactionRepo.UpdateBalanceStatus for async balance status update
		mockTransactionRepo.EXPECT().
			UpdateBalanceStatus(gomock.Any(), organizationID, ledgerID, uuid.MustParse(transactionID), midazconstant.BalanceStatusConfirmed).
			Return(nil).
			Times(1)

		callErr := uc.CreateBalanceTransactionOperationsAsync(ctx, queue)

		assert.NoError(t, callErr) // Duplicate key errors are handled gracefully
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
		mockOutboxRepo := outbox.NewMockRepository(ctrl)

		// Create mock DB and DBProvider
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		mock.ExpectBegin()
		mock.ExpectCommit()

		dbProvider := &mockDBProvider{db: db}

		// Create a UseCase with all required dependencies
		uc := &UseCase{
			TransactionRepo: mockTransactionRepo,
			OperationRepo:   mockOperationRepo,
			MetadataRepo:    mockMetadataRepo,
			BalanceRepo:     mockBalanceRepo,
			RabbitMQRepo:    mockRabbitMQRepo,
			RedisRepo:       mockRedisRepo,
			OutboxRepo:      mockOutboxRepo,
			DBProvider:      dbProvider,
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
					Asset:           "USD",
					Value:           decimal.NewFromInt(50),
					Operation:       constant.DEBIT,
					TransactionType: constant.CREATED,
				},
			},
			To: map[string]pkgTransaction.Amount{
				"alias2": {
					Asset:           "EUR",
					Value:           decimal.NewFromInt(40),
					Operation:       constant.CREDIT,
					TransactionType: constant.CREATED,
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
			Status: transaction.Status{
				Code: midazconstant.CREATED,
			},
		}

		parseDSL := &pkgTransaction.Transaction{}

		// Create a transaction queue with the necessary fields
		transactionQueue := transaction.TransactionQueue{
			Transaction: tran,
			Validate:    validate,
			Balances:    balances,
			ParseDSL:    parseDSL,
		}

		transactionBytes, _ := msgpack.Marshal(transactionQueue)
		queueData := []mmodel.QueueData{
			{
				ID:    uuid.MustParse(tran.ID),
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

		// Note: With the outbox pattern, MetadataRepo.Create is no longer called directly.
		// Metadata is queued to the outbox and processed asynchronously by a worker.

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

		// Mock OutboxRepo.Create for metadata outbox entries
		mockOutboxRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			Return(nil).
			AnyTimes()

		// Mock TransactionRepo.UpdateBalanceStatus for async balance status update
		mockTransactionRepo.EXPECT().
			UpdateBalanceStatus(gomock.Any(), organizationID, ledgerID, uuid.MustParse(transactionID), midazconstant.BalanceStatusConfirmed).
			Return(nil).
			Times(1)

		// Call the method
		callErr := uc.CreateBalanceTransactionOperationsAsync(ctx, queue)

		assert.NoError(t, callErr)
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
		mockOutboxRepo := outbox.NewMockRepository(ctrl)

		// Create mock DB and DBProvider - expect rollback due to operation creation failure
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		mock.ExpectBegin()
		mock.ExpectRollback() // Expect rollback when operation creation fails

		dbProvider := &mockDBProvider{db: db}

		// Create a UseCase with all required dependencies
		uc := &UseCase{
			TransactionRepo: mockTransactionRepo,
			OperationRepo:   mockOperationRepo,
			MetadataRepo:    mockMetadataRepo,
			BalanceRepo:     mockBalanceRepo,
			RabbitMQRepo:    mockRabbitMQRepo,
			RedisRepo:       mockRedisRepo,
			OutboxRepo:      mockOutboxRepo,
			DBProvider:      dbProvider,
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
					Asset:           "USD",
					Value:           decimal.NewFromInt(50),
					Operation:       constant.DEBIT,
					TransactionType: constant.CREATED,
				},
			},
			To: map[string]pkgTransaction.Amount{
				"alias2": {
					Asset:           "EUR",
					Value:           decimal.NewFromInt(40),
					Operation:       constant.CREDIT,
					TransactionType: constant.CREATED,
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
			Status: transaction.Status{
				Code: midazconstant.CREATED,
			},
		}

		parseDSL := &pkgTransaction.Transaction{}

		// Create a transaction queue with the necessary fields
		transactionQueue := transaction.TransactionQueue{
			Transaction: tran,
			Validate:    validate,
			Balances:    balances,
			ParseDSL:    parseDSL,
		}

		transactionBytes, _ := msgpack.Marshal(transactionQueue)
		queueData := []mmodel.QueueData{
			{
				ID:    uuid.MustParse(tran.ID),
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

		// Mock OperationRepo.Create to return an error for the first operation
		// Note: MetadataRepo.Create is NOT called because it happens AFTER the transaction commits,
		// and the transaction rolls back when operation creation fails
		operationError := errors.New("failed to create operation")
		mockOperationRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			Return(nil, operationError).
			Times(1)

		// Call the method
		callErr := uc.CreateBalanceTransactionOperationsAsync(ctx, queue)

		assert.Error(t, callErr)
		var internalErr pkg.InternalServerError
		if errors.As(callErr, &internalErr) {
			assert.Contains(t, internalErr.Err.Error(), "failed to create operation")
		} else {
			assert.Contains(t, callErr.Error(), "failed to create operation")
		}
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
		mockOutboxRepo := outbox.NewMockRepository(ctrl)

		// Create mock DB and DBProvider
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		mock.ExpectBegin()
		mock.ExpectCommit()

		dbProvider := &mockDBProvider{db: db}

		// Create a UseCase with all required dependencies
		uc := &UseCase{
			TransactionRepo: mockTransactionRepo,
			OperationRepo:   mockOperationRepo,
			MetadataRepo:    mockMetadataRepo,
			BalanceRepo:     mockBalanceRepo,
			RabbitMQRepo:    mockRabbitMQRepo,
			RedisRepo:       mockRedisRepo,
			OutboxRepo:      mockOutboxRepo,
			DBProvider:      dbProvider,
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
					Asset:           "USD",
					Value:           decimal.NewFromInt(50),
					Operation:       constant.DEBIT,
					TransactionType: constant.CREATED,
				},
			},
			To: map[string]pkgTransaction.Amount{
				"alias2": {
					Asset:           "EUR",
					Value:           decimal.NewFromInt(40),
					Operation:       constant.CREDIT,
					TransactionType: constant.CREATED,
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
			Status: transaction.Status{
				Code: midazconstant.CREATED,
			},
		}

		parseDSL := &pkgTransaction.Transaction{}

		// Create a transaction queue with the necessary fields
		transactionQueue := transaction.TransactionQueue{
			Transaction: tran,
			Validate:    validate,
			Balances:    balances,
			ParseDSL:    parseDSL,
		}

		transactionBytes, _ := msgpack.Marshal(transactionQueue)
		queueData := []mmodel.QueueData{
			{
				ID:    uuid.MustParse(tran.ID),
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

		// Note: With the outbox pattern, MetadataRepo.Create is no longer called directly.
		// Metadata is queued to the outbox and processed asynchronously by a worker.

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

		// Mock OutboxRepo.Create for metadata outbox entries
		mockOutboxRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			Return(nil).
			AnyTimes()

		// Mock TransactionRepo.UpdateBalanceStatus for async balance status update
		mockTransactionRepo.EXPECT().
			UpdateBalanceStatus(gomock.Any(), organizationID, ledgerID, uuid.MustParse(transactionID), midazconstant.BalanceStatusConfirmed).
			Return(nil).
			Times(1)

		// Call the method
		callErr := uc.CreateBalanceTransactionOperationsAsync(ctx, queue)

		assert.NoError(t, callErr) // Duplicate key errors are handled gracefully
	})

	t.Run("success_queues_operation_metadata_to_outbox", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockTransactionRepo := transaction.NewMockRepository(ctrl)
		mockOperationRepo := operation.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)
		mockOutboxRepo := outbox.NewMockRepository(ctrl)

		// Create mock DB and DBProvider
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		mock.ExpectBegin()
		mock.ExpectCommit()

		dbProvider := &mockDBProvider{db: db}

		// Create a UseCase with all required dependencies
		uc := &UseCase{
			TransactionRepo: mockTransactionRepo,
			OperationRepo:   mockOperationRepo,
			MetadataRepo:    mockMetadataRepo,
			BalanceRepo:     mockBalanceRepo,
			RabbitMQRepo:    mockRabbitMQRepo,
			RedisRepo:       mockRedisRepo,
			OutboxRepo:      mockOutboxRepo,
			DBProvider:      dbProvider,
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
					Asset:           "USD",
					Value:           decimal.NewFromInt(50),
					Operation:       constant.DEBIT,
					TransactionType: constant.CREATED,
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

		// Create operations for the transaction with metadata
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
			Status: transaction.Status{
				Code: midazconstant.CREATED,
			},
		}

		parseDSL := &pkgTransaction.Transaction{}

		// Create a transaction queue with the necessary fields
		transactionQueue := transaction.TransactionQueue{
			Transaction: tran,
			Validate:    validate,
			Balances:    balances,
			ParseDSL:    parseDSL,
		}

		transactionBytes, _ := msgpack.Marshal(transactionQueue)
		queueData := []mmodel.QueueData{
			{
				ID:    uuid.MustParse(tran.ID),
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

		// Mock OperationRepo.Create for the operation
		mockOperationRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			Return(operation1, nil).
			Times(1)

		// With the outbox pattern, metadata is queued to the outbox and processed
		// asynchronously by a worker. MetadataRepo.Create is not called directly.
		// Instead, OutboxRepo.Create is called to queue the metadata writes.

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

		// Mock OutboxRepo.Create for metadata outbox entries - verifies metadata is queued to outbox
		mockOutboxRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			Return(nil).
			AnyTimes()

		// Mock TransactionRepo.UpdateBalanceStatus for async balance status update
		mockTransactionRepo.EXPECT().
			UpdateBalanceStatus(gomock.Any(), organizationID, ledgerID, uuid.MustParse(transactionID), midazconstant.BalanceStatusConfirmed).
			Return(nil).
			Times(1)

		// Call the method
		callErr := uc.CreateBalanceTransactionOperationsAsync(ctx, queue)

		// With the outbox pattern, metadata is written atomically with the transaction.
		// The operation should succeed since all outbox writes succeed.
		assert.NoError(t, callErr, "operation should succeed when metadata is queued to outbox")
	})

	t.Run("error_outbox_create_fails", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockTransactionRepo := transaction.NewMockRepository(ctrl)
		mockOperationRepo := operation.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)
		mockOutboxRepo := outbox.NewMockRepository(ctrl)

		// Create mock DB and DBProvider
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("failed to create sqlmock: %v", err)
		}
		defer db.Close()

		mock.ExpectBegin()
		mock.ExpectRollback()

		dbProvider := &mockDBProvider{db: db}

		// Create a UseCase with all required dependencies
		uc := &UseCase{
			TransactionRepo: mockTransactionRepo,
			OperationRepo:   mockOperationRepo,
			MetadataRepo:    mockMetadataRepo,
			BalanceRepo:     mockBalanceRepo,
			RabbitMQRepo:    mockRabbitMQRepo,
			RedisRepo:       mockRedisRepo,
			OutboxRepo:      mockOutboxRepo,
			DBProvider:      dbProvider,
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
					Asset:           "USD",
					Value:           decimal.NewFromInt(50),
					Operation:       constant.DEBIT,
					TransactionType: constant.CREATED,
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

		// Create operations for the transaction with metadata
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
			Status: transaction.Status{
				Code: midazconstant.CREATED,
			},
		}

		parseDSL := &pkgTransaction.Transaction{}

		// Create a transaction queue with the necessary fields
		transactionQueue := transaction.TransactionQueue{
			Transaction: tran,
			Validate:    validate,
			Balances:    balances,
			ParseDSL:    parseDSL,
		}

		transactionBytes, _ := msgpack.Marshal(transactionQueue)
		queueData := []mmodel.QueueData{
			{
				ID:    uuid.MustParse(tran.ID),
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

		// Mock OperationRepo.Create for the operation
		mockOperationRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			Return(operation1, nil).
			Times(1)

		// Mock OutboxRepo.Create to return an error - simulates outbox persistence failure
		outboxErr := errors.New("outbox persistence failed")
		mockOutboxRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			Return(outboxErr).
			Times(1)

		// Mock RabbitMQRepo.ProducerDefault for transaction events (may still be called in goroutine)
		mockRabbitMQRepo.EXPECT().
			ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, nil).
			AnyTimes()

		// Mock RedisRepo.RemoveMessageFromQueue (may be called in cleanup)
		mockRedisRepo.EXPECT().
			RemoveMessageFromQueue(gomock.Any(), gomock.Any()).
			Return(nil).
			AnyTimes()

		// Call the method
		callErr := uc.CreateBalanceTransactionOperationsAsync(ctx, queue)

		// When outbox persistence fails, the transaction should be rolled back and an error returned
		assert.Error(t, callErr, "operation should fail when outbox persistence fails")
	})
}

func TestCreateOrUpdateTransaction_UnknownStatusCode_Panics(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockTransactionRepo := transaction.NewMockRepository(ctrl)

	uc := &UseCase{
		TransactionRepo: mockTransactionRepo,
	}

	ctx := context.Background()
	logger := &testsupport.MockLogger{}
	tracer := noop.NewTracerProvider().Tracer("test")

	unknownTran := &transaction.Transaction{
		ID: uuid.New().String(),
		Status: transaction.Status{
			Code: "UNKNOWN_STATUS",
		},
	}

	tq := transaction.TransactionQueue{
		Transaction: unknownTran,
		Validate:    &pkgTransaction.Responses{},
		ParseDSL:    &pkgTransaction.Transaction{},
	}

	assert.Panics(t, func() {
		_, _ = uc.CreateOrUpdateTransaction(ctx, logger, tracer, tq)
	}, "expected panic for unknown status code")
}

func TestCreateMetadataAsync(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockMetadataRepo := mongodb.NewMockRepository(ctrl)

	uc := &UseCase{
		MetadataRepo: mockMetadataRepo,
	}

	ctx := context.Background()

	logger := &testsupport.MockLogger{}
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

		callErr := uc.CreateMetadataAsync(ctx, logger, metadata, ID, collection)
		assert.Error(t, callErr)
		var internalErr pkg.InternalServerError
		if errors.As(callErr, &internalErr) {
			assert.Contains(t, internalErr.Err.Error(), "failed to create metadata")
		} else {
			assert.Contains(t, callErr.Error(), "failed to create metadata")
		}
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
	mockOutboxRepo := outbox.NewMockRepository(ctrl)

	// Create mock DB and DBProvider
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectCommit()

	dbProvider := &mockDBProvider{db: db}

	// Create a real UseCase with mock repositories
	uc := &UseCase{
		OperationRepo:   mockOperationRepo,
		TransactionRepo: mockTransactionRepo,
		MetadataRepo:    mockMetadataRepo,
		BalanceRepo:     mockBalanceRepo,
		RabbitMQRepo:    mockRabbitMQRepo,
		RedisRepo:       mockRedisRepo,
		OutboxRepo:      mockOutboxRepo,
		DBProvider:      dbProvider,
	}

	ctx := context.Background()
	organizationID := uuid.New()
	ledgerID := uuid.New()

	// Create a transaction queue with valid data
	validate := &pkgTransaction.Responses{
		Aliases: []string{"alias1"},
		From: map[string]pkgTransaction.Amount{
			"alias1": {
				Asset:           "USD",
				Value:           decimal.NewFromInt(50),
				Operation:       constant.DEBIT,
				TransactionType: constant.CREATED,
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
		Status: transaction.Status{
			Code: midazconstant.CREATED,
		},
	}

	parseDSL := &pkgTransaction.Transaction{}

	transactionQueue := transaction.TransactionQueue{
		Transaction: tran,
		Validate:    validate,
		Balances:    balances,
		ParseDSL:    parseDSL,
	}

	transactionBytes, _ := msgpack.Marshal(transactionQueue)
	queueData := []mmodel.QueueData{
		{
			ID:    uuid.MustParse(tran.ID),
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

	// Mock OutboxRepo.Create for metadata outbox entries
	mockOutboxRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()

	// Mock TransactionRepo.UpdateBalanceStatus for async balance status update
	mockTransactionRepo.EXPECT().
		UpdateBalanceStatus(gomock.Any(), organizationID, ledgerID, uuid.MustParse(tran.ID), midazconstant.BalanceStatusConfirmed).
		Return(nil).
		AnyTimes()

	// Call the method - this should not panic
	uc.CreateBTOSync(ctx, queue)
}
