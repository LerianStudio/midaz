//go:build wireinject
// +build wireinject

package gen

import (
	"fmt"
	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/database/mongodb"
	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/database/postgres"
	"github.com/LerianStudio/midaz/components/ledger/internal/domain/onboarding/ledger"
	"github.com/LerianStudio/midaz/components/ledger/internal/domain/onboarding/organization"
	"github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/account"
	"github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/instrument"
	"github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/portfolio"
	"github.com/LerianStudio/midaz/components/ledger/internal/domain/portfolio/product"
	"github.com/LerianStudio/midaz/components/ledger/internal/ports"
	"sync"

	"github.com/LerianStudio/midaz/common/mmongo"

	"github.com/LerianStudio/midaz/common"
	"github.com/LerianStudio/midaz/common/mpostgres"
	"github.com/LerianStudio/midaz/common/mzap"
	"github.com/LerianStudio/midaz/components/ledger/internal/app/command"
	"github.com/LerianStudio/midaz/components/ledger/internal/app/query"
	"github.com/LerianStudio/midaz/components/ledger/internal/domain/metadata"
	httpHandler "github.com/LerianStudio/midaz/components/ledger/internal/ports/http"
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

var (
	serviceSet = wire.NewSet(
		common.InitLocalEnvConfig,
		mzap.InitializeLogger,
		setupPostgreSQLConnection,
		setupMongoDBConnection,
		service.NewConfig,
		httpHandler.NewRouter,
		service.NewServer,
		postgres.NewOrganizationPostgreSQLRepository,
		postgres.NewLedgerPostgreSQLRepository,
		postgres.NewInstrumentPostgreSQLRepository,
		postgres.NewPortfolioPostgreSQLRepository,
		postgres.NewProductPostgreSQLRepository,
		postgres.NewAccountPostgreSQLRepository,
		mongodb.NewMetadataMongoDBRepository,
		wire.Struct(new(ports.OrganizationHandler), "*"),
		wire.Struct(new(ports.LedgerHandler), "*"),
		wire.Struct(new(ports.InstrumentHandler), "*"),
		wire.Struct(new(ports.PortfolioHandler), "*"),
		wire.Struct(new(ports.ProductHandler), "*"),
		wire.Struct(new(ports.AccountHandler), "*"),
		wire.Struct(new(command.UseCase), "*"),
		wire.Struct(new(query.UseCase), "*"),
		wire.Bind(new(organization.Repository), new(*postgres.OrganizationPostgreSQLRepository)),
		wire.Bind(new(ledger.Repository), new(*postgres.LedgerPostgreSQLRepository)),
		wire.Bind(new(instrument.Repository), new(*postgres.InstrumentPostgreSQLRepository)),
		wire.Bind(new(portfolio.Repository), new(*postgres.PortfolioPostgreSQLRepository)),
		wire.Bind(new(product.Repository), new(*postgres.ProductPostgreSQLRepository)),
		wire.Bind(new(account.Repository), new(*postgres.AccountPostgreSQLRepository)),
		wire.Bind(new(metadata.Repository), new(*mongodb.MetadataMongoDBRepository)),
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
