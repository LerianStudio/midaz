// Package utils provides internal utility functions for the Midaz SDK.
package utils

import (
	"errors"
	"fmt"
	"strings"
)

// ErrorCode represents a standardized error code for Midaz API errors.
type ErrorCode string

// Error code constants
const (
	// CodeValidation indicates a validation error
	CodeValidation ErrorCode = "validation_error"

	// CodeNotFound indicates a resource was not found
	CodeNotFound ErrorCode = "not_found"

	// CodeAuthentication indicates an authentication error
	CodeAuthentication ErrorCode = "authentication_error"

	// CodePermission indicates a permission error
	CodePermission ErrorCode = "permission_error"

	// CodeInsufficientBalance indicates an insufficient balance error
	CodeInsufficientBalance ErrorCode = "insufficient_balance"

	// CodeAccountEligibility indicates an account eligibility error
	CodeAccountEligibility ErrorCode = "account_eligibility_error"

	// CodeAssetMismatch indicates an asset mismatch error
	CodeAssetMismatch ErrorCode = "asset_mismatch"

	// CodeIdempotency indicates an idempotency error
	CodeIdempotency ErrorCode = "idempotency_error"

	// CodeRateLimit indicates a rate limit error
	CodeRateLimit ErrorCode = "rate_limit_exceeded"

	// CodeTimeout indicates a timeout error
	CodeTimeout ErrorCode = "timeout"

	// CodeInternal indicates an internal server error
	CodeInternal ErrorCode = "internal_error"
)

// MidazError represents a structured error from the Midaz API.
// It includes a code and message for more detailed error information.
type MidazError struct {
	// Code is the error code
	Code ErrorCode

	// Message is the error message
	Message string

	// Err is the underlying error
	Err error
}

// Error implements the error interface.
func (e *MidazError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Code, e.Message, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap returns the underlying error.
func (e *MidazError) Unwrap() error {
	return e.Err
}

// NewMidazError creates a new MidazError with the given code and error.
func NewMidazError(code ErrorCode, err error) *MidazError {
	return &MidazError{
		Code:    code,
		Message: err.Error(),
		Err:     err,
	}
}

// Standard error types
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

// Error checking functions
func IsValidationError(err error) bool {
	return errors.Is(err, ErrValidation) || strings.Contains(err.Error(), "validation")
}

func IsInsufficientBalanceError(err error) bool {
	return errors.Is(err, ErrInsufficientBalance) || strings.Contains(err.Error(), "insufficient balance")
}

func IsAccountEligibilityError(err error) bool {
	return errors.Is(err, ErrAccountEligibility) || strings.Contains(err.Error(), "account eligibility")
}

func IsAssetMismatchError(err error) bool {
	return errors.Is(err, ErrAssetMismatch) || strings.Contains(err.Error(), "asset mismatch")
}

func IsAuthenticationError(err error) bool {
	return errors.Is(err, ErrAuthentication) || strings.Contains(err.Error(), "authentication")
}

func IsPermissionError(err error) bool {
	return errors.Is(err, ErrPermission) || strings.Contains(err.Error(), "permission")
}

func IsNotFoundError(err error) bool {
	return errors.Is(err, ErrNotFound) || strings.Contains(err.Error(), "not found")
}

func IsIdempotencyError(err error) bool {
	return errors.Is(err, ErrIdempotency) || strings.Contains(err.Error(), "idempotency")
}

func IsRateLimitError(err error) bool {
	return errors.Is(err, ErrRateLimit) || strings.Contains(err.Error(), "rate limit")
}

func IsTimeoutError(err error) bool {
	return errors.Is(err, ErrTimeout) || strings.Contains(err.Error(), "timeout")
}

func IsInternalError(err error) bool {
	return errors.Is(err, ErrInternal) || strings.Contains(err.Error(), "internal")
}

// FormatTransactionError produces a standardized, user-friendly error message
// for transaction-related errors based on their type.
func FormatTransactionError(err error, operationType string) string {
	if err == nil {
		return ""
	}

	switch {
	case IsValidationError(err):
		return fmt.Sprintf("%s failed: Invalid parameters - %v", operationType, err)
	case IsInsufficientBalanceError(err):
		return fmt.Sprintf("%s failed: Insufficient account balance - %v", operationType, err)
	case IsAccountEligibilityError(err):
		return fmt.Sprintf("%s failed: Account not eligible - %v", operationType, err)
	case IsAssetMismatchError(err):
		return fmt.Sprintf("%s failed: Asset type mismatch - %v", operationType, err)
	case IsAuthenticationError(err):
		return fmt.Sprintf("%s failed: Authentication error - %v", operationType, err)
	case IsPermissionError(err):
		return fmt.Sprintf("%s failed: Permission denied - %v", operationType, err)
	case IsNotFoundError(err):
		return fmt.Sprintf("%s failed: Resource not found - %v", operationType, err)
	case IsIdempotencyError(err):
		return fmt.Sprintf("%s failed: Idempotency issue - %v", operationType, err)
	case IsRateLimitError(err):
		return fmt.Sprintf("%s failed: Rate limit exceeded - %v", operationType, err)
	case IsTimeoutError(err):
		return fmt.Sprintf("%s failed: Operation timed out - %v", operationType, err)
	case IsInternalError(err):
		return fmt.Sprintf("%s failed: Internal server error - %v", operationType, err)
	default:
		return fmt.Sprintf("%s failed: %v", operationType, err)
	}
}

// CategorizeTransactionError provides the error category as a string based on the error type.
// This is useful for logging, analytics, or displaying error category information.
func CategorizeTransactionError(err error) string {
	if err == nil {
		return "none"
	}

	switch {
	case IsValidationError(err):
		return "validation"
	case IsInsufficientBalanceError(err):
		return "insufficient_balance"
	case IsAccountEligibilityError(err):
		return "account_eligibility"
	case IsAssetMismatchError(err):
		return "asset_mismatch"
	case IsAuthenticationError(err):
		return "authentication"
	case IsPermissionError(err):
		return "permission"
	case IsNotFoundError(err):
		return "not_found"
	case IsIdempotencyError(err):
		return "idempotency"
	case IsRateLimitError(err):
		return "rate_limit"
	case IsTimeoutError(err):
		return "timeout"
	case IsInternalError(err):
		return "internal"
	default:
		return "unknown"
	}
}
