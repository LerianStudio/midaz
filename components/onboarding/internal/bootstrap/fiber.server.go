// Package bootstrap provides application initialization and dependency injection for the onboarding service.
// This file defines the HTTP server configuration and lifecycle management.
package bootstrap

import (
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libCommonsServer "github.com/LerianStudio/lib-commons/v2/commons/server"
	"github.com/gofiber/fiber/v2"
)

// Server represents the HTTP server for the onboarding service.
//
// This struct encapsulates the Fiber web application with its configuration,
// logger, and telemetry components. It provides lifecycle management methods
// for starting and stopping the server.
type Server struct {
	app           *fiber.App                 // Fiber web application
	serverAddress string                     // Server listen address (e.g., ":3000")
	logger        libLog.Logger              // Structured logger
	telemetry     libOpentelemetry.Telemetry // OpenTelemetry tracer and metrics
}

// ServerAddress returns the configured server listen address.
//
// This is a convenience method for accessing the server address configuration.
//
// Returns:
//   - string: Server address (e.g., ":3000", "0.0.0.0:8080")
func (s *Server) ServerAddress() string {
	return s.serverAddress
}

// NewServer creates a new HTTP server instance with the provided configuration.
//
// This constructor initializes the server with all necessary components for
// handling HTTP requests, logging, and telemetry.
//
// Parameters:
//   - cfg: Application configuration
//   - app: Configured Fiber application with routes and middleware
//   - logger: Structured logger instance
//   - telemetry: OpenTelemetry instance for tracing
//
// Returns:
//   - *Server: Initialized server ready to run
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
// Graceful Shutdown:
//   - Waits for in-flight requests to complete
//   - Closes database connections
//   - Flushes telemetry data
//   - Closes RabbitMQ connections
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
