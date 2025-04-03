// Package abstractions provides high-level transaction operations for the Midaz platform.
//
// This package contains functions and options for creating and managing financial transactions
// like deposits, withdrawals, and transfers.
package abstractions

import (
	"context"
	"errors"
	"fmt"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
)

// Create creates a withdrawal transaction, removing funds from an internal account.
//
// This method creates a transaction that represents a withdrawal of funds from an account
// within the Midaz system. A withdrawal typically involves debiting an internal account
// without a corresponding credit to another internal account (the funds go outside).
//
// Parameters:
//
//   - ctx: Context for the request, which can be used for cancellation and timeout.
//
//   - organizationID: The ID of the organization that owns the ledger.
//
//   - ledgerID: The ID of the ledger where the transaction will be created.
//
//   - sourceAccountAlias: The alias of the account to withdraw funds from.
//     This should be a valid account alias in the format "type:identifier[:subtype]".
//
//   - amount: The amount to withdraw as a fixed-point integer.
//     The actual amount is calculated as amount / 10^scale.
//
//   - scale: The decimal precision for the amount.
//     For example, a scale of 2 means the amount is in cents (100 = $1.00).
//
//   - assetCode: The currency or asset type for this transaction (e.g., "USD", "EUR").
//
//   - description: A human-readable description of the transaction.
//
//   - options: Optional parameters for the transaction, such as metadata or idempotency key.
//     See the Option type for available options.
//
// Returns:
//
//   - *models.Transaction: The created transaction if successful.
//
//   - error: An error if the operation fails, such as invalid parameters or API errors.
//
// Example:
//
//	// Create a withdrawal of $100.00 USD from a customer account
//	tx, err := abstraction.Withdrawals.Create(
//	    ctx,
//	    "org-123", "ledger-456",
//	    "customer:john.doe",
//	    10000, 2, "USD",
//	    "Customer withdrawal",
//	    abstractions.WithMetadata(map[string]any{"reference": "WDR12345"}),
//	)
func (s *withdrawalService) Create(
	ctx context.Context,
	organizationID, ledgerID string,
	sourceAccountAlias string,
	amount int64, scale int64,
	assetCode string,
	description string,
	options ...Option,
) (*models.Transaction, error) {
	// Validate required parameters
	if sourceAccountAlias == "" {
		return nil, errors.New("source account alias is required")
	}

	if amount <= 0 {
		return nil, errors.New("amount must be greater than zero")
	}

	if assetCode == "" {
		return nil, errors.New("asset code is required")
	}

	// Create the DSL input
	input := &models.TransactionDSLInput{
		Description: description,
		Send: &models.DSLSend{
			Asset: assetCode,
			Value: amount,
			Scale: scale,
			Source: &models.DSLSource{
				From: []models.DSLFromTo{
					{
						Account: sourceAccountAlias,
						Amount: &models.DSLAmount{
							Value: amount,
							Scale: scale,
							Asset: assetCode,
						},
					},
				},
			},
			Distribute: &models.DSLDistribute{
				To: []models.DSLFromTo{
					{
						Account: "external:" + assetCode,
					},
				},
			},
		},
	}

	// Apply options
	for _, opt := range options {
		if err := opt(input); err != nil {
			return nil, fmt.Errorf("option application error: %w", err)
		}
	}

	// Validate the DSL input
	if err := input.Validate(); err != nil {
		return nil, fmt.Errorf("DSL validation error: %w", err)
	}

	// Create the transaction
	return s.createTx(ctx, organizationID, ledgerID, input)
}

// List retrieves withdrawal transactions with optional filtering.
func (s *withdrawalService) List(
	ctx context.Context,
	organizationID, ledgerID string,
	opts *models.ListOptions,
) (*models.ListResponse[models.Transaction], error) {
	if opts == nil {
		opts = &models.ListOptions{}
	}

	// Add filter for withdrawal transactions
	if opts.Filters == nil {
		opts.Filters = make(map[string]string)
	}

	// Add filter to identify withdrawal transactions
	// A withdrawal is a transaction where there's a debit operation from an internal account
	// and no corresponding credit operation to another internal account
	opts.Filters["transaction_type"] = "withdrawal"

	// Delegate to the transactions service
	return s.txService.ListTransactions(ctx, organizationID, ledgerID, opts)
}

// Get retrieves a specific withdrawal transaction by ID.
func (s *withdrawalService) Get(
	ctx context.Context,
	organizationID, ledgerID, transactionID string,
) (*models.Transaction, error) {
	// Fetch the transaction
	tx, err := s.txService.GetTransaction(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		return nil, err
	}

	// Verify this is a withdrawal transaction
	if !isWithdrawalTransaction(tx) {
		return nil, fmt.Errorf("transaction %s is not a withdrawal transaction", transactionID)
	}

	return tx, nil
}

// Update modifies a withdrawal transaction (e.g., metadata or status).
func (s *withdrawalService) Update(
	ctx context.Context,
	organizationID, ledgerID, transactionID string,
	input *models.UpdateTransactionInput,
) (*models.Transaction, error) {
	// First verify this is a withdrawal transaction
	_, err := s.Get(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		return nil, err
	}

	// Update the transaction
	return s.txService.UpdateTransaction(ctx, organizationID, ledgerID, transactionID, input)
}

// isWithdrawalTransaction determines if a transaction is a withdrawal transaction.
// A withdrawal transaction is one where funds are removed from an internal account
// to an external destination.
func isWithdrawalTransaction(tx *models.Transaction) bool {
	if tx == nil || len(tx.Operations) == 0 {
		return false
	}

	// Count credits and debits to internal accounts
	var creditOps, debitOps int

	for _, op := range tx.Operations {
		if op.Type == "credit" {
			creditOps++
		} else if op.Type == "debit" {
			debitOps++
		}
	}

	// A withdrawal typically has debits from internal accounts but no credits
	// or has more debits than credits (indicating external destination)
	return debitOps > 0 && (creditOps == 0 || debitOps > creditOps)
}
