// Package validation provides validation utilities for the Midaz SDK.
//
// This package contains functions for validating various aspects of Midaz data:
// - Transaction validation (DSL, standard inputs)
// - Asset code and type validation
// - Account alias and type validation
// - Metadata validation
// - Address validation
// - Date range validation
//
// These utilities help ensure that data is valid before sending it to the API,
// providing early feedback and preventing unnecessary API calls with invalid data.
package validation

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/LerianStudio/lib-commons/commons"
)

// externalAccountPattern is the regex pattern for external account references
var externalAccountPattern = regexp.MustCompile(`^@external/([A-Z]{3,4})$`)

// accountAliasPattern is the regex pattern for account aliases
var accountAliasPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,50}$`)

// assetCodePattern is the regex pattern for asset codes
var assetCodePattern = regexp.MustCompile(`^[A-Z]{3,4}$`)

// TransactionDSLValidator defines an interface for transaction DSL validation
type TransactionDSLValidator interface {
	GetAsset() string
	GetValue() float64
	GetSourceAccounts() []AccountReference
	GetDestinationAccounts() []AccountReference
	GetMetadata() map[string]any
}

// AccountReference defines an interface for account references in transactions
type AccountReference interface {
	GetAccount() string
}

// ValidateTransactionDSL performs pre-validation of transaction DSL input
// before sending to the API to catch common errors early
func ValidateTransactionDSL(input TransactionDSLValidator) error {
	if input == nil {
		return fmt.Errorf("transaction input cannot be nil")
	}

	// Validate asset code
	asset := input.GetAsset()
	if asset == "" {
		return fmt.Errorf("asset code is required")
	}

	if !assetCodePattern.MatchString(asset) {
		return fmt.Errorf("invalid asset code format: %s (must be 3-4 uppercase letters)", asset)
	}

	// Validate amount
	if input.GetValue() <= 0 {
		return fmt.Errorf("transaction amount must be greater than zero")
	}

	// Validate source accounts
	sourceAccounts := input.GetSourceAccounts()
	if len(sourceAccounts) == 0 {
		return fmt.Errorf("at least one source account is required")
	}

	for i, account := range sourceAccounts {
		if err := validateAccountReference(account.GetAccount(), asset); err != nil {
			return fmt.Errorf("invalid source account at index %d: %w", i, err)
		}
	}

	// Validate destination accounts
	destAccounts := input.GetDestinationAccounts()
	if len(destAccounts) == 0 {
		return fmt.Errorf("at least one destination account is required")
	}

	for i, account := range destAccounts {
		if err := validateAccountReference(account.GetAccount(), asset); err != nil {
			return fmt.Errorf("invalid destination account at index %d: %w", i, err)
		}
	}

	// Validate asset consistency across external accounts
	if err := validateAssetConsistency(input); err != nil {
		return err
	}

	// Validate metadata if present
	metadata := input.GetMetadata()
	if metadata != nil {
		if err := ValidateMetadata(metadata); err != nil {
			return fmt.Errorf("invalid metadata: %w", err)
		}
	}

	return nil
}

// validateAssetConsistency checks that all accounts in the transaction
// are using the same asset code
func validateAssetConsistency(input TransactionDSLValidator) error {
	// Extract asset code from external account references
	for _, account := range input.GetSourceAccounts() {
		matches := externalAccountPattern.FindStringSubmatch(account.GetAccount())
		if len(matches) > 1 {
			externalAsset := matches[1]
			if externalAsset != input.GetAsset() {
				return fmt.Errorf("asset code mismatch: transaction uses %s but external account uses %s",
					input.GetAsset(), externalAsset)
			}
		}
	}

	for _, account := range input.GetDestinationAccounts() {
		matches := externalAccountPattern.FindStringSubmatch(account.GetAccount())
		if len(matches) > 1 {
			externalAsset := matches[1]
			if externalAsset != input.GetAsset() {
				return fmt.Errorf("asset code mismatch: transaction uses %s but external account uses %s",
					input.GetAsset(), externalAsset)
			}
		}
	}

	return nil
}

// validateAccountReference checks if an account reference is valid
// for both regular accounts and external accounts
func validateAccountReference(account string, transactionAsset string) error {
	if account == "" {
		return fmt.Errorf("account reference cannot be empty")
	}

	// Check if it's an external account reference
	if strings.HasPrefix(account, "@external/") {
		// First check if it matches our expected pattern
		matches := externalAccountPattern.FindStringSubmatch(account)
		if len(matches) == 0 {
			return fmt.Errorf("invalid external account format: %s (must be @external/XXX where XXX is a valid asset code)", account)
		}

		externalAsset := matches[1]
		// Validate the external asset code format
		if !assetCodePattern.MatchString(externalAsset) {
			return fmt.Errorf("invalid asset code in external account: %s (must be 3-4 uppercase letters)", externalAsset)
		}

		// Validate that the external asset matches the transaction asset
		if externalAsset != transactionAsset {
			return fmt.Errorf("external account asset (%s) must match transaction asset (%s)",
				externalAsset, transactionAsset)
		}
	}

	return nil
}

// GetExternalAccountReference creates a properly formatted external account reference
// for the given asset code
func GetExternalAccountReference(assetCode string) string {
	return fmt.Sprintf("@external/%s", assetCode)
}

// ValidateAssetCode checks if an asset code is valid.
// Asset codes should be 3-4 uppercase letters (e.g., USD, EUR, BTC).
//
// Example:
//
//	if err := validation.ValidateAssetCode("USD"); err != nil {
//	    log.Fatal(err)
//	}
func ValidateAssetCode(assetCode string) error {
	if assetCode == "" {
		return fmt.Errorf("asset code cannot be empty")
	}

	if !assetCodePattern.MatchString(assetCode) {
		return fmt.Errorf("invalid asset code format: %s (must be 3-4 uppercase letters)", assetCode)
	}

	return nil
}

// ValidateAccountAlias checks if an account alias is valid.
// Account aliases should be alphanumeric with optional underscores and hyphens.
//
// Example:
//
//	if err := validation.ValidateAccountAlias("savings_account"); err != nil {
//	    log.Fatal(err)
//	}
func ValidateAccountAlias(alias string) error {
	if alias == "" {
		return fmt.Errorf("account alias cannot be empty")
	}

	if !accountAliasPattern.MatchString(alias) {
		return fmt.Errorf("invalid account alias format: %s (must be alphanumeric with optional underscores and hyphens, max 50 chars)", alias)
	}

	return nil
}

// ValidateTransactionCode checks if a transaction code is valid.
// Transaction codes should be alphanumeric with optional underscores and hyphens.
//
// Example:
//
//	if err := validation.ValidateTransactionCode("TX_123456"); err != nil {
//	    log.Fatal(err)
//	}
func ValidateTransactionCode(code string) error {
	if code == "" {
		return fmt.Errorf("transaction code cannot be empty")
	}

	// Use the same pattern as account alias for now
	if !accountAliasPattern.MatchString(code) {
		return fmt.Errorf("invalid transaction code format: %s (must be alphanumeric with optional underscores and hyphens, max 50 chars)", code)
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
//	if err := validation.ValidateMetadata(metadata); err != nil {
//	    log.Fatal(err)
//	}
func ValidateMetadata(metadata map[string]any) error {
	if metadata == nil {
		return nil
	}

	// Check metadata key and value constraints
	for key, value := range metadata {
		// Validate key
		if key == "" {
			return fmt.Errorf("metadata key cannot be empty")
		}

		if len(key) > 64 {
			return fmt.Errorf("metadata key '%s' exceeds maximum length of 64 characters", key)
		}

		// Validate value type
		if !isValidMetadataValueType(value) {
			return fmt.Errorf("metadata value for key '%s' has unsupported type: %T (supported types: string, bool, int, float64, nil)", key, value)
		}

		// Check string value length
		if strValue, ok := value.(string); ok {
			if len(strValue) > 256 {
				return fmt.Errorf("metadata string value for key '%s' exceeds maximum length of 256 characters", key)
			}
		}

		// Check numeric value range
		switch v := value.(type) {
		case int:
			if v < -9999999999 || v > 9999999999 {
				return fmt.Errorf("metadata integer value for key '%s' is outside allowed range (-9999999999 to 9999999999)", key)
			}
		case float64:
			if v < -9999999999.0 || v > 9999999999.0 {
				return fmt.Errorf("metadata float value for key '%s' is outside allowed range (-9999999999.0 to 9999999999.0)", key)
			}
		}
	}

	// Check total metadata size (approximate)
	totalSize := 0
	for key, value := range metadata {
		totalSize += len(key)
		switch v := value.(type) {
		case string:
			totalSize += len(v)
		case bool, int, float64:
			totalSize += 8 // Approximate size for these types
		}
	}

	if totalSize > 4096 {
		return fmt.Errorf("total metadata size exceeds maximum allowed size of 4KB")
	}

	return nil
}

// isValidMetadataValueType checks if a value is of a type supported in metadata
func isValidMetadataValueType(value any) bool {
	switch value.(type) {
	case string, bool, int, float64, nil:
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
//	if err := validation.ValidateDateRange(start, end); err != nil {
//	    log.Fatal(err)
//	}
func ValidateDateRange(start, end time.Time) error {
	// Check if either date is zero
	if start.IsZero() {
		return fmt.Errorf("start date cannot be empty")
	}

	if end.IsZero() {
		return fmt.Errorf("end date cannot be empty")
	}

	// Check if start date is after end date
	if start.After(end) {
		return fmt.Errorf("start date (%s) cannot be after end date (%s)",
			start.Format("2006-01-02"), end.Format("2006-01-02"))
	}

	return nil
}

// ValidationSummary holds the results of a validation operation
// with multiple potential errors
type ValidationSummary struct {
	Valid  bool
	Errors []error
}

// AddError adds an error to the validation summary and marks it as invalid
func (vs *ValidationSummary) AddError(err error) {
	vs.Valid = false
	vs.Errors = append(vs.Errors, err)
}

// GetErrorMessages returns all error messages as a slice of strings
func (vs *ValidationSummary) GetErrorMessages() []string {
	if vs.Valid {
		return nil
	}

	messages := make([]string, len(vs.Errors))
	for i, err := range vs.Errors {
		messages[i] = err.Error()
	}

	return messages
}

// GetErrorSummary returns a single string with all error messages
func (vs *ValidationSummary) GetErrorSummary() string {
	if vs.Valid {
		return ""
	}

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Validation failed with %d errors:\n", len(vs.Errors)))

	for i, err := range vs.Errors {
		builder.WriteString(fmt.Sprintf("%d. %s\n", i+1, err.Error()))
	}

	return builder.String()
}

// validateOperation validates a single operation in a transaction
func validateOperation(op map[string]any, index int, transactionAssetCode string) ([]error, bool) {
	var errors []error
	valid := true

	// Validate operation type
	if op["type"] == nil {
		errors = append(errors, fmt.Errorf("operation %d: type is required", index))
		valid = false
	} else if op["type"].(string) != "DEBIT" && op["type"].(string) != "CREDIT" {
		errors = append(errors, fmt.Errorf("operation %d: invalid type '%s' (must be DEBIT or CREDIT)", index, op["type"].(string)))
		valid = false
	}

	// Validate account ID
	if op["account_id"] == nil {
		errors = append(errors, fmt.Errorf("operation %d: account ID is required", index))
		valid = false
	}

	// Validate account alias if provided
	if op["account_alias"] != nil && op["account_alias"].(string) != "" {
		if err := ValidateAccountAlias(op["account_alias"].(string)); err != nil {
			errors = append(errors, fmt.Errorf("operation %d: %w", index, err))
			valid = false
		}
	}

	// Validate asset code if provided and ensure it matches transaction asset code
	if op["asset_code"] != nil && op["asset_code"].(string) != "" {
		if op["asset_code"].(string) != transactionAssetCode {
			errors = append(errors, fmt.Errorf("operation %d: asset code '%s' must match transaction asset code '%s'",
				index, op["asset_code"].(string), transactionAssetCode))
			valid = false
		}
	}

	// Validate amount
	if op["amount"].(float64) <= 0 {
		errors = append(errors, fmt.Errorf("operation %d: amount must be greater than zero", index))
		valid = false
	}

	return errors, valid
}

// validateChartOfAccountsGroupName validates the chart of accounts group name
func validateChartOfAccountsGroupName(name string) error {
	if name == "" {
		return fmt.Errorf("chart of accounts group name cannot be empty")
	}

	if len(name) > 100 {
		return fmt.Errorf("chart of accounts group name '%s' exceeds maximum length of 100 characters", name)
	}

	// Allow alphanumeric characters, spaces, underscores, and hyphens
	validPattern := regexp.MustCompile(`^[a-zA-Z0-9 _-]+$`)
	if !validPattern.MatchString(name) {
		return fmt.Errorf("chart of accounts group name '%s' contains invalid characters (allowed: alphanumeric, space, underscore, hyphen)", name)
	}

	return nil
}

// ValidateCreateTransactionInput performs comprehensive validation on a transaction input
// Returns a validation summary with multiple errors if found
//
// Example:
//
//	// Create a transaction input
//	input := map[string]any{
//		"amount": 10000,
//		"scale":  2,
//		"asset_code": "USD",
//		"operations": []map[string]any{
//			{
//				"type":         "DEBIT",
//				"account_id":   "acc_123",
//				"account_alias": "savings",
//				"amount":       10000,
//			},
//			{
//				"type":         "CREDIT",
//				"account_id":   "acc_456",
//				"account_alias": "checking",
//				"amount":       10000,
//			},
//		},
//		"metadata": map[string]any{
//			"reference": "TX-123456",
//			"purpose": "Monthly transfer",
//		},
//	}
//
//	// Validate the input
//	summary := validation.ValidateCreateTransactionInput(input)
//	if !summary.Valid {
//		// Handle validation errors
//		fmt.Println(summary.GetErrorSummary())
//		return fmt.Errorf("transaction validation failed: %d errors found", len(summary.Errors))
//	}
//
//	// Proceed with creating the transaction
func ValidateCreateTransactionInput(input map[string]any) ValidationSummary {
	summary := ValidationSummary{
		Valid:  true,
		Errors: []error{},
	}

	if input == nil {
		summary.AddError(fmt.Errorf("transaction input cannot be nil"))
		return summary
	}

	// Validate asset code
	if input["asset_code"] == nil {
		summary.AddError(fmt.Errorf("asset code is required"))
	} else if err := ValidateAssetCode(input["asset_code"].(string)); err != nil {
		summary.AddError(err)
	}

	// Validate amount
	if input["amount"].(float64) <= 0 {
		summary.AddError(fmt.Errorf("amount must be greater than zero (got %.2f)", input["amount"].(float64)))
	}

	// Validate scale
	if input["scale"].(int) < 0 || input["scale"].(int) > 18 {
		summary.AddError(fmt.Errorf("scale must be between 0 and 18"))
	}

	// Validate operations
	hasOperations := len(input["operations"].([]map[string]any)) > 0
	if !hasOperations {
		summary.AddError(fmt.Errorf("at least one operation is required"))
	} else {
		// Track total debits and credits to ensure they balance
		var totalDebits, totalCredits int64

		// Validate each operation
		for i, op := range input["operations"].([]map[string]any) {
			errors, valid := validateOperation(op, i, input["asset_code"].(string))
			if !valid {
				for _, err := range errors {
					summary.AddError(err)
				}
			}

			// Track totals for balance check
			if op["type"].(string) == "DEBIT" {
				totalDebits += int64(op["amount"].(float64))
			} else if op["type"].(string) == "CREDIT" {
				totalCredits += int64(op["amount"].(float64))
			}
		}

		// Check if debits and credits balance
		if totalDebits != totalCredits {
			summary.AddError(fmt.Errorf("transaction is unbalanced: total debits (%d) do not equal total credits (%d)",
				totalDebits, totalCredits))
		}

		// Check if total matches transaction amount
		if totalDebits != int64(input["amount"].(float64)) {
			summary.AddError(fmt.Errorf("operation amounts do not match transaction amount: operations total (%d) != transaction amount (%.2f)",
				totalDebits, input["amount"].(float64)))
		}
	}

	// Validate chart of accounts group name if provided
	if input["chart_of_accounts_group_name"] != nil && input["chart_of_accounts_group_name"].(string) != "" {
		if err := validateChartOfAccountsGroupName(input["chart_of_accounts_group_name"].(string)); err != nil {
			summary.AddError(err)
		}
	}

	// Validate metadata if present
	if input["metadata"] != nil {
		if err := ValidateMetadata(input["metadata"].(map[string]any)); err != nil {
			summary.AddError(fmt.Errorf("invalid metadata: %w", err))
		}
	}

	return summary
}

// ValidateAssetType validates if the asset type is one of the supported types
// in the Midaz system.
func ValidateAssetType(assetType string) error {
	if assetType == "" {
		return fmt.Errorf("asset type is required")
	}

	// Use commons.ValidateType to ensure consistency with backend APIs
	// Note: commons.ValidateType expects lowercase types, so we convert to lowercase
	if err := commons.ValidateType(strings.ToLower(assetType)); err != nil {
		// Create a list of valid types for the error message
		validTypes := []string{"crypto", "currency", "commodity", "others"}

		return fmt.Errorf("invalid asset type: %s. Valid types are: %s",
			assetType, strings.Join(validTypes, ", "))
	}

	return nil
}

// ValidateAccountType validates if the account type is one of the supported types
// in the Midaz system.
func ValidateAccountType(accountType string) error {
	if accountType == "" {
		return fmt.Errorf("account type is required")
	}

	// Use commons.ValidateAccountType to ensure consistency with backend APIs
	if err := commons.ValidateAccountType(accountType); err != nil {
		// Convert the error to a more user-friendly message
		// Create a list of valid types for the error message
		validTypes := []string{"deposit", "savings", "loans", "marketplace", "creditCard"}

		return fmt.Errorf("invalid account type: %s. Valid types are: %s",
			accountType, strings.Join(validTypes, ", "))
	}

	return nil
}

// ValidateCurrencyCode checks if the currency code is valid according to ISO 4217.
func ValidateCurrencyCode(code string) error {
	if code == "" {
		return fmt.Errorf("currency code cannot be empty")
	}

	// Use commons.ValidateCurrency to ensure consistency with backend APIs
	if err := commons.ValidateCurrency(code); err != nil {
		return fmt.Errorf("invalid currency code: %s", code)
	}

	return nil
}

// ValidateCountryCode checks if the country code is valid according to ISO 3166-1 alpha-2.
func ValidateCountryCode(code string) error {
	if code == "" {
		return fmt.Errorf("country code cannot be empty")
	}

	// Use commons.ValidateCountryAddress to ensure consistency with backend APIs
	if err := commons.ValidateCountryAddress(code); err != nil {
		return fmt.Errorf("invalid country code: %s (must be a valid ISO 3166-1 alpha-2 code)", code)
	}

	return nil
}

// Address is a simplified address structure for validation purposes.
type Address struct {
	Line1   string
	Line2   *string
	ZipCode string
	City    string
	State   string
	Country string
}

// ValidateAddress validates an address structure for completeness and correctness.
func ValidateAddress(address *Address) error {
	if address == nil {
		return fmt.Errorf("address cannot be nil")
	}

	// Validate required fields
	if address.Line1 == "" {
		return fmt.Errorf("address line 1 is required")
	}

	if len(address.Line1) > 256 {
		return fmt.Errorf("address line 1 exceeds maximum length of 256 characters")
	}

	// Validate optional line 2
	if address.Line2 != nil && len(*address.Line2) > 256 {
		return fmt.Errorf("address line 2 exceeds maximum length of 256 characters")
	}

	// Validate zip code
	if address.ZipCode == "" {
		return fmt.Errorf("zip code is required")
	}

	if len(address.ZipCode) > 20 {
		return fmt.Errorf("zip code exceeds maximum length of 20 characters")
	}

	// Validate city
	if address.City == "" {
		return fmt.Errorf("city is required")
	}

	if len(address.City) > 100 {
		return fmt.Errorf("city exceeds maximum length of 100 characters")
	}

	// Validate state
	if address.State == "" {
		return fmt.Errorf("state is required")
	}

	if len(address.State) > 100 {
		return fmt.Errorf("state exceeds maximum length of 100 characters")
	}

	// Validate country
	if address.Country == "" {
		return fmt.Errorf("country is required")
	}

	return ValidateCountryCode(address.Country)
}
