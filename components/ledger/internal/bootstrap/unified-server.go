package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	libLog "github.com/LerianStudio/lib-commons/v3/commons/log"
	libHTTP "github.com/LerianStudio/lib-commons/v3/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v3/commons/opentelemetry"
	libCommonsServer "github.com/LerianStudio/lib-commons/v3/commons/server"
	tenantmanager "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager"
	"github.com/bxcodec/dbresolver/v2"
	_ "github.com/LerianStudio/midaz/v3/components/ledger/api"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/LerianStudio/midaz/v3/pkg/mbootstrap"
	midazHTTP "github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	jwt "github.com/golang-jwt/jwt/v5"
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

// DualPoolMiddleware provides path-based routing to the correct tenant connection pool.
// It selects the appropriate pool (onboarding or transaction) based on the request path.
// Supports both PostgreSQL (TenantConnectionPool) and MongoDB (MongoPool) connections.
// Optionally triggers on-demand consumer activation for multi-tenant lazy mode.
type DualPoolMiddleware struct {
	onboardingPool       *tenantmanager.TenantConnectionManager
	transactionPool      *tenantmanager.TenantConnectionManager
	onboardingMongoPool  *tenantmanager.MongoManager
	transactionMongoPool *tenantmanager.MongoManager
	consumerTrigger      mbootstrap.ConsumerTrigger
	logger               libLog.Logger
}

// NewDualPoolMiddleware creates a middleware that routes requests to the appropriate pool.
// Supports both PostgreSQL and MongoDB pools for multi-tenant database routing.
// The consumerTrigger parameter is optional (nil in single-tenant mode). When provided,
// it ensures the RabbitMQ consumer is active for the tenant on each request (lazy mode trigger).
func NewDualPoolMiddleware(pools *MultiTenantPools, consumerTrigger mbootstrap.ConsumerTrigger, logger libLog.Logger) *DualPoolMiddleware {
	return &DualPoolMiddleware{
		onboardingPool:       pools.OnboardingPool,
		transactionPool:      pools.TransactionPool,
		onboardingMongoPool:  pools.OnboardingMongoPool,
		transactionMongoPool: pools.TransactionMongoPool,
		consumerTrigger:      consumerTrigger,
		logger:               logger,
	}
}

// WithTenantDB is a Fiber middleware that extracts tenant ID from JWT,
// determines the correct pool based on the request path, and injects
// the tenant-specific database connection into the request context.
func (m *DualPoolMiddleware) WithTenantDB(c *fiber.Ctx) error {
	path := c.Path()

	if m.isPublicPath(path) {
		return c.Next()
	}

	pool := m.selectPool(path)
	if pool == nil || !pool.IsMultiTenant() {
		return c.Next()
	}

	ctx := libOpentelemetry.ExtractHTTPContext(c)
	logger, tracer, _, _ := libCommons.NewTrackingFromContext(ctx)

	ctx, span := tracer.Start(ctx, "middleware.dual_pool.with_tenant_db")
	defer span.End()

	// Extract tenant ID from JWT
	tenantID, err := m.extractTenantIDFromToken(c)
	if err != nil {
		logger.Errorf("Failed to extract tenant ID: %v", err)
		libOpentelemetry.HandleSpanBusinessErrorEvent(&span, "Failed to extract tenant ID", err)

		return midazHTTP.WithError(c, pkg.UnauthorizedError{
			Code:    constant.ErrInvalidToken.Error(),
			Title:   "Unauthorized",
			Message: "tenantId claim is required in JWT token for multi-tenant mode",
		})
	}

	ctx = tenantmanager.ContextWithTenantID(ctx, tenantID)

	// Lazy mode: ensure RabbitMQ consumer is active for this tenant
	if m.consumerTrigger != nil {
		m.consumerTrigger.EnsureConsumerStarted(ctx, tenantID)
	}

	// Resolve tenant-specific PostgreSQL connection
	conn, err := pool.GetConnection(ctx, tenantID)
	if err != nil {
		logger.Errorf("Failed to get PostgreSQL connection for tenant %s: %v", tenantID, err)
		libOpentelemetry.HandleSpanError(&span, "Failed to get tenant connection", err)

		return m.mapConnectionError(c, err, tenantID)
	}

	db, err := conn.GetDB()
	if err != nil {
		logger.Errorf("Failed to get DB interface for tenant %s: %v", tenantID, err)
		libOpentelemetry.HandleSpanError(&span, "Failed to get DB interface", err)

		return midazHTTP.WithError(c, pkg.ServiceUnavailableError{
			Code:    constant.ErrGRPCServiceUnavailable.Error(),
			Title:   "Service Unavailable",
			Message: "database interface unavailable for tenant",
		})
	}

	// Inject module-specific PostgreSQL connections (primary + cross-module for in-process calls)
	ctx = m.injectModuleConnections(ctx, path, tenantID, db, logger)

	// Inject MongoDB connection if pool is configured
	mongoPool := m.selectMongoPool(path)
	if mongoPool != nil {
		var mongoErr error

		ctx, mongoErr = m.injectMongoConnection(c, ctx, mongoPool, tenantID, &span, logger)
		if mongoErr != nil {
			return mongoErr
		}
	}

	c.SetUserContext(ctx)
	logger.Infof("Tenant context resolved: tenant=%s pool=%s", tenantID, m.getPoolName(path))

	return c.Next()
}

