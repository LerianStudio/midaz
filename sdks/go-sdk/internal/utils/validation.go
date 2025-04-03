package utils

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/LerianStudio/midaz/sdks/go-sdk/models"
)

// externalAccountPattern is the regex pattern for external account references
var externalAccountPattern = regexp.MustCompile(`^@external/([A-Z]{3,4})$`)

// accountAliasPattern is the regex pattern for account aliases
var accountAliasPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,50}$`)

// assetCodePattern is the regex pattern for asset codes
var assetCodePattern = regexp.MustCompile(`^[A-Z]{3,4}$`)

// ValidateTransactionDSL performs pre-validation of transaction DSL input
// before sending to the API to catch common errors early
func ValidateTransactionDSL(input *models.TransactionDSLInput) error {
	if input == nil {
		return fmt.Errorf("transaction input cannot be nil")
	}

	// Validate Send object
	if input.Send == nil {
		return fmt.Errorf("send object is required")
	}

	// Validate asset code
	if input.Send.Asset == "" {
		return fmt.Errorf("asset code is required")
	}

	if !assetCodePattern.MatchString(input.Send.Asset) {
		return fmt.Errorf("invalid asset code format: %s (must be 3-4 uppercase letters)", input.Send.Asset)
	}

	// Validate amount
	if input.Send.Value <= 0 {
		return fmt.Errorf("transaction amount must be greater than zero")
	}

	// Validate source accounts
	if len(input.Send.Source.From) == 0 {
		return fmt.Errorf("at least one source account is required")
	}

	// Validate destination accounts
	if len(input.Send.Distribute.To) == 0 {
		return fmt.Errorf("at least one destination account is required")
	}

	// Check for asset consistency across all accounts
	if err := validateAssetConsistency(input); err != nil {
		return err
	}

	// Validate external account references
	for _, source := range input.Send.Source.From {
		if err := validateAccountReference(source.Account, input.Send.Asset); err != nil {
			return err
		}
	}

	for _, dest := range input.Send.Distribute.To {
		if err := validateAccountReference(dest.Account, input.Send.Asset); err != nil {
			return err
		}
	}

	return nil
}

// validateAssetConsistency checks that all accounts in the transaction
// are using the same asset code
func validateAssetConsistency(input *models.TransactionDSLInput) error {
	transactionAsset := input.Send.Asset

	// Check source accounts
	for _, source := range input.Send.Source.From {
		if source.Amount != nil && source.Amount.Asset != "" && source.Amount.Asset != transactionAsset {
			return fmt.Errorf(
				"asset mismatch: transaction uses %s but source account %s uses %s",
				transactionAsset, source.Account, source.Amount.Asset)
		}
	}

	// Check destination accounts
	for _, dest := range input.Send.Distribute.To {
		if dest.Amount != nil && dest.Amount.Asset != "" && dest.Amount.Asset != transactionAsset {
			return fmt.Errorf(
				"asset mismatch: transaction uses %s but destination account %s uses %s",
				transactionAsset, dest.Account, dest.Amount.Asset)
		}
	}

	return nil
}

// validateAccountReference checks if an account reference is valid
// for both regular accounts and external accounts
func validateAccountReference(account string, transactionAsset string) error {
	// Check if this is an external account reference
	if strings.HasPrefix(account, "@external/") {
		matches := externalAccountPattern.FindStringSubmatch(account)

		if len(matches) != 2 {
			return fmt.Errorf(
				"invalid external account format: %s, expected format: @external/ASSET",
				account)
		}

		externalAsset := matches[1]

		if externalAsset != transactionAsset {
			return fmt.Errorf(
				"external account asset (%s) must match transaction asset (%s)",
				externalAsset, transactionAsset)
		}
	} else {
		// Regular account alias validation
		if !accountAliasPattern.MatchString(account) {
			return fmt.Errorf(
				"invalid account alias format: %s, expected alphanumeric characters, underscores, and hyphens (max 50 chars)",
				account)
		}
	}

	return nil
}

// GetExternalAccountReference creates a properly formatted external account reference
// for the given asset code
func GetExternalAccountReference(assetCode string) string {
	return "@external/" + assetCode
}

// ValidateAssetCode checks if an asset code is valid.
// Asset codes should be 3-4 uppercase letters (e.g., USD, EUR, BTC).
//
// Example:
//
//	if err := utils.ValidateAssetCode("USD"); err != nil {
//	    log.Fatal(err)
//	}
func ValidateAssetCode(assetCode string) error {
	if assetCode == "" {
		return fmt.Errorf("asset code cannot be empty")
	}

	if !assetCodePattern.MatchString(assetCode) {
		return fmt.Errorf(
			"invalid asset code format: %s (must be 3-4 uppercase letters)",
			assetCode)
	}

	return nil
}

// ValidateAccountAlias checks if an account alias is valid.
// Account aliases should be alphanumeric with optional underscores and hyphens.
//
// Example:
//
//	if err := utils.ValidateAccountAlias("savings_account"); err != nil {
//	    log.Fatal(err)
//	}
func ValidateAccountAlias(alias string) error {
	if alias == "" {
		return fmt.Errorf("account alias cannot be empty")
	}

	if !accountAliasPattern.MatchString(alias) {
		return fmt.Errorf(
			"invalid account alias format: %s, expected alphanumeric characters, underscores, and hyphens (max 50 chars)",
			alias)
	}

	return nil
}

// ValidateTransactionCode checks if a transaction code is valid.
// Transaction codes should be alphanumeric with optional underscores and hyphens.
//
// Example:
//
//	if err := utils.ValidateTransactionCode("TX_123456"); err != nil {
//	    log.Fatal(err)
//	}
func ValidateTransactionCode(code string) error {
	if code == "" {
		return nil // Code is optional
	}

	codePattern := regexp.MustCompile(`^[a-zA-Z0-9_-]{1,50}$`)

	if !codePattern.MatchString(code) {
		return fmt.Errorf(
			"invalid transaction code format: %s, expected alphanumeric characters, underscores, and hyphens (max 50 chars)",
			code)
	}

	return nil
}

// ValidateMetadata checks if transaction metadata is valid.
// This function verifies that metadata values are of supported types.
//
// Example:
//
//	metadata := map[string]any{
//	    "reference": "inv123",
//	    "amount": 100.50,
//	    "customer_id": 12345,
//	}
//	if err := utils.ValidateMetadata(metadata); err != nil {
//	    log.Fatal(err)
//	}
func ValidateMetadata(metadata map[string]any) error {
	if metadata == nil {
		return nil // Metadata is optional
	}

	// Check each metadata entry
	for key, value := range metadata {
		// Validate key format
		if key == "" {
			return fmt.Errorf("metadata keys cannot be empty")
		}

		keyPattern := regexp.MustCompile(`^[a-zA-Z0-9_-]{1,50}$`)

		if !keyPattern.MatchString(key) {
			return fmt.Errorf(
				"invalid metadata key format: %s, expected alphanumeric characters, underscores, and hyphens (max 50 chars)",
				key)
		}

		// Check value type
		switch v := value.(type) {
		case string, int, int32, int64, float32, float64, bool:
			// These types are allowed
		case time.Time:
			// Time is allowed
		case nil:
			// Nil is allowed
		case []any:
			// Arrays are allowed if they contain valid types
			for i, item := range v {
				if !isValidMetadataValueType(item) {
					return fmt.Errorf(
						"invalid metadata value type in array at index %d for key '%s': %T",
						i, key, item)
				}
			}
		case map[string]any:
			// Nested maps are allowed if they contain valid types
			for nestedKey, nestedValue := range v {
				if !isValidMetadataValueType(nestedValue) {
					return fmt.Errorf(
						"invalid metadata value type in nested map for key '%s.%s': %T",
						key, nestedKey, nestedValue)
				}
			}
		default:
			return fmt.Errorf(
				"invalid metadata value type for key '%s': %T, supported types are: string, number, boolean, time, array, and map",
				key, value)
		}
	}

	return nil
}

// isValidMetadataValueType checks if a value is of a type supported in metadata
func isValidMetadataValueType(value any) bool {
	switch value.(type) {
	case string, int, int32, int64, float32, float64, bool, time.Time, nil:
		return true
	default:
		return false
	}
}

// ValidateDateRange checks if a date range is valid.
// The start date must not be after the end date.
//
// Example:
//
//	start := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
//	end := time.Date(2023, 12, 31, 0, 0, 0, 0, time.UTC)
//	if err := utils.ValidateDateRange(start, end); err != nil {
//	    log.Fatal(err)
//	}
func ValidateDateRange(start, end time.Time) error {
	if start.After(end) {
		return fmt.Errorf(
			"invalid date range: start date (%s) is after end date (%s)",
			start.Format("2006-01-02"), end.Format("2006-01-02"))
	}

	return nil
}

// ValidationSummary holds the results of a validation operation
// with multiple potential errors
type ValidationSummary struct {
	// Whether the validation passed
	Valid bool

	// List of validation errors
	Errors []error
}

// AddError adds an error to the validation summary and marks it as invalid
func (v *ValidationSummary) AddError(err error) {
	v.Valid = false

	v.Errors = append(v.Errors, err)
}

// GetErrorMessages returns all error messages as a slice of strings
func (v *ValidationSummary) GetErrorMessages() []string {
	if v.Valid {
		return nil
	}

	messages := make([]string, len(v.Errors))

	for i, err := range v.Errors {
		messages[i] = err.Error()
	}

	return messages
}

// GetErrorSummary returns a single string with all error messages
func (v *ValidationSummary) GetErrorSummary() string {
	if v.Valid {
		return ""
	}

	messages := v.GetErrorMessages()

	if len(messages) == 1 {
		return messages[0]
	}

	summary := fmt.Sprintf("%d validation errors:\n", len(messages))

	for i, msg := range messages {
		summary += fmt.Sprintf("- Error %d: %s\n", i+1, msg)
	}

	return summary
}

// validateOperation validates a single operation in a transaction
func validateOperation(op models.CreateOperationInput, index int, transactionAssetCode string) ([]error, bool) {
	var opErrors []error

	var hasError bool

	// Validate account identifiers
	if op.AccountID == "" && (op.AccountAlias == nil || *op.AccountAlias == "") {
		opErrors = append(opErrors, fmt.Errorf(
			"operation at index %d must have either accountId or accountAlias", index))
		hasError = true
	}

	// Validate operation type
	if op.Type == "" {
		opErrors = append(opErrors, fmt.Errorf(
			"operation at index %d must have a type", index))
		hasError = true
	} else if op.Type != "DEBIT" && op.Type != "CREDIT" {
		opErrors = append(opErrors, fmt.Errorf(
			"operation at index %d has invalid type: %s (must be DEBIT or CREDIT)", index, op.Type))
		hasError = true
	}

	// Validate asset code consistency
	if op.AssetCode != "" && op.AssetCode != transactionAssetCode {
		opErrors = append(opErrors, fmt.Errorf(
			"operation at index %d has asset code %s that doesn't match transaction asset code %s",
			index, op.AssetCode, transactionAssetCode))
		hasError = true
	}

	return opErrors, hasError
}

// validateChartOfAccountsGroupName validates the chart of accounts group name
func validateChartOfAccountsGroupName(name string) error {
	if name == "" {
		return nil
	}

	chartPattern := regexp.MustCompile(`^[a-zA-Z0-9_-]{1,50}$`)
	if !chartPattern.MatchString(name) {
		return fmt.Errorf(
			"invalid chart of accounts group name format: %s, expected alphanumeric characters, underscores, and hyphens (max 50 chars)",
			name)
	}

	return nil
}

// ValidateCreateTransactionInput performs comprehensive validation on a transaction input
// Returns a validation summary with multiple errors if found
//
// Example:
//
//	// Create a transaction input
//	:= &models.CreateTransactionInput{
//		Amount:    10000,
//		Scale:     2,
//		AssetCode: "USD",
//		Operations: []models.CreateOperationInput{
//			{
//				Type:         "DEBIT",
//				AccountID:    "acc_123",
//				AccountAlias: ptr.String("savings"),
//				Amount:       10000,
//			},
//			{
//				Type:         "CREDIT",
//				AccountID:    "acc_456",
//				AccountAlias: ptr.String("checking"),
//				Amount:       10000,
//			},
//		},
//		Metadata: map[string]any{
//			"reference": "TX-123456",
//			"purpose": "Monthly transfer",
//		},
//	}
//
//	// Validate the input
//	summary := utils.ValidateCreateTransactionInput(input)
//	if !summary.Valid {
//		// Handle validation errors
//		fmt.Println(summary.GetErrorSummary())
//		return fmt.Errorf("transaction validation failed: %d errors found", len(summary.Errors))
//	}
//
//	// Proceed with creating the transaction
func ValidateCreateTransactionInput(input *models.CreateTransactionInput) ValidationSummary {
	summary := ValidationSummary{
		Valid:  true,
		Errors: []error{},
	}

	if input == nil {
		summary.AddError(fmt.Errorf("transaction input cannot be nil"))
		return summary
	}

	// Validate asset code
	if err := ValidateAssetCode(input.AssetCode); err != nil {
		summary.AddError(err)
	}

	// Validate amount
	if input.Amount <= 0 {
		summary.AddError(fmt.Errorf("amount must be greater than zero"))
	}

	// Validate operations
	if len(input.Operations) == 0 {
		summary.AddError(fmt.Errorf("at least one operation is required"))
	} else {
		for i, op := range input.Operations {
			opErrors, _ := validateOperation(op, i, input.AssetCode)
			for _, err := range opErrors {
				summary.AddError(err)
			}
		}
	}

	// Validate chart of accounts group name if provided
	if err := validateChartOfAccountsGroupName(input.ChartOfAccountsGroupName); err != nil {
		summary.AddError(err)
	}

	// Validate metadata if provided
	if input.Metadata != nil {
		if err := ValidateMetadata(input.Metadata); err != nil {
			summary.AddError(err)
		}
	}

	return summary
}
