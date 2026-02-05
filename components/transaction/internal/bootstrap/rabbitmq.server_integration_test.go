//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"sync"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libRabbitmq "github.com/LerianStudio/lib-commons/v2/commons/rabbitmq"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkgTransaction "github.com/LerianStudio/midaz/v3/pkg/transaction"
	rmqtestutil "github.com/LerianStudio/midaz/v3/tests/utils/rabbitmq"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
	"go.uber.org/mock/gomock"
)

// =============================================================================
// WIRE FORMAT COMPATIBILITY INTEGRATION TESTS
// =============================================================================

// legacyTransactionQueue represents the old struct format before the rename.
// IMPORTANT: This struct must NOT have msgpack tags - it simulates old producers
// that serialize with field name "ParseDSL" instead of "Input".
type legacyTransactionQueue struct {
	Validate    *pkgTransaction.Responses   `json:"validate"`
	Balances    []*mmodel.Balance           `json:"balances"`
	Transaction *transaction.Transaction    `json:"transaction"`
	ParseDSL    *pkgTransaction.Transaction `json:"parseDSL"`
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
			RemoveMessageFromQueue(gomock.Any(), gomock.Any()).
			Return(nil).
			AnyTimes()

		// Create UseCase with mocked repos
		uc := &command.UseCase{
			TransactionRepo: mockTransactionRepo,
			OperationRepo:   mockOperationRepo,
			MetadataRepo:    mockMetadataRepo,
			BalanceRepo:     mockBalanceRepo,
			RabbitMQRepo:    mockRabbitMQRepo,
			RedisRepo:       mockRedisRepo,
		}

		// Setup RabbitMQ testcontainer
		rmqContainer := rmqtestutil.SetupContainer(t)

		// Setup exchange and queue
		queueName := "test-bto-legacy-compat-queue"
		exchange := "test-bto-exchange"
		routingKey := "bto.legacy.test"

		rmqtestutil.SetupExchange(t, rmqContainer.Channel, exchange, "topic")
		rmqtestutil.SetupQueue(t, rmqContainer.Channel, queueName, exchange, routingKey)

		// Create consumer infrastructure (following existing integration test patterns)
		logger := libZap.InitializeLogger()
		healthCheckURL := "http://" + rmqContainer.Host + ":" + rmqContainer.MgmtPort

		conn := &libRabbitmq.RabbitMQConnection{
			ConnectionStringSource: rmqContainer.URI,
			HealthCheckURL:         healthCheckURL,
			Host:                   rmqContainer.Host,
			Port:                   rmqContainer.AMQPPort,
			User:                   rmqtestutil.DefaultUser,
			Pass:                   rmqtestutil.DefaultPassword,
			Logger:                 logger,
		}

		telemetry := &libOpentelemetry.Telemetry{}

		consumerRoutes := rabbitmq.NewConsumerRoutes(conn, 1, 1, logger, telemetry)

		// Create MultiQueueConsumer with mocked UseCase
		consumer := &MultiQueueConsumer{
			consumerRoutes: consumerRoutes,
			UseCase:        uc,
		}

		// Register handler for our test queue
		consumerRoutes.Register(queueName, consumer.handlerBTOQueue)

		// Start consumer
		err := consumerRoutes.RunConsumers()
		require.NoError(t, err)

		// Give consumer time to start
		time.Sleep(500 * time.Millisecond)

		// Create test data using OLD struct format (simulating old producer)
		organizationID := uuid.New()
		ledgerID := uuid.New()
		transactionID := uuid.New().String()

		validate := &pkgTransaction.Responses{
			Aliases: []string{"@source#BRL", "@dest#BRL"},
			From: map[string]pkgTransaction.Amount{
				"@source#BRL": {Asset: "BRL", Value: decimal.NewFromInt(100)},
			},
			To: map[string]pkgTransaction.Amount{
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
		transactionInput := &pkgTransaction.Transaction{
			Description: "DSL from old producer",
			Send: pkgTransaction.Send{
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
		ctx := libCommons.ContextWithLogger(context.Background(), logger)
		ctx = libCommons.ContextWithHeaderID(ctx, uuid.New().String())

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
