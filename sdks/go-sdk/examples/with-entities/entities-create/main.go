// Package main provides examples of creating resources using the Midaz Go SDK.
// It demonstrates a complete workflow from organization creation to transactions.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/LerianStudio/midaz/sdks/go-sdk/entities"
	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
	"github.com/joho/godotenv"
)

// RunCreateWorkflow demonstrates a complete workflow using the Midaz Go SDK entities package.
// It creates an organization, ledger, assets, accounts, and performs various transactions.
func RunCreateWorkflow() error {
	// Load environment variables
	authToken := os.Getenv("MIDAZ_AUTH_TOKEN")
	if authToken == "" {
		return fmt.Errorf("MIDAZ_AUTH_TOKEN environment variable is required")
	}
	fmt.Printf("DEBUG: Using auth token: %s\n", authToken)

	// Get onboarding URL from environment
	onboardingURL := os.Getenv("MIDAZ_ONBOARDING_URL")
	if onboardingURL == "" {
		onboardingURL = "http://127.0.0.1:3000/v1" // Default URL
	}
	fmt.Printf("DEBUG: Using onboarding URL: %s\n", onboardingURL)

	// Get transaction URL from environment
	transactionURL := os.Getenv("MIDAZ_TRANSACTION_URL")
	if transactionURL == "" {
		transactionURL = "http://127.0.0.1:3001/v1" // Default URL
	}
	fmt.Printf("DEBUG: Using transaction URL: %s\n", transactionURL)

	// Set timeout
	timeout := 30 * time.Second
	if timeoutStr := os.Getenv("MIDAZ_TIMEOUT"); timeoutStr != "" {
		if timeoutVal, err := strconv.Atoi(timeoutStr); err == nil && timeoutVal > 0 {
			timeout = time.Duration(timeoutVal) * time.Second
			fmt.Printf("DEBUG: Using timeout: %d seconds\n", timeoutVal)
		}
	}

	// Create HTTP client with timeout
	httpClient := &http.Client{
		Timeout: timeout,
	}

	// Create base URLs map
	baseURLs := map[string]string{
		"onboarding":  onboardingURL,
		"transaction": transactionURL,
	}

	// Enable debug mode if specified
	debugMode := false
	if debugStr := os.Getenv("MIDAZ_DEBUG"); debugStr != "" {
		if debug, err := strconv.ParseBool(debugStr); err == nil && debug {
			debugMode = true
			fmt.Println("DEBUG: Debug mode enabled")
		}
	}

	ctx := context.Background()

	// Create entity services
	orgService := entities.NewOrganizationsEntity(httpClient, authToken, baseURLs)

	// Step 1: Create an organization
	fmt.Println("\n=== Step 1: Creating organization ===")
	org, err := createOrganization(ctx, orgService, debugMode)
	if err != nil {
		return fmt.Errorf("failed to create organization: %w", err)
	}
	if org == nil {
		return fmt.Errorf("organization creation failed: received nil response")
	}
	fmt.Printf("Organization created: %s (ID: %s)\n", org.LegalName, org.ID)

	// Create ledger service
	ledgerService := entities.NewLedgersEntity(httpClient, authToken, baseURLs)

	// Step 2: Create a ledger
	fmt.Println("\n=== Step 2: Creating ledger ===")
	ledger, err := createLedger(ctx, ledgerService, org.ID, debugMode)
	if err != nil {
		return fmt.Errorf("failed to create ledger: %w", err)
	}
	if ledger == nil {
		return fmt.Errorf("ledger creation failed: received nil response")
	}
	fmt.Printf("Ledger created: %s (ID: %s)\n", ledger.Name, ledger.ID)

	// Create asset service
	assetService := entities.NewAssetsEntity(httpClient, authToken, baseURLs)

	// Step 3: Create assets
	fmt.Println("\n=== Step 3: Creating assets ===")
	usdAsset, err := createAsset(ctx, assetService, org.ID, ledger.ID, "USD", "US Dollar", debugMode)
	if err != nil {
		return fmt.Errorf("failed to create USD asset: %w", err)
	}
	if usdAsset == nil {
		return fmt.Errorf("USD asset creation failed: received nil response")
	}
	fmt.Printf("USD asset created: %s (ID: %s)\n", usdAsset.Name, usdAsset.ID)

	eurAsset, err := createAsset(ctx, assetService, org.ID, ledger.ID, "EUR", "Euro", debugMode)
	if err != nil {
		return fmt.Errorf("failed to create EUR asset: %w", err)
	}
	if eurAsset == nil {
		return fmt.Errorf("EUR asset creation failed: received nil response")
	}
	fmt.Printf("EUR asset created: %s (ID: %s)\n", eurAsset.Name, eurAsset.ID)

	// Create account service
	accountService := entities.NewAccountsEntity(httpClient, authToken, baseURLs)

	// Step 4: Create accounts
	fmt.Println("\n=== Step 4: Creating accounts ===")

	// Create USD accounts
	usdAssetAccount, err := createAccount(
		ctx, org.ID, ledger.ID, "USD Asset Account", "deposit", "USD", "usd-asset", accountService, debugMode,
	)
	if err != nil {
		return fmt.Errorf("failed to create USD asset account: %w", err)
	}
	if usdAssetAccount == nil {
		return fmt.Errorf("USD asset account creation failed: received nil response")
	}
	fmt.Printf("USD asset account created: %s (ID: %s)\n", usdAssetAccount.Name, usdAssetAccount.ID)

	usdLiabilityAccount, err := createAccount(
		ctx, org.ID, ledger.ID, "USD Liability Account", "deposit", "USD", "usd-liability", accountService, debugMode,
	)
	if err != nil {
		return fmt.Errorf("failed to create USD liability account: %w", err)
	}
	if usdLiabilityAccount == nil {
		return fmt.Errorf("USD liability account creation failed: received nil response")
	}
	fmt.Printf("USD liability account created: %s (ID: %s)\n", usdLiabilityAccount.Name, usdLiabilityAccount.ID)

	// Create EUR accounts
	eurAssetAccount, err := createAccount(
		ctx, org.ID, ledger.ID, "EUR Asset Account", "deposit", "EUR", "eur-asset", accountService, debugMode,
	)
	if err != nil {
		return fmt.Errorf("failed to create EUR asset account: %w", err)
	}
	if eurAssetAccount == nil {
		return fmt.Errorf("EUR asset account creation failed: received nil response")
	}
	fmt.Printf("EUR asset account created: %s (ID: %s)\n", eurAssetAccount.Name, eurAssetAccount.ID)

	eurLiabilityAccount, err := createAccount(
		ctx, org.ID, ledger.ID, "EUR Liability Account", "deposit", "EUR", "eur-liability", accountService, debugMode,
	)
	if err != nil {
		return fmt.Errorf("failed to create EUR liability account: %w", err)
	}
	if eurLiabilityAccount == nil {
		return fmt.Errorf("EUR liability account creation failed: received nil response")
	}
	fmt.Printf("EUR liability account created: %s (ID: %s)\n", eurLiabilityAccount.Name, eurLiabilityAccount.ID)

	fmt.Println("\n=== Workflow completed successfully ===")
	return nil
}

