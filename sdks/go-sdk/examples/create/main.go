// Package main provides examples of creating resources using the Midaz Go SDK.
// It demonstrates a complete workflow from organization creation to transactions.
package main

import (
	"context"
	"fmt"
	"log"
	"math"
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
		options = append(options, midaz.WithAuthToken(authToken))
	} else {
		return fmt.Errorf("MIDAZ_AUTH_TOKEN environment variable is required")
	}

	// Add onboarding URL if specified
	if onboardingURL := os.Getenv("MIDAZ_ONBOARDING_URL"); onboardingURL != "" {
		options = append(options, midaz.WithOnboardingURL(onboardingURL))
	}

	// Add transaction URL if specified
	if transactionURL := os.Getenv("MIDAZ_TRANSACTION_URL"); transactionURL != "" {
		options = append(options, midaz.WithTransactionURL(transactionURL))
	}

	// Add debug mode if specified
	if debugStr := os.Getenv("MIDAZ_DEBUG"); debugStr != "" {
		debug, err := strconv.ParseBool(debugStr)
		if err == nil && debug {
			options = append(options, midaz.WithDebug(true))
		}
	}

	// Add timeout if specified
	if timeoutStr := os.Getenv("MIDAZ_TIMEOUT"); timeoutStr != "" {
		timeout, err := strconv.Atoi(timeoutStr)
		if err == nil && timeout > 0 {
			options = append(options, midaz.WithTimeout(time.Duration(timeout)*time.Second))
		}
	}

	// Enable all API interfaces
	options = append(options, midaz.UseAllAPIs())

	// Create a new Midaz client with options from environment
	client, err := midaz.New(options...)
	if err != nil {
		return fmt.Errorf("failed to create Midaz client: %w", err)
	}

	ctx := context.Background()

	// Step 1: Create an organization
	fmt.Println("Step 1: Creating organization...")
	org, err := CreateOrganization(ctx, client)
	if err != nil {
		return fmt.Errorf("failed to create organization: %w", err)
	}
	fmt.Printf("Organization created: %s (ID: %s)\n\n", org.LegalName, org.ID)

	// Step 2: Create a ledger
	fmt.Println("Step 2: Creating ledger...")
	ledger, err := CreateLedger(ctx, client, org.ID)
	if err != nil {
		return fmt.Errorf("failed to create ledger: %w", err)
	}
	fmt.Printf("Ledger created: %s (ID: %s)\n\n", ledger.Name, ledger.ID)

	// Step 3: Create assets
	fmt.Println("Step 3: Creating assets...")
	usdAsset, err := CreateAsset(ctx, client, org.ID, ledger.ID, "USD", "US Dollar")
	if err != nil {
		return fmt.Errorf("failed to create USD asset: %w", err)
	}
	fmt.Printf("USD asset created: %s (ID: %s)\n", usdAsset.Name, usdAsset.ID)

	eurAsset, err := CreateAsset(ctx, client, org.ID, ledger.ID, "EUR", "Euro")
	if err != nil {
		return fmt.Errorf("failed to create EUR asset: %w", err)
	}
	fmt.Printf("EUR asset created: %s (ID: %s)\n\n", eurAsset.Name, eurAsset.ID)

	// Step 4: Create accounts
	fmt.Println("Step 4: Creating accounts...")

	// Create customer accounts
	customerUsdAccount, err := CreateAccount(
		ctx, client, org.ID, ledger.ID,
		"Customer USD Account", "USD", "ASSET",
		"customer:john:usd",
	)
	if err != nil {
		return fmt.Errorf("failed to create customer USD account: %w", err)
	}
	fmt.Printf("Customer USD account created: %s (ID: %s)\n", customerUsdAccount.Name, customerUsdAccount.ID)

	customerEurAccount, err := CreateAccount(
		ctx, client, org.ID, ledger.ID,
		"Customer EUR Account", "EUR", "ASSET",
		"customer:john:eur",
	)
	if err != nil {
		return fmt.Errorf("failed to create customer EUR account: %w", err)
	}
	fmt.Printf("Customer EUR account created: %s (ID: %s)\n", customerEurAccount.Name, customerEurAccount.ID)

	// Create operational accounts
	operationalUsdAccount, err := CreateAccount(
		ctx, client, org.ID, ledger.ID,
		"Operational USD Account", "USD", "ASSET",
		"operational:usd",
	)
	if err != nil {
		return fmt.Errorf("failed to create operational USD account: %w", err)
	}
	fmt.Printf("Operational USD account created: %s (ID: %s)\n", operationalUsdAccount.Name, operationalUsdAccount.ID)

	operationalEurAccount, err := CreateAccount(
		ctx, client, org.ID, ledger.ID,
		"Operational EUR Account", "EUR", "ASSET",
		"operational:eur",
	)
	if err != nil {
		return fmt.Errorf("failed to create operational EUR account: %w", err)
	}
	fmt.Printf("Operational EUR account created: %s (ID: %s)\n\n", operationalEurAccount.Name, operationalEurAccount.ID)

	// Step 5: Make deposits
	fmt.Println("Step 5: Making deposits...")

	// Deposit to customer USD account
	usdDeposit, err := MakeDeposit(
		ctx, client, org.ID, ledger.ID,
		"customer:john:usd", 10000, 2, "USD",
		"Initial USD deposit",
	)
	if err != nil {
		return fmt.Errorf("failed to make USD deposit: %w", err)
	}
	fmt.Printf("USD deposit created: %s (Amount: $100.00)\n", usdDeposit.ID)

	// Deposit to customer EUR account
	eurDeposit, err := MakeDeposit(
		ctx, client, org.ID, ledger.ID,
		"customer:john:eur", 15000, 2, "EUR",
		"Initial EUR deposit",
	)
	if err != nil {
		return fmt.Errorf("failed to make EUR deposit: %w", err)
	}
	fmt.Printf("EUR deposit created: %s (Amount: €150.00)\n\n", eurDeposit.ID)

	// Wait a moment to ensure deposits are processed
	time.Sleep(1 * time.Second)

	// Step 6: Make transfers
	fmt.Println("Step 6: Making transfers...")

	// Transfer from customer USD account to operational USD account
	usdTransfer, err := MakeTransfer(
		ctx, client, org.ID, ledger.ID,
		"customer:john:usd", "operational:usd",
		5000, 2, "USD",
		"Transfer to operational account",
	)
	if err != nil {
		return fmt.Errorf("failed to make USD transfer: %w", err)
	}
	fmt.Printf("USD transfer created: %s (Amount: $50.00)\n", usdTransfer.ID)

	// Transfer from customer EUR account to operational EUR account
	eurTransfer, err := MakeTransfer(
		ctx, client, org.ID, ledger.ID,
		"customer:john:eur", "operational:eur",
		7500, 2, "EUR",
		"Transfer to operational account",
	)
	if err != nil {
		return fmt.Errorf("failed to make EUR transfer: %w", err)
	}
	fmt.Printf("EUR transfer created: %s (Amount: €75.00)\n\n", eurTransfer.ID)

	// Wait a moment to ensure transfers are processed
	time.Sleep(1 * time.Second)

	// Step 7: Make withdrawals
	fmt.Println("Step 7: Making withdrawals...")

	// Withdrawal from customer USD account
	usdWithdrawal, err := MakeWithdrawal(
		ctx, client, org.ID, ledger.ID,
		"customer:john:usd", 2000, 2, "USD",
		"USD withdrawal",
	)
	if err != nil {
		return fmt.Errorf("failed to make USD withdrawal: %w", err)
	}
	fmt.Printf("USD withdrawal created: %s (Amount: $20.00)\n", usdWithdrawal.ID)

	// Withdrawal from customer EUR account
	eurWithdrawal, err := MakeWithdrawal(
		ctx, client, org.ID, ledger.ID,
		"customer:john:eur", 3000, 2, "EUR",
		"EUR withdrawal",
	)
	if err != nil {
		return fmt.Errorf("failed to make EUR withdrawal: %w", err)
	}
	fmt.Printf("EUR withdrawal created: %s (Amount: €30.00)\n\n", eurWithdrawal.ID)

	// Step 8: Verify final balances
	fmt.Println("Step 8: Verifying final balances...")

	// Get balances for all accounts
	customerUsdBalance, err := GetBalance(ctx, client, org.ID, ledger.ID, "customer:john:usd")
	if err != nil {
		return fmt.Errorf("failed to get customer USD balance: %w", err)
	}
	fmt.Printf("Customer USD balance: $%.2f\n", float64(customerUsdBalance.Available)/math.Pow10(int(customerUsdBalance.Scale)))

	customerEurBalance, err := GetBalance(ctx, client, org.ID, ledger.ID, "customer:john:eur")
	if err != nil {
		return fmt.Errorf("failed to get customer EUR balance: %w", err)
	}
	fmt.Printf("Customer EUR balance: €%.2f\n", float64(customerEurBalance.Available)/math.Pow10(int(customerEurBalance.Scale)))

	operationalUsdBalance, err := GetBalance(ctx, client, org.ID, ledger.ID, "operational:usd")
	if err != nil {
		return fmt.Errorf("failed to get operational USD balance: %w", err)
	}
	fmt.Printf("Operational USD balance: $%.2f\n", float64(operationalUsdBalance.Available)/math.Pow10(int(operationalUsdBalance.Scale)))

	operationalEurBalance, err := GetBalance(ctx, client, org.ID, ledger.ID, "operational:eur")
	if err != nil {
		return fmt.Errorf("failed to get operational EUR balance: %w", err)
	}
	fmt.Printf("Operational EUR balance: €%.2f\n", float64(operationalEurBalance.Available)/math.Pow10(int(operationalEurBalance.Scale)))

	fmt.Println("\nWorkflow completed successfully!")
	return nil
}

// Main function to run the example
func main() {
	// Load .env file if it exists
	if err := godotenv.Load(); err != nil {
		// Try to load from the parent directory if not found in current directory
		if err := godotenv.Load("../../.env"); err != nil {
			log.Println("Warning: .env file not found, using environment variables")
		}
	}

	if err := RunCreateWorkflow(); err != nil {
		log.Fatalf("Error running workflow: %v", err)
	}
}
