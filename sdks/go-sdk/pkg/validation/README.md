# Validation Package

The validation package provides utilities for validating various aspects of Midaz data before sending it to the API. This helps catch errors early, providing immediate feedback and preventing unnecessary API calls with invalid data.

## Usage

Import the package in your Go code:

```go
import "github.com/LerianStudio/midaz/sdks/go-sdk/pkg/validation"
```

## Available Validation Functions

### Transaction Validation

#### `ValidateTransactionDSL`

Validates a transaction DSL input structure before sending it to the API.

```go
func ValidateTransactionDSL(input *models.TransactionDSLInput) error
```

This function checks:
- Required fields are present
- Asset code format is valid
- Transaction amount is positive
- Source and destination accounts are valid
- Asset consistency across accounts
- Metadata format (if present)

#### `ValidateCreateTransactionInput`

Performs comprehensive validation on a standard transaction input.

```go
func ValidateCreateTransactionInput(input *models.CreateTransactionInput) ValidationSummary
```

This function returns a `ValidationSummary` that can contain multiple errors, allowing you to check all validation issues at once.

### Asset Validation

#### `ValidateAssetCode`

Checks if an asset code is valid (3-4 uppercase letters).

```go
func ValidateAssetCode(assetCode string) error
```

#### `ValidateAssetType`

Validates if the asset type is one of the supported types in the Midaz system.

```go
func ValidateAssetType(assetType string) error
```

#### `ValidateCurrencyCode`

Checks if a currency code is valid according to ISO 4217.

```go
func ValidateCurrencyCode(code string) error
```

### Account Validation

#### `ValidateAccountAlias`

Checks if an account alias follows the required format (alphanumeric with optional underscores and hyphens).

```go
func ValidateAccountAlias(alias string) error
```

#### `ValidateAccountType`

Validates if the account type is one of the supported types in the Midaz system.

```go
func ValidateAccountType(accountType string) error
```

#### `GetExternalAccountReference`

Creates a properly formatted external account reference for a given asset code.

```go
func GetExternalAccountReference(assetCode string) string
```

### Other Validations

#### `ValidateTransactionCode`

Checks if a transaction code follows the required format.

```go
func ValidateTransactionCode(code string) error
```

#### `ValidateMetadata`

Validates that transaction metadata contains only supported types and valid values.

```go
func ValidateMetadata(metadata map[string]any) error
```

#### `ValidateDateRange`

Checks if a date range is valid (start date not after end date).

```go
func ValidateDateRange(start, end time.Time) error
```

#### `ValidateCountryCode`

Checks if a country code is valid according to ISO 3166-1 alpha-2.

```go
func ValidateCountryCode(code string) error
```

#### `ValidateAddress`

Validates an address structure for completeness and correctness.

```go
func ValidateAddress(address *Address) error
```

## Working with Validation Summaries

For complex validations that may result in multiple errors, use the `ValidationSummary` type:

```go
// Check if validation passed
if summary.Valid {
    // Proceed with operation
} else {
    // Handle validation errors
    errorMessages := summary.GetErrorMessages() // Get all errors as string slice
    errorSummary := summary.GetErrorSummary()   // Get all errors as a single string
}
```

## Best Practices

1. Always validate inputs before sending them to the API to catch errors early
2. Use `ValidateTransactionDSL` for DSL-style transactions
3. Use `ValidateCreateTransactionInput` for standard transaction inputs
4. Check individual fields with specific validation functions when building forms or UIs
5. For complex validations, check all the errors in the `ValidationSummary` to provide comprehensive feedback
