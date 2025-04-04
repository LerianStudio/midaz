package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
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
		fmt.Println("🔄 Running in mock mode - no actual API calls will be made")
	} else {
		fmt.Println("🔌 Connecting to real Midaz API")
	}

	// Get authentication token
	authToken := os.Getenv("MIDAZ_AUTH_TOKEN")
	if authToken == "" {
		log.Fatal("MIDAZ_AUTH_TOKEN environment variable is required")
	}

	// Get API URLs
	onboardingURL := os.Getenv("MIDAZ_ONBOARDING_URL")
	if onboardingURL == "" {
		onboardingURL = "http://127.0.0.1:3000/v1" // Default URL
	}
	fmt.Printf("📡 Using onboarding URL: %s\n", onboardingURL)

	transactionURL := os.Getenv("MIDAZ_TRANSACTION_URL")
	if transactionURL == "" {
		transactionURL = "http://127.0.0.1:3001/v1" // Default URL
	}
	fmt.Printf("📡 Using transaction URL: %s\n", transactionURL)

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
	fmt.Println("\n🔑 Initializing SDK client...")
	entity, err := entities.NewEntity(httpClient, authToken, baseURLs)
	if err != nil {
		log.Fatalf("Failed to create entity: %v", err)
	}
	fmt.Println("✅ SDK client initialized successfully")

	// Run the create workflow
	fmt.Println("\n🚀 Starting create workflow...")
	if err := RunCreateWorkflow(ctx, entity, false, mockMode); err != nil {
		log.Fatalf("❌ Workflow failed: %v", err)
	}
	fmt.Println("\n🎉 Workflow completed successfully!")
}

// RunCreateWorkflow runs the create workflow.
func RunCreateWorkflow(ctx context.Context, entity *entities.Entity, debugMode, mockMode bool) error {
	// Print workflow header
	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Println("📋 STEP 1: ORGANIZATION CREATION")
	fmt.Println(strings.Repeat("=", 50))

	// Create organization
	fmt.Println("Creating organization...")
	org, err := createOrganization(ctx, entity.Organizations, debugMode, mockMode)
	if err != nil {
		return fmt.Errorf("failed to create organization: %w", err)
	}

	// Get organization ID
	orgID := org.ID
	if orgID == "" {
		return fmt.Errorf("organization created but no ID was returned from the API")
	}
	fmt.Printf("✅ Organization created: %s\n", org.LegalName)
	fmt.Printf("   ID: %s\n", orgID)
	fmt.Printf("   Created: %s\n", org.CreatedAt.Format("2006-01-02 15:04:05"))

	// Print workflow header
	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Println("📒 STEP 2: LEDGER CREATION")
	fmt.Println(strings.Repeat("=", 50))

	// Create ledger
	fmt.Println("Creating ledger...")
	ledger, err := createLedger(ctx, orgID, entity.Ledgers, debugMode, mockMode)
	if err != nil {
		return fmt.Errorf("failed to create ledger: %w", err)
	}

	// Get ledger ID
	ledgerID := ledger.ID
	if ledgerID == "" {
		return fmt.Errorf("ledger created but no ID was returned from the API")
	}
	fmt.Printf("✅ Ledger created: %s\n", ledger.Name)
	fmt.Printf("   ID: %s\n", ledgerID)
	fmt.Printf("   Created: %s\n", ledger.CreatedAt.Format("2006-01-02 15:04:05"))

	// Print workflow header
	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Println("💰 STEP 3: ASSET CREATION")
	fmt.Println(strings.Repeat("=", 50))

	// Create USD asset
	fmt.Println("Creating USD asset...")
	usdAsset, err := createAsset(
		ctx, orgID, ledgerID, "US Dollar", "currency", "USD", entity.Assets, debugMode, mockMode,
	)
	if err != nil {
		return fmt.Errorf("failed to create USD asset: %w", err)
	}

	if usdAsset.ID == "" {
		return fmt.Errorf("USD asset created but no ID was returned from the API")
	}
	fmt.Printf("✅ USD asset created: %s\n", usdAsset.Name)
	fmt.Printf("   Code: %s\n", usdAsset.Code)
	fmt.Printf("   Type: %s\n", usdAsset.Type)
	fmt.Printf("   ID: %s\n", usdAsset.ID)

	// Create EUR asset
	fmt.Println("\nCreating EUR asset...")
	eurAsset, err := createAsset(
		ctx, orgID, ledgerID, "Euro", "currency", "EUR", entity.Assets, debugMode, mockMode,
	)
	if err != nil {
		return fmt.Errorf("failed to create EUR asset: %w", err)
	}

	if eurAsset.ID == "" {
		return fmt.Errorf("EUR asset created but no ID was returned from the API")
	}
	fmt.Printf("✅ EUR asset created: %s\n", eurAsset.Name)
	fmt.Printf("   Code: %s\n", eurAsset.Code)
	fmt.Printf("   Type: %s\n", eurAsset.Type)
	fmt.Printf("   ID: %s\n", eurAsset.ID)

	// Print workflow header
	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Println("🏦 STEP 4: ACCOUNT CREATION")
	fmt.Println(strings.Repeat("=", 50))

	// Create USD accounts
	fmt.Println("Creating USD savings account...")
	usdSavingsAccount, err := createAccount(
		ctx, orgID, ledgerID, "USD Savings", "savings", "USD", "usd_savings", entity.Accounts, debugMode, mockMode,
	)
	if err != nil {
		return fmt.Errorf("failed to create USD savings account: %w", err)
	}

	if usdSavingsAccount.ID == "" {
		return fmt.Errorf("USD savings account created but no ID was returned from the API")
	}
	fmt.Printf("✅ USD savings account created: %s\n", usdSavingsAccount.Name)
	fmt.Printf("   Type: %s\n", usdSavingsAccount.Type)
	fmt.Printf("   Asset: %s\n", usdSavingsAccount.AssetCode)
	fmt.Printf("   ID: %s\n", usdSavingsAccount.ID)

	fmt.Println("\nCreating USD checking account...")
	usdCheckingAccount, err := createAccount(
		ctx, orgID, ledgerID, "USD Checking", "deposit", "USD", "usd_checking", entity.Accounts, debugMode, mockMode,
	)
	if err != nil {
		return fmt.Errorf("failed to create USD checking account: %w", err)
	}

	if usdCheckingAccount.ID == "" {
		return fmt.Errorf("USD checking account created but no ID was returned from the API")
	}
	fmt.Printf("✅ USD checking account created: %s\n", usdCheckingAccount.Name)
	fmt.Printf("   Type: %s\n", usdCheckingAccount.Type)
	fmt.Printf("   Asset: %s\n", usdCheckingAccount.AssetCode)
	fmt.Printf("   ID: %s\n", usdCheckingAccount.ID)

	// Create EUR accounts
	fmt.Println("\nCreating EUR savings account...")
	eurSavingsAccount, err := createAccount(
		ctx, orgID, ledgerID, "EUR Savings", "savings", "EUR", "eur_savings", entity.Accounts, debugMode, mockMode,
	)
	if err != nil {
		return fmt.Errorf("failed to create EUR savings account: %w", err)
	}

	if eurSavingsAccount.ID == "" {
		return fmt.Errorf("EUR savings account created but no ID was returned from the API")
	}
	fmt.Printf("✅ EUR savings account created: %s\n", eurSavingsAccount.Name)
	fmt.Printf("   Type: %s\n", eurSavingsAccount.Type)
	fmt.Printf("   Asset: %s\n", eurSavingsAccount.AssetCode)
	fmt.Printf("   ID: %s\n", eurSavingsAccount.ID)

	fmt.Println("\nCreating EUR checking account...")
	eurCheckingAccount, err := createAccount(
		ctx, orgID, ledgerID, "EUR Checking", "deposit", "EUR", "eur_checking", entity.Accounts, debugMode, mockMode,
	)
	if err != nil {
		return fmt.Errorf("failed to create EUR checking account: %w", err)
	}

	if eurCheckingAccount.ID == "" {
		return fmt.Errorf("EUR checking account created but no ID was returned from the API")
	}
	fmt.Printf("✅ EUR checking account created: %s\n", eurCheckingAccount.Name)
	fmt.Printf("   Type: %s\n", eurCheckingAccount.Type)
	fmt.Printf("   Asset: %s\n", eurCheckingAccount.AssetCode)
	fmt.Printf("   ID: %s\n", eurCheckingAccount.ID)

	// Print workflow summary
	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Println("📊 WORKFLOW SUMMARY")
	fmt.Println(strings.Repeat("=", 50))
	fmt.Printf("✅ Organization: %s (ID: %s)\n", org.LegalName, orgID)
	fmt.Printf("✅ Ledger: %s (ID: %s)\n", ledger.Name, ledgerID)
	fmt.Printf("✅ Assets: USD, EUR\n")
	fmt.Printf("✅ Accounts: 4 accounts created (2 USD, 2 EUR)\n")

	return nil
}

