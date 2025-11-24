package bootstrap

import (
	"fmt"

	"strings"
	"time"

	"github.com/LerianStudio/lib-auth/v2/auth/middleware"
	libCommons "github.com/LerianStudio/lib-commons/v2/commons"
	libMongo "github.com/LerianStudio/lib-commons/v2/commons/mongo"
	libOpentelemetry "github.com/LerianStudio/lib-commons/v2/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/v2/commons/postgres"
	libRabbitmq "github.com/LerianStudio/lib-commons/v2/commons/rabbitmq"
	libRedis "github.com/LerianStudio/lib-commons/v2/commons/redis"
	libZap "github.com/LerianStudio/lib-commons/v2/commons/zap"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/accounttype"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/organization"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/portfolio"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/segment"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transactionroute"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/services/query"
)

const ApplicationName = "transaction"

// Config is the top level configuration struct for the entire application.
type Config struct {
	EnvName       string `env:"ENV_NAME"`
	LogLevel      string `env:"LOG_LEVEL"`
	ServerAddress string `env:"SERVER_ADDRESS"`

	// -- ONBOARDING

	// -- PostgreSQL primary configuration
	PrimaryOnboardingDBHost     string `env:"DB_ONBOARDING_HOST"`
	PrimaryOnboardingDBUser     string `env:"DB_ONBOARDING_USER"`
	PrimaryOnboardingDBPassword string `env:"DB_ONBOARDING_PASSWORD"`
	PrimaryOnboardingDBName     string `env:"DB_ONBOARDING_NAME"`
	PrimaryOnboardingDBPort     string `env:"DB_ONBOARDING_PORT"`
	PrimaryOnboardingDBSSLMode  string `env:"DB_ONBOARDING_SSLMODE"`

	// -- PostgreSQL replica configuration
	ReplicaOnboardingDBHost      string `env:"DB_ONBOARDING_REPLICA_HOST"`
	ReplicaOnboardingDBUser      string `env:"DB_ONBOARDING_REPLICA_USER"`
	ReplicaOnboardingDBPassword  string `env:"DB_ONBOARDING_REPLICA_PASSWORD"`
	ReplicaOnboardingDBName      string `env:"DB_ONBOARDING_REPLICA_NAME"`
	ReplicaOnboardingDBPort      string `env:"DB_ONBOARDING_REPLICA_PORT"`
	ReplicaOnboardingDBSSLMode   string `env:"DB_ONBOARDING_REPLICA_SSLMODE"`
	MaxOnboardingOpenConnections int    `env:"DB_ONBOARDING_MAX_OPEN_CONNS"`
	MaxOnboardingIdleConnections int    `env:"DB_ONBOARDING_MAX_IDLE_CONNS"`

	// -- MongoDB configuration
	MongoOnboardingURI        string `env:"MONGO_ONBOARDING_URI"`
	MongoOnboardingHost       string `env:"MONGO_ONBOARDING_HOST"`
	MongoOnboardingName       string `env:"MONGO_ONBOARDING_NAME"`
	MongoOnboardingUser       string `env:"MONGO_ONBOARDING_USER"`
	MongoOnboardingPassword   string `env:"MONGO_ONBOARDING_PASSWORD"`
	MongoOnboardingPort       string `env:"MONGO_ONBOARDING_PORT"`
	MongoOnboardingParameters string `env:"MONGO_ONBOARDING_PARAMETERS"`
	MaxOnboardingPoolSize     int    `env:"MONGO_ONBOARDING_MAX_POOL_SIZE"`

	// -- TRANSACTION

	// -- PostgreSQL primary configuration
	PrimaryTransactionDBHost     string `env:"DB_TRANSACTION_HOST"`
	PrimaryTransactionDBUser     string `env:"DB_TRANSACTION_USER"`
	PrimaryTransactionDBPassword string `env:"DB_TRANSACTION_PASSWORD"`
	PrimaryTransactionDBName     string `env:"DB_TRANSACTION_NAME"`
	PrimaryTransactionDBPort     string `env:"DB_TRANSACTION_PORT"`
	PrimaryTransactionDBSSLMode  string `env:"DB_TRANSACTION_SSLMODE"`

	// -- PostgreSQL replica configuration
	ReplicaTransactionDBHost      string `env:"DB_TRANSACTION_REPLICA_HOST"`
	ReplicaTransactionDBUser      string `env:"DB_TRANSACTION_REPLICA_USER"`
	ReplicaTransactionDBPassword  string `env:"DB_TRANSACTION_REPLICA_PASSWORD"`
	ReplicaTransactionDBName      string `env:"DB_TRANSACTION_REPLICA_NAME"`
	ReplicaTransactionDBPort      string `env:"DB_TRANSACTION_REPLICA_PORT"`
	ReplicaTransactionDBSSLMode   string `env:"DB_TRANSACTION_REPLICA_SSLMODE"`
	MaxTransactionOpenConnections int    `env:"DB_TRANSACTION_MAX_OPEN_CONNS"`
	MaxTransactionIdleConnections int    `env:"DB_TRANSACTION_MAX_IDLE_CONNS"`

	// -- MongoDB configuration
	MongoTransactionURI        string `env:"MONGO_TRANSACTION_URI"`
	MongoTransactionHost       string `env:"MONGO_TRANSACTION_HOST"`
	MongoTransactionName       string `env:"MONGO_TRANSACTION_NAME"`
	MongoTransactionUser       string `env:"MONGO_TRANSACTION_USER"`
	MongoTransactionPassword   string `env:"MONGO_TRANSACTION_PASSWORD"`
	MongoTransactionPort       string `env:"MONGO_TRANSACTION_PORT"`
	MongoTransactionParameters string `env:"MONGO_TRANSACTION_PARAMETERS"`
	MaxTransactionPoolSize     int    `env:"MONGO_TRANSACTION_MAX_POOL_SIZE"`

	// -- RabbitMQ configuration
	RabbitURI                  string `env:"RABBITMQ_URI"`
	RabbitMQHost               string `env:"RABBITMQ_HOST"`
	RabbitMQPortHost           string `env:"RABBITMQ_PORT_HOST"`
	RabbitMQPortAMQP           string `env:"RABBITMQ_PORT_AMQP"`
	RabbitMQUser               string `env:"RABBITMQ_DEFAULT_USER"`
	RabbitMQPass               string `env:"RABBITMQ_DEFAULT_PASS"`
	RabbitMQConsumerUser       string `env:"RABBITMQ_CONSUMER_USER"`
	RabbitMQConsumerPass       string `env:"RABBITMQ_CONSUMER_PASS"`
	RabbitMQBalanceCreateQueue string `env:"RABBITMQ_BALANCE_CREATE_QUEUE"`
	RabbitMQNumbersOfWorkers   int    `env:"RABBITMQ_NUMBERS_OF_WORKERS"`
	RabbitMQNumbersOfPrefetch  int    `env:"RABBITMQ_NUMBERS_OF_PREFETCH"`
	RabbitMQHealthCheckURL     string `env:"RABBITMQ_HEALTH_CHECK_URL"`

	// -- Otel configuration
	OtelServiceName         string `env:"OTEL_RESOURCE_SERVICE_NAME"`
	OtelLibraryName         string `env:"OTEL_LIBRARY_NAME"`
	OtelServiceVersion      string `env:"OTEL_RESOURCE_SERVICE_VERSION"`
	OtelDeploymentEnv       string `env:"OTEL_RESOURCE_DEPLOYMENT_ENVIRONMENT"`
	OtelColExporterEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	EnableTelemetry         bool   `env:"ENABLE_TELEMETRY"`

	// -- Redis configuration
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

	// -- Auth configuration
	AuthEnabled bool   `env:"PLUGIN_AUTH_ENABLED"`
	AuthHost    string `env:"PLUGIN_AUTH_HOST"`
}

