package bootstrap

import (
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libLog "github.com/LerianStudio/lib-commons/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	"github.com/gofiber/fiber/v2"
	"github.com/pkg/errors"
)

// Server represents the http server for Ledger service.
type Server struct {
	app           *fiber.App
	serverAddress string
	libLog.Logger
	libOpentelemetry.Telemetry
}

// ServerAddress returns is a convenience method to return the server address.
func (s *Server) ServerAddress() string {
	return s.serverAddress
}

// NewServer creates an instance of Server.
func NewServer(cfg *Config, app *fiber.App, logger libLog.Logger, telemetry *libOpentelemetry.Telemetry) *Server {
	return &Server{
		app:           app,
		serverAddress: cfg.ServerAddress,
		Logger:        logger,
		Telemetry:     *telemetry,
	}
}

// Run runs the server.
func (s *Server) Run(l *libCommons.Launcher) error {
	s.InitializeTelemetry(s.Logger)

	defer s.ShutdownTelemetry()

	defer func() {
		if err := s.Sync(); err != nil {
			s.Fatalf("Failed to sync logger: %s", err)
		}
	}()

	err := s.app.Listen(s.ServerAddress())

	if err != nil {
		return errors.Wrap(err, "failed to run the server")
	}

	return nil
}
