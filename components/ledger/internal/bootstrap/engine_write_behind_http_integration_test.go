//go:build integration

// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	authMiddleware "github.com/LerianStudio/lib-auth/v5/auth/middleware"
	libCommons "github.com/LerianStudio/lib-commons/v7/commons"
	libRabbitmq "github.com/LerianStudio/lib-commons/v7/commons/rabbitmq"
	tmcore "github.com/LerianStudio/lib-commons/v7/commons/tenant-manager/core"
	libLog "github.com/LerianStudio/lib-observability/v4/log"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v4/tracing"
	libZap "github.com/LerianStudio/lib-observability/v4/zap"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	httpin "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in"
	ledgerMiddleware "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in/middleware"
	transactionMongo "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/mongodb/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/organization"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/portfolio"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/segment"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/postgres/transactionroute"
	ledgerRabbitMQ "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/rabbitmq"
	transactionRedis "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/redis/transaction"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/domain/accounting"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/v4/components/ledger/internal/services/query"
	"github.com/LerianStudio/midaz/v4/pkg/constant"
	"github.com/LerianStudio/midaz/v4/pkg/utils"
	mongotestutil "github.com/LerianStudio/midaz/v4/tests/utils/mongodb"
	postgrestestutil "github.com/LerianStudio/midaz/v4/tests/utils/postgres"
	rabbitmqtestutil "github.com/LerianStudio/midaz/v4/tests/utils/rabbitmq"
	redistestutil "github.com/LerianStudio/midaz/v4/tests/utils/redis"
)

type engineWriteBehindHTTPIntegration struct {
	db                  *sql.DB
	mongo               *mongo.Database
	redisRepo           *transactionRedis.RedisConsumerRepository
	command             *command.UseCase
	query               *query.UseCase
	handler             *httpin.TransactionHandler
	organization        uuid.UUID
	ledger              uuid.UUID
	rabbit              *rabbitmqtestutil.ContainerResult
	rabbitConn          *libRabbitmq.RabbitMQConnection
	redis               *redis.Client
	engine              command.Engine
	engineHook          *engineWriteBehindBenchmarkHook
	benchmarkSTProducer *ledgerRabbitMQ.EngineWriteBehindProducer
	benchmarkMTProducer *ledgerRabbitMQ.EngineWriteBehindProducer
	exchange            string
	routingKey          string
	queue               string
}

type engineWriteBehindHTTPResponse struct {
	status   int
	header   http.Header
	body     []byte
	decoded  map[string]any
	replayed string
}

type integrationTenantChannelProvider struct {
	connection *amqp.Connection
}

type integrationEngineClientProvider struct {
	client redis.UniversalClient
}

type integrationCountingEngine struct {
	delegate command.Engine
	calls    atomic.Int32
}

func (engine *integrationCountingEngine) Execute(ctx context.Context, input command.EngineExecution) (*accounting.ExecutionResult, error) {
	engine.calls.Add(1)

	return engine.delegate.Execute(ctx, input)
}

type integrationCountingCompleter struct {
	individual      command.AppliedTransactionCompleter
	bulk            command.AppliedTransactionBulkCompleter
	completed       chan struct{}
	individualCalls atomic.Int32
	bulkCalls       atomic.Int32
}

func (completer *integrationCountingCompleter) Complete(ctx context.Context, record *command.TransactionCompletionRecord) (command.TransactionCompletionResult, error) {
	completer.individualCalls.Add(1)
	result, err := completer.individual.Complete(ctx, record)
	if err == nil {
		completer.completed <- struct{}{}
	}

	return result, err
}

func (completer *integrationCountingCompleter) CompleteBulk(ctx context.Context, records []*command.TransactionCompletionRecord) ([]command.TransactionCompletionResult, error) {
	completer.bulkCalls.Add(1)
	result, err := completer.bulk.CompleteBulk(ctx, records)
	if err == nil {
		completer.completed <- struct{}{}
	}

	return result, err
}

func (provider integrationEngineClientProvider) GetClient(context.Context) (redis.UniversalClient, error) {
	return provider.client, nil
}

func (provider integrationTenantChannelProvider) GetChannel(context.Context, string) (ledgerRabbitMQ.PublishableChannel, error) {
	return provider.connection.Channel()
}

func (integrationTenantChannelProvider) Close(context.Context) error { return nil }

