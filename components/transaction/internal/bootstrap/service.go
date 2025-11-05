package bootstrap

import (
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
)

// Service is the application glue where we put all top level components to be used.
type Service struct {
	*Server
	*ServerGRPC
	*MultiQueueConsumer
	*RedisQueueConsumer
	*BalanceSyncWorker
	libLog.Logger
}

// Run starts the application.
// This is the only necessary code to run an app in main.go
func (app *Service) Run() {
	libCommons.NewLauncher(
		libCommons.WithLogger(app.Logger),
		libCommons.RunApp("Fiber Service", app.Server),
		libCommons.RunApp("RabbitMQ Consumer", app.MultiQueueConsumer),
		libCommons.RunApp("Redis Queue Consumer", app.RedisQueueConsumer),
		libCommons.RunApp("Balance Sync Worker", app.BalanceSyncWorker),
		libCommons.RunApp("gRPC Server", app.ServerGRPC),
	).Run()
}
