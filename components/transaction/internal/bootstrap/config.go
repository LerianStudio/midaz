package bootstrap

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	libMongo "github.com/LerianStudio/lib-commons/v2/commons/mongo"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	libRabbitmq "github.com/LerianStudio/lib-commons/v2/commons/rabbitmq"
	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	grpcIn "github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/grpc/in"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transactionroute"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/query"
	pkgMongo "github.com/LerianStudio/midaz/v3/pkg/mongo"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const ApplicationName = "transaction"

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
	MongoURI                     string `env:"MONGO_URI"`
	MongoDBHost                  string `env:"MONGO_HOST"`
	MongoDBName                  string `env:"MONGO_NAME"`
	MongoDBUser                  string `env:"MONGO_USER"`
	MongoDBPassword              string `env:"MONGO_PASSWORD"`
	MongoDBPort                  string `env:"MONGO_PORT"`
	MongoDBParameters            string `env:"MONGO_PARAMETERS"`
	MaxPoolSize                  int    `env:"MONGO_MAX_POOL_SIZE"`
	CasdoorAddress               string `env:"CASDOOR_ADDRESS"`
	CasdoorClientID              string `env:"CASDOOR_CLIENT_ID"`
	CasdoorClientSecret          string `env:"CASDOOR_CLIENT_SECRET"`
	CasdoorOrganizationName      string `env:"CASDOOR_ORGANIZATION_NAME"`
	CasdoorApplicationName       string `env:"CASDOOR_APPLICATION_NAME"`
	CasdoorModelName             string `env:"CASDOOR_MODEL_NAME"`
	JWKAddress                   string `env:"CASDOOR_JWK_ADDRESS"`
	RabbitURI                    string `env:"RABBITMQ_URI"`
	RabbitMQHost                 string `env:"RABBITMQ_HOST"`
	RabbitMQPortHost             string `env:"RABBITMQ_PORT_HOST"`
	RabbitMQPortAMQP             string `env:"RABBITMQ_PORT_AMQP"`
	RabbitMQUser                 string `env:"RABBITMQ_DEFAULT_USER"`
	RabbitMQPass                 string `env:"RABBITMQ_DEFAULT_PASS"`
	RabbitMQConsumerUser         string `env:"RABBITMQ_CONSUMER_USER"`
	RabbitMQConsumerPass         string `env:"RABBITMQ_CONSUMER_PASS"`
	RabbitMQVHost                string `env:"RABBITMQ_VHOST"`
	RabbitMQBalanceCreateQueue   string `env:"RABBITMQ_BALANCE_CREATE_QUEUE"`
	RabbitMQNumbersOfWorkers     int    `env:"RABBITMQ_NUMBERS_OF_WORKERS"`
	RabbitMQNumbersOfPrefetch    int    `env:"RABBITMQ_NUMBERS_OF_PREFETCH"`
	RabbitMQHealthCheckURL       string `env:"RABBITMQ_HEALTH_CHECK_URL"`
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
	ProtoAddress                 string `env:"PROTO_ADDRESS"`
	BalanceSyncWorkerEnabled     bool   `env:"BALANCE_SYNC_WORKER_ENABLED" default:"true"`
	BalanceSyncMaxWorkers        int    `env:"BALANCE_SYNC_MAX_WORKERS"`
}

