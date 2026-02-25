// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	libCommonsLog "github.com/LerianStudio/lib-commons/v3/commons/log"
	libCommonsOtel "github.com/LerianStudio/lib-commons/v3/commons/opentelemetry"
	libCommonsServer "github.com/LerianStudio/lib-commons/v3/commons/server"
	"github.com/gofiber/fiber/v2"
)

// Server represents the http server for Ledger services.
type Server struct {
	app           *fiber.App
	serverAddress string
	logger        libCommonsLog.Logger
	telemetry     libCommonsOtel.Telemetry
}

// ServerAddress returns is a convenience method to return the server address.
func (s *Server) ServerAddress() string {
	return s.serverAddress
}

// NewServer creates an instance of Server.
func NewServer(cfg *Config, app *fiber.App, logger libCommonsLog.Logger, telemetry *libCommonsOtel.Telemetry) *Server {
	return &Server{
		app:           app,
		serverAddress: cfg.ServerAddress,
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
