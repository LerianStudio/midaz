// Package utils provides utility functions for the Midaz SDK.
//
// This package contains functions for common operations when working with Midaz data:
// - Account management (filtering, finding, summarizing)
// - Amount formatting and parsing
// - Validation of inputs before sending to the API
// - Transaction helpers for common operations
//
// These utilities make it easier to work with the data models and perform
// common tasks without having to write boilerplate code.
package utils

import (
	"fmt"
	"strings"
)

// Account represents a simplified account structure for utility functions.
// This avoids the import cycle with the models package.
type Account struct {
	ID              string
	Name            string
	ParentAccountID *string
	AssetCode       string
	Type            string
	Alias           *string
	Status          Status
}

// Status represents a simplified status structure.
type Status struct {
	Code        string
	Description *string
}

// GetAccountIdentifier returns the best identifier for an account (alias if available, ID otherwise).
// This prevents nil pointer exceptions when dealing with the optional Alias field.
//
// Example:
//
//	account := &utils.Account{ID: "acc_123", Alias: &aliasValue}
//	identifier := utils.GetAccountIdentifier(account) // Returns aliasValue
//
//	accountNoAlias := &utils.Account{ID: "acc_456"} // Alias is nil
//	identifier = utils.GetAccountIdentifier(accountNoAlias) // Returns "acc_456"
func GetAccountIdentifier(account *Account) string {
	if account == nil {
		return ""
	}

	if account.Alias != nil && *account.Alias != "" {
		return *account.Alias
	}

	return account.ID
}

// FindAccountByAlias finds an account with the given alias in a list of accounts.
// Returns nil if no account is found with the given alias.
//
// Example:
//
//	// Search for an account by alias in a list of accounts
//	:= []utils.Account{
//		{
//			ID: "acc_123",
//			Name: "Savings Account",
//			AssetCode: "USD",
//			Alias: ptr.String("savings"),
//			Type: "ASSET",
//			Status: utils.Status{Code: "ACTIVE"},
//		},
//		{
//			ID: "acc_456",
//			Name: "Checking Account",
//			AssetCode: "USD",
//			Alias: ptr.String("checking"),
//			Type: "ASSET",
//			Status: utils.Status{Code: "ACTIVE"},
//		},
//	}
//
//	account := utils.FindAccountByAlias(accounts, "savings")
//	if account == nil {
//	    log.Println("Account not found")
//	} else {
//	    log.Printf("Found account: %s", account.ID) // Prints: Found account: acc_123
//	}
func FindAccountByAlias(accounts []Account, alias string) *Account {
	for i, account := range accounts {
		if account.Alias != nil && *account.Alias == alias {
			return &accounts[i]
		}
	}

	return nil
}

// FindAccountByID finds an account with the given ID in a list of accounts.
// Returns nil if no account is found with the given ID.
//
// Example:
//
//	accounts := []utils.Account{...}
//	account := utils.FindAccountByID(accounts, "acc_123")
//	if account == nil {
//	    log.Println("Account not found")
//	} else {
//	    log.Printf("Found account: %s", account.Alias)
//	}
func FindAccountByID(accounts []Account, id string) *Account {
	for i, account := range accounts {
		if account.ID == id {
			return &accounts[i]
		}
	}

	return nil
}

// FindAccountsByAssetCode finds all accounts with the given asset code in a list of accounts.
// Returns an empty slice if no accounts are found with the given asset code.
//
// Example:
//
//	accounts := []utils.Account{...}
//	usdAccounts := utils.FindAccountsByAssetCode(accounts, "USD")
//	log.Printf("Found %d USD accounts", len(usdAccounts))
func FindAccountsByAssetCode(accounts []Account, assetCode string) []Account {
	var result []Account

	for _, account := range accounts {
		if account.AssetCode == assetCode {
			result = append(result, account)
		}
	}

	return result
}

// FindAccountsByStatus finds all accounts with the given status in a list of accounts.
// Returns an empty slice if no accounts are found with the given status.
//
// Example:
//
//	accounts := []utils.Account{...}
//	activeAccounts := utils.FindAccountsByStatus(accounts, "ACTIVE")
//	log.Printf("Found %d active accounts", len(activeAccounts))
func FindAccountsByStatus(accounts []Account, status string) []Account {
	var result []Account

	for _, account := range accounts {
		if account.Status.Code == status {
			result = append(result, account)
		}
	}

	return result
}

// matchesFilter checks if an account matches a specific filter key and value
func matchesFilter(account Account, key, value string) bool {
	switch strings.ToLower(key) {
	case "assetcode":
		return account.AssetCode == value
	case "status":
		return account.Status.Code == value
	case "type":
		return account.Type == value
	case "aliascontains":
		return account.Alias != nil && strings.Contains(*account.Alias, value)
	case "id":
		return account.ID == value
	case "parentaccountid":
		return account.ParentAccountID != nil && *account.ParentAccountID == value
	default:
		return false
	}
}

