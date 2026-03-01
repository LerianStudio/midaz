// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libMongo "github.com/LerianStudio/lib-commons/v2/commons/mongo"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"

	grpcOut "github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/grpc/out"
	httpin "github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transactionroute"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redpanda"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
)

// ConsumerService is a standalone service that processes messages from Redpanda
// and persists them to PostgreSQL and MongoDB. It does not include HTTP or gRPC servers.
// This is the dedicated consumer extracted from the ledger binary to achieve clean
// separation between the API path (ledger) and the persistence path (consumer).
type ConsumerService struct {
	*MultiQueueConsumer
	*RedisQueueConsumer
	*BalanceSyncWorker
	BalanceSyncWorkerEnabled bool
	*ShardRebalanceWorker
	ShardRebalanceWorkerEnabled bool
	*CircuitBreakerManager
	libLog.Logger

	authorizerCloser   io.Closer
	brokerProducer     io.Closer
	telemetry          *libOpentelemetry.Telemetry
	postgresConnection *libPostgres.PostgresConnection
	mongoConnection    *libMongo.MongoConnection
	redisConnection    *libRedis.RedisConnection
	closeOnce          sync.Once
	closeErr           error
}

// Run starts the consumer service with all workers.
func (cs *ConsumerService) Run() {
	cs.Info("Running consumer service (Redpanda consumer + workers)")

	if cs.CircuitBreakerManager != nil {
		cs.CircuitBreakerManager.Start() //nolint:staticcheck // QF1008: explicit field access for clarity
	}

	opts := []libCommons.LauncherOption{
		libCommons.WithLogger(cs.Logger),
		libCommons.RunApp("Broker Consumer", cs.MultiQueueConsumer),
		libCommons.RunApp("Redis Queue Consumer", cs.RedisQueueConsumer),
	}

	if cs.BalanceSyncWorkerEnabled && cs.BalanceSyncWorker != nil {
		opts = append(opts, libCommons.RunApp("Balance Sync Worker", cs.BalanceSyncWorker))
	}

	if cs.ShardRebalanceWorkerEnabled && cs.ShardRebalanceWorker != nil {
		opts = append(opts, libCommons.RunApp("Shard Rebalance Worker", cs.ShardRebalanceWorker))
	}

	if cs.CircuitBreakerManager != nil {
		opts = append(opts, libCommons.RunApp("Circuit Breaker Health Checker", NewCircuitBreakerRunnable(cs.CircuitBreakerManager)))
	}

	libCommons.NewLauncher(opts...).Run()

	if err := cs.Close(); err != nil {
		cs.Warnf("Consumer service shutdown encountered errors: %v", err)
	}
}

// Close releases all external resources.
func (cs *ConsumerService) Close() error {
	if cs == nil {
		return nil
	}

	cs.closeOnce.Do(func() {
		cs.closeErr = cs.closeResources()
	})

	return cs.closeErr
}

func (cs *ConsumerService) closeResources() error {
	return closeSharedResources(closeResourcesParams{
		circuitBreaker:     cs.CircuitBreakerManager,
		multiQueueConsumer: cs.MultiQueueConsumer,
		authorizerCloser:   cs.authorizerCloser,
		brokerProducer:     cs.brokerProducer,
		redisConnection:    cs.redisConnection,
		postgresConnection: cs.postgresConnection,
		mongoConnection:    cs.mongoConnection,
		telemetry:          cs.telemetry,
	})
}

