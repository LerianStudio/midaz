// Package bootstrap provides application initialization and lifecycle management for the transaction component.
//
// This package implements the service composition root, wiring together all dependencies
// and providing the main entry point for the transaction microservice.
//
// Transaction Component Purpose:
//
// The transaction component handles financial transaction processing:
//   - Transaction creation and validation
//   - Balance management (available, on-hold)
//   - Operation tracking (individual entries within transactions)
//   - Asynchronous balance synchronization
//
// Bootstrap Architecture:
//
//	┌─────────────────────────────────────────────────────────────────────────┐
//	│                              Service                                     │
//	│  ┌───────────────────────┐  ┌───────────────────────┐                   │
//	│  │   Server (Fiber)      │  │   ServerGRPC          │                   │
//	│  │   - REST API          │  │   - gRPC endpoints    │                   │
//	│  │   - Health checks     │  │   - Balance service   │                   │
//	│  └───────────────────────┘  └───────────────────────┘                   │
//	│                                                                          │
//	│  ┌───────────────────────┐  ┌───────────────────────┐                   │
//	│  │ MultiQueueConsumer    │  │ RedisQueueConsumer    │                   │
//	│  │ - RabbitMQ listener   │  │ - Redis queue         │                   │
//	│  │ - Transaction events  │  │ - Balance updates     │                   │
//	│  └───────────────────────┘  └───────────────────────┘                   │
//	│                                                                          │
//	│  ┌───────────────────────┐  ┌───────────────────────┐                   │
//	│  │ BalanceSyncWorker     │  │      Logger           │                   │
//	│  │ - Background sync     │  │   - Structured logs   │                   │
//	│  │ - Redis → PostgreSQL  │  │                       │                   │
//	│  └───────────────────────┘  └───────────────────────┘                   │
//	└─────────────────────────────────────────────────────────────────────────┘
//
// Component Responsibilities:
//
//   - Server: REST API for synchronous transaction operations
//   - ServerGRPC: gRPC endpoints for internal service communication (balance queries)
//   - MultiQueueConsumer: RabbitMQ consumer for async transaction processing
//   - RedisQueueConsumer: Redis-based queue for high-throughput balance updates
//   - BalanceSyncWorker: Background worker syncing Redis balances to PostgreSQL
//
// Related Packages:
//   - apps/midaz/components/transaction/internal/adapters: Repository implementations
//   - apps/midaz/components/transaction/internal/services: Use case implementations
package bootstrap

import (
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
)

// Service is the application composition root aggregating all transaction component parts.
//
// Service wires together multiple concurrent components that handle different
// aspects of transaction processing:
//
// Components:
//   - Server: Fiber HTTP server for REST API
//   - ServerGRPC: gRPC server for internal service calls
//   - MultiQueueConsumer: RabbitMQ consumer for async transactions
//   - RedisQueueConsumer: Redis queue consumer for balance updates
//   - BalanceSyncWorker: Background worker for balance persistence
//   - Logger: Structured logging for all components
//
// Lifecycle:
//
//	1. New*Service() creates and wires all components
//	2. Run() starts all components concurrently via Launcher
//	3. Launcher manages graceful shutdown on SIGTERM/SIGINT
//
// Thread Safety:
// Each embedded component handles its own concurrency. Service coordinates
// their lifecycle but does not synchronize their operations.
type Service struct {
	*Server
	*ServerGRPC
	*MultiQueueConsumer
	*RedisQueueConsumer
	*BalanceSyncWorker
	libLog.Logger
}

// Run starts all application components and blocks until shutdown.
//
// This method orchestrates the concurrent startup of all service components.
// The Launcher ensures all components start together and handles graceful
// shutdown when a termination signal is received.
//
// Component Startup Order:
// All components start concurrently. There is no guaranteed order, so each
// component must handle the case where dependencies may not be ready.
//
// Startup Sequence:
//
//	Step 1: Create Launcher with logger
//	Step 2: Register all components for concurrent startup
//	Step 3: Start all components (Fiber, gRPC, consumers, workers)
//	Step 4: Block until shutdown signal (SIGTERM/SIGINT)
//	Step 5: Gracefully stop all components
//
// Usage:
//
//	func main() {
//	    svc := bootstrap.NewService(cfg)
//	    svc.Run()  // Blocks until shutdown
//	}
//
// Graceful Shutdown:
// On SIGTERM/SIGINT, the Launcher coordinates shutdown:
//   - HTTP server stops accepting new connections
//   - In-flight requests complete (with timeout)
//   - Consumers stop consuming (but finish processing current messages)
//   - Workers complete current sync cycle
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
