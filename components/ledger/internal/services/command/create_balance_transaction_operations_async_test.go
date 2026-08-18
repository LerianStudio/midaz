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

	libLog "github.com/LerianStudio/lib-observability/v2/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
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
	"github.com/LerianStudio/midaz/v4/pkg/utils"
)

func TestSendTransactionToRedisQueue_PersistsPhaseZeroRolloutOwner(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	redisRepo := redis.NewMockRedisRepository(ctrl)
	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()
	originID := uuid.New()
	rolloutToken := uuid.NewString()
	redisGeneration := uuid.NewString()
	input := mtransaction.Transaction{Description: "phase-zero reverse", Send: mtransaction.Send{
		Asset: "USD", Value: decimal.NewFromInt(10),
	}}
	attempt := &mmodel.BalanceExecutionAttempt{
		Owner: transactionID.String(), Outcome: mmodel.TransactionOutcomeCommitted, Identity: transactionID,
		RedisGeneration: redisGeneration,
	}

	redisRepo.EXPECT().SeedTransactionBackup(gomock.Any(), organizationID, ledgerID, transactionID,
		gomock.Any(), *attempt).
		DoAndReturn(func(_ context.Context, _, _, _ uuid.UUID, raw []byte, seededAttempt mmodel.BalanceExecutionAttempt) error {
			queued := mmodel.TransactionRedisQueue{}
			require.NoError(t, json.Unmarshal(raw, &queued))
			assert.Equal(t, rolloutToken, queued.RevertRolloutToken)
			assert.Equal(t, "legacy", queued.RevertRolloutMode)
			assert.Equal(t, redisGeneration, queued.RedisGeneration)
			assert.Equal(t, transactionID, queued.TransactionID)
			assert.Equal(t, *attempt, seededAttempt)
			require.NotNil(t, queued.ParentTransactionID)
			assert.Equal(t, originID, *queued.ParentTransactionID)

			return nil
		})

	uc := &UseCase{TransactionRedisRepo: redisRepo}
	err := uc.SendTransactionToRedisQueue(context.Background(), organizationID, ledgerID, transactionID,
		input, &mtransaction.Responses{}, constant.CREATED, constant.ActionRevert,
		time.Date(2026, time.August, 18, 0, 0, 0, 0, time.UTC), nil, &originID,
		TransactionBackupSeedOptions{
			ExecutionAttempt: attempt, RevertRolloutMode: "legacy", RevertRolloutToken: rolloutToken,
			RedisGeneration: redisGeneration,
		})
	require.NoError(t, err)
}

func TestCreateBalanceTransactionOperationsAsync_GenerationPreflightRunsBeforePostgres(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	redisRepo := redis.NewMockRedisRepository(ctrl)
	organizationID := uuid.New()
	ledgerID := uuid.New()
	transactionID := uuid.New()
	generation := uuid.NewString()
	owner := transactionID.String()
	redisRepo.EXPECT().EnrichTransactionBackup(gomock.Any(), organizationID, ledgerID, transactionID,
		gomock.Any(), constant.ActionRevert, gomock.Any()).
		DoAndReturn(func(_ context.Context, _, _, _ uuid.UUID, _ []mmodel.OperationRedis, _ string,
			attempt *mmodel.BalanceExecutionAttempt) ([]mmodel.OperationRedis, []mmodel.BalanceRedis, bool, error) {
			require.NotNil(t, attempt)
			assert.Equal(t, generation, attempt.RedisGeneration)
			assert.Equal(t, owner, attempt.Owner)

			return nil, nil, false, errors.New("financial dataset generation changed")
		})

	parentID := uuid.NewString()
	payload := transaction.TransactionProcessingPayload{
		Transaction: &transaction.Transaction{
			ID: transactionID.String(), OrganizationID: organizationID.String(), LedgerID: ledgerID.String(),
			ParentTransactionID: &parentID,
			Operations:          []*operation.Operation{{ID: uuid.NewString(), TransactionID: transactionID.String()}},
		},
		AttemptOwner: owner, ExpectedOutcome: mmodel.TransactionOutcomeCommitted, RedisGeneration: generation,
	}
	raw, err := msgpack.Marshal(payload)
	require.NoError(t, err)
	uc := &UseCase{TransactionRedisRepo: redisRepo}
	err = uc.CreateBalanceTransactionOperationsAsync(context.Background(), mmodel.Queue{
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		QueueData:      []mmodel.QueueData{{ID: uuid.New(), Value: raw}},
	})
	require.ErrorContains(t, err, "validate current Redis economic outcome before PostgreSQL persistence")
}

