//go:build wireinject
// +build wireinject

package gen

import (
	"fmt"
	"sync"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mcasdoor"
	"github.com/LerianStudio/midaz/common/mgrpc"
	"github.com/LerianStudio/midaz/common/mlog"
	"github.com/LerianStudio/midaz/common/mmongo"
	"github.com/LerianStudio/midaz/common/mpostgres"
	"github.com/LerianStudio/midaz/common/mrabbitmq"
	"github.com/LerianStudio/midaz/common/mzap"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/database/mongodb"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/database/postgres"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/grpc"
	adapter "github.com/LerianStudio/midaz/components/transaction/internal/adapters/grpc"
	"github.com/LerianStudio/midaz/components/transaction/internal/app/command"
	"github.com/LerianStudio/midaz/components/transaction/internal/app/query"
	a "github.com/LerianStudio/midaz/components/transaction/internal/domain/account"
	ar "github.com/LerianStudio/midaz/components/transaction/internal/domain/assetrate"
	m "github.com/LerianStudio/midaz/components/transaction/internal/domain/metadata"
	o "github.com/LerianStudio/midaz/components/transaction/internal/domain/operation"
	t "github.com/LerianStudio/midaz/components/transaction/internal/domain/transaction"
	httpHandler "github.com/LerianStudio/midaz/components/transaction/internal/ports/http"
	"github.com/LerianStudio/midaz/components/transaction/internal/service"
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
		Component:               "transaction",
		Logger:                  log,
	}
}

func setupMongoDBConnection(cfg *service.Config, log mlog.Logger) *mmongo.MongoConnection {
	connStrSource := fmt.Sprintf("mongodb://%s:%s@%s:%s/",
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

func setupGRPCConnection(cfg *service.Config, log mlog.Logger) *mgrpc.GRPCConnection {
	addr := fmt.Sprintf("%s:%s", cfg.LedgerGRPCAddr, cfg.LedgerGRPCPort)

	return &mgrpc.GRPCConnection{
		Addr:   addr,
		Logger: log,
	}
}

func setupRabbitMQConnection(cfg *service.Config, log mlog.Logger) *mrabbitmq.RabbitMQConnection {
	connStrSource := fmt.Sprintf("amqp://%s:%s@%s:%s",
		cfg.RabbitMQUser, cfg.RabbitMQPass, cfg.RabbitMQHost, cfg.RabbitMQPortHost)

	return &mrabbitmq.RabbitMQConnection{
		ConnectionStringSource: connStrSource,
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
		setupGRPCConnection,
		setupRabbitMQConnection,
		service.NewConfig,
		httpHandler.NewRouter,
		service.NewServer,
		postgres.NewTransactionPostgreSQLRepository,
		postgres.NewOperationPostgreSQLRepository,
		postgres.NewAssetRatePostgreSQLRepository,
		mongodb.NewMetadataMongoDBRepository,
		grpc.NewAccountGRPC,
		wire.Struct(new(httpHandler.TransactionHandler), "*"),
		wire.Struct(new(httpHandler.OperationHandler), "*"),
		wire.Struct(new(httpHandler.AssetRateHandler), "*"),
		wire.Struct(new(command.UseCase), "*"),
		wire.Struct(new(query.UseCase), "*"),
		wire.Bind(new(t.Repository), new(*postgres.TransactionPostgreSQLRepository)),
		wire.Bind(new(o.Repository), new(*postgres.OperationPostgreSQLRepository)),
		wire.Bind(new(ar.Repository), new(*postgres.AssetRatePostgreSQLRepository)),
		wire.Bind(new(a.Repository), new(*adapter.AccountGRPCRepository)),
		wire.Bind(new(m.Repository), new(*mongodb.MetadataMongoDBRepository)),
	)

	svcSet = wire.NewSet(
		wire.Struct(new(service.Service), "Server", "Logger"),
	)
)

// InitializeService the setup the dependencies and returns a new *service.Service instance
func InitializeService() *service.Service {
	wire.Build(serviceSet, svcSet)

	return nil
}
