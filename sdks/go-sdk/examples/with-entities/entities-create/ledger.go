// Package main provides examples of creating resources using the Midaz Go SDK.
package main

import (
	"context"
	"encoding/json"
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
	// Debug information
	fmt.Printf("DEBUG: Creating ledger for organization ID: '%s'\n", organizationID)

	// Validate required parameters
	if organizationID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	// Create a new ledger using the Builder interface
	builder := client.Builder.NewLedger().
		WithOrganization(organizationID).
		WithName("Main Ledger").
		WithStatus("ACTIVE").
		WithMetadata(map[string]any{
			"purpose":    "General accounting",
			"department": "Finance",
			"year":       2025,
		}).
		WithTags([]string{"main", "production"})

	// Print builder configuration for debugging
	fmt.Println("DEBUG: Ledger builder configured with the following:")
	fmt.Printf("DEBUG: - Organization ID: %s\n", organizationID)
	fmt.Printf("DEBUG: - Name: Main Ledger\n")
	fmt.Printf("DEBUG: - Status: ACTIVE\n")

	// Execute the create operation
	fmt.Println("DEBUG: Calling Create on ledger builder...")
	ledger, err := builder.Create(ctx)

	if err != nil {
		fmt.Printf("DEBUG: Error creating ledger: %v\n", err)
		return nil, fmt.Errorf("failed to create ledger: %w", err)
	}

	// Print the ledger details for debugging
	ledgerJSON, _ := json.MarshalIndent(ledger, "", "  ")
	fmt.Printf("DEBUG: Ledger response: %s\n", string(ledgerJSON))

	// Check if we got a valid ID
	if ledger.ID == "" {
		fmt.Println("DEBUG: Warning - Ledger created but ID is empty")
		return nil, fmt.Errorf("ledger created but ID is empty")
	}

	fmt.Printf("DEBUG: Ledger ID from API: %s\n", ledger.ID)
	return ledger, nil
}
