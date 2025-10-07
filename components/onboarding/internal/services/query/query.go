// Package query implements the Query side of CQRS pattern for the onboarding service.
//
// This package contains all read operations (queries) for the onboarding domain:
//   - Get operations: Retrieve single entities by ID
//   - List operations: Retrieve paginated collections of entities
//   - Find operations: Search entities by specific criteria
//   - Count operations: Get total counts for pagination
//
// Architecture Pattern: CQRS (Command Query Responsibility Segregation)
//
// The query side is responsible for:
//   - Reading data from PostgreSQL (primary data) and MongoDB (metadata)
//   - Applying filters, sorting, and pagination
//   - Enriching entities with metadata
//   - Providing optimized read paths
//   - Supporting both offset and cursor-based pagination
//
// Key Responsibilities:
//   - Entity retrieval by ID
//   - Paginated list queries with filtering
//   - Metadata enrichment (joining PostgreSQL with MongoDB)
//   - Query parameter validation
//   - Result transformation
//
// Read Optimization:
//   - Queries are optimized for read performance
//   - Metadata is fetched separately and merged
//   - Pagination limits prevent large result sets
//   - Indexes support common query patterns
//
// The UseCase struct aggregates all repositories needed for query operations,
// following the dependency injection pattern. Each query method is a use case
// that orchestrates repository calls to fulfill read requests.
//
// Entities Queried:
//   - Organization: Top-level entities with hierarchical support
//   - Ledger: Financial record-keeping systems
//   - Asset: Currencies, cryptocurrencies, commodities
//   - Account: Financial buckets for tracking balances
//   - Portfolio: Collections of accounts
//   - Segment: Logical divisions of accounts
//   - AccountType: Account classification system
//
// Caching:
//   - Redis is available for caching frequently accessed data
//   - Cache-aside pattern can be implemented per use case
//   - TTL-based cache invalidation
//
// Error Handling:
//   - Not found errors are converted to ErrEntityNotFound
//   - Database errors are wrapped and logged
//   - All errors are traced via OpenTelemetry spans
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

// UseCase aggregates all repositories needed for query operations.
//
// This struct follows the dependency injection pattern for the Query side of CQRS.
// It provides read-only access to data through repository interfaces.
//
// The UseCase struct is instantiated in the bootstrap layer and passed to HTTP handlers.
// Each method on UseCase represents a single query (read operation) use case.
//
// Repository Types:
//   - PostgreSQL repositories: Primary data storage (read-optimized queries)
//   - MongoDB repository: Metadata storage (flexible key-value data)
//   - Redis repository: Caching layer (optional, for performance)
//
// Thread Safety:
//   - UseCase instances are shared across goroutines (HTTP handlers)
//   - Repositories must be thread-safe
//   - No mutable state in UseCase struct
//   - Read operations don't modify data
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

	// AccountTypeRepo provides an abstraction on top of the account type data source.
	AccountTypeRepo accounttype.Repository

	// MetadataRepo provides an abstraction on top of the metadata data source.
	MetadataRepo mongodb.Repository

	// RedisRepo provides an abstraction on top of the redis consumer.
	RedisRepo redis.RedisRepository
}
