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

// CreateDeposit creates a deposit transaction, adding funds to an internal account.
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
//	tx, err := txAbstraction.CreateDeposit(
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
//	tx, err := txAbstraction.CreateDeposit(
//	    ctx,
//	    "org-123", "ledger-456",
//	    "customer:john.doe",
//	    10000, 2, "USD",
//	    "Customer deposit",
//	    abstractions.WithIdempotencyKey("deposit-20230315-123"),
//	)
//
// Example - Pending deposit (requires explicit commit):
//
//	// Create a pending deposit that requires explicit commitment
//	tx, err := txAbstraction.CreateDeposit(
//	    ctx,
//	    "org-123", "ledger-456",
//	    "customer:john.doe",
//	    10000, 2, "USD",
//	    "Customer deposit pending verification",
//	    abstractions.WithPending(true),
//	)
//
//	// Later, after verification:
//	// client.Transactions.CommitTransaction(ctx, "org-123", "ledger-456", tx.ID)
func (a *Abstraction) CreateDeposit(
	ctx context.Context,
	organizationID, ledgerID string,
	targetAccountAlias string,
	amount int64,
	scale int,
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
						Account: externalAccount,
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
