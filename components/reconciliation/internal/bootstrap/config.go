// Package bootstrap provides initialization and configuration for the reconciliation service.
package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"regexp"
	"time"

	_ "github.com/lib/pq"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"

	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/adapters/postgres"
	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/engine"
)

// ApplicationName is the identifier for the reconciliation service.
const ApplicationName = "reconciliation"

// Configuration validation constants.
const (
	minReconciliationIntervalSeconds = 60
	minMaxDiscrepancies              = 1
	maxMaxDiscrepancies              = 1000
	minMaxOpenConnections            = 1
	maxMaxOpenConnections            = 100
	defaultReconciliationInterval    = 5 * time.Minute
	defaultServerPort                = ":3005"
	connectionPingTimeout            = 10 * time.Second
	connectionMaxLifetime            = time.Hour
)

// Default configuration values for reconciliation worker.
const (
	defaultSettlementWaitSeconds    = 300 // 5 minutes
	defaultMaxDiscrepanciesToReport = 100
	defaultMaxOpenConnections       = 10
	defaultMaxIdleConnections       = 5
)

// Sentinel errors for configuration validation.
var (
	ErrInvalidReconciliationInterval      = errors.New("RECONCILIATION_INTERVAL_SECONDS must be >= 60")
	ErrInvalidMaxDiscrepancies            = errors.New("MAX_DISCREPANCIES_TO_REPORT must be between 1 and 1000")
	ErrInvalidMaxOpenConnections          = errors.New("DB_MAX_OPEN_CONNS must be between 1 and 100")
	ErrOnboardingSSLDisabledInProduction  = errors.New("ONBOARDING_DB_SSLMODE=disable is not allowed in production")
	ErrTransactionSSLDisabledInProduction = errors.New("TRANSACTION_DB_SSLMODE=disable is not allowed in production")
	ErrConnectionFailed                   = errors.New("connection failed")
	ErrPingFailed                         = errors.New("ping failed")
	ErrLoadConfig                         = errors.New("failed to load config")
	ErrInvalidConfig                      = errors.New("invalid configuration")
	ErrConnectOnboardingDB                = errors.New("failed to connect to onboarding DB")
	ErrConnectTransactionDB               = errors.New("failed to connect to transaction DB")
	ErrConnectMongoDB                     = errors.New("failed to connect to MongoDB")
	ErrOpenDatabase                       = errors.New("failed to open database")
)

// Pre-compiled regex patterns for credential sanitization (thread-safe)
var (
	passwordRE = regexp.MustCompile(`password=\S+`)
	userinfoRE = regexp.MustCompile(`://[^@]+@`)
)

// Config holds all configuration for the reconciliation worker
type Config struct {
	EnvName  string `env:"ENV_NAME"`
	LogLevel string `env:"LOG_LEVEL"`
	Version  string `env:"VERSION"`

	// HTTP Server (for status endpoints)
	ServerPort string `env:"SERVER_PORT"`

	// PostgreSQL Replica (READ-ONLY) - Onboarding DB
	OnboardingDBHost     string `env:"ONBOARDING_DB_HOST"`
	OnboardingDBUser     string `env:"ONBOARDING_DB_USER"`
	OnboardingDBPassword string `env:"ONBOARDING_DB_PASSWORD"`
	OnboardingDBName     string `env:"ONBOARDING_DB_NAME"`
	OnboardingDBPort     string `env:"ONBOARDING_DB_PORT"`
	OnboardingDBSSLMode  string `env:"ONBOARDING_DB_SSLMODE"`

	// PostgreSQL Replica (READ-ONLY) - Transaction DB
	TransactionDBHost     string `env:"TRANSACTION_DB_HOST"`
	TransactionDBUser     string `env:"TRANSACTION_DB_USER"`
	TransactionDBPassword string `env:"TRANSACTION_DB_PASSWORD"`
	TransactionDBName     string `env:"TRANSACTION_DB_NAME"`
	TransactionDBPort     string `env:"TRANSACTION_DB_PORT"`
	TransactionDBSSLMode  string `env:"TRANSACTION_DB_SSLMODE"`

	// MongoDB
	MongoHost              string `env:"MONGO_HOST"`
	MongoUser              string `env:"MONGO_USER"`
	MongoPassword          string `env:"MONGO_PASSWORD"`
	MongoPort              string `env:"MONGO_PORT"`
	MongoParameters        string `env:"MONGO_PARAMETERS"`
	OnboardingMongoDBName  string `env:"ONBOARDING_MONGO_DB_NAME"`
	TransactionMongoDBName string `env:"TRANSACTION_MONGO_DB_NAME"`

	// Worker Configuration
	ReconciliationIntervalSeconds int   `env:"RECONCILIATION_INTERVAL_SECONDS"`
	SettlementWaitSeconds         int   `env:"SETTLEMENT_WAIT_SECONDS"`
	DiscrepancyThreshold          int64 `env:"DISCREPANCY_THRESHOLD"`
	MaxDiscrepanciesToReport      int   `env:"MAX_DISCREPANCIES_TO_REPORT"`

	// Telemetry
	OtelServiceName         string `env:"OTEL_RESOURCE_SERVICE_NAME"`
	OtelLibraryName         string `env:"OTEL_LIBRARY_NAME"`
	OtelServiceVersion      string `env:"OTEL_RESOURCE_SERVICE_VERSION"`
	OtelDeploymentEnv       string `env:"OTEL_RESOURCE_DEPLOYMENT_ENVIRONMENT"`
	OtelColExporterEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	EnableTelemetry         bool   `env:"ENABLE_TELEMETRY"`

	// Connection Pool
	MaxOpenConnections int `env:"DB_MAX_OPEN_CONNS"`
	MaxIdleConnections int `env:"DB_MAX_IDLE_CONNS"`
}

