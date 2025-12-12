package bootstrap

import (
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	"github.com/LerianStudio/midaz/v3/components/onboarding"
	"github.com/LerianStudio/midaz/v3/components/transaction"
)

const ApplicationName = "ledger"

// Config is the top level configuration struct for the unified ledger component.
type Config struct {
	EnvName  string `env:"ENV_NAME"`
	LogLevel string `env:"LOG_LEVEL"`
	Version  string `env:"VERSION"`

	// OpenTelemetry configuration
	OtelServiceName         string `env:"OTEL_RESOURCE_SERVICE_NAME"`
	OtelLibraryName         string `env:"OTEL_LIBRARY_NAME"`
	OtelServiceVersion      string `env:"OTEL_RESOURCE_SERVICE_VERSION"`
	OtelDeploymentEnv       string `env:"OTEL_RESOURCE_DEPLOYMENT_ENVIRONMENT"`
	OtelColExporterEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	EnableTelemetry         bool   `env:"ENABLE_TELEMETRY"`
}

// InitServers initializes the unified ledger service that composes
// both onboarding and transaction modules in a single process.
// The transaction module is initialized first so its BalancePort (the UseCase)
// can be passed directly to onboarding for in-process calls.
func InitServers() *Service {
	cfg := &Config{}

	if err := libCommons.SetConfigFromEnvVars(cfg); err != nil {
		panic(err)
	}

	logger := libZap.InitializeLogger()

	telemetry := libOpentelemetry.InitializeTelemetry(&libOpentelemetry.TelemetryConfig{
		LibraryName:               cfg.OtelLibraryName,
		ServiceName:               cfg.OtelServiceName,
		ServiceVersion:            cfg.OtelServiceVersion,
		DeploymentEnv:             cfg.OtelDeploymentEnv,
		CollectorExporterEndpoint: cfg.OtelColExporterEndpoint,
		EnableTelemetry:           cfg.EnableTelemetry,
		Logger:                    logger,
	})

	logger.Infof("Starting unified ledger component (onboarding + transaction)")

	logger.Info("Initializing transaction module...")

	// Initialize transaction module first to get the BalancePort
	transactionService := transaction.InitService()

	// Get the BalancePort from transaction for in-process communication
	// This is the transaction.UseCase itself which implements BalancePort directly
	balancePort := transactionService.GetBalancePort()

	logger.Info("Transaction module initialized, BalancePort available for in-process calls")

	logger.Info("Initializing onboarding module in UNIFIED MODE...")

	// Initialize onboarding module in unified mode with the BalancePort for direct calls
	// No intermediate adapter needed - the transaction.UseCase is passed directly
	onboardingService := onboarding.InitServiceWithOptions(&onboarding.Options{
		UnifiedMode: true,
		BalancePort: balancePort,
	})

	return &Service{
		OnboardingService:  onboardingService,
		TransactionService: transactionService,
		Logger:             logger,
		Telemetry:          telemetry,
	}
}
