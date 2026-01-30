package bootstrap

import (
	"os"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	poolmanager "github.com/LerianStudio/lib-commons/v2/commons/pool-manager"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/redis/go-redis/v9"
)

// MultiTenantPools contains all connection pools used in multi-tenant mode.
type MultiTenantPools struct {
	RabbitMQPool *poolmanager.RabbitMQPool
	PostgresPool *poolmanager.Pool
	MongoPool    *poolmanager.MongoPool
}

// MultiTenantProducerResult contains the result of multi-tenant producer initialization.
type MultiTenantProducerResult struct {
	Producer rabbitmq.ProducerRepository
	Pools    *MultiTenantPools
}

// initMultiTenantProducer initializes the RabbitMQ producer for multi-tenant mode.
// It creates connection pools for RabbitMQ, PostgreSQL, and MongoDB using Pool Manager.
// The serviceName parameter determines the service name used for Pool Manager API registration.
func initMultiTenantProducer(cfg *Config, opts *Options, logger libLog.Logger) MultiTenantProducerResult {
	// Determine service name for RabbitMQ pool registration
	// When running as part of unified ledger, use the caller's service name
	serviceName := ApplicationName
	if opts != nil && opts.ServiceName != "" {
		serviceName = opts.ServiceName
	}

	logger.Info("Multi-tenant mode enabled - initializing multi-tenant RabbitMQ producer")

	poolManagerClient := poolmanager.NewClient(cfg.PoolManagerURL, logger)

	// Create RabbitMQ pool for tenant-specific connections
	rabbitMQPool := poolmanager.NewRabbitMQPool(poolManagerClient, serviceName,
		poolmanager.WithRabbitMQModule("transaction"),
		poolmanager.WithRabbitMQLogger(logger),
	)

	// Create PostgreSQL pool for multi-tenant mode
	postgresPool := poolmanager.NewPool(poolManagerClient, serviceName,
		poolmanager.WithModule("transaction"),
		poolmanager.WithPoolLogger(logger),
	)
	logger.Info("Created PostgreSQL connection pool for multi-tenant mode")

	// Create MongoDB pool for multi-tenant mode
	mongoPool := poolmanager.NewMongoPool(poolManagerClient, serviceName,
		poolmanager.WithMongoModule("transaction"),
		poolmanager.WithMongoLogger(logger),
	)
	logger.Info("Created MongoDB connection pool for multi-tenant mode")

	producer := rabbitmq.NewProducerRabbitMQMultiTenant(rabbitMQPool)
	logger.Infof("Multi-tenant RabbitMQ producer initialized for service: %s", serviceName)

	return MultiTenantProducerResult{
		Producer: producer,
		Pools: &MultiTenantPools{
			RabbitMQPool: rabbitMQPool,
			PostgresPool: postgresPool,
			MongoPool:    mongoPool,
		},
	}
}

// initMultiTenantConsumer initializes the RabbitMQ consumer for multi-tenant mode.
// It creates a MultiTenantRabbitMQConsumer that uses Pool Manager for tenant-specific connections.
func initMultiTenantConsumer(
	cfg *Config,
	opts *Options,
	pools *MultiTenantPools,
	redisClient redis.UniversalClient,
	useCase *command.UseCase,
	logger libLog.Logger,
	telemetry *libOpentelemetry.Telemetry,
) *MultiTenantRabbitMQConsumer {
	// Determine service name for consumer registration
	serviceName := ApplicationName
	if opts != nil && opts.ServiceName != "" {
		serviceName = opts.ServiceName
	}

	logger.Info("Multi-tenant mode enabled - initializing multi-tenant RabbitMQ consumer")

	// Get BTO queue name from environment
	btoQueue := os.Getenv("RABBITMQ_TRANSACTION_BALANCE_OPERATION_QUEUE")

	// Create multi-tenant consumer using the pools from producer initialization
	consumer := NewMultiTenantRabbitMQConsumer(
		pools.RabbitMQPool,
		redisClient,
		serviceName,
		cfg.PoolManagerURL,
		cfg.RabbitMQBalanceCreateQueue,
		btoQueue,
		useCase,
		logger,
		telemetry,
		pools.PostgresPool,
		pools.MongoPool,
	)

	logger.Infof("Multi-tenant RabbitMQ consumer initialized for service: %s", serviceName)

	return consumer
}
