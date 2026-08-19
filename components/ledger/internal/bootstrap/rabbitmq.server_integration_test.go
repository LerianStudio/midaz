//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	libRabbitmq "github.com/LerianStudio/lib-commons/v6/commons/rabbitmq"
	libObservability "github.com/LerianStudio/lib-observability/v2"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v2/tracing"
	libZap "github.com/LerianStudio/lib-observability/v2/zap"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
	"go.uber.org/mock/gomock"

	mongodb "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/revertclaim"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/rabbitmq"
	redis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/mmodel"
	"github.com/LerianStudio/midaz/v4/pkg/mtransaction"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	postgrestestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
	rmqtestutil "github.com/LerianStudio/midaz/v4/tests/utils/rabbitmq"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"
)

// =============================================================================
// WIRE FORMAT COMPATIBILITY INTEGRATION TESTS
// =============================================================================

// legacyTransactionQueue represents the old struct format before the rename.
// IMPORTANT: This struct must NOT have msgpack tags - it simulates old producers
// that serialize with field name "ParseDSL" instead of "Input".
type legacyTransactionQueue struct {
	Validate    *mtransaction.Responses   `json:"validate"`
	Balances    []*mmodel.Balance         `json:"balances"`
	Transaction *transaction.Transaction  `json:"transaction"`
	ParseDSL    *mtransaction.Transaction `json:"parseDSL"`
}

type failOnceTransactionFinalizer struct {
	redis.RedisRepository
	failure error
}

func (r *failOnceTransactionFinalizer) FinalizeTransactionPersistence(
	ctx context.Context,
	organizationID, ledgerID, transactionID uuid.UUID,
	attempt mmodel.BalanceExecutionAttempt,
	operations []mmodel.OperationRedis,
	balancesAfter []mmodel.BalanceRedis,
) error {
	if r.failure != nil {
		err := r.failure
		r.failure = nil

		return err
	}

	return r.RedisRepository.FinalizeTransactionPersistence(ctx, organizationID, ledgerID, transactionID,
		attempt, operations, balancesAfter)
}

