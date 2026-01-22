package bootstrap

import (
	"fmt"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	poolmanager "github.com/LerianStudio/lib-commons/v2/commons/pool-manager"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	httpin "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v3/components/onboarding"
	"github.com/LerianStudio/midaz/v3/components/transaction"
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
	// tenant-specific database connections from the Pool Manager
	MultiTenantEnabled bool   `env:"MULTI_TENANT_ENABLED" default:"false"`
	PoolManagerURL     string `env:"POOL_MANAGER_URL"`
	TenantCacheTTL     string `env:"TENANT_CACHE_TTL" default:"24h"`

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

	// PostgreSQL connection pool
	MaxOpenConnections int `env:"DB_MAX_OPEN_CONNS"`
	MaxIdleConnections int `env:"DB_MAX_IDLE_CONNS"`

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

// MultiTenantPools holds the two connection pools for the unified ledger.
// In multi-tenant mode, the Pool Manager returns separate database credentials
// for each module (onboarding and transaction).
type MultiTenantPools struct {
	// OnboardingPool handles connections for onboarding entities:
	// organizations, ledgers, accounts, assets, portfolios, segments, account-types
	OnboardingPool *poolmanager.TenantConnectionPool

	// TransactionPool handles connections for transaction entities:
	// transactions, operations, balances, asset-rates, operation-routes, transaction-routes
	TransactionPool *poolmanager.TenantConnectionPool
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

	// Initialize multi-tenant connection pools (optional, disabled by default)
	// In multi-tenant mode, we create TWO pools: one for onboarding and one for transaction
	var tenantPools *MultiTenantPools

	if cfg.MultiTenantEnabled && cfg.PoolManagerURL != "" {
		tenantPools = initMultiTenantPools(cfg, baseLogger)
		baseLogger.Infof("Multi-tenant mode enabled with Pool Manager URL: %s (dual pools: onboarding + transaction)", cfg.PoolManagerURL)
	} else {
		baseLogger.Info("Running in SINGLE-TENANT MODE - multi-tenant disabled or no Pool Manager URL")
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
	transactionService, err := transaction.InitServiceWithOptionsOrError(&transaction.Options{
		Logger: transactionLogger,
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

	ledgerLogger.Info("Creating unified HTTP server on " + cfg.ServerAddress)

	// Create the unified server that consolidates all routes on a single port
	// Pass the dual pools for path-based routing to the correct database
	unifiedServer := NewUnifiedServer(
		cfg.ServerAddress,
		ledgerLogger,
		telemetry,
		tenantPools,
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

// initMultiTenantPools creates and configures TWO multi-tenant connection pools:
// one for onboarding entities and one for transaction entities.
//
// The Pool Manager returns separate database credentials for each module:
//
//	"databases": {
//	  "ledger": {
//	    "services": {
//	      "onboarding": { "postgresql": {...} },
//	      "transaction": { "postgresql": {...} }
//	    }
//	  }
//	}
//
// This architecture enables:
// - Independent scaling of onboarding vs transaction databases
// - Separate connection pools with isolated limits
// - Module-specific failover and routing strategies
func initMultiTenantPools(cfg *Config, logger libLog.Logger) *MultiTenantPools {
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
		MaxOpenConnections:      cfg.MaxOpenConnections,
		MaxIdleConnections:      cfg.MaxIdleConnections,
	}

	// Default connection for transaction module
	transactionDefaultConn := &libPostgres.PostgresConnection{
		ConnectionStringPrimary: postgreSourcePrimary,
		ConnectionStringReplica: postgreSourceReplica,
		PrimaryDBName:           cfg.PrimaryDBName,
		ReplicaDBName:           cfg.ReplicaDBName,
		Component:               "transaction",
		Logger:                  logger,
		MaxOpenConnections:      cfg.MaxOpenConnections,
		MaxIdleConnections:      cfg.MaxIdleConnections,
	}

	// Create Pool Manager client (shared between both pools)
	poolManagerClient := poolmanager.NewClient(cfg.PoolManagerURL, logger)

	// Create onboarding pool - handles organizations, ledgers, accounts, assets, portfolios, segments, account-types
	// The module parameter "onboarding" tells Pool Manager which service credentials to return
	onboardingPool := poolmanager.NewTenantConnectionPool(poolManagerClient, "ledger", "onboarding", logger).
		WithConnectionLimits(cfg.MaxOpenConnections, cfg.MaxIdleConnections).
		WithDefaultConnection(onboardingDefaultConn)

	logger.Info("Created onboarding connection pool for multi-tenant mode")

	// Create transaction pool - handles transactions, operations, balances, asset-rates, routes
	// The module parameter "transaction" tells Pool Manager which service credentials to return
	transactionPool := poolmanager.NewTenantConnectionPool(poolManagerClient, "ledger", "transaction", logger).
		WithConnectionLimits(cfg.MaxOpenConnections, cfg.MaxIdleConnections).
		WithDefaultConnection(transactionDefaultConn)

	logger.Info("Created transaction connection pool for multi-tenant mode")

	return &MultiTenantPools{
		OnboardingPool:  onboardingPool,
		TransactionPool: transactionPool,
	}
}
