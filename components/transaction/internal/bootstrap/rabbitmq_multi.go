package bootstrap

import (
	"os"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	tenantmanager "github.com/LerianStudio/lib-commons/v2/commons/tenant-manager"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/redis/go-redis/v9"
)

// MultiTenantPools contains all connection pools used in multi-tenant mode.
type MultiTenantPools struct {
	RabbitMQPool *tenantmanager.RabbitMQManager
	PostgresPool *tenantmanager.PostgresManager
	MongoPool    *tenantmanager.MongoManager
}

// MultiTenantProducerResult contains the result of multi-tenant producer initialization.
type MultiTenantProducerResult struct {
	Producer rabbitmq.ProducerRepository
	Pools    *MultiTenantPools
}

// initMultiTenantProducer initializes the RabbitMQ producer for multi-tenant mode.
// It creates connection pools for RabbitMQ, PostgreSQL, and MongoDB using Tenant Manager.
// The serviceName parameter determines the service name used for Tenant Manager API registration.
func initMultiTenantProducer(cfg *Config, opts *Options, logger libLog.Logger) MultiTenantProducerResult {
	// Determine service name for RabbitMQ pool registration
	// When running as part of unified ledger, use the caller's service name
	serviceName := ApplicationName
	if opts != nil && opts.ServiceName != "" {
		serviceName = opts.ServiceName
	}

	logger.Info("Multi-tenant mode enabled - initializing multi-tenant RabbitMQ producer")

	tenantManagerClient := tenantmanager.NewClient(cfg.MultiTenantURL, logger)

	// Create RabbitMQ pool for tenant-specific connections
	rabbitMQPool := tenantmanager.NewRabbitMQManager(tenantManagerClient, serviceName,
		tenantmanager.WithRabbitMQModule("transaction"),
		tenantmanager.WithRabbitMQLogger(logger),
	)

	// Create PostgreSQL pool for multi-tenant mode
	postgresPool := tenantmanager.NewPostgresManager(tenantManagerClient, serviceName,
		tenantmanager.WithModule("transaction"),
		tenantmanager.WithPostgresLogger(logger),
	)
	logger.Info("Created PostgreSQL connection manager for multi-tenant mode")

	// Create MongoDB pool for multi-tenant mode
	mongoPool := tenantmanager.NewMongoManager(tenantManagerClient, serviceName,
		tenantmanager.WithMongoModule("transaction"),
		tenantmanager.WithMongoLogger(logger),
	)
	logger.Info("Created MongoDB connection manager for multi-tenant mode")

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
// It creates a MultiTenantRabbitMQConsumer that uses Tenant Manager for tenant-specific connections.
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
		cfg.MultiTenantURL,
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
