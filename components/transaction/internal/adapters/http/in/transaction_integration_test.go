//go:build integration

package in

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	libRabbitmq "github.com/LerianStudio/lib-commons/v2/commons/rabbitmq"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/query"
	cn "github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	"github.com/LerianStudio/midaz/v3/pkg/net/http"
	mongotestutil "github.com/LerianStudio/midaz/v3/pkg/testutils/mongodb"
	postgrestestutil "github.com/LerianStudio/midaz/v3/pkg/testutils/postgres"
	rabbitmqtestutil "github.com/LerianStudio/midaz/v3/pkg/testutils/rabbitmq"
	redistestutil "github.com/LerianStudio/midaz/v3/pkg/testutils/redis"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vmihailenco/msgpack/v5"
)

// testInfra holds all test infrastructure components.
type testInfra struct {
	pgContainer    *postgrestestutil.ContainerResult
	mongoContainer *mongotestutil.ContainerResult
	redisContainer *redistestutil.ContainerResult
	pgConn         *libPostgres.PostgresConnection
	redisRepo      redis.RedisRepository
	metadataRepo   mongodb.Repository
	handler        *TransactionHandler
	app            *fiber.App
	orgID          uuid.UUID
	ledgerID       uuid.UUID
}

// setupTestInfra initializes all containers and creates the handler.
func setupTestInfra(t *testing.T) *testInfra {
	t.Helper()

	// Disable async RabbitMQ features (no RabbitMQ container in this test)
	t.Setenv("RABBITMQ_TRANSACTION_EVENTS_ENABLED", "false")
	t.Setenv("AUDIT_LOG_ENABLED", "false")

	infra := &testInfra{}

	// Start containers
	infra.pgContainer = postgrestestutil.SetupContainer(t)
	infra.mongoContainer = mongotestutil.SetupContainer(t)
	infra.redisContainer = redistestutil.SetupContainer(t)

	// Register cleanup
	t.Cleanup(func() {
		infra.redisContainer.Cleanup()
		infra.mongoContainer.Cleanup()
		infra.pgContainer.Cleanup()
	})

	// Create PostgreSQL connection following lib-commons pattern
	logger := libZap.InitializeLogger()
	migrationsPath := postgrestestutil.FindMigrationsPath(t, "transaction")
	connStr := postgrestestutil.BuildConnectionString(infra.pgContainer.Host, infra.pgContainer.Port, infra.pgContainer.Config)

	infra.pgConn = &libPostgres.PostgresConnection{
		ConnectionStringPrimary: connStr,
		ConnectionStringReplica: connStr,
		PrimaryDBName:           infra.pgContainer.Config.DBName,
		ReplicaDBName:           infra.pgContainer.Config.DBName,
		MigrationsPath:          migrationsPath,
		Logger:                  logger,
	}

	// Create MongoDB connection
	mongoConn := mongotestutil.CreateConnection(t, infra.mongoContainer.URI, "test_db")

	// Create Redis connection
	redisConn := redistestutil.CreateConnection(t, infra.redisContainer.Addr)

	// Create repositories
	transactionRepo := transaction.NewTransactionPostgreSQLRepository(infra.pgConn)
	operationRepo := operation.NewOperationPostgreSQLRepository(infra.pgConn)
	balanceRepo := balance.NewBalancePostgreSQLRepository(infra.pgConn)
	metadataRepo := mongodb.NewMetadataMongoDBRepository(mongoConn)
	redisRepo, err := redis.NewConsumerRedis(redisConn, false)
	require.NoError(t, err, "failed to create Redis repository")

	// Store repositories for test assertions
	infra.redisRepo = redisRepo
	infra.metadataRepo = metadataRepo

	// Create use cases
	queryUC := &query.UseCase{
		TransactionRepo: transactionRepo,
		OperationRepo:   operationRepo,
		BalanceRepo:     balanceRepo,
		MetadataRepo:    metadataRepo,
		RedisRepo:       redisRepo,
	}
	commandUC := &command.UseCase{
		TransactionRepo: transactionRepo,
		OperationRepo:   operationRepo,
		BalanceRepo:     balanceRepo,
		MetadataRepo:    metadataRepo,
		RedisRepo:       redisRepo,
	}

	// Create handler
	infra.handler = &TransactionHandler{
		Query:   queryUC,
		Command: commandUC,
	}

	// Use fake UUIDs for org and ledger (they're in the onboarding component, not transaction)
	// The transaction component only validates these IDs exist in its own tables
	infra.orgID = libCommons.GenerateUUIDv7()
	infra.ledgerID = libCommons.GenerateUUIDv7()

	// Setup Fiber app with routes
	infra.app = fiber.New()
	infra.setupRoutes()

	return infra
}

// setupRoutes registers handler routes on the Fiber app.
func (infra *testInfra) setupRoutes() {
	// Middleware to inject path params as locals
	paramMiddleware := func(c *fiber.Ctx) error {
		orgIDStr := c.Params("organization_id")
		ledgerIDStr := c.Params("ledger_id")
		txIDStr := c.Params("transaction_id")

		if orgIDStr != "" {
			orgID, _ := uuid.Parse(orgIDStr)
			c.Locals("organization_id", orgID)
		}
		if ledgerIDStr != "" {
			ledgerID, _ := uuid.Parse(ledgerIDStr)
			c.Locals("ledger_id", ledgerID)
		}
		if txIDStr != "" {
			txID, _ := uuid.Parse(txIDStr)
			c.Locals("transaction_id", txID)
		}
		return c.Next()
	}

	infra.app.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/json",
		paramMiddleware, http.WithBody(new(transaction.CreateTransactionInput), infra.handler.CreateTransactionJSON))
	infra.app.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/commit",
		paramMiddleware, infra.handler.CommitTransaction)
	infra.app.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id/revert",
		paramMiddleware, infra.handler.RevertTransaction)
	infra.app.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id",
		paramMiddleware, infra.handler.GetTransaction)
}

// getBalanceFromRedis retrieves a balance from Redis and unmarshals it to BalanceRedis.
// Returns nil if the key is not found or unmarshalling fails.
func getBalanceFromRedis(t *testing.T, ctx context.Context, redisRepo redis.RedisRepository, orgID, ledgerID uuid.UUID, alias, balanceKey string) *mmodel.BalanceRedis {
	t.Helper()

	key := utils.BalanceInternalKey(orgID, ledgerID, alias+"#"+balanceKey)
	value, err := redisRepo.Get(ctx, key)

	if err != nil || value == "" {
		return nil
	}

	var balance mmodel.BalanceRedis
	if err := json.Unmarshal([]byte(value), &balance); err != nil {
		t.Logf("Failed to unmarshal Redis balance for key %s: %v", key, err)
		return nil
	}

	return &balance
}

