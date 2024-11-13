package query

import (
	a "github.com/LerianStudio/midaz/components/transaction/internal/domain/account"
	ar "github.com/LerianStudio/midaz/components/transaction/internal/domain/assetrate"
	m "github.com/LerianStudio/midaz/components/transaction/internal/domain/metadata"
	o "github.com/LerianStudio/midaz/components/transaction/internal/domain/operation"
	rmq "github.com/LerianStudio/midaz/components/transaction/internal/domain/rabbitmq"
	t "github.com/LerianStudio/midaz/components/transaction/internal/domain/transaction"
)

// UseCase is a struct that aggregates various repositories for simplified access in use case implementations.
type UseCase struct {
	// TransactionRepo provides an abstraction on top of the transaction data source.
	TransactionRepo t.Repository

	// AccountGRPCRepo provides an abstraction on top of the account grpc.
	AccountGRPCRepo a.Repository

	// OperationRepo provides an abstraction on top of the operation data source.
	OperationRepo o.Repository

	// AssetRateRepo provides an abstraction on top of the operation data source.
	AssetRateRepo ar.Repository

	// MetadataRepo provides an abstraction on top of the metadata data source.
	MetadataRepo m.Repository

	// RabbitMQRepo provides an abstraction on top of the consumer rabbitmq.
	RabbitMQRepo rmq.ConsumerRepository
}
