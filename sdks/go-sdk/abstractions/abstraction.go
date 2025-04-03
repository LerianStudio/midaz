// Package abstractions provides high-level transaction operations for the Midaz platform.
//
// This package contains interfaces and functions for creating and managing financial transactions
// like deposits, withdrawals, and transfers in a simplified way.
package abstractions

import (
	"context"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
)

// Abstraction provides a centralized access point to all transaction abstraction types.
// It acts as a factory for creating specific transaction operations and follows the same
// pattern as the Entity and Builder types in other packages.
type Abstraction struct {
	// Service interfaces for different transaction types
	Deposits    DepositService
	Withdrawals WithdrawalService
	Transfers   TransferService

	// Implementation function for creating transactions
	createTransactionWithDSL func(context.Context, string, string, *models.TransactionDSLInput) (*models.Transaction, error)
}

// DepositService provides methods for creating deposit transactions.
type DepositService interface {
	// CreateDeposit creates a deposit transaction, adding funds to an internal account.
	CreateDeposit(
		ctx context.Context,
		organizationID, ledgerID string,
		targetAccountAlias string,
		amount int64, scale int64,
		assetCode string,
		description string,
		options ...Option,
	) (*models.Transaction, error)
}

// WithdrawalService provides methods for creating withdrawal transactions.
type WithdrawalService interface {
	// CreateWithdrawal creates a withdrawal transaction, removing funds from an internal account.
	CreateWithdrawal(
		ctx context.Context,
		organizationID, ledgerID string,
		sourceAccountAlias string,
		amount int64, scale int64,
		assetCode string,
		description string,
		options ...Option,
	) (*models.Transaction, error)
}

// TransferService provides methods for creating transfer transactions.
type TransferService interface {
	// CreateTransfer creates a transfer transaction between two internal accounts.
	CreateTransfer(
		ctx context.Context,
		organizationID, ledgerID string,
		sourceAccountAlias, targetAccountAlias string,
		amount int64, scale int64,
		assetCode string,
		description string,
		options ...Option,
	) (*models.Transaction, error)
}

// depositService implements the DepositService interface.
type depositService struct {
	createTx func(context.Context, string, string, *models.TransactionDSLInput) (*models.Transaction, error)
}

// withdrawalService implements the WithdrawalService interface.
type withdrawalService struct {
	createTx func(context.Context, string, string, *models.TransactionDSLInput) (*models.Transaction, error)
}

// transferService implements the TransferService interface.
type transferService struct {
	createTx func(context.Context, string, string, *models.TransactionDSLInput) (*models.Transaction, error)
}

// NewAbstraction creates a new Abstraction instance with the provided transaction creation function.
// This constructor initializes an Abstraction that provides access to all transaction services.
//
// The Abstraction abstracts away the complexities of the DSL (Domain-Specific Language) format
// used by the Midaz API for creating transactions. It provides simplified methods for
// common transaction types like deposits, withdrawals, and transfers.
//
// Parameters:
//
//   - createTransactionWithDSL: A function that creates a transaction using the DSL format.
//     This function should handle the actual API communication and take the following parameters:
//
//   - ctx: A context.Context for the request
//
//   - organizationID: The ID of the organization
//
//   - ledgerID: The ID of the ledger
//
//   - input: A TransactionDSLInput structure containing the transaction details
//
//     This is typically the CreateTransactionWithDSL method from a client.TransactionsService.
//
// Returns:
//   - *Abstraction: A pointer to the newly created Abstraction, ready to create transactions
//
// Example - Creating an abstraction with a client's DSL transaction method:
//
//	// Initialize the abstraction with a client's CreateTransactionWithDSL method
//	txAbstraction := abstractions.NewAbstraction(client.CreateTransactionWithDSL)
//
// Example - Using the abstraction to create a deposit:
//
//	// After creating the abstraction, use it to create a deposit
//	tx, err := txAbstraction.Deposits.CreateDeposit(
//	    ctx,
//	    "org-123", "ledger-456",
//	    "customer:john.doe",
//	    10000, 2, "USD",
//	    "Customer deposit",
//	    abstractions.WithMetadata(map[string]any{"reference": "DEP12345"}),
//	)
func NewAbstraction(
	createTransactionWithDSL func(context.Context, string, string, *models.TransactionDSLInput) (*models.Transaction, error),
) *Abstraction {
	abstraction := &Abstraction{
		createTransactionWithDSL: createTransactionWithDSL,
	}

	// Initialize service interfaces
	abstraction.initServices()

	return abstraction
}

// initServices initializes the service interfaces for the abstraction.
func (a *Abstraction) initServices() {
	a.Deposits = &depositService{createTx: a.createTransactionWithDSL}
	a.Withdrawals = &withdrawalService{createTx: a.createTransactionWithDSL}
	a.Transfers = &transferService{createTx: a.createTransactionWithDSL}
}