// TestIntegration_TransactionHandler_CreateTransactionJSON_Sync validates that
// CreateTransactionJSON correctly creates a non-pending transaction in sync mode.
// This tests the complete flow:
// 1. CreateTransactionJSON receives input, sets status=CREATED
// 2. createTransaction validates and prepares data
// 3. ValidateSendSourceAndDistribute calculates source/destination totals
// 4. GetBalances retrieves balances (Redis cache -> PostgreSQL fallback)
// 5. BuildOperations creates Operation objects with DEBIT/CREDIT types
// 6. TransactionExecute -> CreateBTOExecuteSync (since RABBITMQ_TRANSACTION_ASYNC=false)
// 7. CreateBalanceTransactionOperationsAsync persists all data
// 8. Returns HTTP 201 with complete transaction
func TestIntegration_TransactionHandler_CreateTransactionJSON_Sync(t *testing.T) {
	// Arrange
	infra := setupTestInfra(t)

	// Ensure sync mode is enabled (RABBITMQ_TRANSACTION_ASYNC=false is default when not set)
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	// Use fake account IDs (account table is in onboarding component)
	sourceAccountID := libCommons.GenerateUUIDv7()
	destAccountID := libCommons.GenerateUUIDv7()

	// Create source balance (@source-account) with 1000 USD available
	sourceBalanceParams := postgrestestutil.DefaultBalanceParams()
	sourceBalanceParams.Alias = "@source-account"
	sourceBalanceParams.AssetCode = "USD"
	sourceBalanceParams.Available = decimal.NewFromInt(1000)
	sourceBalanceParams.OnHold = decimal.Zero
	sourceBalanceID := postgrestestutil.CreateTestBalance(t, infra.pgContainer.DB,
		infra.orgID, infra.ledgerID, sourceAccountID, sourceBalanceParams)

	// Create destination balance (@dest-account) with 0 USD available
	destBalanceParams := postgrestestutil.DefaultBalanceParams()
	destBalanceParams.Alias = "@dest-account"
	destBalanceParams.AssetCode = "USD"
	destBalanceParams.Available = decimal.Zero
	destBalanceParams.OnHold = decimal.Zero
	destBalanceID := postgrestestutil.CreateTestBalance(t, infra.pgContainer.DB,
		infra.orgID, infra.ledgerID, destAccountID, destBalanceParams)

	// Build JSON request body for transferring 100 USD from @source-account to @dest-account
	// Includes metadata at transaction level and operation level (from/to)
	requestBody := `{
		"description": "Integration test transaction - 100 USD transfer",
		"pending": false,
		"metadata": {
			"order_id": "ORD-12345",
			"priority": "high"
		},
		"send": {
			"asset": "USD",
			"value": "100",
			"source": {
				"from": [
					{
						"accountAlias": "@source-account",
						"amount": {
							"asset": "USD",
							"value": "100"
						},
						"metadata": {
							"source_ref": "SRC-001",
							"department": "treasury"
						}
					}
				]
			},
			"distribute": {
				"to": [
					{
						"accountAlias": "@dest-account",
						"amount": {
							"asset": "USD",
							"value": "100"
						},
						"metadata": {
							"dest_ref": "DST-001",
							"purpose": "payment"
						}
					}
				]
			}
		}
	}`

	// Act: Call CreateTransactionJSON endpoint
	req := httptest.NewRequest("POST",
		"/v1/organizations/"+infra.orgID.String()+"/ledgers/"+infra.ledgerID.String()+"/transactions/json",
		bytes.NewBufferString(requestBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := infra.app.Test(req, -1)

	// Assert: HTTP Response
	require.NoError(t, err, "HTTP request should not fail")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "should read response body")

	// Debug: print response if not 201
	if resp.StatusCode != 201 {
		t.Logf("Response status: %d, body: %s", resp.StatusCode, string(body))
	}

	assert.Equal(t, 201, resp.StatusCode,
		"expected HTTP 201 Created, got %d: %s", resp.StatusCode, string(body))

	// Parse response JSON
	var result map[string]any
	err = json.Unmarshal(body, &result)
	require.NoError(t, err, "response should be valid JSON")

	// Assert: Transaction ID is valid UUID
	txIDStr, ok := result["id"].(string)
	require.True(t, ok, "response should contain transaction id")
	txID, err := uuid.Parse(txIDStr)
	require.NoError(t, err, "transaction ID should be valid UUID")

	// Assert: HTTP response returns CREATED status (the original transaction object)
	// Note: The HTTP response returns the transaction as it was created, with status CREATED.
	// The status is updated to APPROVED during CreateBalanceTransactionOperationsAsync,
	// which operates on a deserialized copy, not the original object returned to the client.
	status, ok := result["status"].(map[string]any)
	require.True(t, ok, "response should contain status object")
	assert.Equal(t, cn.CREATED, status["code"],
		"HTTP response should return CREATED status (original transaction object)")

	// Assert: Verify transaction exists in database with APPROVED status
	// The database is updated to APPROVED during sync processing, even though
	// the HTTP response returns the original object with CREATED status.
	dbStatus := postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, txID)
	assert.Equal(t, cn.APPROVED, dbStatus,
		"database transaction status should be APPROVED after sync processing")

	// Assert: Source balance decreased by 100 (1000 -> 900)
	sourceAvailable := postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, sourceBalanceID)
	assert.True(t, sourceAvailable.Equal(decimal.NewFromInt(900)),
		"source balance should be 900 after transaction, got %s", sourceAvailable.String())

	// Assert: Destination balance increased by 100 (0 -> 100)
	destAvailable := postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, destBalanceID)
	assert.True(t, destAvailable.Equal(decimal.NewFromInt(100)),
		"destination balance should be 100 after transaction, got %s", destAvailable.String())

	// Assert: 2 operations created (1 DEBIT, 1 CREDIT)
	opCount := postgrestestutil.CountOperationsByTransactionID(t, infra.pgContainer.DB, txID)
	assert.Equal(t, 2, opCount,
		"should have exactly 2 operations (1 DEBIT + 1 CREDIT), got %d", opCount)

	// Assert: Verify operation types in database
	var debitCount, creditCount int
	err = infra.pgContainer.DB.QueryRow(`
		SELECT
			SUM(CASE WHEN type = 'DEBIT' THEN 1 ELSE 0 END) as debit_count,
			SUM(CASE WHEN type = 'CREDIT' THEN 1 ELSE 0 END) as credit_count
		FROM operation
		WHERE transaction_id = $1
	`, txID).Scan(&debitCount, &creditCount)
	require.NoError(t, err, "should query operation types")
	assert.Equal(t, 1, debitCount, "should have exactly 1 DEBIT operation")
	assert.Equal(t, 1, creditCount, "should have exactly 1 CREDIT operation")

	// Assert: Redis balances are updated correctly
	ctx := context.Background()

	// Verify source balance in Redis
	sourceRedis := getBalanceFromRedis(t, ctx, infra.redisRepo, infra.orgID, infra.ledgerID, "@source-account", "default")
	require.NotNil(t, sourceRedis, "source balance should exist in Redis")
	assert.True(t, sourceRedis.Available.Equal(decimal.NewFromInt(900)),
		"Redis source available should be 900, got %s", sourceRedis.Available.String())
	assert.True(t, sourceRedis.OnHold.Equal(decimal.Zero),
		"Redis source onHold should be 0, got %s", sourceRedis.OnHold.String())
	assert.Greater(t, sourceRedis.Version, int64(0),
		"Redis source version should be incremented, got %d", sourceRedis.Version)

	// Verify destination balance in Redis
	destRedis := getBalanceFromRedis(t, ctx, infra.redisRepo, infra.orgID, infra.ledgerID, "@dest-account", "default")
	require.NotNil(t, destRedis, "dest balance should exist in Redis")
	assert.True(t, destRedis.Available.Equal(decimal.NewFromInt(100)),
		"Redis dest available should be 100, got %s", destRedis.Available.String())
	assert.True(t, destRedis.OnHold.Equal(decimal.Zero),
		"Redis dest onHold should be 0, got %s", destRedis.OnHold.String())
	assert.Greater(t, destRedis.Version, int64(0),
		"Redis dest version should be incremented, got %d", destRedis.Version)

	// Assert: Response contains expected fields
	assert.Equal(t, infra.orgID.String(), result["organizationId"],
		"response should contain correct organization ID")
	assert.Equal(t, infra.ledgerID.String(), result["ledgerId"],
		"response should contain correct ledger ID")
	assert.Equal(t, "USD", result["assetCode"],
		"response should contain correct asset code")

	// Assert: Amount is correct
	if amount, ok := result["amount"].(string); ok {
		assert.True(t, strings.Contains(amount, "100"),
			"response amount should be 100, got %s", amount)
	}

	// =====================================
	// Metadata Validation (MongoDB)
	// =====================================

	// Assert: Transaction metadata is saved correctly in MongoDB
	txMetadata, err := infra.metadataRepo.FindByEntity(ctx, "Transaction", txID.String())
	require.NoError(t, err, "should find transaction metadata in MongoDB")
	require.NotNil(t, txMetadata, "transaction metadata should exist in MongoDB")

	assert.Equal(t, txID.String(), txMetadata.EntityID, "metadata should reference correct transaction ID")
	assert.Equal(t, "Transaction", txMetadata.EntityName, "metadata entity name should be Transaction")
	assert.Equal(t, "ORD-12345", txMetadata.Data["order_id"], "transaction metadata order_id should match")
	assert.Equal(t, "high", txMetadata.Data["priority"], "transaction metadata priority should match")

	// Query operation IDs from PostgreSQL (we need them to query operation metadata)
	var debitOpID, creditOpID uuid.UUID
	rows, err := infra.pgContainer.DB.Query(`
		SELECT id, type FROM operation WHERE transaction_id = $1 ORDER BY type
	`, txID)
	require.NoError(t, err, "should query operations")
	defer rows.Close()

	for rows.Next() {
		var opID uuid.UUID
		var opType string
		err := rows.Scan(&opID, &opType)
		require.NoError(t, err, "should scan operation row")

		if opType == "CREDIT" {
			creditOpID = opID
		} else if opType == "DEBIT" {
			debitOpID = opID
		}
	}
	require.NoError(t, rows.Err(), "should iterate operations without error")

	require.NotEqual(t, uuid.Nil, debitOpID, "DEBIT operation should exist")
	require.NotEqual(t, uuid.Nil, creditOpID, "CREDIT operation should exist")

	// Assert: DEBIT operation metadata (source) is saved correctly
	debitMetadata, err := infra.metadataRepo.FindByEntity(ctx, "Operation", debitOpID.String())
	require.NoError(t, err, "should find DEBIT operation metadata in MongoDB")
	require.NotNil(t, debitMetadata, "DEBIT operation metadata should exist in MongoDB")

	assert.Equal(t, debitOpID.String(), debitMetadata.EntityID, "DEBIT metadata should reference correct operation ID")
	assert.Equal(t, "Operation", debitMetadata.EntityName, "DEBIT metadata entity name should be Operation")
	assert.Equal(t, "SRC-001", debitMetadata.Data["source_ref"], "DEBIT operation source_ref should match")
	assert.Equal(t, "treasury", debitMetadata.Data["department"], "DEBIT operation department should match")

	// Assert: CREDIT operation metadata (destination) is saved correctly
	creditMetadata, err := infra.metadataRepo.FindByEntity(ctx, "Operation", creditOpID.String())
	require.NoError(t, err, "should find CREDIT operation metadata in MongoDB")
	require.NotNil(t, creditMetadata, "CREDIT operation metadata should exist in MongoDB")

	assert.Equal(t, creditOpID.String(), creditMetadata.EntityID, "CREDIT metadata should reference correct operation ID")
	assert.Equal(t, "Operation", creditMetadata.EntityName, "CREDIT metadata entity name should be Operation")
	assert.Equal(t, "DST-001", creditMetadata.Data["dest_ref"], "CREDIT operation dest_ref should match")
	assert.Equal(t, "payment", creditMetadata.Data["purpose"], "CREDIT operation purpose should match")
}

