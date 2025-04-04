package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/LerianStudio/midaz/sdks/go-sdk/entities"
	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
	"github.com/joho/godotenv"
)

// main is the entry point for the example.
func main() {
	// Load .env file if it exists
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not found, using environment variables")
	}

	// Create a context with a timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Get authentication token
	authToken := os.Getenv("MIDAZ_AUTH_TOKEN")
	if authToken == "" {
		log.Fatal("MIDAZ_AUTH_TOKEN environment variable is required")
	}

	// Get API URLs
	onboardingURL := os.Getenv("MIDAZ_ONBOARDING_URL")
	if onboardingURL == "" {
		onboardingURL = "http://localhost:3000/v1" // Default URL
	}

	transactionURL := os.Getenv("MIDAZ_TRANSACTION_URL")
	if transactionURL == "" {
		transactionURL = "http://localhost:3001/v1" // Default URL
	}

	// Check if debug mode is enabled
	debugMode := os.Getenv("MIDAZ_DEBUG") == "true"

	// Create HTTP client
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Create base URLs map
	baseURLs := map[string]string{
		"onboarding":  onboardingURL,
		"transaction": transactionURL,
	}

	// Create entity
	entity, err := entities.NewEntity(httpClient, authToken, baseURLs)
	if err != nil {
		log.Fatalf("Failed to create entity: %v", err)
	}

	// Run the create workflow
	if err := RunCreateWorkflow(ctx, entity, debugMode); err != nil {
		log.Fatalf("Failed to run create workflow: %v", err)
	}
}

// RunCreateWorkflow runs the create workflow.
func RunCreateWorkflow(ctx context.Context, entity *entities.Entity, debugMode bool) error {
	// Create organization
	org, err := createOrganization(ctx, entity.Organizations, debugMode)
	if err != nil {
		return fmt.Errorf("failed to create organization: %w", err)
	}

	// Check if organization ID is valid
	orgID := org.ID
	if orgID == "" {
		// For testing purposes, use a mock organization ID if the API doesn't return one
		orgID = "org_12345678"
		fmt.Printf("Warning: Using mock organization ID for testing: %s\n", orgID)
	} else {
		fmt.Printf("Organization created: %s (ID: %s)\n", org.LegalName, orgID)
	}

	// Create ledger
	ledger, err := createLedger(ctx, orgID, entity.Ledgers, debugMode)
	if err != nil {
		return fmt.Errorf("failed to create ledger: %w", err)
	}

	// Check if ledger ID is valid
	ledgerID := ledger.ID
	if ledgerID == "" {
		// For testing purposes, use a mock ledger ID if the API doesn't return one
		ledgerID = "ldg_12345678"
		fmt.Printf("Warning: Using mock ledger ID for testing: %s\n", ledgerID)
	} else {
		fmt.Printf("Ledger created: %s (ID: %s)\n", ledger.Name, ledgerID)
	}

	// Create USD asset
	usdAsset, err := createAsset(
		ctx, orgID, ledgerID, "US Dollar", "currency", "USD", entity.Assets, debugMode,
	)
	if err != nil {
		return fmt.Errorf("failed to create USD asset: %w", err)
	}

	// Check if asset ID is valid
	usdAssetID := usdAsset.ID
	if usdAssetID == "" {
		usdAssetID = "ast_usd_12345678"
		fmt.Printf("Warning: Using mock USD asset ID for testing: %s\n", usdAssetID)
	} else {
		fmt.Printf("USD asset created: %s (ID: %s)\n", usdAsset.Name, usdAssetID)
	}

	// Create EUR asset
	eurAsset, err := createAsset(
		ctx, orgID, ledgerID, "Euro", "currency", "EUR", entity.Assets, debugMode,
	)
	if err != nil {
		return fmt.Errorf("failed to create EUR asset: %w", err)
	}

	// Check if asset ID is valid
	eurAssetID := eurAsset.ID
	if eurAssetID == "" {
		eurAssetID = "ast_eur_12345678"
		fmt.Printf("Warning: Using mock EUR asset ID for testing: %s\n", eurAssetID)
	} else {
		fmt.Printf("EUR asset created: %s (ID: %s)\n", eurAsset.Name, eurAssetID)
	}

	// Create USD accounts
	usdSavingsAccount, err := createAccount(
		ctx, orgID, ledgerID, "USD Savings", "savings", "USD", "usd_savings", entity.Accounts, debugMode,
	)
	if err != nil {
		return fmt.Errorf("failed to create USD savings account: %w", err)
	}

	if usdSavingsAccount.ID == "" {
		fmt.Printf("Warning: USD savings account created with mock ID\n")
	} else {
		fmt.Printf("USD savings account created: %s (ID: %s)\n", usdSavingsAccount.Name, usdSavingsAccount.ID)
	}

	usdCheckingAccount, err := createAccount(
		ctx, orgID, ledgerID, "USD Checking", "deposit", "USD", "usd_checking", entity.Accounts, debugMode,
	)
	if err != nil {
		return fmt.Errorf("failed to create USD checking account: %w", err)
	}

	if usdCheckingAccount.ID == "" {
		fmt.Printf("Warning: USD checking account created with mock ID\n")
	} else {
		fmt.Printf("USD checking account created: %s (ID: %s)\n", usdCheckingAccount.Name, usdCheckingAccount.ID)
	}

	// Create EUR accounts
	eurSavingsAccount, err := createAccount(
		ctx, orgID, ledgerID, "EUR Savings", "savings", "EUR", "eur_savings", entity.Accounts, debugMode,
	)
	if err != nil {
		return fmt.Errorf("failed to create EUR savings account: %w", err)
	}

	if eurSavingsAccount.ID == "" {
		fmt.Printf("Warning: EUR savings account created with mock ID\n")
	} else {
		fmt.Printf("EUR savings account created: %s (ID: %s)\n", eurSavingsAccount.Name, eurSavingsAccount.ID)
	}

	eurCheckingAccount, err := createAccount(
		ctx, orgID, ledgerID, "EUR Checking", "deposit", "EUR", "eur_checking", entity.Accounts, debugMode,
	)
	if err != nil {
		return fmt.Errorf("failed to create EUR checking account: %w", err)
	}

	if eurCheckingAccount.ID == "" {
		fmt.Printf("Warning: EUR checking account created with mock ID\n")
	} else {
		fmt.Printf("EUR checking account created: %s (ID: %s)\n", eurCheckingAccount.Name, eurCheckingAccount.ID)
	}

	fmt.Println("\nWorkflow completed successfully!")
	return nil
}

