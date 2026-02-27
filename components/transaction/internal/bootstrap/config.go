// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libCommons "github.com/LerianStudio/lib-commons/v3/commons"
	libCircuitBreaker "github.com/LerianStudio/lib-commons/v3/commons/circuitbreaker"
	libLog "github.com/LerianStudio/lib-commons/v3/commons/log"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v3/commons/opentelemetry"
	libRedis "github.com/LerianStudio/lib-commons/v3/commons/redis"
	tmclient "github.com/LerianStudio/lib-commons/v3/commons/tenant-manager/client"
	libZap "github.com/LerianStudio/lib-commons/v3/commons/zap"
	grpcIn "github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/grpc/in"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/query"
	"github.com/LerianStudio/midaz/v3/pkg/mbootstrap"
)

const ApplicationName = "transaction"

// initLogger initializes the logger from options or creates a new one.
func initLogger(opts *Options) (libLog.Logger, error) {
	if opts != nil && opts.Logger != nil {
		return opts.Logger, nil
	}

	return libZap.InitializeLoggerWithError()
}

// buildRabbitMQConnectionString constructs an AMQP connection string with optional vhost.
func buildRabbitMQConnectionString(uri, user, pass, host, port, vhost string) string {
	u := &url.URL{
		Scheme: uri,
		User:   url.UserPassword(user, pass),
		Host:   fmt.Sprintf("%s:%s", host, port),
	}
	if vhost != "" {
		u.RawPath = "/" + url.PathEscape(vhost)
		u.Path = "/" + vhost
	}

	return u.String()
}

