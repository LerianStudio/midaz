package bootstrap

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	libLog "github.com/LerianStudio/lib-commons/v3/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v3/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/v3/commons/postgres"
	tmclient "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/client"
	tmcore "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/core"
	tmmiddleware "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/middleware"
	tmmongo "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/mongo"
	tmpg "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/postgres"
	libZap "github.com/LerianStudio/lib-commons/v3/commons/zap"
	httpin "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v3/components/onboarding"
	"github.com/LerianStudio/midaz/v3/components/transaction"
	"github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	midazHTTP "github.com/LerianStudio/midaz/v3/pkg/net/http"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

const ApplicationName = "ledger"

// Config is the top level configuration struct for the unified ledger component.
type Config struct {
	EnvName  string `env:"ENV_NAME"`
	LogLevel string `env:"LOG_LEVEL"`
	Version  string `env:"VERSION"`

	// Server configuration - unified port for all APIs
	ServerAddress string `env:"SERVER_ADDRESS" envDefault:":3002"`

	// OpenTelemetry configuration
	OtelServiceName         string `env:"OTEL_RESOURCE_SERVICE_NAME"`
	OtelLibraryName         string `env:"OTEL_LIBRARY_NAME"`
	OtelServiceVersion      string `env:"OTEL_RESOURCE_SERVICE_VERSION"`
	OtelDeploymentEnv       string `env:"OTEL_RESOURCE_DEPLOYMENT_ENVIRONMENT"`
	OtelColExporterEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	EnableTelemetry         bool   `env:"ENABLE_TELEMETRY"`

	// Auth configuration
	AuthEnabled bool   `env:"PLUGIN_AUTH_ENABLED"`
	AuthHost    string `env:"PLUGIN_AUTH_HOST"`

	// Multi-Tenant Configuration
	// When enabled, the middleware extracts tenant ID from JWT and resolves
	// tenant-specific database connections from the Tenant Manager
	MultiTenantEnabled                  bool   `env:"MULTI_TENANT_ENABLED" default:"false"`
	MultiTenantURL                      string `env:"MULTI_TENANT_URL"`
	MultiTenantEnvironment              string `env:"MULTI_TENANT_ENVIRONMENT" default:"staging"`
	MultiTenantMaxTenantPools           int    `env:"MULTI_TENANT_MAX_TENANT_POOLS" default:"100"`
	MultiTenantIdleTimeoutSec           int    `env:"MULTI_TENANT_IDLE_TIMEOUT_SEC" default:"300"`
	MultiTenantCircuitBreakerThreshold  int    `env:"MULTI_TENANT_CIRCUIT_BREAKER_THRESHOLD" default:"5"`
	MultiTenantCircuitBreakerTimeoutSec int    `env:"MULTI_TENANT_CIRCUIT_BREAKER_TIMEOUT_SEC" default:"30"`

	// PostgreSQL Primary - for multi-tenant default connection
	PrimaryDBHost     string `env:"DB_HOST"`
	PrimaryDBUser     string `env:"DB_USER"`
	PrimaryDBPassword string `env:"DB_PASSWORD"`
	PrimaryDBName     string `env:"DB_NAME"`
	PrimaryDBPort     string `env:"DB_PORT"`
	PrimaryDBSSLMode  string `env:"DB_SSLMODE"`

	// PostgreSQL Replica - for multi-tenant default connection
	ReplicaDBHost     string `env:"DB_REPLICA_HOST"`
	ReplicaDBUser     string `env:"DB_REPLICA_USER"`
	ReplicaDBPassword string `env:"DB_REPLICA_PASSWORD"`
	ReplicaDBName     string `env:"DB_REPLICA_NAME"`
	ReplicaDBPort     string `env:"DB_REPLICA_PORT"`
	ReplicaDBSSLMode  string `env:"DB_REPLICA_SSLMODE"`

	// MongoDB - for multi-tenant default connection
	MongoURI          string `env:"MONGO_URI"`
	MongoDBHost       string `env:"MONGO_HOST"`
	MongoDBName       string `env:"MONGO_NAME"`
	MongoDBUser       string `env:"MONGO_USER"`
	MongoDBPassword   string `env:"MONGO_PASSWORD"`
	MongoDBPort       string `env:"MONGO_PORT"`
	MongoDBParameters string `env:"MONGO_PARAMETERS"`
	MaxPoolSize       int    `env:"MONGO_MAX_POOL_SIZE"`
}

