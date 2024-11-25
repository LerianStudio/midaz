package query

import (
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/database/mongodb"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/database/postgres/assetrate"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/database/postgres/operation"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/database/postgres/transaction"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/database/redis"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/grpc"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/rabbitmq"
)

// UseCase is a struct that aggregates various repositories for simplified access in use case implementations.
type UseCase struct {
	// TransactionRepo provides an abstraction on top of the transaction data source.
	TransactionRepo transaction.Repository

	// AccountGRPCRepo provides an abstraction on top of the account grpc.
	AccountGRPCRepo grpc.Repository

	// OperationRepo provides an abstraction on top of the operation data source.
	OperationRepo operation.Repository

	// AssetRateRepo provides an abstraction on top of the operation data source.
	AssetRateRepo assetrate.Repository

	// MetadataRepo provides an abstraction on top of the metadata data source.
	MetadataRepo mongodb.Repository

	// RabbitMQRepo provides an abstraction on top of the producer rabbitmq.
	RabbitMQRepo rabbitmq.ConsumerRepository

	// RedisRepo provides an abstraction on top of the redis consumer.
	RedisRepo redis.RedisRepository
}