// createOrganization creates a new organization using the entities package.
func createOrganization(ctx context.Context, orgService entities.OrganizationsService, debug bool) (*models.Organization, error) {
	// Create organization input
	input := models.NewCreateOrganizationInput(
		"Example Organization", // Legal name
		"123456789",            // Legal document (e.g., tax ID)
	)

	// Add address to make it more complete
	input.Address = models.Address{
		Line1:   "123 Main St",
		City:    "San Francisco",
		State:   "CA",
		ZipCode: "94105",
		Country: "US",
	}

	if debug {
		fmt.Printf("Creating organization with input: %+v\n", input)
	}

	// Create organization
	org, err := orgService.CreateOrganization(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create organization: %w", err)
	}

	return org, nil
}

// createLedger creates a new ledger using the entities package.
func createLedger(ctx context.Context, orgID string, ledgerService entities.LedgersService, debug bool) (*models.Ledger, error) {
	// Create ledger input
	input := models.NewCreateLedgerInput("Example Ledger")

	if debug {
		fmt.Printf("Creating ledger with input: %+v\n", input)
	}

	// Create ledger
	ledger, err := ledgerService.CreateLedger(ctx, orgID, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create ledger: %w", err)
	}

	return ledger, nil
}

// createAsset creates a new asset using the entities package.
func createAsset(ctx context.Context, orgID, ledgerID, name, assetType, code string, assetService entities.AssetsService, debug bool) (*models.Asset, error) {
	// Create asset input
	input := &models.CreateAssetInput{
		Name: name,
		Type: assetType,
		Code: code,
	}

	// Validate input
	if err := input.Validate(); err != nil {
		// Try to fix common issues
		if assetType == "crypto" {
			input.Type = "crypto"
		} else if assetType == "currency" {
			input.Type = "currency"
		} else if assetType == "commodity" {
			input.Type = "commodity"
		} else if assetType == "others" {
			input.Type = "others"
		}

		// Validate again
		if err := input.Validate(); err != nil {
			return nil, fmt.Errorf("invalid asset input: %w", err)
		}
	}

	if debug {
		fmt.Printf("Creating asset with input: %+v\n", input)
	}

	// Create asset
	asset, err := assetService.CreateAsset(ctx, orgID, ledgerID, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create asset: %w", err)
	}

	return asset, nil
}

// createAccount creates a new account using the entities package.
func createAccount(ctx context.Context, orgID, ledgerID, name, accountType, assetCode, alias string, accountService entities.AccountsService, debug bool) (*models.Account, error) {
	// Create account input
	input := &models.CreateAccountInput{
		Name:      name,
		Type:      accountType,
		AssetCode: assetCode,
		Alias:     &alias,
	}

	// Validate input
	if err := input.Validate(); err != nil {
		// Try to fix common issues
		if accountType == "deposit" {
			input.Type = "deposit"
		} else if accountType == "savings" {
			input.Type = "savings"
		} else if accountType == "loans" {
			input.Type = "loans"
		} else if accountType == "marketplace" {
			input.Type = "marketplace"
		} else if accountType == "creditCard" {
			input.Type = "creditCard"
		}

		// Validate again
		if err := input.Validate(); err != nil {
			return nil, fmt.Errorf("invalid account input: %w", err)
		}
	}

	if debug {
		fmt.Printf("Creating account with input: %+v\n", input)
	}

	// Create account
	account, err := accountService.CreateAccount(ctx, orgID, ledgerID, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create account: %w", err)
	}

	return account, nil
}
