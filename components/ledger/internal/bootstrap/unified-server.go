package bootstrap

import (
	"time"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libHTTP "github.com/LerianStudio/lib-commons/v2/commons/net/http"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libCommonsServer "github.com/LerianStudio/lib-commons/v2/commons/server"
	_ "github.com/LerianStudio/midaz/v3/components/ledger/api"
	httpin "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v3/pkg/mmodel"
	pkghttp "github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/redis/go-redis/v9"
	fiberSwagger "github.com/swaggo/fiber-swagger"
)

// RouteRegistrar is a function that registers routes to an existing Fiber app.
// Each module (onboarding, transaction) implements this to register its routes.
type RouteRegistrar func(app *fiber.App)

// UnifiedServer consolidates all HTTP APIs (onboarding + transaction) in a single Fiber server.
// This enables the unified ledger mode where all routes are accessible on a single port.
type UnifiedServer struct {
	app           *fiber.App
	serverAddress string
	logger        libLog.Logger
	telemetry     *libOpentelemetry.Telemetry
	redisClient   *redis.Client
}

// UnifiedServerOptions contains optional dependencies for the unified server.
type UnifiedServerOptions struct {
	// RedisClient for rate limiting (optional)
	RedisClient *redis.Client
	// AuthClient for authorization (optional)
	AuthClient *middleware.AuthClient
}

// NewUnifiedServer creates a server that exposes all APIs on a single port.
// It accepts route registration functions from each module to compose all routes
// in one Fiber app.
func NewUnifiedServer(
	serverAddress string,
	logger libLog.Logger,
	telemetry *libOpentelemetry.Telemetry,
	routeRegistrars ...RouteRegistrar,
) *UnifiedServer {
	return NewUnifiedServerWithOptions(serverAddress, logger, telemetry, nil, routeRegistrars...)
}

// NewUnifiedServerWithOptions creates a server with additional options like Redis for rate limiting.
func NewUnifiedServerWithOptions(
	serverAddress string,
	logger libLog.Logger,
	telemetry *libOpentelemetry.Telemetry,
	opts *UnifiedServerOptions,
	routeRegistrars ...RouteRegistrar,
) *UnifiedServer {
	app := fiber.New(fiber.Config{
		AppName:               "Midaz Unified Ledger API",
		DisableStartupMessage: true,
		ErrorHandler: func(ctx *fiber.Ctx, err error) error {
			return libHTTP.HandleFiberError(ctx, err)
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

	// Register batch endpoint
	registerBatchEndpoint(app, logger, opts)

	// End tracing spans middleware (must be last)
	app.Use(tlMid.EndTracingSpans)

	var redisClient *redis.Client
	if opts != nil {
		redisClient = opts.RedisClient
	}

	return &UnifiedServer{
		app:           app,
		serverAddress: serverAddress,
		logger:        logger,
		telemetry:     telemetry,
		redisClient:   redisClient,
	}
}

// registerBatchEndpoint registers the batch endpoint with optional rate limiting.
func registerBatchEndpoint(app *fiber.App, logger libLog.Logger, opts *UnifiedServerOptions) {
	var batchHandler *httpin.BatchHandler
	var err error

	// Create batch handler with Redis if available (for idempotency support)
	if opts != nil && opts.RedisClient != nil {
		batchHandler, err = httpin.NewBatchHandlerWithRedis(app, opts.RedisClient)
		logger.Info("Batch handler created with Redis support for idempotency")
	} else {
		batchHandler, err = httpin.NewBatchHandler(app)
		logger.Info("Batch handler created without Redis (idempotency disabled)")
	}

	if err != nil {
		logger.Errorf("Failed to create batch handler: %v", err)

		return
	}

	// Build middleware chain for batch endpoint
	middlewares := make([]fiber.Handler, 0)

	// Add authorization if auth client is available
	if opts != nil && opts.AuthClient != nil {
		middlewares = append(middlewares, opts.AuthClient.Authorize("midaz", "batch", "post"))
	}

	// Add rate limiting if enabled (fail-closed when Redis is unavailable)
	if pkghttp.RateLimitEnabled() {
		var redisClient *redis.Client
		if opts != nil {
			redisClient = opts.RedisClient
		}
		if redisClient == nil {
			logger.Info("Rate limiting enabled but Redis client not configured; batch endpoint will respond 503")
		} else {
			logger.Info("Batch rate limiting enabled")
		}

		batchRateLimiter := pkghttp.NewBatchRateLimiter(pkghttp.BatchRateLimiterConfig{
			MaxItemsPerWindow: pkghttp.GetRateLimitMaxBatchItems(),
			Expiration:        time.Minute,
			RedisClient:       redisClient,
			MaxBatchSize:      pkghttp.GetRateLimitMaxBatchSize(),
		})
		middlewares = append(middlewares, batchRateLimiter)
	}

	// Add body parser middleware
	middlewares = append(middlewares, pkghttp.WithBody(new(mmodel.BatchRequest), batchHandler.ProcessBatch))

	// Register the batch endpoint with all middlewares
	app.Post("/v1/batch", middlewares...)

	logger.Info("Batch endpoint registered at POST /v1/batch")
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