// testAsyncInfra holds all test infrastructure components for async transaction tests.
// It extends testInfra with RabbitMQ container and consumer.
type testAsyncInfra struct {
	pgContainer          *postgrestestutil.ContainerResult
	mongoContainer       *mongotestutil.ContainerResult
	redisContainer       *redistestutil.ContainerResult
	rabbitmqContainer    *rabbitmqtestutil.ContainerResult
	pgConn               *libPostgres.PostgresConnection
	redisRepo            redis.RedisRepository
	handler              *TransactionHandler
	commandUC            *command.UseCase
	consumerRoutes       *rabbitmq.ConsumerRoutes
	consumerRabbitMQConn *libRabbitmq.RabbitMQConnection // Separate connection for consumer (to close on cleanup)
	app                  *fiber.App
	orgID                uuid.UUID
	ledgerID             uuid.UUID
}

// testMultiQueueConsumer is a test-local version of the consumer to avoid import cycles.
type testMultiQueueConsumer struct {
	consumerRoutes *rabbitmq.ConsumerRoutes
	useCase        *command.UseCase
}

// newTestMultiQueueConsumer creates a new test consumer instance.
func newTestMultiQueueConsumer(routes *rabbitmq.ConsumerRoutes, useCase *command.UseCase) *testMultiQueueConsumer {
	consumer := &testMultiQueueConsumer{
		consumerRoutes: routes,
		useCase:        useCase,
	}

	// Register handlers for each queue (mirrors bootstrap.NewMultiQueueConsumer)
	routes.Register(os.Getenv("RABBITMQ_TRANSACTION_BALANCE_OPERATION_QUEUE"), consumer.handlerBTOQueue)

	return consumer
}

