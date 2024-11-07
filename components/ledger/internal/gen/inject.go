//go:build wireinject
// +build wireinject

package gen

import (
	"fmt"
	"sync"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mcasdoor"
	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/common/mmongo"
	"github.com/LerianStudio/midaz/common/mpostgres"
	"github.com/LerianStudio/midaz/common/mrabbitmq"
	"github.com/LerianStudio/midaz/common/mzap"
	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/database/mongodb"
	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/database/postgres"
	rabbitmq "github.com/LerianStudio/midaz/components/ledger/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/components/ledger/internal/app/command"
	"github.com/LerianStudio/midaz/components/ledger/internal/app/query"
	"github.com/LerianStudio/midaz/components/ledger/internal/domain/metadata"
	"github.com/LerianStudio/midaz/components/ledger/internal/domain/onboarding/ledger"
	"github.com/LerianStudio/midaz/components/ledger/internal/domain/onboarding/organization"
	"github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/account"
	"github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/asset"
	"github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/portfolio"
	"github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/product"
	r "github.com/LerianStudio/midaz/components/ledger/internal/domain/rabbitmq"
	portsGRPC "github.com/LerianStudio/midaz/components/ledger/internal/ports/grpc"
	portsHTTP "github.com/LerianStudio/midaz/components/ledger/internal/ports/http"
	"github.com/LerianStudio/midaz/components/ledger/internal/service"
	"github.com/google/wire"
)

var onceConfig sync.Once

const prdEnvName = "production"

func setupPostgreSQLConnection(cfg *service.Config, log mlog.Logger) *mpostgres.PostgresConnection {
	connStrPrimary := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
		cfg.PrimaryDBHost, cfg.PrimaryDBUser, cfg.PrimaryDBPassword, cfg.PrimaryDBName, cfg.PrimaryDBPort)

	connStrReplica := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
		cfg.ReplicaDBHost, cfg.ReplicaDBUser, cfg.ReplicaDBPassword, cfg.ReplicaDBName, cfg.ReplicaDBPort)

	return &mpostgres.PostgresConnection{
		ConnectionStringPrimary: connStrPrimary,
		ConnectionStringReplica: connStrReplica,
		PrimaryDBName:           cfg.PrimaryDBName,
		ReplicaDBName:           cfg.ReplicaDBName,
		Component:               "ledger",
		Logger:                  log,
	}
}

func setupMongoDBConnection(cfg *service.Config, log mlog.Logger) *mmongo.MongoConnection {
	connStrSource := fmt.Sprintf("mongodb://%s:%s@%s:%s",
		cfg.MongoDBUser, cfg.MongoDBPassword, cfg.MongoDBHost, cfg.MongoDBPort)

	return &mmongo.MongoConnection{
		ConnectionStringSource: connStrSource,
		Database:               cfg.MongoDBName,
		Logger:                 log,
	}
}

func setupCasdoorConnection(cfg *service.Config, log mlog.Logger) *mcasdoor.CasdoorConnection {
	casdoor := &mcasdoor.CasdoorConnection{
		JWKUri:           cfg.JWKAddress,
		Endpoint:         cfg.CasdoorAddress,
		ClientID:         cfg.CasdoorClientID,
		ClientSecret:     cfg.CasdoorClientSecret,
		OrganizationName: cfg.CasdoorOrganizationName,
		ApplicationName:  cfg.CasdoorApplicationName,
		EnforcerName:     cfg.CasdoorEnforcerName,
		Logger:           log,
	}

	return casdoor
}

func setupRabbitMQConnection(cfg *service.Config, log mlog.Logger) *mrabbitmq.RabbitMQConnection {
	connStrSource := fmt.Sprintf("amqp://%s:%s@%s:%s",
		cfg.RabbitMQUser, cfg.RabbitMQPass, cfg.RabbitMQHost, cfg.RabbitMQPortHost)

	return &mrabbitmq.RabbitMQConnection{
		ConnectionStringSource: connStrSource,
		Host:                   cfg.RabbitMQHost,
		Port:                   cfg.RabbitMQPortAMQP,
		User:                   cfg.RabbitMQUser,
		Pass:                   cfg.RabbitMQPass,
		Exchange:               cfg.RabbitMQExchange,
		Key:                    cfg.RabbitMQKey,
		Queue:                  cfg.RabbitMQQueue,
		Logger:                 log,
	}
}

var (
	serviceSet = wire.NewSet(
		common.InitLocalEnvConfig,
		mzap.InitializeLogger,
		setupPostgreSQLConnection,
		setupMongoDBConnection,
		setupCasdoorConnection,
		setupRabbitMQConnection,
		portsGRPC.NewRouterGRPC,
		service.NewServerGRPC,
		portsHTTP.NewRouter,
		service.NewConfig,
		service.NewServer,
		postgres.NewOrganizationPostgreSQLRepository,
		postgres.NewLedgerPostgreSQLRepository,
		postgres.NewAssetPostgreSQLRepository,
		postgres.NewPortfolioPostgreSQLRepository,
		postgres.NewProductPostgreSQLRepository,
		postgres.NewAccountPostgreSQLRepository,
		mongodb.NewMetadataMongoDBRepository,
		rabbitmq.NewProducerRabbitMQ,
		rabbitmq.NewConsumerRabbitMQ,
		wire.Struct(new(portsHTTP.OrganizationHandler), "*"),
		wire.Struct(new(portsHTTP.LedgerHandler), "*"),
		wire.Struct(new(portsHTTP.AssetHandler), "*"),
		wire.Struct(new(portsHTTP.PortfolioHandler), "*"),
		wire.Struct(new(portsHTTP.ProductHandler), "*"),
		wire.Struct(new(portsHTTP.AccountHandler), "*"),
		wire.Struct(new(portsHTTP.RabbitMQHandler), "*"),
		wire.Struct(new(command.UseCase), "*"),
		wire.Struct(new(query.UseCase), "*"),
		wire.Bind(new(organization.Repository), new(*postgres.OrganizationPostgreSQLRepository)),
		wire.Bind(new(ledger.Repository), new(*postgres.LedgerPostgreSQLRepository)),
		wire.Bind(new(asset.Repository), new(*postgres.AssetPostgreSQLRepository)),
		wire.Bind(new(portfolio.Repository), new(*postgres.PortfolioPostgreSQLRepository)),
		wire.Bind(new(product.Repository), new(*postgres.ProductPostgreSQLRepository)),
		wire.Bind(new(account.Repository), new(*postgres.AccountPostgreSQLRepository)),
		wire.Bind(new(metadata.Repository), new(*mongodb.MetadataMongoDBRepository)),
		wire.Bind(new(r.ConsumerRepository), new(*rabbitmq.ConsumerRabbitMQRepository)),
		wire.Bind(new(r.ProducerRepository), new(*rabbitmq.ProducerRabbitMQRepository)),
	)

	svcSet = wire.NewSet(
		wire.Struct(new(service.Service), "Server", "ServerGRPC", "Logger"),
	)
)

// InitializeService the setup the dependencies and returns a new *service.Service instance
func InitializeService() *service.Service {
	wire.Build(serviceSet, svcSet)

	return nil
}
