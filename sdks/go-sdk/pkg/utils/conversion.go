package utils

import (
	"fmt"
	"strings"
	"time"
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
func ParseAmount(amountStr string) (amount int64, scale int32, err error) {
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
	var scaleInt int = 0
	if len(parts) == 2 {
		scaleInt = len(parts[1])

		var decimalPart int64
		_, err = fmt.Sscanf(parts[1], "%d", &decimalPart)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid decimal part in amount: %s", amountStr)
		}

		// Combine integer and decimal parts
		multiplier := int64(1)
		for i := 0; i < scaleInt; i++ {
			multiplier *= 10
		}

		result = result*multiplier + decimalPart
	}

	// Apply negative sign if needed
	if negative {
		result = -result
	}

	return result, int32(scaleInt), nil
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
