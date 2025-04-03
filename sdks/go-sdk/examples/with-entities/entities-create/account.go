// Package main provides examples of creating resources using the Midaz Go SDK.
package main

import (
	"context"
	"encoding/json"
	"fmt"

	midaz "github.com/LerianStudio/midaz/sdks/go-sdk"
	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
)

// CreateAccount demonstrates how to create an account using the Builder interface.
//
// This function creates a new account for the specified organization and ledger using the builder pattern.
// It shows how to set required fields like organization ID, ledger ID, name, asset code, and type,
// as well as optional fields like status, metadata, and alias.
//
// Parameters:
//   - ctx: The context for the API request
//   - client: The Midaz client instance
//   - organizationID: The ID of the organization that will own the account
//   - ledgerID: The ID of the ledger that will contain the account
//   - name: The human-readable name for the account
//   - assetCode: The code of the asset for this account (e.g., "USD", "EUR")
//   - accountType: The type of account (e.g., "ASSET", "LIABILITY")
//   - alias: An optional alias/reference for the account
//
// Returns:
//   - *models.Account: The created account
//   - error: An error if the operation fails
func CreateAccount(
	ctx context.Context,
	client *midaz.Client,
	organizationID, ledgerID, name, assetCode, accountType, alias string,
) (*models.Account, error) {
	// Debug information
	fmt.Printf("DEBUG: Creating account %s for organization ID: '%s', ledger ID: '%s'\n",
		name, organizationID, ledgerID)

	// Validate required parameters
	if organizationID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	if ledgerID == "" {
		return nil, fmt.Errorf("ledger ID is required")
	}

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

	// Add alias if provided
	if alias != "" {
		fmt.Printf("DEBUG: Setting account alias: %s\n", alias)
		accountBuilder = accountBuilder.WithAlias(alias)
	}

	// Print builder configuration for debugging
	fmt.Printf("DEBUG: Account builder configured for %s (%s)\n", name, assetCode)

	// Execute the create operation
	fmt.Println("DEBUG: Calling Create on account builder...")
	account, err := accountBuilder.Create(ctx)
	if err != nil {
		fmt.Printf("DEBUG: Error creating account: %v\n", err)
		return nil, fmt.Errorf("failed to create account: %w", err)
	}

	// Print the account details for debugging
	accountJSON, _ := json.MarshalIndent(account, "", "  ")
	fmt.Printf("DEBUG: Account response: %s\n", string(accountJSON))

	// Check if we got a valid ID
	if account.ID == "" {
		fmt.Println("DEBUG: Warning - Account created but ID is empty")
		return nil, fmt.Errorf("account created but ID is empty")
	}

	fmt.Printf("DEBUG: Account ID from API: %s\n", account.ID)
	return account, nil
}
