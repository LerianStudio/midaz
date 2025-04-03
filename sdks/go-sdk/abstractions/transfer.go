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

// Create creates a transfer transaction between two internal accounts.
//
// This method creates a transaction that represents a transfer of funds between two accounts
// within the Midaz system. A transfer involves debiting one internal account and crediting
// another internal account with the same amount and asset type.
//
// Parameters:
//
//   - ctx: Context for the request, which can be used for cancellation and timeout.
//
//   - organizationID: The ID of the organization that owns the ledger.
//
//   - ledgerID: The ID of the ledger where the transaction will be created.
//
//   - sourceAccountAlias: The alias of the account to transfer funds from.
//     This should be a valid account alias in the format "type:identifier[:subtype]".
//
//   - targetAccountAlias: The alias of the account to transfer funds to.
//     This should be a valid account alias in the format "type:identifier[:subtype]".
//
//   - amount: The amount to transfer as a fixed-point integer.
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
//	// Create a transfer of $100.00 USD between two accounts
//	tx, err := abstraction.Transfers.Create(
//	    ctx,
//	    "org-123", "ledger-456",
//	    "customer:john.doe", "merchant:acme",
//	    10000, 2, "USD",
//	    "Payment for services",
//	    abstractions.WithMetadata(map[string]any{"reference": "TRF12345"}),
//	)
func (s *transferService) Create(
	ctx context.Context,
	organizationID, ledgerID string,
	sourceAccountAlias, targetAccountAlias string,
	amount int64, scale int64,
	assetCode string,
	description string,
	options ...Option,
) (*models.Transaction, error) {
	// Validate required parameters
	if sourceAccountAlias == "" {
		return nil, errors.New("source account alias is required")
	}

	if targetAccountAlias == "" {
		return nil, errors.New("target account alias is required")
	}

	if sourceAccountAlias == targetAccountAlias {
		return nil, errors.New("source and target accounts must be different")
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

// List retrieves transfer transactions with optional filtering.
func (s *transferService) List(
	ctx context.Context,
	organizationID, ledgerID string,
	opts *models.ListOptions,
) (*models.ListResponse[models.Transaction], error) {
	if opts == nil {
		opts = &models.ListOptions{}
	}

	// Add filter for transfer transactions
	if opts.Filters == nil {
		opts.Filters = make(map[string]string)
	}

	// Add filter to identify transfer transactions
	// A transfer is a transaction where there are both debit and credit operations
	// between internal accounts
	opts.Filters["transaction_type"] = "transfer"

	// Delegate to the transactions service
	return s.txService.ListTransactions(ctx, organizationID, ledgerID, opts)
}

// Get retrieves a specific transfer transaction by ID.
func (s *transferService) Get(
	ctx context.Context,
	organizationID, ledgerID, transactionID string,
) (*models.Transaction, error) {
	// Fetch the transaction
	tx, err := s.txService.GetTransaction(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		return nil, err
	}

	// Verify this is a transfer transaction
	if !isTransferTransaction(tx) {
		return nil, fmt.Errorf("transaction %s is not a transfer transaction", transactionID)
	}

	return tx, nil
}

// Update modifies a transfer transaction (e.g., metadata or status).
func (s *transferService) Update(
	ctx context.Context,
	organizationID, ledgerID, transactionID string,
	input *models.UpdateTransactionInput,
) (*models.Transaction, error) {
	// First verify this is a transfer transaction
	_, err := s.Get(ctx, organizationID, ledgerID, transactionID)
	if err != nil {
		return nil, err
	}

	// Update the transaction
	return s.txService.UpdateTransaction(ctx, organizationID, ledgerID, transactionID, input)
}

// isTransferTransaction determines if a transaction is a transfer transaction.
// A transfer transaction is one where funds are moved between two internal accounts.
func isTransferTransaction(tx *models.Transaction) bool {
	if tx == nil || len(tx.Operations) < 2 {
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

	// A transfer typically has balanced debits and credits between internal accounts
	return creditOps > 0 && debitOps > 0 && creditOps == debitOps
}
