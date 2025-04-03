// Package main provides examples of creating resources using the Midaz Go SDK.
package main

import (
	"context"
	"fmt"

	midaz "github.com/LerianStudio/midaz/sdks/go-sdk"
	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
)

// MakeWithdrawal demonstrates how to create a withdrawal transaction using the Builder interface.
//
// This function creates a new withdrawal transaction for the specified account using the builder pattern.
// It shows how to set required fields like organization ID, ledger ID, account reference, amount,
// scale, and asset code, as well as optional fields like description.
//
// Parameters:
//   - ctx: The context for the API request
//   - client: The Midaz client instance
//   - organizationID: The ID of the organization
//   - ledgerID: The ID of the ledger
//   - accountRef: The reference of the account to withdraw from
//   - amount: The amount to withdraw (in the smallest unit of the asset)
//   - scale: The scale of the amount (e.g., 2 for cents)
//   - assetCode: The code of the asset (e.g., "USD", "EUR")
//   - description: An optional description for the transaction
//
// Returns:
//   - *models.Transaction: The created transaction
//   - error: An error if the operation fails
func MakeWithdrawal(
	ctx context.Context,
	client *midaz.Client,
	organizationID, ledgerID, accountRef string,
	amount, scale int64,
	assetCode, description string,
) (*models.Transaction, error) {
	// Create a new withdrawal transaction using the Builder interface
	withdrawalBuilder := client.Builder.NewWithdrawal().
		WithOrganization(organizationID).
		WithLedger(ledgerID).
		WithAccountReference(accountRef).
		WithAmount(amount).
		WithScale(scale).
		WithAssetCode(assetCode).
		WithMetadata(map[string]any{
			"destination": "external",
			"method":      "bank_transfer",
			"description": description,
		}).
		WithTags([]string{"withdrawal", "example"})

	// Create the withdrawal transaction
	transaction, err := withdrawalBuilder.Create(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create withdrawal: %w", err)
	}

	return transaction, nil
}
