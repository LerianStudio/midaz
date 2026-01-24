package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	poolmanager "github.com/LerianStudio/lib-commons/v2/commons/pool-manager"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
	"github.com/vmihailenco/msgpack/v5"
)

// MultiTenantRabbitMQConsumer wraps the lib-commons MultiTenantConsumer
// with transaction-specific message handlers.
type MultiTenantRabbitMQConsumer struct {
	consumer           *poolmanager.MultiTenantConsumer
	useCase            *command.UseCase
	logger             libLog.Logger
	telemetry          *libOpentelemetry.Telemetry
	balanceCreateQueue string
	btoQueue           string
	// Database pools for tenant connection injection
	postgresPool *poolmanager.Pool
	mongoPool    *poolmanager.MongoPool
}

// NewMultiTenantRabbitMQConsumer creates a new multi-tenant RabbitMQ consumer.
// Parameters:
//   - pool: RabbitMQ connection pool for tenant vhosts
//   - redisClient: Redis client for tenant cache access
//   - serviceName: Service name for Pool Manager API (e.g., "ledger")
//   - poolManagerURL: Pool Manager URL for fallback tenant discovery
//   - balanceCreateQueue: Queue name for balance create messages
//   - btoQueue: Queue name for balance transaction operation messages
//   - useCase: Transaction use case for processing messages
//   - logger: Logger for operational logging
//   - telemetry: Telemetry for tracing
//   - postgresPool: PostgreSQL connection pool for tenant database access
//   - mongoPool: MongoDB connection pool for tenant database access (optional)
func NewMultiTenantRabbitMQConsumer(
	pool *poolmanager.RabbitMQPool,
	redisClient redis.UniversalClient,
	serviceName string,
	poolManagerURL string,
	balanceCreateQueue string,
	btoQueue string,
	useCase *command.UseCase,
	logger libLog.Logger,
	telemetry *libOpentelemetry.Telemetry,
	postgresPool *poolmanager.Pool,
	mongoPool *poolmanager.MongoPool,
) *MultiTenantRabbitMQConsumer {
	config := poolmanager.DefaultMultiTenantConfig()
	config.Service = serviceName
	config.PoolManagerURL = poolManagerURL
	config.WorkersPerQueue = 5
	config.PrefetchCount = 10

	consumer := poolmanager.NewMultiTenantConsumer(pool, redisClient, config, logger)

	mtc := &MultiTenantRabbitMQConsumer{
		consumer:           consumer,
		useCase:            useCase,
		logger:             logger,
		telemetry:          telemetry,
		balanceCreateQueue: balanceCreateQueue,
		btoQueue:           btoQueue,
		postgresPool:       postgresPool,
		mongoPool:          mongoPool,
	}

	// Register queue handlers
	if balanceCreateQueue != "" {
		consumer.Register(balanceCreateQueue, mtc.handleBalanceCreateMessage)
		logger.Infof("Registered multi-tenant handler for queue: %s", balanceCreateQueue)
	}

	if btoQueue != "" {
		consumer.Register(btoQueue, mtc.handleBTOMessage)
		logger.Infof("Registered multi-tenant handler for queue: %s", btoQueue)
	}

	return mtc
}

// Run starts the multi-tenant consumer.
// Implements the Runnable interface for lib-commons Launcher.
func (c *MultiTenantRabbitMQConsumer) Run(l *libCommons.Launcher) error {
	c.logger.Info("Starting multi-tenant RabbitMQ consumer...")

	ctx := context.Background()

	if err := c.consumer.Run(ctx); err != nil {
		c.logger.Errorf("Failed to start multi-tenant consumer: %v", err)
		return err
	}

	c.logger.Info("Multi-tenant RabbitMQ consumer started successfully")

	// Block until launcher signals shutdown
	// The consumer.Run() starts background goroutines, so we need to wait
	<-ctx.Done()

	return nil
}

