// Package bootstrap provides initialization and configuration for the reconciliation service.
package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"os"
	"regexp"
	"strings"
	"time"

	_ "github.com/lib/pq"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"

	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/adapters/crm"
	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/adapters/crossdb"
	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/adapters/metadata"
	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/adapters/postgres"
	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/engine"
	reconmetrics "github.com/LerianStudio/midaz/v3/components/reconciliation/internal/metrics"
	"github.com/LerianStudio/midaz/v3/components/reconciliation/internal/reportstore"
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
	defaultReportDir                 = "./reports"
	defaultReportMaxFiles            = 100
	defaultReportRetentionDays       = 7
	defaultOutboxStaleSeconds        = 600
	defaultMetadataLookbackDays      = 7
	defaultMetadataMaxScan           = 5000
	defaultCrossDBBatchSize          = 200
	defaultCrossDBMaxScan            = 5000
	defaultRedisSampleSize           = 250
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
	ErrInvalidReportMaxFiles              = errors.New("REPORTS_MAX_FILES must be between 1 and 10000")
	ErrInvalidReportRetentionDays         = errors.New("REPORTS_RETENTION_DAYS must be between 1 and 365")
	ErrInvalidOutboxStaleSeconds          = errors.New("OUTBOX_STALE_SECONDS must be between 60 and 86400")
	ErrInvalidMetadataLookbackDays        = errors.New("METADATA_LOOKBACK_DAYS must be between 1 and 90")
	ErrInvalidMetadataMaxScan             = errors.New("METADATA_MAX_SCAN must be between 1 and 100000")
	ErrInvalidCrossDBBatchSize            = errors.New("CROSSDB_BATCH_SIZE must be between 1 and 5000")
	ErrInvalidCrossDBMaxScan              = errors.New("CROSSDB_MAX_SCAN must be between 1 and 100000")
	ErrInvalidRedisSampleSize             = errors.New("REDIS_SAMPLE_SIZE must be between 1 and 10000")
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
	OutboxStaleSeconds            int   `env:"OUTBOX_STALE_SECONDS"`
	MetadataLookbackDays          int   `env:"METADATA_LOOKBACK_DAYS"`
	MetadataMaxScan               int   `env:"METADATA_MAX_SCAN"`
	CrossDBBatchSize              int   `env:"CROSSDB_BATCH_SIZE"`
	CrossDBMaxScan                int   `env:"CROSSDB_MAX_SCAN"`
	RedisSampleSize               int   `env:"REDIS_SAMPLE_SIZE"`

	// Report persistence
	ReportDir           string `env:"REPORTS_DIR"`
	ReportMaxFiles      int    `env:"REPORTS_MAX_FILES"`
	ReportRetentionDays int    `env:"REPORTS_RETENTION_DAYS"`

	// Telemetry
	OtelServiceName         string `env:"OTEL_RESOURCE_SERVICE_NAME"`
	OtelLibraryName         string `env:"OTEL_LIBRARY_NAME"`
	OtelServiceVersion      string `env:"OTEL_RESOURCE_SERVICE_VERSION"`
	OtelDeploymentEnv       string `env:"OTEL_RESOURCE_DEPLOYMENT_ENVIRONMENT"`
	OtelColExporterEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	EnableTelemetry         bool   `env:"ENABLE_TELEMETRY"`

	// Redis (optional)
	RedisHost            string `env:"REDIS_HOST"`
	RedisPassword        string `env:"REDIS_PASSWORD"`
	RedisDB              int    `env:"REDIS_DB"`
	RedisProtocol        int    `env:"REDIS_PROTOCOL"`
	RedisTLS             bool   `env:"REDIS_TLS"`
	RedisCACert          string `env:"REDIS_CA_CERT"`
	RedisPoolSize        int    `env:"REDIS_POOL_SIZE"`
	RedisReadTimeout     int    `env:"REDIS_READ_TIMEOUT"`
	RedisWriteTimeout    int    `env:"REDIS_WRITE_TIMEOUT"`
	RedisDialTimeout     int    `env:"REDIS_DIAL_TIMEOUT"`
	RedisPoolTimeout     int    `env:"REDIS_POOL_TIMEOUT"`
	RedisMaxRetries      int    `env:"REDIS_MAX_RETRIES"`
	RedisMinRetryBackoff int    `env:"REDIS_MIN_RETRY_BACKOFF"`
	RedisMaxRetryBackoff int    `env:"REDIS_MAX_RETRY_BACKOFF"`

	// CRM MongoDB (optional)
	CRMMongoDBName string `env:"CRM_MONGO_DB_NAME"`

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

	if c.ReportMaxFiles < 1 || c.ReportMaxFiles > 10000 {
		return fmt.Errorf("%w: got %d", ErrInvalidReportMaxFiles, c.ReportMaxFiles)
	}

	if c.ReportRetentionDays < 1 || c.ReportRetentionDays > 365 {
		return fmt.Errorf("%w: got %d", ErrInvalidReportRetentionDays, c.ReportRetentionDays)
	}

	if c.OutboxStaleSeconds < 60 || c.OutboxStaleSeconds > 86400 {
		return fmt.Errorf("%w: got %d", ErrInvalidOutboxStaleSeconds, c.OutboxStaleSeconds)
	}

	if c.MetadataLookbackDays < 1 || c.MetadataLookbackDays > 90 {
		return fmt.Errorf("%w: got %d", ErrInvalidMetadataLookbackDays, c.MetadataLookbackDays)
	}

	if c.MetadataMaxScan < 1 || c.MetadataMaxScan > 100000 {
		return fmt.Errorf("%w: got %d", ErrInvalidMetadataMaxScan, c.MetadataMaxScan)
	}

	if c.CrossDBBatchSize < 1 || c.CrossDBBatchSize > 5000 {
		return fmt.Errorf("%w: got %d", ErrInvalidCrossDBBatchSize, c.CrossDBBatchSize)
	}

	if c.CrossDBMaxScan < 1 || c.CrossDBMaxScan > 100000 {
		return fmt.Errorf("%w: got %d", ErrInvalidCrossDBMaxScan, c.CrossDBMaxScan)
	}

	if c.RedisSampleSize < 1 || c.RedisSampleSize > 10000 {
		return fmt.Errorf("%w: got %d", ErrInvalidRedisSampleSize, c.RedisSampleSize)
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
		OutboxStaleSeconds:            defaultOutboxStaleSeconds,
		MetadataLookbackDays:          defaultMetadataLookbackDays,
		MetadataMaxScan:               defaultMetadataMaxScan,
		CrossDBBatchSize:              defaultCrossDBBatchSize,
		CrossDBMaxScan:                defaultCrossDBMaxScan,
		RedisSampleSize:               defaultRedisSampleSize,
		ReportDir:                     defaultReportDir,
		ReportMaxFiles:                defaultReportMaxFiles,
		ReportRetentionDays:           defaultReportRetentionDays,
		RedisDB:                       0,
		RedisProtocol:                 3,
		RedisPoolSize:                 10,
		RedisReadTimeout:              3,
		RedisWriteTimeout:             3,
		RedisDialTimeout:              5,
		RedisPoolTimeout:              2,
		RedisMaxRetries:               3,
		RedisMinRetryBackoff:          8,
		RedisMaxRetryBackoff:          1,
	}
}

