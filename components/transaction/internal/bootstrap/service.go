// Package bootstrap provides application initialization and dependency injection for the transaction service.
// This file defines the Service struct and application startup logic.
package bootstrap

import (
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
)

// Service is the top-level application container that holds all components.
//
// This struct aggregates all server components for the transaction service:
//   - HTTP Server: Handles REST API requests
//   - RabbitMQ Consumer: Processes async transaction queue messages
//   - Redis Queue Consumer: Processes Redis queue messages
//   - Logger: Application-wide logging
//
// The service coordinates multiple concurrent servers using lib-commons Launcher.
type Service struct {
	*Server
	*MultiQueueConsumer
	*RedisQueueConsumer
	libLog.Logger
}

// Run starts all application components concurrently with graceful shutdown.
//
// This method is the main entry point called from main.go. It:
// 1. Starts HTTP server (REST API)
// 2. Starts RabbitMQ consumer (async transaction processing)
// 3. Starts Redis queue consumer (queue-based processing)
// 4. Handles graceful shutdown on SIGTERM/SIGINT
// 5. Waits for all components to shut down cleanly
//
// All components run concurrently and are managed by lib-commons Launcher,
// which handles signal handling, graceful shutdown, and error propagation.
func (app *Service) Run() {
	libCommons.NewLauncher(
		libCommons.WithLogger(app.Logger),
		libCommons.RunApp("Fiber Service", app.Server),
		libCommons.RunApp("RabbitMQ Consumer", app.MultiQueueConsumer),
		libCommons.RunApp("Redis Queue Consumer", app.RedisQueueConsumer),
	).Run()
}
