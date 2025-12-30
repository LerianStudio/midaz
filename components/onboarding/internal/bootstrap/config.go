// Package bootstrap provides initialization and configuration logic for the
// onboarding service, including dependency injection and server setup.
package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libMongo "github.com/LerianStudio/lib-commons/v2/commons/mongo"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/grpc/out"
	httpin "github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/accounttype"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/organization"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/portfolio"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/segment"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/services/query"
	pkg "github.com/LerianStudio/midaz/v3/pkg"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/mbootstrap"
	"github.com/LerianStudio/midaz/v3/pkg/mgrpc"
	"github.com/LerianStudio/midaz/v3/pkg/mmigration"
)

// ApplicationName is the identifier for the onboarding service used in logging and tracing.
const (
	ApplicationName = "onboarding"
	// indexCreationTimeout is the timeout duration for MongoDB index creation operations
	indexCreationTimeout = 60 * time.Second
)

// Configuration validation constants.
const (
	// maxOpenConnectionsLimit is the maximum allowed value for database open connections.
	maxOpenConnectionsLimit = 10000
	// maxIdleConnectionsLimit is the maximum allowed value for database idle connections.
	maxIdleConnectionsLimit = 5000
	// maxPoolSizeLimit is the maximum allowed value for MongoDB and Redis pool sizes.
	maxPoolSizeLimit = 1000
	// defaultMaxRecoveryPerVersion is the default number of recovery attempts per migration version.
	defaultMaxRecoveryPerVersion = 3
	// defaultMigrationBackoffSeconds is the default backoff duration in seconds for migrations.
	defaultMigrationBackoffSeconds = 30
)

// Sentinel errors for bootstrap initialization.
var (
	// ErrInitializationFailed indicates a panic occurred during initialization.
	ErrInitializationFailed = errors.New("initialization failed")

	// ErrGRPCConfigRequired indicates that gRPC configuration is missing in standalone mode.
	ErrGRPCConfigRequired = errors.New("TRANSACTION_GRPC_ADDRESS and TRANSACTION_GRPC_PORT must be configured in standalone mode")
)