// ApplyEnvDefaults restores defaults for config fields without explicit env values.
func (c *Config) ApplyEnvDefaults() {
	if envMissingOrEmpty("RECONCILIATION_INTERVAL_SECONDS") {
		c.ReconciliationIntervalSeconds = int(defaultReconciliationInterval.Seconds())
	}
	if envMissingOrEmpty("SETTLEMENT_WAIT_SECONDS") {
		c.SettlementWaitSeconds = defaultSettlementWaitSeconds
	}
	if envMissingOrEmpty("MAX_DISCREPANCIES_TO_REPORT") {
		c.MaxDiscrepanciesToReport = defaultMaxDiscrepanciesToReport
	}

	if envMissingOrEmpty("DB_MAX_OPEN_CONNS") {
		c.MaxOpenConnections = defaultMaxOpenConnections
	}
	if envMissingOrEmpty("DB_MAX_IDLE_CONNS") {
		c.MaxIdleConnections = defaultMaxIdleConnections
	}
	if envMissingOrEmpty("ONBOARDING_DB_SSLMODE") {
		c.OnboardingDBSSLMode = "require"
	}
	if envMissingOrEmpty("TRANSACTION_DB_SSLMODE") {
		c.TransactionDBSSLMode = "require"
	}

	if envMissingOrEmpty("OUTBOX_STALE_SECONDS") {
		c.OutboxStaleSeconds = defaultOutboxStaleSeconds
	}
	if envMissingOrEmpty("METADATA_LOOKBACK_DAYS") {
		c.MetadataLookbackDays = defaultMetadataLookbackDays
	}
	if envMissingOrEmpty("METADATA_MAX_SCAN") {
		c.MetadataMaxScan = defaultMetadataMaxScan
	}
	if envMissingOrEmpty("CROSSDB_BATCH_SIZE") {
		c.CrossDBBatchSize = defaultCrossDBBatchSize
	}
	if envMissingOrEmpty("CROSSDB_MAX_SCAN") {
		c.CrossDBMaxScan = defaultCrossDBMaxScan
	}

	if envMissingOrEmpty("REDIS_SAMPLE_SIZE") {
		c.RedisSampleSize = defaultRedisSampleSize
	}

	if envMissingOrEmpty("REPORTS_DIR") {
		c.ReportDir = defaultReportDir
	}
	if envMissingOrEmpty("REPORTS_MAX_FILES") {
		c.ReportMaxFiles = defaultReportMaxFiles
	}
	if envMissingOrEmpty("REPORTS_RETENTION_DAYS") {
		c.ReportRetentionDays = defaultReportRetentionDays
	}

	if envMissingOrEmpty("REDIS_PROTOCOL") {
		c.RedisProtocol = 3
	}
	if envMissingOrEmpty("REDIS_POOL_SIZE") {
		c.RedisPoolSize = 10
	}
	if envMissingOrEmpty("REDIS_READ_TIMEOUT") {
		c.RedisReadTimeout = 3
	}
	if envMissingOrEmpty("REDIS_WRITE_TIMEOUT") {
		c.RedisWriteTimeout = 3
	}
	if envMissingOrEmpty("REDIS_DIAL_TIMEOUT") {
		c.RedisDialTimeout = 5
	}
	if envMissingOrEmpty("REDIS_POOL_TIMEOUT") {
		c.RedisPoolTimeout = 2
	}
	if envMissingOrEmpty("REDIS_MAX_RETRIES") {
		c.RedisMaxRetries = 3
	}
	if envMissingOrEmpty("REDIS_MIN_RETRY_BACKOFF") {
		c.RedisMinRetryBackoff = 8
	}
	if envMissingOrEmpty("REDIS_MAX_RETRY_BACKOFF") {
		c.RedisMaxRetryBackoff = 1
	}
}