// GetReconciliationInterval returns the interval as duration
func (c *Config) GetReconciliationInterval() time.Duration {
	if c.ReconciliationIntervalSeconds <= 0 {
		return defaultReconciliationInterval
	}

	return time.Duration(c.ReconciliationIntervalSeconds) * time.Second
}

// GetServerAddress returns the server address from SERVER_PORT
func (c *Config) GetServerAddress() string {
	if c.ServerPort != "" {
		return ":" + c.ServerPort
	}

	return defaultServerPort
}

// Validate checks configuration values for correctness
func (c *Config) Validate() error {
	if c.ReconciliationIntervalSeconds < minReconciliationIntervalSeconds {
		return fmt.Errorf("%w: got %d", ErrInvalidReconciliationInterval, c.ReconciliationIntervalSeconds)
	}

	if c.MaxDiscrepanciesToReport < minMaxDiscrepancies || c.MaxDiscrepanciesToReport > maxMaxDiscrepancies {
		return fmt.Errorf("%w: got %d", ErrInvalidMaxDiscrepancies, c.MaxDiscrepanciesToReport)
	}

	if c.MaxOpenConnections < minMaxOpenConnections || c.MaxOpenConnections > maxMaxOpenConnections {
		return fmt.Errorf("%w: got %d", ErrInvalidMaxOpenConnections, c.MaxOpenConnections)
	}

	if c.OnboardingDBSSLMode == "disable" && c.EnvName == "production" {
		return ErrOnboardingSSLDisabledInProduction
	}

	if c.TransactionDBSSLMode == "disable" && c.EnvName == "production" {
		return ErrTransactionSSLDisabledInProduction
	}

	return nil
}

// DefaultConfig returns sensible defaults
func DefaultConfig() *Config {
	return &Config{
		ReconciliationIntervalSeconds: int(defaultReconciliationInterval.Seconds()),
		SettlementWaitSeconds:         defaultSettlementWaitSeconds,
		DiscrepancyThreshold:          0, // Report any discrepancy
		MaxDiscrepanciesToReport:      defaultMaxDiscrepanciesToReport,
		MaxOpenConnections:            defaultMaxOpenConnections,
		MaxIdleConnections:            defaultMaxIdleConnections,
		OnboardingDBSSLMode:           "require", // Secure default
		TransactionDBSSLMode:          "require", // Secure default
	}
}

// sanitizeConnectionError removes credentials from error messages
func sanitizeConnectionError(msg string) string {
	msg = passwordRE.ReplaceAllString(msg, "password=REDACTED")
	msg = userinfoRE.ReplaceAllString(msg, "://REDACTED@")

	return msg
}

// Options allows injecting dependencies
type Options struct {
	Logger libLog.Logger
}

