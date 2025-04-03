// Package main provides examples of creating resources using the Midaz Go SDK.
package main

import (
	"context"
	"fmt"

	midaz "github.com/LerianStudio/midaz/sdks/go-sdk"
	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
)

// CreateOrganization demonstrates how to create an organization using the Builder interface.
//
// This function creates a new organization with the specified details using the builder pattern.
// It shows how to set required fields like legal name and legal document, as well as optional
// fields like address, status, and metadata.
//
// Parameters:
//   - ctx: The context for the API request
//   - client: The Midaz client instance
//
// Returns:
//   - *models.Organization: The created organization
//   - error: An error if the operation fails
func CreateOrganization(ctx context.Context, client *midaz.Client) (*models.Organization, error) {
	// Create a new organization using the Builder interface
	org, err := client.Builder.NewOrganization().
		WithLegalName("Example Corporation").
		WithLegalDocument("123456789").
		WithStatus("ACTIVE").
		WithAddress(
			"123 Main Street",
			"94105",
			"San Francisco",
			"CA",
			"USA",
		).
		WithMetadata(map[string]any{
			"industry": "Technology",
			"size":     "Enterprise",
			"public":   false,
		}).
		WithTags([]string{"example", "demo", "test"}).
		Create(ctx)

	if err != nil {
		return nil, fmt.Errorf("failed to create organization: %w", err)
	}

	return org, nil
}