// FilterAccounts returns accounts that match all given filter criteria.
// This provides a flexible way to filter accounts by multiple attributes.
//
// Example:
//
//	// Create a list of accounts
//	:= []utils.Account{
//		{
//			ID:        "acc_123",
//			Name:      "USD Savings",
//			AssetCode: "USD",
//			Type:      "ASSET",
//			Status:    utils.Status{Code: "ACTIVE"},
//			Alias:     ptr.String("usd_savings"),
//		},
//		{
//			ID:        "acc_456",
//			Name:      "EUR Checking",
//			AssetCode: "EUR",
//			Type:      "ASSET",
//			Status:    utils.Status{Code: "ACTIVE"},
//			Alias:     ptr.String("eur_checking"),
//		},
//		{
//			ID:        "acc_789",
//			Name:      "USD Frozen Account",
//			AssetCode: "USD",
//			Type:      "ASSET",
//			Status:    utils.Status{Code: "FROZEN"},
//			Alias:     ptr.String("usd_frozen"),
//		},
//	}
//
//	// Find all active USD accounts
//	filtered := utils.FilterAccounts(accounts, map[string]string{
//	    "assetCode": "USD",
//	    "status": "ACTIVE",
//	})
//	log.Printf("Found %d matching accounts", len(filtered)) // Prints: Found 1 matching accounts
func FilterAccounts(accounts []Account, filters map[string]string) []Account {
	if len(filters) == 0 {
		return accounts
	}

	var result []Account

	for _, account := range accounts {
		match := true

		for key, value := range filters {
			if !matchesFilter(account, key, value) {
				match = false
				break
			}
		}

		if match {
			result = append(result, account)
		}
	}

	return result
}

// AccountBalanceSummary holds balance information for an account
type AccountBalanceSummary struct {
	AccountID    string
	AccountAlias string
	AssetCode    string
	Available    int64
	AvailableStr string
	OnHold       int64
	OnHoldStr    string
	Total        int64
	TotalStr     string
	Scale        int
}

// Balance represents a simplified balance structure for utility functions.
type Balance struct {
	ID        string
	AccountID string
	AssetCode string
	Available int64
	OnHold    int64
	Scale     int32
}

// GetAccountBalanceSummary creates a human-readable balance summary for an account.
// This is useful for displaying account balances in a user interface.
//
// Example:
//
//	// Create account and balance objects
//	account := &utils.Account{
//		ID:        "acc_123",
//		Name:      "Savings Account",
//		AssetCode: "USD",
//		Alias:     ptr.String("savings"),
//	}
//
//	balance := &utils.Balance{
//		ID:        "bal_456",
//		AccountID: "acc_123",
//		AssetCode: "USD",
//		Available: 10000,  // $100.00
//		OnHold:    500,    // $5.00
//		Scale:     2,
//	}
//
//	summary, err := utils.GetAccountBalanceSummary(account, balance)
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	log.Printf("Account %s has available balance: %s",
//		summary.AccountAlias, summary.AvailableStr) // Prints: Account savings has available balance: 100.00
func GetAccountBalanceSummary(account *Account, balance *Balance) (AccountBalanceSummary, error) {
	summary := AccountBalanceSummary{
		Scale: int(balance.Scale),
	}

	// Get account information
	if account == nil {
		return summary, fmt.Errorf("account cannot be nil")
	}

	summary.AccountID = account.ID
	if account.Alias != nil {
		summary.AccountAlias = *account.Alias
	}

	summary.AssetCode = balance.AssetCode

	// Set balance values
	summary.Available = balance.Available
	summary.OnHold = balance.OnHold
	summary.Total = balance.Available + balance.OnHold

	// Format balance strings
	summary.AvailableStr = FormatAmount(balance.Available, int(balance.Scale))
	summary.OnHoldStr = FormatAmount(balance.OnHold, int(balance.Scale))
	summary.TotalStr = FormatAmount(balance.Available+balance.OnHold, int(balance.Scale))

	return summary, nil
}

// FormatAccountSummary returns a formatted summary string for an account.
// This is useful for displaying account information in a user interface.
//
// Example:
//
//	account := &utils.Account{...}
//	summary := utils.FormatAccountSummary(account)
//	log.Println(summary)
//	// Result: "Account: savings (acc_123) - Type: ASSET - Asset: USD - Status: ACTIVE"
func FormatAccountSummary(account *Account) string {
	if account == nil {
		return "Account: <nil>"
	}

	// Build alias part
	aliasStr := "<no alias>"

	if account.Alias != nil && *account.Alias != "" {
		aliasStr = *account.Alias
	}

	// Build status part
	statusStr := "<no status>"

	if account.Status.Code != "" {
		statusStr = account.Status.Code
	}

	// Format summary
	return fmt.Sprintf("Account: %s (%s) - Type: %s - Asset: %s - Status: %s",
		aliasStr, account.ID, account.Type, account.AssetCode, statusStr)
}
