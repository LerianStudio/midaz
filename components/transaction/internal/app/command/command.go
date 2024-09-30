package command

import (
	a "github.com/LerianStudio/midaz/components/transaction/internal/domain/account"
	t "github.com/LerianStudio/midaz/components/transaction/internal/domain/transaction"
)

// UseCase is a struct that aggregates various repositories for simplified access in use case implementations.
type UseCase struct {
	// TransactionRepo provides an abstraction on top of the transaction data source.
	TransactionRepo t.Repository

	// AccountGRPCRepo provides an abstraction on top of the account grpc.
	AccountGRPCRepo a.Repository
}
