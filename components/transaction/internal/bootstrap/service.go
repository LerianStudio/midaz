// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	libLog "github.com/LerianStudio/lib-commons/v3/commons/log"
	"github.com/LerianStudio/lib-commons/v3/commons/opentelemetry/metrics"
	tmconsumer "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/consumer"
	httpin "github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/query"
	"github.com/LerianStudio/midaz/v3/pkg/mbootstrap"
	"github.com/gofiber/fiber/v2"
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
	MultiTenantConsumer *tmconsumer.MultiTenantConsumer // nil in single-tenant mode
	*RedisQueueConsumer
	*BalanceSyncWorker
	BalanceSyncWorkerEnabled bool
	*CircuitBreakerManager
	libLog.Logger

	// Ports groups all external interface dependencies.
	Ports Ports

	// commandUseCase and queryUseCase are stored for Lazy Initialization pattern.
	// SetSettingsPort updates both UseCases after initialization, resolving the
	// circular dependency between transaction and onboarding modules.
	commandUseCase *command.UseCase
	queryUseCase   *query.UseCase

	// Multi-tenant manager handles (opaque interface{} to avoid leaking lib-commons types).
	// nil in single-tenant mode. Populated from pg.pgManager / mgo.mongoManager at construction.
	pgManager               interface{}
	mongoManager            interface{}
	multiTenantConsumerPort interface{}            // RabbitMQ consumer; nil until multi-tenant consumer is wired
	metricsFactory          *metrics.MetricsFactory // nil in single-tenant mode or when telemetry disabled; for tenant consumer gauge

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
// This is the only necessary code to run an app in main.go
func (app *Service) Run() {
	// Start circuit breaker health checker if enabled
	if app.CircuitBreakerManager != nil {
		app.CircuitBreakerManager.Start() //nolint:staticcheck // QF1008: explicit field access for clarity
	}

	opts := []libCommons.LauncherOption{
		libCommons.WithLogger(app.Logger),
		libCommons.RunApp("Fiber Service", app.Server),
		libCommons.RunApp("Redis Queue Consumer", app.RedisQueueConsumer),
		libCommons.RunApp("gRPC Server", app.ServerGRPC),
	}

	// Use multi-tenant consumer if available, otherwise single-tenant
	if app.MultiTenantConsumer != nil {
		opts = append(opts, libCommons.RunApp("RabbitMQ Consumer", &multiTenantConsumerRunnable{consumer: app.MultiTenantConsumer, metricsFactory: app.metricsFactory}))
	} else if app.MultiQueueConsumer != nil {
		opts = append(opts, libCommons.RunApp("RabbitMQ Consumer", app.MultiQueueConsumer))
	}

	if app.BalanceSyncWorkerEnabled {
		opts = append(opts, libCommons.RunApp("Balance Sync Worker", app.BalanceSyncWorker))
	}

	libCommons.NewLauncher(opts...).Run()

	// Stop circuit breaker health checker on shutdown
	if app.CircuitBreakerManager != nil {
		app.CircuitBreakerManager.Stop() //nolint:staticcheck // QF1008: explicit field access for clarity
	}
}

// GetRunnables returns all runnable components for composition in unified deployment.
// Implements mbootstrap.Service interface.
// In unified mode, gRPC server is excluded since communication is done in-process.
func (app *Service) GetRunnables() []mbootstrap.RunnableConfig {
	return app.GetRunnablesWithOptions(true) // exclude gRPC by default for unified mode
}

// GetRunnablesWithOptions returns runnable components with optional gRPC exclusion.
// When excludeGRPC is true, the gRPC server is not included (used in unified ledger mode).
func (app *Service) GetRunnablesWithOptions(excludeGRPC bool) []mbootstrap.RunnableConfig {
	runnables := []mbootstrap.RunnableConfig{
		{Name: "Transaction Fiber Server", Runnable: app.Server},
		{Name: "Transaction Redis Consumer", Runnable: app.RedisQueueConsumer},
	}

	// Use multi-tenant consumer if available, otherwise single-tenant
	if app.MultiTenantConsumer != nil {
		runnables = append(runnables, mbootstrap.RunnableConfig{
			Name: "Transaction RabbitMQ Consumer", Runnable: &multiTenantConsumerRunnable{consumer: app.MultiTenantConsumer, metricsFactory: app.metricsFactory},
		})
	} else if app.MultiQueueConsumer != nil {
		runnables = append(runnables, mbootstrap.RunnableConfig{
			Name: "Transaction RabbitMQ Consumer", Runnable: app.MultiQueueConsumer,
		})
	}

	if app.BalanceSyncWorkerEnabled {
		runnables = append(runnables, mbootstrap.RunnableConfig{
			Name: "Transaction Balance Sync Worker", Runnable: app.BalanceSyncWorker,
		})
	}

	if !excludeGRPC {
		runnables = append(runnables, mbootstrap.RunnableConfig{
			Name: "Transaction gRPC Server", Runnable: app.ServerGRPC,
		})
	}

	// Add circuit breaker health checker as a runnable for unified ledger mode
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

// SetSettingsPort sets the settings port for querying ledger settings.
// This is called after initialization in unified ledger mode to wire the onboarding
// SettingsPort to transaction, resolving the circular dependency between components.
// Uses the Lazy Initialization pattern: both UseCases are created without SettingsPort,
// then this method is called to inject it after both modules exist.
//
// IMPORTANT: This method MUST be called before Run() to ensure thread-safety.
// The SettingsPort fields are not protected by synchronization primitives,
// so setting them after request processing begins could cause data races.
func (app *Service) SetSettingsPort(port mbootstrap.SettingsPort) {
	if app.commandUseCase != nil {
		app.commandUseCase.SettingsPort = port
	}

	if app.queryUseCase != nil {
		app.queryUseCase.SettingsPort = port
	}
}

// GetPGManager returns the multi-tenant PostgreSQL manager as an opaque handle.
// Returns nil in single-tenant mode.
func (app *Service) GetPGManager() interface{} {
	return app.pgManager
}

// GetMongoManager returns the multi-tenant MongoDB manager as an opaque handle.
// Returns nil in single-tenant mode.
func (app *Service) GetMongoManager() interface{} {
	return app.mongoManager
}

// GetMultiTenantConsumer returns the multi-tenant RabbitMQ consumer as an opaque handle.
// Returns nil until multi-tenant consumer is wired.
func (app *Service) GetMultiTenantConsumer() interface{} {
	return app.multiTenantConsumerPort
}

// Ensure Service implements mbootstrap.Service interface at compile time
var _ mbootstrap.Service = (*Service)(nil)