func TestIntegrationEngineWriteBehindHTTPReturnsBeforeProjectionAndConverges(t *testing.T) {
	if testing.Short() {
		t.Skip("requires PostgreSQL, MongoDB, Valkey, and RabbitMQ")
	}

	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("AUDIT_LOG_ENABLED", "false")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "true")

	infra := setupEngineWriteBehindHTTPIntegration(t)

	t.Run("single tenant v1 and v2", func(t *testing.T) {
		producer, err := ledgerRabbitMQ.NewSingleTenantEngineWriteBehindProducer(
			infra.rabbitConn, infra.exchange, infra.routingKey, time.Second,
		)
		require.NoError(t, err)
		infra.command.TransactionWriteBehindDispatcher = engineWriteBehindDispatcher{publisher: producer}
		app := infra.newHTTPApp("")

		infra.runConfirmedAsyncCreate(t, app, "v1", "", false)
		infra.runConfirmedAsyncCreate(t, app, "v2", "", true)
	})

	t.Run("multi tenant v1 and v2", func(t *testing.T) {
		const tenantID = "tenant-a"
		producer, err := ledgerRabbitMQ.NewMultiTenantEngineWriteBehindProducer(
			integrationTenantChannelProvider{connection: infra.rabbit.Conn}, infra.exchange, infra.routingKey, time.Second,
		)
		require.NoError(t, err)
		infra.command.TransactionWriteBehindDispatcher = engineWriteBehindDispatcher{publisher: producer}
		app := infra.newHTTPApp(tenantID)

		infra.runConfirmedAsyncCreate(t, app, "v1", tenantID, false)
		infra.runConfirmedAsyncCreate(t, app, "v2", tenantID, true)
	})

	t.Run("real unroutable publication completes through fallback", func(t *testing.T) {
		producer, err := ledgerRabbitMQ.NewSingleTenantEngineWriteBehindProducer(
			infra.rabbitConn, infra.exchange, "missing.route", time.Second,
		)
		require.NoError(t, err)
		infra.command.TransactionWriteBehindDispatcher = engineWriteBehindDispatcher{publisher: producer}
		app := infra.newHTTPApp("")
		aliases := infra.seedTransfer(t, "fallback")
		response := infra.postCreate(t, app, "v2", aliases, "fallback-"+uuid.NewString())
		require.Equalf(t, http.StatusCreated, response.status, "fallback must preserve accounting success: %s", response.body)
		transactionID := response.transactionID(t)

		infra.requireProjection(t, context.Background(), transactionID, 1, 2, 1)
		_, queued, err := infra.rabbit.Channel.Get(infra.queue, true)
		require.NoError(t, err)
		assert.False(t, queued, "an unroutable publish must not be reported as queued")
	})
}

func TestIntegrationEngineWriteBehindHTTPGetResolvesMissingAndOperationlessTransactions(t *testing.T) {
	if testing.Short() {
		t.Skip("requires PostgreSQL, MongoDB, Valkey, and RabbitMQ")
	}

	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("AUDIT_LOG_ENABLED", "false")

	infra := setupEngineWriteBehindHTTPIntegration(t)
	app := infra.newHTTPApp("")

	missing := map[string]uuid.UUID{"random missing ID": uuid.New(), "nil ID": uuid.Nil}
	for name, transactionID := range missing {
		t.Run(name+" is not found on v1 legacy envelope", func(t *testing.T) {
			get := infra.getTransaction(t, app, "v1", transactionID)
			require.Equalf(t, http.StatusNotFound, get.status, "GET must report a missing transaction: %s", get.body)
			assert.Equal(t, fiber.MIMEApplicationJSON, get.header.Get(fiber.HeaderContentType))
			assert.Equal(t, constant.ErrEntityNotFound.Error(), get.decoded["code"])
			assert.Contains(t, get.decoded, "message")
		})

		t.Run(name+" is not found on v2 problem document", func(t *testing.T) {
			get := infra.getTransaction(t, app, "v2", transactionID)
			require.Equalf(t, http.StatusNotFound, get.status, "GET must report a missing transaction: %s", get.body)
			assert.Equal(t, "application/problem+json", get.header.Get(fiber.HeaderContentType))
			assert.Equal(t, constant.ErrEntityNotFound.Error(), get.decoded["code"])
			assert.Contains(t, get.decoded, "detail")
		})
	}

	persisted := infra.createTransactionWithoutOperations(t)
	for _, version := range []string{"v1", "v2"} {
		t.Run("row without persisted operations is returned with an empty list on "+version, func(t *testing.T) {
			get := infra.getTransaction(t, app, version, persisted)
			require.Equalf(t, http.StatusOK, get.status, "GET must return the persisted row: %s", get.body)
			assert.Equal(t, persisted.String(), get.decoded["id"])
			status, ok := get.decoded["status"].(map[string]any)
			require.Truef(t, ok, "response must carry a status object: %s", get.body)
			assert.Equal(t, constant.NOTED, status["code"])
			assert.Equal(t, []any{}, get.decoded["operations"])
			assert.Equal(t, "false", get.header.Get("X-Cache-Hit"))
		})
	}
}

