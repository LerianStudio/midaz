package command

import (
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/accounttype"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/organization"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/portfolio"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/segment"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/postgres/settings"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/components/onboarding/internal/adapters/redis"
)

// UseCase is a struct that aggregates various repositories for simplified access in use case implementation.
type UseCase struct {
	// OrganizationRepo provides an abstraction on top of the organization data source.
	OrganizationRepo organization.Repository

	// LedgerRepo provides an abstraction on top of the ledger data source.
	LedgerRepo ledger.Repository

	// SegmentRepo provides an abstraction on top of the segment data source.
	SegmentRepo segment.Repository

	// PortfolioRepo provides an abstraction on top of the portfolio data source.
	PortfolioRepo portfolio.Repository

	// AccountRepo provides an abstraction on top of the account data source.
	AccountRepo account.Repository

	// AssetRepo provides an abstraction on top of the asset data source.
	AssetRepo asset.Repository

	// SettingsRepo provides an abstraction on top of the settings data source.
	SettingsRepo settings.Repository

	// AccountTypeRepo provides an abstraction on top of the account type data source.
	AccountTypeRepo accounttype.Repository

	// MetadataRepo provides an abstraction on top of the metadata data source.
	MetadataRepo mongodb.Repository

	// RabbitMQRepo provides an abstraction on top of the producer rabbitmq.
	RabbitMQRepo rabbitmq.ProducerRepository

	// RedisRepo provides an abstraction on top of the redis consumer.
	RedisRepo redis.RedisRepository
}