// injectTenantDBConnections retrieves tenant-specific database connections from pools
// and injects them into the context for use by repositories.
func (c *MultiTenantRabbitMQConsumer) injectTenantDBConnections(ctx context.Context, tenantID string, logger libLog.Logger) (context.Context, error) {
	// Inject PostgreSQL connection
	if c.postgresPool != nil {
		pgConn, err := c.postgresPool.GetConnection(ctx, tenantID)
		if err != nil {
			logger.Errorf("Failed to get PostgreSQL connection for tenant %s: %v", tenantID, err)
			return ctx, fmt.Errorf("failed to get PostgreSQL connection: %w", err)
		}

		db, err := pgConn.GetDB()
		if err != nil {
			logger.Errorf("Failed to get DB interface for tenant %s: %v", tenantID, err)
			return ctx, fmt.Errorf("failed to get DB interface: %w", err)
		}

		ctx = poolmanager.ContextWithTransactionPGConnection(ctx, db)
		logger.Infof("Injected PostgreSQL connection for tenant: %s", tenantID)
	}

	// Inject MongoDB connection (optional)
	if c.mongoPool != nil {
		mongoDB, err := c.mongoPool.GetDatabaseForTenant(ctx, tenantID)
		if err != nil {
			logger.Warnf("Failed to get MongoDB connection for tenant %s: %v (continuing without MongoDB)", tenantID, err)
			// MongoDB is optional, don't fail the entire operation
		} else {
			ctx = poolmanager.ContextWithTenantMongo(ctx, mongoDB)
			logger.Infof("Injected MongoDB connection for tenant: %s", tenantID)
		}
	}

	return ctx, nil
}

// handleBalanceCreateMessage processes balance create messages.
// The context contains the tenant ID via poolmanager.SetTenantIDInContext.
func (c *MultiTenantRabbitMQConsumer) handleBalanceCreateMessage(ctx context.Context, delivery amqp.Delivery) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "multitenant_consumer.handle_balance_create")
	defer span.End()

	tenantID := poolmanager.GetTenantIDFromContext(ctx)
	logger.Infof("Processing balance create message for tenant: %s", tenantID)

	// Inject tenant database connections into context
	var err error
	ctx, err = c.injectTenantDBConnections(ctx, tenantID, logger)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to inject tenant DB connections", err)
		return err
	}

	var message mmodel.Queue

	if err := json.Unmarshal(delivery.Body, &message); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Error unmarshalling balance message", err)
		logger.Errorf("Error unmarshalling balance message: %v", err)

		return err
	}

	logger.Infof("Balance message consumed for tenant %s, account: %s", tenantID, message.AccountID)

	if err := c.useCase.CreateBalance(ctx, message); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Error creating balance", err)
		logger.Errorf("Error creating balance: %v", err)

		return err
	}

	return nil
}

// handleBTOMessage processes balance transaction operation messages.
// The context contains the tenant ID via poolmanager.SetTenantIDInContext.
func (c *MultiTenantRabbitMQConsumer) handleBTOMessage(ctx context.Context, delivery amqp.Delivery) error {
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "multitenant_consumer.handle_bto")
	defer span.End()

	tenantID := poolmanager.GetTenantIDFromContext(ctx)
	logger.Infof("Processing BTO message for tenant: %s", tenantID)

	// Inject tenant database connections into context
	var err error
	ctx, err = c.injectTenantDBConnections(ctx, tenantID, logger)
	if err != nil {
		libOpentelemetry.HandleSpanError(&span, "Failed to inject tenant DB connections", err)
		return err
	}

	var message mmodel.Queue

	if err := msgpack.Unmarshal(delivery.Body, &message); err != nil {
		libOpentelemetry.HandleSpanError(&span, "Error unmarshalling BTO message", err)
		logger.Errorf("Error unmarshalling BTO message: %v", err)

		return err
	}

	logger.Infof("BTO message consumed for tenant %s, transaction: %s", tenantID, message.QueueData[0].ID)

	if err := c.useCase.CreateBalanceTransactionOperationsAsync(ctx, message); err != nil {
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Error creating BTO", err)
		logger.Errorf("Error creating BTO: %v", err)

		return err
	}

	return nil
}

// Close stops all tenant consumers.
func (c *MultiTenantRabbitMQConsumer) Close() error {
	c.logger.Info("Closing multi-tenant RabbitMQ consumer...")

	return c.consumer.Close()
}

// Stats returns statistics about the consumer.
func (c *MultiTenantRabbitMQConsumer) Stats() poolmanager.MultiTenantConsumerStats {
	return c.consumer.Stats()
}