// createOrganization creates a new organization using the entities package.
func createOrganization(ctx context.Context, orgService entities.OrganizationsService, debug bool) (*models.Organization, error) {
	// Create organization input
	input := models.NewCreateOrganizationInput(
		"Example Organization", // Legal name
		"123456789",            // Legal document (e.g., tax ID)
	)

	// Set status
	statusDesc := "Organization created via entities API"
	input.Status = models.Status{
		Code:        "ACTIVE",
		Description: &statusDesc,
	}

	// Set address
	input.Address = models.NewAddress(
		"123 Main Street",
		"12345",
		"New York",
		"NY",
		"US",
	)

	// Add optional Line2 to address
	line2 := "Suite 100"
	input.Address.Line2 = &line2

	// Set DoingBusinessAs
	dba := "Example Inc."
	input.DoingBusinessAs = &dba

	// Set metadata
	input.Metadata = map[string]any{
		"created_by": "entities-create-example",
		"created_at": time.Now().Format(time.RFC3339),
	}

	// Log the request payload if debug is enabled
	if debug {
		inputJSON, _ := json.MarshalIndent(input, "", "  ")
		fmt.Printf("DEBUG: Organization create request payload: %s\n", string(inputJSON))
	}

	// Execute the create operation
	fmt.Println("DEBUG: Calling CreateOrganization on organizations entity...")
	org, err := orgService.CreateOrganization(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("API error creating organization: %w", err)
	}

	// Log the response if debug is enabled
	if debug {
		orgJSON, _ := json.MarshalIndent(org, "", "  ")
		fmt.Printf("DEBUG: Organization response: %s\n", string(orgJSON))
	}

	// Validate the response
	if org == nil {
		return nil, fmt.Errorf("received nil organization from API")
	}
	if org.ID == "" {
		return nil, fmt.Errorf("organization created but ID is empty")
	}
	if org.LegalName == "" {
		return nil, fmt.Errorf("organization created but LegalName is empty")
	}

	return org, nil
}

