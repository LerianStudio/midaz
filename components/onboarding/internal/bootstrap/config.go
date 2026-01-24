package bootstrap

import (
	"context"
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
	grpcout "github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/grpc/out"
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
	"github.com/LerianStudio/midaz/v3/pkg/mbootstrap"
	"github.com/LerianStudio/midaz/v3/pkg/mgrpc"
	pkgMongo "github.com/LerianStudio/midaz/v3/pkg/mongo"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const ApplicationName = "onboarding"

// envFallback returns the prefixed value if not empty, otherwise returns the fallback value.
func envFallback(prefixed, fallback string) string {
	if prefixed != "" {
		return prefixed
	}

	return fallback
}

// envFallbackInt returns the prefixed value if not zero, otherwise returns the fallback value.
func envFallbackInt(prefixed, fallback int) int {
	if prefixed != 0 {
		return prefixed
	}

	return fallback
}

// Config is the top level configuration struct for the entire application.
// Supports prefixed env vars (DB_ONBOARDING_*) with fallback to non-prefixed (DB_*) for backward compatibility.
type Config struct {
	EnvName  string `env:"ENV_NAME"`
	LogLevel string `env:"LOG_LEVEL"`

	// Server address - prefixed for unified ledger deployment
	PrefixedServerAddress string `env:"SERVER_ADDRESS_ONBOARDING"`
	ServerAddress         string `env:"SERVER_ADDRESS"`

	// PostgreSQL Primary - prefixed vars for unified ledger deployment
	PrefixedPrimaryDBHost     string `env:"DB_ONBOARDING_HOST"`
	PrefixedPrimaryDBUser     string `env:"DB_ONBOARDING_USER"`
	PrefixedPrimaryDBPassword string `env:"DB_ONBOARDING_PASSWORD"`
	PrefixedPrimaryDBName     string `env:"DB_ONBOARDING_NAME"`
	PrefixedPrimaryDBPort     string `env:"DB_ONBOARDING_PORT"`
	PrefixedPrimaryDBSSLMode  string `env:"DB_ONBOARDING_SSLMODE"`

	// PostgreSQL Primary - fallback vars for standalone deployment
	PrimaryDBHost     string `env:"DB_HOST"`
	PrimaryDBUser     string `env:"DB_USER"`
	PrimaryDBPassword string `env:"DB_PASSWORD"`
	PrimaryDBName     string `env:"DB_NAME"`
	PrimaryDBPort     string `env:"DB_PORT"`
	PrimaryDBSSLMode  string `env:"DB_SSLMODE"`

	// PostgreSQL Replica - prefixed vars for unified ledger deployment
	PrefixedReplicaDBHost     string `env:"DB_ONBOARDING_REPLICA_HOST"`
	PrefixedReplicaDBUser     string `env:"DB_ONBOARDING_REPLICA_USER"`
	PrefixedReplicaDBPassword string `env:"DB_ONBOARDING_REPLICA_PASSWORD"`
	PrefixedReplicaDBName     string `env:"DB_ONBOARDING_REPLICA_NAME"`
	PrefixedReplicaDBPort     string `env:"DB_ONBOARDING_REPLICA_PORT"`
	PrefixedReplicaDBSSLMode  string `env:"DB_ONBOARDING_REPLICA_SSLMODE"`

	// PostgreSQL Replica - fallback vars for standalone deployment
	ReplicaDBHost     string `env:"DB_REPLICA_HOST"`
	ReplicaDBUser     string `env:"DB_REPLICA_USER"`
	ReplicaDBPassword string `env:"DB_REPLICA_PASSWORD"`
	ReplicaDBName     string `env:"DB_REPLICA_NAME"`
	ReplicaDBPort     string `env:"DB_REPLICA_PORT"`
	ReplicaDBSSLMode  string `env:"DB_REPLICA_SSLMODE"`

	// PostgreSQL connection pool - prefixed with fallback
	PrefixedMaxOpenConnections int `env:"DB_ONBOARDING_MAX_OPEN_CONNS"`
	PrefixedMaxIdleConnections int `env:"DB_ONBOARDING_MAX_IDLE_CONNS"`
	MaxOpenConnections         int `env:"DB_MAX_OPEN_CONNS"`
	MaxIdleConnections         int `env:"DB_MAX_IDLE_CONNS"`

	// MongoDB - prefixed vars for unified ledger deployment
	PrefixedMongoURI          string `env:"MONGO_ONBOARDING_URI"`
	PrefixedMongoDBHost       string `env:"MONGO_ONBOARDING_HOST"`
	PrefixedMongoDBName       string `env:"MONGO_ONBOARDING_NAME"`
	PrefixedMongoDBUser       string `env:"MONGO_ONBOARDING_USER"`
	PrefixedMongoDBPassword   string `env:"MONGO_ONBOARDING_PASSWORD"`
	PrefixedMongoDBPort       string `env:"MONGO_ONBOARDING_PORT"`
	PrefixedMongoDBParameters string `env:"MONGO_ONBOARDING_PARAMETERS"`
	PrefixedMaxPoolSize       int    `env:"MONGO_ONBOARDING_MAX_POOL_SIZE"`

	// MongoDB - fallback vars for standalone deployment
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
	RedisProtocol                int    `env:"REDIS_DB" default:"3"`
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
}

// Options contains optional dependencies that can be injected when running
// in unified ledger mode. When nil, defaults to gRPC-based communication.
type Options struct {
	// Logger allows callers (e.g. cmd/app) to provide a pre-configured logger,
	// avoiding double initialization and ensuring consistent output.
	Logger libLog.Logger

	// UnifiedMode indicates the service is running as part of the unified ledger.
	// When true, all ports must be provided for in-process communication.
	// When false (or Options is nil), uses gRPC adapters for remote communication.
	UnifiedMode bool

	// BalancePort is the transaction module's BalancePort for direct calls.
	// Required when UnifiedMode is true.
	// This is typically the transaction.UseCase which implements mbootstrap.BalancePort.
	BalancePort mbootstrap.BalancePort
}

// InitServers initiate http and grpc servers using default gRPC communication.
func InitServers() (*Service, error) {
	return InitServersWithOptions(nil)
}

// InitServersWithOptions initiates http servers with optional dependency injection.
// When opts is nil or opts.UnifiedMode is false, uses gRPC for balance operations.
// When opts.UnifiedMode is true, uses direct in-process calls (unified ledger mode).
func InitServersWithOptions(opts *Options) (*Service, error) {
	cfg := &Config{}

	if err := libCommons.SetConfigFromEnvVars(cfg); err != nil {
		return nil, fmt.Errorf("failed to load config from environment variables: %w", err)
	}

	var logger libLog.Logger
	if opts != nil && opts.Logger != nil {
		logger = opts.Logger
	} else {
		var err error

		logger, err = libZap.InitializeLoggerWithError()
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
		Logger:                    logger,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize telemetry: %w", err)
	}

	// Apply fallback for prefixed env vars (unified ledger) to non-prefixed (standalone)
	dbHost := envFallback(cfg.PrefixedPrimaryDBHost, cfg.PrimaryDBHost)
	dbUser := envFallback(cfg.PrefixedPrimaryDBUser, cfg.PrimaryDBUser)
	dbPassword := envFallback(cfg.PrefixedPrimaryDBPassword, cfg.PrimaryDBPassword)
	dbName := envFallback(cfg.PrefixedPrimaryDBName, cfg.PrimaryDBName)
	dbPort := envFallback(cfg.PrefixedPrimaryDBPort, cfg.PrimaryDBPort)
	dbSSLMode := envFallback(cfg.PrefixedPrimaryDBSSLMode, cfg.PrimaryDBSSLMode)

	dbReplicaHost := envFallback(cfg.PrefixedReplicaDBHost, cfg.ReplicaDBHost)
	dbReplicaUser := envFallback(cfg.PrefixedReplicaDBUser, cfg.ReplicaDBUser)
	dbReplicaPassword := envFallback(cfg.PrefixedReplicaDBPassword, cfg.ReplicaDBPassword)
	dbReplicaName := envFallback(cfg.PrefixedReplicaDBName, cfg.ReplicaDBName)
	dbReplicaPort := envFallback(cfg.PrefixedReplicaDBPort, cfg.ReplicaDBPort)
	dbReplicaSSLMode := envFallback(cfg.PrefixedReplicaDBSSLMode, cfg.ReplicaDBSSLMode)

	maxOpenConns := envFallbackInt(cfg.PrefixedMaxOpenConnections, cfg.MaxOpenConnections)
	maxIdleConns := envFallbackInt(cfg.PrefixedMaxIdleConnections, cfg.MaxIdleConnections)

	postgreSourcePrimary := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s",
		dbHost, dbUser, dbPassword, dbName, dbPort, dbSSLMode)

	postgreSourceReplica := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s",
		dbReplicaHost, dbReplicaUser, dbReplicaPassword, dbReplicaName, dbReplicaPort, dbReplicaSSLMode)

	// Create default PostgreSQL connection (used in single-tenant mode or as fallback)
	postgresConnection := &libPostgres.PostgresConnection{
		ConnectionStringPrimary: postgreSourcePrimary,
		ConnectionStringReplica: postgreSourceReplica,
		PrimaryDBName:           dbName,
		ReplicaDBName:           dbReplicaName,
		Component:               ApplicationName,
		Logger:                  logger,
		MaxOpenConnections:      maxOpenConns,
		MaxIdleConnections:      maxIdleConns,
	}

	// Apply fallback for MongoDB prefixed env vars
	mongoURI := envFallback(cfg.PrefixedMongoURI, cfg.MongoURI)
	mongoHost := envFallback(cfg.PrefixedMongoDBHost, cfg.MongoDBHost)
	mongoName := envFallback(cfg.PrefixedMongoDBName, cfg.MongoDBName)
	mongoUser := envFallback(cfg.PrefixedMongoDBUser, cfg.MongoDBUser)
	mongoPassword := envFallback(cfg.PrefixedMongoDBPassword, cfg.MongoDBPassword)
	mongoPortRaw := envFallback(cfg.PrefixedMongoDBPort, cfg.MongoDBPort)
	mongoParametersRaw := envFallback(cfg.PrefixedMongoDBParameters, cfg.MongoDBParameters)
	mongoPoolSize := envFallbackInt(cfg.PrefixedMaxPoolSize, cfg.MaxPoolSize)

	// Extract port and parameters for MongoDB connection (handles backward compatibility)
	mongoPort, mongoParameters := pkgMongo.ExtractMongoPortAndParameters(mongoPortRaw, mongoParametersRaw, logger)

	// Build MongoDB connection string using centralized utility (ensures correct format)
	mongoSource := libMongo.BuildConnectionString(
		mongoURI, mongoUser, mongoPassword, mongoHost, mongoPort, mongoParameters, logger)

	// Safe conversion: use uint64 with default, only assign if positive
	var mongoMaxPoolSize uint64 = 100
	if mongoPoolSize > 0 {
		mongoMaxPoolSize = uint64(mongoPoolSize)
	}

	mongoConnection := &libMongo.MongoConnection{
		ConnectionStringSource: mongoSource,
		Database:               mongoName,
		Logger:                 logger,
		MaxPoolSize:            mongoMaxPoolSize,
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

	redisConsumerRepository, err := redis.NewConsumerRedis(redisConnection)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize redis: %w", err)
	}

	organizationPostgreSQLRepository := organization.NewOrganizationPostgreSQLRepository(postgresConnection)
	ledgerPostgreSQLRepository := ledger.NewLedgerPostgreSQLRepository(postgresConnection)
	segmentPostgreSQLRepository := segment.NewSegmentPostgreSQLRepository(postgresConnection)
	portfolioPostgreSQLRepository := portfolio.NewPortfolioPostgreSQLRepository(postgresConnection)
	accountPostgreSQLRepository := account.NewAccountPostgreSQLRepository(postgresConnection)
	assetPostgreSQLRepository := asset.NewAssetPostgreSQLRepository(postgresConnection)
	accountTypePostgreSQLRepository := accounttype.NewAccountTypePostgreSQLRepository(postgresConnection)

	metadataMongoDBRepository := mongodb.NewMetadataMongoDBRepository(mongoConnection)

	// Ensure indexes also for known base collections on fresh installs
	ctxEnsureIndexes, cancelEnsureIndexes := context.WithTimeout(context.Background(), 60*time.Second)
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

	// Choose balance port based on UnifiedMode:
	// - If UnifiedMode is true, validate and use provided ports for in-process calls
	// - Otherwise, use gRPC adapter to call the separate transaction service
	var balancePort mbootstrap.BalancePort

	if opts != nil && opts.UnifiedMode {
		if opts.BalancePort == nil {
			return nil, fmt.Errorf("unified mode requires BalancePort to be provided")
		}

		logger.Info("Running in UNIFIED MODE - using direct balance port (in-process calls)")

		balancePort = opts.BalancePort
	} else {
		if cfg.TransactionGRPCAddress == "" {
			cfg.TransactionGRPCAddress = "midaz-transaction"

			logger.Warn("TRANSACTION_GRPC_ADDRESS not set, using default: midaz-transaction")
		}

		if cfg.TransactionGRPCPort == "" {
			cfg.TransactionGRPCPort = "3011"

			logger.Warn("TRANSACTION_GRPC_PORT not set, using default: 3011")
		}

		grpcConnection := &mgrpc.GRPCConnection{
			Addr:   fmt.Sprintf("%s:%s", cfg.TransactionGRPCAddress, cfg.TransactionGRPCPort),
			Logger: logger,
		}

		logger.Info("Running in MICROSERVICES MODE - using gRPC balance adapter (network calls)")

		balancePort = grpcout.NewBalanceAdapter(grpcConnection)
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
		Ports: Ports{
			MetadataPort: metadataMongoDBRepository,
		},
		auth:                auth,
		accountHandler:      accountHandler,
		portfolioHandler:    portfolioHandler,
		ledgerHandler:       ledgerHandler,
		assetHandler:        assetHandler,
		organizationHandler: organizationHandler,
		segmentHandler:      segmentHandler,
		accountTypeHandler:  accountTypeHandler,
	}, nil
}
