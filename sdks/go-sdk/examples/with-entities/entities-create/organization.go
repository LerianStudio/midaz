// Package main provides examples of creating resources using the Midaz Go SDK.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	midaz "github.com/LerianStudio/midaz/sdks/go-sdk"
	"github.com/LerianStudio/midaz/sdks/go-sdk/entities"
	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
)

// CreateOrganization demonstrates how to create an organization using the Entities interface.
//
// This function creates a new organization with the specified details using the entities package,
// which is the lower-level API in the SDK.
//
// Parameters:
//   - ctx: The context for the API request
//   - client: The Midaz client instance
//
// Returns:
//   - *models.Organization: The created organization
//   - error: An error if the operation fails
func CreateOrganization(ctx context.Context, client *midaz.Client) (*models.Organization, error) {
	// Create a direct entity instance for organizations
	fmt.Println("DEBUG: Creating organization with entities package...")

	// Create HTTP client with timeout
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Get auth token from client
	authToken := "midaz-auth-token-123456" // Using the same token from .env

	// Create base URLs map
	baseURLs := map[string]string{
		"onboarding": "http://127.0.0.1:3000/v1",
	}

	// Create organizations entity
	orgEntity := entities.NewOrganizationsEntity(httpClient, authToken, baseURLs)

	// Create organization input
	input := models.NewCreateOrganizationInput(
		"Upton, Grady and Rau",
		"N4HD0J58949",
	)

	// Set status
	statusDesc := "Organization created"
	input.Status = models.Status{
		Code:        "ACTIVE",
		Description: &statusDesc,
	}

	// Set address
	input.Address = models.NewAddress(
		"15609 Thad Ridges",
		"80503",
		"South Karina",
		"Montana",
		"CV",
	)

	// Add optional Line2 to address
	line2 := "Apt. 610"
	input.Address.Line2 = &line2

	// Set DoingBusinessAs
	dba := "Bogisich Inc"
	input.DoingBusinessAs = &dba

	// Print the input for debugging
	fmt.Println("DEBUG: Organization input configured with the following values:")
	fmt.Println("DEBUG: - Legal Name: Upton, Grady and Rau")
	fmt.Println("DEBUG: - Legal Document: N4HD0J58949")
	fmt.Println("DEBUG: - Status: ACTIVE")
	fmt.Println("DEBUG: - Doing Business As: Bogisich Inc")
	fmt.Println("DEBUG: - Address:")
	fmt.Println("DEBUG:   - Line1: 15609 Thad Ridges")
	fmt.Println("DEBUG:   - Line2: Apt. 610")
	fmt.Println("DEBUG:   - ZipCode: 80503")
	fmt.Println("DEBUG:   - City: South Karina")
	fmt.Println("DEBUG:   - State: Montana")
	fmt.Println("DEBUG:   - Country: CV")

	// Execute the create operation
	fmt.Println("DEBUG: Calling CreateOrganization on organizations entity...")
	org, err := orgEntity.CreateOrganization(ctx, input)
	if err != nil {
		fmt.Printf("DEBUG: Error creating organization: %v\n", err)
		return nil, fmt.Errorf("failed to create organization: %w", err)
	}

	// Print the organization details for debugging
	orgJSON, _ := json.MarshalIndent(org, "", "  ")
	fmt.Printf("DEBUG: Organization response: %s\n", string(orgJSON))

	// Check if we got a valid ID
	if org.ID == "" {
		fmt.Println("DEBUG: Warning - Organization created but ID is empty")
		fmt.Println("DEBUG: This suggests that the API request was successful but the response parsing failed")
		fmt.Println("DEBUG: or the API returned an empty response")
		return nil, fmt.Errorf("organization created but ID is empty")
	}

	fmt.Printf("DEBUG: Organization ID from API: %s\n", org.ID)
	return org, nil
}
