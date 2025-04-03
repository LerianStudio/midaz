// Package main provides examples of creating resources using the Midaz Go SDK.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	midaz "github.com/LerianStudio/midaz/sdks/go-sdk"
	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
)

// CreateAsset demonstrates how to create an asset using direct HTTP requests.
//
// This function creates a new asset for the specified organization and ledger using direct HTTP requests
// to better understand the API's behavior and requirements.
//
// Parameters:
//   - ctx: The context for the API request
//   - client: The Midaz client instance
//   - organizationID: The ID of the organization that will own the asset
//   - ledgerID: The ID of the ledger that will contain the asset
//   - code: The unique code for the asset (e.g., "USD", "EUR")
//   - name: The human-readable name for the asset
//
// Returns:
//   - *models.Asset: The created asset
//   - error: An error if the operation fails
func CreateAsset(
	ctx context.Context,
	client *midaz.Client,
	organizationID, ledgerID, code, name string,
) (*models.Asset, error) {
	// Debug information
	fmt.Printf("DEBUG: Creating asset %s for organization ID: '%s', ledger ID: '%s'\n",
		code, organizationID, ledgerID)

	// Validate required parameters
	if organizationID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	if ledgerID == "" {
		return nil, fmt.Errorf("ledger ID is required")
	}

	// Create a direct HTTP request for better debugging
	fmt.Println("DEBUG: Creating asset with direct HTTP request for detailed debugging...")

	// Create the request body - using a simplified structure based on API requirements
	reqBody := map[string]interface{}{
		"name": name,
		"code": code,
		"type": "CURRENCY",
		"status": map[string]string{
			"code": "ACTIVE",
		},
		"metadata": map[string]interface{}{
			"symbol":      "$",
			"scale":       2,
			"description": fmt.Sprintf("%s - %s", code, name),
			"type":        "currency",
		},
	}

	// Convert the request body to JSON
	reqBodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	// Create the HTTP request URL
	url := fmt.Sprintf("http://127.0.0.1:3000/v1/organizations/%s/ledgers/%s/assets",
		organizationID, ledgerID)

	// Create the HTTP request
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		url,
		bytes.NewBuffer(reqBodyBytes),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", "midaz-auth-token-123456"))

	// Print the request for debugging
	fmt.Println("DEBUG: HTTP Request:")
	fmt.Printf("DEBUG: - Method: %s\n", req.Method)
	fmt.Printf("DEBUG: - URL: %s\n", req.URL.String())
	fmt.Println("DEBUG: - Headers:")
	for k, v := range req.Header {
		fmt.Printf("DEBUG:   - %s: %s\n", k, strings.Join(v, ", "))
	}
	fmt.Printf("DEBUG: - Body: %s\n", string(reqBodyBytes))

	// Make the HTTP request
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make HTTP request: %w", err)
	}
	defer resp.Body.Close()

	// Read the response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Print the response for debugging
	fmt.Println("DEBUG: HTTP Response:")
	fmt.Printf("DEBUG: - Status: %s\n", resp.Status)
	fmt.Println("DEBUG: - Headers:")
	for k, v := range resp.Header {
		fmt.Printf("DEBUG:   - %s: %s\n", k, strings.Join(v, ", "))
	}
	fmt.Printf("DEBUG: - Body: %s\n", string(respBody))

	// Check if the response status is successful
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		fmt.Printf("DEBUG: Error response status: %d\n", resp.StatusCode)
		return nil, fmt.Errorf("API returned error status: %s", resp.Status)
	}

	// Parse the response body
	var asset models.Asset
	if err := json.Unmarshal(respBody, &asset); err != nil {
		fmt.Printf("DEBUG: Error parsing response: %v\n", err)
		return nil, fmt.Errorf("failed to unmarshal response body: %w", err)
	}

	// Check if we got a valid ID
	if asset.ID == "" {
		fmt.Println("DEBUG: Warning - Asset created but ID is empty")
		fmt.Println("DEBUG: This suggests that the API request was successful but the response format is unexpected")

		// Try to extract the ID from the raw response if possible
		var rawResp map[string]interface{}
		if err := json.Unmarshal(respBody, &rawResp); err == nil {
			if id, ok := rawResp["id"].(string); ok && id != "" {
				fmt.Printf("DEBUG: Found ID in raw response: %s\n", id)
				asset.ID = id
				asset.OrganizationID = organizationID
				asset.LedgerID = ledgerID
				asset.Name = name
				asset.Code = code
				asset.Type = "CURRENCY"
				asset.Status.Code = "ACTIVE"
			}
		}

		if asset.ID == "" {
			return nil, fmt.Errorf("asset created but ID is empty")
		}
	}

	// Set the organization ID and ledger ID if they're not already set
	if asset.OrganizationID == "" {
		asset.OrganizationID = organizationID
	}
	if asset.LedgerID == "" {
		asset.LedgerID = ledgerID
	}

	fmt.Printf("DEBUG: Asset ID from API: %s\n", asset.ID)
	return &asset, nil
}
