// Package main provides examples of creating resources using the Midaz Go SDK.
// It demonstrates a complete workflow from organization creation to transactions.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	midaz "github.com/LerianStudio/midaz/sdks/go-sdk"
	"github.com/joho/godotenv"
)

// RunCreateWorkflow demonstrates a complete workflow using the Midaz Go SDK.
// It creates an organization, ledger, assets, accounts, and performs various transactions.
func RunCreateWorkflow() error {
	// Load environment variables
	options := []midaz.Option{}

	// Add auth token from environment
	authToken := os.Getenv("MIDAZ_AUTH_TOKEN")
	if authToken != "" {
		fmt.Printf("DEBUG: Using auth token: %s\n", authToken)
		options = append(options, midaz.WithAuthToken(authToken))
	} else {
		return fmt.Errorf("MIDAZ_AUTH_TOKEN environment variable is required")
	}

	// Add onboarding URL if specified
	if onboardingURL := os.Getenv("MIDAZ_ONBOARDING_URL"); onboardingURL != "" {
		fmt.Printf("DEBUG: Using onboarding URL: %s\n", onboardingURL)
		options = append(options, midaz.WithOnboardingURL(onboardingURL))
	}

	// Add transaction URL if specified
	if transactionURL := os.Getenv("MIDAZ_TRANSACTION_URL"); transactionURL != "" {
		fmt.Printf("DEBUG: Using transaction URL: %s\n", transactionURL)
		options = append(options, midaz.WithTransactionURL(transactionURL))
	}

	// Add debug mode if specified
	if debugStr := os.Getenv("MIDAZ_DEBUG"); debugStr != "" {
		debug, err := strconv.ParseBool(debugStr)
		if err == nil && debug {
			fmt.Println("DEBUG: Debug mode enabled")
			options = append(options, midaz.WithDebug(true))
		}
	}

	// Add timeout if specified
	if timeoutStr := os.Getenv("MIDAZ_TIMEOUT"); timeoutStr != "" {
		timeout, err := strconv.Atoi(timeoutStr)
		if err == nil && timeout > 0 {
			fmt.Printf("DEBUG: Using timeout: %d seconds\n", timeout)
			options = append(options, midaz.WithTimeout(time.Duration(timeout)*time.Second))
		}
	}

	// Enable all API interfaces
	options = append(options, midaz.UseAllAPIs())
	fmt.Println("DEBUG: Enabled all API interfaces")

	// Create a new Midaz client with options from environment
	client, err := midaz.New(options...)
	if err != nil {
		return fmt.Errorf("failed to create Midaz client: %w", err)
	}

	ctx := context.Background()

	// Step 1: Create an organization
	fmt.Println("\n=== Step 1: Creating organization ===")
	org, err := CreateOrganization(ctx, client)
	if err != nil {
		return fmt.Errorf("failed to create organization: %w", err)
	}
	fmt.Printf("Organization created: %s (ID: %s)\n", org.LegalName, org.ID)

	// Step 2: Create a ledger
	fmt.Println("\n=== Step 2: Creating ledger ===")
	ledger, err := CreateLedger(ctx, client, org.ID)
	if err != nil {
		return fmt.Errorf("failed to create ledger: %w", err)
	}
	fmt.Printf("Ledger created: %s (ID: %s)\n", ledger.Name, ledger.ID)

	// Step 3: Create assets
	fmt.Println("\n=== Step 3: Creating assets ===")
	usdAsset, err := CreateAsset(ctx, client, org.ID, ledger.ID, "USD", "US Dollar")
	if err != nil {
		return fmt.Errorf("failed to create USD asset: %w", err)
	}
	fmt.Printf("USD asset created: %s (ID: %s)\n", usdAsset.Name, usdAsset.ID)

	eurAsset, err := CreateAsset(ctx, client, org.ID, ledger.ID, "EUR", "Euro")
	if err != nil {
		return fmt.Errorf("failed to create EUR asset: %w", err)
	}
	fmt.Printf("EUR asset created: %s (ID: %s)\n", eurAsset.Name, eurAsset.ID)

	// Step 4: Create accounts
	fmt.Println("\n=== Step 4: Creating accounts ===")

	// Create USD accounts
	usdAssetAccount, err := CreateAccount(
		ctx, client, org.ID, ledger.ID,
		"USD Asset Account", "USD", "ASSET", "usd-asset",
	)
	if err != nil {
		return fmt.Errorf("failed to create USD asset account: %w", err)
	}
	fmt.Printf("USD asset account created: %s (ID: %s)\n", usdAssetAccount.Name, usdAssetAccount.ID)

	usdLiabilityAccount, err := CreateAccount(
		ctx, client, org.ID, ledger.ID,
		"USD Liability Account", "USD", "LIABILITY", "usd-liability",
	)
	if err != nil {
		return fmt.Errorf("failed to create USD liability account: %w", err)
	}
	fmt.Printf("USD liability account created: %s (ID: %s)\n", usdLiabilityAccount.Name, usdLiabilityAccount.ID)

	// Create EUR accounts
	eurAssetAccount, err := CreateAccount(
		ctx, client, org.ID, ledger.ID,
		"EUR Asset Account", "EUR", "ASSET", "eur-asset",
	)
	if err != nil {
		return fmt.Errorf("failed to create EUR asset account: %w", err)
	}
	fmt.Printf("EUR asset account created: %s (ID: %s)\n", eurAssetAccount.Name, eurAssetAccount.ID)

	eurLiabilityAccount, err := CreateAccount(
		ctx, client, org.ID, ledger.ID,
		"EUR Liability Account", "EUR", "LIABILITY", "eur-liability",
	)
	if err != nil {
		return fmt.Errorf("failed to create EUR liability account: %w", err)
	}
	fmt.Printf("EUR liability account created: %s (ID: %s)\n", eurLiabilityAccount.Name, eurLiabilityAccount.ID)

	fmt.Println("\n=== Workflow completed successfully ===")
	return nil
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
