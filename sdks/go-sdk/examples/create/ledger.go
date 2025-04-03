// Package main provides examples of creating resources using the Midaz Go SDK.
package main

import (
	"context"
	"fmt"

	midaz "github.com/LerianStudio/midaz/sdks/go-sdk"
	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
)

// CreateLedger demonstrates how to create a ledger using the Builder interface.
//
// This function creates a new ledger for the specified organization using the builder pattern.
// It shows how to set required fields like organization ID and name, as well as optional
// fields like status and metadata.
//
// Parameters:
//   - ctx: The context for the API request
//   - client: The Midaz client instance
//   - organizationID: The ID of the organization that will own the ledger
//
// Returns:
//   - *models.Ledger: The created ledger
//   - error: An error if the operation fails
func CreateLedger(ctx context.Context, client *midaz.Client, organizationID string) (*models.Ledger, error) {
	// Create a new ledger using the Builder interface
	ledger, err := client.Builder.NewLedger().
		WithOrganization(organizationID).
		WithName("Main Ledger").
		WithStatus("ACTIVE").
		WithMetadata(map[string]any{
			"purpose":    "General accounting",
			"department": "Finance",
			"year":       2025,
		}).
		WithTags([]string{"main", "production"}).
		Create(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to create ledger: %w", err)
	}

	return ledger, nil
}
