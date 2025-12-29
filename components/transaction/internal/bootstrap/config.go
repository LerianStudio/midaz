package bootstrap

import (
	"context"
	"database/sql"
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
	libRabbitmq "github.com/LerianStudio/lib-commons/v2/commons/rabbitmq"
	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	"github.com/bxcodec/dbresolver/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	grpcIn "github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/grpc/in"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/outbox"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transactionroute"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/query"
	"github.com/LerianStudio/midaz/v3/pkg/assert"
	"github.com/LerianStudio/midaz/v3/pkg/dbtx"
	"github.com/LerianStudio/midaz/v3/pkg/mmigration"
)

// ApplicationName is the identifier for the transaction service used in logging and tracing.
const ApplicationName = "transaction"

const (
	ensureIndexesTimeoutSeconds = 60
)

// Sentinel errors for bootstrap initialization.
var (
	// ErrInitializationFailed indicates a panic occurred during initialization.
	ErrInitializationFailed = errors.New("initialization failed")
)

// dbTxAdapter wraps dbresolver.Tx to implement dbtx.Tx
type dbTxAdapter struct {
	dbresolver.Tx
}

// dbProviderAdapter wraps dbresolver.DB to implement dbtx.TxBeginner
type dbProviderAdapter struct {
	db dbresolver.DB
}

// BeginTx starts a new transaction and returns it wrapped as dbtx.Tx
func (a *dbProviderAdapter) BeginTx(ctx context.Context, opts *sql.TxOptions) (dbtx.Tx, error) {
	tx, err := a.db.BeginTx(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err) //nolint:wrapcheck // BeginTx is infrastructure-level, context added via fmt.Errorf
	}

	return &dbTxAdapter{tx}, nil
}

// Config is the top level configuration struct for the entire application.
type Config struct {
	EnvName                                  string `env:"ENV_NAME"`
	LogLevel                                 string `env:"LOG_LEVEL"`
	ServerAddress                            string `env:"SERVER_ADDRESS"`
	PrimaryDBHost                            string `env:"DB_HOST"`
	PrimaryDBUser                            string `env:"DB_USER"`
	PrimaryDBPassword                        string `env:"DB_PASSWORD"`
	PrimaryDBName                            string `env:"DB_NAME"`
	PrimaryDBPort                            string `env:"DB_PORT"`
	PrimaryDBSSLMode                         string `env:"DB_SSLMODE"`
	ReplicaDBHost                            string `env:"DB_REPLICA_HOST"`
	ReplicaDBUser                            string `env:"DB_REPLICA_USER"`
	ReplicaDBPassword                        string `env:"DB_REPLICA_PASSWORD"`
	ReplicaDBName                            string `env:"DB_REPLICA_NAME"`
	ReplicaDBPort                            string `env:"DB_REPLICA_PORT"`
	ReplicaDBSSLMode                         string `env:"DB_REPLICA_SSLMODE"`
	MaxOpenConnections                       int    `env:"DB_MAX_OPEN_CONNS"`
	MaxIdleConnections                       int    `env:"DB_MAX_IDLE_CONNS"`
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
	RabbitMQBalanceCreateQueue               string `env:"RABBITMQ_BALANCE_CREATE_QUEUE"`
	RabbitMQTransactionBalanceOperationQueue string `env:"RABBITMQ_TRANSACTION_BALANCE_OPERATION_QUEUE"`
	RabbitMQNumbersOfWorkers                 int    `env:"RABBITMQ_NUMBERS_OF_WORKERS"`
	RabbitMQNumbersOfPrefetch                int    `env:"RABBITMQ_NUMBERS_OF_PREFETCH"`
	RabbitMQHealthCheckURL                   string `env:"RABBITMQ_HEALTH_CHECK_URL"`
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
	BalanceSyncWorkerEnabled                 bool   `env:"BALANCE_SYNC_WORKER_ENABLED"`
	BalanceSyncMaxWorkers                    int    `env:"BALANCE_SYNC_MAX_WORKERS"`
	DLQConsumerEnabled                       bool   `env:"DLQ_CONSUMER_ENABLED"`
	MetadataOutboxWorkerEnabled              bool   `env:"METADATA_OUTBOX_WORKER_ENABLED"`
	MetadataOutboxMaxWorkers                 int    `env:"METADATA_OUTBOX_MAX_WORKERS"`
	MetadataOutboxRetentionDays              int    `env:"METADATA_OUTBOX_RETENTION_DAYS"`
	// Migration auto-recovery configuration
	MigrationAutoRecover bool `env:"MIGRATION_AUTO_RECOVER" default:"true"`
	MigrationMaxRetries  int  `env:"MIGRATION_MAX_RETRIES" default:"3"`
}

