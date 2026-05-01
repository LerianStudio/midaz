// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libLog "github.com/LerianStudio/lib-commons/v5/commons/log"
	libHTTP "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v5/commons/opentelemetry"
	libCommonsServer "github.com/LerianStudio/lib-commons/v5/commons/server"
	_ "github.com/LerianStudio/midaz/v3/components/ledger/api"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	fiberSwagger "github.com/swaggo/fiber-swagger"
)

// RouteRegistrar is a function that registers routes to an existing Fiber router.
// Each module (onboarding, transaction) implements this to register its routes.
type RouteRegistrar func(router fiber.Router)

// UnifiedServer consolidates all HTTP APIs (onboarding + transaction) in a single Fiber server.
// This enables the unified ledger mode where all routes are accessible on a single port.
type UnifiedServer struct {
	app           *fiber.App
	serverAddress string
	logger        libLog.Logger
	telemetry     *libOpentelemetry.Telemetry
	readyzHandler *ReadyzHandler
}

// NewUnifiedServer creates a server that exposes all APIs on a single port.
// Route registrars are responsible for attaching any module-specific middleware.
func NewUnifiedServer(
	serverAddress string,
	logger libLog.Logger,
	telemetry *libOpentelemetry.Telemetry,
	readyzHandler *ReadyzHandler,
	routeRegistrars ...RouteRegistrar,
) *UnifiedServer {
	app := fiber.New(fiber.Config{
		AppName:               "Midaz Ledger API",
		DisableStartupMessage: true,
		ErrorHandler: func(ctx *fiber.Ctx, err error) error {
			return libHTTP.FiberErrorHandler(ctx, err)
		},
	})

	// Add common middleware (only once for all routes)
	tlMid := libHTTP.NewTelemetryMiddleware(telemetry)
	app.Use(tlMid.WithTelemetry(telemetry))
	app.Use(cors.New())
	app.Use(libHTTP.WithHTTPLogging(libHTTP.WithCustomLogger(logger)))

	// Health check for the unified server
	app.Get("/health", libHTTP.Ping)

	// Version endpoint
	app.Get("/version", libHTTP.Version)

	// Readyz endpoint - mounted BEFORE auth middleware (before route registrars)
	// This endpoint is public and does not require authentication.
	if readyzHandler != nil {
		app.Get("/readyz", readyzHandler.HandleReadyz)
	}

	// Swagger documentation (unified onboarding + transaction)
	app.Get("/swagger", func(c *fiber.Ctx) error {
		return c.Redirect("/swagger/index.html", fiber.StatusMovedPermanently)
	})
	app.Get("/swagger/*", WithSwaggerEnvConfig(), fiberSwagger.FiberWrapHandler(
		fiberSwagger.InstanceName("swagger"),
	))

	// Register routes from each module
	for _, registrar := range routeRegistrars {
		if registrar != nil {
			registrar(app)
		}
	}

	// End tracing spans middleware (must be last)
	app.Use(tlMid.EndTracingSpans)

	// Register OnListen hook to mark server ready AFTER socket is bound.
	// This avoids the race condition where readyz returns 200 before Fiber is listening.
	if readyzHandler != nil {
		app.Hooks().OnListen(func(ld fiber.ListenData) error {
			readyzHandler.SetServerReady()
			logger.Log(context.Background(), libLog.LevelInfo,
				"Server listening, readyz now returning healthy",
				libLog.String("host", ld.Host),
				libLog.String("port", ld.Port))

			return nil
		})

		// Register OnShutdown hook to enable graceful drain.
		// When SIGTERM is received, this hook:
		// 1. Calls StartDrain() so readyz returns 503
		// 2. Waits DefaultDrainDelay (12s) for load balancers to stop routing traffic
		// 3. Returns, allowing Fiber to proceed with connection draining
		app.Hooks().OnShutdown(func() error {
			readyzHandler.StartDrain()
			logger.Log(context.Background(), libLog.LevelInfo,
				"Graceful drain started, waiting for load balancers to update",
				libLog.String("drain_delay", DefaultDrainDelay.String()))
			time.Sleep(DefaultDrainDelay)
			logger.Log(context.Background(), libLog.LevelInfo, "Drain delay complete, proceeding with shutdown")

			return nil
		})
	}

	return &UnifiedServer{
		app:           app,
		serverAddress: serverAddress,
		logger:        logger,
		telemetry:     telemetry,
		readyzHandler: readyzHandler,
	}
}

// Run implements mbootstrap.Runnable interface.
// Starts the unified HTTP server with graceful shutdown support.
func (s *UnifiedServer) Run(l *libCommons.Launcher) error {
	s.logger.Log(context.Background(), libLog.LevelInfo, fmt.Sprintf("Starting Unified HTTP Server on %s", s.serverAddress))

	// Create server manager with graceful shutdown.
	// The OnListen hook (registered in NewUnifiedServer) will call SetServerReady()
	// after the socket is bound, ensuring readyz only returns 200 when truly ready.
	libCommonsServer.NewServerManager(nil, s.telemetry, s.logger).
		WithHTTPServer(s.app, s.serverAddress).
		StartWithGracefulShutdown()

	return nil
}

// ServerAddress returns the server address for logging/debugging purposes.
func (s *UnifiedServer) ServerAddress() string {
	return s.serverAddress
}
