package bootstrap

import (
	"errors"
	"log"
	"net/http"
	"strings"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	libLog "github.com/LerianStudio/lib-commons/v3/commons/log"
	libHTTP "github.com/LerianStudio/lib-commons/v3/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v3/commons/opentelemetry"
	libCommonsServer "github.com/LerianStudio/lib-commons/v3/commons/server"
	tmcore "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/core"
	tmmiddleware "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/middleware"
	_ "github.com/LerianStudio/midaz/v3/components/ledger/api"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	fiberSwagger "github.com/swaggo/fiber-swagger"
	"go.opentelemetry.io/otel/trace"
)

// RouteRegistrar is a function that registers routes to an existing Fiber app.
// Each module (onboarding, transaction) implements this to register its routes.
type RouteRegistrar func(app *fiber.App)

// transactionPaths defines URL path segments that belong to the transaction module.
var transactionPaths = []string{
	"/transactions",
	"/operations",
	"/balances",
	"/asset-rates",
	"/operation-routes",
	"/transaction-routes",
}

// publicPaths defines endpoints that bypass tenant middleware.
var publicPaths = []string{"/health", "/version", "/swagger"}

// UnifiedServer consolidates all HTTP APIs (onboarding + transaction) in a single Fiber server.
// This enables the unified ledger mode where all routes are accessible on a single port.
type UnifiedServer struct {
	app           *fiber.App
	serverAddress string
	logger        libLog.Logger
	telemetry     *libOpentelemetry.Telemetry
}

// handleUnifiedServerError is a custom error handler that extends the default Fiber error handling
// with multi-tenant specific error detection. It checks for unprovisioned tenant databases
// (PostgreSQL SQLSTATE 42P01) and returns a meaningful TENANT_NOT_PROVISIONED error.
func handleUnifiedServerError(c *fiber.Ctx, err error) error {
	// Safely end spans if user context exists
	ctx := c.UserContext()
	if ctx != nil {
		trace.SpanFromContext(ctx).End()
	}

	// Check for unprovisioned tenant database error (SQLSTATE 42P01)
	// This occurs when the tenant database schema has not been initialized
	if tmcore.IsTenantNotProvisionedError(err) {
		log.Printf("Tenant database not provisioned on %s %s: %v", c.Method(), c.Path(), err)

		return libHTTP.UnprocessableEntity(c, "TENANT_NOT_PROVISIONED", "Unprocessable Entity", "The tenant database schema has not been initialized. Please contact support.")
	}

	// Default error handling using Midaz standard format
	code := fiber.StatusInternalServerError

	var e *fiber.Error
	if errors.As(err, &e) {
		code = e.Code
	}

	if code == fiber.StatusInternalServerError {
		log.Printf("handler error on %s %s: %v", c.Method(), c.Path(), err)

		return c.Status(code).JSON(libCommons.Response{
			Code:    "INTERNAL_SERVER_ERROR",
			Title:   "Internal Server Error",
			Message: "The server encountered an unexpected error. Please try again later or contact support.",
		})
	}

	return c.Status(code).JSON(libCommons.Response{
		Code:    http.StatusText(code),
		Title:   http.StatusText(code),
		Message: err.Error(),
	})
}

// NewUnifiedServer creates a server that exposes all APIs on a single port.
// It accepts route registration functions from each module to compose all routes
// in one Fiber app.
// multiPoolMid is optional - when provided, enables multi-tenant database routing
// with path-based pool selection (onboarding vs transaction) using lib-commons
// MultiPoolMiddleware.
func NewUnifiedServer(
	serverAddress string,
	logger libLog.Logger,
	telemetry *libOpentelemetry.Telemetry,
	multiPoolMid *tmmiddleware.MultiPoolMiddleware,
	routeRegistrars ...RouteRegistrar,
) *UnifiedServer {
	app := fiber.New(fiber.Config{
		AppName:               "Midaz Unified Ledger API",
		DisableStartupMessage: true,
		ErrorHandler:          handleUnifiedServerError,
	})

	// Add common middleware (only once for all routes)
	tlMid := libHTTP.NewTelemetryMiddleware(telemetry)
	app.Use(tlMid.WithTelemetry(telemetry))
	app.Use(cors.New())
	app.Use(libHTTP.WithHTTPLogging(libHTTP.WithCustomLogger(logger)))

	// Add multi-tenant middleware with multi-pool support from lib-commons.
	// The middleware extracts tenant ID from JWT and routes to the appropriate pool
	// based on the request path (onboarding vs transaction).
	if multiPoolMid != nil && multiPoolMid.Enabled() {
		app.Use(multiPoolMid.WithTenantDB)
		logger.Info("Multi-tenant middleware enabled with multi-pool routing")
	} else {
		logger.Info("Running in SINGLE-TENANT mode - multi-tenant support disabled")
	}

	// Health check for the unified server
	app.Get("/health", libHTTP.Ping)

	// Version endpoint
	app.Get("/version", libHTTP.Version)

	// Swagger documentation (unified onboarding + transaction)
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

	return &UnifiedServer{
		app:           app,
		serverAddress: serverAddress,
		logger:        logger,
		telemetry:     telemetry,
	}
}

// Run implements mbootstrap.Runnable interface.
// Starts the unified HTTP server with graceful shutdown support.
func (s *UnifiedServer) Run(l *libCommons.Launcher) error {
	s.logger.Infof("Starting Unified HTTP Server on %s", s.serverAddress)

	libCommonsServer.NewServerManager(nil, s.telemetry, s.logger).
		WithHTTPServer(s.app, s.serverAddress).
		StartWithGracefulShutdown()

	return nil
}

// ServerAddress returns the server address for logging/debugging purposes.
func (s *UnifiedServer) ServerAddress() string {
	return s.serverAddress
}

// isTransactionPath checks if the path belongs to the transaction module.
// This is used by tests and internal routing logic.
func isTransactionPath(path string) bool {
	for _, tp := range transactionPaths {
		if strings.Contains(path, tp) {
			return true
		}
	}

	return false
}
