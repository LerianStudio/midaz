package errors_test

import (
	"errors"
	"testing"

	sdkerrors "github.com/LerianStudio/midaz/sdks/go-sdk/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestMidazError(t *testing.T) {
	t.Run("Error method with underlying error", func(t *testing.T) {
		underlyingErr := errors.New("underlying error")
		midazErr := &sdkerrors.MidazError{
			Code:    sdkerrors.CodeValidation,
			Message: "validation failed",
			Err:     underlyingErr,
		}

		expected := "validation_error: validation failed: underlying error"
		assert.Equal(t, expected, midazErr.Error())
	})

	t.Run("Error method without underlying error", func(t *testing.T) {
		midazErr := &sdkerrors.MidazError{
			Code:    sdkerrors.CodeNotFound,
			Message: "resource not found",
		}

		expected := "not_found: resource not found"
		assert.Equal(t, expected, midazErr.Error())
	})

	t.Run("Unwrap method", func(t *testing.T) {
		underlyingErr := errors.New("underlying error")
		midazErr := &sdkerrors.MidazError{
			Code:    sdkerrors.CodeValidation,
			Message: "validation failed",
			Err:     underlyingErr,
		}

		assert.Equal(t, underlyingErr, midazErr.Unwrap())
	})
}

func TestNewMidazError(t *testing.T) {
	t.Run("Creates error with correct properties", func(t *testing.T) {
		underlyingErr := errors.New("test error")
		midazErr := sdkerrors.NewMidazError(sdkerrors.CodeValidation, underlyingErr)

		assert.Equal(t, sdkerrors.CodeValidation, midazErr.Code)
		assert.Equal(t, "test error", midazErr.Message)
		assert.Equal(t, underlyingErr, midazErr.Err)
	})
}

