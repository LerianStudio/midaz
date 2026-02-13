package bootstrap

import (
	"fmt"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
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
		baseLogger = libZap.InitializeLogger()
	}

	telemetry := libOpentelemetry.InitializeTelemetry(&libOpentelemetry.TelemetryConfig{
		LibraryName:               cfg.OtelLibraryName,
		ServiceName:               cfg.OtelServiceName,
		ServiceVersion:            cfg.OtelServiceVersion,
		DeploymentEnv:             cfg.OtelDeploymentEnv,
		CollectorExporterEndpoint: cfg.OtelColExporterEndpoint,
		EnableTelemetry:           cfg.EnableTelemetry,
		Logger:                    baseLogger,
	})

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

	// Get Redis client from transaction service for rate limiting (if available)
	redisClient := transactionService.GetRedisClient()

	// Create unified server options with Redis and auth for batch endpoint
	serverOpts := &UnifiedServerOptions{
		RedisClient: redisClient,
		AuthClient:  auth,
	}

	// Create the unified server that consolidates all routes on a single port
	unifiedServer := NewUnifiedServerWithOptions(
		cfg.ServerAddress,
		ledgerLogger,
		telemetry,
		serverOpts,
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
