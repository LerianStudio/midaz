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

// Create creates a deposit transaction, adding funds to an internal account.
//
// This method creates a transaction that represents a deposit of funds into an account
// within the Midaz system. A deposit typically involves crediting an internal account
// without a corresponding debit from another internal account (the funds come from outside).
//
// Parameters:
//
//   - ctx: Context for the request, which can be used for cancellation and timeout.
//
//   - organizationID: The ID of the organization that owns the ledger.
//
//   - ledgerID: The ID of the ledger where the transaction will be created.
//
//   - targetAccountAlias: The alias of the account to deposit funds into.
//     This should be a valid account alias in the format "type:identifier[:subtype]".
//
//   - amount: The amount to deposit as a fixed-point integer.
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
//	// Create a deposit of $100.00 USD to a customer account
//	tx, err := abstraction.Deposits.Create(
//	    ctx,
//	    "org-123", "ledger-456",
//	    "customer:john.doe",
//	    10000, 2, "USD",
//	    "Customer deposit",
//	    abstractions.WithMetadata(map[string]any{"reference": "DEP12345"}),
//	)
func (s *depositService) Create(
	ctx context.Context,
	organizationID, ledgerID string,
	targetAccountAlias string,
	amount int64, scale int64,
	assetCode string,
	description string,
	options ...Option,
) (*models.Transaction, error) {
	// Validate required parameters
	if targetAccountAlias == "" {
		return nil, errors.New("target account alias is required")
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
						Account: "external:" + assetCode,
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
						Account: targetAccountAlias,
						Amount: &models.DSLAmount{
							Value: amount,
							Scale: scale,
							Asset: assetCode,
						},
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

// List retrieves deposit transactions with optional filtering.
func (s *depositService) List(
	ctx context.Context,
	organizationID, ledgerID string,
	opts *models.ListOptions,
) (*models.ListResponse[models.Transaction], error) {
	if opts == nil {
		opts = &models.ListOptions{}
	}

	// Add filter for deposit transactions
	if opts.Filters == nil {
		opts.Filters = make(map[string]string)
	}

	// Add filter to identify deposit transactions
	// A deposit is a transaction where there's a credit operation to an internal account
	// and no corresponding debit operation to another internal account
	opts.Filters["transaction_type"] = "deposit"

	// Delegate to the transactions service
	return s.txService.ListTransactions(ctx, organizationID, ledgerID, opts)
}

// Get retrieves a specific deposit transaction by ID.
func (s *depositService) Get(
	ctx context.Context,
	organizationID, ledgerID, transactionID string,
) (*models.Transaction, error) {
	// Fetch the transaction
	tx, err := s.txService.GetTransaction(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		return nil, err
	}

	// Verify this is a deposit transaction
	if !isDepositTransaction(tx) {
		return nil, fmt.Errorf("transaction %s is not a deposit transaction", transactionID)
	}

	return tx, nil
}

// Update modifies a deposit transaction (e.g., metadata or status).
func (s *depositService) Update(
	ctx context.Context,
	organizationID, ledgerID, transactionID string,
	input *models.UpdateTransactionInput,
) (*models.Transaction, error) {
	// First verify this is a deposit transaction
	_, err := s.Get(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		return nil, err
	}

	// Update the transaction
	return s.txService.UpdateTransaction(ctx, organizationID, ledgerID, transactionID, input)
}

// isDepositTransaction determines if a transaction is a deposit transaction.
// A deposit transaction is one where funds are added to an internal account
// from an external source.
func isDepositTransaction(tx *models.Transaction) bool {
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

	// A deposit typically has credits to internal accounts but no debits
	// or has more credits than debits (indicating external source)
	return creditOps > 0 && (debitOps == 0 || creditOps > debitOps)
}