func TestIntegrationEngineWriteBehindHTTPCommitsAndCancelsBeforeProjection(t *testing.T) {
	if testing.Short() {
		t.Skip("requires PostgreSQL, MongoDB, Valkey, and RabbitMQ")
	}

	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("AUDIT_LOG_ENABLED", "false")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "true")

	infra := setupEngineWriteBehindHTTPIntegration(t)
	producer, err := ledgerRabbitMQ.NewSingleTenantEngineWriteBehindProducer(
		infra.rabbitConn, infra.exchange, infra.routingKey, time.Second,
	)
	require.NoError(t, err)
	infra.command.TransactionWriteBehindDispatcher = engineWriteBehindDispatcher{publisher: producer}
	app := infra.newHTTPApp("")
	dispatcher := requireRabbitTransactionDispatcher(t, infra.command, rabbitEngineSingleTenant, false)

	for _, testCase := range []struct {
		version string
		action  string
		status  string
	}{
		{version: "v1", action: "commit", status: constant.APPROVED},
		{version: "v1", action: "cancel", status: constant.CANCELED},
		{version: "v2", action: "commit", status: constant.APPROVED},
		{version: "v2", action: "cancel", status: constant.CANCELED},
	} {
		t.Run(testCase.version+" "+testCase.action, func(t *testing.T) {
			ctx := context.Background()
			aliases := infra.seedTransfer(t, "lifecycle-"+testCase.version+"-"+testCase.action+"-"+strings.ReplaceAll(uuid.NewString(), "-", "")[:8])
			created := infra.postPendingCreate(t, app, testCase.version, aliases)
			require.Equalf(t, http.StatusCreated, created.status, "async pending create must return 201: %s", created.body)
			transactionID := created.transactionID(t)
			infra.requireProjection(t, ctx, transactionID, 0, 0, 0)
			infra.requireBalance(t, ctx, aliases.source, 900, 100)

			transition := infra.postTransition(t, app, testCase.version, transactionID, testCase.action)
			require.Equalf(t, http.StatusCreated, transition.status, "%s before projection must succeed: %s", testCase.action, transition.body)
			require.Equal(t, transactionID, transition.transactionID(t))
			status, ok := transition.decoded["status"].(map[string]any)
			require.Truef(t, ok, "response must carry a status object: %s", transition.body)
			assert.Equal(t, testCase.status, status["code"])
			infra.requireProjection(t, ctx, transactionID, 0, 0, 0)

			if testCase.status == constant.APPROVED {
				infra.requireBalance(t, ctx, aliases.source, 900, 0)
				infra.requireBalance(t, ctx, aliases.destination, 100, 0)
			} else {
				infra.requireBalance(t, ctx, aliases.source, 1000, 0)
			}

			repeated := infra.postTransition(t, app, testCase.version, transactionID, testCase.action)
			require.NotEqualf(t, http.StatusCreated, repeated.status, "a second %s must be refused: %s", testCase.action, repeated.body)
			// The pending lock still held by the first transition or the terminal
			// status refuses the repeat; either way no second movement happens.
			assert.Contains(t, []any{constant.ErrPendingTransactionLocked.Error(), constant.ErrCommitTransactionNotPending.Error()}, repeated.decoded["code"])

			for range 2 {
				delivery, queued, err := infra.rabbit.Channel.Get(infra.queue, false)
				require.NoError(t, err)
				require.True(t, queued, "the create and the transition must each publish their evidence")
				require.NoError(t, dispatcher.handle(ctx, delivery.Body))
				require.NoError(t, delivery.Ack(false))
			}

			_, queued, err := infra.rabbit.Channel.Get(infra.queue, true)
			require.NoError(t, err)
			assert.False(t, queued, "the refused repeat must not publish evidence")

			var projected string
			require.NoError(t, infra.db.QueryRow(`SELECT status FROM transaction WHERE id = $1`, transactionID).Scan(&projected))
			assert.Equal(t, testCase.status, projected)
			if testCase.status == constant.APPROVED {
				infra.requireBalance(t, ctx, aliases.source, 900, 0)
				infra.requireBalance(t, ctx, aliases.destination, 100, 0)
			} else {
				infra.requireBalance(t, ctx, aliases.source, 1000, 0)
			}
		})
	}
}

