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

func main() {
	// Load environment variables from .env file
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: .env file not found or could not be loaded: %v", err)
	}

	// Create context
	ctx := context.Background()

	// Check if mock mode is enabled
	mockMode := os.Getenv("MIDAZ_MOCK_MODE") == "true"
	if mockMode {
		fmt.Println("🧪 Running in mock mode (no real API calls)")
	} else {
		fmt.Println("🔌 Connecting to real Midaz API")
	}

	// Get auth token
	authToken := os.Getenv("MIDAZ_AUTH_TOKEN")
	if authToken == "" {
		log.Fatalf("MIDAZ_AUTH_TOKEN environment variable is required")
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

	// Get timeout
	timeout := 30 * time.Second
	if timeoutStr := os.Getenv("MIDAZ_TIMEOUT"); timeoutStr != "" {
		if t, err := time.ParseDuration(timeoutStr + "s"); err == nil {
			timeout = t
		}
	}

	// Create HTTP client
	httpClient := &http.Client{
		Timeout: timeout,
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

	// Run the account transfer workflow
	fmt.Println("\n🚀 Starting account transfer workflow...")
	if err := RunAccountTransferWorkflow(ctx, entity, false, mockMode); err != nil {
		log.Fatalf("❌ Workflow failed: %v", err)
	}
	fmt.Println("\n🎉 Workflow completed successfully!")
}

// RunAccountTransferWorkflow runs the account transfer workflow.
func RunAccountTransferWorkflow(ctx context.Context, entity *entities.Entity, debugMode, mockMode bool) error {
	// Step 1: Create organization
	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Println("📋 STEP 1: ORGANIZATION CREATION")
	fmt.Println(strings.Repeat("=", 50))

	fmt.Println("Creating organization...")
	org, err := createOrganization(ctx, entity.Organizations, debugMode, mockMode)
	if err != nil {
		return fmt.Errorf("failed to create organization: %w", err)
	}

	orgID := org.ID
	if orgID == "" {
		return fmt.Errorf("organization created but no ID was returned from the API")
	}
	fmt.Printf("✅ Organization created: %s\n", org.LegalName)
	fmt.Printf("   ID: %s\n", orgID)
	fmt.Printf("   Created: %s\n", org.CreatedAt.Format("2006-01-02 15:04:05"))

	// Step 2: Create ledger
	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Println("📒 STEP 2: LEDGER CREATION")
	fmt.Println(strings.Repeat("=", 50))

	fmt.Println("Creating ledger...")
	ledger, err := createLedger(ctx, orgID, entity.Ledgers, debugMode, mockMode)
	if err != nil {
		return fmt.Errorf("failed to create ledger: %w", err)
	}

	ledgerID := ledger.ID
	if ledgerID == "" {
		return fmt.Errorf("ledger created but no ID was returned from the API")
	}
	fmt.Printf("✅ Ledger created: %s\n", ledger.Name)
	fmt.Printf("   ID: %s\n", ledgerID)
	fmt.Printf("   Created: %s\n", ledger.CreatedAt.Format("2006-01-02 15:04:05"))

	// Step 3: Create USD asset
	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Println("💰 STEP 3: ASSET CREATION")
	fmt.Println(strings.Repeat("=", 50))

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

	// Step 4: Create accounts
	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Println("🏦 STEP 4: ACCOUNT CREATION")
	fmt.Println(strings.Repeat("=", 50))

	// Create source account
	fmt.Println("Creating source account...")
	sourceAccount, err := createAccount(
		ctx, orgID, ledgerID, "Source Account", "deposit", "USD", "source_account", entity.Accounts, debugMode, mockMode,
	)
	if err != nil {
		return fmt.Errorf("failed to create source account: %w", err)
	}

	if sourceAccount.ID == "" {
		return fmt.Errorf("source account created but no ID was returned from the API")
	}
	fmt.Printf("✅ Source account created: %s\n", sourceAccount.Name)
	fmt.Printf("   Type: %s\n", sourceAccount.Type)
	fmt.Printf("   Asset: %s\n", sourceAccount.AssetCode)
	fmt.Printf("   ID: %s\n", sourceAccount.ID)

	// Create destination account
	fmt.Println("\nCreating destination account...")
	destAccount, err := createAccount(
		ctx, orgID, ledgerID, "Destination Account", "deposit", "USD", "dest_account", entity.Accounts, debugMode, mockMode,
	)
	if err != nil {
		return fmt.Errorf("failed to create destination account: %w", err)
	}

	if destAccount.ID == "" {
		return fmt.Errorf("destination account created but no ID was returned from the API")
	}
	fmt.Printf("✅ Destination account created: %s\n", destAccount.Name)
	fmt.Printf("   Type: %s\n", destAccount.Type)
	fmt.Printf("   Asset: %s\n", destAccount.AssetCode)
	fmt.Printf("   ID: %s\n", destAccount.ID)

	// Step 5: Perform account transfer
	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Println("💸 STEP 5: ACCOUNT TRANSFER")
	fmt.Println(strings.Repeat("=", 50))

	fmt.Println("Transferring funds between accounts...")

	// Create a transaction to transfer 100 USD from source to destination account
	transaction, err := transferFunds(
		ctx,
		entity,
		orgID,
		ledgerID,
		sourceAccount.ID,
		destAccount.ID,
		"USD",
		10000, // $100.00
		2,     // 2 decimal places
		"Transfer from source to destination",
		debugMode,
		mockMode,
	)
	if err != nil {
		return fmt.Errorf("failed to transfer funds: %w", err)
	}

	fmt.Printf("✅ Transfer completed successfully\n")
	fmt.Printf("   Transaction ID: %s\n", transaction.ID)
	fmt.Printf("   Amount: %d (scale: %d)\n", transaction.Amount, transaction.Scale)
	fmt.Printf("   Status: %s\n", transaction.Status.Code)
	fmt.Printf("   Created: %s\n", transaction.CreatedAt.Format("2006-01-02 15:04:05"))

	// Print workflow summary
	fmt.Println("\n" + strings.Repeat("=", 50))
	fmt.Println("📊 WORKFLOW SUMMARY")
	fmt.Println(strings.Repeat("=", 50))
	fmt.Printf("✅ Organization: %s (ID: %s)\n", org.LegalName, orgID)
	fmt.Printf("✅ Ledger: %s (ID: %s)\n", ledger.Name, ledgerID)
	fmt.Printf("✅ Asset: %s (%s)\n", usdAsset.Name, usdAsset.Code)
	fmt.Printf("✅ Source Account: %s (ID: %s)\n", sourceAccount.Name, sourceAccount.ID)
	fmt.Printf("✅ Destination Account: %s (ID: %s)\n", destAccount.Name, destAccount.ID)
	fmt.Printf("✅ Transfer: $100.00 USD from %s to %s\n", sourceAccount.Name, destAccount.Name)
	fmt.Printf("✅ Transaction ID: %s\n", transaction.ID)

	return nil
}

// transferFunds transfers funds between two accounts.
func transferFunds(
	ctx context.Context,
	entity *entities.Entity,
	orgID,
	ledgerID,
	sourceAccountID,
	destAccountID,
	assetCode string,
	amount,
	scale int64,
	description string,
	debugMode,
	mockMode bool,
) (*models.Transaction, error) {
	// Create a transaction DSL input
	dslInput := &models.TransactionDSLInput{
		Description: description,
		Code:        "TRANSFER",
		Metadata: map[string]any{
			"source":       "go-sdk-example",
			"transferType": "account-to-account",
		},
		Send: &models.DSLSend{
			Asset: assetCode,
			Value: amount,
			Scale: scale,
			Source: &models.DSLSource{
				From: []models.DSLFromTo{
					{
						Account: sourceAccountID,
					},
				},
			},
			Distribute: &models.DSLDistribute{
				To: []models.DSLFromTo{
					{
						Account: destAccountID,
					},
				},
			},
		},
	}

	// Validate the transaction DSL input
	if err := dslInput.Validate(); err != nil {
		return nil, fmt.Errorf("invalid transaction DSL input: %w", err)
	}

	// If in mock mode, return a mock transaction
	if mockMode {
		return &models.Transaction{
			ID:             "mock-transaction-id",
			Amount:         amount,
			Scale:          scale,
			AssetCode:      assetCode,
			Status:         models.Status{Code: models.TransactionStatusCompleted},
			LedgerID:       ledgerID,
			OrganizationID: orgID,
			Description:    description,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}, nil
	}

	// Create the transaction
	transaction, err := entity.Transactions.CreateTransactionWithDSL(ctx, orgID, ledgerID, dslInput)
	if err != nil {
		return nil, fmt.Errorf("failed to create transaction: %w", err)
	}

	return transaction, nil
}

// createOrganization creates a new organization.
func createOrganization(ctx context.Context, service entities.OrganizationsService, debugMode, mockMode bool) (*models.Organization, error) {
	// Create organization input
	input := &models.CreateOrganizationInput{
		LegalName:     "Schowalter, Bahringer and Heller",
		LegalDocument: "123456789",
		Address: models.Address{
			Line1:   "123 Main St",
			City:    "San Francisco",
			State:   "CA",
			ZipCode: "94105",
			Country: "US",
		},
	}

	// Validate the input
	if err := input.Validate(); err != nil {
		return nil, fmt.Errorf("invalid organization input: %w", err)
	}

	// If in mock mode, return a mock organization
	if mockMode {
		return &models.Organization{
			ID:            "mock-org-id",
			LegalName:     input.LegalName,
			LegalDocument: input.LegalDocument,
			Address:       input.Address,
			CreatedAt:     time.Now(),
			UpdatedAt:     time.Now(),
		}, nil
	}

	// Create the organization
	org, err := service.CreateOrganization(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create organization: %w", err)
	}

	return org, nil
}

// createLedger creates a new ledger.
func createLedger(ctx context.Context, orgID string, service entities.LedgersService, debugMode, mockMode bool) (*models.Ledger, error) {
	// Create ledger input
	input := &models.CreateLedgerInput{
		Name: "Example Ledger",
		Metadata: map[string]any{
			"description": "Ledger for account transfer example",
		},
	}

	// Validate the input
	if err := input.Validate(); err != nil {
		return nil, fmt.Errorf("invalid ledger input: %w", err)
	}

	// If in mock mode, return a mock ledger
	if mockMode {
		return &models.Ledger{
			ID:             "mock-ledger-id",
			Name:           input.Name,
			OrganizationID: orgID,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}, nil
	}

	// Create the ledger
	ledger, err := service.CreateLedger(ctx, orgID, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create ledger: %w", err)
	}

	return ledger, nil
}

// createAsset creates a new asset.
func createAsset(
	ctx context.Context,
	orgID, ledgerID, name, assetType, code string,
	service entities.AssetsService,
	debugMode, mockMode bool,
) (*models.Asset, error) {
	// Create asset input
	input := &models.CreateAssetInput{
		Name: name,
		Type: assetType,
		Code: code,
	}

	// Validate the input
	if err := input.Validate(); err != nil {
		return nil, fmt.Errorf("invalid asset input: %w", err)
	}

	// If in mock mode, return a mock asset
	if mockMode {
		return &models.Asset{
			ID:             "mock-asset-id",
			Name:           input.Name,
			Type:           input.Type,
			Code:           input.Code,
			OrganizationID: orgID,
			LedgerID:       ledgerID,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}, nil
	}

	// Create the asset
	asset, err := service.CreateAsset(ctx, orgID, ledgerID, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create asset: %w", err)
	}

	return asset, nil
}

// createAccount creates a new account.
func createAccount(
	ctx context.Context,
	orgID, ledgerID, name, accountType, assetCode, alias string,
	service entities.AccountsService,
	debugMode, mockMode bool,
) (*models.Account, error) {
	// Create account input
	input := &models.CreateAccountInput{
		Name:      name,
		Type:      accountType,
		AssetCode: assetCode,
		Alias:     &alias,
	}

	// Validate the input
	if err := input.Validate(); err != nil {
		return nil, fmt.Errorf("invalid account input: %w", err)
	}

	// If in mock mode, return a mock account
	if mockMode {
		return &models.Account{
			ID:             "mock-account-id",
			Name:           input.Name,
			Type:           input.Type,
			AssetCode:      input.AssetCode,
			Alias:          input.Alias, // Use the pointer that's already in the input
			OrganizationID: orgID,
			LedgerID:       ledgerID,
			CreatedAt:      time.Now(),
			UpdatedAt:      time.Now(),
		}, nil
	}

	// Create the account
	account, err := service.CreateAccount(ctx, orgID, ledgerID, input)
	if err != nil {
		return nil, fmt.Errorf("failed to create account: %w", err)
	}

	return account, nil
}
