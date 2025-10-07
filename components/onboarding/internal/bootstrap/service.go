// Package bootstrap provides application initialization and dependency injection for the onboarding service.
// This file defines the Service struct and application startup logic.
package bootstrap

import (
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
)

// Service is the top-level application container that holds all components.
//
// This struct embeds the HTTP server and logger, providing a unified interface
// for starting and managing the application lifecycle.
type Service struct {
	*Server       // HTTP server (embedded)
	libLog.Logger // Application logger (embedded)
}

// Run starts the onboarding service application.
//
// This method uses the lib-commons launcher to start the HTTP server with:
//   - Graceful shutdown handling
//   - Signal handling (SIGTERM, SIGINT)
//   - Structured logging
//   - Clean resource cleanup on shutdown
//
// This is the only method needed to be called from main.go to run the service.
func (app *Service) Run() {
	libCommons.NewLauncher(
		libCommons.WithLogger(app.Logger),
		libCommons.RunApp("Fiber Server", app.Server),
	).Run()
}
