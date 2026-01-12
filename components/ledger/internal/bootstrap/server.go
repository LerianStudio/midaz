package bootstrap

import (
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libCommonsServer "github.com/LerianStudio/lib-commons/v2/commons/server"
	"github.com/gofiber/fiber/v2"
)

// Server represents the HTTP server for Ledger metadata index services.
type Server struct {
	app           *fiber.App
	serverAddress string
	logger        libLog.Logger
	telemetry     libOpentelemetry.Telemetry
}

// ServerAddress returns the server address.
func (s *Server) ServerAddress() string {
	return s.serverAddress
}

// NewServer creates an instance of Server.
func NewServer(cfg *Config, app *fiber.App, logger libLog.Logger, telemetry *libOpentelemetry.Telemetry) *Server {
	serverAddress := cfg.ServerAddress
	if serverAddress == "" {
		serverAddress = ":3002"
	}

	return &Server{
		app:           app,
		serverAddress: serverAddress,
		logger:        logger,
		telemetry:     *telemetry,
	}
}

// Run runs the server.
func (s *Server) Run(l *libCommons.Launcher) error {
	libCommonsServer.NewServerManager(nil, &s.telemetry, s.logger).
		WithHTTPServer(s.app, s.serverAddress).
		StartWithGracefulShutdown()

	return nil
}
