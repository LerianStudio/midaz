// Package query implements the Query side of CQRS pattern for the transaction service.
//
// This package contains all read operations (get, list, count) for the transaction service.
// It implements queries for:
//   - Transactions (list, get by ID, get by DSL)
//   - Balances (list, get by ID, get by account)
//   - Operations (list, get by ID, get by transaction)
//   - Asset rates (list, get by ID, get by codes)
//   - Transaction routes (list, get by ID)
//   - Operation routes (list, get by ID)
//   - Metadata enrichment (automatic for all entities)
//
// The query side focuses on data retrieval with:
//   - Pagination support (offset and cursor-based)
//   - Metadata enrichment from MongoDB
//   - Cache utilization for performance
//   - Read-optimized queries
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

// UseCase is a struct that aggregates repositories for query operations.
//
// This struct implements the Query side of CQRS, providing read operations
// for transactions, balances, operations, and routes. It follows the use case
// pattern, where each public method represents a query use case.
//
// The UseCase coordinates multiple repositories to:
//   - Fetch data from PostgreSQL
//   - Enrich with metadata from MongoDB
//   - Utilize Redis cache for performance
type UseCase struct {
	// TransactionRepo provides an abstraction on top of the transaction data source.
	TransactionRepo transaction.Repository

	// OperationRepo provides an abstraction on top of the operation data source.
	OperationRepo operation.Repository

	// AssetRateRepo provides an abstraction on top of the operation data source.
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
