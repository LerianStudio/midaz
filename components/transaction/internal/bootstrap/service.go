package bootstrap

import (
	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	httpin "github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v3/pkg/mbootstrap"
	"github.com/gofiber/fiber/v2"
)

// Service is the application glue where we put all top level components to be used.
type Service struct {
	*Server
	*ServerGRPC
	*MultiQueueConsumer
	*RedisQueueConsumer
	*BalanceSyncWorker
	BalanceSyncWorkerEnabled bool
	libLog.Logger

	// balancePort holds the reference for use in unified ledger mode.
	// This is the transaction UseCase which implements BalancePort directly.
	balancePort mbootstrap.BalancePort

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
	opts := []libCommons.LauncherOption{
		libCommons.WithLogger(app.Logger),
		libCommons.RunApp("Fiber Service", app.Server),
		libCommons.RunApp("RabbitMQ Consumer", app.MultiQueueConsumer),
		libCommons.RunApp("Redis Queue Consumer", app.RedisQueueConsumer),
		libCommons.RunApp("gRPC Server", app.ServerGRPC),
	}

	if app.BalanceSyncWorkerEnabled {
		opts = append(opts, libCommons.RunApp("Balance Sync Worker", app.BalanceSyncWorker))
	}

	libCommons.NewLauncher(opts...).Run()
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
		{Name: "Transaction RabbitMQ Consumer", Runnable: app.MultiQueueConsumer},
		{Name: "Transaction Redis Consumer", Runnable: app.RedisQueueConsumer},
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

	return runnables
}

// GetBalancePort returns the balance port for use by onboarding in unified mode.
// This allows direct in-process calls instead of gRPC.
// The returned BalancePort is the transaction UseCase itself, which implements
// the interface directly - no intermediate adapters needed.
func (app *Service) GetBalancePort() mbootstrap.BalancePort {
	return app.balancePort
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

// Ensure Service implements mbootstrap.Service interface at compile time
var _ mbootstrap.Service = (*Service)(nil)
