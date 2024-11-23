package query

import (
	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/database/mongodb"
	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/database/postgres/asset"
	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/database/postgres/ledger"
	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/database/postgres/organization"
	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/database/postgres/product"
	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/database/redis"
	a "github.com/LerianStudio/midaz/components/ledger/internal/adapters/interface/portfolio/account"
	p "github.com/LerianStudio/midaz/components/ledger/internal/adapters/interface/portfolio/portfolio"
	"github.com/LerianStudio/midaz/components/ledger/internal/adapters/rabbitmq"
)

// UseCase is a struct that aggregates various repositories for simplified access in use case implementation.
type UseCase struct {
	// OrganizationRepo provides an abstraction on top of the organization data source.
	OrganizationRepo organization.Repository

	// LedgerRepo provides an abstraction on top of the ledger data source.
	LedgerRepo ledger.Repository

	// ProductRepo provides an abstraction on top of the product data source.
	ProductRepo product.Repository

	// PortfolioRepo provides an abstraction on top of the portfolio data source.
	PortfolioRepo p.Repository

	// AccountRepo provides an abstraction on top of the account data source.
	AccountRepo a.Repository

	// AssetRepo provides an abstraction on top of the asset data source.
	AssetRepo asset.Repository

	// MetadataRepo provides an abstraction on top of the metadata data source.
	MetadataRepo mongodb.Repository

	// RabbitMQRepo provides an abstraction on top of the consumer rabbitmq.
	RabbitMQRepo rabbitmq.ConsumerRepository

	// RedisRepo provides an abstraction on top of the redis consumer.
	RedisRepo redis.RedisRepository
}
