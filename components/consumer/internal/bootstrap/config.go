package bootstrap

import (
	"fmt"
	libCommons "github.com/LerianStudio/lib-commons/commons"
	libMongo "github.com/LerianStudio/lib-commons/commons/mongo"
	libOpentelemetry "github.com/LerianStudio/lib-commons/commons/opentelemetry"
	libPostgres "github.com/LerianStudio/lib-commons/commons/postgres"
	libRabbitmq "github.com/LerianStudio/lib-commons/commons/rabbitmq"
	libZap "github.com/LerianStudio/lib-commons/commons/zap"
	"github.com/LerianStudio/midaz/components/consumer/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/components/consumer/internal/adapters/postgresql/balance"
	"github.com/LerianStudio/midaz/components/consumer/internal/adapters/postgresql/operation"
	"github.com/LerianStudio/midaz/components/consumer/internal/adapters/postgresql/transaction"
	"github.com/LerianStudio/midaz/components/consumer/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/components/consumer/internal/services/commands"
)

const ApplicationName = "consumer"

// Config is the configuration struct for the consumer service.
type Config struct {
	EnvName                    string `env:"ENV_NAME"`
	LogLevel                   string `env:"LOG_LEVEL"`
	PrimaryDBHost              string `env:"DB_HOST"`
	PrimaryDBUser              string `env:"DB_USER"`
	PrimaryDBPassword          string `env:"DB_PASSWORD"`
	PrimaryDBName              string `env:"DB_NAME"`
	PrimaryDBPort              string `env:"DB_PORT"`
	ReplicaDBHost              string `env:"DB_REPLICA_HOST"`
	ReplicaDBUser              string `env:"DB_REPLICA_USER"`
	ReplicaDBPassword          string `env:"DB_REPLICA_PASSWORD"`
	ReplicaDBName              string `env:"DB_REPLICA_NAME"`
	ReplicaDBPort              string `env:"DB_REPLICA_PORT"`
	MaxOpenConnections         int    `env:"DB_MAX_OPEN_CONNS"`
	MaxIdleConnections         int    `env:"DB_MAX_IDLE_CONNS"`
	MongoURI                   string `env:"MONGO_URI"`
	MongoDBHost                string `env:"MONGO_HOST"`
	MongoDBName                string `env:"MONGO_NAME"`
	MongoDBUser                string `env:"MONGO_USER"`
	MongoDBPassword            string `env:"MONGO_PASSWORD"`
	MongoDBPort                string `env:"MONGO_PORT"`
	MaxPoolSize                int    `env:"MONGO_MAX_POOL_SIZE"`
	RabbitURI                  string `env:"RABBITMQ_URI"`
	RabbitMQHost               string `env:"RABBITMQ_HOST"`
	RabbitMQPortHost           string `env:"RABBITMQ_PORT_HOST"`
	RabbitMQPortAMQP           string `env:"RABBITMQ_PORT_AMQP"`
	RabbitMQUser               string `env:"RABBITMQ_DEFAULT_USER"`
	RabbitMQPass               string `env:"RABBITMQ_DEFAULT_PASS"`
	RabbitMQBalanceCreateQueue string `env:"RABBITMQ_BALANCE_CREATE_QUEUE"`
	RabbitMQNumbersOfWorkers   int    `env:"RABBITMQ_NUMBERS_OF_WORKERS"`
	RabbitMQNumbersOfPrefetch  int    `env:"RABBITMQ_NUMBERS_OF_PREFETCH"`
	RabbitMQHealthCheckURL     string `env:"RABBITMQ_HEALTH_CHECK_URL"`
	OtelServiceName            string `env:"OTEL_RESOURCE_SERVICE_NAME"`
	OtelLibraryName            string `env:"OTEL_LIBRARY_NAME"`
	OtelServiceVersion         string `env:"OTEL_RESOURCE_SERVICE_VERSION"`
	OtelDeploymentEnv          string `env:"OTEL_RESOURCE_DEPLOYMENT_ENVIRONMENT"`
	OtelColExporterEndpoint    string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	EnableTelemetry            bool   `env:"ENABLE_TELEMETRY"`
}

// InitConsumer initiate the consumer service.
func InitConsumer() *ConsumerService {
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

	transactionPostgreSQLRepository := transaction.NewTransactionPostgreSQLRepository(postgresConnection)
	operationPostgreSQLRepository := operation.NewOperationPostgreSQLRepository(postgresConnection)
	balancePostgreSQLRepository := balance.NewBalancePostgreSQLRepository(postgresConnection)

	metadataMongoDBRepository := mongodb.NewMetadataMongoDBRepository(mongoConnection)

	routes := rabbitmq.NewConsumerRoutes(rabbitMQConnection, cfg.RabbitMQNumbersOfWorkers, cfg.RabbitMQNumbersOfPrefetch, logger, telemetry)

	useCase := &commands.UseCase{
		TransactionRepo: transactionPostgreSQLRepository,
		OperationRepo:   operationPostgreSQLRepository,
		BalanceRepo:     balancePostgreSQLRepository,
		MetadataRepo:    metadataMongoDBRepository,
	}

	multiQueueConsumer := NewMultiQueueConsumer(routes, useCase)

	return &ConsumerService{
		MultiQueueConsumer: multiQueueConsumer,
		Logger:             logger,
	}
}