// Config is the top level configuration struct for the entire application.
type Config struct {
	EnvName                      string `env:"ENV_NAME"`
	LogLevel                     string `env:"LOG_LEVEL"`
	ServerAddress                string `env:"SERVER_ADDRESS"`
	PrimaryDBHost                string `env:"DB_HOST"`
	PrimaryDBUser                string `env:"DB_USER"`
	PrimaryDBPassword            string `env:"DB_PASSWORD"`
	PrimaryDBName                string `env:"DB_NAME"`
	PrimaryDBPort                string `env:"DB_PORT"`
	PrimaryDBSSLMode             string `env:"DB_SSLMODE"`
	ReplicaDBHost                string `env:"DB_REPLICA_HOST"`
	ReplicaDBUser                string `env:"DB_REPLICA_USER"`
	ReplicaDBPassword            string `env:"DB_REPLICA_PASSWORD"`
	ReplicaDBName                string `env:"DB_REPLICA_NAME"`
	ReplicaDBPort                string `env:"DB_REPLICA_PORT"`
	ReplicaDBSSLMode             string `env:"DB_REPLICA_SSLMODE"`
	MaxOpenConnections           int    `env:"DB_MAX_OPEN_CONNS"`
	MaxIdleConnections           int    `env:"DB_MAX_IDLE_CONNS"`
	MongoURI                     string `env:"MONGO_URI"`
	MongoDBHost                  string `env:"MONGO_HOST"`
	MongoDBName                  string `env:"MONGO_NAME"`
	MongoDBUser                  string `env:"MONGO_USER"`
	MongoDBPassword              string `env:"MONGO_PASSWORD"`
	MongoDBPort                  string `env:"MONGO_PORT"`
	MongoDBParameters            string `env:"MONGO_PARAMETERS"`
	MaxPoolSize                  int    `env:"MONGO_MAX_POOL_SIZE"`
	JWKAddress                   string `env:"CASDOOR_JWK_ADDRESS"`
	OtelServiceName              string `env:"OTEL_RESOURCE_SERVICE_NAME"`
	OtelLibraryName              string `env:"OTEL_LIBRARY_NAME"`
	OtelServiceVersion           string `env:"OTEL_RESOURCE_SERVICE_VERSION"`
	OtelDeploymentEnv            string `env:"OTEL_RESOURCE_DEPLOYMENT_ENVIRONMENT"`
	OtelColExporterEndpoint      string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	EnableTelemetry              bool   `env:"ENABLE_TELEMETRY"`
	RedisHost                    string `env:"REDIS_HOST"`
	RedisMasterName              string `env:"REDIS_MASTER_NAME" default:""`
	RedisPassword                string `env:"REDIS_PASSWORD"`
	RedisDB                      int    `env:"REDIS_DB" default:"0"`
	RedisProtocol                int    `env:"REDIS_PROTOCOL" default:"3"`
	RedisTLS                     bool   `env:"REDIS_TLS" default:"false"`
	RedisCACert                  string `env:"REDIS_CA_CERT"`
	RedisUseGCPIAM               bool   `env:"REDIS_USE_GCP_IAM" default:"false"`
	RedisServiceAccount          string `env:"REDIS_SERVICE_ACCOUNT" default:""`
	GoogleApplicationCredentials string `env:"GOOGLE_APPLICATION_CREDENTIALS" default:""`
	RedisTokenLifeTime           int    `env:"REDIS_TOKEN_LIFETIME" default:"60"`
	RedisTokenRefreshDuration    int    `env:"REDIS_TOKEN_REFRESH_DURATION" default:"45"`
	RedisPoolSize                int    `env:"REDIS_POOL_SIZE" default:"10"`
	RedisMinIdleConns            int    `env:"REDIS_MIN_IDLE_CONNS" default:"0"`
	RedisReadTimeout             int    `env:"REDIS_READ_TIMEOUT" default:"3"`
	RedisWriteTimeout            int    `env:"REDIS_WRITE_TIMEOUT" default:"3"`
	RedisDialTimeout             int    `env:"REDIS_DIAL_TIMEOUT" default:"5"`
	RedisPoolTimeout             int    `env:"REDIS_POOL_TIMEOUT" default:"2"`
	RedisMaxRetries              int    `env:"REDIS_MAX_RETRIES" default:"3"`
	RedisMinRetryBackoff         int    `env:"REDIS_MIN_RETRY_BACKOFF" default:"8"`
	RedisMaxRetryBackoff         int    `env:"REDIS_MAX_RETRY_BACKOFF" default:"1"`
	AuthEnabled                  bool   `env:"PLUGIN_AUTH_ENABLED"`
	AuthHost                     string `env:"PLUGIN_AUTH_HOST"`
	TransactionGRPCAddress       string `env:"TRANSACTION_GRPC_ADDRESS"`
	TransactionGRPCPort          string `env:"TRANSACTION_GRPC_PORT"`
	// Migration auto-recovery configuration
	MigrationAutoRecover bool `env:"MIGRATION_AUTO_RECOVER" default:"true"`
	MigrationMaxRetries  int  `env:"MIGRATION_MAX_RETRIES" default:"3"`
}

