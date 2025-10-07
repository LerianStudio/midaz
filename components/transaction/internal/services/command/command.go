// Package command implements the Command side of CQRS pattern for the transaction service.
//
// This package contains all write operations (create, update, delete) for the transaction service.
// It implements business logic for:
//   - Transaction processing and validation
//   - Balance management (create, update, delete)
//   - Operation tracking (debits, credits, holds, releases)
//   - Asset rate management (currency conversion)
//   - Transaction routes (routing rules for automated transactions)
//   - Operation routes (account selection rules)
//   - Event publishing (audit logs, transaction events)
//   - Cache management (transaction route caching)
//   - Idempotency (duplicate request prevention)
//
// The command side enforces business rules, validates transactions, and coordinates
// with multiple repositories to ensure data consistency and double-entry accounting principles.
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

// UseCase is a struct that aggregates repositories for command operations.
//
// This struct implements the Command side of CQRS, providing write operations
// for transactions, balances, operations, and routes. It follows the use case
// pattern, where each public method represents a business use case.
//
// The UseCase coordinates multiple repositories to ensure:
//   - Transactional consistency across PostgreSQL operations
//   - Metadata synchronization with MongoDB
//   - Event publishing to RabbitMQ
//   - Cache invalidation in Redis
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
