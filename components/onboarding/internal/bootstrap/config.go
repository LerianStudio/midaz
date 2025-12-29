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

// InitServers initiate http and grpc servers.
//
//nolint:panicguardwarn // Legacy function used internally; panics are caught by InitServersWithOptions.
func InitServers() *Service {
	cfg := &Config{}

	err := libCommons.SetConfigFromEnvVars(cfg)
	assert.NoError(err, "configuration required for onboarding",
		"package", "bootstrap",
		"function", "InitServers")

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
	ctx := context.Background()
	_, err = migrationWrapper.SafeGetDBWithRetry(ctx)
	if err != nil {
		logger.Errorf("Migration preflight failed for %s: %v - continuing with standard GetDB", ApplicationName, err)
	} else {
		migrationWrapper.UpdateStatusMetrics()
		logger.Infof("Migration preflight successful for %s", ApplicationName)
	}

	mongoSource := fmt.Sprintf("%s://%s:%s@%s:%s/",
		cfg.MongoURI, cfg.MongoDBUser, cfg.MongoDBPassword, cfg.MongoDBHost, cfg.MongoDBPort)

	if cfg.MaxPoolSize <= 0 {
		cfg.MaxPoolSize = 100
	}

	if cfg.MongoDBParameters != "" {
		mongoSource += "?" + cfg.MongoDBParameters
	}

	mongoConnection := &libMongo.MongoConnection{
		ConnectionStringSource: mongoSource,
		Database:               cfg.MongoDBName,
		Logger:                 logger,
		MaxPoolSize:            uint64(cfg.MaxPoolSize),
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

	if cfg.TransactionGRPCAddress == "" || cfg.TransactionGRPCPort == "" {
		panic("TRANSACTION_GRPC_ADDRESS and TRANSACTION_GRPC_PORT must be configured")
	}

	grpcConnection := &mgrpc.GRPCConnection{
		Addr:   fmt.Sprintf("%s:%s", cfg.TransactionGRPCAddress, cfg.TransactionGRPCPort),
		Logger: logger,
	}

	redisConsumerRepository := redis.NewConsumerRedis(redisConnection)

	organizationPostgreSQLRepository := organization.NewOrganizationPostgreSQLRepository(postgresConnection)
	ledgerPostgreSQLRepository := ledger.NewLedgerPostgreSQLRepository(postgresConnection)
	segmentPostgreSQLRepository := segment.NewSegmentPostgreSQLRepository(postgresConnection)
	portfolioPostgreSQLRepository := portfolio.NewPortfolioPostgreSQLRepository(postgresConnection)
	accountPostgreSQLRepository := account.NewAccountPostgreSQLRepository(postgresConnection)
	assetPostgreSQLRepository := asset.NewAssetPostgreSQLRepository(postgresConnection)
	accountTypePostgreSQLRepository := accounttype.NewAccountTypePostgreSQLRepository(postgresConnection)

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
func getMongoMaxPoolSize(cfg *Config) uint64 {
	if cfg.MaxPoolSize <= 0 {
		return defaultMongoMaxPoolSize
	}

	return uint64(cfg.MaxPoolSize)
}

// newMigrationConfig creates a migration configuration from the application config.
func newMigrationConfig(cfg *Config) mmigration.MigrationConfig {
	return mmigration.MigrationConfig{
		AutoRecoverDirty:      cfg.MigrationAutoRecover,
		MaxRetries:            cfg.MigrationMaxRetries,
		MaxRecoveryPerVersion: 3,
		RetryBackoff:          1 * time.Second,
		MaxBackoff:            30 * time.Second,
		LockTimeout:           30 * time.Second,
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
		return nil, fmt.Errorf("failed to create migration wrapper for %s: %w", ApplicationName, err)
	}

	// Perform preflight check with retry for migration safety
	ctx := context.Background()
	_, err = migrationWrapper.SafeGetDBWithRetry(ctx)
	if err != nil {
		logger.Errorf("Migration preflight failed for %s: %v - continuing with standard GetDB", ApplicationName, err)
	} else {
		migrationWrapper.UpdateStatusMetrics()
		logger.Infof("Migration preflight successful for %s", ApplicationName)
	}

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

	organizationPostgreSQLRepository := organization.NewOrganizationPostgreSQLRepository(postgresConnection)
	ledgerPostgreSQLRepository := ledger.NewLedgerPostgreSQLRepository(postgresConnection)
	segmentPostgreSQLRepository := segment.NewSegmentPostgreSQLRepository(postgresConnection)
	portfolioPostgreSQLRepository := portfolio.NewPortfolioPostgreSQLRepository(postgresConnection)
	accountPostgreSQLRepository := account.NewAccountPostgreSQLRepository(postgresConnection)
	assetPostgreSQLRepository := asset.NewAssetPostgreSQLRepository(postgresConnection)
	accountTypePostgreSQLRepository := accounttype.NewAccountTypePostgreSQLRepository(postgresConnection)

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
