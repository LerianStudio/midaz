package bootstrap

import (
	"fmt"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mcasdoor"
	"github.com/LerianStudio/midaz/common/mmongo"
	"github.com/LerianStudio/midaz/common/mopentelemetry"
	"github.com/LerianStudio/midaz/common/mpostgres"
	"github.com/LerianStudio/midaz/common/mrabbitmq"
	"github.com/LerianStudio/midaz/common/mredis"
	"github.com/LerianStudio/midaz/common/mzap"
	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/database/mongodb"
	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/database/postgres/account"
	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/database/postgres/asset"
	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/database/postgres/ledger"
	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/database/postgres/organization"
	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/database/postgres/portfolio"
	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/database/postgres/product"
	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/database/redis"
	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/components/ledger/internal/bootstrap/grpc"
	"github.com/LerianStudio/midaz/components/ledger/internal/bootstrap/http"
	"github.com/LerianStudio/midaz/components/ledger/internal/services/command"
	"github.com/LerianStudio/midaz/components/ledger/internal/services/query"
)

const ApplicationName = "ledger"

// Config is the top level configuration struct for the entire application.
type Config struct {
	EnvName                 string `env:"ENV_NAME"`
	LogLevel                string `env:"LOG_LEVEL"`
	ServerAddress           string `env:"SERVER_ADDRESS"`
	ProtoAddress            string `env:"PROTO_ADDRESS"`
	PrimaryDBHost           string `env:"DB_HOST"`
	PrimaryDBUser           string `env:"DB_USER"`
	PrimaryDBPassword       string `env:"DB_PASSWORD"`
	PrimaryDBName           string `env:"DB_NAME"`
	PrimaryDBPort           string `env:"DB_PORT"`
	ReplicaDBHost           string `env:"DB_REPLICA_HOST"`
	ReplicaDBUser           string `env:"DB_REPLICA_USER"`
	ReplicaDBPassword       string `env:"DB_REPLICA_PASSWORD"`
	ReplicaDBName           string `env:"DB_REPLICA_NAME"`
	ReplicaDBPort           string `env:"DB_REPLICA_PORT"`
	MongoDBHost             string `env:"MONGO_HOST"`
	MongoDBName             string `env:"MONGO_NAME"`
	MongoDBUser             string `env:"MONGO_USER"`
	MongoDBPassword         string `env:"MONGO_PASSWORD"`
	MongoDBPort             string `env:"MONGO_PORT"`
	CasdoorAddress          string `env:"CASDOOR_ADDRESS"`
	CasdoorClientID         string `env:"CASDOOR_CLIENT_ID"`
	CasdoorClientSecret     string `env:"CASDOOR_CLIENT_SECRET"`
	CasdoorOrganizationName string `env:"CASDOOR_ORGANIZATION_NAME"`
	CasdoorApplicationName  string `env:"CASDOOR_APPLICATION_NAME"`
	CasdoorEnforcerName     string `env:"CASDOOR_ENFORCER_NAME"`
	JWKAddress              string `env:"CASDOOR_JWK_ADDRESS"`
	RabbitMQHost            string `env:"RABBITMQ_HOST"`
	RabbitMQPortHost        string `env:"RABBITMQ_PORT_HOST"`
	RabbitMQPortAMQP        string `env:"RABBITMQ_PORT_AMPQ"`
	RabbitMQUser            string `env:"RABBITMQ_DEFAULT_USER"`
	RabbitMQPass            string `env:"RABBITMQ_DEFAULT_PASS"`
	RabbitMQExchange        string `env:"RABBITMQ_EXCHANGE"`
	RabbitMQKey             string `env:"RABBITMQ_KEY"`
	RabbitMQQueue           string `env:"RABBITMQ_QUEUE"`
	OtelServiceName         string `env:"OTEL_RESOURCE_SERVICE_NAME"`
	OtelLibraryName         string `env:"OTEL_LIBRARY_NAME"`
	OtelServiceVersion      string `env:"OTEL_RESOURCE_SERVICE_VERSION"`
	OtelDeploymentEnv       string `env:"OTEL_RESOURCE_DEPLOYMENT_ENVIRONMENT"`
	OtelColExporterEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	RedisHost               string `env:"REDIS_HOST"`
	RedisPort               string `env:"REDIS_PORT"`
	RedisUser               string `env:"REDIS_USER"`
	RedisPassword           string `env:"REDIS_PASSWORD"`
}