// Validate validates the configuration and panics with clear error messages if invalid.
// This method should be called immediately after loading configuration from environment.
func (cfg *Config) Validate() {
	// Server configuration
	assert.NotEmpty(cfg.ServerAddress, "SERVER_ADDRESS is required",
		"field", "ServerAddress")

	// Primary database configuration
	assert.NotEmpty(cfg.PrimaryDBHost, "DB_HOST is required",
		"field", "PrimaryDBHost")
	assert.NotEmpty(cfg.PrimaryDBUser, "DB_USER is required",
		"field", "PrimaryDBUser")
	assert.NotEmpty(cfg.PrimaryDBName, "DB_NAME is required",
		"field", "PrimaryDBName")
	assert.That(assert.ValidPort(cfg.PrimaryDBPort), "DB_PORT must be valid port (1-65535)",
		"field", "PrimaryDBPort", "value", cfg.PrimaryDBPort)
	assert.That(assert.ValidSSLMode(cfg.PrimaryDBSSLMode), "DB_SSLMODE must be valid PostgreSQL SSL mode",
		"field", "PrimaryDBSSLMode", "value", cfg.PrimaryDBSSLMode)

	// Replica database configuration
	assert.NotEmpty(cfg.ReplicaDBHost, "DB_REPLICA_HOST is required",
		"field", "ReplicaDBHost")
	assert.NotEmpty(cfg.ReplicaDBUser, "DB_REPLICA_USER is required",
		"field", "ReplicaDBUser")
	assert.NotEmpty(cfg.ReplicaDBName, "DB_REPLICA_NAME is required",
		"field", "ReplicaDBName")
	assert.That(assert.ValidPort(cfg.ReplicaDBPort), "DB_REPLICA_PORT must be valid port (1-65535)",
		"field", "ReplicaDBPort", "value", cfg.ReplicaDBPort)
	assert.That(assert.ValidSSLMode(cfg.ReplicaDBSSLMode), "DB_REPLICA_SSLMODE must be valid PostgreSQL SSL mode",
		"field", "ReplicaDBSSLMode", "value", cfg.ReplicaDBSSLMode)

	// Database pool configuration
	assert.That(assert.InRangeInt(cfg.MaxOpenConnections, 1, maxOpenConnectionsLimit), "DB_MAX_OPEN_CONNS must be 1-10000",
		"field", "MaxOpenConnections", "value", cfg.MaxOpenConnections)
	assert.That(assert.InRangeInt(cfg.MaxIdleConnections, 1, maxIdleConnectionsLimit), "DB_MAX_IDLE_CONNS must be 1-5000",
		"field", "MaxIdleConnections", "value", cfg.MaxIdleConnections)

	// MongoDB configuration
	assert.NotEmpty(cfg.MongoDBHost, "MONGO_HOST is required",
		"field", "MongoDBHost")
	assert.NotEmpty(cfg.MongoDBName, "MONGO_NAME is required",
		"field", "MongoDBName")
	assert.That(assert.ValidPort(cfg.MongoDBPort), "MONGO_PORT must be valid port (1-65535)",
		"field", "MongoDBPort", "value", cfg.MongoDBPort)
	assert.That(assert.InRangeInt(cfg.MaxPoolSize, 1, maxPoolSizeLimit), "MONGO_MAX_POOL_SIZE must be 1-1000",
		"field", "MaxPoolSize", "value", cfg.MaxPoolSize)

	// Redis configuration
	assert.NotEmpty(cfg.RedisHost, "REDIS_HOST is required",
		"field", "RedisHost")
	assert.That(assert.InRangeInt(cfg.RedisPoolSize, 1, maxPoolSizeLimit), "REDIS_POOL_SIZE must be 1-1000",
		"field", "RedisPoolSize", "value", cfg.RedisPoolSize)

	// gRPC configuration (required for transaction service communication)
	assert.NotEmpty(cfg.TransactionGRPCAddress, "TRANSACTION_GRPC_ADDRESS is required",
		"field", "TransactionGRPCAddress")
	assert.That(assert.ValidPort(cfg.TransactionGRPCPort), "TRANSACTION_GRPC_PORT must be valid port (1-65535)",
		"field", "TransactionGRPCPort", "value", cfg.TransactionGRPCPort)
}