// Options contains optional dependencies that can be injected by callers.
type Options struct {
	// Logger allows callers to provide a pre-configured logger, avoiding multiple
	// initializations when composing components (e.g. unified ledger).
	Logger libLog.Logger
}

// InitServers initializes the unified ledger service that composes
// both onboarding and transaction modules in a single process.
// The transaction module is initialized first so its BalancePort (the UseCase)
// can be passed directly to onboarding for in-process calls.
func InitServers() (*Service, error) {
	return InitServersWithOptions(nil)
}

// InitServersWithOptions initializes the unified ledger service with optional dependency injection.
func InitServersWithOptions(opts *Options) (*Service, error) {
	cfg := &Config{}

	if err := libCommons.SetConfigFromEnvVars(cfg); err != nil {
		return nil, fmt.Errorf("failed to load config from environment variables: %w", err)
	}

	var baseLogger libLog.Logger
	if opts != nil && opts.Logger != nil {
		baseLogger = opts.Logger
	} else {
		var err error

		baseLogger, err = libZap.InitializeLoggerWithError()
		if err != nil {
			return nil, fmt.Errorf("failed to initialize logger: %w", err)
		}
	}

	telemetry, err := libOpentelemetry.InitializeTelemetryWithError(&libOpentelemetry.TelemetryConfig{
		LibraryName:               cfg.OtelLibraryName,
		ServiceName:               cfg.OtelServiceName,
		ServiceVersion:            cfg.OtelServiceVersion,
		DeploymentEnv:             cfg.OtelDeploymentEnv,
		CollectorExporterEndpoint: cfg.OtelColExporterEndpoint,
		EnableTelemetry:           cfg.EnableTelemetry,
		Logger:                    baseLogger,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize telemetry: %w", err)
	}

	// Generate startup ID for tracing initialization issues
	startupID := uuid.New().String()

	ledgerLogger := baseLogger.WithFields(
		"component", "ledger",
		"startup_id", startupID,
	)
	transactionLogger := baseLogger.WithFields(
		"component", "transaction",
		"startup_id", startupID,
	)
	onboardingLogger := baseLogger.WithFields(
		"component", "onboarding",
		"startup_id", startupID,
	)

	ledgerLogger.WithFields(
		"version", cfg.Version,
		"env", cfg.EnvName,
	).Info("Starting unified ledger component")

	ledgerLogger.Info("Initializing transaction module...")

	// Initialize transaction module first to get the BalancePort
	// Pass ApplicationName as ServiceName so RabbitMQ pool is registered under "ledger"
	transactionService, err := transaction.InitServiceWithOptionsOrError(&transaction.Options{
		Logger:      transactionLogger,
		ServiceName: ApplicationName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize transaction module: %w", err)
	}

	// Get the BalancePort from transaction for in-process communication
	// This is the transaction.UseCase itself which implements BalancePort directly
	balancePort := transactionService.GetBalancePort()

	// Get the metadata port from transaction for metadata index operations
	transactionMetadataRepo := transactionService.GetMetadataIndexPort()
	if transactionMetadataRepo == nil {
		return nil, fmt.Errorf("failed to get MetadataIndexPort from transaction module")
	}

	ledgerLogger.Info("Transaction module initialized, BalancePort and MetadataIndexPort available for in-process calls")

	ledgerLogger.Info("Initializing onboarding module in UNIFIED MODE...")

	// Initialize onboarding module in unified mode with the BalancePort for direct calls
	// No intermediate adapter needed - the transaction.UseCase is passed directly
	onboardingService, err := onboarding.InitServiceWithOptionsOrError(&onboarding.Options{
		Logger:      onboardingLogger,
		UnifiedMode: true,
		BalancePort: balancePort,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize onboarding module: %w", err)
	}

	ledgerLogger.Info("Onboarding module initialized")

	// Get the metadata port from onboarding for metadata index operations
	onboardingMetadataRepo := onboardingService.GetMetadataIndexPort()
	if onboardingMetadataRepo == nil {
		return nil, fmt.Errorf("failed to get MetadataIndexPort from onboarding module")
	}

	ledgerLogger.Info("Both metadata index repositories available for settings routes")

	// Create auth client for metadata index routes
	auth := middleware.NewAuthClient(cfg.AuthHost, cfg.AuthEnabled, &ledgerLogger)

	// Create metadata index handler with both repositories
	metadataIndexHandler := &httpin.MetadataIndexHandler{
		OnboardingMetadataRepo:  onboardingMetadataRepo,
		TransactionMetadataRepo: transactionMetadataRepo,
	}

	// Create route registrar for ledger-specific routes (metadata indexes)
	ledgerRouteRegistrar := httpin.CreateRouteRegistrar(auth, metadataIndexHandler)

	// Get consumer trigger for on-demand RabbitMQ consumer activation (lazy mode).
	// In multi-tenant mode, the tenant middleware uses this to ensure the consumer
	// is active when the first HTTP request arrives for a tenant.
	consumerTrigger := transactionService.GetConsumerTrigger()

	if consumerTrigger != nil {
		ledgerLogger.Info("Consumer trigger available - lazy mode consumer activation enabled in tenant middleware")
	}

	// Initialize multi-tenant middleware (optional, disabled by default)
	// Created AFTER transaction init so consumerTrigger is available.
	var multiPoolMid *tmmiddleware.MultiPoolMiddleware

	if cfg.MultiTenantEnabled && cfg.MultiTenantURL != "" {
		multiPoolMid = initMultiTenantMiddleware(cfg, baseLogger, consumerTrigger)
		baseLogger.Infof("Multi-tenant mode enabled with Tenant Manager URL: %s", cfg.MultiTenantURL)
	} else {
		baseLogger.Info("Running in SINGLE-TENANT MODE - multi-tenant disabled or no Tenant Manager URL")
	}

	ledgerLogger.Info("Creating unified HTTP server on " + cfg.ServerAddress)

	// Create the unified server that consolidates all routes on a single port.
	// Pass the MultiPoolMiddleware for path-based routing to the correct database.
	unifiedServer := NewUnifiedServer(
		cfg.ServerAddress,
		ledgerLogger,
		telemetry,
		multiPoolMid,
		onboardingService.GetRouteRegistrar(),
		transactionService.GetRouteRegistrar(),
		ledgerRouteRegistrar,
	)

	ledgerLogger.WithFields(
		"version", cfg.Version,
		"env", cfg.EnvName,
		"server_address", cfg.ServerAddress,
	).Info("Unified ledger component started successfully with single-port mode")

	return &Service{
		OnboardingService:  onboardingService,
		TransactionService: transactionService,
		UnifiedServer:      unifiedServer,
		Logger:             ledgerLogger,
		Telemetry:          telemetry,
	}, nil
}

// initMultiTenantMiddleware creates and configures the lib-commons MultiPoolMiddleware
// with path-based routing for onboarding and transaction modules.
//
// The Tenant Manager returns separate database credentials for each module:
//
//	"databases": {
//	  "ledger": {
//	    "services": {
//	      "onboarding": { "postgresql": {...}, "mongodb": {...} },
//	      "transaction": { "postgresql": {...}, "mongodb": {...} }
//	    }
//	  }
//	}
//
// This architecture enables:
// - Independent scaling of onboarding vs transaction databases
// - Separate connection pools with isolated limits
// - Module-specific failover and routing strategies
// - Support for both PostgreSQL and MongoDB per module
func initMultiTenantMiddleware(cfg *Config, logger libLog.Logger, consumerTrigger tmmiddleware.ConsumerTrigger) *tmmiddleware.MultiPoolMiddleware {
	// Build client options for Tenant Manager
	var clientOpts []tmclient.ClientOption
	if cfg.MultiTenantCircuitBreakerThreshold > 0 {
		clientOpts = append(clientOpts,
			tmclient.WithCircuitBreaker(
				cfg.MultiTenantCircuitBreakerThreshold,
				time.Duration(cfg.MultiTenantCircuitBreakerTimeoutSec)*time.Second,
			),
		)
	}

	// Create Tenant Manager client (shared between all pools)
	tenantManagerClient := tmclient.NewClient(cfg.MultiTenantURL, logger, clientOpts...)

	idleTimeout := time.Duration(cfg.MultiTenantIdleTimeoutSec) * time.Second

	// Create default PostgreSQL connection for fallback (single-tenant mode)
	// Both pools share the same default connection when no tenant-specific config exists
	postgreSourcePrimary := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s",
		cfg.PrimaryDBHost, cfg.PrimaryDBUser, cfg.PrimaryDBPassword,
		cfg.PrimaryDBName, cfg.PrimaryDBPort, cfg.PrimaryDBSSLMode)

	postgreSourceReplica := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s",
		cfg.ReplicaDBHost, cfg.ReplicaDBUser, cfg.ReplicaDBPassword,
		cfg.ReplicaDBName, cfg.ReplicaDBPort, cfg.ReplicaDBSSLMode)

	// Default connection for onboarding module
	onboardingDefaultConn := &libPostgres.PostgresConnection{
		ConnectionStringPrimary: postgreSourcePrimary,
		ConnectionStringReplica: postgreSourceReplica,
		PrimaryDBName:           cfg.PrimaryDBName,
		ReplicaDBName:           cfg.ReplicaDBName,
		Component:               "onboarding",
		Logger:                  logger,
	}

	// Default connection for transaction module
	transactionDefaultConn := &libPostgres.PostgresConnection{
		ConnectionStringPrimary: postgreSourcePrimary,
		ConnectionStringReplica: postgreSourceReplica,
		PrimaryDBName:           cfg.PrimaryDBName,
		ReplicaDBName:           cfg.ReplicaDBName,
		Component:               "transaction",
		Logger:                  logger,
	}

	// Create onboarding PostgreSQL pool
	onboardingPG := tmpg.NewManager(tenantManagerClient, "ledger",
		tmpg.WithModule("onboarding"),
		tmpg.WithLogger(logger),
		tmpg.WithMaxTenantPools(cfg.MultiTenantMaxTenantPools),
		tmpg.WithIdleTimeout(idleTimeout),
	).
		WithDefaultConnection(onboardingDefaultConn)

	logger.Info("Created onboarding PostgreSQL connection manager for multi-tenant mode")

	// Create transaction PostgreSQL pool
	transactionPG := tmpg.NewManager(tenantManagerClient, "ledger",
		tmpg.WithModule("transaction"),
		tmpg.WithLogger(logger),
		tmpg.WithMaxTenantPools(cfg.MultiTenantMaxTenantPools),
		tmpg.WithIdleTimeout(idleTimeout),
	).
		WithDefaultConnection(transactionDefaultConn)

	logger.Info("Created transaction PostgreSQL connection manager for multi-tenant mode")

	// Create MongoDB pools for multi-tenant mode
	onboardingMongo := tmmongo.NewManager(tenantManagerClient, "ledger",
		tmmongo.WithModule("onboarding"),
		tmmongo.WithLogger(logger),
		tmmongo.WithMaxTenantPools(cfg.MultiTenantMaxTenantPools),
		tmmongo.WithIdleTimeout(idleTimeout),
	)
	logger.Info("Created onboarding MongoDB connection manager for multi-tenant mode")

	transactionMongo := tmmongo.NewManager(tenantManagerClient, "ledger",
		tmmongo.WithModule("transaction"),
		tmmongo.WithLogger(logger),
		tmmongo.WithMaxTenantPools(cfg.MultiTenantMaxTenantPools),
		tmmongo.WithIdleTimeout(idleTimeout),
	)
	logger.Info("Created transaction MongoDB connection manager for multi-tenant mode")

	// Build MultiPoolMiddleware options
	opts := []tmmiddleware.MultiPoolOption{
		tmmiddleware.WithRoute(transactionPaths, "transaction", transactionPG, transactionMongo),
		tmmiddleware.WithDefaultRoute("onboarding", onboardingPG, onboardingMongo),
		tmmiddleware.WithPublicPaths(publicPaths...),
		tmmiddleware.WithCrossModuleInjection(),
		tmmiddleware.WithErrorMapper(midazTenantErrorMapper),
		tmmiddleware.WithMultiPoolLogger(logger),
	}

	if consumerTrigger != nil {
		opts = append(opts, tmmiddleware.WithConsumerTrigger(consumerTrigger))
	}

	return tmmiddleware.NewMultiPoolMiddleware(opts...)
}

// midazTenantErrorMapper translates tenant-manager errors into Midaz HTTP error responses.
// This maps lib-commons tenant-manager errors to the Midaz error types and codes,
// preserving the exact error codes and messages from the original DualPoolMiddleware.
//
// The error mapper is called for ALL errors from MultiPoolMiddleware, including
// JWT extraction errors (missing token, parse failures, missing tenantId claim).
func midazTenantErrorMapper(c *fiber.Ctx, err error, tenantID string) error {
	errMsg := err.Error()

	// JWT / authentication errors (called with tenantID="" from MultiPoolMiddleware)
	// These correspond to the extractTenantID errors in multi_pool.go
	if strings.Contains(errMsg, "authorization token") ||
		strings.Contains(errMsg, "parse authorization token") ||
		strings.Contains(errMsg, "JWT claims") ||
		strings.Contains(errMsg, "tenantId is required") {
		return midazHTTP.WithError(c, pkg.UnauthorizedError{
			Code:    constant.ErrInvalidToken.Error(),
			Title:   "Unauthorized",
			Message: "tenantId claim is required in JWT token for multi-tenant mode",
		})
	}

	if errors.Is(err, tmcore.ErrTenantNotFound) {
		return midazHTTP.WithError(c, pkg.EntityNotFoundError{
			Code:    constant.ErrEntityNotFound.Error(),
			Title:   "Not Found",
			Message: "tenant not found",
		})
	}

	if errors.Is(err, tmcore.ErrManagerClosed) {
		return midazHTTP.WithError(c, pkg.ServiceUnavailableError{
			Code:    constant.ErrGRPCServiceUnavailable.Error(),
			Title:   "Service Unavailable",
			Message: "service temporarily unavailable",
		})
	}

	if errors.Is(err, tmcore.ErrServiceNotConfigured) {
		return midazHTTP.WithError(c, pkg.ServiceUnavailableError{
			Code:    constant.ErrGRPCServiceUnavailable.Error(),
			Title:   "Service Unavailable",
			Message: "database service not configured for tenant",
		})
	}

	var suspErr *tmcore.TenantSuspendedError
	if errors.As(err, &suspErr) {
		return midazHTTP.WithError(c, pkg.ForbiddenError{
			Code:    constant.ErrServiceSuspended.Error(),
			Title:   "Service Suspended",
			Message: fmt.Sprintf("tenant service is %s", suspErr.Status),
		})
	}

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
