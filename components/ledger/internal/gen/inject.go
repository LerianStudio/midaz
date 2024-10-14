//go:build wireinject
// +build wireinject

package gen

import (
	"fmt"
	"sync"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mcasdoor"
	"github.com/LerianStudio/midaz/common/mmongo"
	"github.com/LerianStudio/midaz/common/mpostgres"
	"github.com/LerianStudio/midaz/common/mzap"
	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/database/mongodb"
	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/database/postgres"
	"github.com/LerianStudio/midaz/components/ledger/internal/app/command"
	"github.com/LerianStudio/midaz/components/ledger/internal/app/query"
	"github.com/LerianStudio/midaz/components/ledger/internal/domain/metadata"
	"github.com/LerianStudio/midaz/components/ledger/internal/domain/onboarding/ledger"
	"github.com/LerianStudio/midaz/components/ledger/internal/domain/onboarding/organization"
	"github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/account"
	"github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/asset"
	"github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/portfolio"
	"github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/product"
	portsGRPC "github.com/LerianStudio/midaz/components/ledger/internal/ports/grpc"
	portsHTTP "github.com/LerianStudio/midaz/components/ledger/internal/ports/http"
	"github.com/LerianStudio/midaz/components/ledger/internal/service"
	"github.com/google/wire"
)

var onceConfig sync.Once

const prdEnvName = "production"

func setupPostgreSQLConnection(cfg *service.Config) *mpostgres.PostgresConnection {
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
	}
}

func setupMongoDBConnection(cfg *service.Config) *mmongo.MongoConnection {
	connStrSource := fmt.Sprintf("mongodb://%s:%s@%s:%s",
		cfg.MongoDBUser, cfg.MongoDBPassword, cfg.MongoDBHost, cfg.MongoDBPort)

	return &mmongo.MongoConnection{
		ConnectionStringSource: connStrSource,
		Database:               cfg.MongoDBName,
	}
}

func setupCasdoorConnection(cfg *service.Config) *mcasdoor.CasdoorConnection {
	casdoor := &mcasdoor.CasdoorConnection{
		JWKUri:           cfg.JWKAddress,
		Endpoint:         cfg.CasdoorAddress,
		ClientID:         cfg.CasdoorClientID,
		ClientSecret:     cfg.CasdoorClientSecret,
		OrganizationName: cfg.CasdoorOrganizationName,
		ApplicationName:  cfg.CasdoorApplicationName,
		EnforcerName:     cfg.CasdoorEnforcerName,
	}

	return casdoor
}

var (
	serviceSet = wire.NewSet(
		common.InitLocalEnvConfig,
		mzap.InitializeLogger,
		setupPostgreSQLConnection,
		setupMongoDBConnection,
		setupCasdoorConnection,
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
		wire.Struct(new(portsHTTP.OrganizationHandler), "*"),
		wire.Struct(new(portsHTTP.LedgerHandler), "*"),
		wire.Struct(new(portsHTTP.AssetHandler), "*"),
		wire.Struct(new(portsHTTP.PortfolioHandler), "*"),
		wire.Struct(new(portsHTTP.ProductHandler), "*"),
		wire.Struct(new(portsHTTP.AccountHandler), "*"),
		wire.Struct(new(command.UseCase), "*"),
		wire.Struct(new(query.UseCase), "*"),
		wire.Bind(new(organization.Repository), new(*postgres.OrganizationPostgreSQLRepository)),
		wire.Bind(new(ledger.Repository), new(*postgres.LedgerPostgreSQLRepository)),
		wire.Bind(new(asset.Repository), new(*postgres.AssetPostgreSQLRepository)),
		wire.Bind(new(portfolio.Repository), new(*postgres.PortfolioPostgreSQLRepository)),
		wire.Bind(new(product.Repository), new(*postgres.ProductPostgreSQLRepository)),
		wire.Bind(new(account.Repository), new(*postgres.AccountPostgreSQLRepository)),
		wire.Bind(new(metadata.Repository), new(*mongodb.MetadataMongoDBRepository)),
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
