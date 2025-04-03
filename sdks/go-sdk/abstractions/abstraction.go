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

	// Reference to the transactions service for listing and getting transactions
	transactionsService TransactionsServiceInterface
}

// TransactionsServiceInterface defines the methods needed from the transactions service.
type TransactionsServiceInterface interface {
	ListTransactions(ctx context.Context, organizationID, ledgerID string, opts *models.ListOptions) (*models.ListResponse[models.Transaction], error)
	GetTransaction(ctx context.Context, organizationID, ledgerID, transactionID string) (*models.Transaction, error)
	UpdateTransaction(ctx context.Context, organizationID, ledgerID, transactionID string, input any) (*models.Transaction, error)
}

// DepositService provides methods for creating and managing deposit transactions.
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

	// ListDeposits lists deposit transactions with optional filtering.
	ListDeposits(
		ctx context.Context,
		organizationID, ledgerID string,
		opts *models.ListOptions,
	) (*models.ListResponse[models.Transaction], error)

	// GetDeposit retrieves a specific deposit transaction by ID.
	GetDeposit(
		ctx context.Context,
		organizationID, ledgerID, transactionID string,
	) (*models.Transaction, error)

	// UpdateDeposit updates a deposit transaction (e.g., metadata or status).
	UpdateDeposit(
		ctx context.Context,
		organizationID, ledgerID, transactionID string,
		input *models.UpdateTransactionInput,
	) (*models.Transaction, error)
}

// WithdrawalService provides methods for creating and managing withdrawal transactions.
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

	// ListWithdrawals lists withdrawal transactions with optional filtering.
	ListWithdrawals(
		ctx context.Context,
		organizationID, ledgerID string,
		opts *models.ListOptions,
	) (*models.ListResponse[models.Transaction], error)

	// GetWithdrawal retrieves a specific withdrawal transaction by ID.
	GetWithdrawal(
		ctx context.Context,
		organizationID, ledgerID, transactionID string,
	) (*models.Transaction, error)

	// UpdateWithdrawal updates a withdrawal transaction (e.g., metadata or status).
	UpdateWithdrawal(
		ctx context.Context,
		organizationID, ledgerID, transactionID string,
		input *models.UpdateTransactionInput,
	) (*models.Transaction, error)
}

// TransferService provides methods for creating and managing transfer transactions.
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

	// ListTransfers lists transfer transactions with optional filtering.
	ListTransfers(
		ctx context.Context,
		organizationID, ledgerID string,
		opts *models.ListOptions,
	) (*models.ListResponse[models.Transaction], error)

	// GetTransfer retrieves a specific transfer transaction by ID.
	GetTransfer(
		ctx context.Context,
		organizationID, ledgerID, transactionID string,
	) (*models.Transaction, error)

	// UpdateTransfer updates a transfer transaction (e.g., metadata or status).
	UpdateTransfer(
		ctx context.Context,
		organizationID, ledgerID, transactionID string,
		input *models.UpdateTransactionInput,
	) (*models.Transaction, error)
}

// depositService implements the DepositService interface.
type depositService struct {
	createTx  func(context.Context, string, string, *models.TransactionDSLInput) (*models.Transaction, error)
	txService TransactionsServiceInterface
}

// withdrawalService implements the WithdrawalService interface.
type withdrawalService struct {
	createTx  func(context.Context, string, string, *models.TransactionDSLInput) (*models.Transaction, error)
	txService TransactionsServiceInterface
}

// transferService implements the TransferService interface.
type transferService struct {
	createTx  func(context.Context, string, string, *models.TransactionDSLInput) (*models.Transaction, error)
	txService TransactionsServiceInterface
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
//   - transactionsService: The service to use for listing and getting transactions.
//
// Returns:
//   - *Abstraction: A pointer to the newly created Abstraction, ready to create transactions
//
// Example - Creating an abstraction with a client's DSL transaction method:
//
//	// Initialize the abstraction with a client's CreateTransactionWithDSL method
//	txAbstraction := abstractions.NewAbstraction(client.CreateTransactionWithDSL, client.Transactions)
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
	transactionsService TransactionsServiceInterface,
) *Abstraction {
	abstraction := &Abstraction{
		createTransactionWithDSL: createTransactionWithDSL,
		transactionsService:      transactionsService,
	}

	// Initialize service interfaces
	abstraction.initServices()

	return abstraction
}

// initServices initializes the service interfaces for the abstraction.
func (a *Abstraction) initServices() {
	a.Deposits = &depositService{
		createTx:  a.createTransactionWithDSL,
		txService: a.transactionsService,
	}
	a.Withdrawals = &withdrawalService{
		createTx:  a.createTransactionWithDSL,
		txService: a.transactionsService,
	}
	a.Transfers = &transferService{
		createTx:  a.createTransactionWithDSL,
		txService: a.transactionsService,
	}
}