// InitServers initiate http and grpc servers.
func InitServers() *Service {
	cfg := &Config{}

	if err := common.SetConfigFromEnvVars(cfg); err != nil {
		panic(err)
	}

	logger := mzap.InitializeLogger()

	telemetry := &mopentelemetry.Telemetry{
		LibraryName:               cfg.OtelLibraryName,
		ServiceName:               cfg.OtelServiceName,
		ServiceVersion:            cfg.OtelServiceVersion,
		DeploymentEnv:             cfg.OtelDeploymentEnv,
		CollectorExporterEndpoint: cfg.OtelColExporterEndpoint,
	}

	casDoorConnection := &mcasdoor.CasdoorConnection{
		JWKUri:           cfg.JWKAddress,
		Endpoint:         cfg.CasdoorAddress,
		ClientID:         cfg.CasdoorClientID,
		ClientSecret:     cfg.CasdoorClientSecret,
		OrganizationName: cfg.CasdoorOrganizationName,
		ApplicationName:  cfg.CasdoorApplicationName,
		EnforcerName:     cfg.CasdoorEnforcerName,
		Logger:           logger,
	}

	postgreSourcePrimary := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
		cfg.PrimaryDBHost, cfg.PrimaryDBUser, cfg.PrimaryDBPassword, cfg.PrimaryDBName, cfg.PrimaryDBPort)

	postgreSourceReplica := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
		cfg.ReplicaDBHost, cfg.ReplicaDBUser, cfg.ReplicaDBPassword, cfg.ReplicaDBName, cfg.ReplicaDBPort)

	postgresConnection := &mpostgres.PostgresConnection{
		ConnectionStringPrimary: postgreSourcePrimary,
		ConnectionStringReplica: postgreSourceReplica,
		PrimaryDBName:           cfg.PrimaryDBName,
		ReplicaDBName:           cfg.ReplicaDBName,
		Component:               ApplicationName,
		Logger:                  logger,
	}

	mongoSource := fmt.Sprintf("mongodb://%s:%s@%s:%s",
		cfg.MongoDBUser, cfg.MongoDBPassword, cfg.MongoDBHost, cfg.MongoDBPort)

	mongoConnection := &mmongo.MongoConnection{
		ConnectionStringSource: mongoSource,
		Database:               cfg.MongoDBName,
		Logger:                 logger,
	}

	rabbitSource := fmt.Sprintf("amqp://%s:%s@%s:%s",
		cfg.RabbitMQUser, cfg.RabbitMQPass, cfg.RabbitMQHost, cfg.RabbitMQPortHost)

	rabbitMQConnection := &mrabbitmq.RabbitMQConnection{
		ConnectionStringSource: rabbitSource,
		Host:                   cfg.RabbitMQHost,
		Port:                   cfg.RabbitMQPortAMQP,
		User:                   cfg.RabbitMQUser,
		Pass:                   cfg.RabbitMQPass,
		Exchange:               cfg.RabbitMQExchange,
		Key:                    cfg.RabbitMQKey,
		Queue:                  cfg.RabbitMQQueue,
		Logger:                 logger,
	}

	redisSource := fmt.Sprintf("%s:%s", cfg.RedisHost, cfg.RedisPort)

	redisConnection := &mredis.RedisConnection{
		Addr:     redisSource,
		User:     cfg.RedisUser,
		Password: cfg.RedisPassword,
		DB:       0,
		Protocol: 3,
		Logger:   logger,
	}

	organizationPostgreSQLRepository := organization.NewOrganizationPostgreSQLRepository(postgresConnection)
	ledgerPostgreSQLRepository := ledger.NewLedgerPostgreSQLRepository(postgresConnection)
	productPostgreSQLRepository := product.NewProductPostgreSQLRepository(postgresConnection)
	portfolioPostgreSQLRepository := portfolio.NewPortfolioPostgreSQLRepository(postgresConnection)
	accountPostgreSQLRepository := account.NewAccountPostgreSQLRepository(postgresConnection)
	assetPostgreSQLRepository := asset.NewAssetPostgreSQLRepository(postgresConnection)

	metadataMongoDBRepository := mongodb.NewMetadataMongoDBRepository(mongoConnection)

	producerRabbitMQRepository := rabbitmq.NewProducerRabbitMQ(rabbitMQConnection)
	consumerRabbitMQRepository := rabbitmq.NewConsumerRabbitMQ(rabbitMQConnection)

	redisConsumerRepository := redis.NewConsumerRedis(redisConnection)

	commandUseCase := &command.UseCase{
		OrganizationRepo: organizationPostgreSQLRepository,
		LedgerRepo:       ledgerPostgreSQLRepository,
		ProductRepo:      productPostgreSQLRepository,
		PortfolioRepo:    portfolioPostgreSQLRepository,
		AccountRepo:      accountPostgreSQLRepository,
		AssetRepo:        assetPostgreSQLRepository,
		MetadataRepo:     metadataMongoDBRepository,
		RabbitMQRepo:     producerRabbitMQRepository,
		RedisRepo:        redisConsumerRepository,
	}

	queryUseCase := &query.UseCase{
		OrganizationRepo: organizationPostgreSQLRepository,
		LedgerRepo:       ledgerPostgreSQLRepository,
		ProductRepo:      productPostgreSQLRepository,
		PortfolioRepo:    portfolioPostgreSQLRepository,
		AccountRepo:      accountPostgreSQLRepository,
		AssetRepo:        assetPostgreSQLRepository,
		MetadataRepo:     metadataMongoDBRepository,
		RabbitMQRepo:     consumerRabbitMQRepository,
		RedisRepo:        redisConsumerRepository,
	}

	accountHandler := &http.AccountHandler{
		Command: commandUseCase,
		Query:   queryUseCase,
	}

	portfolioHandler := &http.PortfolioHandler{
		Command: commandUseCase,
		Query:   queryUseCase,
	}

	ledgerHandler := &http.LedgerHandler{
		Command: commandUseCase,
		Query:   queryUseCase,
	}

	assetHandler := &http.AssetHandler{
		Command: commandUseCase,
		Query:   queryUseCase,
	}

	organizationHandler := &http.OrganizationHandler{
		Command: commandUseCase,
		Query:   queryUseCase,
	}

	productHandler := &http.ProductHandler{
		Command: commandUseCase,
		Query:   queryUseCase,
	}

	httpApp := http.NewRouter(logger, telemetry, casDoorConnection, accountHandler, portfolioHandler, ledgerHandler, assetHandler, organizationHandler, productHandler)

	serverAPI := NewServer(cfg, httpApp, logger, telemetry)

	grpcApp := grpc.NewRouterGRPC(logger, telemetry, casDoorConnection, commandUseCase, queryUseCase)

	serverGRPC := NewServerGRPC(cfg, grpcApp, logger)

	return &Service{
		Server:     serverAPI,
		ServerGRPC: serverGRPC,
		Logger:     logger,
	}
}
