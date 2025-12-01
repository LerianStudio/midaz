// Package query provides CQRS query handlers for the onboarding bounded context.
//
// This package implements read-only operations for retrieving onboarding-related
// entities from the ledger system. It follows the CQRS (Command Query Responsibility
// Segregation) pattern, separating read operations from write operations.
//
// Query Handlers:
//
//   - Organizations: Retrieve organization configurations
//   - Ledgers: Fetch ledger configurations within organizations
//   - Segments: Query account segmentation structures
//   - Portfolios: Retrieve portfolio groupings
//   - Accounts: Fetch account details and configurations
//   - Account Types: Query account type definitions
//   - Assets: Retrieve asset/currency configurations
//
// Architecture:
//
// Query handlers use a multi-store approach:
//   - PostgreSQL: Primary data source for onboarding entities
//   - MongoDB: Metadata storage for flexible key-value attributes
//   - Redis/Valkey: Cache layer for frequently accessed configurations
//
// Data Enrichment Pattern:
//
// Most queries follow a two-phase retrieval:
//  1. Fetch core entity data from PostgreSQL
//  2. Enrich with metadata from MongoDB (joined by entity ID)
//
// This separation allows flexible metadata schemas without PostgreSQL migrations.
//
// Thread Safety:
//
// UseCase is safe for concurrent use. All methods are read-only and use
// context-scoped database connections from the repository layer.
//
// Related Packages:
//   - adapters/postgres: PostgreSQL repository implementations
//   - adapters/mongodb: MongoDB metadata repository
//   - adapters/redis: Redis cache repository
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

// UseCase aggregates repositories for onboarding query operations.
//
// This struct provides a unified access point for all read operations in the
// onboarding bounded context. It holds references to various repositories,
// enabling query handlers to access multiple data sources efficiently.
//
// Repository Responsibilities:
//
//   - OrganizationRepo: Query organization entities (root tenant structure)
//   - LedgerRepo: Query ledger configurations within organizations
//   - SegmentRepo: Query account segmentation (grouping accounts by purpose)
//   - PortfolioRepo: Query portfolio groupings (collections of accounts)
//   - AccountRepo: Query individual account records
//   - AssetRepo: Query asset/currency definitions
//   - AccountTypeRepo: Query account type classifications
//   - MetadataRepo: Query flexible metadata for any entity (MongoDB)
//   - RedisRepo: Access cached data and perform cache operations
//
// Lifecycle:
//
// UseCase is typically instantiated once during application startup via
// dependency injection and shared across request handlers.
//
//	uc := &query.UseCase{
//	    OrganizationRepo: orgRepo,
//	    LedgerRepo:       ledgerRepo,
//	    // ... other repos
//	}
//
// Thread Safety:
//
// UseCase is immutable after initialization and safe to share across goroutines.
// All repository implementations handle their own connection pooling and
// concurrency management.
type UseCase struct {
	// OrganizationRepo provides an abstraction on top of the organization data source.
	// Organizations are the root tenant entities in the multi-tenant hierarchy.
	OrganizationRepo organization.Repository

	// LedgerRepo provides an abstraction on top of the ledger data source.
	// Ledgers contain accounts, transactions, and define the chart of accounts.
	LedgerRepo ledger.Repository

	// SegmentRepo provides an abstraction on top of the segment data source.
	// Segments organize accounts into logical groups (e.g., by business unit).
	SegmentRepo segment.Repository

	// PortfolioRepo provides an abstraction on top of the portfolio data source.
	// Portfolios group related accounts for reporting and management.
	PortfolioRepo portfolio.Repository

	// AccountRepo provides an abstraction on top of the account data source.
	// Accounts hold balances and participate in transactions.
	AccountRepo account.Repository

	// AssetRepo provides an abstraction on top of the asset data source.
	// Assets define currencies and other tradable instruments.
	AssetRepo asset.Repository

	// AccountTypeRepo provides an abstraction on top of the account type data source.
	// Account types classify accounts (e.g., ASSET, LIABILITY, EQUITY).
	AccountTypeRepo accounttype.Repository

	// MetadataRepo provides an abstraction on top of the metadata data source.
	// Stores flexible key-value attributes for any entity in MongoDB.
	MetadataRepo mongodb.Repository

	// RedisRepo provides an abstraction on top of the redis consumer.
	// Used for caching frequently accessed data and distributed operations.
	RedisRepo redis.RedisRepository
}