// run starts consumers for all registered queues.
func (mq *testMultiQueueConsumer) run() error {
	return mq.consumerRoutes.RunConsumers()
}

// handlerBTOQueue processes messages from the balance transaction operation queue.
// This mirrors the logic in bootstrap.MultiQueueConsumer.handlerBTOQueue.
func (mq *testMultiQueueConsumer) handlerBTOQueue(ctx context.Context, body []byte) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "consumer.handler_balance_update")
	defer span.End()

	logger.Info("Processing message from balance_retry_queue_fifo")

	var message mmodel.Queue

	err := msgpack.Unmarshal(body, &message)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Error unmarshalling message JSON", err)
		logger.Errorf("Error unmarshalling balance message JSON: %v", err)
		return err
	}

	logger.Infof("Transaction message consumed: %s", message.QueueData[0].ID)

	err = mq.useCase.CreateBalanceTransactionOperationsAsync(ctx, message)
	if err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Error creating transaction", err)
		logger.Errorf("Error creating transaction: %v", err)
		return err
	}

	return nil
}

// setupAsyncTestInfra initializes all containers including RabbitMQ and creates the handler with async support.
func setupAsyncTestInfra(t *testing.T) *testAsyncInfra {
	t.Helper()

	infra := &testAsyncInfra{}

	// Start containers
	infra.pgContainer = postgrestestutil.SetupContainer(t)
	infra.mongoContainer = mongotestutil.SetupContainer(t)
	infra.redisContainer = redistestutil.SetupContainer(t)
	infra.rabbitmqContainer = rabbitmqtestutil.SetupContainer(t)

	// Register cleanup (reverse order of creation)
	// NOTE: Consumer connection must be closed BEFORE containers to avoid reconnection errors.
	// The consumer has an infinite retry loop with exponential backoff (designed for production resilience),
	// so some "connection reset" logs may still appear during cleanup - this is expected behavior.
	t.Cleanup(func() {
		// Close the consumer's RabbitMQ channel and connection first to signal goroutines to stop.
		// The consumer watches for channel closure via NotifyClose, then enters retry mode.
		if infra.consumerRabbitMQConn != nil {
			if infra.consumerRabbitMQConn.Channel != nil {
				_ = infra.consumerRabbitMQConn.Channel.Close()
			}
			if infra.consumerRabbitMQConn.Connection != nil {
				_ = infra.consumerRabbitMQConn.Connection.Close()
			}
			// Wait for the consumer's first retry backoff (~200-400ms) to start,
			// so container termination happens while consumer is sleeping, not connecting.
			time.Sleep(500 * time.Millisecond)
		}
		infra.rabbitmqContainer.Cleanup()
		infra.redisContainer.Cleanup()
		infra.mongoContainer.Cleanup()
		infra.pgContainer.Cleanup()
	})

	// Set RabbitMQ environment variables for async mode
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "true")
	t.Setenv("RABBITMQ_TRANSACTION_BALANCE_OPERATION_EXCHANGE", "test.transaction.exchange")
	t.Setenv("RABBITMQ_TRANSACTION_BALANCE_OPERATION_KEY", "test.transaction.key")
	t.Setenv("RABBITMQ_TRANSACTION_BALANCE_OPERATION_QUEUE", "test.transaction.queue")
	t.Setenv("RABBITMQ_BALANCE_CREATE_QUEUE", "test.balance.create.queue")

	// Build RabbitMQ health check URL (base URL, lib-commons appends the path)
	rabbitHealthCheckURL := "http://" + infra.rabbitmqContainer.Host + ":" + infra.rabbitmqContainer.MgmtPort
	t.Setenv("RABBITMQ_HEALTH_CHECK_URL", rabbitHealthCheckURL)

	// Disable other async features we don't need for this test
	t.Setenv("RABBITMQ_TRANSACTION_EVENTS_ENABLED", "false")
	t.Setenv("AUDIT_LOG_ENABLED", "false")

	// Setup RabbitMQ exchange and queue
	rabbitmqtestutil.SetupExchange(t, infra.rabbitmqContainer.Channel, "test.transaction.exchange", "direct")
	rabbitmqtestutil.SetupQueue(t, infra.rabbitmqContainer.Channel, "test.transaction.queue", "test.transaction.exchange", "test.transaction.key")

	// Create PostgreSQL connection following lib-commons pattern
	logger := libZap.InitializeLogger()
	migrationsPath := postgrestestutil.FindMigrationsPath(t, "transaction")
	connStr := postgrestestutil.BuildConnectionString(infra.pgContainer.Host, infra.pgContainer.Port, infra.pgContainer.Config)

	infra.pgConn = &libPostgres.PostgresConnection{
		ConnectionStringPrimary: connStr,
		ConnectionStringReplica: connStr,
		PrimaryDBName:           infra.pgContainer.Config.DBName,
		ReplicaDBName:           infra.pgContainer.Config.DBName,
		MigrationsPath:          migrationsPath,
		Logger:                  logger,
	}

	// Create MongoDB connection
	mongoConn := mongotestutil.CreateConnection(t, infra.mongoContainer.URI, "test_db")

	// Create Redis connection
	redisConn := redistestutil.CreateConnection(t, infra.redisContainer.Addr)

	// Create repositories
	transactionRepo := transaction.NewTransactionPostgreSQLRepository(infra.pgConn)
	operationRepo := operation.NewOperationPostgreSQLRepository(infra.pgConn)
	balanceRepo := balance.NewBalancePostgreSQLRepository(infra.pgConn)
	metadataRepo := mongodb.NewMetadataMongoDBRepository(mongoConn)
	redisRepo, err := redis.NewConsumerRedis(redisConn, false)
	require.NoError(t, err, "failed to create Redis repository")

	// Store Redis repository for test assertions
	infra.redisRepo = redisRepo

	// Create RabbitMQ producer connection
	rabbitMQConnection := &libRabbitmq.RabbitMQConnection{
		ConnectionStringSource: infra.rabbitmqContainer.URI,
		HealthCheckURL:         rabbitHealthCheckURL,
		Host:                   infra.rabbitmqContainer.Host,
		Port:                   infra.rabbitmqContainer.AMQPPort,
		User:                   rabbitmqtestutil.DefaultUser,
		Pass:                   rabbitmqtestutil.DefaultPassword,
		Logger:                 logger,
	}
	producerRepo := rabbitmq.NewProducerRabbitMQ(rabbitMQConnection)

	// Create use cases with RabbitMQ producer
	queryUC := &query.UseCase{
		TransactionRepo: transactionRepo,
		OperationRepo:   operationRepo,
		BalanceRepo:     balanceRepo,
		MetadataRepo:    metadataRepo,
		RedisRepo:       redisRepo,
	}
	infra.commandUC = &command.UseCase{
		TransactionRepo: transactionRepo,
		OperationRepo:   operationRepo,
		BalanceRepo:     balanceRepo,
		MetadataRepo:    metadataRepo,
		RedisRepo:       redisRepo,
		RabbitMQRepo:    producerRepo,
	}

	// Create handler
	infra.handler = &TransactionHandler{
		Query:   queryUC,
		Command: infra.commandUC,
	}

	// Use fake UUIDs for org and ledger (they're in the onboarding component, not transaction)
	infra.orgID = libCommons.GenerateUUIDv7()
	infra.ledgerID = libCommons.GenerateUUIDv7()

	// Setup Fiber app with routes
	infra.app = fiber.New()
	infra.setupRoutes()

	// Create RabbitMQ consumer connection (separate connection for consumer)
	// Store it in infra so we can close it during cleanup before container termination
	infra.consumerRabbitMQConn = &libRabbitmq.RabbitMQConnection{
		ConnectionStringSource: infra.rabbitmqContainer.URI,
		HealthCheckURL:         rabbitHealthCheckURL,
		Host:                   infra.rabbitmqContainer.Host,
		Port:                   infra.rabbitmqContainer.AMQPPort,
		User:                   rabbitmqtestutil.DefaultUser,
		Pass:                   rabbitmqtestutil.DefaultPassword,
		Logger:                 logger,
	}

	// Initialize telemetry for consumer
	telemetry := libOpentelemetry.InitializeTelemetry(&libOpentelemetry.TelemetryConfig{
		LibraryName:     "test",
		ServiceName:     "transaction-test",
		ServiceVersion:  "test",
		EnableTelemetry: false,
		Logger:          logger,
	})

	// Create consumer routes (used by newTestMultiQueueConsumer during test execution)
	infra.consumerRoutes = rabbitmq.NewConsumerRoutes(infra.consumerRabbitMQConn, 1, 1, logger, telemetry)

	return infra
}