// InitConsumerWithOptions initializes the consumer service with optional dependency injection.
// This creates all infrastructure needed for the Redpanda consumer (PG, Mongo, Redis,
// Redpanda, Authorizer) without HTTP or gRPC servers.
//
//nolint:gocyclo,cyclop // initialization function; complexity is inherent from wiring many dependencies
func InitConsumerWithOptions(opts *Options) (*ConsumerService, error) {
	cfg := &Config{}

	if err := libCommons.SetConfigFromEnvVars(cfg); err != nil {
		return nil, fmt.Errorf("failed to load config from environment variables: %w", err)
	}

	if err := validateConsumerModeConfig(cfg); err != nil {
		return nil, err
	}

	const maxCleanupFuncs = 8

	cleanupFuncs := make([]func(), 0, maxCleanupFuncs)

	success := false

	defer func() {
		if success {
			return
		}

		for i := len(cleanupFuncs) - 1; i >= 0; i-- {
			cleanupFuncs[i]()
		}
	}()

	logger, err := initLogger(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize logger: %w", err)
	}

	if err := validateBrokerSecurity(cfg, logger); err != nil {
		return nil, err
	}

	balanceSyncWorkerEnabled := cfg.BalanceSyncWorkerEnabled
	logger.Infof("BalanceSyncWorker: BALANCE_SYNC_WORKER_ENABLED=%v", balanceSyncWorkerEnabled)

	shardRebalanceWorkerEnabled := cfg.ShardRebalanceWorkerEnabled
	logger.Infof("ShardRebalanceWorker: SHARD_REBALANCE_WORKER_ENABLED=%v", shardRebalanceWorkerEnabled)

	telemetry, err := libOpentelemetry.InitializeTelemetryWithError(&libOpentelemetry.TelemetryConfig{
		LibraryName:               cfg.OtelLibraryName,
		ServiceName:               cfg.OtelServiceName,
		ServiceVersion:            cfg.OtelServiceVersion,
		DeploymentEnv:             cfg.OtelDeploymentEnv,
		CollectorExporterEndpoint: cfg.OtelColExporterEndpoint,
		EnableTelemetry:           cfg.EnableTelemetry,
		Logger:                    logger,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize telemetry: %w", err)
	}

	cleanupFuncs = append(cleanupFuncs, func() {
		telemetry.ShutdownTelemetry()
	})

	postgresConnection, err := initPostgresConnection(cfg, logger)
	if err != nil {
		return nil, err
	}

	if err := ensurePostgresConnectionReady(cfg, postgresConnection, logger); err != nil {
		return nil, err
	}

	cleanupFuncs = append(cleanupFuncs, func() {
		if postgresConnection.ConnectionDB != nil {
			if closeErr := (*postgresConnection.ConnectionDB).Close(); closeErr != nil {
				logger.Warnf("Failed to close consumer PostgreSQL connection during cleanup: %v", closeErr)
			}
		}
	})

	mongoConnection := initMongoConnection(cfg, logger)

	cleanupFuncs = append(cleanupFuncs, func() {
		if mongoConnection.DB != nil {
			if closeErr := mongoConnection.DB.Disconnect(context.Background()); closeErr != nil {
				logger.Warnf("Failed to disconnect consumer MongoDB client during cleanup: %v", closeErr)
			}
		}
	})

	redisConnection := initRedisConnection(cfg, logger)

	cleanupFuncs = append(cleanupFuncs, func() {
		if closeErr := redisConnection.Close(); closeErr != nil {
			logger.Warnf("Failed to close consumer Redis connection during cleanup: %v", closeErr)
		}
	})

	shardRouter, shardManager := initShardRouting(cfg, logger, redisConnection)

	redisConsumerRepository, err := redis.NewConsumerRedis(redisConnection, balanceSyncWorkerEnabled, shardRouter)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize redis: %w", err)
	}

	transactionPostgreSQLRepository := transaction.NewTransactionPostgreSQLRepository(postgresConnection)
	operationPostgreSQLRepository := operation.NewOperationPostgreSQLRepository(postgresConnection)
	assetRatePostgreSQLRepository := assetrate.NewAssetRatePostgreSQLRepository(postgresConnection)
	balancePostgreSQLRepository := balance.NewBalancePostgreSQLRepository(postgresConnection)
	operationRoutePostgreSQLRepository := operationroute.NewOperationRoutePostgreSQLRepository(postgresConnection)
	transactionRoutePostgreSQLRepository := transactionroute.NewTransactionRoutePostgreSQLRepository(postgresConnection)

	metadataMongoDBRepository, err := mongodb.NewMetadataMongoDBRepository(mongoConnection)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize metadata MongoDB repository: %w", err)
	}

	ensureMongoIndexes(mongoConnection, logger)

	seedBrokers := redpanda.ParseSeedBrokers(cfg.RedpandaBrokers)

	brokerInfra, err := initBrokerInfrastructure(cfg, opts, logger, seedBrokers, telemetry)
	if err != nil {
		return nil, err
	}

	cleanupFuncs = append(cleanupFuncs, brokerInfra.cleanupFuncs...)

	useCase := &command.UseCase{
		TransactionRepo:          transactionPostgreSQLRepository,
		OperationRepo:            operationPostgreSQLRepository,
		AssetRateRepo:            assetRatePostgreSQLRepository,
		BalanceRepo:              balancePostgreSQLRepository,
		OperationRouteRepo:       operationRoutePostgreSQLRepository,
		TransactionRouteRepo:     transactionRoutePostgreSQLRepository,
		MetadataRepo:             metadataMongoDBRepository,
		BrokerRepo:               brokerInfra.producer,
		RedisRepo:                redisConsumerRepository,
		ShardRouter:              shardRouter,
		ShardManager:             shardManager,
		BalanceOperationsTopic:   cfg.RedpandaBalanceOperationsTopic,
		BalanceCreateTopic:       cfg.RedpandaBalanceCreateTopic,
		EventsTopic:              cfg.RedpandaEventsTopic,
		DecisionEventsTopic:      cfg.RedpandaDecisionEventsTopic,
		EventsEnabled:            cfg.TransactionEventsEnabled,
		AuditTopic:               cfg.RedpandaAuditTopic,
		AuditLogEnabled:          cfg.AuditLogEnabled,
		TransactionAsync:         cfg.TransactionAsync,
		Version:                  cfg.Version,
		BatchSideEffectsTimeout:  time.Duration(cfg.BatchSideEffectsTimeoutMS) * time.Millisecond,
		IdempotencyReplayTimeout: time.Duration(cfg.IdempotencyReplayTimeoutMS) * time.Millisecond,
	}

	// The consumer needs the authorizer for the PublishBalanceOperations path
	// used by the RedisQueueConsumer (which may be migrated here in a future phase).
	authorizerClient, err := grpcOut.NewAuthorizerClient(
		grpcOut.AuthorizerConfig{
			Enabled:     cfg.AuthorizerEnabled,
			Host:        cfg.AuthorizerHost,
			Port:        cfg.AuthorizerPort,
			Timeout:     time.Duration(cfg.AuthorizerTimeoutMS) * time.Millisecond,
			Streaming:   cfg.AuthorizerUseStreaming,
			TLSEnabled:  cfg.AuthorizerGRPCTLSEnabled,
			Environment: cfg.EnvName,
			RoutingMode: cfg.AuthorizerRoutingMode,
			Instances:   cfg.AuthorizerInstances,
			ShardRanges: cfg.AuthorizerShardRanges,
			ShardCount:  cfg.RedisShardCount,
			PoolSize:    cfg.AuthorizerPoolSize,
		},
		logger,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize authorizer client: %w", err)
	}

	cleanupFuncs = append(cleanupFuncs, func() {
		if closeErr := authorizerClient.Close(); closeErr != nil {
			logger.Warnf("Failed to close consumer authorizer connection during cleanup: %v", closeErr)
		}
	})

	useCase.Authorizer = authorizerClient
	transactionHandler := httpin.TransactionHandler{Command: useCase}

	routes := configureConsumerRoutes(cfg, logger, telemetry, seedBrokers)
	multiQueueConsumer := NewMultiQueueConsumer(routes, useCase)
	redisQueueConsumer := NewRedisQueueConsumer(logger, transactionHandler)

	if err := preWarmExternalPreSplitBalances(cfg, logger, balancePostgreSQLRepository, redisConsumerRepository); err != nil {
		logger.Warnf("External pre-split Redis pre-warm failed: %v", err)
	}

	balanceSyncWorker := newBalanceSyncWorker(cfg, logger, redisConnection, useCase, balanceSyncWorkerEnabled)
	shardRebalanceWorker := newShardRebalanceWorker(cfg, logger, shardManager, shardRouter, shardRebalanceWorkerEnabled)
	resolvedShardRebalanceWorkerEnabled := shardRebalanceWorker != nil

	service := &ConsumerService{
		MultiQueueConsumer:          multiQueueConsumer,
		RedisQueueConsumer:          redisQueueConsumer,
		BalanceSyncWorker:           balanceSyncWorker,
		BalanceSyncWorkerEnabled:    balanceSyncWorkerEnabled,
		ShardRebalanceWorker:        shardRebalanceWorker,
		ShardRebalanceWorkerEnabled: resolvedShardRebalanceWorkerEnabled,
		CircuitBreakerManager:       brokerInfra.circuitBreakerManager,
		Logger:                      logger,
		authorizerCloser:            authorizerClient,
		brokerProducer:              brokerInfra.producer,
		telemetry:                   telemetry,
		postgresConnection:          postgresConnection,
		mongoConnection:             mongoConnection,
		redisConnection:             redisConnection,
	}

	success = true

	return service, nil
}