// InitServers initiate http and grpc servers.
func InitServers() *Service {
	cfg := &Config{}

	err := libCommons.SetConfigFromEnvVars(cfg)
	assert.NoError(err, "configuration required for onboarding",
		"package", "bootstrap",
		"function", "InitServers")

	// Validate configuration before proceeding
	cfg.Validate()

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

	postgreSourcePrimary := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s",
		cfg.PrimaryDBHost, cfg.PrimaryDBUser, cfg.PrimaryDBPassword, cfg.PrimaryDBName, cfg.PrimaryDBPort, cfg.PrimaryDBSSLMode)

	postgreSourceReplica := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s",
		cfg.ReplicaDBHost, cfg.ReplicaDBUser, cfg.ReplicaDBPassword, cfg.ReplicaDBName, cfg.ReplicaDBPort, cfg.ReplicaDBSSLMode)

	postgresConnection := &libPostgres.PostgresConnection{
		ConnectionStringPrimary: postgreSourcePrimary,
		ConnectionStringReplica: postgreSourceReplica,
		PrimaryDBName:           cfg.PrimaryDBName,
		ReplicaDBName:           cfg.ReplicaDBName,
		Component:               ApplicationName,
		Logger:                  logger,
		MaxOpenConnections:      cfg.MaxOpenConnections,
		MaxIdleConnections:      cfg.MaxIdleConnections,
	}

	// Create migration wrapper for safe database access with auto-recovery
	migrationWrapper, err := mmigration.NewMigrationWrapper(postgresConnection, newMigrationConfig(cfg), logger)
	if err != nil {
		logger.Fatalf("Failed to create migration wrapper for %s: %v", ApplicationName, err)
	}

	// Perform preflight check with retry for migration safety
	// CRITICAL: Fail fast on migration errors - do not continue with broken database state
	ctx := context.Background()

	_, err = migrationWrapper.SafeGetDBWithRetry(ctx)
	if err != nil {
		logger.Fatalf("Migration preflight failed for %s: %v - cannot proceed with broken database state", ApplicationName, err)
	}

	migrationWrapper.UpdateStatusMetrics()
	logger.Infof("Migration preflight successful for %s", ApplicationName)

	mongoSource := fmt.Sprintf("%s://%s:%s@%s:%s/",
		cfg.MongoURI, cfg.MongoDBUser, cfg.MongoDBPassword, cfg.MongoDBHost, cfg.MongoDBPort)

	// Note: MaxPoolSize validated in cfg.Validate() above (must be 1-1000)

	if cfg.MongoDBParameters != "" {
		mongoSource += "?" + cfg.MongoDBParameters
	}

	mongoConnection := &libMongo.MongoConnection{
		ConnectionStringSource: mongoSource,
		Database:               cfg.MongoDBName,
		Logger:                 logger,
		MaxPoolSize:            getMongoMaxPoolSize(cfg),
	}

	redisConnection := &libRedis.RedisConnection{
		Address:                      strings.Split(cfg.RedisHost, ","),
		Password:                     cfg.RedisPassword,
		DB:                           cfg.RedisDB,
		Protocol:                     cfg.RedisProtocol,
		MasterName:                   cfg.RedisMasterName,
		UseTLS:                       cfg.RedisTLS,
		CACert:                       cfg.RedisCACert,
		UseGCPIAMAuth:                cfg.RedisUseGCPIAM,
		ServiceAccount:               cfg.RedisServiceAccount,
		GoogleApplicationCredentials: cfg.GoogleApplicationCredentials,
		TokenLifeTime:                time.Duration(cfg.RedisTokenLifeTime) * time.Minute,
		RefreshDuration:              time.Duration(cfg.RedisTokenRefreshDuration) * time.Minute,
		Logger:                       logger,
		PoolSize:                     cfg.RedisPoolSize,
		MinIdleConns:                 cfg.RedisMinIdleConns,
		ReadTimeout:                  time.Duration(cfg.RedisReadTimeout) * time.Second,
		WriteTimeout:                 time.Duration(cfg.RedisWriteTimeout) * time.Second,
		DialTimeout:                  time.Duration(cfg.RedisDialTimeout) * time.Second,
		PoolTimeout:                  time.Duration(cfg.RedisPoolTimeout) * time.Second,
		MaxRetries:                   cfg.RedisMaxRetries,
		MinRetryBackoff:              time.Duration(cfg.RedisMinRetryBackoff) * time.Millisecond,
		MaxRetryBackoff:              time.Duration(cfg.RedisMaxRetryBackoff) * time.Second,
	}

	// Note: gRPC config validated in cfg.Validate() above

	grpcConnection := &mgrpc.GRPCConnection{
		Addr:   fmt.Sprintf("%s:%s", cfg.TransactionGRPCAddress, cfg.TransactionGRPCPort),
		Logger: logger,
	}

	redisConsumerRepository := redis.NewConsumerRedis(redisConnection)

	organizationPostgreSQLRepository := organization.NewOrganizationPostgreSQLRepository(migrationWrapper)
	ledgerPostgreSQLRepository := ledger.NewLedgerPostgreSQLRepository(migrationWrapper)
	segmentPostgreSQLRepository := segment.NewSegmentPostgreSQLRepository(migrationWrapper)
	portfolioPostgreSQLRepository := portfolio.NewPortfolioPostgreSQLRepository(migrationWrapper)
	accountPostgreSQLRepository := account.NewAccountPostgreSQLRepository(migrationWrapper)
	assetPostgreSQLRepository := asset.NewAssetPostgreSQLRepository(migrationWrapper)
	accountTypePostgreSQLRepository := accounttype.NewAccountTypePostgreSQLRepository(migrationWrapper)

	metadataMongoDBRepository := mongodb.NewMetadataMongoDBRepository(mongoConnection)

	// Ensure indexes also for known base collections on fresh installs
	ctxEnsureIndexes, cancelEnsureIndexes := context.WithTimeout(context.Background(), indexCreationTimeout)
	defer cancelEnsureIndexes()

	indexModel := mongo.IndexModel{
		Keys: bson.D{{Key: "entity_id", Value: 1}},
		Options: options.Index().
			SetUnique(false),
	}

	collections := []string{"organization", "ledger", "segment", "account", "portfolio", "asset", "account_type"}
	for _, collection := range collections {
		if err := mongoConnection.EnsureIndexes(ctxEnsureIndexes, collection, indexModel); err != nil {
			logger.Warnf("Failed to ensure indexes for collection %s: %v", collection, err)
		}
	}

	balanceGRPCRepository := out.NewBalanceAdapter(grpcConnection)

	commandUseCase := &command.UseCase{
		OrganizationRepo: organizationPostgreSQLRepository,
		LedgerRepo:       ledgerPostgreSQLRepository,
		SegmentRepo:      segmentPostgreSQLRepository,
		PortfolioRepo:    portfolioPostgreSQLRepository,
		AccountRepo:      accountPostgreSQLRepository,
		AssetRepo:        assetPostgreSQLRepository,
		AccountTypeRepo:  accountTypePostgreSQLRepository,
		MetadataRepo:     metadataMongoDBRepository,
		RedisRepo:        redisConsumerRepository,
		BalancePort:      balanceGRPCRepository,
	}

	queryUseCase := &query.UseCase{
		OrganizationRepo: organizationPostgreSQLRepository,
		LedgerRepo:       ledgerPostgreSQLRepository,
		SegmentRepo:      segmentPostgreSQLRepository,
		PortfolioRepo:    portfolioPostgreSQLRepository,
		AccountRepo:      accountPostgreSQLRepository,
		AssetRepo:        assetPostgreSQLRepository,
		AccountTypeRepo:  accountTypePostgreSQLRepository,
		MetadataRepo:     metadataMongoDBRepository,
		RedisRepo:        redisConsumerRepository,
	}

	accountHandler := &httpin.AccountHandler{
		Command: commandUseCase,
		Query:   queryUseCase,
	}

	portfolioHandler := &httpin.PortfolioHandler{
		Command: commandUseCase,
		Query:   queryUseCase,
	}

	ledgerHandler := &httpin.LedgerHandler{
		Command: commandUseCase,
		Query:   queryUseCase,
	}

	assetHandler := &httpin.AssetHandler{
		Command: commandUseCase,
		Query:   queryUseCase,
	}

	organizationHandler := &httpin.OrganizationHandler{
		Command: commandUseCase,
		Query:   queryUseCase,
	}

	segmentHandler := &httpin.SegmentHandler{
		Command: commandUseCase,
		Query:   queryUseCase,
	}

	accountTypeHandler := &httpin.AccountTypeHandler{
		Command: commandUseCase,
		Query:   queryUseCase,
	}

	auth := middleware.NewAuthClient(cfg.AuthHost, cfg.AuthEnabled, &logger)

	httpApp := httpin.NewRouter(logger, telemetry, auth, accountHandler, portfolioHandler, ledgerHandler, assetHandler, organizationHandler, segmentHandler, accountTypeHandler)

	serverAPI := NewServer(cfg, httpApp, logger, telemetry)

	return &Service{
		Server:              serverAPI,
		Logger:              logger,
		auth:                auth,
		accountHandler:      accountHandler,
		portfolioHandler:    portfolioHandler,
		ledgerHandler:       ledgerHandler,
		assetHandler:        assetHandler,
		organizationHandler: organizationHandler,
		segmentHandler:      segmentHandler,
		accountTypeHandler:  accountTypeHandler,
	}
}