// InitServersWithOptions initializes with optional dependencies
func InitServersWithOptions(opts *Options) (*Service, error) {
	cfg := DefaultConfig()
	if err := libCommons.SetConfigFromEnvVars(cfg); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrLoadConfig, err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidConfig, err)
	}

	// Logger
	var logger libLog.Logger
	if opts != nil && opts.Logger != nil {
		logger = opts.Logger
	} else {
		logger = libZap.InitializeLogger()
	}

	// Telemetry
	telemetry := libOpentelemetry.InitializeTelemetry(&libOpentelemetry.TelemetryConfig{
		LibraryName:               cfg.OtelLibraryName,
		ServiceName:               cfg.OtelServiceName,
		ServiceVersion:            cfg.OtelServiceVersion,
		DeploymentEnv:             cfg.OtelDeploymentEnv,
		CollectorExporterEndpoint: cfg.OtelColExporterEndpoint,
		EnableTelemetry:           cfg.EnableTelemetry,
		Logger:                    logger,
	})

	// PostgreSQL connections (direct, no lib-commons wrapper for isolation)
	onboardingDB, err := connectPostgres(
		cfg.OnboardingDBHost, cfg.OnboardingDBPort, cfg.OnboardingDBUser,
		cfg.OnboardingDBPassword, cfg.OnboardingDBName, cfg.OnboardingDBSSLMode,
		cfg.MaxOpenConnections, cfg.MaxIdleConnections,
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrConnectOnboardingDB, err)
	}

	transactionDB, err := connectPostgres(
		cfg.TransactionDBHost, cfg.TransactionDBPort, cfg.TransactionDBUser,
		cfg.TransactionDBPassword, cfg.TransactionDBName, cfg.TransactionDBSSLMode,
		cfg.MaxOpenConnections, cfg.MaxIdleConnections,
	)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrConnectTransactionDB, err)
	}

	// MongoDB connections
	mongoClient, err := connectMongo(cfg.MongoHost, cfg.MongoPort, cfg.MongoUser, cfg.MongoPassword, cfg.MongoParameters)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrConnectMongoDB, err)
	}

	onboardingMongoDB := mongoClient.Database(cfg.OnboardingMongoDBName)
	transactionMongoDB := mongoClient.Database(cfg.TransactionMongoDBName)

	checkers := []postgres.ReconciliationChecker{
		postgres.NewBalanceChecker(transactionDB),
		postgres.NewDoubleEntryChecker(transactionDB),
		postgres.NewOrphanChecker(transactionDB),
		postgres.NewReferentialChecker(onboardingDB, transactionDB),
		postgres.NewSyncChecker(transactionDB),
		postgres.NewDLQChecker(transactionDB),
	}

	checkerConfigs := engine.CheckerConfigMap{
		postgres.CheckerNameBalance: {
			DiscrepancyThreshold: cfg.DiscrepancyThreshold,
			MaxResults:           cfg.MaxDiscrepanciesToReport,
		},
		postgres.CheckerNameDoubleEntry: {
			MaxResults: cfg.MaxDiscrepanciesToReport,
		},
		postgres.CheckerNameOrphans: {
			MaxResults: cfg.MaxDiscrepanciesToReport,
		},
		postgres.CheckerNameReferential: {},
		postgres.CheckerNameSync: {
			StaleThresholdSeconds: cfg.SettlementWaitSeconds,
			MaxResults:            cfg.MaxDiscrepanciesToReport,
		},
		postgres.CheckerNameDLQ: {
			MaxResults: cfg.MaxDiscrepanciesToReport,
		},
	}

	// Initialize engine with injected checkers and configs
	eng := engine.NewReconciliationEngine(
		onboardingDB,
		transactionDB,
		onboardingMongoDB,
		transactionMongoDB,
		logger,
		cfg.SettlementWaitSeconds,
		checkers,
		checkerConfigs,
	)

	// Initialize worker
	worker := NewReconciliationWorker(eng, logger, cfg)

	// Initialize HTTP server
	httpServer := NewHTTPServer(cfg.GetServerAddress(), eng, logger, telemetry, cfg.Version, cfg.EnvName)

	return &Service{
		Worker:        worker,
		HTTPServer:    httpServer,
		Logger:        logger,
		Config:        cfg,
		onboardingDB:  onboardingDB,
		transactionDB: transactionDB,
		mongoClient:   mongoClient,
	}, nil
}

// connectPostgres creates a direct PostgreSQL connection
func connectPostgres(host, port, user, password, dbname, sslmode string, maxOpen, maxIdle int) (*sql.DB, error) {
	connStr := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s statement_timeout=30000 lock_timeout=10000",
		host, port, user, password, dbname, sslmode,
	)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrOpenDatabase, err)
	}

	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(connectionMaxLifetime)

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), connectionPingTimeout)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrPingFailed, err)
	}

	return db, nil
}

// connectMongo creates a direct MongoDB connection
func connectMongo(host, port, user, password, parameters string) (*mongo.Client, error) {
	uri := fmt.Sprintf("mongodb://%s:%s@%s/?authSource=admin&directConnection=true",
		user, password, net.JoinHostPort(host, port))
	if parameters != "" {
		uri += "&" + parameters
	}

	ctx, cancel := context.WithTimeout(context.Background(), connectionPingTimeout)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		// Sanitize error to prevent credential leakage
		return nil, fmt.Errorf("%w: %s", ErrConnectionFailed, sanitizeConnectionError(err.Error()))
	}

	// Test connection
	if err := client.Ping(ctx, nil); err != nil {
		// Sanitize error to prevent credential leakage
		return nil, fmt.Errorf("%w: %s", ErrPingFailed, sanitizeConnectionError(err.Error()))
	}

	return client, nil
}
