// Package query provides CQRS query handlers for transaction domain read operations.
//
// This package implements the query side of the CQRS pattern for the transaction
// bounded context, handling all read operations for ledger transaction entities:
//   - Transactions and their operations
//   - Account balances and balance history
//   - Asset rates and conversions
//   - Operation routes and transaction routes
//   - Entity metadata retrieval
//
// Query handlers in this package:
//   - Retrieve data from PostgreSQL (primary data) and MongoDB (metadata)
//   - Apply caching strategies via Redis/Valkey for frequently accessed data
//   - Support cursor-based pagination for large result sets
//   - Enrich entities with metadata from document store
//
// Architecture:
//
// Queries follow the UseCase pattern where a single UseCase struct aggregates
// all repository dependencies. Each query method:
//  1. Extracts tracing context (logger, tracer, requestID)
//  2. Creates OpenTelemetry span for observability
//  3. Retrieves data from PostgreSQL (authoritative source)
//  4. Enriches with metadata from MongoDB
//  5. Applies cache overlays for real-time balance data
//  6. Returns entity or paginated list
//
// Caching Strategy:
//
// The query package uses Redis for caching real-time balance data. When querying
// balances, the system:
//   - Retrieves base balance data from PostgreSQL
//   - Overlays with cached values from Redis (if available)
//   - Redis contains the most recent balance state during active transactions
//
// Related Packages:
//   - components/transaction/internal/services/command: Write operations (CQRS command side)
//   - components/transaction/internal/adapters: Repository implementations
//   - pkg/mmodel: Domain models and DTOs
package query

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

// UseCase aggregates repository dependencies for transaction query operations.
//
// This struct provides a centralized access point for all data sources needed
// by query handlers in the transaction bounded context. It follows the dependency
// injection pattern, allowing repositories to be mocked for testing.
//
// Lifecycle:
//
//	uc := &query.UseCase{
//	    TransactionRepo: transactionRepo,
//	    OperationRepo:   operationRepo,
//	    // ... other repositories
//	}
//	// UseCase is then passed to HTTP handlers
//
// Thread Safety:
//
// UseCase is safe for concurrent use as all repository implementations
// are expected to be thread-safe. The struct itself is immutable after
// initialization.
//
// Repository Responsibilities:
//
//   - TransactionRepo: Transaction CRUD and queries (PostgreSQL)
//   - OperationRepo: Operation CRUD and queries (PostgreSQL)
//   - AssetRateRepo: Asset exchange rates (PostgreSQL)
//   - BalanceRepo: Account balance queries (PostgreSQL)
//   - OperationRouteRepo: Operation routing rules (PostgreSQL)
//   - TransactionRouteRepo: Transaction routing rules (PostgreSQL)
//   - MetadataRepo: Entity metadata storage (MongoDB)
//   - RabbitMQRepo: Event publishing (RabbitMQ)
//   - RedisRepo: Real-time balance cache (Redis/Valkey)
type UseCase struct {
	// TransactionRepo provides an abstraction on top of the transaction data source.
	TransactionRepo transaction.Repository

	// OperationRepo provides an abstraction on top of the operation data source.
	OperationRepo operation.Repository

	// AssetRateRepo provides an abstraction on top of the asset rate data source.
	AssetRateRepo assetrate.Repository

	// BalanceRepo provides an abstraction on top of the balance data source.
	BalanceRepo balance.Repository

	// OperationRouteRepo provides an abstraction on top of the operation route data source.
	OperationRouteRepo operationroute.Repository

	// TransactionRouteRepo provides an abstraction on top of the transaction route data source.
	TransactionRouteRepo transactionroute.Repository

	// MetadataRepo provides an abstraction on top of the metadata data source.
	MetadataRepo mongodb.Repository

	// RabbitMQRepo provides an abstraction on top of the producer rabbitmq.
	RabbitMQRepo rabbitmq.ProducerRepository

	// RedisRepo provides an abstraction on top of the redis consumer.
	RedisRepo redis.RedisRepository
}