// Options configures the onboarding service initialization behavior.
type Options struct {
	// Logger allows callers to provide a pre-configured logger.
	Logger libLog.Logger

	// UnifiedMode indicates the service is running as part of the unified ledger.
	UnifiedMode bool

	// BalancePort enables direct in-process communication with the transaction module.
	BalancePort mbootstrap.BalancePort
}

// buildMongoSource builds the MongoDB connection string from configuration.
func buildMongoSource(cfg *Config) string {
	mongoSource := fmt.Sprintf("%s://%s:%s@%s:%s/",
		cfg.MongoURI, cfg.MongoDBUser, cfg.MongoDBPassword, cfg.MongoDBHost, cfg.MongoDBPort)

	if cfg.MongoDBParameters != "" {
		mongoSource += "?" + cfg.MongoDBParameters
	}

	return mongoSource
}

// defaultMongoMaxPoolSize is the default MongoDB connection pool size.
const defaultMongoMaxPoolSize = 100

// getMongoMaxPoolSize returns the max pool size, defaulting to defaultMongoMaxPoolSize if not set or invalid.
// The function validates bounds to prevent integer overflow during int to uint64 conversion.
func getMongoMaxPoolSize(cfg *Config) uint64 {
	if cfg.MaxPoolSize <= 0 || cfg.MaxPoolSize > maxPoolSizeLimit {
		return defaultMongoMaxPoolSize
	}

	return uint64(cfg.MaxPoolSize)
}

