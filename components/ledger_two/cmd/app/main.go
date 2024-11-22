package main

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
	"github.com/LerianStudio/midaz/components/ledger_two/internal/adapters/implementation/database/mongodb"
	"github.com/LerianStudio/midaz/components/ledger_two/internal/adapters/implementation/database/postgres"
	"github.com/LerianStudio/midaz/components/ledger_two/internal/adapters/implementation/database/redis"
	rabbitmq "github.com/LerianStudio/midaz/components/ledger_two/internal/adapters/implementation/rabbitmq"
	"github.com/LerianStudio/midaz/components/ledger_two/internal/bootstrap"
	"github.com/LerianStudio/midaz/components/ledger_two/internal/bootstrap/grpc"
	"github.com/LerianStudio/midaz/components/ledger_two/internal/bootstrap/http"
	"github.com/LerianStudio/midaz/components/ledger_two/internal/services/command"
	"github.com/LerianStudio/midaz/components/ledger_two/internal/services/query"
)

const ApplicationName = "ledger"

func main() {
	common.InitLocalEnvConfig()

	cfg := bootstrap.NewConfig()

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

	metadataMongoDBRepository := mongodb.NewMetadataMongoDBRepository(mongoConnection)

	organizationPostgreSQLRepository := postgres.NewOrganizationPostgreSQLRepository(postgresConnection)
	ledgerPostgreSQLRepository := postgres.NewLedgerPostgreSQLRepository(postgresConnection)
	productPostgreSQLRepository := postgres.NewProductPostgreSQLRepository(postgresConnection)
	portfolioPostgreSQLRepository := postgres.NewPortfolioPostgreSQLRepository(postgresConnection)
	accountPostgreSQLRepository := postgres.NewAccountPostgreSQLRepository(postgresConnection)
	assetPostgreSQLRepository := postgres.NewAssetPostgreSQLRepository(postgresConnection)

	producerRabbitMQRepository := rabbitmq.NewProducerRabbitMQ(rabbitMQConnection)
	consumerRabbitMQRepository := rabbitmq.NewConsumerRabbitMQ(rabbitMQConnection)
	redisConsumerRepository := redis.NewConsumerRedis(redisConnection)

	useCase := &command.UseCase{
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
		Command: useCase,
		Query:   queryUseCase,
	}

	portfolioHandler := &http.PortfolioHandler{
		Command: useCase,
		Query:   queryUseCase,
	}

	ledgerHandler := &http.LedgerHandler{
		Command: useCase,
		Query:   queryUseCase,
	}

	assetHandler := &http.AssetHandler{
		Command: useCase,
		Query:   queryUseCase,
	}

	organizationHandler := &http.OrganizationHandler{
		Command: useCase,
		Query:   queryUseCase,
	}

	productHandler := &http.ProductHandler{
		Command: useCase,
		Query:   queryUseCase,
	}

	app := http.NewRouter(logger, telemetry, casDoorConnection, accountHandler, portfolioHandler, ledgerHandler, assetHandler, organizationHandler, productHandler)

	server := bootstrap.NewServer(cfg, app, logger, telemetry)

	grpcServer := grpc.NewRouterGRPC(logger, telemetry, casDoorConnection, useCase, queryUseCase)

	serverGRPC := bootstrap.NewServerGRPC(cfg, grpcServer, logger)

	serviceService := &bootstrap.Service{
		Server:     server,
		ServerGRPC: serverGRPC,
		Logger:     logger,
	}

	serviceService.Run()
}
