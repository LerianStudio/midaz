// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"os"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v5/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v5/commons/net/http"
	openapi "github.com/LerianStudio/lib-commons/v5/commons/net/http/openapi"
	problem "github.com/LerianStudio/lib-commons/v5/commons/net/http/problem"
	libCommonsServer "github.com/LerianStudio/lib-commons/v5/commons/server"
	libLog "github.com/LerianStudio/lib-observability/log"
	libObsMiddleware "github.com/LerianStudio/lib-observability/middleware"
	libOpentelemetry "github.com/LerianStudio/lib-observability/tracing"
	"github.com/LerianStudio/midaz/v4/pkg/buildinfo"
	midazhttp "github.com/LerianStudio/midaz/v4/pkg/net/http"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
)

// RouteRegistrar is a function that registers routes to an existing Fiber router.
// Each module (onboarding, transaction) implements this to register its routes.
type RouteRegistrar func(router fiber.Router)

// HumaRouteRegistrar registers Huma-migrated routes on the shared /v1 Huma API and
// its backing Fiber group (for the Fiber-level auth/tenant middleware chain that
// runs before each Huma terminal). Nil means no Huma routes are mounted.
type HumaRouteRegistrar func(group fiber.Router, api huma.API)

// swaggerEnabled reports whether the native Huma OpenAPI 3.1 spec + Scalar docs
// surface should be served (openapi.ServeSpec: /v1/openapi.{json,yaml}, /v1/docs).
// Off by default; opt in with LEDGER_HUMA_DOCS_ENABLED=true.
func swaggerEnabled() bool {
	return os.Getenv("LEDGER_HUMA_DOCS_ENABLED") == "true"
}

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
	version string,
	logger libLog.Logger,
	telemetry *libOpentelemetry.Telemetry,
	readyzHandler *ReadyzHandler,
	humaMount HumaRouteRegistrar,
	routeRegistrars ...RouteRegistrar,
) *UnifiedServer {
	app := fiber.New(fiber.Config{
		AppName:               "Midaz Ledger API",
		DisableStartupMessage: true,
		ErrorHandler:          midazhttp.CanonicalFiberErrorHandler,
	})

	// Add common middleware (only once for all routes).
	// WithRecover MUST be first so it wraps every handler and downstream middleware:
	// a panic anywhere unwinds back through this defer and returns a 500 via the
	// Fiber error handler instead of dropping the connection. Previously only CRM's
	// standalone router applied panic recovery; hoisting it here gives onboarding +
	// transaction + crm a single process-wide recovery boundary.
	app.Use(midazhttp.WithRecover(midazhttp.WithRecoverLogger(logger)))

	tlMid := libObsMiddleware.NewTelemetryMiddleware(telemetry)
	app.Use(tlMid.WithTelemetry(telemetry))
	app.Use(cors.New())
	app.Use(libObsMiddleware.WithHTTPLogging(libObsMiddleware.WithCustomLogger(logger)))

	// Health check for the unified server
	app.Get("/health", libHTTP.Ping)

	// Version endpoint
	app.Get("/version", buildinfo.VersionHandler(version))

	// Readyz endpoint - mounted BEFORE auth middleware (before route registrars)
	// This endpoint is public and does not require authentication.
	if readyzHandler != nil {
		app.Get("/readyz", readyzHandler.HandleReadyz)
	}

	// Register routes from each module
	for _, registrar := range routeRegistrars {
		if registrar != nil {
			registrar(app)
		}
	}

	// Huma bootstrap (asset migration DE-RISK). problem.Install() overrides the
	// process-global huma.NewError to the org-wide RFC 9457 model; it MUST run
	// before any huma.Register and is idempotent (sync.Once). The Huma API binds to
	// a /v1 Fiber GROUP with Servers ["/v1"] and GROUP-RELATIVE op paths, so the
	// humafiber v2 adapter registers on that group (Fiber prepends /v1) and the
	// adapter's ctx (built from c.UserContext()) reaches the migrated handlers with
	// the per-route tenant/DB intact — the auth+tenant middleware chain is attached
	// on the SAME group inside humaMount, before each Huma terminal.
	if humaMount != nil {
		problem.Install()

		apiV1 := app.Group("/v1")

		humaAPI := openapi.New(app, apiV1, openapi.Config{
			Title:   "Midaz Ledger API",
			Version: version,
			Servers: []string{"/v1"},
		})

		// Disambiguate the one cross-package schema-name clash (mmodel.Balance vs
		// operation.Balance) before any huma.Register on the shared API; the registry
		// namer is captured on first registration. See InstallLedgerSchemaNamer.
		midazhttp.InstallLedgerSchemaNamer(humaAPI)

		// Declare the security schemes referenced by per-op Security metadata so the
		// generated spec resolves them. SPEC metadata only — runtime auth stays the
		// Fiber guard chain. BearerAuth via the shared lib-commons helper; ApiKeyAuth
		// declared locally (no helper), mirroring the helper's nil-guard.
		openapi.DeclareBearerAuth(humaAPI)

		components := humaAPI.OpenAPI().Components
		if components.SecuritySchemes == nil {
			components.SecuritySchemes = map[string]*huma.SecurityScheme{}
		}

		components.SecuritySchemes["ApiKeyAuth"] = &huma.SecurityScheme{
			Type:        "apiKey",
			In:          "header",
			Name:        "X-API-Key",
			Description: "Static API key presented in the X-API-Key header.",
		}

		humaMount(apiV1, humaAPI)

		// Native Huma OpenAPI 3.1 spec + Scalar docs, gated on swaggerEnabled().
		// Mounted AFTER humaMount so the snapshotted spec is complete.
		if swaggerEnabled() {
			openapi.ServeSpec(app, humaAPI, logger, "/v1", "Midaz Ledger API")
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
	s.logger.Log(context.Background(), libLog.LevelInfo, "Starting Unified HTTP Server", libLog.String("server_address", s.serverAddress))

	// Create server manager with graceful shutdown.
	// The OnListen hook (registered in NewUnifiedServer) will call SetServerReady()
	// after the socket is bound, ensuring readyz only returns 200 when truly ready.
	//
	// ServerManager is the single owner of telemetry teardown and logger sync:
	// it shuts telemetry down only AFTER the HTTP drain completes, so spans from
	// in-flight requests are exported. A signal-fired Launcher runnable cannot
	// provide that ordering (runnables wake concurrently on SIGTERM) — do not
	// move ShutdownTelemetry out of this call.
	libCommonsServer.NewServerManager(nil, s.telemetry, s.logger).
		WithHTTPServer(s.app, s.serverAddress).
		StartWithGracefulShutdown()

	return nil
}

// ServerAddress returns the server address for logging/debugging purposes.
func (s *UnifiedServer) ServerAddress() string {
	return s.serverAddress
}