func TestIntegrationEngineWriteBehindConsumerWiring(t *testing.T) {
	if testing.Short() {
		t.Skip("requires PostgreSQL, MongoDB, Valkey, and RabbitMQ")
	}

	t.Setenv("ALLOW_INSECURE_TLS", "true")
	t.Setenv("AUDIT_LOG_ENABLED", "false")
	t.Setenv("RABBITMQ_TRANSACTION_ASYNC", "true")

	infra := setupEngineWriteBehindHTTPIntegration(t)
	baseCompleter := infra.command.AppliedTransactionCompleter
	bulkCompleter, ok := baseCompleter.(command.AppliedTransactionBulkCompleter)
	require.True(t, ok, "application completion composition must expose bulk capability")
	t.Setenv("RABBITMQ_TRANSACTION_BALANCE_OPERATION_QUEUE", infra.queue)

	for _, testCase := range []struct {
		name    string
		version string
		bulk    bool
	}{
		{name: "individual", version: "v1"},
		{name: "bulk", version: "v2", bulk: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			producer, err := ledgerRabbitMQ.NewSingleTenantEngineWriteBehindProducer(
				infra.rabbitConn, infra.exchange, infra.routingKey, time.Second,
			)
			require.NoError(t, err)
			infra.command.TransactionWriteBehindDispatcher = engineWriteBehindDispatcher{publisher: producer}

			countingEngine := &integrationCountingEngine{delegate: infra.engine}
			infra.command.Engine = countingEngine
			countingCompleter := &integrationCountingCompleter{
				individual: baseCompleter,
				bulk:       bulkCompleter,
				completed:  make(chan struct{}, 1),
			}
			infra.command.AppliedTransactionCompleter = countingCompleter

			app := infra.newHTTPApp("")
			aliases := infra.seedTransfer(t, "consumer-wiring-"+testCase.name)
			idempotencyKey := "consumer-wiring-" + uuid.NewString()
			response := infra.postCreate(t, app, testCase.version, aliases, idempotencyKey)
			require.Equalf(t, http.StatusCreated, response.status, "async create must return before projection: %s", response.body)
			transactionID := response.transactionID(t)
			infra.requireProjection(t, context.Background(), transactionID, 0, 0, 0)
			assert.Equal(t, int32(1), countingEngine.calls.Load(), "HTTP admission must apply accounting exactly once")

			queue, err := infra.rabbit.Channel.QueueInspect(infra.queue)
			require.NoError(t, err)
			require.Equal(t, 1, queue.Messages, "confirmed evidence must be waiting before consumer startup")

			routes, err := ledgerRabbitMQ.NewConsumerRoutes(
				infra.rabbitConn, 1, 1, &libLog.GoLogger{}, &libOpentelemetry.Telemetry{},
			)
			require.NoError(t, err)
			if testCase.bulk {
				routes.ConfigureBulk(&ledgerRabbitMQ.BulkConfig{Enabled: true, Size: 1, FlushTimeout: time.Second})
			}
			consumer, err := NewMultiQueueConsumer(routes, infra.command, testCase.bulk, nil)
			require.NoError(t, err)
			require.NoError(t, consumer.Run(nil))

			select {
			case <-countingCompleter.completed:
			case <-time.After(10 * time.Second):
				routes.StopConsumers()
				t.Fatal("timed out waiting for the first RabbitMQ delivery to complete")
			}
			routes.StopConsumers()

			infra.requireProjection(t, context.Background(), transactionID, 1, 2, 1)
			assert.Equal(t, int32(1), countingEngine.calls.Load(), "consumer completion must not reapply accounting")
			if testCase.bulk {
				assert.Zero(t, countingCompleter.individualCalls.Load())
				assert.Equal(t, int32(1), countingCompleter.bulkCalls.Load())
			} else {
				assert.Equal(t, int32(1), countingCompleter.individualCalls.Load())
				assert.Zero(t, countingCompleter.bulkCalls.Load())
			}

			queue, err = infra.rabbit.Channel.QueueInspect(infra.queue)
			require.NoError(t, err)
			assert.Zero(t, queue.Messages, "successful first delivery must be ACKed without retry or DLQ routing")

			get := infra.getTransaction(t, app, testCase.version, transactionID)
			require.Equalf(t, http.StatusOK, get.status, "GET after recovery ACK must remain readable: %s", get.body)
			require.Equal(t, transactionID.String(), get.decoded["id"])
			operations, ok := get.decoded["operations"].([]any)
			require.Truef(t, ok, "GET after recovery ACK must include operations: %s", get.body)
			require.Len(t, operations, 2)
			metadata, ok := get.decoded["metadata"].(map[string]any)
			require.Truef(t, ok, "GET after recovery ACK must include metadata: %s", get.body)
			require.Equal(t, "write-behind", metadata["proof"])
			assert.Equal(t, int32(1), countingEngine.calls.Load(), "GET after recovery ACK must not reapply accounting")

			replay := infra.postCreate(t, app, testCase.version, aliases, idempotencyKey)
			require.Equal(t, http.StatusCreated, replay.status)
			require.Equal(t, "true", replay.replayed)
			assert.Equal(t, int32(1), countingEngine.calls.Load(), "idempotency replay must not reapply accounting")
		})
	}
}

