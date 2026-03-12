// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libCommons "github.com/LerianStudio/lib-commons/v4/commons"
	libCircuitBreaker "github.com/LerianStudio/lib-commons/v4/commons/circuitbreaker"
	libLog "github.com/LerianStudio/lib-commons/v4/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v4/commons/opentelemetry"
	tmclient "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/client"
	tmcore "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/core"
	tmmiddleware "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/middleware"
	tmmongo "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/mongo"
	tmpostgres "github.com/LerianStudio/lib-commons/v4/commons/tenant-manager/postgres"
	libZap "github.com/LerianStudio/lib-commons/v4/commons/zap"
	httpin "github.com/LerianStudio/midaz/v3/components/ledger/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v3/components/onboarding"
	"github.com/LerianStudio/midaz/v3/components/transaction"
	"github.com/LerianStudio/midaz/v3/pkg/constant"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

const ApplicationName = "ledger"

// Config is the top level configuration struct for the unified ledger component.
type Config struct {
	ApplicationName string `env:"APPLICATION_NAME"`
	EnvName         string `env:"ENV_NAME"`
	LogLevel        string `env:"LOG_LEVEL"`
	Version         string `env:"VERSION"`

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

	// Multi-tenant configuration
	MultiTenantEnabled                  bool   `env:"MULTI_TENANT_ENABLED"`
	MultiTenantURL                      string `env:"MULTI_TENANT_URL"`
	MultiTenantEnvironment              string `env:"MULTI_TENANT_ENVIRONMENT"`
	MultiTenantCircuitBreakerThreshold  int    `env:"MULTI_TENANT_CIRCUIT_BREAKER_THRESHOLD"`
	MultiTenantCircuitBreakerTimeoutSec int    `env:"MULTI_TENANT_CIRCUIT_BREAKER_TIMEOUT_SEC"`
	MultiTenantMaxTenantPools           int    `env:"MULTI_TENANT_MAX_TENANT_POOLS"`
	MultiTenantIdleTimeoutSec           int    `env:"MULTI_TENANT_IDLE_TIMEOUT_SEC"`
}