func TestIntegration_HandlerBTOBulk_DefaultAsyncRedeliveryCompletesOneDurableHandoff(t *testing.T) {
	t.Setenv("RABBITMQ_TRANSACTION_EVENTS_ENABLED", "false")
	t.Setenv("AUDIT_LOG_ENABLED", "false")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "true")
	t.Setenv("BULK_RECORDER_ENABLED", "true")
	require.True(t, shouldUseBulkMode(&Config{RabbitMQTransactionAsync: true, BulkRecorderEnabled: true}),
		"the production async configuration must select this bulk handler")

	ctx := context.Background()
	postgresContainer := postgrestestutil.SetupContainer(t)
	postgresDSN := postgrestestutil.BuildConnectionString(postgresContainer.Host, postgresContainer.Port, postgresContainer.Config)
	postgresClient := postgrestestutil.CreatePostgresClient(t, postgresDSN, postgresDSN,
		postgresContainer.Config.DBName, postgrestestutil.FindMigrationsPath(t, "transaction"))
	redisContainer := redistestutil.SetupContainer(t)
	redisConnection := redistestutil.CreateConnection(t, redisContainer.Addr)
	redisRepo, err := redis.NewConsumerRedis(redisConnection)
	require.NoError(t, err)

	transactionRepo := transaction.NewTransactionPostgreSQLRepository(postgresClient)
	operationRepo := operation.NewOperationPostgreSQLRepository(postgresClient)
	claimRepo := revertclaim.NewPostgreSQLRepository(postgresClient)
	failingRedis := &failOnceTransactionFinalizer{
		RedisRepository: redisRepo,
		failure:         errors.New("simulated lost response after PostgreSQL commit"),
	}
	useCase := &command.UseCase{
		TransactionRepo:      transactionRepo,
		OperationRepo:        operationRepo,
		RevertClaimRepo:      claimRepo,
		TransactionRedisRepo: failingRedis,
	}

	organizationID := uuid.New()
	ledgerID := uuid.New()
	originID := uuid.New()
	reverseID := uuid.New()
	owner := reverseID.String()
	fixedTime := time.Date(2026, time.August, 18, 2, 0, 0, 0, time.UTC)
	amount := decimal.NewFromInt(100)
	approved := constant.APPROVED
	_, err = transactionRepo.Create(ctx, &transaction.Transaction{
		ID:             originID.String(),
		OrganizationID: organizationID.String(),
		LedgerID:       ledgerID.String(),
		Description:    "origin for async bulk redelivery",
		Amount:         &amount,
		AssetCode:      "USD",
		Status:         transaction.Status{Code: approved, Description: &approved},
		CreatedAt:      fixedTime,
		UpdatedAt:      fixedTime,
	})
	require.NoError(t, err)

	created := constant.CREATED
	zero := decimal.Zero
	versionBefore := int64(1)
	versionAfter := int64(2)
	debitAccountID := uuid.New()
	debitBalanceID := uuid.New()
	creditAccountID := uuid.New()
	creditBalanceID := uuid.New()
	operations := []*operation.Operation{
		recoveryOperation(reverseID, organizationID, ledgerID, debitAccountID, debitBalanceID, "@destination", constant.DEBIT,
			constant.DirectionDebit, &amount, &amount, &zero, &versionBefore, &versionAfter, &created, fixedTime),
		recoveryOperation(reverseID, organizationID, ledgerID, creditAccountID, creditBalanceID, "@source", constant.CREDIT,
			constant.DirectionCredit, &amount, &zero, &amount, &versionBefore, &versionAfter, &created, fixedTime),
	}
	beforeBalances := []*mmodel.Balance{
		{
			OrganizationID: organizationID.String(), LedgerID: ledgerID.String(), AccountID: debitAccountID.String(),
			ID: debitBalanceID.String(), Alias: "@destination", Key: constant.DefaultBalanceKey,
			AssetCode: "USD", Available: amount, OnHold: zero, Version: versionBefore,
			AccountType: "deposit", AllowSending: true, AllowReceiving: true,
			Direction: constant.DirectionDebit, OverdraftUsed: zero,
		},
		{
			OrganizationID: organizationID.String(), LedgerID: ledgerID.String(), AccountID: creditAccountID.String(),
			ID: creditBalanceID.String(), Alias: "@source", Key: constant.DefaultBalanceKey,
			AssetCode: "USD", Available: zero, OnHold: zero, Version: versionBefore,
			AccountType: "deposit", AllowSending: true, AllowReceiving: true,
			Direction: constant.DirectionCredit, OverdraftUsed: zero,
		},
	}
	afterBalances := []*mmodel.Balance{
		{
			OrganizationID: organizationID.String(), LedgerID: ledgerID.String(), AccountID: debitAccountID.String(),
			ID: debitBalanceID.String(), Alias: "@destination", Key: constant.DefaultBalanceKey,
			AssetCode: "USD", Available: zero, OnHold: zero, Version: versionAfter,
			AccountType: "deposit", AllowSending: true, AllowReceiving: true,
			Direction: constant.DirectionDebit, OverdraftUsed: zero,
		},
		{
			OrganizationID: organizationID.String(), LedgerID: ledgerID.String(), AccountID: creditAccountID.String(),
			ID: creditBalanceID.String(), Alias: "@source", Key: constant.DefaultBalanceKey,
			AssetCode: "USD", Available: amount, OnHold: zero, Version: versionAfter,
			AccountType: "deposit", AllowSending: true, AllowReceiving: true,
			Direction: constant.DirectionCredit, OverdraftUsed: zero,
		},
	}
	parent := originID.String()
	debit := mtransaction.Amount{
		Asset: "USD", Value: amount, Operation: constant.DEBIT, Direction: constant.DirectionDebit,
	}
	credit := mtransaction.Amount{
		Asset: "USD", Value: amount, Operation: constant.CREDIT, Direction: constant.DirectionCredit,
	}
	input := mtransaction.Transaction{
		Description: "reverse persisted by the default async bulk consumer",
		Send: mtransaction.Send{
			Asset: "USD", Value: amount,
			Source: mtransaction.Source{From: []mtransaction.FromTo{{
				AccountAlias: "@destination", BalanceKey: constant.DefaultBalanceKey, Amount: &debit, IsFrom: true,
			}}},
			Distribute: mtransaction.Distribute{To: []mtransaction.FromTo{{
				AccountAlias: "@source", BalanceKey: constant.DefaultBalanceKey, Amount: &credit,
			}}},
		},
	}
	validate := &mtransaction.Responses{
		From: map[string]mtransaction.Amount{
			mtransaction.ConcatAlias(0, mtransaction.AliasKey("@destination", constant.DefaultBalanceKey)): debit,
		},
		To: map[string]mtransaction.Amount{
			mtransaction.ConcatAlias(0, mtransaction.AliasKey("@source", constant.DefaultBalanceKey)): credit,
		},
	}
	payload := transaction.TransactionProcessingPayload{
		Validate: validate,
		Transaction: &transaction.Transaction{
			ID:                  reverseID.String(),
			ParentTransactionID: &parent,
			OrganizationID:      organizationID.String(),
			LedgerID:            ledgerID.String(),
			Description:         input.Description,
			Amount:              &amount,
			AssetCode:           "USD",
			Status:              transaction.Status{Code: created, Description: &created},
			Operations:          operations,
			CreatedAt:           fixedTime,
			UpdatedAt:           fixedTime,
		},
		Input:           &input,
		Version:         "v2",
		BalancesAfter:   afterBalances,
		AttemptOwner:    owner,
		ExpectedOutcome: mmodel.TransactionOutcomeCommitted,
	}

	legacyHash, err := utils.LegacyTransactionIdempotencyHash(input)
	require.NoError(t, err)
	legacyKey := utils.IdempotencyInternalKey(organizationID, ledgerID, legacyHash)
	originHash := libCommons.HashSHA256(utils.RevertIdempotencyHashSource(originID))
	originKey := utils.IdempotencyInternalKey(organizationID, ledgerID, originHash)
	claim, acquired, err := claimRepo.Claim(ctx, organizationID, ledgerID, originID, reverseID,
		&legacyKey, &owner, nil, nil, nil)
	require.NoError(t, err)
	require.True(t, acquired)
	require.Equal(t, reverseID, claim.ReverseTransactionID)
	for _, key := range []string{originKey, legacyKey} {
		owned, acquireErr := redisRepo.AcquireOwnedKey(ctx, key, owner, 0)
		require.NoError(t, acquireErr)
		require.True(t, owned)
	}

	before := mmodel.BalancesToRedis(beforeBalances)
	after := mmodel.BalancesToRedis(afterBalances)
	backup := mmodel.TransactionRedisQueue{
		TransactionID:        reverseID,
		ParentTransactionID:  &originID,
		OrganizationID:       organizationID,
		LedgerID:             ledgerID,
		Balances:             before,
		BalancesAfter:        after,
		TransactionInput:     input,
		Validate:             payload.Validate,
		TransactionStatus:    created,
		Action:               constant.ActionRevert,
		AttemptOwner:         owner,
		ExpectedOutcome:      mmodel.TransactionOutcomeCommitted,
		RevertLegacyFenceKey: legacyKey,
		TransactionDate:      fixedTime,
		Operations:           []mmodel.OperationRedis{operations[0].ToRedis(), operations[1].ToRedis()},
	}
	backupRaw, err := json.Marshal(backup)
	require.NoError(t, err)
	backupKey := utils.TransactionInternalKey(organizationID, ledgerID, reverseID.String())
	require.NoError(t, redisRepo.AddMessageToQueue(ctx, backupKey, backupRaw))
	outcomeKey := utils.TransactionBalanceOutcomeKey(organizationID, ledgerID, reverseID)
	outcomeRaw, err := json.Marshal(mmodel.BalanceExecutionOutcome{
		Identity: reverseID,
		Outcome:  mmodel.TransactionOutcomeCommitted,
		Owner:    owner,
		Before:   before,
		After:    after,
	})
	require.NoError(t, err)
	require.NoError(t, redisRepo.Set(ctx, outcomeKey, string(outcomeRaw), 0))

	payloadRaw, err := msgpack.Marshal(payload)
	require.NoError(t, err)
	messageRaw, err := msgpack.Marshal(mmodel.Queue{
		OrganizationID: organizationID,
		LedgerID:       ledgerID,
		QueueData:      []mmodel.QueueData{{ID: uuid.New(), Value: payloadRaw}},
	})
	require.NoError(t, err)
	delivery := amqp.Delivery{Body: messageRaw}

	// The production bulk handler receives the real AMQP wire payload while
	// Postgres and Redis are real containers. Re-invoking the same Delivery is
	// the deterministic broker redelivery/lost-ack boundary; ConsumerRoutes'
	// acknowledgement mechanics have their own RabbitMQ integration suite.
	_, err = handlerBTOBulk(ctx, []amqp.Delivery{delivery}, useCase, nil)
	require.ErrorContains(t, err, "simulated lost response after PostgreSQL commit")
	var transactionCount int
	require.NoError(t, postgresContainer.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM transaction WHERE id = $1`, reverseID).Scan(&transactionCount))
	require.Equal(t, 1, transactionCount)
	require.Equal(t, 2, postgrestestutil.CountOperationsByTransactionID(t, postgresContainer.DB, reverseID))
	completed, err := claimRepo.Get(ctx, organizationID, ledgerID, originID)
	require.NoError(t, err)
	require.Equal(t, revertclaim.StateCompleted, completed.State)
	_, err = redisRepo.ReadMessageFromQueue(ctx, backupKey)
	require.NoError(t, err, "a crash after PostgreSQL durability must preserve the exact backup")
	outcomeValue, err := redisRepo.Get(ctx, outcomeKey)
	require.NoError(t, err)
	require.NotEmpty(t, outcomeValue, "a crash after PostgreSQL durability must preserve the immutable outcome")

	_, err = handlerBTOBulk(ctx, []amqp.Delivery{delivery}, useCase, nil)
	require.NoError(t, err, "redelivery must finish exact cleanup without a second persistence")
	_, err = handlerBTOBulk(ctx, []amqp.Delivery{delivery}, useCase, nil)
	require.NoError(t, err, "a lost acknowledgement after cleanup must be an exact no-op")
	require.NoError(t, postgresContainer.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM transaction WHERE id = $1`, reverseID).Scan(&transactionCount))
	require.Equal(t, 1, transactionCount)
	require.Equal(t, 2, postgrestestutil.CountOperationsByTransactionID(t, postgresContainer.DB, reverseID))
	_, err = redisRepo.ReadMessageFromQueue(ctx, backupKey)
	require.Error(t, err)
	outcomeValue, err = redisRepo.Get(ctx, outcomeKey)
	require.NoError(t, err)
	require.Empty(t, outcomeValue)
	for _, key := range []string{originKey, legacyKey} {
		replay, replayErr := redisRepo.Get(ctx, key)
		require.NoError(t, replayErr)
		require.NotEmpty(t, replay)
		var transactionReplay transaction.Transaction
		require.NoError(t, json.Unmarshal([]byte(replay), &transactionReplay))
		require.Equal(t, reverseID.String(), transactionReplay.ID)
		require.Equal(t, originID.String(), *transactionReplay.ParentTransactionID)
	}
}

