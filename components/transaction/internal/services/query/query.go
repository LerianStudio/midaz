package query

import (
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/assetrate"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/balance"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/operation"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/postgres/transaction"
	"github.com/LerianStudio/midaz/components/transaction/internal/adapters/redis"
)

// UseCase is a struct that aggregates various repositories for simplified access in use case implementations.
type UseCase struct {
	// TransactionRepo provides an abstraction on top of the transaction data source.
	TransactionRepo transaction.Repository

	// OperationRepo provides an abstraction on top of the operation data source.
	OperationRepo operation.Repository

	// AssetRateRepo provides an abstraction on top of the operation data source.
	AssetRateRepo assetrate.Repository

	// BalanceRepo provides an abstraction on top of the balance data source.
	BalanceRepo balance.Repository

	// MetadataRepo provides an abstraction on top of the metadata data source.
	MetadataRepo mongodb.Repository

	// RedisRepo provides an abstraction on top of the redis consumer.
	RedisRepo redis.RedisRepository
}