func setupEngineWriteBehindHTTPIntegration(tb testing.TB) *engineWriteBehindHTTPIntegration {
	tb.Helper()
	t := tb

	pgContainer := postgrestestutil.SetupContainer(t)
	mongoContainer := mongotestutil.SetupContainer(t)
	redisContainer := redistestutil.SetupContainerWithConfig(t, redistestutil.FinancialContainerConfig())
	rabbitContainer := rabbitmqtestutil.SetupReusableContainer(t)

	migrationsPath := postgrestestutil.FindMigrationsPath(t, "transaction")
	connectionString := postgrestestutil.BuildConnectionString(pgContainer.Host, pgContainer.Port, pgContainer.Config)
	pgClient := postgrestestutil.CreatePostgresClient(t, connectionString, connectionString, pgContainer.Config.DBName, migrationsPath)
	postgrestestutil.ApplyOnboardingSchema(t, pgContainer.DB)

	mongoConnection := mongotestutil.CreateConnection(t, mongoContainer.URI, mongoContainer.DBName)
	redisConnection := redistestutil.CreateConnection(t, redisContainer.Addr)
	redisRepo, err := transactionRedis.NewConsumerRedis(redisConnection)
	require.NoError(t, err)
	engineHook := &engineWriteBehindBenchmarkHook{}
	redisContainer.Client.AddHook(engineHook)

	transactionRepo := transaction.NewTransactionPostgreSQLRepository(pgClient, false)
	operationRepo := operation.NewOperationPostgreSQLRepository(pgClient)
	balanceRepo := balance.NewBalancePostgreSQLRepository(pgClient, false)
	ledgerRepo := ledger.NewLedgerPostgreSQLRepository(pgClient)
	accountRepo := account.NewAccountPostgreSQLRepository(pgClient)
	assetRepo := asset.NewAssetPostgreSQLRepository(pgClient)
	organizationRepo := organization.NewOrganizationPostgreSQLRepository(pgClient)
	portfolioRepo := portfolio.NewPortfolioPostgreSQLRepository(pgClient)
	segmentRepo := segment.NewSegmentPostgreSQLRepository(pgClient)
	operationRouteRepo := operationroute.NewOperationRoutePostgreSQLRepository(pgClient)
	transactionRouteRepo := transactionroute.NewTransactionRoutePostgreSQLRepository(pgClient, false)
	metadataRepo := transactionMongo.NewMetadataMongoDBRepository(mongoConnection)

	queryUseCase := &query.UseCase{
		OrganizationRepo: organizationRepo, LedgerRepo: ledgerRepo, AssetRepo: assetRepo,
		AccountRepo: accountRepo, PortfolioRepo: portfolioRepo, SegmentRepo: segmentRepo,
		TransactionRepo: transactionRepo, OperationRepo: operationRepo, BalanceRepo: balanceRepo,
		OperationRouteRepo: operationRouteRepo, TransactionRouteRepo: transactionRouteRepo,
		TransactionMetadataRepo: metadataRepo, TransactionRedisRepo: redisRepo,
		EngineWriteBehindRepo: redisRepo, EngineWriteBehindCodec: command.EngineWriteBehindEvidenceCodec{},
	}
	commandUseCase := &command.UseCase{
		OrganizationRepo: organizationRepo, LedgerRepo: ledgerRepo, AssetRepo: assetRepo,
		AccountRepo: accountRepo, PortfolioRepo: portfolioRepo, SegmentRepo: segmentRepo,
		TransactionRepo: transactionRepo, OperationRepo: operationRepo, BalanceRepo: balanceRepo,
		OperationRouteRepo: operationRouteRepo, TransactionRouteRepo: transactionRouteRepo,
		TransactionMetadataRepo: metadataRepo, TransactionRedisRepo: redisRepo,
		AtomicTransactionBatchIdempotencyRepo:  redisRepo,
		AtomicTransactionBatchProjectionReader: queryUseCase,
		TransactionReader:                      queryUseCase, TransactionWriteBehindAsync: true,
		TransactionEvidenceResolver: rabbitEngineEvidenceResolver{repository: redisRepo},
		UUIDv7Generator:             libCommons.GenerateUUIDv7,
		Clock:                       time.Now,
	}

	logger := &libLog.GoLogger{}
	recovery := NewRedisQueueConsumer(logger, commandUseCase, queryUseCase)
	require.NoError(t, configureAppliedTransactionCompletion(recovery, commandUseCase, false, nil))
	require.NoError(t, configureEngine(commandUseCase, integrationEngineClientProvider{client: redisContainer.Client}))
	configuredEngine := commandUseCase.Engine

	organizationID := postgrestestutil.CreateTestOrganization(t, pgContainer.DB)
	ledgerID := postgrestestutil.CreateTestLedger(t, pgContainer.DB, organizationID)

	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	exchange := "engine-write-behind-" + suffix
	routingKey := "engine.write-behind." + suffix
	queue := "engine-write-behind-" + suffix
	rabbitmqtestutil.SetupExchange(t, rabbitContainer.Channel, exchange, "topic")
	rabbitmqtestutil.SetupQueue(t, rabbitContainer.Channel, queue, exchange, routingKey)

	rabbitLogger, err := libZap.New(libZap.Config{Environment: libZap.EnvironmentDevelopment, OTelLibraryName: "midaz-tests"})
	require.NoError(t, err)
	rabbitConnection := &libRabbitmq.RabbitMQConnection{
		ConnectionStringSource:   rabbitContainer.URI,
		HealthCheckURL:           "http://" + rabbitContainer.Host + ":" + rabbitContainer.MgmtPort,
		AllowInsecureHealthCheck: true,
		Host:                     rabbitContainer.Host, Port: rabbitContainer.AMQPPort,
		User: rabbitmqtestutil.DefaultUser, Pass: rabbitmqtestutil.DefaultPassword,
		Logger: rabbitLogger,
	}

	return &engineWriteBehindHTTPIntegration{
		db: pgContainer.DB, mongo: mongoContainer.Database, redisRepo: redisRepo,
		command: commandUseCase, query: queryUseCase,
		handler:      &httpin.TransactionHandler{Command: commandUseCase, Query: queryUseCase, TransactionBatchMaxSize: 50},
		organization: organizationID, ledger: ledgerID,
		rabbit: rabbitContainer, rabbitConn: rabbitConnection,
		redis: redisContainer.Client, engine: configuredEngine, engineHook: engineHook,
		exchange: exchange, routingKey: routingKey, queue: queue,
	}
}

