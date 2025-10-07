// Package command implements the Command side of CQRS pattern for the onboarding service.
//
// This package contains all write operations (commands) for the onboarding domain:
//   - Create operations: Create new entities (organizations, ledgers, accounts, etc.)
//   - Update operations: Modify existing entities
//   - Delete operations: Soft-delete entities
//   - Queue operations: Send messages to RabbitMQ for event-driven processing
//
// Architecture Pattern: CQRS (Command Query Responsibility Segregation)
//
// The command side is responsible for:
//   - Validating business rules before writes
//   - Persisting data to PostgreSQL (primary data) and MongoDB (metadata)
//   - Publishing events to RabbitMQ for downstream processing
//   - Maintaining data consistency and integrity
//   - Handling optimistic concurrency where needed
//
// Key Responsibilities:
//   - Entity creation with validation
//   - Entity updates with immutability checks
//   - Soft deletion with cascade rules
//   - Metadata management (MongoDB)
//   - Event publishing (RabbitMQ)
//   - Cache invalidation (Redis)
//
// The UseCase struct aggregates all repositories needed for command operations,
// following the dependency injection pattern. Each command method is a use case
// that orchestrates multiple repository calls and business logic.
//
// Entities Managed:
//   - Organization: Top-level entities with hierarchical support
//   - Ledger: Financial record-keeping systems
//   - Asset: Currencies, cryptocurrencies, commodities
//   - Account: Financial buckets for tracking balances
//   - Portfolio: Collections of accounts
//   - Segment: Logical divisions of accounts
//   - AccountType: Account classification system
//   - Metadata: Flexible key-value storage for all entities
//
// Transaction Handling:
//   - Database transactions are handled at the repository layer
//   - Each command method represents a single business transaction
//   - Failures trigger rollback and error propagation
//   - OpenTelemetry spans track operation success/failure
//
// Error Handling:
//   - Business errors are converted using pkg.ValidateBusinessError
//   - Database errors are wrapped and logged
//   - All errors are traced via OpenTelemetry spans
//   - Errors include entity type context for better debugging
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

// UseCase aggregates all repositories needed for command operations.
//
// This struct follows the dependency injection pattern, where all dependencies
// (repositories) are injected at construction time. This design:
//   - Enables easy testing via mock repositories
//   - Decouples business logic from infrastructure
//   - Follows hexagonal architecture principles
//   - Supports the CQRS pattern (Command side)
//
// The UseCase struct is instantiated in the bootstrap layer and passed to HTTP handlers.
// Each method on UseCase represents a single command (write operation) use case.
//
// Repository Types:
//   - PostgreSQL repositories: Primary data storage (organizations, ledgers, accounts, etc.)
//   - MongoDB repository: Flexible metadata storage
//   - RabbitMQ repository: Event publishing for async processing
//   - Redis repository: Caching and distributed locking
//
// Thread Safety:
//   - UseCase instances are shared across goroutines (HTTP handlers)
//   - Repositories must be thread-safe
//   - No mutable state in UseCase struct
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

	// RabbitMQRepo provides an abstraction on top of the producer rabbitmq.
	RabbitMQRepo rabbitmq.ProducerRepository

	// RedisRepo provides an abstraction on top of the redis consumer.
	RedisRepo redis.RedisRepository
}
