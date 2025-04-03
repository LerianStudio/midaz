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

// CreateWithdrawal implements the WithdrawalService interface.
//
// A withdrawal represents money leaving the system to an external destination.
// The source must be a valid internal account identified by its alias, and the
// destination is implicitly an external account.
//
// Parameters:
//   - ctx: Context for the request, can be used for cancellation and timeout
//   - organizationID: The unique identifier of the organization (e.g., "org-123")
//   - ledgerID: The unique identifier of the ledger within the organization (e.g., "ledger-456")
//   - sourceAccountAlias: The alias of the account providing the funds (e.g., "merchant:acme")
//   - amount: The amount as a fixed-point integer (actual amount = amount / 10^scale)
//     For example, 5000 with scale 2 represents $50.00
//   - scale: The decimal scale factor for the amount (typically 2 for cents, 0 for whole units)
//   - assetCode: The currency or asset code (e.g., "USD", "EUR", "BTC")
//   - description: A human-readable description of the purpose of the withdrawal
//   - options: Optional settings like metadata, externalID, or idempotency key
//
// Returns:
//   - *models.Transaction: The created transaction with details including ID, status, and operations
//   - error: An error if the operation fails, such as validation errors, insufficient funds, or API communication issues
//
// Example - Basic withdrawal:
//
//	// Withdraw $100.00 from a customer's account
//	tx, err := abstraction.Withdrawals.CreateWithdrawal(
//	    ctx,
//	    "org-123", "ledger-456",
//	    "customer:john.doe",
//	    10000, 2, "USD",
//	    "Customer withdrawal",
//	    abstractions.WithMetadata(map[string]any{"reference": "WD12345"}),
//	)
//
// Example - Withdrawal with idempotency key:
//
//	// Withdrawal with idempotency key to prevent duplicate transactions
//	tx, err := abstraction.Withdrawals.CreateWithdrawal(
//	    ctx,
//	    "org-123", "ledger-456",
//	    "customer:john.doe",
//	    10000, 2, "USD",
//	    "Customer withdrawal",
//	    abstractions.WithIdempotencyKey("withdrawal-2023-03-15-12345"),
//	)
func (s *withdrawalService) CreateWithdrawal(
	ctx context.Context,
	organizationID, ledgerID string,
	sourceAccountAlias string,
	amount int64, scale int64,
	assetCode string,
	description string,
	options ...Option,
) (*models.Transaction, error) {
	// Validate required parameters
	if organizationID == "" {
		return nil, errors.New("organizationID is required")
	}
	if ledgerID == "" {
		return nil, errors.New("ledgerID is required")
	}
	if sourceAccountAlias == "" {
		return nil, errors.New("sourceAccountAlias is required")
	}
	if amount <= 0 {
		return nil, errors.New("amount must be positive")
	}
	if scale < 0 {
		return nil, errors.New("scale must be non-negative")
	}
	if assetCode == "" {
		return nil, errors.New("assetCode is required")
	}

	// Create a DSL transaction input
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
	for _, option := range options {
		option(input)
	}

	// Create the transaction
	return s.createTx(ctx, organizationID, ledgerID, input)
}

// ListWithdrawals lists withdrawal transactions with optional filtering.
func (s *withdrawalService) ListWithdrawals(
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

// GetWithdrawal retrieves a specific withdrawal transaction by ID.
func (s *withdrawalService) GetWithdrawal(
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

// UpdateWithdrawal updates a withdrawal transaction (e.g., metadata or status).
func (s *withdrawalService) UpdateWithdrawal(
	ctx context.Context,
	organizationID, ledgerID, transactionID string,
	input *models.UpdateTransactionInput,
) (*models.Transaction, error) {
	// First verify this is a withdrawal transaction
	_, err := s.GetWithdrawal(ctx, organizationID, ledgerID, transactionID)
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
