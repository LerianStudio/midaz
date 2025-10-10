// Package query implements the Query side of the CQRS pattern for the onboarding service.
//
// This package is responsible for all read operations (queries) within the onboarding
// domain. It follows the Clean Architecture pattern, with the UseCase struct
// orchestrating data retrieval from various repositories.
package query

import (
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/account"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/accounttype"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/asset"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/ledger"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/organization"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/portfolio"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/postgres/segment"
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/redis"
)

// UseCase encapsulates all the repositories required for query operations.
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

	// RedisRepo provides an abstraction for interacting with Redis.
	RedisRepo redis.RedisRepository
}
