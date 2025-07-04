package commands

import (
	"github.com/LerianStudio/midaz/components/consumer/internal/adapters/mongodb"
	"github.com/LerianStudio/midaz/components/consumer/internal/adapters/postgresql/balance"
	"github.com/LerianStudio/midaz/components/consumer/internal/adapters/postgresql/operation"
	"github.com/LerianStudio/midaz/components/consumer/internal/adapters/postgresql/transaction"
	"github.com/LerianStudio/midaz/components/consumer/internal/adapters/rabbitmq"
)

// UseCase is a struct that aggregates various repositories for simplified access in use case implementations.
type UseCase struct {
	// TransactionRepo provides an abstraction on top of the transaction data source.
	TransactionRepo transaction.Repository

	// OperationRepo provides an abstraction on top of the operation data source.
	OperationRepo operation.Repository

	// BalanceRepo provides an abstraction on top of the balance data source.
	BalanceRepo balance.Repository

	// MetadataRepo provides an abstraction on top of the metadata data source.
	MetadataRepo mongodb.Repository

	// RabbitMQRepo provides an abstraction on top of the producer rabbitmq.
	RabbitMQRepo rabbitmq.ProducerRepository
}