// createOrganization creates a new organization using the entities package.
func createOrganization(ctx context.Context, orgService entities.OrganizationsService, debug, mockMode bool) (*models.Organization, error) {
	if mockMode {
		return &models.Organization{
			ID:        "mock-organization-id",
			LegalName: "Mock Organization",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}, nil
	}

	// Create organization input
	input := models.NewCreateOrganizationInput(
		"Schowalter, Bahringer and Heller", // Legal name
		"WXCSPM83ETD7",                     // Legal document (e.g., tax ID)
	)

	// Add doing business as
	doingBusinessAs := "Keeling - Renner"
	input.DoingBusinessAs = &doingBusinessAs

	// Add address to match the successful curl request
	input.Address = models.Address{
		Line1:   "1070 Ericka Parkway",
		Line2:   func() *string { s := "Apt. 664"; return &s }(),
		City:    "Fort Virgiefurt",
		State:   "Oklahoma",
		ZipCode: "48925",
		Country: "BG",
	}

	// Add status
	input.Status = models.Status{
		Code: "ACTIVE",
		Description: func() *string {
			desc := "Organization created"
			return &desc
		}(),
	}

	// Create organization
	org, err := orgService.CreateOrganization(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create organization: %w", err)
	}

	return org, nil
}

// createLedger creates a new ledger using the entities package.
func createLedger(ctx context.Context, orgID string, ledgerService entities.LedgersService, debug, mockMode bool) (*models.Ledger, error) {
	if mockMode {
		return &models.Ledger{
			ID:        "mock-ledger-id",
			Name:      "Example Ledger",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}, nil
	}

	// Create ledger input
	input := models.NewCreateLedgerInput("Example Ledger")

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
			ID:        "mock-asset-id",
			Name:      name,
			Code:      code,
			Type:      assetType,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
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
			ID:        "mock-account-id",
			Name:      name,
			Type:      accountType,
			AssetCode: assetCode,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
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

	// Create account
	account, err := accountService.CreateAccount(ctx, orgID, ledgerID, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create account: %w", err)
	}

	return account, nil
}
