package bootstrap

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	poolmanager "github.com/LerianStudio/lib-commons/v2/commons/pool-manager"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	libCommonsServer "github.com/LerianStudio/lib-commons/v2/commons/server"
	_ "github.com/LerianStudio/midaz/v3/components/ledger/api"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	jwt "github.com/golang-jwt/jwt/v5"
	fiberSwagger "github.com/swaggo/fiber-swagger"
	"go.opentelemetry.io/otel/trace"
)

// RouteRegistrar is a function that registers routes to an existing Fiber app.
// Each module (onboarding, transaction) implements this to register its routes.
type RouteRegistrar func(app *fiber.App)

// TenantContextKey is the context key for storing tenant-specific database connection.
type TenantContextKey string

const (
	// TenantDBConnectionKey is the key for storing tenant database connection in context.
	TenantDBConnectionKey TenantContextKey = "tenant_db_connection"
	// TenantIDKey is the key for storing tenant ID in context.
	TenantIDKey TenantContextKey = "tenant_id"
)

// UnifiedServer consolidates all HTTP APIs (onboarding + transaction) in a single Fiber server.
// This enables the unified ledger mode where all routes are accessible on a single port.
type UnifiedServer struct {
	app           *fiber.App
	serverAddress string
	logger        libLog.Logger
	telemetry     *libOpentelemetry.Telemetry
}

// DualPoolMiddleware provides path-based routing to the correct tenant connection pool.
// It selects the appropriate pool (onboarding or transaction) based on the request path.
type DualPoolMiddleware struct {
	onboardingPool  *poolmanager.TenantConnectionPool
	transactionPool *poolmanager.TenantConnectionPool
	logger          libLog.Logger
}

// NewDualPoolMiddleware creates a middleware that routes requests to the appropriate pool.
func NewDualPoolMiddleware(pools *MultiTenantPools, logger libLog.Logger) *DualPoolMiddleware {
	return &DualPoolMiddleware{
		onboardingPool:  pools.OnboardingPool,
		transactionPool: pools.TransactionPool,
		logger:          logger,
	}
}

// WithTenantDB is a Fiber middleware that extracts tenant ID from JWT,
// determines the correct pool based on the request path, and injects
// the tenant-specific database connection into the request context.
func (m *DualPoolMiddleware) WithTenantDB(c *fiber.Ctx) error {
	// Skip public endpoints that don't require tenant context
	if m.isPublicPath(c.Path()) {
		return c.Next()
	}

	// Select the appropriate pool based on the request path
	pool := m.selectPool(c.Path())
	if pool == nil {
		m.logger.Warn("No pool available for path, passing through")
		return c.Next()
	}

	// Single-tenant mode: pass through if pool is not multi-tenant
	if !pool.IsMultiTenant() {
		return c.Next()
	}

	ctx := libOpentelemetry.ExtractHTTPContext(c)
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "middleware.dual_pool.with_tenant_db")
	defer span.End()

	// Extract tenant ID from JWT token
	// In multi-tenant mode, tenantId is REQUIRED - no fallback to default connection
	tenantID, err := m.extractTenantIDFromToken(c)
	if err != nil {
		logger.Errorf("Failed to extract tenant ID from token: %v", err)
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to extract tenant ID", err)
		return libHTTP.Unauthorized(c, "TENANT_ID_REQUIRED", "Unauthorized", "tenantId claim is required in JWT token for multi-tenant mode")
	}

	logger.Infof("Extracted tenant ID: %s for path: %s", tenantID, c.Path())

	// Store tenant ID in context
	ctx = context.WithValue(ctx, TenantIDKey, tenantID)

	// Get tenant-specific connection from the selected pool
	conn, err := pool.GetConnection(ctx, tenantID)
	if err != nil {
		logger.Errorf("Failed to get connection for tenant %s: %v", tenantID, err)
		libOpentelemetry.HandleSpanError(&span, "Failed to get tenant connection", err)

		// Check for specific errors
		if errors.Is(err, poolmanager.ErrTenantNotFound) {
			return libHTTP.NotFound(c, "TENANT_NOT_FOUND", "Not Found", "tenant not found")
		}

		if errors.Is(err, poolmanager.ErrPoolClosed) {
			return libHTTP.JSONResponse(c, http.StatusServiceUnavailable, libCommons.Response{
				Code:    "SERVICE_UNAVAILABLE",
				Title:   "Service Unavailable",
				Message: "Service temporarily unavailable",
			})
		}

		return libHTTP.InternalServerError(c, "CONNECTION_ERROR", "Internal Server Error", "failed to establish database connection")
	}

	// Get the dbresolver.DB interface from the connection
	db, err := conn.GetDB()
	if err != nil {
		logger.Errorf("Failed to get DB interface for tenant %s: %v", tenantID, err)
		libOpentelemetry.HandleSpanError(&span, "Failed to get DB interface", err)

		return libHTTP.InternalServerError(c, "DB_ERROR", "Internal Server Error", "failed to get database interface")
	}

	// Store connection in context using poolmanager's context functions
	// This ensures repositories can find the tenant connection via GetDBForTenantWithFallback
	ctx = poolmanager.ContextWithTenantPGConnection(ctx, db)
	ctx = poolmanager.SetMultiTenantModeInContext(ctx, true)

	// Also store the full connection for cases that need it (like GetTenantConnection helper)
	ctx = context.WithValue(ctx, TenantDBConnectionKey, conn)
	c.SetUserContext(ctx)

	logger.Infof("Set tenant connection for tenant: %s (pool: %s)", tenantID, m.getPoolName(c.Path()))

	return c.Next()
}