func TestErrorCheckingFunctions(t *testing.T) {
	testCases := []struct {
		name           string
		err            error
		checkFunc      func(error) bool
		expectedResult bool
	}{
		{
			name:           "IsValidationError with ErrValidation",
			err:            sdkerrors.ErrValidation,
			checkFunc:      sdkerrors.IsValidationError,
			expectedResult: true,
		},
		{
			name:           "IsValidationError with string containing 'validation'",
			err:            errors.New("this is a validation error"),
			checkFunc:      sdkerrors.IsValidationError,
			expectedResult: true,
		},
		{
			name:           "IsValidationError with unrelated error",
			err:            errors.New("unrelated error"),
			checkFunc:      sdkerrors.IsValidationError,
			expectedResult: false,
		},
		{
			name:           "IsInsufficientBalanceError with ErrInsufficientBalance",
			err:            sdkerrors.ErrInsufficientBalance,
			checkFunc:      sdkerrors.IsInsufficientBalanceError,
			expectedResult: true,
		},
		{
			name:           "IsAccountEligibilityError with ErrAccountEligibility",
			err:            sdkerrors.ErrAccountEligibility,
			checkFunc:      sdkerrors.IsAccountEligibilityError,
			expectedResult: true,
		},
		{
			name:           "IsAssetMismatchError with ErrAssetMismatch",
			err:            sdkerrors.ErrAssetMismatch,
			checkFunc:      sdkerrors.IsAssetMismatchError,
			expectedResult: true,
		},
		{
			name:           "IsAuthenticationError with ErrAuthentication",
			err:            sdkerrors.ErrAuthentication,
			checkFunc:      sdkerrors.IsAuthenticationError,
			expectedResult: true,
		},
		{
			name:           "IsPermissionError with ErrPermission",
			err:            sdkerrors.ErrPermission,
			checkFunc:      sdkerrors.IsPermissionError,
			expectedResult: true,
		},
		{
			name:           "IsNotFoundError with ErrNotFound",
			err:            sdkerrors.ErrNotFound,
			checkFunc:      sdkerrors.IsNotFoundError,
			expectedResult: true,
		},
		{
			name:           "IsIdempotencyError with ErrIdempotency",
			err:            sdkerrors.ErrIdempotency,
			checkFunc:      sdkerrors.IsIdempotencyError,
			expectedResult: true,
		},
		{
			name:           "IsRateLimitError with ErrRateLimit",
			err:            sdkerrors.ErrRateLimit,
			checkFunc:      sdkerrors.IsRateLimitError,
			expectedResult: true,
		},
		{
			name:           "IsTimeoutError with ErrTimeout",
			err:            sdkerrors.ErrTimeout,
			checkFunc:      sdkerrors.IsTimeoutError,
			expectedResult: true,
		},
		{
			name:           "IsInternalError with ErrInternal",
			err:            sdkerrors.ErrInternal,
			checkFunc:      sdkerrors.IsInternalError,
			expectedResult: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.checkFunc(tc.err)
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

func TestFormatTransactionError(t *testing.T) {
	testCases := []struct {
		name           string
		err            error
		operationType  string
		expectedResult string
	}{
		{
			name:           "nil error",
			err:            nil,
			operationType:  "Transfer",
			expectedResult: "",
		},
		{
			name:           "validation error",
			err:            sdkerrors.ErrValidation,
			operationType:  "Transfer",
			expectedResult: "Transfer failed: Invalid parameters - validation error",
		},
		{
			name:           "insufficient balance error",
			err:            sdkerrors.ErrInsufficientBalance,
			operationType:  "Withdrawal",
			expectedResult: "Withdrawal failed: Insufficient account balance - insufficient balance",
		},
		{
			name:           "account eligibility error",
			err:            sdkerrors.ErrAccountEligibility,
			operationType:  "Deposit",
			expectedResult: "Deposit failed: Account not eligible - account eligibility error",
		},
		{
			name:           "asset mismatch error",
			err:            sdkerrors.ErrAssetMismatch,
			operationType:  "Exchange",
			expectedResult: "Exchange failed: Asset type mismatch - asset mismatch",
		},
		{
			name:           "authentication error",
			err:            sdkerrors.ErrAuthentication,
			operationType:  "Transfer",
			expectedResult: "Transfer failed: Authentication error - authentication error",
		},
		{
			name:           "permission error",
			err:            sdkerrors.ErrPermission,
			operationType:  "Transfer",
			expectedResult: "Transfer failed: Permission denied - permission error",
		},
		{
			name:           "not found error",
			err:            sdkerrors.ErrNotFound,
			operationType:  "Transfer",
			expectedResult: "Transfer failed: Resource not found - not found",
		},
		{
			name:           "idempotency error",
			err:            sdkerrors.ErrIdempotency,
			operationType:  "Transfer",
			expectedResult: "Transfer failed: Idempotency issue - idempotency error",
		},
		{
			name:           "rate limit error",
			err:            sdkerrors.ErrRateLimit,
			operationType:  "Transfer",
			expectedResult: "Transfer failed: Rate limit exceeded - rate limit exceeded",
		},
		{
			name:           "timeout error",
			err:            sdkerrors.ErrTimeout,
			operationType:  "Transfer",
			expectedResult: "Transfer failed: Operation timed out - timeout",
		},
		{
			name:           "internal error",
			err:            sdkerrors.ErrInternal,
			operationType:  "Transfer",
			expectedResult: "Transfer failed: Internal server error - internal error",
		},
		{
			name:           "unknown error",
			err:            errors.New("unknown error"),
			operationType:  "Transfer",
			expectedResult: "Transfer failed: unknown error",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := sdkerrors.FormatTransactionError(tc.err, tc.operationType)
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}

func TestCategorizeTransactionError(t *testing.T) {
	testCases := []struct {
		name           string
		err            error
		expectedResult string
	}{
		{
			name:           "nil error",
			err:            nil,
			expectedResult: "none",
		},
		{
			name:           "validation error",
			err:            sdkerrors.ErrValidation,
			expectedResult: "validation",
		},
		{
			name:           "insufficient balance error",
			err:            sdkerrors.ErrInsufficientBalance,
			expectedResult: "insufficient_balance",
		},
		{
			name:           "account eligibility error",
			err:            sdkerrors.ErrAccountEligibility,
			expectedResult: "account_eligibility",
		},
		{
			name:           "asset mismatch error",
			err:            sdkerrors.ErrAssetMismatch,
			expectedResult: "asset_mismatch",
		},
		{
			name:           "authentication error",
			err:            sdkerrors.ErrAuthentication,
			expectedResult: "authentication",
		},
		{
			name:           "permission error",
			err:            sdkerrors.ErrPermission,
			expectedResult: "permission",
		},
		{
			name:           "not found error",
			err:            sdkerrors.ErrNotFound,
			expectedResult: "not_found",
		},
		{
			name:           "idempotency error",
			err:            sdkerrors.ErrIdempotency,
			expectedResult: "idempotency",
		},
		{
			name:           "rate limit error",
			err:            sdkerrors.ErrRateLimit,
			expectedResult: "rate_limit",
		},
		{
			name:           "timeout error",
			err:            sdkerrors.ErrTimeout,
			expectedResult: "timeout",
		},
		{
			name:           "internal error",
			err:            sdkerrors.ErrInternal,
			expectedResult: "internal",
		},
		{
			name:           "unknown error",
			err:            errors.New("unknown error"),
			expectedResult: "unknown",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := sdkerrors.CategorizeTransactionError(tc.err)
			assert.Equal(t, tc.expectedResult, result)
		})
	}
}