// mapConnectionError translates tenant connection errors into appropriate HTTP responses.
func (m *DualPoolMiddleware) mapConnectionError(c *fiber.Ctx, err error, tenantID string) error {
	if errors.Is(err, tenantmanager.ErrTenantNotFound) {
		return midazHTTP.WithError(c, pkg.EntityNotFoundError{
			Code:    constant.ErrEntityNotFound.Error(),
			Title:   "Not Found",
			Message: "tenant not found",
		})
	}

	if errors.Is(err, tenantmanager.ErrManagerClosed) {
		return midazHTTP.WithError(c, pkg.ServiceUnavailableError{
			Code:    constant.ErrGRPCServiceUnavailable.Error(),
			Title:   "Service Unavailable",
			Message: "service temporarily unavailable",
		})
	}

	if errors.Is(err, tenantmanager.ErrServiceNotConfigured) {
		return midazHTTP.WithError(c, pkg.ServiceUnavailableError{
			Code:    constant.ErrGRPCServiceUnavailable.Error(),
			Title:   "Service Unavailable",
			Message: "database service not configured for tenant",
		})
	}

	var suspErr *tenantmanager.TenantSuspendedError
	if errors.As(err, &suspErr) {
		return midazHTTP.WithError(c, pkg.ForbiddenError{
			Code:    constant.ErrServiceSuspended.Error(),
			Title:   "Service Suspended",
			Message: fmt.Sprintf("tenant service is %s", suspErr.Status),
		})
	}

	errMsg := err.Error()

	if strings.Contains(errMsg, "schema mode requires") {
		return midazHTTP.WithError(c, pkg.UnprocessableOperationError{
			Code:    constant.ErrGRPCServiceUnavailable.Error(),
			Title:   "Unprocessable Entity",
			Message: "invalid schema configuration for tenant database",
		})
	}

	if strings.Contains(errMsg, "failed to connect") {
		return midazHTTP.WithError(c, pkg.ServiceUnavailableError{
			Code:    constant.ErrGRPCServiceUnavailable.Error(),
			Title:   "Service Unavailable",
			Message: "database connection unavailable for tenant",
		})
	}

	return midazHTTP.WithError(c, pkg.ServiceUnavailableError{
		Code:    constant.ErrGRPCServiceUnavailable.Error(),
		Title:   "Service Unavailable",
		Message: "failed to establish database connection for tenant",
	})
}

// injectModuleConnections sets both the primary and cross-module PostgreSQL connections in context.
// In unified ledger mode, both onboarding and transaction connections must be available
// for in-process calls between modules.
func (m *DualPoolMiddleware) injectModuleConnections(ctx context.Context, path, tenantID string, primaryDB dbresolver.DB, logger libLog.Logger) context.Context {
	if m.isTransactionPath(path) {
		ctx = tenantmanager.ContextWithModulePGConnection(ctx, constant.ModuleTransaction, primaryDB)
		ctx = m.injectCrossModuleConnection(ctx, m.onboardingPool, constant.ModuleOnboarding, tenantID, logger)
	} else {
		ctx = tenantmanager.ContextWithModulePGConnection(ctx, constant.ModuleOnboarding, primaryDB)
		ctx = m.injectCrossModuleConnection(ctx, m.transactionPool, constant.ModuleTransaction, tenantID, logger)
	}

	return ctx
}

