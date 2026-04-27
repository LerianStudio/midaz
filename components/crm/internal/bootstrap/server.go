// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libCommonsLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libCommonsOtel "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	libCommonsServer "github.com/LerianStudio/lib-commons/v4/commons/server"
	"github.com/gofiber/fiber/v2"
)

// Server represents the http server for CRM services.
type Server struct {
	app           *fiber.App
	serverAddress string
	logger        libCommonsLog.Logger
	telemetry     libCommonsOtel.Telemetry
	readyzHandler *ReadyzHandler
}

// ServerAddress returns is a convenience method to return the server address.
func (s *Server) ServerAddress() string {
	return s.serverAddress
}

// NewServer creates an instance of Server.
func NewServer(cfg *Config, app *fiber.App, logger libCommonsLog.Logger, telemetry *libCommonsOtel.Telemetry, readyzHandler *ReadyzHandler) *Server {
	return &Server{
		app:           app,
		serverAddress: cfg.ServerAddress,
		logger:        logger,
		telemetry:     *telemetry,
		readyzHandler: readyzHandler,
	}
}

// Run runs the server.
func (s *Server) Run(l *libCommons.Launcher) error {
	// Register lifecycle hooks for readyz
	s.app.Hooks().OnListen(func(_ fiber.ListenData) error {
		if s.readyzHandler != nil {
			s.readyzHandler.SetServerReady()
		}

		return nil
	})

	s.app.Hooks().OnShutdown(func() error {
		if s.readyzHandler != nil {
			s.readyzHandler.StartDrain()
			// Wait for drain delay to allow load balancers to stop sending traffic
			s.logger.Log(context.Background(), libCommonsLog.LevelInfo,
				"Waiting for graceful drain before shutdown",
				libCommonsLog.String("drain_delay", DefaultDrainDelay.String()))
			time.Sleep(DefaultDrainDelay)
		}

		return nil
	})

	libCommonsServer.NewServerManager(nil, &s.telemetry, s.logger).
		WithHTTPServer(s.app, s.serverAddress).
		StartWithGracefulShutdown()

	return nil
}