func (infra *engineWriteBehindHTTPIntegration) newHTTPApp(tenantID string) *fiber.App {
	app := fiber.New()
	app.Use(ledgerMiddleware.ErrorEnvelope())

	if tenantID != "" {
		app.Use(func(c fiber.Ctx) error {
			c.SetContext(tmcore.ContextWithTenantID(c.Context(), tenantID))

			return c.Next()
		})
	}

	auth := &authMiddleware.AuthClient{Enabled: false}
	mountHumaContracts(
		app, &libLog.GoLogger{},
		humaContract{prefix: "/v1", mount: func(group fiber.Router, api huma.API) {
			httpin.RegisterTransactionHumaRoutesToApp(group, api, auth, infra.handler, nil)
		}},
		humaContract{prefix: "/v2", mount: func(group fiber.Router, api huma.API) {
			httpin.RegisterTransactionV2RoutesToApp(group, api, auth, infra.handler, nil)
			httpin.RegisterTransactionMirrorV2RoutesToApp(group, api, auth, infra.handler, nil)
		}},
	)

	return app
}

func (infra *engineWriteBehindHTTPIntegration) runConfirmedAsyncCreate(t *testing.T, app *fiber.App, version, tenantID string, bulk bool) {
	t.Helper()
	aliases := infra.seedTransfer(t, version+"-"+strings.ReplaceAll(uuid.NewString(), "-", "")[:8])
	idempotencyKey := "engine-write-behind-" + uuid.NewString()
	response := infra.postCreate(t, app, version, aliases, idempotencyKey)
	require.Equalf(t, http.StatusCreated, response.status, "async create must return 201: %s", response.body)
	transactionID := response.transactionID(t)

	delivery, queued, err := infra.rabbit.Channel.Get(infra.queue, false)
	require.NoError(t, err)
	require.True(t, queued, "publisher confirm must make the evidence observable before HTTP assertions")
	envelope, err := command.DecodeTransactionWriteBehindEnvelope(delivery.Body)
	require.NoError(t, err)
	require.Equal(t, tenantID, envelope.Record.TenantID)

	ctx := context.Background()
	if tenantID != "" {
		ctx = tmcore.ContextWithTenantID(ctx, tenantID)
	}

	infra.requireProjection(t, ctx, transactionID, 0, 0, 0)
	get := infra.getTransaction(t, app, version, transactionID)
	require.Equalf(t, http.StatusOK, get.status, "GET must reconstruct pending evidence: %s", get.body)
	require.Equal(t, transactionID.String(), get.decoded["id"])

	infra.waitForReplay(t, ctx, idempotencyKey)
	replay := infra.postCreate(t, app, version, aliases, idempotencyKey)
	require.Equalf(t, http.StatusCreated, replay.status, "replay must not depend on SQL/Mongo projection: %s", replay.body)
	require.Equal(t, "true", replay.replayed)
	require.Equal(t, transactionID, replay.transactionID(t))
	infra.requireProjection(t, ctx, transactionID, 0, 0, 0)

	policy := rabbitEngineSingleTenant
	if tenantID != "" {
		policy = rabbitEngineMultiTenant
	}
	dispatcher := requireRabbitTransactionDispatcher(t, infra.command, policy, bulk)
	if bulk {
		results, dispatchErr := dispatcher.handleBulk(ctx, []amqp.Delivery{delivery})
		require.NoError(t, dispatchErr)
		require.Len(t, results, 1)
		require.True(t, results[0].Success, results[0].Error)
	} else {
		require.NoError(t, dispatcher.handle(ctx, delivery.Body))
	}
	require.NoError(t, delivery.Ack(false))

	infra.requireProjection(t, ctx, transactionID, 1, 2, 1)
	sourceBefore, err := infra.redisRepo.ListBalanceByKey(ctx, infra.organization, infra.ledger, aliases.source+"#default")
	require.NoError(t, err)
	require.NotNil(t, sourceBefore)

	require.NoError(t, dispatcher.handle(ctx, delivery.Body), "redelivery must converge idempotently without accounting")
	infra.requireProjection(t, ctx, transactionID, 1, 2, 1)
	sourceAfter, err := infra.redisRepo.ListBalanceByKey(ctx, infra.organization, infra.ledger, aliases.source+"#default")
	require.NoError(t, err)
	require.NotNil(t, sourceAfter)
	assert.True(t, sourceBefore.Available.Equal(sourceAfter.Available))
	assert.Equal(t, sourceBefore.Version, sourceAfter.Version)

	_, queued, err = infra.rabbit.Channel.Get(infra.queue, true)
	require.NoError(t, err)
	assert.False(t, queued, "replay and completed dispatch must not publish or leave another message")
}

