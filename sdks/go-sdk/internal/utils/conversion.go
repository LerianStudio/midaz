package utils

import (
	"fmt"
	"strings"
	"time"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
)

// FormatAmount converts a numeric amount and scale to a human-readable string representation.
// For example, an amount of 12345 with scale 2 becomes "123.45".
//
// Example:
//
//	formattedAmount := utils.FormatAmount(12345, 2)
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

// ParseAmount converts a human-readable amount string to its numeric representation with scale.
// For example, "123.45" becomes amount=12345, scale=2.
//
// Example:
//
//	amount, scale, err := utils.ParseAmount("123.45")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	// Result: amount=12345, scale=2
func ParseAmount(amountStr string) (amount int64, scale int, err error) {
	// Check for empty string
	if amountStr == "" {
		return 0, 0, fmt.Errorf("amount string cannot be empty")
	}

	// Check if negative
	negative := false

	if strings.HasPrefix(amountStr, "-") {
		negative = true

		amountStr = amountStr[1:]
	}

	// Split by decimal point
	parts := strings.Split(amountStr, ".")

	if len(parts) > 2 {
		return 0, 0, fmt.Errorf("invalid amount format: %s", amountStr)
	}

	// Handle integer part
	var result int64

	_, err = fmt.Sscanf(parts[0], "%d", &result)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid integer part in amount: %s", amountStr)
	}

	// Handle decimal part if exists
	scale = 0
	if len(parts) == 2 {
		scale = len(parts[1])

		// Remove trailing zeros from decimal part for parsing
		decimalPart := strings.TrimRight(parts[1], "0")

		if decimalPart != "" {
			var decimal int64

			_, err = fmt.Sscanf(decimalPart, "%d", &decimal)
			if err != nil {
				return 0, 0, fmt.Errorf("invalid decimal part in amount: %s", amountStr)
			}

			// Multiply integer part appropriately and add decimal
			for i := 0; i < scale; i++ {
				result *= 10
			}

			// Calculate proper power of 10 for the actual digits in decimal part
			decimalMultiplier := 1
			for i := 0; i < scale-len(decimalPart); i++ {
				decimalMultiplier *= 10
			}

			result += decimal * int64(decimalMultiplier)
		} else {
			// All zeros in decimal part, just multiply integer part
			for i := 0; i < scale; i++ {
				result *= 10
			}
		}
	}

	// Apply negative sign if needed
	if negative {
		result = -result
	}

	return result, scale, nil
}

// ConvertToISODate formats a time.Time as an ISO date string (YYYY-MM-DD).
//
// Example:
//
//	isoDate := utils.ConvertToISODate(time.Now())
//	// Result: "2025-04-02"
func ConvertToISODate(t time.Time) string {
	return t.Format("2006-01-02")
}

// ConvertToISODateTime formats a time.Time as an ISO date-time string (YYYY-MM-DDThh:mm:ssZ).
//
// Example:
//
//	isoDateTime := utils.ConvertToISODateTime(time.Now())
//	// Result: "2025-04-02T15:04:05Z"
func ConvertToISODateTime(t time.Time) string {
	return t.Format(time.RFC3339)
}

// ConvertTransactionToSummary creates a user-friendly summary of a transaction.
//
// Example:
//
//	tx := &models.Transaction{
//		ID: "tx_123456",
//		Amount: 10000,
//		Scale: 2,
//		AssetCode: "USD",
//		Status: models.NewStatus("COMPLETED"),
//		Operations: []models.Operation{
//			{
//				Type: "DEBIT",
//				AccountID: "acc_source",
//				AccountAlias: ptr.String("savings"),
//			},
//			{
//				Type: "CREDIT",
//				AccountID: "acc_dest",
//				AccountAlias: ptr.String("checking"),
//			},
//		},
//	}
//	summary := utils.ConvertTransactionToSummary(tx)
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
			statusStr = strings.ToUpper(statusStr[:1]) + statusStr[1:]
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

// ConvertMetadataToTags extracts tags from transaction metadata.
// By convention, tags are stored in metadata as a "tags" key with a comma-separated value.
//
// Example:
//
//	// Extract tags from a transaction's metadata
//	tx := &models.Transaction{
//		ID: "tx_123456",
//		Metadata: map[string]any{
//			"reference": "INV-789",
//			"tags": "payment,recurring,automated",
//		},
//	}
//	tags := utils.ConvertMetadataToTags(tx.Metadata)
//	// Result: []string{"payment", "recurring", "automated"}
func ConvertMetadataToTags(metadata map[string]any) []string {
	if metadata == nil {
		return nil
	}

	// Check if there's a tags field
	tagsValue, ok := metadata["tags"]

	if !ok {
		return nil
	}

	// Convert to string
	tagsStr, ok := tagsValue.(string)

	if !ok {
		return nil
	}

	// Split by comma
	tags := strings.Split(tagsStr, ",")

	// Trim whitespace
	for i, tag := range tags {
		tags[i] = strings.TrimSpace(tag)
	}

	// Filter out empty tags
	result := []string{}

	for _, tag := range tags {
		if tag != "" {
			result = append(result, tag)
		}
	}

	return result
}

// ConvertTagsToMetadata adds tags to transaction metadata.
// By convention, tags are stored in metadata as a "tags" key with a comma-separated value.
//
// Example:
//
//	// Adding tags to a transaction
//	txInput := &models.TransactionDSLInput{
//		Description: "Monthly subscription payment",
//		Metadata: map[string]any{
//			"reference": "INV-123",
//			"customerId": "CUST-456",
//		},
//	}
//	tags := []string{"payment", "recurring", "subscription"}
//	txInput.Metadata = utils.ConvertTagsToMetadata(txInput.Metadata, tags)
//	// txInput.Metadata now contains:
//	// map[string]any{
//	//   "reference": "INV-123",
//	//   "customerId": "CUST-456",
//	//   "tags": "payment,recurring,subscription",
//	// }
func ConvertTagsToMetadata(metadata map[string]any, tags []string) map[string]any {
	if len(tags) == 0 {
		return metadata
	}

	// Create metadata if nil
	if metadata == nil {
		metadata = make(map[string]any)
	}

	// Join tags with comma
	tagsStr := strings.Join(tags, ",")

	// Add to metadata
	metadata["tags"] = tagsStr

	return metadata
}