// InitServers initiate http and grpc servers.
func InitServers() *Service {
	cfg := &Config{}

	err := libCommons.SetConfigFromEnvVars(cfg)
	assert.NoError(err, "configuration required for transaction",
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

	// Add statement_timeout and lock_timeout to prevent row-level lock contention from causing 20s+ hangs.
	// statement_timeout=5000ms: Cancel any query running longer than 5 seconds
	// lock_timeout=3000ms: Fail immediately if waiting for a lock more than 3 seconds
	// This ensures integration tests fail fast instead of hanging on hot rows (e.g., @external/USD)
	postgreSourcePrimary := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s statement_timeout=5000 lock_timeout=3000",
		cfg.PrimaryDBHost, cfg.PrimaryDBUser, cfg.PrimaryDBPassword, cfg.PrimaryDBName, cfg.PrimaryDBPort, cfg.PrimaryDBSSLMode)

	postgreSourceReplica := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s statement_timeout=5000 lock_timeout=3000",
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
	migrationConfig := mmigration.MigrationConfig{
		AutoRecoverDirty:      cfg.MigrationAutoRecover,
		MaxRetries:            cfg.MigrationMaxRetries,
		MaxRecoveryPerVersion: 3,
		RetryBackoff:          1 * time.Second,
		MaxBackoff:            30 * time.Second,
		LockTimeout:           30 * time.Second,
		Component:             ApplicationName,
		MigrationsPath:        "/app/components/transaction/migrations",
	}

	migrationWrapper, err := mmigration.NewMigrationWrapper(postgresConnection, migrationConfig, logger)
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

	redisConsumerRepository := redis.NewConsumerRedis(redisConnection)

	transactionPostgreSQLRepository := transaction.NewTransactionPostgreSQLRepository(migrationWrapper)
	operationPostgreSQLRepository := operation.NewOperationPostgreSQLRepository(migrationWrapper)
	assetRatePostgreSQLRepository := assetrate.NewAssetRatePostgreSQLRepository(migrationWrapper)
	balancePostgreSQLRepository := balance.NewBalancePostgreSQLRepository(migrationWrapper)
	operationRoutePostgreSQLRepository := operationroute.NewOperationRoutePostgreSQLRepository(migrationWrapper)
	transactionRoutePostgreSQLRepository := transactionroute.NewTransactionRoutePostgreSQLRepository(migrationWrapper)

	metadataMongoDBRepository := mongodb.NewMetadataMongoDBRepository(mongoConnection)

	outboxPostgreSQLRepository := outbox.NewOutboxPostgreSQLRepository(migrationWrapper)

	// Ensure indexes also for known base collections on fresh installs
	ctxEnsureIndexes, cancelEnsureIndexes := context.WithTimeout(context.Background(), ensureIndexesTimeoutSeconds*time.Second)
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

	rabbitSource := fmt.Sprintf("%s://%s:%s@%s:%s",
		cfg.RabbitURI, cfg.RabbitMQUser, cfg.RabbitMQPass, cfg.RabbitMQHost, cfg.RabbitMQPortHost)

	rabbitMQConnection := &libRabbitmq.RabbitMQConnection{
		ConnectionStringSource: rabbitSource,
		HealthCheckURL:         cfg.RabbitMQHealthCheckURL,
		Host:                   cfg.RabbitMQHost,
		Port:                   cfg.RabbitMQPortAMQP,
		User:                   cfg.RabbitMQUser,
		Pass:                   cfg.RabbitMQPass,
		Queue:                  cfg.RabbitMQBalanceCreateQueue,
		Logger:                 logger,
	}

	producerRabbitMQRepository := rabbitmq.NewProducerRabbitMQ(rabbitMQConnection)

	// Get DB connection from migration wrapper for transaction management in UseCase
	// This ensures DBProvider uses the same validated connection as repositories
	dbConn, err := migrationWrapper.GetConnection().GetDB()
	assert.NoError(err, "database connection required for UseCase DBProvider",
		"package", "bootstrap",
		"function", "InitServers")

	// Wrap dbresolver.DB to implement dbtx.TxBeginner interface
	dbProvider := &dbProviderAdapter{db: dbConn}

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
		OutboxRepo:           outboxPostgreSQLRepository,
		DBProvider:           dbProvider,
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

	rabbitConsumerSource := fmt.Sprintf("%s://%s:%s@%s:%s",
		cfg.RabbitURI, cfg.RabbitMQConsumerUser, cfg.RabbitMQConsumerPass, cfg.RabbitMQHost, cfg.RabbitMQPortHost)

	rabbitMQConsumerConnection := &libRabbitmq.RabbitMQConnection{
		ConnectionStringSource: rabbitConsumerSource,
		HealthCheckURL:         cfg.RabbitMQHealthCheckURL,
		Host:                   cfg.RabbitMQHost,
		Port:                   cfg.RabbitMQPortAMQP,
		User:                   cfg.RabbitMQConsumerUser,
		Pass:                   cfg.RabbitMQConsumerPass,
		Queue:                  cfg.RabbitMQBalanceCreateQueue,
		Logger:                 logger,
	}

	routes := rabbitmq.NewConsumerRoutes(rabbitMQConsumerConnection, cfg.RabbitMQNumbersOfWorkers, cfg.RabbitMQNumbersOfPrefetch, logger, telemetry)

	multiQueueConsumer := NewMultiQueueConsumer(routes, useCase)

	auth := middleware.NewAuthClient(cfg.AuthHost, cfg.AuthEnabled, &logger)

	app := in.NewRouter(logger, telemetry, cfg.OtelServiceVersion, cfg.EnvName, auth, transactionHandler, operationHandler, assetRateHandler, balanceHandler, operationRouteHandler, transactionRouteHandler)

	server := NewServer(cfg, app, logger, telemetry)

	grpcApp := grpcIn.NewRouterGRPC(logger, telemetry, auth, useCase, queryUseCase)
	serverGRPC := NewServerGRPC(cfg, grpcApp, logger, telemetry)

	redisConsumer := NewRedisQueueConsumer(logger, *transactionHandler)

	const (
		defaultBalanceSyncWorkerEnabled = false
		defaultBalanceSyncMaxWorkers    = 5
	)

	balanceSyncWorkerEnabled := cfg.BalanceSyncWorkerEnabled
	balanceSyncMaxWorkers := cfg.BalanceSyncMaxWorkers

	if !balanceSyncWorkerEnabled {
		logger.Info("BalanceSyncWorker using default: BALANCE_SYNC_WORKER_ENABLED=false")
	}

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

	// DLQ Consumer - monitors Dead Letter Queues and replays messages after infrastructure recovery
	var dlqConsumer *DLQConsumer

	// H5: Use cfg field instead of os.Getenv (configuration inconsistency fix)
	if cfg.DLQConsumerEnabled {
		// Get queue names from environment (same ones used by MultiQueueConsumer)
		queueNames := []string{
			cfg.RabbitMQBalanceCreateQueue,
			cfg.RabbitMQTransactionBalanceOperationQueue,
		}

		dlqConsumer = NewDLQConsumer(
			logger,
			rabbitMQConsumerConnection,
			postgresConnection,
			redisConnection,
			queueNames,
		)
		logger.Info("DLQConsumer enabled - will monitor and replay failed messages")
	} else {
		logger.Info("DLQConsumer disabled (set DLQ_CONSUMER_ENABLED=true to enable)")
	}

	// Metadata Outbox Worker - processes pending metadata entries from outbox to MongoDB
	const (
		defaultMetadataOutboxMaxWorkers    = 5
		defaultMetadataOutboxRetentionDays = 7
	)

	var metadataOutboxWorker *MetadataOutboxWorker

	metadataOutboxMaxWorkers := cfg.MetadataOutboxMaxWorkers
	if metadataOutboxMaxWorkers <= 0 {
		metadataOutboxMaxWorkers = defaultMetadataOutboxMaxWorkers
		logger.Infof("MetadataOutboxWorker using default: METADATA_OUTBOX_MAX_WORKERS=%d", defaultMetadataOutboxMaxWorkers)
	}

	metadataOutboxRetentionDays := cfg.MetadataOutboxRetentionDays
	if metadataOutboxRetentionDays <= 0 {
		metadataOutboxRetentionDays = defaultMetadataOutboxRetentionDays
		logger.Infof("MetadataOutboxWorker using default: METADATA_OUTBOX_RETENTION_DAYS=%d", defaultMetadataOutboxRetentionDays)
	}

	if cfg.MetadataOutboxWorkerEnabled {
		metadataOutboxWorker = NewMetadataOutboxWorker(
			logger,
			outboxPostgreSQLRepository,
			metadataMongoDBRepository,
			postgresConnection,
			mongoConnection,
			metadataOutboxMaxWorkers,
			metadataOutboxRetentionDays,
		)
		logger.Infof("MetadataOutboxWorker enabled with %d max workers and %d days retention.",
			metadataOutboxMaxWorkers, metadataOutboxRetentionDays)
	} else {
		logger.Info("MetadataOutboxWorker disabled (set METADATA_OUTBOX_WORKER_ENABLED=true to enable)")
	}

	return &Service{
		Server:                      server,
		ServerGRPC:                  serverGRPC,
		MultiQueueConsumer:          multiQueueConsumer,
		RedisQueueConsumer:          redisConsumer,
		BalanceSyncWorker:           balanceSyncWorker,
		BalanceSyncWorkerEnabled:    cfg.BalanceSyncWorkerEnabled,
		DLQConsumer:                 dlqConsumer,
		DLQConsumerEnabled:          cfg.DLQConsumerEnabled, // H5: Use cfg field consistently
		MetadataOutboxWorker:        metadataOutboxWorker,
		MetadataOutboxWorkerEnabled: cfg.MetadataOutboxWorkerEnabled,
		Logger:                      logger,
		balancePort:                 useCase,
		auth:                        auth,
		transactionHandler:          transactionHandler,
		operationHandler:            operationHandler,
		assetRateHandler:            assetRateHandler,
		balanceHandler:              balanceHandler,
		operationRouteHandler:       operationRouteHandler,
		transactionRouteHandler:     transactionRouteHandler,
	}
}

// Options configures the transaction service initialization behavior.
type Options struct {
	// Logger allows callers to provide a pre-configured logger.
	Logger libLog.Logger
}

// InitServersWithOptions initializes servers with custom options.
// This function provides explicit error handling.
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

	// If options are provided, use InitServers but with the provided logger
	// For now, this just delegates to InitServers since it already initializes
	// everything. In the future, this could be refactored to use opts.Logger.
	return InitServers(), nil
}