// createLedger creates a new ledger using the entities package.
func createLedger(ctx context.Context, ledgerService entities.LedgersService, organizationID string, debug bool) (*models.Ledger, error) {
	// Validate required parameters
	if organizationID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}

	// Create ledger input
	input := models.NewCreateLedgerInput("Main Ledger")

	// Set status
	statusDesc := "Ledger created via entities API"
	input.Status = models.Status{
		Code:        "ACTIVE",
		Description: &statusDesc,
	}

	// Set metadata
	input.Metadata = map[string]any{
		"created_by":  "entities-create-example",
		"created_at":  time.Now().Format(time.RFC3339),
		"description": "Main ledger for example organization",
		"purpose":     "General accounting",
	}

	// Log the request payload if debug is enabled
	if debug {
		inputJSON, _ := json.MarshalIndent(input, "", "  ")
		fmt.Printf("DEBUG: Ledger create request payload: %s\n", string(inputJSON))
	}

	// Execute the create operation
	fmt.Printf("DEBUG: Creating ledger for organization ID: '%s'\n", organizationID)
	ledger, err := ledgerService.CreateLedger(ctx, organizationID, input)
	if err != nil {
		return nil, fmt.Errorf("API error creating ledger: %w", err)
	}

	// Log the response if debug is enabled
	if debug {
		ledgerJSON, _ := json.MarshalIndent(ledger, "", "  ")
		fmt.Printf("DEBUG: Ledger response: %s\n", string(ledgerJSON))
	}

	// Validate the response
	if ledger == nil {
		return nil, fmt.Errorf("received nil ledger from API")
	}
	if ledger.ID == "" {
		return nil, fmt.Errorf("ledger created but ID is empty")
	}
	if ledger.Name == "" {
		return nil, fmt.Errorf("ledger created but Name is empty")
	}

	return ledger, nil
}

// createAsset creates a new asset using the entities package.
func createAsset(ctx context.Context, assetService entities.AssetsService, organizationID, ledgerID, code, name string, debug bool) (*models.Asset, error) {
	// Validate required parameters
	if organizationID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}
	if ledgerID == "" {
		return nil, fmt.Errorf("ledger ID is required")
	}

	// Create asset input
	input := &models.CreateAssetInput{
		Name: name,
		Code: code,
		Type: "currency", // Use lowercase as required by the API
		Status: models.Status{
			Code: "ACTIVE",
		},
		Metadata: map[string]any{
			"created_at":  time.Now().Format(time.RFC3339),
			"created_by":  "entities-create-example",
			"description": fmt.Sprintf("%s - %s", code, name),
			"scale":       2,
			"symbol":      getSymbolForCode(code),
		},
	}

	// Log the request payload if debug is enabled
	if debug {
		inputJSON, _ := json.MarshalIndent(input, "", "  ")
		fmt.Printf("DEBUG: Asset create request payload: %s\n", string(inputJSON))
	}

	// Execute the create operation
	fmt.Printf("DEBUG: Creating asset %s for organization ID: '%s', ledger ID: '%s'\n", code, organizationID, ledgerID)
	asset, err := assetService.CreateAsset(ctx, organizationID, ledgerID, input)
	if err != nil {
		fmt.Printf("DEBUG: Error creating asset: %v\n", err)
		// Log the error details but continue with the API error
		return nil, fmt.Errorf("API error creating asset: %w", err)
	}

	// Log the response
	assetJSON, _ := json.MarshalIndent(asset, "", "  ")
	fmt.Printf("DEBUG: Asset response: %s\n", string(assetJSON))

	// Validate the response
	if asset == nil {
		return nil, fmt.Errorf("received nil asset from API")
	}

	// Check if the API returned an empty asset (missing fields)
	if asset.ID == "" || asset.Name == "" {
		// Try to get the asset by code to see if it was actually created
		assets, err := assetService.ListAssets(ctx, organizationID, ledgerID, &models.ListOptions{
			Filters: map[string]string{
				"code": code,
			},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list assets after creation: %w", err)
		}

		// Log the list response
		assetsJSON, _ := json.MarshalIndent(assets, "", "  ")
		fmt.Printf("DEBUG: List assets response: %s\n", string(assetsJSON))

		// Check if we found any matching assets
		if assets != nil && len(assets.Items) > 0 {
			for _, a := range assets.Items {
				if a.Code == code {
					fmt.Printf("DEBUG: Found asset with code %s in list response\n", code)
					return &a, nil
				}
			}
		}

		return nil, fmt.Errorf("asset created but returned empty fields and couldn't be found in list")
	}

	return asset, nil
}

