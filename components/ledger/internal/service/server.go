package service

import (
	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/gofiber/fiber/v2"
	"github.com/pkg/errors"
)

// Server represents the http server for Ledger service.
type Server struct {
	app           *fiber.App
	serverAddress string
	mlog.Logger
}

// ServerAddress returns is a convenience method to return the server address.
func (s *Server) ServerAddress() string {
	return s.serverAddress
}

// NewServer creates an instance of Server.
func NewServer(cfg *Config, app *fiber.App, logger mlog.Logger) *Server {
	return &Server{
		app:           app,
		serverAddress: cfg.ServerAddress,
		Logger:        logger,
	}
}

// Run runs the server.
func (s *Server) Run(l *common.Launcher) error {
	err := s.app.Listen(s.ServerAddress())
	if err != nil {
		return errors.Wrap(err, "failed to run the server")
	}

	defer func() {
		if err := s.Logger.Sync(); err != nil {
			s.Logger.Fatalf("Failed to sync logger: %s", err)
		}
	}()

	return nil
}
