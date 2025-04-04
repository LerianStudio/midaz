# Errors Package

The errors package provides standardized error handling utilities for the Midaz SDK, making it easier to create, check, and format errors consistently.

## Usage

Import the package in your Go code:

```go
import "github.com/LerianStudio/midaz/sdks/go-sdk/pkg/errors"
```

## Error Types

### MidazError

The `MidazError` type represents a structured error from the Midaz API:

```go
type MidazError struct {
    Code    ErrorCode
    Message string
    Err     error
}
```

### ErrorCode

The `ErrorCode` type represents standardized error codes:

```go
type ErrorCode string
```

Available error codes:
- `CodeValidation`: Validation error
- `CodeNotFound`: Resource not found
- `CodeAuthentication`: Authentication error
- `CodePermission`: Permission error
- `CodeInsufficientBalance`: Insufficient balance
- `CodeAccountEligibility`: Account eligibility error
- `CodeAssetMismatch`: Asset mismatch error
- `CodeIdempotency`: Idempotency error
- `CodeRateLimit`: Rate limit exceeded
- `CodeTimeout`: Timeout error
- `CodeInternal`: Internal server error

## Standard Error Variables

The package provides standard error variables for common error cases:

```go
var (
    ErrValidation          = errors.New("validation error")
    ErrInsufficientBalance = errors.New("insufficient balance")
    ErrAccountEligibility  = errors.New("account eligibility error")
    ErrAssetMismatch       = errors.New("asset mismatch")
    ErrAuthentication      = errors.New("authentication error")
    ErrPermission          = errors.New("permission error")
    ErrNotFound            = errors.New("not found")
    ErrIdempotency         = errors.New("idempotency error")
    ErrRateLimit           = errors.New("rate limit exceeded")
    ErrTimeout             = errors.New("timeout")
    ErrInternal            = errors.New("internal error")
)
```

## Creating Errors

### NewMidazError

Creates a new `MidazError` with the given code and error:

```go
func NewMidazError(code ErrorCode, err error) *MidazError
```

Example:
```go
err := errors.NewMidazError(errors.CodeValidation, fmt.Errorf("invalid input"))
```

## Error Checking Functions

The package provides functions to check for specific error types:

```go
func IsValidationError(err error) bool
func IsInsufficientBalanceError(err error) bool
func IsAccountEligibilityError(err error) bool
func IsAssetMismatchError(err error) bool
func IsAuthenticationError(err error) bool
func IsPermissionError(err error) bool
func IsNotFoundError(err error) bool
func IsIdempotencyError(err error) bool
func IsRateLimitError(err error) bool
func IsTimeoutError(err error) bool
func IsInternalError(err error) bool
```

These functions check both for direct equality with the standard error variables and for error messages containing relevant keywords.

## Error Formatting

### FormatTransactionError

Produces a standardized, user-friendly error message for transaction-related errors:

```go
func FormatTransactionError(err error, operationType string) string
```

Example:
```go
message := errors.FormatTransactionError(err, "Transfer")
// Result: "Transfer failed: Invalid parameters - validation error"
```

### CategorizeTransactionError

Provides the error category as a string based on the error type:

```go
func CategorizeTransactionError(err error) string
```

This is useful for logging, analytics, or displaying error category information.

Example:
```go
category := errors.CategorizeTransactionError(err)
// Result: "validation" or "insufficient_balance", etc.
```

## Best Practices

1. Use the standard error variables when possible
2. Use `NewMidazError` to create structured errors with codes
3. Use the error checking functions to handle specific error types
4. Use `FormatTransactionError` to provide consistent user-facing error messages
5. Use `CategorizeTransactionError` for logging and analytics
