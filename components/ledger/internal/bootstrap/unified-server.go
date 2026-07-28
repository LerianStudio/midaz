// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"os"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v6/commons/net/http"
	openapi "github.com/LerianStudio/lib-commons/v6/commons/net/http/openapi"
	problem "github.com/LerianStudio/lib-commons/v6/commons/net/http/problem"
	libCommonsServer "github.com/LerianStudio/lib-commons/v6/commons/server"
	libLog "github.com/LerianStudio/lib-observability/v2/log"
	libObsMiddleware "github.com/LerianStudio/lib-observability/v2/middleware"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v2/tracing"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"

	"github.com/LerianStudio/midaz/v4/pkg/buildinfo"
	midazhttp "github.com/LerianStudio/midaz/v4/pkg/net/http"
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
	humaMountV2 HumaRouteRegistrar,
	routeRegistrars ...RouteRegistrar,
) *UnifiedServer {
	app := fiber.New(fiber.Config{
		AppName:      "Midaz Ledger API",
		ErrorHandler: midazhttp.CanonicalFiberErrorHandler,
	})

	// Suppress the Fiber startup banner. The banner is gated at listen time via
	// ListenConfig, which the lib-commons ServerManager owns (it calls app.Listen
	// with no config), so the pre-startup hook is the only seam available here to
	// keep boot silent.
	app.Hooks().OnPreStartupMessage(func(data *fiber.PreStartupMessageData) error {
		data.PreventDefault = true

		return nil
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

	// Huma bootstrap (asset migration DE-RISK). Each contract instance binds to its
	// own Fiber GROUP with a GROUP-RELATIVE op path set and its own Huma document;
	// the auth+tenant middleware chain is attached on the SAME group inside the mount
	// closure, before each Huma terminal. Both the /v1 and the /v2 mounts route
	// through mountHumaContract, which owns the invariant scaffolding.
	if humaMount != nil {
		// v1: title "Midaz Ledger API", no Description, WITH the ledger schema namer
		// to disambiguate the one cross-package schema-name clash (mmodel.Balance vs
		// operation.Balance) before any huma.Register on the shared registry.
		mountHumaContract(app, logger, "/v1", "Midaz Ledger API", "", version, true, humaMount)
	}

	// Second, INDEPENDENT contract instance (ADR-003). The /v2 API owns a SEPARATE
	// component registry, so v1 and v2 schema names never collide across contracts.
	// The ledger schema namer is still installed WITHIN the v2 registry because the
	// v2 create output embeds transaction.Transaction, which nests operation.Operation
	// → operation.{Status,Balance,Amount} and clashes with transaction.Status on the
	// bare schema names (the SAME disambiguation the v1 mount applies).
	if humaMountV2 != nil {
		mountHumaContract(app, logger, "/v2", "Midaz Ledger API v2", "Midaz Ledger v2 API contract.", version, true, humaMountV2)
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

		// Register OnPreShutdown hook to enable graceful drain. Fiber v3 split the
		// v2 OnShutdown hook into OnPreShutdown (runs before shutdown begins) and
		// OnPostShutdown; the drain-before-close behavior maps to OnPreShutdown.
		// When SIGTERM is received, this hook:
		// 1. Calls StartDrain() so readyz returns 503
		// 2. Waits DefaultDrainDelay (12s) for load balancers to stop routing traffic
		// 3. Returns, allowing Fiber to proceed with connection draining
		app.Hooks().OnPreShutdown(func() error {
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

// mountHumaContract mounts one independent Huma contract instance under prefix and
// encapsulates the INVARIANT scaffolding shared by every version: problem.Install()
// (idempotent RFC 9457 model override, MUST precede any huma.Register), the Fiber
// group + Huma document creation, the BearerAuth + ApiKeyAuth SPEC-ONLY security
// scheme declarations, the mount closure invocation, and the swaggerEnabled()-gated
// native OpenAPI 3.1 spec + Scalar docs surface.
//
// The DIVERGENT bits are parameters: prefix (Fiber group + Servers entry + ServeSpec
// prefix), title/description/version (Info metadata), installSchemaNamer (v1 needs
// the ledger schema namer to break one cross-package name clash; v2 owns a separate
// registry and opts out), and the mount closure (per-version op registration + the
// per-group Fiber auth/tenant chain). ServeSpec runs AFTER mount so the snapshotted
// spec is complete. Security schemes are SPEC metadata only — runtime auth stays the
// Fiber guard chain the mount closure attaches.
func mountHumaContract(
	app *fiber.App,
	logger libLog.Logger,
	prefix string,
	title string,
	description string,
	version string,
	installSchemaNamer bool,
	mount HumaRouteRegistrar,
) {
	problem.Install()

	group := app.Group(prefix)

	api := openapi.New(app, group, openapi.Config{
		Title:       title,
		Version:     version,
		Description: description,
		Servers:     []string{prefix},
	})

	if installSchemaNamer {
		midazhttp.InstallLedgerSchemaNamer(api)
	}

	openapi.DeclareBearerAuth(api)

	components := api.OpenAPI().Components
	if components.SecuritySchemes == nil {
		components.SecuritySchemes = map[string]*huma.SecurityScheme{}
	}

	components.SecuritySchemes["ApiKeyAuth"] = &huma.SecurityScheme{
		Type:        "apiKey",
		In:          "header",
		Name:        "X-API-Key",
		Description: "Static API key presented in the X-API-Key header.",
	}

	mount(group, api)

	if swaggerEnabled() {
		openapi.ServeSpec(app, api, logger, prefix, title)
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