// Config is the top level configuration struct for the entire application.
// Supports prefixed env vars (DB_TRANSACTION_*) with fallback to non-prefixed (DB_*) for backward compatibility.
type Config struct {
	EnvName  string `env:"ENV_NAME"`
	LogLevel string `env:"LOG_LEVEL"`

	// Server address - prefixed for unified ledger deployment
	PrefixedServerAddress string `env:"SERVER_ADDRESS_TRANSACTION"`
	ServerAddress         string `env:"SERVER_ADDRESS"`

	// PostgreSQL Primary - prefixed vars for unified ledger deployment
	PrefixedPrimaryDBHost     string `env:"DB_TRANSACTION_HOST"`
	PrefixedPrimaryDBUser     string `env:"DB_TRANSACTION_USER"`
	PrefixedPrimaryDBPassword string `env:"DB_TRANSACTION_PASSWORD"`
	PrefixedPrimaryDBName     string `env:"DB_TRANSACTION_NAME"`
	PrefixedPrimaryDBPort     string `env:"DB_TRANSACTION_PORT"`
	PrefixedPrimaryDBSSLMode  string `env:"DB_TRANSACTION_SSLMODE"`

	// PostgreSQL Primary - fallback vars for standalone deployment
	PrimaryDBHost     string `env:"DB_HOST"`
	PrimaryDBUser     string `env:"DB_USER"`
	PrimaryDBPassword string `env:"DB_PASSWORD"`
	PrimaryDBName     string `env:"DB_NAME"`
	PrimaryDBPort     string `env:"DB_PORT"`
	PrimaryDBSSLMode  string `env:"DB_SSLMODE"`

	// PostgreSQL Replica - prefixed vars for unified ledger deployment
	PrefixedReplicaDBHost     string `env:"DB_TRANSACTION_REPLICA_HOST"`
	PrefixedReplicaDBUser     string `env:"DB_TRANSACTION_REPLICA_USER"`
	PrefixedReplicaDBPassword string `env:"DB_TRANSACTION_REPLICA_PASSWORD"`
	PrefixedReplicaDBName     string `env:"DB_TRANSACTION_REPLICA_NAME"`
	PrefixedReplicaDBPort     string `env:"DB_TRANSACTION_REPLICA_PORT"`
	PrefixedReplicaDBSSLMode  string `env:"DB_TRANSACTION_REPLICA_SSLMODE"`

	// PostgreSQL Replica - fallback vars for standalone deployment
	ReplicaDBHost     string `env:"DB_REPLICA_HOST"`
	ReplicaDBUser     string `env:"DB_REPLICA_USER"`
	ReplicaDBPassword string `env:"DB_REPLICA_PASSWORD"`
	ReplicaDBName     string `env:"DB_REPLICA_NAME"`
	ReplicaDBPort     string `env:"DB_REPLICA_PORT"`
	ReplicaDBSSLMode  string `env:"DB_REPLICA_SSLMODE"`

	// PostgreSQL connection pool - prefixed with fallback
	PrefixedMaxOpenConnections int `env:"DB_TRANSACTION_MAX_OPEN_CONNS"`
	PrefixedMaxIdleConnections int `env:"DB_TRANSACTION_MAX_IDLE_CONNS"`
	MaxOpenConnections         int `env:"DB_MAX_OPEN_CONNS"`
	MaxIdleConnections         int `env:"DB_MAX_IDLE_CONNS"`

	// MongoDB - prefixed vars for unified ledger deployment
	PrefixedMongoURI          string `env:"MONGO_TRANSACTION_URI"`
	PrefixedMongoDBHost       string `env:"MONGO_TRANSACTION_HOST"`
	PrefixedMongoDBName       string `env:"MONGO_TRANSACTION_NAME"`
	PrefixedMongoDBUser       string `env:"MONGO_TRANSACTION_USER"`
	PrefixedMongoDBPassword   string `env:"MONGO_TRANSACTION_PASSWORD"`
	PrefixedMongoDBPort       string `env:"MONGO_TRANSACTION_PORT"`
	PrefixedMongoDBParameters string `env:"MONGO_TRANSACTION_PARAMETERS"`
	PrefixedMaxPoolSize       int    `env:"MONGO_TRANSACTION_MAX_POOL_SIZE"`

	// MongoDB - fallback vars for standalone deployment
	MongoURI                                 string `env:"MONGO_URI"`
	MongoDBHost                              string `env:"MONGO_HOST"`
	MongoDBName                              string `env:"MONGO_NAME"`
	MongoDBUser                              string `env:"MONGO_USER"`
	MongoDBPassword                          string `env:"MONGO_PASSWORD"`
	MongoDBPort                              string `env:"MONGO_PORT"`
	MongoDBParameters                        string `env:"MONGO_PARAMETERS"`
	MaxPoolSize                              int    `env:"MONGO_MAX_POOL_SIZE"`
	CasdoorAddress                           string `env:"CASDOOR_ADDRESS"`
	CasdoorClientID                          string `env:"CASDOOR_CLIENT_ID"`
	CasdoorClientSecret                      string `env:"CASDOOR_CLIENT_SECRET"`
	CasdoorOrganizationName                  string `env:"CASDOOR_ORGANIZATION_NAME"`
	CasdoorApplicationName                   string `env:"CASDOOR_APPLICATION_NAME"`
	CasdoorModelName                         string `env:"CASDOOR_MODEL_NAME"`
	JWKAddress                               string `env:"CASDOOR_JWK_ADDRESS"`
	RabbitURI                                string `env:"RABBITMQ_URI"`
	RabbitMQHost                             string `env:"RABBITMQ_HOST"`
	RabbitMQPortHost                         string `env:"RABBITMQ_PORT_HOST"`
	RabbitMQPortAMQP                         string `env:"RABBITMQ_PORT_AMQP"`
	RabbitMQUser                             string `env:"RABBITMQ_DEFAULT_USER"`
	RabbitMQPass                             string `env:"RABBITMQ_DEFAULT_PASS"`
	RabbitMQConsumerUser                     string `env:"RABBITMQ_CONSUMER_USER"`
	RabbitMQConsumerPass                     string `env:"RABBITMQ_CONSUMER_PASS"`
	RabbitMQVHost                            string `env:"RABBITMQ_VHOST"`
	RabbitMQNumbersOfWorkers                 int    `env:"RABBITMQ_NUMBERS_OF_WORKERS"`
	RabbitMQNumbersOfPrefetch                int    `env:"RABBITMQ_NUMBERS_OF_PREFETCH"`
	RabbitMQHealthCheckURL                   string `env:"RABBITMQ_HEALTH_CHECK_URL"`
	RabbitMQTransactionBalanceOperationQueue string `env:"RABBITMQ_TRANSACTION_BALANCE_OPERATION_QUEUE"`
	OtelServiceName                          string `env:"OTEL_RESOURCE_SERVICE_NAME"`
	OtelLibraryName                          string `env:"OTEL_LIBRARY_NAME"`
	OtelServiceVersion                       string `env:"OTEL_RESOURCE_SERVICE_VERSION"`
	OtelDeploymentEnv                        string `env:"OTEL_RESOURCE_DEPLOYMENT_ENVIRONMENT"`
	OtelColExporterEndpoint                  string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	EnableTelemetry                          bool   `env:"ENABLE_TELEMETRY"`
	RedisHost                                string `env:"REDIS_HOST"`
	RedisMasterName                          string `env:"REDIS_MASTER_NAME" default:""`
	RedisPassword                            string `env:"REDIS_PASSWORD"`
	RedisDB                                  int    `env:"REDIS_DB" default:"0"`
	RedisProtocol                            int    `env:"REDIS_PROTOCOL" default:"3"`
	RedisTLS                                 bool   `env:"REDIS_TLS" default:"false"`
	RedisCACert                              string `env:"REDIS_CA_CERT"`
	RedisUseGCPIAM                           bool   `env:"REDIS_USE_GCP_IAM" default:"false"`
	RedisServiceAccount                      string `env:"REDIS_SERVICE_ACCOUNT" default:""`
	GoogleApplicationCredentials             string `env:"GOOGLE_APPLICATION_CREDENTIALS" default:""`
	RedisTokenLifeTime                       int    `env:"REDIS_TOKEN_LIFETIME" default:"60"`
	RedisTokenRefreshDuration                int    `env:"REDIS_TOKEN_REFRESH_DURATION" default:"45"`
	RedisPoolSize                            int    `env:"REDIS_POOL_SIZE" default:"10"`
	RedisMinIdleConns                        int    `env:"REDIS_MIN_IDLE_CONNS" default:"0"`
	RedisReadTimeout                         int    `env:"REDIS_READ_TIMEOUT" default:"3"`
	RedisWriteTimeout                        int    `env:"REDIS_WRITE_TIMEOUT" default:"3"`
	RedisDialTimeout                         int    `env:"REDIS_DIAL_TIMEOUT" default:"5"`
	RedisPoolTimeout                         int    `env:"REDIS_POOL_TIMEOUT" default:"2"`
	RedisMaxRetries                          int    `env:"REDIS_MAX_RETRIES" default:"3"`
	RedisMinRetryBackoff                     int    `env:"REDIS_MIN_RETRY_BACKOFF" default:"8"`
	RedisMaxRetryBackoff                     int    `env:"REDIS_MAX_RETRY_BACKOFF" default:"1"`
	AuthEnabled                              bool   `env:"PLUGIN_AUTH_ENABLED"`
	AuthHost                                 string `env:"PLUGIN_AUTH_HOST"`
	ProtoAddress                             string `env:"PROTO_ADDRESS"`
	BalanceSyncWorkerEnabled                 bool   `env:"BALANCE_SYNC_WORKER_ENABLED" default:"true"`
	BalanceSyncMaxWorkers                    int    `env:"BALANCE_SYNC_MAX_WORKERS"`

	// Circuit Breaker configuration for RabbitMQ
	// Protects against RabbitMQ outages by failing fast when broker is unavailable
	RabbitMQCircuitBreakerConsecutiveFailures int `env:"RABBITMQ_CIRCUIT_BREAKER_CONSECUTIVE_FAILURES"`
	RabbitMQCircuitBreakerFailureRatio        int `env:"RABBITMQ_CIRCUIT_BREAKER_FAILURE_RATIO"` // Stored as percentage (e.g., 50 for 0.5)
	RabbitMQCircuitBreakerInterval            int `env:"RABBITMQ_CIRCUIT_BREAKER_INTERVAL"`      // Stored in seconds
	RabbitMQCircuitBreakerMaxRequests         int `env:"RABBITMQ_CIRCUIT_BREAKER_MAX_REQUESTS"`
	RabbitMQCircuitBreakerMinRequests         int `env:"RABBITMQ_CIRCUIT_BREAKER_MIN_REQUESTS"`
	RabbitMQCircuitBreakerTimeout             int `env:"RABBITMQ_CIRCUIT_BREAKER_TIMEOUT"` // Stored in seconds
	// Health Check configuration for circuit breaker recovery
	RabbitMQCircuitBreakerHealthCheckInterval int `env:"RABBITMQ_CIRCUIT_BREAKER_HEALTH_CHECK_INTERVAL"` // Stored in seconds
	RabbitMQCircuitBreakerHealthCheckTimeout  int `env:"RABBITMQ_CIRCUIT_BREAKER_HEALTH_CHECK_TIMEOUT"`  // Stored in seconds
	// Operation timeout for RabbitMQ connection and publish operations (e.g., "5s", "3s")
	RabbitMQOperationTimeout string `env:"RABBITMQ_OPERATION_TIMEOUT"`
	// Multi-tenant consumer configuration
	RabbitMQMultiTenantSyncInterval     int `env:"RABBITMQ_MULTI_TENANT_SYNC_INTERVAL"`     // Stored in seconds
	RabbitMQMultiTenantDiscoveryTimeout int `env:"RABBITMQ_MULTI_TENANT_DISCOVERY_TIMEOUT"` // Stored in milliseconds

	// Multi-tenant configuration for standalone deployment.
	// When running in unified ledger mode, these are ignored (Options fields take precedence).
	MultiTenantEnabled    bool   `env:"MULTI_TENANT_ENABLED"`
	MultiTenantURL        string `env:"MULTI_TENANT_URL"`
	MultiTenantTimeout    int    `env:"MULTI_TENANT_TIMEOUT"`   // seconds
	MultiTenantCacheTTL   int    `env:"MULTI_TENANT_CACHE_TTL"` // seconds
	MultiTenantCacheSize  int    `env:"MULTI_TENANT_CACHE_SIZE"`
	MultiTenantRetryMax   int    `env:"MULTI_TENANT_RETRY_MAX"`
	MultiTenantRetryDelay int    `env:"MULTI_TENANT_RETRY_DELAY"` // seconds
}