// setupRoutes registers handler routes on the Fiber app for async infra.
func (infra *testAsyncInfra) setupRoutes() {
	// Middleware to inject path params as locals
	paramMiddleware := func(c *fiber.Ctx) error {
		orgIDStr := c.Params("organization_id")
		ledgerIDStr := c.Params("ledger_id")
		txIDStr := c.Params("transaction_id")

		if orgIDStr != "" {
			orgID, _ := uuid.Parse(orgIDStr)
			c.Locals("organization_id", orgID)
		}
		if ledgerIDStr != "" {
			ledgerID, _ := uuid.Parse(ledgerIDStr)
			c.Locals("ledger_id", ledgerID)
		}
		if txIDStr != "" {
			txID, _ := uuid.Parse(txIDStr)
			c.Locals("transaction_id", txID)
		}
		return c.Next()
	}

	infra.app.Post("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/json",
		paramMiddleware, http.WithBody(new(transaction.CreateTransactionInput), infra.handler.CreateTransactionJSON))
	infra.app.Get("/v1/organizations/:organization_id/ledgers/:ledger_id/transactions/:transaction_id",
		paramMiddleware, infra.handler.GetTransaction)
}

// waitForTransactionStatus polls the database until the transaction reaches the expected status or timeout.
// Returns true if the expected status was reached, false if timeout occurred.
func waitForTransactionStatus(t *testing.T, db *sql.DB, transactionID uuid.UUID, expectedStatus string, timeout time.Duration) bool {
	t.Helper()

	deadline := time.Now().Add(timeout)
	pollInterval := 100 * time.Millisecond

	for time.Now().Before(deadline) {
		var status string
		err := db.QueryRow(`SELECT status FROM transaction WHERE id = $1`, transactionID).Scan(&status)
		if err != nil {
			t.Logf("Error querying transaction status: %v", err)
			time.Sleep(pollInterval)
			continue
		}

		if status == expectedStatus {
			return true
		}

		t.Logf("Transaction %s status is %s, waiting for %s...", transactionID, status, expectedStatus)
		time.Sleep(pollInterval)
	}

	return false
}

