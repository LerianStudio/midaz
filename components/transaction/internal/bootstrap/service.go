package bootstrap

import (
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	"github.com/LerianStudio/midaz/v3/pkg/mbootstrap"
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
		{Name: "Transaction Balance Sync Worker", Runnable: app.BalanceSyncWorker},
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

// Ensure Service implements mbootstrap.Service interface at compile time
var _ mbootstrap.Service = (*Service)(nil)
