// Package conversion provides utilities for converting between different data formats
// and creating human-readable representations of Midaz SDK models.
package conversion

import (
	"fmt"
	"strings"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
)

// ConvertTransactionToSummary creates a user-friendly summary of a transaction.
//
// Example:
//
//	tx := &models.Transaction{
//	    ID: "tx_123456",
//	    Amount: 10000,
//	    Scale: 2,
//	    AssetCode: "USD",
//	    Status: models.Status{Code: "COMPLETED"},
//	    Operations: []models.Operation{
//	        {
//	            Type: "DEBIT",
//	            AccountID: "acc_source",
//	            AccountAlias: ptr.String("savings"),
//	        },
//	        {
//	            Type: "CREDIT",
//	            AccountID: "acc_dest",
//	            AccountAlias: ptr.String("checking"),
//	        },
//	    },
//	}
//	summary := conversion.ConvertTransactionToSummary(tx)
//	fmt.Println(summary)
//	// Result: "Transfer: 100.00 USD from savings to checking (Completed)"
func ConvertTransactionToSummary(tx *models.Transaction) string {
	if tx == nil {
		return "Invalid transaction: nil"
	}

	// Determine transaction type based on operations
	txType := determineTransactionType(tx)

	// Format amount with proper scale
	amountStr := FormatAmount(tx.Amount, int(tx.Scale))

	// Build basic summary
	summary := fmt.Sprintf("%s: %s %s", txType, amountStr, tx.AssetCode)

	// Add status
	statusStr := "Unknown"

	if tx.Status.Code != "" {
		statusStr = tx.Status.Code
		// Capitalize first letter
		if len(statusStr) > 0 {
			statusStr = strings.ToUpper(statusStr[:1]) + strings.ToLower(statusStr[1:])
		}
	}

	// Add accounts information if available
	if len(tx.Operations) > 0 {
		accountInfo := extractAccountsFromOperations(tx.Operations)

		if accountInfo != "" {
			summary += " " + accountInfo
		}
	}

	// Add status
	summary += fmt.Sprintf(" (%s)", statusStr)

	return summary
}

// determineTransactionType analyzes a transaction to determine its type.
func determineTransactionType(tx *models.Transaction) string {
	// Default type
	txType := "Transaction"

	// Check if we have operations to determine type
	if len(tx.Operations) > 0 {
		// Look for operations with specific patterns
		hasExternal := false
		hasInternal := false

		for _, op := range tx.Operations {
			if op.AccountAlias != nil && strings.HasPrefix(*op.AccountAlias, "@external/") {
				hasExternal = true
			} else {
				hasInternal = true
			}
		}

		// Determine type based on patterns
		if hasExternal && hasInternal {
			// Check first operation to see if it's from external (deposit) or to external (withdrawal)
			if tx.Operations[0].AccountAlias != nil && strings.HasPrefix(*tx.Operations[0].AccountAlias, "@external/") {
				txType = "Deposit"
			} else {
				txType = "Withdrawal"
			}
		} else if hasInternal && !hasExternal {
			txType = "Transfer"
		}
	}

	return txType
}

// extractAccountsFromOperations extracts a summary of the accounts involved in a transaction.
func extractAccountsFromOperations(operations []models.Operation) string {
	if len(operations) == 0 {
		return ""
	}

	fromAccounts := []string{}
	toAccounts := []string{}

	for _, op := range operations {
		// Skip external accounts for cleaner output
		if op.AccountAlias != nil && strings.HasPrefix(*op.AccountAlias, "@external/") {
			continue
		}

		switch op.Type {
		case "DEBIT":
			if op.AccountAlias != nil {
				fromAccounts = append(fromAccounts, *op.AccountAlias)
			} else {
				fromAccounts = append(fromAccounts, op.AccountID)
			}
		case "CREDIT":
			if op.AccountAlias != nil {
				toAccounts = append(toAccounts, *op.AccountAlias)
			} else {
				toAccounts = append(toAccounts, op.AccountID)
			}
		}
	}

	result := ""

	// Format the from accounts
	if len(fromAccounts) > 0 {
		result += "from "

		if len(fromAccounts) == 1 {
			result += fromAccounts[0]
		} else {
			result += fmt.Sprintf("multiple accounts (%d)", len(fromAccounts))
		}
	}

	// Format the to accounts
	if len(toAccounts) > 0 {
		if result != "" {
			result += " "
		}

		result += "to "

		if len(toAccounts) == 1 {
			result += toAccounts[0]
		} else {
			result += fmt.Sprintf("multiple accounts (%d)", len(toAccounts))
		}
	}

	return result
}

// FormatAmount converts a numeric amount and scale to a human-readable string representation.
// For example, an amount of 12345 with scale 2 becomes "123.45".
//
// Example:
//
//	formattedAmount := conversion.FormatAmount(12345, 2)
//	// Result: "123.45"
func FormatAmount(amount int64, scale int) string {
	if scale <= 0 {
		return fmt.Sprintf("%d", amount)
	}

	// Handle negative amounts
	negative := amount < 0

	if negative {
		amount = -amount
	}

	// Convert to string and pad with leading zeros if needed
	amountStr := fmt.Sprintf("%d", amount)
	for len(amountStr) <= scale {
		amountStr = "0" + amountStr
	}

	// Split into whole and decimal parts
	decimalPos := len(amountStr) - scale
	wholePart := amountStr[:decimalPos]

	if wholePart == "" {
		wholePart = "0"
	}

	decimalPart := amountStr[decimalPos:]

	// Combine with decimal point
	result := wholePart + "." + decimalPart

	// Add negative sign if needed
	if negative {
		result = "-" + result
	}

	return result
}
