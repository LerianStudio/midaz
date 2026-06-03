// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"fmt"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libCommonsServer "github.com/LerianStudio/lib-commons/v5/commons/server"
	libObsLog "github.com/LerianStudio/lib-observability/log"
	libObsOtel "github.com/LerianStudio/lib-observability/tracing"
	"github.com/gofiber/fiber/v2"
)

// HTTPServer represents the http server for Tracer services.
type HTTPServer struct {
	app           *fiber.App
	serverAddress string
	logger        libObsLog.Logger
	telemetry     libObsOtel.Telemetry
}

// ServerAddress is a convenience method to return the server address.
func (s *HTTPServer) ServerAddress() string {
	return s.serverAddress
}

// NewHTTPServer creates an instance of HTTPServer.
// Returns error instead of panic per Ring standards (no panic outside main.go).
func NewHTTPServer(cfg *Config, app *fiber.App, logger libObsLog.Logger, telemetry *libObsOtel.Telemetry) (*HTTPServer, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config must not be nil")
	}

	if app == nil {
		return nil, fmt.Errorf("app must not be nil")
	}

	if logger == nil {
		return nil, fmt.Errorf("logger must not be nil")
	}

	if telemetry == nil {
		return nil, fmt.Errorf("telemetry must not be nil")
	}

	return &HTTPServer{
		app:           app,
		serverAddress: cfg.ServerAddress,
		logger:        logger,
		telemetry:     *telemetry,
	}, nil
}

// Run runs the server.
func (s *HTTPServer) Run(l *libCommons.Launcher) error {
	libCommonsServer.NewServerManager(nil, &s.telemetry, s.logger).
		WithHTTPServer(s.app, s.serverAddress).
		StartWithGracefulShutdown()

	return nil
}
