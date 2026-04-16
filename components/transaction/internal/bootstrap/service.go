// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"io"
	"sync"

	"github.com/gofiber/fiber/v2"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libMongo "github.com/LerianStudio/lib-commons/v2/commons/mongo"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"

	httpin "github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v3/pkg/mbootstrap"
)

// Ports groups all external interface dependencies for the transaction service.
// These are the "ports" in hexagonal architecture that connect to external systems
// or are exposed to other modules (like unified ledger mode).
type Ports struct {
	// BalancePort is exposed for use by onboarding module in unified ledger mode.
	// This is the transaction UseCase which implements BalancePort directly.
	BalancePort mbootstrap.BalancePort

	// MetadataPort is the MongoDB metadata repository for direct access in unified ledger mode.
	MetadataPort mbootstrap.MetadataIndexRepository
}

// Service is the application glue where we put all top level components to be used.
type Service struct {
	*Server
	*ServerGRPC
	*MultiQueueConsumer
	*RedisQueueConsumer
	*BalanceSyncWorker
	BalanceSyncWorkerEnabled bool
	*ShardRebalanceWorker
	ShardRebalanceWorkerEnabled bool
	*ShardRoutingSubscriber
	*CircuitBreakerManager
	libLog.Logger

	// ConsumerEnabled controls whether consumer runnables are included.
	// When false, the Redpanda consumer, Redis queue consumer, and background workers
	// are excluded. Used when a dedicated consumer service handles persistence.
	ConsumerEnabled bool

	// Ports groups all external interface dependencies.
	Ports Ports

	// authorizerCloser closes the authorizer gRPC connection on shutdown.
	authorizerCloser io.Closer

	// brokerProducer closes producer client resources on shutdown.
	brokerProducer io.Closer

	telemetry          *libOpentelemetry.Telemetry
	postgresConnection *libPostgres.PostgresConnection
	mongoConnection    *libMongo.MongoConnection
	redisConnection    *libRedis.RedisConnection
	closeOnce          sync.Once
	closeErr           error

	// Route registration dependencies (for unified ledger mode)
	auth                    *middleware.AuthClient
	transactionHandler      *httpin.TransactionHandler
	operationHandler        *httpin.OperationHandler
	assetRateHandler        *httpin.AssetRateHandler
	balanceHandler          *httpin.BalanceHandler
	operationRouteHandler   *httpin.OperationRouteHandler
	transactionRouteHandler *httpin.TransactionRouteHandler
}

// Run starts the application.
// This is the only necessary code to run an app in main.go.
func (app *Service) Run() {
	// Start circuit breaker health checker if enabled
	if app.CircuitBreakerManager != nil && app.ConsumerEnabled {
		app.CircuitBreakerManager.Start() //nolint:staticcheck // QF1008: explicit field access for clarity
	}

	opts := []libCommons.LauncherOption{
		libCommons.WithLogger(app.Logger),
		libCommons.RunApp("Fiber Service", app.Server),
		libCommons.RunApp("gRPC Server", app.ServerGRPC),
	}

	if app.ConsumerEnabled {
		opts = append(opts,
			libCommons.RunApp("Broker Consumer", app.MultiQueueConsumer),
			libCommons.RunApp("Redis Queue Consumer", app.RedisQueueConsumer),
		)

		if app.BalanceSyncWorkerEnabled && app.BalanceSyncWorker != nil {
			opts = append(opts, libCommons.RunApp("Balance Sync Worker", app.BalanceSyncWorker))
		}

		if app.ShardRebalanceWorkerEnabled && app.ShardRebalanceWorker != nil {
			opts = append(opts, libCommons.RunApp("Shard Rebalance Worker", app.ShardRebalanceWorker))
		}
	}

	// Routing updates subscriber always runs when sharding is active regardless
	// of consumer mode: API-only pods still need to invalidate their local
	// route cache when another pod issues a SetRoutingOverride.
	if app.ShardRoutingSubscriber != nil {
		opts = append(opts, libCommons.RunApp("Shard Routing Subscriber", app.ShardRoutingSubscriber))
	}

	libCommons.NewLauncher(opts...).Run()

	if err := app.Close(); err != nil {
		app.Warnf("Transaction service shutdown encountered errors: %v", err)
	}
}

// closeResources releases all external resources and returns any errors encountered.
func (app *Service) closeResources() error {
	return closeSharedResources(closeResourcesParams{
		circuitBreaker:     app.CircuitBreakerManager,
		multiQueueConsumer: app.MultiQueueConsumer,
		authorizerCloser:   app.authorizerCloser,
		brokerProducer:     app.brokerProducer,
		redisConnection:    app.redisConnection,
		postgresConnection: app.postgresConnection,
		mongoConnection:    app.mongoConnection,
		telemetry:          app.telemetry,
	})
}

