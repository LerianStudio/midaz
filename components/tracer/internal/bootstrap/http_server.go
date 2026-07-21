// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"crypto/tls"
	"fmt"
	"net"

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
	// tlsConfig secures the REST reservation seam under TRACER_TLS_MODE=mtls
	// (Epic 1.3). When non-nil, Run serves over a TLS listener that
	// requires+verifies a client cert; when nil the server listens plaintext
	// (mesh mode, where a sidecar terminates mTLS).
	tlsConfig *tls.Config
	logger    libObsLog.Logger
	telemetry libObsOtel.Telemetry
}

// ServerAddress is a convenience method to return the server address.
func (s *HTTPServer) ServerAddress() string {
	return s.serverAddress
}

// NewHTTPServer creates an instance of HTTPServer. tlsConfig secures the REST
// reservation seam in mtls mode (non-nil) or is nil for plaintext (mesh mode).
// Returns error instead of panic per Ring standards (no panic outside main.go).
func NewHTTPServer(cfg *Config, app *fiber.App, tlsConfig *tls.Config, logger libObsLog.Logger, telemetry *libObsOtel.Telemetry) (*HTTPServer, error) {
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
		tlsConfig:     tlsConfig,
		logger:        logger,
		telemetry:     *telemetry,
	}, nil
}

// Run runs the server.
//
// In mesh/plaintext mode (tlsConfig == nil) the server is driven through the
// lib-commons ServerManager so it inherits the standard graceful-shutdown path.
// In mtls mode (tlsConfig != nil) the ServerManager has no Fiber+TLS option, so
// Run binds a TLS listener itself and hands it to fiber.App.Listener; graceful
// drain is still owned by Service.Shutdown, which calls app.ShutdownWithContext
// (fiber.App.Listener returns nil after Shutdown, mirroring the ServerManager
// path).
func (s *HTTPServer) Run(_ *libCommons.Launcher) error {
	if s.tlsConfig == nil {
		libCommonsServer.NewServerManager(nil, &s.telemetry, s.logger).
			WithHTTPServer(s.app, s.serverAddress).
			StartWithGracefulShutdown()

		return nil
	}

	listener, err := net.Listen("tcp", s.serverAddress)
	if err != nil {
		return fmt.Errorf("failed to bind tracer TLS listener on %q: %w", s.serverAddress, err)
	}

	if err := s.app.Listener(tls.NewListener(listener, s.tlsConfig)); err != nil {
		return fmt.Errorf("tracer TLS HTTP server: %w", err)
	}

	return nil
}
