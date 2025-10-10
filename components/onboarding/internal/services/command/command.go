// Package command implements the Command side of the CQRS pattern for the onboarding service.
//
// This package contains all write operations (commands) for the onboarding domain,
// including creating, updating, and deleting entities. It follows the Clean Architecture
// pattern, where the UseCase struct orchestrates business logic and repository interactions.
package command

import (
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/accounttype"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/organization"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/portfolio"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/segment"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/redis"
)

// UseCase encapsulates all the repositories required for command operations.
//
// This struct follows the dependency injection pattern, where all repository
// dependencies are provided at construction time. This design promotes testability
// by allowing mock repositories to be used in tests and decouples the business logic
// from the underlying data storage technologies.
type UseCase struct {
	// OrganizationRepo provides an abstraction for accessing organization data.
	OrganizationRepo organization.Repository

	// LedgerRepo provides an abstraction for accessing ledger data.
	LedgerRepo ledger.Repository

	// SegmentRepo provides an abstraction for accessing segment data.
	SegmentRepo segment.Repository

	// PortfolioRepo provides an abstraction for accessing portfolio data.
	PortfolioRepo portfolio.Repository

	// AccountRepo provides an abstraction for accessing account data.
	AccountRepo account.Repository

	// AssetRepo provides an abstraction for accessing asset data.
	AssetRepo asset.Repository

	// AccountTypeRepo provides an abstraction for accessing account type data.
	AccountTypeRepo accounttype.Repository

	// MetadataRepo provides an abstraction for accessing metadata.
	MetadataRepo mongodb.Repository

	// RabbitMQRepo provides an abstraction for producing messages to RabbitMQ.
	RabbitMQRepo rabbitmq.ProducerRepository

	// RedisRepo provides an abstraction for interacting with Redis.
	RedisRepo redis.RedisRepository
}