func envMissingOrEmpty(key string) bool {
	value, ok := os.LookupEnv(key)
	return !ok || strings.TrimSpace(value) == ""
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

	cfg.ApplyEnvDefaults()

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
	reconmetrics.Init(telemetry.MetricsFactory)

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

	var crmMongoDB *mongo.Database
	if cfg.CRMMongoDBName != "" {
		crmMongoDB = mongoClient.Database(cfg.CRMMongoDBName)
	}

	var redisConn *libRedis.RedisConnection
	if strings.TrimSpace(cfg.RedisHost) != "" {
		redisConn = &libRedis.RedisConnection{
			Address:         strings.Split(cfg.RedisHost, ","),
			Password:        cfg.RedisPassword,
			DB:              cfg.RedisDB,
			Protocol:        cfg.RedisProtocol,
			UseTLS:          cfg.RedisTLS,
			CACert:          cfg.RedisCACert,
			Logger:          logger,
			PoolSize:        cfg.RedisPoolSize,
			ReadTimeout:     time.Duration(cfg.RedisReadTimeout) * time.Second,
			WriteTimeout:    time.Duration(cfg.RedisWriteTimeout) * time.Second,
			DialTimeout:     time.Duration(cfg.RedisDialTimeout) * time.Second,
			PoolTimeout:     time.Duration(cfg.RedisPoolTimeout) * time.Second,
			MaxRetries:      cfg.RedisMaxRetries,
			MinRetryBackoff: time.Duration(cfg.RedisMinRetryBackoff) * time.Millisecond,
			MaxRetryBackoff: time.Duration(cfg.RedisMaxRetryBackoff) * time.Second,
		}
	}

	store := reportstore.NewFileStore(cfg.ReportDir, cfg.ReportMaxFiles, cfg.ReportRetentionDays, logger)

	checkers := []postgres.ReconciliationChecker{
		postgres.NewBalanceChecker(transactionDB),
		postgres.NewDoubleEntryChecker(transactionDB),
		postgres.NewOrphanChecker(transactionDB),
		postgres.NewReferentialChecker(onboardingDB, transactionDB, logger),
		postgres.NewSyncChecker(transactionDB),
		postgres.NewDLQChecker(transactionDB),
		postgres.NewOutboxChecker(transactionDB),
		metadata.NewMetadataChecker(transactionDB, onboardingMongoDB, transactionMongoDB),
		redis.NewRedisChecker(transactionDB, redisConn, logger),
		crossdb.NewCrossDBChecker(onboardingDB, transactionDB),
		crm.NewAliasChecker(onboardingDB, crmMongoDB),
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
		postgres.CheckerNameOutbox: {
			MaxResults:         cfg.MaxDiscrepanciesToReport,
			OutboxStaleSeconds: cfg.OutboxStaleSeconds,
		},
		postgres.CheckerNameMetadata: {
			MaxResults:      cfg.MaxDiscrepanciesToReport,
			LookbackDays:    cfg.MetadataLookbackDays,
			MetadataMaxScan: cfg.MetadataMaxScan,
		},
		postgres.CheckerNameRedis: {
			MaxResults:      cfg.MaxDiscrepanciesToReport,
			RedisSampleSize: cfg.RedisSampleSize,
		},
		postgres.CheckerNameCrossDB: {
			MaxResults:       cfg.MaxDiscrepanciesToReport,
			CrossDBBatchSize: cfg.CrossDBBatchSize,
			CrossDBMaxScan:   cfg.CrossDBMaxScan,
		},
		postgres.CheckerNameCRMAlias: {
			MaxResults:     cfg.MaxDiscrepanciesToReport,
			CrossDBMaxScan: cfg.CrossDBMaxScan,
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
		store,
		reconmetrics.Get(),
	)

	// Initialize worker
	worker := NewReconciliationWorker(eng, logger, cfg)

	// Initialize HTTP server
	httpServer := NewHTTPServer(cfg.GetServerAddress(), eng, logger, telemetry, cfg.Version, cfg.EnvName, store)

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