// injectCrossModuleConnection resolves and injects a secondary module's connection for in-process calls.
func (m *DualPoolMiddleware) injectCrossModuleConnection(ctx context.Context, pool *tenantmanager.TenantConnectionManager, module, tenantID string, logger libLog.Logger) context.Context {
	if pool == nil || !pool.IsMultiTenant() {
		return ctx
	}

	conn, err := pool.GetConnection(ctx, tenantID)
	if err != nil {
		logger.Debugf("Could not resolve cross-module connection for %s tenant %s: %v", module, tenantID, err)
		return ctx
	}

	db, err := conn.GetDB()
	if err != nil || db == nil {
		logger.Debugf("Could not get DB interface for cross-module %s tenant %s: %v", module, tenantID, err)
		return ctx
	}

	return tenantmanager.ContextWithModulePGConnection(ctx, module, db)
}

// injectMongoConnection resolves and injects the tenant-specific MongoDB connection into context.
func (m *DualPoolMiddleware) injectMongoConnection(c *fiber.Ctx, ctx context.Context, mongoPool *tenantmanager.MongoManager, tenantID string, span *trace.Span, logger libLog.Logger) (context.Context, error) {
	mongoDB, err := mongoPool.GetDatabaseForTenant(ctx, tenantID)
	if err != nil {
		logger.Errorf("Failed to get MongoDB connection for tenant %s: %v", tenantID, err)
		libOpentelemetry.HandleSpanError(span, "Failed to get tenant MongoDB connection", err)

		var suspErr *tenantmanager.TenantSuspendedError
		if errors.As(err, &suspErr) {
			return ctx, midazHTTP.WithError(c, pkg.ForbiddenError{
				Code:    constant.ErrServiceSuspended.Error(),
				Title:   "Service Suspended",
				Message: fmt.Sprintf("tenant service is %s", suspErr.Status),
			})
		}

		return ctx, midazHTTP.WithError(c, pkg.ServiceUnavailableError{
			Code:    constant.ErrGRPCServiceUnavailable.Error(),
			Title:   "Service Unavailable",
			Message: "MongoDB connection unavailable for tenant",
		})
	}

	return tenantmanager.ContextWithTenantMongo(ctx, mongoDB), nil
}

// selectPool determines which PostgreSQL pool to use based on the request path.
// Transaction paths are defined by the package-level transactionPaths var; all others use the onboarding pool.
func (m *DualPoolMiddleware) selectPool(path string) *tenantmanager.TenantConnectionManager {
	if m.isTransactionPath(path) {
		return m.transactionPool
	}
	// Default to onboarding pool for all other paths
	return m.onboardingPool
}

// selectMongoPool determines which MongoDB pool to use based on the request path.
// Mirrors selectPool routing logic; returns nil if no MongoDB pool is configured.
func (m *DualPoolMiddleware) selectMongoPool(path string) *tenantmanager.MongoManager {
	if m.isTransactionPath(path) {
		return m.transactionMongoPool
	}
	// Default to onboarding MongoDB pool for all other paths
	return m.onboardingMongoPool
}

// isTransactionPath checks if the path belongs to the transaction module.
func (m *DualPoolMiddleware) isTransactionPath(path string) bool {
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
	if tenantmanager.IsTenantNotProvisionedError(err) {
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
// consumerTrigger is optional - when provided, triggers on-demand RabbitMQ consumer
// activation in the tenant middleware (lazy mode).
func NewUnifiedServer(
	serverAddress string,
	logger libLog.Logger,
	telemetry *libOpentelemetry.Telemetry,
	tenantPools *MultiTenantPools,
	consumerTrigger mbootstrap.ConsumerTrigger,
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
		dualPoolMid := NewDualPoolMiddleware(tenantPools, consumerTrigger, logger)
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

// GetTenantConnection retrieves the tenant PostgreSQL connection from the context.
// Delegates to tenantmanager.GetTenantPGConnectionFromContext.
func GetTenantConnection(ctx context.Context) interface{} {
	return tenantmanager.GetTenantPGConnectionFromContext(ctx)
}

// GetTenantID retrieves the tenant ID from the context.
// Deprecated: Use tenantmanager.GetTenantIDFromContext() directly instead.
// Returns empty string if no tenant ID is set.
func GetTenantID(ctx context.Context) string {
	return tenantmanager.GetTenantIDFromContext(ctx)
}