// Int64Ptr returns a pointer to the given int64 value
func Int64Ptr(v int64) *int64 {
	return &v
}

func TestTransactionRedisQueue_RemainingReplayUsesPersistedValidation(t *testing.T) {
	t.Parallel()

	input := mtransaction.Transaction{
		Send: mtransaction.Send{
			Asset: "USD",
			Value: decimal.NewFromInt(100),
			Source: mtransaction.Source{From: []mtransaction.FromTo{
				{AccountAlias: "@explicit", Amount: &mtransaction.Amount{Asset: "USD", Value: decimal.NewFromInt(60)}, IsFrom: true},
				{AccountAlias: "@remaining", Remaining: "remaining", IsFrom: true},
			}},
			Distribute: mtransaction.Distribute{To: []mtransaction.FromTo{{
				AccountAlias: "@destination",
				Amount:       &mtransaction.Amount{Asset: "USD", Value: decimal.NewFromInt(100)},
			}}},
		},
	}

	mtransaction.ApplyDefaultBalanceKeys(input.Send.Source.From)
	mtransaction.ApplyDefaultBalanceKeys(input.Send.Distribute.To)
	mtransaction.MutateConcatAliases(input.Send.Source.From)
	mtransaction.MutateConcatAliases(input.Send.Distribute.To)

	validate, err := mtransaction.ValidateSendSourceAndDistribute(context.Background(), input, constant.CREATED)
	require.NoError(t, err)

	raw, err := json.Marshal(mmodel.TransactionRedisQueue{
		TransactionInput:  input,
		Validate:          validate,
		TransactionStatus: constant.CREATED,
	})
	require.NoError(t, err)

	var replay mmodel.TransactionRedisQueue
	require.NoError(t, json.Unmarshal(raw, &replay))
	require.NotNil(t, replay.Validate)

	remaining := replay.TransactionInput.Send.Source.From[1]
	assert.Nil(t, remaining.Amount, "the replay payload must keep the request expression, not a materialized amount")

	resolved, _, err := mtransaction.ValidateFromToOperation(remaining, *replay.Validate, &mtransaction.Balance{
		Available: decimal.NewFromInt(1000),
		OnHold:    decimal.Zero,
	})
	require.NoError(t, err)
	assert.True(t, decimal.NewFromInt(40).Equal(resolved.Value),
		"replay must consume the persisted validation map for the remaining amount")
}

// MockLogger is a mock implementation of logger for testing
type MockLogger struct{}

func (m *MockLogger) Log(_ context.Context, _ libLog.Level, _ string, _ ...libLog.Field) {}
func (m *MockLogger) With(_ ...libLog.Field) libLog.Logger                               { return m }

func (m *MockLogger) WithGroup(_ string) libLog.Logger { return m }

func (m *MockLogger) Enabled(_ libLog.Level) bool { return true }

