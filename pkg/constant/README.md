# Package constant

## Overview

The `constant` package provides system-wide constant values used throughout the Midaz ledger system. This package serves as a central repository for all constants, ensuring consistency and maintainability across the entire codebase.

## Purpose

This package defines:

- **Error codes**: Standardized error identifiers for consistent error handling
- **HTTP constants**: Path parameters and header names for HTTP operations
- **Account constants**: Account type identifiers and validation patterns
- **Balance constants**: Balance key identifiers
- **Operation constants**: Operation types for double-entry accounting (DEBIT, CREDIT, ON_HOLD, RELEASE)
- **Transaction constants**: Transaction status values and database error codes
- **Pagination constants**: Sort order types for query results
- **Operation route constants**: Account rule types for transaction routing

## Package Structure

```
constant/
├── account.go           # Account-related constants
├── balance.go           # Balance-related constants
├── errors.go            # Error code definitions (0001-0124)
├── http.go              # HTTP path parameters and headers
├── operation.go         # Operation type constants
├── operation-route.go   # Operation route rule types
├── pagination.go        # Pagination and sort order types
├── transaction.go       # Transaction status and DB error codes
└── README.md           # This file
```

## Key Components

### Error Codes (errors.go)

The package defines 124 unique error codes (0001-0124) organized by category:

- **Ledger Errors** (0001-0002): Ledger creation and naming conflicts
- **Asset Errors** (0003-0005, 0028, 0034, 0055-0056): Asset management and validation
- **Account Errors** (0010-0012, 0019-0020, 0029, 0052, 0054, 0062-0064, 0074, 0085, 0096, 0098): Account operations and validation
- **Transaction Errors** (0021, 0023-0027, 0030, 0070-0073, 0087-0091, 0099, 0121-0122): Transaction processing
- **Balance Errors** (0016, 0018, 0025, 0061, 0086, 0092-0093, 0124): Balance operations
- **Authentication Errors** (0041-0045): Token validation and permissions
- **HTTP Errors** (0046-0047, 0053, 0065, 0094): Request validation
- **And more...**

Each error code maps to detailed documentation at: https://docs.midaz.io/midaz/api-reference/resources/errors-list

### Operation Types (operation.go)

Defines the four fundamental operation types in the double-entry accounting system:

- **DEBIT**: Decreases liability/equity/revenue or increases asset/expense accounts
- **CREDIT**: Increases liability/equity/revenue or decreases asset/expense accounts
- **ON_HOLD**: Reserves funds without transferring them
- **RELEASE**: Releases previously held funds

### Transaction Status (transaction.go)

Defines transaction lifecycle states:

- **CREATED**: Initial state after submission
- **APPROVED**: Successfully processed and finalized
- **PENDING**: Awaiting approval or future execution
- **CANCELED**: Rejected or cancelled
- **NOTED**: Recorded for informational purposes only

### HTTP Constants (http.go)

- **UUIDPathParameters**: List of path parameter names that require UUID validation
- **XTotalCount**: Header name for total count in paginated responses
- **ContentLength**: Standard content length header

### Account Constants (account.go)

- **DefaultExternalAccountAliasPrefix**: Prefix for external account aliases (`@external/`)
- **ExternalAccountType**: Type identifier for external accounts
- **AccountAliasAcceptedChars**: Regex pattern for valid alias characters

### Pagination (pagination.go)

- **Order** type: Represents sort order direction
- **Asc**: Ascending order (A-Z, 0-9, oldest to newest)
- **Desc**: Descending order (Z-A, 9-0, newest to oldest)

## Usage Examples

### Using Error Codes

```go
import "github.com/LerianStudio/midaz/v3/pkg/constant"

// Return a standardized error
if ledger == nil {
    return constant.ErrLedgerIDNotFound
}

// Check for specific error
if errors.Is(err, constant.ErrInsufficientFunds) {
    // Handle insufficient funds
}
```

### Using Operation Types

```go
import "github.com/LerianStudio/midaz/v3/pkg/constant"

// Create a debit operation
operation := Operation{
    Type:   constant.DEBIT,
    Amount: 100,
}

// Place funds on hold
holdOperation := Operation{
    Type:   constant.ONHOLD,
    Amount: 50,
}
```

### Using Transaction Status

```go
import "github.com/LerianStudio/midaz/v3/pkg/constant"

// Check transaction status
if transaction.Status == constant.APPROVED {
    // Transaction is finalized
}

// Set initial status
newTransaction.Status = constant.CREATED
```

### Validating Account Aliases

```go
import (
    "regexp"
    "github.com/LerianStudio/midaz/v3/pkg/constant"
)

// Validate account alias format
aliasPattern := regexp.MustCompile(constant.AccountAliasAcceptedChars)
if !aliasPattern.MatchString(alias) {
    return constant.ErrAccountAliasInvalid
}
```

### Using Pagination

```go
import "github.com/LerianStudio/midaz/v3/pkg/constant"

// Set sort order
query := Query{
    SortOrder: constant.Desc,
    SortField: "created_at",
}
```

## Design Principles

1. **Centralization**: All constants are defined in one package to avoid duplication
2. **Immutability**: Constants are immutable by nature, ensuring consistency
3. **Type Safety**: Custom types (like `Order`) provide compile-time type checking
4. **Documentation**: Every constant includes comprehensive documentation
5. **Categorization**: Constants are organized by domain (account, transaction, etc.)
6. **Standardization**: Error codes follow a consistent numeric pattern

## Best Practices

1. **Always use constants**: Never hardcode values that are defined in this package
2. **Import explicitly**: Import the constant package explicitly to make dependencies clear
3. **Check error codes**: Use `errors.Is()` to check for specific error codes
4. **Reference documentation**: For error codes, always refer to the API documentation for complete details
5. **Maintain consistency**: When adding new constants, follow the existing patterns and naming conventions

## Dependencies

This package has minimal dependencies:

- `errors` (standard library): For error creation

## Related Packages

- `pkg/mmodel`: Uses constants for status values and validation
- `pkg/net/http`: Uses HTTP constants for request/response handling
- `components/onboarding`: Uses error codes and entity constants
- `components/transaction`: Uses operation types and transaction status

## Maintenance Notes

When adding new constants:

1. **Error Codes**: Use the next available sequential number (0125, 0126, etc.)
2. **Documentation**: Include comprehensive comments explaining purpose and usage
3. **Categorization**: Group related constants together with section comments
4. **API Documentation**: Update the API documentation at docs.midaz.io when adding error codes
5. **Testing**: Ensure new constants are covered by tests in dependent packages

## Version History

This package follows semantic versioning as part of the Midaz v3 module.
