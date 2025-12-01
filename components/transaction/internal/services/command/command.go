// Package command provides CQRS command handlers for the transaction component.
//
// This package implements the Command side of CQRS (Command Query Responsibility Segregation)
// for transaction and balance management operations. Commands are write operations that modify state:
//   - Balance operations (create, update, delete)
//   - Asset rate management (currency exchange rates)
//   - Transaction route caching (performance optimization)
//   - Metadata operations (entity metadata in MongoDB)
//
// # CQRS Pattern
//
// Commands handle all write operations that modify the ledger state:
//
//	┌─────────────────────────────────────────────────────────────┐
//	│                      HTTP/gRPC Handler                       │
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
// # Financial Operations
//
// The transaction command service handles critical financial operations:
//   - Balance updates with optimistic locking (version field prevents lost updates)
//   - Double-entry accounting validation (from/to amounts must balance)
//   - Asset rate conversions for multi-currency transactions
//   - Transaction route caching for high-performance routing decisions
//
// # Repository Dependencies
//
// UseCase aggregates all repositories needed for transaction management.
// This follows the facade pattern, providing a unified interface to multiple
// data sources (PostgreSQL for transactions, MongoDB for metadata, Redis for caching).
//
// # Related Packages
//
//   - apps/midaz/components/transaction/internal/services/query: Query handlers
//   - apps/midaz/components/transaction/internal/adapters: Repository implementations
//   - apps/midaz/components/onboarding: Entity management (accounts, ledgers)
package command

import (
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/operationroute"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/postgres/transactionroute"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/rabbitmq"
	"github.com/LerianStudio/midaz/v3/components/transaction/internal/adapters/redis"
)

// UseCase aggregates repositories for CQRS command operations in transactions.
//
// This struct follows the facade pattern, providing a unified entry point for
// all command operations (create, update, delete) related to transactions,
// balances, and asset rates. Each repository abstracts the underlying data store.
//
// # Repository Architecture
//
//   - PostgreSQL: Primary data store for transactions, balances, operations
//   - MongoDB: Flexible schema for entity metadata (key-value pairs)
//   - Redis: Caching layer for transaction routes and temporary data
//   - RabbitMQ: Event publishing for async processing
//
// # Thread Safety
//
// UseCase is safe for concurrent use. Each repository handles its own
// connection pooling and synchronization. Balance updates use optimistic
// locking via version fields to prevent concurrent modification issues.
//
// # Usage Example
//
//	uc := &command.UseCase{
//	    TransactionRepo: txRepo,
//	    BalanceRepo:     balanceRepo,
//	    AssetRateRepo:   assetRateRepo,
//	    // ... other repos
//	}
//	err := uc.UpdateBalances(ctx, orgID, ledgerID, validate, balances)
type UseCase struct {
	// TransactionRepo provides CRUD operations for transactions.
	//
	// Transactions represent the atomic unit of work in double-entry accounting.
	// Each transaction contains one or more operations that must balance.
	TransactionRepo transaction.Repository

	// OperationRepo provides CRUD operations for transaction operations.
	//
	// Operations are the individual debit/credit entries within a transaction.
	// Operations must always balance (sum of debits == sum of credits).
	OperationRepo operation.Repository

	// AssetRateRepo provides CRUD operations for asset exchange rates.
	//
	// Asset rates define currency conversion rates between different assets.
	// Used for multi-currency transactions and reporting.
	AssetRateRepo assetrate.Repository

	// BalanceRepo provides CRUD operations for account balances.
	//
	// Balances track the current state of each account (available, on hold).
	// Updates use optimistic locking via version field.
	BalanceRepo balance.Repository

	// OperationRouteRepo provides CRUD operations for operation routes.
	//
	// Operation routes define how specific operation types should be processed.
	// Linked to transaction routes for hierarchical routing.
	OperationRouteRepo operationroute.Repository

	// TransactionRouteRepo provides CRUD operations for transaction routes.
	//
	// Transaction routes define routing rules for transaction processing.
	// Cached in Redis for high-performance lookups.
	TransactionRouteRepo transactionroute.Repository

	// MetadataRepo provides CRUD operations for entity metadata in MongoDB.
	//
	// Metadata stores arbitrary key-value pairs associated with any entity.
	// Enables flexible extension of entity data without schema changes.
	MetadataRepo mongodb.Repository

	// RabbitMQRepo provides message publishing to RabbitMQ.
	//
	// Used for async event publishing (transaction completed, balance updated).
	// Enables event-driven architecture and eventual consistency.
	RabbitMQRepo rabbitmq.ProducerRepository

	// RedisRepo provides Redis-based caching and temporary storage.
	//
	// Used for transaction route caching and temporary data during processing.
	// Significantly improves transaction routing performance.
	RedisRepo redis.RedisRepository
}
