package bootstrap

import (
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libCommonsServer "github.com/LerianStudio/lib-commons/v2/commons/server"
	"github.com/LerianStudio/midaz/v3/pkg/utils"
	"github.com/gofiber/fiber/v2"
)

// Server represents the http server for Ledger services.
type Server struct {
	app           *fiber.App
	serverAddress string
	logger        libLog.Logger
	telemetry     libOpentelemetry.Telemetry
}

// ServerAddress returns is a convenience method to return the server address.
func (s *Server) ServerAddress() string {
	return s.serverAddress
}

// NewServer creates an instance of Server.
func NewServer(cfg *Config, app *fiber.App, logger libLog.Logger, telemetry *libOpentelemetry.Telemetry) *Server {
	// Use prefixed server address if available, otherwise fallback to non-prefixed
	serverAddress := utils.EnvFallback(cfg.PrefixedServerAddress, cfg.ServerAddress)

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
