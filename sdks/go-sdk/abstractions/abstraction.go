// Package abstractions provides high-level transaction operations for the Midaz platform.
//
// This package contains functions and options for creating and managing financial transactions
// like deposits, withdrawals, and transfers.
package abstractions

import (
	"context"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
)

// Abstraction provides high-level transaction operations.
// This abstraction simplifies the creation of common transaction types like deposits,
// withdrawals, and transfers by abstracting away the complexities of the underlying
// Domain-Specific Language (DSL) format.
//
// The Abstraction is designed to be used through the TransactionService in the services
// package, but can also be used directly if needed.
type Abstraction struct {
	// createTransactionWithDSL is a function that creates a transaction using the DSL format
	createTransactionWithDSL func(context.Context, string, string, *models.TransactionDSLInput) (*models.Transaction, error)
}

// NewAbstraction creates a new transactions abstraction with the given implementation function.
// This constructor initializes an Abstraction that provides high-level transaction operations.
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
//	tx, err := txAbstraction.CreateDeposit(
//	    ctx,
//	    "org-123", "ledger-456",
//	    "customer:john.doe",
//	    10000, 2, "USD",
//	    "Customer deposit",
//	    abstractions.WithMetadata(map[string]any{"reference": "DEP12345"}),
//	)
//
// Example - Creating an abstraction with a custom implementation:
//
//	// Create an abstraction with a custom implementation for testing or special handling
//	mockCreateTx := func(ctx context.Context, orgID, ledgerID string, input *models.TransactionDSLInput) (*models.Transaction, error) {
//	    // Custom implementation for testing or special handling
//	    return &models.Transaction{
//	        ID:          "tx-mock-123",
//	        Description: input.Description,
//	        Status:      models.StatusCompleted,
//	        // ... other fields
//	    }, nil
//	}
//
//	txAbstraction := abstractions.NewAbstraction(mockCreateTx)
func NewAbstraction(
	createTransactionWithDSL func(context.Context, string, string, *models.TransactionDSLInput) (*models.Transaction, error),
) *Abstraction {
	return &Abstraction{
		createTransactionWithDSL: createTransactionWithDSL,
	}
}
