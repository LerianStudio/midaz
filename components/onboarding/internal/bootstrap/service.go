// Package bootstrap provides application initialization and lifecycle management for the onboarding component.
//
// This package implements the service composition root, wiring together all dependencies
// and providing the main entry point for the onboarding microservice.
//
// Onboarding Component Purpose:
//
// The onboarding component handles entity lifecycle management:
//   - Organization management (create, update, list)
//   - Ledger management (create, update, list)
//   - Account management (create, update, list)
//   - Portfolio management (create, update, list)
//   - Asset management (create, update, list)
//   - Segment management (create, update, list)
//
// Bootstrap Architecture:
//
//	┌─────────────────────────────────────────────────────────┐
//	│                      Service                             │
//	│  ┌─────────────────────────────────────────────────────┐ │
//	│  │                   Server (Fiber)                    │ │
//	│  │  - HTTP handlers                                    │ │
//	│  │  - Middleware (auth, logging, tracing)              │ │
//	│  │  - Health endpoints                                 │ │
//	│  └─────────────────────────────────────────────────────┘ │
//	│                                                          │
//	│  ┌─────────────────────────────────────────────────────┐ │
//	│  │                   Logger                            │ │
//	│  │  - Structured logging                               │ │
//	│  │  - Log level configuration                          │ │
//	│  └─────────────────────────────────────────────────────┘ │
//	└─────────────────────────────────────────────────────────┘
//
// Related Packages:
//   - apps/midaz/components/onboarding/internal/adapters: Repository implementations
//   - apps/midaz/components/onboarding/internal/services: Use case implementations
package bootstrap

import (
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
)

// Service is the application composition root that aggregates all top-level components.
//
// Service acts as the "glue" that wires together the HTTP server and supporting
// infrastructure. It provides a single point of initialization and shutdown for
// the entire application.
//
// Components:
//   - Server: Fiber HTTP server handling REST API requests
//   - Logger: Structured logger for application-wide logging
//
// Lifecycle:
//
//	1. New*Service() called in main.go (creates and wires components)
//	2. Service.Run() called (starts all components)
//	3. Launcher manages graceful shutdown on SIGTERM/SIGINT
//
// Thread Safety:
// Service itself is not thread-safe during initialization, but once Run() is called,
// the underlying components handle concurrent requests safely.
type Service struct {
	*Server
	libLog.Logger
}

// Run starts the application and blocks until shutdown signal is received.
//
// This method is the only code required in main.go to start the application.
// It uses libCommons.Launcher to manage:
//   - Concurrent startup of all components
//   - Graceful shutdown on SIGTERM/SIGINT
//   - Error propagation from failed components
//
// Startup Sequence:
//
//	Step 1: Create Launcher with logger and components
//	Step 2: Start Fiber Server (blocks until shutdown)
//	Step 3: On shutdown signal, gracefully stop all components
//
// Usage:
//
//	func main() {
//	    svc := bootstrap.NewService(cfg)
//	    svc.Run()  // Blocks until shutdown
//	}
//
// Graceful Shutdown:
// The Launcher intercepts SIGTERM/SIGINT and coordinates shutdown of all
// registered components, ensuring in-flight requests complete before exit.
func (app *Service) Run() {
	libCommons.NewLauncher(
		libCommons.WithLogger(app.Logger),
		libCommons.RunApp("Fiber Server", app.Server),
	).Run()
}
