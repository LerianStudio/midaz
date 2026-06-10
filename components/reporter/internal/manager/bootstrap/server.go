// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libCommonsServer "github.com/LerianStudio/lib-commons/v5/commons/server"
	libObsLog "github.com/LerianStudio/lib-observability/log"
	libObsOtel "github.com/LerianStudio/lib-observability/tracing"
	"github.com/gofiber/fiber/v2"
)

// Server represents the http server for Ledger services.
type Server struct {
	app           *fiber.App
	serverAddress string
	logger        libObsLog.Logger
	telemetry     libObsOtel.Telemetry
}

// ServerAddress returns is a convenience method to return the server address.
func (s *Server) ServerAddress() string {
	return s.serverAddress
}

// NewServer creates an instance of Server.
func NewServer(cfg *Config, app *fiber.App, logger libObsLog.Logger, telemetry *libObsOtel.Telemetry) *Server {
	s := &Server{
		app:           app,
		serverAddress: cfg.ServerAddress,
		logger:        logger,
	}

	if telemetry != nil {
		s.telemetry = *telemetry
	}

	return s
}

// Run runs the server.
func (s *Server) Run(l *libCommons.Launcher) error {
	libCommonsServer.NewServerManager(nil, &s.telemetry, s.logger).
		WithHTTPServer(s.app, s.serverAddress).
		StartWithGracefulShutdown()

	return nil
}