// TestIntegration_HandlerBTOQueue_LegacyWireFormatCompatibility tests the full consumer flow:
// 1. Old producer publishes message with ParseDSL field name (legacy format)
// 2. Message goes through RabbitMQ
// 3. New consumer (handlerBTOQueue) receives and processes it
// 4. CreateBalanceTransactionOperationsAsync deserializes using msgpack:"ParseDSL" tag
//
// This validates that rolling deployments work: old producers → new consumers.
func TestIntegration_HandlerBTOQueue_LegacyWireFormatCompatibility(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	t.Run("old_producer_message_through_rabbitmq_to_new_consumer", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		// Setup mocks for repositories
		mockTransactionRepo := transaction.NewMockRepository(ctrl)
		mockOperationRepo := operation.NewMockRepository(ctrl)
		mockMetadataRepo := mongodb.NewMockRepository(ctrl)
		mockBalanceRepo := balance.NewMockRepository(ctrl)
		mockRabbitMQRepo := rabbitmq.NewMockProducerRepository(ctrl)
		mockRedisRepo := redis.NewMockRedisRepository(ctrl)

		// Track when processing completes
		var processingDone sync.WaitGroup
		processingDone.Add(1)

		// Setup expectations - processing should succeed
		mockRedisRepo.EXPECT().
			ListBalanceByKey(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, nil).
			AnyTimes()

		mockBalanceRepo.EXPECT().
			BalancesUpdate(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil).
			Times(1)

		mockTransactionRepo.EXPECT().
			Create(gomock.Any(), gomock.Any()).
			DoAndReturn(func(_ context.Context, tran *transaction.Transaction) (*transaction.Transaction, error) {
				// Signal that processing completed successfully
				defer processingDone.Done()
				t.Logf("Transaction created: ID=%s, Description=%s", tran.ID, tran.Description)
				return tran, nil
			}).
			Times(1)

		mockMetadataRepo.EXPECT().
			Create(gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil).
			AnyTimes()

		mockRabbitMQRepo.EXPECT().
			ProducerDefault(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, nil).
			AnyTimes()

		mockRedisRepo.EXPECT().
			RemoveMessageFromQueueIfStatus(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), false).
			Return(true, nil).
			AnyTimes()

		mockRedisRepo.EXPECT().
			Del(gomock.Any(), gomock.Any()).
			Return(nil).
			AnyTimes()

		// Create UseCase with mocked repos
		uc := &command.UseCase{
			TransactionRepo:         mockTransactionRepo,
			OperationRepo:           mockOperationRepo,
			TransactionMetadataRepo: mockMetadataRepo,
			BalanceRepo:             mockBalanceRepo,
			RabbitMQRepo:            mockRabbitMQRepo,
			TransactionRedisRepo:    mockRedisRepo,
		}

		// Setup RabbitMQ testcontainer
		rmqContainer := rmqtestutil.SetupReusableContainer(t)

		// Setup exchange and queue
		queueName := "test-bto-legacy-compat-queue"
		exchange := "test-bto-exchange"
		routingKey := "bto.legacy.test"

		rmqtestutil.SetupExchange(t, rmqContainer.Channel, exchange, "topic")
		rmqtestutil.SetupQueue(t, rmqContainer.Channel, queueName, exchange, routingKey)

		// Create consumer infrastructure (following existing integration test patterns)
		logger, err := libZap.New(libZap.Config{Environment: libZap.EnvironmentDevelopment, OTelLibraryName: "midaz-tests"})
		require.NoError(t, err)
		healthCheckURL := "http://" + rmqContainer.Host + ":" + rmqContainer.MgmtPort

		conn := &libRabbitmq.RabbitMQConnection{
			ConnectionStringSource:   rmqContainer.URI,
			HealthCheckURL:           healthCheckURL,
			AllowInsecureHealthCheck: true,
			Host:                     rmqContainer.Host,
			Port:                     rmqContainer.AMQPPort,
			User:                     rmqtestutil.DefaultUser,
			Pass:                     rmqtestutil.DefaultPassword,
			Logger:                   logger,
		}

		telemetry := &libOpentelemetry.Telemetry{}

		consumerRoutes, err := rabbitmq.NewConsumerRoutes(conn, 1, 1, logger, telemetry)
		require.NoError(t, err, "failed to create consumer routes")
		t.Cleanup(consumerRoutes.StopConsumers)

		// Create MultiQueueConsumer with mocked UseCase
		consumer := &MultiQueueConsumer{
			consumerRoutes: consumerRoutes,
			UseCase:        uc,
		}

		// Register handler for our test queue
		consumerRoutes.Register(queueName, consumer.handlerBTOQueue)

		// Start consumer
		err = consumerRoutes.RunConsumers()
		require.NoError(t, err)

		// Give consumer time to start
		time.Sleep(500 * time.Millisecond)

		// Create test data using OLD struct format (simulating old producer)
		organizationID := uuid.New()
		ledgerID := uuid.New()
		transactionID := uuid.New().String()

		validate := &mtransaction.Responses{
			Aliases: []string{"@source#BRL", "@dest#BRL"},
			From: map[string]mtransaction.Amount{
				"@source#BRL": {Asset: "BRL", Value: decimal.NewFromInt(100)},
			},
			To: map[string]mtransaction.Amount{
				"@dest#BRL": {Asset: "BRL", Value: decimal.NewFromInt(100)},
			},
		}

		balances := []*mmodel.Balance{
			{
				ID:             uuid.New().String(),
				AccountID:      uuid.New().String(),
				OrganizationID: organizationID.String(),
				LedgerID:       ledgerID.String(),
				Alias:          "@source#BRL",
				Available:      decimal.NewFromInt(1000),
				OnHold:         decimal.NewFromInt(0),
				Version:        1,
				AccountType:    "deposit",
				AllowSending:   true,
				AllowReceiving: true,
				AssetCode:      "BRL",
			},
		}

		tran := &transaction.Transaction{
			ID:             transactionID,
			OrganizationID: organizationID.String(),
			LedgerID:       ledgerID.String(),
			Description:    "Legacy format transaction from old producer",
			AssetCode:      "BRL",
			Status: transaction.Status{
				Code: constant.CREATED,
			},
			Operations: []*operation.Operation{},
			Metadata:   map[string]interface{}{},
		}

		// KEY: Use ParseDSL field (old name) - this is what old producers send
		transactionInput := &mtransaction.Transaction{
			Description: "Legacy input from old producer",
			Send: mtransaction.Send{
				Asset: "BRL",
				Value: decimal.NewFromInt(100),
			},
		}

		// Create message using OLD struct (no msgpack tags = field names as-is)
		oldFormatPayload := legacyTransactionQueue{
			Validate:    validate,
			Balances:    balances,
			Transaction: tran,
			ParseDSL:    transactionInput,
		}

		// Serialize the payload with msgpack
		payloadBytes, err := msgpack.Marshal(oldFormatPayload)
		require.NoError(t, err, "failed to marshal legacy payload")

		// Wrap in mmodel.Queue structure (as the handler expects)
		queueMessage := mmodel.Queue{
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			QueueData: []mmodel.QueueData{
				{
					ID:    uuid.New(),
					Value: payloadBytes,
				},
			},
		}

		// Serialize the queue message with msgpack (handler expects msgpack)
		messageBody, err := msgpack.Marshal(queueMessage)
		require.NoError(t, err, "failed to marshal queue message")

		// Create context with tracing
		ctx := libObservability.ContextWithLogger(context.Background(), logger)
		ctx = libObservability.ContextWithHeaderID(ctx, uuid.New().String())

		// Publish to RabbitMQ (simulating old producer)
		t.Log("Publishing legacy format message to RabbitMQ...")
		err = rmqContainer.Channel.PublishWithContext(
			ctx,
			exchange,
			routingKey,
			false,
			false,
			amqp.Publishing{
				ContentType: "application/msgpack",
				Body:        messageBody,
			},
		)
		require.NoError(t, err, "failed to publish message")

		// Wait for processing to complete (with timeout)
		done := make(chan struct{})
		go func() {
			processingDone.Wait()
			close(done)
		}()

		select {
		case <-done:
			t.Log("SUCCESS: Legacy format message was processed by new consumer!")
		case <-time.After(10 * time.Second):
			t.Fatal("TIMEOUT: Message was not processed within 10 seconds")
		}

		// Verify mocks were called correctly (implicit via gomock expectations)
		assert.True(t, true, "All mock expectations were met")
	})
}
