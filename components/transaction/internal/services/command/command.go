// Package command implements the Command side of the CQRS pattern for the transaction service.
//
// This package contains all write operations (commands) for the transaction domain,
// including creating, updating, and deleting transactions, balances, and routing rules.
// It enforces business logic, ensures data consistency, and coordinates with various
// repositories and services.
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

// UseCase encapsulates all the dependencies required for command use cases.
//
// This struct follows the Clean Architecture pattern, centralizing all write
// operations for the transaction domain and orchestrating interactions between
// repositories and other services.
type UseCase struct {
	// TransactionRepo provides an abstraction for accessing transaction data.
	TransactionRepo transaction.Repository

	// OperationRepo provides an abstraction for accessing operation data.
	OperationRepo operation.Repository

	// AssetRateRepo provides an abstraction for accessing asset rate data.
	AssetRateRepo assetrate.Repository

	// BalanceRepo provides an abstraction for accessing balance data.
	BalanceRepo balance.Repository

	// OperationRouteRepo provides an abstraction for accessing operation route data.
	OperationRouteRepo operationroute.Repository

	// TransactionRouteRepo provides an abstraction for accessing transaction route data.
	TransactionRouteRepo transactionroute.Repository

	// MetadataRepo provides an abstraction for accessing metadata in MongoDB.
	MetadataRepo mongodb.Repository

	// RabbitMQRepo provides an abstraction for publishing messages to RabbitMQ.
	RabbitMQRepo rabbitmq.ProducerRepository

	// RedisRepo provides an abstraction for interacting with Redis.
	RedisRepo redis.RedisRepository
}