// Options contains optional dependencies that can be injected by callers.
type Options struct {
	// Logger allows callers to provide a pre-configured logger, avoiding multiple
	// initializations when composing components (e.g. unified ledger).
	Logger libLog.Logger

	// CircuitBreakerStateListener receives notifications when circuit breaker state changes.
	// This is optional - pass nil if you don't need state change notifications.
	CircuitBreakerStateListener libCircuitBreaker.StateChangeListener

	// TenantClient is the tenant manager client for multi-tenant mode.
	// Nil when multi-tenant is disabled.
	TenantClient *tmclient.Client
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

	if cfg.MultiTenantEnabled && !cfg.AuthEnabled {
		return nil, fmt.Errorf("MULTI_TENANT_ENABLED=true requires PLUGIN_AUTH_ENABLED=true; " +
			"running multi-tenant mode without authentication allows cross-tenant data access")
	}

	var baseLogger libLog.Logger
	if opts != nil && opts.Logger != nil {
		baseLogger = opts.Logger
	} else {
		var err error

		baseLogger, err = libZap.New(libZap.Config{
			Environment:     libZap.EnvironmentDevelopment,
			Level:           cfg.LogLevel,
			OTelLibraryName: cfg.OtelLibraryName,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to initialize logger: %w", err)
		}
	}

	telemetry, err := libOpentelemetry.NewTelemetry(libOpentelemetry.TelemetryConfig{
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

	ledgerLogger := baseLogger.With(
		libLog.String("component", "ledger"),
		libLog.String("startup_id", startupID),
	)
	transactionLogger := baseLogger.With(
		libLog.String("component", "transaction"),
		libLog.String("startup_id", startupID),
	)
	onboardingLogger := baseLogger.With(
		libLog.String("component", "onboarding"),
		libLog.String("startup_id", startupID),
	)

	ledgerLogger.Log(context.Background(), libLog.LevelInfo, "Starting unified ledger component",
		libLog.String("version", cfg.Version),
		libLog.String("env", cfg.EnvName),
	)

	// Multi-tenant setup: prefer injected client from Options, fall back to config-based creation.
	var tenantClient *tmclient.Client

	var tenantServiceName string

	if opts != nil && opts.TenantClient != nil {
		tenantClient = opts.TenantClient
		tenantServiceName = strings.TrimSpace(cfg.ApplicationName)
	} else {
		var err error

		tenantClient, tenantServiceName, err = initTenantClient(cfg, ledgerLogger)
		if err != nil {
			return nil, err
		}
	}

	ledgerLogger.Log(context.Background(), libLog.LevelInfo, "Initializing transaction module...")

	var stateListener libCircuitBreaker.StateChangeListener

	if opts != nil {
		stateListener = opts.CircuitBreakerStateListener
	}

	transactionOpts := &transaction.Options{
		Logger:                      transactionLogger,
		CircuitBreakerStateListener: stateListener,
		MultiTenantEnabled:          cfg.MultiTenantEnabled,
		TenantClient:                tenantClient,
		TenantServiceName:           tenantServiceName,
		TenantEnvironment:           cfg.MultiTenantEnvironment,
		TenantManagerURL:            strings.TrimSpace(cfg.MultiTenantURL),
	}

	// Initialize transaction module first to get the BalancePort
	transactionService, err := transaction.InitServiceWithOptionsOrError(transactionOpts)
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

	ledgerLogger.Log(context.Background(), libLog.LevelInfo, "Transaction module initialized, BalancePort and MetadataIndexPort available for in-process calls")

	ledgerLogger.Log(context.Background(), libLog.LevelInfo, "Initializing onboarding module in UNIFIED MODE...")

	// Initialize onboarding module in unified mode with the BalancePort for direct calls
	// No intermediate adapter needed - the transaction.UseCase is passed directly
	onboardingService, err := onboarding.InitServiceWithOptionsOrError(&onboarding.Options{
		Logger:             onboardingLogger,
		UnifiedMode:        true,
		BalancePort:        balancePort,
		MultiTenantEnabled: cfg.MultiTenantEnabled,
		TenantClient:       tenantClient,
		TenantServiceName:  tenantServiceName,
		TenantEnvironment:  cfg.MultiTenantEnvironment,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize onboarding module: %w", err)
	}

	ledgerLogger.Log(context.Background(), libLog.LevelInfo, "Onboarding module initialized")

	// Get the metadata port from onboarding for metadata index operations
	onboardingMetadataRepo := onboardingService.GetMetadataIndexPort()
	if onboardingMetadataRepo == nil {
		return nil, fmt.Errorf("failed to get MetadataIndexPort from onboarding module")
	}

	// Wire the SettingsPort from onboarding to transaction for ledger settings queries
	// This resolves the circular dependency: transaction is initialized first (for BalancePort),
	// then onboarding (with BalancePort), then SettingsPort is wired back to transaction.
	settingsPort := onboardingService.GetSettingsPort()
	if settingsPort == nil {
		return nil, fmt.Errorf("failed to get SettingsPort from onboarding module")
	}

	transactionService.SetSettingsPort(settingsPort)

	ledgerLogger.Log(context.Background(), libLog.LevelInfo, "SettingsPort wired from onboarding to transaction for in-process settings queries")

	ledgerLogger.Log(context.Background(), libLog.LevelInfo, "Both metadata index repositories available for settings routes")

	// Build MultiPoolMiddleware for per-tenant database routing
	multiPoolMiddleware := buildMultiPoolMiddleware(cfg, ledgerLogger, transactionService, onboardingService)

	// Create auth client for metadata index routes
	auth := middleware.NewAuthClient(cfg.AuthHost, cfg.AuthEnabled, nil)

	// Create metadata index handler with both repositories
	metadataIndexHandler := &httpin.MetadataIndexHandler{
		OnboardingMetadataRepo:  onboardingMetadataRepo,
		TransactionMetadataRepo: transactionMetadataRepo,
	}

	// Create route registrar for ledger-specific routes (metadata indexes)
	ledgerRouteRegistrar := httpin.CreateRouteRegistrar(auth, metadataIndexHandler)

	ledgerLogger.Log(context.Background(), libLog.LevelInfo, "Creating unified HTTP server on "+cfg.ServerAddress)

	// Create the unified server that consolidates all routes on a single port
	unifiedServer := NewUnifiedServer(
		cfg.ServerAddress,
		ledgerLogger,
		telemetry,
		multiPoolMiddleware,
		onboardingService.GetRouteRegistrar(),
		transactionService.GetRouteRegistrar(),
		ledgerRouteRegistrar,
	)

	ledgerLogger.Log(context.Background(), libLog.LevelInfo, "Unified ledger component started successfully with single-port mode",
		libLog.String("version", cfg.Version),
		libLog.String("env", cfg.EnvName),
		libLog.String("server_address", cfg.ServerAddress),
	)

	return &Service{
		OnboardingService:  onboardingService,
		TransactionService: transactionService,
		UnifiedServer:      unifiedServer,
		Logger:             ledgerLogger,
		Telemetry:          telemetry,
	}, nil
}

// initTenantClient creates the multi-tenant client when multi-tenant mode is enabled.
// Returns the client, the resolved tenantServiceName, and error.
// Returns (nil, serviceName, nil) when multi-tenant is disabled.
func initTenantClient(cfg *Config, logger libLog.Logger) (*tmclient.Client, string, error) {
	tenantServiceName := cfg.ApplicationName

	if !cfg.MultiTenantEnabled {
		return nil, tenantServiceName, nil
	}

	tenantManagerURL := strings.TrimSpace(cfg.MultiTenantURL)
	if tenantManagerURL == "" {
		return nil, "", fmt.Errorf("MULTI_TENANT_URL is required when MULTI_TENANT_ENABLED=true")
	}

	tenantServiceName = strings.TrimSpace(cfg.ApplicationName)
	if tenantServiceName == "" {
		return nil, "", fmt.Errorf("APPLICATION_NAME is required when MULTI_TENANT_ENABLED=true")
	}

	// Apply safe defaults for circuit breaker when not configured
	cbThreshold := cfg.MultiTenantCircuitBreakerThreshold
	if cbThreshold <= 0 {
		cbThreshold = 5
	}

	cbTimeoutSec := cfg.MultiTenantCircuitBreakerTimeoutSec
	if cbTimeoutSec <= 0 {
		cbTimeoutSec = 30
	}

	tenantClient, err := tmclient.NewClient(
		tenantManagerURL,
		logger,
		tmclient.WithCircuitBreaker(cbThreshold, time.Duration(cbTimeoutSec)*time.Second),
	)
	if err != nil {
		return nil, "", fmt.Errorf("failed to initialize tenant manager client: %w", err)
	}

	logger.Log(context.Background(), libLog.LevelInfo, "Multi-tenant mode enabled",
		libLog.String("service", tenantServiceName),
		libLog.String("environment", cfg.MultiTenantEnvironment),
		libLog.Bool("tenant_manager_configured", true),
	)

	return tenantClient, tenantServiceName, nil
}

// buildMultiPoolMiddleware constructs the per-tenant database routing middleware.
// Returns nil when multi-tenant is disabled.
func buildMultiPoolMiddleware(
	cfg *Config,
	logger libLog.Logger,
	transactionService transaction.TransactionService,
	onboardingService onboarding.OnboardingService,
) *tmmiddleware.MultiPoolMiddleware {
	if !cfg.MultiTenantEnabled {
		return nil
	}

	var multiPoolOpts []tmmiddleware.MultiPoolOption

	// Transaction module route: safe two-return type assertions with typed nil checks
	rawTxnPG := transactionService.GetPGManager()
	rawTxnMgo := transactionService.GetMongoManager()

	txnPGMgr, pgOk := rawTxnPG.(*tmpostgres.Manager)
	txnMgoMgr, mgoOk := rawTxnMgo.(*tmmongo.Manager)

	if pgOk && mgoOk && txnPGMgr != nil && txnMgoMgr != nil {
		multiPoolOpts = append(multiPoolOpts,
			tmmiddleware.WithRoute(
				[]string{"/v1/organizations"},
				"transaction",
				txnPGMgr,
				txnMgoMgr,
			),
		)
	} else {
		logger.Log(context.Background(), libLog.LevelWarn, "Transaction module managers not available for multi-tenant routing")
	}

	// Onboarding module (default route): safe two-return type assertions with typed nil checks
	rawOnbPG := onboardingService.GetPGManager()
	rawOnbMgo := onboardingService.GetMongoManager()

	onbPGMgr, pgOk := rawOnbPG.(*tmpostgres.Manager)
	onbMgoMgr, mgoOk := rawOnbMgo.(*tmmongo.Manager)

	if pgOk && mgoOk && onbPGMgr != nil && onbMgoMgr != nil {
		multiPoolOpts = append(multiPoolOpts,
			tmmiddleware.WithDefaultRoute(
				"onboarding",
				onbPGMgr,
				onbMgoMgr,
			),
		)
	} else {
		logger.Log(context.Background(), libLog.LevelWarn, "Onboarding module managers not available for multi-tenant default route")
	}

	multiPoolOpts = append(multiPoolOpts,
		tmmiddleware.WithCrossModuleInjection(),
		tmmiddleware.WithPublicPaths("/health", "/version", "/swagger"),
		tmmiddleware.WithMultiPoolLogger(logger),
		tmmiddleware.WithErrorMapper(midazErrorMapper),
	)

	// Consumer trigger
	if mtConsumer := transactionService.GetMultiTenantConsumer(); mtConsumer != nil {
		if consumer, ok := mtConsumer.(tmmiddleware.ConsumerTrigger); ok {
			multiPoolOpts = append(multiPoolOpts,
				tmmiddleware.WithConsumerTrigger(consumer),
			)
		}
	}

	multiPoolMiddleware := tmmiddleware.NewMultiPoolMiddleware(multiPoolOpts...)

	logger.Log(context.Background(), libLog.LevelInfo, "MultiPoolMiddleware configured for per-tenant database routing")

	return multiPoolMiddleware
}

// midazErrorMapper converts tenant-manager errors into Midaz-specific HTTP responses.
// It handles the TenantNotProvisionedError case with a 422 status code.
// For all other errors, it returns the error to let the default error handler process it.
func midazErrorMapper(c *fiber.Ctx, err error, tenantID string) error {
	if err == nil {
		return nil
	}

	if tmcore.IsTenantNotProvisionedError(err) {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(fiber.Map{
			"code":    constant.ErrTenantNotProvisioned.Error(),
			"title":   "Tenant Not Provisioned",
			"message": "Database schema not initialized for this tenant. Contact your administrator.",
		})
	}

	return err
}
