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

// CreateDeposit implements the DepositService interface.
//
// A deposit represents money coming into the system from an external source.
// The source is implicitly an external account, and the target must be a valid
// internal account identified by its alias.
//
// Parameters:
//   - ctx: Context for the request, can be used for cancellation and timeout
//   - organizationID: The unique identifier of the organization (e.g., "org-123")
//   - ledgerID: The unique identifier of the ledger within the organization (e.g., "ledger-456")
//   - targetAccountAlias: The alias of the account receiving the funds (e.g., "customer:john.doe")
//   - amount: The amount as a fixed-point integer (actual amount = amount / 10^scale)
//     For example, 10000 with scale 2 represents $100.00
//   - scale: The decimal scale factor for the amount (typically 2 for cents, 0 for whole units)
//   - assetCode: The currency or asset code (e.g., "USD", "EUR", "BTC")
//   - description: A human-readable description of the purpose of the deposit
//   - options: Optional settings like metadata, externalID, or idempotency key
//
// Returns:
//   - *models.Transaction: The created transaction with details including ID, status, and operations
//   - error: An error if the operation fails, such as validation errors or API communication issues
//
// Example - Basic deposit:
//
//	// Deposit $100.00 to a customer's account
//	tx, err := abstraction.Deposits.CreateDeposit(
//	    ctx,
//	    "org-123", "ledger-456",
//	    "customer:john.doe",
//	    10000, 2, "USD",
//	    "Customer deposit",
//	    abstractions.WithMetadata(map[string]any{"reference": "DEP12345"}),
//	)
//
// Example - Deposit with idempotency key:
//
//	// Deposit with idempotency key to prevent duplicate transactions
//	tx, err := abstraction.Deposits.CreateDeposit(
//	    ctx,
//	    "org-123", "ledger-456",
//	    "customer:john.doe",
//	    10000, 2, "USD",
//	    "Customer deposit",
//	    abstractions.WithIdempotencyKey("deposit-2023-03-15-12345"),
//	)
func (s *depositService) CreateDeposit(
	ctx context.Context,
	organizationID, ledgerID string,
	targetAccountAlias string,
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
	if targetAccountAlias == "" {
		return nil, errors.New("targetAccountAlias is required")
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
	for _, option := range options {
		option(input)
	}

	// Create the transaction
	return s.createTx(ctx, organizationID, ledgerID, input)
}

// ListDeposits lists deposit transactions with optional filtering.
func (s *depositService) ListDeposits(
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

// GetDeposit retrieves a specific deposit transaction by ID.
func (s *depositService) GetDeposit(
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

// UpdateDeposit updates a deposit transaction (e.g., metadata or status).
func (s *depositService) UpdateDeposit(
	ctx context.Context,
	organizationID, ledgerID, transactionID string,
	input *models.UpdateTransactionInput,
) (*models.Transaction, error) {
	// First verify this is a deposit transaction
	_, err := s.GetDeposit(ctx, organizationID, ledgerID, transactionID)
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
