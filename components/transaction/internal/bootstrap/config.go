package bootstrap

import (
	"fmt"

	"strings"
	"time"

	"github.com/LerianStudio/lib-auth/auth/middleware"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libMongo "github.com/LerianStudio/lib-commons/commons/mongo"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/commons/postgres"
	libRabbitmq "github.com/LerianStudio/lib-commons/commons/rabbitmq"
	libRedis "github.com/LerianStudio/lib-commons/commons/redis"
	libZap "github.com/LerianStudio/lib-commons/commons/zap"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/http/in"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/settings"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/transactionroute"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/redis"
	"github.com/LerianStudio/midaz/components/transaction/internal/services/command"
	"github.com/LerianStudio/midaz/components/transaction/internal/services/query"
)

const ApplicationName = "transaction"

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
	ReplicaDBHost                string `env:"DB_REPLICA_HOST"`
	ReplicaDBUser                string `env:"DB_REPLICA_USER"`
	ReplicaDBPassword            string `env:"DB_REPLICA_PASSWORD"`
	ReplicaDBName                string `env:"DB_REPLICA_NAME"`
	ReplicaDBPort                string `env:"DB_REPLICA_PORT"`
	MaxOpenConnections           int    `env:"DB_MAX_OPEN_CONNS"`
	MaxIdleConnections           int    `env:"DB_MAX_IDLE_CONNS"`
	MongoURI                     string `env:"MONGO_URI"`
	MongoDBHost                  string `env:"MONGO_HOST"`
	MongoDBName                  string `env:"MONGO_NAME"`
	MongoDBUser                  string `env:"MONGO_USER"`
	MongoDBPassword              string `env:"MONGO_PASSWORD"`
	MongoDBPort                  string `env:"MONGO_PORT"`
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
	AuthEnabled                  bool   `env:"PLUGIN_AUTH_ENABLED"`
	AuthHost                     string `env:"PLUGIN_AUTH_HOST"`
}

// InitServers initiate http and grpc servers.
func InitServers() *Service {
	cfg := &Config{}

	if err := libCommons.SetConfigFromEnvVars(cfg); err != nil {
		panic(err)
	}

	logger := libZap.InitializeLogger()

	telemetry := &libOpentelemetry.Telemetry{
		LibraryName:               cfg.OtelLibraryName,
		ServiceName:               cfg.OtelServiceName,
		ServiceVersion:            cfg.OtelServiceVersion,
		DeploymentEnv:             cfg.OtelDeploymentEnv,
		CollectorExporterEndpoint: cfg.OtelColExporterEndpoint,
		EnableTelemetry:           cfg.EnableTelemetry,
	}

	postgreSourcePrimary := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
		cfg.PrimaryDBHost, cfg.PrimaryDBUser, cfg.PrimaryDBPassword, cfg.PrimaryDBName, cfg.PrimaryDBPort)

	postgreSourceReplica := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
		cfg.ReplicaDBHost, cfg.ReplicaDBUser, cfg.ReplicaDBPassword, cfg.ReplicaDBName, cfg.ReplicaDBPort)

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

	mongoSource := fmt.Sprintf("%s://%s:%s@%s:%s",
		cfg.MongoURI, cfg.MongoDBUser, cfg.MongoDBPassword, cfg.MongoDBHost, cfg.MongoDBPort)

	if cfg.MaxPoolSize <= 0 {
		cfg.MaxPoolSize = 100
	}

	mongoConnection := &libMongo.MongoConnection{
		ConnectionStringSource: mongoSource,
		Database:               cfg.MongoDBName,
		Logger:                 logger,
		MaxPoolSize:            uint64(cfg.MaxPoolSize),
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
	}

	redisConsumerRepository := redis.NewConsumerRedis(redisConnection)

	transactionPostgreSQLRepository := transaction.NewTransactionPostgreSQLRepository(postgresConnection)
	operationPostgreSQLRepository := operation.NewOperationPostgreSQLRepository(postgresConnection)
	assetRatePostgreSQLRepository := assetrate.NewAssetRatePostgreSQLRepository(postgresConnection)
	balancePostgreSQLRepository := balance.NewBalancePostgreSQLRepository(postgresConnection)
	operationRoutePostgreSQLRepository := operationroute.NewOperationRoutePostgreSQLRepository(postgresConnection)
	transactionRoutePostgreSQLRepository := transactionroute.NewTransactionRoutePostgreSQLRepository(postgresConnection)
	settingsPostgreSQLRepository := settings.NewSettingsPostgreSQLRepository(postgresConnection)

	metadataMongoDBRepository := mongodb.NewMetadataMongoDBRepository(mongoConnection)

	producerRabbitMQRepository := rabbitmq.NewProducerRabbitMQ(rabbitMQConnection)
	routes := rabbitmq.NewConsumerRoutes(rabbitMQConnection, cfg.RabbitMQNumbersOfWorkers, cfg.RabbitMQNumbersOfPrefetch, logger, telemetry)

	useCase := &command.UseCase{
		TransactionRepo:      transactionPostgreSQLRepository,
		OperationRepo:        operationPostgreSQLRepository,
		AssetRateRepo:        assetRatePostgreSQLRepository,
		BalanceRepo:          balancePostgreSQLRepository,
		OperationRouteRepo:   operationRoutePostgreSQLRepository,
		TransactionRouteRepo: transactionRoutePostgreSQLRepository,
		SettingsRepo:         settingsPostgreSQLRepository,
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
		SettingsRepo:         settingsPostgreSQLRepository,
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

	settingsHandler := &in.SettingsHandler{
		Command: useCase,
		Query:   queryUseCase,
	}

	multiQueueConsumer := NewMultiQueueConsumer(routes, useCase)

	auth := middleware.NewAuthClient(cfg.AuthHost, cfg.AuthEnabled, &logger)

	app := in.NewRouter(logger, telemetry, auth, transactionHandler, operationHandler, assetRateHandler, balanceHandler, operationRouteHandler, transactionRouteHandler, settingsHandler)

	server := NewServer(cfg, app, logger, telemetry)

	return &Service{
		Server:             server,
		MultiQueueConsumer: multiQueueConsumer,
		Logger:             logger,
	}
}