// TestIntegration_TransactionHandler_CreateTransactionJSON_Async validates that
// CreateTransactionJSON correctly creates a non-pending transaction in async mode.
// This tests the complete async flow:
// 1. CreateTransactionJSON receives input, sets status=CREATED
// 2. createTransaction validates and prepares data
// 3. ValidateSendSourceAndDistribute calculates source/destination totals
// 4. GetBalances retrieves balances (Redis cache -> PostgreSQL fallback)
// 5. BuildOperations creates Operation objects with DEBIT/CREDIT types
// 6. TransactionExecute -> SendBTOExecuteAsync (since RABBITMQ_TRANSACTION_ASYNC=true)
// 7. Message is sent to RabbitMQ queue
// 8. Returns HTTP 201 with transaction in CREATED status (immediate response)
// 9. Consumer processes the message and updates status to APPROVED
// 10. Balances are updated in the database
func TestIntegration_TransactionHandler_CreateTransactionJSON_Async(t *testing.T) {
	// Arrange
	infra := setupAsyncTestInfra(t)

	// Create and start the consumer
	consumer := newTestMultiQueueConsumer(infra.consumerRoutes, infra.commandUC)

	// Start the consumer in a goroutine
	consumerStarted := make(chan struct{})
	go func() {
		close(consumerStarted)
		_ = consumer.run() // Run returns immediately after starting goroutines
	}()
	<-consumerStarted

	// Give the consumer a moment to start consuming
	time.Sleep(500 * time.Millisecond)

	// Use fake account IDs (account table is in onboarding component)
	sourceAccountID := libCommons.GenerateUUIDv7()
	destAccountID := libCommons.GenerateUUIDv7()

	// Create source balance (@source-async) with 1000 USD available
	sourceBalanceParams := postgrestestutil.DefaultBalanceParams()
	sourceBalanceParams.Alias = "@source-async"
	sourceBalanceParams.AssetCode = "USD"
	sourceBalanceParams.Available = decimal.NewFromInt(1000)
	sourceBalanceParams.OnHold = decimal.Zero
	sourceBalanceID := postgrestestutil.CreateTestBalance(t, infra.pgContainer.DB,
		infra.orgID, infra.ledgerID, sourceAccountID, sourceBalanceParams)

	// Create destination balance (@dest-async) with 0 USD available
	destBalanceParams := postgrestestutil.DefaultBalanceParams()
	destBalanceParams.Alias = "@dest-async"
	destBalanceParams.AssetCode = "USD"
	destBalanceParams.Available = decimal.Zero
	destBalanceParams.OnHold = decimal.Zero
	destBalanceID := postgrestestutil.CreateTestBalance(t, infra.pgContainer.DB,
		infra.orgID, infra.ledgerID, destAccountID, destBalanceParams)

	// Build JSON request body for transferring 100 USD from @source-async to @dest-async
	requestBody := `{
		"description": "Async integration test transaction - 100 USD transfer",
		"pending": false,
		"send": {
			"asset": "USD",
			"value": "100",
			"source": {
				"from": [
					{
						"accountAlias": "@source-async",
						"amount": {
							"asset": "USD",
							"value": "100"
						}
					}
				]
			},
			"distribute": {
				"to": [
					{
						"accountAlias": "@dest-async",
						"amount": {
							"asset": "USD",
							"value": "100"
						}
					}
				]
			}
		}
	}`

	// Act: Call CreateTransactionJSON endpoint
	req := httptest.NewRequest("POST",
		"/v1/organizations/"+infra.orgID.String()+"/ledgers/"+infra.ledgerID.String()+"/transactions/json",
		bytes.NewBufferString(requestBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := infra.app.Test(req, -1)

	// Assert: HTTP Response (immediate response with CREATED status)
	require.NoError(t, err, "HTTP request should not fail")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "should read response body")

	// Debug: print response if not 201
	if resp.StatusCode != 201 {
		t.Logf("Response status: %d, body: %s", resp.StatusCode, string(body))
	}

	assert.Equal(t, 201, resp.StatusCode,
		"expected HTTP 201 Created, got %d: %s", resp.StatusCode, string(body))

	// Parse response JSON
	var result map[string]any
	err = json.Unmarshal(body, &result)
	require.NoError(t, err, "response should be valid JSON")

	// Assert: Transaction ID is valid UUID
	txIDStr, ok := result["id"].(string)
	require.True(t, ok, "response should contain transaction id")
	txID, err := uuid.Parse(txIDStr)
	require.NoError(t, err, "transaction ID should be valid UUID")

	// Assert: HTTP response returns CREATED status (immediate response before async processing)
	status, ok := result["status"].(map[string]any)
	require.True(t, ok, "response should contain status object")
	assert.Equal(t, cn.CREATED, status["code"],
		"HTTP response should return CREATED status (before async processing)")

	// Poll database until transaction status is APPROVED (async processing completed)
	// Timeout: 10 seconds, Poll interval: 100ms
	statusReached := waitForTransactionStatus(t, infra.pgContainer.DB, txID, cn.APPROVED, 10*time.Second)
	require.True(t, statusReached,
		"transaction status should reach APPROVED within timeout (async processing should complete)")

	// Assert: Verify transaction exists in database with APPROVED status
	dbStatus := postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, txID)
	assert.Equal(t, cn.APPROVED, dbStatus,
		"database transaction status should be APPROVED after async processing")

	// Assert: Source balance decreased by 100 (1000 -> 900)
	sourceAvailable := postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, sourceBalanceID)
	assert.True(t, sourceAvailable.Equal(decimal.NewFromInt(900)),
		"source balance should be 900 after transaction, got %s", sourceAvailable.String())

	// Assert: Destination balance increased by 100 (0 -> 100)
	destAvailable := postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, destBalanceID)
	assert.True(t, destAvailable.Equal(decimal.NewFromInt(100)),
		"destination balance should be 100 after transaction, got %s", destAvailable.String())

	// Assert: 2 operations created (1 DEBIT, 1 CREDIT)
	opCount := postgrestestutil.CountOperationsByTransactionID(t, infra.pgContainer.DB, txID)
	assert.Equal(t, 2, opCount,
		"should have exactly 2 operations (1 DEBIT + 1 CREDIT), got %d", opCount)

	// Assert: Verify operation types in database
	var debitCount, creditCount int
	err = infra.pgContainer.DB.QueryRow(`
		SELECT
			SUM(CASE WHEN type = 'DEBIT' THEN 1 ELSE 0 END) as debit_count,
			SUM(CASE WHEN type = 'CREDIT' THEN 1 ELSE 0 END) as credit_count
		FROM operation
		WHERE transaction_id = $1
	`, txID).Scan(&debitCount, &creditCount)
	require.NoError(t, err, "should query operation types")
	assert.Equal(t, 1, debitCount, "should have exactly 1 DEBIT operation")
	assert.Equal(t, 1, creditCount, "should have exactly 1 CREDIT operation")

	// Assert: Redis balances are updated correctly
	ctx := context.Background()

	// Verify source balance in Redis
	sourceRedis := getBalanceFromRedis(t, ctx, infra.redisRepo, infra.orgID, infra.ledgerID, "@source-async", "default")
	require.NotNil(t, sourceRedis, "source balance should exist in Redis")
	assert.True(t, sourceRedis.Available.Equal(decimal.NewFromInt(900)),
		"Redis source available should be 900, got %s", sourceRedis.Available.String())
	assert.True(t, sourceRedis.OnHold.Equal(decimal.Zero),
		"Redis source onHold should be 0, got %s", sourceRedis.OnHold.String())
	assert.Greater(t, sourceRedis.Version, int64(0),
		"Redis source version should be incremented, got %d", sourceRedis.Version)

	// Verify destination balance in Redis
	destRedis := getBalanceFromRedis(t, ctx, infra.redisRepo, infra.orgID, infra.ledgerID, "@dest-async", "default")
	require.NotNil(t, destRedis, "dest balance should exist in Redis")
	assert.True(t, destRedis.Available.Equal(decimal.NewFromInt(100)),
		"Redis dest available should be 100, got %s", destRedis.Available.String())
	assert.True(t, destRedis.OnHold.Equal(decimal.Zero),
		"Redis dest onHold should be 0, got %s", destRedis.OnHold.String())
	assert.Greater(t, destRedis.Version, int64(0),
		"Redis dest version should be incremented, got %d", destRedis.Version)

	// Assert: Response contains expected fields
	assert.Equal(t, infra.orgID.String(), result["organizationId"],
		"response should contain correct organization ID")
	assert.Equal(t, infra.ledgerID.String(), result["ledgerId"],
		"response should contain correct ledger ID")
	assert.Equal(t, "USD", result["assetCode"],
		"response should contain correct asset code")

	// Assert: Amount is correct
	if amount, ok := result["amount"].(string); ok {
		assert.True(t, strings.Contains(amount, "100"),
			"response amount should be 100, got %s", amount)
	}
}