// Close releases external resources created during service initialization.
func (app *Service) Close() error {
	if app == nil {
		return nil
	}

	app.closeOnce.Do(func() {
		app.closeErr = app.closeResources()
	})

	return app.closeErr
}

// GetRunnables returns all runnable components for composition in unified deployment.
// Implements mbootstrap.Service interface.
// In unified mode, gRPC server is excluded since communication is done in-process.
func (app *Service) GetRunnables() []mbootstrap.RunnableConfig {
	return app.GetRunnablesWithOptions(true) // exclude gRPC by default for unified mode
}

// GetRunnablesWithOptions returns runnable components with optional gRPC exclusion.
// When excludeGRPC is true, the gRPC server is not included (used in unified ledger mode).
// When ConsumerEnabled is false, consumer and worker runnables are excluded (used when
// a dedicated consumer service handles persistence separately).
func (app *Service) GetRunnablesWithOptions(excludeGRPC bool) []mbootstrap.RunnableConfig {
	runnables := []mbootstrap.RunnableConfig{
		{Name: "Transaction Fiber Server", Runnable: app.Server},
	}

	// Consumer runnables are only included when ConsumerEnabled is true.
	// When a dedicated consumer service is deployed, the ledger sets CONSUMER_ENABLED=false
	// to avoid duplicate message processing.
	if !app.ConsumerEnabled {
		app.Info("Consumer runnables disabled (CONSUMER_ENABLED=false). Persistence handled by dedicated consumer service.")
	} else {
		runnables = app.appendConsumerRunnables(runnables)
	}

	if !excludeGRPC {
		runnables = append(runnables, mbootstrap.RunnableConfig{
			Name: "Transaction gRPC Server", Runnable: app.ServerGRPC,
		})
	}

	return runnables
}

// appendConsumerRunnables appends the consumer, worker and circuit breaker runnables.
func (app *Service) appendConsumerRunnables(runnables []mbootstrap.RunnableConfig) []mbootstrap.RunnableConfig {
	runnables = append(runnables,
		mbootstrap.RunnableConfig{Name: "Transaction Broker Consumer", Runnable: app.MultiQueueConsumer},
		mbootstrap.RunnableConfig{Name: "Transaction Redis Consumer", Runnable: app.RedisQueueConsumer},
	)

	if app.BalanceSyncWorkerEnabled && app.BalanceSyncWorker != nil {
		runnables = append(runnables, mbootstrap.RunnableConfig{
			Name: "Transaction Balance Sync Worker", Runnable: app.BalanceSyncWorker,
		})
	}

	if app.ShardRebalanceWorkerEnabled && app.ShardRebalanceWorker != nil {
		runnables = append(runnables, mbootstrap.RunnableConfig{
			Name: "Transaction Shard Rebalance Worker", Runnable: app.ShardRebalanceWorker,
		})
	}

	if app.ShardRoutingSubscriber != nil {
		runnables = append(runnables, mbootstrap.RunnableConfig{
			Name: "Transaction Shard Routing Subscriber", Runnable: app.ShardRoutingSubscriber,
		})
	}

	if app.CircuitBreakerManager != nil {
		runnables = append(runnables, mbootstrap.RunnableConfig{
			Name: "Transaction Circuit Breaker Health Checker", Runnable: NewCircuitBreakerRunnable(app.CircuitBreakerManager),
		})
	}

	return runnables
}

// GetBalancePort returns the balance port for use by onboarding in unified mode.
// This allows direct in-process calls instead of gRPC.
// The returned BalancePort is the transaction UseCase itself, which implements
// the interface directly - no intermediate adapters needed.
func (app *Service) GetBalancePort() mbootstrap.BalancePort {
	return app.Ports.BalancePort
}

// GetMetadataIndexPort returns the metadata index port for use by ledger in unified mode.
// This allows direct in-process calls for metadata index operations.
func (app *Service) GetMetadataIndexPort() mbootstrap.MetadataIndexRepository {
	return app.Ports.MetadataPort
}

// GetRouteRegistrar returns a function that registers transaction routes to an existing Fiber app.
// This is used by the unified ledger server to consolidate all routes in a single port.
func (app *Service) GetRouteRegistrar() func(*fiber.App) {
	return func(fiberApp *fiber.App) {
		httpin.RegisterRoutesToApp(
			fiberApp,
			app.auth,
			app.transactionHandler,
			app.operationHandler,
			app.assetRateHandler,
			app.balanceHandler,
			app.operationRouteHandler,
			app.transactionRouteHandler,
		)
	}
}

// Ensure Service implements mbootstrap.Service interface at compile time.
var _ mbootstrap.Service = (*Service)(nil)