// Options contains optional dependencies that can be injected by callers.
type Options struct {
	// Logger allows callers to provide a pre-configured logger, avoiding double
	// initialization when the cmd/app wants to handle bootstrap errors.
	Logger libLog.Logger
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

	var logger libLog.Logger
	if opts != nil && opts.Logger != nil {
		logger = opts.Logger
	} else {
		logger = libZap.InitializeLogger()
	}

	// BalanceSyncWorkerEnabled defaults to true via struct tag
	balanceSyncWorkerEnabled := cfg.BalanceSyncWorkerEnabled
	logger.Infof("BalanceSyncWorker: BALANCE_SYNC_WORKER_ENABLED=%v", balanceSyncWorkerEnabled)

	telemetry := libOpentelemetry.InitializeTelemetry(&libOpentelemetry.TelemetryConfig{
		LibraryName:               cfg.OtelLibraryName,
		ServiceName:               cfg.OtelServiceName,
		ServiceVersion:            cfg.OtelServiceVersion,
		DeploymentEnv:             cfg.OtelDeploymentEnv,
		CollectorExporterEndpoint: cfg.OtelColExporterEndpoint,
		EnableTelemetry:           cfg.EnableTelemetry,
		Logger:                    logger,
	})

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
	mongoSource := pkgMongo.BuildMongoConnectionString(
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

	redisConsumerRepository, err := redis.NewConsumerRedis(redisConnection, balanceSyncWorkerEnabled)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize redis: %w", err)
	}

	transactionPostgreSQLRepository := transaction.NewTransactionPostgreSQLRepository(postgresConnection)
	operationPostgreSQLRepository := operation.NewOperationPostgreSQLRepository(postgresConnection)
	assetRatePostgreSQLRepository := assetrate.NewAssetRatePostgreSQLRepository(postgresConnection)
	balancePostgreSQLRepository := balance.NewBalancePostgreSQLRepository(postgresConnection)
	operationRoutePostgreSQLRepository := operationroute.NewOperationRoutePostgreSQLRepository(postgresConnection)
	transactionRoutePostgreSQLRepository := transactionroute.NewTransactionRoutePostgreSQLRepository(postgresConnection)

	metadataMongoDBRepository := mongodb.NewMetadataMongoDBRepository(mongoConnection)

	// Ensure indexes also for known base collections on fresh installs
	ctxEnsureIndexes, cancelEnsureIndexes := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancelEnsureIndexes()

	indexModel := mongo.IndexModel{
		Keys: bson.D{{Key: "entity_id", Value: 1}},
		Options: options.Index().
			SetUnique(false),
	}

	collections := []string{"operation", "transaction", "operation_route", "transaction_route"}
	for _, collection := range collections {
		if err := mongoConnection.EnsureIndexes(ctxEnsureIndexes, collection, indexModel); err != nil {
			logger.Warnf("Failed to ensure indexes for collection %s: %v", collection, err)
		}
	}

	rabbitSource := buildRabbitMQConnectionString(
		cfg.RabbitURI, cfg.RabbitMQUser, cfg.RabbitMQPass, cfg.RabbitMQHost, cfg.RabbitMQPortHost, cfg.RabbitMQVHost)

	rabbitMQConnection := &libRabbitmq.RabbitMQConnection{
		ConnectionStringSource: rabbitSource,
		HealthCheckURL:         cfg.RabbitMQHealthCheckURL,
		Host:                   cfg.RabbitMQHost,
		Port:                   cfg.RabbitMQPortAMQP,
		User:                   cfg.RabbitMQUser,
		Pass:                   cfg.RabbitMQPass,
		VHost:                  cfg.RabbitMQVHost,
		Queue:                  cfg.RabbitMQBalanceCreateQueue,
		Logger:                 logger,
	}

	producerRabbitMQRepository := rabbitmq.NewProducerRabbitMQ(rabbitMQConnection)

	useCase := &command.UseCase{
		TransactionRepo:      transactionPostgreSQLRepository,
		OperationRepo:        operationPostgreSQLRepository,
		AssetRateRepo:        assetRatePostgreSQLRepository,
		BalanceRepo:          balancePostgreSQLRepository,
		OperationRouteRepo:   operationRoutePostgreSQLRepository,
		TransactionRouteRepo: transactionRoutePostgreSQLRepository,
		MetadataRepo:         metadataMongoDBRepository,
		RabbitMQRepo:         producerRabbitMQRepository,
		RedisRepo:            redisConsumerRepository,
	}

	queryUseCase := &query.UseCase{
		TransactionRepo:      transactionPostgreSQLRepository,
		OperationRepo:        operationPostgreSQLRepository,
		AssetRateRepo:        assetRatePostgreSQLRepository,
		BalanceRepo:          balancePostgreSQLRepository,
		OperationRouteRepo:   operationRoutePostgreSQLRepository,
		TransactionRouteRepo: transactionRoutePostgreSQLRepository,
		MetadataRepo:         metadataMongoDBRepository,
		RabbitMQRepo:         producerRabbitMQRepository,
		RedisRepo:            redisConsumerRepository,
	}

	transactionHandler := &in.TransactionHandler{
		Command: useCase,
		Query:   queryUseCase,
	}

	operationHandler := &in.OperationHandler{
		Command: useCase,
		Query:   queryUseCase,
	}

	assetRateHandler := &in.AssetRateHandler{
		Command: useCase,
		Query:   queryUseCase,
	}

	balanceHandler := &in.BalanceHandler{
		Command: useCase,
		Query:   queryUseCase,
	}

	operationRouteHandler := &in.OperationRouteHandler{
		Command: useCase,
		Query:   queryUseCase,
	}

	transactionRouteHandler := &in.TransactionRouteHandler{
		Command: useCase,
		Query:   queryUseCase,
	}

	rabbitConsumerSource := buildRabbitMQConnectionString(
		cfg.RabbitURI, cfg.RabbitMQConsumerUser, cfg.RabbitMQConsumerPass, cfg.RabbitMQHost, cfg.RabbitMQPortHost, cfg.RabbitMQVHost)

	rabbitMQConsumerConnection := &libRabbitmq.RabbitMQConnection{
		ConnectionStringSource: rabbitConsumerSource,
		HealthCheckURL:         cfg.RabbitMQHealthCheckURL,
		Host:                   cfg.RabbitMQHost,
		Port:                   cfg.RabbitMQPortAMQP,
		User:                   cfg.RabbitMQConsumerUser,
		Pass:                   cfg.RabbitMQConsumerPass,
		VHost:                  cfg.RabbitMQVHost,
		Queue:                  cfg.RabbitMQBalanceCreateQueue,
		Logger:                 logger,
	}

	routes := rabbitmq.NewConsumerRoutes(rabbitMQConsumerConnection, cfg.RabbitMQNumbersOfWorkers, cfg.RabbitMQNumbersOfPrefetch, logger, telemetry)

	multiQueueConsumer := NewMultiQueueConsumer(routes, useCase)

	auth := middleware.NewAuthClient(cfg.AuthHost, cfg.AuthEnabled, &logger)

	app := in.NewRouter(logger, telemetry, auth, transactionHandler, operationHandler, assetRateHandler, balanceHandler, operationRouteHandler, transactionRouteHandler)

	server := NewServer(cfg, app, logger, telemetry)

	if cfg.ProtoAddress == "" || cfg.ProtoAddress == ":" {
		cfg.ProtoAddress = ":3011"

		logger.Warn("PROTO_ADDRESS not set or invalid, using default: :3011")
	}

	grpcApp := grpcIn.NewRouterGRPC(logger, telemetry, auth, useCase, queryUseCase)
	serverGRPC := NewServerGRPC(cfg, grpcApp, logger, telemetry)

	redisConsumer := NewRedisQueueConsumer(logger, *transactionHandler)

	const defaultBalanceSyncMaxWorkers = 5

	balanceSyncMaxWorkers := cfg.BalanceSyncMaxWorkers

	if balanceSyncMaxWorkers <= 0 {
		balanceSyncMaxWorkers = defaultBalanceSyncMaxWorkers
		logger.Infof("BalanceSyncWorker using default: BALANCE_SYNC_MAX_WORKERS=%d", defaultBalanceSyncMaxWorkers)
	}

	var balanceSyncWorker *BalanceSyncWorker
	if balanceSyncWorkerEnabled {
		balanceSyncWorker = NewBalanceSyncWorker(redisConnection, logger, useCase, balanceSyncMaxWorkers)
		logger.Infof("BalanceSyncWorker enabled with %d max workers.", balanceSyncMaxWorkers)
	} else {
		logger.Info("BalanceSyncWorker disabled.")
	}

	return &Service{
		Server:                   server,
		ServerGRPC:               serverGRPC,
		MultiQueueConsumer:       multiQueueConsumer,
		RedisQueueConsumer:       redisConsumer,
		BalanceSyncWorker:        balanceSyncWorker,
		BalanceSyncWorkerEnabled: balanceSyncWorkerEnabled,
		Logger:                   logger,
		Ports: Ports{
			BalancePort:  useCase,
			MetadataPort: metadataMongoDBRepository,
		},
		auth:                    auth,
		transactionHandler:      transactionHandler,
		operationHandler:        operationHandler,
		assetRateHandler:        assetRateHandler,
		balanceHandler:          balanceHandler,
		operationRouteHandler:   operationRouteHandler,
		transactionRouteHandler: transactionRouteHandler,
	}, nil
}
