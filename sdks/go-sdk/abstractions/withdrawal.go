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

// CreateWithdrawal creates a withdrawal transaction, removing funds from an internal account.
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
//	tx, err := txAbstraction.CreateWithdrawal(
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
//	tx, err := txAbstraction.CreateWithdrawal(
//	    ctx,
//	    "org-123", "ledger-456",
//	    "customer:john.doe",
//	    10000, 2, "USD",
//	    "Customer withdrawal",
//	    abstractions.WithIdempotencyKey("withdrawal-20230315-123"),
//	)
//
// Example - Pending withdrawal (requires explicit commit):
//
//	// Create a pending withdrawal that requires explicit approval
//	tx, err := txAbstraction.CreateWithdrawal(
//	    ctx,
//	    "org-123", "ledger-456",
//	    "merchant:acme",
//	    100000, 2, "USD", // $1,000.00
//	    "Large withdrawal pending approval",
//	    abstractions.WithPending(true),
//	    abstractions.WithNotes("Requires manager approval for amounts over $500"),
//	)
//
//	// Later, after approval:
//	// client.Transactions.CommitTransaction(ctx, "org-123", "ledger-456", tx.ID)
func (a *Abstraction) CreateWithdrawal(
	ctx context.Context,
	organizationID, ledgerID string,
	sourceAccountAlias string,
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

	if amount <= 0 {
		return nil, errors.New("amount must be greater than zero")
	}

	if assetCode == "" {
		return nil, errors.New("asset code is required")
	}

	// Create the external account reference
	externalAccount := utils.GetExternalAccountReference(assetCode)

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
						Account: externalAccount,
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
