// Package command provides CQRS command handlers for the onboarding component.
//
// This package implements the Command side of CQRS (Command Query Responsibility Segregation)
// for entity management operations. Commands are write operations that modify state:
//   - Create operations (organization, ledger, account, etc.)
//   - Update operations (status changes, metadata updates)
//   - Delete operations (soft deletes)
//
// CQRS Pattern:
//
//	┌─────────────────────────────────────────────────────────────┐
//	│                      HTTP Handler                            │
//	│                          │                                   │
//	│           ┌──────────────┴──────────────┐                   │
//	│           ▼                             ▼                    │
//	│    ┌─────────────┐               ┌─────────────┐            │
//	│    │   Command   │               │    Query    │            │
//	│    │   UseCase   │               │   UseCase   │            │
//	│    └──────┬──────┘               └──────┬──────┘            │
//	│           │                             │                    │
//	│           ▼                             ▼                    │
//	│    ┌─────────────┐               ┌─────────────┐            │
//	│    │ Write Model │               │ Read Model  │            │
//	│    │ (Postgres)  │               │ (Postgres)  │            │
//	│    └─────────────┘               └─────────────┘            │
//	└─────────────────────────────────────────────────────────────┘
//
// Why Separate Command/Query:
//
//   - Commands may trigger side effects (events, notifications)
//   - Queries can use optimized read paths (denormalized views)
//   - Different validation rules for reads vs writes
//   - Commands require stricter consistency guarantees
//
// Repository Dependencies:
//
// UseCase aggregates all repositories needed for entity management.
// This follows the facade pattern, providing a unified interface to multiple
// data sources (PostgreSQL for entities, MongoDB for metadata, Redis for caching).
//
// Related Packages:
//   - apps/midaz/components/onboarding/internal/services/query: Query handlers
//   - apps/midaz/components/onboarding/internal/adapters: Repository implementations
package command

import (
	"github.com/LerianStudio/midaz/v3/components/onboarding/internal/adapters/grpc/out"
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

// UseCase aggregates repositories for CQRS command operations in onboarding.
//
// This struct follows the facade pattern, providing a unified entry point for
// all command operations (create, update, delete) across multiple entity types.
// Each repository abstracts the underlying data store, enabling:
//   - Unit testing with mock repositories
//   - Swapping implementations without changing business logic
//   - Clear dependency boundaries
//
// Repository Types:
//
//   - PostgreSQL: Primary data store for entities (organization, ledger, account)
//   - MongoDB: Flexible schema for metadata (key-value pairs per entity)
//   - Redis: Caching layer and async queue for balance operations
//   - gRPC: Inter-service communication (balance service)
//
// Thread Safety:
// UseCase is safe for concurrent use. Each repository handles its own
// connection pooling and synchronization.
//
// Usage:
//
//	uc := &command.UseCase{
//	    OrganizationRepo: orgRepo,
//	    LedgerRepo:       ledgerRepo,
//	    // ... other repos
//	}
//	account, err := uc.CreateAccount(ctx, orgID, ledgerID, input, token)
type UseCase struct {
	// OrganizationRepo provides CRUD operations for organizations.
	//
	// Organizations are the top-level tenant entities that own ledgers.
	OrganizationRepo organization.Repository

	// LedgerRepo provides CRUD operations for ledgers.
	//
	// Ledgers are the primary accounting containers within an organization.
	LedgerRepo ledger.Repository

	// SegmentRepo provides CRUD operations for segments.
	//
	// Segments enable grouping accounts for reporting purposes.
	SegmentRepo segment.Repository

	// PortfolioRepo provides CRUD operations for portfolios.
	//
	// Portfolios group accounts belonging to a single entity (customer).
	PortfolioRepo portfolio.Repository

	// AccountRepo provides CRUD operations for accounts.
	//
	// Accounts are the fundamental units that hold balances.
	AccountRepo account.Repository

	// AssetRepo provides CRUD operations for assets.
	//
	// Assets define the currencies/instruments that can be held in accounts.
	AssetRepo asset.Repository

	// AccountTypeRepo provides CRUD operations for account types.
	//
	// Account types define the chart of accounts structure.
	AccountTypeRepo accounttype.Repository

	// MetadataRepo provides CRUD operations for entity metadata in MongoDB.
	//
	// Metadata stores arbitrary key-value pairs associated with entities.
	MetadataRepo mongodb.Repository

	// RedisRepo provides Redis-based caching and queue operations.
	//
	// Used for async balance creation and caching frequently accessed data.
	RedisRepo redis.RedisRepository

	// BalanceGRPCRepo provides gRPC client for balance service communication.
	//
	// Enables synchronous balance creation via the transaction component.
	BalanceGRPCRepo out.Repository
}
