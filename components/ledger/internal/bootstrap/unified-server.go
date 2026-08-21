// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"time"

	libCommons "github.com/LerianStudio/lib-commons/v6/commons"
	libHTTP "github.com/LerianStudio/lib-commons/v6/commons/net/http"
	openapi "github.com/LerianStudio/lib-commons/v6/commons/net/http/openapi"
	libCommonsServer "github.com/LerianStudio/lib-commons/v6/commons/server"
	libLog "github.com/LerianStudio/lib-observability/v2/log"
	libObsMiddleware "github.com/LerianStudio/lib-observability/v2/middleware"
	libOpentelemetry "github.com/LerianStudio/lib-observability/v2/tracing"
	"github.com/danielgtaylor/huma/v2"
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"

	httpin "github.com/LerianStudio/midaz/v4/components/ledger/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v4/pkg/buildinfo"
	midazhttp "github.com/LerianStudio/midaz/v4/pkg/net/http"
)

// RouteRegistrar mounts routes on the Fiber app root, outside the version-prefixed
// Huma groups. It is the seam for a surface that carries no OAS contract and so cannot
// be a Huma terminal — currently only the streaming manifest. Every surface in the
// OAS contract mounts through HumaRouteRegistrar instead.
type RouteRegistrar func(router fiber.Router)

// HumaRouteRegistrar registers one API version's Huma-migrated operations on a
// version-prefixed Huma Group and its backing Fiber group (which carries the
// Fiber-level auth/tenant middleware chain that runs before each Huma terminal).
// The Group and the Fiber group resolve to the same raw path, so guard and terminal
// chain on one route. Nil means that version mounts no routes.
type HumaRouteRegistrar func(group fiber.Router, api huma.API)

// openAPIDocsEnabled reports whether the native Huma OpenAPI 3.1 spec + Scalar docs
// surface should be served (serveVersionedDocs: the unified /openapi.{json,yaml}, the
// per-version /openapi.v{1,2}.json view specs, and the /docs version selector, all at
// the root since one document backs every version). Off by default; opt in with
// OPENAPI_DOCS_ENABLED=true. An absent or unparseable value leaves the contract unserved.
func openAPIDocsEnabled() bool {
	return libCommons.GetenvBoolOrDefault("OPENAPI_DOCS_ENABLED", false)
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

	// Mount app-root routes, which sit outside the versioned Huma groups.
	for _, registrar := range routeRegistrars {
		if registrar != nil {
			registrar(app)
		}
	}

	// Huma bootstrap (asset migration DE-RISK). ONE OpenAPI document backs every
	// version: mountHumaContracts assembles a single Huma contract on the app root and
	// mounts each version under its own path prefix — a Fiber group for the auth/tenant
	// guard chain and a Huma Group for the operations. v1 and v2 share the component
	// registry, so a duplicate operation ID or a schema name reused for a different type
	// is a boot panic rather than a silent second document. A nil registrar mounts
	// nothing for that version; all-nil mounts no document at all.
	mountHumaContracts(
		app, logger, version,
		humaContract{prefix: "/v1", mount: humaMount},
		humaContract{prefix: "/v2", mount: humaMountV2},
	)

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

// humaContract pairs a URL version prefix with the registrar that mounts that
// version's operations. mountHumaContracts turns each into a Fiber guard-chain group
// and a Huma operation Group over the ONE shared document.
type humaContract struct {
	prefix string
	mount  HumaRouteRegistrar
}

// mountHumaContracts mounts every non-nil contract onto ONE OpenAPI document, so the
// ledger describes itself with a single spec whose operation paths carry the version
// prefix. Each numbered step reads as it does for a specific reason:
//
//  1. When every contract mount is nil there is no document to build and no route to
//     serve, so return first — this preserves the pre-Huma posture of a server with no
//     version mounted.
//  2. Assemble the ONE contract on the app ROOT (app passed as both app and router):
//     the Huma Group's PrefixModifier is then the only thing prefixing an operation
//     path, so "/v1/organizations" is produced exactly once — a Fiber group feeding a
//     Huma group of the same name would double the prefix. Servers is the explicit
//     root "/": the version rides the operation PATH, not the servers block, and the
//     explicit "/" keeps the dump symmetric for downstream spec consumers.
//     AssembleHumaContract installs the schema namer, which REPLACES the component
//     registry; it must run EXACTLY ONCE, which is why one document backs every
//     version and a second AssembleHumaContract would discard the first's schemas.
//  3. Per contract: a Fiber group carries the auth/tenant guard chain and a Huma Group
//     carries the operations. Both resolve to the SAME raw path — the Fiber group's
//     getGroupPath(prefix, opPath) equals the prefix the PrefixModifier writes into
//     op.Path before the Fiber adapter sees it — so the guard and its terminal chain
//     on one Fiber route.
//  4. Serve the docs LAST and ONCE, at the root: serveVersionedDocs snapshots the
//     document bytes and derives the per-version view specs, so it must run after the
//     final huma.Register. Exposure is bootstrap policy gated on openAPIDocsEnabled(),
//     which is why it lives here and not inside AssembleHumaContract.
func mountHumaContracts(app *fiber.App, logger libLog.Logger, version string, contracts ...humaContract) {
	anyMounted := false

	for _, c := range contracts {
		if c.mount != nil {
			anyMounted = true

			break
		}
	}

	if !anyMounted {
		return
	}

	const title = "Midaz Ledger API"

	api := httpin.AssembleHumaContract(app, app, openapi.Config{
		Title:       title,
		Version:     version,
		Description: "Midaz Ledger API. Operations are served under the /v1 and /v2 path prefixes on this single document.",
		Servers:     []string{"/"},
	})

	for _, c := range contracts {
		if c.mount == nil {
			continue
		}

		fiberGroup := app.Group(c.prefix)
		humaGroup := huma.NewGroup(api, c.prefix)
		c.mount(fiberGroup, humaGroup)
	}

	httpin.MarkV1OperationsDeprecated(api)
	httpin.ApplyVersionTagGroups(api)

	if openAPIDocsEnabled() {
		serveVersionedDocs(app, api, logger, title)
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