// newMigrationConfig creates a migration configuration from the application config.
func newMigrationConfig(cfg *Config) mmigration.MigrationConfig {
	return mmigration.MigrationConfig{
		AutoRecoverDirty:      cfg.MigrationAutoRecover,
		MaxRetries:            cfg.MigrationMaxRetries,
		MaxRecoveryPerVersion: defaultMaxRecoveryPerVersion,
		RetryBackoff:          1 * time.Second,
		MaxBackoff:            defaultMigrationBackoffSeconds * time.Second,
		LockTimeout:           defaultMigrationBackoffSeconds * time.Second,
		Component:             ApplicationName,
		MigrationsPath:        "/app/components/onboarding/migrations",
	}
}

// resolveBalancePort determines the balance port based on mode and configuration.
func resolveBalancePort(opts *Options, cfg *Config, logger libLog.Logger) (mbootstrap.BalancePort, error) {
	if opts.UnifiedMode && opts.BalancePort != nil {
		return opts.BalancePort, nil
	}

	if cfg.TransactionGRPCAddress == "" || cfg.TransactionGRPCPort == "" {
		return nil, pkg.ValidateInternalError(ErrGRPCConfigRequired, "BalancePort")
	}

	grpcConnection := &mgrpc.GRPCConnection{
		Addr:   fmt.Sprintf("%s:%s", cfg.TransactionGRPCAddress, cfg.TransactionGRPCPort),
		Logger: logger,
	}

	return out.NewBalanceAdapter(grpcConnection), nil
}