// Options contains optional dependencies that can be injected by callers.
type Options struct {
	// Logger allows callers to provide a pre-configured logger, avoiding double
	// initialization when the cmd/app wants to handle bootstrap errors.
	Logger libLog.Logger

	// CircuitBreakerStateListener receives notifications when circuit breaker state changes.
	// This is optional - pass nil if you don't need state change notifications.
	CircuitBreakerStateListener libCircuitBreaker.StateChangeListener

	// SettingsPort enables direct in-process communication with the onboarding module
	// for querying ledger settings. Optional - if not provided, settings functionality
	// will not be available.
	SettingsPort mbootstrap.SettingsPort

	// Multi-tenant configuration (only used in unified ledger mode).
	MultiTenantEnabled bool
	TenantClient       *tmclient.Client
	TenantServiceName  string
	TenantEnvironment  string
	TenantManagerURL   string
}

// InitServers initiate http and grpc servers.
func InitServers() (*Service, error) {
	return InitServersWithOptions(nil)
}

// InitServersWithOptions initiates http and grpc servers with optional dependency injection.
func InitServersWithOptions(opts *Options) (*Service, error) {
	cfg := &Config{}

	if err := libCommons.SetConfigFromEnvVars(cfg); err != nil {
		return nil, fmt.Errorf("failed to load config from environment variables: %w", err)
	}

	logger, err := initLogger(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize logger: %w", err)
	}

	// BalanceSyncWorkerEnabled defaults to true via struct tag
	balanceSyncWorkerEnabled := cfg.BalanceSyncWorkerEnabled
	logger.Infof("BalanceSyncWorker: BALANCE_SYNC_WORKER_ENABLED=%v", balanceSyncWorkerEnabled)

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

	// PostgreSQL: single-tenant or multi-tenant (decided internally)
	pg, err := initPostgres(opts, cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize PostgreSQL: %w", err)
	}

	// MongoDB: single-tenant or multi-tenant (decided internally)
	mgo, err := initMongo(opts, cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize MongoDB: %w", err)
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

	redisConsumerRepository, err := redis.NewConsumerRedis(redisConnection, balanceSyncWorkerEnabled)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize redis: %w", err)
	}

	// RabbitMQ: producer + consumer (multi-tenant or single-tenant, decided internally)
	rmq, err := initRabbitMQ(opts, cfg, logger, telemetry, redisConnection)
	if err != nil {
		return nil, err
	}

	// Pass PG and Mongo managers to RabbitMQ components for per-message tenant resolution
	if rmq != nil {
		rmq.pgManager = pg.pgManager
		rmq.mongoManager = mgo.mongoManager
	}

	// UseCases are created without SettingsPort initially.
	// The Lazy Initialization pattern is used: SetSettingsPort is called after
	// both transaction and onboarding modules exist, resolving the circular dependency.
	// If opts.SettingsPort is provided (e.g., in tests), it's set immediately.
	commandUseCase := &command.UseCase{
		TransactionRepo:      pg.transactionRepo,
		OperationRepo:        pg.operationRepo,
		AssetRateRepo:        pg.assetRateRepo,
		BalanceRepo:          pg.balanceRepo,
		OperationRouteRepo:   pg.operationRouteRepo,
		TransactionRouteRepo: pg.transactionRouteRepo,
		MetadataRepo:         mgo.metadataRepo,
		RabbitMQRepo:         rmq.producerRepo,
		RedisRepo:            redisConsumerRepository,
	}

	queryUseCase := &query.UseCase{
		TransactionRepo:      pg.transactionRepo,
		OperationRepo:        pg.operationRepo,
		AssetRateRepo:        pg.assetRateRepo,
		BalanceRepo:          pg.balanceRepo,
		OperationRouteRepo:   pg.operationRouteRepo,
		TransactionRouteRepo: pg.transactionRouteRepo,
		MetadataRepo:         mgo.metadataRepo,
		RabbitMQRepo:         rmq.producerRepo,
		RedisRepo:            redisConsumerRepository,
	}

	// If SettingsPort is provided via options (e.g., tests), set it immediately
	if opts != nil && opts.SettingsPort != nil {
		commandUseCase.SettingsPort = opts.SettingsPort
		queryUseCase.SettingsPort = opts.SettingsPort
	}

	// Wire consumer with UseCase (registers handler or creates MultiQueueConsumer)
	rmq.wireConsumer(commandUseCase)

	transactionHandler := &in.TransactionHandler{
		Command: commandUseCase,
		Query:   queryUseCase,
	}

	operationHandler := &in.OperationHandler{
		Command: commandUseCase,
		Query:   queryUseCase,
	}

	assetRateHandler := &in.AssetRateHandler{
		Command: commandUseCase,
		Query:   queryUseCase,
	}

	balanceHandler := &in.BalanceHandler{
		Command: commandUseCase,
		Query:   queryUseCase,
	}

	operationRouteHandler := &in.OperationRouteHandler{
		Command: commandUseCase,
		Query:   queryUseCase,
	}

	transactionRouteHandler := &in.TransactionRouteHandler{
		Command: commandUseCase,
		Query:   queryUseCase,
	}

	auth := middleware.NewAuthClient(cfg.AuthHost, cfg.AuthEnabled, &logger)

	tenantMiddleware, err := initTenantMiddleware(opts, cfg, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize tenant middleware: %w", err)
	}

	app := in.NewRouter(logger, telemetry, auth, tenantMiddleware, transactionHandler, operationHandler, assetRateHandler, balanceHandler, operationRouteHandler, transactionRouteHandler)

	server := NewServer(cfg, app, logger, telemetry)

	if cfg.ProtoAddress == "" || cfg.ProtoAddress == ":" {
		cfg.ProtoAddress = ":3011"

		logger.Warn("PROTO_ADDRESS not set or invalid, using default: :3011")
	}

	grpcApp := grpcIn.NewRouterGRPC(logger, telemetry, auth, commandUseCase, queryUseCase)
	serverGRPC := NewServerGRPC(cfg, grpcApp, logger, telemetry)

	// RedisQueueConsumer: multi-tenant or single-tenant
	var redisConsumer *RedisQueueConsumer
	if opts != nil && opts.MultiTenantEnabled {
		redisConsumer = NewRedisQueueConsumerMultiTenant(logger, *transactionHandler, true, opts.TenantClient, pg.pgManager)
	} else {
		redisConsumer = NewRedisQueueConsumer(logger, *transactionHandler)
	}

	const defaultBalanceSyncMaxWorkers = 5

	balanceSyncMaxWorkers := cfg.BalanceSyncMaxWorkers

	if balanceSyncMaxWorkers <= 0 {
		balanceSyncMaxWorkers = defaultBalanceSyncMaxWorkers
		logger.Infof("BalanceSyncWorker using default: BALANCE_SYNC_MAX_WORKERS=%d", defaultBalanceSyncMaxWorkers)
	}

	// BalanceSyncWorker: multi-tenant or single-tenant
	var balanceSyncWorker *BalanceSyncWorker
	if balanceSyncWorkerEnabled {
		if opts != nil && opts.MultiTenantEnabled {
			balanceSyncWorker = NewBalanceSyncWorkerMultiTenant(redisConnection, logger, commandUseCase, balanceSyncMaxWorkers, true, opts.TenantClient, pg.pgManager)
		} else {
			balanceSyncWorker = NewBalanceSyncWorker(redisConnection, logger, commandUseCase, balanceSyncMaxWorkers)
		}

		logger.Infof("BalanceSyncWorker enabled with %d max workers.", balanceSyncMaxWorkers)
	} else {
		logger.Info("BalanceSyncWorker disabled.")
	}

	return &Service{
		Server:                   server,
		ServerGRPC:               serverGRPC,
		MultiQueueConsumer:       rmq.multiQueueConsumer,
		MultiTenantConsumer:      rmq.multiTenantConsumer,
		RedisQueueConsumer:       redisConsumer,
		BalanceSyncWorker:        balanceSyncWorker,
		BalanceSyncWorkerEnabled: balanceSyncWorkerEnabled,
		CircuitBreakerManager:    rmq.circuitBreakerManager,
		Logger:                   logger,
		Ports: Ports{
			BalancePort:  commandUseCase,
			MetadataPort: mgo.metadataRepo,
		},
		commandUseCase:          commandUseCase,
		queryUseCase:            queryUseCase,
		multiTenantConsumerPort: rmq.multiTenantConsumer,
		auth:                    auth,
		transactionHandler:      transactionHandler,
		operationHandler:        operationHandler,
		assetRateHandler:        assetRateHandler,
		balanceHandler:          balanceHandler,
		operationRouteHandler:   operationRouteHandler,
		transactionRouteHandler: transactionRouteHandler,
	}, nil
}
