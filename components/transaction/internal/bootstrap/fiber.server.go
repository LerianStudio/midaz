// Package bootstrap provides application initialization and dependency injection for the transaction service.
// This file defines the HTTP server component.
package bootstrap

import (
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libCommonsServer "github.com/LerianStudio/lib-commons/v2/commons/server"
	"github.com/gofiber/fiber/v2"
)

// Server represents the HTTP server for the transaction service.
//
// This struct encapsulates the Fiber HTTP server and its configuration.
// The server handles REST API requests for transaction processing, balance
// queries, operation tracking, and routing management.
type Server struct {
	app           *fiber.App
	serverAddress string
	logger        libLog.Logger
	telemetry     libOpentelemetry.Telemetry
}

// ServerAddress returns the configured server address.
//
// Returns:
//   - string: Server address (e.g., ":3000")
func (s *Server) ServerAddress() string {
	return s.serverAddress
}

// NewServer creates a new HTTP server instance.
//
// Parameters:
//   - cfg: Configuration with server address
//   - app: Configured Fiber application with routes
//   - logger: Logger instance
//   - telemetry: Telemetry instance for tracing
//
// Returns:
//   - *Server: Configured server ready to run
func NewServer(cfg *Config, app *fiber.App, logger libLog.Logger, telemetry *libOpentelemetry.Telemetry) *Server {
	return &Server{
		app:           app,
		serverAddress: cfg.ServerAddress,
		logger:        logger,
		telemetry:     *telemetry,
	}
}

// Run starts the HTTP server with graceful shutdown support.
//
// This method starts the Fiber HTTP server and configures graceful shutdown handling.
// The server will:
//   - Listen on the configured address
//   - Handle incoming HTTP requests
//   - Respond to shutdown signals (SIGTERM, SIGINT)
//   - Clean up resources on shutdown
//   - Close telemetry connections
//
// Parameters:
//   - l: Launcher instance (unused in current implementation)
//
// Returns:
//   - error: Always returns nil (errors are handled by lib-commons)
func (s *Server) Run(l *libCommons.Launcher) error {
	libCommonsServer.NewServerManager(nil, &s.telemetry, s.logger).
		WithHTTPServer(s.app, s.serverAddress).
		StartWithGracefulShutdown()

	return nil
}
