// Package main provides examples of creating resources using the Midaz Go SDK.
package main

import (
	"context"
	"fmt"

	midaz "github.com/LerianStudio/midaz/sdks/go-sdk"
	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
)

// CreateAsset demonstrates how to create an asset using the Builder interface.
//
// This function creates a new asset for the specified organization and ledger using the builder pattern.
// It shows how to set required fields like organization ID, ledger ID, code, and name, as well as optional
// fields like status and metadata.
//
// Parameters:
//   - ctx: The context for the API request
//   - client: The Midaz client instance
//   - organizationID: The ID of the organization that will own the asset
//   - ledgerID: The ID of the ledger that will contain the asset
//   - code: The unique code for the asset (e.g., "USD", "EUR")
//   - name: The human-readable name for the asset (e.g., "US Dollar", "Euro")
//
// Returns:
//   - *models.Asset: The created asset
//   - error: An error if the operation fails
func CreateAsset(
	ctx context.Context,
	client *midaz.Client,
	organizationID, ledgerID, code, name string,
) (*models.Asset, error) {
	// Create a new asset using the Builder interface
	asset, err := client.Builder.NewAsset().
		WithOrganization(organizationID).
		WithLedger(ledgerID).
		WithCode(code).
		WithName(name).
		WithStatus("ACTIVE").
		WithMetadata(map[string]any{
			"type":        "currency",
			"description": fmt.Sprintf("%s currency asset", name),
			"symbol":      code,
		}).
		WithTags([]string{code, "currency"}).
		Create(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to create asset: %w", err)
	}

	return asset, nil
}