type engineWriteBehindAliases struct {
	source      string
	destination string
}

func (infra *engineWriteBehindHTTPIntegration) seedTransfer(tb testing.TB, suffix string) engineWriteBehindAliases {
	tb.Helper()
	t := tb
	aliases := engineWriteBehindAliases{source: "@source-" + suffix, destination: "@destination-" + suffix}

	for alias, available := range map[string]decimal.Decimal{
		aliases.source: decimal.NewFromInt(1000), aliases.destination: decimal.Zero,
	} {
		accountID := postgrestestutil.CreateTestAccount(t, infra.db, infra.organization, infra.ledger, nil, alias, alias, "USD", nil)
		params := postgrestestutil.DefaultBalanceParams()
		params.Alias, params.AssetCode, params.Available = alias, "USD", available
		postgrestestutil.CreateTestBalance(t, infra.db, infra.organization, infra.ledger, accountID, params)
	}

	return aliases
}

func (infra *engineWriteBehindHTTPIntegration) postCreate(tb testing.TB, app *fiber.App, version string, aliases engineWriteBehindAliases, idempotencyKey string) engineWriteBehindHTTPResponse {
	tb.Helper()
	t := tb
	path := "/v1/organizations/" + infra.organization.String() + "/ledgers/" + infra.ledger.String() + "/transactions/json"
	body := fmt.Sprintf(`{"description":"async projection","metadata":{"proof":"write-behind"},"send":{"asset":"USD","value":"100","source":{"from":[{"accountAlias":%q,"amount":{"asset":"USD","value":"100"}}]},"distribute":{"to":[{"accountAlias":%q,"amount":{"asset":"USD","value":"100"}}]}}}`, aliases.source, aliases.destination)
	if version == "v2" {
		path = "/v2/transactions/direct"
		body = fmt.Sprintf(`{"description":"async projection","asset":"USD","amount":"100","metadata":{"proof":"write-behind"},"debits":[{"alias":%q,"organizationId":%q,"ledgerId":%q,"amount":"100"}],"credits":[{"alias":%q,"organizationId":%q,"ledgerId":%q,"amount":"100"}]}`,
			aliases.source, infra.organization.String(), infra.ledger.String(), aliases.destination, infra.organization.String(), infra.ledger.String())
	}

	request := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Idempotency", idempotencyKey)
	request.Header.Set("X-TTL", "60")

	return performEngineWriteBehindHTTPRequest(t, app, request)
}

func (infra *engineWriteBehindHTTPIntegration) postPendingCreate(tb testing.TB, app *fiber.App, version string, aliases engineWriteBehindAliases) engineWriteBehindHTTPResponse {
	tb.Helper()
	t := tb
	path := "/v1/organizations/" + infra.organization.String() + "/ledgers/" + infra.ledger.String() + "/transactions/json"
	body := fmt.Sprintf(`{"description":"async pending","pending":true,"send":{"asset":"USD","value":"100","source":{"from":[{"accountAlias":%q,"amount":{"asset":"USD","value":"100"}}]},"distribute":{"to":[{"accountAlias":%q,"amount":{"asset":"USD","value":"100"}}]}}}`, aliases.source, aliases.destination)
	if version == "v2" {
		path = "/v2/transactions/hold"
		body = fmt.Sprintf(`{"description":"async pending","asset":"USD","amount":"100","debits":[{"alias":%q,"organizationId":%q,"ledgerId":%q,"amount":"100"}],"credits":[{"alias":%q,"organizationId":%q,"ledgerId":%q,"amount":"100"}]}`,
			aliases.source, infra.organization.String(), infra.ledger.String(), aliases.destination, infra.organization.String(), infra.ledger.String())
	}

	request := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Idempotency", "pending-lifecycle-"+uuid.NewString())

	return performEngineWriteBehindHTTPRequest(t, app, request)
}

func (infra *engineWriteBehindHTTPIntegration) postTransition(tb testing.TB, app *fiber.App, version string, transactionID uuid.UUID, action string) engineWriteBehindHTTPResponse {
	tb.Helper()
	t := tb
	path := "/" + version + "/organizations/" + infra.organization.String() + "/ledgers/" + infra.ledger.String() + "/transactions/" + transactionID.String() + "/" + action

	return performEngineWriteBehindHTTPRequest(t, app, httptest.NewRequest(http.MethodPost, path, nil))
}