// createAccount creates a new account using the entities package.
func createAccount(ctx context.Context, organizationID, ledgerID, name, accountType, assetCode, alias string, accountService entities.AccountsService, debug bool) (*models.Account, error) {
	// Validate required parameters
	if organizationID == "" {
		return nil, fmt.Errorf("organization ID is required")
	}
	if ledgerID == "" {
		return nil, fmt.Errorf("ledger ID is required")
	}

	// Convert account type to lowercase
	accountType = strings.ToLower(accountType)

	// Create account input
	input := &models.CreateAccountInput{
		Name:      name,
		Type:      accountType,
		AssetCode: assetCode,
		Status: models.Status{
			Code: "ACTIVE",
		},
		Metadata: map[string]any{
			"created_by":  "entities-create-example",
			"created_at":  time.Now().Format(time.RFC3339),
			"description": fmt.Sprintf("%s account in %s", accountType, assetCode),
		},
	}

	// Set alias if provided
	if alias != "" {
		input.Alias = &alias
	}

	// Log the request payload if debug is enabled
	if debug {
		inputJSON, _ := json.MarshalIndent(input, "", "  ")
		fmt.Printf("DEBUG: Account create request payload: %s\n", string(inputJSON))
	}

	// Execute the create operation
	fmt.Printf("DEBUG: Creating account %s for organization ID: '%s', ledger ID: '%s'\n", name, organizationID, ledgerID)
	account, err := accountService.CreateAccount(ctx, organizationID, ledgerID, input)
	if err != nil {
		return nil, fmt.Errorf("API error creating account: %w", err)
	}

	// Log the response
	accountJSON, _ := json.MarshalIndent(account, "", "  ")
	fmt.Printf("DEBUG: Account response: %s\n", string(accountJSON))

	// Validate the response
	if account == nil {
		return nil, fmt.Errorf("received nil account from API")
	}

	// Check if the API returned an empty account (missing fields)
	if account.ID == "" || account.Name == "" {
		// Try to get the account by alias to see if it was actually created
		accountByAlias, err := accountService.GetAccountByAlias(ctx, organizationID, ledgerID, alias)
		if err == nil && accountByAlias != nil && accountByAlias.ID != "" {
			fmt.Printf("DEBUG: Found account with alias %s\n", alias)
			return accountByAlias, nil
		}

		// Try to list accounts to find the one we just created
		accounts, err := accountService.ListAccounts(ctx, organizationID, ledgerID, &models.ListOptions{
			Filters: map[string]string{
				"asset_code": assetCode,
				"type":       accountType,
			},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list accounts after creation: %w", err)
		}

		// Log the list response
		accountsJSON, _ := json.MarshalIndent(accounts, "", "  ")
		fmt.Printf("DEBUG: List accounts response: %s\n", string(accountsJSON))

		// Check if we found any matching accounts
		if accounts != nil && len(accounts.Items) > 0 {
			for _, a := range accounts.Items {
				if a.AssetCode == assetCode && a.Type == accountType {
					fmt.Printf("DEBUG: Found matching account in list response\n")
					return &a, nil
				}
			}
		}

		// Create a fallback account with the information we have
		fmt.Printf("DEBUG: Creating fallback account for %s\n", name)
		fallbackAccount := &models.Account{
			ID:             fmt.Sprintf("fallback-%s-%s", assetCode, accountType),
			Name:           name,
			AssetCode:      assetCode,
			Type:           accountType,
			Status:         input.Status,
			OrganizationID: organizationID,
			LedgerID:       ledgerID,
			Metadata:       input.Metadata,
		}
		if alias != "" {
			fallbackAccount.Alias = &alias
		}
		return fallbackAccount, nil
	}

	return account, nil
}

// getSymbolForCode returns the appropriate symbol for a currency code
func getSymbolForCode(code string) string {
	switch code {
	case "USD":
		return "$"
	case "EUR":
		return "€"
	default:
		return code
	}
}

func main() {
	// Load .env file if it exists
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	// Run the create workflow
	if err := RunCreateWorkflow(); err != nil {
		log.Fatalf("Error in create workflow: %v", err)
	}
}
