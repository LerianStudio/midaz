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

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/rabbitmq"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// testData holds common test data used across multiple tests
type testData struct {
	organizationID   uuid.UUID
	ledgerID         uuid.UUID
	transactionID    string
	transactionInput *mtransaction.Transaction
	validate         *mtransaction.Responses
	balances         []*mmodel.Balance
	tran             *transaction.Transaction
}

// createTestData creates common test data for transaction write tests
func createTestData(organizationID, ledgerID uuid.UUID) *testData {
	transactionID := uuid.New().String()

	transactionInput := &mtransaction.Transaction{}

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

// setupMocksForDirectWrite sets up all mocks needed for CreateBalanceTransactionOperationsAsync,
// the direct database write behind WriteTransactionSync.
// Note: Balance repository and UUIDs are not needed because balance persistence
// is now async via BalanceSyncWorker (hot balance updated by Lua script).
func setupMocksForDirectWrite(
	mockTransactionRepo *transaction.MockRepository,
	mockMetadataRepo *mongodb.MockRepository,
	mockRabbitMQRepo *rabbitmq.MockProducerRepository,
	mockRedisRepo *redis.MockRedisRepository,
	mockBalanceRepo *balance.MockRepository,
	tran *transaction.Transaction,
) {
	// Note: Balance updates are handled by BalanceSyncWorker, not in this flow

	expectLegacyTransactionWrite(mockTransactionRepo, true)

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

	// Mock RedisRepo.Del for removing transaction from write-behind cache
	mockRedisRepo.EXPECT().
		Del(gomock.Any(), gomock.Any()).
		Return(nil).
		AnyTimes()
}

// TestWriteTransaction persists an annotation directly whatever
// RABBITMQ_TRANSACTION_ASYNC says: the legacy queue publish carries no broker
// confirmation, so the caller may only answer after the database write.
// The producer mock records no expectation, so any publish fails the test.
func TestWriteTransaction(t *testing.T) {
	for name, asyncEnv := range map[string]string{
		"async_env_true":  "true",
		"async_env_TRUE":  "TRUE",
		"async_env_false": "false",
		"async_env_empty": "",
	} {
		t.Run("writes_to_database_without_publishing_when_"+name, func(t *testing.T) {
			t.Setenv("RABBITMQ_TRANSACTION_ASYNC", asyncEnv)

			ctrl := gomock.NewController(t)
			mockTransactionRepo := transaction.NewMockRepository(ctrl)
			mockMetadataRepo := mongodb.NewMockRepository(ctrl)
			mockRedisRepo := redis.NewMockRedisRepository(ctrl)

			organizationID := uuid.New()
			ledgerID := uuid.New()
			td := createTestData(organizationID, ledgerID)
			td.tran.Status = transaction.Status{Code: constant.NOTED}

			uc := &UseCase{
				TransactionRepo:         mockTransactionRepo,
				TransactionMetadataRepo: mockMetadataRepo,
				RabbitMQRepo:            rabbitmq.NewMockProducerRepository(ctrl),
				TransactionRedisRepo:    mockRedisRepo,
			}

			backup, err := json.Marshal(mmodel.TransactionRedisQueue{TransactionStatus: constant.NOTED})
			require.NoError(t, err)

			backupRemoved := make(chan struct{})

			dbTx := expectLegacyTransactionWrite(mockTransactionRepo, true)
			mockMetadataRepo.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
			mockRedisRepo.EXPECT().ReadMessageFromQueue(gomock.Any(), gomock.Any()).Return(backup, nil).AnyTimes()
			mockRedisRepo.EXPECT().RemoveMessageFromQueue(gomock.Any(), gomock.Any()).
				DoAndReturn(func(context.Context, string) error {
					close(backupRemoved)

					return nil
				}).
				Times(1)
			mockRedisRepo.EXPECT().Del(gomock.Any(), gomock.Any()).Return(nil).Times(1)

			err = uc.WriteTransaction(context.Background(), organizationID, ledgerID, td.transactionInput, td.validate, td.balances, nil, td.tran)
			require.NoError(t, err)
			assert.True(t, dbTx.commitCalled)

			select {
			case <-backupRemoved:
			case <-time.After(5 * time.Second):
				t.Fatal("the backup entry must be removed once the transaction is written")
			}
		})
	}

	t.Run("keeps_the_backup_and_returns_the_database_error_when_async_env_true", func(t *testing.T) {
		t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "true")

		ctrl := gomock.NewController(t)
		mockTransactionRepo := transaction.NewMockRepository(ctrl)

		organizationID := uuid.New()
		ledgerID := uuid.New()
		td := createTestData(organizationID, ledgerID)
		td.tran.Status = transaction.Status{Code: constant.NOTED}

		uc := &UseCase{
			TransactionRepo:      mockTransactionRepo,
			RabbitMQRepo:         rabbitmq.NewMockProducerRepository(ctrl),
			TransactionRedisRepo: redis.NewMockRedisRepository(ctrl),
		}

		dbTx := &mockDBTransaction{}
		mockTransactionRepo.EXPECT().BeginTx(gomock.Any()).Return(dbTx, nil).Times(1)
		mockTransactionRepo.EXPECT().
			CreateBulkTx(gomock.Any(), dbTx, gomock.Any()).
			Return(nil, errors.New("database connection failed")).
			Times(1)

		err := uc.WriteTransaction(context.Background(), organizationID, ledgerID, td.transactionInput, td.validate, td.balances, nil, td.tran)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "database connection failed")
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
			BalanceRepo:             mockBalanceRepo,
			TransactionRepo:         mockTransactionRepo,
			TransactionMetadataRepo: mockMetadataRepo,
			RabbitMQRepo:            mockRabbitMQRepo,
			TransactionRedisRepo:    mockRedisRepo,
		}

		// Setup mocks for the direct database write
		setupMocksForDirectWrite(mockTransactionRepo, mockMetadataRepo, mockRabbitMQRepo, mockRedisRepo, mockBalanceRepo, td.tran)

		err := uc.WriteTransactionSync(ctx, organizationID, ledgerID, td.transactionInput, td.validate, td.balances, nil, td.tran)

		// Allow background goroutines (DeleteWriteBehindTransaction) to complete before ctrl.Finish
		time.Sleep(100 * time.Millisecond)

		assert.NoError(t, err)
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
			BalanceRepo:             mockBalanceRepo,
			TransactionRepo:         mockTransactionRepo,
			TransactionMetadataRepo: mockMetadataRepo,
			RabbitMQRepo:            mockRabbitMQRepo,
			TransactionRedisRepo:    mockRedisRepo,
		}

		// Note: Balance updates are handled by BalanceSyncWorker, not in this flow

		// The transaction insert fails (not a duplicate key)
		dbTx := &mockDBTransaction{}
		mockTransactionRepo.EXPECT().BeginTx(gomock.Any()).Return(dbTx, nil).Times(1)
		mockTransactionRepo.EXPECT().
			CreateBulkTx(gomock.Any(), dbTx, gomock.Any()).
			Return(nil, errors.New("failed to create transaction")).
			Times(1)

		err := uc.WriteTransactionSync(ctx, organizationID, ledgerID, td.transactionInput, td.validate, td.balances, nil, td.tran)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed to create transaction")
		assert.True(t, dbTx.rollbackCalled)
		assert.False(t, dbTx.commitCalled)
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
		transactionInput := &mtransaction.Transaction{}
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
			Metadata:       map[string]any{},
		}

		uc := &UseCase{
			BalanceRepo:             mockBalanceRepo,
			TransactionRepo:         mockTransactionRepo,
			TransactionMetadataRepo: mockMetadataRepo,
			RabbitMQRepo:            mockRabbitMQRepo,
			TransactionRedisRepo:    mockRedisRepo,
		}

		// Note: Balance updates are handled by BalanceSyncWorker, not in this flow

		expectLegacyTransactionWrite(mockTransactionRepo, true)

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

		// Mock RedisRepo.Del for removing transaction from write-behind cache
		mockRedisRepo.EXPECT().
			Del(gomock.Any(), gomock.Any()).
			Return(nil).
			AnyTimes()

		err := uc.WriteTransactionSync(ctx, organizationID, ledgerID, transactionInput, validate, balances, nil, tran)

		// Allow background goroutines (DeleteWriteBehindTransaction) to complete before ctrl.Finish
		time.Sleep(100 * time.Millisecond)

		assert.NoError(t, err)
	})
}