func (infra *engineWriteBehindHTTPIntegration) requireBalance(tb testing.TB, ctx context.Context, alias string, available, onHold int64) {
	tb.Helper()
	t := tb
	current, err := infra.redisRepo.ListBalanceByKey(ctx, infra.organization, infra.ledger, alias+"#default")
	require.NoError(t, err)
	require.NotNil(t, current)
	assert.Truef(t, current.Available.Equal(decimal.NewFromInt(available)), "%s available: got %s, want %d", alias, current.Available, available)
	assert.Truef(t, current.OnHold.Equal(decimal.NewFromInt(onHold)), "%s on hold: got %s, want %d", alias, current.OnHold, onHold)
}

// createTransactionWithoutOperations persists only the transaction row, the
// shape a reader observes while the legacy persistence path has written the row
// but not yet its operations.
func (infra *engineWriteBehindHTTPIntegration) createTransactionWithoutOperations(tb testing.TB) uuid.UUID {
	tb.Helper()
	t := tb
	transactionID := uuid.New()
	amount := decimal.NewFromInt(100)
	recordedAt := time.Date(2026, time.September, 24, 12, 0, 0, 0, time.UTC)

	_, err := infra.query.TransactionRepo.Create(context.Background(), &transaction.Transaction{
		ID: transactionID.String(), Description: "annotation", Status: transaction.Status{Code: constant.NOTED},
		Amount: &amount, AssetCode: "USD", ChartOfAccountsGroupName: "annotation",
		OrganizationID: infra.organization.String(), LedgerID: infra.ledger.String(),
		CreatedAt: recordedAt, UpdatedAt: recordedAt,
	})
	require.NoError(t, err)
	infra.requireProjection(t, context.Background(), transactionID, 1, 0, 0)

	return transactionID
}

func (infra *engineWriteBehindHTTPIntegration) getTransaction(tb testing.TB, app *fiber.App, version string, transactionID uuid.UUID) engineWriteBehindHTTPResponse {
	tb.Helper()
	t := tb
	path := "/" + version + "/organizations/" + infra.organization.String() + "/ledgers/" + infra.ledger.String() + "/transactions/" + transactionID.String()

	return performEngineWriteBehindHTTPRequest(t, app, httptest.NewRequest(http.MethodGet, path, nil))
}

func performEngineWriteBehindHTTPRequest(tb testing.TB, app *fiber.App, request *http.Request) engineWriteBehindHTTPResponse {
	tb.Helper()
	t := tb
	response, err := app.Test(request, fiber.TestConfig{Timeout: 0})
	require.NoError(t, err)
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())

	result := engineWriteBehindHTTPResponse{status: response.StatusCode, header: response.Header, body: body, replayed: response.Header.Get("X-Idempotency-Replayed")}
	_ = json.Unmarshal(body, &result.decoded)

	return result
}

func (response engineWriteBehindHTTPResponse) transactionID(tb testing.TB) uuid.UUID {
	tb.Helper()
	t := tb
	value, ok := response.decoded["id"].(string)
	require.Truef(t, ok, "response must contain a transaction id: %s", response.body)
	id, err := uuid.Parse(value)
	require.NoError(t, err)

	return id
}

func (infra *engineWriteBehindHTTPIntegration) waitForReplay(tb testing.TB, ctx context.Context, idempotencyKey string) {
	tb.Helper()
	t := tb
	key := utils.IdempotencyInternalKey(infra.organization, infra.ledger, idempotencyKey)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		value, err := infra.redisRepo.Get(ctx, key)
		if err == nil && strings.HasPrefix(strings.TrimSpace(value), "{") {
			return
		}

		runtime.Gosched()
	}

	t.Fatalf("idempotency replay was not materialized for %q", idempotencyKey)
}

func (infra *engineWriteBehindHTTPIntegration) requireProjection(tb testing.TB, ctx context.Context, transactionID uuid.UUID, transactions, operations, metadata int64) {
	tb.Helper()
	t := tb
	var transactionCount, operationCount int64
	require.NoError(t, infra.db.QueryRow(`SELECT COUNT(*) FROM transaction WHERE id = $1`, transactionID).Scan(&transactionCount))
	require.NoError(t, infra.db.QueryRow(`SELECT COUNT(*) FROM operation WHERE transaction_id = $1`, transactionID).Scan(&operationCount))
	metadataCount, err := infra.mongo.Collection("transaction").CountDocuments(ctx, bson.M{"entity_id": transactionID.String()})
	require.NoError(t, err)
	assert.Equal(t, transactions, transactionCount)
	assert.Equal(t, operations, operationCount)
	assert.Equal(t, metadata, metadataCount)
}

var _ ledgerRabbitMQ.ChannelProvider = integrationTenantChannelProvider{}