// InitServersWithOptions initializes servers with custom options.
// This function provides explicit error handling and supports unified mode.
// It recovers from panics (e.g., from assert.NoError in constructors) and converts them to errors.
func InitServersWithOptions(opts *Options) (service *Service, err error) {
	// Panic recovery to convert assertion panics from constructors to errors.
	// Per CLAUDE.md: "Only initialization-time panics allowed (repository constructors)"
	// This allows constructors to keep their assertions while InitServiceOrError returns errors.
	defer func() {
		if r := recover(); r != nil {
			service = nil
			err = fmt.Errorf("%w: %v", ErrInitializationFailed, r)
		}
	}()

	if opts == nil {
		return InitServers(), nil
	}

	cfg := &Config{}

	err = libCommons.SetConfigFromEnvVars(cfg)
	if err != nil {
		return nil, pkg.ValidateInternalError(err, "Config")
	}

	// Validate configuration before proceeding
	cfg.Validate()

	// Use provided logger or initialize a new one
	logger := opts.Logger
	if logger == nil {
		logger = libZap.InitializeLogger()
	}

	telemetry := libOpentelemetry.InitializeTelemetry(&libOpentelemetry.TelemetryConfig{
		LibraryName:               cfg.OtelLibraryName,
		ServiceName:               cfg.OtelServiceName,
		ServiceVersion:            cfg.OtelServiceVersion,
		DeploymentEnv:             cfg.OtelDeploymentEnv,
		CollectorExporterEndpoint: cfg.OtelColExporterEndpoint,
		EnableTelemetry:           cfg.EnableTelemetry,
		Logger:                    logger,
	})

	postgreSourcePrimary := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s",
		cfg.PrimaryDBHost, cfg.PrimaryDBUser, cfg.PrimaryDBPassword, cfg.PrimaryDBName, cfg.PrimaryDBPort, cfg.PrimaryDBSSLMode)

	postgreSourceReplica := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s",
		cfg.ReplicaDBHost, cfg.ReplicaDBUser, cfg.ReplicaDBPassword, cfg.ReplicaDBName, cfg.ReplicaDBPort, cfg.ReplicaDBSSLMode)

	postgresConnection := &libPostgres.PostgresConnection{
		ConnectionStringPrimary: postgreSourcePrimary,
		ConnectionStringReplica: postgreSourceReplica,
		PrimaryDBName:           cfg.PrimaryDBName,
		ReplicaDBName:           cfg.ReplicaDBName,
		Component:               ApplicationName,
		Logger:                  logger,
		MaxOpenConnections:      cfg.MaxOpenConnections,
		MaxIdleConnections:      cfg.MaxIdleConnections,
	}

	// Create migration wrapper for safe database access with auto-recovery
	migrationWrapper, err := mmigration.NewMigrationWrapper(postgresConnection, newMigrationConfig(cfg), logger)
	if err != nil {
		return nil, pkg.ValidateInternalError(err, "MigrationWrapper")
	}

	// Perform preflight check with retry for migration safety
	// CRITICAL: Fail fast on migration errors - do not continue with broken database state
	ctx := context.Background()

	_, err = migrationWrapper.SafeGetDBWithRetry(ctx)
	if err != nil {
		logger.Fatalf("Migration preflight failed for %s: %v - cannot proceed with broken database state", ApplicationName, err)
	}

	migrationWrapper.UpdateStatusMetrics()
	logger.Infof("Migration preflight successful for %s", ApplicationName)

	mongoConnection := &libMongo.MongoConnection{
		ConnectionStringSource: buildMongoSource(cfg),
		Database:               cfg.MongoDBName,
		Logger:                 logger,
		MaxPoolSize:            getMongoMaxPoolSize(cfg),
	}

	redisConnection := &libRedis.RedisConnection{
		Address:                      strings.Split(cfg.RedisHost, ","),
		Password:                     cfg.RedisPassword,
		DB:                           cfg.RedisDB,
		Protocol:                     cfg.RedisProtocol,
		MasterName:                   cfg.RedisMasterName,
		UseTLS:                       cfg.RedisTLS,
		CACert:                       cfg.RedisCACert,
		UseGCPIAMAuth:                cfg.RedisUseGCPIAM,
		ServiceAccount:               cfg.RedisServiceAccount,
		GoogleApplicationCredentials: cfg.GoogleApplicationCredentials,
		TokenLifeTime:                time.Duration(cfg.RedisTokenLifeTime) * time.Minute,
		RefreshDuration:              time.Duration(cfg.RedisTokenRefreshDuration) * time.Minute,
		Logger:                       logger,
		PoolSize:                     cfg.RedisPoolSize,
		MinIdleConns:                 cfg.RedisMinIdleConns,
		ReadTimeout:                  time.Duration(cfg.RedisReadTimeout) * time.Second,
		WriteTimeout:                 time.Duration(cfg.RedisWriteTimeout) * time.Second,
		DialTimeout:                  time.Duration(cfg.RedisDialTimeout) * time.Second,
		PoolTimeout:                  time.Duration(cfg.RedisPoolTimeout) * time.Second,
		MaxRetries:                   cfg.RedisMaxRetries,
		MinRetryBackoff:              time.Duration(cfg.RedisMinRetryBackoff) * time.Millisecond,
		MaxRetryBackoff:              time.Duration(cfg.RedisMaxRetryBackoff) * time.Second,
	}

	redisConsumerRepository := redis.NewConsumerRedis(redisConnection)

	organizationPostgreSQLRepository := organization.NewOrganizationPostgreSQLRepository(migrationWrapper)
	ledgerPostgreSQLRepository := ledger.NewLedgerPostgreSQLRepository(migrationWrapper)
	segmentPostgreSQLRepository := segment.NewSegmentPostgreSQLRepository(migrationWrapper)
	portfolioPostgreSQLRepository := portfolio.NewPortfolioPostgreSQLRepository(migrationWrapper)
	accountPostgreSQLRepository := account.NewAccountPostgreSQLRepository(migrationWrapper)
	assetPostgreSQLRepository := asset.NewAssetPostgreSQLRepository(migrationWrapper)
	accountTypePostgreSQLRepository := accounttype.NewAccountTypePostgreSQLRepository(migrationWrapper)

	metadataMongoDBRepository := mongodb.NewMetadataMongoDBRepository(mongoConnection)

	// Ensure indexes also for known base collections on fresh installs
	ctxEnsureIndexes, cancelEnsureIndexes := context.WithTimeout(context.Background(), indexCreationTimeout)
	defer cancelEnsureIndexes()

	indexModel := mongo.IndexModel{
		Keys: bson.D{{Key: "entity_id", Value: 1}},
		Options: options.Index().
			SetUnique(false),
	}

	collections := []string{"organization", "ledger", "segment", "account", "portfolio", "asset", "account_type"}
	for _, collection := range collections {
		if err := mongoConnection.EnsureIndexes(ctxEnsureIndexes, collection, indexModel); err != nil {
			logger.Warnf("Failed to ensure indexes for collection %s: %v", collection, err)
		}
	}

	// Determine balance port: use provided port (unified mode) or gRPC adapter (standalone mode)
	balancePort, err := resolveBalancePort(opts, cfg, logger)
	if err != nil {
		return nil, err
	}

	commandUseCase := &command.UseCase{
		OrganizationRepo: organizationPostgreSQLRepository,
		LedgerRepo:       ledgerPostgreSQLRepository,
		SegmentRepo:      segmentPostgreSQLRepository,
		PortfolioRepo:    portfolioPostgreSQLRepository,
		AccountRepo:      accountPostgreSQLRepository,
		AssetRepo:        assetPostgreSQLRepository,
		AccountTypeRepo:  accountTypePostgreSQLRepository,
		MetadataRepo:     metadataMongoDBRepository,
		RedisRepo:        redisConsumerRepository,
		BalancePort:      balancePort,
	}

	queryUseCase := &query.UseCase{
		OrganizationRepo: organizationPostgreSQLRepository,
		LedgerRepo:       ledgerPostgreSQLRepository,
		SegmentRepo:      segmentPostgreSQLRepository,
		PortfolioRepo:    portfolioPostgreSQLRepository,
		AccountRepo:      accountPostgreSQLRepository,
		AssetRepo:        assetPostgreSQLRepository,
		AccountTypeRepo:  accountTypePostgreSQLRepository,
		MetadataRepo:     metadataMongoDBRepository,
		RedisRepo:        redisConsumerRepository,
	}

	accountHandler := &httpin.AccountHandler{
		Command: commandUseCase,
		Query:   queryUseCase,
	}

	portfolioHandler := &httpin.PortfolioHandler{
		Command: commandUseCase,
		Query:   queryUseCase,
	}

	ledgerHandler := &httpin.LedgerHandler{
		Command: commandUseCase,
		Query:   queryUseCase,
	}

	assetHandler := &httpin.AssetHandler{
		Command: commandUseCase,
		Query:   queryUseCase,
	}

	organizationHandler := &httpin.OrganizationHandler{
		Command: commandUseCase,
		Query:   queryUseCase,
	}

	segmentHandler := &httpin.SegmentHandler{
		Command: commandUseCase,
		Query:   queryUseCase,
	}

	accountTypeHandler := &httpin.AccountTypeHandler{
		Command: commandUseCase,
		Query:   queryUseCase,
	}

	auth := middleware.NewAuthClient(cfg.AuthHost, cfg.AuthEnabled, &logger)

	httpApp := httpin.NewRouter(logger, telemetry, auth, accountHandler, portfolioHandler, ledgerHandler, assetHandler, organizationHandler, segmentHandler, accountTypeHandler)

	serverAPI := NewServer(cfg, httpApp, logger, telemetry)

	return &Service{
		Server: serverAPI,
		Logger: logger,
	}, nil
}
