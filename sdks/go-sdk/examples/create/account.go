// Package main provides examples of creating resources using the Midaz Go SDK.
package main

import (
	"context"
	"fmt"

	midaz "github.com/LerianStudio/midaz/sdks/go-sdk"
	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
)

// CreateAccount demonstrates how to create an account using the Builder interface.
//
// This function creates a new account for the specified organization and ledger using the builder pattern.
// It shows how to set required fields like organization ID, ledger ID, name, asset code, and type,
// as well as optional fields like status, metadata, and reference.
//
// Parameters:
//   - ctx: The context for the API request
//   - client: The Midaz client instance
//   - organizationID: The ID of the organization that will own the account
//   - ledgerID: The ID of the ledger that will contain the account
//   - name: The human-readable name for the account
//   - assetCode: The code of the asset for this account (e.g., "USD", "EUR")
//   - accountType: The type of account (e.g., "ASSET", "LIABILITY")
//   - reference: An optional external reference for the account
//
// Returns:
//   - *models.Account: The created account
//   - error: An error if the operation fails
func CreateAccount(
	ctx context.Context,
	client *midaz.Client,
	organizationID, ledgerID, name, assetCode, accountType, reference string,
) (*models.Account, error) {
	// Create a new account using the Builder interface
	accountBuilder := client.Builder.NewAccount().
		WithOrganization(organizationID).
		WithLedger(ledgerID).
		WithName(name).
		WithAssetCode(assetCode).
		WithType(accountType).
		WithStatus("ACTIVE").
		WithMetadata(map[string]any{
			"description": fmt.Sprintf("%s account in %s", name, assetCode),
			"created_at":  "2023-01-01T00:00:00Z",
		}).
		WithTags([]string{accountType, assetCode, "example"})

	// Add reference if provided
	if reference != "" {
		accountBuilder = accountBuilder.WithReference(reference)
	}

	// Create the account
	account, err := accountBuilder.Create(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create account: %w", err)
	}

	return account, nil
}