// TestIntegration_TransactionHandler_PendingTransaction_CreateAndCommit validates the full
// pending transaction lifecycle: creation with pending=true, then commit.
//
// Pending transaction flow:
// 1. Creation (pending=true):
//   - Source: Available decreases, OnHold increases (funds reserved)
//   - Destination: NO change (credit not applied yet)
//   - Transaction status: PENDING
//   - Only source operation created (DEBIT with ONHOLD)
//
// 2. Commit:
//   - Source: OnHold decreases (funds released from hold)
//   - Destination: Available increases (credit applied)
//   - Transaction status: APPROVED
//   - Destination operation created (CREDIT)
func TestIntegration_TransactionHandler_PendingTransaction_CreateAndCommit(t *testing.T) {
	// Arrange
	infra := setupTestInfra(t)

	// Ensure sync mode is enabled
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "false")

	// Use fake account IDs (account table is in onboarding component)
	sourceAccountID := libCommons.GenerateUUIDv7()
	destAccountID := libCommons.GenerateUUIDv7()

	// Create source balance (@source-pending) with 1000 USD available
	sourceBalanceParams := postgrestestutil.DefaultBalanceParams()
	sourceBalanceParams.Alias = "@source-pending"
	sourceBalanceParams.AssetCode = "USD"
	sourceBalanceParams.Available = decimal.NewFromInt(1000)
	sourceBalanceParams.OnHold = decimal.Zero
	sourceBalanceID := postgrestestutil.CreateTestBalance(t, infra.pgContainer.DB,
		infra.orgID, infra.ledgerID, sourceAccountID, sourceBalanceParams)

	// Create destination balance (@dest-pending) with 0 USD available
	destBalanceParams := postgrestestutil.DefaultBalanceParams()
	destBalanceParams.Alias = "@dest-pending"
	destBalanceParams.AssetCode = "USD"
	destBalanceParams.Available = decimal.Zero
	destBalanceParams.OnHold = decimal.Zero
	destBalanceID := postgrestestutil.CreateTestBalance(t, infra.pgContainer.DB,
		infra.orgID, infra.ledgerID, destAccountID, destBalanceParams)

	// Build JSON request body for PENDING transaction (100 USD transfer)
	requestBody := `{
		"description": "Pending transaction test - 100 USD transfer",
		"pending": true,
		"send": {
			"asset": "USD",
			"value": "100",
			"source": {
				"from": [
					{
						"accountAlias": "@source-pending",
						"amount": {
							"asset": "USD",
							"value": "100"
						}
					}
				]
			},
			"distribute": {
				"to": [
					{
						"accountAlias": "@dest-pending",
						"amount": {
							"asset": "USD",
							"value": "100"
						}
					}
				]
			}
		}
	}`

	// =========================================
	// PHASE 1: Create pending transaction
	// =========================================

	req := httptest.NewRequest("POST",
		"/v1/organizations/"+infra.orgID.String()+"/ledgers/"+infra.ledgerID.String()+"/transactions/json",
		bytes.NewBufferString(requestBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := infra.app.Test(req, -1)
	require.NoError(t, err, "HTTP request should not fail")

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err, "should read response body")

	if resp.StatusCode != 201 {
		t.Logf("Response status: %d, body: %s", resp.StatusCode, string(body))
	}
	assert.Equal(t, 201, resp.StatusCode, "expected HTTP 201 Created")

	// Parse response JSON
	var result map[string]any
	err = json.Unmarshal(body, &result)
	require.NoError(t, err, "response should be valid JSON")

	// Get transaction ID
	txIDStr, ok := result["id"].(string)
	require.True(t, ok, "response should contain transaction id")
	txID, err := uuid.Parse(txIDStr)
	require.NoError(t, err, "transaction ID should be valid UUID")

	// =========================================
	// Assert: State after PENDING creation
	// =========================================

	ctx := context.Background()

	// Assert: Transaction status is PENDING
	dbStatus := postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, txID)
	assert.Equal(t, cn.PENDING, dbStatus,
		"transaction status should be PENDING after creation")

	// Assert: Source balance in PostgreSQL - Available decreased, OnHold increased
	sourceAvailableAfterCreate := postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, sourceBalanceID)
	sourceOnHoldAfterCreate := postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, sourceBalanceID)
	assert.True(t, sourceAvailableAfterCreate.Equal(decimal.NewFromInt(900)),
		"source available should be 900 after pending creation, got %s", sourceAvailableAfterCreate.String())
	assert.True(t, sourceOnHoldAfterCreate.Equal(decimal.NewFromInt(100)),
		"source onHold should be 100 after pending creation, got %s", sourceOnHoldAfterCreate.String())

	// Assert: Destination balance in PostgreSQL - UNCHANGED (no credit yet)
	destAvailableAfterCreate := postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, destBalanceID)
	destOnHoldAfterCreate := postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, destBalanceID)
	assert.True(t, destAvailableAfterCreate.Equal(decimal.Zero),
		"dest available should be 0 after pending creation (no credit yet), got %s", destAvailableAfterCreate.String())
	assert.True(t, destOnHoldAfterCreate.Equal(decimal.Zero),
		"dest onHold should be 0 after pending creation, got %s", destOnHoldAfterCreate.String())

	// Assert: Source balance in Redis - Available=900, OnHold=100
	sourceRedisAfterCreate := getBalanceFromRedis(t, ctx, infra.redisRepo, infra.orgID, infra.ledgerID, "@source-pending", "default")
	require.NotNil(t, sourceRedisAfterCreate, "source balance should exist in Redis after pending creation")
	assert.True(t, sourceRedisAfterCreate.Available.Equal(decimal.NewFromInt(900)),
		"Redis source available should be 900, got %s", sourceRedisAfterCreate.Available.String())
	assert.True(t, sourceRedisAfterCreate.OnHold.Equal(decimal.NewFromInt(100)),
		"Redis source onHold should be 100, got %s", sourceRedisAfterCreate.OnHold.String())

	// Assert: Destination balance in Redis - should NOT exist or be unchanged
	// (destination is not processed during pending creation)
	destRedisAfterCreate := getBalanceFromRedis(t, ctx, infra.redisRepo, infra.orgID, infra.ledgerID, "@dest-pending", "default")
	// Destination may not be in Redis at all if never touched, or may have initial values
	if destRedisAfterCreate != nil {
		assert.True(t, destRedisAfterCreate.Available.Equal(decimal.Zero),
			"Redis dest available should be 0 after pending creation, got %s", destRedisAfterCreate.Available.String())
		assert.True(t, destRedisAfterCreate.OnHold.Equal(decimal.Zero),
			"Redis dest onHold should be 0 after pending creation, got %s", destRedisAfterCreate.OnHold.String())
	}

	// Assert: Only 1 operation created (source DEBIT with ONHOLD)
	opCountAfterCreate := postgrestestutil.CountOperationsByTransactionID(t, infra.pgContainer.DB, txID)
	assert.Equal(t, 1, opCountAfterCreate,
		"should have exactly 1 operation after pending creation (source only), got %d", opCountAfterCreate)

	// =========================================
	// PHASE 2: Commit the pending transaction
	// =========================================

	commitReq := httptest.NewRequest("POST",
		"/v1/organizations/"+infra.orgID.String()+"/ledgers/"+infra.ledgerID.String()+"/transactions/"+txID.String()+"/commit",
		nil)
	commitReq.Header.Set("Content-Type", "application/json")

	commitResp, err := infra.app.Test(commitReq, -1)
	require.NoError(t, err, "commit HTTP request should not fail")

	commitBody, err := io.ReadAll(commitResp.Body)
	require.NoError(t, err, "should read commit response body")

	if commitResp.StatusCode != 201 {
		t.Logf("Commit response status: %d, body: %s", commitResp.StatusCode, string(commitBody))
	}
	assert.Equal(t, 201, commitResp.StatusCode, "expected HTTP 201 Created for commit (creates new transaction)")

	// =========================================
	// Assert: State after COMMIT
	// =========================================

	// Assert: Transaction status is APPROVED
	dbStatusAfterCommit := postgrestestutil.GetTransactionStatus(t, infra.pgContainer.DB, txID)
	assert.Equal(t, cn.APPROVED, dbStatusAfterCommit,
		"transaction status should be APPROVED after commit")

	// Assert: Source balance in PostgreSQL - Available=900, OnHold=0 (released)
	sourceAvailableAfterCommit := postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, sourceBalanceID)
	sourceOnHoldAfterCommit := postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, sourceBalanceID)
	assert.True(t, sourceAvailableAfterCommit.Equal(decimal.NewFromInt(900)),
		"source available should remain 900 after commit, got %s", sourceAvailableAfterCommit.String())
	assert.True(t, sourceOnHoldAfterCommit.Equal(decimal.Zero),
		"source onHold should be 0 after commit (released), got %s", sourceOnHoldAfterCommit.String())

	// Assert: Destination balance in PostgreSQL - Available=100 (credit applied)
	destAvailableAfterCommit := postgrestestutil.GetBalanceAvailable(t, infra.pgContainer.DB, destBalanceID)
	destOnHoldAfterCommit := postgrestestutil.GetBalanceOnHold(t, infra.pgContainer.DB, destBalanceID)
	assert.True(t, destAvailableAfterCommit.Equal(decimal.NewFromInt(100)),
		"dest available should be 100 after commit (credit applied), got %s", destAvailableAfterCommit.String())
	assert.True(t, destOnHoldAfterCommit.Equal(decimal.Zero),
		"dest onHold should be 0 after commit, got %s", destOnHoldAfterCommit.String())

	// Assert: Source balance in Redis - Available=900, OnHold=0
	sourceRedisAfterCommit := getBalanceFromRedis(t, ctx, infra.redisRepo, infra.orgID, infra.ledgerID, "@source-pending", "default")
	require.NotNil(t, sourceRedisAfterCommit, "source balance should exist in Redis after commit")
	assert.True(t, sourceRedisAfterCommit.Available.Equal(decimal.NewFromInt(900)),
		"Redis source available should be 900 after commit, got %s", sourceRedisAfterCommit.Available.String())
	assert.True(t, sourceRedisAfterCommit.OnHold.Equal(decimal.Zero),
		"Redis source onHold should be 0 after commit, got %s", sourceRedisAfterCommit.OnHold.String())

	// Assert: Destination balance in Redis - Available=100
	destRedisAfterCommit := getBalanceFromRedis(t, ctx, infra.redisRepo, infra.orgID, infra.ledgerID, "@dest-pending", "default")
	require.NotNil(t, destRedisAfterCommit, "dest balance should exist in Redis after commit")
	assert.True(t, destRedisAfterCommit.Available.Equal(decimal.NewFromInt(100)),
		"Redis dest available should be 100 after commit, got %s", destRedisAfterCommit.Available.String())
	assert.True(t, destRedisAfterCommit.OnHold.Equal(decimal.Zero),
		"Redis dest onHold should be 0 after commit, got %s", destRedisAfterCommit.OnHold.String())

	// Assert: 3 operations total after commit
	// The pending transaction flow creates operations as follows:
	// - Pending creation: 1 ON_HOLD for source (reserve funds from available to onHold)
	// - Commit: 1 DEBIT for source (release from hold) + 1 CREDIT for destination (apply credit)
	opCountAfterCommit := postgrestestutil.CountOperationsByTransactionID(t, infra.pgContainer.DB, txID)
	assert.Equal(t, 3, opCountAfterCommit,
		"should have exactly 3 operations after commit (1 ON_HOLD + 1 DEBIT + 1 CREDIT), got %d", opCountAfterCommit)

	// Assert: Verify operation types (ON_HOLD, DEBIT, CREDIT)
	var onHoldCount, debitCount, creditCount int
	err = infra.pgContainer.DB.QueryRow(`
		SELECT
			SUM(CASE WHEN type = 'ON_HOLD' THEN 1 ELSE 0 END) as on_hold_count,
			SUM(CASE WHEN type = 'DEBIT' THEN 1 ELSE 0 END) as debit_count,
			SUM(CASE WHEN type = 'CREDIT' THEN 1 ELSE 0 END) as credit_count
		FROM operation
		WHERE transaction_id = $1
	`, txID).Scan(&onHoldCount, &debitCount, &creditCount)
	require.NoError(t, err, "should query operation types")
	assert.Equal(t, 1, onHoldCount, "should have exactly 1 ON_HOLD operation (pending creation)")
	assert.Equal(t, 1, debitCount, "should have exactly 1 DEBIT operation (release from hold)")
	assert.Equal(t, 1, creditCount, "should have exactly 1 CREDIT operation (destination credit)")
}