// InitServers initiate http and grpc servers.
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

	// -- ONBOARDING

	postgreSourcePrimaryOnboarding := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s",
		cfg.PrimaryOnboardingDBHost, cfg.PrimaryOnboardingDBUser, cfg.PrimaryOnboardingDBPassword, cfg.PrimaryOnboardingDBName, cfg.PrimaryOnboardingDBPort, cfg.PrimaryOnboardingDBSSLMode)

	postgreSourceReplicaOnboarding := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s",
		cfg.ReplicaOnboardingDBHost, cfg.ReplicaOnboardingDBUser, cfg.ReplicaOnboardingDBPassword, cfg.ReplicaOnboardingDBName, cfg.ReplicaOnboardingDBPort, cfg.ReplicaOnboardingDBSSLMode)

	postgresConnectionOnboarding := &libPostgres.PostgresConnection{
		ConnectionStringPrimary: postgreSourcePrimaryOnboarding,
		ConnectionStringReplica: postgreSourceReplicaOnboarding,
		PrimaryDBName:           cfg.PrimaryOnboardingDBName,
		ReplicaDBName:           cfg.ReplicaOnboardingDBName,
		Component:               "onboarding",
		Logger:                  logger,
		MaxOpenConnections:      cfg.MaxOnboardingOpenConnections,
		MaxIdleConnections:      cfg.MaxOnboardingIdleConnections,
	}

	mongoSourceOnboarding := fmt.Sprintf("%s://%s:%s@%s:%s/",
		cfg.MongoOnboardingURI, cfg.MongoOnboardingUser, cfg.MongoOnboardingPassword, cfg.MongoOnboardingHost, cfg.MongoOnboardingPort)

	if cfg.MaxOnboardingPoolSize <= 0 {
		cfg.MaxOnboardingPoolSize = 100
	}

	if cfg.MongoOnboardingParameters != "" {
		mongoSourceOnboarding += "?" + cfg.MongoOnboardingParameters
	}

	mongoConnectionOnboarding := &libMongo.MongoConnection{
		ConnectionStringSource: mongoSourceOnboarding,
		Database:               cfg.MongoOnboardingName,
		Logger:                 logger,
		MaxPoolSize:            uint64(cfg.MaxOnboardingPoolSize),
	}

	// -- TRANSACTION

	postgreSourcePrimaryTransaction := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s",
		cfg.PrimaryTransactionDBHost, cfg.PrimaryTransactionDBUser, cfg.PrimaryTransactionDBPassword, cfg.PrimaryTransactionDBName, cfg.PrimaryTransactionDBPort, cfg.PrimaryTransactionDBSSLMode)

	postgreSourceReplicaTransaction := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=%s",
		cfg.ReplicaTransactionDBHost, cfg.ReplicaTransactionDBUser, cfg.ReplicaTransactionDBPassword, cfg.ReplicaTransactionDBName, cfg.ReplicaTransactionDBPort, cfg.ReplicaTransactionDBSSLMode)

	postgresConnectionTransaction := &libPostgres.PostgresConnection{
		ConnectionStringPrimary: postgreSourcePrimaryTransaction,
		ConnectionStringReplica: postgreSourceReplicaTransaction,
		PrimaryDBName:           cfg.PrimaryTransactionDBName,
		ReplicaDBName:           cfg.ReplicaTransactionDBName,
		Component:               "transaction",
		Logger:                  logger,
		MaxOpenConnections:      cfg.MaxTransactionOpenConnections,
		MaxIdleConnections:      cfg.MaxTransactionIdleConnections,
	}

	mongoSourceTransaction := fmt.Sprintf("%s://%s:%s@%s:%s/",
		cfg.MongoTransactionURI, cfg.MongoTransactionUser, cfg.MongoTransactionPassword, cfg.MongoTransactionHost, cfg.MongoTransactionPort)

	if cfg.MaxTransactionPoolSize <= 0 {
		cfg.MaxTransactionPoolSize = 100
	}

	if cfg.MongoTransactionParameters != "" {
		mongoSourceTransaction += "?" + cfg.MongoTransactionParameters
	}

	mongoConnectionTransaction := &libMongo.MongoConnection{
		ConnectionStringSource: mongoSourceTransaction,
		Database:               cfg.MongoTransactionName,
		Logger:                 logger,
		MaxPoolSize:            uint64(cfg.MaxTransactionPoolSize),
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

	// -- ONBOARDING
	organizationPostgreSQLRepository := organization.NewOrganizationPostgreSQLRepository(postgresConnectionOnboarding)
	ledgerPostgreSQLRepository := ledger.NewLedgerPostgreSQLRepository(postgresConnectionOnboarding)
	segmentPostgreSQLRepository := segment.NewSegmentPostgreSQLRepository(postgresConnectionOnboarding)
	portfolioPostgreSQLRepository := portfolio.NewPortfolioPostgreSQLRepository(postgresConnectionOnboarding)
	accountPostgreSQLRepository := account.NewAccountPostgreSQLRepository(postgresConnectionOnboarding)
	assetPostgreSQLRepository := asset.NewAssetPostgreSQLRepository(postgresConnectionOnboarding)
	accountTypePostgreSQLRepository := accounttype.NewAccountTypePostgreSQLRepository(postgresConnectionOnboarding)

	metadataOnboardingMongoDBRepository := mongodb.NewMetadataMongoDBRepository(mongoConnectionOnboarding)

	// -- TRANSACTION
	transactionPostgreSQLRepository := transaction.NewTransactionPostgreSQLRepository(postgresConnectionTransaction)
	operationPostgreSQLRepository := operation.NewOperationPostgreSQLRepository(postgresConnectionTransaction)
	assetRatePostgreSQLRepository := assetrate.NewAssetRatePostgreSQLRepository(postgresConnectionTransaction)
	balancePostgreSQLRepository := balance.NewBalancePostgreSQLRepository(postgresConnectionTransaction)
	operationRoutePostgreSQLRepository := operationroute.NewOperationRoutePostgreSQLRepository(postgresConnectionTransaction)
	transactionRoutePostgreSQLRepository := transactionroute.NewTransactionRoutePostgreSQLRepository(postgresConnectionTransaction)

	metadataTransactionMongoDBRepository := mongodb.NewMetadataMongoDBRepository(mongoConnectionTransaction)

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

	useCase := &command.UseCase{
		// -- ONBOARDING
		OrganizationRepo:       organizationPostgreSQLRepository,
		LedgerRepo:             ledgerPostgreSQLRepository,
		SegmentRepo:            segmentPostgreSQLRepository,
		PortfolioRepo:          portfolioPostgreSQLRepository,
		AccountRepo:            accountPostgreSQLRepository,
		AssetRepo:              assetPostgreSQLRepository,
		AccountTypeRepo:        accountTypePostgreSQLRepository,
		MetadataOnboardingRepo: metadataOnboardingMongoDBRepository,

		// -- TRANSACTION
		TransactionRepo:         transactionPostgreSQLRepository,
		OperationRepo:           operationPostgreSQLRepository,
		AssetRateRepo:           assetRatePostgreSQLRepository,
		BalanceRepo:             balancePostgreSQLRepository,
		OperationRouteRepo:      operationRoutePostgreSQLRepository,
		TransactionRouteRepo:    transactionRoutePostgreSQLRepository,
		MetadataTransactionRepo: metadataTransactionMongoDBRepository,
		RabbitMQRepo:            producerRabbitMQRepository,
		RedisRepo:               redisConsumerRepository,
	}

	queryUseCase := &query.UseCase{
		// -- ONBOARDING
		OrganizationRepo:       organizationPostgreSQLRepository,
		LedgerRepo:             ledgerPostgreSQLRepository,
		SegmentRepo:            segmentPostgreSQLRepository,
		PortfolioRepo:          portfolioPostgreSQLRepository,
		AccountRepo:            accountPostgreSQLRepository,
		AssetRepo:              assetPostgreSQLRepository,
		AccountTypeRepo:        accountTypePostgreSQLRepository,
		MetadataOnboardingRepo: metadataOnboardingMongoDBRepository,

		// -- TRANSACTION
		TransactionRepo:         transactionPostgreSQLRepository,
		OperationRepo:           operationPostgreSQLRepository,
		AssetRateRepo:           assetRatePostgreSQLRepository,
		BalanceRepo:             balancePostgreSQLRepository,
		OperationRouteRepo:      operationRoutePostgreSQLRepository,
		TransactionRouteRepo:    transactionRoutePostgreSQLRepository,
		MetadataTransactionRepo: metadataTransactionMongoDBRepository,
		RabbitMQRepo:            producerRabbitMQRepository,
		RedisRepo:               redisConsumerRepository,
	}

	accountHandler := &in.AccountHandler{
		Command: useCase,
		Query:   queryUseCase,
	}

	portfolioHandler := &in.PortfolioHandler{
		Command: useCase,
		Query:   queryUseCase,
	}

	ledgerHandler := &in.LedgerHandler{
		Command: useCase,
		Query:   queryUseCase,
	}

	assetHandler := &in.AssetHandler{
		Command: useCase,
		Query:   queryUseCase,
	}

	organizationHandler := &in.OrganizationHandler{
		Command: useCase,
		Query:   queryUseCase,
	}

	segmentHandler := &in.SegmentHandler{
		Command: useCase,
		Query:   queryUseCase,
	}

	accountTypeHandler := &in.AccountTypeHandler{
		Command: useCase,
		Query:   queryUseCase,
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

	handlers := &in.Handlers{
		Account:          accountHandler,
		Portfolio:        portfolioHandler,
		Ledger:           ledgerHandler,
		Asset:            assetHandler,
		Organization:     organizationHandler,
		Segment:          segmentHandler,
		AccountType:      accountTypeHandler,
		Transaction:      transactionHandler,
		Operation:        operationHandler,
		AssetRate:        assetRateHandler,
		Balance:          balanceHandler,
		OperationRoute:   operationRouteHandler,
		TransactionRoute: transactionRouteHandler,
	}

	app := in.NewRouter(logger, telemetry, auth, handlers)

	server := NewServer(cfg, app, logger, telemetry)

	redisConsumer := NewRedisQueueConsumer(logger, *transactionHandler)
	balanceSyncWorker := NewBalanceSyncWorker(redisConnection, logger, useCase)

	return &Service{
		Server:             server,
		MultiQueueConsumer: multiQueueConsumer,
		RedisQueueConsumer: redisConsumer,
		BalanceSyncWorker:  balanceSyncWorker,
		Logger:             logger,
	}
}
