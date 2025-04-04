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

	// Check if mock mode is enabled
	mockMode := os.Getenv("MIDAZ_MOCK_MODE") == "true"
	if mockMode {
		log.Println("Running in mock mode - no actual API calls will be made")
	}

	// Get authentication token
	authToken := os.Getenv("MIDAZ_AUTH_TOKEN")
	if authToken == "" {
		log.Fatal("MIDAZ_AUTH_TOKEN environment variable is required")
	}

	// Print token info (first 10 chars only for security)
	tokenPreview := authToken
	if len(authToken) > 10 {
		tokenPreview = authToken[:10] + "..."
	}
	log.Printf("Using auth token: %s", tokenPreview)

	// Get API URLs
	onboardingURL := os.Getenv("MIDAZ_ONBOARDING_URL")
	if onboardingURL == "" {
		onboardingURL = "http://localhost:3000/v1" // Default URL
	}
	log.Printf("Using onboarding URL: %s", onboardingURL)

	transactionURL := os.Getenv("MIDAZ_TRANSACTION_URL")
	if transactionURL == "" {
		transactionURL = "http://localhost:3001/v1" // Default URL
	}
	log.Printf("Using transaction URL: %s", transactionURL)

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
	log.Println("Creating entity client...")
	entity, err := entities.NewEntity(httpClient, authToken, baseURLs)
	if err != nil {
		log.Fatalf("Failed to create entity: %v", err)
	}
	log.Println("Entity client created successfully")

	// Run the create workflow
	log.Println("Starting create workflow...")
	if err := RunCreateWorkflow(ctx, entity, debugMode, mockMode); err != nil {
		log.Fatalf("Failed to run create workflow: %v", err)
	}
}

// RunCreateWorkflow runs the create workflow.
func RunCreateWorkflow(ctx context.Context, entity *entities.Entity, debugMode, mockMode bool) error {
	// Create organization
	org, err := createOrganization(ctx, entity.Organizations, debugMode, mockMode)
	if err != nil {
		return fmt.Errorf("failed to create organization: %w", err)
	}

	// Get organization ID
	orgID := org.ID
	if orgID == "" {
		return fmt.Errorf("organization created but no ID was returned from the API")
	}
	fmt.Printf("Organization created: %s (ID: %s)\n", org.LegalName, orgID)

	// Create ledger
	ledger, err := createLedger(ctx, orgID, entity.Ledgers, debugMode, mockMode)
	if err != nil {
		return fmt.Errorf("failed to create ledger: %w", err)
	}

	// Get ledger ID
	ledgerID := ledger.ID
	if ledgerID == "" {
		return fmt.Errorf("ledger created but no ID was returned from the API")
	}
	fmt.Printf("Ledger created: %s (ID: %s)\n", ledger.Name, ledgerID)

	// Create USD asset
	usdAsset, err := createAsset(
		ctx, orgID, ledgerID, "US Dollar", "currency", "USD", entity.Assets, debugMode, mockMode,
	)
	if err != nil {
		return fmt.Errorf("failed to create USD asset: %w", err)
	}

	if usdAsset.ID == "" {
		return fmt.Errorf("USD asset created but no ID was returned from the API")
	}
	fmt.Printf("USD asset created: %s (ID: %s)\n", usdAsset.Name, usdAsset.ID)

	// Create EUR asset
	eurAsset, err := createAsset(
		ctx, orgID, ledgerID, "Euro", "currency", "EUR", entity.Assets, debugMode, mockMode,
	)
	if err != nil {
		return fmt.Errorf("failed to create EUR asset: %w", err)
	}

	if eurAsset.ID == "" {
		return fmt.Errorf("EUR asset created but no ID was returned from the API")
	}
	fmt.Printf("EUR asset created: %s (ID: %s)\n", eurAsset.Name, eurAsset.ID)

	// Create USD accounts
	usdSavingsAccount, err := createAccount(
		ctx, orgID, ledgerID, "USD Savings", "savings", "USD", "usd_savings", entity.Accounts, debugMode, mockMode,
	)
	if err != nil {
		return fmt.Errorf("failed to create USD savings account: %w", err)
	}

	if usdSavingsAccount.ID == "" {
		return fmt.Errorf("USD savings account created but no ID was returned from the API")
	}
	fmt.Printf("USD savings account created: %s (ID: %s)\n", usdSavingsAccount.Name, usdSavingsAccount.ID)

	usdCheckingAccount, err := createAccount(
		ctx, orgID, ledgerID, "USD Checking", "deposit", "USD", "usd_checking", entity.Accounts, debugMode, mockMode,
	)
	if err != nil {
		return fmt.Errorf("failed to create USD checking account: %w", err)
	}

	if usdCheckingAccount.ID == "" {
		return fmt.Errorf("USD checking account created but no ID was returned from the API")
	}
	fmt.Printf("USD checking account created: %s (ID: %s)\n", usdCheckingAccount.Name, usdCheckingAccount.ID)

	// Create EUR accounts
	eurSavingsAccount, err := createAccount(
		ctx, orgID, ledgerID, "EUR Savings", "savings", "EUR", "eur_savings", entity.Accounts, debugMode, mockMode,
	)
	if err != nil {
		return fmt.Errorf("failed to create EUR savings account: %w", err)
	}

	if eurSavingsAccount.ID == "" {
		return fmt.Errorf("EUR savings account created but no ID was returned from the API")
	}
	fmt.Printf("EUR savings account created: %s (ID: %s)\n", eurSavingsAccount.Name, eurSavingsAccount.ID)

	eurCheckingAccount, err := createAccount(
		ctx, orgID, ledgerID, "EUR Checking", "deposit", "EUR", "eur_checking", entity.Accounts, debugMode, mockMode,
	)
	if err != nil {
		return fmt.Errorf("failed to create EUR checking account: %w", err)
	}

	if eurCheckingAccount.ID == "" {
		return fmt.Errorf("EUR checking account created but no ID was returned from the API")
	}
	fmt.Printf("EUR checking account created: %s (ID: %s)\n", eurCheckingAccount.Name, eurCheckingAccount.ID)

	fmt.Println("\nWorkflow completed successfully!")
	return nil
}

// createOrganization creates a new organization using the entities package.
func createOrganization(ctx context.Context, orgService entities.OrganizationsService, debug, mockMode bool) (*models.Organization, error) {
	if mockMode {
		return &models.Organization{
			ID: "mock-organization-id",
		}, nil
	}

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
	log.Println("Calling API to create organization...")
	org, err := orgService.CreateOrganization(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create organization: %w", err)
	}

	log.Printf("API response for organization: %+v\n", org)
	return org, nil
}

// createLedger creates a new ledger using the entities package.
func createLedger(ctx context.Context, orgID string, ledgerService entities.LedgersService, debug, mockMode bool) (*models.Ledger, error) {
	if mockMode {
		return &models.Ledger{
			ID: "mock-ledger-id",
		}, nil
	}

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
func createAsset(ctx context.Context, orgID, ledgerID, name, assetType, code string, assetService entities.AssetsService, debug, mockMode bool) (*models.Asset, error) {
	if mockMode {
		return &models.Asset{
			ID: "mock-asset-id",
		}, nil
	}

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
func createAccount(ctx context.Context, orgID, ledgerID, name, accountType, assetCode, alias string, accountService entities.AccountsService, debug, mockMode bool) (*models.Account, error) {
	if mockMode {
		return &models.Account{
			ID: "mock-account-id",
		}, nil
	}

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