func (m *MockLogger) Sync(_ context.Context) error { return nil }

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
			Transaction:     tran,
			Validate:        validate,
			Balances:        balances,
			Input:           transactionInput,
			Version:         "v2",
			AttemptOwner:    "attempt-owner",
			ExpectedOutcome: mmodel.TransactionOutcomeCommitted,
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

		mockRedisRepo.EXPECT().
			RemoveMessageFromQueueIfStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
			Return(true, nil).
			AnyTimes()

		// The economic outcome and backup survive transport and are cleared
		// atomically only after the transaction and all operations are durable.
		mockTransactionRepo.EXPECT().
			FindWithOperations(gomock.Any(), organizationID, ledgerID, uuid.MustParse(transactionID)).
			Return(tran, nil).
			Times(1)
		mockRedisRepo.EXPECT().
			FinalizeTransactionPersistence(gomock.Any(), organizationID, ledgerID, uuid.MustParse(transactionID),
				mmodel.BalanceExecutionAttempt{
					ExecutionKey: utils.TransactionBalanceExecutionKey(organizationID, ledgerID, uuid.MustParse(transactionID)),
					OutcomeKey:   utils.TransactionBalanceOutcomeKey(organizationID, ledgerID, uuid.MustParse(transactionID)),
					Owner:        "attempt-owner",
					Outcome:      mmodel.TransactionOutcomeCommitted,
					Identity:     uuid.MustParse(transactionID),
				}, gomock.Any()).
			Return(nil).
			Times(1)

		// Mock RedisRepo.Del for removing transaction from write-behind cache.
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
			Transaction:     tran,
			Validate:        validate,
			Balances:        balances,
			Input:           transactionInput,
			Version:         "v2",
			AttemptOwner:    "attempt-owner",
			ExpectedOutcome: mmodel.TransactionOutcomeCommitted,
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

		mockRedisRepo.EXPECT().
			RemoveMessageFromQueueIfStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
			Return(true, nil).
			AnyTimes()

		// Mock RedisRepo.Del for removing transaction from write-behind cache
		mockRedisRepo.EXPECT().
			Del(gomock.Any(), gomock.Any()).
			Return(nil).
			AnyTimes()
		mockTransactionRepo.EXPECT().
			FindWithOperations(gomock.Any(), organizationID, ledgerID, uuid.MustParse(transactionID)).
			Return(tran, nil).
			Times(1)
		mockRedisRepo.EXPECT().
			FinalizeTransactionPersistence(gomock.Any(), organizationID, ledgerID, uuid.MustParse(transactionID), gomock.Any(), gomock.Any()).
			Return(nil).
			Times(1)

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
			Transaction:     tran,
			Validate:        validate,
			Balances:        balances,
			Input:           transactionInput,
			Version:         "v2",
			AttemptOwner:    "attempt-owner",
			ExpectedOutcome: mmodel.TransactionOutcomeCommitted,
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

		// Mock OperationRepo.Create for both operations and assert versions exist.
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
			Create(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, op *operation.Operation) (*operation.Operation, error) {
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

				return op, nil
			}).
			Times(2)

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

		mockRedisRepo.EXPECT().
			RemoveMessageFromQueueIfStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
			Return(true, nil).
			AnyTimes()

		// Mock RedisRepo.Del for removing transaction from write-behind cache
		mockRedisRepo.EXPECT().
			Del(gomock.Any(), gomock.Any()).
			Return(nil).
			AnyTimes()
		mockTransactionRepo.EXPECT().
			FindWithOperations(gomock.Any(), organizationID, ledgerID, uuid.MustParse(transactionID)).
			Return(tran, nil).
			Times(1)
		mockRedisRepo.EXPECT().
			FinalizeTransactionPersistence(gomock.Any(), organizationID, ledgerID, uuid.MustParse(transactionID), gomock.Any(), gomock.Any()).
			Return(nil).
			Times(1)

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

		mockRedisRepo.EXPECT().
			RemoveMessageFromQueueIfStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
			Return(true, nil).
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

	mockRedisRepo.EXPECT().
		RemoveMessageFromQueueIfStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
		Return(true, nil).
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
			TransactionRedisRepo: mockRedisRepo,
		}

		ctx := context.Background()
		organizationID := uuid.New()
		ledgerID := uuid.New()
		transactionID := uuid.New()

		amount := decimal.NewFromFloat(100.00)
		avail := decimal.NewFromFloat(500.00)
		onHold := decimal.NewFromFloat(0)
		version := int64(1)

		operations := []*operation.Operation{
			{
				ID:            "op-1",
				TransactionID: transactionID.String(),
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

		mockRedisRepo.EXPECT().
			EnrichTransactionBackup(gomock.Any(), organizationID, ledgerID, transactionID, gomock.Any(), constant.ActionCommit, nil).
			DoAndReturn(func(_ context.Context, _, _, _ uuid.UUID, redisOperations []mmodel.OperationRedis, _ string, _ *mmodel.BalanceExecutionAttempt) ([]mmodel.OperationRedis, []mmodel.BalanceRedis, bool, error) {
				assert.Len(t, redisOperations, 1)
				assert.Equal(t, "op-1", redisOperations[0].ID)
				assert.Equal(t, "DEBIT", redisOperations[0].Type)
				return redisOperations, nil, false, nil
			}).
			Times(1)

		canonical, terminal, err := uc.UpdateTransactionBackupOperations(ctx, organizationID, ledgerID, transactionID,
			operations, nil, constant.ActionCommit, nil)
		require.NoError(t, err)
		assert.False(t, terminal)
		require.Len(t, canonical, 1)
		require.Equal(t, "op-1", canonical[0].ID)
	})

	t.Run("redis_failure_is_returned", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		uc := &UseCase{
			TransactionRedisRepo: mockRedisRepo,
		}

		organizationID := uuid.New()
		ledgerID := uuid.New()
		transactionID := uuid.New()
		attempt := &mmodel.BalanceExecutionAttempt{Identity: transactionID}
		mockRedisRepo.EXPECT().
			EnrichTransactionBackup(gomock.Any(), organizationID, ledgerID, transactionID,
				gomock.Any(), constant.ActionCancel, attempt).
			Return(nil, nil, false, errors.New("redis write failed"))

		_, _, err := uc.UpdateTransactionBackupOperations(context.Background(), organizationID, ledgerID,
			transactionID, nil, nil, constant.ActionCancel, attempt)
		require.ErrorContains(t, err, "redis write failed")
	})

	t.Run("terminal_receipt_with_different_balance_snapshot_is_rejected", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)
		organizationID := uuid.New()
		ledgerID := uuid.New()
		transactionID := uuid.New()
		economicOperation := &operation.Operation{ID: uuid.NewString(), TransactionID: transactionID.String()}
		expectedBalance := (&mmodel.Balance{ID: uuid.NewString(), Key: "default"}).ToRedis()
		foreignBalance := expectedBalance
		foreignBalance.Direction = constant.DirectionDebit
		attempt := &mmodel.BalanceExecutionAttempt{
			ExecutionKey: utils.TransactionBalanceExecutionKey(organizationID, ledgerID, transactionID),
			OutcomeKey:   utils.TransactionBalanceOutcomeKey(organizationID, ledgerID, transactionID),
			Owner:        uuid.NewString(), Outcome: mmodel.TransactionOutcomeCommitted, Identity: transactionID,
		}
		mockRedisRepo.EXPECT().EnrichTransactionBackup(gomock.Any(), organizationID, ledgerID, transactionID,
			gomock.Any(), constant.ActionCommit, attempt).
			Return([]mmodel.OperationRedis{economicOperation.ToRedis()}, []mmodel.BalanceRedis{foreignBalance}, true, nil)

		uc := &UseCase{TransactionRedisRepo: mockRedisRepo}
		_, _, err := uc.UpdateTransactionBackupOperations(context.Background(), organizationID, ledgerID,
			transactionID, []*operation.Operation{economicOperation}, []mmodel.BalanceRedis{expectedBalance},
			constant.ActionCommit, attempt)
		require.ErrorContains(t, err, "transaction economic effect differs from its authoritative Redis envelope")
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