// selectPool determines which pool to use based on the request path.
// Onboarding routes: /v1/organizations, /v1/organizations/:org/ledgers,
//
//	/v1/organizations/:org/ledgers/:ledger/accounts,
//	/v1/organizations/:org/ledgers/:ledger/assets,
//	/v1/organizations/:org/ledgers/:ledger/portfolios,
//	/v1/organizations/:org/ledgers/:ledger/segments,
//	/v1/organizations/:org/ledgers/:ledger/account-types
//
// Transaction routes: /v1/organizations/:org/ledgers/:ledger/transactions,
//
//	/v1/organizations/:org/ledgers/:ledger/operations,
//	/v1/organizations/:org/ledgers/:ledger/balances,
//	/v1/organizations/:org/ledgers/:ledger/asset-rates,
//	/v1/organizations/:org/ledgers/:ledger/operation-routes,
//	/v1/organizations/:org/ledgers/:ledger/transaction-routes
func (m *DualPoolMiddleware) selectPool(path string) *poolmanager.TenantConnectionPool {
	if m.isTransactionPath(path) {
		return m.transactionPool
	}
	// Default to onboarding pool for all other paths
	return m.onboardingPool
}

// isTransactionPath checks if the path belongs to transaction module.
func (m *DualPoolMiddleware) isTransactionPath(path string) bool {
	// Transaction module paths (under ledger context)
	transactionPaths := []string{
		"/transactions",
		"/operations",
		"/balances",
		"/asset-rates",
		"/operation-routes",
		"/transaction-routes",
	}

	for _, tp := range transactionPaths {
		if strings.Contains(path, tp) {
			return true
		}
	}

	return false
}

// getPoolName returns the pool name for logging purposes.
func (m *DualPoolMiddleware) getPoolName(path string) string {
	if m.isTransactionPath(path) {
		return "transaction"
	}
	return "onboarding"
}

// isPublicPath checks if the path is a public endpoint that doesn't require tenant context.
func (m *DualPoolMiddleware) isPublicPath(path string) bool {
	publicPaths := []string{
		"/health",
		"/version",
		"/swagger",
	}

	for _, pp := range publicPaths {
		if path == pp || strings.HasPrefix(path, pp) {
			return true
		}
	}

	return false
}

// extractTenantIDFromToken extracts the tenant ID from the JWT token's claims.
// Only accepts the "tenantId" claim (camelCase) - no fallbacks to other claim names.
func (m *DualPoolMiddleware) extractTenantIDFromToken(c *fiber.Ctx) (string, error) {
	accessToken := libHTTP.ExtractTokenFromHeader(c)
	if accessToken == "" {
		return "", errors.New("no authorization token provided")
	}

	// Parse token without validation (validation is done by auth middleware)
	token, _, err := new(jwt.Parser).ParseUnverified(accessToken, jwt.MapClaims{})
	if err != nil {
		return "", err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", errors.New("invalid token claims format")
	}

	// Extract tenantId from claims (camelCase only - no fallbacks)
	tenantID, ok := claims["tenantId"].(string)
	if !ok || tenantID == "" {
		return "", errors.New("tenantId claim not found in token")
	}

	return tenantID, nil
}

// GetTenantConnection retrieves the tenant database connection from the context.
// Returns nil if not in multi-tenant mode or no connection is set.
func GetTenantConnection(ctx context.Context) *libPostgres.PostgresConnection {
	if conn, ok := ctx.Value(TenantDBConnectionKey).(*libPostgres.PostgresConnection); ok {
		return conn
	}
	return nil
}

// GetTenantID retrieves the tenant ID from the context.
// Returns empty string if no tenant ID is set.
func GetTenantID(ctx context.Context) string {
	if tenantID, ok := ctx.Value(TenantIDKey).(string); ok {
		return tenantID
	}
	return ""
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
	if poolmanager.IsTenantNotProvisionedError(err) {
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
// tenantPools is optional - when provided, enables multi-tenant database routing
// with path-based pool selection (onboarding vs transaction).
func NewUnifiedServer(
	serverAddress string,
	logger libLog.Logger,
	telemetry *libOpentelemetry.Telemetry,
	tenantPools *MultiTenantPools,
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

	// Add multi-tenant middleware with dual pool support
	// The middleware extracts tenant ID from JWT and routes to the appropriate pool
	// based on the request path (onboarding vs transaction)
	if tenantPools != nil {
		dualPoolMid := NewDualPoolMiddleware(tenantPools, logger)
		app.Use(dualPoolMid.WithTenantDB)
		logger.Info("Multi-tenant middleware enabled with dual pools (onboarding + transaction)")
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
