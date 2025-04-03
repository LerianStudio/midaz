// Package abstractions provides high-level transaction operations for the Midaz platform.
//
// This package contains functions and options for creating and managing financial transactions
// like deposits, withdrawals, and transfers.
package abstractions

import (
	"context"
	"errors"

	"github.com/LerianStudio/midaz/sdks/go-sdk/internal/utils"
	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
)

// CreateTransfer creates a transfer transaction between two internal accounts.
//
// A transfer moves funds from one internal account to another within the same ledger.
// Both the source and target must be valid internal accounts identified by their aliases.
//
// Parameters:
//   - ctx: Context for the request, can be used for cancellation and timeout
//   - organizationID: The unique identifier of the organization (e.g., "org-123")
//   - ledgerID: The unique identifier of the ledger within the organization (e.g., "ledger-456")
//   - sourceAccountAlias: The alias of the account providing the funds (e.g., "user:alice")
//   - targetAccountAlias: The alias of the account receiving the funds (e.g., "user:bob")
//   - amount: The amount as a fixed-point integer (actual amount = amount / 10^scale)
//     For example, 2500 with scale 2 represents $25.00
//   - scale: The decimal scale factor for the amount (typically 2 for cents, 0 for whole units)
//   - assetCode: The currency or asset code (e.g., "USD", "EUR", "BTC")
//   - description: A human-readable description of the purpose of the transfer
//   - options: Optional settings like metadata, externalID, or idempotency key
//
// Returns:
//   - *models.Transaction: The created transaction with details including ID, status, and operations
//   - error: An error if the operation fails, such as validation errors, insufficient funds, or API communication issues
//
// Example - Basic transfer:
//
//	// Transfer $100.00 between two accounts
//	tx, err := txAbstraction.CreateTransfer(
//	    ctx,
//	    "org-123", "ledger-456",
//	    "customer:john.doe",
//	    "merchant:acme",
//	    10000, 2, "USD",
//	    "Payment for services",
//	    abstractions.WithMetadata(map[string]any{"reference": "INV12345"}),
//	)
//
// Example - Transfer with idempotency key:
//
//	// Transfer with idempotency key to prevent duplicate transactions
//	tx, err := txAbstraction.CreateTransfer(
//	    ctx,
//	    "org-123", "ledger-456",
//	    "customer:john.doe",
//	    "merchant:acme",
//	    10000, 2, "USD",
//	    "Payment for services",
//	    abstractions.WithIdempotencyKey("payment-20230315-123"),
//	)
//
// Example - Pending transfer (requires explicit commit):
//
//	// Create a pending transfer that requires explicit approval
//	tx, err := txAbstraction.CreateTransfer(
//	    ctx,
//	    "org-123", "ledger-456",
//	    "account:reserves",
//	    "account:investments",
//	    1000000, 2, "USD", // $10,000.00
//	    "Investment allocation pending approval",
//	    abstractions.WithPending(true),
//	    abstractions.WithNotes("Requires CFO approval for investment allocations"),
//	)
//
//	// Later, after approval:
//	// client.Transactions.CommitTransaction(ctx, "org-123", "ledger-456", tx.ID)
func (a *Abstraction) CreateTransfer(
	ctx context.Context,
	organizationID, ledgerID string,
	sourceAccountAlias string,
	targetAccountAlias string,
	amount int64,
	scale int,
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

	// Build the DSL input
	input := &models.TransactionDSLInput{
		Description: description,
		Send: &models.DSLSend{
			Asset: assetCode,
			Value: amount,
			Scale: int64(scale),
			Source: &models.DSLSource{
				From: []models.DSLFromTo{
					{
						Account: sourceAccountAlias,
						Amount: &models.DSLAmount{
							Value: amount,
							Scale: int64(scale),
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
							Scale: int64(scale),
							Asset: assetCode,
						},
					},
				},
			},
		},
	}

	// Apply any optional configuration
	for _, option := range options {
		option(input)
	}

	// Validate the transaction before sending to the API
	if err := utils.ValidateTransactionDSL(input); err != nil {
		return nil, err
	}

	// Create the transaction using DSL
	return a.createTransactionWithDSL(ctx, organizationID, ledgerID, input)
}
